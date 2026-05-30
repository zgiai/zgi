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
	assets     repository.DocumentAssetRepository
	chunks     repository.DocumentChunkRepository
	embeddings repository.DocumentChunkEmbeddingRepository
	chunkEmbed DocumentChunkEmbeddingService
}

func NewFileAssetChunkEditService(
	assets repository.DocumentAssetRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
	chunkEmbed DocumentChunkEmbeddingService,
) FileAssetChunkEditService {
	return &fileAssetChunkEditService{
		assets:     assets,
		chunks:     chunks,
		embeddings: embeddings,
		chunkEmbed: chunkEmbed,
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
	if !isEditableChunkType(chunk.ChunkType) {
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
	var embeddingResult *model.DocumentChunkEmbedding
	embeddingReady := false
	if input.Content != nil && s.chunkEmbed != nil {
		embeddingResult, err = s.chunkEmbed.GenerateChunkEmbedding(ctx, GenerateDocumentChunkEmbeddingInput{
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
			return nil, err
		}
		embeddingReady = true
	}
	return &FileAssetChunkEditResult{
		Asset:          asset,
		Chunk:          updatedChunk,
		Embedding:      embeddingResult,
		EmbeddingReady: embeddingReady,
	}, nil
}

func isEditableChunkType(chunkType string) bool {
	switch chunkType {
	case model.DocumentChunkTypeChild, model.DocumentChunkTypeAuto, model.DocumentChunkTypeManual:
		return true
	default:
		return false
	}
}
