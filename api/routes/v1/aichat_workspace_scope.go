package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

var errAIChatWorkspaceScopeUnavailable = errors.New("AIChat workspace scope is unavailable")

type aichatCurrentWorkspaceProvider interface {
	GetCurrentWorkspace(ctx context.Context, accountID string) (*model.Workspace, error)
}

type aichatWorkspaceProvider interface {
	GetWorkspace(ctx context.Context, workspaceID uuid.UUID) (*model.Workspace, error)
	IsWorkspaceMember(ctx context.Context, workspaceID, accountID uuid.UUID) (bool, error)
}

type gormAIChatWorkspaceProvider struct {
	db *gorm.DB
}

func (provider gormAIChatWorkspaceProvider) GetWorkspace(ctx context.Context, workspaceID uuid.UUID) (*model.Workspace, error) {
	if provider.db == nil {
		return nil, errors.New("AIChat workspace database is unavailable")
	}
	var workspace model.Workspace
	if err := provider.db.WithContext(ctx).Where("id = ?", workspaceID.String()).Take(&workspace).Error; err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (provider gormAIChatWorkspaceProvider) IsWorkspaceMember(ctx context.Context, workspaceID, accountID uuid.UUID) (bool, error) {
	if provider.db == nil {
		return false, errors.New("AIChat workspace database is unavailable")
	}
	var count int64
	if err := provider.db.WithContext(ctx).
		Model(&model.WorkspaceMember{}).
		Where("workspace_id = ? AND account_id = ?", workspaceID.String(), accountID.String()).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type aichatWorkspaceScopeResolver struct {
	workspaces aichatWorkspaceProvider
	accounts   aichatCurrentWorkspaceProvider
}

// aichatWorkspaceScopeMiddleware establishes one canonical workspace scope for
// every AIChat endpoint. In particular, connection preferences and later tool
// execution must not independently fall back to different workspace values.
func aichatWorkspaceScopeMiddleware(db *gorm.DB, accounts aichatCurrentWorkspaceProvider) gin.HandlerFunc {
	resolver := aichatWorkspaceScopeResolver{
		workspaces: gormAIChatWorkspaceProvider{db: db},
		accounts:   accounts,
	}
	return aichatWorkspaceScopeMiddlewareWithResolver(resolver)
}

func aichatWorkspaceScopeMiddlewareWithResolver(resolver aichatWorkspaceScopeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, organizationErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
		accountID, accountErr := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
		if organizationErr != nil || organizationID == uuid.Nil || accountErr != nil || accountID == uuid.Nil {
			response.Fail(c, response.ErrUnauthorized)
			c.Abort()
			return
		}

		workspaceID, err := resolver.Resolve(
			c.Request.Context(),
			organizationID,
			accountID,
			util.GetWorkspaceID(c),
		)
		if err != nil {
			writeAIChatWorkspaceScopeError(c, err)
			c.Abort()
			return
		}
		if workspaceID != nil {
			util.SetWorkspaceID(c, workspaceID.String())
		}
		c.Next()
	}
}

func writeAIChatWorkspaceScopeError(c *gin.Context, err error) {
	if errors.Is(err, errAIChatWorkspaceScopeUnavailable) {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	logger.ErrorContext(c.Request.Context(), "failed to resolve AIChat workspace scope", err)
	response.Fail(c, response.ErrSystemError)
}

// Resolve prefers an already-established workspace scope. When none exists it
// resolves the account's current workspace. Both paths validate the same
// organization and lifecycle invariants before exposing the workspace to
// AIChat runtime code.
func (resolver aichatWorkspaceScopeResolver) Resolve(
	ctx context.Context,
	organizationID uuid.UUID,
	accountID uuid.UUID,
	explicitWorkspaceID string,
) (*uuid.UUID, error) {
	explicitWorkspaceID = strings.TrimSpace(explicitWorkspaceID)
	if explicitWorkspaceID != "" {
		workspaceID, err := uuid.Parse(explicitWorkspaceID)
		if err != nil || workspaceID == uuid.Nil {
			return nil, fmt.Errorf("%w: workspace ID is invalid", errAIChatWorkspaceScopeUnavailable)
		}
		if resolver.workspaces == nil {
			return nil, errors.New("AIChat workspace provider is unavailable")
		}

		workspace, err := resolver.workspaces.GetWorkspace(ctx, workspaceID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: workspace was not found", errAIChatWorkspaceScopeUnavailable)
		}
		if err != nil {
			return nil, fmt.Errorf("load explicit AIChat workspace: %w", err)
		}
		if workspace == nil {
			return nil, fmt.Errorf("%w: workspace was not found", errAIChatWorkspaceScopeUnavailable)
		}
		resolvedID, err := validateAIChatWorkspaceScope(workspace, organizationID)
		if err != nil {
			return nil, err
		}
		return resolver.requireWorkspaceMembership(ctx, *resolvedID, accountID)
	}

	if resolver.accounts == nil {
		return nil, errors.New("AIChat current workspace provider is unavailable")
	}
	workspace, err := resolver.accounts.GetCurrentWorkspace(ctx, accountID.String())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: current workspace was not found", errAIChatWorkspaceScopeUnavailable)
	}
	if err != nil {
		return nil, fmt.Errorf("load current AIChat workspace: %w", err)
	}
	if workspace == nil {
		// Preserve organization-mode AIChat semantics for accounts without a
		// current workspace. Such requests continue with WorkspaceID == nil.
		return nil, nil
	}
	resolvedID, err := validateAIChatWorkspaceScope(workspace, organizationID)
	if err != nil {
		return nil, err
	}
	return resolver.requireWorkspaceMembership(ctx, *resolvedID, accountID)
}

func (resolver aichatWorkspaceScopeResolver) requireWorkspaceMembership(ctx context.Context, workspaceID, accountID uuid.UUID) (*uuid.UUID, error) {
	if resolver.workspaces == nil {
		return nil, errors.New("AIChat workspace provider is unavailable")
	}
	member, err := resolver.workspaces.IsWorkspaceMember(ctx, workspaceID, accountID)
	if err != nil {
		return nil, fmt.Errorf("validate AIChat workspace membership: %w", err)
	}
	if !member {
		return nil, fmt.Errorf("%w: account is not a workspace member", errAIChatWorkspaceScopeUnavailable)
	}
	return &workspaceID, nil
}

func validateAIChatWorkspaceScope(workspace *model.Workspace, organizationID uuid.UUID) (*uuid.UUID, error) {
	if workspace == nil {
		return nil, fmt.Errorf("%w: workspace is missing", errAIChatWorkspaceScopeUnavailable)
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(workspace.ID))
	if err != nil || workspaceID == uuid.Nil {
		return nil, fmt.Errorf("%w: workspace ID is invalid", errAIChatWorkspaceScopeUnavailable)
	}
	if workspace.Status != model.WorkspaceStatusNormal {
		return nil, fmt.Errorf("%w: workspace is not active", errAIChatWorkspaceScopeUnavailable)
	}
	if workspace.OrganizationID == nil || !strings.EqualFold(strings.TrimSpace(*workspace.OrganizationID), organizationID.String()) {
		return nil, fmt.Errorf("%w: workspace does not belong to the active organization", errAIChatWorkspaceScopeUnavailable)
	}
	return &workspaceID, nil
}
