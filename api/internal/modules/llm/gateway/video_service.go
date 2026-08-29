package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/gorm"
)

type videoCapableAdapter interface {
	CreateVideo(ctx context.Context, request *adapter.VideoRequest) (*adapter.VideoResponse, error)
	GetVideoTask(ctx context.Context, request *adapter.VideoTaskRequest) (*adapter.VideoResponse, error)
}

const (
	defaultVideoPredeductPointsPerSecond int64 = 143
	internalCreditsPerPoint              int64 = 1000
	defaultVideoPredeductDurationSeconds       = 5
)

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
	quote := defaultVideoPredeductQuoteFromCreate(selection, req)
	if quote.TotalCredits <= 0 {
		return nil, nil
	}
	requestID, attemptID := videoPredeductIDs(appCtx, attemptIdx)
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

func defaultVideoPredeductQuoteFromCreate(selection *ProviderSelection, req *adapter.VideoRequest) PricingQuote {
	seconds := defaultVideoPredeductDurationSeconds
	if req != nil && req.Duration != nil && *req.Duration > 0 {
		seconds = *req.Duration
	}
	count := 1
	if req != nil && req.N != nil && *req.N > 0 {
		count = *req.N
	}
	return defaultVideoPredeductQuote(selection, strings.TrimSpace(reqModelName(req)), seconds, count)
}

func defaultVideoPredeductQuoteFromTask(selection *ProviderSelection, req *adapter.VideoTaskRequest) PricingQuote {
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
	return defaultVideoPredeductQuote(selection, modelName, seconds, count)
}

func defaultVideoPredeductQuote(selection *ProviderSelection, requestModel string, seconds int, count int) PricingQuote {
	if seconds <= 0 {
		seconds = defaultVideoPredeductDurationSeconds
	}
	if count <= 0 {
		count = 1
	}
	totalCredits := int64(seconds) * int64(count) * defaultVideoPredeductPointsPerSecond * internalCreditsPerPoint
	totalUSD := decimal.NewFromInt(totalCredits).Div(decimal.NewFromInt(pricingCreditsPerUSD))
	provider := ""
	modelName := strings.TrimSpace(requestModel)
	modelID := ""
	if selection != nil {
		provider = strings.TrimSpace(selection.Model.Provider)
		if strings.TrimSpace(selection.Model.Model) != "" {
			modelName = strings.TrimSpace(selection.Model.Model)
		}
		if selection.Model.ID != uuid.Nil {
			modelID = selection.Model.ID.String()
		}
	}
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":             PricingSourceCodeDefaultFallback,
		"usage_source":               UsageSourceRequestParameters,
		"operation":                  PricingOperationVideo,
		"model_id":                   modelID,
		"provider":                   provider,
		"model":                      modelName,
		"request_model":              strings.TrimSpace(requestModel),
		"duration_seconds":           seconds,
		"count":                      count,
		"points_per_second":          defaultVideoPredeductPointsPerSecond,
		"internal_credits_per_point": internalCreditsPerPoint,
	})
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceCodeDefaultFallback, "video.default.per_second", UsageSourceRequestParameters, snapshot)
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
	if fallback > 0 {
		return fallback
	}
	return defaultVideoPredeductQuoteFromTask(nil, req).TotalCredits
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
	config := s.createAdapterConfig(selection, organizationID)
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
	config := s.createAdapterConfig(selection, organizationID)
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
	if estimated := videoTaskEstimatedCredits(req, defaultVideoPredeductQuoteFromTask(selection, req).TotalCredits); estimated > 0 {
		resp.EstimatedCredits = estimated
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

	quote, err := s.quoteVideoPricingForSelection(ctx, selection, req, resp)
	if err != nil {
		return err
	}
	fallbackQuote := defaultVideoPredeductQuoteFromTask(selection, req)
	if quote.TotalCredits <= 0 {
		quote = fallbackQuote
	}
	if quote.TotalCredits <= 0 {
		return nil
	}

	billingCtx, err := s.videoBillingContextFromTask(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		req,
		videoTaskEstimatedCredits(req, fallbackQuote.TotalCredits),
	)
	if err != nil {
		return err
	}
	billingCtx.PricingOperation = PricingOperationVideo
	if resp.Usage != nil {
		billingCtx.PromptTokens = resp.Usage.PromptTokens
		billingCtx.CompletionTokens = resp.Usage.CompletionTokens
		billingCtx.TotalTokens = resp.Usage.TotalTokens
	}
	billingCtx.ActualCredits = quote.TotalCredits
	billingCtx.Status = billingContextStatusSuccess
	billingCtx.ResponseTime = 0
	applyPricingQuoteToBillingContext(billingCtx, quote)

	if err := s.settleVideoBillingContext(ctx, selection, billingCtx); err != nil {
		return err
	}

	resp.EstimatedCredits = billingCtx.EstimatedCredits
	resp.ActualCredits = billingCtx.ActualCredits
	return nil
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
	quote := defaultVideoPredeductQuoteFromTask(selection, req)
	billingCtx, err := s.videoBillingContextFromTask(
		ctx,
		apiKey,
		appCtx,
		selection,
		billingOrganizationID,
		ownerID,
		req,
		videoTaskEstimatedCredits(req, quote.TotalCredits),
	)
	if err != nil {
		return err
	}
	billingCtx.PricingOperation = PricingOperationVideo
	billingCtx.ActualCredits = 0
	billingCtx.Status = billingContextStatusError
	billingCtx.ResponseTime = 0
	billingCtx.InputUSD = decimal.Zero
	billingCtx.OutputUSD = decimal.Zero
	billingCtx.TotalUSD = decimal.Zero
	billingCtx.InputCost = decimal.Zero
	billingCtx.OutputCost = decimal.Zero
	billingCtx.TotalCost = decimal.Zero
	if resp.Error != nil {
		billingCtx.ErrorMessage = fmt.Sprint(resp.Error)
	}

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
	_ = ownerID
	return billingCtx, nil
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
		TotalPoints int64 `gorm:"column:total_points"`
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
	return row.TotalPoints, true, nil
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
		rateUSDToCNY := s.organizationUSDToCNYRate(ctx, selection.OrganizationID)
		priceUSD = rate.Div(rateUSDToCNY)
	}
	totalUSD := decimal.Zero
	usageSource := UsageSourceRequestParameters
	videoTokens := videoPricingTokenCount(req, resp)
	durationSeconds := intFromAny(videoTaskAdditionalParam(req, "duration"))
	if durationSeconds <= 0 {
		durationSeconds = defaultVideoPredeductDurationSeconds
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
		totalUSD = priceUSD.Mul(decimal.NewFromInt(int64(durationSeconds))).Mul(decimal.NewFromInt(int64(count)))
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
		"duration_seconds": durationSeconds,
		"count":            count,
		"source_currency":  currency,
	})
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, "", usageSource, snapshot), nil
}

type videoPricingRate struct {
	Resolution               string          `json:"resolution"`
	InputVideo               *bool           `json:"input_video"`
	PricePerMillionTokens    decimal.Decimal `json:"price_per_million_tokens"`
	PricePerMillionVideo     decimal.Decimal `json:"price_per_million_video_tokens"`
	PricePerSecond           decimal.Decimal `json:"price_per_second"`
	PriceUSDPerSecond        decimal.Decimal `json:"price_usd_per_second"`
	TokensPerSecond          decimal.Decimal `json:"tokens_per_second"`
	VideoTokensPerSecond     decimal.Decimal `json:"video_tokens_per_second"`
	VideoTokens              decimal.Decimal `json:"video_tokens"`
	MillionVideoTokensPerSec decimal.Decimal `json:"million_video_tokens_per_second"`
}

func videoPriceForSelection(selection *ProviderSelection, req *adapter.VideoTaskRequest, resp *adapter.VideoResponse) (decimal.Decimal, string, bool, string, bool, error) {
	if selection == nil || len(selection.Model.Pricing) == 0 || string(selection.Model.Pricing) == "null" {
		return decimal.Zero, "", false, "", false, nil
	}
	var pricing struct {
		VideoGeneration struct {
			BillingUnit     string             `json:"billing_unit"`
			Rates           []videoPricingRate `json:"rates"`
			ResolutionRates []struct {
				Resolution string             `json:"resolution"`
				Rates      []videoPricingRate `json:"rates"`
			} `json:"resolution_rates"`
		} `json:"video_generation"`
	}
	if err := json.Unmarshal(selection.Model.Pricing, &pricing); err != nil {
		return decimal.Zero, "", false, "", false, fmt.Errorf("invalid video pricing: %w", err)
	}
	resolution := videoPricingResolution(req, resp)
	inputVideo := videoPricingInputVideo(req, resp)
	if rate, matchedResolution, mode, ok := matchVideoRate(pricing.VideoGeneration.Rates, resolution, inputVideo, pricing.VideoGeneration.BillingUnit); ok {
		return rate, matchedResolution, inputVideo, mode, true, nil
	}
	for _, resolutionRate := range pricing.VideoGeneration.ResolutionRates {
		if resolution != "" && !strings.EqualFold(strings.TrimSpace(resolutionRate.Resolution), resolution) {
			continue
		}
		if rate, matchedResolution, mode, ok := matchVideoRate(resolutionRate.Rates, resolution, inputVideo, pricing.VideoGeneration.BillingUnit); ok {
			if matchedResolution == "" {
				matchedResolution = strings.TrimSpace(resolutionRate.Resolution)
			}
			return rate, matchedResolution, inputVideo, mode, true, nil
		}
	}
	if resolution != "" {
		return decimal.Zero, resolution, inputVideo, "", false, nil
	}
	for _, resolutionRate := range pricing.VideoGeneration.ResolutionRates {
		if rate, matchedResolution, mode, ok := matchVideoRate(resolutionRate.Rates, "", inputVideo, pricing.VideoGeneration.BillingUnit); ok {
			if matchedResolution == "" {
				matchedResolution = strings.TrimSpace(resolutionRate.Resolution)
			}
			return rate, matchedResolution, inputVideo, mode, true, nil
		}
	}
	return decimal.Zero, resolution, inputVideo, "", false, nil
}

func matchVideoRate(rates []videoPricingRate, resolution string, inputVideo bool, billingUnit string) (decimal.Decimal, string, string, bool) {
	for _, rate := range rates {
		if resolution != "" && strings.TrimSpace(rate.Resolution) != "" && !strings.EqualFold(strings.TrimSpace(rate.Resolution), resolution) {
			continue
		}
		if rate.InputVideo != nil && *rate.InputVideo != inputVideo {
			continue
		}
		matchedResolution := strings.TrimSpace(rate.Resolution)
		if matchedResolution == "" {
			matchedResolution = resolution
		}
		if rate.PricePerMillionTokens.IsPositive() {
			return rate.PricePerMillionTokens, matchedResolution, "million_video_tokens", true
		}
		if rate.PricePerMillionVideo.IsPositive() {
			return rate.PricePerMillionVideo, matchedResolution, "million_video_tokens", true
		}
		if rate.PricePerSecond.IsPositive() {
			return rate.PricePerSecond, matchedResolution, "second", true
		}
		if rate.PriceUSDPerSecond.IsPositive() {
			return rate.PriceUSDPerSecond, matchedResolution, "second", true
		}
	}
	if strings.EqualFold(strings.TrimSpace(billingUnit), "second") {
		return decimal.Zero, resolution, "second", false
	}
	return decimal.Zero, resolution, "", false
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

func (s *llmGatewayServiceImpl) organizationUSDToCNYRate(ctx context.Context, organizationID uuid.UUID) decimal.Decimal {
	if s == nil {
		return decimal.NewFromInt(7)
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
