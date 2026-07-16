package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// CreateResponseWithAppContext handles response creation requests with app context
func (s *llmGatewayServiceImpl) CreateResponseWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.CreateResponseRequest,
) (*adapter.CreateResponseResponse, error) {
	return s.createResponseInternal(ctx, apiKey, appCtx, req)
}

// CreateEmbeddingsWithAppContext handles embeddings creation requests with app context
func (s *llmGatewayServiceImpl) CreateEmbeddingsWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.EmbeddingsRequest,
) (*adapter.EmbeddingsResponse, error) {
	return s.createEmbeddingsInternal(ctx, apiKey, appCtx, req)
}

// RerankWithAppContext handles rerank requests with app context
func (s *llmGatewayServiceImpl) RerankWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.RerankRequest,
) (*adapter.RerankResponse, error) {
	return s.rerankInternal(ctx, apiKey, appCtx, req)
}

// createResponseInternal is the internal implementation for create response with optional AppContext
func (s *llmGatewayServiceImpl) createResponseInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.CreateResponseRequest,
) (*adapter.CreateResponseResponse, error) {
	startTime := time.Now()

	// 1. Validate request
	if err := s.validateCreateResponseRequest(req); err != nil {
		return nil, err
	}
	effectiveReq := s.policyPrompt.injectCreateResponseRequest(req)

	// 2. Check model authorization
	if err := s.checkModelAuthorization(apiKey, appCtx, effectiveReq.Model); err != nil {
		return nil, err
	}

	// 3. Estimate tokens
	promptTokens := s.tokenEstimator.EstimateCreateResponsePromptTokens(effectiveReq)
	completionTokens := s.tokenEstimator.EstimateCreateResponseCompletionTokens(effectiveReq)

	// 4. Select providers
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", effectiveReq.Model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(effectiveReq.Model, shadowOrganizationID.String())
	}

	// 5. Try each provider selection
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		requestID := uuid.New().String()
		quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, pricingModelRefFromSelection(providerSelection), promptTokens, completionTokens)
		if err != nil {
			lastErr = fmt.Errorf("failed to calculate credits: %w", err)
			continue
		}
		estimatedCredits := quote.TotalCredits

		channelID := getChannelID(providerSelection)
		attemptID := buildAttemptID(requestID, attemptIdx)

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
			lastErr = err
			continue
		}
		lockTokenPricingQuote(billingCtx, quote)
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.responses")
		ctx = withPlatformProxyMetadata(ctx, billingCtx)

		adapterConfig := s.createAdapterConfig(providerSelection, organizationID)
		providerAdapter, err := s.adapterFactory.CreateAdapter(adapterConfig)
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
			lastErr = err
			continue
		}
		response, err := providerAdapter.CreateResponse(ctx, effectiveReq)
		responseTime := time.Since(startTime).Milliseconds()

		if err != nil {
			if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
				s.traceCreateResponse(ctx, req, response, startTime, time.Now(), billingCtx, settleErr)
				return nil, settleErr
			}
			s.traceCreateResponse(ctx, req, response, startTime, time.Now(), billingCtx, err)
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				logger.WarnContext(ctx, "llm provider attempt failed, trying next provider",
					"provider", providerSelection.Provider.Provider,
					"route_id", providerSelection.RouteID.String(),
					"attempt", attemptIdx+1,
					err,
				)
				continue
			}
			return nil, fmt.Errorf("all providers failed: %w", lastErr)
		}

		var responseUsage *adapter.Usage
		if response != nil {
			responseUsage = response.Usage
		}
		if response != nil && !providerSelection.UseSystemProvider {
			if usage, estimated := s.completeCreateResponseUsageFromText(effectiveReq, response.Usage, createResponseText(response), promptTokens); hasBillableTokenUsage(usage) {
				response.Usage = usage
				responseUsage = usage
				if estimated {
					markEstimatedUsageSource(billingCtx, usage)
				}
			}
		}

		if err := s.settleChatSuccess(ctx, billingCtx, providerSelection, channelID, responseUsage, nil, responseTime); err != nil {
			return nil, err
		}

		s.traceCreateResponse(ctx, req, response, startTime, time.Now(), billingCtx, nil)
		return response, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, NewNoProviderAvailableError(req.Model, shadowOrganizationID.String())
}

// createEmbeddingsInternal is the internal implementation for embeddings with optional AppContext
func (s *llmGatewayServiceImpl) createEmbeddingsInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.EmbeddingsRequest,
) (*adapter.EmbeddingsResponse, error) {
	startTime := time.Now()

	// 1. Validate request
	if err := s.validateEmbeddingsRequest(req); err != nil {
		return nil, err
	}

	// 2. Check model authorization
	if err := s.checkModelAuthorization(apiKey, appCtx, req.Model); err != nil {
		return nil, err
	}

	// 3. Estimate tokens
	promptTokens := s.tokenEstimator.EstimateEmbeddingTokens(req.Input, req.Model)

	// 4. Select providers
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", req.Model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(req.Model, shadowOrganizationID.String())
	}

	// 5. Try each provider selection
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		requestID := uuid.New().String()
		modelRef := pricingModelRefFromSelection(providerSelection)
		modelRef.Operation = PricingOperationEmbedding
		quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, modelRef, promptTokens, 0)
		if err != nil {
			lastErr = fmt.Errorf("failed to calculate credits: %w", err)
			continue
		}
		estimatedCredits := quote.TotalCredits

		channelID := getChannelID(providerSelection)
		attemptID := buildAttemptID(requestID, attemptIdx)

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
			lastErr = err
			continue
		}
		billingCtx.PricingOperation = PricingOperationEmbedding
		lockTokenPricingQuote(billingCtx, quote)
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.embeddings")
		ctx = withPlatformProxyMetadata(ctx, billingCtx)

		adapterConfig := s.createAdapterConfig(providerSelection, organizationID)
		providerAdapter, err := s.adapterFactory.CreateAdapter(adapterConfig)
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
			lastErr = err
			continue
		}
		response, err := providerAdapter.CreateEmbeddings(ctx, req)
		responseTime := time.Since(startTime).Milliseconds()

		if err != nil {
			if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
				s.traceEmbeddings(ctx, req, response, startTime, time.Now(), billingCtx, settleErr)
				return nil, settleErr
			}
			s.traceEmbeddings(ctx, req, response, startTime, time.Now(), billingCtx, err)
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, lastErr
		}

		actualTokens, estimatedUsage := ensureEmbeddingUsageForSelection(providerSelection, response, promptTokens)
		if estimatedUsage {
			markEstimatedUsageSource(billingCtx, &response.Usage)
		}

		if err := s.settleEmbeddingsSuccess(ctx, billingCtx, providerSelection, channelID, actualTokens, response.Settlement, responseTime); err != nil {
			return nil, err
		}

		s.traceEmbeddings(ctx, req, response, startTime, time.Now(), billingCtx, nil)
		return response, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, NewNoProviderAvailableError(req.Model, shadowOrganizationID.String())
}

// rerankInternal is the internal implementation for rerank with optional AppContext
func (s *llmGatewayServiceImpl) rerankInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.RerankRequest,
) (*adapter.RerankResponse, error) {
	startTime := time.Now()

	// 1. Validate request
	if err := s.validateRerankRequest(req); err != nil {
		return nil, err
	}

	// 2. Check model authorization
	if err := s.checkModelAuthorization(apiKey, appCtx, req.Model); err != nil {
		return nil, err
	}

	// 3. Estimate tokens (for rerank, we estimate based on query + documents)
	promptTokens := s.tokenEstimator.EstimateRerankTokens(req.Query, req.Documents, req.Model)

	// 4. Select providers
	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}

	shadowOrganizationID, shadowOwnerID, err := s.resolveShadowContext(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	providerSelections, err := s.selectProvidersWithChannelRouter(ctx, shadowOrganizationID, "", req.Model, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to select provider: %w", err)
	}
	if len(providerSelections) == 0 {
		return nil, NewNoProviderAvailableError(req.Model, shadowOrganizationID.String())
	}

	// 5. Try each provider selection
	var lastErr error
	for attemptIdx, providerSelection := range providerSelections {
		requestID := uuid.New().String()
		modelRef := pricingModelRefFromSelection(providerSelection)
		modelRef.Operation = PricingOperationRerank
		quote, err := s.quoteTokenPricingForSelection(ctx, providerSelection, modelRef, promptTokens, 0)
		if err != nil {
			lastErr = fmt.Errorf("failed to calculate credits: %w", err)
			continue
		}
		estimatedCredits := quote.TotalCredits

		channelID := getChannelID(providerSelection)
		attemptID := buildAttemptID(requestID, attemptIdx)

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
			lastErr = err
			continue
		}
		billingCtx.PricingOperation = PricingOperationRerank
		lockTokenPricingQuote(billingCtx, quote)
		ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.rerank")
		ctx = withPlatformProxyMetadata(ctx, billingCtx)

		adapterConfig := s.createAdapterConfig(providerSelection, organizationID)
		providerAdapter, err := s.adapterFactory.CreateAdapter(adapterConfig)
		if err != nil {
			if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
				return nil, rollbackErr
			}
			lastErr = fmt.Errorf("failed to create adapter: %w", err)
			s.logProviderError(ctx, attemptIdx, providerSelection, err, "adapter_creation_failed")
			continue
		}

		if err := s.activateUpstreamProbeForAttempt(ctx, providerSelection, billingCtx); err != nil {
			lastErr = err
			continue
		}
		response, err := providerAdapter.Rerank(ctx, req)
		responseTime := time.Since(startTime).Milliseconds()

		if err != nil {
			if settleErr := s.handleProviderError(ctx, billingCtx, providerSelection, channelID, responseTime, attemptIdx, err); settleErr != nil {
				s.traceRerank(ctx, req, response, startTime, time.Now(), billingCtx, settleErr)
				return nil, settleErr
			}
			s.traceRerank(ctx, req, response, startTime, time.Now(), billingCtx, err)
			lastErr = err
			if attemptIdx < len(providerSelections)-1 {
				continue
			}
			return nil, lastErr
		}

		actualTokens, estimatedUsage := ensureRerankUsageForSelection(providerSelection, response, promptTokens)
		if estimatedUsage {
			markEstimatedUsageSource(billingCtx, response.Usage)
		}

		if err := s.settleEmbeddingsSuccess(ctx, billingCtx, providerSelection, channelID, actualTokens, response.Settlement, responseTime); err != nil {
			return nil, err
		}

		s.traceRerank(ctx, req, response, startTime, time.Now(), billingCtx, nil)
		return response, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed: %w", lastErr)
	}
	return nil, NewNoProviderAvailableError(req.Model, shadowOrganizationID.String())
}
