package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AgentsService = interfaces.AgentsService

var errCurrentOrganizationNotFound = errors.New("current organization not found")

type textIcon struct {
	Icon           string `json:"icon"`
	IconBackground string `json:"icon_background"`
}

func convertBase64IconToText(name string, base64Icon string) (icon string, iconType string) {
	if !strings.HasPrefix(base64Icon, "data:image") {
		return base64Icon, "base64"
	}

	namePrefix := getNamePrefix(name)
	tIcon := textIcon{
		Icon:           namePrefix,
		IconBackground: "#000000",
	}
	iconJSON, _ := json.Marshal(tIcon)
	return string(iconJSON), "text"
}

func getNamePrefix(name string) string {
	name = strings.TrimSpace(name)
	runeCount := 0
	for range name {
		runeCount++
	}

	if runeCount >= 2 {
		runes := []rune(name)
		return string(runes[0]) + string(runes[1])
	} else if runeCount == 1 {
		runes := []rune(name)
		return string(runes[0])
	}
	return "?"
}

type agentsService struct {
	agentsRepo                AgentsRepository
	accountService            interfaces.AccountService
	tenantService             interfaces.WorkspaceManagementService
	workflowService           interfaces.WorkflowService
	chatRuntimeService        runtimeservice.Service
	resourcePermissionService interfaces.ResourcePermissionService
	enterpriseService         interfaces.OrganizationService
	quotaService              interfaces.QuotaService
	fileService               interfaces.FileService
	db                        *gorm.DB
}

func NewAgentsService(
	agentsRepo AgentsRepository,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	workflowService interfaces.WorkflowService,
	chatRuntimeService runtimeservice.Service,
	resourcePermissionService interfaces.ResourcePermissionService,
	enterpriseService interfaces.OrganizationService,
	quotaService interfaces.QuotaService,
	fileService interfaces.FileService,
	db *gorm.DB,
) AgentsService {
	return &agentsService{
		agentsRepo:                agentsRepo,
		accountService:            accountService,
		tenantService:             tenantService,
		workflowService:           workflowService,
		chatRuntimeService:        chatRuntimeService,
		resourcePermissionService: resourcePermissionService,
		enterpriseService:         enterpriseService,
		quotaService:              quotaService,
		fileService:               fileService,
		db:                        db,
	}
}

func (s *agentsService) GetRunnableWebApps(ctx context.Context, accountID string, req dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error) {
	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}
	if currentOrganization == nil || currentOrganization.OrganizationID == "" {
		return nil, errCurrentOrganizationNotFound
	}

	workspaces, err := s.enterpriseService.GetOrganizationWorkspacesList(ctx, currentOrganization.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization workspaces: %w", err)
	}

	resp := &dto.RunnableWebAppsResponse{
		Items: make([]dto.RunnableWebAppItem, 0),
	}

	workspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace == nil || workspace.Status != model.WorkspaceStatusNormal {
			continue
		}
		workspaceIDs = append(workspaceIDs, workspace.ID)
	}
	if len(workspaceIDs) == 0 {
		return resp, nil
	}

	if req.WorkspaceID != "" && !slices.Contains(workspaceIDs, req.WorkspaceID) {
		return resp, nil
	}

	items, err := s.agentsRepo.ListRunnableWebApps(ctx, workspaceIDs, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list runnable web apps: %w", err)
	}

	resp.Items = make([]dto.RunnableWebAppItem, 0, len(items))
	for _, item := range items {
		icon := item.AgentIcon
		iconType := item.AgentIconType
		iconUrl := ""

		if iconType != nil && *iconType == "base64" && icon != nil && strings.HasPrefix(*icon, "data:image") {
			convertedIcon, convertedType := convertBase64IconToText(item.AgentName, *icon)
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

		resp.Items = append(resp.Items, dto.RunnableWebAppItem{
			AgentID:      item.AgentID,
			WorkspaceID:  item.WorkspaceID,
			WebAppID:     item.WebAppID,
			WebAppStatus: string(NormalizeAgentWebAppStatus(AgentWebAppStatus(item.WebAppStatus))),
			MetaData: dto.RunnableWebAppMetaData{
				Name:      item.AgentName,
				Icon:      icon,
				IconType:  iconType,
				IconUrl:   iconUrl,
				Desc:      item.AgentDesc,
				AgentType: item.AgentType,
			},
		})
	}

	return resp, nil
}

// getUserRoleInfo retrieves user role information for RBAC filtering
// It queries members, workspace_members, and enterprise_group_tenant_joins
// to determine if the user is an organization admin, department admin, or regular member
// Requirement 9.3: Return 500 for getUserRoleInfo failures with logging
func (s *agentsService) getUserRoleInfo(ctx context.Context, accountID string) (*UserRoleInfo, error) {
	roleInfo := &UserRoleInfo{
		OrganizationIDs: []string{},
		DepartmentIDs:   []string{},
	}

	// 1. Check if user is organization admin
	var orgAdminJoins []model.OrganizationMember
	if err := s.db.WithContext(ctx).
		Where("account_id = ? AND role IN ?", accountID, []string{"owner", "admin"}).
		Find(&orgAdminJoins).Error; err != nil {
		logger.Error(fmt.Sprintf("getUserRoleInfo: Failed to query organization admin roles for account %s: %v", accountID, err), err)
		// Continue execution, don't fail the entire operation for org admin check
		// User might not be an org admin, which is fine
	} else {
		for _, join := range orgAdminJoins {
			roleInfo.OrganizationIDs = append(roleInfo.OrganizationIDs, join.OrganizationID)
		}
		roleInfo.IsOrgAdmin = len(roleInfo.OrganizationIDs) > 0
	}

	// 2. Get user's department roles - this is critical, so we fail if it errors
	var tenantJoins []model.WorkspaceMember
	if err := s.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Find(&tenantJoins).Error; err != nil {
		logger.Error(fmt.Sprintf("getUserRoleInfo: Failed to query tenant roles for account %s: %v", accountID, err), err)
		return nil, fmt.Errorf("failed to get user role info: database error querying tenant memberships: %w", err)
	}

	// Process tenant joins to extract department IDs, current department, and admin status
	for _, join := range tenantJoins {
		roleInfo.DepartmentIDs = append(roleInfo.DepartmentIDs, join.WorkspaceID)

		// Check if user is department admin
		if join.Role == model.WorkspaceRoleOwner || join.Role == model.WorkspaceRoleAdmin {
			roleInfo.IsDeptAdmin = true
		}

		// Set current department
		if join.Current {
			roleInfo.CurrentDepartment = join.WorkspaceID
		}
	}

	// 3. Get organizations that user's departments belong to (if not already org admin)
	if !roleInfo.IsOrgAdmin && len(roleInfo.DepartmentIDs) > 0 {
		var workspaces []struct {
			OrganizationID string `gorm:"column:organization_id"`
		}
		if err := s.db.WithContext(ctx).Table("workspaces").
			Select("organization_id").
			Where("id IN ? AND organization_id IS NOT NULL", roleInfo.DepartmentIDs).
			Find(&workspaces).Error; err != nil {
			logger.Error(fmt.Sprintf("getUserRoleInfo: Failed to query workspace organizations for account %s: %v", accountID, err), err)
			// Continue execution, user may not belong to any organization
			// This is not a critical error
		} else {
			// Use a map to deduplicate organization IDs
			orgMap := make(map[string]bool)
			for _, ws := range workspaces {
				if ws.OrganizationID != "" {
					orgMap[ws.OrganizationID] = true
				}
			}
			for orgID := range orgMap {
				roleInfo.OrganizationIDs = append(roleInfo.OrganizationIDs, orgID)
			}
		}
	}

	logger.Info("getUserRoleInfo: User role info for account %s: IsOrgAdmin=%v, IsDeptAdmin=%v, OrgCount=%d, DeptCount=%d, CurrentDept=%s",
		accountID, roleInfo.IsOrgAdmin, roleInfo.IsDeptAdmin, len(roleInfo.OrganizationIDs), len(roleInfo.DepartmentIDs), roleInfo.CurrentDepartment)

	return roleInfo, nil
}

// buildPermissionContext builds a permission context for the given user
// This determines which agents the user can access based on their organization
// and department memberships
// Requirements: 1.1, 1.2, 1.4, 2.1, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 11.2, 11.3, 11.5
func (s *agentsService) buildPermissionContext(ctx context.Context, accountID string) (*PermissionContext, error) {
	permCtx := &PermissionContext{
		AccountID: accountID,
	}

	// Step 1: Get current organization (tenant with current=true)
	// Requirement 1.1: Query members WHERE account_id matches
	// Requirement 11.3: Handle database errors (500) with logging
	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get current tenant for account_id=%s", accountID), err)
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}

	// Requirement 1.3, 11.2: Return error if no current tenant found (404)
	if currentOrganization == nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: No current tenant found for account_id=%s", accountID), nil)
		return nil, fmt.Errorf("no current organization found for user")
	}

	// Requirement 1.2: Extract organization ID and role
	permCtx.OrganizationID = currentOrganization.OrganizationID
	permCtx.OrganizationRole = string(currentOrganization.Role)

	// Requirement 1.4: Get all departments belonging to the organization
	// Requirement 11.3: Handle database errors (500) with logging
	orgDeptIDs, err := s.tenantService.GetWorkspaceIDsByOrganizationID(ctx, permCtx.OrganizationID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get organization departments for account_id=%s, org_id=%s",
			accountID, permCtx.OrganizationID), err)
		return nil, fmt.Errorf("failed to get organization departments: %w", err)
	}
	permCtx.OrganizationDeptIDs = orgDeptIDs

	// Step 2: Check if user is organization admin/owner
	// Requirement 2.1: If role is owner or admin, return early with all departments
	if permCtx.OrganizationRole == "owner" || permCtx.OrganizationRole == "admin" {
		// Requirement 11.5: Add structured logging with context
		logger.Info("buildPermissionContext: User is org admin/owner", map[string]interface{}{
			"account_id":      accountID,
			"organization_id": permCtx.OrganizationID,
			"role":            permCtx.OrganizationRole,
			"dept_count":      len(orgDeptIDs),
		})
		// Org admins have access to all departments, no need to calculate intersections
		return permCtx, nil
	}

	// Step 3: For normal users, get department memberships and calculate intersection
	// Requirement 3.1: Query user's department memberships (current=false)
	// Requirement 11.3: Handle database errors (500) with logging
	userDepts, err := s.tenantService.GetUserWorkspaceMemberships(ctx, accountID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get user departments for account_id=%s, org_id=%s",
			accountID, permCtx.OrganizationID), err)
		return nil, fmt.Errorf("failed to get user department memberships: %w", err)
	}

	// Convert interface type to local type
	permCtx.UserDepartments = make([]DepartmentMembership, len(userDepts))
	for i, dept := range userDepts {
		permCtx.UserDepartments[i] = DepartmentMembership{
			TenantID: dept.WorkspaceID,
			Role:     string(dept.Role),
		}
	}

	// Requirement 3.2, 3.3: Calculate intersection of organization departments and user departments
	orgDeptSet := make(map[string]bool)
	for _, deptID := range orgDeptIDs {
		orgDeptSet[deptID] = true
	}

	// Build valid department IDs (intersection) and separate by role
	// Requirement 4.3: Check department roles for each valid department
	for _, userDept := range permCtx.UserDepartments {
		// Check if user's department is in the organization
		if orgDeptSet[userDept.TenantID] {
			permCtx.ValidDepartmentIDs = append(permCtx.ValidDepartmentIDs, userDept.TenantID)

			// Requirement 4.1, 4.2: Separate departments by role
			if userDept.Role == "owner" || userDept.Role == "admin" {
				permCtx.AdminDepartmentIDs = append(permCtx.AdminDepartmentIDs, userDept.TenantID)
			} else {
				permCtx.NormalDepartmentIDs = append(permCtx.NormalDepartmentIDs, userDept.TenantID)
			}
		}
	}

	// Requirement 3.4: If user has no valid departments, they will see an empty list
	// (This is handled by the repository layer, not an error)
	// Requirement 11.5: Add structured logging with context
	logger.Info("buildPermissionContext: Permission context built for normal user", map[string]interface{}{
		"account_id":        accountID,
		"organization_id":   permCtx.OrganizationID,
		"valid_dept_count":  len(permCtx.ValidDepartmentIDs),
		"admin_dept_count":  len(permCtx.AdminDepartmentIDs),
		"normal_dept_count": len(permCtx.NormalDepartmentIDs),
		"valid_dept_ids":    permCtx.ValidDepartmentIDs,
		"admin_dept_ids":    permCtx.AdminDepartmentIDs,
		"normal_dept_ids":   permCtx.NormalDepartmentIDs,
	})

	return permCtx, nil
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
		model.WorkspacePermissionAgentView,
		model.WorkspacePermissionAgentManage,
		model.WorkspacePermissionAgentLock,
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
			ResourceID:  a.ID.String(),
			WorkspaceID: a.TenantID.String(),
			CreatedBy:   createdBy,
			GroupID:     nil, // Agents don't have group_id, only tenant_id
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

func (s *agentsService) CreateAgent(ctx context.Context, tenantID string, req interface{}, accountID string) (interface{}, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(accountID) == "" || req == nil {
		return nil, fmt.Errorf("invalid parameters")
	}

	var (
		name        string
		iconType    string
		icon        string
		agentType   string
		description string
		internal    bool
	)

	switch v := req.(type) {
	case *dto.CreateAgentRequest:
		name = v.Name
		iconType = v.IconType
		icon = v.Icon
		agentType = v.AgentType
		if agentType == "" && v.AgentType != "" {
			agentType = v.AgentType
		}
		description = v.Description
		if v.Internal != nil {
			internal = *v.Internal
		}
	case dto.CreateAgentRequest:
		name = v.Name
		iconType = v.IconType
		icon = v.Icon
		agentType = v.AgentType
		if agentType == "" && v.AgentType != "" {
			agentType = v.AgentType
		}
		description = v.Description
		if v.Internal != nil {
			internal = *v.Internal
		}
	case map[string]interface{}:
		if s2, ok2 := v["name"].(string); ok2 {
			name = s2
		}
		if s2, ok2 := v["icon_type"].(string); ok2 {
			iconType = s2
		}
		if s2, ok2 := v["icon"].(string); ok2 {
			icon = s2
		}
		if s2, ok2 := v["agentType"].(string); ok2 {
			agentType = s2
		}
		if s2, ok2 := v["agent_type"].(string); ok2 && agentType == "" {
			agentType = s2
		}
		if s2, ok2 := v["description"].(string); ok2 {
			description = s2
		}
		if b, ok := v["internal"].(bool); ok {
			internal = b
		}
	default:
		return nil, fmt.Errorf("unsupported request type")
	}

	if strings.TrimSpace(name) == "" || strings.TrimSpace(agentType) == "" {
		return nil, fmt.Errorf("name and agentType are required")
	}

	// Permission: editor can create
	isEditor, err := s.accountService.IsEditor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !isEditor {
		return nil, errors.New("permission denied")
	}

	// Duplicate name in tenant
	exists, err := s.agentsRepo.ExistsByName(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("agent with the same name already exists")
	}

	// Step 1: Get organization ID from workspace for quota checking.
	var groupID *uuid.UUID
	if s.enterpriseService != nil {
		group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, tenantID)
		if err == nil && group != nil {
			// Parse organization ID string to UUID.
			parsedGroupID, parseErr := uuid.Parse(group.ID)
			if parseErr == nil {
				groupID = &parsedGroupID
			}
		}
	}

	// Step 2: Check AI agents quota if groupID exists
	if groupID != nil && s.quotaService != nil {
		canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *groupID, "ai_agents", 1)
		if err != nil {
			return nil, fmt.Errorf("failed to check AI agents quota: %w", err)
		}

		// Step 3: If quota exceeded, return error
		if !canProceed {
			return nil, fmt.Errorf("AI智能体配额不足。当前: %d, 限制: %d", currentUsage, limit)
		}
	}

	// Build Agent model
	ag := &Agent{
		TenantID:    parseUUID(tenantID),
		Name:        name,
		Description: description,
		AgentsType:  normalizeMode(agentType),
		EnableAPI:   true,
		Internal:    internal,
	}
	if iconType != "" {
		ag.IconType = &iconType
	}
	if icon != "" {
		ag.Icon = &icon
	}

	// Generate web_app_id
	ag.WebAppID = uuid.New()

	// Set created_by
	if uid, err := uuid.Parse(accountID); err == nil {
		ag.CreatedBy = &uid
	}

	// Step 4: Create agent and record usage in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create agent
		if err := tx.WithContext(ctx).Create(ag).Error; err != nil {
			return fmt.Errorf("failed to create agent: %w", err)
		}

		// Create installed record
		inst := &InstalledAgent{TenantID: ag.TenantID, AgentID: ag.ID, AgentOwnerTenantID: ag.TenantID, Position: 0, IsPinned: false}
		if err := tx.WithContext(ctx).Create(inst).Error; err != nil {
			return fmt.Errorf("failed to create installed agent: %w", err)
		}

		// Step 5: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse workspaceID to UUID.
			workspaceUUID, err := uuid.Parse(tenantID)
			if err != nil {
				return fmt.Errorf("failed to parse tenant ID: %w", err)
			}

			// Create usage history record
			agentIDStr := ag.ID.String()
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &workspaceUUID,
				ResourceType: quota_model.ResourceTypeAIAgents,
				Delta:        1, // +1 for creating an agent
				ResourceID:   &agentIDStr,
				ResourceName: &ag.Name,
				Metadata: &quota_model.JSONMap{
					"agent_id":   ag.ID.String(),
					"agent_name": ag.Name,
					"agent_type": ag.AgentsType,
				},
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record AI agent usage: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if ag.AgentsType == "AGENT" || ag.AgentsType == "CHAT_AGENT" || ag.AgentsType == "GENERATION_AGENT" {
		cfg := &AgentsConfig{AgentsID: ag.ID, PromptType: "simple"}
		if err := s.agentsRepo.CreateAgentsConfig(ctx, cfg); err != nil {
			return nil, err
		}
		ag.AgentsModelConfigID = &cfg.ID
	}

	if ag.AgentsType == "WORKFLOW" || ag.AgentsType == "CONVERSATIONAL_WORKFLOW" || ag.AgentsType == "CHAT_AGENT" {
		if s.workflowService != nil {
			// Create default workflow using SyncDraftWorkflow
			// defaultGraph := map[string]interface{}{
			// 	"nodes": []map[string]interface{}{
			// 		{
			// 			"id":   fmt.Sprintf("%d", time.Now().UnixMilli()),
			// 			"type": "custom",
			// 			"data": map[string]interface{}{
			// 				"type":      "start",
			// 				"title":     "Start",
			// 				"desc":      "",
			// 				"variables": []interface{}{},
			// 			},
			// 			"position": map[string]interface{}{
			// 				"x": 80,
			// 				"y": 282,
			// 			},
			// 			"targetPosition": "left",
			// 			"sourcePosition": "right",
			// 		},
			// 	},
			// 	"edges": []interface{}{},
			// }

			// defaultFeatures := map[string]interface{}{
			// 	"opening_statement":   "",
			// 	"suggested_questions": []string{},
			// 	"suggested_questions_after_answer": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"speech_to_text": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"text_to_speech": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"retriever_resource": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"annotation_reply": map[string]interface{}{
			// 		"enabled": false,
			// 	},
			// 	"file_upload": map[string]interface{}{
			// 		"image": map[string]interface{}{
			// 			"enabled":          false,
			// 			"number_limits":    3,
			// 			"detail":           "high",
			// 			"transfer_methods": []string{"remote_url", "local_file"},
			// 		},
			// 	},
			// }

			syncReq := &dto.SyncDraftWorkflowRequest{
				Graph:    nil,
				Features: nil,
				Type:     changeWorkflowType(agentType),
				Internal: &internal,
			}

			logCtx := logger.WithFields(ctx,
				zap.String("agent_id", ag.ID.String()),
				zap.String("tenant_id", tenantID),
				zap.String("account_id", accountID),
			)

			// Create workflow
			logger.DebugContext(logCtx, "creating default workflow for agent")
			_, err := s.workflowService.SyncDraftWorkflow(ctx, tenantID, ag.ID.String(), syncReq, accountID)
			if err != nil {
				logger.ErrorContext(logCtx, "failed to create default workflow for agent", err)
				return nil, fmt.Errorf("failed to create default workflow: %w", err)
			}
			logger.DebugContext(logCtx, "default workflow created for agent")

			// Get the created workflow to set WorkflowID
			workflowData, err := s.workflowService.GetDraftWorkflow(ctx, ag.ID.String())
			if err != nil {
				logger.ErrorContext(logCtx, "failed to get default workflow for agent", err)
				return nil, fmt.Errorf("failed to get created workflow: %w", err)
			}
			logger.DebugContext(logCtx, "retrieved default workflow for agent")

			if workflowMap, ok := workflowData.(map[string]interface{}); ok {
				if workflowID, ok := workflowMap["id"].(string); ok {
					logCtx = logger.WithFields(logCtx, zap.String("workflow_id", workflowID))
					logger.DebugContext(logCtx, "found workflow id for agent")
					if workflowUUID, err := uuid.Parse(workflowID); err == nil {
						ag.WorkflowID = &workflowUUID
						logger.DebugContext(logCtx, "set workflow id for agent")
					} else {
						logger.ErrorContext(logCtx, "failed to parse workflow id for agent", err)
					}
				} else {
					logger.ErrorContext(logCtx, "workflow id missing from default workflow response")
				}
			} else {
				logger.ErrorContext(logCtx, "default workflow response has unexpected shape")
			}
		}
	}

	// Persist agent updates if any
	if ag.AgentsModelConfigID != nil || ag.WorkflowID != nil {
		if err := s.agentsRepo.Update(ctx, ag); err != nil {
			return nil, err
		}
	}

	return ag, nil
}

func (s *agentsService) GetAgentsList(ctx context.Context, accountID, tenantID string, req interface{}) (interface{}, error) {
	logger.Info("GetAgentsList called with accountID: %s, tenantID: %s", accountID, tenantID)
	page := 1
	limit := 20
	name := ""
	isCreatedByMe := false
	agentType := ""
	var internal *bool

	switch v := req.(type) {
	case dto.GetAgentsListRequest:
		if v.Page > 0 {
			page = v.Page
		}
		// Handle pageSize and limit priority: pageSize > limit > default(20)
		if v.PageSize > 0 {
			limit = v.PageSize
		} else if v.Limit > 0 {
			limit = v.Limit
		}
		name = strings.TrimSpace(v.Name)
		isCreatedByMe = v.IsCreatedByMe
		agentType = strings.TrimSpace(v.AgentType)
		internal = v.Internal
	case *dto.GetAgentsListRequest:
		if v != nil {
			if v.Page > 0 {
				page = v.Page
			}
			// Handle pageSize and limit priority: pageSize > limit > default(20)
			if v.PageSize > 0 {
				limit = v.PageSize
			} else if v.Limit > 0 {
				limit = v.Limit
			}
			name = strings.TrimSpace(v.Name)
			isCreatedByMe = v.IsCreatedByMe
			agentType = strings.TrimSpace(v.AgentType)
			internal = v.Internal
		}
	case map[string]interface{}:
		if p, ok := v["page"].(float64); ok && int(p) > 0 {
			page = int(p)
		}
		if p, ok := v["limit"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		// Handle pageSize parameter
		if p, ok := v["pageSize"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		if s2, ok := v["name"].(string); ok {
			name = strings.TrimSpace(s2)
		}
		if b, ok := v["is_created_by_me"].(bool); ok {
			isCreatedByMe = b
		}
		if s2, ok := v["agent_type"].(string); ok {
			agentType = strings.TrimSpace(s2)
		}
		if b, ok := v["internal"].(bool); ok {
			internal = &b
		}
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	if page > 99999 {
		page = 99999
	}

	// Get user role information for RBAC filtering
	// Requirement 9.3: Log and return appropriate error for getUserRoleInfo failures
	roleInfo, err := s.getUserRoleInfo(ctx, accountID)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsList: Failed to get user role info for account %s: %v", accountID, err), err)
		return nil, fmt.Errorf("failed to determine user permissions: %w", err)
	}

	// Build filter
	filter := AgentsFilter{}
	if name != "" {
		if len(name) > 30 {
			name = name[:30]
		}
		filter.Name = name
	}
	if agentType != "" {
		filter.AgentsType = normalizeMode(agentType)
	}
	if isCreatedByMe {
		filter.CreatedBy = accountID
	}
	if internal != nil {
		filter.Internal = internal
	}

	internalStr := "nil"
	if filter.Internal != nil {
		if *filter.Internal {
			internalStr = "true"
		} else {
			internalStr = "false"
		}
	}
	logger.Info("GetAgentsList filter: Name=%s, AgentsType=%s, CreatedBy=%s, Internal=%s, page=%d, limit=%d", filter.Name, filter.AgentsType, filter.CreatedBy, internalStr, page, limit)

	// Use RBAC-aware query
	// Requirement 9.4: Handle repository errors with appropriate logging
	list, total, err := s.agentsRepo.GetPaginatedAgentsWithRBAC(ctx, accountID, roleInfo, filter, page, limit)
	logger.Info("GetAgentsList result: total=%d, list_count=%d, error=%v", total, len(list), err)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsList: Repository query failed for account %s: %v", accountID, err), err)
		return nil, fmt.Errorf("failed to retrieve agents: %w", err)
	}

	type AgentListItem struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		AgentType    string  `json:"agent_type"`
		IconType     *string `json:"icon_type,omitempty"`
		Icon         *string `json:"icon,omitempty"`
		IconUrl      string  `json:"icon_url,omitempty"`
		IsPublic     bool    `json:"is_public"`
		IsPublished  bool    `json:"is_published"`
		WebAppStatus string  `json:"web_app_status"`
		CreatedBy    *string `json:"created_by,omitempty"`
		CreatedAt    int64   `json:"created_at"`
		UpdatedAt    int64   `json:"updated_at"`
		Internal     bool    `json:"internal"`
	}

	items := make([]AgentListItem, 0, len(list))
	for _, a := range list {
		createdBy := ""
		if a.CreatedBy != nil {
			createdBy = a.CreatedBy.String()
		}
		var createdByPtr *string
		if createdBy != "" {
			createdByPtr = &createdBy
		}

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

		items = append(items, AgentListItem{
			ID:           a.ID.String(),
			Name:         a.Name,
			Description:  a.Description,
			AgentType:    a.AgentsType,
			IconType:     iconType,
			Icon:         icon,
			IconUrl:      iconUrl,
			IsPublic:     a.IsPublic,
			WebAppStatus: string(NormalizeAgentWebAppStatus(a.WebAppStatus)),
			CreatedBy:    createdByPtr,
			CreatedAt:    a.CreatedAt.Unix(),
			UpdatedAt:    a.UpdatedAt.Unix(),
			Internal:     a.Internal,
		})
	}

	type PageResp struct {
		Page    int             `json:"page"`
		Limit   int             `json:"limit"`
		Total   int64           `json:"total"`
		HasMore bool            `json:"has_more"`
		Data    []AgentListItem `json:"data"`
	}
	hasMore := int64(page*limit) < total
	return PageResp{Page: page, Limit: limit, Total: total, HasMore: hasMore, Data: items}, nil
}

func (s *agentsService) GetAgentsListMultipleTenants(ctx context.Context, accountID string, tenantIDs []string, req interface{}) (interface{}, error) {
	logger.Info("GetAgentsListMultipleTenants called with accountID: %s, tenantIDs: %v", accountID, tenantIDs)

	if len(tenantIDs) == 0 {
		return nil, fmt.Errorf("no tenant IDs provided")
	}

	page := 1
	limit := 20
	name := ""
	isCreatedByMe := false
	agentType := ""
	var internal *bool

	switch v := req.(type) {
	case dto.GetAgentsListRequest:
		if v.Page > 0 {
			page = v.Page
		}
		if v.PageSize > 0 {
			limit = v.PageSize
		} else if v.Limit > 0 {
			limit = v.Limit
		}
		name = strings.TrimSpace(v.Name)
		isCreatedByMe = v.IsCreatedByMe
		agentType = strings.TrimSpace(v.AgentType)
		internal = v.Internal
	case *dto.GetAgentsListRequest:
		if v != nil {
			if v.Page > 0 {
				page = v.Page
			}
			if v.PageSize > 0 {
				limit = v.PageSize
			} else if v.Limit > 0 {
				limit = v.Limit
			}
			name = strings.TrimSpace(v.Name)
			isCreatedByMe = v.IsCreatedByMe
			agentType = strings.TrimSpace(v.AgentType)
			internal = v.Internal
		}
	case map[string]interface{}:
		if p, ok := v["page"].(float64); ok && int(p) > 0 {
			page = int(p)
		}
		if p, ok := v["limit"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		if p, ok := v["pageSize"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		if s2, ok := v["name"].(string); ok {
			name = strings.TrimSpace(s2)
		}
		if b, ok := v["is_created_by_me"].(bool); ok {
			isCreatedByMe = b
		}
		if s2, ok := v["agent_type"].(string); ok {
			agentType = strings.TrimSpace(s2)
		}
		if b, ok := v["internal"].(bool); ok {
			internal = &b
		}
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	if page > 99999 {
		page = 99999
	}

	// Get user role information for RBAC filtering
	// Requirement 9.3: Log and return appropriate error for getUserRoleInfo failures
	roleInfo, err := s.getUserRoleInfo(ctx, accountID)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsListMultipleTenants: Failed to get user role info for account %s: %v", accountID, err), err)
		return nil, fmt.Errorf("failed to determine user permissions: %w", err)
	}

	// Build filter
	filter := AgentsFilter{}
	if name != "" {
		if len(name) > 30 {
			name = name[:30]
		}
		filter.Name = name
	}
	if agentType != "" {
		filter.AgentsType = normalizeMode(agentType)
	}
	if isCreatedByMe {
		filter.CreatedBy = accountID
	}
	if internal != nil {
		filter.Internal = internal
	}

	internalStr := "nil"
	if filter.Internal != nil {
		if *filter.Internal {
			internalStr = "true"
		} else {
			internalStr = "false"
		}
	}
	logger.Info("GetAgentsListMultipleTenants filter: Name=%s, AgentsType=%s, CreatedBy=%s, Internal=%s, page=%d, limit=%d", filter.Name, filter.AgentsType, filter.CreatedBy, internalStr, page, limit)

	// Use RBAC-aware query (the RBAC logic will handle multiple tenants internally)
	// Requirement 9.4: Handle repository errors with appropriate logging
	list, total, err := s.agentsRepo.GetPaginatedAgentsWithRBAC(ctx, accountID, roleInfo, filter, page, limit)
	logger.Info("GetAgentsListMultipleTenants result: total=%d, list_count=%d, error=%v", total, len(list), err)
	if err != nil {
		logger.Error(fmt.Sprintf("GetAgentsListMultipleTenants: Repository query failed for account %s: %v", accountID, err), err)
		return nil, fmt.Errorf("failed to retrieve agents: %w", err)
	}

	type AgentListItem struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		AgentType    string  `json:"agent_type"`
		IconType     *string `json:"icon_type,omitempty"`
		Icon         *string `json:"icon,omitempty"`
		IconUrl      string  `json:"icon_url,omitempty"`
		IsPublic     bool    `json:"is_public"`
		IsPublished  bool    `json:"is_published"`
		WebAppStatus string  `json:"web_app_status"`
		CreatedBy    *string `json:"created_by,omitempty"`
		CreatedAt    int64   `json:"created_at"`
		UpdatedAt    int64   `json:"updated_at"`
		Internal     bool    `json:"internal"`
	}

	items := make([]AgentListItem, 0, len(list))
	for _, a := range list {
		createdBy := ""
		if a.CreatedBy != nil {
			createdBy = a.CreatedBy.String()
		}
		var createdByPtr *string
		if createdBy != "" {
			createdByPtr = &createdBy
		}

		isPublished := false
		if hasPublished, err := s.hasPublishedVersion(ctx, &a); err == nil {
			isPublished = hasPublished
		} else {
			logger.Error(fmt.Sprintf("GetAgentsListMultipleTenants: Failed to check published workflow for agent %s: %v", a.ID.String(), err), err)
			// Continue with isPublished=false, don't fail the entire operation
		}

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

		items = append(items, AgentListItem{
			ID:           a.ID.String(),
			Name:         a.Name,
			Description:  a.Description,
			AgentType:    a.AgentsType,
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
		})
	}

	type PageResp struct {
		Page    int             `json:"page"`
		Limit   int             `json:"limit"`
		Total   int64           `json:"total"`
		HasMore bool            `json:"has_more"`
		Data    []AgentListItem `json:"data"`
	}
	hasMore := int64(page*limit) < total
	return PageResp{Page: page, Limit: limit, Total: total, HasMore: hasMore, Data: items}, nil
}

func (s *agentsService) GetInternalAgentsList(ctx context.Context, accountID string, tenantIDs []string, req interface{}) (interface{}, error) {
	logger.Info("GetInternalAgentsList called with accountID: %s, tenantIDs: %v", accountID, tenantIDs)

	if len(tenantIDs) == 0 {
		return nil, fmt.Errorf("no tenant IDs provided")
	}

	page := 1
	limit := 20
	name := ""

	switch v := req.(type) {
	case dto.GetAgentsListRequest:
		if v.Page > 0 {
			page = v.Page
		}
		if v.PageSize > 0 {
			limit = v.PageSize
		} else if v.Limit > 0 {
			limit = v.Limit
		}
		name = strings.TrimSpace(v.Name)
	case *dto.GetAgentsListRequest:
		if v != nil {
			if v.Page > 0 {
				page = v.Page
			}
			if v.PageSize > 0 {
				limit = v.PageSize
			} else if v.Limit > 0 {
				limit = v.Limit
			}
			name = strings.TrimSpace(v.Name)
		}
	case map[string]interface{}:
		if p, ok := v["page"].(float64); ok && int(p) > 0 {
			page = int(p)
		}
		if p, ok := v["limit"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		if p, ok := v["pageSize"].(float64); ok && int(p) > 0 {
			limit = int(p)
		}
		if s2, ok := v["name"].(string); ok {
			name = strings.TrimSpace(s2)
		}
	}

	if limit > 100 {
		limit = 100
	}

	trueVal := true
	filter := AgentsFilter{
		Name:     name,
		Internal: &trueVal,
	}

	logger.Info("GetInternalAgentsList filter: TenantIDs=%v, Name=%s, Internal=true, page=%d, limit=%d", tenantIDs, filter.Name, page, limit)

	list, total, err := s.agentsRepo.GetPaginatedAgentsMultipleTenants(ctx, tenantIDs, filter, page, limit)
	logger.Info("GetInternalAgentsList result: total=%d, list_count=%d, error=%v", total, len(list), err)
	if err != nil {
		return nil, err
	}

	type AgentListItem struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		AgentType    string  `json:"agent_type"`
		IconType     *string `json:"icon_type,omitempty"`
		Icon         *string `json:"icon,omitempty"`
		IconUrl      string  `json:"icon_url,omitempty"`
		IsPublic     bool    `json:"is_public"`
		IsPublished  bool    `json:"is_published"`
		WebAppStatus string  `json:"web_app_status"`
		CreatedBy    *string `json:"created_by,omitempty"`
		CreatedAt    int64   `json:"created_at"`
		UpdatedAt    int64   `json:"updated_at"`
		Internal     bool    `json:"internal"`
	}

	items := make([]AgentListItem, 0, len(list))

	for _, a := range list {
		createdBy := ""
		if a.CreatedBy != nil {
			createdBy = a.CreatedBy.String()
		}
		var createdByPtr *string
		if createdBy != "" {
			createdByPtr = &createdBy
		}

		isPublished := false
		if hasPublished, err := s.hasPublishedVersion(ctx, &a); err == nil {
			isPublished = hasPublished
		} else {
			logger.Error(fmt.Sprintf("GetInternalAgentsList: Failed to check published workflow for agent %s: %v", a.ID.String(), err), err)
			// Continue with isPublished=false, don't fail the entire operation
		}

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

		items = append(items, AgentListItem{
			ID:           a.ID.String(),
			Name:         a.Name,
			Description:  a.Description,
			AgentType:    a.AgentsType,
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
		})
	}

	type PageResp struct {
		Page    int             `json:"page"`
		Limit   int             `json:"limit"`
		Total   int64           `json:"total"`
		HasMore bool            `json:"has_more"`
		Data    []AgentListItem `json:"data"`
	}
	hasMore := int64(page*limit) < total
	return PageResp{Page: page, Limit: limit, Total: total, HasMore: hasMore, Data: items}, nil
}

func (s *agentsService) GetAgent(ctx context.Context, agentID string) (interface{}, error) {
	// Implement detailed agent retrieval based on type and related entities
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}

	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if accountID != "" && callerOrganizationID != "" && s.enterpriseService != nil {
		canView, err := s.enterpriseService.CheckWorkspacePermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			model.WorkspacePermissionAgentView,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("GetAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
		if !canView {
			return nil, fmt.Errorf("permission denied")
		}
	}

	// Check edit permission using permission service
	canEdit := false
	if accountID != "" && s.resourcePermissionService != nil {
		var createdBy string
		if ag.CreatedBy != nil {
			createdBy = ag.CreatedBy.String()
		}

		canEditResult, err := s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID: accountID,
			TenantID:  ag.TenantID.String(),
			CreatedBy: createdBy,
			GroupID:   nil, // Agents don't have group_id
		})
		if err != nil {
			logger.Error(fmt.Sprintf("GetAgent: Failed to check edit permission for agent %s, account %s: %v", agentID, accountID, err), err)
			// Continue with canEdit=false on error
		} else {
			canEdit = canEditResult
		}
	}

	iconUrl := ""
	if ag.IconType != nil && *ag.IconType == "image" && ag.Icon != nil && *ag.Icon != "" {
		fileURL, err := s.fileService.GetFileURL(ctx, *ag.Icon)
		if err != nil {
			logger.Warn(fmt.Sprintf("failed to get file URL for icon %s: %v", *ag.Icon, err))
		} else {
			iconUrl = fileURL
		}
	}

	isPublished := false
	if hasPublished, err := s.hasPublishedVersion(ctx, ag); err == nil {
		isPublished = hasPublished
	} else {
		logger.Warn("GetAgent: Failed to check published workflow", map[string]interface{}{
			"agent_id": ag.ID.String(),
			"error":    err.Error(),
		})
	}

	resp := map[string]interface{}{
		"id":             ag.ID.String(),
		"name":           ag.Name,
		"description":    ag.Description,
		"agent_type":     ag.AgentsType,
		"icon_type":      ag.IconType,
		"icon":           ag.Icon,
		"icon_url":       iconUrl,
		"enable_api":     ag.EnableAPI,
		"web_app_status": string(NormalizeAgentWebAppStatus(ag.WebAppStatus)),
		"is_published":   isPublished,
		"created_at":     ag.CreatedAt.Unix(),
		"updated_at":     ag.UpdatedAt.Unix(),
		"is_editor":      false,
		"can_edit":       canEdit,
	}

	if ag.CreatedBy != nil {
		createdBy := ag.CreatedBy.String()
		resp["created_by"] = createdBy
	}
	if ag.UpdatedBy != nil {
		updatedBy := ag.UpdatedBy.String()
		resp["updated_by"] = updatedBy
	}

	// Permission from extension (best-effort)
	if ext, err := s.agentsRepo.GetExtensionByAgentID(ctx, ag.ID.String()); err == nil && ext != nil && ext.Permission != nil {
		resp["permission"] = *ext.Permission
	}

	// Set internal field from agent
	resp["internal"] = ag.Internal

	if ag.AgentsType == "WORKFLOW" || ag.AgentsType == "CONVERSATIONAL_WORKFLOW" || ag.AgentsType == "CHAT_AGENT" {
		// Workflow information will be handled by workflow service
		resp["workflow"] = nil
		resp["agent_config"] = nil
	} else {
		if ag.AgentsModelConfigID != nil {
			if cfg, err := s.agentsRepo.GetAgentsConfigByID(ctx, ag.AgentsModelConfigID.String()); err == nil && cfg != nil {
				cfgMap := map[string]interface{}{
					"id":               cfg.ID.String(),
					"model_provider":   cfg.ModelProvider,
					"model_version_id": cfg.ModelVersionID,
					"prompt_type":      cfg.PromptType,
					"created_at":       cfg.CreatedAt.Unix(),
					"updated_at":       cfg.UpdatedAt.Unix(),
				}
				resp["agent_config"] = cfgMap
			}
		}
	}

	// Attach tenant brief
	tenantID := ag.TenantID.String()
	if t, err := s.tenantService.GetWorkspaceByID(ctx, tenantID); err == nil && t != nil {
		resp["tenant"] = map[string]interface{}{"id": t.ID, "name": t.Name}
	}

	// Attach owner account brief
	if ag.CreatedBy != nil {
		if owner, err := s.accountService.GetAccountByID(ctx, ag.CreatedBy.String()); err == nil && owner != nil {
			resp["owner_account"] = map[string]interface{}{"id": owner.ID, "name": owner.Name}
		}
	}

	// Determine is_editor based on current account in context
	if ag.CreatedBy != nil && accountID != "" {
		resp["is_editor"] = strings.EqualFold(accountID, ag.CreatedBy.String())
	}

	return resp, nil
}

func (s *agentsService) UpdateAgent(ctx context.Context, agentID string, req interface{}) (interface{}, error) {
	// Validate agent ID
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("invalid agent ID")
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}
	if accountID == "" {
		return nil, fmt.Errorf("unauthorized: account ID not found in context")
	}

	// Load agent
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found")
	}

	creatorID := ""
	if ag.CreatedBy != nil {
		creatorID = ag.CreatedBy.String()
	}

	canUpdate := false
	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if callerOrganizationID != "" && s.enterpriseService != nil {
		canUpdate, err = s.enterpriseService.CheckWorkspacePermission(
			ctx,
			callerOrganizationID,
			ag.TenantID.String(),
			accountID,
			model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("UpdateAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canUpdate, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID: accountID,
			TenantID:  ag.TenantID.String(),
			CreatedBy: creatorID,
			GroupID:   nil,
		})
		if err != nil {
			logger.Error(fmt.Sprintf("UpdateAgent: failed to check edit permission for agent %s, account %s", agentID, accountID), err)
			return nil, fmt.Errorf("failed to verify permissions")
		}
	} else if creatorID != "" && strings.EqualFold(creatorID, accountID) {
		canUpdate = true
	}

	if !canUpdate {
		return nil, fmt.Errorf("permission denied")
	}

	isEditor, err := s.accountService.IsEditor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !isEditor {
		return nil, fmt.Errorf("permission denied")
	}

	// Parse request fields
	var (
		namePtr        *string
		descPtr        *string
		iconTypePtr    *string
		iconPtr        *string
		workspaceIDPtr *string

		internalPtr *bool
	)

	switch v := req.(type) {
	case map[string]interface{}:
		if s2, ok := v["name"].(string); ok {
			namePtr = &s2
		}
		if s2, ok := v["description"].(string); ok {
			descPtr = &s2
		}
		if s2, ok := v["icon_type"].(string); ok {
			iconTypePtr = &s2
		}
		if s2, ok := v["icon"].(string); ok {
			iconPtr = &s2
		}
		if s2, ok := v["tenant_id"].(string); ok {
			workspaceIDPtr = &s2
		}

		if b, ok := v["internal"].(bool); ok {
			internalPtr = &b
		}
	case *dto.CreateAgentRequest:
		if v != nil {
			if v.Name != "" {
				namePtr = &v.Name
			}
			if v.Description != "" {
				descPtr = &v.Description
			}
			if v.IconType != "" {
				iconTypePtr = &v.IconType
			}
			if v.Icon != "" {
				iconPtr = &v.Icon
			}
			if v.WorkspaceID != "" {
				workspaceIDPtr = &v.WorkspaceID
			}

			if v.Internal != nil {
				internalPtr = v.Internal
			}
		}
	case dto.CreateAgentRequest:
		if v.Name != "" {
			namePtr = &v.Name
		}
		if v.Description != "" {
			descPtr = &v.Description
		}
		if v.IconType != "" {
			iconTypePtr = &v.IconType
		}
		if v.Icon != "" {
			iconPtr = &v.Icon
		}
		if v.WorkspaceID != "" {
			workspaceIDPtr = &v.WorkspaceID
		}

		if v.Internal != nil {
			internalPtr = v.Internal
		}
	default:
		// Allow empty update
	}

	// Apply name change with duplicate check (within target tenant)
	if namePtr != nil {
		newName := strings.TrimSpace(*namePtr)
		if newName == "" {
			return nil, fmt.Errorf("invalid name")
		}
		// Determine target workspace for name conflict check.
		workspaceForName := ag.TenantID.String()
		if workspaceIDPtr != nil && strings.TrimSpace(*workspaceIDPtr) != "" {
			workspaceForName = strings.TrimSpace(*workspaceIDPtr)
		}
		if !strings.EqualFold(newName, ag.Name) {
			if exists, err := s.agentsRepo.ExistsByName(ctx, workspaceForName, newName); err != nil {
				return nil, err
			} else if exists {
				return nil, fmt.Errorf("agent with the same name already exists")
			}
		}
		ag.Name = newName
	}

	// Apply description
	if descPtr != nil {
		ag.Description = *descPtr
	}
	// Apply icon/icon_type
	if iconTypePtr != nil {
		val := strings.TrimSpace(*iconTypePtr)
		ag.IconType = &val
	}
	if iconPtr != nil {
		val := strings.TrimSpace(*iconPtr)
		ag.Icon = &val
	}

	// Apply workspace change.
	if workspaceIDPtr != nil && strings.TrimSpace(*workspaceIDPtr) != "" {
		if uid, err := uuid.Parse(strings.TrimSpace(*workspaceIDPtr)); err == nil {
			ag.TenantID = uid
		} else {
			return nil, fmt.Errorf("invalid tenant_id")
		}
	}

	// Apply internal field
	if internalPtr != nil {
		ag.Internal = *internalPtr
	}

	// Update audit fields
	if uid, err := uuid.Parse(accountID); err == nil {
		ag.UpdatedBy = &uid
	}
	ag.UpdatedAt = time.Now()

	// Persist agent
	if err := s.agentsRepo.Update(ctx, ag); err != nil {
		return nil, err
	}

	// Update workflow's internal field if provided
	if internalPtr != nil && s.workflowService != nil {
		// Get the current workflow to update its internal field
		workflowData, err := s.workflowService.GetDraftWorkflow(ctx, ag.ID.String())
		if err == nil && workflowData != nil {
			if workflowMap, ok := workflowData.(map[string]interface{}); ok {
				// Prepare sync request with current workflow data and updated internal field
				syncReq := &dto.SyncDraftWorkflowRequest{
					Internal: internalPtr,
				}

				// Extract existing graph and features to preserve them
				if graph, ok := workflowMap["graph"].(map[string]interface{}); ok {
					syncReq.Graph = graph
				}
				if features, ok := workflowMap["features"].(map[string]interface{}); ok {
					syncReq.Features = features
				}
				if workflowType, ok := workflowMap["type"].(dto.WorkflowType); ok {
					syncReq.Type = workflowType
				} else if workflowTypeStr, ok := workflowMap["type"].(string); ok {
					syncReq.Type = dto.WorkflowType(workflowTypeStr)
				}

				// Extract environment and conversation variables
				if envVars, ok := workflowMap["environment_variables"].([]dto.Variable); ok {
					syncReq.EnvironmentVariables = envVars
				}
				if convVars, ok := workflowMap["conversation_variables"].([]dto.Variable); ok {
					syncReq.ConversationVariables = convVars
				}

				// Update the workflow with the new internal field
				_, updateErr := s.workflowService.SyncDraftWorkflow(ctx, ag.TenantID.String(), ag.ID.String(), syncReq, accountID)
				if updateErr != nil {
					logger.Error("Failed to update workflow internal field: %v", updateErr)
					// Don't fail the entire update operation if workflow update fails
				}
			}
		}
	}

	return ag, nil
}

func (s *agentsService) DeleteAgent(ctx context.Context, agentID string) error {
	// Validate agent ID parameter
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("invalid agent ID")
	}

	// Get current account ID from context
	accountID := ""
	if v := ctx.Value("account_id"); v != nil {
		if id, ok := v.(string); ok {
			accountID = id
		}
	}
	if accountID == "" {
		return fmt.Errorf("unauthorized: account ID not found in context")
	}

	// Get agent by ID to check if it exists and get creator info
	agent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		logger.Error("Failed to get agent by ID: %v", err)
		return fmt.Errorf("agent not found")
	}

	// Step 1: Get organization ID from workspace for quota recording.
	var groupID *uuid.UUID
	workspaceID := agent.TenantID.String()
	if s.enterpriseService != nil {
		group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err == nil && group != nil {
			// Parse organization ID string to UUID.
			parsedGroupID, parseErr := uuid.Parse(group.ID)
			if parseErr == nil {
				groupID = &parsedGroupID
			}
		} else if err != nil {
			logger.Warn("DeleteAgent: failed to resolve organization for workspace", map[string]interface{}{
				"agent_id":      agentID,
				"workspace_id":  workspaceID,
				"account_id":    accountID,
				"error_message": err.Error(),
			})
		}
	}

	creatorID := ""
	if agent.CreatedBy != nil {
		creatorID = agent.CreatedBy.String()
	}

	canDelete := false
	callerOrganizationID := ""
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			callerOrganizationID = strings.TrimSpace(id)
		}
	}

	if callerOrganizationID != "" && s.enterpriseService != nil {
		canDelete, err = s.enterpriseService.CheckWorkspacePermission(
			ctx,
			callerOrganizationID,
			workspaceID,
			accountID,
			model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("DeleteAgent: failed to check workspace permission for agent %s, account %s", agentID, accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	} else if s.resourcePermissionService != nil {
		canDelete, err = s.resourcePermissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
			AccountID: accountID,
			TenantID:  workspaceID,
			CreatedBy: creatorID,
			GroupID:   uuidPtrToString(groupID),
		})
		if err != nil {
			logger.Error(fmt.Sprintf("DeleteAgent: failed to check edit permission for agent %s, account %s", agentID, accountID), err)
			return fmt.Errorf("failed to verify permissions")
		}
	}
	if !canDelete {
		logger.Info("DeleteAgent: permission denied", map[string]interface{}{
			"agent_id":     agentID,
			"account_id":   accountID,
			"creator_id":   creatorID,
			"workspace_id": workspaceID,
		})
		return fmt.Errorf("permission denied")
	}

	// Check if user has editor permission
	isEditor, err := s.accountService.IsEditor(ctx, accountID)
	if err != nil {
		logger.Error("Failed to check editor permission: %v", err)
		return fmt.Errorf("failed to verify permissions")
	}
	if !isEditor {
		return fmt.Errorf("permission denied")
	}

	// Step 2: Perform soft delete and record usage in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Perform soft delete by calling repository
		if err := tx.WithContext(ctx).Model(&Agent{}).Where("id = ?", agentID).Update("deleted_at", time.Now()).Error; err != nil {
			return fmt.Errorf("failed to delete agent: %w", err)
		}

		// Step 3: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Create usage history record with negative delta
			agentIDCopy := agentID
			agentNameCopy := agent.Name
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &agent.TenantID,
				ResourceType: quota_model.ResourceTypeAIAgents,
				Delta:        -1, // -1 for deleting an agent
				ResourceID:   &agentIDCopy,
				ResourceName: &agentNameCopy,
				Metadata: &quota_model.JSONMap{
					"agent_id":   agentID,
					"agent_name": agent.Name,
					"action":     "deleted",
				},
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record AI agent deletion: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		logger.Error("Failed to delete agent: %v", err)
		return fmt.Errorf("failed to delete agent")
	}

	logger.Info("Agent %s successfully deleted by user %s", agentID, accountID)
	return nil
}

func normalizeMode(agentType string) string {
	m := strings.TrimSpace(agentType)
	switch strings.ToUpper(m) {
	case "AGENT":
		return "AGENT"
	case "CONVERSATIONAL_AGENT", "CONVERSATIONAL-AGENT":
		return "CONVERSATIONAL_AGENT"
	case "WORKFLOW":
		return "WORKFLOW"
	case "CONVERSATIONAL_WORKFLOW":
		return "CONVERSATIONAL_WORKFLOW"
	default:
		return "AGENT"
	}
}

func (s *agentsService) hasPublishedVersion(ctx context.Context, ag *Agent) (bool, error) {
	if ag == nil {
		return false, fmt.Errorf("agent is required")
	}
	if ag.AgentsType == "AGENT" {
		return s.agentsRepo.HasPublishedAgentVersion(ctx, ag.ID.String())
	}
	return s.agentsRepo.HasPublishedWorkflow(ctx, ag.ID.String())
}

func changeWorkflowType(workflowType string) dto.WorkflowType {
	m := strings.TrimSpace(workflowType)
	switch strings.ToUpper(m) {
	case "WORKFLOW":
		return dto.WorkflowTypeWorkflow
	case "CONVERSATIONAL_WORKFLOW":
		return dto.WorkflowTypeChat
	default:
		return dto.WorkflowTypeWorkflow
	}
}

func parseUUID(id string) uuid.UUID {
	v, _ := uuid.Parse(id)
	return v
}

func uuidPtrToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	value := id.String()
	return &value
}
