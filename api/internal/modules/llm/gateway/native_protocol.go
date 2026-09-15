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
	protocolOpenAIChatCompletions  = "openai_chat_completions"
	protocolOpenAIResponses        = "openai_responses"
	protocolAnthropicMessages      = "anthropic_messages"
	modelCategoryImage             = "image"
	modelCategoryResponses         = "responses"
	modelCategoryAnthropicMessages = "anthropic_messages"
	protocolGeminiGenerateContent  = "gemini_generate_content"
	modelCategoryGemini            = "gemini_generate_content"
	// Native protocol providers may report input tokens for protocol framing,
	// system instructions, or other server-side prompt material that is not
	// present in the public JSON body. Reservations are refunded to actual usage,
	// so a small floor prevents a successful response from failing settlement
	// without increasing the final charge.
	nativeProtocolInputReservationOverheadTokens = 128
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
	effectiveReq, err = s.withPrincipalRawResponseOutputLimit(apiKey, effectiveReq)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryResponses)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationResponses)

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
	effectiveReq, err = s.withPrincipalRawResponseOutputLimit(apiKey, effectiveReq)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryResponses)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationResponses)

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
	effectiveReq, err = s.withPrincipalAnthropicOutputLimit(apiKey, effectiveReq)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryAnthropicMessages)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationAnthropicMessages)

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
	effectiveReq, err = s.withPrincipalAnthropicOutputLimit(apiKey, effectiveReq)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryAnthropicMessages)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationAnthropicMessages)

	return s.createNativeAnthropicMessageStream(ctx, apiKey, effectiveReq)
}

func (s *llmGatewayServiceImpl) CreateGeminiContent(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.GeminiGenerateContentRequest,
) (*adapter.RawResponse, error) {
	if err := validateGeminiGenerateContentRequest(req); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryGemini)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationGeminiGenerate)
	return s.runNativeNonStream(ctx, apiKey, req.Model, req.Body, "llm.gemini.generate_content", nativeUsageBodyFormatGemini, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (*adapter.RawResponse, error) {
		capable, ok := providerAdapter.(adapter.GeminiGenerateContentCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support Gemini generateContent", adapter.ErrCapabilityUnsupported)
		}
		return capable.CreateGeminiContent(callCtx, req)
	})
}

func (s *llmGatewayServiceImpl) CreateGeminiContentStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.GeminiGenerateContentRequest,
) (<-chan adapter.RawStreamEvent, error) {
	if err := validateGeminiGenerateContentRequest(req); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, modelCategoryGemini)
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationGeminiGenerate)
	return s.runNativeStream(ctx, apiKey, req.Model, req.Body, "llm.gemini.stream_generate_content", nativeUsageBodyFormatGemini, func(callCtx context.Context, providerAdapter adapter.LLMProviderAdapter) (<-chan adapter.RawStreamEvent, error) {
		capable, ok := providerAdapter.(adapter.GeminiGenerateContentCapable)
		if !ok {
			return nil, fmt.Errorf("%w: selected provider does not support Gemini streamGenerateContent", adapter.ErrCapabilityUnsupported)
		}
		return capable.CreateGeminiContentStream(callCtx, req)
	})
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

func validateGeminiGenerateContentRequest(req *adapter.GeminiGenerateContentRequest) error {
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
) (response *adapter.RawResponse, retErr error) {
	startTime := time.Now()
	traceCtx := ctx
	var traceBillingCtx *BillingContext
	defer func() {
		s.traceNativeLLMOperation(
			traceCtx,
			traceName,
			nativeTraceOperation(traceName),
			body,
			nativeRawResponseOutput(response),
			nativeRawResponseUsage(response),
			startTime,
			time.Time{},
			traceBillingCtx,
			retErr,
		)
	}()
	if err := s.checkModelAuthorization(apiKey, nil, model); err != nil {
		return nil, err
	}

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, model, nil)
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}
	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	ctx = withRoutingTokenEstimate(ctx, promptTokens, completionTokens, PricingOperationChat)
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", model, 3)
	if err != nil {
		reportLLMSelectionFailure(ctx, err, model, organizationID.String(), shadowOrganizationID.String())
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, reportedNoProviderAvailableError(ctx, model, organizationID.String(), shadowOrganizationID.String())
	}

	requestID := gatewayRequestIDFromContext(ctx)
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		reservedPrompt, reservedCompletion := conservativeReasoningReservationTokens(providerSelection, promptTokens, completionTokens)
		quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, pricingModelRefFromSelection(providerSelection), reservedPrompt, reservedCompletion)
		if err != nil {
			lastErr = wrapPricingCalculationError(err)
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
			quote,
		)
		if err != nil {
			lastErr = err
			continue
		}
		billingCtx.PromptTokens = reservedPrompt
		billingCtx.CompletionTokens = reservedCompletion
		billingCtx.EstimatedInputTokens = reservedPrompt
		billingCtx.EstimatedOutputTokens = reservedCompletion
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, traceName)
		ctx = withPlatformProxyMetadata(ctx, billingCtx)
		traceCtx = ctx
		traceBillingCtx = billingCtx

		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(providerSelection, organizationID))
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			reportLLMAdapterFailureForSelection(ctx, err, providerSelection, billingCtx, attemptIdx, attemptIdx == len(providerSelections)-1)
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
			lastErr = err
			continue
		}
		response, err = call(ctx, providerAdapter)
		responseTime := time.Since(startTime).Milliseconds()
		if err != nil {
			if adapter.IsCapabilityUnsupported(err) {
				if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
					return nil, rollbackErr
				}
			} else {
				reportLLMProviderFailureForSelection(ctx, err, "llm.provider.request_failed", providerSelection, billingCtx, attemptIdx, attemptIdx == len(providerSelections)-1)
				if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
					return nil, settleErr
				}
			}
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, NewReportedProviderFailureError(lastErr)
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
		response.Body = sanitizePublicNativeBody(response.Body, model)
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
) (output <-chan adapter.RawStreamEvent, retErr error) {
	startTime := time.Now()
	traceCtx := ctx
	var traceBillingCtx *BillingContext
	traceHandedOff := false
	defer func() {
		if traceHandedOff {
			return
		}
		s.traceNativeLLMOperation(
			traceCtx,
			traceName,
			nativeTraceOperation(traceName),
			body,
			nil,
			nil,
			startTime,
			time.Time{},
			traceBillingCtx,
			retErr,
		)
	}()
	if err := s.checkModelAuthorization(apiKey, nil, model); err != nil {
		return nil, err
	}

	promptTokens, completionTokens := s.estimateNativeProtocolTokens(body, model, nil)
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}
	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	ctx = withRoutingTokenEstimate(ctx, promptTokens, completionTokens, PricingOperationChat)
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", model, 3)
	if err != nil {
		reportLLMSelectionFailure(ctx, err, model, organizationID.String(), shadowOrganizationID.String())
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, reportedNoProviderAvailableError(ctx, model, organizationID.String(), shadowOrganizationID.String())
	}

	requestID := gatewayRequestIDFromContext(ctx)
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		reservedPrompt, reservedCompletion := conservativeReasoningReservationTokens(providerSelection, promptTokens, completionTokens)
		quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, pricingModelRefFromSelection(providerSelection), reservedPrompt, reservedCompletion)
		if err != nil {
			lastErr = wrapPricingCalculationError(err)
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
			quote,
		)
		if err != nil {
			lastErr = err
			continue
		}
		billingCtx.PromptTokens = reservedPrompt
		billingCtx.CompletionTokens = reservedCompletion
		billingCtx.EstimatedInputTokens = reservedPrompt
		billingCtx.EstimatedOutputTokens = reservedCompletion
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, traceName)
		ctx = withPlatformProxyMetadata(ctx, billingCtx)
		traceCtx = ctx
		traceBillingCtx = billingCtx

		providerAdapter, err := s.adapterFactory.CreateAdapter(s.createAdapterConfig(providerSelection, organizationID))
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			reportLLMAdapterFailureForSelection(ctx, err, providerSelection, billingCtx, attemptIdx, attemptIdx == len(providerSelections)-1)
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
			lastErr = err
			continue
		}
		streamChan, err := call(ctx, providerAdapter)
		if err != nil {
			setBillingFailure(billingCtx, err)
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			reportLLMProviderFailureForSelection(ctx, err, "llm.provider.stream_failed", providerSelection, billingCtx, attemptIdx, attemptIdx == len(providerSelections)-1)
			s.recordUpstreamProviderError(ctx, providerSelection, billingCtx, err)
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, fmt.Errorf("all providers failed: %w", lastErr)
		}

		outputChan := make(chan adapter.RawStreamEvent)
		// Billing runs on a detached context so a client disconnect cannot abort
		// settlement, but the goroutine still needs the request context's Done
		// channel: without it every send on the unbuffered outputChan blocks
		// forever once the handler stops reading, leaking the goroutine and the
		// upstream connection behind it.
		go s.handleNativeStreamBilling(context.WithoutCancel(ctx), ctx.Done(), streamChan, outputChan, billingCtx, providerSelection, channelID, attemptIdx, startTime, model, body, traceName, usageFormat)
		traceHandedOff = true
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
	clientGone <-chan struct{},
	inputChan <-chan adapter.RawStreamEvent,
	outputChan chan<- adapter.RawStreamEvent,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	channelID *uuid.UUID,
	attemptIdx int,
	startTime time.Time,
	model string,
	requestBody json.RawMessage,
	traceName string,
	usageFormat nativeUsageBodyFormat,
) {
	defer close(outputChan)

	// emit never blocks indefinitely. Once the client is gone the remaining
	// events are dropped, but the loop below keeps draining inputChan so the
	// upstream reader can finish and settlement still sees the real usage.
	clientConnected := true
	emit := func(event adapter.RawStreamEvent) {
		if !clientConnected {
			return
		}
		select {
		case outputChan <- event:
		case <-clientGone:
			clientConnected = false
		}
	}

	var lastUsage *adapter.Usage
	var lastSettlement *adapter.SettlementResult
	doneSeen := false
	var lastError error
	var pendingTerminal *adapter.RawStreamEvent
	var pendingFailure *adapter.RawStreamEvent
	var sawUsage bool
	var collectedText strings.Builder
	for event := range inputChan {
		terminal := isNativeTerminalStreamEvent(usageFormat, event)
		failed := isNativeFailureStreamEvent(usageFormat, event)
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
			// Adapter events arrive before Gateway billing/finalization errors are
			// introduced, making this the safe boundary for timeout classification.
			event.Error = adapter.NormalizeTransportError(event.Error)
			lastError = event.Error
			break
		}
		if event.Done {
			// Providers/proxies may append settlement metadata after the model
			// completion frame. Keep draining until the producer closes the
			// stream; settlement events remain internal and are never emitted.
			doneSeen = true
			continue
		}
		if doneSeen {
			continue
		}
		event.Data = sanitizePublicNativeBody(event.Data, model)
		if failed {
			pending := event
			pendingFailure = &pending
			lastError = fmt.Errorf("%w: provider returned terminal %s event", adapter.ErrUpstreamError, strings.TrimSpace(event.Event))
			break
		}
		if terminal {
			pending := event
			pendingTerminal = &pending
			continue
		}
		emit(event)
	}

	responseTime := time.Since(startTime).Milliseconds()
	traceOutput := collectedText.String()
	if lastError != nil {
		reportLLMProviderFailureForSelection(ctx, lastError, "llm.provider.stream_failed", providerSelection, billingCtx, attemptIdx, true)
		if err := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, lastError); err != nil {
			s.traceNativeLLMStreamOperation(ctx, traceName, nativeTraceOperation(traceName), requestBody, traceOutput, lastUsage, startTime, time.Time{}, billingCtx, err)
			if pendingFailure != nil {
				emit(*pendingFailure)
				emit(adapter.RawStreamEvent{Done: true, Usage: lastUsage})
				return
			}
			emit(adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage})
			return
		}
		s.traceNativeLLMStreamOperation(ctx, traceName, nativeTraceOperation(traceName), requestBody, traceOutput, lastUsage, startTime, time.Time{}, billingCtx, lastError)
		if pendingFailure != nil {
			emit(*pendingFailure)
			emit(adapter.RawStreamEvent{Done: true, Usage: lastUsage})
			return
		}
		emit(adapter.RawStreamEvent{Error: lastError, Done: true, Usage: lastUsage})
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
		s.traceNativeLLMStreamOperation(ctx, traceName, nativeTraceOperation(traceName), requestBody, traceOutput, lastUsage, startTime, time.Time{}, billingCtx, err)
		emit(adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage})
		return
	}
	s.traceNativeLLMStreamOperation(ctx, traceName, nativeTraceOperation(traceName), requestBody, traceOutput, lastUsage, startTime, time.Time{}, billingCtx, nil)
	if pendingTerminal != nil {
		if estimatedUsage {
			if updated, ok := injectUsageIntoNativeTerminalEvent(*pendingTerminal, model, lastUsage, usageFormat); ok {
				emit(updated)
			} else {
				emit(nativeUsageStreamEvent(model, lastUsage, usageFormat))
				emit(*pendingTerminal)
			}
		} else {
			emit(*pendingTerminal)
		}
	} else if estimatedUsage {
		emit(nativeUsageStreamEvent(model, lastUsage, usageFormat))
	}
	emit(adapter.RawStreamEvent{Done: true, Usage: lastUsage})
}

func (s *llmGatewayServiceImpl) estimateNativeProtocolTokens(body json.RawMessage, model string, selection *ProviderSelection) (int, int) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		promptTokens, completionTokens := 1, s.tokenEstimator.EstimateCompletionTokens(nil, model)
		promptTokens, completionTokens = conservativeReasoningReservationTokens(selection, promptTokens, completionTokens)
		return promptTokens, completionTokens
	}

	promptTokens := s.tokenEstimator.EstimateTextTokensForModel(model, nativePromptTokenSource(body, payload))
	promptTokens = saturatingAddPositive(promptTokens, nativeProtocolInputReservationOverheadTokens)
	completionTokens := s.tokenEstimator.EstimateCompletionTokens(rawMaxTokens(payload), model)
	return conservativeReasoningReservationTokens(selection, promptTokens, completionTokens)
}

func saturatingAddPositive(value, overhead int) int {
	if value < 0 {
		value = 0
	}
	if overhead <= 0 {
		return value
	}
	if value > int(^uint(0)>>1)-overhead {
		return int(^uint(0) >> 1)
	}
	return value + overhead
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

func gatewayRequestIDFromContext(ctx context.Context) string {
	if ctx != nil {
		if requestID, ok := ctx.Value(shared.ContextKeyRequestID).(string); ok && strings.TrimSpace(requestID) != "" {
			return strings.TrimSpace(requestID)
		}
	}
	return uuid.NewString()
}
