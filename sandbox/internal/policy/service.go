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
}

type Service struct {
	config             config.Config
	dependencyProfiles []DependencyProfile
	networkProfiles    []map[string]any
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
		"dependency_policy":   map[string]any{"mode": "managed-profiles", "supports_user_update": false},
		"dependency_profiles": s.dependencyProfiles,
		"limits": map[string]any{
			"max_workers":               s.config.MaxWorkers,
			"default_timeout":           s.config.TimeoutSeconds,
			"output_limit_kb":           s.config.OutputLimitKB,
			"session_ttl_secs":          s.config.SessionTTL,
			"interactive_ttl_secs":      s.config.InteractiveTTL,
			"max_active_sandboxes":      s.config.MaxActive,
			"max_command_timeout_secs":  s.config.CommandTimeout,
			"max_file_size_kb":          s.config.MaxFileSizeKB,
			"max_compat_ttl_secs":       300,
			"network_policy_enforced":   true,
			"dependency_updates_locked": true,
		},
	}
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

func (s *Service) NormalizeCreate(profile string, ttlSeconds int, networkEnabled bool, networkPolicy string, dependencyProfile string, activeCount int) (CreateDecision, error) {
	if s.config.MaxActive > 0 && activeCount >= s.config.MaxActive {
		return CreateDecision{}, fmt.Errorf("active sandbox limit reached: %d", s.config.MaxActive)
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

	return CreateDecision{
		RuntimeProfile:    runtimeProfile,
		TTL:               s.normalizeTTL(runtimeProfile, ttlSeconds),
		NetworkEnabled:    networkEnabled,
		NetworkPolicy:     policyName,
		DependencyProfile: dependencyName,
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

func (s *Service) MaxFileSizeBytes() int64 {
	return int64(s.config.MaxFileSizeKB) * 1024
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
