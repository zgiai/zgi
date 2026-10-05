package agents

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AppCodeAgentExecutionRuntimeUnavailable identifies a denied or unavailable
// operator-approved execution environment without exposing internal details.
var AppCodeAgentExecutionRuntimeUnavailable = apperror.MustCode("agent.execution.runtime.unavailable")

const agentExecutionDependencyPreparationModeOffline = "offline-bundled"
const agentExecutionDependencyPreparationModeControlledHTTP = "controlled-http"

// CatalogDefinitions returns the public Agent execution error definition.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{{
		Code: AppCodeAgentExecutionRuntimeUnavailable, Category: appcatalog.CategoryValidation,
		HTTPStatus: http.StatusBadRequest, Retryable: false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "This Agent execution environment is not approved or available.",
			appcatalog.LocaleChineseSimplified: "该 Agent 执行环境尚未获批或当前不可用。",
		},
	}}
}

// agentExecutionApproval is an operator-owned allowlist, not Agent-authored
// configuration. An absent or malformed list always disables publication.
type agentExecutionApproval struct {
	Backend                   string   `json:"backend"`
	OrganizationIDs           []string `json:"organization_ids"`
	PolicyIDs                 []string `json:"policy_ids"`
	DependencyPreparation     bool     `json:"dependency_preparation"`
	DependencyPreparationMode string   `json:"dependency_preparation_mode,omitempty"`
}

type agentExecutionApprovals struct {
	Profiles      map[string]agentExecutionApproval
	HomeSnapshots map[string]agentExecutionHomeApproval
	GatewayReady  bool
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
		Profiles:      profiles,
		HomeSnapshots: snapshots,
		GatewayReady:  validAgentRuntimeGatewayURL(os.Getenv("ZGI_AGENT_RUNTIME_URL")) && strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY")) != "",
	}
}

func validAgentRuntimeGatewayURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func (a agentExecutionApprovals) validate(organizationID string, cfg *dto.AgentExecutionRuntime) error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	invalid := func(operation string) error {
		return apperror.New(AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation(operation))
	}
	if !a.GatewayReady {
		return invalid("agent_execution.gateway_unconfigured")
	}
	profile, ok := a.Profiles[strings.TrimSpace(cfg.ProfileID)]
	if !ok || (profile.Backend != "local-gvisor" && profile.Backend != "e2b") {
		return invalid("agent_execution.profile_unapproved")
	}
	if !containsAgentExecutionApproval(profile.OrganizationIDs, organizationID) {
		return invalid("agent_execution.organization_unapproved")
	}
	if !containsAgentExecutionApproval(profile.PolicyIDs, strings.TrimSpace(cfg.PolicyID)) {
		return invalid("agent_execution.policy_unapproved")
	}
	dependencyMode := strings.ToLower(strings.TrimSpace(profile.DependencyPreparationMode))
	if dependencyMode != "" && dependencyMode != agentExecutionDependencyPreparationModeOffline && dependencyMode != agentExecutionDependencyPreparationModeControlledHTTP {
		return invalid("agent_execution.dependency_preparation_mode_unapproved")
	}
	if (dependencyMode == agentExecutionDependencyPreparationModeControlledHTTP || strings.EqualFold(strings.TrimSpace(cfg.PolicyID), agentExecutionDependencyPreparationModeControlledHTTP)) && profile.Backend != "local-gvisor" {
		return invalid("agent_execution.controlled_http_backend_unavailable")
	}
	if !profile.DependencyPreparation && dependencyMode != "" {
		return invalid("agent_execution.dependency_preparation_mode_without_capability")
	}
	if cfg.HomeSnapshotID != nil && strings.TrimSpace(*cfg.HomeSnapshotID) != "" {
		snapshot, ok := a.HomeSnapshots[strings.TrimSpace(*cfg.HomeSnapshotID)]
		if !ok || snapshot.Backend != profile.Backend || snapshot.ProfileID != strings.TrimSpace(cfg.ProfileID) ||
			!containsAgentExecutionApproval(snapshot.OrganizationIDs, organizationID) {
			return invalid("agent_execution.home_snapshot_unavailable")
		}
	}
	return nil
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
