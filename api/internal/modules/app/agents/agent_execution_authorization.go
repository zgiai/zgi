package agents

import (
	"context"
	"time"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	apiKey "github.com/zgiai/zgi/api/internal/modules/api_key"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Recheck console permission and current opt-in without creating a missing
// draft. This remains an Agent entry concern, not a shared relay assumption.
func (s *agentsService) authorizeRuntimePlatform(ctx context.Context, agentID, accountID, organizationID string) error {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false, agentRuntimePlatformPermissionCodes("AGENT")...)
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

// Recheck the authenticated credential by ID, without retaining its token or
// confusing its owner with the final user. Mirrors the API middleware's live
// application surface contract for long-running tool invocations.
func (s *agentsService) authorizePublishedExecutionAPI(ctx context.Context, application, keyID, credentialWorkspace uuid.UUID) error {
	if s.db == nil || s.agentsRepo == nil || application == uuid.Nil || keyID == uuid.Nil || credentialWorkspace == uuid.Nil {
		return execution.ErrInvalidIdentity
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&apiKey.APIKey{}).
		Where("id = ? AND agent_id = ? AND tenant_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", keyID, application, credentialWorkspace, "active", time.Now()).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return execution.ErrInvalidIdentity
	}
	ag, err := s.agentsRepo.GetByID(ctx, application.String())
	if err != nil {
		return err
	}
	if ag == nil || ag.ID != application || ag.TenantID != credentialWorkspace || ag.AgentsType != "AGENT" || ag.DeletedAt != nil || !ag.IsWebAppActive() {
		return execution.ErrInvalidIdentity
	}
	fallback, auth, err := s.publishedRuntimeAuthorizationForAgent(ctx, ag)
	if err != nil {
		return err
	}
	if !runtimeauth.PolicyFromAuthorization(fallback, auth).Allows(runtimeauth.PublishedRuntimeSurfaceAPI) {
		return execution.ErrInvalidIdentity
	}
	return nil
}
