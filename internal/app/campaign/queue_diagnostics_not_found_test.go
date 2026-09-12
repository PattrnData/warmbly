package campaign

import (
	"os"
	"strings"
	"testing"
)

func TestQueueDiagnosticsMapsRepositoryNotFoundToNotFound(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (s *campaignService) QueueDiagnostics")
	if start < 0 {
		t.Fatal("missing QueueDiagnostics service method")
	}
	end := strings.Index(s[start:], "func (s *campaignService) Overview")
	if end < 0 {
		t.Fatal("missing QueueDiagnostics service end marker")
	}
	section := s[start : start+end]
	for _, want := range []string{"errx.ErrResourceNotFound", "errx.ErrNotFound"} {
		if !strings.Contains(section, want) {
			t.Fatalf("QueueDiagnostics service must map repo not-found to typed 404; missing %q", want)
		}
	}
}
