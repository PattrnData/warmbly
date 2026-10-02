package wmail

import (
	"context"
	"crypto/sha256"
	"fmt"

	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/worker/sendauth"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emsg"
	"github.com/warmbly/warmbly/internal/pkg/sendpayload"
)

type fakeAuthorizer func(context.Context, sendauth.Request) error

func (f fakeAuthorizer) Authorize(ctx context.Context, req sendauth.Request) error {
	return f(ctx, req)
}

func TestSendAuthorizationBeforeProviderForAllProvidersAndModes(t *testing.T) {
	for _, provider := range []models.InboxProvider{models.InboxProviderGoogle, models.InboxProviderOutlook, models.InboxProviderSMTPIMAP} {
		for _, warmup := range []bool{false, true} {
			t.Run(string(provider)+"/warmup="+map[bool]string{true: "true", false: "false"}[warmup], func(t *testing.T) {
				mail := &WMail{ID: uuid.New(), Email: "sender@example.org", EmailType: provider}
				calls := 0
				mail.sendAttempt = func(_ context.Context, _ *SendRequest, _ string) *SendResult {
					calls++
					return &SendResult{Success: true}
				}
				req := &SendRequest{TaskID: uuid.New(), EmailID: mail.ID, OrgID: uuid.New(), WorkerID: uuid.New(), MessageID: "<msg@example.org>", From: mail.Email, Provider: provider, IsWarmup: warmup}
				req.Authorizer = fakeAuthorizer(func(_ context.Context, got sendauth.Request) error {
					if got.TaskID != req.TaskID || got.EmailID != req.EmailID || got.OrgID != req.OrgID || got.WorkerID != req.WorkerID || got.MessageID != req.MessageID || got.From != req.From || got.Provider != req.Provider || got.IsWarmup != req.IsWarmup {
						t.Fatalf("binding mismatch: %+v", got)
					}
					return sendauth.ErrDenied
				})
				if r := mail.Send(context.Background(), req); r.Success || r.Error == nil || r.Error.Type != errx.MailErrorCritical || calls != 0 {
					t.Fatalf("denied send invoked provider: %+v calls=%d", r, calls)
				}
				req.Authorizer = nil
				if r := mail.Send(context.Background(), req); r.Success || calls != 0 {
					t.Fatalf("nil authorizer invoked provider: %+v calls=%d", r, calls)
				}
				req.Authorizer = fakeAuthorizer(func(context.Context, sendauth.Request) error { return nil })
				req.EmailID = uuid.New()
				if r := mail.Send(context.Background(), req); r.Success || calls != 0 {
					t.Fatalf("cached account mismatch invoked provider: %+v calls=%d", r, calls)
				}
			})
		}
	}
}

func TestSendHashesActualProviderPayloadBeforeClaim(t *testing.T) {
	mail := &WMail{ID: uuid.New(), Email: "sender@example.org", EmailType: models.InboxProviderGoogle}
	providerCalls := 0
	mail.sendAttempt = func(context.Context, *SendRequest, string) *SendResult {
		providerCalls++
		return &SendResult{Success: true}
	}
	refs := []emsg.Attachment{{S3Key: "object", Filename: "one.txt", MimeType: "text/plain"}}
	req := &SendRequest{
		TaskID: uuid.New(), EmailID: mail.ID, OrgID: uuid.New(), WorkerID: uuid.New(),
		MessageID: "msg", From: mail.Email, Provider: mail.EmailType,
		To: []string{"to@example.org"}, Cc: []string{"cc@example.org"}, Bcc: []string{"bcc@example.org"},
		Subject: "subject", BodyPlain: "plain", BodyHTML: "<p>html</p>",
		Attachments:    []Attachment{{Filename: "one.txt", MimeType: "text/plain", Data: []byte("bytes")}},
		AttachmentRefs: refs,
	}
	expected := (sendpayload.Content{
		From: req.From, MessageID: req.MessageID, To: req.To, CC: req.Cc, BCC: req.Bcc,
		Subject: req.Subject, Plain: req.BodyPlain, HTML: req.BodyHTML, Attachments: refs,
	}).Fingerprint()
	req.Authorizer = fakeAuthorizer(func(_ context.Context, got sendauth.Request) error {
		if got.PayloadHash != expected {
			t.Fatalf("worker hash %q differs from producer %q", got.PayloadHash, expected)
		}
		return sendauth.ErrDenied
	})
	if r := mail.Send(context.Background(), req); r.Success || providerCalls != 0 {
		t.Fatalf("denied hash reached provider: %+v", r)
	}
	req.Subject = "tampered"
	req.Authorizer = fakeAuthorizer(func(_ context.Context, got sendauth.Request) error {
		if got.PayloadHash == expected {
			t.Fatal("changed subject retained hash")
		}
		return sendauth.ErrDenied
	})
	if r := mail.Send(context.Background(), req); r.Success || providerCalls != 0 {
		t.Fatalf("tampered send reached provider: %+v", r)
	}
}

func TestSendDeniesChangedAttachmentBytesAndParentBeforeClaim(t *testing.T) {
	mail := &WMail{ID: uuid.New(), Email: "sender@example.org", EmailType: models.InboxProviderGoogle}
	providerCalls, claimCalls := 0, 0
	mail.sendAttempt = func(context.Context, *SendRequest, string) *SendResult {
		providerCalls++
		return &SendResult{Success: true}
	}
	data := []byte("original")
	req := &SendRequest{TaskID: uuid.New(), EmailID: mail.ID, OrgID: uuid.New(), WorkerID: uuid.New(),
		MessageID: "msg", From: mail.Email, Provider: mail.EmailType, InReplyTo: "prior",
		Attachments:    []Attachment{{Filename: "a.txt", MimeType: "text/plain", Data: data}},
		AttachmentRefs: []emsg.Attachment{{S3Key: "same-key", Filename: "a.txt", MimeType: "text/plain", SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}},
	}
	req.Authorizer = fakeAuthorizer(func(context.Context, sendauth.Request) error { claimCalls++; return nil })
	req.Attachments[0].Data = []byte("replaced at same key")
	if r := mail.Send(context.Background(), req); r.Success || providerCalls != 0 || claimCalls != 0 {
		t.Fatalf("changed bytes reached claim/provider: %+v claim=%d provider=%d", r, claimCalls, providerCalls)
	}
	req.Attachments[0].Data = data
	if r := mail.Send(context.Background(), req); !r.Success || providerCalls != 1 || claimCalls != 1 {
		t.Fatalf("matching attachment failed before provider: %+v claim=%d provider=%d", r, claimCalls, providerCalls)
	}
	req.Parent = &models.EmailParent{MessageID: "forged", ThreadID: "other"}
	approved := (sendpayload.Content{From: req.From, MessageID: req.MessageID, InReplyTo: req.InReplyTo, Attachments: req.AttachmentRefs}).Fingerprint()
	req.Authorizer = fakeAuthorizer(func(_ context.Context, got sendauth.Request) error {
		claimCalls++
		if got.PayloadHash == approved {
			t.Fatal("forged parent retained approved hash")
		}
		return sendauth.ErrDenied
	})
	if r := mail.Send(context.Background(), req); r.Success || providerCalls != 1 {
		t.Fatalf("forged parent reached provider: %+v", r)
	}
}

func TestSendNeverRetriesAfterClaimedAttempt(t *testing.T) {
	for _, warmup := range []bool{false, true} {
		for _, provider := range []models.InboxProvider{models.InboxProviderGoogle, models.InboxProviderOutlook, models.InboxProviderSMTPIMAP} {
			mail := &WMail{ID: uuid.New(), Email: "sender@example.org", EmailType: provider}
			providerCalls := 0
			mail.sendAttempt = func(context.Context, *SendRequest, string) *SendResult {
				providerCalls++
				return &SendResult{Error: errx.ErrMailServerUnreachable}
			}
			checks := 0
			req := &SendRequest{TaskID: uuid.New(), EmailID: mail.ID, OrgID: uuid.New(), WorkerID: uuid.New(), MessageID: "<msg@example.org>", From: mail.Email, Provider: mail.EmailType, IsWarmup: warmup}
			req.Authorizer = fakeAuthorizer(func(context.Context, sendauth.Request) error {
				checks++
				if checks > 1 {
					return sendauth.ErrDenied
				}
				return nil
			})
			result := mail.Send(context.Background(), req)
			wantChecks := 1 // A claim consumes the task's only provider attempt.
			if result.Success || checks != wantChecks || providerCalls != 1 {
				t.Fatalf("retry leaked (%s warmup=%v): %+v checks=%d provider=%d", provider, warmup, result, checks, providerCalls)
			}
		}
	}
}

func TestSendHTTPDenialsNeverInvokeProvider(t *testing.T) {
	for _, response := range []string{`{"allowed":false}`, `{"allowed":null}`, `{"allowed":"true"}`, `{`, ``} {
		t.Run(response, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			auth, err := sendauth.NewHTTPAuthorizer(server.URL, "token", 100000000)
			if err != nil {
				t.Fatal(err)
			}
			mail := &WMail{ID: uuid.New(), Email: "sender@example.org", EmailType: models.InboxProviderGoogle}
			calls := 0
			mail.sendAttempt = func(context.Context, *SendRequest, string) *SendResult { calls++; return &SendResult{Success: true} }
			req := &SendRequest{TaskID: uuid.New(), EmailID: mail.ID, OrgID: uuid.New(), WorkerID: uuid.New(), MessageID: "msg", From: mail.Email, Provider: mail.EmailType, Authorizer: auth}
			if result := mail.Send(context.Background(), req); result.Success || calls != 0 || requests.Load() != 1 {
				t.Fatalf("unexpected: %+v provider=%d http=%d", result, calls, requests.Load())
			}
		})
	}

}
