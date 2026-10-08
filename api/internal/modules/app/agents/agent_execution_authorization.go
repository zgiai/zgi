package agents

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Recheck console permission and current opt-in without creating a missing
// draft. This remains an Agent entry concern, not a shared relay assumption.
func (s *agentsService) authorizeRuntimePlatform(ctx context.Context, agentID, accountID, organizationID string) error {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false, agentRuntimeConfigReadPermissionCodes("AGENT")...)
	if err != nil {
		return apperror.Wrap(err, catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.agent_access"))
	}
	if cfg == nil || s.organizationIDForAgentWorkspace(ctx, ag.TenantID.String()) != organizationID {
		return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.agent_scope"))
	}
	current := agentConfigResponse(agentID, cfg)
	if current.ExecutionRuntime == nil || !current.ExecutionRuntime.Enabled {
		return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("runtime_platform.agent_disabled"))
	}
	return s.executionApprovals.validate(organizationID, current.ExecutionRuntime)
}
