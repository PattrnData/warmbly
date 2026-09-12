package repository

import (
	"os"
	"strings"
	"testing"
)

func TestCreateCampaignEmbedsStandardReplyActionPolicy(t *testing.T) {
	modelSrc, err := os.ReadFile("../models/campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(modelSrc)
	if !strings.Contains(s, "DefaultReplyActionPolicy") {
		t.Fatalf("CreateCampaign must expose default_reply_action_policy for standardized reply scaffolding")
	}
	for _, want := range []string{"Conditions", "*BranchConditions", "Action", "*ActionConfig", "Kind", "*string"} {
		if !strings.Contains(s, want) {
			t.Fatalf("CreateSequenceInput must accept kind/action/conditions so initial campaign creation can persist reply-action graphs atomically; missing %q", want)
		}
	}

	repoSrc, err := os.ReadFile("pg_campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	repo := string(repoSrc)
	for _, want := range []string{
		"applyStandardReplyActionScaffold",
		"reply_positive",
		"reply_question",
		"reply_wrong_person",
		"reply_bad_timing",
		"reply_referral",
		"reply_automated",
		"reply_negative",
		"unsubscribe",
		"fire_event",
	} {
		if !strings.Contains(repo, want) {
			t.Fatalf("standard reply scaffold missing %q", want)
		}
	}
}
