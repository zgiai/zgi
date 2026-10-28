package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// capabilityGatewayService is additive on purpose: adding ModelCapabilities to
// the shared GatewayService interface would break every existing test double, so
// the gate type-asserts for it and stays inert when the implementation does not
// provide it.
type capabilityGatewayService interface {
	ModelCapabilities(ctx context.Context, model string) (types.ModelCapabilities, error)
}

// chatCapability is one thing a chat request can ask the model to do. The value
// is the request field the client used, so the rejection can name it.
type chatCapability string

const (
	chatCapabilityStructuredOutput chatCapability = "response_format.type=json_schema"
	chatCapabilityJSONMode         chatCapability = "response_format.type=json_object"
	chatCapabilityToolCall         chatCapability = "tools"
	chatCapabilityVision           chatCapability = "messages[].content[].image_url"
)

// requiredChatCapabilities lists the capabilities a request depends on. Fields
// the model metadata does not track are ignored rather than guessed.
func requiredChatCapabilities(req *adapter.ChatRequest) []chatCapability {
	if req == nil {
		return nil
	}
	needs := make([]chatCapability, 0, 4)
	if format := req.ResponseFormat; format != nil {
		switch strings.ToLower(strings.TrimSpace(format.Type)) {
		case "json_schema":
			needs = append(needs, chatCapabilityStructuredOutput)
		case "json_object":
			needs = append(needs, chatCapabilityJSONMode)
		}
	}
	if len(req.Tools) > 0 || len(req.Functions) > 0 {
		needs = append(needs, chatCapabilityToolCall)
	}
	if chatMessagesContainImage(req.Messages) {
		needs = append(needs, chatCapabilityVision)
	}
	return needs
}

// chatMessagesContainImage reports whether any message carries an image part.
// The content field is an interface{} because it is either a plain string or a
// multimodal part list, and the decoded form can be a typed part slice or a
// generic []interface{} depending on the caller, so both shapes are inspected.
func chatMessagesContainImage(messages []adapter.Message) bool {
	for _, message := range messages {
		switch content := message.Content.(type) {
		case []adapter.MessageContentPart:
			for _, part := range content {
				if part.ImageURL != nil || isImageContentPartType(part.Type) {
					return true
				}
			}
		case []interface{}:
			for _, raw := range content {
				part, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if _, hasImage := part["image_url"]; hasImage {
					return true
				}
				partType, _ := part["type"].(string)
				if isImageContentPartType(partType) {
					return true
				}
			}
		}
	}
	return false
}

func isImageContentPartType(partType string) bool {
	switch strings.ToLower(strings.TrimSpace(partType)) {
	case "image_url", "image", "input_image":
		return true
	default:
		return false
	}
}

// capabilitySupported maps a request capability onto the model declaration.
func capabilitySupported(caps types.ModelCapabilities, need chatCapability) bool {
	switch need {
	case chatCapabilityStructuredOutput:
		return caps.StructuredOutput
	case chatCapabilityJSONMode:
		return caps.JSONMode
	case chatCapabilityToolCall:
		return caps.ToolCall
	case chatCapabilityVision:
		return caps.Vision
	default:
		return true
	}
}

// missingChatCapability returns the first requested capability the model did not
// declare.
func missingChatCapability(caps types.ModelCapabilities, needs []chatCapability) (chatCapability, bool) {
	for _, need := range needs {
		if !capabilitySupported(caps, need) {
			return need, true
		}
	}
	return "", false
}

// unsupportedCapabilityProtocolError is a non-retryable 400: the caller asked
// for a capability the selected model never declared, so retrying cannot help.
func unsupportedCapabilityProtocolError(need chatCapability, model string) protocolError {
	message := fmt.Sprintf("The model %q does not support %s", strings.TrimSpace(model), string(need))
	return newAppProtocolError(
		llmerrors.AppCodeRequestInvalid,
		http.StatusBadRequest,
		"invalid_request_error",
		"unsupported_capability",
		"invalid_request_error",
		message,
		false,
	)
}

// enforceChatCapabilities rejects, before billing and before any upstream call,
// a request that asks for a capability the model does not declare. It fails open
// on every uncertainty (gate disabled, capability provider unavailable, lookup
// error, metadata unknown) so a data gap can never block a working model.
func (h *LLMHandler) enforceChatCapabilities(c *gin.Context, req *adapter.ChatRequest) bool {
	if h == nil || c == nil || req == nil {
		return true
	}
	needs := requiredChatCapabilities(req)
	if len(needs) == 0 || !gateway.CapabilityGateEnabled() {
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
	if !capabilities.Known {
		return true
	}
	missing, found := missingChatCapability(capabilities, needs)
	if !found {
		return true
	}
	writeOpenAIProtocolError(c, unsupportedCapabilityProtocolError(missing, req.Model))
	return false
}
