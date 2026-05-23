package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNextMigrationIDPrefixUsesUTCTimestampAndRandomSuffix(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 5, 24, 0, 13, 42, 0, time.FixedZone("CST", 8*60*60))

	got, err := nextMigrationIDPrefix(now, dir, func() (int, error) {
		return 827, nil
	})
	if err != nil {
		t.Fatalf("next migration ID prefix: %v", err)
	}

	want := "202605231613420827"
	if got != want {
		t.Fatalf("migration ID prefix = %q, want %q", got, want)
	}
}

func TestNextMigrationIDPrefixRetriesLocalPrefixCollision(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	existing := filepath.Join(dir, "202606010900000001_existing.go")
	if err := os.WriteFile(existing, []byte("package migrations\n"), 0644); err != nil {
		t.Fatalf("write existing migration: %v", err)
	}

	suffixes := []int{1, 2}
	got, err := nextMigrationIDPrefix(now, dir, func() (int, error) {
		if len(suffixes) == 0 {
			t.Fatal("random suffix called too many times")
		}
		next := suffixes[0]
		suffixes = suffixes[1:]
		return next, nil
	})
	if err != nil {
		t.Fatalf("next migration ID prefix: %v", err)
	}

	want := "202606010900000002"
	if got != want {
		t.Fatalf("migration ID prefix = %q, want %q", got, want)
	}
}

func TestNextMigrationIDPrefixFailsOnRandomError(t *testing.T) {
	wantErr := errors.New("entropy unavailable")

	_, err := nextMigrationIDPrefix(time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), t.TempDir(), func() (int, error) {
		return 0, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected random error %v, got %v", wantErr, err)
	}
}
