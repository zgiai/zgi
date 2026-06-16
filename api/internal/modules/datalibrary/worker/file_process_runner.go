package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	contentparsecap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
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
	processingRequests       repository.ProcessingRequestRepository
	assets                   repository.DocumentAssetRepository
	files                    filerepository.FileRepository
	storage                  storage.Storage
	contentParse             contracts.ContentParseService
	contentParseOrchestrator *contentparsecap.Orchestrator
	contentParsePlanner      routing.Planner
	providerCatalogs         contentparseservice.ProviderCatalogResolver
	contentParseCatalog      *contracts.ParseProviderCatalog
	state                    datalibraryservice.FileAssetProcessingStateService
	artifactPersistence      datalibraryservice.ParseArtifactPersistenceService
	quality                  datalibraryservice.ParseArtifactQualityService
	processingService        datalibraryservice.ProcessingRequestService
	taskEnqueuer             interface {
		EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error
	}
}

type FileProcessRunnerDeps struct {
	ProcessingRequests       repository.ProcessingRequestRepository
	Assets                   repository.DocumentAssetRepository
	Files                    filerepository.FileRepository
	Storage                  storage.Storage
	ContentParse             contracts.ContentParseService
	ContentParseOrchestrator *contentparsecap.Orchestrator
	ContentParsePlanner      routing.Planner
	ProviderCatalogs         contentparseservice.ProviderCatalogResolver
	ContentParseCatalog      *contracts.ParseProviderCatalog
	State                    datalibraryservice.FileAssetProcessingStateService
	ArtifactPersistence      datalibraryservice.ParseArtifactPersistenceService
	Quality                  datalibraryservice.ParseArtifactQualityService
	ProcessingService        datalibraryservice.ProcessingRequestService
	TaskEnqueuer             interface {
		EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error
	}
}

func NewFileProcessRunner(deps FileProcessRunnerDeps) *FileProcessRunner {
	return &FileProcessRunner{
		processingRequests:       deps.ProcessingRequests,
		assets:                   deps.Assets,
		files:                    deps.Files,
		storage:                  deps.Storage,
		contentParse:             deps.ContentParse,
		contentParseOrchestrator: deps.ContentParseOrchestrator,
		contentParsePlanner:      deps.ContentParsePlanner,
		providerCatalogs:         deps.ProviderCatalogs,
		contentParseCatalog:      deps.ContentParseCatalog,
		state:                    deps.State,
		artifactPersistence:      deps.ArtifactPersistence,
		quality:                  deps.Quality,
		processingService:        deps.ProcessingService,
		taskEnqueuer:             deps.TaskEnqueuer,
	}
}

func (r *FileProcessRunner) SetGenerateCurrentResultEnqueuer(enqueuer interface {
	EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error
}) {
	if r == nil {
		return
	}
	r.taskEnqueuer = enqueuer
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
	artifact, routePlan, err := r.parseFile(ctx, parseRequest, request)
	if err != nil {
		return r.failRequest(ctx, request, asset, "parse_failed", err)
	}

	summary := map[string]interface{}{
		"source_content_hash": asset.ContentHash,
	}
	if routePlan != nil {
		summary["route_plan"] = contentparseservice.RoutePlanSummary(routePlan)
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
		OrganizationID:      request.OrganizationID,
		WorkspaceID:         asset.WorkspaceID,
		AssetID:             asset.ID,
		ProcessingRunID:     runID,
		GenerationNo:        generationNo,
		CreatedBy:           request.RequestedBy,
		SourceFileExtension: uploadFile.Extension,
		SourceFileMimeType:  uploadFile.MimeType,
		Artifact:            artifact,
	})
	if err != nil {
		return r.failRequest(ctx, request, asset, "quality_check_failed", err)
	}

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
	if shouldQueueGenerateAfterParse(request.TargetLevel) {
		if _, err := r.queueGenerateCurrentResultRequest(ctx, request, asset, runID, generationNo); err != nil {
			return r.failRequest(ctx, request, asset, "generate_enqueue_failed", err)
		}
	}

	_, err = r.processingService.CompleteRequest(ctx, request.OrganizationID, started.ID, map[string]any{
		"parse_artifact_id":          persisted.Artifact.ID.String(),
		"artifact_storage_key":       persisted.ArtifactStorageKey,
		"pending_confirmation_count": quality.PendingCount,
		"next_product_status":        model.DocumentAssetProductStatusGenerating,
		"generation_no":              generationNo,
		"parse_provider":             parseProviderFromRequest(request),
		"requested_parse_provider":   parseProviderFromRequest(request),
		"final_parse_provider":       contentparseservice.FinalProviderKey(routePlan, artifact),
		"final_parse_adapter":        contentparseservice.FinalAdapterName(routePlan, artifact),
		"final_parse_engine":         string(contentparseservice.FinalEngineName(routePlan, artifact)),
		"parse_route_fallback_used":  artifact.FallbackUsed,
		"attempted_parse_providers":  contentparseservice.AttemptedProviderOrder(routePlan, artifact),
	})
	return err
}

func (r *FileProcessRunner) queueGenerateCurrentResultRequest(ctx context.Context, request *model.ProcessingRequest, asset *model.DocumentAsset, runID uuid.UUID, generationNo int64) (*datalibraryservice.ProcessingRequestView, error) {
	planned, err := r.processingService.CreatePlannedRequest(ctx, datalibraryservice.ProcessingRequest{
		OrganizationID: request.OrganizationID,
		WorkspaceID:    request.WorkspaceID,
		AssetID:        request.AssetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		RequestedBy:    request.RequestedBy,
		Force:          request.Force,
		RequestMetadata: map[string]any{
			"source":            "file_process_worker",
			"mode":              "generate_current_result",
			"processing_run_id": runID.String(),
			"generation_no":     generationNo,
			"parse_request_id":  request.ID.String(),
			"source_file_id":    asset.SourceFileID,
		},
	})
	if err != nil {
		return nil, err
	}
	queued, err := r.processingService.QueueRequest(ctx, request.OrganizationID, planned.ID)
	if err != nil {
		return nil, err
	}
	if r.taskEnqueuer != nil {
		if err := r.taskEnqueuer.EnqueueGenerateCurrentResult(ctx, planned.ID); err != nil {
			return nil, err
		}
	}
	return queued, nil
}

func (r *FileProcessRunner) parseFile(ctx context.Context, req contracts.ParseRequest, request *model.ProcessingRequest) (*contracts.ParseArtifact, *routing.RoutePlan, error) {
	if r.contentParse == nil {
		return nil, nil, fmt.Errorf("content parse service is not configured")
	}
	if r.contentParsePlanner == nil || r.contentParseOrchestrator == nil {
		artifact, err := r.contentParse.Parse(ctx, req)
		return artifact, nil, err
	}

	catalog, err := r.resolveProviderCatalog(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	health, err := r.contentParse.Health(ctx)
	if err != nil {
		return nil, nil, err
	}
	plan, effectiveReq, err := r.planParseRequest(req, parseProviderFromRequest(request), catalog, health)
	if err != nil {
		return nil, nil, err
	}
	artifact, err := r.executeRoutePlan(ctx, effectiveReq, plan)
	return artifact, plan, err
}

func (r *FileProcessRunner) resolveProviderCatalog(ctx context.Context, request *model.ProcessingRequest) (*contracts.ParseProviderCatalog, error) {
	if r.providerCatalogs == nil {
		if r.contentParseCatalog != nil {
			return r.contentParseCatalog, nil
		}
		return nil, fmt.Errorf("content parse provider catalog is not configured")
	}
	var workspaceID *uuid.UUID
	if request != nil && request.WorkspaceID != nil {
		if parsed, err := uuid.Parse(strings.TrimSpace(*request.WorkspaceID)); err == nil {
			workspaceID = &parsed
		}
	}
	catalog, _, err := r.providerCatalogs.Resolve(ctx, workspaceID)
	return catalog, err
}

func (r *FileProcessRunner) planParseRequest(req contracts.ParseRequest, provider string, catalog *contracts.ParseProviderCatalog, health *contracts.ParseHealth) (*routing.RoutePlan, contracts.ParseRequest, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" || provider == "auto" {
		plan, err := r.contentParsePlanner.Plan(req, catalog, health)
		if err != nil {
			return nil, req, err
		}
		if plan == nil || plan.Primary == nil {
			return nil, req, fmt.Errorf("content parse route plan has no primary provider")
		}
		if plan.Primary.EngineName != "" {
			req.EngineHint = plan.Primary.EngineName
		}
		return plan, req, nil
	}
	if !routing.FileExtensionAllowsProvider(req.FileName, provider) {
		return nil, req, fmt.Errorf("content parse provider %q is not supported for file %q", provider, req.FileName)
	}

	for _, item := range safeProviderCatalog(catalog).Providers {
		if strings.ToLower(strings.TrimSpace(item.Name)) != provider {
			continue
		}
		if !item.Enabled {
			return nil, req, fmt.Errorf("content parse provider %q is not configured or enabled", provider)
		}
		if !adapterAvailable(health, item.Adapter) {
			return nil, req, fmt.Errorf("content parse adapter %q for provider %q is unavailable", item.Adapter, provider)
		}
		if item.Engine != "" {
			req.EngineHint = item.Engine
		}
		return forcedFileRoutePlan(req.Profile, item), req, nil
	}
	return nil, req, fmt.Errorf("unknown content parse provider %q", provider)
}

func (r *FileProcessRunner) executeRoutePlan(ctx context.Context, req contracts.ParseRequest, plan *routing.RoutePlan) (*contracts.ParseArtifact, error) {
	if plan == nil || plan.Primary == nil {
		return r.contentParse.Parse(ctx, req)
	}
	candidates := make([]routing.RouteCandidate, 0, len(plan.FallbackCandidates)+1)
	candidates = append(candidates, *plan.Primary)
	candidates = append(candidates, plan.FallbackCandidates...)
	var lastErr error
	attemptedProviders := make([]string, 0, len(candidates))
	attemptedAdapters := make([]string, 0, len(candidates))
	for index, candidate := range candidates {
		adapterName := strings.TrimSpace(candidate.AdapterName)
		if adapterName == "" {
			continue
		}
		providerKey := strings.TrimSpace(candidate.ProviderKey)
		if providerKey != "" {
			attemptedProviders = append(attemptedProviders, providerKey)
		}
		attemptedAdapters = append(attemptedAdapters, adapterName)
		attemptReq := req
		if candidate.EngineName != "" {
			attemptReq.EngineHint = candidate.EngineName
		}
		artifact, err := r.contentParseOrchestrator.ParseWithAdapter(ctx, adapterName, attemptReq)
		if err != nil {
			lastErr = err
			continue
		}
		contentparseservice.ApplyRouteExecutionMetadata(artifact, candidate, attemptedProviders, attemptedAdapters, index > 0)
		return artifact, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("content parse route failed: %w", lastErr)
	}
	return nil, fmt.Errorf("content parse route has no executable provider")
}

func forcedFileRoutePlan(profile contracts.ParseProfile, provider contracts.ParseProviderConfig) *routing.RoutePlan {
	return &routing.RoutePlan{
		Mode:            profile,
		RequestedEngine: provider.Engine,
		Primary: &routing.RouteCandidate{
			ProviderKey:  provider.Name,
			AdapterName:  provider.Adapter,
			EngineName:   provider.Engine,
			Priority:     provider.Priority,
			FallbackOnly: provider.FallbackOnly,
			Reason: map[string]any{
				"selection": "file_processing_forced_provider",
			},
		},
		Metadata: map[string]any{
			"forced_provider": true,
		},
	}
}

func safeProviderCatalog(catalog *contracts.ParseProviderCatalog) *contracts.ParseProviderCatalog {
	if catalog != nil {
		return catalog
	}
	return &contracts.ParseProviderCatalog{}
}

func adapterAvailable(health *contracts.ParseHealth, adapter string) bool {
	adapter = strings.TrimSpace(adapter)
	if adapter == "" {
		return false
	}
	if health == nil {
		return true
	}
	for _, item := range health.Adapters {
		if item.Name == adapter {
			return item.Available
		}
	}
	return false
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
	if request != nil && asset != nil && asset.ProcessingRunID != nil && *asset.ProcessingRunID == request.ID {
		_, _ = r.state.MarkFailed(ctx, datalibraryservice.FailedStateInput{
			RunStateInput: datalibraryservice.RunStateInput{
				OrganizationID:     asset.OrganizationID,
				AssetID:            asset.ID,
				ProcessingRunID:    request.ID,
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
	provider := parseProviderFromRequest(request)
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
			"parse_provider":        provider,
		},
	}
}

func parseProviderFromRequest(request *model.ProcessingRequest) string {
	if request == nil {
		return "auto"
	}
	provider := metadataString(request.RequestMetadata, "parse_provider")
	if provider == "" {
		provider = metadataString(request.RequestMetadata, "provider")
	}
	if provider == "" {
		return "auto"
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case contracts.ParseEngine:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func shouldQueueGenerateAfterParse(targetLevel string) bool {
	switch targetLevel {
	case model.DocumentProcessingLevelVectorize, model.DocumentProcessingLevelFull:
		return true
	default:
		return false
	}
}
