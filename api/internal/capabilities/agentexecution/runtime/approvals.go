package executionruntime

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

const dependencyPreparationModeOffline = "offline-bundled"
const dependencyPreparationModeControlledHTTP = "controlled-http"
const packageNetworkMode = "package-install"

// ApprovalError is an internal operator diagnosis. Entry adapters project it
// through their own public error catalog, never exposing Operation to clients.
type ApprovalError struct{ Operation string }

func (*ApprovalError) Error() string { return "execution environment is not approved or available" }

// ProfileApproval is an operator-owned allowlist, not application-authored
// configuration. An absent or malformed list always denies execution.
type ProfileApproval struct {
	Backend                   string   `json:"backend"`
	OrganizationIDs           []string `json:"organization_ids"`
	PolicyIDs                 []string `json:"policy_ids"`
	DependencyPreparation     bool     `json:"dependency_preparation"`
	DependencyPreparationMode string   `json:"dependency_preparation_mode,omitempty"`
	DependencyInstallation    bool     `json:"dependency_installation"`
	PackageNetworkMode        string   `json:"package_network_mode,omitempty"`
}

// Approvals is a deployment snapshot shared by execution entry adapters.
// Callers must not mutate its maps while resolving requests.
type Approvals struct {
	Profiles      map[string]ProfileApproval
	HomeSnapshots map[string]HomeApproval
	// Enabled is an operator/deployment setting. It is deliberately not part
	// of application configuration so an editor can only opt in to the
	// already approved Linux environment.
	Enabled               bool
	DefaultProfileID      string
	DefaultPolicyID       string
	DefaultHomeSnapshotID string
	GatewayReady          bool
}

// HomeApproval restricts a prebuilt Home to one backend/Profile and its tenants.
type HomeApproval struct {
	Backend         string   `json:"backend"`
	ProfileID       string   `json:"profile_id"`
	OrganizationIDs []string `json:"organization_ids"`
}

// ApprovalsFromEnv reads the existing operator settings. Invalid or missing
// approval JSON cannot grant execution to an application.
func ApprovalsFromEnv() Approvals {
	var profiles map[string]ProfileApproval
	if err := json.Unmarshal([]byte(os.Getenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON")), &profiles); err != nil {
		profiles = nil
	}
	var snapshots map[string]HomeApproval
	if err := json.Unmarshal([]byte(os.Getenv("ZGI_AGENT_RUNTIME_APPROVED_HOME_SNAPSHOTS_JSON")), &snapshots); err != nil {
		snapshots = nil
	}
	return Approvals{
		Profiles:              profiles,
		HomeSnapshots:         snapshots,
		Enabled:               EnvBool("ZGI_AGENT_RUNTIME_ENABLED", false),
		DefaultProfileID:      strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_PROFILE_ID")),
		DefaultPolicyID:       strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_POLICY_ID")),
		DefaultHomeSnapshotID: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_DEFAULT_HOME_SNAPSHOT_ID")),
		GatewayReady:          ValidGatewayURL(os.Getenv("ZGI_AGENT_RUNTIME_URL")) && strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY")) != "",
	}
}

// Resolve turns an application opt-in into operator-approved settings. No
// application-supplied Profile, policy, backend or snapshot is accepted here.
// A disabled application returns nil without needing a configured gateway.
func (a Approvals) Resolve(organizationID string, enabled bool) (*dto.AgentExecutionRuntime, error) {
	if !enabled {
		return nil, nil
	}
	invalid := func(operation string) error {
		return &ApprovalError{Operation: operation}
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
	if !containsApproval(profile.OrganizationIDs, organizationID) {
		return nil, invalid("agent_execution.organization_unapproved")
	}
	policyID := strings.TrimSpace(a.DefaultPolicyID)
	if policyID == "" && len(profile.PolicyIDs) == 1 {
		policyID = strings.TrimSpace(profile.PolicyIDs[0])
	}
	if profile.DependencyInstallation && policyID == "" {
		policyID = packageNetworkMode
	}
	if !containsApproval(profile.PolicyIDs, policyID) {
		return nil, invalid("agent_execution.policy_unapproved")
	}
	packageMode := strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode))
	if packageMode != "" && packageMode != packageNetworkMode {
		return nil, invalid("agent_execution.package_network_mode_unapproved")
	}
	if packageMode == packageNetworkMode && !profile.DependencyInstallation {
		return nil, invalid("agent_execution.package_install_capability_missing")
	}
	if profile.DependencyInstallation {
		if profile.Backend != "local-gvisor" && profile.Backend != "e2b" {
			return nil, invalid("agent_execution.package_install_backend_unavailable")
		}
		if packageMode != packageNetworkMode {
			return nil, invalid("agent_execution.package_network_mode_required")
		}
		if !containsApproval(profile.PolicyIDs, packageNetworkMode) {
			return nil, invalid("agent_execution.package_network_policy_unapproved")
		}
		// Package installation is an operator-owned capability. Override the
		// legacy policy value so stored/user-facing configs cannot select a
		// broader network mode.
		policyID = packageNetworkMode
	}
	dependencyMode := strings.ToLower(strings.TrimSpace(profile.DependencyPreparationMode))
	if dependencyMode != "" && dependencyMode != dependencyPreparationModeOffline && dependencyMode != dependencyPreparationModeControlledHTTP {
		return nil, invalid("agent_execution.dependency_preparation_mode_unapproved")
	}
	if (dependencyMode == dependencyPreparationModeControlledHTTP || strings.EqualFold(policyID, dependencyPreparationModeControlledHTTP)) && profile.Backend != "local-gvisor" {
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
			!containsApproval(snapshot.OrganizationIDs, organizationID) {
			return nil, invalid("agent_execution.home_snapshot_unavailable")
		}
	}
	return &dto.AgentExecutionRuntime{Enabled: true, ProfileID: profileID, PolicyID: policyID, HomeSnapshotID: homeSnapshotID}, nil
}

func containsApproval(allowed []string, value string) bool {
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
