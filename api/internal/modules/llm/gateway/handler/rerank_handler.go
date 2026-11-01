package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// rerankCapability names the declaration a rerank request depends on, in the
// form the unsupported-capability rejection reports it.
const rerankCapability chatCapability = "rerank"

// rerankGatewayService is additive for the same reason as geminiGatewayService:
// the shared GatewayService interface and its test doubles stay unchanged.
type rerankGatewayService interface {
	Rerank(context.Context, *apikeymodel.TenantAPIKey, *adapter.RerankRequest) (*adapter.RerankResponse, error)
}

// Rerank handles POST /v1/rerank (Cohere-compatible, same body as Console).
func (h *LLMHandler) Rerank(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	var req adapter.RerankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIProtocolError(c, h.localizedInvalidRequest(c, invalidRequestProtocolError("Invalid JSON request body")))
		return
	}

	// The service validates too, but its errors are not public input errors and
	// would surface as 5xx; reject a malformed body here as a 400 instead.
	if err := validatePublicRerankRequest(&req); err != nil {
		writeOpenAIProtocolError(c, h.localizedInvalidRequest(c, invalidRequestProtocolError(publicInputMessage(err))))
		return
	}

	if !h.enforceRerankCapability(c, &req) {
		return
	}

	service, ok := h.gatewayService.(rerankGatewayService)
	if !ok {
		writeOpenAIProtocolError(c, classifyProtocolError(adapter.ErrCapabilityUnsupported))
		return
	}

	// Publish the server-generated billing/audit ID, as the chat lane does.
	c.Request = c.Request.WithContext(types.WithInvocationIDObserver(c.Request.Context(), func(id string) {
		c.Header(types.InvocationIDHeader, id)
	}))
	resp, err := service.Rerank(gatewayRequestContext(c), apiKey, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func validatePublicRerankRequest(req *adapter.RerankRequest) error {
	if req == nil || strings.TrimSpace(req.Model) == "" {
		return newPublicInputErrorf("model is required")
	}
	if strings.TrimSpace(req.Query) == "" {
		return newPublicInputErrorf("query is required")
	}
	documents, ok := req.Documents.([]interface{})
	if !ok || len(documents) == 0 {
		return newPublicInputErrorf("documents must contain at least one item")
	}
	if req.TopN != nil && *req.TopN <= 0 {
		return newPublicInputErrorf("top_n must be a positive integer")
	}
	return nil
}

// enforceRerankCapability rejects, before billing and before any upstream call,
// a rerank request for a model whose use_cases do not include rerank. It fails
// open on every uncertainty exactly like enforceChatCapabilities.
func (h *LLMHandler) enforceRerankCapability(c *gin.Context, req *adapter.RerankRequest) bool {
	if h == nil || c == nil || req == nil || !gateway.CapabilityGateEnabled() {
		return true
	}
	capabilityService, ok := h.gatewayService.(capabilityGatewayService)
	if !ok {
		return true
	}
	capabilities, err := capabilityService.ModelCapabilities(c.Request.Context(), req.Model)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "model capability lookup failed; allowing request", err)
		return true
	}
	if !capabilities.Known || !capabilities.UseCasesDeclared || capabilities.Rerank {
		return true
	}
	writeOpenAIProtocolError(c, unsupportedCapabilityProtocolError(rerankCapability, req.Model))
	return false
}
