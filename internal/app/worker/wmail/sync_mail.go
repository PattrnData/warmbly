package wmail

import (
	"context"

	"github.com/warmbly/warmbly/internal/models"
)

func (w *WMail) SyncMail(ctx context.Context) error {
	switch w.EmailType {
	case models.InboxProviderGoogle:
		return w.SyncGoogle(ctx)
	case models.InboxProviderOutlook:
		return w.SyncGraph(ctx)
	case models.InboxProviderSMTPIMAP:
		return w.Sync(ctx)
	default:
		return nil
	}
}
