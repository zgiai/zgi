package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// CreateImageWithAppContext handles image generation requests with app context
func (s *llmGatewayServiceImpl) CreateImageWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ImageRequest,
) (*adapter.ImageResponse, error) {
	return s.createImageInternal(ctx, apiKey, appCtx, req)
}

// validateImageRequest validates the image generation request
func (s *llmGatewayServiceImpl) validateImageRequest(req *adapter.ImageRequest) error {
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}

func buildImagePricingRequest(req *adapter.ImageRequest) *adapter.ImageRequest {
	if req == nil {
		return nil
	}

	effectiveReq := *req
	if strings.TrimSpace(effectiveReq.Size) == "" {
		effectiveReq.Size = "1024x1024"
	}
	if strings.TrimSpace(effectiveReq.Quality) == "" {
		effectiveReq.Quality = "standard"
	}

	return &effectiveReq
}

func buildProviderImageRequest(req *adapter.ImageRequest) adapter.ImageRequest {
	normalizedReq := *req
	normalizedReq.Model = normalizeRequestedModelName(req.Model)
	return normalizedReq
}

func (s *llmGatewayServiceImpl) settleImageSuccess(
	ctx context.Context,
	billingCtx *BillingContext,
	providerSelection *ProviderSelection,
	actualQuote PricingQuote,
	settlement *adapter.SettlementResult,
	responseTime int64,
) error {
	billingCtx.ResponseTime = responseTime
	billingCtx.Status = "success"

	decision, laneErr := s.resolveBillingDecision(providerSelection, billingCtx)
	if laneErr != nil {
		wrappedLaneErr := wrapBillingLaneMismatchError(laneErr)
		logBillingEvent(
			billingCode("BILLING_LANE_MISMATCH", decision.UseSystemProvider),
			billingCtx,
			decision.RouteID,
			decision.UseSystemProvider,
			"settle",
			"error",
			wrappedLaneErr,
		)
		return wrappedLaneErr
	}
	if decision.UseSystemProvider {
		if err := s.finalizePlatformProxySettlement(ctx, billingCtx, settlement, decision); err != nil {
			return err
		}
		return nil
	}

	billingCtx.ActualCredits = actualQuote.TotalCredits
	applyPricingQuoteToBillingContext(billingCtx, actualQuote)

	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID
	if err := s.billingProviderForDecision(decision).Settle(ctx, billingCtx); err != nil {
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"settle",
			"error",
			err,
		)
		return err
	}

	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"settle",
		"ok",
		nil,
	)

	return nil
}

// createImageInternal handles the internal logic for image generation
func (s *llmGatewayServiceImpl) createImageInternal(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.ImageRequest,
) (*adapter.ImageResponse, error) {
	startTime := time.Now()
	requestID := uuid.New().String()

	// 1. Validate request
	if err := s.validateImageRequest(req); err != nil {
		return nil, err
	}
	effectiveReq := s.policyPrompt.injectImageRequest(req)

	// 2. Get shadow tenant info
	orgUUID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}
	billingOrganizationID, ownerID, err := s.getShadowTenantInfo(ctx, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shadow tenant info: %w", err)
	}

	// 3. Select provider using V2 ChannelRouter
	// Note: We use maxSelections=1 because we don't support fallback for image generation yet (or maybe later)
	// Tag the request category so the router can apply capability-aware matching.
	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, "image")
	selections, err := s.selectProvidersWithChannelRouter(ctx, billingOrganizationID, "", effectiveReq.Model, 1)
	if err != nil {
		return nil, err
	}
	selection := selections[0] // Use the first selection (highest priority)

	pricingReq := buildImagePricingRequest(effectiveReq)

	// 4. Calculate estimated credits
	estimatedQuote, err := s.quoteImagePricing(ctx, pricingModelRefFromSelection(selection), pricingReq)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate image credits: %w", err)
	}
	estimatedCredits := estimatedQuote.TotalCredits

	// 5. Begin billing attempt using the shared billing pipeline.
	attemptID := buildAttemptID(requestID, 0)
	billingCtx, err := s.beginBillingAttempt(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		estimatedCredits,
		false,
		startTime,
		requestID,
		attemptID,
	)
	if err != nil {
		return nil, err
	}
	ctx = withLLMLangfuseTraceContext(ctx, billingCtx, "llm.images")
	ctx = withPlatformProxyMetadata(ctx, billingCtx)

	// 8. Create adapter config
	config := s.createAdapterConfig(selection, billingOrganizationID)

	// 9. Get adapter instance
	providerAdapter, err := s.adapterFactory.CreateAdapter(config)
	if err != nil {
		// Release pre-deduction
		s.rollbackPreDeduction(ctx, billingCtx)
		return nil, fmt.Errorf("failed to get adapter for provider %s: %w", config.ProviderName, err)
	}

	// 10. Call adapter
	providerReq := buildProviderImageRequest(effectiveReq)
	if err := s.activateUpstreamProbeForAttempt(ctx, selection, billingCtx); err != nil {
		return nil, err
	}
	resp, err := providerAdapter.CreateImage(ctx, &providerReq)
	responseTime := time.Since(startTime).Milliseconds()
	if err != nil {
		// Log provider error
		s.logProviderError(ctx, 0, selection, err, "image_generation")
		s.recordUpstreamProviderError(ctx, selection, billingCtx, err)

		// Record failure for health tracking
		if selection.HasRoute() {
			autoBan := selection.RouteID != uuid.Nil
			s.healthTracker.RecordFailure(ctx, selection.RouteID, autoBan)
		}

		// Release pre-deduction
		s.rollbackPreDeduction(ctx, billingCtx)
		s.traceImageGeneration(ctx, req, resp, startTime, time.Now(), billingCtx, err)
		return nil, err
	}

	// Record success for health tracking
	if selection.HasRoute() {
		s.healthTracker.RecordSuccess(selection.RouteID)
	}
	s.recordUpstreamProviderSuccess(ctx, selection, billingCtx)

	// 11. Settle billing
	actualQuote := estimatedQuote
	decision, laneErr := s.resolveBillingDecision(selection, billingCtx)
	if laneErr != nil {
		return nil, wrapBillingLaneMismatchError(laneErr)
	}
	if decision.UseSystemProvider {
		actualQuote = PricingQuote{}
	}

	if err := s.settleImageSuccess(ctx, billingCtx, selection, actualQuote, resp.Settlement, responseTime); err != nil {
		s.traceImageGeneration(ctx, req, resp, startTime, time.Now(), billingCtx, err)
		return nil, err
	}

	s.traceImageGeneration(ctx, req, resp, startTime, time.Now(), billingCtx, nil)
	return resp, nil
}

func (s *llmGatewayServiceImpl) quoteImagePricingForSettlement(
	ctx context.Context,
	model PricingModelRef,
	req *adapter.ImageRequest,
	estimatedQuote PricingQuote,
) (PricingQuote, error) {
	if estimatedQuote.TotalCredits > 0 || !estimatedQuote.TotalUSD.IsZero() {
		return estimatedQuote, nil
	}
	return s.quoteImagePricing(ctx, model, req)
}
