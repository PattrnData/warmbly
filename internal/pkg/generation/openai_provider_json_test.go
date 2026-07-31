package generation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompleteJSONModeRetriesWhenResponseFormatUnsupported(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if attempts == 1 {
			if body["response_format"] == nil {
				t.Fatalf("first request missing response_format: %#v", body)
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"unsupported response_format","type":"invalid_request_error","param":"response_format"}}`))
			return
		}
		if body["response_format"] != nil {
			t.Fatalf("retry should omit response_format after compatibility error: %#v", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"class\":\"question\",\"confidence\":0.8}"}}],"usage":{"total_tokens":7}}`))
	}))
	defer srv.Close()

	p := newOpenAIProvider(ProviderConfig{OpenAIAPIKey: "test", OpenAIBaseURL: srv.URL, OpenAIModelTrial: "test-model"})
	got, err := p.Complete(context.Background(), CompletionRequest{System: "json only", Prompt: "classify", JSONMode: true})
	if err != nil {
		t.Fatalf("Complete JSONMode retry returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if got.Text != `{"class":"question","confidence":0.8}` {
		t.Fatalf("text = %q", got.Text)
	}
	if got.TokensUsed != 7 {
		t.Fatalf("tokens = %d, want 7", got.TokensUsed)
	}
}
