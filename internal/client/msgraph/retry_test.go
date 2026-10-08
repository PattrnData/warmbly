package msgraph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetRetriesTransientGraphFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()

	c := &Client{hc: server.Client()}
	var result struct {
		Value []any `json:"value"`
	}
	if err := c.doJSON(context.Background(), http.MethodGet, server.URL, nil, &result); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || result.Value == nil {
		t.Fatalf("attempts=%d result=%+v, want 3 and decoded data", attempts, result)
	}
}

func TestGetDoesNotRetryAuthorizationFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				w.WriteHeader(status)
			}))
			defer server.Close()
			c := &Client{hc: server.Client()}
			if err := c.doJSON(context.Background(), http.MethodGet, server.URL, nil, nil); err == nil || attempts != 1 {
				t.Fatalf("err=%v attempts=%d, want auth error after one request", err, attempts)
			}
		})
	}
}

func TestGetHonorsRateLimitAndContextCancellation(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel while waiting on the provider's long Retry-After; no second probe.
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelTimeout()
	defer cancel()
	c := &Client{hc: server.Client()}
	if err := c.doJSON(ctx, http.MethodGet, server.URL, nil, nil); !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("err=%v attempts=%d, want deadline and one request", err, attempts)
	}
}

func TestGetRetriesRateLimitAfterHeader(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()
	c := &Client{hc: server.Client()}
	if err := c.doJSON(context.Background(), http.MethodGet, server.URL, nil, nil); err != nil || attempts != 2 {
		t.Fatalf("err=%v attempts=%d, want successful second attempt", err, attempts)
	}
}

func TestGetStopsAfterBoundedServerFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c := &Client{hc: server.Client()}
	if err := c.doJSON(context.Background(), http.MethodGet, server.URL, nil, nil); err == nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d, want error after three attempts", err, attempts)
	}
}

func TestMutationDoesNotRetryTransientFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c := &Client{hc: server.Client()}
	if err := c.doJSON(context.Background(), http.MethodPost, server.URL, map[string]string{"a": "b"}, nil); err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d, want error and no replay", err, attempts)
	}
}
