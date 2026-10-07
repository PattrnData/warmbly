package repository

import (
	"context"
	"os"
	"strings"
	"sync"
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
	cfg.MaxConns = 8
	cfg.MinConns = 8
	// Isolate each run: never use an existing application's table/schema.
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TABLE email_account_errors (
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
	// Existing duplicate connection warnings must migrate without touching
	// auth warnings or task-bound send warnings.
	migrationAccount, migrationUser, migrationTask := uuid.New(), uuid.New(), uuid.New()
	for _, row := range []struct {
		code, severity, method string
		task                   *uuid.UUID
		occurred               time.Time
	}{
		{"SERVER_UNREACHABLE", "WARNING", "RETRY", nil, time.Now().Add(-2 * time.Hour)},
		{"SERVER_UNREACHABLE", "WARNING", "RETRY", nil, time.Now().Add(-time.Hour)},
		{"SERVER_UNREACHABLE", "WARNING", "RETRY", &migrationTask, time.Now().Add(-2 * time.Hour)},
		{"AUTH_FAILED", "CRITICAL", "OAUTH", nil, time.Now().Add(-2 * time.Hour)},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO email_account_errors (email_account_id,user_id,error_code,severity,resolve_method,title,message,task_id,occurred_at) VALUES ($1,$2,$3,$4,$5,$3,$3,$6,$7)`, migrationAccount, migrationUser, row.code, row.severity, row.method, row.task, row.occurred); err != nil {
			t.Fatal(err)
		}
	}
	concurrencyMigration, err := os.ReadFile("../infrastructure/db/migrations/000085_email_error_warning_atomic.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(concurrencyMigration)); err != nil {
		t.Fatal(err)
	}
	var active, preserved int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE error_code='SERVER_UNREACHABLE' AND task_id IS NULL AND resolved_at IS NULL), count(*) FILTER (WHERE (task_id IS NOT NULL OR error_code='AUTH_FAILED') AND resolved_at IS NULL) FROM email_account_errors WHERE email_account_id=$1`, migrationAccount).Scan(&active, &preserved); err != nil {
		t.Fatal(err)
	}
	if active != 1 || preserved != 2 {
		t.Fatalf("migration active connection=%d, unaffected=%d; want 1,2", active, preserved)
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
	// A fresh failure must survive an older success committing between its
	// read of the unresolved row and its attempted refresh.
	oldRace := create(mailbox, "SERVER_UNREACHABLE", "WARNING", "RETRY", nil, start.Add(5*time.Minute))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var blockerPID int
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE email_account_errors SET resolved_at=NOW() WHERE id=$1", oldRace); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		id  uuid.UUID
		err error
	}, 1)
	go func() {
		row, xerr := repo.Create(ctx, &CreateEmailAccountError{EmailAccountID: mailbox, UserID: user, ErrorCode: "SERVER_UNREACHABLE", Severity: "WARNING", ResolveMethod: "RETRY", Title: "SERVER_UNREACHABLE", Message: "SERVER_UNREACHABLE", OccurredAt: start.Add(7 * time.Minute)})
		var id uuid.UUID
		if row != nil {
			id = row.ID
		}
		var createErr error
		if xerr != nil {
			createErr = xerr
		}
		result <- struct {
			id  uuid.UUID
			err error
		}{id, createErr}
	}()
	// Wait for the attempted INSERT/UPDATE to be blocked by the old row.
	deadline := time.After(5 * time.Second)
	for {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)) AND query LIKE '%email_account_errors%'`, blockerPID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Create did not reach the locked row")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	raced := <-result
	if raced.err != nil {
		t.Fatal(raced.err)
	}
	if raced.id == oldRace {
		t.Fatal("new failure returned resolved row")
	}
	assertResolved(raced.id, false)
	// Concurrent first failures must leave exactly one unresolved warning.
	if _, err := pool.Exec(ctx, "UPDATE email_account_errors SET resolved_at=NOW() WHERE email_account_id=$1 AND error_code='SERVER_UNREACHABLE'", mailbox); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			if _, xerr := repo.Create(ctx, &CreateEmailAccountError{EmailAccountID: mailbox, UserID: user, ErrorCode: "SERVER_UNREACHABLE", Severity: "WARNING", ResolveMethod: "RETRY", Title: "SERVER_UNREACHABLE", Message: "SERVER_UNREACHABLE", OccurredAt: start.Add(8 * time.Minute)}); xerr != nil {
				t.Errorf("concurrent Create: %v", xerr)
			}
		}()
	}
	close(gate)
	wg.Wait()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM email_account_errors WHERE email_account_id=$1 AND error_code='SERVER_UNREACHABLE' AND resolved_at IS NULL", mailbox).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent warnings: got %d unresolved, want 1", count)
	}
}
