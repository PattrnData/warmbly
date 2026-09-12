package repository

import (
	"os"
	"strings"
	"testing"
)

func TestContactCreateNormalizesMissingVerificationStatus(t *testing.T) {
	src, err := os.ReadFile("pg_contact.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "id, user_id, organization_id, first_name, last_name, email, company, phone, custom_fields, verification_status") || !strings.Contains(s, "gen_random_uuid(), $1, $2, $3, $4, LOWER($5), $6, $7, $8, 'unknown'") {
		t.Fatalf("contact create/upsert must persist missing verification_status as 'unknown' instead of NULL")
	}
}
