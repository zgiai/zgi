package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentruntime"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestAgentRuntimeMigrationAndBindingRepository(t *testing.T) {
	dsn := testutil.CreateTestPostgresDSN(t)
	store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var migrationCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = $1`, int64(2026092901)).Scan(&migrationCount); err != nil {
		t.Fatalf("query migration: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration count = %d, want 1", migrationCount)
	}

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	binding := agentruntime.Binding{
		ID:             "binding-1",
		OrganizationID: "org-1",
		SubjectID:      "user-1",
		AgentID:        "agent-1",
		ConversationID: "conversation-1",
		ProfileID:      "profile-1",
		WorkspaceID:    "workspace-1",
		Backend:        "fake",
		IdempotencyKey: "request-1",
		RequestHash:    "hash-1",
		Status:         agentruntime.BindingStatusAllocating,
		Generation:     1,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(30 * time.Minute),
	}
	created, wasCreated, err := store.ReserveBinding(context.Background(), binding)
	if err != nil {
		t.Fatalf("reserve binding: %v", err)
	}
	if !wasCreated || created.ID != binding.ID {
		t.Fatalf("unexpected reservation: created=%v binding=%+v", wasCreated, created)
	}
	duplicate, wasCreated, err := store.ReserveBinding(context.Background(), agentruntime.Binding{
		ID:             "binding-other",
		OrganizationID: binding.OrganizationID,
		SubjectID:      binding.SubjectID,
		IdempotencyKey: binding.IdempotencyKey,
	})
	if err != nil {
		t.Fatalf("reserve duplicate: %v", err)
	}
	if wasCreated || duplicate.ID != binding.ID {
		t.Fatalf("duplicate reservation = (%+v, %v), want original", duplicate, wasCreated)
	}

	binding.Status = agentruntime.BindingStatusActive
	binding.Generation = 2
	binding.BackendRef = "backend-1"
	if err := store.UpdateBinding(context.Background(), binding, 1); err != nil {
		t.Fatalf("update binding: %v", err)
	}
	if err := store.UpdateBinding(context.Background(), binding, 1); !errors.Is(err, agentruntime.ErrGenerationConflict) {
		t.Fatalf("stale update error = %v, want generation conflict", err)
	}
	loaded, err := store.Binding(context.Background(), binding.ID)
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	if loaded.Status != agentruntime.BindingStatusActive || loaded.Generation != 2 {
		t.Fatalf("loaded binding = %+v", loaded)
	}
}
