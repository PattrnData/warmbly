package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/repository"
)

// The URL must name a disposable localhost PostgreSQL instance, never a live database.
func TestSignedEscrowConversionAndReadbackPostgres(t *testing.T) {
	endpoint := os.Getenv("OUTLOOK_CONVERT_TEST_DATABASE_URL")
	if endpoint == "" {
		t.Skip("set OUTLOOK_CONVERT_TEST_DATABASE_URL to disposable localhost PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "localhost" {
		t.Fatal("disposable localhost PostgreSQL required")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "runner_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	for _, statement := range []string{
		"CREATE TABLE " + schema + ".email_accounts (id uuid PRIMARY KEY, user_id uuid, organization_id uuid, worker_id uuid, email text, provider text, status text)",
		"CREATE TABLE " + schema + ".email_accounts_oauth (email_account_id uuid PRIMARY KEY, access_token text, refresh_token text, expires_at timestamptz, scope text)",
		"CREATE TABLE " + schema + ".tasks (email_account_id uuid, status text)",
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	d := &db.DB{Pool: pool}
	p := validPermit()
	fifth := uuid.MustParse("a5f28cfb-b10f-4597-b445-28e647e0dd92")
	for _, id := range []uuid.UUID{p.TargetID, fifth} {
		if _, err := pool.Exec(ctx, `INSERT INTO email_accounts VALUES($1,$2,$3,$4,$5,'outlook','inactive')`, id, p.OwnerID, p.OrgID, p.WorkerID, p.Email); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO email_accounts_oauth VALUES($1,'sealed-access','sealed-refresh',now()-interval '1 day','fixture-scope')`, id); err != nil {
			t.Fatal(err)
		}
	}
	s, err := readSnapshot(ctx, d, p.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	pre := preimage{Account: s.Account, OAuth: s.OAuth}
	if err := pool.QueryRow(ctx, `SELECT md5(refresh_token || ':' || access_token || ':' || expires_at::text) FROM email_accounts_oauth WHERE email_account_id=$1`, p.TargetID).Scan(&p.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	command := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("gpg", args...).CombinedOutput(); err != nil {
			t.Fatalf("gpg: %v %s", err, out)
		}
	}
	command("--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Reviewer Fixture <reviewer@example.test>", "default", "default", "never")
	list, err := exec.Command("gpg", "--with-colons", "--list-keys").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(list), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" {
			p.ReviewerFingerprint = fields[9]
			break
		}
	}
	if p.ReviewerFingerprint == "" {
		t.Fatal("no reviewer fingerprint")
	}
	raw, err := json.Marshal(pre)
	if err != nil {
		t.Fatal(err)
	}
	prePath := filepath.Join(home, "preimage.json")
	if err := os.WriteFile(prePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	escrow := filepath.Join(home, "escrow.gpg")
	command("--batch", "--yes", "--trust-model", "always", "--recipient", "reviewer@example.test", "--encrypt", "--output", escrow, prePath)
	encrypted, err := os.ReadFile(escrow)
	if err != nil {
		t.Fatal(err)
	}
	p.EscrowSHA256 = digest(encrypted)
	permitBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	permitPath := filepath.Join(home, "permit.json")
	if err := os.WriteFile(permitPath, permitBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sig := filepath.Join(home, "permit.sig")
	command("--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase", "", "--output", sig, "--detach-sign", permitPath)
	if err := gpgVerify(ctx, filepath.Join(home, "pubring.kbx"), sig, permitBytes, p.ReviewerFingerprint); err != nil {
		t.Fatal(err)
	}
	if gpgVerify(ctx, filepath.Join(home, "pubring.kbx"), sig, append(permitBytes, ' '), p.ReviewerFingerprint) == nil {
		t.Fatal("altered permit accepted")
	}
	decrypted, err := decryptEscrow(ctx, encrypted)
	if err != nil || !json.Valid(decrypted) {
		t.Fatalf("escrow decrypt: %v", err)
	}
	var actual preimage
	if err := decodeStrict(decrypted, &actual); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect(ctx, d, p, actual); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts_oauth SET access_token='drift' WHERE email_account_id=$1`, p.TargetID); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect(ctx, d, p, actual); err == nil {
		t.Fatal("preflight accepted drift")
	}
	repo := repository.NewEmailRepostory(d, nil)
	if xerr := repo.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), p.OrgID, p.TargetID, p.WorkerID, p.Email, p.CredentialVersion); xerr == nil {
		t.Fatal("stale version converted")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts_oauth SET access_token='sealed-access' WHERE email_account_id=$1`, p.TargetID); err != nil {
		t.Fatal(err)
	}
	fifthBefore, err := inspect(ctx, d, p, actual)
	if err != nil {
		t.Fatal(err)
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), p.OrgID, fifth, p.WorkerID, p.Email, p.CredentialVersion); xerr == nil {
		t.Fatal("excluded fifth converted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks VALUES($1,'pending')`, p.TargetID); err != nil {
		t.Fatal(err)
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), p.OrgID, p.TargetID, p.WorkerID, p.Email, p.CredentialVersion); xerr == nil {
		t.Fatal("queued target converted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM tasks WHERE email_account_id=$1`, p.TargetID); err != nil {
		t.Fatal(err)
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), p.OrgID, p.TargetID, p.WorkerID, p.Email, p.CredentialVersion); xerr != nil {
		t.Fatalf("CAS: %v", xerr)
	}
	if err := reconcile(ctx, d, p, actual, fifthBefore); err != nil {
		t.Fatalf("readback: %v", err)
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), p.OrgID, p.TargetID, p.WorkerID, p.Email, p.CredentialVersion); xerr == nil {
		t.Fatal("replay converted")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts_oauth SET scope='tampered' WHERE email_account_id=$1`, p.TargetID); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(ctx, d, p, actual, fifthBefore); err == nil {
		t.Fatal("readback accepted unrelated OAuth metadata drift")
	}
}
