package sendauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

func validRequest() Request {
	return Request{TaskID: uuid.New(), EmailID: uuid.New(), OrgID: uuid.New(), WorkerID: uuid.New(), MessageID: "message-id", PayloadHash: strings.Repeat("a", 64), From: "sender@example.com", Provider: models.InboxProviderGoogle}
}

func TestHTTPAuthorizerFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		delay  time.Duration
		permit bool
	}{
		{"permit", 200, `{"allowed":true}`, 0, true},
		{"deny", 200, `{"allowed":false}`, 0, false},
		{"empty", 200, `{}`, 0, false},
		{"null", 200, `null`, 0, false},
		{"string", 200, `{"allowed":"true"}`, 0, false},
		{"unknown", 200, `{"allowed":true,"unexpected":1}`, 0, false},
		{"trailing", 200, `{"allowed":true}{}`, 0, false},
		{"unavailable", 503, `{"allowed":true}`, 0, false},
		{"timeout", 200, `{"allowed":true}`, 100 * time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/internal/send-authorization" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer secret-token" {
					t.Errorf("unexpected request metadata")
					w.WriteHeader(http.StatusForbidden)
					return
				}
				time.Sleep(tc.delay)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client, err := NewHTTPAuthorizer(srv.URL, "secret-token", 20*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			err = client.Authorize(context.Background(), validRequest())
			if tc.permit && err != nil || !tc.permit && !errors.Is(err, ErrDenied) {
				t.Fatalf("permit=%v err=%v", tc.permit, err)
			}
			if err != nil && (strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), tc.body)) {
				t.Fatal("error leaked response or token")
			}
		})
	}
}

func TestHTTPAuthorizerRequiresConfigurationAndBinding(t *testing.T) {
	for _, cfg := range []struct{ base, token string }{{"", "token"}, {"http://example.com", ""}, {"http://example.com/path", "token"}} {
		if _, err := NewHTTPAuthorizer(cfg.base, cfg.token, time.Second); err == nil {
			t.Fatalf("accepted invalid config: %q", cfg.base)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid binding reached backend") }))
	defer srv.Close()
	client, err := NewHTTPAuthorizer(srv.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req := validRequest()
	req.WorkerID = uuid.Nil
	if err := client.Authorize(context.Background(), req); !errors.Is(err, ErrDenied) {
		t.Fatalf("err=%v", err)
	}
}
