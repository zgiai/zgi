package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

type FileAssetChunkService interface {
	ListCurrentFileChunks(ctx context.Context, input FileAssetChunkListInput) (*FileAssetChunkListView, error)
}

type FileAssetChunkListInput struct {
	OrganizationID string
	SourceFileID   string
	Search         string
	Status         string
	ChunkTypes     []string
	Enabled        *bool
	ParentChunkID  *uuid.UUID
	IncludeTree    bool
	Limit          int
	Offset         int
}

type FileAssetChunkListView struct {
	Asset               *model.DocumentAsset  `json:"asset"`
	Items               []*FileAssetChunkView `json:"items"`
	Tree                []*FileAssetChunkView `json:"tree,omitempty"`
	Total               int64                 `json:"total"`
	PrimaryChunkCount   int64                 `json:"primary_chunk_count"`
	SecondaryChunkCount int64                 `json:"secondary_chunk_count"`
	EmbeddingCount      int64                 `json:"embedding_count"`
	Limit               int                   `json:"limit"`
	Offset              int                   `json:"offset"`
	GenerationNo        int64                 `json:"generation_no"`
}

type FileAssetChunkView struct {
	ID                 uuid.UUID             `json:"id"`
	AssetID            uuid.UUID             `json:"asset_id"`
	ProcessingRunID    uuid.UUID             `json:"processing_run_id"`
	GenerationNo       int64                 `json:"generation_no"`
	ChunkArtifactSetID *uuid.UUID            `json:"chunk_artifact_set_id,omitempty"`
	ParentChunkID      *uuid.UUID            `json:"parent_chunk_id,omitempty"`
	Position           int                   `json:"position"`
	ChunkType          string                `json:"chunk_type"`
	Content            string                `json:"content"`
	ContentHash        string                `json:"content_hash"`
	SourceLocatorJSON  map[string]any        `json:"source_locator_json,omitempty"`
	Enabled            bool                  `json:"enabled"`
	Status             string                `json:"status"`
	MetadataJSON       map[string]any        `json:"metadata_json,omitempty"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
	Children           []*FileAssetChunkView `json:"children,omitempty"`
}

type fileAssetChunkService struct {
	assets     repository.DocumentAssetRepository
	chunks     repository.DocumentChunkRepository
	embeddings repository.DocumentChunkEmbeddingRepository
}

func NewFileAssetChunkService(assets repository.DocumentAssetRepository, chunks repository.DocumentChunkRepository, embeddings repository.DocumentChunkEmbeddingRepository) FileAssetChunkService {
	return &fileAssetChunkService{assets: assets, chunks: chunks, embeddings: embeddings}
}

func (s *fileAssetChunkService) ListCurrentFileChunks(ctx context.Context, input FileAssetChunkListInput) (*FileAssetChunkListView, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.SourceFileID == "" {
		return nil, ErrSourceFileIDRequired
	}
	asset, err := s.assets.FindAssetBySourceFileID(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	limit := input.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	view := &FileAssetChunkListView{
		Asset:        asset,
		Items:        []*FileAssetChunkView{},
		Tree:         []*FileAssetChunkView{},
		Limit:        limit,
		Offset:       offset,
		GenerationNo: asset.GenerationNo,
	}
	if asset.GenerationNo <= 0 {
		return view, nil
	}
	generationNo := asset.GenerationNo
	if err := s.applyCounts(ctx, view, asset, generationNo); err != nil {
		return nil, err
	}
	chunkTypes := normalizeChunkTypes(input.ChunkTypes)
	if input.ParentChunkID == nil && len(chunkTypes) == 0 {
		chunkTypes = []string{model.DocumentChunkTypeParent}
	}
	items, total, err := s.chunks.List(ctx, repository.DocumentChunkListFilter{
		OrganizationID: input.OrganizationID,
		AssetID:        asset.ID,
		GenerationNo:   &generationNo,
		ParentChunkID:  input.ParentChunkID,
		ChunkTypes:     chunkTypes,
		Enabled:        input.Enabled,
		Status:         strings.TrimSpace(input.Status),
		Search:         strings.TrimSpace(input.Search),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		return nil, err
	}
	view.Total = total
	view.Items = documentChunksToViews(items)
	if input.IncludeTree {
		tree, err := s.buildChunkTreeForItems(ctx, input, asset, generationNo, items)
		if err != nil {
			return nil, err
		}
		view.Tree = tree
	}
	return view, nil
}

func (s *fileAssetChunkService) buildChunkTreeForItems(ctx context.Context, input FileAssetChunkListInput, asset *model.DocumentAsset, generationNo int64, items []*model.DocumentChunk) ([]*FileAssetChunkView, error) {
	roots := documentChunksToViews(items)
	if input.ParentChunkID != nil || len(roots) == 0 {
		return roots, nil
	}
	status := strings.TrimSpace(input.Status)
	for _, root := range roots {
		if root == nil || root.ChunkType != model.DocumentChunkTypeParent {
			continue
		}
		parentID := root.ID
		children, _, err := s.chunks.List(ctx, repository.DocumentChunkListFilter{
			OrganizationID: input.OrganizationID,
			AssetID:        asset.ID,
			GenerationNo:   &generationNo,
			ParentChunkID:  &parentID,
			ChunkTypes:     []string{model.DocumentChunkTypeChild},
			Enabled:        input.Enabled,
			Status:         status,
			Limit:          500,
			Offset:         0,
		})
		if err != nil {
			return nil, err
		}
		root.Children = documentChunksToViews(children)
	}
	return roots, nil
}

func (s *fileAssetChunkService) applyCounts(ctx context.Context, view *FileAssetChunkListView, asset *model.DocumentAsset, generationNo int64) error {
	primaryCount, err := s.chunks.CountByAssetGenerationAndTypes(ctx, asset.OrganizationID, asset.ID, generationNo, []string{model.DocumentChunkTypeParent})
	if err != nil {
		return err
	}
	secondaryCount, err := s.chunks.CountByAssetGenerationAndTypes(ctx, asset.OrganizationID, asset.ID, generationNo, []string{model.DocumentChunkTypeChild})
	if err != nil {
		return err
	}
	view.PrimaryChunkCount = primaryCount
	view.SecondaryChunkCount = secondaryCount
	if s.embeddings != nil {
		embeddingCount, err := s.embeddings.CountReadyByAssetGeneration(ctx, asset.OrganizationID, asset.ID, generationNo)
		if err != nil {
			return err
		}
		view.EmbeddingCount = embeddingCount
	}
	return nil
}

func normalizeChunkTypes(input []string) []string {
	out := make([]string, 0, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		switch value {
		case model.DocumentChunkTypeParent,
			model.DocumentChunkTypeChild,
			model.DocumentChunkTypeAuto,
			model.DocumentChunkTypeManual:
			out = append(out, value)
		}
	}
	return out
}

func documentChunksToViews(items []*model.DocumentChunk) []*FileAssetChunkView {
	views := make([]*FileAssetChunkView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, &FileAssetChunkView{
			ID:                 item.ID,
			AssetID:            item.AssetID,
			ProcessingRunID:    item.ProcessingRunID,
			GenerationNo:       item.GenerationNo,
			ChunkArtifactSetID: item.ChunkArtifactSetID,
			ParentChunkID:      item.ParentChunkID,
			Position:           item.Position,
			ChunkType:          item.ChunkType,
			Content:            item.Content,
			ContentHash:        item.ContentHash,
			SourceLocatorJSON:  item.SourceLocatorJSON,
			Enabled:            item.Enabled,
			Status:             item.Status,
			MetadataJSON:       item.MetadataJSON,
			CreatedAt:          item.CreatedAt.Format(timeFormatRFC3339Nano),
			UpdatedAt:          item.UpdatedAt.Format(timeFormatRFC3339Nano),
		})
	}
	return views
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
