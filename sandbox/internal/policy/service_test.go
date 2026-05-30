package policy

import (
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

func TestNormalizeCreateClampsTTLAndRejectsDeniedNetwork(t *testing.T) {
	cfg := config.FromEnv()
	cfg.SessionTTL = 120
	cfg.RuntimeBackend = "linux-secure"
	service := NewService(cfg)

	decision, err := service.NormalizeCreate("session", 999, false, "", "stdlib", 0, "", 0)
	if err != nil {
		t.Fatalf("expected normalize create, got %v", err)
	}
	if decision.TTL.Seconds() != 120 {
		t.Fatalf("expected ttl clamp to 120 seconds, got %.0f", decision.TTL.Seconds())
	}

	if _, err := service.NormalizeCreate("session", 60, true, "deny-by-default", "stdlib", 0, "", 0); err == nil {
		t.Fatal("expected denied network policy to reject outbound access")
	}
}

func TestNormalizeCreateRejectsNetworkWhenBackendCannotEnforcePolicy(t *testing.T) {
	cfg := config.FromEnv()
	cfg.RuntimeBackend = "preview"
	service := NewService(cfg)

	if _, err := service.NormalizeCreate("session", 60, true, "workflow-safe", "stdlib", 0, "", 0); err == nil {
		t.Fatal("expected preview backend to reject network-enabled sandbox")
	}
}

func TestNetworkPolicySurfaceReportsBackendEnforcement(t *testing.T) {
	previewCfg := config.FromEnv()
	previewCfg.RuntimeBackend = "preview"
	preview := NewService(previewCfg)
	if preview.RuntimeBackend() != "preview-process" {
		t.Fatalf("expected normalized preview backend, got %s", preview.RuntimeBackend())
	}
	if preview.NetworkPolicyEnforced() {
		t.Fatal("expected preview backend to report network policy as not runtime-enforced")
	}
	previewSnapshot := preview.Snapshot()
	previewLimits := previewSnapshot["limits"].(sandbox.ResourceLimits)
	if previewLimits.NetworkPolicyEnforced {
		t.Fatalf("expected preview snapshot to report network_policy_enforced=false, got %#v", previewLimits.NetworkPolicyEnforced)
	}

	secureCfg := config.FromEnv()
	secureCfg.RuntimeBackend = "linux-secure"
	secure := NewService(secureCfg)
	if !secure.NetworkPolicyEnforced() {
		t.Fatal("expected linux-secure backend to report runtime network enforcement")
	}
	secureSnapshot := secure.Snapshot()
	secureLimits := secureSnapshot["limits"].(sandbox.ResourceLimits)
	if !secureLimits.NetworkPolicyEnforced {
		t.Fatalf("expected secure snapshot to report network_policy_enforced=true, got %#v", secureLimits.NetworkPolicyEnforced)
	}
}

func TestNormalizeCreateReturnsEffectiveLimitsAndStructuredLimitError(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 2
	cfg.MaxFileSizeKB = 128
	service := NewService(cfg)

	decision, err := service.NormalizeCreate("session", 60, false, "", "stdlib", 1, "organization-1", 1)
	if err != nil {
		t.Fatalf("expected normalize create, got %v", err)
	}
	if decision.EffectiveLimits.MaxActiveSandboxes != 2 {
		t.Fatalf("expected max active limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxActiveSandboxesPerOrganization != 0 {
		t.Fatalf("expected organization active limit to default to disabled, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxFileSizeBytes != 128*1024 {
		t.Fatalf("expected max file size bytes in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxArtifactManifestFiles != 100 || decision.EffectiveLimits.MaxArtifactManifestTotalBytes != 128*1024*256 {
		t.Fatalf("expected artifact manifest limits in decision, got %+v", decision.EffectiveLimits)
	}

	_, err = service.NormalizeCreate("session", 60, false, "", "stdlib", 2, "organization-1", 1)
	limitErr, ok := err.(*LimitError)
	if !ok {
		t.Fatalf("expected LimitError, got %T %v", err, err)
	}
	if limitErr.Code != "active_sandbox_limit_exceeded" || limitErr.Limit != "max_active_sandboxes" {
		t.Fatalf("unexpected limit error: %+v", limitErr)
	}
}

func TestNormalizeCreateRejectsOrganizationActiveLimit(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 10
	cfg.MaxActivePerOrganization = 2
	service := NewService(cfg)

	decision, err := service.NormalizeCreate("session", 60, false, "", "stdlib", 2, "organization-1", 1)
	if err != nil {
		t.Fatalf("expected organization create below limit, got %v", err)
	}
	if decision.EffectiveLimits.MaxActiveSandboxesPerOrganization != 2 {
		t.Fatalf("expected organization limit in decision, got %+v", decision.EffectiveLimits)
	}

	_, err = service.NormalizeCreate("session", 60, false, "", "stdlib", 2, "organization-1", 2)
	limitErr, ok := err.(*LimitError)
	if !ok {
		t.Fatalf("expected LimitError, got %T %v", err, err)
	}
	if limitErr.Code != "organization_active_sandbox_limit_exceeded" || limitErr.Limit != "max_active_sandboxes_per_organization" {
		t.Fatalf("unexpected organization limit error: %+v", limitErr)
	}
	if limitErr.Details["organization_id"] != "organization-1" {
		t.Fatalf("expected organization id in details, got %+v", limitErr.Details)
	}

	if _, err := service.NormalizeCreate("session", 60, false, "", "stdlib", 2, "", 2); err != nil {
		t.Fatalf("expected empty organization to bypass organization quota, got %v", err)
	}
}

func TestNormalizeCommandLimitsUsesProfileAndClampsRequest(t *testing.T) {
	cfg := config.FromEnv()
	cfg.CommandTimeout = 10
	cfg.OutputLimitKB = 512
	service := NewService(cfg)

	limits, err := service.NormalizeCommandLimits("skill-python", 60, 0, 2048, 2048)
	if err != nil {
		t.Fatalf("expected command limits, got %v", err)
	}
	if limits.Profile != "skill-python" {
		t.Fatalf("unexpected profile: %s", limits.Profile)
	}
	if limits.Timeout != 10*time.Second {
		t.Fatalf("expected timeout clamp to 10s, got %s", limits.Timeout)
	}
	if limits.StdoutLimitBytes != 512*1024 || limits.StderrLimitBytes != 512*1024 {
		t.Fatalf("expected output limits to clamp to config cap, got stdout=%d stderr=%d", limits.StdoutLimitBytes, limits.StderrLimitBytes)
	}

	if _, err := service.NormalizeCommandLimits("unknown", 0, 0, 0, 0); err == nil {
		t.Fatal("expected unknown command profile to be rejected")
	}
}

func TestValidateCodeExecutionRejectsUnauthorizedNetwork(t *testing.T) {
	service := NewService(config.FromEnv())
	box := sandbox.Sandbox{
		NetworkEnabled: false,
		NetworkPolicy:  "deny-by-default",
	}

	if err := service.ValidateCodeExecution(box, true); err == nil {
		t.Fatal("expected network validation failure")
	}
}
