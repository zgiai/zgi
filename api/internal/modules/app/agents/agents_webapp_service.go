package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
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

	if err := s.ensureCanManageAgent(ctx, ag, accountID); err != nil {
		return nil, err
	}

	if err := s.agentsRepo.UpdateWebAppStatus(ctx, agentID, status, reason, accountID); err != nil {
		return nil, err
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

func (s *agentsService) ensureCanManageAgent(ctx context.Context, ag *Agent, accountID string) error {
	creatorID := ""
	if ag.CreatedBy != nil {
		creatorID = ag.CreatedBy.String()
	}

	canManage := false
	var err error
	callerOrganizationID := callerOrganizationIDFromContext(ctx)
	if callerOrganizationID != "" && s.enterpriseService != nil {
		canManage, err = s.enterpriseService.CheckWorkspacePermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("ensureCanManageAgent: failed to check workspace permission for agent %s, account %s", ag.ID.String(), accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canManage, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID: accountID,
			TenantID:  ag.TenantID.String(),
			CreatedBy: creatorID,
			GroupID:   nil,
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

	if s.accountService == nil {
		return nil
	}
	isEditor, err := s.accountService.IsEditor(ctx, accountID)
	if err != nil {
		return err
	}
	if !isEditor {
		return fmt.Errorf("permission denied")
	}

	return nil
}
