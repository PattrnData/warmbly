package wmail

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

func TestAuthAlertPublishFailureQuarantinesUntilReplayed(t *testing.T) {
	var attempts, terminations, providerCalls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}, func(kind models.JobEventType, _ any) error {
		if kind != models.JobEventTypeEmailAuthError {
			t.Errorf("unexpected event %s", kind)
		}
		if attempts.Add(1) == 1 {
			return errors.New("event bus unavailable")
		}
		return nil
	})
	defer closeServer()
	mail.Ctx = ctx
	mail.Cancel = cancel
	mail.TerminateFunc = func() { terminations.Add(1) }
	if mail.syncOnce(ctx) || mail.pendingSyncAlert == nil {
		t.Fatal("auth warning must remain pending")
	}
	if ctx.Err() != nil || terminations.Load() != 0 {
		t.Fatal("quarantined mailbox must remain available for alert replay")
	}
	if result := mail.Send(context.Background(), &SendRequest{TaskID: uuid.New(), To: []string{"test@example.com"}}); result.Success || result.Error == nil {
		t.Fatal("quarantined mailbox sent mail")
	}
	if providerCalls.Load() != 1 {
		t.Fatal("send called provider after auth quarantine")
	}
	if mail.syncOnce(ctx) {
		t.Fatal("auth alert replay must not count as a sync success")
	}
	if providerCalls.Load() != 1 || attempts.Load() != 2 || terminations.Load() != 1 || ctx.Err() == nil || mail.pendingSyncAlert != nil {
		t.Fatalf("provider=%d alerts=%d terminations=%d cancelled=%v pending=%v", providerCalls.Load(), attempts.Load(), terminations.Load(), ctx.Err(), mail.pendingSyncAlert)
	}
	if mail.syncOnce(ctx) || providerCalls.Load() != 1 {
		t.Fatal("terminated mailbox synced again")
	}
	other, closeOther := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"cursor"}`))
	}, func(models.JobEventType, any) error { return nil })
	defer closeOther()
	if !other.syncOnce(context.Background()) {
		t.Fatal("other mailbox was blocked")
	}
}

func TestAuthAlertRetryLoopStopsOnlyAfterPublish(t *testing.T) {
	var attempts, providerCalls, terminations atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mail, closeServer := graphSyncFixture(t, func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}, func(kind models.JobEventType, _ any) error {
		if kind != models.JobEventTypeEmailAuthError {
			t.Errorf("unexpected event %s", kind)
		}
		if attempts.Add(1) < 3 {
			return errors.New("event bus unavailable")
		}
		return nil
	})
	defer closeServer()
	mail.Ctx = ctx
	mail.Cancel = cancel
	mail.TerminateFunc = func() { terminations.Add(1) }
	mail.runSyncLoop(ctx, time.Millisecond)
	if ctx.Err() != context.Canceled || providerCalls.Load() != 1 || attempts.Load() != 3 || terminations.Load() != 1 {
		t.Fatalf("ctx=%v provider=%d alerts=%d terminations=%d", ctx.Err(), providerCalls.Load(), attempts.Load(), terminations.Load())
	}
}
