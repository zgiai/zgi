package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
)

// QuotaRepository Quota repository interface
type QuotaRepository interface {
	// CreateUsageHistory Create usage history record
	CreateUsageHistory(ctx context.Context, history *quota_model.QuotaUsageHistory) error

	// CreateUsageHistoryInTx Create usage history record in transaction
	CreateUsageHistoryInTx(ctx context.Context, tx *gorm.DB, history *quota_model.QuotaUsageHistory) error

	// GetCurrentUsage Get current usage by aggregating history records
	GetCurrentUsage(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType) (int64, error)

	// GetUsageHistory Query usage history records
	GetUsageHistory(ctx context.Context, groupID uuid.UUID, resourceType *quota_model.ResourceType, startTime, endTime *time.Time, page, limit int) ([]*quota_model.QuotaUsageHistory, int64, error)

	// GetUsageHistoryByFilter Query usage history records by filter conditions
	GetUsageHistoryByFilter(ctx context.Context, filter map[string]interface{}, page, limit int, orderBy string) ([]*quota_model.QuotaUsageHistory, int64, error)

	// BeginTx Begin transaction
	BeginTx(ctx context.Context) (*gorm.DB, error)
}

type quotaRepository struct {
	db *gorm.DB
}

// NewQuotaRepository Create quota repository instance
func NewQuotaRepository(db *gorm.DB) QuotaRepository {
	return &quotaRepository{db: db}
}

// CreateUsageHistory Create usage history record
func (r *quotaRepository) CreateUsageHistory(ctx context.Context, history *quota_model.QuotaUsageHistory) error {
	// Generate ID
	if history.ID == "" {
		history.ID = uuid.New().String()
	}

	// Set creation time
	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}

	if err := r.db.WithContext(ctx).Create(history).Error; err != nil {
		return fmt.Errorf("failed to create usage history: %w", err)
	}

	return nil
}

// CreateUsageHistoryInTx Create usage history record in transaction
func (r *quotaRepository) CreateUsageHistoryInTx(ctx context.Context, tx *gorm.DB, history *quota_model.QuotaUsageHistory) error {
	// Generate ID
	if history.ID == "" {
		history.ID = uuid.New().String()
	}

	// Set creation time
	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}

	if err := tx.WithContext(ctx).Create(history).Error; err != nil {
		return fmt.Errorf("failed to create usage history in transaction: %w", err)
	}

	return nil
}

// GetCurrentUsage Get current usage by aggregating history records
func (r *quotaRepository) GetCurrentUsage(ctx context.Context, groupID uuid.UUID, resourceType quota_model.ResourceType) (int64, error) {
	var currentUsage int64

	err := r.db.WithContext(ctx).
		Model(&quota_model.QuotaUsageHistory{}).
		Select("COALESCE(SUM(delta), 0)").
		Where("group_id = ? AND resource_type = ?", groupID, resourceType).
		Scan(&currentUsage).Error

	if err != nil {
		return 0, fmt.Errorf("failed to calculate current usage: %w", err)
	}

	return currentUsage, nil
}

// GetUsageHistory Query usage history records
func (r *quotaRepository) GetUsageHistory(ctx context.Context, groupID uuid.UUID, resourceType *quota_model.ResourceType, startTime, endTime *time.Time, page, limit int) ([]*quota_model.QuotaUsageHistory, int64, error) {
	var histories []*quota_model.QuotaUsageHistory
	var total int64

	query := r.db.WithContext(ctx).Model(&quota_model.QuotaUsageHistory{}).Where("group_id = ?", groupID)

	// Add resource type filter
	if resourceType != nil {
		query = query.Where("resource_type = ?", *resourceType)
	}

	// Add time range filter
	if startTime != nil {
		query = query.Where("created_at >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("created_at <= ?", *endTime)
	}

	// Calculate total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count usage history: %w", err)
	}

	// Paginated query
	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&histories).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get usage history: %w", err)
	}

	return histories, total, nil
}

// GetUsageHistoryByFilter Query usage history records by filter conditions
func (r *quotaRepository) GetUsageHistoryByFilter(ctx context.Context, filter map[string]interface{}, page, limit int, orderBy string) ([]*quota_model.QuotaUsageHistory, int64, error) {
	var histories []*quota_model.QuotaUsageHistory
	var total int64

	query := r.db.WithContext(ctx).Model(&quota_model.QuotaUsageHistory{})

	// Apply filter conditions
	for key, value := range filter {
		if value != nil {
			query = query.Where(fmt.Sprintf("%s = ?", key), value)
		}
	}

	// Calculate total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count usage history: %w", err)
	}

	// Set sorting
	if orderBy == "" {
		orderBy = "created_at DESC"
	}

	// Paginated query
	offset := (page - 1) * limit
	if err := query.Order(orderBy).Offset(offset).Limit(limit).Find(&histories).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get usage history: %w", err)
	}

	return histories, total, nil
}

// BeginTx Begin transaction
func (r *quotaRepository) BeginTx(ctx context.Context) (*gorm.DB, error) {
	return r.db.WithContext(ctx).Begin(), nil
}
