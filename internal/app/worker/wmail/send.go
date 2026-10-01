package wmail

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/worker/sendauth"
	"github.com/warmbly/warmbly/internal/client/goog"
	"github.com/warmbly/warmbly/internal/client/msgraph"
	"github.com/warmbly/warmbly/internal/client/smtpimap/smtp"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Attachment is a fully-resolved attachment ready to be MIME-encoded: the
// worker has already fetched Data from object storage.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// SendRequest contains all parameters needed to send an email
type SendRequest struct {
	TaskID      uuid.UUID
	EmailID     uuid.UUID
	OrgID       uuid.UUID
	WorkerID    uuid.UUID
	From        string
	Provider    models.InboxProvider
	Authorizer  sendauth.Authorizer
	To          []string
	Cc          []string
	Bcc         []string
	MessageID   string
	Subject     string
	BodyPlain   string
	BodyHTML    string
	InReplyTo   string
	Parent      *models.EmailParent
	IsWarmup    bool
	WarmupToken string
	// UnsubscribeURL, when set (campaign sends with the unsubscribe header
	// enabled), produces RFC 8058 one-click unsubscribe headers.
	UnsubscribeURL string
	// Attachments, when present, are encoded as multipart/mixed parts after the
	// multipart/alternative text body. Warmup sends never carry attachments.
	Attachments []Attachment
}

// buildSendHeaders assembles the outbound custom headers: the warmup
// verification token (warmup sends) and RFC 8058 one-click unsubscribe headers
// (campaign sends). Returns nil when there are none so callers can branch.
func buildSendHeaders(req *SendRequest) map[string]string {
	h := map[string]string{}
	if req.WarmupToken != "" {
		h[config.WarmupVerifyHeader] = req.WarmupToken
	}
	if req.UnsubscribeURL != "" {
		// RFC 8058: the HTTPS URI in List-Unsubscribe plus the one-click marker
		// tells Gmail/Yahoo/Microsoft to POST List-Unsubscribe=One-Click here.
		h["List-Unsubscribe"] = "<" + req.UnsubscribeURL + ">"
		h["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

// SendResult contains the result of a send operation
type SendResult struct {
	Success           bool
	ProviderAttempted bool
	MessageID         string
	ProviderMsgID     string
	SentAt            time.Time
	Error             *errx.MailError
}

const maxSendRetries = 3

// Send attempts to send an email with retry for transient failures
func (w *WMail) Send(ctx context.Context, req *SendRequest) *SendResult {
	if req == nil {
		return deniedSendResult()
	}
	// For warmup emails, ensure HTML is empty
	bodyHTML := req.BodyHTML
	if req.IsWarmup {
		bodyHTML = ""
	}

	var result *SendResult
	providerAttempted := false
	for attempt := 0; attempt <= maxSendRetries; attempt++ {
		result = &SendResult{Success: false, SentAt: time.Now()}
		// Never reuse a decision across retries.
		if w == nil || req.Authorizer == nil || req.EmailID != w.ID ||
			!strings.EqualFold(req.From, w.Email) || req.Provider != w.EmailType ||
			req.Authorizer.Authorize(ctx, sendauth.Request{
				TaskID: req.TaskID, EmailID: req.EmailID, OrgID: req.OrgID,
				MessageID: req.MessageID, WorkerID: req.WorkerID,
				From: req.From, Provider: req.Provider, IsWarmup: req.IsWarmup,
			}) != nil {
			return deniedSendResult(providerAttempted)
		}

		providerAttempted = true
		if w.sendAttempt != nil {
			result = w.sendAttempt(ctx, req, bodyHTML)
		} else {
			switch w.EmailType {
			case models.InboxProviderGoogle:
				result = w.sendViaGmail(ctx, req, bodyHTML)
			case models.InboxProviderOutlook:
				result = w.sendViaGraph(ctx, req, bodyHTML)
			case models.InboxProviderSMTPIMAP:
				result = w.sendViaSMTP(ctx, req, bodyHTML)
			default:
				result.Error = errx.MError(
					errx.MailErrorCritical,
					errx.MailErrorCodeUnsupported,
					"Unsupported email provider",
					errx.MailErrorResolveMethodNone,
				)
				return result
			}

		}
		if result.Success {
			result.ProviderAttempted = true
			return result
		}
		result.ProviderAttempted = true

		// Graph sendMail can accept a request even when its response is lost.
		// Neither a transient error nor a fresh authorization check proves
		// non-acceptance; a second attempt could deliver the same email twice.
		// This includes app-only shared mailboxes (the same Graph transport).
		if w.EmailType == models.InboxProviderOutlook {
			return result
		}

		// Don't retry critical/auth errors - only transient ones
		if result.Error != nil && result.Error.Type == errx.MailErrorCritical {
			return result
		}

		if attempt < maxSendRetries {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 1s, 2s, 4s
			select {
			case <-ctx.Done():
				return result
			case <-time.After(backoff):
			}
		}
	}

	return result
}

func deniedSendResult(providerAttempted ...bool) *SendResult {
	attempted := len(providerAttempted) > 0 && providerAttempted[0]
	code := errx.MailErrorCodeInternalSendAuthorization
	if attempted {
		code = errx.MailErrorCodeInternalSendAuthorizationAfterAttempt
	}
	return &SendResult{SentAt: time.Now(), ProviderAttempted: attempted, Error: errx.MError(
		errx.MailErrorCritical, code,
		"Internal send authorization denied or unavailable", errx.MailErrorResolveMethodNone,
	)}
}

// sendViaGmail sends an email using the Gmail API
func (w *WMail) sendViaGmail(ctx context.Context, req *SendRequest, bodyHTML string) *SendResult {
	result := &SendResult{
		Success: false,
		SentAt:  time.Now(),
	}

	if w.GoogleData == nil || w.GoogleData.Client == nil {
		result.Error = errx.MError(
			errx.MailErrorCritical,
			errx.MailErrorCodeAuthenticationFailed,
			"Gmail client not initialized",
			errx.MailErrorResolveMethodReload,
		)
		return result
	}

	// Build parent reference for replies. Warmup replies often only have the
	// RFC Message-ID from the token flow, not a local provider thread record.
	var parent *models.EmailMessageData
	if req.InReplyTo != "" && req.Parent != nil {
		parent = &models.EmailMessageData{
			MessageID: req.Parent.MessageID,
			ThreadID:  req.Parent.ThreadID,
		}
	} else if req.InReplyTo != "" {
		parent = &models.EmailMessageData{
			MessageID: strings.Trim(req.InReplyTo, "<>"),
		}
	}

	// Build custom headers (warmup token + RFC 8058 one-click unsubscribe).
	customHeaders := buildSendHeaders(req)

	// Convert resolved attachments to the goog transport shape (warmup sends
	// carry none, but threading req.Attachments is harmless when empty).
	attachments := toGoogAttachments(req.Attachments)

	// Send via Gmail API
	gmailMsg, err := w.GoogleData.Client.SendMessage(
		ctx,
		req.To,
		req.Cc,
		req.Bcc,
		req.MessageID,
		req.Subject,
		req.BodyPlain,
		bodyHTML,
		parent,
		attachments,
		customHeaders,
	)
	if err != nil {
		// Convert to MailError using goog.HandleError
		if mailErr := goog.HandleError(err); mailErr != nil {
			result.Error = mailErr
		} else {
			// Generic error
			result.Error = errx.MError(
				errx.MailErrorWarning,
				errx.MailErrorCodeServerUnreachable,
				err.Error(),
				errx.MailErrorResolveMethodRetry,
			)
		}
		return result
	}

	result.Success = true
	result.MessageID = req.MessageID
	result.ProviderMsgID = gmailMsg.Id
	return result
}

// sendViaGraph sends an email through Microsoft Graph (RAW MIME sendMail).
// sendMail returns 202 with no provider id, so ProviderMsgID is the RFC
// Message-ID we minted, mirroring the SMTP path.
func (w *WMail) sendViaGraph(ctx context.Context, req *SendRequest, bodyHTML string) *SendResult {
	result := &SendResult{
		Success: false,
		SentAt:  time.Now(),
	}

	if w.GraphData == nil || w.GraphData.Client == nil {
		result.Error = errx.MError(
			errx.MailErrorCritical,
			errx.MailErrorCodeAuthenticationFailed,
			"Microsoft Graph client not initialized",
			errx.MailErrorResolveMethodReload,
		)
		return result
	}

	// Build parent reference for replies. Warmup replies often only have the RFC
	// Message-ID from the token flow, not a local provider thread record.
	var parent *models.EmailMessageData
	if req.InReplyTo != "" && req.Parent != nil {
		parent = &models.EmailMessageData{
			MessageID: req.Parent.MessageID,
			ThreadID:  req.Parent.ThreadID,
		}
	} else if req.InReplyTo != "" {
		parent = &models.EmailMessageData{
			MessageID: strings.Trim(req.InReplyTo, "<>"),
		}
	}

	// Warmup token + RFC 8058 one-click unsubscribe headers; RAW MIME carries
	// them verbatim (the JSON message shape cannot).
	customHeaders := buildSendHeaders(req)
	attachments := toGraphAttachments(req.Attachments)

	err := w.GraphData.Client.SendMessage(
		ctx,
		req.To,
		req.Cc,
		req.Bcc,
		req.MessageID,
		req.Subject,
		req.BodyPlain,
		bodyHTML,
		parent,
		attachments,
		customHeaders,
	)
	if err != nil {
		var mailErr *errx.MailError
		if errors.As(err, &mailErr) {
			result.Error = mailErr
		} else {
			result.Error = errx.MError(
				errx.MailErrorWarning,
				errx.MailErrorCodeServerUnreachable,
				err.Error(),
				errx.MailErrorResolveMethodRetry,
			)
		}
		return result
	}

	result.Success = true
	result.MessageID = req.MessageID
	result.ProviderMsgID = req.MessageID // Graph sendMail returns no id
	return result
}

// sendViaSMTP sends an email using SMTP
func (w *WMail) sendViaSMTP(ctx context.Context, req *SendRequest, bodyHTML string) *SendResult {
	result := &SendResult{
		Success: false,
		SentAt:  time.Now(),
	}

	if w.SmtpImapData == nil || w.SmtpImapData.SmtpClient == nil {
		result.Error = errx.MError(
			errx.MailErrorCritical,
			errx.MailErrorCodeAuthenticationFailed,
			"SMTP client not initialized",
			errx.MailErrorResolveMethodReload,
		)
		return result
	}

	// Build custom headers (warmup token + RFC 8058 one-click unsubscribe).
	smtpCustomHeaders := buildSendHeaders(req)

	// Convert resolved attachments to the SMTP transport shape.
	smtpAttachments := toSMTPAttachments(req.Attachments)

	// Send via SMTP. Attachments are passed explicitly (not variadic) so an
	// empty list still selects the same code path.
	merr := w.SmtpImapData.SmtpClient.Send(
		ctx,
		req.To,
		req.Cc,
		req.Bcc,
		req.MessageID,
		req.Subject,
		req.BodyPlain,
		bodyHTML,
		req.InReplyTo,
		smtpAttachments,
		smtpCustomHeaders,
	)
	if merr != nil {
		result.Error = merr
		return result
	}

	result.Success = true
	result.MessageID = req.MessageID
	result.ProviderMsgID = req.MessageID // SMTP uses the same message ID
	return result
}

// toGoogAttachments maps wmail attachments to the Gmail transport shape.
func toGoogAttachments(in []Attachment) []goog.Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]goog.Attachment, 0, len(in))
	for _, a := range in {
		out = append(out, goog.Attachment{
			Filename: a.Filename,
			MimeType: a.MimeType,
			Data:     a.Data,
		})
	}
	return out
}

// toGraphAttachments maps wmail attachments to the Graph transport shape.
func toGraphAttachments(in []Attachment) []msgraph.Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]msgraph.Attachment, 0, len(in))
	for _, a := range in {
		out = append(out, msgraph.Attachment{
			Filename: a.Filename,
			MimeType: a.MimeType,
			Data:     a.Data,
		})
	}
	return out
}

// toSMTPAttachments maps wmail attachments to the SMTP transport shape.
func toSMTPAttachments(in []Attachment) []smtp.Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]smtp.Attachment, 0, len(in))
	for _, a := range in {
		out = append(out, smtp.Attachment{
			Filename: a.Filename,
			MimeType: a.MimeType,
			Data:     a.Data,
		})
	}
	return out
}

// DetermineErrorEventType maps a MailError to the appropriate JobEventType
func DetermineErrorEventType(err *errx.MailError) models.JobEventType {
	if err == nil {
		return models.JobEventTypeEmailFailed
	}

	switch err.Code {
	case errx.MailErrorCodeGoogleAuth, errx.MailErrorCodeAuthenticationFailed:
		return models.JobEventTypeEmailAuthError

	case errx.MailErrorCodeAccountSuspended, errx.MailErrorCodeAuthorizationFailed:
		return models.JobEventTypeEmailDisabled

	case errx.MailErrorCodeRateLimitExceeded, errx.MailErrorCodeSendingTooFast, errx.MailErrorCodeQuotaExceeded:
		return models.JobEventTypeEmailRateLimited

	case errx.MailErrorCodeServerUnreachable, errx.MailErrorCodeConnectionLost:
		return models.JobEventTypeEmailServerError

	default:
		return models.JobEventTypeEmailFailed
	}
}

// MailErrorToSendError converts a MailError to an EmailSendError for transport
func MailErrorToSendError(err *errx.MailError) *models.EmailSendError {
	if err == nil {
		return nil
	}

	userInfo := err.GetUserErrorInfo()

	return &models.EmailSendError{
		Code:           string(err.Code),
		Type:           string(err.Type),
		Message:        err.Message,
		ResolveMethod:  string(err.ResolveMethod),
		UserVisible:    err.IsUserVisible(),
		UserTitle:      userInfo.Title,
		UserMessage:    userInfo.Message,
		ActionRequired: userInfo.ActionRequired,
	}
}
