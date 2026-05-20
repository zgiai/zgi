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
	manager, err := NewManager(observer.NewRecorder(100), policyService)
	if err != nil {
		t.Fatalf("expected manager, got %v", err)
	}

	box, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		TTLSeconds:     60,
	})
	if err != nil {
		t.Fatalf("expected sandbox creation, got %v", err)
	}

	if box.RuntimeProfile != sandbox.RuntimeSession {
		t.Fatalf("unexpected runtime profile: %s", box.RuntimeProfile)
	}

	items := manager.List()
	if len(items) == 0 {
		t.Fatal("expected listed sandboxes after creation")
	}

	renewed, err := manager.Renew(box.ID, 120)
	if err != nil {
		t.Fatalf("expected renew, got %v", err)
	}
	if renewed.TTLSeconds != 120 {
		t.Fatalf("expected ttl to be updated, got %d", renewed.TTLSeconds)
	}

	if _, err := manager.Create(CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	}); err == nil {
		t.Fatal("expected create to fail after reaching max active sandboxes")
	}

	if err := manager.Delete(box.ID); err != nil {
		t.Fatalf("expected delete, got %v", err)
	}
}
