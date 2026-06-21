package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// DashboardService provides dashboard statistics functionality
type DashboardService interface {
	GetDashboardStats(ctx context.Context, organizationID string) (*model.DashboardStatsResponse, error)
	GetRecentWork(ctx context.Context, organizationID string, workspaceID string, accountID string, limit int) (*model.RecentWorkResponse, error)
}

// AvailableModelsLister lists organization-scoped models that are callable by business features.
type AvailableModelsLister interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelsvc.AvailableModel, error)
}

type dashboardService struct {
	db              *gorm.DB
	availableModels AvailableModelsLister
	tableCache      map[string]bool
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
		tableCache:      make(map[string]bool),
	}
}

// tableExists checks if a table exists in the database (cached per service lifetime).
func (s *dashboardService) tableExists(ctx context.Context, tableName string) bool {
	if cached, ok := s.tableCache[tableName]; ok {
		return cached
	}
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
		return false
	}
	s.tableCache[tableName] = exists
	return exists
}

// safeCount performs a COUNT query, returning 0 on any error
func (s *dashboardService) safeCount(ctx context.Context, table string, where string, args ...interface{}) int64 {
	if !s.tableExists(ctx, table) {
		return 0
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
		return 0
	}
	return count
}

// GetDashboardStats retrieves dashboard statistics for a given organization
func (s *dashboardService) GetDashboardStats(ctx context.Context, organizationID string) (*model.DashboardStatsResponse, error) {
	stats := model.DashboardStatsResponse{
		Models: model.ModelsStats{ByUseCase: make(map[string]int64)},
	}

	stats.Models = s.getModelStats(ctx, organizationID)
	stats.Resources = s.getResourceStats(ctx, organizationID)

	return &stats, nil
}

// GetRecentWork retrieves recently updated console work items for one workspace.
func (s *dashboardService) GetRecentWork(ctx context.Context, organizationID string, workspaceID string, accountID string, limit int) (*model.RecentWorkResponse, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if workspaceID == "" {
		return &model.RecentWorkResponse{Items: []model.RecentWorkItem{}}, nil
	}

	workspaceIDs := []string{workspaceID}
	items := make([]model.RecentWorkItem, 0, limit)

	items = append(items, s.getRecentAgents(ctx, workspaceIDs, limit)...)
	items = append(items, s.getRecentDatasets(ctx, workspaceIDs, limit)...)
	items = append(items, s.getRecentAgentConversations(ctx, workspaceIDs, accountID, limit)...)
	items = append(items, s.getRecentDataSources(ctx, organizationID, workspaceID, limit)...)

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})

	if len(items) > limit {
		items = items[:limit]
	}

	return &model.RecentWorkResponse{Items: items}, nil
}

func (s *dashboardService) getModelStats(ctx context.Context, organizationID string) model.ModelsStats {
	if s.availableModels != nil {
		return s.getAvailableModelStats(ctx, organizationID)
	}
	return s.getGlobalModelStats(ctx)
}

func (s *dashboardService) getAvailableModelStats(ctx context.Context, organizationID string) model.ModelsStats {
	stats := model.ModelsStats{
		ByUseCase: make(map[string]int64),
	}

	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		logger.WarnContext(ctx, "Dashboard available model stats skipped for invalid organization id",
			zap.String("organization_id", organizationID),
			zap.Error(err),
		)
		return stats
	}

	models, err := s.availableModels.ListAvailable(ctx, orgUUID, "", "")
	if err != nil {
		logger.WarnContext(ctx, "Dashboard available model stats query failed",
			zap.String("organization_id", organizationID),
			zap.Error(err),
		)
		return stats
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

	return stats
}

// getGlobalModelStats retrieves model statistics from llm_models grouped by use_cases.
// It is kept as a defensive fallback when the available-models service is not wired.
func (s *dashboardService) getGlobalModelStats(ctx context.Context) model.ModelsStats {
	stats := model.ModelsStats{
		ByUseCase: make(map[string]int64),
	}

	if !s.tableExists(ctx, "llm_models") {
		return stats
	}

	stats.Total = s.safeCount(ctx, "llm_models", "is_active = ? AND deleted_at IS NULL", true)

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
		return stats
	}

	for _, uc := range ucCounts {
		if uc.UseCase != "" {
			stats.ByUseCase[uc.UseCase] = uc.Count
		}
	}

	return stats
}

// getResourceStats retrieves resource statistics for the organization
func (s *dashboardService) getResourceStats(ctx context.Context, organizationID string) model.ResourceStats {
	var stats model.ResourceStats

	wsIDs := s.getWorkspaceIDs(ctx, organizationID)

	if len(wsIDs) > 0 {
		stats.Agents = s.safeCount(ctx, "agents",
			"tenant_id IN ? AND deleted_at IS NULL AND is_universal = ? AND (internal = ? OR internal IS NULL)",
			wsIDs, false, false)

		stats.Datasets = s.safeCount(ctx, "datasets", "workspace_id IN ?", wsIDs)
	}

	stats.DataSources = s.safeCount(ctx, "data_sources", "organization_id = ?", organizationID)

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
	ID         string
	Title      string
	ResourceID string
	ParentID   string
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

func (s *dashboardService) getRecentAgents(ctx context.Context, workspaceIDs []string, limit int) []model.RecentWorkItem {
	if !s.tableExists(ctx, "agents") {
		return nil
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
		return nil
	}

	return makeRecentWorkItems("agent", rows)
}

func (s *dashboardService) getRecentDatasets(ctx context.Context, workspaceIDs []string, limit int) []model.RecentWorkItem {
	if !s.tableExists(ctx, "datasets") {
		return nil
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
		return nil
	}

	return makeRecentWorkItems("dataset", rows)
}

func (s *dashboardService) getRecentDataSources(ctx context.Context, organizationID string, workspaceID string, limit int) []model.RecentWorkItem {
	if !s.tableExists(ctx, "data_sources") {
		return nil
	}

	var rows []recentWorkRow
	err := s.db.WithContext(ctx).
		Table("data_sources").
		Select("id, name AS title, id AS resource_id, updated_at, created_at").
		Where("organization_id = ? AND workspace_id = ?", organizationID, workspaceID).
		Order("updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		logger.WarnContext(ctx, "Dashboard recent data sources query failed", zap.Error(err))
		return nil
	}

	return makeRecentWorkItems("database", rows)
}

func (s *dashboardService) getRecentAgentConversations(ctx context.Context, workspaceIDs []string, accountID string, limit int) []model.RecentWorkItem {
	if accountID == "" || !s.tableExists(ctx, "agents_conversations") || !s.tableExists(ctx, "agents") {
		return nil
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
		return nil
	}

	return makeRecentWorkItems("conversation", rows)
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
