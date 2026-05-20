package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

func TestPricingEngineQuoteTokensUsesUSDPriceAsAuthority(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "openai",
		InputPrice:  2,
		OutputPrice: 4,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceUSDPrice {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceUSDPrice)
	}
	if !got.InputUSD.Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("inputUSD = %s, want 0.002", got.InputUSD)
	}
	if !got.OutputUSD.Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("outputUSD = %s, want 0.002", got.OutputUSD)
	}
	if !got.TotalUSD.Equal(decimal.RequireFromString("0.004")) {
		t.Fatalf("totalUSD = %s, want 0.004", got.TotalUSD)
	}
	if got.InputCredits != 2000 {
		t.Fatalf("inputCredits = %d, want 2000", got.InputCredits)
	}
	if got.OutputCredits != 2000 {
		t.Fatalf("outputCredits = %d, want 2000", got.OutputCredits)
	}
	if got.TotalCredits != 4000 {
		t.Fatalf("totalCredits = %d, want 4000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensUsesDefaultUSDFallbackForMissingUsedSide(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "openai",
		InputPrice:  0,
		OutputPrice: 4,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultUSDFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultUSDFallback)
	}
	if !got.InputUSD.Equal(decimal.RequireFromString("0.001")) {
		t.Fatalf("inputUSD = %s, want 0.001", got.InputUSD)
	}
	if !got.OutputUSD.Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("outputUSD = %s, want 0.002", got.OutputUSD)
	}
	if !got.TotalUSD.Equal(decimal.RequireFromString("0.003")) {
		t.Fatalf("totalUSD = %s, want 0.003", got.TotalUSD)
	}
	if got.InputCredits != 1000 {
		t.Fatalf("inputCredits = %d, want 1000", got.InputCredits)
	}
	if got.OutputCredits != 2000 {
		t.Fatalf("outputCredits = %d, want 2000", got.OutputCredits)
	}
	if got.TotalCredits != 3000 {
		t.Fatalf("totalCredits = %d, want 3000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensOnlyChargesUsedSide(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "openai",
		InputPrice:  2,
		OutputPrice: 0,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), 1000, 0)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceUSDPrice {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceUSDPrice)
	}
	if !got.InputUSD.Equal(decimal.RequireFromString("0.002")) {
		t.Fatalf("inputUSD = %s, want 0.002", got.InputUSD)
	}
	if !got.OutputUSD.IsZero() {
		t.Fatalf("outputUSD = %s, want 0", got.OutputUSD)
	}
	if got.InputCredits != 2000 || got.OutputCredits != 0 || got.TotalCredits != 2000 {
		t.Fatalf("credits = %d/%d/%d, want 2000/0/2000", got.InputCredits, got.OutputCredits, got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensUsesCeilForLowPriceRequests(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "openai",
		InputPrice:  0.075,
		OutputPrice: 0.3,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), 1000, 1000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if !got.TotalUSD.Equal(decimal.RequireFromString("0.000375")) {
		t.Fatalf("totalUSD = %s, want 0.000375", got.TotalUSD)
	}
	if got.TotalCredits != 375 {
		t.Fatalf("totalCredits = %d, want 375", got.TotalCredits)
	}
	if got.InputCredits+got.OutputCredits != got.TotalCredits {
		t.Fatalf("credits do not sum: input=%d output=%d total=%d", got.InputCredits, got.OutputCredits, got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensUsesDefaultUSDFallbackWhenModelMissing(t *testing.T) {
	db := newPricingEngineTestDB(t)

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(uuid.New(), PricingModelSourceGlobal), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultUSDFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultUSDFallback)
	}
	if !got.InputUSD.Equal(decimal.RequireFromString("0.001")) {
		t.Fatalf("inputUSD = %s, want 0.001", got.InputUSD)
	}
	if !got.OutputUSD.Equal(decimal.RequireFromString("0.001")) {
		t.Fatalf("outputUSD = %s, want 0.001", got.OutputUSD)
	}
	if got.TotalCredits != 2000 {
		t.Fatalf("totalCredits = %d, want 2000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensPassthroughSourceDoesNotQueryModelTables(t *testing.T) {
	dsn := fmt.Sprintf("file:pricing_engine_passthrough_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(uuid.New(), PricingModelSourcePassthrough), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultUSDFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultUSDFallback)
	}
	if got.TotalCredits != 2000 {
		t.Fatalf("totalCredits = %d, want 2000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensCustomSourceUsesCustomModelPrice(t *testing.T) {
	db := newPricingEngineTestDB(t)
	createPricingTestCustomModelTable(t, db)

	modelID := uuid.New()
	insertPricingTestCustomModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "ollama",
		InputPrice:  3,
		OutputPrice: 5,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceCustom), 1000, 2000)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceUSDPrice {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceUSDPrice)
	}
	if !got.InputUSD.Equal(decimal.RequireFromString("0.003")) {
		t.Fatalf("inputUSD = %s, want 0.003", got.InputUSD)
	}
	if !got.OutputUSD.Equal(decimal.RequireFromString("0.01")) {
		t.Fatalf("outputUSD = %s, want 0.01", got.OutputUSD)
	}
	if got.TotalCredits != 13000 {
		t.Fatalf("totalCredits = %d, want 13000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensUsesDefaultForCustomModelWithoutPrice(t *testing.T) {
	db := newPricingEngineTestDB(t)
	createPricingTestCustomModelTable(t, db)

	modelID := uuid.New()
	insertPricingTestCustomModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "ollama",
		InputPrice:  0,
		OutputPrice: 0,
	})

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceCustom), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultUSDFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultUSDFallback)
	}
	if got.TotalCredits != 2000 {
		t.Fatalf("totalCredits = %d, want 2000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensGlobalSourceIgnoresLegacyCustomModelTable(t *testing.T) {
	db := newPricingEngineTestDB(t)
	createPricingTestLegacyCustomModelTable(t, db)

	modelID := uuid.New()
	if err := db.Exec(
		`INSERT INTO llm_custom_models (id, provider, cost_input, cost_output) VALUES (?, ?, ?, ?)`,
		modelID.String(),
		"ollama",
		3,
		5,
	).Error; err != nil {
		t.Fatalf("insert legacy llm_custom_model: %v", err)
	}

	engine := NewPricingEngine(db)
	got, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), 1000, 500)
	if err != nil {
		t.Fatalf("QuoteTokens returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultUSDFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultUSDFallback)
	}
	if got.TotalCredits != 2000 {
		t.Fatalf("totalCredits = %d, want 2000", got.TotalCredits)
	}
}

func TestPricingEngineQuoteTokensCustomSourceFailsWhenPriceColumnsMissing(t *testing.T) {
	db := newPricingEngineTestDB(t)
	createPricingTestLegacyCustomModelTable(t, db)

	modelID := uuid.New()
	if err := db.Exec(
		`INSERT INTO llm_custom_models (id, provider, cost_input, cost_output) VALUES (?, ?, ?, ?)`,
		modelID.String(),
		"ollama",
		3,
		5,
	).Error; err != nil {
		t.Fatalf("insert legacy llm_custom_model: %v", err)
	}

	engine := NewPricingEngine(db)
	_, err := engine.QuoteTokens(context.Background(), pricingTestModelRef(modelID, PricingModelSourceCustom), 1000, 500)
	if err == nil {
		t.Fatal("QuoteTokens returned nil error, want missing column error")
	}
}

func TestPricingEngineQuoteImageUsesModelUSDPriceAsAuthority(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "qwen",
		InputPrice:  0.0287,
		OutputPrice: 0,
		ImagePrices: []llmmodel.PricingRule{
			{
				ID:         "ignored",
				Priority:   100,
				Conditions: map[string]any{},
				Price: llmmodel.PricingDetail{
					Credits: 999,
					Amount:  0.25,
				},
			},
		},
	})

	engine := NewPricingEngine(db)
	n := 2
	got, err := engine.QuoteImage(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), &adapter.ImageRequest{
		Size: "1024x1024",
		N:    &n,
	})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}

	if got.Source != PricingSourceUSDPrice {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceUSDPrice)
	}
	if !got.TotalUSD.Equal(decimal.RequireFromString("0.0574")) {
		t.Fatalf("totalUSD = %s, want 0.0574", got.TotalUSD)
	}
	if !got.InputUSD.IsZero() {
		t.Fatalf("inputUSD = %s, want 0", got.InputUSD)
	}
	if !got.OutputUSD.Equal(decimal.RequireFromString("0.0574")) {
		t.Fatalf("outputUSD = %s, want 0.0574", got.OutputUSD)
	}
	if got.InputCredits != 0 || got.OutputCredits != 57400 || got.TotalCredits != 57400 {
		t.Fatalf("credits = %d/%d/%d, want 0/57400/57400", got.InputCredits, got.OutputCredits, got.TotalCredits)
	}
}

func TestPricingEngineQuoteImageSumsInputAndOutputUSD(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "qwen",
		InputPrice:  0.1,
		OutputPrice: 0.02,
	})

	engine := NewPricingEngine(db)
	n := 2
	got, err := engine.QuoteImage(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), &adapter.ImageRequest{N: &n})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}

	if !got.TotalUSD.Equal(decimal.RequireFromString("0.24")) {
		t.Fatalf("totalUSD = %s, want 0.24", got.TotalUSD)
	}
	if got.InputCredits != 0 || got.OutputCredits != 240000 || got.TotalCredits != 240000 {
		t.Fatalf("credits = %d/%d/%d, want 0/240000/240000", got.InputCredits, got.OutputCredits, got.TotalCredits)
	}
}

func TestPricingEngineQuoteImageFallsBackToProviderDefaultsWhenModelHasNoUSDPrice(t *testing.T) {
	db := newPricingEngineTestDB(t)

	modelID := uuid.New()
	insertPricingTestModel(t, db, pricingTestModelRow{
		ID:          modelID,
		Provider:    "qwen",
		InputPrice:  0,
		OutputPrice: 0,
		ImagePrices: []llmmodel.PricingRule{
			{
				ID:         "ignored",
				Priority:   100,
				Conditions: map[string]any{},
				Price: llmmodel.PricingDetail{
					Credits: 999,
					Amount:  0.25,
				},
			},
		},
	})

	engine := NewPricingEngine(db)
	n := 2
	got, err := engine.QuoteImage(context.Background(), pricingTestModelRef(modelID, PricingModelSourceGlobal), &adapter.ImageRequest{
		Model: "qwen/qwen-image-2.0",
		N:     &n,
	})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultImageRuleFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultImageRuleFallback)
	}
	if got.TotalCredits != 320 {
		t.Fatalf("totalCredits = %d, want 320", got.TotalCredits)
	}
}

func TestPricingEngineQuoteImageFallsBackToProviderDefaultsWhenModelTableMissing(t *testing.T) {
	dsn := fmt.Sprintf("file:pricing_engine_missing_table_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	engine := NewPricingEngine(db)
	n := 2
	got, err := engine.QuoteImage(context.Background(), pricingTestModelRef(uuid.Nil, PricingModelSourcePassthrough), &adapter.ImageRequest{
		Model: "qwen/qwen-image-2.0",
		Size:  "1024x1024",
		N:     &n,
	})
	if err != nil {
		t.Fatalf("QuoteImage returned error: %v", err)
	}

	if got.Source != PricingSourceDefaultImageRuleFallback {
		t.Fatalf("source = %s, want %s", got.Source, PricingSourceDefaultImageRuleFallback)
	}
	if got.TotalCredits != 320 {
		t.Fatalf("totalCredits = %d, want 320", got.TotalCredits)
	}
}

type pricingTestModelRow struct {
	ID          uuid.UUID
	Provider    string
	InputPrice  float64
	OutputPrice float64
	CostRate    map[string]any
	ImagePrices []llmmodel.PricingRule
}

func pricingTestModelRef(modelID uuid.UUID, source PricingModelSource) PricingModelRef {
	return PricingModelRef{
		ModelID: modelID,
		Source:  source,
	}
}

func newPricingEngineTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:pricing_engine_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			input_price NUMERIC NOT NULL DEFAULT 0,
			output_price NUMERIC NOT NULL DEFAULT 0,
			cost_rate TEXT,
			image_prices TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create llm_models: %v", err)
	}

	return db
}

func insertPricingTestModel(t *testing.T, db *gorm.DB, row pricingTestModelRow) {
	t.Helper()

	costRate := "{}"
	if row.CostRate != nil {
		data, err := json.Marshal(row.CostRate)
		if err != nil {
			t.Fatalf("marshal cost_rate: %v", err)
		}
		costRate = string(data)
	}

	imagePrices := "[]"
	if row.ImagePrices != nil {
		data, err := json.Marshal(row.ImagePrices)
		if err != nil {
			t.Fatalf("marshal image_prices: %v", err)
		}
		imagePrices = string(data)
	}

	if err := db.Exec(
		`INSERT INTO llm_models (id, provider, input_price, output_price, cost_rate, image_prices) VALUES (?, ?, ?, ?, ?, ?)`,
		row.ID.String(),
		row.Provider,
		row.InputPrice,
		row.OutputPrice,
		costRate,
		imagePrices,
	).Error; err != nil {
		t.Fatalf("insert llm_model: %v", err)
	}
}

func createPricingTestCustomModelTable(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			input_price NUMERIC NOT NULL DEFAULT 0,
			output_price NUMERIC NOT NULL DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create llm_custom_models: %v", err)
	}
}

func createPricingTestLegacyCustomModelTable(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			cost_input NUMERIC NOT NULL DEFAULT 0,
			cost_output NUMERIC NOT NULL DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create legacy llm_custom_models: %v", err)
	}
}

func insertPricingTestCustomModel(t *testing.T, db *gorm.DB, row pricingTestModelRow) {
	t.Helper()

	if err := db.Exec(
		`INSERT INTO llm_custom_models (id, provider, input_price, output_price) VALUES (?, ?, ?, ?)`,
		row.ID.String(),
		row.Provider,
		row.InputPrice,
		row.OutputPrice,
	).Error; err != nil {
		t.Fatalf("insert llm_custom_model: %v", err)
	}
}
