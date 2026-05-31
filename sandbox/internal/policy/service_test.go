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
	cfg.MaxConcurrentExecutions = 3
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
	if decision.EffectiveLimits.MaxConcurrentExecutionsPerProfile != cfg.MaxConcurrentExecutionsPerProfile {
		t.Fatalf("expected profile concurrent execution limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxExecutionsPerMinutePerOrganization != cfg.MaxExecutionsPerMinutePerOrganization {
		t.Fatalf("expected organization execution rate limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxConcurrentExecutionsPerOrganization != cfg.MaxConcurrentExecutionsPerOrganization {
		t.Fatalf("expected organization concurrent execution limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxConcurrentExecutions != cfg.MaxConcurrentExecutions {
		t.Fatalf("expected service concurrent execution limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxQueuedExecutionsPerOrganization != cfg.MaxQueuedExecutionsPerOrganization {
		t.Fatalf("expected organization queued execution limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxWorkspaceBytes != cfg.MaxWorkspaceBytes {
		t.Fatalf("expected workspace byte limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxWorkspaceFiles != cfg.MaxWorkspaceFiles {
		t.Fatalf("expected workspace file limit in decision, got %+v", decision.EffectiveLimits)
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

func TestNormalizeTemplateLimits(t *testing.T) {
	service := NewService(config.FromEnv())

	limits, err := service.NormalizeTemplateLimits("", "", 500, 1)
	if err != nil {
		t.Fatalf("expected default template limits, got %v", err)
	}
	if limits.Profile != "template-short" || limits.Engine != "go-text" {
		t.Fatalf("unexpected template profile: %+v", limits)
	}
	if limits.TimeoutMS != 500 || limits.OutputLimitBytes != 1024 {
		t.Fatalf("expected request to tighten timeout and output limits, got %+v", limits)
	}

	limits, err = service.NormalizeTemplateLimits("template-short", "go-text", 60000, 1024)
	if err != nil {
		t.Fatalf("expected raised template limits to be capped, got %v", err)
	}
	if limits.TimeoutMS != 2000 || limits.OutputLimitBytes != 64*1024 {
		t.Fatalf("expected template limits to keep policy caps, got %+v", limits)
	}

	if _, err := service.NormalizeTemplateLimits("unknown", "", 0, 0); err == nil {
		t.Fatal("expected unknown template profile to be rejected")
	}
	if _, err := service.NormalizeTemplateLimits("template-short", "unknown", 0, 0); err == nil {
		t.Fatal("expected unknown template engine to be rejected")
	}
	if _, err := service.NormalizeTemplateLimits("template-short", "go-text", -1, 0); err == nil {
		t.Fatal("expected negative template timeout to be rejected")
	}
	if _, err := service.NormalizeTemplateLimits("template-short", "go-text", 0, -1); err == nil {
		t.Fatal("expected negative template output limit to be rejected")
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
