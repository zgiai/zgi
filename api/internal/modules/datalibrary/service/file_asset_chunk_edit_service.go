package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrFileChunkEditNotAllowed = errors.New("file chunk edit is not allowed")
)

type FileAssetChunkEditService interface {
	UpdateCurrentFileChunk(ctx context.Context, input FileAssetChunkEditInput) (*FileAssetChunkEditResult, error)
	SetDatasetRefSyncEnqueuer(enqueuer FileAssetChunkEditDatasetRefSyncEnqueuer)
}

type FileAssetChunkEditInput struct {
	OrganizationID    string
	SourceFileID      string
	ChunkID           uuid.UUID
	Content           *string
	Enabled           *bool
	UpdatedBy         string
	EmbeddingProvider string
	EmbeddingModel    string
}

type FileAssetChunkEditResult struct {
	Asset          *model.DocumentAsset          `json:"asset"`
	Chunk          *model.DocumentChunk          `json:"chunk"`
	Embedding      *model.DocumentChunkEmbedding `json:"embedding,omitempty"`
	EmbeddingReady bool                          `json:"embedding_ready"`
}

type fileAssetChunkEditService struct {
	assets      repository.DocumentAssetRepository
	chunks      repository.DocumentChunkRepository
	embeddings  repository.DocumentChunkEmbeddingRepository
	chunkEmbed  DocumentChunkEmbeddingService
	vectorIndex FileAssetVectorIndexService
	refs        fileAssetChunkEditRefStore
	documents   fileAssetChunkEditDocumentStore
	refSync     FileAssetChunkEditDatasetRefSyncEnqueuer
}

func NewFileAssetChunkEditService(
	assets repository.DocumentAssetRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	chunkEmbed DocumentChunkEmbeddingService,
	vectorIndex ...FileAssetVectorIndexService,
) FileAssetChunkEditService {
	var vectorIndexService FileAssetVectorIndexService
	if len(vectorIndex) > 0 {
		vectorIndexService = vectorIndex[0]
	}
	return newFileAssetChunkEditService(assets, chunks, embeddings, chunkEmbed, vectorIndexService, nil, nil, nil)
}

func NewFileAssetChunkEditServiceWithDatasetRefs(
	assets repository.DocumentAssetRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	chunkEmbed DocumentChunkEmbeddingService,
	vectorIndex FileAssetVectorIndexService,
	refs fileAssetChunkEditRefStore,
	documents fileAssetChunkEditDocumentStore,
	refSync FileAssetChunkEditDatasetRefSyncEnqueuer,
) FileAssetChunkEditService {
	return newFileAssetChunkEditService(assets, chunks, embeddings, chunkEmbed, vectorIndex, refs, documents, refSync)
}

func newFileAssetChunkEditService(
	assets repository.DocumentAssetRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	chunkEmbed DocumentChunkEmbeddingService,
	vectorIndex FileAssetVectorIndexService,
	refs fileAssetChunkEditRefStore,
	documents fileAssetChunkEditDocumentStore,
	refSync FileAssetChunkEditDatasetRefSyncEnqueuer,
) FileAssetChunkEditService {
	return &fileAssetChunkEditService{
		assets:      assets,
		chunks:      chunks,
		embeddings:  embeddings,
		chunkEmbed:  chunkEmbed,
		vectorIndex: vectorIndex,
		refs:        refs,
		documents:   documents,
		refSync:     refSync,
	}
}

type fileAssetChunkEditRefStore interface {
	ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error)
	MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error)
}

type fileAssetChunkEditDocumentStore interface {
	DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error
}

type FileAssetChunkEditDatasetRefSyncEnqueuer interface {
	EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error
}

func (s *fileAssetChunkEditService) SetDatasetRefSyncEnqueuer(enqueuer FileAssetChunkEditDatasetRefSyncEnqueuer) {
	if s == nil {
		return
	}
	s.refSync = enqueuer
}

func (s *fileAssetChunkEditService) UpdateCurrentFileChunk(ctx context.Context, input FileAssetChunkEditInput) (*FileAssetChunkEditResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.SourceFileID == "" {
		return nil, ErrSourceFileIDRequired
	}
	if input.ChunkID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	asset, err := s.assets.FindAssetBySourceFileID(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.GenerationNo <= 0 || asset.ProcessingRunID == nil {
		return nil, ErrProcessingRunMismatch
	}
	chunk, err := s.chunks.GetByID(ctx, input.ChunkID)
	if err != nil {
		return nil, err
	}
	if chunk == nil || chunk.OrganizationID != input.OrganizationID || chunk.AssetID != asset.ID {
		return nil, ErrDocumentAssetNotFound
	}
	if chunk.GenerationNo != asset.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}
	if !isEditableChunkUpdateAllowed(chunk, input) {
		return nil, ErrFileChunkEditNotAllowed
	}

	patch := repository.DocumentChunkPatch{
		OrganizationID: input.OrganizationID,
		UpdatedBy:      input.UpdatedBy,
	}
	if input.Content != nil {
		content := strings.TrimSpace(*input.Content)
		patch.Content = &content
		contentHash := documentChunkContentHash(content)
		patch.ContentHash = &contentHash
	}
	if input.Enabled != nil {
		patch.Enabled = input.Enabled
	}
	updatedChunk, err := s.chunks.Update(ctx, chunk.ID, patch)
	if err != nil {
		return nil, err
	}
	embeddingResult, embeddingReady, err := s.syncEditedChunkEmbedding(ctx, asset, chunk, updatedChunk, input)
	if err != nil {
		return nil, err
	}
	if err := s.enqueueDatasetRefSyncsForAssetEdit(ctx, asset, input.UpdatedBy); err != nil {
		return nil, err
	}
	return &FileAssetChunkEditResult{
		Asset:          asset,
		Chunk:          updatedChunk,
		Embedding:      embeddingResult,
		EmbeddingReady: embeddingReady,
	}, nil
}

func (s *fileAssetChunkEditService) syncEditedChunkEmbedding(ctx context.Context, asset *model.DocumentAsset, originalChunk *model.DocumentChunk, updatedChunk *model.DocumentChunk, input FileAssetChunkEditInput) (*model.DocumentChunkEmbedding, bool, error) {
	if updatedChunk == nil {
		return nil, false, nil
	}
	if updatedChunk.ChunkType == model.DocumentChunkTypeParent {
		if input.Content != nil {
			embeddingReady, err := s.rebuildParentChildChunks(ctx, asset, originalChunk, updatedChunk, input)
			return nil, embeddingReady, err
		}
		if err := s.syncChildEnabledWithParent(ctx, asset, updatedChunk, input); err != nil {
			return nil, false, err
		}
		if s.vectorIndex == nil || input.Enabled == nil {
			return nil, false, nil
		}
		if updatedChunk.Enabled {
			return nil, false, s.vectorIndex.EnsureAssetIndexed(ctx, asset)
		}
		return nil, false, s.vectorIndex.DeleteChildVectorsByParent(ctx, asset, updatedChunk.ID)
	}
	if !updatedChunk.Enabled || updatedChunk.Status != model.DocumentChunkStatusReady {
		if s.vectorIndex != nil {
			if err := s.vectorIndex.DeleteChunkVector(ctx, asset, updatedChunk.ID); err != nil {
				return nil, false, err
			}
		}
		if s.embeddings != nil {
			if err := s.embeddings.DeleteByChunkID(ctx, input.OrganizationID, updatedChunk.ID); err != nil {
				return nil, false, err
			}
		}
		return nil, false, nil
	}
	if input.Content == nil {
		if s.vectorIndex != nil && input.Enabled != nil && updatedChunk.Enabled {
			return nil, false, s.vectorIndex.EnsureAssetIndexed(ctx, asset)
		}
		return nil, false, nil
	}
	if s.chunkEmbed != nil {
		embeddingProvider, embeddingModel := resolveChunkEditEmbeddingModel(asset, input)
		embeddingResult, err := s.chunkEmbed.GenerateChunkEmbedding(ctx, GenerateDocumentChunkEmbeddingInput{
			OrganizationID:    input.OrganizationID,
			AssetID:           asset.ID,
			ProcessingRunID:   *asset.ProcessingRunID,
			GenerationNo:      asset.GenerationNo,
			EmbeddingProvider: embeddingProvider,
			EmbeddingModel:    embeddingModel,
			RequestedBy:       input.UpdatedBy,
			Chunk:             updatedChunk,
		})
		if err != nil {
			return nil, false, err
		}
		return embeddingResult, true, nil
	}
	return nil, false, nil
}

func (s *fileAssetChunkEditService) rebuildParentChildChunks(ctx context.Context, asset *model.DocumentAsset, originalChunk *model.DocumentChunk, updatedChunk *model.DocumentChunk, input FileAssetChunkEditInput) (bool, error) {
	if asset == nil || originalChunk == nil || updatedChunk == nil {
		return false, nil
	}
	if s.vectorIndex != nil {
		if err := s.vectorIndex.DeleteChildVectorsByParent(ctx, asset, updatedChunk.ID); err != nil {
			return false, err
		}
	}
	if err := s.deleteChildEmbeddingsByParent(ctx, asset, updatedChunk.ID); err != nil {
		return false, err
	}
	if err := s.chunks.DeleteChildrenByParent(ctx, input.OrganizationID, updatedChunk.ID); err != nil {
		return false, err
	}

	parentDTO := dto.TransformedChunk{
		Content:  updatedChunk.Content,
		Metadata: cloneAnyMap(originalChunk.MetadataJSON),
	}
	children := splitDocumentChildChunks(parentDTO, updatedChunk.Content, parseDocumentSubchunkRule(nil))
	items := make([]*model.DocumentChunk, 0, len(children))
	for index, child := range children {
		childContent := strings.TrimSpace(child.Content)
		if childContent == "" {
			continue
		}
		item := newDocumentChunk(
			asset,
			GenerateDocumentChunksInput{
				OrganizationID:     input.OrganizationID,
				AssetID:            asset.ID,
				ProcessingRunID:    updatedChunk.ProcessingRunID,
				GenerationNo:       updatedChunk.GenerationNo,
				ChunkArtifactSetID: updatedChunk.ChunkArtifactSetID,
				CreatedBy:          input.UpdatedBy,
			},
			uuid.New(),
			&updatedChunk.ID,
			index,
			model.DocumentChunkTypeChild,
			childContent,
			child.BBox,
			child.Metadata,
		)
		item.Enabled = updatedChunk.Enabled
		items = append(items, item)
	}
	if len(items) == 0 {
		return false, nil
	}
	if err := s.chunks.CreateBatch(ctx, items); err != nil {
		return false, err
	}
	if !updatedChunk.Enabled || s.chunkEmbed == nil {
		return false, nil
	}
	embeddingProvider, embeddingModel := resolveChunkEditEmbeddingModel(asset, input)
	for _, item := range items {
		if _, err := s.chunkEmbed.GenerateChunkEmbedding(ctx, GenerateDocumentChunkEmbeddingInput{
			OrganizationID:    input.OrganizationID,
			AssetID:           asset.ID,
			ProcessingRunID:   updatedChunk.ProcessingRunID,
			GenerationNo:      updatedChunk.GenerationNo,
			EmbeddingProvider: embeddingProvider,
			EmbeddingModel:    embeddingModel,
			RequestedBy:       input.UpdatedBy,
			Chunk:             item,
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *fileAssetChunkEditService) deleteChildEmbeddingsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	if s.embeddings == nil || asset == nil || parentChunkID == uuid.Nil {
		return nil
	}
	children, _, err := s.chunks.List(ctx, repository.DocumentChunkListFilter{
		OrganizationID: asset.OrganizationID,
		AssetID:        asset.ID,
		GenerationNo:   &asset.GenerationNo,
		ParentChunkID:  &parentChunkID,
		ChunkTypes:     []string{model.DocumentChunkTypeChild},
		Limit:          500,
		Offset:         0,
	})
	if err != nil {
		return err
	}
	for _, child := range children {
		if child == nil {
			continue
		}
		if err := s.embeddings.DeleteByChunkID(ctx, asset.OrganizationID, child.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *fileAssetChunkEditService) syncChildEnabledWithParent(ctx context.Context, asset *model.DocumentAsset, parentChunk *model.DocumentChunk, input FileAssetChunkEditInput) error {
	if input.Enabled == nil || asset == nil || parentChunk == nil {
		return nil
	}
	children, _, err := s.chunks.List(ctx, repository.DocumentChunkListFilter{
		OrganizationID: asset.OrganizationID,
		AssetID:        asset.ID,
		GenerationNo:   &asset.GenerationNo,
		ParentChunkID:  &parentChunk.ID,
		ChunkTypes:     []string{model.DocumentChunkTypeChild},
		Limit:          500,
		Offset:         0,
	})
	if err != nil {
		return err
	}
	for _, child := range children {
		if child == nil || child.Enabled == *input.Enabled {
			continue
		}
		if _, err := s.chunks.Update(ctx, child.ID, repository.DocumentChunkPatch{
			OrganizationID: input.OrganizationID,
			Enabled:        input.Enabled,
			UpdatedBy:      input.UpdatedBy,
		}); err != nil {
			return err
		}
	}
	return nil
}

func resolveChunkEditEmbeddingModel(asset *model.DocumentAsset, input FileAssetChunkEditInput) (string, string) {
	provider := strings.TrimSpace(input.EmbeddingProvider)
	modelName := strings.TrimSpace(input.EmbeddingModel)
	if asset != nil {
		if asset.EmbeddingProvider != nil && strings.TrimSpace(*asset.EmbeddingProvider) != "" {
			provider = strings.TrimSpace(*asset.EmbeddingProvider)
		}
		if asset.EmbeddingModel != nil && strings.TrimSpace(*asset.EmbeddingModel) != "" {
			modelName = strings.TrimSpace(*asset.EmbeddingModel)
		}
	}
	return provider, modelName
}

func (s *fileAssetChunkEditService) enqueueDatasetRefSyncsForAssetEdit(ctx context.Context, asset *model.DocumentAsset, accountID string) error {
	if s == nil || s.refs == nil || s.documents == nil || s.refSync == nil || asset == nil {
		return nil
	}
	refs, err := s.refs.ListActiveByAsset(ctx, asset.OrganizationID, asset.ID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		if ref.DatasetDocumentID != nil && *ref.DatasetDocumentID != uuid.Nil {
			if err := s.documents.DisableDocuments(ctx, ref.DatasetID, []string{ref.DatasetDocumentID.String()}, accountID); err != nil {
				return err
			}
		}
		syncRunID := uuid.New()
		if _, err := s.refs.MarkPending(ctx, asset.OrganizationID, ref.ID, syncRunID, nil, nil); err != nil {
			return err
		}
		if err := s.refSync.EnqueueDatasetRefSync(ctx, ref.ID, asset.ID, ref.DatasetID, asset.GenerationNo, syncRunID); err != nil {
			return err
		}
	}
	return nil
}

func isEditableChunkUpdateAllowed(chunk *model.DocumentChunk, input FileAssetChunkEditInput) bool {
	if chunk == nil {
		return false
	}
	switch chunk.ChunkType {
	case model.DocumentChunkTypeChild, model.DocumentChunkTypeAuto, model.DocumentChunkTypeManual:
		return true
	case model.DocumentChunkTypeParent:
		return input.Content != nil || input.Enabled != nil
	default:
		return false
	}
}
