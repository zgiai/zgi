package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *PlaygroundHandler) SaveRun(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	if sessionID := strings.TrimSpace(c.PostForm("parse_session_id")); sessionID != "" {
		exec, err := h.executionFromParseSession(c, sessionID)
		if err != nil {
			if !errors.Is(err, errPlaygroundParseSessionUnavailable) {
				failPlaygroundRequest(c, err)
				return
			}
		} else {
			h.savePlaygroundExecution(c, exec)
			return
		}
	}
	h.saveRunByParsing(c)
}

func (h *PlaygroundHandler) saveRunByParsing(c *gin.Context) {
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
	h.savePlaygroundExecution(c, exec)
}

func (h *PlaygroundHandler) savePlaygroundExecution(c *gin.Context, exec *playgroundExecution) {
	run := buildPlaygroundRunRecord(c, exec)
	if err := persistPlaygroundSourceFile(run, exec); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if err := h.runs.Create(c.Request.Context(), run); err != nil {
		cleanupPlaygroundSourceFile(run)
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, playgroundSaveResponse{
		Run:         run,
		ParseResult: exec.Response,
	})
}

func (h *PlaygroundHandler) EnableRunShare(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid playground run id")
		return
	}
	item, err := h.runs.EnableShare(c.Request.Context(), id, playgroundRunScope(c))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "playground run not found"})
		return
	}
	response.Success(c, playgroundShareResponse{
		Run:      item,
		ShareURL: buildPlaygroundShareURL(c, item.ShareToken),
	})
}

func (h *PlaygroundHandler) ListRuns(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	filter := playgroundRunScope(c)
	filter.SourceContentHash = strings.TrimSpace(c.Query("source_hash"))
	filter.Limit = parseListLimit(c.Query("limit"), 20, 100)
	items, err := h.runs.List(c.Request.Context(), filter)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, playgroundRunListResponse{Items: items})
}

func (h *PlaygroundHandler) GetRun(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid playground run id")
		return
	}
	item, err := h.runs.GetByID(c.Request.Context(), id, playgroundRunScope(c))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "playground run not found"})
		return
	}
	response.Success(c, item)
}

func (h *PlaygroundHandler) GetSharedRun(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	item, err := h.runs.GetByShareToken(c.Request.Context(), c.Param("token"))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "shared playground run not found"})
		return
	}
	response.Success(c, item)
}

func (h *PlaygroundHandler) CompareRuns(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	sourceHash := strings.TrimSpace(c.Param("hash"))
	if sourceHash == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "source content hash is required")
		return
	}
	filter := playgroundRunScope(c)
	filter.SourceContentHash = sourceHash
	filter.Limit = parseListLimit(c.Query("limit"), 20, 100)
	items, err := h.runs.CompareBySourceHash(c.Request.Context(), filter)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, playgroundCompareResponse{
		SourceContentHash: sourceHash,
		Items:             items,
	})
}

func (h *PlaygroundHandler) GetProviderSummary(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	filter := playgroundRunScope(c)
	filter.Limit = parseListLimit(c.Query("limit"), 200, 500)
	items, err := h.runs.GetProviderSummary(c.Request.Context(), filter)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, playgroundProviderSummaryResponse{Items: items})
}

func buildPlaygroundRunRecord(c *gin.Context, exec *playgroundExecution) *model.PlaygroundRun {
	result := exec.Response
	summary := result.QualitySummary
	durationMS := int(summary.DurationMS)
	finalProvider, adapterName, engineName := resolvePlaygroundProviderIdentity(exec)

	return &model.PlaygroundRun{
		WorkspaceID:          parseContextUUID(c, "workspace_id", "tenant_id"),
		AccountID:            parseContextUUID(c, "account_id"),
		FileName:             result.File.Name,
		FileSize:             result.File.Size,
		SourceContentHash:    result.File.SHA256,
		SourceMimeType:       exec.SourceMimeType,
		SourceFileExt:        exec.SourceFileExt,
		RequestedProviderKey: exec.RequestedProviderKey,
		FinalProviderKey:     finalProvider,
		AdapterName:          adapterName,
		EngineName:           engineName,
		Profile:              string(exec.EffectiveRequest.Profile),
		OCREngine:            summary.OCREngine,
		Status:               string(summary.Status),
		QualityLevel:         string(summary.QualityLevel),
		FallbackUsed:         summary.FallbackUsed,
		DurationMS:           &durationMS,
		ArtifactJSON:         toJSONMap(result.Artifact),
		RoutePlanJSON:        toJSONMap(result.RoutePlan),
		ChunkSourceJSON:      toJSONMap(result.ChunkSource),
		ChunkPlanJSON:        toJSONMap(result.ChunkPlan),
		QualitySummaryJSON:   toJSONMap(summary),
		SummaryJSON: map[string]any{
			"file_name":              result.File.Name,
			"file_size":              result.File.Size,
			"source_content_hash":    result.File.SHA256,
			"requested_provider_key": exec.RequestedProviderKey,
			"final_provider_key":     finalProvider,
			"adapter_name":           adapterName,
			"engine_name":            engineName,
			"text_length":            summary.TextLength,
			"markdown_length":        summary.MarkdownLength,
			"element_count":          summary.ElementCount,
			"bbox_count":             summary.BBoxCount,
			"page_count":             summary.PageCount,
			"ocr_engine":             summary.OCREngine,
			"ocr_strategy":           summary.OCRStrategy,
			"chunk_execution":        result.ChunkExecution,
			"performance_summary":    result.Performance,
		},
	}
}

func resolvePlaygroundProviderIdentity(exec *playgroundExecution) (string, string, string) {
	if exec == nil {
		return "", "", ""
	}
	finalProvider := exec.RequestedProviderKey
	adapterName := exec.AdapterName
	engineName := string(exec.EffectiveRequest.EngineHint)
	if exec.Response.RoutePlan != nil && len(exec.Response.RoutePlan.Metadata) > 0 {
		if value := metadataString(exec.Response.RoutePlan.Metadata, "executed_provider_key"); value != "" {
			finalProvider = value
		}
		if value := metadataString(exec.Response.RoutePlan.Metadata, "executed_adapter_name"); value != "" {
			adapterName = value
		}
		if value := metadataString(exec.Response.RoutePlan.Metadata, "executed_engine_name"); value != "" {
			engineName = value
		}
	}
	if exec.Response.RoutePlan != nil && exec.Response.RoutePlan.Primary != nil {
		primary := exec.Response.RoutePlan.Primary
		if finalProvider == "" && primary.ProviderKey != "" {
			finalProvider = primary.ProviderKey
		}
		if adapterName == "" && primary.AdapterName != "" {
			adapterName = primary.AdapterName
		}
		if engineName == "" && primary.EngineName != "" {
			engineName = string(primary.EngineName)
		}
	}
	if exec.Response.QualitySummary.EngineUsed != "" {
		engineName = string(exec.Response.QualitySummary.EngineUsed)
	}
	return finalProvider, adapterName, engineName
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
