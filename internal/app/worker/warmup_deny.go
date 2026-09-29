package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WarmupDenyChecker returns the durable status for exactly one mailbox.
type WarmupDenyChecker interface {
	Denied(ctx context.Context, emailID uuid.UUID) (bool, error)
}

type httpWarmupDenyChecker struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPWarmupDenyChecker(baseURL, token string) (WarmupDenyChecker, error) {
	if baseURL == "" || token == "" {
		return nil, errors.New("warmup deny: backend URL and worker token required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("warmup deny: invalid backend URL")
	}
	return &httpWarmupDenyChecker{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (h *httpWarmupDenyChecker) Denied(ctx context.Context, emailID uuid.UUID) (bool, error) {
	if emailID == uuid.Nil {
		return false, errors.New("warmup deny: missing mailbox ID")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+"/api/v1/internal/warmup-deny/"+emailID.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := h.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("warmup deny lookup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("warmup deny lookup: status %d", resp.StatusCode)
	}
	var state struct {
		EmailID uuid.UUID `json:"email_id"`

		WarmupDenied *bool `json:"warmup_denied"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return false, fmt.Errorf("warmup deny decode: %w", err)
	}
	if state.EmailID != emailID || state.WarmupDenied == nil {
		return false, errors.New("warmup deny: incomplete or mismatched mailbox status")
	}
	return *state.WarmupDenied, nil
}
