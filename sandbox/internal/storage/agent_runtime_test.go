package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
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
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = $1`, int64(2026093001)).Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("retention migration count = %d, error=%v", migrationCount, err)
	}

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	binding := agentruntime.Binding{
		ID:                 "binding-1",
		OrganizationID:     "org-1",
		SubjectID:          "user-1",
		AgentID:            "agent-1",
		ConversationID:     "conversation-1",
		ProfileID:          "profile-1",
		WorkspaceID:        "workspace-1",
		Backend:            "fake",
		IdempotencyKey:     "request-1",
		RequestHash:        "hash-1",
		Status:             agentruntime.BindingStatusAllocating,
		Generation:         1,
		CreatedAt:          now,
		UpdatedAt:          now,
		LastActivityAt:     now,
		ExpiresAt:          now.Add(30 * time.Minute),
		RetentionExpiresAt: now.Add(7 * 24 * time.Hour),
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
	binding.CheckpointKey = "checkpoint-reference"
	binding.PendingCheckpointKey = "pending-checkpoint-reference"
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
	if loaded.Status != agentruntime.BindingStatusActive || loaded.Generation != 2 || loaded.CheckpointKey != binding.CheckpointKey || loaded.PendingCheckpointKey != binding.PendingCheckpointKey {
		t.Fatalf("loaded binding = %+v", loaded)
	}
	touchedAt := now.Add(time.Minute)
	touched, err := store.TouchBinding(context.Background(), binding.ID, loaded.Generation, touchedAt, touchedAt.Add(30*time.Minute), touchedAt.Add(7*24*time.Hour))
	if err != nil {
		t.Fatalf("touch binding: %v", err)
	}
	if touched.Generation != 3 || !touched.LastActivityAt.Equal(touchedAt) {
		t.Fatalf("touched binding = %+v", touched)
	}
	if expired, err := store.ExpiredBindings(context.Background(), touched.ExpiresAt.Add(-time.Second), 10); err != nil || len(expired) != 0 {
		t.Fatalf("early expiry = %+v, error=%v", expired, err)
	}
	if expired, err := store.ExpiredBindings(context.Background(), touched.ExpiresAt, 10); err != nil || len(expired) != 1 || expired[0].Generation != touched.Generation {
		t.Fatalf("expiry = %+v, error=%v", expired, err)
	}
	touched.Status = agentruntime.BindingStatusSuspended
	touched.Generation++
	touched.CheckpointKey = "next-checkpoint-reference"
	if err := store.UpdateBinding(context.Background(), touched, touched.Generation-1); err != nil {
		t.Fatal(err)
	}
	cleanup, claimed, err := store.ClaimCleanup(context.Background(), "checkpoint-worker", touched.UpdatedAt, touched.UpdatedAt.Add(time.Minute))
	if err != nil || !claimed || cleanup.ResourceType != "checkpoint" || cleanup.ResourceID != "checkpoint-reference" || cleanup.Action != "delete" {
		t.Fatalf("superseded checkpoint cleanup = %+v, claimed=%v, error=%v", cleanup, claimed, err)
	}
	if retained, err := store.ExpiredRetainedBindings(context.Background(), touched.RetentionExpiresAt.Add(-time.Second), 10); err != nil || len(retained) != 0 {
		t.Fatalf("early retention = %+v, error=%v", retained, err)
	}
	if retained, err := store.ExpiredRetainedBindings(context.Background(), touched.RetentionExpiresAt, 10); err != nil || len(retained) != 1 || retained[0].ID != touched.ID {
		t.Fatalf("retention = %+v, error=%v", retained, err)
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
		ID: "cleanup-1", ResourceType: "binding", ResourceID: "binding-1", BindingID: "binding-1", Backend: "fake",
		BackendRef: "backend-1", WorkspaceRef: "workspace-1", Action: "destroy", Status: "pending", NotBefore: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.EnqueueCleanup(context.Background(), cleanup); err != nil {
		t.Fatalf("enqueue cleanup: %v", err)
	}
	claimed, ok, err := store.ClaimCleanup(context.Background(), "worker-1", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim cleanup: ok=%v error=%v", ok, err)
	}
	if claimed.Attempts != 1 || claimed.LeaseOwner != "worker-1" || claimed.WorkspaceRef != "workspace-1" || claimed.BindingID != "binding-1" {
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

func TestAgentExecutionJobRepository(t *testing.T) {
	dsn := testutil.CreateTestPostgresDSN(t)
	store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	insertActiveJobBinding(t, store, now)
	job := testAgentExecutionJob(now, "job-1", "request-1")
	created, wasCreated, err := store.CreateJob(context.Background(), job)
	if err != nil || !wasCreated {
		t.Fatalf("create job: created=%v error=%v", wasCreated, err)
	}
	if active, err := store.HasActiveJobs(context.Background(), job.BindingID); err != nil || !active {
		t.Fatalf("queued job activity = %v, %v", active, err)
	}
	duplicate := job
	duplicate.ID = "job-other"
	duplicate.RequestHash = "different"
	existing, wasCreated, err := store.CreateJob(context.Background(), duplicate)
	if err != nil || wasCreated || existing.ID != job.ID {
		t.Fatalf("duplicate job: job=%+v created=%v error=%v", existing, wasCreated, err)
	}

	claimed, ok, err := store.ClaimJob(context.Background(), "worker-1", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim job: ok=%v error=%v", ok, err)
	}
	if claimed.Status != agentruntime.JobStatusRunning || claimed.Generation != created.Generation+1 {
		t.Fatalf("claimed job = %+v", claimed)
	}
	first, err := store.AppendJobOutput(context.Background(), job.ID, agentruntime.OutputChunk{Sequence: 0, Stream: agentruntime.OutputStreamStdout, Data: []byte("hello")}, 16, now)
	if err != nil {
		t.Fatalf("append stdout: %v", err)
	}
	second, err := store.AppendJobOutput(context.Background(), job.ID, agentruntime.OutputChunk{Sequence: 1, Stream: agentruntime.OutputStreamStderr, Data: []byte("warn")}, 16, now)
	if err != nil {
		t.Fatalf("append stderr: %v", err)
	}
	if first.Sequence != 0 || second.Sequence != 1 {
		t.Fatalf("output sequences = %d, %d", first.Sequence, second.Sequence)
	}
	duplicateChunk, err := store.AppendJobOutput(context.Background(), job.ID, agentruntime.OutputChunk{Sequence: 0, Stream: agentruntime.OutputStreamStdout, Data: []byte("hello")}, 16, now)
	if err != nil || duplicateChunk.Sequence != 0 {
		t.Fatalf("append duplicate output: chunk=%+v error=%v", duplicateChunk, err)
	}
	if _, err := store.AppendJobOutput(context.Background(), job.ID, agentruntime.OutputChunk{Sequence: 2, Stream: agentruntime.OutputStreamStdout, Data: []byte("too-large")}, 16, now); !errors.Is(err, agentruntime.ErrOutputLimit) {
		t.Fatalf("output limit error = %v, want ErrOutputLimit", err)
	}
	chunks, err := store.JobOutput(context.Background(), job.ID, 0, 10)
	if err != nil || len(chunks) != 1 || chunks[0].Sequence != 1 {
		t.Fatalf("paged output = %+v error=%v", chunks, err)
	}
	cancelRequested, err := store.RequestJobCancellation(context.Background(), job.ID, claimed.Generation, now.Add(time.Second))
	if err != nil || !cancelRequested.CancelRequested {
		t.Fatalf("request cancellation: job=%+v error=%v", cancelRequested, err)
	}
	completed, err := store.CompleteJob(context.Background(), job.ID, "worker-1", agentruntime.JobResult{ExitCode: 0}, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("complete canceled job: %v", err)
	}
	if completed.Status != agentruntime.JobStatusCanceled {
		t.Fatalf("completed status = %q, want canceled", completed.Status)
	}

	job = testAgentExecutionJob(now, "job-2", "request-2")
	if _, _, err := store.CreateJob(context.Background(), job); err != nil {
		t.Fatalf("create recoverable job: %v", err)
	}
	claimed, ok, err = store.ClaimJob(context.Background(), "worker-dead", now, now.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("claim recoverable job: ok=%v error=%v", ok, err)
	}
	afterExpiry := now.Add(2 * time.Minute)
	if err := store.RenewJobClaim(context.Background(), claimed.ID, "worker-dead", afterExpiry, afterExpiry.Add(time.Minute)); !errors.Is(err, agentruntime.ErrJobClaimLost) {
		t.Fatalf("renew expired claim error = %v, want claim lost", err)
	}
	recovered, ok, err := store.ClaimJob(context.Background(), "worker-recovery", afterExpiry, afterExpiry.Add(time.Minute))
	if err != nil || !ok || recovered.Attempts != 2 {
		t.Fatalf("recover job: job=%+v ok=%v error=%v", recovered, ok, err)
	}
	if _, err := store.CompleteJob(context.Background(), recovered.ID, "worker-recovery", agentruntime.JobResult{ExitCode: 0}, afterExpiry.Add(time.Second)); err != nil {
		t.Fatalf("complete recovered job: %v", err)
	}
	if active, err := store.HasActiveJobs(context.Background(), job.BindingID); err != nil || active {
		t.Fatalf("terminal job activity = %v, %v", active, err)
	}
	binding, err := store.Binding(context.Background(), job.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	binding.Status = agentruntime.BindingStatusSuspending
	binding.Generation++
	if err := store.UpdateBinding(context.Background(), binding, binding.Generation-1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateJob(context.Background(), testAgentExecutionJob(now, "job-3", "request-3")); !errors.Is(err, agentruntime.ErrGenerationConflict) {
		t.Fatalf("job admitted after suspension = %v", err)
	}
}

func testAgentExecutionJob(now time.Time, id string, idempotencyKey string) agentruntime.Job {
	return agentruntime.Job{
		ID: id, BindingID: "binding-1", BindingGeneration: 2,
		OrganizationID: "org-1", SubjectID: "user-1",
		Backend: "fake", BackendRef: "backend-1", WorkspaceRef: "workspace-1",
		IdempotencyKey: idempotencyKey, RequestHash: "hash-" + id,
		Status: agentruntime.JobStatusQueued, Generation: 1,
		Command: "python3", Args: []string{"-c", "print('ok')"},
		Environment: map[string]string{"LANG": "C.UTF-8"}, WorkDir: "workspace",
		Timeout: time.Minute, CreatedAt: now, UpdatedAt: now,
	}
}

func insertActiveJobBinding(t *testing.T, store *Store, now time.Time) {
	t.Helper()
	binding := agentruntime.Binding{
		ID: "binding-1", OrganizationID: "org-1", SubjectID: "user-1",
		AgentID: "agent-1", ConversationID: "conversation-1", ProfileID: "profile-1",
		WorkspaceID: "workspace-1", Backend: "fake", IdempotencyKey: "binding-request-1",
		RequestHash: "binding-hash", Status: agentruntime.BindingStatusAllocating,
		Generation: 1, CreatedAt: now, UpdatedAt: now, LastActivityAt: now, ExpiresAt: now.Add(time.Hour), RetentionExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	if _, _, err := store.ReserveBinding(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	binding.Status = agentruntime.BindingStatusActive
	binding.Generation = 2
	if err := store.UpdateBinding(context.Background(), binding, 1); err != nil {
		t.Fatal(err)
	}
}
