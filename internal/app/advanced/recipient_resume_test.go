package advanced

import (
	"os"
	"strings"
	"testing"
)

func TestProcessIncomingReplySetsRecipientResumeForOOOAndBadTiming(t *testing.T) {
	src, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{"MarkRecipientResumeAt", "ClassOutOfOffice", "ClassBadTiming", "72*time.Hour"} {
		if !strings.Contains(s, want) {
			t.Fatalf("reply processing must set recipient-level resume_at for OOO/later replies; missing %q", want)
		}
	}
}
