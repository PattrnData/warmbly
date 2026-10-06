package wmail

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/client/msgraph"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"golang.org/x/oauth2"
)

func graphSyncFixture(t *testing.T, handler http.HandlerFunc, publish func(models.JobEventType, any) error) (*WMail, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	client := &msgraph.Client{DeltaLinks: map[string]string{
		msgraph.FolderInbox: server.URL + "/inbox",
		msgraph.FolderJunk:  server.URL + "/junk",
	}, OnTokenRefresh: func(context.Context, *oauth2.Token) error { return nil }}
	if err := client.Init(context.Background(), &oauth2.Token{AccessToken: "test", Expiry: time.Now().Add(time.Hour)}, oauth2.Config{}); err != nil {
		server.Close()
		t.Fatal(err)
	}
	return &WMail{ID: uuid.New(), UserID: uuid.New(), EmailType: models.InboxProviderOutlook, GraphData: &GraphData{Client: client}, onEvent: publish}, server.Close
}

func TestGraphSyncFailureIsNotSuccess(t *testing.T) {
	var success atomic.Int32
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}, func(kind models.JobEventType, _ any) error {
		if kind == models.JobEventTypeMailboxProviderSync {
			success.Add(1)
		}
		return nil
	})
	defer closeServer()
	if err := mail.SyncGraph(context.Background()); err == nil {
		t.Fatal("transient Graph failure must propagate")
	}
	if success.Load() != 0 {
		t.Fatal("failed pass emitted success")
	}
}

func TestGraphSyncPublishFailureIsNotSuccess(t *testing.T) {
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"cursor"}`))
	}, func(kind models.JobEventType, _ any) error { return errors.New("event bus unavailable") })
	defer closeServer()
	if err := mail.SyncGraph(context.Background()); err == nil {
		t.Fatal("failed success receipt must propagate")
	}
}

func TestSyncRetryBackoffAndReset(t *testing.T) {
	base := 10 * time.Millisecond
	for _, tc := range []struct {
		failures int
		want     time.Duration
	}{
		{0, base}, {1, 2 * base}, {2, 4 * base}, {3, 8 * base}, {99, 15 * time.Minute},
	} {
		if got := syncRetryInterval(base, tc.failures); got != tc.want {
			t.Fatalf("failures %d: got %s want %s", tc.failures, got, tc.want)
		}
	}
	var requests atomic.Int32
	var successes atomic.Int32
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"cursor"}`))
	}, func(kind models.JobEventType, _ any) error {
		if kind == models.JobEventTypeMailboxProviderSync {
			successes.Add(1)
		}
		return nil
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go mail.runSyncLoop(ctx, base)
	for successes.Load() == 0 && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	if successes.Load() == 0 {
		t.Fatal("mailbox did not recover after transient failure")
	}
}

func TestSyncFailedAlertPublishRetriedBeforeProviderSuccess(t *testing.T) {
	var requests, attempts, successes atomic.Int32
	failProvider := true
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if failProvider {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"cursor"}`))
	}, func(kind models.JobEventType, _ any) error {
		if kind == models.JobEventTypeEmailServerError {
			if attempts.Add(1) == 1 {
				return errors.New("event bus unavailable")
			}
		}
		if kind == models.JobEventTypeMailboxProviderSync {
			successes.Add(1)
		}
		return nil
	})
	defer closeServer()
	if mail.syncOnce(context.Background()) {
		t.Fatal("failed warning publish must not count as success")
	}
	firstRequests := requests.Load()
	if firstRequests == 0 || successes.Load() != 0 || mail.pendingSyncAlert == nil {
		t.Fatal("failed warning publish must remain pending")
	}
	failProvider = false
	if !mail.syncOnce(context.Background()) {
		t.Fatal("retry should publish warning then perform a complete sync")
	}
	if requests.Load() <= firstRequests {
		t.Fatal("provider did not resume after warning publish")
	}
	if attempts.Load() != 2 || successes.Load() != 1 {
		t.Fatalf("warning attempts %d, success %d", attempts.Load(), successes.Load())
	}
}

func TestSyncMailboxesIsolatedAndAuthStops(t *testing.T) {
	var cancelled atomic.Bool
	auth := errx.MError(errx.MailErrorCritical, errx.MailErrorCodeAuthenticationFailed, "unauthorized", errx.MailErrorResolveMethodReload)
	ctx, cancel := context.WithCancel(context.Background())
	mail := &WMail{ID: uuid.New(), UserID: uuid.New(), pendingSyncAlert: auth, Cancel: func() { cancelled.Store(true); cancel() }, onEvent: func(models.JobEventType, any) error { return nil }}
	if mail.syncOnce(ctx) {
		t.Fatal("auth failure should stop sync")
	}
	if !cancelled.Load() {
		t.Fatal("auth failure did not cancel mailbox")
	}
	var events atomic.Int32
	other, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"cursor"}`))
	}, func(kind models.JobEventType, _ any) error {
		if kind == models.JobEventTypeMailboxProviderSync {
			events.Add(1)
		}
		return nil
	})
	defer closeServer()
	if !other.syncOnce(context.Background()) || events.Load() != 1 {
		t.Fatal("another mailbox must remain runnable")
	}
}

func TestGraphAuthFailureStopsOnlyItsMailbox(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var authEvents atomic.Int32
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}, func(kind models.JobEventType, _ any) error {
		if kind == models.JobEventTypeEmailAuthError {
			authEvents.Add(1)
		}
		return nil
	})
	defer closeServer()
	mail.Cancel = cancel
	mail.runSyncLoop(ctx, time.Millisecond)
	if ctx.Err() == nil || authEvents.Load() != 1 {
		t.Fatalf("auth cancel = %v, auth events = %d", ctx.Err(), authEvents.Load())
	}
}
