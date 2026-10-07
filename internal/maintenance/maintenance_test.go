package maintenance

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFailClosed(t *testing.T) {
	for _, tc := range []struct {
		mode, scope     string
		active, invalid bool
	}{
		{"", "", false, false},
		{"health-only", "host", true, false},
		{"health-only", "org", false, true},
		{"health-only", "", false, true},
		{"off", "host", false, true},
		{"other", "host", false, true},
		{"", "host", false, true},
	} {
		active, err := Parse(tc.mode, tc.scope)
		if active != tc.active || (err != nil) != tc.invalid {
			t.Errorf("Parse(%q,%q) = %v,%v", tc.mode, tc.scope, active, err)
		}
	}
}

func TestHealthOnlyRoutes(t *testing.T) {
	h := HealthOnlyHandler()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", http.StatusOK},
		{"HEAD", "/health", http.StatusOK},
		{"POST", "/health", http.StatusNotFound},
		{"POST", "/api/v1/tasks", http.StatusNotFound},
		{"GET", "/api/v1/emails", http.StatusNotFound},
		{"GET", "/metrics", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s %s = %d want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
}
