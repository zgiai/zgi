package storage

import (
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestPostgresStorePersistsSandboxAndEvents(t *testing.T) {
	cfg := config.FromEnv()
	cfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)

	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer store.Close()

	box := sandbox.Sandbox{
		ID:                "sbx_store_test",
		RuntimeProfile:    sandbox.RuntimeSession,
		Status:            sandbox.StatusActive,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(5 * time.Minute),
		RootPath:          "/tmp/sbx_store_test",
		Metadata:          map[string]string{"tenant_id": "tenant-1"},
		NetworkEnabled:    true,
		NetworkPolicy:     "workflow-safe",
		DependencyProfile: "stdlib",
		WorkspaceBinding:  "wf_1",
		TTLSeconds:        300,
		WorkerID:          "worker-a",
		WorkerAddr:        "http://127.0.0.1:2660",
	}
	if err := store.SaveSandbox(box); err != nil {
		t.Fatalf("save sandbox: %v", err)
	}

	loaded, err := store.GetSandbox(box.ID)
	if err != nil {
		t.Fatalf("get sandbox: %v", err)
	}
	if loaded.WorkerID != box.WorkerID {
		t.Fatalf("expected worker id %q, got %q", box.WorkerID, loaded.WorkerID)
	}

	event := observer.Event{
		ID:        "evt_store_test",
		SandboxID: box.ID,
		Type:      "sandbox.created",
		Message:   "created",
		CreatedAt: time.Now().UTC(),
		Metadata:  map[string]any{"worker_id": "worker-a"},
	}
	if err := store.AppendEvent(event); err != nil {
		t.Fatalf("append event: %v", err)
	}

	events, err := store.QueryEvents(observer.Query{SandboxID: box.ID, Limit: 10})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Type != "sandbox.created" {
		t.Fatalf("unexpected event type: %s", events[0].Type)
	}
}
