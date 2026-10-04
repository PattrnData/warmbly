package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Connected test: real signed permit, encrypted escrow, CLI execute, Graph stub,
// PostgreSQL CAS/readback. The fifth account is deliberately excluded.
func TestRunnerSignedPermitGraphCASAndDeadline(t *testing.T) {
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
	for _, lock := range []bool{false, true} {
		name := "success"
		if lock {
			name = "lock-wait-past-evidence-expiry"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			admin, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			schema := "runner_connected_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
					t.Errorf("cleanup: %v", err)
				}
			}()
			// Only the columns read by the conversion service and runner are needed.
			for _, sql := range []string{
				"CREATE TABLE " + schema + `.email_accounts (id uuid PRIMARY KEY, user_id uuid, organization_id uuid, worker_id uuid, email text, name text DEFAULT 'fixture', signature_plain text DEFAULT '', signature_html text DEFAULT '', signature_sync boolean DEFAULT true, signature_code boolean DEFAULT false, provider text, status text, last_synced_at timestamp, last_id bigint, campaign_limit integer DEFAULT 50, min_wait_time integer DEFAULT 600, reply_to text DEFAULT '', tracking_domain text DEFAULT '', tracking_domain_verified boolean DEFAULT false, tracking_domain_verified_at timestamptz, auth_state text DEFAULT 'unknown', auth_spf boolean DEFAULT false, auth_dkim boolean DEFAULT false, auth_dmarc boolean DEFAULT false, auth_dmarc_policy text DEFAULT '', auth_reason text DEFAULT '', auth_checked_at timestamptz, warmup timestamp, warmup_paused_at timestamptz, warmup_base integer DEFAULT 10, warmup_max integer DEFAULT 40, warmup_increase integer DEFAULT 1, warmup_start_time time DEFAULT '08:00', warmup_end_time time DEFAULT '20:00', warmup_days smallint DEFAULT 0, created_at timestamp DEFAULT now(), updated_at timestamp DEFAULT now())`,
				"CREATE TABLE " + schema + `.email_accounts_oauth (email_account_id uuid PRIMARY KEY, access_token text, refresh_token text, expires_at timestamptz, scope text)`,
				"CREATE TABLE " + schema + `.email_tags (email_id uuid, tag_id uuid)`,
				"CREATE TABLE " + schema + `.tasks (email_account_id uuid, status text)`,
			} {
				if _, err := admin.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
			localCfg := cfg.Copy()
			localCfg.ConnConfig.RuntimeParams["search_path"] = schema
			pool, err := pgxpool.NewWithConfig(ctx, localCfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			p := validPermit()
			fifthID := uuid.MustParse("a5f28cfb-b10f-4597-b445-28e647e0dd92")
			for _, id := range []uuid.UUID{p.TargetID, fifthID} {
				if _, err := pool.Exec(ctx, `INSERT INTO email_accounts (id,user_id,organization_id,worker_id,email,provider,status) VALUES ($1,$2,$3,$4,$5,'outlook','inactive')`, id, p.OwnerID, p.OrgID, p.WorkerID, p.Email); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `INSERT INTO email_accounts_oauth VALUES ($1,'sealed-access','sealed-refresh',now()-interval '1 day','fixture-scope')`, id); err != nil {
					t.Fatal(err)
				}
			}
			d := &db.DB{Pool: pool}
			s, err := readSnapshot(ctx, d, p.TargetID)
			if err != nil {
				t.Fatal(err)
			}
			fifthBefore, err := readFifth(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
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
			key, err := exec.Command("gpg", "--with-colons", "--list-keys").Output()
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(key), "\n") {
				fields := strings.Split(line, ":")
				if len(fields) > 9 && fields[0] == "fpr" {
					p.ReviewerFingerprint = fields[9]
					break
				}
			}
			if p.ReviewerFingerprint == "" {
				t.Fatal("fingerprint missing")
			}
			put := func(name string, v any) (string, []byte) {
				t.Helper()
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(home, name)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				return path, raw
			}
			prePath, _ := put("preimage.json", preimage{Account: s.Account, OAuth: s.OAuth})
			escrow := filepath.Join(home, "escrow.gpg")
			command("--batch", "--yes", "--trust-model", "always", "--recipient", "reviewer@example.test", "--encrypt", "--output", escrow, prePath)
			cipher, err := os.ReadFile(escrow)
			if err != nil {
				t.Fatal(err)
			}
			p.EscrowSHA256 = digest(cipher)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			p.BinarySHA256, err = fileDigest(exe)
			if err != nil {
				t.Fatal(err)
			}
			previousSource, previousReviewer := sourceSHA256, trustedReviewerFingerprint
			sourceSHA256 = p.SourceSHA256
			trustedReviewerFingerprint = p.ReviewerFingerprint
			t.Cleanup(func() { sourceSHA256 = previousSource; trustedReviewerFingerprint = previousReviewer })
			t.Setenv("WARMBLY_IMAGE_DIGEST", p.ImageDigest)
			t.Setenv("BOX_OUTLOOK_CLIENT_ID", p.AppID)
			t.Setenv("BOX_OUTLOOK_CLIENT_SECRET", "fixture-secret")
			t.Setenv("BOX_OUTLOOK_TENANT_ID", p.TenantID.String())
			q, err := url.Parse(endpoint)
			if err != nil {
				t.Fatal(err)
			}
			values := q.Query()
			values.Set("search_path", schema)
			q.RawQuery = values.Encode()
			dbURL := q.String()
			t.Setenv("PRIMARY_DB", dbURL)
			configBytes, _ := json.Marshal([]string{dbURL, p.AppID, "fixture-secret", p.TenantID.String()})
			p.ConfigSHA256 = digest(configBytes)
			observedAt := time.Now().UTC()
			if lock {
				observedAt = observedAt.Add(-4*time.Minute - 50*time.Second)
			}
			grant := grantEvidence{TargetID: p.TargetID, TenantID: p.TenantID, AppID: p.AppID, Email: p.Email, MailReadWrite: true, MailSend: true, RestrictionEffective: true, ObservedAt: observedAt}
			fence := fenceEvidence{TargetID: p.TargetID, DispatchFenced: true, ObservedAt: observedAt, FenceUntil: p.ExpiresAt.Add(time.Minute)}
			grantPath, grantBytes := put("grant.json", grant)
			p.GrantEvidenceSHA256 = digest(grantBytes)
			fencePath, fenceBytes := put("fence.json", fence)
			p.FenceEvidenceSHA256 = digest(fenceBytes)
			permitPath, permitBytes := put("permit.json", p)
			signature := filepath.Join(home, "permit.sig")
			command("--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase", "", "--output", signature, "--detach-sign", permitPath)
			claims, _ := json.Marshal(map[string]any{"tid": p.TenantID.String(), "appid": p.AppID, "aud": "https://graph.microsoft.com", "roles": []string{"Mail.ReadWrite", "Mail.Send"}})
			token := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
			origTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = origTransport })
			var graphCalls, tokenCalls int
			var heldTx interface{ Rollback(context.Context) error }
			http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
				var body string
				switch r.URL.Host {
				case "login.microsoftonline.com":
					if r.Method != "POST" || !strings.Contains(r.URL.Path, p.TenantID.String()+"/oauth2/v2.0/token") {
						return nil, fmt.Errorf("unexpected token request: %s", r.URL)
					}
					tokenCalls++
					body = fmt.Sprintf(`{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, token)
				case "graph.microsoft.com":
					if r.Method != "GET" || !strings.Contains(r.URL.Path, "/users/"+p.Email+"/mailFolders/inbox") || r.Header.Get("Authorization") != "Bearer "+token {
						return nil, fmt.Errorf("unexpected Graph request: %s", r.URL)
					}
					graphCalls++
					body = `{"id":"inbox","displayName":"Inbox"}`
					if lock {
						tx, err := pool.Begin(ctx)
						if err != nil {
							return nil, err
						}
						if _, err := tx.Exec(ctx, `SELECT email_account_id FROM email_accounts_oauth WHERE email_account_id=$1 FOR UPDATE`, p.TargetID); err != nil {
							_ = tx.Rollback(ctx)
							return nil, err
						}
						heldTx = tx
					}
				default:
					return nil, fmt.Errorf("unexpected external host: %s", r.URL.Host)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			receipt := filepath.Join(home, "receipt.json")
			args := []string{"--execute", "--target", p.TargetID.String(), "--permit", permitPath, "--permit-sha256", digest(permitBytes), "--permit-signature", signature, "--reviewer-keyring", filepath.Join(home, "pubring.kbx"), "--escrow", escrow, "--grant-evidence", grantPath, "--fence-evidence", fencePath, "--receipt", receipt}
			var stdout, stderr bytes.Buffer
			result := run(args, &stdout, &stderr)
			if heldTx != nil {
				if err := heldTx.Rollback(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if tokenCalls != 1 || graphCalls != 1 {
				t.Fatalf("token=%d Graph=%d; status=%d stderr=%s", tokenCalls, graphCalls, result, stderr.String())
			}
			if lock {
				if result == 0 || time.Now().Before(writeDeadline(p, grant, fence)) || !strings.Contains(stderr.String(), "conversion rejected") {
					t.Fatalf("lock wait did not reject at evidence deadline: status=%d stderr=%s", result, stderr.String())
				}
				if b, err := os.ReadFile(receipt); err != nil || len(b) != 0 {
					t.Fatalf("rejected conversion left success receipt: size=%d err=%v", len(b), err)
				}
				unchanged, err := readSnapshot(ctx, d, p.TargetID)
				if err != nil {
					t.Fatal(err)
				}
				if !sameJSON(unchanged.Account, s.Account) || !sameJSON(unchanged.OAuth, s.OAuth) {
					t.Fatal("expired lock-wait changed target")
				}
				return
			}
			if result != 0 {
				t.Fatalf("execute failed: %s", stderr.String())
			}
			if err := reconcile(ctx, d, p, preimage{Account: s.Account, OAuth: s.OAuth}, fifthBefore); err != nil {
				t.Fatal(err)
			}
			receiptBytes, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Reconciled bool      `json:"reconciled"`
				TargetID   uuid.UUID `json:"target_id"`
			}
			if err := json.Unmarshal(receiptBytes, &report); err != nil || !report.Reconciled || report.TargetID != p.TargetID {
				t.Fatalf("receipt invalid: %v", err)
			}
			if replay := run(args, &stdout, &stderr); replay == 0 {
				t.Fatal("replay accepted")
			}
		})
	}
}
