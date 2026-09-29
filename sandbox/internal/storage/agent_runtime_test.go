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

func TestAgentRuntimeLeaseAndCleanupClaims(t *testing.T) {
	dsn := testutil.CreateTestPostgresDSN(t)
	store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	lease := agentruntime.AdmissionLease{
		ID: "lease-1", BindingID: "binding-1", OwnerID: "worker-1", Generation: 1,
		ExpiresAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	acquired, err := store.AcquireLease(context.Background(), lease, now)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	competing := lease
	competing.ID = "lease-2"
	competing.OwnerID = "worker-2"
	if _, err := store.AcquireLease(context.Background(), competing, now); !errors.Is(err, agentruntime.ErrLeaseHeld) {
		t.Fatalf("competing lease error = %v, want ErrLeaseHeld", err)
	}
	renewed, err := store.RenewLease(context.Background(), acquired.ID, acquired.OwnerID, acquired.Generation, now.Add(10*time.Second), now.Add(70*time.Second))
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if renewed.Generation != acquired.Generation+1 {
		t.Fatalf("renewed generation = %d, want %d", renewed.Generation, acquired.Generation+1)
	}
	if err := store.ReleaseLease(context.Background(), acquired.ID, acquired.OwnerID, acquired.Generation); !errors.Is(err, agentruntime.ErrGenerationConflict) {
		t.Fatalf("stale release error = %v, want generation conflict", err)
	}
	if err := store.ReleaseLease(context.Background(), renewed.ID, renewed.OwnerID, renewed.Generation); err != nil {
		t.Fatalf("release lease: %v", err)
	}

	cleanup := agentruntime.CleanupTask{
		ID: "cleanup-1", ResourceType: "binding", ResourceID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", Action: "destroy", Status: "pending", NotBefore: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.EnqueueCleanup(context.Background(), cleanup); err != nil {
		t.Fatalf("enqueue cleanup: %v", err)
	}
	claimed, ok, err := store.ClaimCleanup(context.Background(), "worker-1", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim cleanup: ok=%v error=%v", ok, err)
	}
	if claimed.Attempts != 1 || claimed.LeaseOwner != "worker-1" {
		t.Fatalf("claimed cleanup = %+v", claimed)
	}
	if _, ok, err := store.ClaimCleanup(context.Background(), "worker-2", now, now.Add(time.Minute)); err != nil || ok {
		t.Fatalf("concurrent claim: ok=%v error=%v", ok, err)
	}
	retryAt := now.Add(30 * time.Second)
	if err := store.CompleteCleanup(context.Background(), claimed.ID, "worker-1", false, "retry", retryAt, now); err != nil {
		t.Fatalf("schedule cleanup retry: %v", err)
	}
	if _, ok, err := store.ClaimCleanup(context.Background(), "worker-2", now.Add(20*time.Second), now.Add(time.Minute)); err != nil || ok {
		t.Fatalf("early retry claim: ok=%v error=%v", ok, err)
	}
	claimed, ok, err = store.ClaimCleanup(context.Background(), "worker-2", retryAt, retryAt.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("retry claim: ok=%v error=%v", ok, err)
	}
	if err := store.CompleteCleanup(context.Background(), claimed.ID, "worker-2", true, "", retryAt, retryAt); err != nil {
		t.Fatalf("complete cleanup: %v", err)
	}

	cleanup.ID = "cleanup-2"
	cleanup.ResourceID = "binding-2"
	if err := store.EnqueueCleanup(context.Background(), cleanup); err != nil {
		t.Fatalf("enqueue expiring cleanup: %v", err)
	}
	claimed, ok, err = store.ClaimCleanup(context.Background(), "worker-dead", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim expiring cleanup: ok=%v error=%v", ok, err)
	}
	afterExpiry := now.Add(2 * time.Minute)
	if err := store.CompleteCleanup(context.Background(), claimed.ID, "worker-dead", true, "", afterExpiry, afterExpiry); !errors.Is(err, agentruntime.ErrCleanupClaimLost) {
		t.Fatalf("expired completion error = %v, want claim lost", err)
	}
	claimed, ok, err = store.ClaimCleanup(context.Background(), "worker-recovery", afterExpiry, afterExpiry.Add(time.Minute))
	if err != nil || !ok || claimed.Attempts != 2 {
		t.Fatalf("recover expired cleanup: cleanup=%+v ok=%v error=%v", claimed, ok, err)
	}
}
