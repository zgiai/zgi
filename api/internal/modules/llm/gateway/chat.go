package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// ChatCompletion handles chat completion requests
func (s *llmGatewayServiceImpl) ChatCompletion(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.ChatRequest,
) (*adapter.ChatResponse, error) {
	return s.chatCompletionInternal(ctx, apiKey, nil, req)
}

// ChatCompletionWithAppContext handles chat completion requests with app context
func (s *llmGatewayServiceImpl) ChatCompletionWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
) (*adapter.ChatResponse, error) {
	return s.chatCompletionInternal(ctx, apiKey, appCtx, req)
}

// chatCompletionInternal is the internal implementation that supports optional AppContext
func (s *llmGatewayServiceImpl) chatCompletionInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
) (*adapter.ChatResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()
	ctx = logger.WithFields(ctx,
		zap.String("gateway_request_id", requestID),
		zap.String("model", req.Model),
		zap.String("organization_id", apiKey.OrganizationID),
	)
	logger.DebugContext(ctx, "llm gateway request started")

	// 1. Validate request
	t1 := time.Now()
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}
	effectiveReq := s.policyPrompt.injectChatRequest(req)
	logger.DebugContext(ctx, "llm gateway timing", "step", "validate_request", "latency_ms", time.Since(t1).Milliseconds())

	// 2. Check model authorization
	t2 := time.Now()
	if err := s.checkModelAuthorization(apiKey, appCtx, effectiveReq.Model); err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "llm gateway timing", "step", "check_model_authorization", "latency_ms", time.Since(t2).Milliseconds())

	// 3. Estimate tokens
	t3 := time.Now()
	promptTokens, completionTokens, _ := s.tokenEstimator.EstimateTotalTokens(
		effectiveReq.Messages,
		effectiveReq.MaxTokens,
		effectiveReq.Model,
	)
	logger.DebugContext(ctx, "llm gateway timing", "step", "estimate_tokens", "latency_ms", time.Since(t3).Milliseconds())

	// 4. Select providers
	t4 := time.Now()
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}

	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID) // fixme
	if err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "llm gateway timing", "step", "resolve_shadow_tenant", "latency_ms", time.Since(t4).Milliseconds())

	t4b := time.Now()

	// Tag the request category so the router can apply capability-aware matching.
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, "chat")
	if useCase := modelUseCaseForAppContext(appCtx); useCase != "" {
		ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, useCase)
	}

	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, effectiveReq.Provider, effectiveReq.Model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}

	logger.DebugContext(ctx, "llm gateway timing", "step", "select_providers", "latency_ms", time.Since(t4b).Milliseconds(), "provider_count", len(providerSelections))

	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(effectiveReq.Model, organizationID.String())
	}

	// 5. Try each provider selection with failover
	t5 := time.Now()
	var lastErr error

	for attemptIdx, providerSelection := range providerSelections {
		response, err := s.tryChatCompletion(ctx, apiKey, appCtx, effectiveReq, req, providerSelection, promptTokens, completionTokens,
			organizationID, shadowOrganizationID, shadowOwnerID, requestID, startTime, attemptIdx)
		if err == nil {
			logger.DebugContext(ctx, "llm gateway timing", "step", "try_completion", "latency_ms", time.Since(t5).Milliseconds())
			logger.InfoContext(ctx, "llm gateway request completed", "latency_ms", time.Since(startTime).Milliseconds(), "attempt_count", attemptIdx+1)

			return response, nil
		}

		lastErr = err
		if errors.Is(err, ErrBillingSettleFailed) {
			return nil, err
		}

		if attemptIdx < len(providerSelections)-1 {
			logger.WarnContext(ctx, "llm provider attempt failed, trying next provider",
				"provider", providerSelection.Provider.Provider,
				"route_id", providerSelection.RouteID.String(),
				"attempt", attemptIdx+1,
				err,
			)

			continue
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}

	return nil, ErrNoProviderAvailable
}

// tryChatCompletion attempts a chat completion with a single provider
func (s *llmGatewayServiceImpl) tryChatCompletion(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
	traceReq *adapter.ChatRequest,
	providerSelection *ProviderSelection,
	promptTokens, completionTokens int,
	organizationID uuid.UUID,
	shadowOrganizationID uuid.UUID,
	shadowOwnerID uuid.UUID,
	requestID string,
	startTime time.Time,
	attemptIdx int,
) (*adapter.ChatResponse, error) {
	// Calculate estimated credits
	tCalc := time.Now()
	attemptID := buildAttemptID(requestID, attemptIdx)

	quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, pricingModelRefFromSelection(providerSelection), promptTokens, completionTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate credits: %w", err)
	}
	estimatedCredits := quote.TotalCredits

	logger.DebugContext(ctx, "llm gateway timing", "step", "calculate_credits", "latency_ms", time.Since(tCalc).Milliseconds(), "attempt", attemptIdx+1)

	// Get channel ID
	channelID := getChannelID(providerSelection)

	// Start billing: create context + check balance + pre-deduct
	tBillingCtx := time.Now()

	billingCtx, err := s.beginBillingAttempt(
		ctx,
		apiKey,
		appCtx,
		providerSelection,
		shadowOrganizationID,
		shadowOwnerID,
		estimatedCredits,
		false,
		startTime,
		requestID,
		attemptID,
	)
	if err != nil {
		return nil, err
	}
	lockTokenPricingQuote(billingCtx, quote)
	ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.chat")
	ctx = withPlatformProxyMetadata(ctx, billingCtx)

	logger.DebugContext(ctx, "llm gateway timing", "step", "begin_billing_attempt", "latency_ms", time.Since(tBillingCtx).Milliseconds(), "attempt", attemptIdx+1)

	// Unified adapter path: both official and private channels go through the same adapter flow.
	// Official channels have APIBaseURL pointing to console-api (which returns OpenAI format).
	var response *adapter.ChatResponse
	var callErr error

	logger.DebugContext(ctx, "llm provider attempt started",
		"provider", providerSelection.Provider.Provider,
		"route_id", providerSelection.RouteID.String(),
		"use_system_provider", providerSelection.UseSystemProvider,
		"attempt", attemptIdx+1,
	)

	adapterConfig := s.createAdapterConfig(providerSelection, organizationID)
	providerAdapter, adapterErr := s.adapterFactory.CreateAdapter(adapterConfig)
	if adapterErr != nil {
		if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
			return nil, rollbackErr
		}
		s.logProviderError(ctx, attemptIdx, providerSelection, adapterErr, "adapter_creation_failed")
		return nil, fmt.Errorf("failed to create adapter: %w", adapterErr)
	}

	normalizedReq := cloneChatRequestWithNormalizedModel(req)

	if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
		return nil, err
	}
	response, callErr = providerAdapter.ChatCompletion(ctx, normalizedReq)

	responseTime := time.Since(startTime).Milliseconds()

	if callErr != nil {
		if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, callErr); settleErr != nil {
			s.traceChatCompletion(ctx, traceReq, response, startTime, time.Now(), billingCtx, settleErr)
			return nil, settleErr
		}
		s.traceChatCompletion(ctx, traceReq, response, startTime, time.Now(), billingCtx, callErr)
		return nil, callErr
	}

	if !providerSelection.UseSystemProvider {
		usage, estimated := s.completeChatUsageFromText(normalizedReq, response.Usage, chatResponseText(response), 0)
		response.Usage = usage
		if estimated {
			markEstimatedUsageSource(billingCtx, response.Usage)
		}
	}

	// Success - settle billing
	tSettle := time.Now()
	if err := s.settleChatSuccess(ctx, billingCtx, providerSelection, channelID, response.Usage, response.Settlement, responseTime); err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "llm gateway timing", "step", "settle_billing", "latency_ms", time.Since(tSettle).Milliseconds(), "attempt", attemptIdx+1)

	// Trace to OpenTelemetry after billing has final usage and cost details.
	endTime := time.Now()
	s.traceChatCompletion(ctx, traceReq, response, startTime, endTime, billingCtx, nil)

	return response, nil
}

// ChatCompletionStream handles streaming chat completion requests
func (s *llmGatewayServiceImpl) ChatCompletionStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.ChatRequest,
) (<-chan adapter.StreamResponse, error) {
	return s.chatCompletionStreamInternal(ctx, apiKey, nil, req)
}

// ChatCompletionStreamWithAppContext handles streaming chat completion requests with app context
func (s *llmGatewayServiceImpl) ChatCompletionStreamWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
) (<-chan adapter.StreamResponse, error) {
	return s.chatCompletionStreamInternal(ctx, apiKey, appCtx, req)
}

// chatCompletionStreamInternal is the internal implementation for streaming
func (s *llmGatewayServiceImpl) chatCompletionStreamInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
) (<-chan adapter.StreamResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()
	ctx = logger.WithFields(ctx,
		zap.String("gateway_request_id", requestID),
		zap.String("model", req.Model),
		zap.String("organization_id", apiKey.OrganizationID),
	)
	logger.DebugContext(ctx, "llm gateway stream request started")

	// 1. Validate request
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}
	effectiveReq := s.policyPrompt.injectChatRequest(req)

	// 2. Check model authorization
	if err := s.checkModelAuthorization(apiKey, appCtx, effectiveReq.Model); err != nil {
		return nil, err
	}

	// 3. Estimate tokens
	promptTokens, completionTokens, _ := s.tokenEstimator.EstimateTotalTokens(
		effectiveReq.Messages,
		effectiveReq.MaxTokens,
		effectiveReq.Model,
	)

	// 4. Select providers
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organizationID: %w", err)
	}

	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "llm gateway stream shadow tenant resolved", "shadow_organization_id", shadowOrganizationID.String())

	// Tag the request category so the router can apply capability-aware matching.
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, "chat")
	if useCase := modelUseCaseForAppContext(appCtx); useCase != "" {
		ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, useCase)
	}

	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, effectiveReq.Provider, effectiveReq.Model, 3)
	if err != nil {
		observability.CaptureError(ctx, "llm.provider.selection_failed", err,
			observability.Tags(map[string]string{"llm.provider": "unknown", "llm.model": req.Model}),
			observability.Attributes(map[string]any{
				"organization_id":  organizationID.String(),
				"shadow_tenant_id": shadowOrganizationID.String(),
			}),
		)
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		err := ErrNoProviderAvailable
		observability.CaptureError(ctx, "llm.provider.unavailable", err,
			observability.Tags(map[string]string{"llm.provider": "unknown", "llm.model": req.Model}),
			observability.Attributes(map[string]any{
				"organization_id":  organizationID.String(),
				"shadow_tenant_id": shadowOrganizationID.String(),
			}),
		)
		return nil, err
	}

	// 5. Try each provider selection with failover
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		outputChan, err := s.tryChatCompletionStream(ctx, apiKey, appCtx, effectiveReq, req, providerSelection, promptTokens, completionTokens, shadowOrganizationID, shadowOwnerID, organizationID, requestID, startTime, attemptIdx)
		if err == nil {
			return outputChan, nil
		}

		lastErr = err
		if errors.Is(err, ErrBillingSettleFailed) {
			return nil, err
		}
		if attemptIdx < len(providerSelections)-1 {
			logger.WarnContext(ctx, "llm stream provider attempt failed, trying next provider",
				"provider", providerSelection.Provider.Provider,
				"route_id", providerSelection.RouteID.String(),
				"attempt", attemptIdx+1,
				err,
			)
			continue
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, ErrNoProviderAvailable
}

func modelUseCaseForAppContext(appCtx *AppContext) string {
	if appCtx == nil {
		return ""
	}
	if appCtx.ModelUseCase != nil {
		if useCase := strings.TrimSpace(*appCtx.ModelUseCase); useCase != "" {
			return useCase
		}
	}
	if appCtx.AppType == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(*appCtx.AppType)) {
	case "agent", "aichat":
		return string(llmmodel.UseCaseAgent)
	case "workflow":
		return string(llmmodel.UseCaseTextChat)
	default:
		return ""
	}
}

// tryChatCompletionStream attempts a streaming chat completion with a single provider
func (s *llmGatewayServiceImpl) tryChatCompletionStream(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ChatRequest,
	traceReq *adapter.ChatRequest,
	providerSelection *ProviderSelection,
	promptTokens, completionTokens int,
	shadowOrganizationID uuid.UUID,
	shadowOwnerID uuid.UUID,
	organizationID uuid.UUID,
	requestID string,
	startTime time.Time,
	attemptIdx int,
) (<-chan adapter.StreamResponse, error) {
	// Calculate estimated credits
	attemptID := buildAttemptID(requestID, attemptIdx)
	quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, pricingModelRefFromSelection(providerSelection), promptTokens, completionTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate credits: %w", err)
	}
	estimatedCredits := quote.TotalCredits

	// Get channel ID
	channelID := getChannelID(providerSelection)

	// Start billing: create context + check balance + pre-deduct
	billingCtx, err := s.beginBillingAttempt(
		ctx,
		apiKey,
		appCtx,
		providerSelection,
		shadowOrganizationID,
		shadowOwnerID,
		estimatedCredits,
		true,
		startTime,
		requestID,
		attemptID,
	)
	if err != nil {
		return nil, err
	}
	lockTokenPricingQuote(billingCtx, quote)
	billingCtx.PromptTokens = promptTokens
	billingCtx.CompletionTokens = completionTokens
	ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.chat.stream")
	ctx = withPlatformProxyMetadata(ctx, billingCtx)

	// Unified adapter path: both official and private channels go through the same adapter flow.
	// Official channels have APIBaseURL pointing to console-api (which returns OpenAI-compatible SSE).
	adapterConfig := s.createAdapterConfig(providerSelection, organizationID)
	providerAdapter, err := s.adapterFactory.CreateAdapter(adapterConfig)
	if err != nil {
		if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
			return nil, rollbackErr
		}
		s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	normalizedReq := cloneChatRequestWithNormalizedModel(req)

	if normalizedReq.StreamOptions == nil {
		normalizedReq.StreamOptions = &adapter.StreamOptions{IncludeUsage: true}
	} else {
		normalizedReq.StreamOptions.IncludeUsage = true
	}

	if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
		return nil, err
	}
	streamChan, err := providerAdapter.ChatCompletionStream(ctx, normalizedReq)
	if err != nil {
		if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
			return nil, rollbackErr
		}
		s.logProviderError(ctx, attemptIdx, providerSelection, err, "stream_call_failed")
		s.recordUpstreamProviderError(ctx, providerSelection, billingCtx, err)

		observability.CaptureError(ctx, "llm.provider.stream_failed", err,
			observability.Tags(map[string]string{
				"llm.provider": providerSelection.Provider.Provider,
				"llm.model":    providerSelection.Model.Model,
			}),
			observability.Attributes(map[string]any{
				"organization_id":     organizationID.String(),
				"attempt_index":       attemptIdx,
				"channel_id":          channelID,
				"use_system_provider": providerSelection.UseSystemProvider,
			}),
		)

		if channelID != nil {
			autoBan := providerSelection.HasRoute() && providerSelection.AutoBan
			s.healthTracker.RecordFailure(ctx, *channelID, autoBan)
		}

		return nil, fmt.Errorf("provider stream call failed: %w", err)
	}

	// Success! Wrap stream to handle billing asynchronously
	outputChan := make(chan adapter.StreamResponse)
	// Preserve trace values while allowing billing and tracing to finish after request cancellation.
	billingBgCtx := context.WithoutCancel(ctx)
	go s.handleStreamBilling(billingBgCtx, streamChan, outputChan, billingCtx, traceReq, &providerSelection.Model, startTime, channelID)

	return outputChan, nil
}
