package policy

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type DependencyProfile struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Status      string              `json:"status"`
	Enabled     bool                `json:"enabled"`
	OwnerScope  string              `json:"owner_scope"`
	Languages   []string            `json:"languages"`
	Packages    []DependencyPackage `json:"packages"`
	BaseRuntime string              `json:"base_runtime"`
	Checksum    string              `json:"checksum"`
	Description string              `json:"description"`
}

type DependencyPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type NetworkProfile struct {
	Name                 string   `json:"name"`
	Default              bool     `json:"default"`
	NetworkEnabled       bool     `json:"network_enabled"`
	AllowedHosts         []string `json:"allowed_hosts"`
	AllowedPorts         []int    `json:"allowed_ports"`
	AllowedProtocols     []string `json:"allowed_protocols"`
	DeniedCIDRRanges     []string `json:"denied_cidr_ranges"`
	DNSBehavior          string   `json:"dns_behavior"`
	MaxRequestDurationMS int      `json:"max_request_duration_ms"`
}

type CreateDecision struct {
	RuntimeProfile           sandbox.RuntimeProfile
	TTL                      time.Duration
	NetworkEnabled           bool
	NetworkPolicy            string
	DependencyProfile        string
	DependencyProfileVersion string
	EffectiveLimits          sandbox.ResourceLimits
}

type CommandLimits struct {
	Profile            string        `json:"profile"`
	Timeout            time.Duration `json:"-"`
	StdoutLimitBytes   int           `json:"stdout_limit_bytes"`
	StderrLimitBytes   int           `json:"stderr_limit_bytes"`
	MaxStdinBytes      int           `json:"max_stdin_bytes"`
	MaxRequestBytes    int           `json:"max_request_bytes"`
	MaxResultJSONBytes int           `json:"max_result_json_bytes"`
	Stateless          bool          `json:"stateless"`
	NetworkAllowed     bool          `json:"network_allowed"`
}

type TemplateLimits struct {
	Profile                string        `json:"profile"`
	Engine                 string        `json:"engine"`
	Timeout                time.Duration `json:"-"`
	TimeoutMS              int64         `json:"timeout_ms"`
	OutputLimitBytes       int           `json:"output_limit_bytes"`
	MaxTemplateBytes       int           `json:"max_template_bytes"`
	MaxVariableCount       int           `json:"max_variable_count"`
	MaxVariableDepth       int           `json:"max_variable_depth"`
	MaxVariableStringBytes int           `json:"max_variable_string_bytes"`
}

type Service struct {
	config             config.Config
	dependencyProfiles []DependencyProfile
	networkProfiles    []NetworkProfile
	commandProfiles    map[string]CommandLimits
	templateProfiles   map[string]TemplateLimits
}

type LimitError struct {
	Code    string         `json:"code"`
	Limit   string         `json:"limit"`
	Maximum int            `json:"maximum"`
	Actual  int            `json:"actual"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *LimitError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s exceeded: %s maximum is %d, actual is %d", e.Code, e.Limit, e.Maximum, e.Actual)
}

func (e *LimitError) ResponseDetails() map[string]any {
	if e == nil {
		return nil
	}
	details := map[string]any{
		"error_type": "limit_exceeded",
		"code":       e.Code,
		"limit":      e.Limit,
		"maximum":    e.Maximum,
		"actual":     e.Actual,
	}
	for key, value := range e.Details {
		details[key] = value
	}
	return details
}

func NewService(cfg config.Config) *Service {
	return &Service{
		config: cfg,
		dependencyProfiles: []DependencyProfile{
			{
				Name:        "stdlib",
				Version:     "2026.05.01",
				Status:      "ready",
				Enabled:     true,
				OwnerScope:  "global",
				Languages:   []string{"python3", "nodejs"},
				Packages:    []DependencyPackage{},
				BaseRuntime: "preview-process",
				Checksum:    "profile:stdlib:2026.05.01",
				Description: "Base language runtime with only built-in packages.",
			},
			{
				Name:        "workflow-safe",
				Version:     "2026.05.01",
				Status:      "ready",
				Enabled:     true,
				OwnerScope:  "global",
				Languages:   []string{"python3"},
				Packages:    []DependencyPackage{},
				BaseRuntime: "preview-process",
				Checksum:    "profile:workflow-safe:2026.05.01",
				Description: "Managed Python profile for deterministic workflow execution.",
			},
			{
				Name:        "node-basic",
				Version:     "2026.05.01",
				Status:      "ready",
				Enabled:     true,
				OwnerScope:  "global",
				Languages:   []string{"nodejs"},
				Packages:    []DependencyPackage{},
				BaseRuntime: "preview-process",
				Checksum:    "profile:node-basic:2026.05.01",
				Description: "Managed Node.js profile for script-style automation tasks.",
			},
			{
				Name:        "agent-tools",
				Version:     "2026.05.01",
				Status:      "ready",
				Enabled:     true,
				OwnerScope:  "global",
				Languages:   []string{"python3", "nodejs"},
				Packages:    []DependencyPackage{},
				BaseRuntime: "preview-process",
				Checksum:    "profile:agent-tools:2026.05.01",
				Description: "Broader operator-managed profile for internal agent tooling.",
			},
			{
				Name:        "python-data-preview",
				Version:     "2026.05.01",
				Status:      "disabled",
				Enabled:     false,
				OwnerScope:  "global",
				Languages:   []string{"python3"},
				Packages:    []DependencyPackage{{Name: "data-tools", Version: "managed"}},
				BaseRuntime: "preview-process",
				Checksum:    "profile:python-data-preview:2026.05.01",
				Description: "Reserved managed profile that is not available for sandbox creation.",
			},
		},
		networkProfiles: []NetworkProfile{
			{
				Name:                 "deny-by-default",
				Default:              true,
				NetworkEnabled:       false,
				AllowedHosts:         []string{},
				AllowedPorts:         []int{},
				AllowedProtocols:     []string{},
				DeniedCIDRRanges:     defaultDeniedCIDRRanges(),
				DNSBehavior:          "disabled",
				MaxRequestDurationMS: 0,
			},
			{
				Name:                 "workflow-safe",
				Default:              false,
				NetworkEnabled:       true,
				AllowedHosts:         []string{},
				AllowedPorts:         []int{443},
				AllowedProtocols:     []string{"https"},
				DeniedCIDRRanges:     defaultDeniedCIDRRanges(),
				DNSBehavior:          "resolve-and-check-denied-ranges",
				MaxRequestDurationMS: 5000,
			},
			{
				Name:                 "interactive-preview",
				Default:              false,
				NetworkEnabled:       true,
				AllowedHosts:         []string{},
				AllowedPorts:         []int{80, 443},
				AllowedProtocols:     []string{"http", "https"},
				DeniedCIDRRanges:     defaultDeniedCIDRRanges(),
				DNSBehavior:          "resolve-and-check-denied-ranges",
				MaxRequestDurationMS: 10000,
			},
		},
		commandProfiles: map[string]CommandLimits{
			"code-short": {
				Profile:            "code-short",
				Timeout:            5 * time.Second,
				StdoutLimitBytes:   64 * 1024,
				StderrLimitBytes:   64 * 1024,
				MaxStdinBytes:      64 * 1024,
				MaxRequestBytes:    128 * 1024,
				MaxResultJSONBytes: 64 * 1024,
				Stateless:          true,
			},
			"skill-python": {
				Profile:            "skill-python",
				Timeout:            30 * time.Second,
				StdoutLimitBytes:   1024 * 1024,
				StderrLimitBytes:   1024 * 1024,
				MaxStdinBytes:      1024 * 1024,
				MaxRequestBytes:    4 * 1024 * 1024,
				MaxResultJSONBytes: 256 * 1024,
			},
			"skill-node": {
				Profile:            "skill-node",
				Timeout:            30 * time.Second,
				StdoutLimitBytes:   1024 * 1024,
				StderrLimitBytes:   1024 * 1024,
				MaxStdinBytes:      1024 * 1024,
				MaxRequestBytes:    4 * 1024 * 1024,
				MaxResultJSONBytes: 256 * 1024,
			},
		},
		templateProfiles: map[string]TemplateLimits{
			"template-short": {
				Profile:                "template-short",
				Engine:                 "go-text",
				Timeout:                2 * time.Second,
				TimeoutMS:              2000,
				OutputLimitBytes:       64 * 1024,
				MaxTemplateBytes:       64 * 1024,
				MaxVariableCount:       128,
				MaxVariableDepth:       8,
				MaxVariableStringBytes: 16 * 1024,
			},
		},
	}
}

func (s *Service) Snapshot() map[string]any {
	return map[string]any{
		"runtime_profiles": []map[string]any{
			{
				"name":         sandbox.RuntimeLite,
				"isolation":    "process",
				"network":      "optional request flag",
				"user_updates": false,
			},
			{
				"name":         sandbox.RuntimeSession,
				"isolation":    "workspace directory",
				"network":      "policy controlled",
				"user_updates": false,
			},
			{
				"name":         sandbox.RuntimeInteractive,
				"isolation":    "preview workspace + routed endpoint",
				"network":      "policy controlled",
				"user_updates": false,
			},
		},
		"network_profiles":    s.networkProfiles,
		"network_enforcement": s.networkEnforcementSnapshot(),
		"command_profiles":    s.commandProfileSnapshot(),
		"template_profiles":   s.templateProfileSnapshot(),
		"dependency_policy":   map[string]any{"mode": "managed-profiles", "supports_user_update": false},
		"dependency_profiles": s.dependencyProfiles,
		"limits":              s.EffectiveLimits(),
	}
}

func (s *Service) NormalizeTemplateLimits(profile string, engine string, timeoutMS int, outputLimitKB int) (TemplateLimits, error) {
	if timeoutMS < 0 {
		return TemplateLimits{}, errors.New("template timeout_ms must be non-negative")
	}
	if outputLimitKB < 0 {
		return TemplateLimits{}, errors.New("template output_limit_kb must be non-negative")
	}
	name := strings.TrimSpace(profile)
	if name == "" {
		name = "template-short"
	}
	limits, ok := s.templateProfiles[name]
	if !ok {
		return TemplateLimits{}, fmt.Errorf("unsupported template profile: %s", name)
	}
	normalizedEngine := strings.TrimSpace(engine)
	if normalizedEngine == "" {
		normalizedEngine = limits.Engine
	}
	if normalizedEngine != limits.Engine {
		return TemplateLimits{}, fmt.Errorf("unsupported template engine: %s", normalizedEngine)
	}
	if timeoutMS > 0 {
		requested := time.Duration(timeoutMS) * time.Millisecond
		if requested < limits.Timeout {
			limits.Timeout = requested
			limits.TimeoutMS = int64(timeoutMS)
		}
	}
	if outputLimitKB > 0 {
		requested := outputLimitKB * 1024
		if requested < limits.OutputLimitBytes {
			limits.OutputLimitBytes = requested
		}
	}
	return limits, nil
}

func (s *Service) DependencyCatalog(language string) map[string]any {
	items := make([]DependencyProfile, 0, len(s.dependencyProfiles))
	for _, profile := range s.dependencyProfiles {
		if language == "" || slices.Contains(profile.Languages, normalizeLanguage(language)) {
			items = append(items, profile)
		}
	}

	return map[string]any{
		"language":             defaultString(normalizeLanguage(language), "python3"),
		"mode":                 "managed-profiles",
		"supports_user_update": false,
		"profiles":             items,
		"note":                 "Dynamic dependency installation is intentionally disabled in this preview runtime.",
	}
}

func (s *Service) ValidateDependencyProfileForLanguage(value string, language string) (DependencyProfile, error) {
	profile, err := s.normalizeDependencyProfile(value)
	if err != nil {
		return DependencyProfile{}, err
	}
	normalizedLanguage := normalizeLanguage(language)
	if normalizedLanguage != "" && !slices.Contains(profile.Languages, normalizedLanguage) {
		return DependencyProfile{}, fmt.Errorf("dependency profile %s does not support language: %s", profile.Name, normalizedLanguage)
	}
	return profile, nil
}

func (s *Service) NormalizeCreate(profile string, ttlSeconds int, networkEnabled bool, networkPolicy string, dependencyProfile string, activeCount int, organizationID string, organizationActiveCount int) (CreateDecision, error) {
	runtimeProfile, err := s.normalizeProfile(profile)
	if err != nil {
		return CreateDecision{}, err
	}

	policyName, err := s.normalizeNetworkPolicy(runtimeProfile, networkPolicy)
	if err != nil {
		return CreateDecision{}, err
	}

	dependency, err := s.normalizeDependencyProfile(dependencyProfile)
	if err != nil {
		return CreateDecision{}, err
	}

	if networkEnabled && !s.networkPolicyAllowsEgress(policyName) {
		return CreateDecision{}, errors.New("the selected network policy does not allow outbound network access")
	}
	if networkEnabled && !s.runtimeBackendEnforcesNetworkPolicy() {
		return CreateDecision{}, fmt.Errorf("runtime backend %q does not enforce network policy", s.normalizedRuntimeBackend())
	}

	if s.config.MaxActive > 0 && activeCount >= s.config.MaxActive {
		return CreateDecision{}, &LimitError{
			Code:    "active_sandbox_limit_exceeded",
			Limit:   "max_active_sandboxes",
			Maximum: s.config.MaxActive,
			Actual:  activeCount + 1,
			Details: map[string]any{"active_sandboxes": activeCount},
		}
	}
	if organizationID != "" && s.config.MaxActivePerOrganization > 0 && organizationActiveCount >= s.config.MaxActivePerOrganization {
		return CreateDecision{}, &LimitError{
			Code:    "organization_active_sandbox_limit_exceeded",
			Limit:   "max_active_sandboxes_per_organization",
			Maximum: s.config.MaxActivePerOrganization,
			Actual:  organizationActiveCount + 1,
			Details: map[string]any{
				"organization_id":  organizationID,
				"active_sandboxes": organizationActiveCount,
			},
		}
	}

	return CreateDecision{
		RuntimeProfile:           runtimeProfile,
		TTL:                      s.normalizeTTL(runtimeProfile, ttlSeconds),
		NetworkEnabled:           networkEnabled,
		NetworkPolicy:            policyName,
		DependencyProfile:        dependency.Name,
		DependencyProfileVersion: dependency.Version,
		EffectiveLimits:          s.EffectiveLimits(),
	}, nil
}

func (s *Service) NormalizeRenew(profile sandbox.RuntimeProfile, ttlSeconds int) (time.Duration, error) {
	if _, err := s.normalizeProfile(string(profile)); err != nil {
		return 0, err
	}
	return s.normalizeTTL(profile, ttlSeconds), nil
}

func (s *Service) ValidateCodeExecution(box sandbox.Sandbox, enableNetwork bool) error {
	if enableNetwork && !box.NetworkEnabled {
		return errors.New("network access is disabled for this sandbox")
	}
	if enableNetwork && !s.networkPolicyAllowsEgress(box.NetworkPolicy) {
		return fmt.Errorf("network policy %q does not allow outbound access", box.NetworkPolicy)
	}
	return nil
}

func (s *Service) ValidateCommandProfileNetwork(limits CommandLimits, enableNetwork bool) error {
	if enableNetwork && !limits.NetworkAllowed {
		return fmt.Errorf("network access is disabled for command profile: %s", limits.Profile)
	}
	return nil
}

func (s *Service) NormalizeCommandTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = s.config.CommandTimeout
	}
	if s.config.CommandTimeout > 0 && timeoutSeconds > s.config.CommandTimeout {
		timeoutSeconds = s.config.CommandTimeout
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func (s *Service) NormalizeCommandLimits(profile string, timeoutSeconds int, timeoutMS int, stdoutLimitKB int, stderrLimitKB int) (CommandLimits, error) {
	name := strings.TrimSpace(profile)
	if name == "" {
		name = "skill-python"
	}
	limits, ok := s.commandProfiles[name]
	if !ok {
		return CommandLimits{}, fmt.Errorf("unsupported command profile: %s", name)
	}

	if timeoutMS > 0 {
		limits.Timeout = time.Duration(timeoutMS) * time.Millisecond
	} else if timeoutSeconds > 0 {
		limits.Timeout = time.Duration(timeoutSeconds) * time.Second
	}
	maxTimeout := time.Duration(s.config.CommandTimeout) * time.Second
	if maxTimeout > 0 && limits.Timeout > maxTimeout {
		limits.Timeout = maxTimeout
	}
	if limits.Timeout <= 0 {
		limits.Timeout = 5 * time.Second
	}

	if stdoutLimitKB > 0 {
		limits.StdoutLimitBytes = stdoutLimitKB * 1024
	}
	if stderrLimitKB > 0 {
		limits.StderrLimitBytes = stderrLimitKB * 1024
	}
	maxOutputBytes := s.config.OutputLimitKB * 1024
	if maxOutputBytes > 0 {
		if limits.StdoutLimitBytes > maxOutputBytes {
			limits.StdoutLimitBytes = maxOutputBytes
		}
		if limits.StderrLimitBytes > maxOutputBytes {
			limits.StderrLimitBytes = maxOutputBytes
		}
	}
	if limits.StdoutLimitBytes <= 0 {
		limits.StdoutLimitBytes = 64 * 1024
	}
	if limits.StderrLimitBytes <= 0 {
		limits.StderrLimitBytes = 64 * 1024
	}
	if limits.MaxResultJSONBytes <= 0 {
		limits.MaxResultJSONBytes = 64 * 1024
	}
	if limits.MaxRequestBytes <= 0 {
		limits.MaxRequestBytes = 128 * 1024
	}
	return limits, nil
}

func (s *Service) MaxFileSizeBytes() int64 {
	if s.config.MaxFileSizeKB <= 0 {
		return 256 * 1024
	}
	return int64(s.config.MaxFileSizeKB) * 1024
}

func (s *Service) EffectiveLimits() sandbox.ResourceLimits {
	maxFileSizeBytes := s.MaxFileSizeBytes()
	maxFileSizeKB := s.config.MaxFileSizeKB
	if maxFileSizeKB <= 0 {
		maxFileSizeKB = 256
	}
	maxArtifactManifestFiles := s.config.MaxArtifactManifestFiles
	if maxArtifactManifestFiles <= 0 {
		maxArtifactManifestFiles = 100
	}
	maxArtifactManifestBytes := s.config.MaxArtifactManifestBytes
	if maxArtifactManifestBytes <= 0 {
		maxArtifactManifestBytes = maxFileSizeBytes * 256
	}
	return sandbox.ResourceLimits{
		RuntimeBackend:                             s.normalizedRuntimeBackend(),
		NetworkPolicyEnforced:                      s.runtimeBackendEnforcesNetworkPolicy(),
		MaxWorkers:                                 s.config.MaxWorkers,
		MaxActiveSandboxes:                         s.config.MaxActive,
		MaxConcurrentExecutions:                    s.config.MaxConcurrentExecutions,
		MaxConcurrentExecutionsPerProfile:          s.config.MaxConcurrentExecutionsPerProfile,
		MaxActiveSandboxesPerOrganization:          s.config.MaxActivePerOrganization,
		MaxConcurrentExecutionsPerOrganization:     s.config.MaxConcurrentExecutionsPerOrganization,
		MaxExecutionsPerMinutePerOrganization:      s.config.MaxExecutionsPerMinutePerOrganization,
		MaxQueuedExecutionsPerOrganization:         s.config.MaxQueuedExecutionsPerOrganization,
		MaxWorkspaceFiles:                          s.config.MaxWorkspaceFiles,
		MaxWorkspaceBytes:                          s.config.MaxWorkspaceBytes,
		MaxWorkspaceBytesPerOrganization:           s.config.MaxWorkspaceBytesPerOrganization,
		QueueTimeoutMS:                             s.config.QueueTimeoutMS,
		DefaultTimeoutSeconds:                      s.config.TimeoutSeconds,
		DefaultExecutionTimeoutMS:                  int64(s.config.TimeoutSeconds) * 1000,
		OutputLimitKB:                              s.config.OutputLimitKB,
		MaxCommandTimeoutMS:                        int64(s.config.CommandTimeout) * 1000,
		MaxCommandTimeoutSeconds:                   s.config.CommandTimeout,
		OutputLimitBytes:                           s.config.OutputLimitKB * 1024,
		MaxFileSizeKB:                              maxFileSizeKB,
		MaxFileSizeBytes:                           maxFileSizeBytes,
		MaxArchiveFiles:                            256,
		MaxArchiveTotalBytes:                       maxFileSizeBytes * 256,
		MaxArtifactManifestFiles:                   maxArtifactManifestFiles,
		MaxArtifactManifestTotalBytes:              maxArtifactManifestBytes,
		MaxArtifactManifestBytes:                   maxArtifactManifestBytes,
		MaxArtifactBytesPerOrganization:            s.config.MaxArtifactBytesPerOrganization,
		MaxDependencyProfilesPerOrganization:       s.config.MaxDependencyProfilesPerOrganization,
		SessionTTLSecs:                             s.config.SessionTTL,
		SessionTTLSeconds:                          s.config.SessionTTL,
		InteractiveTTLSecs:                         s.config.InteractiveTTL,
		InteractiveTTLSeconds:                      s.config.InteractiveTTL,
		MaxCompatTTLSecs:                           300,
		MaxCompatTTLSeconds:                        300,
		DependencyUpdatesLocked:                    true,
		WorkspaceFileLimitEnforced:                 s.config.MaxWorkspaceFiles > 0,
		WorkspaceByteLimitEnforced:                 s.config.MaxWorkspaceBytes > 0,
		OrganizationWorkspaceByteLimitEnforced:     s.config.MaxWorkspaceBytesPerOrganization > 0,
		OrganizationArtifactByteLimitEnforced:      s.config.MaxArtifactBytesPerOrganization > 0,
		OrganizationDependencyProfileLimitEnforced: s.config.MaxDependencyProfilesPerOrganization > 0,
	}
}

func (s *Service) MaxExecutionsPerMinutePerOrganization() int {
	return s.config.MaxExecutionsPerMinutePerOrganization
}

func (s *Service) MaxConcurrentExecutions() int {
	return s.config.MaxConcurrentExecutions
}

func (s *Service) MaxConcurrentExecutionsPerProfile() int {
	return s.config.MaxConcurrentExecutionsPerProfile
}

func (s *Service) MaxConcurrentExecutionsPerOrganization() int {
	return s.config.MaxConcurrentExecutionsPerOrganization
}

func (s *Service) MaxQueuedExecutionsPerOrganization() int {
	return s.config.MaxQueuedExecutionsPerOrganization
}

func (s *Service) QueueTimeoutMS() int {
	return s.config.QueueTimeoutMS
}

func (s *Service) MaxWorkspaceBytes() int64 {
	return s.config.MaxWorkspaceBytes
}

func (s *Service) MaxWorkspaceBytesPerOrganization() int64 {
	return s.config.MaxWorkspaceBytesPerOrganization
}

func (s *Service) MaxWorkspaceFiles() int {
	return s.config.MaxWorkspaceFiles
}

func (s *Service) MaxArtifactManifestBytes() int64 {
	return s.EffectiveLimits().MaxArtifactManifestTotalBytes
}

func (s *Service) MaxArtifactManifestFiles() int {
	return s.EffectiveLimits().MaxArtifactManifestFiles
}

func (s *Service) MaxArtifactBytesPerOrganization() int64 {
	return s.config.MaxArtifactBytesPerOrganization
}

func (s *Service) MaxDependencyProfilesPerOrganization() int {
	return s.config.MaxDependencyProfilesPerOrganization
}

func (s *Service) RuntimeBackend() string {
	return s.normalizedRuntimeBackend()
}

func (s *Service) NetworkPolicyEnforced() bool {
	return s.runtimeBackendEnforcesNetworkPolicy()
}

func (s *Service) commandProfileSnapshot() []map[string]any {
	names := []string{"code-short", "skill-python", "skill-node"}
	items := make([]map[string]any, 0, len(names))
	for _, name := range names {
		profile, ok := s.commandProfiles[name]
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"name":                  profile.Profile,
			"default_timeout_ms":    profile.Timeout.Milliseconds(),
			"stdout_limit_bytes":    profile.StdoutLimitBytes,
			"stderr_limit_bytes":    profile.StderrLimitBytes,
			"max_stdin_bytes":       profile.MaxStdinBytes,
			"max_request_bytes":     profile.MaxRequestBytes,
			"max_result_json_bytes": profile.MaxResultJSONBytes,
			"stateless":             profile.Stateless,
			"network_allowed":       profile.NetworkAllowed,
			"network":               networkProfileSummary(profile),
		})
	}
	return items
}

func (s *Service) networkEnforcementSnapshot() map[string]any {
	enforced := s.NetworkPolicyEnforced()
	return map[string]any{
		"runtime_backend":                   s.RuntimeBackend(),
		"network_policy_enforced":           enforced,
		"network_enabled_requests_rejected": !enforced,
		"rejection_code":                    networkEnforcementRejectionCode(enforced),
		"rejection_reason":                  networkEnforcementRejectionReason(enforced, s.RuntimeBackend()),
	}
}

func networkEnforcementRejectionCode(enforced bool) string {
	if enforced {
		return ""
	}
	return "network_policy_not_enforced"
}

func networkEnforcementRejectionReason(enforced bool, backend string) string {
	if enforced {
		return ""
	}
	return fmt.Sprintf("runtime backend %q does not enforce network policy", backend)
}

func networkProfileSummary(profile CommandLimits) string {
	if profile.NetworkAllowed {
		return "requires sandbox policy"
	}
	return "disabled"
}

func (s *Service) templateProfileSnapshot() []TemplateLimits {
	items := make([]TemplateLimits, 0, len(s.templateProfiles))
	if profile, ok := s.templateProfiles["template-short"]; ok {
		items = append(items, profile)
	}
	return items
}

func (s *Service) normalizeProfile(profile string) (sandbox.RuntimeProfile, error) {
	switch sandbox.RuntimeProfile(strings.TrimSpace(profile)) {
	case "", sandbox.RuntimeSession:
		return sandbox.RuntimeSession, nil
	case sandbox.RuntimeInteractive:
		return sandbox.RuntimeInteractive, nil
	case sandbox.RuntimeLite:
		return sandbox.RuntimeLite, nil
	default:
		return "", errors.New("unsupported runtime profile")
	}
}

func (s *Service) normalizeNetworkPolicy(profile sandbox.RuntimeProfile, value string) (string, error) {
	policyName := strings.TrimSpace(value)
	if policyName == "" {
		switch profile {
		case sandbox.RuntimeInteractive:
			return "interactive-preview", nil
		default:
			return "deny-by-default", nil
		}
	}

	for _, item := range s.networkProfiles {
		if item.Name == policyName {
			if profile == sandbox.RuntimeSession && policyName == "interactive-preview" {
				return "", errors.New("interactive-preview network policy is only valid for interactive sandboxes")
			}
			return policyName, nil
		}
	}

	return "", fmt.Errorf("unsupported network policy: %s", policyName)
}

func (s *Service) normalizeDependencyProfile(value string) (DependencyProfile, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		name = "stdlib"
	}
	for _, profile := range s.dependencyProfiles {
		if profile.Name == name {
			if !profile.Enabled || profile.Status != "ready" {
				return DependencyProfile{}, fmt.Errorf("dependency profile is not enabled: %s", name)
			}
			return profile, nil
		}
	}
	return DependencyProfile{}, fmt.Errorf("unsupported dependency profile: %s", name)
}

func (s *Service) normalizeTTL(profile sandbox.RuntimeProfile, ttlSeconds int) time.Duration {
	maxTTL := s.maxTTL(profile)
	if ttlSeconds <= 0 {
		return maxTTL
	}

	requested := time.Duration(ttlSeconds) * time.Second
	if requested > maxTTL {
		return maxTTL
	}
	return requested
}

func (s *Service) maxTTL(profile sandbox.RuntimeProfile) time.Duration {
	switch profile {
	case sandbox.RuntimeInteractive:
		return time.Duration(s.config.InteractiveTTL) * time.Second
	case sandbox.RuntimeLite:
		return 5 * time.Minute
	default:
		return time.Duration(s.config.SessionTTL) * time.Second
	}
}

func (s *Service) networkPolicyAllowsEgress(policyName string) bool {
	for _, item := range s.networkProfiles {
		if item.Name == policyName {
			return item.NetworkEnabled
		}
	}
	return false
}

func defaultDeniedCIDRRanges() []string {
	return []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
}

func (s *Service) runtimeBackendEnforcesNetworkPolicy() bool {
	return s.config.NetworkPolicyEnforced()
}

func (s *Service) normalizedRuntimeBackend() string {
	return s.config.RuntimeBackendName()
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "python", "python3":
		return "python3"
	case "node", "nodejs", "javascript":
		return "nodejs"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
