package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

type DocumentAssetListFilter struct {
	OrganizationID string
	WorkspaceID    *string
	Status         string
	ProductStatus  string
	Limit          int
	Offset         int
}

type DocumentAssetCurrentResultPatch struct {
	OrganizationID              string
	ProductStatus               *string
	ProcessingStage             *string
	ProcessingProgress          *int
	ActiveProcessingRequestID   *uuid.UUID
	ProcessingRunID             *uuid.UUID
	GenerationNo                *int64
	ParseArtifactID             *uuid.UUID
	ChunkArtifactSetID          *uuid.UUID
	ChunkCount                  *int
	EmbeddingProvider           *string
	EmbeddingModel              *string
	EmbeddingDimension          *int
	VectorStatus                *string
	LastErrorCode               *string
	LastErrorMessage            *string
	RequireProcessingRunID      *uuid.UUID
	RequireGenerationNo         *int64
	ClearActiveProcessingRunIDs bool
	ClearError                  bool
}

type DocumentAssetRepository interface {
	CreateAsset(ctx context.Context, item *model.DocumentAsset) error
	CreateAssetWithVersion(ctx context.Context, asset *model.DocumentAsset, version *model.DocumentVersion) error
	GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error)
	FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error)
	FindAssetsBySourceFileIDs(ctx context.Context, organizationID string, sourceFileIDs []string) (map[string]*model.DocumentAsset, error)
	ListAssets(ctx context.Context, filter DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error)
	UpdateCurrentResult(ctx context.Context, id uuid.UUID, patch DocumentAssetCurrentResultPatch) (*model.DocumentAsset, error)

	CreateVersion(ctx context.Context, item *model.DocumentVersion) error
	GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error)
	ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error)
}

type documentAssetRepository struct {
	db *gorm.DB
}

func NewDocumentAssetRepository(db *gorm.DB) DocumentAssetRepository {
	return &documentAssetRepository{db: db}
}

func (r *documentAssetRepository) CreateAsset(ctx context.Context, item *model.DocumentAsset) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *documentAssetRepository) CreateAssetWithVersion(ctx context.Context, asset *model.DocumentAsset, version *model.DocumentVersion) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		return tx.Create(version).Error
	})
}

func (r *documentAssetRepository) GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error) {
	var item model.DocumentAsset
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentAssetRepository) FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	var item model.DocumentAsset
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND source_file_id = ?", organizationID, sourceFileID).
		Order("updated_at DESC").
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentAssetRepository) FindAssetsBySourceFileIDs(ctx context.Context, organizationID string, sourceFileIDs []string) (map[string]*model.DocumentAsset, error) {
	result := make(map[string]*model.DocumentAsset, len(sourceFileIDs))
	if organizationID == "" || len(sourceFileIDs) == 0 {
		return result, nil
	}

	var items []*model.DocumentAsset
	if err := r.db.WithContext(ctx).
		Where("organization_id = ? AND source_file_id IN ?", organizationID, sourceFileIDs).
		Order("updated_at DESC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, exists := result[item.SourceFileID]; exists {
			continue
		}
		result[item.SourceFileID] = item
	}
	return result, nil
}

func (r *documentAssetRepository) ListAssets(ctx context.Context, filter DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DocumentAsset{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *filter.WorkspaceID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ProductStatus != "" {
		query = query.Where("product_status = ?", filter.ProductStatus)
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

	var items []*model.DocumentAsset
	if err := query.Order("updated_at DESC, created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *documentAssetRepository) UpdateCurrentResult(ctx context.Context, id uuid.UUID, patch DocumentAssetCurrentResultPatch) (*model.DocumentAsset, error) {
	if id == uuid.Nil {
		return nil, nil
	}
	updates := map[string]any{
		"updated_at": time.Now(),
	}
	if patch.ProductStatus != nil {
		updates["product_status"] = *patch.ProductStatus
	}
	if patch.ProcessingStage != nil {
		updates["processing_stage"] = *patch.ProcessingStage
	}
	if patch.ProcessingProgress != nil {
		updates["processing_progress"] = *patch.ProcessingProgress
	}
	if patch.ActiveProcessingRequestID != nil {
		updates["active_processing_request_id"] = *patch.ActiveProcessingRequestID
	}
	if patch.ProcessingRunID != nil {
		updates["processing_run_id"] = *patch.ProcessingRunID
	}
	if patch.GenerationNo != nil {
		updates["generation_no"] = *patch.GenerationNo
	}
	if patch.ParseArtifactID != nil {
		updates["parse_artifact_id"] = *patch.ParseArtifactID
	}
	if patch.ChunkArtifactSetID != nil {
		updates["chunk_artifact_set_id"] = *patch.ChunkArtifactSetID
	}
	if patch.ChunkCount != nil {
		updates["chunk_count"] = *patch.ChunkCount
	}
	if patch.EmbeddingProvider != nil {
		updates["embedding_provider"] = *patch.EmbeddingProvider
	}
	if patch.EmbeddingModel != nil {
		updates["embedding_model"] = *patch.EmbeddingModel
	}
	if patch.EmbeddingDimension != nil {
		updates["embedding_dimension"] = *patch.EmbeddingDimension
	}
	if patch.VectorStatus != nil {
		updates["vector_status"] = *patch.VectorStatus
	}
	if patch.LastErrorCode != nil {
		updates["last_error_code"] = *patch.LastErrorCode
	}
	if patch.LastErrorMessage != nil {
		updates["last_error_message"] = *patch.LastErrorMessage
	}
	if patch.ClearActiveProcessingRunIDs {
		updates["active_processing_request_id"] = nil
		updates["processing_run_id"] = nil
	}
	if patch.ClearError {
		updates["last_error_code"] = nil
		updates["last_error_message"] = nil
	}

	query := r.db.WithContext(ctx).Model(&model.DocumentAsset{}).
		Where("id = ?", id).
		Where("deleted_at IS NULL")
	if patch.OrganizationID != "" {
		query = query.Where("organization_id = ?", patch.OrganizationID)
	}
	if patch.RequireProcessingRunID != nil {
		query = query.Where("processing_run_id = ?", *patch.RequireProcessingRunID)
	}
	if patch.RequireGenerationNo != nil {
		query = query.Where("generation_no = ?", *patch.RequireGenerationNo)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	return r.GetAssetByID(ctx, id)
}

func (r *documentAssetRepository) CreateVersion(ctx context.Context, item *model.DocumentVersion) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *documentAssetRepository) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error) {
	var item model.DocumentVersion
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *documentAssetRepository) ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error) {
	var items []*model.DocumentVersion
	err := r.db.WithContext(ctx).
		Where("asset_id = ?", assetID).
		Order("version_no DESC, created_at DESC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}
