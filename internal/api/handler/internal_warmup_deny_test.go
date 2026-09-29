package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type warmupDenyEmailStub struct {
	repository.EmailRepository
	id      uuid.UUID
	mailbox *models.Email
	failure bool
}

func (s *warmupDenyEmailStub) GetByID(_ context.Context, id uuid.UUID) (*models.Email, *errx.Error) {
	s.id = id
	if s.failure {
		return nil, errx.InternalError()
	}
	return s.mailbox, nil
}
func TestInternalGetWarmupDeny_DurableReadAndFailures(t *testing.T) {
	id := uuid.New()
	stub := &warmupDenyEmailStub{mailbox: &models.Email{ID: id, WarmupDenied: true}}
	h := &Handler{WarmupDenyEmails: stub}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/internal/:emailID", h.InternalGetWarmupDeny)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		return r
	}
	if got := request("/internal/" + id.String()); got.Code != 200 || got.Body.String() == "" || stub.id != id || !containsWarmupDeny(got.Body.String()) {
		t.Fatalf("durable deny response: %d %s", got.Code, got.Body.String())
	}
	stub.failure = true
	if got := request("/internal/" + id.String()); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("lookup failure: %d", got.Code)
	}
	stub.failure = false
	stub.mailbox = nil
	if got := request("/internal/" + id.String()); got.Code != http.StatusNotFound {
		t.Fatalf("missing mailbox: %d", got.Code)
	}
	if got := request("/internal/not-a-uuid"); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid mailbox: %d", got.Code)
	}
	h.WarmupDenyEmails = nil
	if got := request("/internal/" + id.String()); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired repo: %d", got.Code)
	}

}
func containsWarmupDeny(s string) bool { return strings.Contains(s, `"warmup_denied":true`) }
