package repository

import (
	"os"
	"strings"
	"testing"
)

func TestFindNextRoutedPairAllowsUnverifiedQueuedContacts(t *testing.T) {
	// Contacts imported without a verifier result have NULL verification_status in
	// production. They must remain schedulable unless explicitly marked invalid or
	// risky; otherwise the campaign lead list shows queued contacts while the
	// scheduler sees no sendable pair and marks the campaign completed.
	src, err := os.ReadFile("pg_campaign_progress.go")
	if err != nil {
		t.Fatal(err)
	}
	query := string(src)
	if !strings.Contains(query, "(c.verification_status IS NULL OR c.verification_status <> 'invalid')") {
		t.Fatalf("FindNextRoutedPair must allow NULL verification_status as not invalid")
	}
	if !strings.Contains(query, "(camp0.risky_emails OR c.verification_status IS NULL OR c.verification_status <> 'risky')") {
		t.Fatalf("FindNextRoutedPair must allow NULL verification_status as not risky when risky sends are disabled")
	}
}
