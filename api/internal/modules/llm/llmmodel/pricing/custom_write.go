package pricing

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

var errInvalidPrice = errors.New("invalid price")

const customCatalogOrigin = "pre_custom"

// CustomModelScalars carries scalar token prices submitted by custom model forms.
type CustomModelScalars struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheReadPrice    *float64
	CacheWritePrice   *float64
	CacheWrite5mPrice *float64
	CacheWrite1hPrice *float64
}

// PrepareCustomModelPricing normalizes inbound pricing, synthesizes rules[] when
// missing, derives legacy projections, and projects scalar columns for Pre's
// existing quote paths. overrides carries the price columns explicitly present
// in the request so scalar edits stay authoritative over stored rules.
func PrepareCustomModelPricing(
	raw json.RawMessage,
	currency string,
	useCases []string,
	scalars CustomModelScalars,
	overrides CustomModelScalars,
) (json.RawMessage, CustomModelScalars, error) {
	legacyScalars := LegacyScalarPrices{
		InputPrice:        scalars.InputPrice,
		OutputPrice:       scalars.OutputPrice,
		CacheReadPrice:    scalars.CacheReadPrice,
		CacheWritePrice:   scalars.CacheWritePrice,
		CacheWrite5mPrice: scalars.CacheWrite5mPrice,
		CacheWrite1hPrice: scalars.CacheWrite1hPrice,
	}
	opts := PricingWriteOptions{
		UseCases:     useCases,
		UserAuthored: true,
	}
	if legacyOverrides := CustomScalarsToLegacy(overrides); legacyScalarsHaveValues(legacyOverrides) {
		opts.ScalarOverrides = &legacyOverrides
	}
	prepared := PrepareStructuredPricingForWrite(raw, currency, legacyScalars, opts)
	if len(prepared) == 0 || string(prepared) == "null" {
		return prepared, scalars, nil
	}
	if !pricingHasAnyPrice(prepared) {
		// The tenant cleared the last price. Persist "not configured" instead of
		// a valueless envelope: leaving the stored pricing untouched would keep
		// billing the previous image/video price, and the strict validator would
		// reject a valueless envelope as half-written.
		return json.RawMessage(`{}`), scalars, nil
	}
	if _, err := ValidateStructuredPricing(prepared); err != nil {
		return nil, scalars, err
	}
	var envelope structuredPricing
	if err := json.Unmarshal(prepared, &envelope); err != nil {
		return nil, scalars, err
	}
	if strings.TrimSpace(envelope.CatalogOrigin) == "" {
		envelope.CatalogOrigin = customCatalogOrigin
	}
	if strings.TrimSpace(envelope.DeploymentScope) == "" {
		envelope.DeploymentScope = "organization"
	}
	next, err := json.Marshal(envelope)
	if err != nil {
		return nil, scalars, err
	}
	derivedScalars := DeriveScalarsFromPricing(next)
	return next, customScalarsFromLegacy(derivedScalars), nil
}

// OptionalFloatFromString parses an optional non-negative model price string.
func OptionalFloatFromString(value string) (*float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	price, err := decimal.NewFromString(trimmed)
	if err != nil {
		return nil, err
	}
	if price.IsNegative() {
		return nil, errInvalidPrice
	}
	f, _ := price.Float64()
	return &f, nil
}

func customScalarsFromLegacy(scalars LegacyScalarPrices) CustomModelScalars {
	return CustomModelScalars{
		InputPrice:        scalars.InputPrice,
		OutputPrice:       scalars.OutputPrice,
		CacheReadPrice:    scalars.CacheReadPrice,
		CacheWritePrice:   scalars.CacheWritePrice,
		CacheWrite5mPrice: scalars.CacheWrite5mPrice,
		CacheWrite1hPrice: scalars.CacheWrite1hPrice,
	}
}

// CustomScalarsToLegacy converts request-level scalar prices into the legacy
// scalar shape used for rule synthesis and overrides.
func CustomScalarsToLegacy(scalars CustomModelScalars) LegacyScalarPrices {
	return LegacyScalarPrices{
		InputPrice:        scalars.InputPrice,
		OutputPrice:       scalars.OutputPrice,
		CacheReadPrice:    scalars.CacheReadPrice,
		CachedInputPrice:  scalars.CacheReadPrice,
		CacheWritePrice:   scalars.CacheWritePrice,
		CacheWrite5mPrice: scalars.CacheWrite5mPrice,
		CacheWrite1hPrice: scalars.CacheWrite1hPrice,
	}
}

func ScalarsFromDecimalPrices(
	inputPrice, outputPrice decimal.Decimal,
	cacheRead, cacheWrite, cacheWrite5m, cacheWrite1h decimal.Decimal,
	input, output, cacheReadConfigured, cacheWriteConfigured, cacheWrite5mConfigured, cacheWrite1hConfigured bool,
) CustomModelScalars {
	scalars := CustomModelScalars{}
	if input {
		scalars.InputPrice = DecimalPtr(inputPrice)
	}
	if output {
		scalars.OutputPrice = DecimalPtr(outputPrice)
	}
	if cacheReadConfigured {
		scalars.CacheReadPrice = DecimalPtr(cacheRead)
	}
	if cacheWriteConfigured {
		scalars.CacheWritePrice = DecimalPtr(cacheWrite)
	}
	if cacheWrite5mConfigured {
		scalars.CacheWrite5mPrice = DecimalPtr(cacheWrite5m)
	}
	if cacheWrite1hConfigured {
		scalars.CacheWrite1hPrice = DecimalPtr(cacheWrite1h)
	}
	return scalars
}

// pricingHasAnyPrice reports whether a pricing envelope still carries at least
// one price, mirroring the structural validator's notion of "priced".
func pricingHasAnyPrice(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var envelope structuredPricing
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		// Malformed payloads are left for the validator to report.
		return true
	}
	return len(envelope.Rules) > 0 ||
		len(envelope.TokenTiers) > 0 ||
		envelope.PricePerImage != nil ||
		envelope.VideoGeneration != nil ||
		len(envelope.Metered) > 0
}
