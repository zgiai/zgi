package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrFileChunkEditNotAllowed = errors.New("file chunk edit is not allowed")
)

type FileAssetChunkEditService interface {
	UpdateCurrentFileChunk(ctx context.Context, input FileAssetChunkEditInput) (*FileAssetChunkEditResult, error)
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
	return &fileAssetChunkEditService{
		assets:      assets,
		chunks:      chunks,
		embeddings:  embeddings,
		chunkEmbed:  chunkEmbed,
		vectorIndex: vectorIndexService,
	}
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
	embeddingResult, embeddingReady, err := s.syncEditedChunkEmbedding(ctx, asset, updatedChunk, input)
	if err != nil {
		return nil, err
	}
	return &FileAssetChunkEditResult{
		Asset:          asset,
		Chunk:          updatedChunk,
		Embedding:      embeddingResult,
		EmbeddingReady: embeddingReady,
	}, nil
}

func (s *fileAssetChunkEditService) syncEditedChunkEmbedding(ctx context.Context, asset *model.DocumentAsset, updatedChunk *model.DocumentChunk, input FileAssetChunkEditInput) (*model.DocumentChunkEmbedding, bool, error) {
	if updatedChunk == nil {
		return nil, false, nil
	}
	if updatedChunk.ChunkType == model.DocumentChunkTypeParent {
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
		embeddingResult, err := s.chunkEmbed.GenerateChunkEmbedding(ctx, GenerateDocumentChunkEmbeddingInput{
			OrganizationID:    input.OrganizationID,
			AssetID:           asset.ID,
			ProcessingRunID:   *asset.ProcessingRunID,
			GenerationNo:      asset.GenerationNo,
			EmbeddingProvider: input.EmbeddingProvider,
			EmbeddingModel:    input.EmbeddingModel,
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

func isEditableChunkUpdateAllowed(chunk *model.DocumentChunk, input FileAssetChunkEditInput) bool {
	if chunk == nil {
		return false
	}
	switch chunk.ChunkType {
	case model.DocumentChunkTypeChild, model.DocumentChunkTypeAuto, model.DocumentChunkTypeManual:
		return true
	case model.DocumentChunkTypeParent:
		return input.Content == nil && input.Enabled != nil
	default:
		return false
	}
}
