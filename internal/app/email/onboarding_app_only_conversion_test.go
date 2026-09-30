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
	target, parent            *models.Email
	versionReads, conversions int
}

func (r *conversionRepo) Get(_ context.Context, _, id string) (*models.Email, *errx.Error) {
	if id == r.target.ID.String() {
		return r.target, nil
	}
	if id == r.parent.ID.String() {
		return r.parent, nil
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
	org, targetID, parentID, tenantID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	target := &models.Email{ID: targetID, UserID: "owner", OrganizationID: &org, Email: "shared@example.test", Provider: "outlook", Status: "inactive"}
	parent := &models.Email{ID: parentID, UserID: "owner", OrganizationID: &org, Email: "delegate@example.test", Provider: "outlook", Status: "active"}
	repo := &conversionRepo{target: target, parent: parent}
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
	call := func(user string, orgID *uuid.UUID, id, delegate uuid.UUID, email string) *errx.Error {
		_, xerr := svc.ConvertOutlookAppOnly(context.Background(), user, orgID, id, delegate, email, tenantID)
		return xerr
	}
	for _, tc := range []struct {
		name, user, email string
		org               *uuid.UUID
		id, parent        uuid.UUID
	}{
		{"wrong user", "intruder", target.Email, &org, targetID, parentID},
		{"wrong org", "owner", target.Email, ptrUUID(uuid.New()), targetID, parentID},
		{"wrong id", "owner", target.Email, &org, uuid.New(), parentID},
		{"wrong email", "owner", "another@example.test", &org, targetID, parentID},
		{"no parent", "owner", target.Email, &org, targetID, uuid.Nil},
		{"self parent", "owner", target.Email, &org, targetID, targetID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if call(tc.user, tc.org, tc.id, tc.parent, tc.email) == nil {
				t.Fatal("mismatch accepted")
			}
		})
	}
	if repo.versionReads != 0 || graphReads != 0 || repo.conversions != 0 {
		t.Fatal("mismatch reached credential/Graph path")
	}
	graphStatus = http.StatusForbidden
	if call("owner", &org, targetID, parentID, target.Email) == nil || repo.conversions != 0 {
		t.Fatal("Graph rejection changed credential")
	}
	graphStatus = http.StatusOK
	if _, xerr := svc.ConvertOutlookAppOnly(context.Background(), "owner", &org, targetID, parentID, target.Email, uuid.New()); xerr == nil || repo.conversions != 0 {
		t.Fatal("wrong tenant accepted")
	}
	acc, xerr := svc.ConvertOutlookAppOnly(context.Background(), "owner", &org, targetID, parentID, target.Email, tenantID)
	if xerr != nil || acc != target || acc.Status != "inactive" || repo.conversions != 1 {
		t.Fatalf("conversion failed or activated sender: %v %+v", xerr, acc)
	}
	parent.Status = "inactive"
	if call("owner", &org, targetID, parentID, target.Email) == nil || repo.conversions != 1 {
		t.Fatal("inactive parent accepted")
	}
}
