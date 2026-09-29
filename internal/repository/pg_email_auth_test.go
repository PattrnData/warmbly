package repository

import (
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/encrypt"
)

func TestSealOAuthTokensFailClosedAndRoundTrip(t *testing.T) {
	r := &emailRepository{}
	if _, _, err := r.sealOAuthTokens("synthetic-access", "synthetic-refresh"); err == nil {
		t.Fatal("missing encryption key accepted")
	}
	if _, _, err := r.sealOAuthTokens("", ""); err == nil {
		t.Fatal("empty delegated tokens accepted")
	}
	key := []byte(strings.Repeat("x", 32)) // test-only key, no customer credentials
	var err error
	r.Encrypt, err = encrypt.NewEncrypter(key)
	if err != nil {
		t.Fatal(err)
	}
	a, b, err := r.sealOAuthTokens("synthetic-access", "synthetic-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if a == "synthetic-access" || b == "synthetic-refresh" || a == b {
		t.Fatal("delegated credentials were not independently sealed")
	}
	da, err := r.Encrypt.Decrypt(a)
	if err != nil || da != "synthetic-access" {
		t.Fatal("access token cannot round trip")
	}
	db, err := r.Encrypt.Decrypt(b)
	if err != nil || db != "synthetic-refresh" {
		t.Fatal("refresh token cannot round trip")
	}
	a, b, err = r.sealOAuthTokens("", models.GraphAppOnlyRefreshToken)
	if err != nil || a != "" || b != models.GraphAppOnlyRefreshToken {
		t.Fatal("app-only sentinel changed")
	}
	if _, _, err := r.sealOAuthTokens("synthetic-access", models.GraphAppOnlyRefreshToken); err == nil {
		t.Fatal("mixed delegated/app-only credential shape accepted")
	}
}
