package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const maxWebAppOfflineReasonLength = 500

var (
	errInvalidWebAppStatus        = errors.New("invalid web app status")
	errWebAppOfflineReasonTooLong = errors.New("web app offline reason too long")
)

func (s *agentsService) UpdateWebAppStatus(ctx context.Context, agentID string, req dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("invalid agent ID")
	}

	status := AgentWebAppStatus(strings.TrimSpace(req.Status))
	if !IsValidAgentWebAppStatus(status) {
		return nil, errInvalidWebAppStatus
	}

	reason := strings.TrimSpace(req.Reason)
	if utf8.RuneCountInString(reason) > maxWebAppOfflineReasonLength {
		return nil, errWebAppOfflineReasonTooLong
	}
	if status == AgentWebAppStatusActive {
		reason = ""
	}

	accountID := accountIDFromContext(ctx)
	if accountID == "" {
		return nil, fmt.Errorf("unauthorized: account ID not found in context")
	}

	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil || isSystemManagedAgent(ag) {
		return nil, fmt.Errorf("agent not found")
	}

	if err := s.ensureCanManageAgent(ctx, ag, accountID, agentRuntimeAccessManagePermissionCodes(ag.AgentsType)...); err != nil {
		return nil, err
	}

	if s.db != nil {
		if err := s.updateAgentWebAppStatusAndRuntimeSurface(ctx, ag, status, reason, accountID); err != nil {
			return nil, err
		}
	} else {
		if err := s.agentsRepo.UpdateWebAppStatus(ctx, agentID, status, reason, accountID); err != nil {
			return nil, err
		}
	}

	updatedAgent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to load updated agent: %w", err)
	}

	return &dto.WebAppStatusResponse{
		AgentID:      updatedAgent.ID.String(),
		WebAppID:     updatedAgent.WebAppID.String(),
		WebAppStatus: string(NormalizeAgentWebAppStatus(updatedAgent.WebAppStatus)),
		UpdatedAt:    updatedAgent.UpdatedAt.Unix(),
	}, nil
}

func isSystemManagedAgent(ag *Agent) bool {
	return ag == nil || ag.TenantID == uuid.Nil
}

func (s *agentsService) RequireAgentManageAccess(ctx context.Context, agentID, accountID string) error {
	agentID = strings.TrimSpace(agentID)
	accountID = strings.TrimSpace(accountID)
	if agentID == "" {
		return fmt.Errorf("%w: agent id is required", runtimeservice.ErrInvalidInput)
	}
	if accountID == "" {
		return runtimeservice.ErrUnauthorized
	}
	if _, err := uuid.Parse(agentID); err != nil {
		return fmt.Errorf("%w: invalid agent id", runtimeservice.ErrInvalidInput)
	}
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil || isSystemManagedAgent(ag) {
		return fmt.Errorf("%w: agent not found", runtimeservice.ErrNotFound)
	}
	if err := s.ensureCanManageAgent(ctx, ag, accountID, agentManageGatePermissionCodes(ag.AgentsType)...); err != nil {
		if strings.EqualFold(err.Error(), "permission denied") {
			return runtimeservice.ErrPermissionDenied
		}
		return err
	}
	return nil
}

func accountIDFromContext(ctx context.Context) string {
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func callerOrganizationIDFromContext(ctx context.Context) string {
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func (s *agentsService) checkWorkspacePermission(ctx context.Context, workspaceID, accountID string, permission model.WorkspacePermissionCode) (bool, bool, error) {
	return s.checkWorkspaceAnyPermission(ctx, workspaceID, accountID, permission)
}

func (s *agentsService) checkWorkspaceAnyPermission(ctx context.Context, workspaceID, accountID string, permissions ...model.WorkspacePermissionCode) (bool, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if workspaceID == "" || accountID == "" {
		return true, true, nil
	}
	if len(permissions) == 0 {
		return true, false, nil
	}
	if s.enterpriseService == nil {
		return false, false, nil
	}
	organizationID := callerOrganizationIDFromContext(ctx)
	if organizationID == "" {
		organizationID = s.organizationIDForAgentWorkspace(ctx, workspaceID)
	}
	if organizationID == "" {
		organizationID = workspaceID
	}
	allowed, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, permissions...)
	return true, allowed, err
}

func (s *agentsService) ensureWorkspacePermission(ctx context.Context, workspaceID, accountID string, permission model.WorkspacePermissionCode) error {
	checked, allowed, err := s.checkWorkspacePermission(ctx, workspaceID, accountID, permission)
	if err != nil {
		return fmt.Errorf("failed to verify permissions")
	}
	if checked && !allowed {
		return fmt.Errorf("permission denied")
	}
	return nil
}

func (s *agentsService) ensureCanManageAgent(ctx context.Context, ag *Agent, accountID string, permissionCodes ...model.WorkspacePermissionCode) error {
	if len(permissionCodes) == 0 {
		permissionCodes = agentUpdatePermissionCodes(ag.AgentsType)
	}
	permissionCodes = append([]model.WorkspacePermissionCode(nil), permissionCodes...)

	creatorID := ""
	if ag.CreatedBy != nil {
		creatorID = ag.CreatedBy.String()
	}

	canManage := false
	var err error
	checked, allowed, err := s.checkWorkspaceAnyPermission(ctx, ag.TenantID.String(), accountID, permissionCodes...)
	if checked {
		if err != nil {
			logger.Error(fmt.Sprintf("ensureCanManageAgent: failed to check workspace permission for agent %s, account %s", ag.ID.String(), accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
		canManage = allowed
	} else if s.resourcePermissionService != nil {
		canManage, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID:       accountID,
			TenantID:        ag.TenantID.String(),
			OrganizationID:  callerOrganizationIDFromContext(ctx),
			CreatedBy:       creatorID,
			GroupID:         nil,
			PermissionCodes: permissionCodes,
		})
		if err != nil {
			logger.Error(fmt.Sprintf("ensureCanManageAgent: failed to check edit permission for agent %s, account %s", ag.ID.String(), accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if creatorID != "" && strings.EqualFold(creatorID, accountID) {
		canManage = true
	}
	if !canManage {
		return fmt.Errorf("permission denied")
	}

	return nil
}

func (s *agentsService) ensureCanManageAgentRuntimeSurfaces(ctx context.Context, ag *Agent, accountID string) error {
	return s.ensureCanManageAgent(ctx, ag, accountID, agentRuntimeAccessManagePermissionCodes(ag.AgentsType)...)
}
