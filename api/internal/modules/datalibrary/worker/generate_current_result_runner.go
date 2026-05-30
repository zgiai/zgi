package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	contentparserepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

const GenerateCurrentResultExecutorKey = "data-library-generate-current-result"

type GenerateCurrentResultRunner struct {
	processingRequests  repository.ProcessingRequestRepository
	assets              repository.DocumentAssetRepository
	artifacts           contentparserepo.ArtifactRepository
	state               datalibraryservice.FileAssetProcessingStateService
	artifactPersistence datalibraryservice.ParseArtifactPersistenceService
	transform           datalibraryservice.ParseArtifactChunkTransformService
	chunkGeneration     datalibraryservice.DocumentChunkGenerationService
	embedding           datalibraryservice.DocumentChunkEmbeddingService
	processingService   datalibraryservice.ProcessingRequestService
}

type GenerateCurrentResultRunnerDeps struct {
	ProcessingRequests  repository.ProcessingRequestRepository
	Assets              repository.DocumentAssetRepository
	Artifacts           contentparserepo.ArtifactRepository
	State               datalibraryservice.FileAssetProcessingStateService
	ArtifactPersistence datalibraryservice.ParseArtifactPersistenceService
	Transform           datalibraryservice.ParseArtifactChunkTransformService
	ChunkGeneration     datalibraryservice.DocumentChunkGenerationService
	Embedding           datalibraryservice.DocumentChunkEmbeddingService
	ProcessingService   datalibraryservice.ProcessingRequestService
}

func NewGenerateCurrentResultRunner(deps GenerateCurrentResultRunnerDeps) *GenerateCurrentResultRunner {
	return &GenerateCurrentResultRunner{
		processingRequests:  deps.ProcessingRequests,
		assets:              deps.Assets,
		artifacts:           deps.Artifacts,
		state:               deps.State,
		artifactPersistence: deps.ArtifactPersistence,
		transform:           deps.Transform,
		chunkGeneration:     deps.ChunkGeneration,
		embedding:           deps.Embedding,
		processingService:   deps.ProcessingService,
	}
}

func (r *GenerateCurrentResultRunner) Run(ctx context.Context, processingRequestID uuid.UUID) error {
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
	started, err := r.processingService.StartRequest(ctx, request.OrganizationID, request.ID, GenerateCurrentResultExecutorKey)
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
	runID, generationNo, err := generationRequestRun(request, asset)
	if err != nil {
		return r.failRequest(ctx, request, asset, "processing_run_mismatch", err)
	}
	if asset.ProcessingRunID == nil ||
		*asset.ProcessingRunID != runID ||
		asset.GenerationNo != generationNo ||
		asset.ParseArtifactID == nil {
		return r.failRequest(ctx, request, asset, "processing_run_mismatch", datalibraryservice.ErrProcessingRunMismatch)
	}

	if _, err := r.state.MarkGenerating(ctx, datalibraryservice.RunStateInput{
		OrganizationID:     request.OrganizationID,
		AssetID:            asset.ID,
		ProcessingRunID:    runID,
		GenerationNo:       generationNo,
		ProcessingStage:    model.DocumentAssetProcessingStageChunk,
		ProcessingProgress: 55,
		ParseArtifactID:    asset.ParseArtifactID,
	}); err != nil {
		return r.failRequest(ctx, request, asset, "mark_generating_failed", err)
	}

	artifactRecord, err := r.artifacts.GetByID(ctx, *asset.ParseArtifactID)
	if err != nil {
		return r.failRequest(ctx, request, asset, "artifact_load_failed", err)
	}
	if artifactRecord == nil || strings.TrimSpace(artifactRecord.ArtifactStorageKey) == "" {
		return r.failRequest(ctx, request, asset, "artifact_not_ready", datalibraryservice.ErrParsePreviewNotReady)
	}
	parseArtifact, err := r.artifactPersistence.LoadParseArtifact(ctx, artifactRecord.ArtifactStorageKey)
	if err != nil {
		return r.failRequest(ctx, request, asset, "artifact_storage_load_failed", err)
	}

	transformResult, err := r.transform.TransformAuto(ctx, datalibraryservice.ParseArtifactAutoChunkTransformInput{
		TenantID: request.OrganizationID,
		Artifact: parseArtifact,
		FileName: parseArtifact.FileName,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "chunk_transform_failed", err)
	}
	chunkResult, err := r.chunkGeneration.GenerateChunks(ctx, datalibraryservice.GenerateDocumentChunksInput{
		OrganizationID:  request.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		Chunks:          transformResult.Chunks,
		CreatedBy:       request.RequestedBy,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "chunk_persist_failed", err)
	}
	embeddingResult, err := r.embedding.GenerateEmbeddings(ctx, datalibraryservice.GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    request.OrganizationID,
		AssetID:           asset.ID,
		ProcessingRunID:   runID,
		GenerationNo:      generationNo,
		EmbeddingProvider: requestMetadataString(request.RequestMetadata, "embedding_provider"),
		EmbeddingModel:    requestMetadataString(request.RequestMetadata, "embedding_model"),
		RequestedBy:       request.RequestedBy,
		Chunks:            chunkResult.Chunks,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "embedding_failed", err)
	}

	ready, err := r.state.MarkReady(ctx, datalibraryservice.ReadyStateInput{
		RunStateInput: datalibraryservice.RunStateInput{
			OrganizationID:  request.OrganizationID,
			AssetID:         asset.ID,
			ProcessingRunID: runID,
			GenerationNo:    generationNo,
			ParseArtifactID: asset.ParseArtifactID,
		},
		ChunkArtifactSetID: nil,
		ChunkCount:         chunkResult.ChunkCount,
		EmbeddingProvider:  embeddingResult.EmbeddingProvider,
		EmbeddingModel:     embeddingResult.EmbeddingModel,
		EmbeddingDimension: embeddingResult.EmbeddingDimension,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "mark_ready_failed", err)
	}

	_, err = r.processingService.CompleteRequest(ctx, request.OrganizationID, started.ID, map[string]any{
		"processing_run_id":   runID.String(),
		"generation_no":       generationNo,
		"parse_artifact_id":   asset.ParseArtifactID.String(),
		"chunk_count":         chunkResult.ChunkCount,
		"leaf_chunk_count":    chunkResult.LeafCount,
		"embedding_count":     embeddingResult.EmbeddingCount,
		"embedding_provider":  embeddingResult.EmbeddingProvider,
		"embedding_model":     embeddingResult.EmbeddingModel,
		"embedding_dimension": embeddingResult.EmbeddingDimension,
		"chunk_index_type":    string(transformResult.IndexType),
		"chunk_process_mode":  transformResult.ProcessOptions.Mode,
		"chunk_routing":       transformResult.Routing,
		"next_product_status": ready.ProductStatus,
		"next_vector_status":  ready.VectorStatus,
	})
	return err
}

func (r *GenerateCurrentResultRunner) failRequest(ctx context.Context, request *model.ProcessingRequest, asset *model.DocumentAsset, code string, cause error) error {
	if request != nil && r.processingService != nil {
		_, _ = r.processingService.FailRequest(ctx, request.OrganizationID, request.ID, code, cause.Error(), map[string]any{
			"executor_key": GenerateCurrentResultExecutorKey,
		})
	}
	if asset != nil && asset.ProcessingRunID != nil {
		_, _ = r.state.MarkFailed(ctx, datalibraryservice.FailedStateInput{
			RunStateInput: datalibraryservice.RunStateInput{
				OrganizationID:     asset.OrganizationID,
				AssetID:            asset.ID,
				ProcessingRunID:    *asset.ProcessingRunID,
				GenerationNo:       asset.GenerationNo,
				ProcessingStage:    model.DocumentAssetProcessingStageVectorize,
				ProcessingProgress: asset.ProcessingProgress,
				ParseArtifactID:    asset.ParseArtifactID,
			},
			ErrorCode:    code,
			ErrorMessage: cause.Error(),
		})
	}
	return cause
}

func generationRequestRun(request *model.ProcessingRequest, asset *model.DocumentAsset) (uuid.UUID, int64, error) {
	runIDValue := requestMetadataString(request.RequestMetadata, "processing_run_id")
	if runIDValue == "" && asset.ProcessingRunID != nil {
		runIDValue = asset.ProcessingRunID.String()
	}
	runID, err := uuid.Parse(runIDValue)
	if err != nil || runID == uuid.Nil {
		if err != nil {
			return uuid.Nil, 0, err
		}
		return uuid.Nil, 0, datalibraryservice.ErrProcessingRunMismatch
	}
	generationNo := requestMetadataInt64(request.RequestMetadata, "generation_no")
	if generationNo == 0 {
		generationNo = asset.GenerationNo
	}
	if generationNo <= 0 {
		return uuid.Nil, 0, datalibraryservice.ErrProcessingRunMismatch
	}
	return runID, generationNo, nil
}

func requestMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	switch value := metadata[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func requestMetadataInt64(metadata map[string]any, key string) int64 {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case jsonNumber:
		parsed, _ := strconv.ParseInt(value.String(), 10, 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed
	default:
		return 0
	}
}

type jsonNumber interface {
	String() string
}
