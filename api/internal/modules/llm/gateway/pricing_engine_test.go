package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openPricingEngineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`CREATE TABLE llm_models (
		id text PRIMARY KEY,
		provider text,
		name text,
		input_price decimal,
		output_price decimal,
		input_price_configured boolean,
		output_price_configured boolean,
		image_prices json
	)`).Error; err != nil {
		t.Fatalf("create llm_models: %v", err)
	}
	if err := db.Exec(`CREATE TABLE llm_model_configs (
		id text PRIMARY KEY,
		organization_id text,
		model_id text,
		input_price_override decimal,
		output_price_override decimal,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create llm_model_configs: %v", err)
	}
	if err := db.Exec(`CREATE TABLE llm_pricing_fallback_overrides (
		organization_id text PRIMARY KEY,
		enabled boolean,
		rules json,
		updated_by text,
		created_at datetime,
		updated_at datetime
	)`).Error; err != nil {
		t.Fatalf("create llm_pricing_fallback_overrides: %v", err)
	}
	return db
}

func insertPricingModel(t *testing.T, db *gorm.DB, modelID uuid.UUID, provider string, inputPrice string, outputPrice string, inputConfigured bool, outputConfigured bool, imagePrices string) {
	insertPricingModelNamed(t, db, modelID, provider, "test-model", inputPrice, outputPrice, inputConfigured, outputConfigured, imagePrices)
}

func insertPricingModelNamed(t *testing.T, db *gorm.DB, modelID uuid.UUID, provider string, name string, inputPrice string, outputPrice string, inputConfigured bool, outputConfigured bool, imagePrices string) {
	t.Helper()
	if imagePrices == "" {
		imagePrices = "[]"
	}
	if err := db.Exec(
		`INSERT INTO llm_models (id, provider, name, input_price, output_price, input_price_configured, output_price_configured, image_prices) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		modelID.String(), provider, name, inputPrice, outputPrice, inputConfigured, outputConfigured, imagePrices,
	).Error; err != nil {
		t.Fatalf("insert model: %v", err)
	}
}

func savePricingFallbackForOrg(t *testing.T, db *gorm.DB, organizationID uuid.UUID, req UpdatePricingFallbackRequest) {
	t.Helper()
	if _, err := SavePricingFallbackConfig(context.Background(), db, organizationID, req, "test"); err != nil {
		t.Fatalf("save override: %v", err)
	}
}

func enableDefaultPricingFallbackForOrg(t *testing.T, db *gorm.DB, organizationID uuid.UUID) {
	t.Helper()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{Enabled: true})
}

func insertPricingModelConfigOverride(t *testing.T, db *gorm.DB, organizationID, modelID uuid.UUID, inputPriceOverride, outputPriceOverride *string) {
	t.Helper()
	var inputValue interface{}
	if inputPriceOverride != nil {
		inputValue = *inputPriceOverride
	}
	var outputValue interface{}
	if outputPriceOverride != nil {
		outputValue = *outputPriceOverride
	}
	if err := db.Exec(
		`INSERT INTO llm_model_configs (id, organization_id, model_id, input_price_override, output_price_override) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), organizationID.String(), modelID.String(), inputValue, outputValue,
	).Error; err != nil {
		t.Fatalf("insert model config override: %v", err)
	}
}

func TestPricingEngineQuoteTokensUsesStoredModelPricesWhenConfigured(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "1", "2", true, true, "[]")

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 3000 {
		t.Fatalf("total credits = %d, want 3000", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want upstream", quote.PricingSource)
	}
}

func TestPricingEngineQuoteTokensPrefersOrganizationOverridePrices(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	organizationID := uuid.New()
	inputOverride := "5"
	outputOverride := "7"
	insertPricingModel(t, db, modelID, "openai", "1", "2", true, true, "[]")
	insertPricingModelConfigOverride(t, db, organizationID, modelID, &inputOverride, &outputOverride)

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: organizationID,
		Source:         PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 12000 {
		t.Fatalf("total credits = %d, want organization override credits 12000", quote.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensUsesOverrideWhenBasePriceMissing(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	organizationID := uuid.New()
	inputOverride := "3"
	outputOverride := "4"
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")
	insertPricingModelConfigOverride(t, db, organizationID, modelID, &inputOverride, &outputOverride)

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: organizationID,
		Source:         PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 7000 {
		t.Fatalf("total credits = %d, want override credits 7000", quote.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensMissingBaseAndOverrideFails(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")

	_, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: uuid.New(),
		Source:         PricingModelSourceGlobal,
	}, 1000, 1000)
	if err == nil {
		t.Fatalf("QuoteTokens error = nil, want missing price error")
	}
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured", err)
	}
}

func TestPricingEngineQuoteTokensConfiguredZeroIsFree(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", true, true, "[]")

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 0 {
		t.Fatalf("total credits = %d, want 0", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want upstream", quote.PricingSource)
	}
}

func TestPricingEngineQuoteTokensUnconfiguredZeroFailsWithoutOrganizationFallback(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")

	_, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, 1000, 1000)
	if err == nil {
		t.Fatalf("QuoteTokens error = nil, want missing organization fallback error")
	}
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured", err)
	}
}

func TestPricingEngineQuoteTokensUsesAdminFallbackBeforeCodeDefault(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "token.chat.input.override",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				PriceUSDPer1MTokens: "3",
			},
			{
				ID:                  "token.chat.output.override",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				PriceUSDPer1MTokens: "4",
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 7000 {
		t.Fatalf("total credits = %d, want 7000", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceAdminFallback {
		t.Fatalf("pricing source = %q, want admin fallback", quote.PricingSource)
	}
}

func TestPricingEngineQuoteTokensOrganizationFallbackIsScoped(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationA := uuid.New()
	organizationB := uuid.New()
	savePricingFallbackForOrg(t, db, organizationA, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "token.chat.input.org-a",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				PriceUSDPer1MTokens: "3",
			},
			{
				ID:                  "token.chat.output.org-a",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				PriceUSDPer1MTokens: "4",
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationA,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens for organization A returned error: %v", err)
	}
	if quote.TotalCredits != 7000 {
		t.Fatalf("organization A total credits = %d, want 7000", quote.TotalCredits)
	}

	_, err = NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationB,
	}, 1000, 1000)
	if err == nil {
		t.Fatalf("QuoteTokens for organization B error = nil, want missing fallback config error")
	}
}

func TestPricingEngineQuoteTokensSpecificAdminRuleBeatsEarlierWildcard(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	modelID := uuid.New()
	insertPricingModelNamed(t, db, modelID, "deepseek", "deepseek-chat", "0", "0", false, false, "[]")

	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "token.chat.input.wildcard",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				Provider:            "*",
				Model:               "*",
				PriceUSDPer1MTokens: "9",
			},
			{
				ID:                  "token.chat.output.wildcard",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				Provider:            "*",
				Model:               "*",
				PriceUSDPer1MTokens: "9",
			},
			{
				ID:                  "token.chat.input.deepseek",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				Provider:            "deepseek",
				Model:               "deepseek-chat",
				PriceUSDPer1MTokens: "3",
			},
			{
				ID:                  "token.chat.output.deepseek",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				Provider:            "deepseek",
				Model:               "deepseek-chat",
				PriceUSDPer1MTokens: "4",
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 7000 {
		t.Fatalf("total credits = %d, want specific rule credits 7000", quote.TotalCredits)
	}
	if quote.InputRuleID != "token.chat.input.deepseek" || quote.OutputRuleID != "token.chat.output.deepseek" {
		t.Fatalf("rule ids = %q/%q, want deepseek-specific rules", quote.InputRuleID, quote.OutputRuleID)
	}
	if quote.PricingSource != PricingSourceAdminFallback {
		t.Fatalf("pricing source = %q, want admin fallback", quote.PricingSource)
	}
}

func TestPricingEngineQuoteTokensPreservesConfiguredInputWhenOutputFallsBack(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "5", "0", true, false, "[]")

	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "token.chat.input.override",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				PriceUSDPer1MTokens: "9",
			},
			{
				ID:                  "token.chat.output.override",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				PriceUSDPer1MTokens: "4",
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.InputCredits != 5000 || quote.OutputCredits != 4000 {
		t.Fatalf("credits = %d/%d, want configured input 5000 and fallback output 4000", quote.InputCredits, quote.OutputCredits)
	}
	if quote.InputRuleID != "" || quote.OutputRuleID != "token.chat.output.override" {
		t.Fatalf("rule ids = %q/%q, want only output fallback rule", quote.InputRuleID, quote.OutputRuleID)
	}
}

func TestPricingEngineQuoteTokensFailsWhenFallbackDisabled(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{Enabled: false})

	_, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, 1000, 0)
	if err == nil {
		t.Fatalf("QuoteTokens error = nil, want fallback disabled error")
	}
}

func TestPricingEngineQuoteTokensEmbeddingAndRerankRequireOnlyInputPrice(t *testing.T) {
	for _, operation := range []PricingOperation{PricingOperationEmbedding, PricingOperationRerank} {
		t.Run(string(operation), func(t *testing.T) {
			db := openPricingEngineTestDB(t)
			modelID := uuid.New()
			insertPricingModel(t, db, modelID, "openai", "1", "0", true, false, "[]")

			quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
				ModelID:   modelID,
				Source:    PricingModelSourceGlobal,
				Operation: operation,
			}, 1000, 1000)
			if err != nil {
				t.Fatalf("QuoteTokens returned error: %v", err)
			}
			if quote.TotalCredits != 1000 {
				t.Fatalf("total credits = %d, want input-only credits 1000", quote.TotalCredits)
			}
			if !quote.InputTokenPriceResolved || quote.OutputTokenPriceResolved {
				t.Fatalf("resolved input/output = %v/%v, want input only", quote.InputTokenPriceResolved, quote.OutputTokenPriceResolved)
			}
		})
	}
}

func TestPricingEngineQuoteImageUsesConfiguredImagePrices(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	imagePrices := `[{
		"id":"configured",
		"priority":100,
		"conditions":{"size":"1024x1024","quality":"standard"},
		"price":{"credits":321}
	}]`
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, imagePrices)
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "gpt-image", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 642 {
		t.Fatalf("total credits = %d, want 642", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want upstream", quote.PricingSource)
	}
}

func TestPricingEngineQuoteImageFailsWithoutOrganizationFallback(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")

	_, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: uuid.New(),
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "gpt-image-1"})
	if err == nil {
		t.Fatalf("QuoteImage error = nil, want missing organization fallback error")
	}
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured", err)
	}
}

func TestPricingEngineQuoteImagePrefersOrganizationOutputOverrideOverConfiguredImagePrices(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	organizationID := uuid.New()
	outputOverride := "0.20"
	imagePrices := `[{
		"id":"configured",
		"priority":100,
		"conditions":{"size":"1024x1024","quality":"standard"},
		"price":{"credits":321}
	}]`
	insertPricingModel(t, db, modelID, "qwen", "0", "0", false, false, imagePrices)
	insertPricingModelConfigOverride(t, db, organizationID, modelID, nil, &outputOverride)
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: organizationID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "qwen-image-2.0", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 400000 {
		t.Fatalf("total credits = %d, want organization override credits 400000", quote.TotalCredits)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(quote.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	if snapshot["price_column"] != "output_price_override" {
		t.Fatalf("snapshot price_column = %v, want output_price_override", snapshot["price_column"])
	}
}

func TestPricingEngineQuoteImageUsesConfiguredScalarImagePrice(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModelNamed(t, db, modelID, "qwen", "qwen-image-2.0", "0.0287", "0", true, false, "[]")
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "qwen-image-2.0", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 57400 {
		t.Fatalf("total credits = %d, want 57400", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want upstream", quote.PricingSource)
	}
}

func TestPricingEngineQuoteImagePrefersOutputPriceForScalarImagePrice(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModelNamed(t, db, modelID, "qwen", "qwen-image-2.0", "0.10", "0.20", true, true, "[]")
	n := 1

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "qwen-image-2.0", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 200000 {
		t.Fatalf("total credits = %d, want output-price credits 200000", quote.TotalCredits)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(quote.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	if snapshot["price_column"] != "output_price" {
		t.Fatalf("snapshot price_column = %v, want output_price", snapshot["price_column"])
	}
}

func TestPricingEngineQuoteImageUsesOrganizationInputOverrideWhenOutputMissing(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	organizationID := uuid.New()
	inputOverride := "0.10"
	insertPricingModelNamed(t, db, modelID, "qwen", "qwen-image-2.0", "0", "0", false, false, "[]")
	insertPricingModelConfigOverride(t, db, organizationID, modelID, &inputOverride, nil)
	n := 1

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: organizationID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "qwen-image-2.0", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 100000 {
		t.Fatalf("total credits = %d, want input-override credits 100000", quote.TotalCredits)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(quote.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	if snapshot["price_column"] != "input_price_override" {
		t.Fatalf("snapshot price_column = %v, want input_price_override", snapshot["price_column"])
	}
}

func TestPricingEngineQuoteImageFailsWithPricingNotConfigured(t *testing.T) {
	db := openPricingEngineTestDB(t)

	_, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{}, &adapter.ImageRequest{Model: "qwen-image"})
	if err == nil {
		t.Fatalf("QuoteImage error = nil, want missing price error")
	}
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured", err)
	}
}

func TestPricingEngineQuoteImageUsesCodeFallbackAndMultipliesCount(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	enableDefaultPricingFallbackForOrg(t, db, organizationID)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "0", "0", false, false, "[]")
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "gpt-image-1", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 400 {
		t.Fatalf("total credits = %d, want 400", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default", quote.PricingSource)
	}
}

func TestPricingEngineQuoteImageUsesLegacyOutputPriceAsScalarPrice(t *testing.T) {
	db := openPricingEngineTestDB(t)
	modelID := uuid.New()
	insertPricingModel(t, db, modelID, "openai", "1", "2", false, false, "[]")
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		ModelID: modelID,
		Source:  PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "legacy-image", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 4000000 {
		t.Fatalf("total credits = %d, want legacy output USD price 4000000", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceUpstreamModelPrice {
		t.Fatalf("pricing source = %q, want upstream", quote.PricingSource)
	}
}

func TestPricingEngineQuoteImageCanonicalizesProviderAlias(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	enableDefaultPricingFallbackForOrg(t, db, organizationID)
	modelID := uuid.New()
	insertPricingModelNamed(t, db, modelID, "dashscope", "wanx-image", "0", "0", false, false, "[]")

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        modelID,
		Source:         PricingModelSourceGlobal,
	}, &adapter.ImageRequest{Model: "wanx-image"})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 160 {
		t.Fatalf("total credits = %d, want qwen alias default 160", quote.TotalCredits)
	}
}

func TestPricingEngineQuoteImageSnapshotExplainsTheBill(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	enableDefaultPricingFallbackForOrg(t, db, organizationID)
	n := 2

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, &adapter.ImageRequest{Model: "qwen-image", N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(quote.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	if snapshot["pricing_source"] != string(PricingSourceCodeDefaultFallback) {
		t.Fatalf("snapshot pricing_source = %v, want code default", snapshot["pricing_source"])
	}
	if snapshot["credits_per_image"] != float64(160) || snapshot["image_count"] != float64(2) {
		t.Fatalf("snapshot = %#v, want qwen credits_per_image=160 image_count=2", snapshot)
	}
}

func TestPricingEngineQuoteTokensUsesPassthroughIdentityForFallback(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "token.chat.input.wildcard",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				Provider:            "*",
				Model:               "*",
				PriceUSDPer1MTokens: "9",
			},
			{
				ID:                  "token.chat.output.wildcard",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				Provider:            "*",
				Model:               "*",
				PriceUSDPer1MTokens: "9",
			},
			{
				ID:                  "token.chat.input.deepseek",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				Provider:            "deepseek",
				Model:               "deepseek-chat",
				PriceUSDPer1MTokens: "3",
			},
			{
				ID:                  "token.chat.output.deepseek",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterOutputToken,
				Provider:            "deepseek",
				Model:               "deepseek-chat",
				PriceUSDPer1MTokens: "4",
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		Source:         PricingModelSourcePassthrough,
		Provider:       "deepseek",
		Model:          "deepseek-chat",
	}, 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if quote.TotalCredits != 7000 {
		t.Fatalf("total credits = %d, want passthrough-specific rule credits 7000", quote.TotalCredits)
	}
	if quote.InputRuleID != "token.chat.input.deepseek" || quote.OutputRuleID != "token.chat.output.deepseek" {
		t.Fatalf("rule ids = %q/%q, want deepseek-specific rules", quote.InputRuleID, quote.OutputRuleID)
	}
}

func TestPricingEngineQuoteImageAdminWildcardDoesNotOverrideSpecificDefault(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:            "image.generic.override",
				Operation:     PricingOperationImage,
				Meter:         PricingMeterImage,
				Provider:      "*",
				Model:         "*",
				Size:          "1024x1024",
				Quality:       "standard",
				Style:         "*",
				Credits:       999,
				PricingSource: PricingSourceAdminFallback,
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, &adapter.ImageRequest{Model: "qwen-image"})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 160 {
		t.Fatalf("total credits = %d, want qwen default 160", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default", quote.PricingSource)
	}

	unknownQuote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, &adapter.ImageRequest{Model: "unknown-image-model"})
	if err != nil {
		t.Fatalf("QuoteImage for unknown provider returned error: %v", err)
	}
	if unknownQuote.TotalCredits != 999 {
		t.Fatalf("unknown provider credits = %d, want admin wildcard 999", unknownQuote.TotalCredits)
	}
	if unknownQuote.PricingSource != PricingSourceAdminFallback {
		t.Fatalf("unknown provider pricing source = %q, want admin fallback", unknownQuote.PricingSource)
	}
}

func TestPricingEngineQuoteImageAdminSpecificOverridesSpecificDefault(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:            "image.qwen.override",
				Operation:     PricingOperationImage,
				Meter:         PricingMeterImage,
				Provider:      "qwen",
				Model:         "*",
				Size:          "*",
				Quality:       "*",
				Style:         "*",
				Credits:       111,
				PricingSource: PricingSourceAdminFallback,
			},
		},
	})

	quote, err := NewPricingEngine(db).QuoteImage(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
	}, &adapter.ImageRequest{Model: "qwen-image"})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}
	if quote.TotalCredits != 111 {
		t.Fatalf("total credits = %d, want admin qwen 111", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceAdminFallback {
		t.Fatalf("pricing source = %q, want admin fallback", quote.PricingSource)
	}
}

func TestPricingEngineQuoteTokensZeroEstimateFailsWhenFallbackDisabled(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	savePricingFallbackForOrg(t, db, organizationID, UpdatePricingFallbackRequest{Enabled: false})

	_, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		Operation:      PricingOperationEmbedding,
	}, 0, 0)
	if err == nil {
		t.Fatalf("QuoteTokens error = nil, want missing price error before provider")
	}
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("error = %v, want ErrPricingNotConfigured", err)
	}
}

func TestPricingEngineQuoteTokensZeroEstimateLocksEmbeddingInputFallback(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	enableDefaultPricingFallbackForOrg(t, db, organizationID)

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		Operation:      PricingOperationEmbedding,
	}, 0, 0)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if !quote.InputTokenPriceResolved {
		t.Fatalf("input price resolved = false, want locked input fallback price")
	}
	if quote.OutputTokenPriceResolved {
		t.Fatalf("output price resolved = true, want embedding to lock input only")
	}
	if quote.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default fallback", quote.PricingSource)
	}

	repriced, err := repriceLockedTokenQuote(quote, 1000, 0)
	if err != nil {
		t.Fatalf("repriceLockedTokenQuote returned error: %v", err)
	}
	if repriced.TotalCredits != 1000 {
		t.Fatalf("repriced credits = %d, want 1000", repriced.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensZeroEstimateLocksChatInputAndOutputFallback(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()
	enableDefaultPricingFallbackForOrg(t, db, organizationID)

	quote, err := NewPricingEngine(db).QuoteTokens(context.Background(), PricingModelRef{
		OrganizationID: organizationID,
		Operation:      PricingOperationChat,
	}, 0, 0)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}
	if !quote.InputTokenPriceResolved || !quote.OutputTokenPriceResolved {
		t.Fatalf("resolved input/output = %v/%v, want both locked", quote.InputTokenPriceResolved, quote.OutputTokenPriceResolved)
	}

	repriced, err := repriceLockedTokenQuote(quote, 1000, 1000)
	if err != nil {
		t.Fatalf("repriceLockedTokenQuote returned error: %v", err)
	}
	if repriced.TotalCredits != 3000 {
		t.Fatalf("repriced credits = %d, want 3000", repriced.TotalCredits)
	}
}

func TestSavePricingFallbackConfigRejectsDuplicateTargets(t *testing.T) {
	db := openPricingEngineTestDB(t)
	organizationID := uuid.New()

	_, err := SavePricingFallbackConfig(context.Background(), db, organizationID, UpdatePricingFallbackRequest{
		Enabled: true,
		OverrideRules: []PricingFallbackRule{
			{
				ID:                  "chat-input-a",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				Provider:            "*",
				Model:               "*",
				PriceUSDPer1MTokens: "1",
			},
			{
				ID:                  "chat-input-b",
				Operation:           PricingOperationChat,
				Meter:               PricingMeterInputToken,
				PriceUSDPer1MTokens: "2",
			},
		},
	}, "test")
	if err == nil {
		t.Fatalf("SavePricingFallbackConfig error = nil, want duplicate target error")
	}
	if !strings.Contains(err.Error(), "target is duplicated") {
		t.Fatalf("error = %v, want duplicate target error", err)
	}
}

func TestEffectivePricingFallbackRulesDedupeByTarget(t *testing.T) {
	overrideRules := []PricingFallbackRule{
		{
			ID:                  "admin-chat-input-first",
			Operation:           PricingOperationChat,
			Meter:               PricingMeterInputToken,
			PriceUSDPer1MTokens: "3",
		},
		{
			ID:                  "admin-chat-input-duplicate",
			Operation:           PricingOperationChat,
			Meter:               PricingMeterInputToken,
			Provider:            "*",
			Model:               "*",
			PriceUSDPer1MTokens: "4",
		},
	}

	effective := effectivePricingFallbackRules(DefaultPricingFallbackRules(), overrideRules)
	var chatInputRules []PricingFallbackRule
	for _, rule := range effective {
		if rule.Operation == PricingOperationChat && rule.Meter == PricingMeterInputToken {
			chatInputRules = append(chatInputRules, rule)
		}
	}
	if len(chatInputRules) != 1 {
		t.Fatalf("chat input rules = %#v, want one rule after target dedupe", chatInputRules)
	}
	if chatInputRules[0].ID != "admin-chat-input-first" {
		t.Fatalf("chat input rule id = %q, want first admin rule", chatInputRules[0].ID)
	}
	if chatInputRules[0].PricingSource != PricingSourceAdminFallback {
		t.Fatalf("chat input pricing source = %q, want admin fallback", chatInputRules[0].PricingSource)
	}
}

func TestPricingEngineHasColumnCachesResult(t *testing.T) {
	db := openPricingEngineTestDB(t)
	engine := NewPricingEngine(db).(*pricingEngine)

	if !engine.hasColumn("llm_models", "image_prices") {
		t.Fatalf("first hasColumn returned false, want true")
	}
	if err := db.Exec(`DROP TABLE llm_models`).Error; err != nil {
		t.Fatalf("drop llm_models: %v", err)
	}
	if !engine.hasColumn("llm_models", "image_prices") {
		t.Fatalf("second hasColumn returned false, want cached true after table drop")
	}
}

func TestPricingFallbackHandlerUsesCurrentOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openPricingEngineTestDB(t)
	organizationA := uuid.New()
	organizationB := uuid.New()
	handler := NewPricingFallbackHandler(db)

	router := gin.New()
	router.PUT("/pricing/fallback",
		func(c *gin.Context) {
			c.Set("organization_id", organizationA.String())
			c.Set("account_id", "account-a")
		},
		handler.Update,
	)
	router.GET("/pricing/fallback",
		func(c *gin.Context) {
			c.Set("organization_id", c.Query("organization_id"))
		},
		handler.Get,
	)

	updateRecorder := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/pricing/fallback", strings.NewReader(`{"enabled":true}`))
	updateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateReq)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200 body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}

	getConfig := func(organizationID uuid.UUID) PricingFallbackConfigResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/pricing/fallback?organization_id="+organizationID.String(), nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("get status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
		}
		var resp struct {
			Data PricingFallbackConfigResponse `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		return resp.Data
	}

	configA := getConfig(organizationA)
	if !configA.Enabled {
		t.Fatalf("organization A enabled = false, want true")
	}
	configB := getConfig(organizationB)
	if configB.Enabled {
		t.Fatalf("organization B enabled = true, want false")
	}
}

func TestRepriceLockedTokenQuoteUsesLockedUnitPrices(t *testing.T) {
	locked := withTokenPricingBasis(
		newUSDQuote(decimal.Zero, decimal.Zero, PricingSourceCodeDefaultFallback, "in,out", UsageSourceProviderUsage, nil),
		decimal.RequireFromString("1"),
		decimal.RequireFromString("2"),
		true,
		true,
		"in",
		"out",
	)

	quote, err := repriceLockedTokenQuote(locked, 2000, 3000)
	if err != nil {
		t.Fatalf("repriceLockedTokenQuote returned error: %v", err)
	}
	if quote.TotalCredits != 8000 {
		t.Fatalf("total credits = %d, want 8000", quote.TotalCredits)
	}
	if quote.PricingSource != PricingSourceCodeDefaultFallback {
		t.Fatalf("pricing source = %q, want code default fallback", quote.PricingSource)
	}
}

func TestRepriceLockedTokenQuoteFailsWhenOutputPriceWasNotLocked(t *testing.T) {
	locked := withTokenPricingBasis(
		newUSDQuote(decimal.Zero, decimal.Zero, PricingSourceCodeDefaultFallback, "in", UsageSourceProviderUsage, nil),
		decimal.RequireFromString("1"),
		decimal.Zero,
		true,
		false,
		"in",
		"",
	)

	_, err := repriceLockedTokenQuote(locked, 1000, 1)
	if err == nil {
		t.Fatalf("repriceLockedTokenQuote error = nil, want missing output price error")
	}
}
