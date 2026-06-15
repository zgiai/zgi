package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparserepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

const GenerateCurrentResultExecutorKey = "data-library-generate-current-result"

type GenerateCurrentResultRunner struct {
	processingRequests  repository.ProcessingRequestRepository
	assets              repository.DocumentAssetRepository
	artifacts           contentparserepo.ArtifactRepository
	state               datalibraryservice.FileAssetProcessingStateService
	artifactPersistence datalibraryservice.ParseArtifactPersistenceService
	parseConfirmations  repository.ParseConfirmationItemRepository
	transform           datalibraryservice.ParseArtifactChunkTransformService
	chunkGeneration     datalibraryservice.DocumentChunkGenerationService
	embedding           datalibraryservice.DocumentChunkEmbeddingService
	embeddingTargets    generateCurrentResultEmbeddingTargetStore
	processingService   datalibraryservice.ProcessingRequestService
	refs                generateCurrentResultRefStore
	datasets            generateCurrentResultDatasetStore
	datasetRefSync      generateCurrentResultDatasetRefSyncEnqueuer
}

type GenerateCurrentResultRunnerDeps struct {
	ProcessingRequests  repository.ProcessingRequestRepository
	Assets              repository.DocumentAssetRepository
	Artifacts           contentparserepo.ArtifactRepository
	State               datalibraryservice.FileAssetProcessingStateService
	ArtifactPersistence datalibraryservice.ParseArtifactPersistenceService
	ParseConfirmations  repository.ParseConfirmationItemRepository
	Transform           datalibraryservice.ParseArtifactChunkTransformService
	ChunkGeneration     datalibraryservice.DocumentChunkGenerationService
	Embedding           datalibraryservice.DocumentChunkEmbeddingService
	EmbeddingTargets    generateCurrentResultEmbeddingTargetStore
	ProcessingService   datalibraryservice.ProcessingRequestService
	Refs                generateCurrentResultRefStore
	Datasets            generateCurrentResultDatasetStore
	DatasetRefSync      generateCurrentResultDatasetRefSyncEnqueuer
}

type generateCurrentResultRefStore interface {
	ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error)
	MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error)
}

type generateCurrentResultEmbeddingTargetStore interface {
	ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error)
	ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error)
}

type generateCurrentResultDatasetStore interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error)
}

type generateCurrentResultDatasetRefSyncEnqueuer interface {
	EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error
}

func NewGenerateCurrentResultRunner(deps GenerateCurrentResultRunnerDeps) *GenerateCurrentResultRunner {
	return &GenerateCurrentResultRunner{
		processingRequests:  deps.ProcessingRequests,
		assets:              deps.Assets,
		artifacts:           deps.Artifacts,
		state:               deps.State,
		artifactPersistence: deps.ArtifactPersistence,
		parseConfirmations:  deps.ParseConfirmations,
		transform:           deps.Transform,
		chunkGeneration:     deps.ChunkGeneration,
		embedding:           deps.Embedding,
		embeddingTargets:    deps.EmbeddingTargets,
		processingService:   deps.ProcessingService,
		refs:                deps.Refs,
		datasets:            deps.Datasets,
		datasetRefSync:      deps.DatasetRefSync,
	}
}

func (r *GenerateCurrentResultRunner) SetDatasetRefSyncEnqueuer(enqueuer generateCurrentResultDatasetRefSyncEnqueuer) {
	if r == nil {
		return
	}
	r.datasetRefSync = enqueuer
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
	embeddingTargets, err := datalibraryservice.CollectEmbeddingTargets(ctx, datalibraryservice.CollectEmbeddingTargetsInput{
		OrganizationID:    request.OrganizationID,
		Asset:             asset,
		AssetID:           asset.ID,
		EmbeddingProvider: requestMetadataString(request.RequestMetadata, "embedding_provider"),
		EmbeddingModel:    requestMetadataString(request.RequestMetadata, "embedding_model"),
		Embeddings:        r.embeddingTargets,
		Refs:              r.refs,
		Datasets:          r.datasets,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "embedding_target_collect_failed", err)
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
	parseQualityItems, err := r.loadPendingParseQualityItems(ctx, request.OrganizationID, asset.ID, runID, generationNo)
	if err != nil {
		return r.failRequest(ctx, request, asset, "quality_items_load_failed", err)
	}
	attachParseQualityIssues(parseArtifact, parseQualityItems)

	transformResult, err := r.transform.TransformAuto(ctx, datalibraryservice.ParseArtifactAutoChunkTransformInput{
		TenantID: request.OrganizationID,
		Artifact: parseArtifact,
		FileName: parseArtifact.FileName,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "chunk_transform_failed", err)
	}
	var processRule map[string]interface{}
	if transformResult.ProcessOptions != nil {
		processRule = transformResult.ProcessOptions.ProcessRule
	}
	chunkResult, err := r.chunkGeneration.GenerateChunks(ctx, datalibraryservice.GenerateDocumentChunksInput{
		OrganizationID:  request.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		Chunks:          transformResult.Chunks,
		ProcessRule:     processRule,
		CreatedBy:       request.RequestedBy,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "chunk_persist_failed", err)
	}
	embeddingResult, totalEmbeddingCount, err := r.generateEmbeddingsForTargets(ctx, datalibraryservice.GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:  request.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: runID,
		GenerationNo:    generationNo,
		RequestedBy:     request.RequestedBy,
		Chunks:          chunkResult.Chunks,
	}, embeddingTargets)
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
		"processing_run_id":     runID.String(),
		"generation_no":         generationNo,
		"parse_artifact_id":     asset.ParseArtifactID.String(),
		"chunk_count":           chunkResult.PrimaryChunkCount,
		"primary_chunk_count":   chunkResult.PrimaryChunkCount,
		"secondary_chunk_count": chunkResult.SecondaryChunkCount,
		"embedding_count":       totalEmbeddingCount,
		"embedding_provider":    embeddingResult.EmbeddingProvider,
		"embedding_model":       embeddingResult.EmbeddingModel,
		"embedding_dimension":   embeddingResult.EmbeddingDimension,
		"chunk_index_type":      string(transformResult.IndexType),
		"chunk_process_mode":    transformResult.ProcessOptions.Mode,
		"chunk_routing":         transformResult.Routing,
		"next_product_status":   ready.ProductStatus,
		"next_vector_status":    ready.VectorStatus,
	})
	if err != nil {
		return err
	}
	return r.enqueueDatasetRefSyncs(ctx, ready, generationNo)
}

func (r *GenerateCurrentResultRunner) generateEmbeddingsForTargets(ctx context.Context, base datalibraryservice.GenerateDocumentChunkEmbeddingsInput, targets []datalibraryservice.EmbeddingTarget) (*datalibraryservice.GenerateDocumentChunkEmbeddingsResult, int, error) {
	if len(targets) == 0 {
		targets = []datalibraryservice.EmbeddingTarget{{}}
	}
	var first *datalibraryservice.GenerateDocumentChunkEmbeddingsResult
	total := 0
	for index, target := range targets {
		input := base
		input.EmbeddingProvider = target.Provider
		input.EmbeddingModel = target.Model
		var (
			result *datalibraryservice.GenerateDocumentChunkEmbeddingsResult
			err    error
		)
		if index == 0 {
			result, err = r.embedding.GenerateEmbeddings(ctx, input)
		} else {
			result, err = r.embedding.GenerateAdditionalEmbeddings(ctx, input)
		}
		if err != nil {
			return nil, total, err
		}
		if first == nil {
			first = result
		}
		if result != nil {
			total += result.EmbeddingCount
		}
	}
	return first, total, nil
}

func (r *GenerateCurrentResultRunner) loadPendingParseQualityItems(ctx context.Context, organizationID string, assetID uuid.UUID, runID uuid.UUID, generationNo int64) ([]*model.ParseConfirmationItem, error) {
	if r == nil || r.parseConfirmations == nil {
		return nil, nil
	}
	items := make([]*model.ParseConfirmationItem, 0)
	offset := 0
	for {
		pageItems, total, err := r.parseConfirmations.List(ctx, repository.ParseConfirmationItemListFilter{
			OrganizationID:  organizationID,
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    &generationNo,
			Status:          model.ParseConfirmationItemStatusPending,
			Limit:           200,
			Offset:          offset,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		offset += len(pageItems)
		if len(pageItems) == 0 || int64(offset) >= total {
			break
		}
	}
	return items, nil
}

func attachParseQualityIssues(artifact *contracts.ParseArtifact, items []*model.ParseConfirmationItem) {
	if artifact == nil || len(artifact.Elements) == 0 || len(items) == 0 {
		return
	}
	issuesByElementIndex := make(map[int][]map[string]any)
	for _, item := range items {
		if item == nil {
			continue
		}
		elementIndex, ok := locatorInt(item.SourceLocatorJSON, "element_index")
		if !ok || elementIndex < 0 {
			continue
		}
		issue := map[string]any{
			"id":             item.ID.String(),
			"type":           item.ItemType,
			"status":         item.Status,
			"source_locator": item.SourceLocatorJSON,
		}
		if item.ReviewReason != nil && strings.TrimSpace(*item.ReviewReason) != "" {
			issue["reason"] = strings.TrimSpace(*item.ReviewReason)
		}
		if item.Confidence != nil {
			issue["confidence"] = *item.Confidence
		}
		if content := strings.TrimSpace(item.OriginalContent); content != "" {
			issue["original_content"] = content
			issue["content_excerpt"] = textExcerpt(content, 120)
		}
		issuesByElementIndex[elementIndex] = append(issuesByElementIndex[elementIndex], issue)
	}
	for index, issues := range issuesByElementIndex {
		if index >= len(artifact.Elements) || len(issues) == 0 {
			continue
		}
		if artifact.Elements[index].Metadata == nil {
			artifact.Elements[index].Metadata = make(map[string]any)
		}
		artifact.Elements[index].Metadata["quality_issues"] = issues
		artifact.Elements[index].Metadata["quality_issue_count"] = len(issues)
		artifact.Elements[index].Metadata["has_quality_issues"] = true
	}
}

func locatorInt(locator map[string]any, key string) (int, bool) {
	if locator == nil {
		return 0, false
	}
	switch value := locator[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case float32:
		return int(value), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func textExcerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func (r *GenerateCurrentResultRunner) failRequest(ctx context.Context, request *model.ProcessingRequest, asset *model.DocumentAsset, code string, cause error) error {
	if request != nil && r.processingService != nil {
		_, _ = r.processingService.FailRequest(ctx, request.OrganizationID, request.ID, code, cause.Error(), map[string]any{
			"executor_key": GenerateCurrentResultExecutorKey,
		})
	}
	if runID, generationNo, ok := strictGenerationRequestRun(request); ok &&
		asset != nil &&
		asset.ProcessingRunID != nil &&
		*asset.ProcessingRunID == runID &&
		asset.GenerationNo == generationNo {
		_, _ = r.state.MarkFailed(ctx, datalibraryservice.FailedStateInput{
			RunStateInput: datalibraryservice.RunStateInput{
				OrganizationID:     asset.OrganizationID,
				AssetID:            asset.ID,
				ProcessingRunID:    runID,
				GenerationNo:       generationNo,
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

func (r *GenerateCurrentResultRunner) enqueueDatasetRefSyncs(ctx context.Context, asset *model.DocumentAsset, generationNo int64) error {
	if r == nil || r.refs == nil || r.datasetRefSync == nil || asset == nil {
		return nil
	}
	refs, err := r.refs.ListActiveByAsset(ctx, asset.OrganizationID, asset.ID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref == nil || ref.DatasetID == "" {
			continue
		}
		syncRunID := uuid.New()
		if _, err := r.refs.MarkPending(ctx, asset.OrganizationID, ref.ID, syncRunID, nil, nil); err != nil {
			return err
		}
		if err := r.datasetRefSync.EnqueueDatasetRefSync(ctx, ref.ID, asset.ID, ref.DatasetID, generationNo, syncRunID); err != nil {
			return err
		}
	}
	return nil
}

func strictGenerationRequestRun(request *model.ProcessingRequest) (uuid.UUID, int64, bool) {
	if request == nil {
		return uuid.Nil, 0, false
	}
	runIDValue := requestMetadataString(request.RequestMetadata, "processing_run_id")
	runID, err := uuid.Parse(runIDValue)
	if err != nil || runID == uuid.Nil {
		return uuid.Nil, 0, false
	}
	generationNo := requestMetadataInt64(request.RequestMetadata, "generation_no")
	if generationNo <= 0 {
		return uuid.Nil, 0, false
	}
	return runID, generationNo, true
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
