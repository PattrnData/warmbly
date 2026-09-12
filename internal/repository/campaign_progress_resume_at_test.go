package repository

import (
	"os"
	"strings"
	"testing"
)

func TestCampaignProgressSupportsRecipientResumeAt(t *testing.T) {
	migration, err := os.ReadFile("../infrastructure/db/migrations/000083_campaign_contact_resume_at.up.sql")
	if err != nil {
		t.Fatalf("missing migration for recipient-level resume_at: %v", err)
	}
	if !strings.Contains(string(migration), "resume_at") || !strings.Contains(string(migration), "campaign_contact_progress") {
		t.Fatalf("migration must add campaign_contact_progress.resume_at")
	}

	src, err := os.ReadFile("pg_campaign_progress.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{"resume_at", "MarkRecipientResumeAt", "lp.resume_at IS NULL OR lp.resume_at <= NOW()"} {
		if !strings.Contains(s, want) {
			t.Fatalf("campaign scheduler/progress must support recipient-level resume_at guard %q", want)
		}
	}
}
