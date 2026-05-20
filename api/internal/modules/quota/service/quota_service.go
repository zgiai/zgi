package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/internal/dto"
	quota_model "github.com/zgiai/ginext/internal/modules/quota/model"
	"github.com/zgiai/ginext/internal/modules/quota/repository"
)

// quotaService Quota service implementation
type quotaService struct {
	repo repository.QuotaRepository
	db   *gorm.DB
}

// NewQuotaService Create quota service instance
func NewQuotaService(repo repository.QuotaRepository, db *gorm.DB) *quotaService {
	return &quotaService{
		repo: repo,
		db:   db,
	}
}

// CheckQuota Check if quota is sufficient
func (s *quotaService) CheckQuota(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType, amount int64) (bool, int64, int64, error) {
	currentUsage, err := s.repo.GetCurrentUsage(ctx, groupID, resourceType)
	if err != nil {
		return false, 0, 0, fmt.Errorf("failed to calculate current usage: %w", err)
	}

	LogQuotaCheck(groupID.String(), resourceType, amount, currentUsage, -1, true)
	return true, currentUsage, -1, nil
}

// GetCurrentUsage Get current usage
func (s *quotaService) GetCurrentUsage(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType) (int64, error) {
	return s.repo.GetCurrentUsage(ctx, groupID, resourceType)
}

// RecordUsage Record usage history
func (s *quotaService) RecordUsage(ctx context.Context, record *quota_model.QuotaUsageHistory) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.recordUsageInternal(ctx, tx, record)
	})
}

// RecordUsageInTx Record usage history in transaction
func (s *quotaService) RecordUsageInTx(ctx context.Context, tx *gorm.DB, record *quota_model.QuotaUsageHistory) error {
	return s.recordUsageInternal(ctx, tx, record)
}

// recordUsageInternal Internal implementation for recording usage history
func (s *quotaService) recordUsageInternal(ctx context.Context, tx *gorm.DB, record *quota_model.QuotaUsageHistory) error {
	if record.ValueBefore == 0 && record.Delta != 0 {
		var currentUsage int64
		err := tx.WithContext(ctx).
			Model(&quota_model.QuotaUsageHistory{}).
			Select("COALESCE(SUM(delta), 0)").
			Where("group_id = ? AND resource_type = ?", record.GroupID, record.ResourceType).
			Scan(&currentUsage).Error

		if err != nil {
			return fmt.Errorf("failed to calculate current usage: %w", err)
		}

		record.ValueBefore = currentUsage
		record.ValueAfter = currentUsage + record.Delta
	} else if record.Delta == 0 {
		record.ValueAfter = record.ValueBefore
	}

	if record.Delta > 0 {
		record.OperationType = quota_model.OperationTypeIncrease
	} else if record.Delta < 0 {
		record.OperationType = quota_model.OperationTypeDecrease
	} else {
		record.OperationType = quota_model.OperationTypeIncrease
	}

	if err := s.repo.CreateUsageHistoryInTx(ctx, tx, record); err != nil {
		return err
	}

	return nil
}

// GetQuotaStatus Get quota usage status
func (s *quotaService) GetQuotaStatus(ctx context.Context, groupID uuid.UUID) (*dto.QuotaStatusDTO, error) {
	status := &dto.QuotaStatusDTO{
		GroupID:   groupID,
		PlanCode:  "none",
		Resources: make(map[string]*dto.ResourceQuotaDTO),
		UpdatedAt: time.Now(),
	}

	resourceTypes := []quota_model.ResourceType{
		quota_model.ResourceTypeSeats,
		quota_model.ResourceTypeStorage,
		quota_model.ResourceTypeDBRows,
		quota_model.ResourceTypeKnowledgeBases,
		quota_model.ResourceTypeAIAgents,
		quota_model.ResourceTypeWorkflows,
		quota_model.ResourceTypeWorkflowExecutions,
		quota_model.ResourceTypeOCRPages,
	}

	for _, resourceType := range resourceTypes {
		used, err := s.repo.GetCurrentUsage(ctx, groupID, resourceType)
		if err != nil {
			used = 0
		}

		status.Resources[string(resourceType)] = &dto.ResourceQuotaDTO{
			Used:      used,
			Limit:     -1,
			Unlimited: true,
			Usage:     0,
			Unit:      resourceType.GetUnit(),
		}
	}

	return status, nil
}

// GetUsageHistory Query usage history
func (s *quotaService) GetUsageHistory(ctx context.Context, filter *dto.QuotaUsageHistoryFilterDTO) (*dto.QuotaUsageHistoryListResponseDTO, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	filterMap := make(map[string]interface{})
	if filter.GroupID != nil {
		filterMap["group_id"] = *filter.GroupID
	}
	if filter.AccountID != nil {
		filterMap["account_id"] = *filter.AccountID
	}
	if filter.TenantID != nil {
		filterMap["tenant_id"] = *filter.TenantID
	}
	if filter.ResourceType != nil {
		filterMap["resource_type"] = *filter.ResourceType
	}
	if filter.OperationType != nil {
		filterMap["operation_type"] = *filter.OperationType
	}
	if filter.ResourceID != nil {
		filterMap["resource_id"] = *filter.ResourceID
	}

	orderBy := "created_at DESC"
	if filter.Sort != "" {
		switch filter.Sort {
		case "created_at_asc":
			orderBy = "created_at ASC"
		case "created_at_desc":
			orderBy = "created_at DESC"
		}
	}

	histories, total, err := s.repo.GetUsageHistoryByFilter(ctx, filterMap, filter.Page, filter.Limit, orderBy)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage history: %w", err)
	}

	items := make([]dto.QuotaUsageHistoryDTO, 0, len(histories))
	for _, history := range histories {
		items = append(items, *dto.ToQuotaUsageHistoryDTO(history))
	}

	return &dto.QuotaUsageHistoryListResponseDTO{
		Items: items,
		Total: total,
		Page:  filter.Page,
		Limit: filter.Limit,
	}, nil
}
