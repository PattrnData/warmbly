package email

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
	"golang.org/x/oauth2/clientcredentials"
)

type conversionRepo struct {
	repository.EmailRepository
	target                    *models.Email
	versionReads, conversions int
}

func (r *conversionRepo) Get(_ context.Context, _, id string) (*models.Email, *errx.Error) {
	if id == r.target.ID.String() {
		return r.target, nil
	}
	return nil, errx.ErrEmailOnboardState
}
func (r *conversionRepo) OutlookAppOnlyConversionVersion(_ context.Context, _ string, _, _, _ uuid.UUID, _ string) (string, *errx.Error) {
	r.versionReads++
	return "fixture-version", nil
}
func (r *conversionRepo) ConvertOutlookAppOnly(_ context.Context, _ string, _, _, _ uuid.UUID, _, version string) *errx.Error {
	if version != "fixture-version" {
		return errx.ErrEmailOnboardState
	}
	r.conversions++
	return nil
}

func TestAppTokenMatchesTenantAndRoles(t *testing.T) {
	tenant := uuid.New()
	makeToken := func(payload string) string {
		return "header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
	}
	for _, tc := range []struct {
		name, token string
		want        bool
	}{
		{"valid", makeToken(`{"tid":"` + tenant.String() + `","appid":"client","aud":"https://graph.microsoft.com","roles":["Mail.ReadWrite","Mail.Send"]}`), true},
		{"other tenant", makeToken(`{"tid":"` + uuid.NewString() + `","appid":"client","aud":"https://graph.microsoft.com","roles":["Mail.ReadWrite","Mail.Send"]}`), false},
		{"other client", makeToken(`{"tid":"` + tenant.String() + `","appid":"other","aud":"https://graph.microsoft.com","roles":["Mail.ReadWrite","Mail.Send"]}`), false},
		{"no send role", makeToken(`{"tid":"` + tenant.String() + `","appid":"client","aud":"https://graph.microsoft.com","roles":["Mail.ReadWrite"]}`), false},
		{"wrong audience", makeToken(`{"tid":"` + tenant.String() + `","appid":"client","aud":"other","roles":["Mail.ReadWrite","Mail.Send"]}`), false},
		{"not JWT", "opaque", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := appTokenMatchesTenantAndRoles(tc.token, tenant, "client"); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConvertOutlookAppOnlyExactInactiveSharedSender(t *testing.T) {
	org, targetID, workerID, tenantID := uuid.New(), uuid.MustParse("af571c6e-e6f0-4cb9-90fe-a7d5105babd7"), uuid.New(), uuid.New()
	target := &models.Email{ID: targetID, UserID: "owner", OrganizationID: &org, WorkerID: &workerID, Email: "shared@example.test", Provider: "outlook", Status: "inactive"}
	repo := &conversionRepo{target: target}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := `{"tid":"` + tenantID.String() + `","appid":"synthetic","aud":"https://graph.microsoft.com","roles":["Mail.ReadWrite","Mail.Send"]}`
		accessToken := "header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
		_, _ = io.WriteString(w, `{"access_token":"`+accessToken+`","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	oldClient := httpClient
	defer func() { httpClient = oldClient }()
	graphStatus := http.StatusOK
	graphReads := 0
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		graphReads++
		if r.Method != http.MethodGet || !strings.Contains(r.URL.String(), "/users/shared@example.test/mailFolders/inbox") {
			t.Errorf("unexpected Graph target: %s", r.URL)
		}
		return &http.Response{StatusCode: graphStatus, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
	})}
	svc := &emailService{emailRepository: repo, oauthInbox: &config.Oauth2Inbox{OutlookAppOnly: &clientcredentials.Config{
		ClientID: "synthetic", ClientSecret: "synthetic", TokenURL: tokenServer.URL + "/" + tenantID.String() + "/oauth2/v2.0/token",
	}}}
	call := func(user string, orgID *uuid.UUID, id, worker uuid.UUID, email, version string) *errx.Error {
		_, xerr := svc.ConvertOutlookAppOnly(context.Background(), user, orgID, id, worker, email, tenantID, version)
		return xerr
	}
	for _, tc := range []struct {
		name, user, email, version string
		org                        *uuid.UUID
		id, worker                 uuid.UUID
	}{
		{"wrong user", "intruder", target.Email, "fixture-version", &org, targetID, workerID},
		{"wrong org", "owner", target.Email, "fixture-version", ptrUUID(uuid.New()), targetID, workerID},
		{"wrong id", "owner", target.Email, "fixture-version", &org, uuid.New(), workerID},
		{"quarantined fifth", "owner", target.Email, "fixture-version", &org, uuid.MustParse("a5f28cfb-b10f-4597-b445-28e647e0dd92"), workerID},
		{"wrong email", "owner", "another@example.test", "fixture-version", &org, targetID, workerID},
		{"wrong worker", "owner", target.Email, "fixture-version", &org, targetID, uuid.New()},
		{"no worker", "owner", target.Email, "fixture-version", &org, targetID, uuid.Nil},
		{"no escrow version", "owner", target.Email, "", &org, targetID, workerID},
		{"stale escrow version", "owner", target.Email, "old-version", &org, targetID, workerID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if call(tc.user, tc.org, tc.id, tc.worker, tc.email, tc.version) == nil {
				t.Fatal("mismatch accepted")
			}
		})
	}
	if graphReads != 0 || repo.conversions != 0 {
		t.Fatal("mismatch reached Graph/write path")
	}
	graphStatus = http.StatusForbidden
	if call("owner", &org, targetID, workerID, target.Email, "fixture-version") == nil || repo.conversions != 0 {
		t.Fatal("Graph rejection changed credential")
	}
	graphStatus = http.StatusOK
	if _, xerr := svc.ConvertOutlookAppOnly(context.Background(), "owner", &org, targetID, workerID, target.Email, uuid.New(), "fixture-version"); xerr == nil || repo.conversions != 0 {
		t.Fatal("wrong tenant accepted")
	}
	acc, xerr := svc.ConvertOutlookAppOnly(context.Background(), "owner", &org, targetID, workerID, target.Email, tenantID, "fixture-version")
	if xerr != nil || acc != target || acc.Status != "inactive" || repo.conversions != 1 {
		t.Fatalf("conversion failed or activated sender: %v %+v", xerr, acc)
	}
	target.Status = "active"
	if call("owner", &org, targetID, workerID, target.Email, "fixture-version") == nil || repo.conversions != 1 {
		t.Fatal("active target accepted")
	}
}
