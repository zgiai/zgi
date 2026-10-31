package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/gorm"
)

type videoCapableAdapter interface {
	CreateVideo(ctx context.Context, request *adapter.VideoRequest) (*adapter.VideoResponse, error)
	GetVideoTask(ctx context.Context, request *adapter.VideoTaskRequest) (*adapter.VideoResponse, error)
}

const defaultVideoPredeductDurationSeconds = 5

func (s *llmGatewayServiceImpl) CreateVideo(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.VideoRequest,
) (*adapter.VideoResponse, error) {
	return s.CreateVideoWithAppContext(ctx, apiKey, nil, req)
}

func (s *llmGatewayServiceImpl) CreateVideoWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.VideoRequest,
) (*adapter.VideoResponse, error) {
	startTime := time.Now()
	if err := s.validateVideoRequest(req); err != nil {
		return nil, err
	}
	if err := s.checkModelAuthorization(apiKey, appCtx, req.Model); err != nil {
		return nil, err
	}
	billingOrganizationID, ownerID, err := s.videoBillingTenant(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, "video")
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationVideoCreate)
	if useCase := modelUseCaseForAppContext(appCtx); useCase != "" {
		ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, useCase)
	}
	selections, err := s.selectProvidersWithChannelRouter(ctx, billingOrganizationID, req.Provider, req.Model, 3)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for idx, selection := range selections {
		providerReq := *req
		providerReq.Model = normalizeRequestedModelName(req.Model)
		billingCtx, err := s.beginVideoPredeductAttempt(ctx, apiKey, appCtx, selection, billingOrganizationID, ownerID, &providerReq, startTime, idx)
		if err != nil {
			lastErr = err
			continue
		}
		callCtx := ctx
		if billingCtx != nil {
			callCtx = withPlatformProxyMetadata(callCtx, billingCtx)
			if err := s.activateUpstreamProbeForAttempt(callCtx, selection, billingCtx); err != nil {
				if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
					return nil, rollbackErr
				}
				lastErr = err
				continue
			}
		}
		resp, err := s.callVideoCreate(callCtx, selection, billingOrganizationID, &providerReq)
		if err != nil {
			if billingCtx != nil {
				if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
					return nil, rollbackErr
				}
			}
			lastErr = err
			s.logProviderError(ctx, idx, selection, err, "video_generation")
			if selection.HasRoute() {
				s.healthTracker.RecordFailure(ctx, selection.RouteID, selection.AutoBan)
			}
			continue
		}
		if selection.HasRoute() {
			s.healthTracker.RecordSuccess(selection.RouteID)
		}
		annotateVideoResponse(resp, selection)
		if billingCtx != nil {
			resp.EstimatedCredits = billingCtx.EstimatedCredits
			resp.BillingRequestID = billingCtx.RequestID
			resp.BillingAttemptID = billingCtx.AttemptID
		}
		if isFailedVideoStatus(resp.Status) || !videoResponseHasTaskID(resp) {
			if billingCtx != nil {
				if rollbackErr := s.rollbackPreDeduction(ctx, billingCtx); rollbackErr != nil {
					return nil, rollbackErr
				}
			}
			lastErr = videoResponseFailureError(resp, "upstream video task id is empty")
			if isFailedVideoStatus(resp.Status) {
				lastErr = videoResponseFailureError(resp, "upstream video task failed")
			}
			continue
		}
		_ = time.Since(startTime)
		return resp, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no available video channel found for model: %s", req.Model)
}

func (s *llmGatewayServiceImpl) GetVideoTask(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.VideoTaskRequest,
) (*adapter.VideoResponse, error) {
	return s.GetVideoTaskWithAppContext(ctx, apiKey, nil, req)
}

func (s *llmGatewayServiceImpl) GetVideoTaskWithAppContext(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	req *adapter.VideoTaskRequest,
) (*adapter.VideoResponse, error) {
	if err := s.validateVideoTaskRequest(req); err != nil {
		return nil, err
	}
	if err := s.checkModelAuthorization(apiKey, appCtx, req.Model); err != nil {
		return nil, err
	}
	billingOrganizationID, ownerID, err := s.videoBillingTenant(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, shared.ContextKeyModelCategory, "video")
	ctx = context.WithValue(ctx, shared.ContextKeyModelOperation, operationVideoRetrieve)
	if useCase := modelUseCaseForAppContext(appCtx); useCase != "" {
		ctx = context.WithValue(ctx, shared.ContextKeyModelUseCase, useCase)
	}
	selections, err := s.selectProvidersWithChannelRouter(ctx, billingOrganizationID, req.Provider, req.Model, 3)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for idx, selection := range selections {
		providerReq := *req
		providerReq.Model = normalizeRequestedModelName(req.Model)
		resp, err := s.callVideoTask(ctx, selection, billingOrganizationID, &providerReq)
		if err != nil {
			lastErr = err
			s.logProviderError(ctx, idx, selection, err, "video_task_query")
			if selection.HasRoute() {
				s.healthTracker.RecordFailure(ctx, selection.RouteID, selection.AutoBan)
			}
			continue
		}
		if selection.HasRoute() {
			s.healthTracker.RecordSuccess(selection.RouteID)
		}
		annotateVideoResponse(resp, selection)
		if err := s.settleVideoTask(ctx, apiKey, appCtx, selection, billingOrganizationID, ownerID, req, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no available video channel found for model: %s", req.Model)
}

func (s *llmGatewayServiceImpl) beginVideoPredeductAttempt(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	req *adapter.VideoRequest,
	startTime time.Time,
	attemptIdx int,
) (*BillingContext, error) {
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve billing lane for video prededuct: %w", err)
	}
	var quote PricingQuote
	if lane == UsageBillingLanePlatform {
		quote = PricingQuote{
			PricingSource: PricingSourcePlatformRemoteAuthority,
			UsageSource:   UsageSourceRequestParameters,
		}
	} else {
		quote, err = s.videoPredeductQuoteFromCreate(ctx, selection, req)
		if err != nil {
			return nil, err
		}
	}
	requestID, attemptID := videoPredeductIDs(appCtx, attemptIdx)
	ctx = withVideoMeteredUsage(ctx, selection, req)
	billingCtx, err := s.beginBillingAttempt(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		quote.TotalCredits,
		false,
		startTime,
		requestID,
		attemptID,
		quote,
	)
	if err != nil {
		return nil, err
	}
	billingCtx.PricingOperation = PricingOperationVideo
	applyPricingQuoteToBillingContext(billingCtx, quote)
	return billingCtx, nil
}

func videoPredeductIDs(appCtx *AppContext, attemptIdx int) (string, string) {
	requestID := videoPredeductRequestID(appCtx)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return requestID, buildAttemptID(requestID, attemptIdx)
}

func videoPredeductRequestID(appCtx *AppContext) string {
	if appCtx == nil || appCtx.AppID == nil || *appCtx.AppID == uuid.Nil {
		return ""
	}
	return "video:" + appCtx.AppID.String()
}

func (s *llmGatewayServiceImpl) videoPredeductQuoteFromCreate(ctx context.Context, selection *ProviderSelection, req *adapter.VideoRequest) (PricingQuote, error) {
	seconds := defaultVideoPredeductDurationSeconds
	if req != nil && req.Duration != nil && *req.Duration > 0 {
		seconds = *req.Duration
	}
	count := 1
	if req != nil && req.N != nil && *req.N > 0 {
		count = *req.N
	}
	costReq := videoGenerationCostRequestFromCreate(req, seconds, count)
	return s.videoPredeductQuote(ctx, selection, strings.TrimSpace(reqModelName(req)), costReq)
}

func (s *llmGatewayServiceImpl) videoPredeductQuoteFromTask(ctx context.Context, selection *ProviderSelection, req *adapter.VideoTaskRequest) (PricingQuote, error) {
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("failed to resolve billing lane for video prededuct: %w", err)
	}
	if lane == UsageBillingLanePlatform {
		return PricingQuote{
			PricingSource: PricingSourcePlatformRemoteAuthority,
			UsageSource:   UsageSourceRequestParameters,
		}, nil
	}
	seconds := intFromAny(videoTaskAdditionalParam(req, "duration"))
	if seconds <= 0 {
		seconds = defaultVideoPredeductDurationSeconds
	}
	count := intFromAny(videoTaskAdditionalParam(req, "count"))
	if count <= 0 {
		count = 1
	}
	modelName := ""
	if req != nil {
		modelName = strings.TrimSpace(req.Model)
	}
	costReq := videoGenerationCostRequestFromTask(req, nil, seconds, count)
	return s.videoPredeductQuote(ctx, selection, modelName, costReq)
}

func (s *llmGatewayServiceImpl) videoPredeductQuote(ctx context.Context, selection *ProviderSelection, requestModel string, costReq modelpricing.VideoGenerationCostRequest) (PricingQuote, error) {
	if costReq.DurationSeconds <= 0 {
		costReq.DurationSeconds = defaultVideoPredeductDurationSeconds
	}
	if costReq.Count <= 0 {
		costReq.Count = 1
	}
	// The organization rate is only consulted for CNY pricing. A missing rate
	// fails closed inside quoteVideoFromModelPricing; a USD price book does not
	// require one.
	rate := decimal.Zero
	if s != nil && selection != nil {
		if loaded, err := s.organizationUSDToCNYRate(ctx, selection.OrganizationID); err == nil {
			rate = loaded
		}
	}
	quote, err := quoteVideoFromModelPricing(selection, requestModel, costReq, rate)
	if err != nil {
		return PricingQuote{}, err
	}
	if quote.TotalCredits <= 0 {
		return PricingQuote{}, fmt.Errorf("%w: video pricing is not configured", ErrPricingNotConfigured)
	}
	return quote, nil
}

// defaultVideoTokenReservation bounds the reservation for a token-metered video
// model that has no token_estimation rule. The upstream token count is only
// known after generation, so the pre-deduct reserves one priced unit and the
// settlement trues up to the actual count.
const defaultVideoTokenReservation = 1_000_000

// quoteVideoFromModelPricing derives the reservation from the selected model's
// structured video rule. A CNY price book without a positive rate is
// fail-closed.
func quoteVideoFromModelPricing(selection *ProviderSelection, requestModel string, costReq modelpricing.VideoGenerationCostRequest, cnyPerUSD decimal.Decimal) (PricingQuote, error) {
	if selection == nil || len(selection.Model.Pricing) == 0 {
		return PricingQuote{}, fmt.Errorf("%w: video pricing is not configured", ErrPricingNotConfigured)
	}
	cost, present, err := modelpricing.ResolveVideoGenerationCost(json.RawMessage(selection.Model.Pricing), costReq)
	if err != nil {
		// Token-metered video without an estimator: retry with one priced unit.
		fallbackReq := costReq
		fallbackReq.VideoTokens = defaultVideoTokenReservation
		if retryCost, retryPresent, retryErr := modelpricing.ResolveVideoGenerationCost(json.RawMessage(selection.Model.Pricing), fallbackReq); retryErr == nil {
			cost, present, err = retryCost, retryPresent, nil
		}
	}
	if err != nil || !present || cost.SourceCost <= 0 {
		return PricingQuote{}, fmt.Errorf("%w: video pricing is not configured", ErrPricingNotConfigured)
	}
	currency := strings.ToUpper(strings.TrimSpace(cost.Currency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(selection.Model.Currency))
	}
	// Explicit boundary: the video cost resolver projects legacy JSON as a
	// float. Convert once here; settlement arithmetic below is decimal.
	totalSource := decimal.NewFromFloat(cost.SourceCost)
	totalUSD := totalSource
	if currency == "CNY" {
		if !cnyPerUSD.IsPositive() {
			return PricingQuote{}, fmt.Errorf("%w: video CNY pricing requires a positive USD to CNY rate", ErrExchangeRateUnavailable)
		}
		totalUSD = totalSource.Div(cnyPerUSD)
	} else if currency != "" && currency != "USD" {
		return PricingQuote{}, fmt.Errorf("%w: unsupported video pricing currency %q", ErrPricingNotConfigured, currency)
	}
	modelName := strings.TrimSpace(requestModel)
	if strings.TrimSpace(selection.Model.Model) != "" {
		modelName = strings.TrimSpace(selection.Model.Model)
	}
	snapshot := buildPricingSnapshot(map[string]any{
		"pricing_source":     PricingSourceUpstreamModelPrice,
		"usage_source":       UsageSourceRequestParameters,
		"operation":          PricingOperationVideo,
		"model_id":           selection.Model.ID.String(),
		"provider":           selection.Model.Provider,
		"model":              modelName,
		"billing_unit":       cost.BillingUnit,
		"unit_price":         cost.UnitPrice,
		"estimated_quantity": cost.Quantity,
		"duration_seconds":   cost.DurationSeconds,
		"count":              costReq.Count,
		"resolution":         cost.Resolution,
		"input_video":        cost.InputVideo,
		"audio":              cost.Audio,
		"pricing_rule_id":    cost.RuleID,
	})
	ruleID := strings.TrimSpace(cost.RuleID)
	if ruleID == "" {
		ruleID = "model.video_generation"
	}
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, ruleID, UsageSourceRequestParameters, snapshot), nil
}

// videoMeteredFact declares the video metered identity used by the unified
// metered pre-deduct. The billing unit and the integer base-unit quantity come
// from the synced catalog, so the platform lane reserves exactly what the
// catalogue defines; when the model has no video price there is no identity to
// declare and the caller keeps the legacy path.
func videoMeteredFact(pricing json.RawMessage, req *adapter.VideoRequest) (shared.MeteredUsageFact, bool) {
	seconds := defaultVideoPredeductDurationSeconds
	if req != nil && req.Duration != nil && *req.Duration > 0 {
		seconds = *req.Duration
	}
	count := 1
	if req != nil && req.N != nil && *req.N > 0 {
		count = *req.N
	}
	if len(pricing) == 0 {
		return shared.MeteredUsageFact{}, false
	}
	costReq := videoGenerationCostRequestFromCreate(req, seconds, count)
	cost, present, err := modelpricing.ResolveVideoGenerationCost(pricing, costReq)
	if err != nil {
		// Token-metered video without an estimator: retry with one priced unit so
		// the platform pre-deduct still holds a bounded reservation.
		fallbackReq := costReq
		fallbackReq.VideoTokens = defaultVideoTokenReservation
		if retryCost, retryPresent, retryErr := modelpricing.ResolveVideoGenerationCost(pricing, fallbackReq); retryErr == nil {
			cost, present, err = retryCost, retryPresent, nil
		}
	}
	if err != nil || !present || strings.TrimSpace(cost.BillingUnit) == "" {
		return shared.MeteredUsageFact{}, false
	}
	quantity := int64(cost.Quantity)
	if cost.BillingUnit == "million_video_tokens" {
		quantity = int64(cost.VideoTokens)
	}
	if quantity <= 0 {
		return shared.MeteredUsageFact{}, false
	}
	fact := shared.MeteredUsageFact{
		Operation: string(PricingOperationVideo),
		Meter:     "video",
		BaseUnit:  cost.BillingUnit,
		Quantity:  quantity,
		Dimensions: map[string]string{
			"resolution":       cost.Resolution,
			"input_video":      strconv.FormatBool(cost.InputVideo),
			"audio":            strconv.FormatBool(cost.Audio),
			"duration_seconds": strconv.FormatFloat(cost.DurationSeconds, 'f', -1, 64),
			"count":            strconv.Itoa(costReq.Count),
		},
	}
	// Console rebuilds the metered snapshot from these dimensions, so the token
	// count used for the estimate must be the resolved one (which may be the
	// bounded fallback), not only the request-provided count.
	if cost.VideoTokens > 0 {
		fact.Dimensions["video_tokens"] = strconv.Itoa(cost.VideoTokens)
	}
	return fact, true
}

func withVideoMeteredUsage(ctx context.Context, selection *ProviderSelection, req *adapter.VideoRequest) context.Context {
	if selection == nil {
		return ctx
	}
	fact, ok := videoMeteredFact(json.RawMessage(selection.Model.Pricing), req)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, shared.ContextKeyMeteredUsage, &fact)
}

func videoGenerationCostRequestFromCreate(req *adapter.VideoRequest, seconds, count int) modelpricing.VideoGenerationCostRequest {
	costReq := modelpricing.VideoGenerationCostRequest{
		DurationSeconds: float64(seconds),
		Count:           count,
	}
	if req == nil {
		return costReq
	}
	costReq.Resolution = videoCreateResolution(req)
	costReq.InputVideo = videoCreateInputVideo(req)
	costReq.Audio = videoCreateAudio(req)
	if tokens := videoCreateTokenCount(req); tokens > 0 {
		costReq.VideoTokens = tokens
	}
	return costReq
}

func videoGenerationCostRequestFromTask(req *adapter.VideoTaskRequest, resp *adapter.VideoResponse, seconds, count int) modelpricing.VideoGenerationCostRequest {
	costReq := modelpricing.VideoGenerationCostRequest{
		DurationSeconds: float64(seconds),
		Count:           count,
		Resolution:      videoPricingResolution(req, resp),
		InputVideo:      videoPricingInputVideo(req, resp),
	}
	if req != nil && req.AdditionalParameters != nil {
		if value, ok := boolFromAny(req.AdditionalParameters["audio"]); ok {
			costReq.Audio = value
		} else if value, ok := boolFromAny(req.AdditionalParameters["output_audio"]); ok {
			costReq.Audio = value
		} else if value, ok := boolFromAny(req.AdditionalParameters["generate_audio"]); ok {
			costReq.Audio = value
		}
	}
	if resp != nil && resp.Raw != nil {
		if value, ok := boolFromAny(resp.Raw["audio"]); ok {
			costReq.Audio = value
		} else if value, ok := boolFromAny(resp.Raw["output_audio"]); ok {
			costReq.Audio = value
		} else if value, ok := boolFromAny(resp.Raw["generate_audio"]); ok {
			costReq.Audio = value
		}
	}
	costReq.VideoTokens = videoPricingTokenCount(req, resp)
	return costReq
}

func videoCreateResolution(req *adapter.VideoRequest) string {
	if req == nil {
		return ""
	}
	if value := strings.ToLower(strings.TrimSpace(req.Resolution)); value != "" {
		return value
	}
	if value := strings.ToLower(strings.TrimSpace(req.Size)); value != "" {
		return value
	}
	return ""
}

func videoCreateInputVideo(req *adapter.VideoRequest) bool {
	if req == nil {
		return false
	}
	if strings.TrimSpace(req.VideoURL) != "" {
		return true
	}
	for _, refType := range req.ReferenceTypes {
		switch strings.ToLower(strings.TrimSpace(refType)) {
		case "video", "video_url", "input_video":
			return true
		}
	}
	for _, refURL := range req.ReferenceURLs {
		if strings.TrimSpace(refURL) != "" {
			return true
		}
	}
	return false
}

func videoCreateAudio(req *adapter.VideoRequest) bool {
	if req == nil {
		return false
	}
	if req.GenerateAudio != nil {
		return *req.GenerateAudio
	}
	if strings.TrimSpace(req.AudioURL) != "" {
		return true
	}
	return false
}

func videoCreateTokenCount(req *adapter.VideoRequest) int {
	if req == nil || req.AdditionalParameters == nil {
		return 0
	}
	for _, key := range []string{"video_tokens", "total_video_tokens", "output_video_tokens", "completion_tokens", "total_tokens"} {
		if value := intFromAny(req.AdditionalParameters[key]); value > 0 {
			return value
		}
	}
	return 0
}

func reqModelName(req *adapter.VideoRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

func videoTaskAdditionalParam(req *adapter.VideoTaskRequest, key string) interface{} {
	if req == nil || req.AdditionalParameters == nil {
		return nil
	}
	return req.AdditionalParameters[key]
}

func videoTaskStringParam(req *adapter.VideoTaskRequest, key string) string {
	return stringFromAny(videoTaskAdditionalParam(req, key))
}

func intFromAny(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		parsed, _ := v.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func int64FromAny(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case json.Number:
		parsed, _ := v.Int64()
		return parsed
	default:
		return 0
	}
}

func videoTaskEstimatedCredits(req *adapter.VideoTaskRequest, fallback int64) int64 {
	if estimated := int64FromAny(videoTaskAdditionalParam(req, "billing_estimated_credits")); estimated > 0 {
		return estimated
	}
	return fallback
}
func videoBillingIDsFromTask(appCtx *AppContext, req *adapter.VideoTaskRequest) (string, string) {
	requestID := videoTaskStringParam(req, "billing_request_id")
	attemptID := videoTaskStringParam(req, "billing_attempt_id")
	if requestID == "" {
		requestID = videoPredeductRequestID(appCtx)
	}
	if attemptID == "" && requestID != "" {
		attemptID = buildAttemptID(requestID, 0)
	}
	return requestID, attemptID
}

func isFailedVideoStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "canceled", "cancelled", "rejected", "timeout", "timed_out", "expired":
		return true
	default:
		return false
	}
}
func (s *llmGatewayServiceImpl) validateVideoRequest(req *adapter.VideoRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	req.Model = normalizeRequestedModelName(req.Model)
	if strings.TrimSpace(req.Model) == "" {
		return ErrMissingModel
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", adapter.ErrInvalidRequest)
	}
	return nil
}

func (s *llmGatewayServiceImpl) validateVideoTaskRequest(req *adapter.VideoTaskRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	req.Model = normalizeRequestedModelName(req.Model)
	if strings.TrimSpace(req.Model) == "" {
		return ErrMissingModel
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", adapter.ErrInvalidRequest)
	}
	return nil
}

func (s *llmGatewayServiceImpl) videoBillingTenant(ctx context.Context, apiKey *apikeymodel.TenantAPIKey) (uuid.UUID, uuid.UUID, error) {
	orgUUID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid organization id: %w", err)
	}
	billingOrganizationID, ownerID, err := s.getShadowTenantInfo(ctx, orgUUID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("failed to get shadow tenant info: %w", err)
	}
	return billingOrganizationID, ownerID, nil
}

func (s *llmGatewayServiceImpl) callVideoCreate(ctx context.Context, selection *ProviderSelection, organizationID uuid.UUID, req *adapter.VideoRequest) (*adapter.VideoResponse, error) {
	config := s.createAdapterConfig(ctx, selection, organizationID)
	providerAdapter, err := s.adapterFactory.CreateAdapter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get adapter for provider %s: %w", config.ProviderName, err)
	}
	videoAdapter, ok := providerAdapter.(videoCapableAdapter)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support video generation", adapter.ErrCapabilityUnsupported, config.ProviderName)
	}
	return videoAdapter.CreateVideo(ctx, req)
}

func (s *llmGatewayServiceImpl) callVideoTask(ctx context.Context, selection *ProviderSelection, organizationID uuid.UUID, req *adapter.VideoTaskRequest) (*adapter.VideoResponse, error) {
	config := s.createAdapterConfig(ctx, selection, organizationID)
	providerAdapter, err := s.adapterFactory.CreateAdapter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get adapter for provider %s: %w", config.ProviderName, err)
	}
	videoAdapter, ok := providerAdapter.(videoCapableAdapter)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support video task query", adapter.ErrCapabilityUnsupported, config.ProviderName)
	}
	return videoAdapter.GetVideoTask(ctx, req)
}

func videoResponseHasTaskOrURL(resp *adapter.VideoResponse) bool {
	if resp == nil {
		return false
	}
	if strings.TrimSpace(resp.TaskID) != "" || strings.TrimSpace(resp.ID) != "" || strings.TrimSpace(resp.VideoURL) != "" {
		return true
	}
	for _, item := range resp.Data {
		if strings.TrimSpace(item.URL) != "" || strings.TrimSpace(item.B64JSON) != "" {
			return true
		}
	}
	return false
}

func videoResponseHasTaskID(resp *adapter.VideoResponse) bool {
	if resp == nil {
		return false
	}
	return strings.TrimSpace(resp.TaskID) != "" || strings.TrimSpace(resp.ID) != ""
}

func videoResponseFailureError(resp *adapter.VideoResponse, fallback string) error {
	if message := videoGatewayResponseErrorMessage(resp); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}
	return fmt.Errorf("%s", fallback)
}

func videoGatewayResponseErrorMessage(resp *adapter.VideoResponse) string {
	if resp == nil {
		return ""
	}
	if message := videoGatewayErrorMessageFromAny(resp.Error); message != "" {
		return message
	}
	if resp.Raw != nil {
		if message := videoGatewayErrorMessageFromAny(resp.Raw["error"]); message != "" {
			return message
		}
		if message := videoGatewayNestedErrorMessage(resp.Raw); message != "" {
			return message
		}
		if message := videoGatewayErrorMessageFromAny(resp.Raw["error_message"]); message != "" {
			return message
		}
		if message := videoGatewayErrorMessageFromAny(resp.Raw["message"]); message != "" {
			if isVideoGatewaySuccessMessage(message) {
				return ""
			}
			return message
		}
	}
	return ""
}

func videoGatewayNestedErrorMessage(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case map[string]interface{}:
		for _, key := range []string{"error", "errors", "error_message", "errorMessage"} {
			if message := videoGatewayErrorMessageFromAny(typed[key]); message != "" {
				return message
			}
		}
		for _, item := range typed {
			if message := videoGatewayNestedErrorMessage(item); message != "" {
				return message
			}
		}
	case []interface{}:
		for _, item := range typed {
			if message := videoGatewayNestedErrorMessage(item); message != "" {
				return message
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		var mapped map[string]interface{}
		if err := json.Unmarshal(raw, &mapped); err != nil {
			return ""
		}
		return videoGatewayNestedErrorMessage(mapped)
	}
	return ""
}

func isVideoGatewaySuccessMessage(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "success", "ok", "succeeded":
		return true
	default:
		return false
	}
}

func videoGatewayErrorMessageFromAny(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		for _, key := range []string{"message", "error_message", "msg"} {
			if message := videoGatewayErrorMessageFromAny(typed[key]); message != "" {
				return message
			}
		}
		if message := videoGatewayErrorMessageFromAny(typed["error"]); message != "" {
			return message
		}
		if code := videoGatewayErrorMessageFromAny(typed["code"]); code != "" {
			return code
		}
	case []interface{}:
		for _, item := range typed {
			if message := videoGatewayErrorMessageFromAny(item); message != "" {
				return message
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		var mapped map[string]interface{}
		if err := json.Unmarshal(raw, &mapped); err != nil {
			return ""
		}
		return videoGatewayErrorMessageFromAny(mapped)
	}
	return ""
}

func annotateVideoResponse(resp *adapter.VideoResponse, selection *ProviderSelection) {
	if resp == nil || selection == nil {
		return
	}
	resp.Provider = selection.ChannelProvider
	resp.Protocol = "openai"
	if selection.RouteID != uuid.Nil {
		resp.ChannelID = selection.RouteID.String()
	}
}

func (s *llmGatewayServiceImpl) settleVideoTask(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	req *adapter.VideoTaskRequest,
	resp *adapter.VideoResponse,
) error {
	if resp == nil {
		return nil
	}
	if isSuccessfulVideoStatus(resp.Status) {
		return s.settleVideoTaskSuccess(ctx, apiKey, appCtx, selection, billingOrganizationID, ownerID, req, resp)
	}
	if isFailedVideoStatus(resp.Status) {
		return s.settleVideoTaskFailure(ctx, apiKey, appCtx, selection, billingOrganizationID, ownerID, req, resp)
	}
	if quote, err := s.videoPredeductQuoteFromTask(ctx, selection, req); err == nil && quote.TotalCredits > 0 {
		resp.EstimatedCredits = videoTaskEstimatedCredits(req, quote.TotalCredits)
	}
	return nil
}

func (s *llmGatewayServiceImpl) settleVideoTaskSuccess(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	req *adapter.VideoTaskRequest,
	resp *adapter.VideoResponse,
) error {
	if resp == nil || !isSuccessfulVideoStatus(resp.Status) {
		return nil
	}
	if points, ok, err := s.findSettledVideoUsageBill(ctx, appCtx); err != nil {
		return err
	} else if ok {
		resp.EstimatedCredits = points
		resp.ActualCredits = points
		return nil
	}

	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return fmt.Errorf("failed to resolve billing lane for video settlement: %w", err)
	}
	quote, err := s.quoteVideoPricingForSelection(ctx, selection, req, resp)
	if err != nil {
		return err
	}
	// Platform lane settles from Console authority (resp.Settlement), so a zero
	// local quote is expected; only the private lane must fail closed here.
	if videoSettlementRequiresLocalPrice(lane) && quote.TotalCredits <= 0 {
		return fmt.Errorf("%w: video pricing is not configured", ErrPricingNotConfigured)
	}
	predeductQuote, err := s.videoPredeductQuoteFromTask(ctx, selection, req)
	if err != nil {
		return err
	}

	billingCtx, err := s.videoBillingContextFromTask(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		req,
		videoTaskEstimatedCredits(req, predeductQuote.TotalCredits),
	)
	if err != nil {
		return err
	}
	billingCtx.PricingOperation = PricingOperationVideo
	if !selection.UseSystemProvider {
		// Video usage is a separate metered dimension. Keep it in total_tokens
		// for the local bill, without fabricating prompt/completion tokens.
		videoTokens := videoResponseTokenCount(resp)
		if videoTokens > 0 {
			billingCtx.PromptTokens = 0
			billingCtx.CompletionTokens = 0
			billingCtx.TotalTokens = videoTokens
		}
	}
	billingCtx.ActualCredits = quote.TotalCredits
	billingCtx.Status = billingContextStatusSuccess
	applyPricingQuoteToBillingContext(billingCtx, quote)
	finalizeVideoTimingSnapshot(billingCtx, resp.Status)
	if selection.UseSystemProvider && resp.Settlement != nil {
		billingCtx.DeductionID = resp.Settlement.SettlementID
		decision, err := s.resolveBillingDecision(selection, billingCtx)
		if err != nil {
			return err
		}
		settlementCtx, cancel := billingFinalizationContext(ctx)
		defer cancel()
		if err := s.finalizePlatformProxySettlement(settlementCtx, billingCtx, resp.Settlement, decision); err != nil {
			return err
		}
		resp.EstimatedCredits = billingCtx.EstimatedCredits
		resp.ActualCredits = billingCtx.ActualCredits
		return nil
	}

	if err := s.settleVideoBillingContext(ctx, selection, billingCtx); err != nil {
		return err
	}

	resp.EstimatedCredits = billingCtx.EstimatedCredits
	resp.ActualCredits = billingCtx.ActualCredits
	return nil
}

func videoResponseTokenCount(resp *adapter.VideoResponse) int {
	if resp == nil {
		return 0
	}
	if resp.Usage != nil {
		if resp.Usage.TotalTokens > 0 {
			return resp.Usage.TotalTokens
		}
		if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
			return resp.Usage.PromptTokens + resp.Usage.CompletionTokens
		}
	}
	for _, key := range []string{"video_tokens", "total_video_tokens", "output_video_tokens"} {
		if value := intFromAny(resp.Raw[key]); value > 0 {
			return value
		}
	}
	return 0
}

func (s *llmGatewayServiceImpl) settleVideoTaskFailure(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	req *adapter.VideoTaskRequest,
	resp *adapter.VideoResponse,
) error {
	if resp == nil || !isFailedVideoStatus(resp.Status) {
		return nil
	}
	predeductQuote, err := s.videoPredeductQuoteFromTask(ctx, selection, req)
	if err != nil {
		return err
	}
	billingCtx, err := s.videoBillingContextFromTask(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		req,
		videoTaskEstimatedCredits(req, predeductQuote.TotalCredits),
	)
	if err != nil {
		return err
	}
	billingCtx.PricingOperation = PricingOperationVideo
	billingCtx.ActualCredits = 0
	billingCtx.Status = billingContextStatusError
	billingCtx.InputUSD = decimal.Zero
	billingCtx.OutputUSD = decimal.Zero
	billingCtx.TotalUSD = decimal.Zero
	billingCtx.InputCost = decimal.Zero
	billingCtx.OutputCost = decimal.Zero
	billingCtx.TotalCost = decimal.Zero
	if resp.Error != nil {
		billingCtx.ErrorMessage = fmt.Sprint(resp.Error)
	}
	finalizeVideoTimingSnapshot(billingCtx, resp.Status)

	if err := s.settleVideoBillingContext(ctx, selection, billingCtx); err != nil {
		return err
	}
	resp.EstimatedCredits = billingCtx.EstimatedCredits
	resp.ActualCredits = 0
	return nil
}

func (s *llmGatewayServiceImpl) videoBillingContextFromTask(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	appCtx *AppContext,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	ownerID uuid.UUID,
	req *adapter.VideoTaskRequest,
	estimatedCredits int64,
) (*BillingContext, error) {
	requestID, attemptID := videoBillingIDsFromTask(appCtx, req)
	if requestID == "" || attemptID == "" {
		return nil, fmt.Errorf("missing video billing attempt metadata")
	}
	channelID := getChannelID(selection)
	billingCtx := s.createBillingContext(
		apiKey,
		appCtx,
		selection,
		channelID,
		billingOrganizationID,
		estimatedCredits,
		false,
		time.Now(),
		requestID,
		attemptID,
	)
	billingCtx.InvocationSource = resolveInvocationSource(ctx, appCtx)
	billingCtx.RoutingStrategy = selection.RoutingStrategy
	if s != nil && s.db != nil {
		var attempt BillingAttempt
		if err := s.db.WithContext(ctx).
			Select("created_at").
			Where("attempt_id = ?", attemptID).
			Take(&attempt).Error; err == nil && !attempt.CreatedAt.IsZero() {
			billingCtx.RequestCreatedAt = attempt.CreatedAt.UTC()
		}
	}
	_ = ownerID
	return billingCtx, nil
}

func finalizeVideoTimingSnapshot(billingCtx *BillingContext, providerStatus string) {
	if billingCtx == nil {
		return
	}
	finishedAt := time.Now().UTC()
	if billingCtx.RequestCreatedAt.IsZero() || billingCtx.RequestCreatedAt.After(finishedAt) {
		billingCtx.RequestCreatedAt = finishedAt
	}
	billingCtx.ResponseTime = finishedAt.Sub(billingCtx.RequestCreatedAt).Milliseconds()
	values := map[string]interface{}{}
	if len(billingCtx.PricingSnapshot) > 0 && string(billingCtx.PricingSnapshot) != "null" {
		_ = json.Unmarshal(billingCtx.PricingSnapshot, &values)
	}
	values["video_task_submitted_at"] = billingCtx.RequestCreatedAt.Format(time.RFC3339Nano)
	values["video_task_finalized_at"] = finishedAt.Format(time.RFC3339Nano)
	values["video_end_to_end_ms"] = billingCtx.ResponseTime
	values["video_provider_status"] = strings.TrimSpace(providerStatus)
	values["video_queue_time"] = "provider_not_reported"
	values["video_generation_time"] = "provider_not_reported"
	billingCtx.PricingSnapshot = buildPricingSnapshot(values)
}

func (s *llmGatewayServiceImpl) settleVideoBillingContext(ctx context.Context, selection *ProviderSelection, billingCtx *BillingContext) error {
	decision, laneErr := s.resolveBillingDecision(selection, billingCtx)
	if laneErr != nil {
		return wrapBillingLaneMismatchError(laneErr)
	}
	if decision.UseSystemProvider {
		if err := s.attachRemoteDeductionID(ctx, billingCtx); err != nil {
			return err
		}
	}
	billingCtxForSettle, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if err := s.billingProviderForDecision(decision).Settle(billingCtxForSettle, billingCtx); err != nil {
		return wrapBillingSettleError(err, billingCtx, decision.UseSystemProvider, decision.RouteID)
	}
	return nil
}

func (s *llmGatewayServiceImpl) attachRemoteDeductionID(ctx context.Context, billingCtx *BillingContext) error {
	if billingCtx == nil || strings.TrimSpace(billingCtx.DeductionID) != "" {
		return nil
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("missing db for remote deduction lookup")
	}
	var entry BillingAttemptEntry
	err := s.db.WithContext(ctx).
		Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", billingCtx.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).
		Take(&entry).Error
	if err != nil {
		return err
	}
	if entry.IdempotencyKey == nil || strings.TrimSpace(*entry.IdempotencyKey) == "" {
		return fmt.Errorf("missing remote deduction id for video billing attempt %s", billingCtx.AttemptID)
	}
	billingCtx.DeductionID = strings.TrimSpace(*entry.IdempotencyKey)
	return nil
}
func (s *llmGatewayServiceImpl) findSettledVideoUsageBill(ctx context.Context, appCtx *AppContext) (int64, bool, error) {
	if s == nil || s.db == nil || appCtx == nil || appCtx.AppID == nil || appCtx.AppType == nil {
		return 0, false, nil
	}
	appType := strings.TrimSpace(*appCtx.AppType)
	if appType == "" {
		return 0, false, nil
	}
	var row struct {
		TotalPoints millipoints.MilliPoints `gorm:"column:total_points"`
	}
	err := s.db.WithContext(ctx).
		Table("llm_usage_bills").
		Select("total_points").
		Where("app_id = ? AND app_type = ? AND status IN ?", *appCtx.AppID, appType, []string{usageBillStatusSuccess, usageBillStatusPartial}).
		Order("settled_at DESC").
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return row.TotalPoints.Int64(), true, nil
}

// videoSettlementRequiresLocalPrice reports whether a billing lane settles from
// local model pricing. Platform-lane video settles from Console authority, so a
// zero local quote is expected there and must not fail closed.
func videoSettlementRequiresLocalPrice(lane UsageBillingLane) bool {
	return lane != UsageBillingLanePlatform
}

func (s *llmGatewayServiceImpl) quoteVideoPricingForSelection(ctx context.Context, selection *ProviderSelection, req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) (PricingQuote, error) {
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("failed to resolve billing lane for video pricing: %w", err)
	}
	if lane == UsageBillingLanePlatform {
		return PricingQuote{}, nil
	}
	if selection == nil || resp == nil {
		return PricingQuote{}, nil
	}
	rate, resolution, inputVideo, pricingMode, ok, err := videoPriceForSelection(selection, req, resp)
	if err != nil {
		return PricingQuote{}, err
	}
	if !ok || !rate.IsPositive() {
		return PricingQuote{}, nil
	}
	priceUSD := rate
	currency := strings.ToUpper(strings.TrimSpace(selection.Model.Currency))
	if currency == "CNY" {
		// CNY settlement needs the organization rate. A missing rate is
		// fail-closed; it never falls back to a hard-coded 7.
		rateUSDToCNY, rateErr := s.organizationUSDToCNYRate(ctx, selection.OrganizationID)
		if rateErr != nil {
			return PricingQuote{}, fmt.Errorf("%w: video CNY settlement requires an exchange rate: %v", ErrExchangeRateUnavailable, rateErr)
		}
		priceUSD = rate.Div(rateUSDToCNY)
	}
	totalUSD := decimal.Zero
	usageSource := UsageSourceRequestParameters
	videoTokens := videoPricingTokenCount(req, resp)
	// The requested duration is only a reservation estimate. Settlement must
	// never fall back to it: the upstream-reported actual duration is the only
	// acceptable input for by-second pricing.
	actualDurationSeconds := resp.VideoActualDurationSeconds()
	durationSeconds := intFromAny(videoTaskAdditionalParam(req, "duration"))
	if durationSeconds <= 0 {
		durationSeconds = defaultVideoPredeductDurationSeconds
	}
	var snapshotDuration interface{} = durationSeconds
	if actualDurationSeconds > 0 {
		snapshotDuration = actualDurationSeconds
	}
	count := intFromAny(videoTaskAdditionalParam(req, "count"))
	if count <= 0 {
		count = 1
	}
	switch pricingMode {
	case "million_video_tokens":
		if videoTokens <= 0 {
			return PricingQuote{}, fmt.Errorf("%w: video token usage is required for million_video_tokens video settlement", ErrPricingNotConfigured)
		}
		totalUSD = tokenUSD(priceUSD, videoTokens)
		usageSource = UsageSourceProviderUsage
	case "second":
		if actualDurationSeconds <= 0 {
			// Fail closed without settling: the generated video exists but the
			// upstream did not report how long it is, so there is no trustworthy
			// amount to charge. The reservation stays for reconciliation.
			return PricingQuote{}, fmt.Errorf("%w: upstream video duration is unavailable for by-second settlement", ErrBillingSettleFailed)
		}
		// The measured duration is not money. It is converted to exact decimal
		// once here so the rate multiplication stays exact.
		totalUSD = priceUSD.Mul(decimal.NewFromFloat(actualDurationSeconds)).Mul(decimal.NewFromInt(int64(count)))
	case "task":
		// Task-priced video is billed per task. The same count drives
		// pre-deduct and settlement, so the reservation equals the charge.
		totalUSD = priceUSD.Mul(decimal.NewFromInt(int64(count)))
	default:
		return PricingQuote{}, nil
	}
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":   PricingSourceUpstreamModelPrice,
		"usage_source":     usageSource,
		"operation":        PricingOperationVideo,
		"model_id":         selection.Model.ID.String(),
		"provider":         strings.TrimSpace(selection.Model.Provider),
		"model":            strings.TrimSpace(selection.Model.Model),
		"request_model":    strings.TrimSpace(req.Model),
		"resolution":       resolution,
		"input_video":      inputVideo,
		"pricing_mode":     pricingMode,
		"video_tokens":     videoTokens,
		"duration_seconds": snapshotDuration,
		"count":            count,
		"source_currency":  currency,
	})
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, "", usageSource, snapshot), nil
}

func videoPriceForSelection(selection *ProviderSelection, req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) (decimal.Decimal, string, bool, string, bool, error) {
	if selection == nil || len(selection.Model.Pricing) == 0 || string(selection.Model.Pricing) == "null" {
		return decimal.Zero, "", false, "", false, nil
	}
	seconds := intFromAny(videoTaskAdditionalParam(req, "duration"))
	if actual := resp.VideoActualDurationSeconds(); actual > 0 {
		// Rate conditions may pin a duration; match against the upstream actual
		// duration rather than the requested estimate.
		seconds = int(actual)
	}
	if seconds <= 0 {
		seconds = defaultVideoPredeductDurationSeconds
	}
	count := intFromAny(videoTaskAdditionalParam(req, "count"))
	if count <= 0 {
		count = 1
	}
	costReq := videoGenerationCostRequestFromTask(req, resp, seconds, count)
	cost, present, err := modelpricing.ResolveVideoGenerationUnitPrice(json.RawMessage(selection.Model.Pricing), costReq)
	if err != nil {
		return decimal.Zero, "", false, "", false, fmt.Errorf("invalid video pricing: %w", err)
	}
	if !present {
		return decimal.Zero, costReq.Resolution, costReq.InputVideo, "", false, nil
	}
	mode := cost.BillingUnit
	var rate decimal.Decimal
	// Explicit boundary: the unit price comes from legacy JSON as a float and
	// is converted once per settlement mode; rate math below stays decimal.
	switch mode {
	case "second":
		rate = decimal.NewFromFloat(cost.UnitPrice)
	case "million_video_tokens":
		rate = decimal.NewFromFloat(cost.UnitPrice)
	case "task":
		rate = decimal.NewFromFloat(cost.UnitPrice)
	default:
		return decimal.Zero, cost.Resolution, cost.InputVideo, "", false, nil
	}
	if !rate.IsPositive() {
		return decimal.Zero, cost.Resolution, cost.InputVideo, mode, false, nil
	}
	return rate, cost.Resolution, cost.InputVideo, mode, true, nil
}

func videoPricingTokenCount(req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) int {
	if resp != nil && resp.Usage != nil {
		if resp.Usage.CompletionTokens > 0 {
			return resp.Usage.CompletionTokens
		}
		if resp.Usage.TotalTokens > 0 {
			return resp.Usage.TotalTokens
		}
	}
	for _, key := range []string{"video_tokens", "total_video_tokens", "output_video_tokens", "completion_tokens", "total_tokens"} {
		if value := intFromAny(videoTaskAdditionalParam(req, key)); value > 0 {
			return value
		}
	}
	if resp != nil && resp.Raw != nil {
		for _, key := range []string{"video_tokens", "total_video_tokens", "output_video_tokens", "completion_tokens", "total_tokens"} {
			if value := intFromAny(resp.Raw[key]); value > 0 {
				return value
			}
		}
		if usage, ok := resp.Raw["usage"].(map[string]interface{}); ok {
			for _, key := range []string{"video_tokens", "total_video_tokens", "output_video_tokens", "completion_tokens", "total_tokens"} {
				if value := intFromAny(usage[key]); value > 0 {
					return value
				}
			}
		}
	}
	return 0
}

func videoPricingResolution(req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) string {
	if req != nil && req.AdditionalParameters != nil {
		if value := stringFromAny(req.AdditionalParameters["resolution"]); value != "" {
			return strings.ToLower(value)
		}
	}
	if resp != nil && resp.Raw != nil {
		if value := stringFromAny(resp.Raw["resolution"]); value != "" {
			return strings.ToLower(value)
		}
	}
	return ""
}

func videoPricingInputVideo(req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) bool {
	if req != nil && req.AdditionalParameters != nil {
		if value, ok := boolFromAny(req.AdditionalParameters["input_video"]); ok {
			return value
		}
	}
	if resp != nil && resp.Raw != nil {
		if value, ok := boolFromAny(resp.Raw["input_video"]); ok {
			return value
		}
	}
	return false
}

func (s *llmGatewayServiceImpl) organizationUSDToCNYRate(ctx context.Context, organizationID uuid.UUID) (decimal.Decimal, error) {
	if s == nil {
		return decimal.Zero, fmt.Errorf("%w: billing service is nil", ErrExchangeRateUnavailable)
	}
	return loadOrganizationUSDToCNYRate(ctx, s.db, organizationID)
}

func isSuccessfulVideoStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed", "done":
		return true
	default:
		return false
	}
}

func stringFromAny(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func boolFromAny(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		}
	}
	return false, false
}
