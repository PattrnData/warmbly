package wmail

import (
	"errors"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

func (w *WMail) CaptureError(err error) error {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("user_id", w.UserID.String())
		scope.SetTag("email_id", w.ID.String())
		sentry.CaptureException(err)
	})

	// If the error is a critical mail error (auth, disabled, rate limit), publish
	// an event so the consumer can mark the account inactive and stop syncing.
	var mailErr *errx.MailError
	if !errors.As(err, &mailErr) {
		return nil
	}

	eventType := mailErrorToJobEventType(mailErr)
	if eventType == "" {
		return nil
	}
	terminal := eventType == models.JobEventTypeEmailAuthError || eventType == models.JobEventTypeEmailDisabled
	if terminal {
		w.quarantined.Store(true)
	}

	userInfo := mailErr.GetUserErrorInfo()
	errorEvent := models.EmailErrorEvent{
		EmailAccountID: w.ID.String(),
		UserID:         w.UserID.String(),
		ErrorCode:      string(mailErr.Code),
		ErrorType:      string(mailErr.Type),
		ResolveMethod:  string(mailErr.ResolveMethod),
		Message:        mailErr.Message,
		UserVisible:    mailErr.IsUserVisible(),
		UserTitle:      userInfo.Title,
		UserMessage:    userInfo.Message,
		ActionRequired: userInfo.ActionRequired,
		Timestamp:      time.Now().Unix(),
		OccurredAt:     time.Now().UTC(),
	}

	if err := w.onEvent(eventType, errorEvent); err != nil {
		return err
	}

	// Critical errors should stop the sync loop and remove the account from the
	// worker's local state until the user re-authenticates.
	if terminal {
		if w.Cancel != nil {
			w.Cancel()
		}
		if w.TerminateFunc != nil {
			w.TerminateFunc()
		}
	}
	return nil
}

// mailErrorToJobEventType maps a mail error code to the matching job event type
// so the consumer knows how to handle it.
func mailErrorToJobEventType(mailErr *errx.MailError) models.JobEventType {
	switch mailErr.Code {
	case errx.MailErrorCodeGoogleAuth,
		errx.MailErrorCodeAuthenticationFailed,
		errx.MailErrorCodeAuthorizationFailed,
		errx.MailErrorCodeInvalidCredentials:
		return models.JobEventTypeEmailAuthError
	case errx.MailErrorCodeRateLimitExceeded,
		errx.MailErrorCodeSendingTooFast:
		return models.JobEventTypeEmailRateLimited
	case errx.MailErrorCodeServerUnreachable,
		errx.MailErrorCodeConnectionLost,
		errx.MailErrorCodeImapUnknown:
		return models.JobEventTypeEmailServerError
	}
	return ""
}
