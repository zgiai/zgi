package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparseservice "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	filerepository "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const FileProcessExecutorKey = "data-library-file-process"

type FileProcessRunner struct {
	processingRequests  repository.ProcessingRequestRepository
	assets              repository.DocumentAssetRepository
	files               filerepository.FileRepository
	storage             storage.Storage
	contentParse        contracts.ContentParseService
	state               datalibraryservice.FileAssetProcessingStateService
	artifactPersistence datalibraryservice.ParseArtifactPersistenceService
	quality             datalibraryservice.ParseArtifactQualityService
	processingService   datalibraryservice.ProcessingRequestService
}

type FileProcessRunnerDeps struct {
	ProcessingRequests  repository.ProcessingRequestRepository
	Assets              repository.DocumentAssetRepository
	Files               filerepository.FileRepository
	Storage             storage.Storage
	ContentParse        contracts.ContentParseService
	State               datalibraryservice.FileAssetProcessingStateService
	ArtifactPersistence datalibraryservice.ParseArtifactPersistenceService
	Quality             datalibraryservice.ParseArtifactQualityService
	ProcessingService   datalibraryservice.ProcessingRequestService
}

func NewFileProcessRunner(deps FileProcessRunnerDeps) *FileProcessRunner {
	return &FileProcessRunner{
		processingRequests:  deps.ProcessingRequests,
		assets:              deps.Assets,
		files:               deps.Files,
		storage:             deps.Storage,
		contentParse:        deps.ContentParse,
		state:               deps.State,
		artifactPersistence: deps.ArtifactPersistence,
		quality:             deps.Quality,
		processingService:   deps.ProcessingService,
	}
}

func (r *FileProcessRunner) Run(ctx context.Context, processingRequestID uuid.UUID) error {
	if processingRequestID == uuid.Nil {
		return datalibraryservice.ErrProcessingRequestIDRequired
	}
	request, err := r.processingRequests.GetByID(ctx, processingRequestID)
	if err != nil {
		return err
	}
	if request == nil {
		return datalibraryservice.ErrProcessingRequestNotFound
	}
	if request.Status != model.ProcessingRequestStatusQueued {
		return datalibraryservice.ErrProcessingRequestTransitionInvalid
	}
	started, err := r.processingService.StartRequest(ctx, request.OrganizationID, request.ID, FileProcessExecutorKey)
	if err != nil {
		return err
	}

	asset, err := r.assets.GetAssetByID(ctx, request.AssetID)
	if err != nil {
		return r.failRequest(ctx, request, nil, "asset_load_failed", err)
	}
	if asset == nil || asset.OrganizationID != request.OrganizationID {
		return r.failRequest(ctx, request, nil, "asset_not_found", datalibraryservice.ErrDocumentAssetNotFound)
	}
	runID := request.ID
	generationNo := asset.GenerationNo
	if asset.ProcessingRunID == nil || *asset.ProcessingRunID != runID || generationNo <= 0 {
		return r.failRequest(ctx, request, asset, "processing_run_mismatch", datalibraryservice.ErrProcessingRunMismatch)
	}

	if _, err := r.state.MarkParsing(ctx, datalibraryservice.RunStateInput{
		OrganizationID:     request.OrganizationID,
		AssetID:            asset.ID,
		ProcessingRunID:    runID,
		GenerationNo:       generationNo,
		ProcessingProgress: 15,
	}); err != nil {
		return r.failRequest(ctx, request, asset, "mark_parsing_failed", err)
	}

	uploadFile, sourceBytes, err := r.loadSourceFile(ctx, asset)
	if err != nil {
		return r.failRequest(ctx, request, asset, "source_load_failed", err)
	}
	parseRequest := buildFileParseRequest(asset, uploadFile, sourceBytes, request)
	artifact, err := r.contentParse.Parse(ctx, parseRequest)
	if err != nil {
		return r.failRequest(ctx, request, asset, "parse_failed", err)
	}

	summary := map[string]interface{}{
		"source_content_hash": asset.ContentHash,
	}
	contentparseservice.ApplyDatasetShadowArtifactSummary(summary, artifact)
	persisted, err := r.artifactPersistence.PersistAssetParseArtifact(ctx, datalibraryservice.PersistAssetParseArtifactInput{
		OrganizationID:    request.OrganizationID,
		AssetID:           asset.ID,
		ProcessingRunID:   runID,
		GenerationNo:      generationNo,
		SourceFileID:      asset.SourceFileID,
		SourceContentHash: asset.ContentHash,
		ParseRequest:      parseRequest,
		Artifact:          artifact,
		Summary:           summary,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "artifact_persist_failed", err)
	}

	quality, err := r.quality.CreateConfirmationItems(ctx, datalibraryservice.ParseArtifactQualityInput{
		OrganizationID:  request.OrganizationID,
		WorkspaceID:     asset.WorkspaceID,
		AssetID:         asset.ID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		CreatedBy:       request.RequestedBy,
		Artifact:        artifact,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "quality_check_failed", err)
	}

	if quality.PendingCount > 0 {
		if _, err := r.state.MarkConfirming(ctx, datalibraryservice.RunStateInput{
			OrganizationID:     request.OrganizationID,
			AssetID:            asset.ID,
			ProcessingRunID:    runID,
			GenerationNo:       generationNo,
			ProcessingProgress: 40,
			ParseArtifactID:    &persisted.Artifact.ID,
		}); err != nil {
			return r.failRequest(ctx, request, asset, "mark_confirming_failed", err)
		}
	} else {
		if _, err := r.state.MarkGenerating(ctx, datalibraryservice.RunStateInput{
			OrganizationID:     request.OrganizationID,
			AssetID:            asset.ID,
			ProcessingRunID:    runID,
			GenerationNo:       generationNo,
			ProcessingProgress: 50,
			ParseArtifactID:    &persisted.Artifact.ID,
		}); err != nil {
			return r.failRequest(ctx, request, asset, "mark_generating_failed", err)
		}
	}

	_, err = r.processingService.CompleteRequest(ctx, request.OrganizationID, started.ID, map[string]any{
		"parse_artifact_id":          persisted.Artifact.ID.String(),
		"artifact_storage_key":       persisted.ArtifactStorageKey,
		"pending_confirmation_count": quality.PendingCount,
		"next_product_status":        nextProductStatusAfterParse(quality.PendingCount),
		"generation_no":              generationNo,
	})
	return err
}

func (r *FileProcessRunner) loadSourceFile(ctx context.Context, asset *model.DocumentAsset) (*filemodel.UploadFile, []byte, error) {
	if asset.SourceFileID == "" {
		return nil, nil, fmt.Errorf("asset source_file_id is empty")
	}
	uploadFile, err := r.files.GetByID(ctx, asset.SourceFileID)
	if err != nil {
		return nil, nil, err
	}
	if uploadFile == nil {
		return nil, nil, fmt.Errorf("upload file not found")
	}
	data, err := r.storage.Load(uploadFile.Key)
	if err != nil {
		return nil, nil, err
	}
	return uploadFile, data, nil
}

func (r *FileProcessRunner) failRequest(ctx context.Context, request *model.ProcessingRequest, asset *model.DocumentAsset, code string, cause error) error {
	if request != nil && r.processingService != nil {
		_, _ = r.processingService.FailRequest(ctx, request.OrganizationID, request.ID, code, cause.Error(), map[string]any{
			"executor_key": FileProcessExecutorKey,
		})
	}
	if asset != nil && asset.ProcessingRunID != nil {
		_, _ = r.state.MarkFailed(ctx, datalibraryservice.FailedStateInput{
			RunStateInput: datalibraryservice.RunStateInput{
				OrganizationID:     asset.OrganizationID,
				AssetID:            asset.ID,
				ProcessingRunID:    *asset.ProcessingRunID,
				GenerationNo:       asset.GenerationNo,
				ProcessingProgress: asset.ProcessingProgress,
			},
			ErrorCode:    code,
			ErrorMessage: cause.Error(),
		})
	}
	return cause
}

func buildFileParseRequest(asset *model.DocumentAsset, uploadFile *filemodel.UploadFile, data []byte, request *model.ProcessingRequest) contracts.ParseRequest {
	return contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeUploadFile,
		SourceRef:  uploadFile.ID,
		FileName:   uploadFile.Name,
		Data:       data,
		Intent:     contracts.ParseIntentDatasetIndex,
		Profile:    contracts.ParseProfileDatasetIndex,
		Force:      request.Force,
		Metadata: map[string]any{
			"organization_id":       request.OrganizationID,
			"workspace_id":          asset.WorkspaceID,
			"asset_id":              asset.ID.String(),
			"processing_request_id": request.ID.String(),
			"processing_run_id":     request.ID.String(),
			"generation_no":         asset.GenerationNo,
			"source_file_id":        asset.SourceFileID,
			"source_content_hash":   asset.ContentHash,
		},
	}
}

func nextProductStatusAfterParse(pendingCount int64) string {
	if pendingCount > 0 {
		return model.DocumentAssetProductStatusConfirming
	}
	return model.DocumentAssetProductStatusGenerating
}
