package handler

import (
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	chunkexecutor "github.com/zgiai/zgi/api/internal/capabilities/chunking/executor"
	contentparsecap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func NewPlaygroundHandler(module *contentparsecap.Module, runServices ...service.PlaygroundRunService) *PlaygroundHandler {
	if module == nil {
		return nil
	}
	var runs service.PlaygroundRunService
	if len(runServices) > 0 {
		runs = runServices[0]
	}
	return &PlaygroundHandler{
		orchestrator: module.Orchestrator,
		planner:      module.Planner,
		chunkMapper:  module.ChunkMapper,
		chunkPlanner: module.ChunkPlanner,
		catalog:      module.Catalog,
		runs:         runs,
		sessions:     newPlaygroundParseSessionCache(playgroundParseSessionTTL),
	}
}

func (h *PlaygroundHandler) SetProviderCatalogResolver(resolver service.ProviderCatalogResolver) {
	if h != nil {
		h.catalogs = resolver
	}
}

func (h *PlaygroundHandler) SetOrganizationService(service interfaces.OrganizationService) {
	if h != nil {
		h.organization = service
	}
}

func (h *PlaygroundHandler) SetAccountService(service interfaces.AccountService) {
	if h != nil {
		h.account = service
	}
}

func (h *PlaygroundHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.Use(h.requirePlaygroundWorkspace())
	rg.GET("/playground/providers", h.ListProviders)
	rg.GET("/file-route/providers", h.ListFileRouteProviders)
	rg.POST("/playground/parse", h.Parse)
	rg.POST("/playground/save", h.SaveRun)
	rg.GET("/playground/admin/provider-summary", h.GetProviderSummary)
	rg.GET("/playground/compare/:hash", h.CompareRuns)
	rg.GET("/playground/share/:token/source-preview", h.RenderSharedRunSource)
	rg.GET("/playground/share/:token", h.GetSharedRun)
	rg.GET("/playground/runs", h.ListRuns)
	rg.POST("/playground/runs/:id/share", h.EnableRunShare)
	rg.GET("/playground/runs/:id/source-preview", h.RenderSavedRunSource)
	rg.GET("/playground/runs/:id", h.GetRun)
	rg.POST("/playground/pdf-render", h.RenderPDF)
}

func (h *PlaygroundHandler) requirePlaygroundWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetBool(contentParseInternalRouteKey) {
			c.Next()
			return
		}
		if h == nil || h.organization == nil {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		organizationID := strings.TrimSpace(util.GetOrganizationID(c))
		accountID := strings.TrimSpace(util.GetAccountID(c))
		if organizationID == "" || accountID == "" {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		workspaceID, ok := h.resolvePlaygroundWorkspaceID(c, accountID)
		if !ok {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		allowed, err := h.organization.CheckWorkspacePermission(
			c.Request.Context(),
			organizationID,
			workspaceID,
			accountID,
			workspace_model.WorkspacePermissionWorkspaceView,
		)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			c.Abort()
			return
		}
		if !allowed {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *PlaygroundHandler) resolvePlaygroundWorkspaceID(c *gin.Context, accountID string) (string, bool) {
	workspaceID := strings.TrimSpace(util.GetWorkspaceID(c))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(c.Query("workspace_id"))
	}
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(c.PostForm("workspace_id"))
	}
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(c.GetHeader("X-Workspace-ID"))
	}
	if workspaceID == "" && h != nil && h.account != nil {
		accountContext, err := h.account.GetAccountContext(c.Request.Context(), accountID)
		if err == nil && accountContext != nil && accountContext.CurrentWorkspaceID != nil {
			workspaceID = strings.TrimSpace(*accountContext.CurrentWorkspaceID)
		}
	}
	if workspaceID == "" {
		return "", false
	}
	util.SetWorkspaceID(c, workspaceID)
	return workspaceID, true
}

func (h *PlaygroundHandler) ListProviders(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground is not initialized")
		return
	}

	health, err := h.orchestrator.Health(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	catalog, source, err := h.catalogForRequest(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, playgroundProvidersResponse{
		Source:    source,
		Providers: buildPlaygroundProviderStatuses(catalog, health),
		OCR:       buildPlaygroundOCRStatuses(),
	})
}

func (h *PlaygroundHandler) ListFileRouteProviders(c *gin.Context) {
	if h == nil || h.orchestrator == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse is not initialized")
		return
	}
	fileName := strings.TrimSpace(c.Query("file_name"))
	if fileName == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "file_name is required")
		return
	}
	health, err := h.orchestrator.Health(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	catalog, source, err := h.catalogForRequest(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	providers, ext := buildFileRouteProviderStatuses(fileName, catalog, health)
	response.Success(c, fileRouteProvidersResponse{
		Source:    source,
		FileExt:   ext,
		Providers: providers,
	})
}

func (h *PlaygroundHandler) Parse(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(c, response.ErrNoFileUploaded, "please upload a file")
		return
	}
	exec, err := h.executePlaygroundParse(c, fileHeader)
	if err != nil {
		failPlaygroundRequest(c, err)
		return
	}
	h.storePlaygroundParseSession(c, exec)
	response.Success(c, exec.Response)
}

func (h *PlaygroundHandler) executePlaygroundParse(c *gin.Context, fileHeader *multipart.FileHeader) (*playgroundExecution, error) {
	requestStartedAt := time.Now()
	if h == nil || h.orchestrator == nil || h.planner == nil || h.chunkMapper == nil || h.chunkPlanner == nil {
		return nil, newPlaygroundRequestError(response.ErrSystemError, "content parse playground is not initialized")
	}
	if fileHeader.Size > playgroundMaxFileSize {
		return nil, newPlaygroundRequestError(response.ErrFileTooLarge, "file size cannot exceed 64MB")
	}

	readStartedAt := time.Now()
	data, err := readMultipartFile(fileHeader)
	readDuration := time.Since(readStartedAt)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrInvalidParam, err: err}
	}

	provider := strings.TrimSpace(c.PostForm("provider"))
	if provider == "" {
		provider = strings.TrimSpace(c.PostForm("engine"))
	}
	if provider == "" {
		provider = "auto"
	}
	profile := parseProfile(c.PostForm("profile"))
	intent := parseIntent(c.PostForm("intent"))
	ocrEngine := strings.TrimSpace(c.PostForm("ocr_engine"))
	req := contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   fileHeader.Filename,
		Data:       data,
		Intent:     intent,
		Profile:    profile,
		Force:      parseBool(c.PostForm("fresh")),
		Metadata: map[string]any{
			"source":          "content_parse_playground",
			"file_size":       fileHeader.Size,
			"file_sha256":     fileSHA256(data),
			"organization_id": firstNonEmptyString(c.GetString("organization_id"), c.GetString("tenant_id")),
			"workspace_id":    firstNonEmptyString(c.GetString("workspace_id"), c.GetString("tenant_id")),
			"account_id":      c.GetString("account_id"),
		},
	}
	if ocrEngine != "" && ocrEngine != "auto" {
		req.Metadata["ocr_engine"] = ocrEngine
	}
	catalog, catalogSource, err := h.catalogForRequest(c)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrSystemError, err: err}
	}
	req.Metadata["provider_catalog_source"] = catalogSource

	healthStartedAt := time.Now()
	health, err := h.orchestrator.Health(c.Request.Context())
	healthDuration := time.Since(healthStartedAt)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrSystemError, err: err}
	}

	planStartedAt := time.Now()
	plan, effectiveReq, _, err := h.planRequest(req, provider, catalog, health)
	planDuration := time.Since(planStartedAt)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrInvalidParam, err: err}
	}

	artifact, executedReq, executedCandidate, duration, err := h.executeRoutePlan(c, catalog, plan, effectiveReq)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrSystemError, err: err}
	}

	mapStartedAt := time.Now()
	chunkSource, err := h.chunkMapper.FromParseArtifact(artifact)
	mapDuration := time.Since(mapStartedAt)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrSystemError, err: err}
	}
	chunkPlanStartedAt := time.Now()
	chunkPlan, err := h.chunkPlanner.Plan(chunkSource, contracts.ChunkUseCasePreview)
	chunkPlanDuration := time.Since(chunkPlanStartedAt)
	if err != nil {
		return nil, &playgroundRequestError{code: response.ErrSystemError, err: err}
	}
	chunkExecuteStartedAt := time.Now()
	chunkExecution, chunkExecuteErr := chunkexecutor.New().Execute(c.Request.Context(), chunkSource, chunkPlan)
	chunkExecuteDuration := time.Since(chunkExecuteStartedAt)
	performance := buildPlaygroundPerformanceSummary(playgroundPerformanceInput{
		FileSize:               fileHeader.Size,
		TotalDuration:          time.Since(requestStartedAt),
		UploadReadDuration:     readDuration,
		ProviderHealthDuration: healthDuration,
		RoutePlanDuration:      planDuration,
		ParseDuration:          duration,
		ChunkMapDuration:       mapDuration,
		ChunkPlanDuration:      chunkPlanDuration,
		ChunkExecuteDuration:   chunkExecuteDuration,
		RoutePlan:              plan,
		Artifact:               artifact,
		ChunkExecution:         chunkExecution,
		ChunkExecuteErr:        chunkExecuteErr,
	})

	return &playgroundExecution{
		RequestedProviderKey: provider,
		AdapterName:          executedCandidate.AdapterName,
		EffectiveRequest:     executedReq,
		SourceData:           data,
		SourceMimeType:       detectPlaygroundSourceMimeType(fileHeader, data),
		SourceFileExt:        normalizePlaygroundFileExt(fileHeader.Filename, detectPlaygroundSourceMimeType(fileHeader, data)),
		Response: playgroundParseResponse{
			File: playgroundFileSummary{
				Name:   fileHeader.Filename,
				Size:   fileHeader.Size,
				SHA256: fileSHA256(data),
			},
			RoutePlan:      plan,
			Artifact:       artifact,
			ChunkSource:    chunkSource,
			ChunkPlan:      chunkPlan,
			ChunkExecution: chunkExecution,
			QualitySummary: buildPlaygroundQualitySummary(artifact, duration),
			Performance:    performance,
		},
	}, nil
}

func (h *PlaygroundHandler) executeRoutePlan(c *gin.Context, catalog *contracts.ParseProviderCatalog, plan *routing.RoutePlan, req contracts.ParseRequest) (*contracts.ParseArtifact, contracts.ParseRequest, routing.RouteCandidate, time.Duration, error) {
	candidates := routePlanExecutionCandidates(plan)
	if len(candidates) == 0 {
		return nil, req, routing.RouteCandidate{}, 0, fmt.Errorf("content parse route plan has no executable provider")
	}

	startedAt := time.Now()
	attempts := make([]map[string]any, 0, len(candidates))
	var lastErr error
	for index, candidate := range candidates {
		adapterName := strings.TrimSpace(candidate.AdapterName)
		if adapterName == "" {
			continue
		}
		attemptReq := req
		if candidate.EngineName != "" {
			attemptReq.EngineHint = candidate.EngineName
		}
		attemptReq.ProviderRuntime = contentparsecap.RuntimeConfigForCandidate(catalog, candidate)

		attemptStartedAt := time.Now()
		artifact, err := h.orchestrator.ParseWithAdapter(c.Request.Context(), adapterName, attemptReq)
		attempt := map[string]any{
			"provider_key": candidate.ProviderKey,
			"adapter_name": adapterName,
			"engine_name":  candidate.EngineName,
			"duration_ms":  time.Since(attemptStartedAt).Milliseconds(),
		}
		if err != nil {
			attempt["status"] = "failed"
			attempt["error"] = err.Error()
			attempts = append(attempts, attempt)
			lastErr = err
			continue
		}

		attempt["status"] = "succeeded"
		attempts = append(attempts, attempt)
		recordExecutedRoute(plan, candidate, index > 0, attempts)
		if artifact != nil {
			if index > 0 {
				artifact.FallbackUsed = true
			}
			if artifact.Metadata == nil {
				artifact.Metadata = map[string]any{}
			}
			artifact.Metadata["executed_provider_key"] = candidate.ProviderKey
			artifact.Metadata["executed_adapter_name"] = adapterName
			artifact.Metadata["executed_engine_name"] = candidate.EngineName
			artifact.Metadata["route_fallback_used"] = index > 0
		}
		return artifact, attemptReq, candidate, time.Since(startedAt), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no adapter was executable")
	}
	recordExecutedRoute(plan, routing.RouteCandidate{}, false, attempts)
	return nil, req, routing.RouteCandidate{}, time.Since(startedAt), fmt.Errorf("content parse route failed: %w", lastErr)
}

func routePlanExecutionCandidates(plan *routing.RoutePlan) []routing.RouteCandidate {
	if plan == nil {
		return nil
	}
	candidates := make([]routing.RouteCandidate, 0, len(plan.FallbackCandidates)+1)
	if plan.Primary != nil {
		candidates = append(candidates, *plan.Primary)
	}
	candidates = append(candidates, plan.FallbackCandidates...)
	return candidates
}

func recordExecutedRoute(plan *routing.RoutePlan, candidate routing.RouteCandidate, fallbackUsed bool, attempts []map[string]any) {
	if plan == nil {
		return
	}
	if plan.Metadata == nil {
		plan.Metadata = map[string]any{}
	}
	if candidate.ProviderKey != "" {
		plan.Metadata["executed_provider_key"] = candidate.ProviderKey
		plan.Metadata["executed_adapter_name"] = candidate.AdapterName
		plan.Metadata["executed_engine_name"] = candidate.EngineName
		plan.Metadata["fallback_executed"] = fallbackUsed
	}
	if len(attempts) > 0 {
		plan.Metadata["execution_attempts"] = attempts
	}
}
