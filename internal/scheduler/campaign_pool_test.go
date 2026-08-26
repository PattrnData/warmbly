package scheduler

import (
	"errors"
	"testing"
)

func TestCampaignDeferredDoesNotMasqueradeAsNoEmailAccounts(t *testing.T) {
	if err := campaignPoolUnavailable(0); !errors.Is(err, ErrNoEmailAccounts) {
		t.Fatalf("an empty configured pool must report no accounts, got %v", err)
	}
	err := campaignPoolUnavailable(140)
	if !errors.Is(err, ErrCampaignDeferred) {
		t.Fatalf("a configured but temporarily ineligible pool must defer, got %v", err)
	}
	if errors.Is(err, ErrNoEmailAccounts) {
		t.Fatal("a temporarily ineligible mailbox pool must defer, not auto-pause as no accounts")
	}
}
