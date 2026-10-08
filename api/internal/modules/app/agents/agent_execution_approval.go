package agents

import (
	"errors"
	"net/http"

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

// Keep the Agent error and stored configuration contract at this boundary.
// Operator-owned environment selection is shared with other application types.
type agentExecutionApproval = executionruntime.ProfileApproval
type agentExecutionHomeApproval = executionruntime.HomeApproval
type agentExecutionApprovals executionruntime.Approvals

func agentExecutionApprovalsFromEnv() agentExecutionApprovals {
	return agentExecutionApprovals(executionruntime.ApprovalsFromEnv())
}

func agentExecutionEnvBool(name string, fallback bool) bool {
	return executionruntime.EnvBool(name, fallback)
}
func validAgentRuntimeGatewayURL(raw string) bool { return executionruntime.ValidGatewayURL(raw) }

func (a agentExecutionApprovals) validate(organizationID string, cfg *dto.AgentExecutionRuntime) error {
	_, err := a.resolve(organizationID, cfg)
	return err
}

func (a agentExecutionApprovals) resolve(organizationID string, cfg *dto.AgentExecutionRuntime) (*dto.AgentExecutionRuntime, error) {
	if cfg == nil || !cfg.Enabled {
		return cfg, nil
	}
	resolved, err := executionruntime.Approvals(a).Resolve(organizationID, cfg.Enabled)
	if err == nil {
		return resolved, nil
	}
	operation := "agent_execution.runtime_unavailable"
	var denied *executionruntime.ApprovalError
	if errors.As(err, &denied) {
		operation = denied.Operation
	}
	return nil, apperror.Wrap(err, AppCodeAgentExecutionRuntimeUnavailable, apperror.WithOperation(operation))
}
