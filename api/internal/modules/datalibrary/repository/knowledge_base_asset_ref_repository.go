package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type KnowledgeBaseAssetRefListFilter struct {
	OrganizationID string
	DatasetID      string
	AssetID        uuid.UUID
	VersionID      uuid.UUID
	Status         string
	SyncStatus     string
	Limit          int
	Offset         int
}

type KnowledgeBaseAssetRefRepository interface {
	Create(ctx context.Context, item *model.KnowledgeBaseAssetRef) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	List(ctx context.Context, filter KnowledgeBaseAssetRefListFilter) ([]*model.KnowledgeBaseAssetRef, int64, error)
	FindActive(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error)
	CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error)
	UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error)
	MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error)
	MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
	MarkSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64, syncedAt time.Time) (*model.KnowledgeBaseAssetRef, error)
	MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*model.KnowledgeBaseAssetRef, error)
	SoftDelete(ctx context.Context, organizationID string, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error)
}

type knowledgeBaseAssetRefRepository struct {
	db *gorm.DB
}

func NewKnowledgeBaseAssetRefRepository(db *gorm.DB) KnowledgeBaseAssetRefRepository {
	return &knowledgeBaseAssetRefRepository{db: db}
}

func (r *knowledgeBaseAssetRefRepository) Create(ctx context.Context, item *model.KnowledgeBaseAssetRef) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *knowledgeBaseAssetRefRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	var item model.KnowledgeBaseAssetRef
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *knowledgeBaseAssetRefRepository) List(ctx context.Context, filter KnowledgeBaseAssetRefListFilter) ([]*model.KnowledgeBaseAssetRef, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.DatasetID != "" {
		query = query.Where("dataset_id = ?", filter.DatasetID)
	}
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.VersionID != uuid.Nil {
		query = query.Where("version_id = ?", filter.VersionID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.SyncStatus != "" {
		query = query.Where("sync_status = ?", filter.SyncStatus)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var items []*model.KnowledgeBaseAssetRef
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *knowledgeBaseAssetRefRepository) FindActive(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	var item model.KnowledgeBaseAssetRef
	query := r.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND asset_id = ? AND status = ?",
			organizationID, datasetID, assetID, model.KnowledgeBaseAssetRefStatusActive)
	if versionID != uuid.Nil {
		query = query.Where("version_id = ?", versionID)
	}
	err := query.First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *knowledgeBaseAssetRefRepository) FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	var item model.KnowledgeBaseAssetRef
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND asset_id = ? AND status = ?",
			organizationID, datasetID, assetID, model.KnowledgeBaseAssetRefStatusActive).
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *knowledgeBaseAssetRefRepository) ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error) {
	var items []*model.KnowledgeBaseAssetRef
	err := r.db.WithContext(ctx).
		Model(&model.KnowledgeBaseAssetRef{}).
		Joins("JOIN datasets ON datasets.id = data_library_knowledge_base_asset_refs.dataset_id").
		Where("data_library_knowledge_base_asset_refs.organization_id = ? AND data_library_knowledge_base_asset_refs.asset_id = ? AND data_library_knowledge_base_asset_refs.status = ?",
			organizationID, assetID, model.KnowledgeBaseAssetRefStatusActive).
		Order("data_library_knowledge_base_asset_refs.updated_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *knowledgeBaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Joins("JOIN datasets ON datasets.id = data_library_knowledge_base_asset_refs.dataset_id").
		Where("data_library_knowledge_base_asset_refs.organization_id = ? AND data_library_knowledge_base_asset_refs.asset_id = ? AND data_library_knowledge_base_asset_refs.status = ?",
			organizationID, assetID, model.KnowledgeBaseAssetRefStatusActive).
		Count(&count).Error
	return count, err
}

func (r *knowledgeBaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error) {
	result := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND id = ?", organizationID, id).
		Update("status", status)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}

func (r *knowledgeBaseAssetRefRepository) MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error) {
	updates := map[string]any{
		"sync_status":        model.KnowledgeBaseAssetRefSyncStatusPending,
		"sync_run_id":        syncRunID,
		"sync_error_code":    errorCode,
		"sync_error_message": errorMessage,
	}
	return r.updateByID(ctx, organizationID, id, updates)
}

func (r *knowledgeBaseAssetRefRepository) MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	updates := map[string]any{
		"sync_status": model.KnowledgeBaseAssetRefSyncStatusSyncing,
	}
	return r.updateByIDAndSyncRun(ctx, organizationID, id, syncRunID, updates)
}

func (r *knowledgeBaseAssetRefRepository) MarkSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64, syncedAt time.Time) (*model.KnowledgeBaseAssetRef, error) {
	updates := map[string]any{
		"dataset_document_id":  datasetDocumentID,
		"sync_status":          model.KnowledgeBaseAssetRefSyncStatusSynced,
		"synced_generation_no": generationNo,
		"last_synced_at":       syncedAt,
		"sync_error_code":      nil,
		"sync_error_message":   nil,
	}
	return r.updateByIDAndSyncRun(ctx, organizationID, id, syncRunID, updates)
}

func (r *knowledgeBaseAssetRefRepository) MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*model.KnowledgeBaseAssetRef, error) {
	updates := map[string]any{
		"sync_status":        model.KnowledgeBaseAssetRefSyncStatusFailed,
		"sync_error_code":    errorCode,
		"sync_error_message": errorMessage,
	}
	return r.updateByIDAndSyncRun(ctx, organizationID, id, syncRunID, updates)
}

func (r *knowledgeBaseAssetRefRepository) SoftDelete(ctx context.Context, organizationID string, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	item, err := r.GetByID(ctx, id)
	if err != nil || item == nil {
		return item, err
	}
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", organizationID, id).
		Delete(&model.KnowledgeBaseAssetRef{}).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *knowledgeBaseAssetRefRepository) updateByID(ctx context.Context, organizationID string, id uuid.UUID, updates map[string]any) (*model.KnowledgeBaseAssetRef, error) {
	result := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND id = ?", organizationID, id).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}

func (r *knowledgeBaseAssetRefRepository) updateByIDAndSyncRun(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, updates map[string]any) (*model.KnowledgeBaseAssetRef, error) {
	result := r.db.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND id = ? AND sync_run_id = ?", organizationID, id, syncRunID).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id)
}
