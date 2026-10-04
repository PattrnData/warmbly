package wmail

import (
	"context"
	"errors"
	"time"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// SyncGraph walks the Microsoft Graph delta stream for the mailbox. It is the
// Graph analogue of SyncGoogle: a critical MailError (auth/disabled) is returned
// so the sync loop stops and the account is flagged for re-auth; anything else is
// captured and swallowed so a transient blip doesn't tear down the account.
func (w *WMail) SyncGraph(ctx context.Context) *errx.MailError {
	startedAt := time.Now().UTC()
	if err := w.GraphData.Client.Sync(ctx); err != nil {
		var mailErr *errx.MailError
		if errors.As(err, &mailErr) {
			return mailErr
		}
		w.CaptureError(err)
		return nil
	}
	if err := w.onEvent(models.JobEventTypeMailboxProviderSync, &models.JobEventMailboxProviderSync{EmailID: w.ID, StartedAt: startedAt}); err != nil {
		w.CaptureError(err)
	}
	return nil
}
