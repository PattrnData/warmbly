package wmail

import (
	"context"

	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/worker/sendauth"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
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
