package repository

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/encrypt"
)

// Run only against an explicitly supplied disposable/local PostgreSQL database.
// Each test gets its own schema; no production data or tables are touched.
func TestReconnectRepositoryPostgres(t *testing.T) {
	endpoint := os.Getenv("RECONNECT_TEST_DATABASE_URL")
	if endpoint == "" {
		t.Skip("set RECONNECT_TEST_DATABASE_URL to an isolated non-production PostgreSQL instance")
	}
	cfg, err := pgxpool.ParseConfig(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Host != "127.0.0.1" && cfg.ConnConfig.Host != "localhost" {
		t.Fatal("integration test requires localhost PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "reconnect_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop isolated schema: %v", err)
		}
	}()
	for _, table := range []string{"email_accounts", "email_accounts_oauth", "email_tags"} {
		if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE TABLE %s.%s (LIKE public.%s INCLUDING DEFAULTS INCLUDING CONSTRAINTS)", schema, table, table)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(ctx, "ALTER TABLE "+schema+".email_accounts ADD PRIMARY KEY (id)"); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	encrypter, err := encrypt.NewEncrypter([]byte(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	repo := &emailRepository{DB: &db.DB{Pool: pool}, Encrypt: encrypter}
	org, otherOrg, user, otherUser, id, otherID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seed := func(accountID, ownerID, orgID uuid.UUID, address string) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO email_accounts (id,user_id,organization_id,email,name,signature_plain,signature_html,provider,status)
            VALUES ($1,$2,$3,$4,'Fixture','','','outlook','inactive')`, accountID, ownerID, orgID, address)
		if err != nil {
			t.Fatal(err)
		}
		access, refresh, err := repo.sealOAuthTokens("fixture-old-access", "fixture-old-refresh")
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO email_accounts_oauth (email_account_id,access_token,refresh_token,expires_at)
            VALUES ($1,$2,$3,now())`, accountID, access, refresh)
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(id, user, org, "owner@example.test")
	seed(otherID, otherUser, otherOrg, "other@example.test")
	account, xerr := repo.Get(ctx, org.String(), id.String())
	if xerr != nil || account == nil || account.UserID != user.String() || account.OrganizationID == nil || *account.OrganizationID != org {
		t.Fatalf("scoped lookup did not hydrate owner/org: account=%+v err=%v", account, xerr)
	}
	if _, xerr := repo.Get(ctx, otherOrg.String(), id.String()); xerr == nil {
		t.Fatal("cross-organization Get succeeded")
	}
	if _, xerr := repo.ReconnectOutlookCredentialVersion(ctx, otherUser.String(), org, id); xerr == nil {
		t.Fatal("other owner could start")
	}
	if _, xerr := repo.ReconnectOutlookCredentialVersion(ctx, user.String(), otherOrg, id); xerr == nil {
		t.Fatal("other org could start")
	}
	versionA, xerr := repo.ReconnectOutlookCredentialVersion(ctx, user.String(), org, id)
	if xerr != nil || versionA == "" {
		t.Fatalf("delegated credential version: %v", xerr)
	}
	versionB, xerr := repo.ReconnectOutlookCredentialVersion(ctx, user.String(), org, id)
	if xerr != nil || versionA != versionB {
		t.Fatal("simultaneous starts must observe same version")
	}
	outcomes := make(chan *errx.Error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outcomes <- repo.ReconnectOutlookToken(ctx, user.String(), org, id, "owner@example.test", versionA,
				fmt.Sprintf("fixture-new-access-%d", i), fmt.Sprintf("fixture-new-refresh-%d", i), time.Now().Add(time.Hour))
		}(i)
	}
	wg.Wait()
	close(outcomes)
	success, stale := 0, 0
	for result := range outcomes {
		if result == nil {
			success++
		} else if result == errx.ErrEmailOnboardState {
			stale++
		} else {
			t.Fatalf("unexpected reconnect error: %v", result)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("concurrent flows: success=%d stale=%d; want 1/1", success, stale)
	}
	if xerr := repo.ReconnectOutlookToken(ctx, user.String(), org, id, "owner@example.test", versionB, "fixture-third-access", "fixture-third-refresh", time.Now()); xerr != errx.ErrEmailOnboardState {
		t.Fatalf("stale replay accepted: %v", xerr)
	}
	var encrypted, status string
	if err := pool.QueryRow(ctx, `SELECT o.refresh_token, ea.status FROM email_accounts_oauth o JOIN email_accounts ea ON ea.id=o.email_account_id WHERE ea.id=$1`, id).Scan(&encrypted, &status); err != nil {
		t.Fatal(err)
	}
	clear, err := repo.Encrypt.Decrypt(encrypted)
	if err != nil || !strings.HasPrefix(clear, "fixture-new-refresh-") || status != "inactive" {
		t.Fatal("winner's encrypted credential or inactive status not preserved")
	}
	if xerr := repo.ReconnectOutlookToken(ctx, user.String(), org, otherID, "other@example.test", versionA, "fixture-bad-access", "fixture-bad-refresh", time.Now()); xerr != errx.ErrEmailOnboardState {
		t.Fatal("cross-org credential write accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts_oauth SET access_token='', refresh_token=$1 WHERE email_account_id=$2`, models.GraphAppOnlyRefreshToken, id); err != nil {
		t.Fatal(err)
	}
	if _, xerr := repo.ReconnectOutlookCredentialVersion(ctx, user.String(), org, id); xerr != errx.ErrEmailOnboardState {
		t.Fatal("app-only account accepted for delegated reconnect")
	}

	// An inactive shared sender must retain its ID/history while only its
	// credential row changes. The active delegate holds the identical sealed
	// credential copied by the existing shared-mailbox onboarding path.
	parentID, sharedID := uuid.New(), uuid.New()
	seed(parentID, user, org, "delegate@example.test")
	seed(sharedID, user, org, "shared@example.test")
	if _, err := pool.Exec(ctx, `UPDATE email_accounts SET status='active' WHERE id=$1`, parentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts_oauth SET access_token=p.access_token, refresh_token=p.refresh_token
		FROM email_accounts_oauth p WHERE email_accounts_oauth.email_account_id=$1 AND p.email_account_id=$2`, sharedID, parentID); err != nil {
		t.Fatal(err)
	}
	version, xerr := repo.OutlookAppOnlyConversionVersion(ctx, user.String(), org, sharedID, parentID, "shared@example.test")
	if xerr != nil || version == "" {
		t.Fatalf("eligible shared sender: %v", xerr)
	}
	if _, xerr := repo.OutlookAppOnlyConversionVersion(ctx, user.String(), org, sharedID, otherID, "shared@example.test"); xerr != errx.ErrEmailOnboardState {
		t.Fatal("unrelated parent accepted as credential provenance")
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, user.String(), org, sharedID, parentID, "delegate@example.test", version); xerr != errx.ErrEmailOnboardState {
		t.Fatal("wrong expected target email accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts SET status='active' WHERE id=$1`, sharedID); err != nil {
		t.Fatal(err)
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, user.String(), org, sharedID, parentID, "shared@example.test", version); xerr != errx.ErrEmailOnboardState {
		t.Fatal("active target converted")
	}
	if _, err := pool.Exec(ctx, `UPDATE email_accounts SET status='inactive' WHERE id=$1`, sharedID); err != nil {
		t.Fatal(err)
	}
	results := make(chan *errx.Error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- repo.ConvertOutlookAppOnly(ctx, user.String(), org, sharedID, parentID, "shared@example.test", version)
		}()
	}
	wg.Wait()
	close(results)
	success, stale = 0, 0
	for result := range results {
		switch result {
		case nil:
			success++
		case errx.ErrEmailOnboardState:
			stale++
		default:
			t.Fatalf("unexpected conversion error: %v", result)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("concurrent conversions: success=%d stale=%d; want 1/1", success, stale)
	}
	var targetToken, targetAccess, targetStatus, parentToken, parentStatus string
	if err := pool.QueryRow(ctx, `SELECT o.refresh_token,o.access_token,ea.status FROM email_accounts_oauth o JOIN email_accounts ea ON ea.id=o.email_account_id WHERE ea.id=$1`, sharedID).Scan(&targetToken, &targetAccess, &targetStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT o.refresh_token,ea.status FROM email_accounts_oauth o JOIN email_accounts ea ON ea.id=o.email_account_id WHERE ea.id=$1`, parentID).Scan(&parentToken, &parentStatus); err != nil {
		t.Fatal(err)
	}
	if targetToken != models.GraphAppOnlyRefreshToken || targetAccess != "" || targetStatus != "inactive" || parentToken == targetToken || parentStatus != "active" {
		t.Fatal("conversion changed account status/parent or failed to set exact sentinel")
	}
	if _, xerr := repo.OutlookAppOnlyConversionVersion(ctx, user.String(), org, sharedID, parentID, "shared@example.test"); xerr != errx.ErrEmailOnboardState {
		t.Fatal("converted sender accepted for replay")
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, otherUser.String(), org, sharedID, parentID, "shared@example.test", version); xerr != errx.ErrEmailOnboardState {
		t.Fatal("cross-owner conversion accepted")
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, user.String(), otherOrg, sharedID, parentID, "shared@example.test", version); xerr != errx.ErrEmailOnboardState {
		t.Fatal("cross-org conversion accepted")
	}
	if xerr := repo.ConvertOutlookAppOnly(ctx, user.String(), org, otherID, parentID, "other@example.test", version); xerr != errx.ErrEmailOnboardState {
		t.Fatal("unrelated mailbox conversion accepted")
	}
}
