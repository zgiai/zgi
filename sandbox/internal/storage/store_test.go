package storage

import (
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
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
		ID:                         "sbx_store_test",
		RuntimeProfile:             sandbox.RuntimeSession,
		Status:                     sandbox.StatusActive,
		CreatedAt:                  time.Now().UTC(),
		UpdatedAt:                  time.Now().UTC(),
		ExpiresAt:                  time.Now().UTC().Add(5 * time.Minute),
		RootPath:                   "/tmp/sbx_store_test",
		Metadata:                   map[string]string{"organization_id": "organization-1", "dependency_profile_version": "2026.05.01"},
		OrganizationID:             "organization-1",
		WorkspaceID:                "workspace-1",
		AppID:                      "app-1",
		WorkflowRunID:              "run-1",
		UserID:                     "user-1",
		NetworkEnabled:             true,
		NetworkPolicy:              "workflow-safe",
		DependencyProfile:          "stdlib",
		DependencyProfileVersion:   "2026.05.01",
		DependencyArtifactChecksum: "sha256:stdlib-artifact",
		WorkspaceBinding:           "wf_1",
		TTLSeconds:                 300,
		WorkerID:                   "worker-a",
		WorkerAddr:                 "http://127.0.0.1:2660",
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
	if loaded.DependencyProfileVersion != box.DependencyProfileVersion {
		t.Fatalf("expected dependency profile version to round trip, got %+v", loaded)
	}
	if loaded.DependencyArtifactChecksum != box.DependencyArtifactChecksum {
		t.Fatalf("expected dependency artifact checksum to round trip, got %+v", loaded)
	}
	organizationCount, err := store.CountActiveByOrganization(box.OrganizationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("count organization active sandboxes: %v", err)
	}
	if organizationCount != 1 {
		t.Fatalf("expected one active sandbox for organization, got %d", organizationCount)
	}
	workflowBox := box
	workflowBox.ID = "sbx_store_test_workflow_profile"
	workflowBox.DependencyProfile = "workflow-safe"
	if err := store.SaveSandbox(workflowBox); err != nil {
		t.Fatalf("save workflow profile sandbox: %v", err)
	}
	expiredBox := box
	expiredBox.ID = "sbx_store_test_expired_profile"
	expiredBox.DependencyProfile = "node-basic"
	expiredBox.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	if err := store.SaveSandbox(expiredBox); err != nil {
		t.Fatalf("save expired profile sandbox: %v", err)
	}
	otherOrganizationBox := box
	otherOrganizationBox.ID = "sbx_store_test_other_organization_profile"
	otherOrganizationBox.OrganizationID = "organization-2"
	otherOrganizationBox.DependencyProfile = "node-basic"
	if err := store.SaveSandbox(otherOrganizationBox); err != nil {
		t.Fatalf("save other organization profile sandbox: %v", err)
	}
	profiles, err := store.ListActiveDependencyProfilesByOrganization(box.OrganizationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("list active dependency profiles by organization: %v", err)
	}
	if len(profiles) != 2 || profiles[0] != "stdlib" || profiles[1] != "workflow-safe" {
		t.Fatalf("expected active dependency profiles for organization, got %+v", profiles)
	}

	dependencyProfile := policy.DependencyProfile{
		Name:        "office-safe",
		Version:     "2026.05.31",
		Status:      "ready",
		Enabled:     true,
		OwnerScope:  "global",
		Languages:   []string{"python3"},
		Packages:    []policy.DependencyPackage{{Name: "data-tools", Version: "managed", Ecosystem: "python3"}},
		BaseRuntime: "preview-process",
		Checksum:    "sha256:office-safe",
		SizeBytes:   1024,
		Description: "Managed document automation profile.",
	}
	if err := store.SaveDependencyProfile(dependencyProfile); err != nil {
		t.Fatalf("save dependency profile: %v", err)
	}
	dependencyProfiles, err := store.ListDependencyProfiles()
	if err != nil {
		t.Fatalf("list dependency profiles: %v", err)
	}
	if len(dependencyProfiles) != 1 {
		t.Fatalf("expected one dependency profile, got %+v", dependencyProfiles)
	}
	loadedProfile := dependencyProfiles[0]
	if loadedProfile.Name != dependencyProfile.Name ||
		loadedProfile.Version != dependencyProfile.Version ||
		loadedProfile.Status != dependencyProfile.Status ||
		!loadedProfile.Enabled ||
		loadedProfile.OwnerScope != dependencyProfile.OwnerScope ||
		loadedProfile.BaseRuntime != dependencyProfile.BaseRuntime ||
		loadedProfile.Checksum != dependencyProfile.Checksum ||
		loadedProfile.SizeBytes != dependencyProfile.SizeBytes ||
		loadedProfile.Description != dependencyProfile.Description {
		t.Fatalf("dependency profile fields did not round trip: %+v", loadedProfile)
	}
	if len(loadedProfile.Languages) != 1 || loadedProfile.Languages[0] != "python3" {
		t.Fatalf("dependency profile languages did not round trip: %+v", loadedProfile.Languages)
	}
	if len(loadedProfile.Packages) != 1 || loadedProfile.Packages[0].Name != "data-tools" || loadedProfile.Packages[0].Ecosystem != "python3" {
		t.Fatalf("dependency profile packages did not round trip: %+v", loadedProfile.Packages)
	}
	organizationProfile := dependencyProfile
	organizationProfile.Name = "team-data"
	organizationProfile.Scope = "organization"
	organizationProfile.OwnerScope = "organization"
	organizationProfile.OrganizationID = "organization-1"
	organizationProfile.Checksum = "sha256:team-data"
	organizationProfile.ArtifactChecksum = "sha256:office-safe"
	organizationProfile.PublicReusable = false
	if err := store.SaveDependencyProfile(organizationProfile); err != nil {
		t.Fatalf("save organization dependency profile: %v", err)
	}
	dependencyProfiles, err = store.ListDependencyProfiles()
	if err != nil {
		t.Fatalf("list dependency profiles with organization profile: %v", err)
	}
	byName := map[string]policy.DependencyProfile{}
	for _, item := range dependencyProfiles {
		byName[item.Name] = item
	}
	if byName["team-data"].Scope != "organization" || byName["team-data"].OrganizationID != "organization-1" || byName["team-data"].ArtifactChecksum != "sha256:office-safe" {
		t.Fatalf("organization dependency profile did not round trip: %+v", byName["team-data"])
	}

	buildRecord, err := store.UpsertDependencyBuildRequest(DependencyBuildRequestRecord{
		BuildID:               "depbuild_1234",
		Fingerprint:           "sha256:1234",
		Status:                "queued",
		OrganizationID:        "organization-1",
		ProfileName:           "auto-1234",
		DependencyRequestJSON: []byte(`{"schema_version":1,"language":"python3","base_runtime":"linux-secure"}`),
		PackagesJSON:          []byte(`[{"ecosystem":"python3","name":"pandas","version":"==2.2.3"}]`),
		SourcesJSON:           []byte(`["requirements.txt"]`),
		WarningsJSON:          []byte(`[]`),
		PackageCount:          1,
	})
	if err != nil {
		t.Fatalf("upsert dependency build request: %v", err)
	}
	if buildRecord.Status != "queued" || buildRecord.ProfileName != "auto-1234" || buildRecord.PackageCount != 1 {
		t.Fatalf("dependency build request did not round trip: %+v", buildRecord)
	}
	loadedBuildRecord, err := store.GetDependencyBuildRequest("sha256:1234")
	if err != nil {
		t.Fatalf("get dependency build request: %v", err)
	}
	if loadedBuildRecord.BuildID != buildRecord.BuildID || string(loadedBuildRecord.PackagesJSON) == "" {
		t.Fatalf("loaded dependency build request did not match: %+v", loadedBuildRecord)
	}
	claimedBuildRecord, err := store.ClaimNextDependencyBuildRequest()
	if err != nil {
		t.Fatalf("claim dependency build request: %v", err)
	}
	if claimedBuildRecord == nil || claimedBuildRecord.Fingerprint != "sha256:1234" || claimedBuildRecord.Status != "building" {
		t.Fatalf("dependency build request claim did not mark building: %+v", claimedBuildRecord)
	}
	emptyBuildRecord, err := store.ClaimNextDependencyBuildRequest()
	if err != nil {
		t.Fatalf("claim empty dependency build request: %v", err)
	}
	if emptyBuildRecord != nil {
		t.Fatalf("expected no queued dependency build request, got %+v", emptyBuildRecord)
	}
	readyBuildRecord, err := store.UpdateDependencyBuildRequestStatus("sha256:1234", "ready", "sha256:artifact", 2048, "")
	if err != nil {
		t.Fatalf("mark dependency build request ready: %v", err)
	}
	if readyBuildRecord.Status != "ready" || readyBuildRecord.ArtifactChecksum != "sha256:artifact" || readyBuildRecord.SizeBytes != 2048 {
		t.Fatalf("dependency build request status did not update: %+v", readyBuildRecord)
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

	recentEvents, err := store.QueryEvents(observer.Query{SandboxID: box.ID, After: event.CreatedAt, Limit: 10})
	if err != nil {
		t.Fatalf("query recent events: %v", err)
	}
	if len(recentEvents) != 1 {
		t.Fatalf("expected one recent event, got %d", len(recentEvents))
	}
	if recentEvents[0].Message != "code executed" {
		t.Fatalf("expected recent execution event, got %q", recentEvents[0].Message)
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
