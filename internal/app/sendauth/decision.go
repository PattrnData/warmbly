// Package sendauth checks a queued send against current database state.
package sendauth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("send binding not found")

type Request struct {
	TaskID         uuid.UUID `json:"task_id"`
	EmailAccountID uuid.UUID `json:"email_account_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	WorkerID       uuid.UUID `json:"worker_id"`
	MessageID      string    `json:"message_id"`
	PayloadHash    string    `json:"payload_hash"`
	From           string    `json:"from"`
	Provider       string    `json:"provider"`
	IsWarmup       bool      `json:"is_warmup"`
}
type Snapshot struct{ Provider, From, TaskType string }
type Repository interface {
	Claim(context.Context, Request) (Snapshot, error)
}
type Service struct{ Repository Repository }

func (s Service) Allowed(ctx context.Context, r Request) bool {
	if s.Repository == nil || r.TaskID == uuid.Nil || r.EmailAccountID == uuid.Nil || r.OrganizationID == uuid.Nil || r.WorkerID == uuid.Nil || strings.TrimSpace(r.MessageID) == "" || len(r.MessageID) > 512 || len(r.PayloadHash) != 64 || strings.TrimSpace(r.From) == "" || r.Provider == "" {
		return false
	}
	snap, err := s.Repository.Claim(ctx, r)
	return err == nil && strings.EqualFold(snap.From, r.From) && snap.Provider == r.Provider && r.IsWarmup == (snap.TaskType == "warmup")
}
