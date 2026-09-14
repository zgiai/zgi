package gateway

import (
	"encoding/json"
	"fmt"
	"strings"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func isPrincipalBoundAPIKey(apiKey *apikeymodel.TenantAPIKey) bool {
	return apiKey != nil &&
		apiKey.AccessGrantID != nil && strings.TrimSpace(*apiKey.AccessGrantID) != "" &&
		apiKey.PrincipalType != nil && strings.TrimSpace(*apiKey.PrincipalType) != "" &&
		apiKey.PrincipalID != nil && strings.TrimSpace(*apiKey.PrincipalID) != "" &&
		apiKey.WorkspaceID != nil && strings.TrimSpace(*apiKey.WorkspaceID) != ""
}

func (s *llmGatewayServiceImpl) withPrincipalChatOutputLimit(
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.ChatRequest,
) *adapter.ChatRequest {
	if s == nil || s.tokenEstimator == nil || !isPrincipalBoundAPIKey(apiKey) || req == nil || req.MaxTokens != nil {
		return req
	}
	limit := s.tokenEstimator.EstimateCompletionTokens(nil, req.Model)
	if limit <= 0 {
		return req
	}
	cloned := *req
	cloned.MaxTokens = &limit
	return &cloned
}

func (s *llmGatewayServiceImpl) withPrincipalRawResponseOutputLimit(
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RawResponseRequest,
) (*adapter.RawResponseRequest, error) {
	if req == nil {
		return req, nil
	}
	body, err := s.withPrincipalNativeOutputLimit(apiKey, req.Body, req.Model, protocolOpenAIResponses)
	if err != nil || string(body) == string(req.Body) {
		return req, err
	}
	cloned := *req
	cloned.Body = body
	return &cloned, nil
}

func (s *llmGatewayServiceImpl) withPrincipalAnthropicOutputLimit(
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.AnthropicMessageRequest,
) (*adapter.AnthropicMessageRequest, error) {
	if req == nil {
		return req, nil
	}
	body, err := s.withPrincipalNativeOutputLimit(apiKey, req.Body, req.Model, protocolAnthropicMessages)
	if err != nil || string(body) == string(req.Body) {
		return req, err
	}
	cloned := *req
	cloned.Body = body
	return &cloned, nil
}

func (s *llmGatewayServiceImpl) withPrincipalNativeOutputLimit(
	apiKey *apikeymodel.TenantAPIKey,
	body json.RawMessage,
	model string,
	protocol string,
) (json.RawMessage, error) {
	if s == nil || s.tokenEstimator == nil || !isPrincipalBoundAPIKey(apiKey) {
		return body, nil
	}
	limit := s.tokenEstimator.EstimateCompletionTokens(nil, model)
	if limit <= 0 {
		return body, nil
	}
	payload := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: request body must be valid JSON", adapter.ErrInvalidRequest)
	}

	switch protocol {
	case protocolOpenAIResponses:
		if err := ensurePositiveNativeLimit(payload, []string{"max_output_tokens", "max_tokens"}); err != nil {
			return nil, err
		} else if nativeLimitPresent(payload, []string{"max_output_tokens", "max_tokens"}) {
			return body, nil
		}
		payload["max_output_tokens"] = mustMarshalJSON(limit)
	case protocolAnthropicMessages:
		if err := ensurePositiveNativeLimit(payload, []string{"max_tokens"}); err != nil {
			return nil, err
		} else if nativeLimitPresent(payload, []string{"max_tokens"}) {
			return body, nil
		}
		payload["max_tokens"] = mustMarshalJSON(limit)
	default:
		return body, nil
	}
	return json.Marshal(payload)
}

func nativeLimitPresent(payload map[string]json.RawMessage, keys []string) bool {
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func ensurePositiveNativeLimit(payload map[string]json.RawMessage, keys []string) error {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value <= 0 {
			return fmt.Errorf("%w: %s must be a positive integer", adapter.ErrInvalidRequest, key)
		}
	}
	return nil
}
