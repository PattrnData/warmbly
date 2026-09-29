package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/api/middleware"
	emailapp "github.com/warmbly/warmbly/internal/app/email"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

type getEmailSpy struct {
	emailapp.EmailService
	gotOrg, gotAccount string
}

func (s *getEmailSpy) Get(_ context.Context, orgID, accountID string) (*models.Email, *errx.Error) {
	s.gotOrg, s.gotAccount = orgID, accountID
	return &models.Email{}, nil
}

func TestGetEmailUsesSelectedOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orgID, userID, accountID := uuid.New(), uuid.New(), uuid.New()
	service := &getEmailSpy{}
	h := &Handler{EmailService: service}
	r := gin.New()
	r.GET("/emails/:id", func(c *gin.Context) {
		c.Set(middleware.UserIDKey, userID.String())
		c.Set(middleware.OrganizationIDKey, orgID)
		c.Next()
	}, h.GetEmail)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/emails/"+accountID.String(), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if service.gotOrg != orgID.String() || service.gotAccount != accountID.String() {
		t.Fatalf("Get args org=%q account=%q; want org=%q account=%q", service.gotOrg, service.gotAccount, orgID, accountID)
	}
}

func TestGetEmailRejectsMissingOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &getEmailSpy{}
	h := &Handler{EmailService: service}
	r := gin.New()
	r.GET("/emails/:id", h.GetEmail)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/emails/"+uuid.NewString(), nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if service.gotAccount != "" {
		t.Fatal("service called without selected organization")
	}
}
