package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestCalculateNextWarmupTimePersistsComputedTargetVolume(t *testing.T) {
	src, err := os.ReadFile("warmup_scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func (s *schedulerService) CalculateNextWarmupTime")
	if start < 0 {
		t.Fatal("CalculateNextWarmupTime not found")
	}
	section := body[start:]
	if !strings.Contains(section, "GetOrCreateDailyStats(ctx, accountID") {
		t.Fatal("CalculateNextWarmupTime must persist computed target_volume for warmup analytics")
	}
	if strings.Contains(section, "GetOrCreateDailyStats(ctx, accountID, time.Now(), 0)") {
		t.Fatal("CalculateNextWarmupTime must not persist zero target_volume")
	}
}
