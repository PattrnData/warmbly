package repository

import (
	"os"
	"strings"
	"testing"
)

func TestQueueDiagnosticsMatchesSchedulerSafetyPredicates(t *testing.T) {
	src, err := os.ReadFile("pg_campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (r *campaignRepository) QueueDiagnostics")
	if start < 0 {
		t.Fatal("missing QueueDiagnostics")
	}
	end := strings.Index(s[start:], "func (r *campaignRepository) Delete")
	if end < 0 {
		t.Fatal("missing QueueDiagnostics function end marker")
	}
	section := s[start : start+end]
	for _, want := range []string{
		"WITH campaign_scope AS",
		"WHERE id = $2 AND organization_id = $1",
		"suppressed_recipients",
		"resume_at IS NULL OR resume_at <= NOW()",
		"errors.Is(err, pgx.ErrNoRows)",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("QueueDiagnostics must be scoped and scheduler-equivalent; missing %q", want)
		}
	}
}

func TestCompletedResumeGuardMatchesSchedulerSafetyPredicates(t *testing.T) {
	src, err := os.ReadFile("pg_campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (r *campaignRepository) HasQueuedSendableLeads")
	if start < 0 {
		t.Fatal("missing HasQueuedSendableLeads")
	}
	section := s[start:]
	for _, want := range []string{
		"suppressed_recipients",
		"pr.resume_at IS NULL OR pr.resume_at <= NOW()",
		"c.verification_status IS NULL OR c.verification_status <> 'invalid'",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("HasQueuedSendableLeads must match scheduler safety predicates; missing %q", want)
		}
	}
}
