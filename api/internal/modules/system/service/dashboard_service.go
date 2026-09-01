package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	chatruntimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	dashboardcache "github.com/zgiai/zgi/api/internal/modules/system/cache"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// DashboardService provides dashboard statistics functionality
type DashboardService interface {
	GetDashboardStats(ctx context.Context, organizationID string, accountID string, scopes model.DashboardWorkspaceScopes) (*model.DashboardStatsResponse, error)
	GetRecentWork(ctx context.Context, req model.RecentWorkRequest) (*model.RecentWorkResponse, error)
}

// AvailableModelsLister lists organization-scoped models that are callable by business features.
type AvailableModelsLister interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelsvc.AvailableModel, error)
}

type dashboardService struct {
	db              *gorm.DB
	availableModels AvailableModelsLister
	dashboardCache  *dashboardcache.DashboardCache
	tableCacheMu    sync.RWMutex
	tableCache      map[string]bool
	statsGroup      singleflight.Group
	recentWorkGroup singleflight.Group
}

const (
	dashboardAgentTypeAgent = "AGENT"
	dashboardLoadTimeout    = 5 * time.Second
)

var dashboardWorkflowAgentTypes = []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW"}

// NewDashboardService creates a new DashboardService instance
func NewDashboardService(db *gorm.DB) DashboardService {
	return NewDashboardServiceWithAvailableModels(db, nil)
}

// NewDashboardServiceWithAvailableModels creates a dashboard service using available-model stats when wired.
func NewDashboardServiceWithAvailableModels(db *gorm.DB, availableModels AvailableModelsLister) DashboardService {
	return &dashboardService{
		db:              db,
		availableModels: availableModels,
		dashboardCache:  dashboardcache.NewDashboardCache(),
		tableCache:      make(map[string]bool),
	}
}

// tableExistsWithHealth checks if a table exists in the database (cached per
// service lifetime). The health result distinguishes an expected missing table
// from a failed probe so degraded statistics are not stored as stable zeros.
func (s *dashboardService) tableExistsWithHealth(ctx context.Context, tableName string) (bool, bool) {
	s.tableCacheMu.RLock()
	if cached, ok := s.tableCache[tableName]; ok {
		s.tableCacheMu.RUnlock()
		return cached, true
	}
	s.tableCacheMu.RUnlock()

	s.tableCacheMu.Lock()
	defer s.tableCacheMu.Unlock()
	if cached, ok := s.tableCache[tableName]; ok {
		return cached, true
	}

	var exists bool
	err := s.db.WithContext(ctx).Raw(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = ?)",
		tableName,
	).Scan(&exists).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard table existence check failed",
			zap.String("table", tableName),
			zap.Error(err),
		)
		return false, false
	}
	s.tableCache[tableName] = exists
	return exists, true
}

func (s *dashboardService) tableExists(ctx context.Context, tableName string) bool {
	exists, _ := s.tableExistsWithHealth(ctx, tableName)
	return exists
}

// safeCount performs a COUNT query, returning 0 on any error
func (s *dashboardService) safeCount(ctx context.Context, table string, where string, args ...interface{}) int64 {
	count, _ := s.safeCountWithHealth(ctx, table, where, args...)
	return count
}

// safeCountWithHealth reports whether a zero is an authoritative result. A
// missing optional table is healthy; a failed table probe or COUNT is not.
func (s *dashboardService) safeCountWithHealth(ctx context.Context, table string, where string, args ...interface{}) (int64, bool) {
	exists, healthy := s.tableExistsWithHealth(ctx, table)
	if !healthy {
		return 0, false
	}
	if !exists {
		return 0, true
	}
	var count int64
	q := s.db.WithContext(ctx).Table(table)
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Count(&count).Error; err != nil {
		logger.WarnContext(ctx, "Dashboard count query failed",
			zap.String("table", table),
			zap.Error(err),
		)
		return 0, false
	}
	return count, true
}

// GetDashboardStats retrieves dashboard statistics for the current account's visible organization scope.
func (s *dashboardService) GetDashboardStats(ctx context.Context, organizationID string, accountID string, scopes model.DashboardWorkspaceScopes) (*model.DashboardStatsResponse, error) {
	scopeKey := dashboardStatsScopeKey(accountID, scopes)
	if cached, ok := s.dashboardCache.GetStats(ctx, organizationID, accountID, scopeKey); ok {
		cached.Activity = s.getActivityStats(ctx, organizationID, accountID, scopes)
		return cached, nil
	}

	value, err, _ := s.statsGroup.Do("stats:"+organizationID+":"+accountID+":"+scopeKey, func() (interface{}, error) {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dashboardLoadTimeout)
		defer cancel()
		if cached, ok := s.dashboardCache.GetStats(loadCtx, organizationID, accountID, scopeKey); ok {
			cached.Activity = s.getActivityStats(loadCtx, organizationID, accountID, scopes)
			return cached, nil
		}

		stats := model.DashboardStatsResponse{
			Models: model.ModelsStats{ByUseCase: make(map[string]int64)},
		}
		models, modelsHealthy := s.getModelStats(loadCtx, organizationID)
		stats.Models = models
		resources, resourcesHealthy := s.getResourceStatsWithHealth(loadCtx, organizationID, accountID, scopes)
		stats.Resources = resources
		if modelsHealthy && resourcesHealthy {
			// Cache only stable model/resource data. Personal activity drives the
			// onboarding checklist and must be refreshed on every request.
			s.dashboardCache.SetStats(loadCtx, organizationID, accountID, scopeKey, &stats)
		}
		stats.Activity = s.getActivityStats(loadCtx, organizationID, accountID, scopes)
		return &stats, nil
	})
	if err != nil {
		return nil, err
	}

	return value.(*model.DashboardStatsResponse), nil
}

// GetRecentWork retrieves recently updated console work items for the visible workspace sets.
func (s *dashboardService) GetRecentWork(ctx context.Context, req model.RecentWorkRequest) (*model.RecentWorkResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if len(req.AgentWorkspaceIDs) == 0 &&
		len(req.WorkflowWorkspaceIDs) == 0 &&
		len(req.AgentConversationWorkspaceIDs) == 0 &&
		len(req.WorkflowConversationWorkspaceIDs) == 0 &&
		len(req.DatasetWorkspaceIDs) == 0 &&
		len(req.DataSourceWorkspaceIDs) == 0 &&
		len(req.FileWorkspaceIDs) == 0 &&
		(req.OrganizationID == "" || req.AccountID == "") {
		return &model.RecentWorkResponse{Items: []model.RecentWorkItem{}}, nil
	}
	req.Limit = limit
	scopeKey := dashboardRecentWorkScopeKey(req)

	value, err, _ := s.recentWorkGroup.Do(
		fmt.Sprintf("recent-work:%s:%s:%d:%s", req.OrganizationID, req.AccountID, limit, scopeKey),
		func() (interface{}, error) {
			loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dashboardLoadTimeout)
			defer cancel()

			result := s.loadRecentWork(loadCtx, req)
			return result, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return value.(*model.RecentWorkResponse), nil
}

func (s *dashboardService) loadRecentWork(ctx context.Context, req model.RecentWorkRequest) *model.RecentWorkResponse {
	limit := req.Limit

	items := make([]model.RecentWorkItem, 0, limit)

	items = append(items, s.getRecentAgents(ctx, req.AgentWorkspaceIDs, []string{dashboardAgentTypeAgent}, "agent", limit)...)
	items = append(items, s.getRecentAgents(ctx, req.WorkflowWorkspaceIDs, dashboardWorkflowAgentTypes, "workflow", limit)...)
	items = append(items, s.getRecentDatasets(ctx, req.DatasetWorkspaceIDs, req.AccountID, limit)...)
	items = append(items, s.getRecentAgentConversations(ctx, req.AgentConversationWorkspaceIDs, []string{dashboardAgentTypeAgent}, req.AccountID, limit)...)
	items = append(items, s.getRecentAgentConversations(ctx, req.WorkflowConversationWorkspaceIDs, dashboardWorkflowAgentTypes, req.AccountID, limit)...)
	items = append(items, s.getRecentAIChatConversations(ctx, req.OrganizationID, req.AccountID, req.WorkspaceIDs, limit)...)
	items = append(items, s.getRecentDataSources(ctx, req.OrganizationID, req.DataSourceWorkspaceIDs, limit)...)

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})

	if len(items) > limit {
		items = items[:limit]
	}

	return &model.RecentWorkResponse{Items: items}
}

func (s *dashboardService) getModelStats(ctx context.Context, organizationID string) (model.ModelsStats, bool) {
	if s.availableModels != nil {
		return s.getAvailableModelStats(ctx, organizationID)
	}
	return s.getGlobalModelStats(ctx)
}

func (s *dashboardService) getAvailableModelStats(ctx context.Context, organizationID string) (model.ModelsStats, bool) {
	stats := model.ModelsStats{
		ByUseCase: make(map[string]int64),
	}

	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		logger.WarnContext(ctx, "Dashboard available model stats skipped for invalid organization id",
			zap.String("organization_id", organizationID),
			zap.Error(err),
		)
		return stats, false
	}

	models, err := s.availableModels.ListAvailable(ctx, orgUUID, "", "")
	if err != nil {
		logger.WarnContext(ctx, "Dashboard available model stats query failed",
			zap.String("organization_id", organizationID),
			zap.Error(err),
		)
		return stats, false
	}

	for _, availableModel := range models {
		if availableModel == nil {
			continue
		}
		stats.Total++
		for _, useCase := range availableModel.UseCases {
			if useCase != "" {
				stats.ByUseCase[useCase]++
			}
		}
	}

	return stats, true
}

// getGlobalModelStats retrieves model statistics from llm_models grouped by use_cases.
// It is kept as a defensive fallback when the available-models service is not wired.
func (s *dashboardService) getGlobalModelStats(ctx context.Context) (model.ModelsStats, bool) {
	stats := model.ModelsStats{
		ByUseCase: make(map[string]int64),
	}

	exists, healthy := s.tableExistsWithHealth(ctx, "llm_models")
	if !healthy {
		return stats, false
	}
	if !exists {
		return stats, true
	}

	var countHealthy bool
	stats.Total, countHealthy = s.safeCountWithHealth(ctx, "llm_models", "is_active = ? AND deleted_at IS NULL", true)
	if !countHealthy {
		return stats, false
	}

	type useCaseCount struct {
		UseCase string `gorm:"column:use_case"`
		Count   int64  `gorm:"column:count"`
	}
	var ucCounts []useCaseCount
	if err := s.db.WithContext(ctx).
		Raw(`SELECT unnest(use_cases) AS use_case, COUNT(*) AS count
			FROM llm_models
			WHERE is_active = ? AND deleted_at IS NULL
			GROUP BY use_case`, true).
		Scan(&ucCounts).Error; err != nil {
		logger.WarnContext(ctx, "Dashboard model stats query failed", zap.Error(err))
		return stats, false
	}

	for _, uc := range ucCounts {
		if uc.UseCase != "" {
			stats.ByUseCase[uc.UseCase] = uc.Count
		}
	}

	return stats, true
}

// getResourceStats retrieves resource statistics for the account-visible workspace scopes.
func (s *dashboardService) getResourceStats(ctx context.Context, organizationID string, accountID string, scopes model.DashboardWorkspaceScopes) model.ResourceStats {
	stats, _ := s.getResourceStatsWithHealth(ctx, organizationID, accountID, scopes)
	return stats
}

func (s *dashboardService) getResourceStatsWithHealth(ctx context.Context, organizationID string, accountID string, scopes model.DashboardWorkspaceScopes) (model.ResourceStats, bool) {
	var stats model.ResourceStats
	healthy := true

	stats.Workspaces = int64(len(scopes.WorkspaceIDs))

	if len(scopes.AgentWorkspaceIDs) > 0 || len(scopes.WorkflowWorkspaceIDs) > 0 {
		var countHealthy bool
		stats.Agents, countHealthy = s.countAgentAssetsWithHealth(ctx, scopes.AgentWorkspaceIDs, scopes.WorkflowWorkspaceIDs)
		healthy = healthy && countHealthy
	}

	if len(scopes.DatasetWorkspaceIDs) > 0 {
		var countHealthy bool
		stats.Datasets, countHealthy = s.safeCountWithHealth(ctx, "datasets", "workspace_id IN ?", scopes.DatasetWorkspaceIDs)
		healthy = healthy && countHealthy
	}

	if len(scopes.DataSourceWorkspaceIDs) > 0 {
		var countHealthy bool
		stats.DataSources, countHealthy = s.safeCountWithHealth(ctx, "data_sources", "organization_id = ? AND workspace_id IN ?", organizationID, scopes.DataSourceWorkspaceIDs)
		healthy = healthy && countHealthy
	}

	if len(scopes.FileWorkspaceIDs) > 0 {
		var countHealthy bool
		stats.Files, countHealthy = s.safeCountWithHealth(ctx, "upload_files", "workspace_id IN ?", scopes.FileWorkspaceIDs)
		healthy = healthy && countHealthy
	}

	return stats, healthy
}

func (s *dashboardService) getActivityStats(ctx context.Context, organizationID string, accountID string, scopes model.DashboardWorkspaceScopes) model.ActivityStats {
	var stats model.ActivityStats
	if organizationID == "" || accountID == "" {
		return stats
	}

	directConversationWhere := "organization_id = ? AND account_id = ? AND source = ? AND caller_type = ? AND conversation_type = ? AND dialogue_count > 0"
	directConversationArgs := []interface{}{
		organizationID,
		accountID,
		chatruntimemodel.ConversationSourceConsole,
		chatruntimemodel.ConversationCallerAIChat,
		chatruntimemodel.ConversationTypeChat,
	}
	if len(scopes.WorkspaceIDs) > 0 {
		directConversationWhere += " AND (workspace_id IN ? OR workspace_id IS NULL)"
		directConversationArgs = append(directConversationArgs, scopes.WorkspaceIDs)
	} else {
		directConversationWhere += " AND workspace_id IS NULL"
	}
	stats.DirectConversations = s.safeCount(
		ctx,
		"chat_runtime_conversations",
		directConversationWhere,
		directConversationArgs...,
	)
	if len(scopes.WorkspaceIDs) > 0 {
		stats.CreatedAgents = s.safeCount(
			ctx,
			"agents",
			"tenant_id IN ? AND created_by = ? AND agent_type = ? AND is_universal = ? AND (internal = ? OR internal IS NULL)",
			scopes.WorkspaceIDs,
			accountID,
			dashboardAgentTypeAgent,
			false,
			false,
		)
	}
	if len(scopes.WorkspaceIDs) > 0 {
		stats.CreatedDatasets = s.safeCount(
			ctx,
			"datasets",
			"workspace_id IN ? AND created_by = ?",
			scopes.WorkspaceIDs,
			accountID,
		)
	}
	return stats
}

// getWorkspaceIDs retrieves all workspace IDs belonging to an organization
func (s *dashboardService) getWorkspaceIDs(ctx context.Context, organizationID string) []string {
	if !s.tableExists(ctx, "workspaces") {
		return nil
	}
	var ids []string
	if err := s.db.WithContext(ctx).
		Table("workspaces").
		Where("organization_id = ?", organizationID).
		Pluck("id", &ids).Error; err != nil {
		logger.WarnContext(ctx, "Dashboard workspace ids query failed",
			zap.String("organization_id", organizationID),
			zap.Error(err),
		)
		return nil
	}
	return ids
}

type recentWorkRow struct {
	ID            string
	Title         string
	ResourceID    string
	ParentID      string
	WorkspaceID   string
	WorkspaceName string
	UpdatedAt     time.Time
	CreatedAt     time.Time
}

func (s *dashboardService) countAgentAssets(ctx context.Context, agentWorkspaceIDs []string, workflowWorkspaceIDs []string) int64 {
	count, _ := s.countAgentAssetsWithHealth(ctx, agentWorkspaceIDs, workflowWorkspaceIDs)
	return count
}

func (s *dashboardService) countAgentAssetsWithHealth(ctx context.Context, agentWorkspaceIDs []string, workflowWorkspaceIDs []string) (int64, bool) {
	exists, healthy := s.tableExistsWithHealth(ctx, "agents")
	if !healthy {
		return 0, false
	}
	if !exists {
		return 0, true
	}

	query := s.db.WithContext(ctx).
		Table("agents").
		Where("deleted_at IS NULL AND is_universal = ? AND (internal = ? OR internal IS NULL)", false, false)
	query = applyAgentTypeWorkspaceScope(query, agentWorkspaceIDs, workflowWorkspaceIDs)

	var count int64
	if err := query.Count(&count).Error; err != nil {
		logger.WarnContext(ctx, "Dashboard agent asset count query failed", zap.Error(err))
		return 0, false
	}
	return count, true
}

func (s *dashboardService) getRecentAgents(ctx context.Context, workspaceIDs []string, agentTypes []string, itemType string, limit int) []model.RecentWorkItem {
	if len(workspaceIDs) == 0 || !s.tableExists(ctx, "agents") || !s.tableExists(ctx, "workspaces") {
		return nil
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("agents AS a").
		Select("a.id, a.name AS title, a.id AS resource_id, a.tenant_id AS workspace_id, w.name AS workspace_name, a.updated_at, a.created_at").
		Joins("INNER JOIN workspaces AS w ON w.id = a.tenant_id").
		Where("a.tenant_id IN ? AND a.agent_type IN ? AND a.deleted_at IS NULL AND a.is_universal = ? AND (a.internal = ? OR a.internal IS NULL)", workspaceIDs, agentTypes, false, false).
		Order("a.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent agent assets query failed",
			zap.String("item_type", itemType),
			zap.Error(err),
		)
		return nil
	}

	return makeRecentWorkItems(itemType, rows)
}

func (s *dashboardService) getRecentDatasets(ctx context.Context, workspaceIDs []string, accountID string, limit int) []model.RecentWorkItem {
	if len(workspaceIDs) == 0 || !s.tableExists(ctx, "datasets") || !s.tableExists(ctx, "workspaces") {
		return nil
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("datasets AS d").
		Select("d.id, d.name AS title, d.id AS resource_id, d.workspace_id, w.name AS workspace_name, d.updated_at, d.created_at").
		Joins("INNER JOIN workspaces AS w ON w.id = d.workspace_id").
		Where("d.workspace_id IN ?", workspaceIDs).
		Order("d.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent datasets query failed", zap.Error(err))
		return nil
	}

	return makeRecentWorkItems("dataset", rows)
}

func (s *dashboardService) getRecentDataSources(ctx context.Context, organizationID string, workspaceIDs []string, limit int) []model.RecentWorkItem {
	if len(workspaceIDs) == 0 || !s.tableExists(ctx, "data_sources") || !s.tableExists(ctx, "workspaces") {
		return nil
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("data_sources AS ds").
		Select("ds.id, ds.name AS title, ds.id AS resource_id, ds.workspace_id, w.name AS workspace_name, ds.updated_at, ds.created_at").
		Joins("INNER JOIN workspaces AS w ON w.id = ds.workspace_id").
		Where("ds.organization_id = ? AND ds.workspace_id IN ?", organizationID, workspaceIDs).
		Order("ds.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent data sources query failed", zap.Error(err))
		return nil
	}

	return makeRecentWorkItems("database", rows)
}

func (s *dashboardService) getRecentAgentConversations(ctx context.Context, workspaceIDs []string, agentTypes []string, accountID string, limit int) []model.RecentWorkItem {
	if len(workspaceIDs) == 0 || accountID == "" || !s.tableExists(ctx, "agents_conversations") || !s.tableExists(ctx, "agents") || !s.tableExists(ctx, "workspaces") {
		return nil
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("agents_conversations AS c").
		Select("c.id, c.name AS title, c.id AS resource_id, c.agent_id AS parent_id, a.tenant_id AS workspace_id, w.name AS workspace_name, c.updated_at, c.created_at").
		Joins("INNER JOIN agents AS a ON a.id = c.agent_id").
		Joins("INNER JOIN workspaces AS w ON w.id = a.tenant_id").
		Where("a.tenant_id IN ? AND a.agent_type IN ? AND a.deleted_at IS NULL AND c.deleted_at IS NULL AND c.from_account_id = ?", workspaceIDs, agentTypes, accountID).
		Order("c.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent conversations query failed", zap.Error(err))
		return nil
	}

	return makeRecentWorkItems("conversation", rows)
}

func (s *dashboardService) getRecentAIChatConversations(ctx context.Context, organizationID, accountID string, workspaceIDs []string, limit int) []model.RecentWorkItem {
	if organizationID == "" || accountID == "" || !s.tableExists(ctx, "chat_runtime_conversations") || !s.tableExists(ctx, "workspaces") {
		return nil
	}

	query := s.db.WithContext(ctx).
		Table("chat_runtime_conversations AS c").
		Select("c.id, c.title, c.id AS resource_id, c.workspace_id, w.name AS workspace_name, c.updated_at, c.created_at").
		Joins("LEFT JOIN workspaces AS w ON w.id = c.workspace_id").
		Where(
			"c.organization_id = ? AND c.account_id = ? AND c.deleted_at IS NULL AND c.source = ? AND c.caller_type = ? AND c.conversation_type = ? AND c.dialogue_count > 0",
			organizationID,
			accountID,
			chatruntimemodel.ConversationSourceConsole,
			chatruntimemodel.ConversationCallerAIChat,
			chatruntimemodel.ConversationTypeChat,
		)
	if len(workspaceIDs) > 0 {
		query = query.Where("c.workspace_id IN ? OR c.workspace_id IS NULL", workspaceIDs)
	} else {
		query = query.Where("c.workspace_id IS NULL")
	}

	var rows []recentWorkRow
	if err := query.Order("c.updated_at DESC").Limit(limit).Scan(&rows).Error; err != nil {
		logger.WarnContext(ctx, "Dashboard recent direct conversations query failed", zap.Error(err))
		return nil
	}

	return makeRecentWorkItems("conversation", rows)
}

func applyAgentTypeWorkspaceScope(query *gorm.DB, agentWorkspaceIDs []string, workflowWorkspaceIDs []string) *gorm.DB {
	if len(agentWorkspaceIDs) > 0 && len(workflowWorkspaceIDs) > 0 {
		return query.Where(
			"(tenant_id IN ? AND agent_type = ?) OR (tenant_id IN ? AND agent_type IN ?)",
			agentWorkspaceIDs,
			dashboardAgentTypeAgent,
			workflowWorkspaceIDs,
			dashboardWorkflowAgentTypes,
		)
	}
	if len(agentWorkspaceIDs) > 0 {
		return query.Where("tenant_id IN ? AND agent_type = ?", agentWorkspaceIDs, dashboardAgentTypeAgent)
	}
	return query.Where("tenant_id IN ? AND agent_type IN ?", workflowWorkspaceIDs, dashboardWorkflowAgentTypes)
}

func makeRecentWorkItems(itemType string, rows []recentWorkRow) []model.RecentWorkItem {
	items := make([]model.RecentWorkItem, 0, len(rows))
	for _, row := range rows {
		updatedAt := row.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = row.CreatedAt
		}
		if updatedAt.IsZero() {
			continue
		}

		resourceID := row.ResourceID
		if resourceID == "" {
			resourceID = row.ID
		}
		items = append(items, model.RecentWorkItem{
			ID:            fmt.Sprintf("%s:%s", itemType, row.ID),
			Type:          itemType,
			Title:         row.Title,
			ResourceID:    resourceID,
			ParentID:      row.ParentID,
			WorkspaceID:   row.WorkspaceID,
			WorkspaceName: row.WorkspaceName,
			UpdatedAt:     updatedAt.Unix(),
		})
	}
	return items
}

func dashboardStatsScopeKey(accountID string, scopes model.DashboardWorkspaceScopes) string {
	return dashboardScopeFingerprint(struct {
		AccountID                        string
		WorkspaceIDs                     []string
		AgentWorkspaceIDs                []string
		WorkflowWorkspaceIDs             []string
		AgentConversationWorkspaceIDs    []string
		WorkflowConversationWorkspaceIDs []string
		DatasetWorkspaceIDs              []string
		DataSourceWorkspaceIDs           []string
		FileWorkspaceIDs                 []string
	}{
		AccountID:                        accountID,
		WorkspaceIDs:                     sortedWorkspaceIDs(scopes.WorkspaceIDs),
		AgentWorkspaceIDs:                sortedWorkspaceIDs(scopes.AgentWorkspaceIDs),
		WorkflowWorkspaceIDs:             sortedWorkspaceIDs(scopes.WorkflowWorkspaceIDs),
		AgentConversationWorkspaceIDs:    sortedWorkspaceIDs(scopes.AgentConversationWorkspaceIDs),
		WorkflowConversationWorkspaceIDs: sortedWorkspaceIDs(scopes.WorkflowConversationWorkspaceIDs),
		DatasetWorkspaceIDs:              sortedWorkspaceIDs(scopes.DatasetWorkspaceIDs),
		DataSourceWorkspaceIDs:           sortedWorkspaceIDs(scopes.DataSourceWorkspaceIDs),
		FileWorkspaceIDs:                 sortedWorkspaceIDs(scopes.FileWorkspaceIDs),
	})
}

func dashboardRecentWorkScopeKey(req model.RecentWorkRequest) string {
	return dashboardScopeFingerprint(struct {
		WorkspaceIDs                     []string
		AgentWorkspaceIDs                []string
		WorkflowWorkspaceIDs             []string
		AgentConversationWorkspaceIDs    []string
		WorkflowConversationWorkspaceIDs []string
		DatasetWorkspaceIDs              []string
		DataSourceWorkspaceIDs           []string
		FileWorkspaceIDs                 []string
	}{
		WorkspaceIDs:                     sortedWorkspaceIDs(req.WorkspaceIDs),
		AgentWorkspaceIDs:                sortedWorkspaceIDs(req.AgentWorkspaceIDs),
		WorkflowWorkspaceIDs:             sortedWorkspaceIDs(req.WorkflowWorkspaceIDs),
		AgentConversationWorkspaceIDs:    sortedWorkspaceIDs(req.AgentConversationWorkspaceIDs),
		WorkflowConversationWorkspaceIDs: sortedWorkspaceIDs(req.WorkflowConversationWorkspaceIDs),
		DatasetWorkspaceIDs:              sortedWorkspaceIDs(req.DatasetWorkspaceIDs),
		DataSourceWorkspaceIDs:           sortedWorkspaceIDs(req.DataSourceWorkspaceIDs),
		FileWorkspaceIDs:                 sortedWorkspaceIDs(req.FileWorkspaceIDs),
	})
}

func dashboardScopeFingerprint(value interface{}) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum[:8])
}

func sortedWorkspaceIDs(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
