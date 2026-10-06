package wmail

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
)

// StartSyncWorker runs a periodic mail sync loop until the context is cancelled.
// It dispatches to the right provider (Google history-based or IMAP poll-based)
// and keeps the inbox up to date by emitting JobEventTypeNewEmail and other
// events whenever changes are detected on the upstream mail server.
func (w *WMail) StartSyncWorker(ctx context.Context) {
	interval := ImapCheckInterval
	if w.EmailType == models.InboxProviderGoogle {
		// Google polling can be slightly slower because the API is more efficient
		// and rate-limited.
		interval = 1 * time.Minute
	}

	w.runSyncLoop(ctx, interval)
}

// syncRetryInterval doubles failed Outlook poll intervals, capped per mailbox.
func syncRetryInterval(base time.Duration, failures int) time.Duration {
	delay := base
	for i := 0; i < failures; i++ {
		if delay >= 15*time.Minute/2 {
			return 15 * time.Minute
		}
		delay *= 2
	}
	return delay
}

func (w *WMail) runSyncLoop(ctx context.Context, interval time.Duration) {
	failures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		if w.syncOnce(ctx) {
			failures = 0
		} else if w.EmailType == models.InboxProviderOutlook {
			failures++
		}
		// A canceled auth mailbox must not begin another sync pass.
		if ctx.Err() != nil {
			return
		}
		delay := interval
		if w.EmailType == models.InboxProviderOutlook {
			delay = syncRetryInterval(interval, failures)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// syncOnce runs one sync pass, containing panics: the worker is multi-tenant,
// so one mailbox's bad server response must not take down every other
// account's sync and send loops.
func (w *WMail) syncOnce(ctx context.Context) (success bool) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("mail sync panic: %v", r)
			_ = w.CaptureError(err)
			log.Error().Err(err).Str("email_id", w.ID.String()).Msg("mail sync panicked")
			success = false
		}
	}()
	if w.pendingSyncAlert != nil {
		if err := w.CaptureError(w.pendingSyncAlert); err != nil {
			log.Warn().Err(err).Str("email_id", w.ID.String()).Msg("mail alert publish retry failed")
			return false
		}
		w.pendingSyncAlert = nil
		if ctx.Err() != nil {
			return false
		}
	}
	if err := w.SyncMail(ctx); err != nil {
		if publishErr := w.CaptureError(err); publishErr != nil {
			w.pendingSyncAlert = err
			log.Warn().Err(publishErr).Str("email_id", w.ID.String()).Msg("mail alert publish failed")
		}
		log.Warn().Err(err).Str("email_id", w.ID.String()).Msg("mail sync error")
		return false
	}
	return true
}
