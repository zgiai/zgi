package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

const fileAssetVectorIndexPageSize = 500

type FileAssetVectorIndexService interface {
	EnsureAssetIndexed(ctx context.Context, asset *model.DocumentAsset) error
	RebuildAssetIndex(ctx context.Context, asset *model.DocumentAsset) (int, error)
	IndexChunkEmbeddings(ctx context.Context, asset *model.DocumentAsset, chunks []*model.DocumentChunk, embeddings []*model.DocumentChunkEmbedding, resetAsset bool) error
	DeleteAssetIndex(ctx context.Context, asset *model.DocumentAsset) error
	DeleteChunkVector(ctx context.Context, asset *model.DocumentAsset, chunkID uuid.UUID) error
	DeleteChildVectorsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error
	Search(ctx context.Context, asset *model.DocumentAsset, queryVector []float64, limit int) ([]map[string]interface{}, error)
}

type vectorClassDeleter interface {
	DeleteClass(ctx context.Context, className string) error
}

type fileAssetVectorIndexService struct {
	chunks     repository.DocumentChunkRepository
	embeddings repository.DocumentChunkEmbeddingRepository
	vectorDB   vectordb.VectorDB
}

func NewFileAssetVectorIndexService(chunks repository.DocumentChunkRepository, embeddings repository.DocumentChunkEmbeddingRepository, vectorDB vectordb.VectorDB) FileAssetVectorIndexService {
	return &fileAssetVectorIndexService{
		chunks:     chunks,
		embeddings: embeddings,
		vectorDB:   vectorDB,
	}
}

func (s *fileAssetVectorIndexService) EnsureAssetIndexed(ctx context.Context, asset *model.DocumentAsset) error {
	_, err := s.indexAssetEmbeddings(ctx, asset, false)
	return err
}

func (s *fileAssetVectorIndexService) RebuildAssetIndex(ctx context.Context, asset *model.DocumentAsset) (int, error) {
	return s.indexAssetEmbeddings(ctx, asset, true)
}

func (s *fileAssetVectorIndexService) indexAssetEmbeddings(ctx context.Context, asset *model.DocumentAsset, resetAsset bool) (int, error) {
	if asset == nil {
		return 0, ErrDocumentAssetNotFound
	}
	if err := s.ensureConfigured(); err != nil {
		return 0, err
	}
	generationNo := asset.GenerationNo
	if generationNo <= 0 {
		return 0, ErrProcessingRunMismatch
	}
	if resetAsset {
		if err := s.DeleteAssetIndex(ctx, asset); err != nil {
			return 0, err
		}
	}
	offset := 0
	indexed := 0
	for {
		filter := repository.DocumentChunkEmbeddingListFilter{
			OrganizationID: asset.OrganizationID,
			AssetID:        asset.ID,
			GenerationNo:   &generationNo,
			Status:         model.DocumentChunkEmbeddingStatusReady,
			Limit:          fileAssetVectorIndexPageSize,
			Offset:         offset,
		}
		if asset.EmbeddingProvider != nil {
			filter.EmbeddingProvider = strings.TrimSpace(*asset.EmbeddingProvider)
		}
		if asset.EmbeddingModel != nil {
			filter.EmbeddingModel = strings.TrimSpace(*asset.EmbeddingModel)
		}
		items, _, err := s.embeddings.List(ctx, filter)
		if err != nil {
			return indexed, err
		}
		if len(items) == 0 {
			return indexed, nil
		}
		chunkIDs := make([]uuid.UUID, 0, len(items))
		for _, item := range items {
			if item != nil && item.ChunkID != uuid.Nil {
				chunkIDs = append(chunkIDs, item.ChunkID)
			}
		}
		chunks, err := s.chunks.ListByIDs(ctx, asset.OrganizationID, chunkIDs)
		if err != nil {
			return indexed, err
		}
		chunksByID := make(map[uuid.UUID]*model.DocumentChunk, len(chunks))
		parentIDs := make([]uuid.UUID, 0, len(chunks))
		for _, chunk := range chunks {
			chunksByID[chunk.ID] = chunk
			if chunk.ParentChunkID != nil && *chunk.ParentChunkID != uuid.Nil {
				parentIDs = append(parentIDs, *chunk.ParentChunkID)
			}
		}
		parents, err := s.chunks.ListByIDs(ctx, asset.OrganizationID, parentIDs)
		if err != nil {
			return indexed, err
		}
		parentsByID := make(map[uuid.UUID]*model.DocumentChunk, len(parents))
		for _, parent := range parents {
			parentsByID[parent.ID] = parent
		}
		selectedChunks := make([]*model.DocumentChunk, 0, len(items))
		selectedEmbeddings := make([]*model.DocumentChunkEmbedding, 0, len(items))
		for _, item := range items {
			chunk := chunksByID[item.ChunkID]
			var parent *model.DocumentChunk
			if chunk != nil && chunk.ParentChunkID != nil {
				parent = parentsByID[*chunk.ParentChunkID]
			}
			if isVectorIndexableChildChunk(asset, chunk, parent, item) {
				selectedChunks = append(selectedChunks, chunk)
				selectedEmbeddings = append(selectedEmbeddings, item)
			}
		}
		if err := s.IndexChunkEmbeddings(ctx, asset, selectedChunks, selectedEmbeddings, false); err != nil {
			return indexed, err
		}
		indexed += len(selectedEmbeddings)
		if len(items) < fileAssetVectorIndexPageSize {
			return indexed, nil
		}
		offset += len(items)
	}
}

func (s *fileAssetVectorIndexService) IndexChunkEmbeddings(ctx context.Context, asset *model.DocumentAsset, chunks []*model.DocumentChunk, embeddings []*model.DocumentChunkEmbedding, resetAsset bool) error {
	if asset == nil {
		return ErrDocumentAssetNotFound
	}
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	className := FileAssetVectorCollectionName(asset.ID)
	if err := s.ensureClass(ctx, className); err != nil {
		return err
	}
	if resetAsset {
		if err := s.deleteAssetVectors(ctx, asset); err != nil {
			return err
		}
	}
	if len(chunks) == 0 || len(embeddings) == 0 {
		return nil
	}
	chunksByID := make(map[uuid.UUID]*model.DocumentChunk, len(chunks))
	for _, chunk := range chunks {
		if chunk != nil {
			chunksByID[chunk.ID] = chunk
		}
	}
	objects := make([]vectordb.VectorObject, 0, len(embeddings))
	for _, item := range embeddings {
		if item == nil {
			continue
		}
		chunk := chunksByID[item.ChunkID]
		if !isVectorIndexableChildChunkBasic(asset, chunk, item) {
			continue
		}
		parentID := ""
		if chunk.ParentChunkID != nil {
			parentID = chunk.ParentChunkID.String()
		}
		objects = append(objects, vectordb.VectorObject{
			ID:    chunk.ID.String(),
			Class: className,
			Properties: map[string]interface{}{
				"text":        chunk.Content,
				"doc_id":      chunk.ID.String(),
				"document_id": parentID,
				"dataset_id":  asset.ID.String(),
				"doc_hash":    chunk.ContentHash,
			},
			Vector: float32ArrayTo64(item.EmbeddingVector),
		})
	}
	if len(objects) == 0 {
		return nil
	}
	if batchDB, ok := s.vectorDB.(vectordb.BatchVectorDB); ok {
		return batchDB.StoreVectors(ctx, objects)
	}
	for _, object := range objects {
		if err := s.vectorDB.StoreVector(ctx, object.ID, object.Class, object.Properties, object.Vector); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileAssetVectorIndexService) DeleteChunkVector(ctx context.Context, asset *model.DocumentAsset, chunkID uuid.UUID) error {
	if asset == nil || chunkID == uuid.Nil {
		return nil
	}
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	return s.vectorDB.DeleteVector(ctx, chunkID.String(), FileAssetVectorCollectionName(asset.ID))
}

func (s *fileAssetVectorIndexService) DeleteAssetIndex(ctx context.Context, asset *model.DocumentAsset) error {
	if asset == nil {
		return nil
	}
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	className := FileAssetVectorCollectionName(asset.ID)
	if deleter, ok := s.vectorDB.(vectorClassDeleter); ok {
		return deleter.DeleteClass(ctx, className)
	}
	return s.deleteAssetVectors(ctx, asset)
}

func (s *fileAssetVectorIndexService) DeleteChildVectorsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	if asset == nil || parentChunkID == uuid.Nil {
		return nil
	}
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	className := FileAssetVectorCollectionName(asset.ID)
	if deleter, ok := s.vectorDB.(vectordb.FieldDeleteVectorDB); ok {
		if err := deleter.DeleteObjectsByField(ctx, className, "document_id", parentChunkID.String()); err == nil {
			return nil
		}
	}
	return s.deleteChildVectorsByParentIndividually(ctx, asset, parentChunkID)
}

func (s *fileAssetVectorIndexService) deleteChildVectorsByParentIndividually(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	generationNo := asset.GenerationNo
	children, _, err := s.chunks.List(ctx, repository.DocumentChunkListFilter{
		OrganizationID: asset.OrganizationID,
		AssetID:        asset.ID,
		GenerationNo:   &generationNo,
		ParentChunkID:  &parentChunkID,
		ChunkTypes:     []string{model.DocumentChunkTypeChild},
		Limit:          fileAssetVectorIndexPageSize,
		Offset:         0,
	})
	if err != nil {
		return err
	}
	for _, child := range children {
		if child == nil {
			continue
		}
		if err := s.DeleteChunkVector(ctx, asset, child.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileAssetVectorIndexService) Search(ctx context.Context, asset *model.DocumentAsset, queryVector []float64, limit int) ([]map[string]interface{}, error) {
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if len(queryVector) == 0 {
		return nil, ErrDocumentChunkEmbeddingsRequired
	}
	if limit <= 0 {
		limit = 24
	}
	return s.vectorDB.SearchVectors(ctx, FileAssetVectorCollectionName(asset.ID), queryVector, limit)
}

func (s *fileAssetVectorIndexService) ensureConfigured() error {
	if s == nil || s.vectorDB == nil {
		return ErrEmbeddingServiceRequired
	}
	return nil
}

func (s *fileAssetVectorIndexService) ensureClass(ctx context.Context, className string) error {
	return s.vectorDB.CreateClass(ctx, className, []map[string]interface{}{
		{"name": "text", "dataType": []string{"text"}},
		{"name": "doc_id", "dataType": []string{"text"}},
		{"name": "document_id", "dataType": []string{"text"}},
		{"name": "dataset_id", "dataType": []string{"text"}},
		{"name": "doc_hash", "dataType": []string{"text"}},
	})
}

func (s *fileAssetVectorIndexService) deleteAssetVectors(ctx context.Context, asset *model.DocumentAsset) error {
	offset := 0
	for {
		items, _, err := s.embeddings.List(ctx, repository.DocumentChunkEmbeddingListFilter{
			OrganizationID: asset.OrganizationID,
			AssetID:        asset.ID,
			Limit:          fileAssetVectorIndexPageSize,
			Offset:         offset,
		})
		if err != nil {
			return err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := s.DeleteChunkVector(ctx, asset, item.ChunkID); err != nil {
				return err
			}
		}
		if len(items) < fileAssetVectorIndexPageSize {
			return nil
		}
		offset += len(items)
	}
}

func isVectorIndexableChildChunk(asset *model.DocumentAsset, chunk *model.DocumentChunk, parent *model.DocumentChunk, item *model.DocumentChunkEmbedding) bool {
	if asset == nil || chunk == nil || parent == nil || item == nil {
		return false
	}
	return chunk.OrganizationID == asset.OrganizationID &&
		chunk.AssetID == asset.ID &&
		chunk.GenerationNo == asset.GenerationNo &&
		chunk.ChunkType == model.DocumentChunkTypeChild &&
		parent.OrganizationID == asset.OrganizationID &&
		parent.AssetID == asset.ID &&
		parent.GenerationNo == asset.GenerationNo &&
		parent.ChunkType == model.DocumentChunkTypeParent &&
		parent.Enabled &&
		parent.Status == model.DocumentChunkStatusReady &&
		chunk.Enabled &&
		chunk.Status == model.DocumentChunkStatusReady &&
		item.OrganizationID == asset.OrganizationID &&
		item.AssetID == asset.ID &&
		item.ChunkID == chunk.ID &&
		item.GenerationNo == asset.GenerationNo &&
		item.Status == model.DocumentChunkEmbeddingStatusReady &&
		len(item.EmbeddingVector) > 0 &&
		strings.TrimSpace(chunk.Content) != ""
}

func isVectorIndexableChildChunkBasic(asset *model.DocumentAsset, chunk *model.DocumentChunk, item *model.DocumentChunkEmbedding) bool {
	if asset == nil || chunk == nil || item == nil {
		return false
	}
	return chunk.OrganizationID == asset.OrganizationID &&
		chunk.AssetID == asset.ID &&
		chunk.GenerationNo == asset.GenerationNo &&
		chunk.ChunkType == model.DocumentChunkTypeChild &&
		chunk.Enabled &&
		chunk.Status == model.DocumentChunkStatusReady &&
		item.OrganizationID == asset.OrganizationID &&
		item.AssetID == asset.ID &&
		item.ChunkID == chunk.ID &&
		item.GenerationNo == asset.GenerationNo &&
		item.Status == model.DocumentChunkEmbeddingStatusReady &&
		len(item.EmbeddingVector) > 0 &&
		strings.TrimSpace(chunk.Content) != ""
}

func FileAssetVectorCollectionName(assetID uuid.UUID) string {
	return fmt.Sprintf("File_asset_%s_Chunk", strings.ReplaceAll(assetID.String(), "-", "_"))
}

func float32ArrayTo64(values model.Float32Array) []float64 {
	out := make([]float64, len(values))
	for i, value := range values {
		out[i] = float64(value)
	}
	return out
}
