// Command outlook-convert is an incident-scoped, one-account credential-mode runner.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/app/email"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// Set these at build time from independently reviewed source/image evidence.
// Empty metadata cannot authorize execution.
var sourceSHA256, trustedReviewerFingerprint, version = "", "", "dev"

type permit struct {
	TargetID            uuid.UUID `json:"target_id"`
	OwnerID             uuid.UUID `json:"owner_id"`
	OrgID               uuid.UUID `json:"org_id"`
	WorkerID            uuid.UUID `json:"worker_id"`
	TenantID            uuid.UUID `json:"tenant_id"`
	Email               string    `json:"email"`
	AppID               string    `json:"app_id"`
	CredentialVersion   string    `json:"credential_version"`
	SourceSHA256        string    `json:"source_sha256"`
	ImageDigest         string    `json:"image_digest"`
	BinarySHA256        string    `json:"binary_sha256"`
	ConfigSHA256        string    `json:"config_sha256"`
	EscrowSHA256        string    `json:"escrow_sha256"`
	GrantEvidenceSHA256 string    `json:"grant_evidence_sha256"`
	FenceEvidenceSHA256 string    `json:"fence_evidence_sha256"`
	Reviewer            string    `json:"reviewer"`
	ReviewerFingerprint string    `json:"reviewer_fingerprint"`
	ReviewedAt          time.Time `json:"reviewed_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	NoSendFrom          time.Time `json:"no_send_from"`
	NoSendUntil         time.Time `json:"no_send_until"`
}

type preimage struct {
	Account json.RawMessage `json:"account"`
	OAuth   json.RawMessage `json:"oauth"`
}

// Reviewer-signed permit binds these external, fresh evidence files by digest.
type grantEvidence struct {
	TargetID             uuid.UUID `json:"target_id"`
	TenantID             uuid.UUID `json:"tenant_id"`
	AppID                string    `json:"app_id"`
	Email                string    `json:"email"`
	MailReadWrite        bool      `json:"mail_read_write"`
	MailSend             bool      `json:"mail_send"`
	RestrictionEffective bool      `json:"restriction_effective"`
	ObservedAt           time.Time `json:"observed_at"`
}
type fenceEvidence struct {
	TargetID       uuid.UUID `json:"target_id"`
	DispatchFenced bool      `json:"dispatch_fenced"`
	BrokerPending  int64     `json:"broker_pending"`
	WorkerInFlight int64     `json:"worker_in_flight"`
	ObservedAt     time.Time `json:"observed_at"`
	FenceUntil     time.Time `json:"fence_until"`
}

func verifyEvidence(p permit, grant grantEvidence, fence fenceEvidence, now time.Time) error {
	fresh := func(t time.Time) bool { return !t.IsZero() && !t.After(now) && now.Sub(t) < 5*time.Minute }
	if grant.TargetID != p.TargetID || grant.TenantID != p.TenantID || grant.AppID != p.AppID || !strings.EqualFold(grant.Email, p.Email) || !grant.MailReadWrite || !grant.MailSend || !grant.RestrictionEffective || !fresh(grant.ObservedAt) {
		return errors.New("grant evidence missing, stale or mismatched")
	}
	if fence.TargetID != p.TargetID || !fence.DispatchFenced || fence.BrokerPending != 0 || fence.WorkerInFlight != 0 || !fresh(fence.ObservedAt) || !fence.FenceUntil.After(p.ExpiresAt) {
		return errors.New("dispatch fence or in-flight evidence missing/stale")
	}
	return nil
}

type snapshot struct {
	Account     json.RawMessage
	OAuth       json.RawMessage
	ActiveTasks int64
	OAuthRows   int64
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func validDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}
func validVersion(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 16 && s == strings.ToLower(s)
}
func approved(id uuid.UUID) bool {
	switch id.String() {
	case "af571c6e-e6f0-4cb9-90fe-a7d5105babd7", "e165f276-cc96-4907-991a-a7b860e1ed6f", "e7ce131c-4a6f-4529-9bd4-9f88bfd99208", "ea4b17db-80b9-445b-9c28-8d67679dc4a5":
		return true
	}
	return false
}
func validatePermit(p permit, now time.Time) error {
	if !approved(p.TargetID) || p.OwnerID == uuid.Nil || p.OrgID == uuid.Nil || p.WorkerID == uuid.Nil || p.TenantID == uuid.Nil || p.Email == "" || strings.TrimSpace(p.Email) != p.Email || p.AppID == "" || p.Reviewer == "" || !validVersion(p.CredentialVersion) {
		return errors.New("invalid exact target or reviewer")
	}
	for _, s := range []string{p.SourceSHA256, p.BinarySHA256, p.ConfigSHA256, p.EscrowSHA256, p.GrantEvidenceSHA256, p.FenceEvidenceSHA256} {
		if !validDigest(s) {
			return errors.New("missing or invalid evidence digest")
		}
	}
	if p.ImageDigest != "sha256:"+strings.TrimPrefix(p.ImageDigest, "sha256:") || !validDigest(strings.TrimPrefix(p.ImageDigest, "sha256:")) {
		return errors.New("invalid image digest")
	}
	if p.ReviewedAt.IsZero() || p.ReviewedAt.After(now) || !now.Before(p.ExpiresAt) || p.ExpiresAt.Sub(p.ReviewedAt) > 24*time.Hour || !p.ExpiresAt.After(p.ReviewedAt) || now.Before(p.NoSendFrom) || !now.Before(p.NoSendUntil) || p.NoSendUntil.Before(p.ExpiresAt) {
		return errors.New("permit expired, unsigned time, or no-send window absent")
	}
	return nil
}
func decodeStrict(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON data")
	}
	return nil
}
func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	ax := json.NewDecoder(bytes.NewReader(a))
	by := json.NewDecoder(bytes.NewReader(b))
	ax.UseNumber()
	by.UseNumber()
	if ax.Decode(&x) != nil || by.Decode(&y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}
func samePreimage(e preimage, account, oauth json.RawMessage) error {
	if len(e.Account) == 0 || len(e.OAuth) == 0 || !sameJSON(e.Account, account) || !sameJSON(e.OAuth, oauth) {
		return errors.New("preimage differs from exact account/OAuth rows")
	}
	return nil
}

// gpgVerify only accepts a cryptographically valid signature from the pinned reviewer.
func gpgVerify(ctx context.Context, keyring, signature, document, fingerprint string) error {
	if fingerprint == "" || keyring == "" {
		return errors.New("reviewer key not pinned")
	}
	cmd := exec.CommandContext(ctx, "gpg", "--batch", "--no-tty", "--no-default-keyring", "--keyring", keyring, "--status-fd", "1", "--verify", signature, document)
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err != nil {
		return errors.New("reviewer signature invalid")
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "[GNUPG:]" && fields[1] == "VALIDSIG" && fields[2] == fingerprint {
			return nil
		}
	}
	return errors.New("reviewer signature does not match pinned fingerprint")
}
func decryptEscrow(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gpg", "--batch", "--no-tty", "--decrypt", path)
	cmd.Stderr = io.Discard
	data, err := cmd.Output()
	if err != nil {
		return nil, errors.New("escrow decryption failed")
	}
	return data, nil
}
func readSnapshot(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id uuid.UUID) (snapshot, error) {
	var s snapshot
	var a, o string
	err := q.QueryRow(ctx, `SELECT row_to_json(ea)::text,
 COALESCE((SELECT row_to_json(o)::text FROM email_accounts_oauth o WHERE o.email_account_id=ea.id),'null'),
 (SELECT count(*) FROM email_accounts_oauth o WHERE o.email_account_id=ea.id),
 (SELECT count(*) FROM tasks t WHERE t.email_account_id=ea.id AND t.status IN ('pending','active'))
 FROM email_accounts ea WHERE ea.id=$1`, id).Scan(&a, &o, &s.OAuthRows, &s.ActiveTasks)
	if err != nil {
		return s, errors.New("target snapshot unavailable")
	}
	s.Account, s.OAuth = []byte(a), []byte(o)
	return s, nil
}
func readFifth(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, error) {
	var a, o string
	err := q.QueryRow(ctx, `SELECT row_to_json(ea)::text,COALESCE((SELECT row_to_json(o)::text FROM email_accounts_oauth o WHERE o.email_account_id=ea.id),'null') FROM email_accounts ea WHERE ea.id='a5f28cfb-b10f-4597-b445-28e647e0dd92'`).Scan(&a, &o)
	if err == pgx.ErrNoRows {
		return "absent", nil
	}
	if err != nil {
		return "", errors.New("fifth snapshot unavailable")
	}
	return digest([]byte(a + "\n" + o)), nil
}
func inspect(ctx context.Context, d *db.DB, p permit, e preimage) (string, error) {
	tx, err := d.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	s, err := readSnapshot(ctx, tx, p.TargetID)
	if err != nil {
		return "", err
	}
	if s.OAuthRows != 1 || s.ActiveTasks != 0 {
		return "", errors.New("ambiguous OAuth row or queued task")
	}
	if err := samePreimage(e, s.Account, s.OAuth); err != nil {
		return "", err
	}
	var a struct {
		ID             uuid.UUID `json:"id"`
		UserID         string    `json:"user_id"`
		OrganizationID uuid.UUID `json:"organization_id"`
		WorkerID       uuid.UUID `json:"worker_id"`
		Email          string    `json:"email"`
		Provider       string    `json:"provider"`
		Status         string    `json:"status"`
	}
	var o struct {
		EmailAccountID uuid.UUID `json:"email_account_id"`
		AccessToken    string    `json:"access_token"`
		RefreshToken   string    `json:"refresh_token"`
		ExpiresAt      time.Time `json:"expires_at"`
	}
	if json.Unmarshal(s.Account, &a) != nil || json.Unmarshal(s.OAuth, &o) != nil || a.ID != p.TargetID || a.UserID != p.OwnerID.String() || a.OrganizationID != p.OrgID || a.WorkerID != p.WorkerID || !strings.EqualFold(a.Email, p.Email) || a.Provider != "outlook" || a.Status != "inactive" || o.EmailAccountID != p.TargetID || o.AccessToken == "" || o.RefreshToken == "" || o.RefreshToken == models.GraphAppOnlyRefreshToken {
		return "", errors.New("target account/credential mismatch")
	}
	var version string
	if err := tx.QueryRow(ctx, `SELECT md5(refresh_token || ':' || access_token || ':' || expires_at::text) FROM email_accounts_oauth WHERE email_account_id=$1`, p.TargetID).Scan(&version); err != nil || version != p.CredentialVersion {
		return "", errors.New("credential version mismatch")
	}
	fifth, err := readFifth(ctx, tx)
	if err != nil {
		return "", err
	}
	return fifth, nil
}
func reconcile(ctx context.Context, d *db.DB, p permit, e preimage, fifthBefore string) error {
	tx, err := d.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	s, err := readSnapshot(ctx, tx, p.TargetID)
	if err != nil {
		return err
	}
	if s.OAuthRows != 1 || s.ActiveTasks != 0 || !sameJSON(s.Account, e.Account) {
		return errors.New("post-write account/task drift")
	}
	var o struct {
		EmailAccountID uuid.UUID `json:"email_account_id"`
		AccessToken    string    `json:"access_token"`
		RefreshToken   string    `json:"refresh_token"`
		ExpiresAt      time.Time `json:"expires_at"`
	}
	if json.Unmarshal(s.OAuth, &o) != nil || o.EmailAccountID != p.TargetID || o.AccessToken != "" || o.RefreshToken != models.GraphAppOnlyRefreshToken {
		return errors.New("post-write OAuth mismatch")
	}
	fifth, err := readFifth(ctx, tx)
	if err != nil || fifth != fifthBefore {
		return errors.New("excluded fifth changed")
	}
	return nil
}
func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digest(b), nil
}
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("outlook-convert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var execute, dry bool
	var target, permitPath, permitHash, signature, keyring, escrow, receipt, grantPath, fencePath string
	fs.BoolVar(&execute, "execute", false, "execute one permitted conversion")
	fs.BoolVar(&dry, "dry-run", false, "validate local permit without network or database access")
	fs.StringVar(&target, "target", "", "single approved target UUID")
	fs.StringVar(&permitPath, "permit", "", "immutable permit JSON")
	fs.StringVar(&permitHash, "permit-sha256", "", "independently approved permit SHA-256")
	fs.StringVar(&signature, "permit-signature", "", "detached reviewer signature")
	fs.StringVar(&keyring, "reviewer-keyring", "", "trusted reviewer public keyring")
	fs.StringVar(&escrow, "escrow", "", "GPG-encrypted exact-row preimage JSON")
	fs.StringVar(&grantPath, "grant-evidence", "", "fresh tenant-scoped Graph grant evidence JSON")
	fs.StringVar(&fencePath, "fence-evidence", "", "fresh dispatch fence and drained queue evidence JSON")
	fs.StringVar(&receipt, "receipt", "", "new private receipt file (execute only)")
	fs.Usage = func() {
		fmt.Fprintln(stdout, "Usage: outlook-convert --dry-run|--execute --target UUID --permit FILE --permit-sha256 HEX [execute: --permit-signature FILE --reviewer-keyring FILE --escrow FILE --receipt FILE]")
	}
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fs.Usage()
			return 0
		}
		if arg == "--version" {
			fmt.Fprintln(stdout, version)
			return 0
		}
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 0 || execute == dry || (!execute && !dry) {
		fmt.Fprintln(stderr, "exactly one of --execute or --dry-run required, no positional arguments")
		return 2
	}
	id, err := uuid.Parse(target)
	if err != nil || !approved(id) || permitPath == "" || !validDigest(permitHash) {
		fmt.Fprintln(stderr, "exact target and permit digest required")
		return 2
	}
	data, err := os.ReadFile(permitPath)
	if err != nil || digest(data) != permitHash {
		fmt.Fprintln(stderr, "permit unreadable or digest mismatch")
		return 2
	}
	var p permit
	if err := decodeStrict(data, &p); err != nil || p.TargetID != id || validatePermit(p, time.Now().UTC()) != nil {
		fmt.Fprintln(stderr, "permit invalid, expired, or target mismatch")
		return 2
	}
	if dry {
		fmt.Fprintln(stdout, "local dry-run only: permit parsed; no DB, Graph, escrow, or reviewer validation; production HOLD")
		return 0
	}
	imageDigest := os.Getenv("WARMBLY_IMAGE_DIGEST") // externally attested deployed image ID; never derived from self-hashing the binary
	if signature == "" || keyring == "" || escrow == "" || receipt == "" || grantPath == "" || fencePath == "" || sourceSHA256 == "" || imageDigest == "" || trustedReviewerFingerprint == "" || p.ReviewerFingerprint != trustedReviewerFingerprint || p.SourceSHA256 != sourceSHA256 || p.ImageDigest != imageDigest {
		fmt.Fprintln(stderr, "execute requires pinned build provenance, reviewer, encrypted escrow and receipt")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := gpgVerify(ctx, keyring, signature, permitPath, trustedReviewerFingerprint); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "binary unavailable")
		return 1
	}
	binHash, err := fileDigest(exe)
	if err != nil || binHash != p.BinarySHA256 {
		fmt.Fprintln(stderr, "binary digest mismatch")
		return 1
	}
	encrypted, err := os.ReadFile(escrow)
	if err != nil || digest(encrypted) != p.EscrowSHA256 {
		fmt.Fprintln(stderr, "escrow digest mismatch")
		return 1
	}
	raw, err := decryptEscrow(ctx, escrow)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var pre preimage
	if decodeStrict(raw, &pre) != nil || len(pre.Account) == 0 || len(pre.OAuth) == 0 {
		fmt.Fprintln(stderr, "escrow preimage invalid")
		return 1
	}
	grantBytes, ge := os.ReadFile(grantPath)
	fenceBytes, fe := os.ReadFile(fencePath)
	var grant grantEvidence
	var fence fenceEvidence
	if ge != nil || fe != nil || digest(grantBytes) != p.GrantEvidenceSHA256 || digest(fenceBytes) != p.FenceEvidenceSHA256 || decodeStrict(grantBytes, &grant) != nil || decodeStrict(fenceBytes, &fence) != nil || verifyEvidence(p, grant, fence, time.Now().UTC()) != nil {
		fmt.Fprintln(stderr, "grant or dispatch fence evidence invalid, stale or mismatched")
		return 1
	}
	dbURL, clientID, secret, tenant := os.Getenv("PRIMARY_DB"), os.Getenv("BOX_OUTLOOK_CLIENT_ID"), os.Getenv("BOX_OUTLOOK_CLIENT_SECRET"), os.Getenv("BOX_OUTLOOK_TENANT_ID")
	if dbURL == "" || clientID == "" || secret == "" || tenant != p.TenantID.String() || clientID != p.AppID {
		fmt.Fprintln(stderr, "runtime config incomplete or different from permit")
		return 1
	}
	cfg, _ := json.Marshal([]string{dbURL, clientID, secret, tenant})
	if digest(cfg) != p.ConfigSHA256 {
		fmt.Fprintln(stderr, "runtime config digest mismatch")
		return 1
	}
	// Exclusive-create receipt before any external write; no success without a durable readback.
	f, err := os.OpenFile(receipt, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(stderr, "private receipt path not available")
		return 1
	}
	defer f.Close()
	d, err := db.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(stderr, "database connection failed")
		return 1
	}
	defer d.Close()
	fifth, err := inspect(ctx, d, p, pre)
	if err != nil {
		fmt.Fprintln(stderr, "preflight rejected:", err)
		return 1
	}
	if err := validatePermit(p, time.Now().UTC()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := verifyEvidence(p, grant, fence, time.Now().UTC()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	repo := repository.NewEmailRepostory(d, nil)
	oauth := config.Oauth2Inbox{OutlookAppOnly: config.OutlookAppOnlyInbox()}
	service := email.NewServiceWithWorker(repo, nil, nil, nil, nil, nil, &oauth, nil)
	if _, xerr := service.ConvertOutlookAppOnly(ctx, p.OwnerID.String(), &p.OrgID, p.TargetID, p.WorkerID, p.Email, p.TenantID, p.CredentialVersion); xerr != nil {
		fmt.Fprintln(stderr, "conversion rejected")
		return 1
	}
	// A failed readback is a serious partial outcome: do not repeat the CAS.
	readbackErr := reconcile(ctx, d, p, pre, fifth)
	result := map[string]any{"target_id": p.TargetID, "permit_sha256": permitHash, "binary_sha256": binHash, "escrow_sha256": p.EscrowSHA256, "credential_version": p.CredentialVersion, "at": time.Now().UTC(), "reconciled": readbackErr == nil, "status": "inactive expected; no activation or sends"}
	receiptData, _ := json.MarshalIndent(result, "", "  ")
	if _, err := f.Write(append(receiptData, '\n')); err != nil {
		fmt.Fprintln(stderr, "receipt write failed after CAS: manual reconciliation required")
		return 1
	}
	if err := f.Sync(); err != nil {
		fmt.Fprintln(stderr, "receipt sync failed after CAS: manual reconciliation required")
		return 1
	}
	if readbackErr != nil {
		fmt.Fprintln(stderr, "CAS committed but readback failed; manual reconciliation required:", readbackErr)
		return 1
	}
	fmt.Fprintln(stdout, "one-row CAS reconciled; private receipt written; mailbox remains inactive")
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
