package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
)

// Run with TEST_DATABASE_URL pointing at a disposable PostgreSQL database.
func TestResolveConnectionWarningsPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1 // Temporary table must remain on the same connection.
	cfg.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE email_account_errors (
 id uuid DEFAULT gen_random_uuid(), email_account_id uuid NOT NULL, user_id uuid NOT NULL,
 error_code text NOT NULL, severity text NOT NULL, resolve_method text NOT NULL,
 title text NOT NULL, message text NOT NULL, user_message text, action_required text,
 task_id uuid, resolved_at timestamptz, resolved_by text, created_at timestamptz DEFAULT NOW()
 )`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../infrastructure/db/migrations/000084_email_error_occurred_at.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	repo := NewEmailAccountErrorRepository(&db.DB{Pool: pool})
	mailbox, otherMailbox, user, sendTask := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	start := time.Now().UTC().Add(-time.Hour)
	create := func(account uuid.UUID, code, severity, method string, taskID *uuid.UUID, at time.Time) uuid.UUID {
		t.Helper()
		row, xerr := repo.Create(ctx, &CreateEmailAccountError{EmailAccountID: account, UserID: user, ErrorCode: code, Severity: severity, ResolveMethod: method, Title: code, Message: code, TaskID: taskID, OccurredAt: at})
		if xerr != nil {
			t.Fatalf("create %s: %v", code, xerr)
		}
		return row.ID
	}
	old := create(mailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", nil, start.Add(-time.Minute))
	other := create(otherMailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", nil, start.Add(-time.Minute))
	send := create(mailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", &sendTask, start.Add(-time.Minute))
	if send == old {
		t.Fatal("send and sync warnings must not deduplicate into one row")
	}
	auth := create(mailbox, "AUTH_FAILED", "CRITICAL", "OAUTH", nil, start.Add(-time.Minute))
	if xerr := repo.ResolveConnectionWarnings(ctx, mailbox, start); xerr != nil {
		t.Fatal(xerr)
	}
	assertResolved := func(id uuid.UUID, want bool) {
		t.Helper()
		var got bool
		if err := pool.QueryRow(ctx, "SELECT resolved_at IS NOT NULL FROM email_account_errors WHERE id=$1", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("row %s resolved=%t, want %t", id, got, want)
		}
	}
	assertResolved(old, true) // Ordinary successful sync recovers the older warning.
	for _, id := range []uuid.UUID{other, send, auth} {
		assertResolved(id, false)
	}
	// Warning W was stored after success S but S is consumed again (replay).
	newWarning := create(mailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", nil, start.Add(time.Minute))
	if old == newWarning {
		t.Fatal("new warning must not be folded into a resolved warning")
	}
	if xerr := repo.ResolveConnectionWarnings(ctx, mailbox, start); xerr != nil {
		t.Fatal(xerr)
	}
	assertResolved(newWarning, false)
	// A repeated warning refreshes occurrence time rather than inheriting the original time.
	if duplicate := create(mailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", nil, start.Add(3*time.Minute)); duplicate != newWarning {
		t.Fatal("unresolved warning was not deduplicated")
	}
	if xerr := repo.ResolveConnectionWarnings(ctx, mailbox, start.Add(2*time.Minute)); xerr != nil {
		t.Fatal(xerr)
	}
	assertResolved(newWarning, false)
	// Only a later complete pass can recover W.
	if xerr := repo.ResolveConnectionWarnings(ctx, mailbox, start.Add(4*time.Minute)); xerr != nil {
		t.Fatal(xerr)
	}
	assertResolved(newWarning, true)
	for _, id := range []uuid.UUID{other, send, auth} {
		assertResolved(id, false)
	}
}
