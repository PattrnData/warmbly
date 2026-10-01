package sendauth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type lookupFunc func(context.Context, Request) (Snapshot, error)

func (f lookupFunc) Lookup(ctx context.Context, r Request) (Snapshot, error) { return f(ctx, r) }

func TestSendAuthorizationFailsClosedAndBindsTask(t *testing.T) {
	account, org, worker, task := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	valid := Request{TaskID: task, EmailAccountID: account, OrganizationID: org, WorkerID: worker, MessageID: "msg", From: "sender@example.com", Provider: "gmail"}
	service := Service{Repository: lookupFunc(func(_ context.Context, r Request) (Snapshot, error) {
		if r != valid {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{Provider: "gmail", From: "sender@example.com", TaskType: "campaign"}, nil
	})}
	if !service.Allowed(context.Background(), valid) {
		t.Fatal("active bound task denied")
	}
	for _, mutate := range []func(*Request){func(r *Request) { r.TaskID = uuid.New() }, func(r *Request) { r.EmailAccountID = uuid.New() }, func(r *Request) { r.OrganizationID = uuid.New() }, func(r *Request) { r.WorkerID = uuid.New() }, func(r *Request) { r.MessageID = "other" }, func(r *Request) { r.From = "other@example.com" }, func(r *Request) { r.Provider = "outlook" }, func(r *Request) { r.IsWarmup = true }} {
		r := valid
		mutate(&r)
		if service.Allowed(context.Background(), r) {
			t.Fatalf("changed binding allowed: %+v", r)
		}
	}
	service.Repository = lookupFunc(func(context.Context, Request) (Snapshot, error) { return Snapshot{}, errors.New("db unavailable") })
	if service.Allowed(context.Background(), valid) {
		t.Fatal("database outage allowed")
	}
	service.Repository = nil
	if service.Allowed(context.Background(), valid) {
		t.Fatal("missing repository allowed")
	}
}
