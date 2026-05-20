package agents

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type AgentsFilter struct {
	TenantID   string
	Name       string
	Keyword    string
	AgentsType string
	CreatedBy  string
	Internal   *bool
}

// UserRoleInfo contains user role information for RBAC filtering
type UserRoleInfo struct {
	IsOrgAdmin        bool     // Whether user is organization admin
	IsDeptAdmin       bool     // Whether user is department admin
	OrganizationIDs   []string // Organization IDs user belongs to
	DepartmentIDs     []string // Department IDs user belongs to
	CurrentDepartment string   // User's current department (current=true)
}

type runnableWebAppItem struct {
	AgentID       string  `gorm:"column:agent_id"`
	WorkspaceID   string  `gorm:"column:workspace_id"`
	WebAppID      string  `gorm:"column:web_app_id"`
	WebAppStatus  string  `gorm:"column:web_app_status"`
	AgentName     string  `gorm:"column:agent_name"`
	AgentIcon     *string `gorm:"column:agent_icon"`
	AgentIconType *string `gorm:"column:agent_icon_type"`
	AgentDesc     string  `gorm:"column:agent_desc"`
	AgentType     string  `gorm:"column:agent_type"`
}

// AgentsRepository defines the interface for agent data access operations in agents module
type AgentsRepository interface {
	Create(ctx context.Context, ag *Agent) error
	GetByID(ctx context.Context, id string) (*Agent, error)
	GetByWebAppID(ctx context.Context, webAppID string) (*Agent, error)
	Update(ctx context.Context, ag *Agent) error
	Delete(ctx context.Context, id string, deletedBy string) error

	GetByTenantID(ctx context.Context, tenantID string) ([]Agent, error)
	GetPaginatedAgents(ctx context.Context, filter AgentsFilter, page, limit int) ([]Agent, int64, error)
	GetPaginatedAgentsMultipleTenants(ctx context.Context, tenantIDs []string, filter AgentsFilter, page, limit int) ([]Agent, int64, error)
	GetPaginatedAgentsWithRBAC(ctx context.Context, accountID string, roleInfo *UserRoleInfo, filter AgentsFilter, page, limit int) ([]Agent, int64, error)
	GetPaginatedAgentsWithPermissions(ctx context.Context, accountID string, permissionContext *PermissionContext, filter AgentsFilter, page, limit int) ([]Agent, int64, error)

	ExistsByName(ctx context.Context, tenantID, name string) (bool, error)
	CreateExtension(ctx context.Context, ext *AgentExtension) error
	GetExtensionByAgentID(ctx context.Context, agentID string) (*AgentExtension, error)
	UpdateExtension(ctx context.Context, ext *AgentExtension) error
	UpdateWebAppStatus(ctx context.Context, agentID string, status AgentWebAppStatus, reason string, updatedBy string) error
	CreateInstalled(ctx context.Context, inst *InstalledAgent) error
	CreateAgentsConfig(ctx context.Context, cfg *AgentsConfig) error
	GetAgentsConfigByID(ctx context.Context, id string) (*AgentsConfig, error)
	UpdateWorkflowID(ctx context.Context, agentID, workflowID string) error
	UpdateWorkflowConfig(ctx context.Context, agentID, workflowConfig string) error
	HasPublishedWorkflow(ctx context.Context, agentID string) (bool, error)
	ListRunnableWebApps(ctx context.Context, workspaceIDs []string, workspaceID string) ([]runnableWebAppItem, error)
}

// agentsRepository implements AgentsRepository
type agentsRepository struct {
	db *gorm.DB
}

// NewAgentsRepository creates a new AgentsRepository instance
func NewAgentsRepository(db *gorm.DB) AgentsRepository {
	return &agentsRepository{
		db: db,
	}
}

// Create creates a new agent
func (r *agentsRepository) Create(ctx context.Context, ag *Agent) error {
	if err := r.db.WithContext(ctx).Create(ag).Error; err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	return nil
}

// ExistsByName checks if an agent with the same name exists under the tenant
func (r *agentsRepository) ExistsByName(ctx context.Context, tenantID, name string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Agent{}).
		Where("tenant_id = ? AND name = ? AND deleted_at IS NULL", tenantID, name).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check agent name: %w", err)
	}
	return count > 0, nil
}

// GetByID retrieves an agent by ID
func (r *agentsRepository) GetByID(ctx context.Context, id string) (*Agent, error) {
	var ag Agent
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&ag).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	ag.Source = AgentSourceUser
	return &ag, nil
}

// GetByWebAppID retrieves an agent by web_app_id
func (r *agentsRepository) GetByWebAppID(ctx context.Context, webAppID string) (*Agent, error) {
	var ag Agent
	if err := r.db.WithContext(ctx).Where("web_app_id = ? AND deleted_at IS NULL", webAppID).First(&ag).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, fmt.Errorf("failed to get agent by web_app_id: %w", err)
	}
	ag.Source = AgentSourceUser
	return &ag, nil
}

// Update updates an existing agent
func (r *agentsRepository) Update(ctx context.Context, ag *Agent) error {
	if err := r.db.WithContext(ctx).Save(ag).Error; err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}
	return nil
}

// Delete soft deletes an agent by ID
func (r *agentsRepository) Delete(ctx context.Context, id string, deletedBy string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"deleted_at": &now,
		"updated_at": now,
	}

	// Add deleted_by if provided
	if deletedBy != "" {
		updates["deleted_by"] = deletedBy
	}

	if err := r.db.WithContext(ctx).Model(&Agent{}).Where("id = ? AND deleted_at IS NULL", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to soft delete agent: %w", err)
	}
	return nil
}

// GetByTenantID retrieves all agents for a tenant
func (r *agentsRepository) GetByTenantID(ctx context.Context, tenantID string) ([]Agent, error) {
	var list []Agent
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("failed to get agents by tenant_id: %w", err)
	}
	return list, nil
}

func (r *agentsRepository) ListRunnableWebApps(ctx context.Context, workspaceIDs []string, workspaceID string) ([]runnableWebAppItem, error) {
	if len(workspaceIDs) == 0 {
		return []runnableWebAppItem{}, nil
	}

	var items []runnableWebAppItem
	query := r.db.WithContext(ctx).
		Table("agents").
		Select("agents.id AS agent_id, agents.tenant_id AS workspace_id, agents.web_app_id AS web_app_id, agents.web_app_status AS web_app_status, agents.name AS agent_name, agents.icon AS agent_icon, agents.icon_type AS agent_icon_type, agents.description AS agent_desc, agents.agent_type AS agent_type").
		Where("agents.deleted_at IS NULL").
		Where("agents.web_app_status = ?", AgentWebAppStatusActive).
		Where("agents.tenant_id IN ?", workspaceIDs).
		Where(`
			EXISTS (
				SELECT 1
				FROM workflows
				WHERE workflows.agent_id = agents.id
				  AND workflows.version != ?
			)
		`, "draft")

	if workspaceID != "" {
		query = query.Where("agents.tenant_id = ?", workspaceID)
	}

	if err := query.
		Order("agents.tenant_id ASC").
		Order("agents.created_at DESC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list runnable web apps: %w", err)
	}

	return items, nil
}

// GetPaginatedAgents retrieves paginated agents with filters
func (r *agentsRepository) GetPaginatedAgents(ctx context.Context, filter AgentsFilter, page, limit int) ([]Agent, int64, error) {
	var (
		list  []Agent
		total int64
	)

	query := r.db.WithContext(ctx).Model(&Agent{}).Where("deleted_at IS NULL")

	// Apply filters
	query = r.applyFilters(query, filter)

	// Count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agents: %w", err)
	}

	// Pagination and ordering
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get paginated agents: %w", err)
	}

	return list, total, nil
}

func (r *agentsRepository) GetPaginatedAgentsMultipleTenants(ctx context.Context, tenantIDs []string, filter AgentsFilter, page, limit int) ([]Agent, int64, error) {
	var (
		list  []Agent
		total int64
	)

	query := r.db.WithContext(ctx).Model(&Agent{}).Where("deleted_at IS NULL")

	if len(tenantIDs) > 0 {
		query = query.Where("tenant_id IN ?", tenantIDs)
	}

	// Apply other filters
	query = r.applyFiltersMultipleTenants(query, filter)

	// Count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agents: %w", err)
	}

	// Pagination and ordering
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get paginated agents: %w", err)
	}

	return list, total, nil
}

// applyFilters applies filter conditions to the query
func (r *agentsRepository) applyFilters(query *gorm.DB, filter AgentsFilter) *gorm.DB {
	// Always filter out soft deleted records
	query = query.Where("deleted_at IS NULL")

	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}
	if filter.Keyword != "" {
		query = query.Where("(name ILIKE ? OR description ILIKE ?)", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.AgentsType != "" {
		query = query.Where("agent_type = ?", filter.AgentsType)
	}
	if filter.CreatedBy != "" {
		query = query.Where("created_by = ?", filter.CreatedBy)
	}
	if filter.Internal != nil {
		query = query.Where("internal = ?", *filter.Internal)
	}
	return query
}

func (r *agentsRepository) applyFiltersMultipleTenants(query *gorm.DB, filter AgentsFilter) *gorm.DB {
	query = query.Where("deleted_at IS NULL")

	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}
	if filter.Keyword != "" {
		query = query.Where("(name ILIKE ? OR description ILIKE ?)", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.AgentsType != "" {
		query = query.Where("agent_type = ?", filter.AgentsType)
	}
	if filter.CreatedBy != "" {
		query = query.Where("created_by = ?", filter.CreatedBy)
	}
	if filter.Internal != nil {
		query = query.Where("internal = ?", *filter.Internal)
	}
	return query
}

// CreateExtension creates a record in agent_extensions
func (r *agentsRepository) CreateExtension(ctx context.Context, ext *AgentExtension) error {
	if err := r.db.WithContext(ctx).Create(ext).Error; err != nil {
		return fmt.Errorf("failed to create agent extension: %w", err)
	}
	return nil
}

// GetExtensionByAgentID retrieves the agent extension by agent_id
func (r *agentsRepository) GetExtensionByAgentID(ctx context.Context, agentID string) (*AgentExtension, error) {
	var ext AgentExtension
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&ext).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent extension: %w", err)
	}
	return &ext, nil
}

// UpdateExtension updates an existing agent extension
func (r *agentsRepository) UpdateExtension(ctx context.Context, ext *AgentExtension) error {
	if err := r.db.WithContext(ctx).Save(ext).Error; err != nil {
		return fmt.Errorf("failed to update agent extension: %w", err)
	}
	return nil
}

// CreateInstalled creates a record in installed_agents
func (r *agentsRepository) CreateInstalled(ctx context.Context, inst *InstalledAgent) error {
	if err := r.db.WithContext(ctx).Create(inst).Error; err != nil {
		return fmt.Errorf("failed to create installed agent: %w", err)
	}
	return nil
}

// CreateAgentsConfig creates a record in agents_configs
func (r *agentsRepository) CreateAgentsConfig(ctx context.Context, cfg *AgentsConfig) error {
	if err := r.db.WithContext(ctx).Create(cfg).Error; err != nil {
		return fmt.Errorf("failed to create agents config: %w", err)
	}
	return nil
}

// GetAgentsConfigByID retrieves an agents_config by id
func (r *agentsRepository) GetAgentsConfigByID(ctx context.Context, id string) (*AgentsConfig, error) {
	var cfg AgentsConfig
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&cfg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agents config: %w", err)
	}
	return &cfg, nil
}

// UpdateWorkflowID updates the workflow_id for an agent
func (r *agentsRepository) UpdateWorkflowID(ctx context.Context, agentID, workflowID string) error {
	result := r.db.WithContext(ctx).
		Model(&Agent{}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		Updates(map[string]interface{}{
			"workflow_id": workflowID,
			"updated_at":  time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update workflow_id: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found or already deleted")
	}

	return nil
}

// UpdateWorkflowConfig updates the workflow_config for an agent
func (r *agentsRepository) UpdateWorkflowConfig(ctx context.Context, agentID, workflowConfig string) error {
	result := r.db.WithContext(ctx).
		Model(&Agent{}).
		Where("id = ? AND deleted_at IS NULL", agentID).
		Updates(map[string]interface{}{
			"workflow_config": workflowConfig,
			"updated_at":      time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update workflow_config: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found or already deleted")
	}

	return nil
}

// HasPublishedWorkflow checks if an agent has any published workflow (non-draft version)
func (r *agentsRepository) HasPublishedWorkflow(ctx context.Context, agentID string) (bool, error) {
	var count int64
	// Check if there's any workflow with version != 'draft' for this agent
	if err := r.db.WithContext(ctx).
		Table("workflows").
		Where("agent_id = ? AND version != ?", agentID, "draft").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check published workflow: %w", err)
	}
	if count > 0 {
		return true, nil
	}

	return false, nil
}

// GetPaginatedAgentsWithRBAC retrieves paginated agents with RBAC filtering based on user role
// Requirement 9.4: Return 500 for database query failures with logging
func (r *agentsRepository) GetPaginatedAgentsWithRBAC(
	ctx context.Context,
	accountID string,
	roleInfo *UserRoleInfo,
	filter AgentsFilter,
	page, limit int,
) ([]Agent, int64, error) {
	var (
		list  []Agent
		total int64
	)

	// Build base query
	query := r.db.WithContext(ctx).Model(&Agent{}).Where("deleted_at IS NULL")

	// Apply RBAC filtering based on user role
	if roleInfo.IsOrgAdmin {
		// Organization Admin: can see all agents in all departments within their organizations
		query = r.buildOrgAdminQuery(query, roleInfo)
	} else if roleInfo.IsDeptAdmin {
		// Department Admin: can see all agents in their departments + org-wide shared agents
		query = r.buildDeptAdminQuery(query, accountID, roleInfo)
	} else {
		// Regular Member: apply permission-based filtering
		query = r.buildRegularMemberQuery(query, accountID, roleInfo)
	}

	// Apply additional filters (name, agent_type, internal, etc.)
	query = r.applyAdditionalFilters(query, filter)

	// Count total - Requirement 9.4: Log database errors
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agents with RBAC for account %s: database error: %w", accountID, err)
	}

	// Pagination and ordering - Requirement 9.4: Log database errors
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get paginated agents with RBAC for account %s (page=%d, limit=%d): database error: %w", accountID, page, limit, err)
	}

	return list, total, nil
}

// buildOrgAdminQuery builds query for organization administrators
// Organization admins can see all agents in all departments within their organizations
func (r *agentsRepository) buildOrgAdminQuery(query *gorm.DB, roleInfo *UserRoleInfo) *gorm.DB {
	if len(roleInfo.OrganizationIDs) == 0 {
		// No organizations, return empty result
		return query.Where("1 = 0")
	}

	// Get all tenant IDs that belong to user's organizations
	query = query.Where("tenant_id IN (?)",
		r.db.Table("workspaces").
			Select("id").
			Where("organization_id IN ?", roleInfo.OrganizationIDs),
	)

	return query
}

// buildDeptAdminQuery builds query for department administrators
// Department admins can see:
// 1. All agents in their departments (ignoring permission field)
// 2. Agents with permission='all_group' from their organizations
func (r *agentsRepository) buildDeptAdminQuery(query *gorm.DB, accountID string, roleInfo *UserRoleInfo) *gorm.DB {
	// Use LEFT JOIN with agent_extensions to check permission for org-wide agents
	query = query.Joins("LEFT JOIN agent_extensions ON agents.id = agent_extensions.agent_id")

	// Build OR conditions
	var conditions *gorm.DB

	// Condition 1: All agents in user's departments (ignore permission field)
	if len(roleInfo.DepartmentIDs) > 0 {
		conditions = r.db.Where("agents.tenant_id IN ?", roleInfo.DepartmentIDs)
	}

	// Condition 2: Organization-wide shared agents (permission='all_group')
	if len(roleInfo.OrganizationIDs) > 0 {
		orgCondition := r.db.Where("agent_extensions.permission = ? AND agents.tenant_id IN (?)", "all_group",
			r.db.Table("workspaces").
				Select("id").
				Where("organization_id IN ?", roleInfo.OrganizationIDs),
		)

		if conditions != nil {
			conditions = conditions.Or(orgCondition)
		} else {
			conditions = orgCondition
		}
	}

	// If no conditions were built, return empty result
	if conditions == nil {
		return query.Where("1 = 0")
	}

	query = query.Where(conditions)
	return query
}

// buildRegularMemberQuery builds query for regular members
// They can see:
// 1. Agents created by themselves (regardless of permission)
// 2. Agents with permission='all_team' in their departments
// 3. Agents with permission='all_group' in their organizations
func (r *agentsRepository) buildRegularMemberQuery(query *gorm.DB, accountID string, roleInfo *UserRoleInfo) *gorm.DB {
	// Use LEFT JOIN with agent_extensions to check permission
	query = query.Joins("LEFT JOIN agent_extensions ON agents.id = agent_extensions.agent_id")

	// Build OR conditions
	conditions := r.db.Where("agents.created_by = ?", accountID)

	// Add department visible agents (all_team)
	if len(roleInfo.DepartmentIDs) > 0 {
		conditions = conditions.Or(
			r.db.Where("agent_extensions.permission = ? AND agents.tenant_id IN ?", "all_team", roleInfo.DepartmentIDs),
		)
	}

	// Add organization visible agents (all_group)
	if len(roleInfo.OrganizationIDs) > 0 {
		conditions = conditions.Or(
			r.db.Where("agent_extensions.permission = ? AND agents.tenant_id IN (?)", "all_group",
				r.db.Table("workspaces").
					Select("id").
					Where("organization_id IN ?", roleInfo.OrganizationIDs),
			),
		)
	}

	query = query.Where(conditions)

	return query
}

// applyAdditionalFilters applies non-RBAC filters (name, keyword, agent_type, internal, is_created_by_me)
// This method ensures filters work correctly with table aliases when using JOINs
func (r *agentsRepository) applyAdditionalFilters(query *gorm.DB, filter AgentsFilter) *gorm.DB {
	// Apply name filter using ILIKE for case-insensitive partial match
	if filter.Name != "" {
		query = query.Where("agents.name ILIKE ?", "%"+filter.Name+"%")
	}

	// Apply keyword filter to search in both name and description
	if filter.Keyword != "" {
		query = query.Where("(agents.name ILIKE ? OR agents.description ILIKE ?)", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}

	// Apply agent_type filter using exact match
	if filter.AgentsType != "" {
		query = query.Where("agents.agent_type = ?", filter.AgentsType)
	}

	// Apply internal filter (true/false/nil)
	if filter.Internal != nil {
		query = query.Where("agents.internal = ?", *filter.Internal)
	}

	// Apply is_created_by_me filter when requested
	// When CreatedBy is set, filter to show only agents created by this user
	if filter.CreatedBy != "" {
		query = query.Where("agents.created_by = ?", filter.CreatedBy)
	}

	return query
}

// normalizePaginationParams validates and normalizes pagination parameters
// Requirements: 8.5, 8.6
func (r *agentsRepository) normalizePaginationParams(page, limit int) (int, int) {
	// Validate and normalize page parameter (default 1, max 99999)
	// Requirement 8.5: WHEN page is less than 1, THE System SHALL default to page 1
	if page < 1 {
		page = 1
	}
	if page > 99999 {
		page = 99999
	}

	// Validate and normalize limit parameter (default 20, max 100)
	// Requirement 8.6: WHEN limit exceeds 100, THE System SHALL cap it at 100
	if limit < 1 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100
	}

	return page, limit
}

// GetPaginatedAgentsWithPermissions retrieves paginated agents with permission-based filtering
// This method implements the new RBAC permission logic based on organization hierarchy,
// department memberships, and agent-level permissions.
//
// Requirements: 2.1, 2.2, 4.1, 4.2, 5.1, 5.2, 5.3, 5.4, 5.5, 6.1, 7.1, 7.2, 8.1, 8.2, 8.3, 8.4, 8.5, 8.6
func (r *agentsRepository) GetPaginatedAgentsWithPermissions(
	ctx context.Context,
	accountID string,
	permissionContext *PermissionContext,
	filter AgentsFilter,
	page, limit int,
) ([]Agent, int64, error) {
	// Normalize pagination parameters
	// Requirements: 8.5, 8.6
	page, limit = r.normalizePaginationParams(page, limit)

	// Determine which query path to use based on organization role
	isOrgAdmin := permissionContext.OrganizationRole == "owner" || permissionContext.OrganizationRole == "admin"

	if isOrgAdmin {
		// Organization Admin/Owner: Simple query path
		return r.getAgentsForOrgAdmin(ctx, permissionContext, filter, page, limit)
	}

	// Normal User: Complex permission logic
	return r.getAgentsForNormalUser(ctx, accountID, permissionContext, filter, page, limit)
}

// getAgentsForOrgAdmin retrieves agents for organization admins/owners
// Org admins can see all agents in all departments within their organization
// Requirements: 2.1, 2.2, 2.3, 8.1, 8.2, 8.3, 8.4, 11.3, 11.5
func (r *agentsRepository) getAgentsForOrgAdmin(
	ctx context.Context,
	permissionContext *PermissionContext,
	filter AgentsFilter,
	page, limit int,
) ([]Agent, int64, error) {
	var (
		list  []Agent
		total int64
	)

	// Build base query
	query := r.db.WithContext(ctx).Model(&Agent{}).
		Where("deleted_at IS NULL")

	// Filter by organization department IDs
	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	} else {
		if len(permissionContext.OrganizationDeptIDs) > 0 {
			query = query.Where("tenant_id IN ?", permissionContext.OrganizationDeptIDs)
		} else {
			// No departments in organization, return empty result
			return []Agent{}, 0, nil
		}
	}

	// Apply additional filters (name, keyword, agent_type, internal)
	// Requirement 9.1, 9.2, 9.3
	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}
	if filter.Keyword != "" {
		query = query.Where("(name ILIKE ? OR description ILIKE ?)", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.AgentsType != "" {
		query = query.Where("agent_type = ?", filter.AgentsType)
	}
	if filter.Internal != nil {
		query = query.Where("internal = ?", *filter.Internal)
	}

	// Count total
	// Requirement 8.4: WHEN counting total results, THE System SHALL count distinct agent IDs
	// Requirement 11.3: Handle database errors (500) with logging
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("database error counting agents for org admin (org_id=%s, account_id=%s): %w",
			permissionContext.OrganizationID, permissionContext.AccountID, err)
	}

	// Apply pagination and ordering
	// Requirement 8.1: WHEN a page parameter is provided, THE System SHALL apply OFFSET based on (page - 1) * limit
	// Requirement 8.2: WHEN a limit parameter is provided, THE System SHALL apply LIMIT to the query
	// Requirement 8.3: WHEN pagination is applied, THE System SHALL apply it AFTER deduplication
	// Requirement 11.3: Handle database errors (500) with logging
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("database error retrieving agents for org admin (org_id=%s, account_id=%s, page=%d, limit=%d): %w",
			permissionContext.OrganizationID, permissionContext.AccountID, page, limit, err)
	}

	return list, total, nil
}

// getAgentsForNormalUser retrieves agents for normal users with complex permission logic
// Normal users can see:
// 1. Agents they created (creator-based access)
// 2. All agents in departments where they are admin
// 3. Agents with permission='all_team' in their normal departments
// 4. Agents with permission='all_group' in their organization
// Requirements: 4.1, 4.2, 5.1, 5.2, 5.3, 5.4, 5.5, 6.1, 7.1, 7.2, 8.1, 8.2, 8.3, 8.4, 11.3, 11.5
func (r *agentsRepository) getAgentsForNormalUser(
	ctx context.Context,
	accountID string,
	permissionContext *PermissionContext,
	filter AgentsFilter,
	page, limit int,
) ([]Agent, int64, error) {
	var (
		list  []Agent
		total int64
	)

	// Build the main query with DISTINCT to handle deduplication (Requirement 7.1, 7.2)
	// Use a subquery to get distinct agent IDs first, then join to get full agent data
	subquery := r.buildPermissionSubquery(ctx, accountID, permissionContext, filter)

	// Main query: select agents by IDs from subquery
	query := r.db.WithContext(ctx).Model(&Agent{}).
		Where("id IN (?)", subquery).
		Where("deleted_at IS NULL")

	// Count total (using DISTINCT on agent IDs)
	// Requirement 8.4: WHEN counting total results, THE System SHALL count distinct agent IDs
	// Requirement 11.3: Handle database errors (500) with logging
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("database error counting agents for normal user (account_id=%s, org_id=%s, valid_depts=%v): %w",
			accountID, permissionContext.OrganizationID, permissionContext.ValidDepartmentIDs, err)
	}

	// Apply pagination and ordering
	// Requirement 8.1: WHEN a page parameter is provided, THE System SHALL apply OFFSET based on (page - 1) * limit
	// Requirement 8.2: WHEN a limit parameter is provided, THE System SHALL apply LIMIT to the query
	// Requirement 8.3: WHEN pagination is applied, THE System SHALL apply it AFTER deduplication
	// Requirement 11.3: Handle database errors (500) with logging
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("database error retrieving agents for normal user (account_id=%s, org_id=%s, valid_depts=%v, page=%d, limit=%d): %w",
			accountID, permissionContext.OrganizationID, permissionContext.ValidDepartmentIDs, page, limit, err)
	}

	return list, total, nil
}

// buildPermissionSubquery builds a subquery that returns distinct agent IDs based on permission rules
// This uses a CTE-like approach with UNION to combine different permission scenarios
func (r *agentsRepository) buildPermissionSubquery(
	ctx context.Context,
	accountID string,
	permissionContext *PermissionContext,
	filter AgentsFilter,
) *gorm.DB {
	// Start with base query that will be used for UNION
	// We'll build multiple queries and combine them with OR conditions

	baseQuery := r.db.WithContext(ctx).Model(&Agent{}).
		Select("DISTINCT agents.id").
		Where("agents.deleted_at IS NULL")

	if filter.TenantID != "" {
		baseQuery = baseQuery.Where("agents.tenant_id = ?", filter.TenantID)
	}

	// Apply filters to base query
	if filter.Name != "" {
		baseQuery = baseQuery.Where("agents.name ILIKE ?", "%"+filter.Name+"%")
	}
	if filter.Keyword != "" {
		baseQuery = baseQuery.Where("(agents.name ILIKE ? OR agents.description ILIKE ?)", "%"+filter.Keyword+"%", "%"+filter.Keyword+"%")
	}
	if filter.AgentsType != "" {
		baseQuery = baseQuery.Where("agents.agent_type = ?", filter.AgentsType)
	}
	if filter.Internal != nil {
		baseQuery = baseQuery.Where("agents.internal = ?", *filter.Internal)
	}

	// Build permission conditions using OR logic
	var permissionConditions []string
	var permissionArgs []interface{}

	// Condition 1: Creator-based access (Requirement 6.1)
	// User always sees agents they created
	permissionConditions = append(permissionConditions, "agents.created_by = ?")
	permissionArgs = append(permissionArgs, accountID)

	// Condition 2: Department admin access (Requirement 4.1)
	// User sees all agents in departments where they are admin
	if len(permissionContext.AdminDepartmentIDs) > 0 {
		placeholders := "?"
		permissionConditions = append(permissionConditions, "agents.tenant_id IN ("+placeholders+")")
		permissionArgs = append(permissionArgs, permissionContext.AdminDepartmentIDs)
	}

	// Condition 3: department-level visibility for normal members
	// NOTE: The original all_team-based permission check is temporarily disabled.
	// The permission model has been simplified so that agents are only visible
	// within the user's department tenants, regardless of per-agent all_team flag.
	// This aligns with dataset and datasource permission simplification where
	// only team tenant-level visibility is preserved for now.
	//
	// // Original logic using all_team permission:
	// // if len(permissionContext.NormalDepartmentIDs) > 0 {
	// // 	permissionConditions = append(permissionConditions,
	// // 		"(EXISTS (SELECT 1 FROM agent_extensions ae WHERE ae.agent_id = agents.id AND ae.permission = 'all_team') AND agents.tenant_id IN (?))")
	// // 	permissionArgs = append(permissionArgs, permissionContext.NormalDepartmentIDs)
	// // }
	if len(permissionContext.NormalDepartmentIDs) > 0 {
		permissionConditions = append(permissionConditions, "agents.tenant_id IN (?)")
		permissionArgs = append(permissionArgs, permissionContext.NormalDepartmentIDs)
	}

	// NOTE: Organization-wide visibility (all_group) is temporarily disabled.
	// The permission model has been simplified so that agents are only visible
	// to members within the current organization/department scope. Future
	// support for organization-wide visibility can re-enable the logic below.
	//
	// // Condition 4: all_group permission in organization (Requirement 5.3)
	// // User sees agents with all_group permission in any department of their organization
	// if len(permissionContext.OrganizationDeptIDs) > 0 {
	// 	permissionConditions = append(permissionConditions,
	// 		"(EXISTS (SELECT 1 FROM agent_extensions ae WHERE ae.agent_id = agents.id AND ae.permission = 'all_group') AND agents.tenant_id IN (?))")
	// 	permissionArgs = append(permissionArgs, permissionContext.OrganizationDeptIDs)
	// }

	// Combine all conditions with OR
	if len(permissionConditions) > 0 {
		conditionSQL := "(" + permissionConditions[0]
		for i := 1; i < len(permissionConditions); i++ {
			conditionSQL += " OR " + permissionConditions[i]
		}
		conditionSQL += ")"

		baseQuery = baseQuery.Where(conditionSQL, permissionArgs...)
	} else {
		// No valid conditions, return empty result
		baseQuery = baseQuery.Where("1 = 0")
	}

	return baseQuery
}
