package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	dashboardcache "github.com/zgiai/zgi/api/internal/modules/system/cache"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// DashboardService provides dashboard statistics functionality
type DashboardService interface {
	GetDashboardStats(ctx context.Context, organizationID string) (*model.DashboardStatsResponse, error)
	GetRecentWork(ctx context.Context, organizationID string, accountID string, limit int) (*model.RecentWorkResponse, error)
}

// AvailableModelsLister lists organization-scoped models that are callable by business features.
type AvailableModelsLister interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelsvc.AvailableModel, error)
}

type dashboardService struct {
	db              *gorm.DB
	availableModels AvailableModelsLister
	dashboardCache  *dashboardcache.DashboardCache
	tableCache      map[string]bool
	tableCacheMu    sync.RWMutex
	statsGroup      singleflight.Group
	recentWorkGroup singleflight.Group
}

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

// tableExists checks if a table exists in the database (cached per service lifetime).
// The second result is false only when the database check itself failed.
func (s *dashboardService) tableExists(ctx context.Context, tableName string) (bool, bool) {
	s.tableCacheMu.RLock()
	if cached, ok := s.tableCache[tableName]; ok {
		s.tableCacheMu.RUnlock()
		return cached, true
	}
	s.tableCacheMu.RUnlock()

	var exists bool
	err := s.db.Raw(
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
	s.tableCacheMu.Lock()
	s.tableCache[tableName] = exists
	s.tableCacheMu.Unlock()
	return exists, true
}

// safeCount performs a COUNT query. The second result reports whether the
// result is safe to cache; a missing optional table is a legitimate zero.
func (s *dashboardService) safeCount(ctx context.Context, table string, where string, args ...interface{}) (int64, bool) {
	exists, checked := s.tableExists(ctx, table)
	if !checked {
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

// GetDashboardStats retrieves dashboard statistics for a given organization
func (s *dashboardService) GetDashboardStats(ctx context.Context, organizationID string) (*model.DashboardStatsResponse, error) {
	if cached, ok := s.dashboardCache.GetStats(ctx, organizationID); ok {
		return cached, nil
	}

	value, err, _ := s.statsGroup.Do("stats:"+organizationID, func() (interface{}, error) {
		if cached, ok := s.dashboardCache.GetStats(ctx, organizationID); ok {
			return cached, nil
		}

		stats := model.DashboardStatsResponse{
			Models: model.ModelsStats{ByUseCase: make(map[string]int64)},
		}
		models, modelsCacheable := s.getModelStats(ctx, organizationID)
		resources, resourcesCacheable := s.getResourceStats(ctx, organizationID)
		stats.Models = models
		stats.Resources = resources

		if modelsCacheable && resourcesCacheable {
			s.dashboardCache.SetStats(ctx, organizationID, &stats)
		}
		return &stats, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*model.DashboardStatsResponse), nil
}

// GetRecentWork retrieves recently updated console work items for an organization.
func (s *dashboardService) GetRecentWork(ctx context.Context, organizationID string, accountID string, limit int) (*model.RecentWorkResponse, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if cached, ok := s.dashboardCache.GetRecentWork(ctx, organizationID, accountID, limit); ok {
		return cached, nil
	}

	value, err, _ := s.recentWorkGroup.Do(fmt.Sprintf("recent-work:%s:%s:%d", organizationID, accountID, limit), func() (interface{}, error) {
		if cached, ok := s.dashboardCache.GetRecentWork(ctx, organizationID, accountID, limit); ok {
			return cached, nil
		}

		wsIDs, workspaceIDsCacheable := s.getWorkspaceIDs(ctx, organizationID)
		items := make([]model.RecentWorkItem, 0, limit)
		cacheable := workspaceIDsCacheable

		if len(wsIDs) > 0 {
			agents, agentsCacheable := s.getRecentAgents(ctx, wsIDs, limit)
			datasets, datasetsCacheable := s.getRecentDatasets(ctx, wsIDs, limit)
			conversations, conversationsCacheable := s.getRecentAgentConversations(ctx, wsIDs, accountID, limit)
			items = append(items, agents...)
			items = append(items, datasets...)
			items = append(items, conversations...)
			cacheable = cacheable && agentsCacheable && datasetsCacheable && conversationsCacheable
		}
		dataSources, dataSourcesCacheable := s.getRecentDataSources(ctx, organizationID, limit)
		items = append(items, dataSources...)
		cacheable = cacheable && dataSourcesCacheable

		sort.SliceStable(items, func(i, j int) bool {
			return items[i].UpdatedAt > items[j].UpdatedAt
		})

		if len(items) > limit {
			items = items[:limit]
		}

		result := &model.RecentWorkResponse{Items: items}
		if cacheable {
			s.dashboardCache.SetRecentWork(ctx, organizationID, accountID, limit, result)
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*model.RecentWorkResponse), nil
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

	exists, checked := s.tableExists(ctx, "llm_models")
	if !checked {
		return stats, false
	}
	if !exists {
		return stats, true
	}

	var countCacheable bool
	stats.Total, countCacheable = s.safeCount(ctx, "llm_models", "is_active = ? AND deleted_at IS NULL", true)

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

	return stats, countCacheable
}

// getResourceStats retrieves resource statistics for the organization
func (s *dashboardService) getResourceStats(ctx context.Context, organizationID string) (model.ResourceStats, bool) {
	var stats model.ResourceStats
	cacheable := true

	wsIDs, workspaceIDsCacheable := s.getWorkspaceIDs(ctx, organizationID)
	cacheable = cacheable && workspaceIDsCacheable

	if len(wsIDs) > 0 {
		var agentsCacheable, datasetsCacheable bool
		stats.Agents, agentsCacheable = s.safeCount(ctx, "agents",
			"tenant_id IN ? AND deleted_at IS NULL AND is_universal = ? AND (internal = ? OR internal IS NULL)",
			wsIDs, false, false)
		cacheable = cacheable && agentsCacheable

		stats.Datasets, datasetsCacheable = s.safeCount(ctx, "datasets", "workspace_id IN ?", wsIDs)
		cacheable = cacheable && datasetsCacheable
	}

	var dataSourcesCacheable bool
	stats.DataSources, dataSourcesCacheable = s.safeCount(ctx, "data_sources", "organization_id = ?", organizationID)
	cacheable = cacheable && dataSourcesCacheable

	return stats, cacheable
}

// getWorkspaceIDs retrieves all workspace IDs belonging to an organization
func (s *dashboardService) getWorkspaceIDs(ctx context.Context, organizationID string) ([]string, bool) {
	exists, checked := s.tableExists(ctx, "workspaces")
	if !checked {
		return nil, false
	}
	if !exists {
		return nil, true
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
		return nil, false
	}
	return ids, true
}

type recentWorkRow struct {
	ID         string
	Title      string
	ResourceID string
	ParentID   string
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

func (s *dashboardService) getRecentAgents(ctx context.Context, workspaceIDs []string, limit int) ([]model.RecentWorkItem, bool) {
	exists, checked := s.tableExists(ctx, "agents")
	if !checked {
		return nil, false
	}
	if !exists {
		return nil, true
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("agents").
		Select("id, name AS title, id AS resource_id, updated_at, created_at").
		Where("tenant_id IN ? AND deleted_at IS NULL AND is_universal = ? AND (internal = ? OR internal IS NULL)", workspaceIDs, false, false).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent agents query failed", zap.Error(err))
		return nil, false
	}

	return makeRecentWorkItems("agent", rows), true
}

func (s *dashboardService) getRecentDatasets(ctx context.Context, workspaceIDs []string, limit int) ([]model.RecentWorkItem, bool) {
	exists, checked := s.tableExists(ctx, "datasets")
	if !checked {
		return nil, false
	}
	if !exists {
		return nil, true
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("datasets").
		Select("id, name AS title, id AS resource_id, updated_at, created_at").
		Where("workspace_id IN ?", workspaceIDs).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent datasets query failed", zap.Error(err))
		return nil, false
	}

	return makeRecentWorkItems("dataset", rows), true
}

func (s *dashboardService) getRecentDataSources(ctx context.Context, organizationID string, limit int) ([]model.RecentWorkItem, bool) {
	exists, checked := s.tableExists(ctx, "data_sources")
	if !checked {
		return nil, false
	}
	if !exists {
		return nil, true
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("data_sources").
		Select("id, name AS title, id AS resource_id, updated_at, created_at").
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent data sources query failed", zap.Error(err))
		return nil, false
	}

	return makeRecentWorkItems("database", rows), true
}

func (s *dashboardService) getRecentAgentConversations(ctx context.Context, workspaceIDs []string, accountID string, limit int) ([]model.RecentWorkItem, bool) {
	if accountID == "" {
		return nil, true
	}
	conversationsExist, conversationsChecked := s.tableExists(ctx, "agents_conversations")
	if !conversationsChecked {
		return nil, false
	}
	agentsExist, agentsChecked := s.tableExists(ctx, "agents")
	if !agentsChecked {
		return nil, false
	}
	if !conversationsExist || !agentsExist {
		return nil, true
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("agents_conversations AS c").
		Select("c.id, c.name AS title, c.id AS resource_id, c.agent_id AS parent_id, c.updated_at, c.created_at").
		Joins("INNER JOIN agents AS a ON a.id = c.agent_id").
		Where("a.tenant_id IN ? AND a.deleted_at IS NULL AND c.deleted_at IS NULL AND c.from_account_id = ?", workspaceIDs, accountID).
		Order("c.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent conversations query failed", zap.Error(err))
		return nil, false
	}

	return makeRecentWorkItems("conversation", rows), true
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
			ID:         fmt.Sprintf("%s:%s", itemType, row.ID),
			Type:       itemType,
			Title:      row.Title,
			ResourceID: resourceID,
			ParentID:   row.ParentID,
			UpdatedAt:  updatedAt.Unix(),
		})
	}
	return items
}
