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
	datasetindexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	"github.com/zgiai/zgi/api/internal/modules/dataset/splitter"
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
	ProcessRule        map[string]interface{}
	CreatedBy          string
}

type GenerateDocumentChunksResult struct {
	Asset               *model.DocumentAsset   `json:"asset"`
	Chunks              []*model.DocumentChunk `json:"chunks"`
	ChunkCount          int                    `json:"chunk_count"`
	PrimaryChunkCount   int                    `json:"primary_chunk_count"`
	SecondaryChunkCount int                    `json:"secondary_chunk_count"`
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
	if err := s.chunks.DeleteByAsset(ctx, input.OrganizationID, input.AssetID); err != nil {
		return nil, err
	}
	if err := s.chunks.CreateBatch(ctx, items); err != nil {
		return nil, err
	}

	primaryChunkCount := countDocumentChunksByType(items, model.DocumentChunkTypeParent)
	secondaryChunkCount := countDocumentChunksByType(items, model.DocumentChunkTypeChild)
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
		ChunkCount:             &primaryChunkCount,
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
		Asset:               updated,
		Chunks:              items,
		ChunkCount:          primaryChunkCount,
		PrimaryChunkCount:   primaryChunkCount,
		SecondaryChunkCount: secondaryChunkCount,
	}, nil
}

func buildDocumentChunksFromTransformed(asset *model.DocumentAsset, input GenerateDocumentChunksInput) []*model.DocumentChunk {
	items := make([]*model.DocumentChunk, 0, len(input.Chunks))
	rule := parseDocumentSubchunkRule(input.ProcessRule)
	for position, chunk := range input.Chunks {
		content := strings.TrimSpace(chunk.Content)
		children := nonEmptyChildChunks(chunk.Children)
		if content == "" && len(children) > 0 {
			content = joinedChildContent(children)
		}
		if content == "" {
			continue
		}
		parentID := uuid.New()
		items = append(items, newDocumentChunk(asset, input, parentID, nil, position, model.DocumentChunkTypeParent, content, chunk.BBox, chunk.Metadata))
		if len(children) == 0 {
			children = splitDocumentChildChunks(chunk, content, rule)
		}
		for childPosition, child := range children {
			childContent := strings.TrimSpace(child.Content)
			if childContent == "" {
				continue
			}
			items = append(items, newDocumentChunk(asset, input, uuid.New(), &parentID, childPosition, model.DocumentChunkTypeChild, childContent, child.BBox, child.Metadata))
		}
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

func countDocumentChunksByType(items []*model.DocumentChunk, chunkType string) int {
	count := 0
	for _, item := range items {
		if item != nil && item.ChunkType == chunkType {
			count++
		}
	}
	return count
}

func parseDocumentSubchunkRule(processRule map[string]interface{}) *datasetindexing.Rule {
	rule, err := datasetindexing.ParseRule(processRule)
	if err != nil || rule == nil {
		rule, _ = datasetindexing.ParseRule(nil)
	}
	return rule
}

func splitDocumentChildChunks(parent dto.TransformedChunk, content string, rule *datasetindexing.Rule) []dto.TransformedChildChunk {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if rule == nil || rule.SubchunkSegmentation == nil {
		return []dto.TransformedChildChunk{{Content: content, BBox: parent.BBox, Metadata: childChunkMetadata(parent.Metadata, 0)}}
	}
	fixedSeparator, separators := documentSubchunkSeparators(rule.SubchunkSegmentation.Separator)
	textSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(
		fixedSeparator,
		separators,
		rule.SubchunkSegmentation.MaxTokens,
		rule.SubchunkSegmentation.ChunkOverlap,
		nil,
		false,
		false,
	)
	rawChunks := textSplitter.SplitText(content)
	children := make([]dto.TransformedChildChunk, 0, len(rawChunks))
	for _, raw := range rawChunks {
		childContent := strings.TrimSpace(raw)
		if childContent == "" {
			continue
		}
		children = append(children, dto.TransformedChildChunk{
			Content:  childContent,
			BBox:     parent.BBox,
			Metadata: childChunkMetadata(parent.Metadata, len(children)),
		})
	}
	if len(children) == 0 {
		children = append(children, dto.TransformedChildChunk{Content: content, BBox: parent.BBox, Metadata: childChunkMetadata(parent.Metadata, 0)})
	}
	return children
}

func childChunkMetadata(parent map[string]any, childIndex int) map[string]any {
	metadata := cloneAnyMap(parent)
	metadata["is_child"] = true
	metadata["child_index"] = childIndex
	if parentID, ok := metadata["doc_id"]; ok {
		metadata["parent_id"] = parentID
	}
	return metadata
}

func documentSubchunkSeparators(preferredSeparator string) (string, []string) {
	defaultSeparators := []string{"\n\n", "\n", "。", "！", "？", "；", "：", ". ", "! ", "? ", "; ", ": ", ".", "!", "?", ";", ":", "，", ",", "、", " ", ""}
	fixedSeparator := preferredSeparator
	if fixedSeparator == "" {
		fixedSeparator = "\n"
	}
	separators := make([]string, 0, len(defaultSeparators)+1)
	seen := make(map[string]struct{}, len(defaultSeparators)+1)
	for _, separator := range append([]string{fixedSeparator}, defaultSeparators...) {
		if _, ok := seen[separator]; ok {
			continue
		}
		seen[separator] = struct{}{}
		separators = append(separators, separator)
	}
	return fixedSeparator, separators
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
