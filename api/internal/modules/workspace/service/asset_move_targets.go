package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

func (s *WorkspaceAssetMoveService) EligibleTargets(ctx context.Context, organizationID, accountID string, req dto.WorkspaceAssetMoveEligibleTargetsRequest) (*dto.WorkspaceAssetMoveEligibleTargetsResponse, error) {
	organizationID = strings.TrimSpace(organizationID)
	accountID = strings.TrimSpace(accountID)
	if organizationID == "" || accountID == "" || len(req.Items) == 0 || s.authorizationService == nil {
		return nil, ErrAssetMoveInvalidRequest
	}

	if _, err := s.authorizationService.RequireOrganizationMember(ctx, interfaces.OrganizationScopeRequest{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}); err != nil {
		return nil, assetMoveAuthorizationError(err)
	}

	requiredPermissions := make(map[workspace_model.WorkspacePermissionCode]struct{})
	sourceWorkspaceIDs := make(map[string]struct{})
	checkedSources := make(map[string]struct{})
	seenItems := make(map[string]struct{}, len(req.Items))
	for _, rawItem := range req.Items {
		item := dto.WorkspaceAssetMoveItem{
			Type: strings.ToLower(strings.TrimSpace(rawItem.Type)),
			ID:   strings.TrimSpace(rawItem.ID),
		}
		if item.Type == "" || item.ID == "" {
			return nil, ErrAssetMoveInvalidRequest
		}
		itemKey := item.Type + ":" + item.ID
		if _, exists := seenItems[itemKey]; exists {
			return nil, ErrAssetMoveInvalidRequest
		}
		seenItems[itemKey] = struct{}{}

		scope, err := s.resolveAssetMoveScope(ctx, s.db, organizationID, item)
		if err != nil {
			return nil, err
		}
		requiredPermissions[scope.permission] = struct{}{}
		if scope.sourceWorkspaceID == "" {
			continue
		}
		sourceWorkspaceIDs[scope.sourceWorkspaceID] = struct{}{}
		checkKey := scope.sourceWorkspaceID + ":" + string(scope.permission)
		if _, exists := checkedSources[checkKey]; exists {
			continue
		}
		if err := s.requireAssetMovePermission(ctx, organizationID, scope.sourceWorkspaceID, accountID, scope.permission); err != nil {
			return nil, err
		}
		checkedSources[checkKey] = struct{}{}
	}

	eligibleWorkspaceIDs, err := s.intersectEligibleWorkspaceIDs(ctx, organizationID, accountID, requiredPermissions)
	if err != nil {
		return nil, err
	}
	for workspaceID := range sourceWorkspaceIDs {
		delete(eligibleWorkspaceIDs, workspaceID)
	}
	return s.listEligibleWorkspaceTargets(ctx, organizationID, eligibleWorkspaceIDs, req)
}

func (s *WorkspaceAssetMoveService) intersectEligibleWorkspaceIDs(
	ctx context.Context,
	organizationID, accountID string,
	requiredPermissions map[workspace_model.WorkspacePermissionCode]struct{},
) (map[string]struct{}, error) {
	var eligibleWorkspaceIDs map[string]struct{}
	for permission := range requiredPermissions {
		workspaceIDs, err := s.authorizationService.ListWorkspaceIDsByPermission(ctx, interfaces.WorkspaceListPermissionRequest{
			OrganizationID: organizationID,
			AccountID:      accountID,
			PermissionCode: permission,
		})
		if err != nil {
			return nil, assetMoveAuthorizationError(err)
		}
		permissionWorkspaceIDs := make(map[string]struct{}, len(workspaceIDs))
		for _, workspaceID := range workspaceIDs {
			workspaceID = strings.TrimSpace(workspaceID)
			if workspaceID != "" {
				permissionWorkspaceIDs[workspaceID] = struct{}{}
			}
		}
		if eligibleWorkspaceIDs == nil {
			eligibleWorkspaceIDs = permissionWorkspaceIDs
			continue
		}
		for workspaceID := range eligibleWorkspaceIDs {
			if _, allowed := permissionWorkspaceIDs[workspaceID]; !allowed {
				delete(eligibleWorkspaceIDs, workspaceID)
			}
		}
	}
	return eligibleWorkspaceIDs, nil
}

func (s *WorkspaceAssetMoveService) listEligibleWorkspaceTargets(
	ctx context.Context,
	organizationID string,
	eligibleWorkspaceIDs map[string]struct{},
	req dto.WorkspaceAssetMoveEligibleTargetsRequest,
) (*dto.WorkspaceAssetMoveEligibleTargetsResponse, error) {
	page, limit := normalizeAssetMoveTargetPage(req.Page, req.Limit)
	response := &dto.WorkspaceAssetMoveEligibleTargetsResponse{
		Data:  []dto.WorkspaceAssetMoveWorkspace{},
		Page:  page,
		Limit: limit,
	}
	if len(eligibleWorkspaceIDs) == 0 {
		return response, nil
	}

	ids := make([]string, 0, len(eligibleWorkspaceIDs))
	for workspaceID := range eligibleWorkspaceIDs {
		ids = append(ids, workspaceID)
	}
	query := s.db.WithContext(ctx).
		Model(&workspace_model.Workspace{}).
		Where("organization_id = ?", organizationID).
		Where("status = ?", workspace_model.WorkspaceStatusNormal).
		Where("id IN ?", ids)
	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}
	if err := query.Count(&response.Total).Error; err != nil {
		return nil, fmt.Errorf("count eligible workspace asset move targets: %w", err)
	}
	if response.Total == 0 {
		return response, nil
	}
	if err := query.
		Select("id, name").
		Order("LOWER(name) ASC").
		Order("id ASC").
		Offset((page - 1) * limit).
		Limit(limit).
		Scan(&response.Data).Error; err != nil {
		return nil, fmt.Errorf("list eligible workspace asset move targets: %w", err)
	}
	response.HasMore = int64(page*limit) < response.Total
	return response, nil
}

type assetMoveScope struct {
	sourceWorkspaceID string
	permission        workspace_model.WorkspacePermissionCode
	resolvedAgentType string
}

func (s *WorkspaceAssetMoveService) resolveAssetMoveScope(ctx context.Context, db *gorm.DB, organizationID string, item dto.WorkspaceAssetMoveItem) (assetMoveScope, error) {
	permission, supported := assetMovePermissionForType(item.Type)
	if !supported {
		return assetMoveScope{}, ErrAssetMoveInvalidRequest
	}

	scope := assetMoveScope{permission: permission}
	switch item.Type {
	case AssetMoveTypeAgent:
		var agent struct {
			TenantID  string
			AgentType string
		}
		err := db.WithContext(ctx).
			Table("agents").
			Select("tenant_id, agent_type").
			Where("id = ? AND deleted_at IS NULL", item.ID).
			Take(&agent).Error
		if err != nil {
			return assetMoveScope{}, assetMoveScopeLookupError(err)
		}
		scope.sourceWorkspaceID = strings.TrimSpace(agent.TenantID)
		scope.resolvedAgentType = strings.TrimSpace(agent.AgentType)
		scope.permission = agentMovePermissionForType(agent.AgentType)
	case AssetMoveTypeDataset:
		var dataset struct {
			OrganizationID string
			WorkspaceID    string
		}
		err := db.WithContext(ctx).
			Table("datasets").
			Select("organization_id, workspace_id").
			Where("id = ?", item.ID).
			Take(&dataset).Error
		if err != nil {
			return assetMoveScope{}, assetMoveScopeLookupError(err)
		}
		if dataset.OrganizationID != organizationID {
			return assetMoveScope{}, ErrAssetMovePermissionDenied
		}
		scope.sourceWorkspaceID = strings.TrimSpace(dataset.WorkspaceID)
	case AssetMoveTypeFile:
		var file struct {
			OrganizationID string
			WorkspaceID    *string
		}
		err := db.WithContext(ctx).
			Table("upload_files").
			Select("organization_id, workspace_id").
			Where("id = ?", item.ID).
			Take(&file).Error
		if err != nil {
			return assetMoveScope{}, assetMoveScopeLookupError(err)
		}
		if file.OrganizationID != organizationID {
			return assetMoveScope{}, ErrAssetMovePermissionDenied
		}
		if file.WorkspaceID != nil {
			scope.sourceWorkspaceID = strings.TrimSpace(*file.WorkspaceID)
		}
	case AssetMoveTypeDatabase:
		var dataSource struct {
			OrganizationID string
			WorkspaceID    *string
		}
		err := db.WithContext(ctx).
			Table("data_sources").
			Select("organization_id, workspace_id").
			Where("id = ?", item.ID).
			Take(&dataSource).Error
		if err != nil {
			return assetMoveScope{}, assetMoveScopeLookupError(err)
		}
		if dataSource.OrganizationID != organizationID {
			return assetMoveScope{}, ErrAssetMovePermissionDenied
		}
		if dataSource.WorkspaceID != nil {
			scope.sourceWorkspaceID = strings.TrimSpace(*dataSource.WorkspaceID)
		}
	}
	return scope, nil
}

func assetMoveScopeLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAssetMoveInvalidRequest
	}
	return err
}

func normalizeAssetMoveTargetPage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 100
	}
	return page, limit
}
