package service

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

type FileAssetDetailService interface {
	GetCurrentFileAssetDetail(ctx context.Context, input FileAssetDetailInput) (*FileAssetDetailView, error)
}

type FileAssetDetailInput struct {
	OrganizationID string
	SourceFileID   string
}

type FileAssetDetailView struct {
	Asset                    *model.DocumentAsset      `json:"asset"`
	LatestProcessing         *ProcessingRequestView    `json:"latest_processing_request,omitempty"`
	ProcessingSummary        ProcessingSummaryView     `json:"processing_summary"`
	PendingConfirmationCount int64                     `json:"pending_confirmation_count"`
	ChunkCount               int64                     `json:"chunk_count"`
	EmbeddingCount           int64                     `json:"embedding_count"`
	ArtifactState            FileAssetArtifactState    `json:"artifact_state"`
	Error                    *FileAssetProcessingError `json:"error,omitempty"`
}

type FileAssetArtifactState struct {
	HasParseArtifact bool `json:"has_parse_artifact"`
	HasChunks        bool `json:"has_chunks"`
	HasEmbeddings    bool `json:"has_embeddings"`
}

type FileAssetProcessingError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type fileAssetDetailService struct {
	assets             repository.DocumentAssetRepository
	processingRequests repository.ProcessingRequestRepository
	confirmationItems  repository.ParseConfirmationItemRepository
	chunks             repository.DocumentChunkRepository
	embeddings         repository.DocumentChunkEmbeddingRepository
}

func NewFileAssetDetailService(
	assets repository.DocumentAssetRepository,
	processingRequests repository.ProcessingRequestRepository,
	confirmationItems repository.ParseConfirmationItemRepository,
	chunks repository.DocumentChunkRepository,
	embeddings repository.DocumentChunkEmbeddingRepository,
) FileAssetDetailService {
	return &fileAssetDetailService{
		assets:             assets,
		processingRequests: processingRequests,
		confirmationItems:  confirmationItems,
		chunks:             chunks,
		embeddings:         embeddings,
	}
}

func (s *fileAssetDetailService) GetCurrentFileAssetDetail(ctx context.Context, input FileAssetDetailInput) (*FileAssetDetailView, error) {
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
	view := &FileAssetDetailView{
		Asset: asset,
		ArtifactState: FileAssetArtifactState{
			HasParseArtifact: asset.ParseArtifactID != nil,
		},
	}
	if asset.LastErrorCode != nil || asset.LastErrorMessage != nil {
		view.Error = &FileAssetProcessingError{}
		if asset.LastErrorCode != nil {
			view.Error.Code = *asset.LastErrorCode
		}
		if asset.LastErrorMessage != nil {
			view.Error.Message = *asset.LastErrorMessage
		}
	}
	if err := s.applyLatestProcessing(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyProcessingSummary(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyCurrentResultCounts(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyPendingConfirmations(ctx, view); err != nil {
		return nil, err
	}
	return view, nil
}

func (s *fileAssetDetailService) applyLatestProcessing(ctx context.Context, view *FileAssetDetailView) error {
	if view == nil || view.Asset == nil || s.processingRequests == nil {
		return nil
	}
	items, _, err := s.processingRequests.List(ctx, repository.ProcessingRequestListFilter{
		OrganizationID: view.Asset.OrganizationID,
		AssetID:        view.Asset.ID,
		Limit:          1,
	})
	if err != nil {
		return err
	}
	if len(items) > 0 {
		view.LatestProcessing = newProcessingRequestView(items[0])
	}
	return nil
}

func (s *fileAssetDetailService) applyProcessingSummary(ctx context.Context, view *FileAssetDetailView) error {
	if view == nil || view.Asset == nil || s.processingRequests == nil {
		return nil
	}
	summaries, err := s.processingRequests.StatusSummaryByAssetID(ctx, view.Asset.OrganizationID, view.Asset.ID)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		view.ProcessingSummary.Total += summary.Count
		switch summary.Status {
		case model.ProcessingRequestStatusPlanned:
			view.ProcessingSummary.Planned = summary.Count
		case model.ProcessingRequestStatusQueued:
			view.ProcessingSummary.Queued = summary.Count
		case model.ProcessingRequestStatusRunning:
			view.ProcessingSummary.Running = summary.Count
		case model.ProcessingRequestStatusCompleted:
			view.ProcessingSummary.Completed = summary.Count
		case model.ProcessingRequestStatusFailed:
			view.ProcessingSummary.Failed = summary.Count
		case model.ProcessingRequestStatusCancelled:
			view.ProcessingSummary.Cancelled = summary.Count
		}
	}
	return nil
}

func (s *fileAssetDetailService) applyCurrentResultCounts(ctx context.Context, view *FileAssetDetailView) error {
	if view == nil || view.Asset == nil || view.Asset.GenerationNo <= 0 {
		return nil
	}
	if s.chunks != nil {
		count, err := s.chunks.CountByAssetGenerationAndTypes(ctx, view.Asset.OrganizationID, view.Asset.ID, view.Asset.GenerationNo, []string{model.DocumentChunkTypeParent})
		if err != nil {
			return err
		}
		view.ChunkCount = count
		view.ArtifactState.HasChunks = count > 0
	}
	if s.embeddings != nil {
		count, err := s.embeddings.CountReadyByAssetGeneration(ctx, view.Asset.OrganizationID, view.Asset.ID, view.Asset.GenerationNo)
		if err != nil {
			return err
		}
		view.EmbeddingCount = count
		view.ArtifactState.HasEmbeddings = count > 0
	}
	return nil
}

func (s *fileAssetDetailService) applyPendingConfirmations(ctx context.Context, view *FileAssetDetailView) error {
	if view == nil || view.Asset == nil || s.confirmationItems == nil || view.Asset.ProcessingRunID == nil || view.Asset.GenerationNo <= 0 {
		return nil
	}
	count, err := s.confirmationItems.CountPendingByRun(ctx, view.Asset.OrganizationID, view.Asset.ID, *view.Asset.ProcessingRunID, view.Asset.GenerationNo)
	if err != nil {
		return err
	}
	view.PendingConfirmationCount = count
	return nil
}
