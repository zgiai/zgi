package lifecycle

import (
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

func TestCreateRenewDeleteSandbox(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 1
	policyService := policy.NewService(cfg)
	recorder := observer.NewRecorder(100)
	manager, err := NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected manager, got %v", err)
	}

	box, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		TTLSeconds:     60,
		OrganizationID: " organization-test ",
		WorkspaceID:    "workspace-test",
		AppID:          "app-test",
		WorkflowRunID:  "run-test",
		UserID:         "user-test",
	})
	if err != nil {
		t.Fatalf("expected sandbox creation, got %v", err)
	}

	if box.RuntimeProfile != sandbox.RuntimeSession {
		t.Fatalf("unexpected runtime profile: %s", box.RuntimeProfile)
	}
	if box.EffectiveLimits == nil {
		t.Fatal("expected effective limits on created sandbox")
	}
	if box.EffectiveLimits.MaxActiveSandboxes != 1 {
		t.Fatalf("expected max active limit on sandbox, got %+v", box.EffectiveLimits)
	}
	if box.OrganizationID != "organization-test" || box.WorkspaceID != "workspace-test" || box.AppID != "app-test" || box.WorkflowRunID != "run-test" || box.UserID != "user-test" {
		t.Fatalf("expected normalized ownership fields, got %+v", box)
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "sandbox.created", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected sandbox.created observer event, got %d", len(events))
	}
	if _, ok := events[0].Metadata["limit_decisions"]; !ok {
		t.Fatalf("expected limit decisions in observer metadata, got %+v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-test" || events[0].Metadata["workspace_id"] != "workspace-test" || events[0].Metadata["workflow_run_id"] != "run-test" {
		t.Fatalf("expected ownership metadata in created event, got %+v", events[0].Metadata)
	}

	items := manager.List()
	if len(items) == 0 {
		t.Fatal("expected listed sandboxes after creation")
	}
	if items[0].EffectiveLimits == nil {
		t.Fatal("expected effective limits on listed sandbox")
	}

	renewed, err := manager.Renew(box.ID, 120)
	if err != nil {
		t.Fatalf("expected renew, got %v", err)
	}
	if renewed.TTLSeconds != 120 {
		t.Fatalf("expected ttl to be updated, got %d", renewed.TTLSeconds)
	}
	renewEvents := recorder.Query(observer.Query{SandboxID: box.ID, Type: "sandbox.renewed", Limit: 1})
	if len(renewEvents) != 1 || renewEvents[0].Metadata["organization_id"] != "organization-test" {
		t.Fatalf("expected ownership metadata in renewed event, got %+v", renewEvents)
	}

	if _, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	}); err == nil {
		t.Fatal("expected create to fail after reaching max active sandboxes")
	}

	if err := manager.Delete(box.ID); err != nil {
		t.Fatalf("expected delete, got %v", err)
	}
	deleteEvents := recorder.Query(observer.Query{SandboxID: box.ID, Type: "sandbox.deleted", Limit: 1})
	if len(deleteEvents) != 1 || deleteEvents[0].Metadata["organization_id"] != "organization-test" {
		t.Fatalf("expected ownership metadata in deleted event, got %+v", deleteEvents)
	}
}

func TestCreateRejectsInvalidOwnershipFields(t *testing.T) {
	manager, err := NewManager(observer.NewRecorder(100), policy.NewService(config.FromEnv()))
	if err != nil {
		t.Fatalf("expected manager, got %v", err)
	}

	if _, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization ok",
	}); err == nil {
		t.Fatal("expected invalid organization ID to be rejected")
	}
	if _, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		WorkspaceID:    string(make([]byte, 129)),
	}); err == nil {
		t.Fatal("expected oversized workspace ID to be rejected")
	}
}

func TestActiveSandboxLimitIsScopedToWorker(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 1
	cfg.DataDir = t.TempDir()
	store := newMemoryStore()
	cache := newNoopCache()
	recorder := observer.NewRecorder(100)

	workerOneCfg := cfg
	workerOneCfg.WorkerID = "worker-one"
	workerOnePolicy := policy.NewService(workerOneCfg)
	workerOne, err := NewManagerWithConfig(recorder, workerOnePolicy, workerOneCfg, store, cache)
	if err != nil {
		t.Fatalf("expected worker one manager, got %v", err)
	}

	workerTwoCfg := cfg
	workerTwoCfg.WorkerID = "worker-two"
	workerTwoPolicy := policy.NewService(workerTwoCfg)
	workerTwo, err := NewManagerWithConfig(recorder, workerTwoPolicy, workerTwoCfg, store, cache)
	if err != nil {
		t.Fatalf("expected worker two manager, got %v", err)
	}

	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)}); err != nil {
		t.Fatalf("expected worker one sandbox, got %v", err)
	}
	if _, err := workerTwo.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)}); err != nil {
		t.Fatalf("expected worker two sandbox despite worker one limit, got %v", err)
	}
	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)}); err == nil {
		t.Fatal("expected worker one to hit its own active sandbox limit")
	}
}

func TestOrganizationActiveSandboxLimitSpansWorkers(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 10
	cfg.MaxActivePerOrganization = 2
	cfg.DataDir = t.TempDir()
	store := newMemoryStore()
	cache := newNoopCache()
	recorder := observer.NewRecorder(100)

	workerOneCfg := cfg
	workerOneCfg.WorkerID = "worker-one"
	workerOne, err := NewManagerWithConfig(recorder, policy.NewService(workerOneCfg), workerOneCfg, store, cache)
	if err != nil {
		t.Fatalf("expected worker one manager, got %v", err)
	}

	workerTwoCfg := cfg
	workerTwoCfg.WorkerID = "worker-two"
	workerTwo, err := NewManagerWithConfig(recorder, policy.NewService(workerTwoCfg), workerTwoCfg, store, cache)
	if err != nil {
		t.Fatalf("expected worker two manager, got %v", err)
	}

	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession), OrganizationID: "organization-one"}); err != nil {
		t.Fatalf("expected first organization sandbox, got %v", err)
	}
	if _, err := workerTwo.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession), OrganizationID: "organization-one"}); err != nil {
		t.Fatalf("expected second organization sandbox across worker, got %v", err)
	}
	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession), OrganizationID: "organization-one"}); err == nil {
		t.Fatal("expected organization active sandbox limit")
	}
	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession), OrganizationID: "organization-two"}); err != nil {
		t.Fatalf("expected different organization to have its own quota, got %v", err)
	}
	if _, err := workerOne.Create(CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)}); err != nil {
		t.Fatalf("expected empty organization to bypass organization quota, got %v", err)
	}
}
