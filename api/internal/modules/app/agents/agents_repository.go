package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AgentsFilter struct {
	TenantID   string
	Name       string
	Keyword    string
	AgentsType string
	AgentTypes []string
	CreatedBy  string
	Internal   *bool
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
	GetPaginatedAgentsMultipleTenants(ctx context.Context, tenantIDs []string, filter AgentsFilter, page, limit int) ([]Agent, int64, error)
	GetPaginatedAgentsWithPermissions(ctx context.Context, accountID string, permissionContext *PermissionContext, filter AgentsFilter, page, limit int) ([]Agent, int64, error)

	ExistsByName(ctx context.Context, tenantID, name string) (bool, error)
	CreateExtension(ctx context.Context, ext *AgentExtension) error
	GetExtensionByAgentID(ctx context.Context, agentID string) (*AgentExtension, error)
	UpdateExtension(ctx context.Context, ext *AgentExtension) error
	UpdateWebAppStatus(ctx context.Context, agentID string, status AgentWebAppStatus, reason string, updatedBy string) error
	CreateInstalled(ctx context.Context, inst *InstalledAgent) error
	CreateAgentsConfig(ctx context.Context, cfg *AgentsConfig) error
	GetAgentsConfigByID(ctx context.Context, id string) (*AgentsConfig, error)
	GetAgentsConfigByAgentID(ctx context.Context, agentID string) (*AgentsConfig, error)
	UpdateAgentsConfig(ctx context.Context, cfg *AgentsConfig) error
	CreateAgentPublishedVersion(ctx context.Context, version *AgentPublishedVersion) error
	GetAgentPublishedVersionByID(ctx context.Context, agentID, versionID string) (*AgentPublishedVersion, error)
	GetLatestAgentPublishedVersion(ctx context.Context, agentID string) (*AgentPublishedVersion, error)
	ListAgentPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*AgentPublishedVersion, int64, error)
	HasPublishedAgentVersion(ctx context.Context, agentID string) (bool, error)
	UpdateWorkflowID(ctx context.Context, agentID, workflowID string) error
	UpdateWorkflowConfig(ctx context.Context, agentID, workflowConfig string) error
	HasPublishedWorkflow(ctx context.Context, agentID string) (bool, error)
	ListRunnableWebApps(ctx context.Context, workspaceIDs []string, workspaceID, keyword string) ([]runnableWebAppItem, error)
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

func (r *agentsRepository) ListRunnableWebApps(ctx context.Context, workspaceIDs []string, workspaceID, keyword string) ([]runnableWebAppItem, error) {
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
			(
				agents.agent_type = ?
				AND EXISTS (
					SELECT 1
					FROM agent_published_versions
					WHERE agent_published_versions.agent_id = agents.id
					  AND agent_published_versions.deleted_at IS NULL
				)
			)
			OR (
				agents.agent_type != ?
				AND EXISTS (
					SELECT 1
					FROM workflows
					WHERE workflows.agent_id = agents.id
					  AND workflows.version != ?
				)
			)
		`, "AGENT", "AGENT", "draft")

	if workspaceID != "" {
		query = query.Where("agents.tenant_id = ?", workspaceID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("(agents.name ILIKE ? OR agents.description ILIKE ?)", pattern, pattern)
	}

	if err := query.
		Order("agents.tenant_id ASC").
		Order("agents.created_at DESC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list runnable web apps: %w", err)
	}

	return items, nil
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
	if len(filter.AgentTypes) > 0 {
		query = query.Where("agent_type IN ?", filter.AgentTypes)
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

func (r *agentsRepository) GetAgentsConfigByAgentID(ctx context.Context, agentID string) (*AgentsConfig, error) {
	var cfg AgentsConfig
	if err := r.db.WithContext(ctx).Where("agents_id = ? AND deleted_at IS NULL", agentID).Order("created_at DESC").First(&cfg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agents config by agent: %w", err)
	}
	return &cfg, nil
}

func (r *agentsRepository) UpdateAgentsConfig(ctx context.Context, cfg *AgentsConfig) error {
	if cfg == nil {
		return fmt.Errorf("agents config is required")
	}
	cfg.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(cfg).Error; err != nil {
		return fmt.Errorf("failed to update agents config: %w", err)
	}
	return nil
}

func (r *agentsRepository) CreateAgentPublishedVersion(ctx context.Context, version *AgentPublishedVersion) error {
	if version == nil {
		return fmt.Errorf("agent published version is required")
	}
	if err := r.db.WithContext(ctx).Create(version).Error; err != nil {
		return fmt.Errorf("failed to create agent published version: %w", err)
	}
	return nil
}

func (r *agentsRepository) GetAgentPublishedVersionByID(ctx context.Context, agentID, versionID string) (*AgentPublishedVersion, error) {
	var version AgentPublishedVersion
	if err := r.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND deleted_at IS NULL", versionID, agentID).
		First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent published version: %w", err)
	}
	return &version, nil
}

func (r *agentsRepository) GetLatestAgentPublishedVersion(ctx context.Context, agentID string) (*AgentPublishedVersion, error) {
	var version AgentPublishedVersion
	if err := r.db.WithContext(ctx).
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Order("created_at DESC, version DESC").
		First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest agent published version: %w", err)
	}
	return &version, nil
}

func (r *agentsRepository) ListAgentPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*AgentPublishedVersion, int64, error) {
	var versions []*AgentPublishedVersion
	var total int64
	query := r.db.WithContext(ctx).Model(&AgentPublishedVersion{}).Where("agent_id = ? AND deleted_at IS NULL", agentID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent published versions: %w", err)
	}
	if err := query.Order("created_at DESC, version DESC").Limit(limit).Offset(offset).Find(&versions).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list agent published versions: %w", err)
	}
	return versions, total, nil
}

func (r *agentsRepository) HasPublishedAgentVersion(ctx context.Context, agentID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&AgentPublishedVersion{}).Where("agent_id = ? AND deleted_at IS NULL", agentID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check published agent version: %w", err)
	}
	return count > 0, nil
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

	return r.getAgentsForNormalUser(ctx, accountID, permissionContext, filter, page, limit)
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
	if len(filter.AgentTypes) > 0 {
		baseQuery = baseQuery.Where("agents.agent_type IN ?", filter.AgentTypes)
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
