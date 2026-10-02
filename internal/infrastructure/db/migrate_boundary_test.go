package db

import (
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// The deployed database is at version 83; the embedded source must retain that
// history so golang-migrate can start its upgrade to 84-87.
func TestEmbeddedMigration83UpgradeBoundary(t *testing.T) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := source.ReadDown(83); err != nil {
		t.Fatalf("version 83 down: %v", err)
	}
	if _, _, err := source.ReadUp(83); err != nil {
		t.Fatalf("version 83 up: %v", err)
	}
	if _, _, err := source.ReadDown(82); err != nil {
		t.Fatalf("version 82 down: %v", err)
	}
	if _, _, err := source.ReadUp(82); err != nil {
		t.Fatalf("version 82 up: %v", err)
	}
	next, err := source.Next(83)
	if err != nil || next != 84 {
		t.Fatalf("next after 83: %d, %v", next, err)
	}
}
