package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/pkg/response"
)

// ChannelHandler handles channel and route HTTP requests
type ChannelHandler struct {
	shared.BaseHandler
	service         service.ChannelService
	consoleProvider console.ConsoleProvider
}

func NewChannelHandler(svc service.ChannelService, cp console.ConsoleProvider) *ChannelHandler {
	return &ChannelHandler{service: svc, consoleProvider: cp}
}

func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	orgIDStr := c.GetString("organization_id")
	if orgIDStr == "" {
		orgIDStr = c.GetHeader("X-Organization-ID")
	}
	if orgIDStr == "" {
		return uuid.Nil, fmt.Errorf("organization_id not found")
	}
	return uuid.Parse(orgIDStr)
}

// Tenant Route Operations

func (h *ChannelHandler) CreateRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid organization id")
		return
	}

	req, ok := parseCreateRouteRequest(c)
	if !ok {
		return
	}

	route, err := h.service.CreateRoute(c.Request.Context(), organizationID, req)
	if err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, route)
}

func parseCreateRouteRequest(c *gin.Context) (*dto.CreateRouteRequest, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "failed to read request body")
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		response.FailWithMessage(c, response.ErrInvalidParam, "request body is required")
		return nil, false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return nil, false
	}

	var req dto.CreateRouteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return nil, false
	}

	if _, exists := payload["initial_quota"]; exists {
		response.FailWithMessage(c, response.ErrInvalidParam, "initial_quota is not supported, please use initial_funds")
		return nil, false
	}
	if _, exists := payload["protocol"]; exists {
		response.FailWithMessage(c, response.ErrInvalidParam, "protocol is not supported, please use channel_provider")
		return nil, false
	}
	if _, exists := payload["supported_protocols"]; exists {
		response.FailWithMessage(c, response.ErrInvalidParam, "supported_protocols is not supported, please use channel_provider")
		return nil, false
	}

	if strings.TrimSpace(req.ChannelProvider) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "channel_provider is required")
		return nil, false
	}
	spec, err := channelprovider.ValidateConnectionFields(req.ChannelProvider, req.APIBaseURL)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return nil, false
	}
	if err := channelprovider.ValidateAPIKey(spec, req.APIKey); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return nil, false
	}

	return &req, true
}

func (h *ChannelHandler) GetRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid organization id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid route id")
		return
	}
	route, err := h.service.GetRoute(c.Request.Context(), organizationID, id)
	if err != nil {
		h.NotFound(c, err)
		return
	}
	h.Success(c, route)
}

func (h *ChannelHandler) ListRoutes(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListRouteRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	routes, total, err := h.service.ListRoutes(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.SuccessList(c, routes, total, req.Page, req.PageSize)
}

// ListRoutesAggregated returns tenant routes grouped for display.
// Official routes are grouped together, while user-managed routes remain separate.
func (h *ChannelHandler) ListRoutesAggregated(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.ListRoutesAggregatedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	result, err := h.service.ListRoutesAggregated(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

func (h *ChannelHandler) UpdateRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid channel id")
		return
	}
	var req dto.UpdateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	route, err := h.service.UpdateRoute(c.Request.Context(), organizationID, id, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, route)
}

func (h *ChannelHandler) DeleteRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid channel id")
		return
	}
	if err := h.service.DeleteRoute(c.Request.Context(), organizationID, id); err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, nil)
}

// SelectRoute selects the best route for a given model
// POST /routes/select
func (h *ChannelHandler) SelectRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	var req dto.SelectRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	route, err := h.service.SelectRoute(c.Request.Context(), organizationID, req.Model)
	if err != nil {
		h.NotFound(c, err)
		return
	}
	h.Success(c, route)
}

// GetRoutesForModel returns all available routes for a given model
// GET /routes/by-model?model=xxx
func (h *ChannelHandler) GetRoutesForModel(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	model := c.Query("model")
	if model == "" {
		h.InvalidParam(c, "model is required")
		return
	}
	routes, err := h.service.GetRoutesForModel(c.Request.Context(), organizationID, model)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, routes)
}

// InitTenantRoutes initializes routes for a tenant from system channels
// POST /routes/init
func (h *ChannelHandler) InitTenantRoutes(c *gin.Context) {
	h.Success(c, gin.H{"message": "auto-initialization disabled - please create routes manually"})
}

// ToggleRoute quickly enables or disables a route
// POST /routes/:id/toggle
func (h *ChannelHandler) ToggleRoute(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid tenant id")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid channel id")
		return
	}
	var req dto.ToggleRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	route, err := h.service.UpdateRoute(c.Request.Context(), organizationID, id, &dto.UpdateRouteRequest{
		IsEnabled: &req.IsEnabled,
	})
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, route)
}

// TestRoute tests a tenant route connectivity
// POST /routes/:id/test
func (h *ChannelHandler) TestRoute(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	id, ok := h.ParseUUID(c, "id")
	if !ok {
		return
	}

	// Parse optional request body for model parameter
	var req dto.TestRouteRequest
	// Body is optional, so we ignore bind errors (empty body is valid)
	_ = c.ShouldBindJSON(&req)

	result, err := h.service.TestRoute(c.Request.Context(), organizationID, id, req.Model)
	if err != nil {
		h.NotFound(c, err)
		return
	}
	h.Success(c, result)
}

// UpdateChannelBalance updates the balance for a channel
// PUT /channels/:id/balance
func (h *ChannelHandler) UpdateChannelBalance(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	id, ok := h.ParseUUID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.UpdateChannelBalance(c.Request.Context(), id, organizationID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// AdjustChannelWallet manually adjusts private channel wallet funds.
// POST /channels/:id/wallet/adjust
func (h *ChannelHandler) AdjustChannelWallet(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	id, ok := h.ParseUUID(c, "id")
	if !ok {
		return
	}

	var req dto.AdjustChannelWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	result, err := h.service.AdjustChannelWallet(c.Request.Context(), organizationID, id, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// GetPlatformChannel returns the aggregated official channel view for the current tenant.
// GET /channels/platform
func (h *ChannelHandler) GetPlatformChannel(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	result, err := h.service.GetPlatformChannel(c.Request.Context(), organizationID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// UpdatePlatformChannelSettings updates the tenant's official channel priority/weight/is_enabled.
// PUT /channels/platform
func (h *ChannelHandler) UpdatePlatformChannelSettings(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}

	req, ok := parsePlatformUpdateChannelRequest(c)
	if !ok {
		return
	}

	if err := h.service.UpdatePlatformChannelSettings(c.Request.Context(), organizationID, req); err != nil {
		if errors.Is(err, service.ErrNoAvailableOfficialChannel) {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "official channel settings updated"})
}

// UpdatePlatformChannel updates routing-related fields of a platform channel.
// PATCH /channels/platform/:id
func (h *ChannelHandler) UpdatePlatformChannel(c *gin.Context) {
	channelID := c.Param("id")
	if channelID == "" {
		h.Error(c, fmt.Errorf("channel ID is required"))
		return
	}

	req, ok := parsePlatformUpdateChannelRequest(c)
	if !ok {
		return
	}

	if err := h.service.UpdatePlatformChannel(c.Request.Context(), channelID, req); err != nil {
		h.Error(c, err)
		return
	}

	h.Success(c, gin.H{"message": "channel updated"})
}

func parsePlatformUpdateChannelRequest(c *gin.Context) (*dto.UpdatePlatformChannelRequest, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "failed to read request body")
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		response.FailWithMessage(c, response.ErrInvalidParam, "request body is required")
		return nil, false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return nil, false
	}
	if _, exists := payload["is_active"]; exists {
		response.FailWithMessage(c, response.ErrInvalidParam, "is_active is not supported, please use is_enabled")
		return nil, false
	}

	var req dto.UpdatePlatformChannelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return nil, false
	}
	return &req, true
}

// ListPlatformChannelModels returns deduplicated models across all active platform channels.
// GET /channels/platform/models
func (h *ChannelHandler) ListPlatformChannelModels(c *gin.Context) {
	if h.consoleProvider == nil || !h.consoleProvider.IsAvailable() {
		response.FailWithMessage(c, response.ErrSystemError, "Console-API is not available")
		return
	}
	result, err := h.consoleProvider.ListPlatformChannelModels(c.Request.Context())
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// InitOfficialChannel initializes official channels for a tenant
// POST /channels/official/init
func (h *ChannelHandler) InitOfficialChannel(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	if err := h.service.InitOfficialChannel(c.Request.Context(), organizationID); err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, nil)
}

// UpdateOfficialChannelSettings updates priority/weight for all routes in an official channel group
// PUT /channels/official/:group_id/settings
func (h *ChannelHandler) UpdateOfficialChannelSettings(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	groupID := c.Param("group_id")
	if groupID == "" {
		h.Error(c, fmt.Errorf("group_id is required"))
		return
	}
	var req dto.UpdateOfficialChannelSettingsRequest
	if !h.BindJSON(c, &req) {
		return
	}
	req.GroupID = groupID
	updated, err := h.service.UpdateOfficialChannelSettings(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, gin.H{"updated": updated})
}

// TestChannelModel tests a specific model on a channel
// POST /channels/:id/test/model
func (h *ChannelHandler) TestChannelModel(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	channelID, ok := h.ParseUUID(c, "id")
	if !ok {
		return
	}
	var req dto.TestChannelModelRequest
	if !h.BindJSON(c, &req) {
		return
	}
	result, err := h.service.TestChannelModel(c.Request.Context(), channelID, organizationID, req.Model, req.TestMethod)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// TestDraftChannelModel tests a single model before the channel is created.
// POST /channels/draft/test/model
func (h *ChannelHandler) TestDraftChannelModel(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}

	req, ok := parseDraftTestChannelModelRequest(c)
	if !ok {
		return
	}

	result, err := h.service.TestDraftChannelModel(c.Request.Context(), organizationID, req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// DiscoverOllamaModels lists locally available Ollama models from /api/tags.
// POST /channels/ollama/discover-models
func (h *ChannelHandler) DiscoverOllamaModels(c *gin.Context) {
	if _, ok := h.GetOrganizationID(c); !ok {
		return
	}

	var req dto.DiscoverOllamaModelsRequest
	if !h.BindJSON(c, &req) {
		return
	}

	result, err := h.service.DiscoverOllamaModels(c.Request.Context(), &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// BatchTestChannelModels tests multiple models on a channel with streaming response
// POST /channels/:id/test/batch
func (h *ChannelHandler) BatchTestChannelModels(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	channelID, ok := h.ParseUUID(c, "id")
	if !ok {
		return
	}
	var req dto.BatchTestChannelModelsRequest
	if !h.BindJSON(c, &req) {
		return
	}

	resultChan := make(chan *dto.BatchTestChannelModelsStreamResponse, len(req.Models)+1)

	go h.service.BatchTestChannelModels(c.Request.Context(), channelID, organizationID, req.Models, req.TestMethod, resultChan)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	c.Stream(func(w io.Writer) bool {
		if result, ok := <-resultChan; ok {
			data, _ := json.Marshal(result)
			c.SSEvent("message", string(data))
			return !result.Completed
		}
		return false
	})
}

// BatchToggleRoutes toggles multiple routes at once
func (h *ChannelHandler) BatchToggleRoutes(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	var req dto.BatchToggleRoutesRequest
	if !h.BindJSON(c, &req) {
		return
	}
	result, err := h.service.BatchToggleRoutes(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// BatchDeleteRoutes deletes multiple routes at once
func (h *ChannelHandler) BatchDeleteRoutes(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	var req dto.BatchDeleteRoutesRequest
	if !h.BindJSON(c, &req) {
		return
	}
	result, err := h.service.BatchDeleteRoutes(c.Request.Context(), organizationID, &req)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, result)
}

// GetAvailableProviders returns all provider values actually used by tenant's channels.
// GET /channels/providers
func (h *ChannelHandler) GetAvailableProviders(c *gin.Context) {
	organizationID, ok := h.GetOrganizationID(c)
	if !ok {
		return
	}
	providers, err := h.service.GetAvailableProviders(c.Request.Context(), organizationID)
	if err != nil {
		h.Error(c, err)
		return
	}
	h.Success(c, providers)
}

func parseDraftTestChannelModelRequest(c *gin.Context) (*dto.DraftTestChannelModelRequest, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "failed to read request body")
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		response.FailWithMessage(c, response.ErrInvalidParam, "request body is required")
		return nil, false
	}

	var req dto.DraftTestChannelModelRequest
	if err := json.Unmarshal(body, &req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return nil, false
	}

	if strings.TrimSpace(req.ChannelProvider) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "channel_provider is required")
		return nil, false
	}
	if strings.TrimSpace(req.Model) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "model is required")
		return nil, false
	}
	spec, err := channelprovider.ValidateConnectionFields(req.ChannelProvider, req.APIBaseURL)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return nil, false
	}
	if err := channelprovider.ValidateAPIKey(spec, req.APIKey); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return nil, false
	}

	return &req, true
}
