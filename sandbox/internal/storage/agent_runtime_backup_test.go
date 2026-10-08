package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestCheckpointRecoverySnapshotAndEmptyTarget(t *testing.T) {
	dsn := testutil.CreateTestPostgresDSN(t)
	store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := admissionBinding("source", "org")
	b.Status = agentexecution.BindingStatusSuspended
	b.Generation = 3
	b.CheckpointKey = strings.Repeat("a", 64)
	b.BackendRef = "private-source-compute"
	b.WorkspaceRef = "private-source-workspace"
	b.FailureReason = "old private diagnostic"
	b.RetentionExpiresAt = time.Now().Add(time.Hour)
	if _, _, err := store.ReserveBinding(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	bindings, err := ReadCheckpointBindings(t.Context(), dsn, "fake")
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].BackendRef != "" || bindings[0].WorkspaceRef != "" || bindings[0].FailureReason != "" {
		t.Fatalf("unsafe backup records: %+v", bindings)
	}
	if err := CheckRecoveryTarget(t.Context(), dsn); err == nil {
		t.Fatal("populated source accepted as target")
	}
	if err := store.RestoreCheckpointBindings(t.Context(), "fake", bindings); err == nil {
		t.Fatal("source overwritten by restore")
	}
	targetDSN := testutil.CreateTestPostgresDSN(t)
	if err := CheckRecoveryTarget(t.Context(), targetDSN); err != nil {
		t.Fatal(err)
	}
	target, err := Open(config.Config{DatabaseURL: targetDSN, MaxWorkers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := target.RestoreCheckpointBindings(t.Context(), "fake", bindings); err != nil {
		t.Fatal(err)
	}
	actual, err := target.Binding(t.Context(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Generation != b.Generation+1 || actual.BackendRef != "" || actual.WorkspaceRef != "" || actual.CheckpointKey != b.CheckpointKey || !actual.RetentionExpiresAt.Equal(b.RetentionExpiresAt.Truncate(time.Microsecond)) {
		t.Fatalf("recovered=%+v", actual)
	}
	if err := target.RestoreCheckpointBindings(t.Context(), "fake", bindings); err == nil {
		t.Fatal("restore overwritten on retry")
	}
	original, err := store.Binding(t.Context(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if original.BackendRef != b.BackendRef || original.Generation != b.Generation {
		t.Fatal("source resource mutated")
	}
	active := b
	active.ID = "active"
	active.IdempotencyKey = "active"
	active.Status = agentexecution.BindingStatusActive
	if _, _, err := store.ReserveBinding(t.Context(), active); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCheckpointBindings(t.Context(), dsn, "fake"); err == nil {
		t.Fatal("live backup accepted")
	}
}
