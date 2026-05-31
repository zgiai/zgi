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
	Name        string   `json:"name"`
	Languages   []string `json:"languages"`
	Description string   `json:"description"`
}

type CreateDecision struct {
	RuntimeProfile    sandbox.RuntimeProfile
	TTL               time.Duration
	NetworkEnabled    bool
	NetworkPolicy     string
	DependencyProfile string
	EffectiveLimits   sandbox.ResourceLimits
}

type CommandLimits struct {
	Profile          string        `json:"profile"`
	Timeout          time.Duration `json:"-"`
	StdoutLimitBytes int           `json:"stdout_limit_bytes"`
	StderrLimitBytes int           `json:"stderr_limit_bytes"`
	MaxStdinBytes    int           `json:"max_stdin_bytes"`
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
	networkProfiles    []map[string]any
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
				Languages:   []string{"python3", "nodejs"},
				Description: "Base language runtime with only built-in packages.",
			},
			{
				Name:        "workflow-safe",
				Languages:   []string{"python3"},
				Description: "Managed Python profile for deterministic workflow execution.",
			},
			{
				Name:        "node-basic",
				Languages:   []string{"nodejs"},
				Description: "Managed Node.js profile for script-style automation tasks.",
			},
			{
				Name:        "agent-tools",
				Languages:   []string{"python3", "nodejs"},
				Description: "Broader operator-managed profile for internal agent tooling.",
			},
		},
		networkProfiles: []map[string]any{
			{"name": "deny-by-default", "default": true, "network_enabled": false},
			{"name": "workflow-safe", "default": false, "network_enabled": true},
			{"name": "interactive-preview", "default": false, "network_enabled": true},
		},
		commandProfiles: map[string]CommandLimits{
			"code-short": {
				Profile:          "code-short",
				Timeout:          5 * time.Second,
				StdoutLimitBytes: 64 * 1024,
				StderrLimitBytes: 64 * 1024,
				MaxStdinBytes:    64 * 1024,
			},
			"skill-python": {
				Profile:          "skill-python",
				Timeout:          30 * time.Second,
				StdoutLimitBytes: 1024 * 1024,
				StderrLimitBytes: 1024 * 1024,
				MaxStdinBytes:    1024 * 1024,
			},
			"skill-node": {
				Profile:          "skill-node",
				Timeout:          30 * time.Second,
				StdoutLimitBytes: 1024 * 1024,
				StderrLimitBytes: 1024 * 1024,
				MaxStdinBytes:    1024 * 1024,
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

func (s *Service) NormalizeCreate(profile string, ttlSeconds int, networkEnabled bool, networkPolicy string, dependencyProfile string, activeCount int, organizationID string, organizationActiveCount int) (CreateDecision, error) {
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

	runtimeProfile, err := s.normalizeProfile(profile)
	if err != nil {
		return CreateDecision{}, err
	}

	policyName, err := s.normalizeNetworkPolicy(runtimeProfile, networkPolicy)
	if err != nil {
		return CreateDecision{}, err
	}

	dependencyName, err := s.normalizeDependencyProfile(dependencyProfile)
	if err != nil {
		return CreateDecision{}, err
	}

	if networkEnabled && !s.networkPolicyAllowsEgress(policyName) {
		return CreateDecision{}, errors.New("the selected network policy does not allow outbound network access")
	}
	if networkEnabled && !s.runtimeBackendEnforcesNetworkPolicy() {
		return CreateDecision{}, fmt.Errorf("runtime backend %q does not enforce network policy", s.normalizedRuntimeBackend())
	}

	return CreateDecision{
		RuntimeProfile:    runtimeProfile,
		TTL:               s.normalizeTTL(runtimeProfile, ttlSeconds),
		NetworkEnabled:    networkEnabled,
		NetworkPolicy:     policyName,
		DependencyProfile: dependencyName,
		EffectiveLimits:   s.EffectiveLimits(),
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
	return sandbox.ResourceLimits{
		RuntimeBackend:                         s.normalizedRuntimeBackend(),
		NetworkPolicyEnforced:                  s.runtimeBackendEnforcesNetworkPolicy(),
		MaxWorkers:                             s.config.MaxWorkers,
		MaxActiveSandboxes:                     s.config.MaxActive,
		MaxActiveSandboxesPerOrganization:      s.config.MaxActivePerOrganization,
		MaxConcurrentExecutionsPerOrganization: s.config.MaxConcurrentExecutionsPerOrganization,
		MaxExecutionsPerMinutePerOrganization:  s.config.MaxExecutionsPerMinutePerOrganization,
		MaxWorkspaceFiles:                      s.config.MaxWorkspaceFiles,
		MaxWorkspaceBytes:                      s.config.MaxWorkspaceBytes,
		QueueTimeoutMS:                         s.config.QueueTimeoutMS,
		DefaultTimeoutSeconds:                  s.config.TimeoutSeconds,
		DefaultExecutionTimeoutMS:              int64(s.config.TimeoutSeconds) * 1000,
		OutputLimitKB:                          s.config.OutputLimitKB,
		MaxCommandTimeoutMS:                    int64(s.config.CommandTimeout) * 1000,
		MaxCommandTimeoutSeconds:               s.config.CommandTimeout,
		OutputLimitBytes:                       s.config.OutputLimitKB * 1024,
		MaxFileSizeKB:                          maxFileSizeKB,
		MaxFileSizeBytes:                       maxFileSizeBytes,
		MaxArchiveFiles:                        256,
		MaxArchiveTotalBytes:                   maxFileSizeBytes * 256,
		MaxArtifactManifestFiles:               100,
		MaxArtifactManifestTotalBytes:          maxFileSizeBytes * 256,
		SessionTTLSecs:                         s.config.SessionTTL,
		SessionTTLSeconds:                      s.config.SessionTTL,
		InteractiveTTLSecs:                     s.config.InteractiveTTL,
		InteractiveTTLSeconds:                  s.config.InteractiveTTL,
		MaxCompatTTLSecs:                       300,
		MaxCompatTTLSeconds:                    300,
		DependencyUpdatesLocked:                true,
		WorkspaceFileLimitEnforced:             s.config.MaxWorkspaceFiles > 0,
		WorkspaceByteLimitEnforced:             s.config.MaxWorkspaceBytes > 0,
	}
}

func (s *Service) MaxExecutionsPerMinutePerOrganization() int {
	return s.config.MaxExecutionsPerMinutePerOrganization
}

func (s *Service) MaxConcurrentExecutionsPerOrganization() int {
	return s.config.MaxConcurrentExecutionsPerOrganization
}

func (s *Service) MaxWorkspaceBytes() int64 {
	return s.config.MaxWorkspaceBytes
}

func (s *Service) MaxWorkspaceFiles() int {
	return s.config.MaxWorkspaceFiles
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
			"name":               profile.Profile,
			"default_timeout_ms": profile.Timeout.Milliseconds(),
			"stdout_limit_bytes": profile.StdoutLimitBytes,
			"stderr_limit_bytes": profile.StderrLimitBytes,
			"max_stdin_bytes":    profile.MaxStdinBytes,
			"network":            "inherits sandbox policy",
		})
	}
	return items
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
		if item["name"] == policyName {
			if profile == sandbox.RuntimeSession && policyName == "interactive-preview" {
				return "", errors.New("interactive-preview network policy is only valid for interactive sandboxes")
			}
			return policyName, nil
		}
	}

	return "", fmt.Errorf("unsupported network policy: %s", policyName)
}

func (s *Service) normalizeDependencyProfile(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "stdlib", nil
	}
	for _, profile := range s.dependencyProfiles {
		if profile.Name == name {
			return name, nil
		}
	}
	return "", fmt.Errorf("unsupported dependency profile: %s", name)
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
	switch policyName {
	case "workflow-safe", "interactive-preview":
		return true
	default:
		return false
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
