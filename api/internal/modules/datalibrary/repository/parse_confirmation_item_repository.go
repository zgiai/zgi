package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type ParseConfirmationItemListFilter struct {
	OrganizationID  string
	AssetID         uuid.UUID
	ProcessingRunID uuid.UUID
	GenerationNo    *int64
	Status          string
	Limit           int
	Offset          int
}

type ParseConfirmationItemResolvePatch struct {
	OrganizationID  string
	AssetID         uuid.UUID
	ProcessingRunID uuid.UUID
	GenerationNo    *int64
	Status          string
	FinalContent    *string
	UpdatedBy       string
	ResolvedAt      *time.Time
	AllowedFrom     []string
}

type ParseConfirmationItemRepository interface {
	Create(ctx context.Context, item *model.ParseConfirmationItem) error
	CreateBatch(ctx context.Context, items []*model.ParseConfirmationItem) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ParseConfirmationItem, error)
	List(ctx context.Context, filter ParseConfirmationItemListFilter) ([]*model.ParseConfirmationItem, int64, error)
	CountPendingByRun(ctx context.Context, organizationID string, assetID uuid.UUID, processingRunID uuid.UUID, generationNo int64) (int64, error)
	Resolve(ctx context.Context, id uuid.UUID, patch ParseConfirmationItemResolvePatch) (*model.ParseConfirmationItem, error)
}

type parseConfirmationItemRepository struct {
	db *gorm.DB
}

func NewParseConfirmationItemRepository(db *gorm.DB) ParseConfirmationItemRepository {
	return &parseConfirmationItemRepository{db: db}
}

func (r *parseConfirmationItemRepository) Create(ctx context.Context, item *model.ParseConfirmationItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *parseConfirmationItemRepository) CreateBatch(ctx context.Context, items []*model.ParseConfirmationItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&items).Error
}

func (r *parseConfirmationItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ParseConfirmationItem, error) {
	var item model.ParseConfirmationItem
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *parseConfirmationItemRepository) List(ctx context.Context, filter ParseConfirmationItemListFilter) ([]*model.ParseConfirmationItem, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ParseConfirmationItem{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.ProcessingRunID != uuid.Nil {
		query = query.Where("processing_run_id = ?", filter.ProcessingRunID)
	}
	if filter.GenerationNo != nil {
		query = query.Where("generation_no = ?", *filter.GenerationNo)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var items []*model.ParseConfirmationItem
	if err := query.Order("created_at ASC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *parseConfirmationItemRepository) CountPendingByRun(ctx context.Context, organizationID string, assetID uuid.UUID, processingRunID uuid.UUID, generationNo int64) (int64, error) {
	if organizationID == "" || assetID == uuid.Nil || processingRunID == uuid.Nil {
		return 0, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ParseConfirmationItem{}).
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("processing_run_id = ?", processingRunID).
		Where("generation_no = ?", generationNo).
		Where("status = ?", model.ParseConfirmationItemStatusPending).
		Where("deleted_at IS NULL").
		Count(&count).Error
	return count, err
}

func (r *parseConfirmationItemRepository) Resolve(ctx context.Context, id uuid.UUID, patch ParseConfirmationItemResolvePatch) (*model.ParseConfirmationItem, error) {
	if id == uuid.Nil {
		return nil, nil
	}
	resolvedAt := patch.ResolvedAt
	if resolvedAt == nil {
		now := time.Now()
		resolvedAt = &now
	}
	updates := map[string]any{
		"status":      patch.Status,
		"resolved_at": *resolvedAt,
		"updated_at":  time.Now(),
	}
	if patch.FinalContent != nil {
		updates["final_content"] = *patch.FinalContent
	}
	if patch.UpdatedBy != "" {
		updates["updated_by"] = patch.UpdatedBy
	}

	query := r.db.WithContext(ctx).Model(&model.ParseConfirmationItem{}).
		Where("id = ?", id).
		Where("deleted_at IS NULL")
	if patch.OrganizationID != "" {
		query = query.Where("organization_id = ?", patch.OrganizationID)
	}
	if patch.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", patch.AssetID)
	}
	if patch.ProcessingRunID != uuid.Nil {
		query = query.Where("processing_run_id = ?", patch.ProcessingRunID)
	}
	if patch.GenerationNo != nil {
		query = query.Where("generation_no = ?", *patch.GenerationNo)
	}
	if len(patch.AllowedFrom) > 0 {
		query = query.Where("status IN ?", patch.AllowedFrom)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}
