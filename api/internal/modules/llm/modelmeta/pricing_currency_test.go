package modelmeta

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCatalogPriceCurrencyConvertsCNYToBillingUSD(t *testing.T) {
	input := 2.0
	output := 8.0
	service := &Service{cnyPerUSD: 7}
	model := &ModelMetaData{
		Currency:         "CNY",
		InputPrice:       &input,
		OutputPrice:      &output,
		CachedInputPrice: 0.7,
		Pricing:          json.RawMessage(`{"deployment_scope":"china-mainland"}`),
	}

	service.normalizeCatalogPriceCurrency(model)

	require.Equal(t, "USD", model.Currency)
	require.InDelta(t, 2.0/7.0, *model.InputPrice, 0.0000001)
	require.InDelta(t, 8.0/7.0, *model.OutputPrice, 0.0000001)
	require.InDelta(t, 0.1, model.CachedInputPrice, 0.0000001)
	var pricing map[string]interface{}
	require.NoError(t, json.Unmarshal(model.Pricing, &pricing))
	require.Equal(t, "CNY", pricing["source_currency"])
	require.Equal(t, "USD", pricing["billing_currency"])
	require.Equal(t, float64(7), pricing["cny_per_usd"])
}

func TestNormalizeCatalogPriceCurrencyLeavesUSDUnchanged(t *testing.T) {
	input := 1.25
	service := &Service{cnyPerUSD: 7}
	model := &ModelMetaData{Currency: "USD", InputPrice: &input}

	service.normalizeCatalogPriceCurrency(model)

	require.Equal(t, "USD", model.Currency)
	require.Equal(t, 1.25, *model.InputPrice)
}

func TestNormalizeCatalogPriceCurrencyUsesSafeDefaultRate(t *testing.T) {
	input := 7.0
	service := &Service{}
	model := &ModelMetaData{Currency: "CNY", InputPrice: &input}

	service.normalizeCatalogPriceCurrency(model)

	require.Equal(t, 1.0, *model.InputPrice)
}
