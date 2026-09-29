package repository

import (
	"context"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	pgdb "github.com/warmbly/warmbly/internal/infrastructure/db"
)

// Run against a disposable Postgres only: WARMBLY_WARMUP_DENY_TEST_DSN=...
func TestMailboxWarmupDenyDatabase(t *testing.T) {
	dsn := os.Getenv("WARMBLY_WARMUP_DENY_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable Postgres DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Path != "/warmup_deny_test" {
		t.Fatal("integration fixture requires local disposable database /warmup_deny_test")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixture := `CREATE TYPE task_status AS ENUM ('pending','active','cancelled');
CREATE TYPE task_type AS ENUM ('warmup','campaign');
CREATE TABLE email_accounts (id uuid PRIMARY KEY, status text DEFAULT 'active', worker_id uuid DEFAULT gen_random_uuid(), warmup timestamptz DEFAULT now(), warmup_paused_at timestamptz);
CREATE TABLE email_tags (email_id uuid, tag_id uuid);
CREATE TABLE campaign_email_tags (campaign_id uuid, tag_id uuid);
CREATE TABLE campaigns (id uuid PRIMARY KEY, status text);
CREATE TABLE warmup_pool_participants (pool_id uuid, email_account_id uuid, joined_at timestamptz, spam_score integer, participant_role text, PRIMARY KEY(pool_id,email_account_id));
CREATE TABLE tasks (id uuid PRIMARY KEY, task_type task_type, email_account_id uuid, status task_status, message_id text, scheduled_at timestamptz, created_at timestamptz, updated_at timestamptz);
CREATE TABLE warmup_tasks (task_id uuid, target_account_id uuid);`
	if _, err = db.Exec(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../infrastructure/db/migrations/000082_mailbox_warmup_deny.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	mailbox, pool := uuid.New(), uuid.New()
	if _, err = db.Exec(ctx, `INSERT INTO email_accounts(id) VALUES ($1)`, mailbox); err != nil {
		t.Fatal(err)
	}
	list := func() []uuid.UUID {
		ids, e := (&emailRepository{DB: &pgdb.DB{Pool: db}}).ListWarmupScheduleCandidates(ctx, 10)
		if e != nil {
			t.Fatal(e)
		}
		return ids
	}
	if ids := list(); len(ids) != 1 || ids[0] != mailbox {
		t.Fatalf("candidate before deny: %v", ids)
	}
	repo := NewWarmupRepository(db)
	tasks := NewTaskRepository(db)
	taskID := uuid.New()
	scheduleAt := time.Now()
	create := func() (bool, error) {
		return tasks.CreateWarmupTaskWithLock(ctx, &Task{ID: taskID, TaskType: "warmup", EmailAccountID: mailbox, Status: "pending", MessageID: "", ScheduledAt: &scheduleAt}, &WarmupTask{TaskID: taskID, TargetAccountID: &mailbox})
	}
	if err = repo.JoinPool(ctx, pool, mailbox); err != nil {
		t.Fatal(err)
	}
	if created, e := create(); e != nil || !created {
		t.Fatalf("initial task: %v %v", created, e)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, e := db.Exec(ctx, `UPDATE email_accounts SET warmup_denied=true WHERE id=$1`, mailbox)
		errs <- e
	}()
	go func() { defer wg.Done(); errs <- repo.JoinPool(ctx, pool, mailbox) }()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = repo.JoinPool(ctx, pool, mailbox); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if created, e := create(); e != nil || created {
			t.Fatalf("denied task attempt %d: %v %v", i, created, e)
		}
	}
	var membership, live int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM warmup_pool_participants WHERE email_account_id=$1`, mailbox).Scan(&membership); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE email_account_id=$1 AND status IN ('pending','active')`, mailbox).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if membership != 0 || live != 0 {
		t.Fatalf("denied mailbox: pool=%d liveTasks=%d", membership, live)
	}
	if ids := list(); len(ids) != 0 {
		t.Fatalf("denied candidate returned: %v", ids)
	}
	// Campaign health-check lane cannot resurrect scheduling, even with warmup disabled.
	if _, err = db.Exec(ctx, `UPDATE email_accounts SET warmup=NULL WHERE id=$1`, mailbox); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO campaigns VALUES ($1,'active')`, pool); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO email_tags VALUES ($1,$2)`, mailbox, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO campaign_email_tags VALUES ($1,$2)`, pool, taskID); err != nil {
		t.Fatal(err)
	}
	if ids := list(); len(ids) != 0 {
		t.Fatalf("denied campaign candidate returned: %v", ids)
	}
	if _, err = db.Exec(ctx, `UPDATE email_accounts SET warmup_denied=true WHERE id=$1`, mailbox); err != nil {
		t.Fatal(err)
	}
}
