package wmail

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/models"
)

// SyncGraph walks the Microsoft Graph delta stream for the mailbox and returns
// any incomplete pass or failed success receipt to the mailbox retry loop.
func (w *WMail) SyncGraph(ctx context.Context) error {
	startedAt := time.Now().UTC()
	if err := w.GraphData.Client.Sync(ctx); err != nil {
		return err
	}
	if err := w.onEvent(models.JobEventTypeMailboxProviderSync, &models.JobEventMailboxProviderSync{EmailID: w.ID, StartedAt: startedAt}); err != nil {
		w.CaptureError(err)
		return err
	}
	return nil
}
