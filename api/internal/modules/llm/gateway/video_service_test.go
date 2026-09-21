package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVideoBillingContextFromTaskPreservesProductAttribution(t *testing.T) {
	appID := uuid.New()
	appType := "video-runtime"
	apiKey := &apikeymodel.TenantAPIKey{ID: uuid.NewString()}
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{ID: uuid.New(), Model: "video-test"},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{
		"billing_request_id": "video-request-1",
		"billing_attempt_id": "video-request-1:0",
	}}

	billingCtx, err := (&llmGatewayServiceImpl{}).videoBillingContextFromTask(
		context.Background(),
		apiKey,
		&AppContext{AppID: &appID, AppType: &appType},
		selection,
		uuid.New(),
		uuid.New(),
		req,
		1000,
	)
	if err != nil {
		t.Fatalf("videoBillingContextFromTask() error = %v", err)
	}
	if billingCtx.InvocationSource != InvocationSourceProduct {
		t.Fatalf("invocation source = %q, want %q", billingCtx.InvocationSource, InvocationSourceProduct)
	}
	if billingCtx.AppType == nil || *billingCtx.AppType != appType {
		t.Fatalf("app type = %#v, want %q", billingCtx.AppType, appType)
	}
	if billingCtx.AppID == nil || *billingCtx.AppID != appID {
		t.Fatalf("app id = %#v, want %s", billingCtx.AppID, appID)
	}
}

func TestVideoResponseTokenCountKeepsVideoTokensInTotalOnly(t *testing.T) {
	resp := &adapter.VideoResponse{Usage: &adapter.Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 321}}
	if got := videoResponseTokenCount(resp); got != 321 {
		t.Fatalf("video token count = %d, want 321", got)
	}
	bc := &BillingContext{}
	bc.TotalTokens = videoResponseTokenCount(resp)
	if bc.TotalTokens != 321 || bc.PromptTokens != 0 || bc.CompletionTokens != 0 {
		t.Fatalf("video billing buckets = %#v", bc)
	}
}

func TestVideoBillingContextRestoresAttemptStartAndRecordsEndToEndTiming(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE billing_attempts (attempt_id TEXT PRIMARY KEY, created_at DATETIME NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	if err := db.Exec(`INSERT INTO billing_attempts (attempt_id, created_at) VALUES (?, ?)`, "video-request-2:0", startedAt).Error; err != nil {
		t.Fatal(err)
	}
	selection := &ProviderSelection{Model: llmmodel.LLMModel{ID: uuid.New(), Model: "video-test"}}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{
		"billing_request_id": "video-request-2",
		"billing_attempt_id": "video-request-2:0",
	}}
	billingCtx, err := (&llmGatewayServiceImpl{db: db}).videoBillingContextFromTask(
		context.Background(), &apikeymodel.TenantAPIKey{ID: uuid.NewString()}, nil, selection,
		uuid.New(), uuid.New(), req, 1000,
	)
	if err != nil {
		t.Fatal(err)
	}
	finalizeVideoTimingSnapshot(billingCtx, "succeeded")
	if billingCtx.ResponseTime < 1500 {
		t.Fatalf("response time = %dms, want restored end-to-end duration", billingCtx.ResponseTime)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(billingCtx.PricingSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["video_queue_time"] != "provider_not_reported" || snapshot["video_generation_time"] != "provider_not_reported" {
		t.Fatalf("timing snapshot = %#v", snapshot)
	}
}

func TestVideoPriceForSelectionMatchesFlatTokenRates(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "doubao",
			Model:    "doubao-seedance-2-0-mini-260615",
			Currency: "CNY",
			Pricing: datatypes.JSON([]byte(`{
				"video_generation": {
					"currency": "CNY",
					"billing_unit": "million_video_tokens",
					"rates": [
						{"resolution":"480p","input_video":true,"price_per_million_tokens":14},
						{"resolution":"480p","input_video":false,"price_per_million_tokens":23}
					]
				}
			}`)),
		},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{
		"resolution":  "480p",
		"input_video": true,
	}}

	rate, resolution, inputVideo, mode, ok, err := videoPriceForSelection(selection, req, &adapter.VideoResponse{})
	if err != nil {
		t.Fatalf("videoPriceForSelection() error = %v", err)
	}
	if !ok || rate.String() != "14" || resolution != "480p" || !inputVideo || mode != "million_video_tokens" {
		t.Fatalf("videoPriceForSelection() = rate=%s resolution=%q inputVideo=%v mode=%q ok=%v", rate, resolution, inputVideo, mode, ok)
	}
}

func TestVideoPriceForSelectionMatchesPerSecondRates(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "example",
			Model:    "video-per-second",
			Currency: "USD",
			Pricing: datatypes.JSON([]byte(`{
				"video_generation": {
					"currency": "USD",
					"billing_unit": "second",
					"rates": [
						{"resolution":"720p","price_per_second":0.02}
					]
				}
			}`)),
		},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{"resolution": "720p"}}

	rate, resolution, _, mode, ok, err := videoPriceForSelection(selection, req, &adapter.VideoResponse{})
	if err != nil {
		t.Fatalf("videoPriceForSelection() error = %v", err)
	}
	if !ok || rate.String() != "0.02" || resolution != "720p" || mode != "second" {
		t.Fatalf("videoPriceForSelection() = rate=%s resolution=%q mode=%q ok=%v", rate, resolution, mode, ok)
	}
}

func TestVideoPricingTokenCountReadsVideoTokenFields(t *testing.T) {
	resp := &adapter.VideoResponse{Raw: map[string]interface{}{
		"usage": map[string]interface{}{
			"completion_tokens": float64(12345),
			"total_tokens":      float64(12345),
		},
	}}
	if got := videoPricingTokenCount(nil, resp); got != 12345 {
		t.Fatalf("videoPricingTokenCount() = %d, want 12345", got)
	}
}

func TestVideoResponseFailureErrorPreservesUpstreamErrorMessage(t *testing.T) {
	resp := &adapter.VideoResponse{
		Status: "failed",
		Raw: map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "InvalidParameter",
				"message": "Error while downloading image, error: expected the width to be at least 300px",
			},
		},
	}

	err := videoResponseFailureError(resp, "upstream video task failed")
	if err == nil {
		t.Fatal("videoResponseFailureError() error = nil")
	}
	const want = "upstream error: Error while downloading image, error: expected the width to be at least 300px"
	if err.Error() != want {
		t.Fatalf("videoResponseFailureError() = %q, want %q", err.Error(), want)
	}
}

func TestVideoCreateResponseRequiresTaskIDEvenWhenHTTP200BodyHasError(t *testing.T) {
	resp := &adapter.VideoResponse{
		Raw: map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Error while downloading image",
			},
		},
	}

	if videoResponseHasTaskID(resp) {
		t.Fatal("videoResponseHasTaskID() = true, want false for response without task id")
	}
	if videoResponseFailureError(resp, "upstream video task id is empty").Error() != "upstream error: Error while downloading image" {
		t.Fatalf("videoResponseFailureError() did not preserve HTTP 200 error body message")
	}
}

func TestVideoCreateResponseMissingTaskIDUsesWrappedUpstreamError(t *testing.T) {
	resp := &adapter.VideoResponse{
		Raw: map[string]interface{}{
			"code":    float64(0),
			"message": "success",
			"data": map[string]interface{}{
				"error": map[string]interface{}{
					"code":    "InvalidParameter",
					"message": "Error while downloading image, error: expected the width to be at least 300px",
				},
			},
		},
	}

	err := videoResponseFailureError(resp, "upstream video task id is empty")
	const want = "upstream error: Error while downloading image, error: expected the width to be at least 300px"
	if err == nil || err.Error() != want {
		t.Fatalf("videoResponseFailureError() = %v, want %q", err, want)
	}
}

func TestDefaultVideoPredeductQuoteUsesModelTokenEstimation(t *testing.T) {
	selection := &ProviderSelection{Model: llmmodel.LLMModel{
		ID:       uuid.New(),
		Provider: "qwen",
		Model:    "wan-video",
		Pricing:  datatypes.JSON(`{"video_generation":{"currency":"USD","billing_unit":"million_video_tokens","rates":[{"price_per_million_tokens":10}],"token_estimation":{"unit":"video_tokens","rules":[{"tokens_per_second":1000}]}}}`),
	}}
	costReq := modelpricing.VideoGenerationCostRequest{
		DurationSeconds: 5,
		Count:           2,
	}
	quote, err := quoteVideoFromModelPricing(selection, "wan-video", costReq, decimal.NewFromInt(7))
	if err != nil {
		t.Fatalf("quoteVideoFromModelPricing() error = %v", err)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want model price", quote.PricingSource)
	}
	// 5s * 2 * 1000 tokens/s at $10 per million = $0.10.
	if quote.TotalCredits != 100000 {
		t.Fatalf("total credits = %d, want 100000", quote.TotalCredits)
	}
}

func TestVideoPredeductQuoteFromTaskUsesPlatformRemoteAuthority(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "official-video"},
	}
	quote, err := svc.videoPredeductQuoteFromTask(context.Background(), selection, &adapter.VideoTaskRequest{Model: "official-video"})
	if err != nil {
		t.Fatalf("videoPredeductQuoteFromTask() error = %v", err)
	}
	if quote.TotalCredits != 0 || quote.PricingSource != PricingSourcePlatformRemoteAuthority {
		t.Fatalf("quote = %#v, want zero local quote with platform_remote_authority", quote)
	}
}

func TestVideoPredeductPlatformLaneQuotePreservesProxyMetadata(t *testing.T) {
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "official-video"},
	}
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		t.Fatalf("usageBillingLaneFromContext() error = %v", err)
	}
	if lane != UsageBillingLanePlatform {
		t.Fatalf("lane = %q, want platform", lane)
	}
	quote := PricingQuote{
		PricingSource: PricingSourcePlatformRemoteAuthority,
		UsageSource:   UsageSourceRequestParameters,
	}
	billingCtx := &BillingContext{
		OrganizationID: uuid.New().String(),
		RequestID:      "video:test-request",
		AttemptID:      "video:test-request:0",
		PricingSource:  quote.PricingSource,
		UsageSource:    quote.UsageSource,
		BillingLane:    UsageBillingLanePlatform,
	}
	callCtx := withPlatformProxyMetadata(context.Background(), billingCtx)
	meta, ok := callCtx.Value(platformProxyContextKey{}).(platformProxyMetadata)
	if !ok || strings.TrimSpace(meta.AttemptID) == "" || strings.TrimSpace(meta.RequestID) == "" {
		t.Fatalf("platform proxy metadata missing from context: %#v ok=%v", meta, ok)
	}
}

func TestVideoPriceForSelectionMatchesAudioCondition(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "example",
			Model:    "video-audio",
			Currency: "USD",
			Pricing: datatypes.JSON([]byte(`{
				"version":2,
				"video_generation":{
					"currency":"USD",
					"billing_unit":"second",
					"rates":[
						{"resolution":"720p","audio":false,"price_per_second":0.01},
						{"resolution":"720p","audio":true,"price_per_second":0.03}
					]
				},
				"rules":[
					{"id":"video-720p-no-audio","operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.01","configured":true,"conditions":{"resolution":"720p","audio":"false"}},
					{"id":"video-720p-audio","operation":"video_generation","meter":"output_video_second","unit":"second","price":"0.03","configured":true,"conditions":{"resolution":"720p","audio":"true"}}
				]
			}`)),
		},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{
		"resolution": "720p",
		"audio":      true,
	}}
	rate, resolution, _, mode, ok, err := videoPriceForSelection(selection, req, &adapter.VideoResponse{})
	if err != nil {
		t.Fatalf("videoPriceForSelection() error = %v", err)
	}
	if !ok || rate.String() != "0.03" || resolution != "720p" || mode != "second" {
		t.Fatalf("videoPriceForSelection() = rate=%s resolution=%q mode=%q ok=%v", rate, resolution, mode, ok)
	}
}
