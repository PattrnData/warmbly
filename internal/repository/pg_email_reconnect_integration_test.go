package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/encrypt"
)

// TEST_DATABASE_URL must target a disposable PostgreSQL database.
func TestReconnectOutlookCredentialsPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
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
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	_, err = pool.Exec(ctx, `CREATE TABLE email_accounts (id uuid PRIMARY KEY, user_id text, organization_id uuid, email text, provider text, status text, sync_state text);
 CREATE TABLE email_accounts_oauth (email_account_id uuid PRIMARY KEY REFERENCES email_accounts(id), access_token text, refresh_token text, expires_at timestamptz)`)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := encrypt.NewEncrypter([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	seal := func(s string) string {
		t.Helper()
		v, e := enc.Encrypt(s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	id, user, org := uuid.New(), "user-one", uuid.New()
	oldAccess, oldRefresh := seal("old-access"), seal("old-refresh")
	oldExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	_, err = pool.Exec(ctx, `INSERT INTO email_accounts VALUES ($1,$2,$3,'Owner@Example.com','outlook','active','paused')`, id, user, org)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO email_accounts_oauth VALUES ($1,$2,$3,$4)`, id, oldAccess, oldRefresh, oldExpiry)
	if err != nil {
		t.Fatal(err)
	}
	repo := &emailRepository{DB: &db.DB{Pool: pool}, Encrypt: enc}
	expiry := oldExpiry.Add(2 * time.Hour)
	attempt := func(account uuid.UUID, owner string, tenant uuid.UUID, email, prior string) bool {
		return repo.ReconnectOutlookCredentials(ctx, account, owner, tenant, email, prior, "new-access", "new-refresh", expiry) == nil
	}
	for name, input := range map[string]struct {
		id           uuid.UUID
		user         string
		org          uuid.UUID
		email, prior string
	}{
		"account":     {uuid.New(), user, org, "Owner@Example.com", "old-refresh"},
		"user":        {id, "other-user", org, "Owner@Example.com", "old-refresh"},
		"tenant":      {id, user, uuid.New(), "Owner@Example.com", "old-refresh"},
		"email":       {id, user, org, "other@example.com", "old-refresh"},
		"stale token": {id, user, org, "Owner@Example.com", "stale-refresh"},
	} {
		t.Run(name, func(t *testing.T) {
			if attempt(input.id, input.user, input.org, input.email, input.prior) {
				t.Fatal("unsafe reconnect succeeded")
			}
			var a, r string
			var e time.Time
			if err := pool.QueryRow(ctx, "SELECT access_token,refresh_token,expires_at FROM email_accounts_oauth WHERE email_account_id=$1", id).Scan(&a, &r, &e); err != nil || a != oldAccess || r != oldRefresh || !e.Equal(oldExpiry) {
				t.Fatal("rejected reconnect changed credentials", err)
			}
		})
	}
	// A stale service read cannot override a concurrently disabled or
	// provider-converted account. A rejected transaction leaves both tokens.
	for _, state := range []struct{ column, value, original string }{
		{"status", "inactive", "active"},
		{"provider", "gmail", "outlook"},
	} {
		if _, err := pool.Exec(ctx, "UPDATE email_accounts SET "+state.column+"=$1 WHERE id=$2", state.value, id); err != nil {
			t.Fatal(err)
		}
		if attempt(id, user, org, "owner@example.com", "old-refresh") {
			t.Fatalf("stale read accepted %s=%s", state.column, state.value)
		}
		var stored string
		if err := pool.QueryRow(ctx, "SELECT refresh_token FROM email_accounts_oauth WHERE email_account_id=$1", id).Scan(&stored); err != nil || stored != oldRefresh {
			t.Fatalf("rejected %s change failed to roll back: %v", state.column, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE email_accounts SET "+state.column+"=$1 WHERE id=$2", state.original, id); err != nil {
			t.Fatal(err)
		}
	}
	if !attempt(id, user, org, "owner@example.com", "old-refresh") {
		t.Fatal("valid reconnect rejected")
	}
	var a, r, state string
	var e time.Time
	if err := pool.QueryRow(ctx, `SELECT o.access_token,o.refresh_token,o.expires_at,x.sync_state FROM email_accounts_oauth o JOIN email_accounts x ON x.id=o.email_account_id WHERE x.id=$1`, id).Scan(&a, &r, &e, &state); err != nil {
		t.Fatal(err)
	}
	if a == "new-access" || r == "new-refresh" || a == oldAccess || r == oldRefresh || !e.Equal(expiry) || state != "paused" {
		t.Fatal("reconnect failed to seal new credentials or preserve state")
	}
	if plain, err := enc.Decrypt(r); err != nil || plain != "new-refresh" {
		t.Fatal("new refresh token cannot be decrypted")
	}
	if attempt(id, user, org, "owner@example.com", "old-refresh") {
		t.Fatal("replay accepted")
	}
	var after string
	if err := pool.QueryRow(ctx, "SELECT refresh_token FROM email_accounts_oauth WHERE email_account_id=$1", id).Scan(&after); err != nil || after != r {
		t.Fatal("replay changed credentials", err)
	}
	app := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO email_accounts VALUES ($1,$2,$3,'app@example.com','outlook','active','paused')`, app, user, org)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO email_accounts_oauth VALUES ($1,$2,$3,$4)`, app, oldAccess, seal(models.GraphAppOnlyRefreshToken), oldExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if attempt(app, user, org, "app@example.com", models.GraphAppOnlyRefreshToken) {
		t.Fatal("app-only sentinel accepted")
	}
}
