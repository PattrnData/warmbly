package email

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/cache"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
	"golang.org/x/oauth2"
)

type reconnectRepo struct {
	repository.EmailRepository
	account   *models.Email
	delegated bool
	writes    int
}

func (r *reconnectRepo) Get(_ context.Context, _ string, _ string) (*models.Email, *errx.Error) {
	return r.account, nil
}
func (r *reconnectRepo) ReconnectOutlookCredentialVersion(_ context.Context, _ string, _ uuid.UUID, _ uuid.UUID) (string, *errx.Error) {
	if !r.delegated {
		return "", errx.ErrEmailOnboardState
	}
	return "synthetic-version", nil
}
func (r *reconnectRepo) ReconnectOutlookToken(_ context.Context, _ string, _, _ uuid.UUID, _, version, _, _ string, _ time.Time) *errx.Error {
	if version != "synthetic-version" {
		return errx.ErrEmailOnboardState
	}
	r.writes++
	return nil
}

func TestReconnectTargetRejectsMismatches(t *testing.T) {
	id, org := uuid.New(), uuid.New()
	base := models.Email{ID: id, UserID: "user-a", OrganizationID: &org, Email: "owner@example.test", Provider: "outlook", Status: "inactive"}
	cases := []struct {
		name   string
		mutate func(*models.Email)
		user   string
		org    *uuid.UUID
		id     uuid.UUID
		owner  string
		reject bool
	}{
		{name: "valid", user: "user-a", org: &org, id: id, owner: "OWNER@example.test"},
		{name: "wrong user", user: "user-b", org: &org, id: id, owner: base.Email, reject: true},
		{name: "wrong account", user: "user-a", org: &org, id: uuid.New(), owner: base.Email, reject: true},
		{name: "wrong org", user: "user-a", org: ptrUUID(uuid.New()), id: id, owner: base.Email, reject: true},
		{name: "missing org", user: "user-a", id: id, owner: base.Email, reject: true},
		{name: "wrong Microsoft owner", user: "user-a", org: &org, id: id, owner: "other@example.test", reject: true},
		{name: "non Outlook", mutate: func(a *models.Email) { a.Provider = "gmail" }, user: "user-a", org: &org, id: id, owner: base.Email, reject: true},
		{name: "active", mutate: func(a *models.Email) { a.Status = "active" }, user: "user-a", org: &org, id: id, owner: base.Email, reject: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := base
			if tc.mutate != nil {
				tc.mutate(&acc)
			}
			got := validateReconnectTarget(&acc, tc.user, tc.org, tc.id, tc.owner)
			if (got != nil) != tc.reject {
				t.Fatalf("reject=%v, want %v", got != nil, tc.reject)
			}
		})
	}
}
func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }

func TestOAuthStateIsAtomicSingleUse(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	svc := &emailService{r: &cache.Cache{Client: client}}
	ctx := context.Background()
	state := "synthetic-single-use-state"
	org, id := uuid.New(), uuid.New()
	if xerr := svc.saveOnboardingState(ctx, state, &models.EmailOnboardingState{UserID: "owner", OrganizationID: &org, Provider: "outlook", Nonce: state, ReconnectAccountID: &id}); xerr != nil {
		t.Fatal(xerr)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, xerr := svc.takeOnboardingState(ctx, state)
			results <- xerr == nil && got != nil && got.ReconnectAccountID != nil && *got.ReconnectAccountID == id
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("state redeemed %d times, want once", successes)
	}
	if _, xerr := svc.takeOnboardingState(ctx, state); xerr == nil {
		t.Fatal("replay accepted")
	}
}

func TestOAuthFinishReconnectBindsOwnerAndRejectsReplay(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	oldClient := httpClient
	defer func() { httpClient = oldClient }()
	owner := "intruder@example.test"
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "graph.microsoft.com" {
			t.Errorf("unexpected request to %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"mail":"` + owner + `"}`)), Request: r}, nil
	})}
	org, id := uuid.New(), uuid.New()
	repo := &reconnectRepo{account: &models.Email{ID: id, UserID: "owner", OrganizationID: &org, Email: "owner@example.test", Provider: "outlook", Status: "inactive"}, delegated: true}
	svc := &emailService{emailRepository: repo, r: &cache.Cache{Client: client}, oauthInbox: &config.Oauth2Inbox{Outlook: &oauth2.Config{ClientID: "synthetic", ClientSecret: "synthetic", Endpoint: oauth2.Endpoint{AuthURL: "https://example.test/authorize", TokenURL: tokenServer.URL}}}}
	ctx := context.Background()
	save := func(state string) {
		t.Helper()
		if xerr := svc.saveOnboardingState(ctx, state, &models.EmailOnboardingState{UserID: "owner", OrganizationID: &org, Provider: "outlook", Nonce: state, ReconnectAccountID: &id, ReconnectCredentialVersion: "synthetic-version"}); xerr != nil {
			t.Fatal(xerr)
		}
	}
	save("wrong-user")
	if _, xerr := svc.OAuthFinish(ctx, "intruder", &org, "synthetic-code", "wrong-user"); xerr == nil {
		t.Fatal("wrong user callback accepted")
	}
	save("wrong-org")
	if _, xerr := svc.OAuthFinish(ctx, "owner", ptrUUID(uuid.New()), "synthetic-code", "wrong-org"); xerr == nil {
		t.Fatal("wrong org callback accepted")
	}
	save("wrong-account")
	repo.account.ID = uuid.New()
	if _, xerr := svc.OAuthFinish(ctx, "owner", &org, "synthetic-code", "wrong-account"); xerr == nil {
		t.Fatal("wrong account callback accepted")
	}
	repo.account.ID = id
	save("wrong-mailbox-owner")
	if _, xerr := svc.OAuthFinish(ctx, "owner", &org, "synthetic-code", "wrong-mailbox-owner"); xerr == nil {
		t.Fatal("wrong Microsoft owner accepted")
	}
	if repo.writes != 0 {
		t.Fatalf("writes after mismatch: %d", repo.writes)
	}
	owner = "owner@example.test"
	save("valid-once")
	if _, xerr := svc.OAuthFinish(ctx, "owner", &org, "synthetic-code", "valid-once"); xerr != nil {
		t.Fatalf("valid callback rejected: %v", xerr)
	}
	if _, xerr := svc.OAuthFinish(ctx, "owner", &org, "synthetic-code", "valid-once"); xerr == nil {
		t.Fatal("replay accepted")
	}
	if repo.writes != 1 {
		t.Fatalf("writes after valid callback and replay: %d", repo.writes)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReconnectStartBindsExistingRowAndRequestsConsent(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	org, id := uuid.New(), uuid.New()
	repo := &reconnectRepo{account: &models.Email{ID: id, UserID: "owner", OrganizationID: &org, Email: "owner@example.test", Provider: "outlook", Status: "inactive"}, delegated: true}
	svc := &emailService{emailRepository: repo, r: &cache.Cache{Client: client}, oauthInbox: &config.Oauth2Inbox{Outlook: &oauth2.Config{ClientID: "synthetic", RedirectURL: "https://example.test/callback", Scopes: []string{"offline_access", "https://graph.microsoft.com/User.Read"}, Endpoint: oauth2.Endpoint{AuthURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"}}}}
	out, xerr := svc.OAuthReconnectStart(context.Background(), "owner", &org, id)
	if xerr != nil {
		t.Fatalf("start: %v", xerr)
	}
	parsed, err := url.Parse(out.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if parsed.Host != "login.microsoftonline.com" || q.Get("prompt") != "consent" || q.Get("state") != out.State || !strings.Contains(q.Get("scope"), "offline_access") {
		t.Fatal("Microsoft consent URL missing required state, scope, or prompt")
	}
	state, xerr := svc.takeOnboardingState(context.Background(), out.State)
	if xerr != nil || state.ReconnectAccountID == nil || *state.ReconnectAccountID != id || state.UserID != "owner" || state.OrganizationID == nil || *state.OrganizationID != org {
		t.Fatal("state was not bound to owner/org/account")
	}
}

func TestReconnectStartRejectsWrongAccountBeforeState(t *testing.T) {
	id, org := uuid.New(), uuid.New()
	acc := &models.Email{ID: id, UserID: "owner", OrganizationID: &org, Email: "owner@example.test", Provider: "outlook", Status: "inactive"}
	s := &emailService{emailRepository: &reconnectRepo{account: acc, delegated: true}} // no Redis; mismatch must fail before state creation
	if _, err := s.OAuthReconnectStart(context.Background(), "intruder", &org, id); err == nil {
		t.Fatal("cross-user start accepted")
	}
	if _, err := s.OAuthReconnectStart(context.Background(), "owner", ptrUUID(uuid.New()), id); err == nil {
		t.Fatal("cross-org start accepted")
	}
	if _, err := s.OAuthReconnectStart(context.Background(), "owner", &org, uuid.New()); err == nil {
		t.Fatal("different account start accepted")
	}
}
