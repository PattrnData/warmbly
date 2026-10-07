package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// HandleMailboxProviderSync resolves transient warnings after a complete mailbox sync.
// App-only Graph access does not prove delegated authentication is healthy.
func (s *JobsService) HandleMailboxProviderSync(ctx context.Context, e *models.JobEventMailboxProviderSync) error {
	if e == nil || e.EmailID == uuid.Nil || e.StartedAt.IsZero() {
		return fmt.Errorf("provider sync event has no mailbox id or pass start")
	}
	if s.EmailAccountErrorRepository == nil {
		return nil
	}
	if err := s.EmailAccountErrorRepository.ResolveConnectionWarnings(ctx, e.EmailID, e.StartedAt); err != nil {
		return fmt.Errorf("resolve mailbox connection warning: %s", err.Message)
	}
	return nil
}
