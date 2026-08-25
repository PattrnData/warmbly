package jobs

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/infrastructure/pubsub"
	"github.com/warmbly/warmbly/internal/models"
)

const warmupDeletionTamperingGrace = 2 * time.Hour

// HandleRemoveEmail processes a message removal observed during mailbox sync.
//
// Tampering protection: if the removed message was a warmup email (tracked in
// warmup_received), the recipient deleted pool warmup mail — that harms the
// pool, so we record a tampering strike against the mailbox and ban it from
// warmup once the threshold is crossed. The owner can appeal.
//
// It also drops the local unibox entry for the removed message (best-effort).
func (s *JobsService) HandleRemoveEmail(ctx context.Context, e *models.JobEventRemoveEmail) error {
	if s.WarmupRepo != nil {
		if rec, _ := s.WarmupRepo.GetWarmupReceived(ctx, e.EmailID, e.ID); rec != nil {
			if shouldRecordWarmupDeletionTampering(rec.CreatedAt, time.Now()) {
				if s.WarmupService != nil {
					health, _ := s.WarmupService.RecordTampering(ctx, e.EmailID, rec.MessageID, "deletion")
					s.markRiskBandFromWarmupHealth(ctx, e.EmailID, health)
				}
			} else {
				log.Info().
					Str("email_id", e.EmailID.String()).
					Str("message_id", rec.MessageID).
					Dur("age", time.Since(rec.CreatedAt)).
					Msg("Skipping warmup deletion tampering: recent provider removal is expected after Warmbly folder move")
			}
		}
	}

	if s.UniboxRepository != nil {
		_ = s.UniboxRepository.Delete(ctx, e.UserID, e.ID)
	}

	// Tell open dashboards the row is gone (org-scoped so every teammate's
	// unibox drops it live, not just the mailbox owner's).
	if s.StreamingPublisher != nil {
		var orgID string
		if account, err := s.EmailRepository.GetByID(ctx, e.EmailID); err == nil && account != nil && account.OrganizationID != nil {
			orgID = account.OrganizationID.String()
		}
		s.StreamingPublisher.PublishEmailDeleted(ctx, &pubsub.EmailInboxEvent{
			BaseEvent:      pubsub.BaseEvent{UserID: e.UserID.String()},
			OrgID:          orgID,
			EmailAccountID: e.EmailID.String(),
			MessageID:      e.ID.String(),
		})
	}
	return nil
}

func shouldRecordWarmupDeletionTampering(receivedAt, now time.Time) bool {
	if receivedAt.IsZero() {
		return true
	}
	if now.Before(receivedAt) {
		return false
	}
	return now.Sub(receivedAt) > warmupDeletionTamperingGrace
}
