package email

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
	"golang.org/x/oauth2"
)

type reconnectRepo struct {
	repository.EmailRepository
	account *models.Email
	creds   *repository.OAuthCredentials
	writes  int
	failure *errx.Error
}

func (r *reconnectRepo) Get(_ context.Context, userID, _ string) (*models.Email, *errx.Error) {
	if r.account.UserID != userID {
		return nil, errx.ErrNotFound
	}
	return r.account, nil
}
func (r *reconnectRepo) GetOAuthCredentials(_ context.Context, _ uuid.UUID) (*repository.OAuthCredentials, *errx.Error) {
	return r.creds, nil
}
func (r *reconnectRepo) ReconnectOutlookCredentials(_ context.Context, id uuid.UUID, userID string, orgID uuid.UUID, email, previous, access, refresh string, expiry time.Time) *errx.Error {
	r.writes++
	if id != r.account.ID || userID != r.account.UserID || orgID != *r.account.OrganizationID || email != r.account.Email || previous != r.creds.RefreshToken || access != "fresh-access" || refresh != "fresh-refresh" || expiry.IsZero() {
		return errx.ErrInvalid
	}
	return r.failure
}

type graphRoundTrip func(*http.Request) (*http.Response, error)

func (f graphRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestReconnectOutlookIdentityAndScope(t *testing.T) {
	org, id := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, graphEmail       string
		change                 func(*reconnectRepo)
		wrongOrg, failWrite    bool
		badToken, expiredToken bool
		wantWrites             int
	}{
		{name: "valid", graphEmail: "Owner@Example.com", wantWrites: 1},
		{name: "different delegated identity", graphEmail: "other@example.com"},
		{name: "missing owner", graphEmail: ""},
		{name: "wrong organization", graphEmail: "owner@example.com", wrongOrg: true},
		{name: "app-only token", graphEmail: "owner@example.com", change: func(r *reconnectRepo) { r.creds.RefreshToken = models.GraphAppOnlyRefreshToken }},
		{name: "shared mailbox mismatch", graphEmail: "delegate@example.com"},
		{name: "CAS conflict", graphEmail: "owner@example.com", failWrite: true, wantWrites: 1},
		{name: "inactive never reactivated", graphEmail: "owner@example.com", change: func(r *reconnectRepo) { r.account.Status = "inactive" }},
		{name: "different provider never converted", graphEmail: "owner@example.com", change: func(r *reconnectRepo) { r.account.Provider = "gmail" }},
		{name: "oauth exchange rejected", graphEmail: "owner@example.com", badToken: true},
		{name: "oauth token expired", graphEmail: "owner@example.com", expiredToken: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &reconnectRepo{account: &models.Email{ID: id, UserID: "operator-user", OrganizationID: &org, Email: "owner@example.com", Provider: "outlook", Status: "active"}, creds: &repository.OAuthCredentials{RefreshToken: "old-refresh"}}
			if tc.change != nil {
				tc.change(r)
			}
			if tc.failWrite {
				r.failure = errx.ErrInvalid
			}
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodPost {
					t.Errorf("unexpected method %s", req.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.badToken {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
					return
				}
				if tc.expiredToken {
					_, _ = io.WriteString(w, `{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":-3600,"token_type":"Bearer"}`)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":3600,"token_type":"Bearer"}`)
			}))
			defer tokenEndpoint.Close()
			oldClient := httpClient
			httpClient = &http.Client{Transport: graphRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != "https://graph.microsoft.com/v1.0/me" || req.Header.Get("Authorization") != "Bearer fresh-access" {
					t.Errorf("unexpected Graph request %s", req.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"mail":%q}`, tc.graphEmail))), Header: make(http.Header)}, nil
			})}
			defer func() { httpClient = oldClient }()
			s := &emailService{emailRepository: r, oauthInbox: &config.Oauth2Inbox{Outlook: &oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: tokenEndpoint.URL}}}}
			requestedOrg := org
			if tc.wrongOrg {
				requestedOrg = uuid.New()
			}
			got, xerr := s.ReconnectOutlook(context.Background(), "operator-user", requestedOrg, id, "code")
			if r.writes != tc.wantWrites {
				t.Fatalf("writes = %d, want %d", r.writes, tc.wantWrites)
			}
			if tc.name == "valid" {
				if xerr != nil || got == nil || got.ID != id {
					t.Fatalf("valid reconnect: account=%v error=%v", got, xerr)
				}
			} else if xerr == nil || got != nil {
				t.Fatalf("unsafe reconnect: account=%v error=%v", got, xerr)
			}
		})
	}
}
