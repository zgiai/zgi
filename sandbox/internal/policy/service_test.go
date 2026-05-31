package policy

import (
	"strings"
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

func TestNormalizeCreateRecordsDependencyProfileVersionAndRejectsUnavailableProfiles(t *testing.T) {
	service := NewService(config.FromEnv())

	decision, err := service.NormalizeCreate("session", 60, false, "", "workflow-safe", 0, "", 0)
	if err != nil {
		t.Fatalf("expected dependency profile selection, got %v", err)
	}
	if decision.DependencyProfile != "workflow-safe" {
		t.Fatalf("expected workflow-safe dependency profile, got %s", decision.DependencyProfile)
	}
	if decision.DependencyProfileVersion == "" {
		t.Fatal("expected dependency profile version in create decision")
	}

	if _, err := service.NormalizeCreate("session", 60, false, "", "missing-profile", 0, "", 0); err == nil {
		t.Fatal("expected unknown dependency profile to be rejected")
	}
	if _, err := service.NormalizeCreate("session", 60, false, "", "python-data-preview", 0, "", 0); err == nil {
		t.Fatal("expected disabled dependency profile to be rejected")
	}
	if _, err := service.NormalizeCreate("session", 60, false, "", "python-data-preview", 999, "organization-1", 999); err == nil {
		t.Fatal("expected disabled dependency profile to be rejected before quota checks")
	} else if _, ok := err.(*LimitError); ok {
		t.Fatalf("expected disabled dependency profile error before quota checks, got %v", err)
	}
}

func TestDependencyPackagePolicyRejectsUnlistedAndUnpinnedPackages(t *testing.T) {
	service := NewService(config.FromEnv())

	if err := service.ValidateDependencyProfilePackages(DependencyProfile{
		Name:      "allowed-profile",
		Languages: []string{"python3"},
		Packages:  []DependencyPackage{{Name: "data-tools", Version: "managed"}},
	}); err != nil {
		t.Fatalf("expected allowlisted package, got %v", err)
	}

	for name, profile := range map[string]DependencyProfile{
		"unlisted": {
			Name:      "unlisted-profile",
			Languages: []string{"python3"},
			Packages:  []DependencyPackage{{Name: "unknown-package", Version: "1.0.0"}},
		},
		"latest": {
			Name:      "latest-profile",
			Languages: []string{"python3"},
			Packages:  []DependencyPackage{{Name: "data-tools", Version: "latest"}},
		},
		"remote-url": {
			Name:      "remote-profile",
			Languages: []string{"python3"},
			Packages:  []DependencyPackage{{Name: "remote-url", Version: "1.0.0"}},
		},
	} {
		if err := service.ValidateDependencyProfilePackages(profile); err == nil {
			t.Fatalf("expected package policy rejection for %s", name)
		}
	}
}

func TestNormalizeCreateAppliesDependencyPackagePolicy(t *testing.T) {
	service := NewService(config.FromEnv())
	service.dependencyProfiles = append(service.dependencyProfiles, DependencyProfile{
		Name:        "unsafe-profile",
		Version:     "2026.05.01",
		Status:      "ready",
		Enabled:     true,
		OwnerScope:  "global",
		Languages:   []string{"python3"},
		Packages:    []DependencyPackage{{Name: "unknown-package", Version: "1.0.0"}},
		BaseRuntime: "preview-process",
		Checksum:    "profile:unsafe-profile:2026.05.01",
	})

	if _, err := service.NormalizeCreate("session", 60, false, "", "unsafe-profile", 0, "", 0); err == nil || !strings.Contains(err.Error(), "not in the managed allowlist") {
		t.Fatalf("expected package policy rejection, got %v", err)
	}
}

func TestNormalizeCreateAppliesDependencyProfileBuildLimits(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxDependencyProfileSizeBytes = 1024
	cfg.DependencyProfileBuildTimeoutSeconds = 60
	service := NewService(cfg)
	service.dependencyProfiles = append(service.dependencyProfiles, DependencyProfile{
		Name:        "oversized-profile",
		Version:     "2026.05.01",
		Status:      "ready",
		Enabled:     true,
		OwnerScope:  "global",
		Languages:   []string{"python3"},
		Packages:    []DependencyPackage{},
		BaseRuntime: "preview-process",
		Checksum:    "profile:oversized-profile:2026.05.01",
		SizeBytes:   2048,
	})

	if _, err := service.NormalizeCreate("session", 60, false, "", "oversized-profile", 0, "", 0); err == nil || !strings.Contains(err.Error(), "exceeds max profile size") {
		t.Fatalf("expected dependency profile size limit rejection, got %v", err)
	}
}

func TestBuildDependencyProfileRegistersReadyProfile(t *testing.T) {
	service := NewService(config.FromEnv())

	result, err := service.BuildDependencyProfile(DependencyProfileBuildRequest{
		Name:        "office-safe",
		Version:     "2026.05.31",
		Languages:   []string{"python"},
		Packages:    []DependencyPackage{{Name: "data-tools", Version: "managed"}},
		BaseRuntime: "preview-process",
		Checksum:    "sha256:office-safe",
		SizeBytes:   1024,
		Description: "Managed document automation profile.",
	})
	if err != nil {
		t.Fatalf("expected dependency profile build to succeed, got %v", err)
	}
	if !result.Accepted || result.Status != "ready" || result.Profile == nil {
		t.Fatalf("expected ready build result, got %+v", result)
	}
	if result.Profile.Name != "office-safe" || result.Profile.Languages[0] != "python3" {
		t.Fatalf("expected normalized profile, got %+v", result.Profile)
	}

	decision, err := service.NormalizeCreate("session", 60, false, "", "office-safe", 0, "", 0)
	if err != nil {
		t.Fatalf("expected built profile to be selectable, got %v", err)
	}
	if decision.DependencyProfile != "office-safe" || decision.DependencyProfileVersion != "2026.05.31" {
		t.Fatalf("expected built profile in decision, got %+v", decision)
	}
}

func TestBuildDependencyProfilePromotesReservedProfile(t *testing.T) {
	service := NewService(config.FromEnv())

	result, err := service.BuildDependencyProfile(DependencyProfileBuildRequest{
		Name:        "skill-office",
		Version:     "2026.05.31",
		Languages:   []string{"python3", "nodejs"},
		Packages:    []DependencyPackage{{Ecosystem: "python3", Name: "office-tools", Version: "managed"}, {Ecosystem: "nodejs", Name: "office-tools", Version: "managed"}},
		BaseRuntime: "linux-secure",
		Checksum:    "sha256:skill-office",
		SizeBytes:   1024,
		Description: "Managed document automation profile.",
	})
	if err != nil {
		t.Fatalf("expected reserved dependency profile promotion to succeed, got %v", err)
	}
	if result.Profile == nil || result.Profile.Name != "skill-office" || !result.Profile.Enabled || result.Profile.Status != "ready" {
		t.Fatalf("expected promoted skill-office profile, got %+v", result)
	}

	decision, err := service.NormalizeCreate("session", 60, false, "", "skill-office", 0, "", 0)
	if err != nil {
		t.Fatalf("expected promoted profile to be selectable, got %v", err)
	}
	if decision.DependencyProfile != "skill-office" || decision.DependencyProfileVersion != "2026.05.31" {
		t.Fatalf("expected promoted profile in decision, got %+v", decision)
	}
}

func TestBuildDependencyProfileRejectsReadyProfileReplacement(t *testing.T) {
	service := NewService(config.FromEnv())

	_, err := service.BuildDependencyProfile(DependencyProfileBuildRequest{
		Name:        "workflow-safe",
		Version:     "2026.05.31",
		Languages:   []string{"python3"},
		BaseRuntime: "preview-process",
		Checksum:    "sha256:workflow-safe",
		SizeBytes:   1024,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected ready profile replacement rejection, got %v", err)
	}
}

func TestLoadDependencyProfilesReplacesReservedProfile(t *testing.T) {
	service := NewService(config.FromEnv())

	if err := service.LoadDependencyProfiles([]DependencyProfile{{
		Name:        "skill-office",
		Version:     "2026.05.31",
		Status:      "ready",
		Enabled:     true,
		OwnerScope:  "global",
		Languages:   []string{"python3", "nodejs"},
		Packages:    []DependencyPackage{{Ecosystem: "python3", Name: "office-tools", Version: "managed"}, {Ecosystem: "nodejs", Name: "office-tools", Version: "managed"}},
		BaseRuntime: "linux-secure",
		Checksum:    "sha256:skill-office",
		SizeBytes:   1024,
		Description: "Managed document automation profile.",
	}}); err != nil {
		t.Fatalf("load cached profile: %v", err)
	}

	decision, err := service.NormalizeCreate("session", 60, false, "", "skill-office", 0, "", 0)
	if err != nil {
		t.Fatalf("expected cached profile to replace reserved profile, got %v", err)
	}
	if decision.DependencyProfileVersion != "2026.05.31" {
		t.Fatalf("expected cached profile version, got %+v", decision)
	}
}

func TestBuildDependencyProfileReportsValidationFailure(t *testing.T) {
	service := NewService(config.FromEnv())

	result, err := service.BuildDependencyProfile(DependencyProfileBuildRequest{
		Name:      "bad-profile",
		Version:   "latest",
		Languages: []string{"python3"},
		Checksum:  "sha256:bad",
		SizeBytes: 1024,
	})
	if err == nil || !strings.Contains(err.Error(), "version must be pinned") {
		t.Fatalf("expected pinned version error, got %v", err)
	}
	if !result.Accepted || result.Status != "failed" || !strings.Contains(result.Error, "version must be pinned") {
		t.Fatalf("expected failed build result, got %+v", result)
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
	networkProfiles := previewSnapshot["network_profiles"].([]NetworkProfile)
	if len(networkProfiles) != 3 {
		t.Fatalf("expected network profiles, got %#v", networkProfiles)
	}
	if networkProfiles[0].Name != "deny-by-default" || !networkProfiles[0].Default || networkProfiles[0].NetworkEnabled {
		t.Fatalf("expected deny-by-default network profile, got %#v", networkProfiles[0])
	}
	if len(networkProfiles[0].DeniedCIDRRanges) == 0 || networkProfiles[0].DNSBehavior != "disabled" {
		t.Fatalf("expected deny-by-default egress policy fields, got %#v", networkProfiles[0])
	}
	if networkProfiles[1].Name != "workflow-safe" || !networkProfiles[1].NetworkEnabled || networkProfiles[1].MaxRequestDurationMS != 5000 {
		t.Fatalf("expected workflow-safe egress policy fields, got %#v", networkProfiles[1])
	}
	if len(networkProfiles[1].AllowedProtocols) != 1 || networkProfiles[1].AllowedProtocols[0] != "https" || len(networkProfiles[1].DeniedCIDRRanges) == 0 {
		t.Fatalf("expected workflow-safe protocol and denied range policy, got %#v", networkProfiles[1])
	}
	previewEnforcement := previewSnapshot["network_enforcement"].(map[string]any)
	if previewEnforcement["runtime_backend"] != "preview-process" || previewEnforcement["network_policy_enforced"] != false || previewEnforcement["network_enabled_requests_rejected"] != true {
		t.Fatalf("expected preview network enforcement surface, got %#v", previewEnforcement)
	}
	if previewEnforcement["rejection_code"] != "network_policy_not_enforced" {
		t.Fatalf("expected preview rejection code, got %#v", previewEnforcement)
	}
	if previewEnforcement["rejection_reason"] != `runtime backend "preview-process" does not enforce network policy` {
		t.Fatalf("expected preview rejection reason, got %#v", previewEnforcement)
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
	secureEnforcement := secureSnapshot["network_enforcement"].(map[string]any)
	if secureEnforcement["runtime_backend"] != "linux-secure" || secureEnforcement["network_policy_enforced"] != true || secureEnforcement["network_enabled_requests_rejected"] != false {
		t.Fatalf("expected secure network enforcement surface, got %#v", secureEnforcement)
	}
	if secureEnforcement["rejection_code"] != "" {
		t.Fatalf("expected empty secure rejection code, got %#v", secureEnforcement)
	}
	if secureEnforcement["rejection_reason"] != "" {
		t.Fatalf("expected empty secure rejection reason, got %#v", secureEnforcement)
	}
}

func TestNormalizeCreateReturnsEffectiveLimitsAndStructuredLimitError(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxActive = 2
	cfg.MaxConcurrentExecutions = 3
	cfg.MaxFileSizeKB = 128
	cfg.MaxWorkspaceBytesPerOrganization = 4096
	cfg.MaxArtifactManifestFiles = 7
	cfg.MaxArtifactManifestBytes = 8192
	cfg.MaxArtifactBytesPerOrganization = 16384
	cfg.MaxNetworkRequestsPerMinutePerOrganization = 5
	cfg.MaxDependencyProfilesPerOrganization = 2
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRuntimeCPUSeconds = 3
	cfg.SecureRuntimeMemoryBytes = 134217728
	cfg.SecureRuntimeProcessLimit = 32
	cfg.SecureRuntimeOpenFileLimit = 64
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
	if decision.EffectiveLimits.MaxNetworkRequestsPerMinutePerOrganization != cfg.MaxNetworkRequestsPerMinutePerOrganization {
		t.Fatalf("expected organization network request rate limit in decision, got %+v", decision.EffectiveLimits)
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
	if decision.EffectiveLimits.MaxWorkspaceBytesPerOrganization != cfg.MaxWorkspaceBytesPerOrganization {
		t.Fatalf("expected organization workspace byte limit in decision, got %+v", decision.EffectiveLimits)
	}
	if !decision.EffectiveLimits.OrganizationWorkspaceByteLimitEnforced {
		t.Fatalf("expected organization workspace byte limit enforcement in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxWorkspaceFiles != cfg.MaxWorkspaceFiles {
		t.Fatalf("expected workspace file limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxFileSizeBytes != 128*1024 {
		t.Fatalf("expected max file size bytes in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxArtifactManifestFiles != 7 || decision.EffectiveLimits.MaxArtifactManifestTotalBytes != 8192 || decision.EffectiveLimits.MaxArtifactManifestBytes != 8192 {
		t.Fatalf("expected artifact manifest limits in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxArtifactBytesPerOrganization != cfg.MaxArtifactBytesPerOrganization {
		t.Fatalf("expected organization artifact byte limit in decision, got %+v", decision.EffectiveLimits)
	}
	if !decision.EffectiveLimits.OrganizationArtifactByteLimitEnforced {
		t.Fatalf("expected organization artifact byte limit enforcement in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxDependencyProfilesPerOrganization != cfg.MaxDependencyProfilesPerOrganization {
		t.Fatalf("expected organization dependency profile limit in decision, got %+v", decision.EffectiveLimits)
	}
	if !decision.EffectiveLimits.OrganizationDependencyProfileLimitEnforced {
		t.Fatalf("expected organization dependency profile limit enforcement in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.MaxDependencyProfileSizeBytes != cfg.MaxDependencyProfileSizeBytes {
		t.Fatalf("expected dependency profile size limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.DependencyProfileBuildTimeoutSeconds != cfg.DependencyProfileBuildTimeoutSeconds {
		t.Fatalf("expected dependency profile build timeout in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.SecureRuntimeCPUSeconds != cfg.SecureRuntimeCPUSeconds {
		t.Fatalf("expected secure runtime cpu limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.SecureRuntimeMemoryBytes != cfg.SecureRuntimeMemoryBytes {
		t.Fatalf("expected secure runtime memory limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.SecureRuntimeProcessLimit != cfg.SecureRuntimeProcessLimit {
		t.Fatalf("expected secure runtime process limit in decision, got %+v", decision.EffectiveLimits)
	}
	if decision.EffectiveLimits.SecureRuntimeOpenFileLimit != cfg.SecureRuntimeOpenFileLimit {
		t.Fatalf("expected secure runtime open file limit in decision, got %+v", decision.EffectiveLimits)
	}
	if !decision.EffectiveLimits.SecureRuntimeResourceLimitsEnforced {
		t.Fatalf("expected secure runtime resource limits enforcement in decision, got %+v", decision.EffectiveLimits)
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
	if limits.MaxResultJSONBytes != 256*1024 {
		t.Fatalf("expected skill result JSON limit, got %d", limits.MaxResultJSONBytes)
	}
	if limits.MaxRequestBytes != 4*1024*1024 {
		t.Fatalf("expected skill request body limit, got %d", limits.MaxRequestBytes)
	}
	if limits.Stateless {
		t.Fatalf("expected skill-python to be workspace-bound, got %+v", limits)
	}

	shortLimits, err := service.NormalizeCommandLimits("code-short", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("expected code-short limits, got %v", err)
	}
	if !shortLimits.Stateless {
		t.Fatalf("expected code-short to be stateless, got %+v", shortLimits)
	}
	if shortLimits.MaxResultJSONBytes != 64*1024 {
		t.Fatalf("expected code-short result JSON limit, got %d", shortLimits.MaxResultJSONBytes)
	}
	if shortLimits.MaxRequestBytes != 128*1024 {
		t.Fatalf("expected code-short request body limit, got %d", shortLimits.MaxRequestBytes)
	}
	if shortLimits.NetworkAllowed {
		t.Fatalf("expected code-short network to be disabled by default, got %+v", shortLimits)
	}

	snapshot := service.Snapshot()
	profiles, ok := snapshot["command_profiles"].([]map[string]any)
	if !ok || len(profiles) == 0 {
		t.Fatalf("expected command profiles in snapshot, got %#v", snapshot["command_profiles"])
	}
	if profiles[0]["name"] != "code-short" || profiles[0]["max_result_json_bytes"] != 64*1024 {
		t.Fatalf("expected code-short result JSON limit in snapshot, got %#v", profiles[0])
	}
	if profiles[0]["max_request_bytes"] != 128*1024 {
		t.Fatalf("expected code-short request body limit in snapshot, got %#v", profiles[0])
	}
	if profiles[0]["network_allowed"] != false || profiles[0]["network"] != "disabled" {
		t.Fatalf("expected code-short profile network denial in snapshot, got %#v", profiles[0])
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

func TestValidateCommandProfileNetworkRequiresProfilePermission(t *testing.T) {
	service := NewService(config.FromEnv())
	limits, err := service.NormalizeCommandLimits("skill-python", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("expected command limits, got %v", err)
	}

	if err := service.ValidateCommandProfileNetwork(limits, true); err == nil {
		t.Fatal("expected profile network validation failure")
	}
	if err := service.ValidateCommandProfileNetwork(limits, false); err != nil {
		t.Fatalf("expected network-disabled request to pass profile validation, got %v", err)
	}
}
