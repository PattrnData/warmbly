package sendauth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the actual atomic claim on a disposable local PostgreSQL instance.
// The test never connects to a non-loopback database.
func TestPGClaimLifecycle(t *testing.T) {
	url := os.Getenv("SEND_AUTH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SEND_AUTH_TEST_DATABASE_URL to a disposable localhost PostgreSQL")
	}
	if !strings.Contains(url, "127.0.0.1:") && !strings.Contains(url, "localhost:") {
		t.Fatal("integration database must be localhost")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "sendauth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ddl := []string{
		`CREATE TABLE workers (id uuid PRIMARY KEY, active boolean NOT NULL)`,
		`CREATE TABLE email_accounts (id uuid PRIMARY KEY, worker_id uuid, organization_id uuid, provider text, email text, status text, warmup boolean, warmup_paused_at timestamptz)`,
		`CREATE TABLE email_accounts_oauth (email_account_id uuid PRIMARY KEY)`,
		`CREATE TABLE email_accounts_smtp_imap (email_account_id uuid PRIMARY KEY)`,
		`CREATE TABLE tasks (id uuid PRIMARY KEY, email_account_id uuid, message_id text, status text, task_type text)`,
		`CREATE TABLE campaigns (id uuid PRIMARY KEY, organization_id uuid, status text)`,
		`CREATE TABLE campaign_tasks (task_id uuid, campaign_id uuid)`,
		`CREATE TABLE warmup_tasks (task_id uuid)`,
		`CREATE TABLE email_tasks (task_id uuid)`,
	}
	for _, sql := range ddl {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../infrastructure/db/migrations/000084_send_attempt_claim.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	worker, org, account, task := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workers VALUES ($1,true)`, []any{worker}},
		{`INSERT INTO email_accounts (id,worker_id,organization_id,provider,email,status,warmup) VALUES ($1,$2,$3,'gmail','sender@example.org','active',true)`, []any{account, worker, org}},
		{`INSERT INTO email_accounts_oauth VALUES ($1)`, []any{account}},
		{`INSERT INTO tasks VALUES ($1,$2,'msg-1','completed','email')`, []any{task, account}},
		{`INSERT INTO email_tasks VALUES ($1)`, []any{task}},
	} {
		if _, err := pool.Exec(ctx, query.sql, query.args...); err != nil {
			t.Fatal(err)
		}
	}
	req := Request{TaskID: task, EmailAccountID: account, OrganizationID: org, WorkerID: worker, MessageID: "msg-1", From: "sender@example.org", Provider: "gmail"}
	service := Service{Repository: PGRepository{Pool: pool}}
	for _, bad := range []Request{
		func() Request { r := req; r.MessageID = "wrong"; return r }(),
		func() Request { r := req; r.From = "other@example.org"; return r }(),
		func() Request { r := req; r.IsWarmup = true; return r }(),
		func() Request { r := req; r.Provider = "outlook"; return r }(),
	} {
		if service.Allowed(ctx, bad) {
			t.Fatal("invalid binding allowed")
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE workers SET active=false WHERE id=$1`, worker); err != nil {
		t.Fatal(err)
	}
	if service.Allowed(ctx, req) {
		t.Fatal("inactive worker allowed")
	}
	if _, err := pool.Exec(ctx, `UPDATE workers SET active=true WHERE id=$1`, worker); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if service.Allowed(ctx, req) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("concurrent claim: got %d approvals, want 1", got)
	}
	if service.Allowed(ctx, req) {
		t.Fatal("replay allowed after completed task claim")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM send_attempt_claims WHERE task_id=$1 AND message_id=$2`, task, req.MessageID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal(fmt.Sprintf("persisted claims: %d", count))
	}
	// The other producers publish under different task types. Both require
	// their own live linkage, and a completed queued task is still claimable once.
	for _, kind := range []string{"warmup", "campaign"} {
		id := uuid.New()
		messageID := "msg-" + kind
		if _, err := pool.Exec(ctx, `INSERT INTO tasks VALUES ($1,$2,$3,'completed',$4)`, id, account, messageID, kind); err != nil {
			t.Fatal(err)
		}
		if kind == "warmup" {
			if _, err := pool.Exec(ctx, `INSERT INTO warmup_tasks VALUES ($1)`, id); err != nil {
				t.Fatal(err)
			}
		} else {
			campaign := uuid.New()
			if _, err := pool.Exec(ctx, `INSERT INTO campaigns VALUES ($1,$2,'active')`, campaign, org); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO campaign_tasks VALUES ($1,$2)`, id, campaign); err != nil {
				t.Fatal(err)
			}
		}
		r := req
		r.TaskID, r.MessageID, r.IsWarmup = id, messageID, kind == "warmup"
		if !service.Allowed(ctx, r) || service.Allowed(ctx, r) {
			t.Fatalf("%s should claim exactly once", kind)
		}
	}
}
