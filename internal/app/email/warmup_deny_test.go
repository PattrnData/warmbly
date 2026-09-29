package email

import (
	"context"
	"testing"

	"github.com/google/uuid"
	warmupapp "github.com/warmbly/warmbly/internal/app/warmup"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

type deniedPoolService struct {
	warmupapp.Service
	removed []string
	joined  int
}

func (s *deniedPoolService) RemovePoolMembership(_ context.Context, _ uuid.UUID, pool string) *errx.Error {
	s.removed = append(s.removed, pool)
	return nil
}
func (s *deniedPoolService) EnsurePoolMembershipWithRole(_ context.Context, _ uuid.UUID, _, _ string) *errx.Error {
	s.joined++
	return nil
}
func TestDeniedMailboxSyncRemovesAllPoolsWithoutJoining(t *testing.T) {
	pool := &deniedPoolService{}
	service := &emailService{warmupService: pool}
	service.syncWarmupPoolMembership(context.Background(), &models.Email{ID: uuid.New(), Status: "active", WarmupDenied: true})
	if pool.joined != 0 || len(pool.removed) != 2 || pool.removed[0] != "premium" || pool.removed[1] != "free" {
		t.Fatalf("joined=%d removed=%v", pool.joined, pool.removed)
	}
}
