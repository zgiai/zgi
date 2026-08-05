package modelmeta

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCatalogBillingPriceKeepsNativeUSDPrice(t *testing.T) {
	input := 1.25
	model := &ModelMetaData{Currency: "USD", InputPrice: &input}

	normalizeCatalogBillingPrice(model)

	require.Equal(t, "USD", model.Currency)
	require.Equal(t, 1.25, *model.InputPrice)
}

func TestNormalizeCatalogBillingPriceUsesCatalogCanonicalUSD(t *testing.T) {
	nativeInput := 2.0
	nativeOutput := 8.0
	billingInput := 0.285714
	billingOutput := 1.142857
	model := &ModelMetaData{
		Currency:    "CNY",
		InputPrice:  &nativeInput,
		OutputPrice: &nativeOutput,
		Pricing:     json.RawMessage(`{"deployment_scope":"china-mainland"}`),
		BillingPricing: &ModelMetaBillingPrice{
			Currency:    "USD",
			InputPrice:  &billingInput,
			OutputPrice: &billingOutput,
			Conversion:  json.RawMessage(`{"source":"catalog-fx","as_of":"2026-08-05"}`),
		},
	}

	normalizeCatalogBillingPrice(model)

	require.Equal(t, "USD", model.Currency)
	require.Equal(t, billingInput, *model.InputPrice)
	require.Equal(t, billingOutput, *model.OutputPrice)
	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal(model.Pricing, &pricing))
	require.Contains(t, pricing, "native_price")
	require.Contains(t, pricing, "billing_price")
}

func TestNormalizeCatalogBillingPriceDoesNotRelabelStructuredCNYAmountsAsUSD(t *testing.T) {
	nativeInput := 2.0
	billingInput := 0.285714
	model := &ModelMetaData{
		Currency:   "CNY",
		InputPrice: &nativeInput,
		Pricing: json.RawMessage(`{
			"token_tiers":[{"up_to":1000000,"input_price":2}],
			"image_price":0.2
		}`),
		BillingPricing: &ModelMetaBillingPrice{
			Currency:   "USD",
			InputPrice: &billingInput,
			Conversion: json.RawMessage(`{"source":"catalog-fx","as_of":"2026-08-05"}`),
		},
	}

	normalizeCatalogBillingPrice(model)

	require.Equal(t, "USD", model.Currency)
	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal(model.Pricing, &pricing))
	require.NotContains(t, pricing, "token_tiers")
	require.NotContains(t, pricing, "image_price")
	nativePrice := pricing["native_price"].(map[string]interface{})
	require.Equal(t, "CNY", nativePrice["currency"])
	details := nativePrice["details"].(map[string]interface{})
	require.Contains(t, details, "token_tiers")
	require.Equal(t, 0.2, details["image_price"])
	billingPrice := pricing["billing_price"].(map[string]interface{})
	require.Equal(t, "USD", billingPrice["currency"])
}

func TestNormalizeCatalogBillingPriceDoesNotGuessForeignExchangeRate(t *testing.T) {
	input := 2.0
	output := 8.0
	model := &ModelMetaData{
		Currency:    "CNY",
		InputPrice:  &input,
		OutputPrice: &output,
	}

	normalizeCatalogBillingPrice(model)

	require.Equal(t, "CNY", model.Currency)
	require.Nil(t, model.InputPrice)
	require.Nil(t, model.OutputPrice)
	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal(model.Pricing, &pricing))
	require.Contains(t, pricing, "native_price")
}

func TestNormalizeCatalogBillingPriceRejectsNonUSDBillingPrice(t *testing.T) {
	nativeInput := 100.0
	billingInput := 0.62
	model := &ModelMetaData{
		Currency:   "JPY",
		InputPrice: &nativeInput,
		BillingPricing: &ModelMetaBillingPrice{
			Currency:   "EUR",
			InputPrice: &billingInput,
		},
	}

	normalizeCatalogBillingPrice(model)

	require.Nil(t, model.InputPrice)
	require.Equal(t, "JPY", model.Currency)
}

func TestNormalizeCatalogBillingPriceRejectsUnspecifiedCurrency(t *testing.T) {
	input := 1.0
	model := &ModelMetaData{InputPrice: &input}

	normalizeCatalogBillingPrice(model)

	require.Nil(t, model.InputPrice)
}

func TestNormalizeCatalogBillingPriceRequiresConversionProvenance(t *testing.T) {
	nativeInput := 2.0
	billingInput := 0.285714
	model := &ModelMetaData{
		Currency:   "CNY",
		InputPrice: &nativeInput,
		BillingPricing: &ModelMetaBillingPrice{
			Currency:   "USD",
			InputPrice: &billingInput,
		},
	}

	normalizeCatalogBillingPrice(model)

	require.Equal(t, "CNY", model.Currency)
	require.Nil(t, model.InputPrice)
}
