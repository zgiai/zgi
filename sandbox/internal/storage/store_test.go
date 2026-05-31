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
		Metadata:          map[string]string{"organization_id": "organization-1"},
		OrganizationID:    "organization-1",
		WorkspaceID:       "workspace-1",
		AppID:             "app-1",
		WorkflowRunID:     "run-1",
		UserID:            "user-1",
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
	if loaded.OrganizationID != box.OrganizationID || loaded.WorkspaceID != box.WorkspaceID || loaded.AppID != box.AppID || loaded.WorkflowRunID != box.WorkflowRunID || loaded.UserID != box.UserID {
		t.Fatalf("expected ownership fields to round trip, got %+v", loaded)
	}
	organizationCount, err := store.CountActiveByOrganization(box.OrganizationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("count organization active sandboxes: %v", err)
	}
	if organizationCount != 1 {
		t.Fatalf("expected one active sandbox for organization, got %d", organizationCount)
	}

	event := observer.Event{
		ID:        "evt_store_test",
		SandboxID: box.ID,
		Type:      "sandbox.created",
		Message:   "created",
		CreatedAt: time.Now().UTC().Add(2 * time.Second),
		Metadata: map[string]any{
			"worker_id":       "worker-a",
			"organization_id": "organization-1",
			"workspace_id":    "workspace-1",
			"app_id":          "app-1",
			"workflow_run_id": "run-1",
			"user_id":         "user-1",
			"request_id":      "req-store-match",
		},
	}
	if err := store.AppendEvent(event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	otherEvent := observer.Event{
		ID:        "evt_store_test_other_scope",
		SandboxID: box.ID,
		Type:      "sandbox.created",
		Message:   "other scope",
		CreatedAt: event.CreatedAt.Add(-500 * time.Millisecond),
		Metadata: map[string]any{
			"worker_id":       "worker-a",
			"organization_id": "organization-2",
			"workspace_id":    "workspace-2",
			"app_id":          "app-2",
			"workflow_run_id": "run-2",
			"user_id":         "user-2",
			"request_id":      "req-store-miss",
		},
	}
	if err := store.AppendEvent(otherEvent); err != nil {
		t.Fatalf("append other scoped event: %v", err)
	}
	olderEvent := observer.Event{
		ID:        "evt_store_test_older",
		SandboxID: box.ID,
		Type:      "sandbox.created",
		Message:   "older",
		CreatedAt: event.CreatedAt.Add(-time.Second),
		Metadata:  map[string]any{"worker_id": "worker-a"},
	}
	if err := store.AppendEvent(olderEvent); err != nil {
		t.Fatalf("append older event: %v", err)
	}
	execEvent := observer.Event{
		ID:        "evt_store_test_exec",
		SandboxID: box.ID,
		Type:      "exec.code",
		Message:   "code executed",
		CreatedAt: event.CreatedAt.Add(500 * time.Millisecond),
		Metadata:  map[string]any{"worker_id": "worker-a"},
	}
	if err := store.AppendEvent(execEvent); err != nil {
		t.Fatalf("append execution event: %v", err)
	}

	events, err := store.QueryEvents(observer.Query{SandboxID: box.ID, Limit: 1})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Type != "exec.code" {
		t.Fatalf("unexpected event type: %s", events[0].Type)
	}
	if events[0].Message != "code executed" {
		t.Fatalf("expected newest event first, got %q", events[0].Message)
	}

	execEvents, err := store.QueryEvents(observer.Query{SandboxID: box.ID, TypePrefix: "exec.", Limit: 10})
	if err != nil {
		t.Fatalf("query execution events: %v", err)
	}
	if len(execEvents) != 1 {
		t.Fatalf("expected one execution event, got %d", len(execEvents))
	}
	if execEvents[0].Message != "code executed" {
		t.Fatalf("expected execution event, got %q", execEvents[0].Message)
	}

	scopedEvents, err := store.QueryEvents(observer.Query{
		SandboxID:      box.ID,
		OrganizationID: "organization-1",
		WorkspaceID:    "workspace-1",
		AppID:          "app-1",
		WorkflowRunID:  "run-1",
		UserID:         "user-1",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("query scoped events: %v", err)
	}
	if len(scopedEvents) != 1 {
		t.Fatalf("expected one scoped event, got %d", len(scopedEvents))
	}
	if scopedEvents[0].Message != "created" {
		t.Fatalf("expected scoped event, got %q", scopedEvents[0].Message)
	}

	requestEvents, err := store.QueryEvents(observer.Query{
		SandboxID: box.ID,
		RequestID: "req-store-match",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("query request events: %v", err)
	}
	if len(requestEvents) != 1 {
		t.Fatalf("expected one request event, got %d", len(requestEvents))
	}
	if requestEvents[0].Message != "created" {
		t.Fatalf("expected request event, got %q", requestEvents[0].Message)
	}

	olderEvents, err := store.QueryEvents(observer.Query{SandboxID: box.ID, Before: otherEvent.CreatedAt, Limit: 10})
	if err != nil {
		t.Fatalf("query older events: %v", err)
	}
	if len(olderEvents) != 1 {
		t.Fatalf("expected one older event, got %d", len(olderEvents))
	}
	if olderEvents[0].Message != "older" {
		t.Fatalf("expected older event, got %q", olderEvents[0].Message)
	}
}

func TestPostgresStorePrunesObserverEventsByAgeAndCount(t *testing.T) {
	cfg := config.FromEnv()
	cfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)
	cfg.ObserverRetentionDays = 1
	cfg.ObserverMaxEvents = 2

	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	events := []observer.Event{
		{
			ID:        "evt_retention_old",
			SandboxID: "sbx_retention",
			Type:      "sandbox.test",
			Message:   "old",
			CreatedAt: now.Add(-48 * time.Hour),
			Metadata:  map[string]any{},
		},
		{
			ID:        "evt_retention_first",
			SandboxID: "sbx_retention",
			Type:      "sandbox.test",
			Message:   "first",
			CreatedAt: now.Add(time.Second),
			Metadata:  map[string]any{},
		},
		{
			ID:        "evt_retention_second",
			SandboxID: "sbx_retention",
			Type:      "sandbox.test",
			Message:   "second",
			CreatedAt: now.Add(2 * time.Second),
			Metadata:  map[string]any{},
		},
		{
			ID:        "evt_retention_third",
			SandboxID: "sbx_retention",
			Type:      "sandbox.test",
			Message:   "third",
			CreatedAt: now.Add(3 * time.Second),
			Metadata:  map[string]any{},
		},
	}

	for _, event := range events {
		if err := store.AppendEvent(event); err != nil {
			t.Fatalf("append event %s: %v", event.ID, err)
		}
	}

	kept, err := store.QueryEvents(observer.Query{SandboxID: "sbx_retention", Limit: 10})
	if err != nil {
		t.Fatalf("query retained events: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("expected two retained events, got %d", len(kept))
	}
	if kept[0].Message != "third" || kept[1].Message != "second" {
		t.Fatalf("expected newest two events to be retained, got %#v", kept)
	}
}
