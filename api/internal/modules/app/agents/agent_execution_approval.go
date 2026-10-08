package agents

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AppCodeAgentExecutionRuntimeUnavailable identifies a denied or unavailable
// operator-approved execution environment without exposing internal details.
var AppCodeAgentExecutionRuntimeUnavailable = apperror.MustCode("agent.execution.runtime.unavailable")

const agentExecutionDependencyPreparationModeOffline = "offline-bundled"
const agentExecutionDependencyPreparationModeControlledHTTP = "controlled-http"
const agentExecutionPackageNetworkMode = "package-install"

// CatalogDefinitions returns the public Agent execution error definition.
func CatalogDefinitions() []appcatalog.Definition {
	return append([]appcatalog.Definition{{
		Code: AppCodeAgentExecutionRuntimeUnavailable, Category: appcatalog.CategoryValidation,
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "This Agent execution environment is not approved or available.",
			appcatalog.LocaleChineseSimplified: "该 Agent 执行环境尚未获批或当前不可用。",
		},
	}}, executionruntime.JobErrorDefinitions()...)
}

// agentExecutionApproval is an operator-owned allowlist, not Agent-authored
// configuration. An absent or malformed list always disables publication.
type agentExecutionApproval struct {
	Backend                   string   `json:"backend"`
	OrganizationIDs           []string `json:"organization_ids"`
	PolicyIDs                 []string `json:"policy_ids"`
	DependencyPreparation     bool     `json:"dependency_preparation"`
	DependencyPreparationMode string   `json:"dependency_preparation_mode,omitempty"`
	DependencyInstallation    bool     `json:"dependency_installation"`
	PackageNetworkMode        string   `json:"package_network_mode,omitempty"`
}

type agentExecutionApprovals struct {
	Profiles      map[string]agentExecutionApproval
	HomeSnapshots map[string]agentExecutionHomeApproval
	// Enabled is an operator/deployment setting. It is deliberately not part
	// of the Agent configuration so an Agent editor can only opt in to the
	// already approved Linux environment.
	Enabled               bool
	DefaultProfileID      string
	DefaultPolicyID       string
	DefaultHomeSnapshotID string
	GatewayReady          bool
}

type agentExecutionHomeApproval struct {
	Backend         string   `json:"backend"`
	ProfileID       string   `json:"profile_id"`
	OrganizationIDs []string `json:"organization_ids"`
}

func agentExecutionApprovalsFromEnv() agentExecutionApprovals {
	var profiles map[string]agentExecutionApproval
	if err := json.Unmarshal([]byte(os.Getenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON")), &profiles); err != nil {
		profiles = nil
	}
	var snapshots map[string]agentExecutionHomeApproval
	if err := json.Unmarshal([]byte(os.Getenv("ZGI_AGENT_RUNTIME_APPROVED_HOME_SNAPSHOTS_JSON")), &snapshots); err != nil {
		snapshots = nil
	}
	return agentExecutionApprovals{
		Profiles:              profiles,
		HomeSnapshots:         snapshots,
		Enabled:               agentExecutionEnvBool("ZGI_AGENT_RUNTIME_ENABLED", false),
		DefaultProfileID:      strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_PROFILE_ID")),
		DefaultPolicyID:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_POLICY_ID")),
		DefaultHomeSnapshotID: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_HOME_SNAPSHOT_ID")),
		GatewayReady:          validAgentRuntimeGatewayURL(os.Getenv("ZGI_AGENT_RUNTIME_URL")) && strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY")) != "",
	}
}

func agentExecutionEnvBool(name string, fallback bool) bool {
	return executionruntime.EnvBool(name, fallback)
}

func validAgentRuntimeGatewayURL(raw string) bool { return executionruntime.ValidGatewayURL(raw) }

func (a agentExecutionApprovals) validate(organizationID string, cfg *dto.AgentExecutionRuntime) error {
	_, err := a.resolve(organizationID, cfg)
	return err
}

// resolve turns the user-facing boolean into the operator-approved runtime
// configuration. Profile, policy and snapshot values from the request are
// intentionally ignored. This keeps old stored configs readable while making
// deployment env the only source of execution-environment configuration.
func (a agentExecutionApprovals) resolve(organizationID string, cfg *dto.AgentExecutionRuntime) (*dto.AgentExecutionRuntime, error) {
	if cfg == nil || !cfg.Enabled {
		return cfg, nil
	}
	invalid := func(operation string) error {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation(operation))
	}
	if !a.Enabled {
		return nil, invalid("agent_execution.disabled_by_env")
	}
	if !a.GatewayReady {
		return nil, invalid("agent_execution.gateway_unconfigured")
	}
	profileID := strings.TrimSpace(a.DefaultProfileID)
	if profileID == "" && len(a.Profiles) == 1 {
		for id := range a.Profiles {
			profileID = strings.TrimSpace(id)
		}
	}
	if profileID == "" {
		return nil, invalid("agent_execution.default_profile_unconfigured")
	}
	profile, ok := a.Profiles[profileID]
	if !ok || (profile.Backend != "local-gvisor" && profile.Backend != "e2b") {
		return nil, invalid("agent_execution.profile_unapproved")
	}
	if !containsAgentExecutionApproval(profile.OrganizationIDs, organizationID) {
		return nil, invalid("agent_execution.organization_unapproved")
	}
	policyID := strings.TrimSpace(a.DefaultPolicyID)
	if policyID == "" && len(profile.PolicyIDs) == 1 {
		policyID = strings.TrimSpace(profile.PolicyIDs[0])
	}
	if profile.DependencyInstallation && policyID == "" {
		policyID = agentExecutionPackageNetworkMode
	}
	if !containsAgentExecutionApproval(profile.PolicyIDs, policyID) {
		return nil, invalid("agent_execution.policy_unapproved")
	}
	packageMode := strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode))
	if packageMode != "" && packageMode != agentExecutionPackageNetworkMode {
		return nil, invalid("agent_execution.package_network_mode_unapproved")
	}
	if packageMode == agentExecutionPackageNetworkMode && !profile.DependencyInstallation {
		return nil, invalid("agent_execution.package_install_capability_missing")
	}
	if profile.DependencyInstallation {
		if profile.Backend != "local-gvisor" && profile.Backend != "e2b" {
			return nil, invalid("agent_execution.package_install_backend_unavailable")
		}
		if packageMode != agentExecutionPackageNetworkMode {
			return nil, invalid("agent_execution.package_network_mode_required")
		}
		if !containsAgentExecutionApproval(profile.PolicyIDs, agentExecutionPackageNetworkMode) {
			return nil, invalid("agent_execution.package_network_policy_unapproved")
		}
		// Package installation is an operator-owned capability. Override the
		// legacy policy value so stored/user-facing configs cannot select a
		// broader network mode.
		policyID = agentExecutionPackageNetworkMode
	}
	dependencyMode := strings.ToLower(strings.TrimSpace(profile.DependencyPreparationMode))
	if dependencyMode != "" && dependencyMode != agentExecutionDependencyPreparationModeOffline && dependencyMode != agentExecutionDependencyPreparationModeControlledHTTP {
		return nil, invalid("agent_execution.dependency_preparation_mode_unapproved")
	}
	if (dependencyMode == agentExecutionDependencyPreparationModeControlledHTTP || strings.EqualFold(policyID, agentExecutionDependencyPreparationModeControlledHTTP)) && profile.Backend != "local-gvisor" {
		return nil, invalid("agent_execution.controlled_http_backend_unavailable")
	}
	if !profile.DependencyPreparation && dependencyMode != "" {
		return nil, invalid("agent_execution.dependency_preparation_mode_without_capability")
	}
	var homeSnapshotID *string
	if a.DefaultHomeSnapshotID != "" {
		homeSnapshotID = &a.DefaultHomeSnapshotID
		snapshot, ok := a.HomeSnapshots[a.DefaultHomeSnapshotID]
		if !ok || snapshot.Backend != profile.Backend || snapshot.ProfileID != profileID ||
			!containsAgentExecutionApproval(snapshot.OrganizationIDs, organizationID) {
			return nil, invalid("agent_execution.home_snapshot_unavailable")
		}
	}
	return &dto.AgentExecutionRuntime{Enabled: true, ProfileID: profileID, PolicyID: policyID, HomeSnapshotID: homeSnapshotID}, nil
}

func containsAgentExecutionApproval(allowed []string, value string) bool {
	if value == "" {
		return false
	}
	for _, item := range allowed {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}
