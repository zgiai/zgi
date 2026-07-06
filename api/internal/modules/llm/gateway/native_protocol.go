package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	modelCategoryResponses         = "responses"
	modelCategoryAnthropicMessages = "anthropic_messages"
)

func (s *llmGatewayServiceImpl) CreateResponseRaw(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RawResponseRequest,
) (*adapter.RawResponse, error) {
	if err := validateRawResponseRequest(req); err != nil {
		return nil, err
	}
	effectiveReq, err := s.policyPrompt.injectRawResponseRequest(req)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryResponses)

	return s.createNativeResponse(ctx, apiKey, effectiveReq)
}

func (s *llmGatewayServiceImpl) CreateResponseStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RawResponseRequest,
) (<-chan adapter.RawStreamEvent, error) {
	if err := validateRawResponseRequest(req); err != nil {
		return nil, err
	}
	effectiveReq, err := s.policyPrompt.injectRawResponseRequest(req)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryResponses)

	return s.createNativeResponseStream(ctx, apiKey, effectiveReq)
}

func (s *llmGatewayServiceImpl) CreateAnthropicMessage(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.AnthropicMessageRequest,
) (*adapter.RawResponse, error) {
	if err := validateAnthropicMessageRequest(req); err != nil {
		return nil, err
	}
	effectiveReq, err := s.policyPrompt.injectAnthropicMessageRequest(req)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryAnthropicMessages)

	return s.createNativeAnthropicMessage(ctx, apiKey, effectiveReq)
}

func (s *llmGatewayServiceImpl) CreateAnthropicMessageStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.AnthropicMessageRequest,
) (<-chan adapter.RawStreamEvent, error) {
	if err := validateAnthropicMessageRequest(req); err != nil {
		return nil, err
	}
	effectiveReq, err := s.policyPrompt.injectAnthropicMessageRequest(req)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryAnthropicMessages)

	return s.createNativeAnthropicMessageStream(ctx, apiKey, effectiveReq)
}

func validateRawResponseRequest(req *adapter.RawResponseRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if len(req.Body) == 0 || !json.Valid(req.Body) {
		return fmt.Errorf("%w: request body must be valid JSON", adapter.ErrInvalidRequest)
	}
	return nil
}

func validateAnthropicMessageRequest(req *adapter.AnthropicMessageRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if len(req.Body) == 0 || !json.Valid(req.Body) {
		return fmt.Errorf("%w: request body must be valid JSON", adapter.ErrInvalidRequest)
	}
	return nil
}

func (s *llmGatewayServiceImpl) createNativeResponse(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RawResponseRequest,
) (*adapter.RawResponse, error) {
	return s.runNativeNonStream(ctx, apiKey, req.Model, req.Body, "llm.responses", nativeUsageBodyFormatResponses, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (*adapter.RawResponse, error) {
		rawCapable, ok := providerAdapter.(adapter.RawResponseCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support OpenAI Responses", adapter.ErrCapabilityUnsupported)
		}
		return rawCapable.CreateResponseRaw(callCtx, req)
	})
}

func (s *llmGatewayServiceImpl) createNativeAnthropicMessage(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.AnthropicMessageRequest,
) (*adapter.RawResponse, error) {
	return s.runNativeNonStream(ctx, apiKey, req.Model, req.Body, "llm.anthropic.messages", nativeUsageBodyFormatAnthropic, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (*adapter.RawResponse, error) {
		rawCapable, ok := providerAdapter.(adapter.AnthropicMessagesCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support Anthropic Messages", adapter.ErrCapabilityUnsupported)
		}
		return rawCapable.CreateAnthropicMessage(callCtx, req)
	})
}

func (s *llmGatewayServiceImpl) runNativeNonStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	model string,
	body json.RawMessage,
	traceName string,
	bodyFormat nativeUsageBodyFormat,
	call func(context.Context, adapter.LLMProviderAdapter) (*adapter.RawResponse, error),
) (*adapter.RawResponse, error) {
	startTime := time.Now()
	if err := s.checkModelAuthorization(apiKey, nil, model); err != nil {
		return nil, err
	}

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, model)
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}
	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(model, shadowOrganizationID.String())
	}

	requestID := uuid.New().String()
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		quote, err := s.quoteTokenPricing(ctx, pricingModelRefFromSelection(providerSelection), promptTokens, completionTokens)
		if err != nil {
			lastErr = fmt.Errorf("failed to calculate credits: %w", err)
			continue
		}

		channelID := getChannelID(providerSelection)
		billingCtx, err := s.beginBillingAttempt(
			ctx,
			apiKey,
			nil,
			providerSelection,
			shadowOrganizationID,
			shadowOwnerID,
			quote.TotalCredits,
			false,
			startTime,
			requestID,
			buildAttemptID(requestID, attemptIdx),
		)
		if err != nil {
			lastErr = err
			continue
		}
		billingCtx.PromptTokens = promptTokens
		billingCtx.CompletionTokens = completionTokens
		lockTokenPricingQuote(billingCtx, quote)
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, traceName)
		ctx = withPlatformProxyMetadata(ctx, billingCtx)

		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(providerSelection, organizationID))
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		response, err := call(ctx, providerAdapter)
		responseTime := time.Since(startTime).Milliseconds()
		if err != nil {
			if adapter.IsCapabilityUnsupported(err) {
				if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
					return nil, rollbackErr
				}
			} else if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
				return nil, settleErr
			}
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, fmt.Errorf("all providers failed: %w", lastErr)
		}

		if _, err := s.ensureNativeResponseUsageForSelection(providerSelection, billingCtx, response, model, bodyFormat); err != nil {
			if settleErr := s.settleNativeUsageFallbackError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
				return nil, settleErr
			}
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, fmt.Errorf("all providers failed: %w", lastErr)
		}
		if err := s.settleChatSuccess(ctx, billingCtx, providerSelection, channelID, response.Usage, response.Settlement, responseTime); err != nil {
			return nil, err
		}
		return response, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, NewNoProviderAvailableError(model, shadowOrganizationID.String())
}

func (s *llmGatewayServiceImpl) createNativeResponseStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RawResponseRequest,
) (<-chan adapter.RawStreamEvent, error) {
	return s.runNativeStream(ctx, apiKey, req.Model, req.Body, "llm.responses.stream", nativeUsageBodyFormatResponses, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (<-chan adapter.RawStreamEvent, error) {
		rawCapable, ok := providerAdapter.(adapter.RawResponseCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support OpenAI Responses", adapter.ErrCapabilityUnsupported)
		}
		return rawCapable.CreateResponseStream(callCtx, req)
	})
}

func (s *llmGatewayServiceImpl) createNativeAnthropicMessageStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.AnthropicMessageRequest,
) (<-chan adapter.RawStreamEvent, error) {
	return s.runNativeStream(ctx, apiKey, req.Model, req.Body, "llm.anthropic.messages.stream", nativeUsageBodyFormatAnthropic, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (<-chan adapter.RawStreamEvent, error) {
		rawCapable, ok := providerAdapter.(adapter.AnthropicMessagesCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support Anthropic Messages", adapter.ErrCapabilityUnsupported)
		}
		return rawCapable.CreateAnthropicMessageStream(callCtx, req)
	})
}

func (s *llmGatewayServiceImpl) runNativeStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	model string,
	body json.RawMessage,
	traceName string,
	usageFormat nativeUsageBodyFormat,
	call func(context.Context, adapter.LLMProviderAdapter) (<-chan adapter.RawStreamEvent, error),
) (<-chan adapter.RawStreamEvent, error) {
	startTime := time.Now()
	if err := s.checkModelAuthorization(apiKey, nil, model); err != nil {
		return nil, err
	}

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, model)
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}
	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(model, shadowOrganizationID.String())
	}

	requestID := uuid.New().String()
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		quote, err := s.quoteTokenPricing(ctx, pricingModelRefFromSelection(providerSelection), promptTokens, completionTokens)
		if err != nil {
			lastErr = fmt.Errorf("failed to calculate credits: %w", err)
			continue
		}

		channelID := getChannelID(providerSelection)
		billingCtx, err := s.beginBillingAttempt(
			ctx,
			apiKey,
			nil,
			providerSelection,
			shadowOrganizationID,
			shadowOwnerID,
			quote.TotalCredits,
			true,
			startTime,
			requestID,
			buildAttemptID(requestID, attemptIdx),
		)
		if err != nil {
			lastErr = err
			continue
		}
		lockTokenPricingQuote(billingCtx, quote)
		billingCtx.PromptTokens = promptTokens
		billingCtx.CompletionTokens = completionTokens
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, traceName)
		ctx = withPlatformProxyMetadata(ctx, billingCtx)

		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(providerSelection, organizationID))
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		streamChan, err := call(ctx, providerAdapter)
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, fmt.Errorf("all providers failed: %w", lastErr)
		}

		outputChan := make(chan adapter.RawStreamEvent)
		go s.handleNativeStreamBilling(context.WithoutCancel(ctx), streamChan, outputChan, billingCtx, providerSelection, channelID, startTime, model, usageFormat)
		return outputChan, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, NewNoProviderAvailableError(model, shadowOrganizationID.String())
}

func (s *llmGatewayServiceImpl) settleNativeUsageFallbackError(
	ctx context.Context,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	responseTime int64,
	attemptIdx int,
	err error,
) error {
	if err == nil {
		return nil
	}
	return s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err)
}

func (s *llmGatewayServiceImpl) handleNativeStreamBilling(
	ctx context.Context,
	inputChan <-chan adapter.RawStreamEvent,
	outputChan chan<- adapter.RawStreamEvent,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	startTime time.Time,
	model string,
	usageFormat nativeUsageBodyFormat,
) {
	defer close(outputChan)

	var lastUsage *adapter.Usage
	var lastSettlement *adapter.SettlementResult
	var lastError error
	var pendingTerminal *adapter.RawStreamEvent
	var sawUsage bool
	var collectedText strings.Builder
	for event := range inputChan {
		terminal := isNativeTerminalStreamEvent(usageFormat, event)
		if event.Usage != nil && (hasBillableTokenUsage(event.Usage) || event.Usage.TotalTokens > 0) {
			lastUsage = event.Usage
			sawUsage = true
		}
		if len(event.Data) > 0 {
			text := nativeResponseText(event.Data)
			if !terminal || strings.TrimSpace(collectedText.String()) == "" {
				collectedText.WriteString(text)
			}
		}
		if event.Settlement != nil {
			lastSettlement = event.Settlement
		}
		if event.Error != nil {
			lastError = event.Error
			break
		}
		if event.Done {
			break
		}
		if terminal {
			pending := event
			pendingTerminal = &pending
			continue
		}
		outputChan <- event
	}

	responseTime := time.Since(startTime).Milliseconds()
	if lastError != nil {
		if err := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, 0, lastError); err != nil {
			outputChan <- adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage}
			return
		}
		outputChan <- adapter.RawStreamEvent{Error: lastError, Done: true, Usage: lastUsage}
		return
	}

	estimatedUsage := false
	if lastSettlement == nil && providerSelection != nil && !providerSelection.UseSystemProvider {
		if usage, estimated := s.completeNativeUsageFromText(model, lastUsage, collectedText.String(), nativePromptTokens(billingCtx)); hasBillableTokenUsage(usage) && (!hasBillableTokenUsage(lastUsage) || estimated) {
			lastUsage = usage
			estimatedUsage = estimated || !sawUsage
			if estimated {
				markEstimatedUsageSource(billingCtx, usage)
			}
		}
	}

	if err := s.settleChatSuccess(ctx, billingCtx, providerSelection, channelID, lastUsage, lastSettlement, responseTime); err != nil {
		outputChan <- adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage}
		return
	}
	if pendingTerminal != nil {
		if estimatedUsage {
			if updated, ok := injectUsageIntoNativeTerminalEvent(*pendingTerminal, model, lastUsage, usageFormat); ok {
				outputChan <- updated
			} else {
				outputChan <- nativeUsageStreamEvent(model, lastUsage, usageFormat)
				outputChan <- *pendingTerminal
			}
		} else {
			outputChan <- *pendingTerminal
		}
	} else if estimatedUsage {
		outputChan <- nativeUsageStreamEvent(model, lastUsage, usageFormat)
	}
	outputChan <- adapter.RawStreamEvent{Done: true, Usage: lastUsage}
}

func (s *llmGatewayServiceImpl) estimateNativeProtocolTokens(body json.RawMessage, model string) (int, int) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return 1, s.tokenEstimator.EstimateCompletionTokens(nil, model)
	}

	promptTokens := s.tokenEstimator.EstimateTextTokensForModel(model, nativePromptTokenSource(body, payload))
	completionTokens := s.tokenEstimator.EstimateCompletionTokens(rawMaxTokens(payload), model)
	return promptTokens, completionTokens
}

func nativePromptTokenSource(body json.RawMessage, payload map[string]json.RawMessage) string {
	var source strings.Builder
	for _, key := range []string{"input", "messages", "instructions", "system", "tools", "tool_choice", "response_format"} {
		raw, ok := payload[key]
		if !ok || len(raw) == 0 {
			continue
		}
		if source.Len() > 0 {
			source.WriteByte('\n')
		}
		source.Write(raw)
	}
	if source.Len() == 0 {
		return string(body)
	}
	return source.String()
}

func rawMaxTokens(payload map[string]json.RawMessage) *int {
	for _, key := range []string{"max_output_tokens", "max_tokens"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value int
		if err := json.Unmarshal(raw, &value); err == nil && value > 0 {
			return &value
		}
	}
	return nil
}

func logNativeProtocolUnsupported(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.DebugContext(ctx, "native protocol capability rejected", err)
}
