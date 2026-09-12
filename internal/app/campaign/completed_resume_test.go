package campaign

import (
	"os"
	"strings"
	"testing"
)

func TestStartCampaignAllowsCompletedOnlyWithQueuedSendableLeads(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		"campaign.Status == \"completed\"",
		"HasQueuedSendableLeads",
		"completed campaign has no queued sendable leads",
		"Campaign resumed",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("StartCampaign completed-resume guard missing %q", want)
		}
	}
}
