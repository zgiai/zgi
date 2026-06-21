package APIKey

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"

	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// APIKeyHandler handles HTTP requests for API key operations
type APIKeyHandler struct {
	apiKeyService          *APIKeyService
	apiKeyRepo             APIKeyRepository
	apiKeyUsageLogRepo     APIKeyUsageLogRepository
	organizationService    apiKeyWorkspacePermissionChecker
	agentWorkspaceResolver apiKeyAgentWorkspaceResolver
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(apiKeyService *APIKeyService, apiKeyRepo APIKeyRepository, apiKeyUsageLogRepo APIKeyUsageLogRepository, organizationService interfaces.OrganizationService, db *gorm.DB) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService:          apiKeyService,
		apiKeyRepo:             apiKeyRepo,
		apiKeyUsageLogRepo:     apiKeyUsageLogRepo,
		organizationService:    organizationService,
		agentWorkspaceResolver: newDBAPIKeyAgentWorkspaceResolver(db),
	}
}

// CreateAPIKey creates a new API key
// @Summary Create API key
// @Description Create a new API key for an agent
// @Tags API Keys
// @Accept json
// @Produce json
// @Param request body CreateAPIKeyRequest true "Create API key request"
// @Success 201 {object} CreateAPIKeyResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/api-keys [post]
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			logger.CriticalContext(c.Request.Context(), "panic in create api key", "panic", r)
			c.JSON(500, gin.H{"error": fmt.Sprintf("panic: %v", r)})
		}
	}()

	logger.DebugContext(c.Request.Context(), "create api key request started")

	// Get agent ID from path parameter
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "invalid agent id in create api key request", "agent_id", agentIDStr, err)
		response.Fail(c, response.ErrInvalidUuid)
		return
	}
	logger.DebugContext(c.Request.Context(), "create api key agent parsed", "agent_id", agentID.String())

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	// Parse request body without agent_id
	var req CreateAPIKeyRequestWithoutAgentID
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid create api key request body", err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	logger.DebugContext(c.Request.Context(), "create api key request parsed", "agent_id", agentID.String(), "name", req.Name)

	// Check if API key name already exists for this agent
	nameExists, err := h.apiKeyRepo.CheckNameExists(c.Request.Context(), req.Name, agentID, tenantUUID, nil)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to check api key name existence", "agent_id", agentID.String(), "tenant_id", tenantUUID.String(), err)
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Failed to check name existence", UserVisible: false})
		return
	}
	if nameExists {
		logger.WarnContext(c.Request.Context(), "api key name already exists", "agent_id", agentID.String(), "tenant_id", tenantUUID.String(), "name", req.Name)
		response.Fail(c, response.ErrorCode{Code: 400001, Message: "API key name already exists for this agent", UserVisible: true})
		return
	}

	// Create API key
	apiKeyRecord, apiKey, err := h.apiKeyService.CreateAPIKey(c.Request.Context(), agentID, tenantUUID, req.Name, req.ExpiresAt)
	if err != nil {
		c.JSON(500, gin.H{"error": "CreateAPIKey failed", "details": err.Error()})
		return
	}

	response.Success(c, apiKeyRecord.ToCreateResponse(apiKey))
}

// ListAPIKeys lists all API keys for an agent
// @Summary List API keys
// @Description Get all API keys for a specific agent
// @Tags API Keys
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} ListAPIKeysResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys [get]
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	apiKeys, err := h.apiKeyRepo.List(c.Request.Context(), agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 301003, Message: "Failed to list API keys", UserVisible: true})
		return
	}

	// Filter out revoked (deleted) API keys
	activeAPIKeys := make([]*APIKey, 0)
	for _, apiKey := range apiKeys {
		activeAPIKeys = append(activeAPIKeys, apiKey)
	}

	// Convert to response format
	apiKeyResponses := make([]APIKeyResponse, len(activeAPIKeys))
	for i, apiKey := range activeAPIKeys {
		apiKeyResponses[i] = apiKey.ToResponse()
	}

	response.Success(c, ListAPIKeysResponse{
		APIKeys: apiKeyResponses,
		Total:   len(apiKeyResponses),
	})
}

// GetAPIKey gets a specific API key
// @Summary Get API key
// @Description Get details of a specific API key
// @Tags API Keys
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param key_id path string true "API Key ID"
// @Success 200 {object} APIKeyResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{key_id} [get]
func (h *APIKeyHandler) GetAPIKey(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	apiKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID, agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "API key not found", UserVisible: true})
		return
	}

	response.Success(c, apiKey.ToResponse())
}

// UpdateAPIKey updates an existing API key
// @Summary Update API key
// @Description Update an existing API key
// @Tags API Keys
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param key_id path string true "API Key ID"
// @Param request body UpdateAPIKeyRequest true "Update API key request"
// @Success 200 {object} APIKeyResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{key_id} [put]
func (h *APIKeyHandler) UpdateAPIKey(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.Name != nil {
		// Check if the new name already exists for this agent (excluding current key)
		nameExists, err := h.apiKeyRepo.CheckNameExists(c.Request.Context(), *req.Name, agentID, tenantUUID, &keyID)
		if err != nil {
			response.Fail(c, response.ErrorCode{Code: 500001, Message: "Failed to check name existence", UserVisible: false})
			return
		}
		if nameExists {
			response.Fail(c, response.ErrorCode{Code: 400001, Message: "API key name already exists for this agent", UserVisible: true})
			return
		}
		updates["name"] = *req.Name
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Update the API key
	err = h.apiKeyRepo.Update(c.Request.Context(), keyID, agentID, tenantUUID, updates)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 301004, Message: "Failed to update API key", UserVisible: true})
		return
	}

	// Get updated API key
	updatedAPIKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID, agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 301005, Message: "Failed to retrieve updated API key", UserVisible: true})
		return
	}

	response.Success(c, updatedAPIKey.ToResponse())
}

// DeleteAPIKey soft deletes an API key by setting its status to 'revoked'
// @Summary Delete API key
// @Description Soft delete an existing API key (sets status to revoked)
// @Tags API Keys
// @Param agent_id path string true "Agent ID"
// @Param api_key_id path string true "API Key ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{api_key_id} [delete]
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Soft delete: set status to revoked instead of hard delete
	err = h.apiKeyRepo.Update(c.Request.Context(), keyID, agentID, tenantUUID, map[string]interface{}{
		"status": APIKeyStatusRevoked,
	})
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "API key not found", UserVisible: true})
		return
	}

	response.Success(c, gin.H{"message": "API key deleted successfully"})
}

// RevokeAPIKey revokes an API key
// @Summary Revoke API key
// @Description Revoke an existing API key
// @Tags API Keys
// @Param agent_id path string true "Agent ID"
// @Param key_id path string true "API Key ID"
// @Success 200 {object} APIKeyResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{key_id}/revoke [post]
func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Revoke the API key
	err = h.apiKeyRepo.Update(c.Request.Context(), keyID, agentID, tenantUUID, map[string]interface{}{
		"status": APIKeyStatusRevoked,
	})
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 301006, Message: "Failed to revoke API key", UserVisible: true})
		return
	}

	// Get updated API key
	updatedAPIKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID, agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 301007, Message: "Failed to retrieve revoked API key", UserVisible: true})
		return
	}

	response.Success(c, updatedAPIKey.ToResponse())
}

// hashAPIKey creates a hash of the API key for validation
func (h *APIKeyHandler) hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetAPIKeyUsageLogs retrieves usage logs for a specific API key
// @Summary Get API key usage logs
// @Description Get usage logs for a specific API key with pagination and date filtering
// @Tags API Keys
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param api_key_id path string true "API Key ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Page size" default(20)
// @Param start_date query string false "Start date (RFC3339 format)"
// @Param end_date query string false "End date (RFC3339 format)"
// @Success 200 {object} ListAPIKeyUsageLogsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{api_key_id}/usage [get]
func (h *APIKeyHandler) GetAPIKeyUsageLogs(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Verify API key belongs to the agent and tenant
	apiKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID, agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "API key not found", UserVisible: true})
		return
	}

	// Parse pagination parameters
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Parse date filters
	var startDate, endDate *time.Time
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if t, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			startDate = &t
		}
	}
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if t, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			endDate = &t
		}
	}

	// Get usage logs
	logs, total, err := h.apiKeyUsageLogRepo.GetByAPIKeyID(c.Request.Context(), apiKey.ID, page, limit, startDate, endDate)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Failed to get usage logs", UserVisible: false})
		return
	}

	// Convert to response format
	logResponses := make([]APIKeyUsageLogResponse, len(logs))
	for i, log := range logs {
		logResponses[i] = log.ToResponse()
	}

	hasMore := int64(page*limit) < total

	response.Success(c, ListAPIKeyUsageLogsResponse{
		Logs:    logResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: hasMore,
	})
}

// GetAPIKeyUsageStats retrieves usage statistics for a specific API key
// @Summary Get API key usage statistics
// @Description Get aggregated usage statistics for a specific API key
// @Tags API Keys
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param api_key_id path string true "API Key ID"
// @Param start_date query string false "Start date (RFC3339 format)"
// @Param end_date query string false "End date (RFC3339 format)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /api/v1/agents/{agent_id}/api-keys/{api_key_id}/usage/stats [get]
func (h *APIKeyHandler) GetAPIKeyUsageStats(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	tenantUUID, ok := h.requireAgentAPIKeyAccess(c, agentID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	keyIDStr := c.Param("api_key_id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Verify API key belongs to the agent and tenant
	apiKey, err := h.apiKeyRepo.GetByID(c.Request.Context(), keyID, agentID, tenantUUID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "API key not found", UserVisible: true})
		return
	}

	// Parse date filters
	var startDate, endDate *time.Time
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if t, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			startDate = &t
		}
	}
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if t, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			endDate = &t
		}
	}

	// Get usage statistics
	stats, err := h.apiKeyUsageLogRepo.GetUsageStats(c.Request.Context(), apiKey.ID, startDate, endDate)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Failed to get usage statistics", UserVisible: false})
		return
	}

	response.Success(c, stats)
}
