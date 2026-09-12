package repository

import (
	"os"
	"strings"
	"testing"
)

func TestCreateCampaignDoesNotDefaultToVisualReplyActionScaffold(t *testing.T) {
	modelSrc, err := os.ReadFile("../models/campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(modelSrc)
	if !strings.Contains(s, "DefaultReplyActionPolicy") {
		t.Fatalf("CreateCampaign must retain default_reply_action_policy as an explicit opt-in compatibility field")
	}
	if strings.Contains(s, "Empty defaults to \"standard_pattrn\"") {
		t.Fatalf("default_reply_action_policy docs must not say visual reply scaffolding is the default")
	}

	repoSrc, err := os.ReadFile("pg_campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	repo := string(repoSrc)
	if strings.Contains(repo, "policy := \"standard_pattrn\"") {
		t.Fatalf("new campaign creation must not default to visual reply-action scaffolding")
	}
	if !strings.Contains(repo, "policy := \"none\"") {
		t.Fatalf("new campaign creation should default reply action policy to none/webhook-first")
	}
	if !strings.Contains(repo, "applyStandardReplyActionScaffold") {
		t.Fatalf("explicit standard_pattrn compatibility path should remain available for legacy/manual visual flows")
	}
}
