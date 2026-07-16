package tool_file

import (
	"errors"
	"testing"
	"time"
)

func TestResolveLifecycleUsesSevenDayTemporaryTTL(t *testing.T) {
	before := time.Now().Add(DefaultTemporaryToolFileTTL)
	lifecycle, expiresAt, err := resolveLifecycle(ToolFileLifecycleTemporary, nil)
	after := time.Now().Add(DefaultTemporaryToolFileTTL)
	if err != nil {
		t.Fatalf("resolveLifecycle() error = %v", err)
	}
	if lifecycle != ToolFileLifecycleTemporary || expiresAt == nil {
		t.Fatalf("resolveLifecycle() = %q, %v, want temporary expiry", lifecycle, expiresAt)
	}
	if expiresAt.Before(before) || expiresAt.After(after) {
		t.Fatalf("expiresAt = %v, want between %v and %v", expiresAt, before, after)
	}
}

func TestEnsureToolFileAvailableRejectsExpiredTemporaryFile(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(-time.Second)
	err := ensureToolFileAvailable(&ToolFile{
		ID:        "tool-1",
		Lifecycle: string(ToolFileLifecycleTemporary),
		ExpiresAt: &expiresAt,
	}, now)
	if !errors.Is(err, ErrToolFileExpired) {
		t.Fatalf("ensureToolFileAvailable() error = %v, want ErrToolFileExpired", err)
	}
}

func TestEnsureToolFileAvailableKeepsUnexpiredAndPersistentFiles(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	for _, toolFile := range []*ToolFile{
		{ID: "temporary", Lifecycle: string(ToolFileLifecycleTemporary), ExpiresAt: &future},
		{ID: "persistent", Lifecycle: string(ToolFileLifecyclePersistent)},
	} {
		if err := ensureToolFileAvailable(toolFile, now); err != nil {
			t.Fatalf("ensureToolFileAvailable(%s) error = %v", toolFile.ID, err)
		}
	}
}
