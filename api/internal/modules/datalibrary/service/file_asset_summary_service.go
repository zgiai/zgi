package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

type FileAssetSummaryService interface {
	ListCurrentFileAssetSummaries(ctx context.Context, input FileAssetSummaryListInput) (map[string]FileAssetSummaryView, error)
}

type FileAssetSummaryListInput struct {
	OrganizationID string
	SourceFileIDs  []string
}

type FileAssetSummaryView struct {
	AssetID                   uuid.UUID  `json:"asset_id"`
	SourceFileID              string     `json:"source_file_id"`
	ProductStatus             string     `json:"product_status"`
	ProcessingStage           string     `json:"processing_stage,omitempty"`
	ProcessingProgress        int        `json:"processing_progress"`
	ActiveProcessingRequestID *uuid.UUID `json:"active_processing_request_id,omitempty"`
	ProcessingRunID           *uuid.UUID `json:"processing_run_id,omitempty"`
	GenerationNo              int64      `json:"generation_no"`
	PendingConfirmationCount  int64      `json:"pending_confirmation_count"`
	ChunkCount                int64      `json:"chunk_count"`
	EmbeddingCount            int64      `json:"embedding_count"`
	VectorStatus              string     `json:"vector_status"`
	LastErrorCode             string     `json:"last_error_code,omitempty"`
	LastErrorMessage          string     `json:"last_error_message,omitempty"`
}

type fileAssetSummaryService struct {
	assets            repository.DocumentAssetRepository
	confirmationItems repository.ParseConfirmationItemRepository
	chunks            repository.DocumentChunkRepository
	embeddings        repository.DocumentChunkEmbeddingRepository
}

func NewFileAssetSummaryService(
	assets repository.DocumentAssetRepository,
	confirmationItems repository.ParseConfirmationItemRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
) FileAssetSummaryService {
	return &fileAssetSummaryService{
		assets:            assets,
		confirmationItems: confirmationItems,
		chunks:            chunks,
		embeddings:        embeddings,
	}
}

func (s *fileAssetSummaryService) ListCurrentFileAssetSummaries(ctx context.Context, input FileAssetSummaryListInput) (map[string]FileAssetSummaryView, error) {
	result := map[string]FileAssetSummaryView{}
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if len(input.SourceFileIDs) == 0 {
		return result, nil
	}
	assets, err := s.assets.FindAssetsBySourceFileIDs(ctx, input.OrganizationID, input.SourceFileIDs)
	if err != nil {
		return nil, err
	}
	for sourceFileID, asset := range assets {
		if asset == nil {
			continue
		}
		view := FileAssetSummaryView{
			AssetID:                   asset.ID,
			SourceFileID:              asset.SourceFileID,
			ProductStatus:             asset.ProductStatus,
			ProcessingProgress:        asset.ProcessingProgress,
			ActiveProcessingRequestID: asset.ActiveProcessingRequestID,
			ProcessingRunID:           asset.ProcessingRunID,
			GenerationNo:              asset.GenerationNo,
			ChunkCount:                int64(asset.ChunkCount),
			VectorStatus:              asset.VectorStatus,
		}
		if asset.ProcessingStage != nil {
			view.ProcessingStage = *asset.ProcessingStage
		}
		if asset.LastErrorCode != nil {
			view.LastErrorCode = *asset.LastErrorCode
		}
		if asset.LastErrorMessage != nil {
			view.LastErrorMessage = *asset.LastErrorMessage
		}
		if asset.ProcessingRunID != nil && asset.GenerationNo > 0 && s.confirmationItems != nil {
			count, err := s.confirmationItems.CountPendingByRun(ctx, asset.OrganizationID, asset.ID, *asset.ProcessingRunID, asset.GenerationNo)
			if err != nil {
				return nil, err
			}
			view.PendingConfirmationCount = count
		}
		if asset.GenerationNo > 0 {
			if s.chunks != nil {
				count, err := s.chunks.CountByAssetGeneration(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo)
				if err != nil {
					return nil, err
				}
				view.ChunkCount = count
			}
			if s.embeddings != nil {
				count, err := s.embeddings.CountReadyByAssetGeneration(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo)
				if err != nil {
					return nil, err
				}
				view.EmbeddingCount = count
			}
		}
		result[sourceFileID] = view
	}
	return result, nil
}
