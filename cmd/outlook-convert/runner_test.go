package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validPermit() permit {
	now := time.Now().UTC()
	return permit{
		TargetID: uuid.MustParse("af571c6e-e6f0-4cb9-90fe-a7d5105babd7"),
		OwnerID:  uuid.New(), OrgID: uuid.New(), WorkerID: uuid.New(), TenantID: uuid.New(),
		Email: "sender@example.test", AppID: "app-id", CredentialVersion: strings.Repeat("a", 32),
		SourceSHA256: strings.Repeat("b", 64), ImageDigest: "sha256:" + strings.Repeat("c", 64),
		BinarySHA256: strings.Repeat("d", 64), ConfigSHA256: strings.Repeat("e", 64), EscrowSHA256: strings.Repeat("f", 64),
		Reviewer: "independent-reviewer", GrantEvidenceSHA256: strings.Repeat("1", 64), FenceEvidenceSHA256: strings.Repeat("2", 64),
		ReviewedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), NoSendFrom: now.Add(-time.Minute), NoSendUntil: now.Add(time.Hour),
	}
}

func TestPermitRejectsUnsafeAndExpired(t *testing.T) {
	p := validPermit()
	if err := validatePermit(p, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*permit)
	}{
		{"fifth", func(p *permit) { p.TargetID = uuid.MustParse("a5f28cfb-b10f-4597-b445-28e647e0dd92") }},
		{"nil worker", func(p *permit) { p.WorkerID = uuid.Nil }},
		{"no reviewer", func(p *permit) { p.Reviewer = "" }},
		{"no fence evidence", func(p *permit) { p.FenceEvidenceSHA256 = "" }},
		{"expired", func(p *permit) { p.ExpiresAt = time.Now().Add(-time.Second) }},
		{"window not started", func(p *permit) { p.NoSendFrom = time.Now().Add(time.Minute) }},
		{"window ended", func(p *permit) { p.NoSendUntil = time.Now().Add(-time.Second) }},
		{"invalid config digest", func(p *permit) { p.ConfigSHA256 = "abc" }},
		{"future review", func(p *permit) { p.ReviewedAt = time.Now().Add(time.Minute) }},
		{"long permit", func(p *permit) { p.ExpiresAt = time.Now().Add(48 * time.Hour); p.NoSendUntil = p.ExpiresAt }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := p
			tc.change(&q)
			if validatePermit(q, time.Now().UTC()) == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestArgumentsNeverTouchEnvironmentBeforeExecute(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"--version"}, {"--execute"}, {"--unknown"}, {"--execute", "--target", "abc"}, {"--dry-run", "--target", uuid.NewString()}} {
		var out, errs bytes.Buffer
		code := run(args, &out, &errs)
		if code == 0 && len(args) == 0 {
			t.Fatal("empty args accepted")
		}
		if strings.Contains(out.String(), "postgres") || strings.Contains(errs.String(), "postgres") {
			t.Fatal("environment accessed")
		}
	}
}

func TestWriteWindowRecheckedAfterGraph(t *testing.T) {
	p := validPermit()
	now := time.Now().UTC()
	g := grantEvidence{TargetID: p.TargetID, TenantID: p.TenantID, AppID: p.AppID, Email: p.Email, MailReadWrite: true, MailSend: true, RestrictionEffective: true, ObservedAt: now}
	f := fenceEvidence{TargetID: p.TargetID, DispatchFenced: true, ObservedAt: now, FenceUntil: p.ExpiresAt.Add(time.Minute)}
	if !writeWindowOpen(p, g, f, now) {
		t.Fatal("valid write window rejected")
	}
	for _, tc := range []struct {
		name string
		at   time.Time
	}{
		{"permit expired", p.ExpiresAt},
		{"grant stale after Graph", now.Add(5 * time.Minute)},
		{"fence expired", f.FenceUntil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if writeWindowOpen(p, g, f, tc.at) {
				t.Fatal("expired write window accepted")
			}
		})
	}
	f.FenceUntil = now.Add(-time.Second)
	if writeWindowOpen(p, g, f, now) {
		t.Fatal("elapsed fence accepted")
	}
}

func TestEscrowPreimageExactEquality(t *testing.T) {
	acc := json.RawMessage(`{"id":"one","status":"inactive"}`)
	oauth := json.RawMessage(`{"email_account_id":"one","access_token":"sealed","refresh_token":"sealed2"}`)
	e := preimage{Account: acc, OAuth: oauth}
	if err := samePreimage(e, acc, oauth); err != nil {
		t.Fatal(err)
	}
	if err := samePreimage(e, acc, json.RawMessage(`{"email_account_id":"one","access_token":"different","refresh_token":"sealed2"}`)); err == nil {
		t.Fatal("credential drift accepted")
	}
	if err := samePreimage(e, json.RawMessage(`{"id":"one","status":"active"}`), oauth); err == nil {
		t.Fatal("account drift accepted")
	}
}

func TestEvidenceRequiresFreshTargetGrantAndDrainedFence(t *testing.T) {
	p := validPermit()
	now := time.Now().UTC()
	g := grantEvidence{TargetID: p.TargetID, TenantID: p.TenantID, AppID: p.AppID, Email: p.Email, MailReadWrite: true, MailSend: true, RestrictionEffective: true, ObservedAt: now}
	f := fenceEvidence{TargetID: p.TargetID, DispatchFenced: true, BrokerPending: 0, WorkerInFlight: 0, ObservedAt: now, FenceUntil: p.ExpiresAt.Add(time.Minute)}
	if err := verifyEvidence(p, g, f, now); err != nil {
		t.Fatal(err)
	}
	g.RestrictionEffective = false
	if verifyEvidence(p, g, f, now) == nil {
		t.Fatal("accepted unrestricted grant")
	}
	g.RestrictionEffective = true
	g.ObservedAt = now.Add(-10 * time.Minute)
	if verifyEvidence(p, g, f, now) == nil {
		t.Fatal("accepted stale grant")
	}
	g.ObservedAt = now
	f.BrokerPending = 1
	if verifyEvidence(p, g, f, now) == nil {
		t.Fatal("accepted pending broker message")
	}
	f.BrokerPending = 0
	f.TargetID = uuid.New()
	if verifyEvidence(p, g, f, now) == nil {
		t.Fatal("accepted wrong fence target")
	}
}
func TestReviewerSignaturePinnedToExactPermit(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg unavailable")
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	cmd := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Reviewer Test <reviewer@example.test>", "default", "default", "never")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v: %s", err, out)
	}
	list, err := exec.Command("gpg", "--with-colons", "--list-keys").Output()
	if err != nil {
		t.Fatal(err)
	}
	var fingerprint string
	for _, line := range strings.Split(string(list), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) > 9 && parts[0] == "fpr" {
			fingerprint = parts[9]
			break
		}
	}
	if fingerprint == "" {
		t.Fatal("test fingerprint missing")
	}
	permitFile := filepath.Join(home, "permit.json")
	if err := os.WriteFile(permitFile, []byte(`{"target_id":"approved"}`), 0600); err != nil {
		t.Fatal(err)
	}
	sig := filepath.Join(home, "permit.sig")
	cmd = exec.Command("gpg", "--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase", "", "--output", sig, "--detach-sign", permitFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sign test permit: %v: %s", err, out)
	}
	signedBytes, err := os.ReadFile(permitFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := gpgVerify(context.Background(), filepath.Join(home, "pubring.kbx"), sig, signedBytes, fingerprint); err != nil {
		t.Fatal(err)
	}
	if gpgVerify(context.Background(), filepath.Join(home, "pubring.kbx"), sig, signedBytes, strings.Repeat("A", 40)) == nil {
		t.Fatal("accepted wrong reviewer")
	}
	if err := os.WriteFile(permitFile, []byte(`{"target_id":"modified"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// An atomic path swap cannot turn a signature on B into approval of parsed A.
	if gpgVerify(context.Background(), filepath.Join(home, "pubring.kbx"), sig, []byte(`{"target_id":"modified"}`), fingerprint) == nil {
		t.Fatal("accepted unsigned parsed bytes")
	}
	if err := gpgVerify(context.Background(), filepath.Join(home, "pubring.kbx"), sig, signedBytes, fingerprint); err != nil {
		t.Fatalf("path replacement changed verification of captured bytes: %v", err)
	}
}

func TestDryRunLocalAndExecuteClosedWithoutReviewedBuild(t *testing.T) {
	p := validPermit()
	data, _ := json.Marshal(p)
	path := filepath.Join(t.TempDir(), "permit.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--dry-run", "--target", p.TargetID.String(), "--permit", path, "--permit-sha256", digest(data)}
	var out, errs bytes.Buffer
	if code := run(args, &out, &errs); code != 0 {
		t.Fatalf("dry-run failed %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "production HOLD") {
		t.Fatal(out.String())
	}
	out.Reset()
	errs.Reset()
	args[0] = "--execute"
	if code := run(args, &out, &errs); code == 0 {
		t.Fatal("unreviewed build executed")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "receipt.json")); !os.IsNotExist(err) {
		t.Fatal("side effect on invalid args")
	}
}

func TestEscrowDecryptsOnlyCapturedCiphertext(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg unavailable")
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	path := filepath.Join(home, "escrow.gpg")
	key := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Escrow Test <escrow@example.test>", "default", "default", "never")
	if out, err := key.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v: %s", err, out)
	}
	encrypt := func(text string) []byte {
		t.Helper()
		cmd := exec.Command("gpg", "--batch", "--yes", "--trust-model", "always", "--recipient", "escrow@example.test", "--encrypt", "--output", path)
		cmd.Stdin = strings.NewReader(text)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("encrypt: %v: %s", err, out)
		}
		ciphertext, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return ciphertext
	}
	approved := encrypt("approved-preimage")
	_ = encrypt("swapped-preimage")
	got, err := decryptEscrow(context.Background(), approved)
	if err != nil || string(got) != "approved-preimage" {
		t.Fatalf("escrow path swap affected captured bytes: %v %q", err, got)
	}
}
