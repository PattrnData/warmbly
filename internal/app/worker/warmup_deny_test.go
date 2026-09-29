package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/worker/mailmanager"
	"github.com/warmbly/warmbly/internal/app/worker/wmail"
	"github.com/warmbly/warmbly/internal/infrastructure/codec"
	"github.com/warmbly/warmbly/internal/infrastructure/eventbus"
	"github.com/warmbly/warmbly/internal/models"
)

type denyTestBus struct {
	published int
	fail      bool
}

func (b *denyTestBus) Publish(_ context.Context, _, _ string, _ []byte) error {
	b.published++
	if b.fail {
		return errors.New("bus unavailable")
	}
	return nil
}
func (*denyTestBus) Subscribe(context.Context, []string, string, eventbus.Handler) error { return nil }
func (*denyTestBus) Close() error                                                        { return nil }
func (*denyTestBus) Name() string                                                        { return "test" }

type denyTestStore struct{ gets int }

func (s *denyTestStore) Get(context.Context, string) (io.ReadCloser, error) {
	s.gets++
	return nil, errors.New("fixture: body absent")
}
func (*denyTestStore) Put(context.Context, string, io.Reader, string) error { return nil }
func (*denyTestStore) PutPublic(context.Context, string, io.Reader, string) (string, error) {
	return "", nil
}
func (*denyTestStore) Delete(context.Context, string) error      { return nil }
func (*denyTestStore) Has(context.Context, string) (bool, error) { return false, nil }
func (*denyTestStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (*denyTestStore) Name() string { return "test" }

func TestWarmupDenyDispatch_ExactMailboxFailClosedAndCampaignBypass(t *testing.T) {
	deniedID, otherID := uuid.New(), uuid.New()
	token := "fixture-worker-token"
	calls := map[uuid.UUID]int{}
	unavailable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unauthenticated lookup")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(strings.TrimPrefix(r.URL.Path, "/api/v1/internal/warmup-deny/"))
		if err != nil {
			t.Errorf("bad path: %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls[id]++
		if unavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"email_id": id, "warmup_denied": id == deniedID})
	}))
	defer server.Close()
	checker, err := NewHTTPWarmupDenyChecker(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	bus, store := &denyTestBus{}, &denyTestStore{}
	worker := &WorkerService{WarmupDenyChecker: checker, Bus: bus, Codec: &codec.JSONCodec{}, Storage: store}
	worker.mailManager = mailmanager.NewMailManager(nil, nil, nil, nil, nil, nil)
	worker.mailManager.Emails[deniedID] = &wmail.WMail{}
	worker.mailManager.Emails[otherID] = &wmail.WMail{}
	send := func(id uuid.UUID, warmup bool) error {
		return worker.HandleSendEmail(context.Background(), models.SendEmail{TaskID: uuid.New(), EmailID: id, OrgID: uuid.New(), IsWarmup: warmup})
	}
	if err := send(deniedID, true); err != nil {
		t.Fatalf("denied command: %v", err)
	}
	if bus.published != 1 || store.gets != 0 {
		t.Fatalf("denied reached body/send: publishes=%d gets=%d", bus.published, store.gets)
	}
	unavailable = true
	if err := send(otherID, true); err != nil {
		t.Fatalf("unavailable must emit failure: %v", err)
	}
	if store.gets != 0 || bus.published != 2 {
		t.Fatal("unavailable warmup reached body or lost terminal event")
	}
	// An ordinary campaign command for the denied mailbox never consults the gate.
	if err := send(deniedID, false); err == nil || !strings.Contains(err.Error(), "fixture: body absent") {
		t.Fatalf("campaign should reach normal body fetch: %v", err)
	}
	if calls[deniedID] != 1 || calls[otherID] != 1 || store.gets != 1 {
		t.Fatalf("not per-mailbox: calls=%v body=%d", calls, store.gets)
	}
	unavailable = false
	if err := send(otherID, true); err == nil || !strings.Contains(err.Error(), "fixture: body absent") {
		t.Fatalf("other mailbox should pass gate: %v", err)
	}
	if store.gets != 2 {
		t.Fatal("other mailbox did not reach normal send pipeline")
	}
	bus.fail = true
	if err := send(deniedID, true); err == nil || err.Error() != "bus unavailable" {
		t.Fatalf("failed terminal publish must not ack: %v", err)
	}
}

func TestHTTPWarmupDenyChecker_RejectsUntrustedOrIncompleteStatus(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, ""}, {"missing", 404, ""}, {"database", 503, ""},
		{"malformed", 200, "not-json"}, {"missing flag", 200, fmt.Sprintf(`{"email_id":%q}`, id)},
		{"wrong mailbox", 200, fmt.Sprintf(`{"email_id":%q,"warmup_denied":false}`, uuid.New())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			checker, err := NewHTTPWarmupDenyChecker(server.URL, "token")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := checker.Denied(context.Background(), id); err == nil {
				t.Fatal("status must not authorize warmup")
			}
		})
	}
	if _, err := NewHTTPWarmupDenyChecker("", "token"); err == nil {
		t.Fatal("missing backend must fail startup")
	}
	if _, err := NewHTTPWarmupDenyChecker("https://backend", ""); err == nil {
		t.Fatal("missing token must fail startup")
	}
}
