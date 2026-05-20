package repository

import (
	"context"
	"strings"

	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
	"gorm.io/gorm"
)

type statisticsRepositoryImpl struct {
	db *gorm.DB
}

type workspaceQuotaSummaryRow struct {
	TotalWorkspaces  int64 `gorm:"column:total_workspaces"`
	UnlimitedCount   int64 `gorm:"column:unlimited_count"`
	TotalUsedQuota   int64 `gorm:"column:total_used_quota"`
	TotalRemainQuota int64 `gorm:"column:total_remain_quota"`
	TotalQuotaLimit  int64 `gorm:"column:total_quota_limit"`
}

func NewStatisticsRepository(db *gorm.DB) StatisticsRepository {
	return &statisticsRepositoryImpl{db: db}
}

func (r *statisticsRepositoryImpl) GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error) {
	var summaryRow workspaceQuotaSummaryRow
	summaryQuery := r.db.WithContext(ctx).
		Table("llm_workspace_quotas q").
		Select(`
			COUNT(*) as total_workspaces,
			COALESCE(SUM(CASE WHEN q.quota_limit IS NULL THEN 1 ELSE 0 END), 0) as unlimited_count,
			COALESCE(SUM(q.used_quota), 0) as total_used_quota,
			COALESCE(SUM(q.remain_quota), 0) as total_remain_quota,
			COALESCE(SUM(COALESCE(q.quota_limit, 0)), 0) as total_quota_limit
		`).
		Where("q.organization_id = ?", organizationID)

	if hasText(req.WorkspaceID) {
		summaryQuery = summaryQuery.Where("q.workspace_id = ?", strings.TrimSpace(*req.WorkspaceID))
	}
	if err := summaryQuery.Scan(&summaryRow).Error; err != nil {
		return nil, err
	}

	var items []dto.WorkspaceQuotaItem
	itemsQuery := r.db.WithContext(ctx).
		Table("llm_workspace_quotas q").
		Select(`
			q.workspace_id,
			COALESCE(w.name, '') as workspace_name,
			q.used_quota,
			q.remain_quota,
			q.quota_limit,
			CASE WHEN q.quota_limit IS NULL THEN true ELSE false END as is_unlimited
		`).
		Joins("LEFT JOIN workspaces w ON w.id = q.workspace_id").
		Where("q.organization_id = ?", organizationID)

	if hasText(req.WorkspaceID) {
		itemsQuery = itemsQuery.Where("q.workspace_id = ?", strings.TrimSpace(*req.WorkspaceID))
	}
	if err := itemsQuery.Order("q.used_quota DESC, q.workspace_id ASC").Scan(&items).Error; err != nil {
		return nil, err
	}

	return &dto.WorkspaceQuotaResponse{
		Summary: dto.WorkspaceQuotaSummary{
			TotalWorkspaces:  summaryRow.TotalWorkspaces,
			UnlimitedCount:   summaryRow.UnlimitedCount,
			TotalUsedQuota:   summaryRow.TotalUsedQuota,
			TotalRemainQuota: summaryRow.TotalRemainQuota,
			TotalQuotaLimit:  summaryRow.TotalQuotaLimit,
		},
		Items: items,
	}, nil
}

func hasText(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func column(alias, name string) string {
	if alias == "" {
		return name
	}
	return alias + "." + name
}
