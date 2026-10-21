package modelmeta

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func cnyCatalogModel() PublishedModel {
	pricing := `{
		"deployment_scope":"global",
		"token_tiers":[{"min_input_tokens":0,"input_price_per_million":14,"output_price_per_million":70}],
		"rules":[{"id":"chat-input","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"14","configured":true}],
		"source":{"amount":"14","currency":"CNY","rate":"7","rate_at":"2026-09-20T00:00:00Z"}
	}`
	return PublishedModel{
		Provider:              "agicto",
		Vendor:                "Agicto Supplier",
		Model:                 "gpt-x",
		ModelName:             "GPT X",
		Status:                llmmodel.ModelStatusActive,
		IsActive:              true,
		IsSystemEnabled:       true,
		Currency:              "CNY",
		InputPrice:            14,
		OutputPrice:           70,
		InputPriceConfigured:  true,
		OutputPriceConfigured: true,
		Pricing:               json.RawMessage(pricing),
	}
}

func cnyCatalog() PublishedCatalog {
	return PublishedCatalog{
		Version:     7,
		PublishedAt: time.Now().UTC(),
		Providers: []PublishedProvider{{
			Provider:        "agicto",
			ProviderName:    "Agicto",
			Status:          llmmodel.ModelStatusActive,
			IsActive:        true,
			IsSystemEnabled: true,
		}},
		Vendors: []PublishedVendor{{Vendor: "agicto supplier", VendorName: "Agicto Supplier"}},
		Models:  []PublishedModel{cnyCatalogModel()},
	}
}

// T19 acceptance 1: applying a snapshot normalizes CNY prices to USD, records
// the normalization status/rate source in the pricing JSON, and is idempotent.
func TestApplyPublishedCatalogNormalizesCNYToUSDIdempotently(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	svc := NewService(db)

	report, err := svc.ApplyPublishedCatalogWithReport(context.Background(), cnyCatalog())
	require.NoError(t, err)
	require.Equal(t, int64(7), report.Version)
	require.Equal(t, 1, report.Normalized)
	require.Len(t, report.Models, 1)

	diff := report.Models[0]
	require.Equal(t, "agicto", diff.Provider)
	require.Equal(t, "gpt-x", diff.Model)
	require.Equal(t, "agicto", diff.ProviderKey)
	require.Equal(t, "agicto", diff.VendorKey)
	require.Equal(t, "CNY", diff.CurrencyBefore)
	require.Equal(t, "USD", diff.CurrencyAfter)
	require.True(t, diff.IdentityChanged)
	require.Contains(t, diff.UnitChanges, "per_1m_tokens")

	var inputChange *CatalogPriceChange
	for i := range diff.PriceChanges {
		if diff.PriceChanges[i].Field == "input_price" {
			inputChange = &diff.PriceChanges[i]
		}
	}
	require.NotNil(t, inputChange, "diff report must include the input price change")
	require.Equal(t, "14", inputChange.Before)
	require.Equal(t, "2", inputChange.After)

	var stored struct {
		Currency    string
		InputPrice  float64
		OutputPrice float64
		Pricing     string
	}
	require.NoError(t, db.Table("llm_models").
		Select("currency, input_price, output_price, pricing").
		Where("provider = ? AND name = ?", "agicto", "gpt-x").
		Take(&stored).Error)
	require.Equal(t, "USD", stored.Currency)
	require.InDelta(t, 2, stored.InputPrice, 1e-9)
	require.InDelta(t, 10, stored.OutputPrice, 1e-9)

	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stored.Pricing), &pricing))
	require.Equal(t, "7", pricing["price_exchange_rate"])
	require.Equal(t, "converted_from_cny", pricing["price_currency_status"])
	require.Equal(t, "console_system", pricing["rate_source"])
	source := pricing["source"].(map[string]interface{})
	require.Equal(t, "CNY", source["currency"])
	require.Equal(t, "7", source["rate"])
	require.Equal(t, "14", source["amount"])
	identity := pricing["identity"].(map[string]interface{})
	require.Equal(t, "agicto", identity["provider_key"])
	require.Equal(t, "agicto", identity["vendor_key"])
	tier := pricing["token_tiers"].([]interface{})[0].(map[string]interface{})
	require.InDelta(t, 2, tier["input_price_per_million"], 1e-9)
	require.InDelta(t, 10, tier["output_price_per_million"], 1e-9)
	rule := pricing["rules"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "2", rule["price"])

	// Replaying the same immutable snapshot must not convert a second time.
	secondReport, err := svc.ApplyPublishedCatalogWithReport(context.Background(), cnyCatalog())
	require.NoError(t, err)
	require.Equal(t, 1, secondReport.Normalized)

	var storedAgain struct {
		Currency    string
		InputPrice  float64
		OutputPrice float64
		Pricing     string
	}
	require.NoError(t, db.Table("llm_models").
		Select("currency, input_price, output_price, pricing").
		Where("provider = ? AND name = ?", "agicto", "gpt-x").
		Take(&storedAgain).Error)
	require.Equal(t, stored, storedAgain)
}

// T19: a CNY snapshot without a carried rate fails closed instead of silently
// treating CNY as USD.
func TestApplyPublishedCatalogFailsClosedWithoutCNYRate(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	svc := NewService(db)

	model := cnyCatalogModel()
	model.Pricing = json.RawMessage(`{"deployment_scope":"global","pricing_version":"v1"}`)
	catalog := cnyCatalog()
	catalog.Models = []PublishedModel{model}

	report, err := svc.ApplyPublishedCatalogWithReport(context.Background(), catalog)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCatalogExchangeRateUnavailable), "error = %v", err)
	require.Zero(t, report.Version)

	var count int64
	require.NoError(t, db.Table("llm_models").
		Where("provider = ? AND name = ?", "agicto", "gpt-x").Count(&count).Error)
	require.Zero(t, count, "a fail-closed normalization must not persist a half-written model")
}

// T19 acceptance 4: a locally authored private price is never overwritten by
// the published catalog.
func TestApplyPublishedCatalogDoesNotOverwriteLocalPrivatePrice(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	localPricing := `{"catalog_origin":"pre_custom","deployment_scope":"organization","rules":[{"operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"9","configured":true}]}`
	require.NoError(t, db.Table("llm_models").Create(map[string]interface{}{
		"id":                "local-model-id",
		"provider":          "agicto",
		"name":              "gpt-x",
		"display_name":      "Local GPT X",
		"status":            llmmodel.ModelStatusActive,
		"currency":          "USD",
		"input_price":       9,
		"output_price":      9,
		"pricing":           localPricing,
		"is_active":         true,
		"is_system_enabled": true,
		"created_at":        time.Now().UTC(),
		"updated_at":        time.Now().UTC(),
	}).Error)

	svc := NewService(db)
	_, err := svc.ApplyPublishedCatalogWithReport(context.Background(), cnyCatalog())
	require.NoError(t, err)

	var stored struct {
		ID         string
		Currency   string
		InputPrice float64
		Pricing    string
		Status     string
	}
	require.NoError(t, db.Table("llm_models").
		Select("id, currency, input_price, pricing, status").
		Where("provider = ? AND name = ?", "agicto", "gpt-x").
		Take(&stored).Error)
	require.Equal(t, "local-model-id", stored.ID)
	require.Equal(t, "USD", stored.Currency)
	require.InDelta(t, 9, stored.InputPrice, 1e-9)
	require.Contains(t, stored.Pricing, "pre_custom")
	require.NotContains(t, stored.Pricing, "converted_from_cny")
	require.Equal(t, llmmodel.ModelStatusActive, stored.Status)
}

// T19 acceptance 1: the direct ModelMeta sync write path uses the same
// normalization boundary as the catalog apply path.
func TestNormalizePublishedModelForWriteConvertsCNY(t *testing.T) {
	normalized, err := normalizePublishedModelForWrite(cnyCatalogModel())
	require.NoError(t, err)
	require.Equal(t, "USD", normalized.Currency)
	require.InDelta(t, 2, normalized.InputPrice, 1e-9)

	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal(normalized.Pricing, &pricing))
	require.Equal(t, "converted_from_cny", pricing["price_currency_status"])
}

// A USD model keeps its exact pricing bytes so the existing snapshot contract
// is unchanged.
func TestNormalizePublishedModelKeepsUSDWithoutVendor(t *testing.T) {
	raw := json.RawMessage(`{"deployment_scope":"global","token_tiers":[{"min_input_tokens":0,"input_price_per_million":5}]}`)
	normalized, err := normalizePublishedModelForWrite(PublishedModel{
		Provider: "openai",
		Model:    "gpt-5.5",
		Currency: "USD",
		Pricing:  raw,
	})
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(normalized.Pricing))
	require.Equal(t, "USD", normalized.Currency)
}
