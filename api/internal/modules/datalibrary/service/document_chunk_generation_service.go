package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var ErrDocumentChunksRequired = errors.New("document chunks are required")

type DocumentChunkGenerationService interface {
	GenerateChunks(ctx context.Context, input GenerateDocumentChunksInput) (*GenerateDocumentChunksResult, error)
}

type GenerateDocumentChunksInput struct {
	OrganizationID     string
	AssetID            uuid.UUID
	ProcessingRunID    uuid.UUID
	GenerationNo       int64
	ChunkArtifactSetID *uuid.UUID
	Chunks             []dto.TransformedChunk
	CreatedBy          string
}

type GenerateDocumentChunksResult struct {
	Asset      *model.DocumentAsset   `json:"asset"`
	Chunks     []*model.DocumentChunk `json:"chunks"`
	ChunkCount int                    `json:"chunk_count"`
	LeafCount  int                    `json:"leaf_count"`
}

type documentChunkGenerationService struct {
	assets repository.DocumentAssetRepository
	chunks repository.DocumentChunkRepository
}

func NewDocumentChunkGenerationService(assets repository.DocumentAssetRepository, chunks repository.DocumentChunkRepository) DocumentChunkGenerationService {
	return &documentChunkGenerationService{assets: assets, chunks: chunks}
}

func (s *documentChunkGenerationService) GenerateChunks(ctx context.Context, input GenerateDocumentChunksInput) (*GenerateDocumentChunksResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo <= 0 {
		return nil, ErrProcessingRunMismatch
	}
	if len(input.Chunks) == 0 {
		return nil, ErrDocumentChunksRequired
	}
	asset, err := s.assets.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != input.OrganizationID {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ProcessingRunID == nil ||
		*asset.ProcessingRunID != input.ProcessingRunID ||
		asset.GenerationNo != input.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}

	items := buildDocumentChunksFromTransformed(asset, input)
	if len(items) == 0 {
		return nil, ErrDocumentChunksRequired
	}
	if err := s.chunks.DeleteByAssetGeneration(ctx, input.OrganizationID, input.AssetID, input.GenerationNo); err != nil {
		return nil, err
	}
	if err := s.chunks.CreateBatch(ctx, items); err != nil {
		return nil, err
	}

	chunkCount := len(items)
	progress := 70
	status := model.DocumentAssetProductStatusGenerating
	stage := model.DocumentAssetProcessingStageChunk
	vectorStatus := model.DocumentAssetVectorStatusIndexing
	updated, err := s.assets.UpdateCurrentResult(ctx, input.AssetID, repository.DocumentAssetCurrentResultPatch{
		OrganizationID:         input.OrganizationID,
		ProductStatus:          &status,
		ProcessingStage:        &stage,
		ProcessingProgress:     &progress,
		ChunkArtifactSetID:     input.ChunkArtifactSetID,
		ChunkCount:             &chunkCount,
		VectorStatus:           &vectorStatus,
		RequireProcessingRunID: &input.ProcessingRunID,
		RequireGenerationNo:    &input.GenerationNo,
		ClearError:             true,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.ProcessingRunID == nil || *updated.ProcessingRunID != input.ProcessingRunID || updated.GenerationNo != input.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}
	return &GenerateDocumentChunksResult{
		Asset:      updated,
		Chunks:     items,
		ChunkCount: chunkCount,
		LeafCount:  countLeafDocumentChunks(items),
	}, nil
}

func buildDocumentChunksFromTransformed(asset *model.DocumentAsset, input GenerateDocumentChunksInput) []*model.DocumentChunk {
	items := make([]*model.DocumentChunk, 0, len(input.Chunks))
	for position, chunk := range input.Chunks {
		content := strings.TrimSpace(chunk.Content)
		children := nonEmptyChildChunks(chunk.Children)
		if len(children) > 0 {
			parentID := uuid.New()
			if content != "" {
				items = append(items, newDocumentChunk(asset, input, parentID, nil, position, model.DocumentChunkTypeParent, content, chunk.BBox, chunk.Metadata))
			} else {
				items = append(items, newDocumentChunk(asset, input, parentID, nil, position, model.DocumentChunkTypeParent, joinedChildContent(children), chunk.BBox, chunk.Metadata))
			}
			for childPosition, child := range children {
				items = append(items, newDocumentChunk(asset, input, uuid.New(), &parentID, childPosition, model.DocumentChunkTypeChild, strings.TrimSpace(child.Content), child.BBox, child.Metadata))
			}
			continue
		}
		if content == "" {
			continue
		}
		items = append(items, newDocumentChunk(asset, input, uuid.New(), nil, position, model.DocumentChunkTypeAuto, content, chunk.BBox, chunk.Metadata))
	}
	return items
}

func newDocumentChunk(
	asset *model.DocumentAsset,
	input GenerateDocumentChunksInput,
	id uuid.UUID,
	parentID *uuid.UUID,
	position int,
	chunkType string,
	content string,
	bbox *dto.ExtractBoundingBox,
	metadata map[string]any,
) *model.DocumentChunk {
	metadataJSON := cloneAnyMap(metadata)
	if chunkType == model.DocumentChunkTypeParent {
		metadataJSON["leaf_embedding"] = false
	}
	return &model.DocumentChunk{
		ID:                 id,
		OrganizationID:     input.OrganizationID,
		WorkspaceID:        asset.WorkspaceID,
		AssetID:            input.AssetID,
		ProcessingRunID:    input.ProcessingRunID,
		GenerationNo:       input.GenerationNo,
		ChunkArtifactSetID: input.ChunkArtifactSetID,
		ParentChunkID:      parentID,
		Position:           position,
		ChunkType:          chunkType,
		Content:            content,
		ContentHash:        documentChunkContentHash(content),
		SourceLocatorJSON:  documentChunkSourceLocator(bbox, metadata),
		Enabled:            true,
		Status:             model.DocumentChunkStatusReady,
		MetadataJSON:       metadataJSON,
		CreatedBy:          input.CreatedBy,
		UpdatedBy:          input.CreatedBy,
	}
}

func nonEmptyChildChunks(children []dto.TransformedChildChunk) []dto.TransformedChildChunk {
	out := make([]dto.TransformedChildChunk, 0, len(children))
	for _, child := range children {
		if strings.TrimSpace(child.Content) != "" {
			out = append(out, child)
		}
	}
	return out
}

func joinedChildContent(children []dto.TransformedChildChunk) string {
	contents := make([]string, 0, len(children))
	for _, child := range children {
		if content := strings.TrimSpace(child.Content); content != "" {
			contents = append(contents, content)
		}
	}
	return strings.Join(contents, "\n")
}

func countLeafDocumentChunks(items []*model.DocumentChunk) int {
	count := 0
	for _, item := range items {
		switch item.ChunkType {
		case model.DocumentChunkTypeChild, model.DocumentChunkTypeAuto, model.DocumentChunkTypeManual:
			count++
		}
	}
	return count
}

func documentChunkSourceLocator(bbox *dto.ExtractBoundingBox, metadata map[string]any) map[string]any {
	locator := map[string]any{}
	if bbox != nil {
		locator["bbox"] = map[string]any{
			"left":   bbox.Left,
			"top":    bbox.Top,
			"right":  bbox.Right,
			"bottom": bbox.Bottom,
		}
	}
	if metadata != nil {
		if page, ok := metadata["page"]; ok {
			locator["page"] = page
		}
		if elementType, ok := metadata["element_type"]; ok {
			locator["element_type"] = elementType
		}
	}
	return locator
}

func documentChunkContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
