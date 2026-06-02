package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/gorm"
)

var ErrFileAssetDeletionBlocked = errors.New("file asset is referenced")

type FileAssetDeletionBlockedError struct {
	AssetID           uuid.UUID
	KnowledgeBaseRefs int64
	DatabaseRefs      int64
}

func (e *FileAssetDeletionBlockedError) Error() string {
	if e == nil {
		return ErrFileAssetDeletionBlocked.Error()
	}
	return fmt.Sprintf("file asset %s is referenced by %d knowledge base refs and %d database refs", e.AssetID, e.KnowledgeBaseRefs, e.DatabaseRefs)
}

func (e *FileAssetDeletionBlockedError) Is(target error) bool {
	return target == ErrFileAssetDeletionBlocked
}

type FileAssetDeletionService interface {
	DeleteBySourceFile(ctx context.Context, organizationID string, sourceFileID string) error
}

type fileAssetDeletionService struct {
	db          *gorm.DB
	vectorIndex FileAssetVectorIndexService
}

func NewFileAssetDeletionService(db *gorm.DB, vectorIndex FileAssetVectorIndexService) FileAssetDeletionService {
	return &fileAssetDeletionService{
		db:          db,
		vectorIndex: vectorIndex,
	}
}

func (s *fileAssetDeletionService) DeleteBySourceFile(ctx context.Context, organizationID string, sourceFileID string) error {
	if s == nil || s.db == nil || organizationID == "" || sourceFileID == "" {
		return nil
	}

	asset, err := s.findAsset(ctx, organizationID, sourceFileID)
	if err != nil || asset == nil {
		return err
	}

	kbRefCount, dbRefCount, err := s.countBlockingRefs(ctx, asset)
	if err != nil {
		return err
	}
	if kbRefCount > 0 || dbRefCount > 0 {
		return &FileAssetDeletionBlockedError{
			AssetID:           asset.ID,
			KnowledgeBaseRefs: kbRefCount,
			DatabaseRefs:      dbRefCount,
		}
	}

	if s.vectorIndex != nil {
		if err := s.vectorIndex.DeleteAssetIndex(ctx, asset); err != nil {
			return fmt.Errorf("failed to delete file asset vector index: %w", err)
		}
	}

	return s.deleteAssetRows(ctx, asset)
}

func (s *fileAssetDeletionService) findAsset(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	var asset model.DocumentAsset
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND source_file_id = ?", organizationID, sourceFileID).
		Order("updated_at DESC").
		First(&asset).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &asset, nil
}

func (s *fileAssetDeletionService) countBlockingRefs(ctx context.Context, asset *model.DocumentAsset) (int64, int64, error) {
	var kbRefCount int64
	if err := s.db.WithContext(ctx).
		Model(&model.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND asset_id = ? AND deleted_at IS NULL", asset.OrganizationID, asset.ID).
		Count(&kbRefCount).Error; err != nil {
		return 0, 0, err
	}

	var dbRefCount int64
	if err := s.db.WithContext(ctx).
		Model(&model.DatabaseAssetRef{}).
		Where("organization_id = ? AND asset_id = ? AND deleted_at IS NULL", asset.OrganizationID, asset.ID).
		Count(&dbRefCount).Error; err != nil {
		return 0, 0, err
	}
	return kbRefCount, dbRefCount, nil
}

func (s *fileAssetDeletionService) deleteAssetRows(ctx context.Context, asset *model.DocumentAsset) error {
	parseArtifactIDs, chunkArtifactSetIDs, err := s.collectArtifactIDs(ctx, asset)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		nowExpr := tx.NowFunc()
		if err := tx.Model(&model.ProcessingRequest{}).
			Where("asset_id = ? AND status IN ?", asset.ID, []string{
				model.ProcessingRequestStatusPlanned,
				model.ProcessingRequestStatusQueued,
				model.ProcessingRequestStatusRunning,
			}).
			Updates(map[string]any{
				"status":       model.ProcessingRequestStatusCancelled,
				"cancelled_at": nowExpr,
				"updated_at":   nowExpr,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.DocumentAsset{}).
			Where("id = ?", asset.ID).
			Updates(map[string]any{
				"current_version_id":           nil,
				"active_processing_request_id": nil,
				"parse_artifact_id":            nil,
				"chunk_artifact_set_id":        nil,
			}).Error; err != nil {
			return err
		}

		deleteByAsset := []any{
			&model.DocumentChunkEmbedding{},
			&model.DocumentChunk{},
			&model.ParseConfirmationItem{},
			&model.ProcessingRequest{},
			&model.KnowledgeBaseAssetRef{},
			&model.DatabaseAssetRef{},
			&model.ReuseEvent{},
			&model.VectorArtifact{},
			&model.ExtractionArtifact{},
			&model.DocumentVersion{},
		}
		for _, item := range deleteByAsset {
			if err := tx.Unscoped().Where("asset_id = ?", asset.ID).Delete(item).Error; err != nil {
				return err
			}
		}

		if err := tx.Unscoped().
			Where("id = ?", asset.ID).
			Delete(&model.DocumentAsset{}).Error; err != nil {
			return err
		}

		if len(chunkArtifactSetIDs) > 0 {
			if err := tx.Unscoped().
				Where("chunk_artifact_set_id IN ?", chunkArtifactSetIDs).
				Delete(&contentparsemodel.ChunkingRun{}).Error; err != nil {
				return err
			}
		}
		if len(parseArtifactIDs) > 0 {
			if err := tx.Unscoped().
				Where("artifact_id IN ?", parseArtifactIDs).
				Delete(&contentparsemodel.ParseRun{}).Error; err != nil {
				return err
			}
		}
		if err := s.deleteUnreferencedChunkArtifactSets(ctx, tx, chunkArtifactSetIDs); err != nil {
			return err
		}
		return s.deleteUnreferencedParseArtifacts(ctx, tx, parseArtifactIDs)
	})
}

func (s *fileAssetDeletionService) collectArtifactIDs(ctx context.Context, asset *model.DocumentAsset) ([]uuid.UUID, []uuid.UUID, error) {
	parseIDs := make(map[uuid.UUID]struct{})
	chunkSetIDs := make(map[uuid.UUID]struct{})
	if asset.ParseArtifactID != nil && *asset.ParseArtifactID != uuid.Nil {
		parseIDs[*asset.ParseArtifactID] = struct{}{}
	}
	if asset.ChunkArtifactSetID != nil && *asset.ChunkArtifactSetID != uuid.Nil {
		chunkSetIDs[*asset.ChunkArtifactSetID] = struct{}{}
	}

	var versions []*model.DocumentVersion
	if err := s.db.WithContext(ctx).Unscoped().
		Where("asset_id = ?", asset.ID).
		Find(&versions).Error; err != nil {
		return nil, nil, err
	}
	for _, version := range versions {
		if version.ParseArtifactID != nil && *version.ParseArtifactID != uuid.Nil {
			parseIDs[*version.ParseArtifactID] = struct{}{}
		}
		if version.ChunkArtifactSetID != nil && *version.ChunkArtifactSetID != uuid.Nil {
			chunkSetIDs[*version.ChunkArtifactSetID] = struct{}{}
		}
	}

	var chunks []*model.DocumentChunk
	if err := s.db.WithContext(ctx).Unscoped().
		Where("asset_id = ?", asset.ID).
		Find(&chunks).Error; err != nil {
		return nil, nil, err
	}
	for _, chunk := range chunks {
		if chunk.ChunkArtifactSetID != nil && *chunk.ChunkArtifactSetID != uuid.Nil {
			chunkSetIDs[*chunk.ChunkArtifactSetID] = struct{}{}
		}
	}

	var vectorArtifacts []*model.VectorArtifact
	if err := s.db.WithContext(ctx).Unscoped().
		Where("asset_id = ?", asset.ID).
		Find(&vectorArtifacts).Error; err != nil {
		return nil, nil, err
	}
	for _, artifact := range vectorArtifacts {
		if artifact.ChunkArtifactSetID != uuid.Nil {
			chunkSetIDs[artifact.ChunkArtifactSetID] = struct{}{}
		}
	}

	var extractionArtifacts []*model.ExtractionArtifact
	if err := s.db.WithContext(ctx).Unscoped().
		Where("asset_id = ?", asset.ID).
		Find(&extractionArtifacts).Error; err != nil {
		return nil, nil, err
	}
	for _, artifact := range extractionArtifacts {
		if artifact.ParseArtifactID != nil && *artifact.ParseArtifactID != uuid.Nil {
			parseIDs[*artifact.ParseArtifactID] = struct{}{}
		}
	}

	return uuidSetToSlice(parseIDs), uuidSetToSlice(chunkSetIDs), nil
}

func (s *fileAssetDeletionService) deleteUnreferencedChunkArtifactSets(ctx context.Context, tx *gorm.DB, ids []uuid.UUID) error {
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if referenced, err := chunkArtifactSetReferenced(ctx, tx, id); err != nil || referenced {
			return err
		}
		if err := tx.Unscoped().
			Where("id = ?", id).
			Delete(&contentparsemodel.ChunkArtifactSet{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *fileAssetDeletionService) deleteUnreferencedParseArtifacts(ctx context.Context, tx *gorm.DB, ids []uuid.UUID) error {
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if referenced, err := parseArtifactReferenced(ctx, tx, id); err != nil || referenced {
			return err
		}
		if err := tx.Unscoped().
			Where("id = ?", id).
			Delete(&contentparsemodel.Artifact{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func parseArtifactReferenced(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error) {
	tables := []struct {
		table string
		col   string
	}{
		{"data_library_document_assets", "parse_artifact_id"},
		{"data_library_document_versions", "parse_artifact_id"},
		{"data_library_extraction_artifacts", "parse_artifact_id"},
		{"data_library_database_asset_refs", "parse_artifact_id"},
		{"content_parse_chunk_artifact_sets", "parse_artifact_id"},
		{"content_parse_runs", "artifact_id"},
	}
	return anyReferenceExists(ctx, tx, tables, id)
}

func chunkArtifactSetReferenced(ctx context.Context, tx *gorm.DB, id uuid.UUID) (bool, error) {
	tables := []struct {
		table string
		col   string
	}{
		{"data_library_document_assets", "chunk_artifact_set_id"},
		{"data_library_document_versions", "chunk_artifact_set_id"},
		{"data_library_document_chunks", "chunk_artifact_set_id"},
		{"data_library_vector_artifacts", "chunk_artifact_set_id"},
		{"data_library_knowledge_base_asset_refs", "chunk_artifact_set_id"},
		{"content_parse_chunking_runs", "chunk_artifact_set_id"},
	}
	return anyReferenceExists(ctx, tx, tables, id)
}

func anyReferenceExists(ctx context.Context, tx *gorm.DB, tables []struct {
	table string
	col   string
}, id uuid.UUID) (bool, error) {
	for _, ref := range tables {
		var count int64
		if err := tx.WithContext(ctx).
			Table(ref.table).
			Where(fmt.Sprintf("%s = ?", ref.col), id).
			Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func uuidSetToSlice(values map[uuid.UUID]struct{}) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}
