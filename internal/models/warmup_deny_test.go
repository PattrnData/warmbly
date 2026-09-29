package models

import (
	"testing"
	"time"
)

func TestWarmupDenyOverridesEnabledAndPausedState(t *testing.T) {
	now := time.Now()
	mailbox := Email{Warmup: &now}
	if !mailbox.IsWarmingActive() {
		t.Fatal("enabled mailbox should warm")
	}
	mailbox.WarmupDenied = true
	if mailbox.IsWarmingActive() {
		t.Fatal("denied enabled mailbox should not warm")
	}
	mailbox.Warmup = nil
	if mailbox.IsWarmingActive() {
		t.Fatal("denied disabled mailbox should not warm")
	}
}
