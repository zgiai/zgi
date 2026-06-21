package APIKey

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

type apiKeyAgentWorkspaceResolver interface {
	ResolveAgentWorkspace(ctx context.Context, organizationID string, agentID uuid.UUID) (string, error)
}

type apiKeyWorkspacePermissionChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error)
}

type dbAPIKeyAgentWorkspaceResolver struct {
	db *gorm.DB
}

func newDBAPIKeyAgentWorkspaceResolver(db *gorm.DB) apiKeyAgentWorkspaceResolver {
	if db == nil {
		return nil
	}
	return &dbAPIKeyAgentWorkspaceResolver{db: db}
}

func (r *dbAPIKeyAgentWorkspaceResolver) ResolveAgentWorkspace(ctx context.Context, organizationID string, agentID uuid.UUID) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("api key agent workspace resolver is not configured")
	}

	var row struct {
		WorkspaceID string `gorm:"column:workspace_id"`
	}
	err := r.db.WithContext(ctx).
		Table("agents").
		Select("agents.tenant_id AS workspace_id").
		Joins("JOIN workspaces ON workspaces.id = agents.tenant_id").
		Where("agents.id = ? AND agents.deleted_at IS NULL AND workspaces.organization_id = ?", agentID, organizationID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return row.WorkspaceID, nil
}

func (h *APIKeyHandler) requireAgentAPIKeyAccess(c *gin.Context, agentID uuid.UUID, permissionCode workspace_model.WorkspacePermissionCode) (uuid.UUID, bool) {
	accountID := strings.TrimSpace(c.GetString("account_id"))
	organizationID := strings.TrimSpace(util.GetOrganizationIDCompat(c))
	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return uuid.Nil, false
	}
	if h.organizationService == nil || h.agentWorkspaceResolver == nil {
		response.Fail(c, response.ErrPermissionDenied)
		return uuid.Nil, false
	}

	workspaceID, err := h.agentWorkspaceResolver.ResolveAgentWorkspace(c.Request.Context(), organizationID, agentID)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to resolve api key agent workspace", "agent_id", agentID.String(), err)
		response.Fail(c, response.ErrSystemError)
		return uuid.Nil, false
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		response.Fail(c, response.ErrPermissionDenied)
		return uuid.Nil, false
	}

	hasPermission, err := h.organizationService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		permissionCode,
	)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to check api key workspace permission", "agent_id", agentID.String(), "workspace_id", workspaceID, err)
		response.Fail(c, response.ErrSystemError)
		return uuid.Nil, false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return uuid.Nil, false
	}

	workspaceUUID, err := uuid.Parse(workspaceID)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "invalid api key agent workspace id", "agent_id", agentID.String(), "workspace_id", workspaceID, err)
		response.Fail(c, response.ErrSystemError)
		return uuid.Nil, false
	}

	return workspaceUUID, true
}
