package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type agentListAssetKind string

const (
	agentListAssetKindAll      agentListAssetKind = ""
	agentListAssetKindAgent    agentListAssetKind = "agent"
	agentListAssetKindWorkflow agentListAssetKind = "workflow"
)

var errInvalidAgentListAssetKind = errors.New("invalid agent asset_kind")

func resolveAgentListAssetKind(req dto.GetAgentsListRequest) (agentListAssetKind, error) {
	switch strings.ToLower(strings.TrimSpace(req.AssetKind)) {
	case "":
		if strings.TrimSpace(req.AgentType) == "" {
			return agentListAssetKindAll, nil
		}
		if isWorkflowRuntimePermissionType(req.AgentType) {
			return agentListAssetKindWorkflow, nil
		}
		if normalizeMode(req.AgentType) == "AGENT" {
			return agentListAssetKindAgent, nil
		}
		return agentListAssetKindAll, nil
	case string(agentListAssetKindAgent):
		return agentListAssetKindAgent, nil
	case string(agentListAssetKindWorkflow):
		return agentListAssetKindWorkflow, nil
	default:
		return agentListAssetKindAll, errInvalidAgentListAssetKind
	}
}

func agentTypeFiltersForAssetKind(kind agentListAssetKind) []string {
	switch kind {
	case agentListAssetKindAgent:
		return []string{"AGENT"}
	case agentListAssetKindWorkflow:
		return []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL_AGENT"}
	default:
		return nil
	}
}

func visiblePermissionCodesForAssetKind(kind agentListAssetKind) []workspace_model.WorkspacePermissionCode {
	switch kind {
	case agentListAssetKindAgent:
		return agentVisiblePermissionCodes()
	case agentListAssetKindWorkflow:
		return workflowVisiblePermissionCodes()
	default:
		return agentAssetVisiblePermissionCodes()
	}
}

// GetAgentsListWithPermissions retrieves a paginated list of agents using the new permission system
// This method uses buildPermissionContext to determine user permissions and calls the new
// repository method GetPaginatedAgentsWithPermissions for permission-based filtering
// Requirements: 1.1, 1.2, 1.3, 11.1, 11.2, 11.3, 11.4, 11.5
func (s *agentsService) GetAgentsListWithPermissions(
	ctx context.Context,
	accountID string,
	req dto.GetAgentsListRequest,
) (*dto.AgentsListResponse, error) {
	// Requirement 11.5: Add structured logging with context
	logger.Info("GetAgentsListWithPermissions: Starting request", map[string]interface{}{
		"account_id": accountID,
		"page":       req.Page,
		"limit":      req.Limit,
		"page_size":  req.PageSize,
	})

	// Validate and normalize pagination parameters
	// Requirement 8.5: Default to page 1 if less than 1
	page := req.Page
	if page <= 0 {
		page = 1
	}
	// Requirement 8.5: Cap page at 99999
	if page > 99999 {
		page = 99999
	}

	// Handle pageSize and limit priority: pageSize > limit > default(20)
	limit := 20
	if req.PageSize > 0 {
		limit = req.PageSize
	} else if req.Limit > 0 {
		limit = req.Limit
	}
	// Requirement 8.6: Cap limit at 100
	if limit > 100 {
		limit = 100
	}
	if limit <= 0 {
		limit = 20
	}

	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsListWithPermissions: Failed to get current organization for account_id=%s", accountID), err)
		return nil, fmt.Errorf("failed to determine user permissions: %w", err)
	}
	if currentOrganization == nil || currentOrganization.OrganizationID == "" {
		logger.Error(fmt.Sprintf("GetAgentsListWithPermissions: No current organization found for account_id=%s", accountID), nil)
		return nil, fmt.Errorf("tenant not found")
	}

	// Step 1: Build filter from request parameters
	filter := AgentsFilter{}

	workspaceID := req.WorkspaceID
	if workspaceID != "" {
		filter.TenantID = strings.TrimSpace(workspaceID)
	}

	// Requirement 9.1: Apply name filter with ILIKE
	if req.Name != "" {
		name := strings.TrimSpace(req.Name)
		// Truncate long names to prevent abuse
		if len(name) > 30 {
			name = name[:30]
		}
		filter.Name = name
	}

	// Apply keyword filter to search in name and description
	if req.Keyword != "" {
		keyword := strings.TrimSpace(req.Keyword)
		// Truncate long keywords to prevent abuse
		if len(keyword) > 50 {
			keyword = keyword[:50]
		}
		filter.Keyword = keyword
	}

	// Requirement 9.2: Apply agent_type filter
	if req.AgentType != "" {
		filter.AgentsType = normalizeMode(req.AgentType)
	}

	assetKind, err := resolveAgentListAssetKind(req)
	if err != nil {
		return nil, err
	}
	if len(agentTypeFiltersForAssetKind(assetKind)) > 0 {
		filter.AgentTypes = agentTypeFiltersForAssetKind(assetKind)
	}

	// Requirement 9.3: Apply internal filter
	if req.Internal != nil {
		filter.Internal = req.Internal
	}

	// Apply created_by filter if requested
	if req.IsCreatedByMe {
		filter.CreatedBy = accountID
	}

	// Log filter details for debugging
	// Requirement 11.5: Add structured logging with context
	internalStr := "nil"
	if filter.Internal != nil {
		if *filter.Internal {
			internalStr = "true"
		} else {
			internalStr = "false"
		}
	}
	logger.Info("GetAgentsListWithPermissions: Applying filters", map[string]interface{}{
		"account_id":      accountID,
		"organization_id": currentOrganization.OrganizationID,
		"name":            filter.Name,
		"keyword":         filter.Keyword,
		"agent_type":      filter.AgentsType,
		"asset_kind":      assetKind,
		"created_by":      filter.CreatedBy,
		"internal":        internalStr,
		"page":            page,
		"limit":           limit,
	})

	visibleWorkspaceIDs, err := shared_visibility.ResolveVisibleWorkspaceIDs(
		ctx,
		s.enterpriseService,
		currentOrganization.OrganizationID,
		accountID,
		filter.TenantID,
		visiblePermissionCodesForAssetKind(assetKind)...,
	)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsListWithPermissions: Failed to resolve visible workspaces for account_id=%s, org_id=%s",
			accountID, currentOrganization.OrganizationID), err)
		return nil, fmt.Errorf("failed to determine user permissions: %w", err)
	}

	if len(visibleWorkspaceIDs) == 0 {
		return &dto.AgentsListResponse{
			Page:    page,
			Limit:   limit,
			Total:   0,
			HasMore: false,
			Data:    []dto.AgentListItem{},
		}, nil
	}

	// Step 2: Query agents only from visible workspaces
	list, total, err := s.agentsRepo.GetPaginatedAgentsMultipleTenants(ctx, visibleWorkspaceIDs, filter, page, limit)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsListWithPermissions: Repository query failed for account_id=%s, org_id=%s, workspace_ids=%v",
			accountID, currentOrganization.OrganizationID, visibleWorkspaceIDs), err)
		return nil, fmt.Errorf("failed to retrieve agents: %w", err)
	}

	logger.Info("GetAgentsListWithPermissions: Repository query successful", map[string]interface{}{
		"account_id":      accountID,
		"organization_id": currentOrganization.OrganizationID,
		"total":           total,
		"list_count":      len(list),
	})

	// Step 4: Check edit permissions for all agents using batch permission service
	permissionResources := make([]interfaces.ResourcePermissionInfo, 0, len(list))
	for _, a := range list {
		var createdBy string
		if a.CreatedBy != nil {
			createdBy = a.CreatedBy.String()
		}

		permissionResources = append(permissionResources, interfaces.ResourcePermissionInfo{
			ResourceID:      a.ID.String(),
			WorkspaceID:     a.TenantID.String(),
			OrganizationID:  currentOrganization.OrganizationID,
			CreatedBy:       createdBy,
			GroupID:         nil, // Agents don't have group_id, only tenant_id
			PermissionCodes: agentUpdatePermissionCodes(a.AgentsType),
		})
	}

	// Call batch permission check
	permissionResults, err := s.resourcePermissionService.CheckBatchResourceEditPermission(ctx, interfaces.BatchResourcePermissionParams{
		AccountID: accountID,
		Resources: permissionResources,
	})
	if err != nil {
		// Log error but continue with can_edit=false for all
		logger.Error(fmt.Sprintf("GetAgentsListWithPermissions: Failed to check permissions for account_id=%s", accountID), err)
		permissionResults = make(map[string]bool)
	}

	// Step 5: Transform repository results to DTOs with can_edit field
	items := make([]dto.AgentListItem, 0, len(list))
	for _, a := range list {
		// Convert created_by UUID to string
		var createdByPtr *string
		if a.CreatedBy != nil {
			createdBy := a.CreatedBy.String()
			createdByPtr = &createdBy
		}

		isPublished := false
		if hasPublished, err := s.hasPublishedVersion(ctx, &a); err == nil {
			isPublished = hasPublished
		} else {
			// Requirement 11.5: Add structured logging with context
			logger.Warn("GetAgentsListWithPermissions: Failed to check published workflow", map[string]interface{}{
				"account_id": accountID,
				"agent_id":   a.ID.String(),
				"error":      err.Error(),
			})
			// Continue with isPublished=false, don't fail the entire operation
		}

		// Get can_edit from permission results
		canEdit := permissionResults[a.ID.String()]

		// Process icon for base64 or image type icons
		icon := a.Icon
		iconType := a.IconType
		iconUrl := ""
		if iconType != nil && *iconType == "base64" && icon != nil && strings.HasPrefix(*icon, "data:image") {
			convertedIcon, convertedType := convertBase64IconToText(a.Name, *icon)
			icon = &convertedIcon
			iconType = &convertedType
		} else if iconType != nil && *iconType == "image" && icon != nil && *icon != "" {
			fileURL, err := s.fileService.GetFileURL(ctx, *icon)
			if err != nil {
				logger.Warn(fmt.Sprintf("failed to get file URL for icon %s: %v", *icon, err))
			} else {
				iconUrl = fileURL
			}
		}

		items = append(items, dto.AgentListItem{
			ID:           a.ID.String(),
			Name:         a.Name,
			Description:  a.Description,
			AgentType:    a.AgentsType,
			TenantID:     a.TenantID.String(),
			WorkspaceID:  a.TenantID.String(),
			IconType:     iconType,
			Icon:         icon,
			IconUrl:      iconUrl,
			IsPublic:     a.IsPublic,
			IsPublished:  isPublished,
			WebAppStatus: string(NormalizeAgentWebAppStatus(a.WebAppStatus)),
			CreatedBy:    createdByPtr,
			CreatedAt:    a.CreatedAt.Unix(),
			UpdatedAt:    a.UpdatedAt.Unix(),
			Internal:     a.Internal,
			CanEdit:      canEdit,
		})
	}

	// Step 6: Build response with pagination info
	hasMore := int64(page*limit) < total
	response := &dto.AgentsListResponse{
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
		Data:    items,
	}

	return response, nil
}
