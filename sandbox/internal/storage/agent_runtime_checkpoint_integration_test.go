package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

func TestAgentRuntimeCheckpointSurvivesControlRestartAndRetention(t *testing.T) {
	directory := t.TempDir()
	testAgentRuntimeCheckpointLifecycle(t, func() (agentruntime.WorkspaceCheckpointStore, func() error, error) {
		store, err := workspacecheckpoint.NewFileStore(directory)
		if err != nil {
			return nil, nil, err
		}
		return store, store.Close, nil
	})
}

// Set an endpoint and a pre-created private test bucket to run this same
// PostgreSQL lifecycle against two separate S3 client instances.
func TestAgentRuntimeS3CheckpointSurvivesControlRestartAndRetention(t *testing.T) {
	endpoint := os.Getenv("ZGI_CHECKPOINT_S3_TEST_ENDPOINT")
	bucket := os.Getenv("ZGI_CHECKPOINT_S3_TEST_BUCKET")
	if endpoint == "" || bucket == "" {
		t.Skip("set ZGI_CHECKPOINT_S3_TEST_ENDPOINT and ZGI_CHECKPOINT_S3_TEST_BUCKET for S3 lifecycle acceptance")
	}
	prefix := fmt.Sprintf("m5-control-%d", time.Now().UnixNano())
	testAgentRuntimeCheckpointLifecycle(t, func() (agentruntime.WorkspaceCheckpointStore, func() error, error) {
		store, err := workspacecheckpoint.NewS3Store(context.Background(), workspacecheckpoint.S3Config{
			Endpoint: endpoint, Bucket: bucket, Region: "us-east-1", Prefix: prefix, UsePathStyle: true,
		})
		return store, func() error { return nil }, err
	})
}

// The backend deliberately loses compute so resume must restore archived
// files. PostgreSQL and checkpoint storage are independently reopened.
func testAgentRuntimeCheckpointLifecycle(t *testing.T, openCheckpoint func() (agentruntime.WorkspaceCheckpointStore, func() error, error)) {
	t.Helper()
	ctx := context.Background()
	dsn := testutil.CreateTestPostgresDSN(t)
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	backend := &restartCheckpointBackend{files: []byte("csv-result-v1")}
	openControl := func() (*Store, agentruntime.WorkspaceCheckpointStore, func() error, *agentruntime.Service) {
		t.Helper()
		store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 1})
		if err != nil {
			t.Fatal(err)
		}
		checkpoints, closeCheckpoint, err := openCheckpoint()
		if err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		service, err := agentruntime.NewService(store, map[string]agentruntime.Backend{"fake": backend}, agentruntime.ServiceOptions{
			Now: func() time.Time { return now }, Checkpoints: checkpoints, Jobs: store, Snapshots: store,
		})
		if err != nil {
			_ = closeCheckpoint()
			_ = store.Close()
			t.Fatal(err)
		}
		return store, checkpoints, closeCheckpoint, service
	}
	store, checkpoints, closeCheckpoint, service := openControl()
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO agent_runtime_home_snapshots (
			id, organization_id, subject_id, profile_id, backend, backend_ref,
			status, generation, created_at, updated_at
		) VALUES ($1, $2, '', $3, $4, $5, 'approved', 1, $6, $6)
	`, "prepared-v1", "org-1", "standard", "fake", "immutable-template-v1", now)
	if err != nil {
		t.Fatal(err)
	}
	scope := agentruntime.Scope{OrganizationID: "org-1", SubjectID: "user-1"}
	binding, err := service.CreateBinding(ctx, agentruntime.CreateBindingRequest{
		Scope: scope, AgentID: "agent-1", ConversationID: "conversation-1", ProfileID: "standard",
		HomeSnapshotID: "prepared-v1", Backend: "fake", IdempotencyKey: "request-1",
	})
	if err != nil || binding.HomeSnapshotRef != "immutable-template-v1" {
		t.Fatalf("create binding = %+v, %v", binding, err)
	}
	now = now.Add(31 * time.Minute)
	suspended, err := service.SuspendBinding(ctx, scope, binding.ID, binding.Generation)
	if err != nil || suspended.CheckpointKey == "" {
		t.Fatalf("suspend binding = %+v, %v", suspended, err)
	}
	firstCheckpointKey := suspended.CheckpointKey
	if err := closeCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	backend.lost = true
	store, checkpoints, closeCheckpoint, service = openControl()
	defer closeCheckpoint()
	defer store.Close()
	now = now.Add(24 * time.Hour)
	resumed, err := service.ResumeBinding(ctx, scope, binding.ID, suspended.Generation)
	if err != nil || resumed.Status != agentruntime.BindingStatusActive {
		t.Fatalf("resume binding = %+v, %v", resumed, err)
	}
	if !bytes.Equal(backend.restored, []byte("csv-result-v1")) || backend.lastTemplate != "immutable-template-v1" || backend.creates != 2 {
		t.Fatalf("restored=%q template=%q creates=%d", backend.restored, backend.lastTemplate, backend.creates)
	}
	backend.files = []byte("csv-result-v2")
	suspended, err = service.SuspendBinding(ctx, scope, resumed.ID, resumed.Generation)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := checkpoints.Open(ctx, suspended.CheckpointKey)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close()); err != nil || !bytes.Equal(data, backend.files) {
		t.Fatalf("latest checkpoint = %q, %v", data, err)
	}
	reconciler, err := agentruntime.NewReconciler(store, map[string]agentruntime.Backend{"fake": backend}, agentruntime.ReconcilerOptions{
		OwnerID: "checkpoint-reaper", Now: func() time.Time { return now }, Checkpoints: checkpoints,
	})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reconciler.ReconcileOne(ctx); err != nil || !processed {
		t.Fatalf("superseded checkpoint cleanup = %t, %v", processed, err)
	}
	if _, err := checkpoints.Open(ctx, firstCheckpointKey); !errors.Is(err, agentruntime.ErrNotFound) {
		t.Fatalf("superseded checkpoint error = %v", err)
	}
	now = suspended.RetentionExpiresAt.Add(time.Second)
	reaper, err := agentruntime.NewRetentionReaper(store, service, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := reaper.RunOne(ctx); err != nil || !processed {
		t.Fatalf("retention cleanup = %t, %v", processed, err)
	}
	deleted, err := store.Binding(ctx, binding.ID)
	if err != nil || deleted.Status != agentruntime.BindingStatusDeleted {
		t.Fatalf("deleted binding = %+v, %v", deleted, err)
	}
	if _, err := checkpoints.Open(ctx, suspended.CheckpointKey); !errors.Is(err, agentruntime.ErrNotFound) {
		t.Fatalf("expired checkpoint error = %v", err)
	}
}

type restartCheckpointBackend struct {
	files        []byte
	restored     []byte
	lastTemplate string
	creates      int
	lost         bool
}

func (b *restartCheckpointBackend) CreateBinding(_ context.Context, spec agentruntime.CreateSpec) (agentruntime.Allocation, error) {
	b.creates++
	b.lastTemplate = spec.HomeSnapshotRef
	return agentruntime.Allocation{BindingRef: fmt.Sprintf("compute-%d", b.creates), WorkspaceRef: fmt.Sprintf("workspace-%d", b.creates)}, nil
}

func (b *restartCheckpointBackend) SuspendBinding(context.Context, agentruntime.Allocation) error {
	return nil
}

func (b *restartCheckpointBackend) ResumeBinding(context.Context, agentruntime.CreateSpec, agentruntime.Allocation) (agentruntime.Allocation, error) {
	if b.lost {
		return agentruntime.Allocation{}, agentruntime.ErrNotFound
	}
	return agentruntime.Allocation{BindingRef: "resumed", WorkspaceRef: "existing"}, nil
}

func (b *restartCheckpointBackend) DestroyBinding(context.Context, agentruntime.Allocation) error {
	return nil
}

func (b *restartCheckpointBackend) ExportWorkspace(context.Context, agentruntime.Allocation) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.files)), nil
}

func (b *restartCheckpointBackend) ImportWorkspace(_ context.Context, _ agentruntime.Allocation, archive io.Reader) error {
	var err error
	b.restored, err = io.ReadAll(archive)
	return err
}
