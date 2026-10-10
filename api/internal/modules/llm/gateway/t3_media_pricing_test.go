package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// T3 acceptance scenario 3: realtime-audio prefers adapter audio_tokens, falls
// back to prompt/completion when they are missing, and only fails closed when
// no realtime_audio price exists at all.
const t3RealtimePricing = `{"deployment_scope":"global","rules":[{"operation":"realtime_audio","meter":"input_token","unit":"per_1m_tokens","price":"32","configured":true},{"operation":"realtime_audio","meter":"output_token","unit":"per_1m_tokens","price":"64","configured":true}]}`

func insertT3ModelPricing(t *testing.T, db *gorm.DB, modelID uuid.UUID, name, pricing string, inputPrice, outputPrice string, inputConfigured, outputConfigured bool) {
	t.Helper()
	insertPricingModelNamed(t, db, modelID, "openai", name, inputPrice, outputPrice, inputConfigured, outputConfigured, "[]")
	if err := db.Exec(`UPDATE llm_models SET pricing = ? WHERE id = ?`, pricing, modelID.String()).Error; err != nil {
		t.Fatalf("update pricing: %v", err)
	}
}

func TestT3QuoteTokenUsageRealtimePrefersAudioTokens(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertT3ModelPricing(t, db, modelID, "gpt-realtime", t3RealtimePricing, "0", "0", false, false)

	engine := NewPricingEngine(db).(cacheTokenPricingEngine)
	quote, err := engine.QuoteTokenUsage(context.Background(), PricingModelRef{
		ModelID:   modelID,
		Source:    PricingModelSourceGlobal,
		Operation: PricingOperationRealtimeAudio,
	}, TokenUsage{
		InputTokens:       5000,
		OutputTokens:      7000,
		AudioInputTokens:  1000,
		AudioOutputTokens: 1000,
	})
	if err != nil {
		t.Fatalf("QuoteTokenUsage error = %v", err)
	}
	// $32/1M audio in + $64/1M audio out over 1k + 1k audio tokens = $0.096.
	if quote.TotalCredits != 96000 {
		t.Fatalf("total credits = %d, want 96000 (audio tokens, not prompt/completion)", quote.TotalCredits)
	}
}

func TestT3QuoteTokenUsageRealtimeFallsBackToPromptCompletion(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertT3ModelPricing(t, db, modelID, "gpt-realtime", t3RealtimePricing, "0", "0", false, false)

	engine := NewPricingEngine(db).(cacheTokenPricingEngine)
	quote, err := engine.QuoteTokenUsage(context.Background(), PricingModelRef{
		ModelID:   modelID,
		Source:    PricingModelSourceGlobal,
		Operation: PricingOperationRealtimeAudio,
	}, TokenUsage{InputTokens: 1000, OutputTokens: 1000})
	if err != nil {
		t.Fatalf("QuoteTokenUsage error = %v, want fallback to prompt/completion", err)
	}
	if quote.TotalCredits != 96000 {
		t.Fatalf("total credits = %d, want 96000 from prompt/completion fallback", quote.TotalCredits)
	}
}

func TestT3QuoteTokenUsageRealtimeFailsClosedWithoutRealtimeRules(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	// Chat scalar prices exist, but there is no realtime_audio rule.
	insertPricingModelNamed(t, db, modelID, "openai", "gpt-realtime", "1", "2", true, true, "[]")

	engine := NewPricingEngine(db).(cacheTokenPricingEngine)
	_, err := engine.QuoteTokenUsage(context.Background(), PricingModelRef{
		ModelID:   modelID,
		Source:    PricingModelSourceGlobal,
		Operation: PricingOperationRealtimeAudio,
	}, TokenUsage{AudioInputTokens: 1000, AudioOutputTokens: 1000})
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured when no realtime price exists", err)
	}
}

// T3 acceptance scenario 1: gpt-image-2 private lane bills the three
// image_token tiers from V006 and never falls back to per_image.
func TestT3QuoteImageTokenUsageThreeTiers(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	pricing := `{"deployment_scope":"global","price_per_image":0.1,"image_token_pricing":{"currency":"USD","text_input_price_per_million":5,"image_input_price_per_million":8,"output_price_per_million":30}}`
	insertT3ModelPricing(t, db, modelID, "gpt-image-2", pricing, "0", "0", false, false)

	engine := NewPricingEngine(db).(imageTokenPricingEngine)
	quote, err := engine.QuoteImageTokenUsage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, ImageTokenUsage{TextInputTokens: 1000, ImageInputTokens: 2000, OutputTokens: 500})
	if err != nil {
		t.Fatalf("QuoteImageTokenUsage error = %v", err)
	}
	// V006: (5*1000 + 8*2000 + 30*500) / 1M = 0.036 USD = 36 points = 36000 milli-points.
	if !quote.TotalUSD.Equal(decimal.RequireFromString("0.036")) {
		t.Fatalf("total USD = %s, want 0.036", quote.TotalUSD)
	}
	if quote.TotalCredits != 36000 {
		t.Fatalf("total credits = %d, want 36000", quote.TotalCredits)
	}
}

func TestT3QuoteImageTokenUsageRejectsPerImageOnlyPricing(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	pricing := `{"deployment_scope":"global","price_per_image":0.1}`
	insertT3ModelPricing(t, db, modelID, "gpt-image-2", pricing, "0", "0", false, false)

	engine := NewPricingEngine(db).(imageTokenPricingEngine)
	_, err := engine.QuoteImageTokenUsage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, ImageTokenUsage{TextInputTokens: 1000, ImageInputTokens: 2000, OutputTokens: 500})
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured (no per_image fallback)", err)
	}
}

func TestT3ImageTokenUsageSplitUsesInputTokensDetails(t *testing.T) {
	text, imageInput, output, ok := imageTokenUsageSplit(&adapter.Usage{
		InputTokens:        50,
		OutputTokens:       50,
		InputTokensDetails: adapter.InputTokensDetails{TextTokens: 10, ImageTokens: 40},
	})
	if !ok || text != 10 || imageInput != 40 || output != 50 {
		t.Fatalf("split = text=%d image=%d output=%d ok=%v, want 10/40/50 true", text, imageInput, output, ok)
	}

	text, imageInput, output, ok = imageTokenUsageSplit(&adapter.Usage{PromptTokens: 25, CompletionTokens: 5})
	if !ok || text != 25 || imageInput != 0 || output != 5 {
		t.Fatalf("split without details = text=%d image=%d output=%d ok=%v, want 25/0/5 true", text, imageInput, output, ok)
	}
}

// T3 acceptance scenario 2: video_generation base_unit=task pre-deduct equals
// settle, and the settlement quote is nonzero (previously it silently quoted 0).
func TestT3VideoTaskQuoteMatchesPrededuct(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "example",
			Model:    "video-task",
			Currency: "USD",
			Pricing:  datatypes.JSON([]byte(`{"deployment_scope":"global","video_generation":{"currency":"USD","billing_unit":"task","rates":[{"resolution":"720p","price_per_task":1.2}]}}`)),
		},
		BillingLane: UsageBillingLanePrivate,
	}
	svc := &llmGatewayServiceImpl{}
	req := &adapter.VideoTaskRequest{
		Model: "video-task",
		AdditionalParameters: map[string]interface{}{
			"resolution": "720p",
			"count":      2,
		},
	}
	resp := &adapter.VideoResponse{Status: "succeeded"}

	quote, err := svc.quoteVideoPricingForSelection(context.Background(), selection, req, resp)
	if err != nil {
		t.Fatalf("quoteVideoPricingForSelection error = %v", err)
	}
	// 2 tasks x 1.2 USD = 2.4 USD = 2,400,000 milli-points.
	if quote.TotalCredits != 2400000 {
		t.Fatalf("settle credits = %d, want 2400000 (2 tasks x 1.2 USD)", quote.TotalCredits)
	}

	prededuct, err := svc.videoPredeductQuoteFromTask(context.Background(), selection, req)
	if err != nil {
		t.Fatalf("videoPredeductQuoteFromTask error = %v", err)
	}
	if prededuct.TotalCredits != quote.TotalCredits {
		t.Fatalf("pre-deduct credits = %d, settle credits = %d, want equal", prededuct.TotalCredits, quote.TotalCredits)
	}
}

// The private image settlement helper must select the token path when the
// selected model declares image_token_pricing and the upstream reported the
// text/image/output split.
func TestT3QuoteImageTokenPricingForSettlementUsesTokens(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	pricing := `{"deployment_scope":"global","price_per_image":0.1,"image_token_pricing":{"currency":"USD","text_input_price_per_million":5,"image_input_price_per_million":8,"output_price_per_million":30}}`
	insertT3ModelPricing(t, db, modelID, "gpt-image-2", pricing, "0", "0", false, false)

	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:      modelID,
			Pricing: datatypes.JSON([]byte(pricing)),
		},
		BillingLane: UsageBillingLanePrivate,
	}
	svc := &llmGatewayServiceImpl{db: db}
	quote, tokenPriced, err := svc.quoteImageTokenPricingForSettlement(context.Background(), selection, &adapter.Usage{
		InputTokens:        3000,
		OutputTokens:       500,
		InputTokensDetails: adapter.InputTokensDetails{TextTokens: 1000, ImageTokens: 2000},
	})
	if err != nil {
		t.Fatalf("quoteImageTokenPricingForSettlement error = %v", err)
	}
	if !tokenPriced {
		t.Fatal("tokenPriced = false, want the image_token_pricing path")
	}
	if quote.TotalCredits != 36000 {
		t.Fatalf("total credits = %d, want 36000 (V006)", quote.TotalCredits)
	}
}

func TestT3QuoteImageTokenPricingForSettlementSkipsPerImageModel(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	pricing := `{"deployment_scope":"global","price_per_image":0.1}`
	insertT3ModelPricing(t, db, modelID, "gpt-image-2", pricing, "0", "0", false, false)

	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:      modelID,
			Pricing: datatypes.JSON([]byte(pricing)),
		},
		BillingLane: UsageBillingLanePrivate,
	}
	svc := &llmGatewayServiceImpl{db: db}
	_, tokenPriced, err := svc.quoteImageTokenPricingForSettlement(context.Background(), selection, &adapter.Usage{
		InputTokens:  3000,
		OutputTokens: 500,
	})
	if err != nil {
		t.Fatalf("quoteImageTokenPricingForSettlement error = %v", err)
	}
	if tokenPriced {
		t.Fatal("tokenPriced = true, want the per-image fallback for a model without image_token_pricing")
	}
}
