package modelmeta

import (
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

var (
	ErrProviderRequired = response.ErrorCode{Code: 20001, Message: "provider is required", UserVisible: true}
	ErrSyncFailed       = response.ErrorCode{Code: 20002, Message: "sync failed", UserVisible: true}
)

// Handler handles model metadata sync requests
type Handler struct {
	service                 *Service
	platformCatalogReadOnly bool
}

// NewHandler creates a new model metadata handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) SetPlatformCatalogReadOnly(readOnly bool) {
	h.platformCatalogReadOnly = readOnly
}

func (h *Handler) denyPlatformCatalogWrite(c *gin.Context) bool {
	if !h.platformCatalogReadOnly {
		return false
	}

	response.FailWithMessage(c, response.ErrActionNotAllowed, "当前模式下平台模型目录不可写")
	return true
}

// GetDiff compares local models with remote models and returns differences
// GET /console/api/llm/modelmeta/diff/:provider
func (h *Handler) GetDiff(c *gin.Context) {
	provider := c.Param("provider")
	if provider == "" {
		response.Fail(c, ErrProviderRequired)
		return
	}

	diff, err := h.service.ComputeDiff(c.Request.Context(), provider)
	if err != nil {
		response.FailWithMessage(c, ErrSyncFailed, err.Error())
		return
	}

	response.Success(c, diff)
}

// SyncProviderRequest represents the request body for selective sync
type SyncProviderRequest struct {
	Models []string `json:"models,omitempty"` // Optional: specific models to sync. If empty, sync all.
}

// SyncProvider syncs models for a specific provider
// POST /console/api/llm/modelmeta/sync/:provider
// Body (optional): {"models": ["gpt-4", "gpt-4-turbo"]} for selective sync
func (h *Handler) SyncProvider(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	provider := c.Param("provider")
	if provider == "" {
		response.Fail(c, ErrProviderRequired)
		return
	}

	req, err := bindSyncProviderRequest(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "invalid JSON request body")
		return
	}

	result, err := h.service.SyncProviderModels(c.Request.Context(), provider, req.Models)
	if err != nil {
		response.FailWithMessage(c, ErrSyncFailed, err.Error())
		return
	}

	response.Success(c, result)
}

func bindSyncProviderRequest(c *gin.Context) (SyncProviderRequest, error) {
	var req SyncProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, nil
		}
		return req, err
	}
	return req, nil
}

// GetSyncStatus returns a lightweight sync status summary
// GET /console/api/llm/modelmeta/status
func (h *Handler) GetSyncStatus(c *gin.Context) {
	result, err := h.service.GetSyncStatus(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, ErrSyncFailed, err.Error())
		return
	}
	response.Success(c, result)
}

// DiffProviders returns a detailed diff of all providers (local vs upstream)
// GET /console/api/llm/modelmeta/diff/providers
func (h *Handler) DiffProviders(c *gin.Context) {
	result, err := h.service.DiffProviders(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, ErrSyncFailed, err.Error())
		return
	}
	response.Success(c, result)
}

// SyncProviderWithModels syncs a provider and all its models
// POST /console/api/llm/modelmeta/sync-provider-full/:provider
func (h *Handler) SyncProviderWithModels(c *gin.Context) {
	if h.denyPlatformCatalogWrite(c) {
		return
	}
	provider := c.Param("provider")
	if provider == "" {
		response.Fail(c, ErrProviderRequired)
		return
	}

	result, err := h.service.SyncProviderWithModels(c.Request.Context(), provider)
	if err != nil {
		response.FailWithMessage(c, ErrSyncFailed, err.Error())
		return
	}

	response.Success(c, result)
}
