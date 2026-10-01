// Package sendauth obtains a fresh backend decision before every provider send.
package sendauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// ErrDenied is deliberately nonsecret: neither transport errors nor backend bodies escape.
var ErrDenied = errors.New("send authorization denied or unavailable")

// Request binds the backend decision to the exact queued send and cached mailbox.
type Request struct {
	TaskID      uuid.UUID            `json:"task_id"`
	EmailID     uuid.UUID            `json:"email_account_id"`
	OrgID       uuid.UUID            `json:"organization_id"`
	MessageID   string               `json:"message_id"`
	PayloadHash string               `json:"payload_hash"`
	WorkerID    uuid.UUID            `json:"worker_id"`
	From        string               `json:"from"`
	Provider    models.InboxProvider `json:"provider"`
	IsWarmup    bool                 `json:"is_warmup"`
}

type response struct {
	Allowed *bool `json:"allowed"`
}

// Authorizer is the send-time decision boundary; nil is never a permit.
type Authorizer interface {
	Authorize(context.Context, Request) error
}

type HTTPAuthorizer struct {
	endpoint string
	token    string
	client   *http.Client
}

// NewHTTPAuthorizer rejects missing or ambiguous configuration at startup.
func NewHTTPAuthorizer(baseURL, token string, timeout time.Duration) (*HTTPAuthorizer, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.TrimSpace(token) == "" || timeout <= 0 || timeout > 2*time.Second {
		return nil, errors.New("invalid send authorization backend configuration")
	}
	return &HTTPAuthorizer{endpoint: strings.TrimRight(u.String(), "/") + "/api/v1/internal/send-authorization", token: token, client: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Authorize accepts only HTTP 200 with an explicit JSON boolean allowed.
func (a *HTTPAuthorizer) Authorize(ctx context.Context, decision Request) error {
	if a == nil || a.client == nil || decision.TaskID == uuid.Nil || decision.EmailID == uuid.Nil || decision.OrgID == uuid.Nil || decision.WorkerID == uuid.Nil || strings.TrimSpace(decision.MessageID) == "" || len(decision.MessageID) > 512 || len(decision.PayloadHash) != 64 || strings.TrimSpace(decision.From) == "" || !validProvider(decision.Provider) {
		return ErrDenied
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(payload))
	if err != nil {
		return ErrDenied
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return ErrDenied
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ErrDenied
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(body) > 4096 {
		return ErrDenied
	}
	var decoded response
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&decoded) != nil || decoded.Allowed == nil || !*decoded.Allowed {
		return ErrDenied
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrDenied
	}
	return nil
}

func validProvider(p models.InboxProvider) bool {
	return p == models.InboxProviderGoogle || p == models.InboxProviderOutlook || p == models.InboxProviderSMTPIMAP
}
