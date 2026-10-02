package sendauth

import (
	"strings"
	"testing"
)

func TestBindingSQLChecksCurrentAccountAndTaskAtSendTime(t *testing.T) {
	for _, fragment := range []string{"t.id = $1", "t.email_account_id = $2", "ea.organization_id = $3", "ea.worker_id = $4", "t.message_id = $5", "t.status IN ('active','completed')", "ea.status = 'active'", "w.active IS TRUE"} {
		if !strings.Contains(BindingSQL, fragment) {
			t.Fatalf("missing binding %q", fragment)
		}
	}
}
