package modelmeta

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
)

// Pre has no price_currency_status / price_exchange_rate columns, so the
// normalization classification and rate source travel inside the existing
// pricing JSON (rating spec v1, section 4).
const (
	// priceCurrencyStatusConvertedFromCNY is written into the pricing JSON when
	// this task folded a CNY snapshot into canonical USD.
	priceCurrencyStatusConvertedFromCNY = "converted_from_cny"

	// catalogRateSourceConsoleSystem labels a rate authored by the Console
	// system configuration, mirroring the rating spec rate_source vocabulary.
	catalogRateSourceConsoleSystem = "console_system"

	priceCurrencyUSD = "USD"
	priceCurrencyCNY = "CNY"
)

// ErrCatalogExchangeRateUnavailable marks a CNY catalog price that cannot be
// normalized because no conversion rate is carried by the snapshot. It is
// fail-closed: the catalog apply must not silently treat CNY as USD.
var ErrCatalogExchangeRateUnavailable = errors.New("catalog exchange rate is unavailable")

// CatalogPriceChange is one price conversion inside a catalog model. Field is
// the pricing path (for example "input_price" or "rules[0].price").
type CatalogPriceChange struct {
	Field  string `json:"field"`
	Unit   string `json:"unit,omitempty"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// CatalogModelDiff reports the normalization applied to one catalog model. It
// is locatable by provider+model and carries price, unit, currency, and
// normalized identity keys.
type CatalogModelDiff struct {
	Provider        string               `json:"provider"`
	Model           string               `json:"model"`
	ProviderKey     string               `json:"provider_key"`
	VendorKey       string               `json:"vendor_key"`
	CurrencyBefore  string               `json:"currency_before,omitempty"`
	CurrencyAfter   string               `json:"currency_after,omitempty"`
	PriceChanges    []CatalogPriceChange `json:"price_changes,omitempty"`
	UnitChanges     []string             `json:"unit_changes,omitempty"`
	IdentityChanged bool                 `json:"identity_changed"`
}

// CatalogApplyReport is the diff report emitted while applying a Console
// catalog snapshot. Normalized counts models whose currency was folded to USD.
type CatalogApplyReport struct {
	Version    int64              `json:"version"`
	AppliedAt  time.Time          `json:"applied_at"`
	Normalized int                `json:"normalized"`
	Models     []CatalogModelDiff `json:"models,omitempty"`
}

// normalizePublishedCatalog folds every model in a Console snapshot into
// canonical USD, records the diff report, and injects normalized identity keys.
// It is a pure transformation of the snapshot and is therefore idempotent.
func normalizePublishedCatalog(catalog PublishedCatalog) (PublishedCatalog, CatalogApplyReport, error) {
	report := CatalogApplyReport{Version: catalog.Version, AppliedAt: time.Now().UTC()}
	normalized := catalog
	normalized.Models = make([]PublishedModel, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		next, diff, err := normalizePublishedModel(model)
		if err != nil {
			return PublishedCatalog{}, CatalogApplyReport{}, fmt.Errorf("normalize catalog model %s/%s: %w", model.Provider, model.Model, err)
		}
		normalized.Models = append(normalized.Models, next)
		if diff == nil {
			continue
		}
		if diff.CurrencyBefore != "" && !strings.EqualFold(diff.CurrencyBefore, diff.CurrencyAfter) {
			report.Normalized++
		}
		if diff.CurrencyBefore == "" && !diff.IdentityChanged {
			// Nothing monetary or identity-bearing changed for this model.
			continue
		}
		report.Models = append(report.Models, *diff)
	}
	return normalized, report, nil
}

// normalizePublishedModelForWrite is the write-boundary entry point shared by
// the catalog apply path and Pre's direct ModelMeta sync path.
func normalizePublishedModelForWrite(model PublishedModel) (PublishedModel, error) {
	normalized, _, err := normalizePublishedModel(model)
	return normalized, err
}

// normalizePublishedModel converts a single model's CNY prices to USD exactly
// once and records the normalization source/status inside the pricing JSON.
func normalizePublishedModel(model PublishedModel) (PublishedModel, *CatalogModelDiff, error) {
	diff := &CatalogModelDiff{
		Provider:    model.Provider,
		Model:       model.Model,
		ProviderKey: channelprovider.NormalizeIdentityKey(model.Provider),
		VendorKey:   channelprovider.NormalizeIdentityKey(model.Vendor),
	}

	switch currency := strings.ToUpper(strings.TrimSpace(model.Currency)); currency {
	case "", priceCurrencyUSD:
		model.Currency = priceCurrencyUSD
	case priceCurrencyCNY:
		if status := publishedPricingCurrencyStatus(model.Pricing); status != "" {
			// The snapshot already records its normalization outcome. Trust it
			// and only relabel so a replayed snapshot is idempotent.
			model.Currency = priceCurrencyUSD
			diff.CurrencyBefore = priceCurrencyCNY
			diff.CurrencyAfter = priceCurrencyUSD
			break
		}
		rate, ok := extractCatalogExchangeRate(model.Pricing)
		if !ok {
			return model, nil, fmt.Errorf("%w: %s/%s", ErrCatalogExchangeRateUnavailable, model.Provider, model.Model)
		}
		converted, changes, units, err := convertPublishedPricingCNYToUSD(model, rate)
		if err != nil {
			return model, nil, err
		}
		model = converted
		model.Currency = priceCurrencyUSD
		diff.CurrencyBefore = priceCurrencyCNY
		diff.CurrencyAfter = priceCurrencyUSD
		diff.PriceChanges = changes
		diff.UnitChanges = units
	default:
		return model, nil, fmt.Errorf("unsupported catalog pricing currency %q for %s/%s", currency, model.Provider, model.Model)
	}

	if strings.TrimSpace(model.Vendor) != "" {
		pricing, err := injectCatalogIdentityKeys(model.Pricing, diff.ProviderKey, diff.VendorKey)
		if err != nil {
			return model, nil, err
		}
		model.Pricing = pricing
		// A key is "changed" only when folding actually rewrote the spelling,
		// so a plain same-name vendor does not flood the diff report.
		rawProvider := strings.ToLower(strings.TrimSpace(model.Provider))
		rawVendor := strings.ToLower(strings.TrimSpace(model.Vendor))
		diff.IdentityChanged = diff.ProviderKey != rawProvider || diff.VendorKey != rawVendor
	}
	return model, diff, nil
}

var structuredPricingNumberKeys = map[string]struct{}{
	"price_per_image": {}, "input_price_per_million": {}, "cached_input_price_per_million": {},
	"cache_write_price_per_million": {}, "cache_write_5m_price_per_million": {}, "cache_write_1h_price_per_million": {},
	"output_price_per_million": {}, "batch_input_price_per_million": {}, "batch_output_price_per_million": {},
	"text_input_price_per_million": {}, "image_input_price_per_million": {},
	"price_per_second": {}, "price_per_task": {}, "price_per_million_tokens": {},
}

// convertPublishedPricingCNYToUSD divides every known price by the USD-to-CNY
// rate once and records the author's original amount for audit only.
func convertPublishedPricingCNYToUSD(model PublishedModel, rate decimal.Decimal) (PublishedModel, []CatalogPriceChange, []string, error) {
	changes := make([]CatalogPriceChange, 0, 8)
	amount := firstCatalogSourceAmount(model)

	if model.InputPriceConfigured {
		model.InputPrice = convertCatalogScalar(model.InputPrice, "input_price", rate, &changes)
	}
	if model.OutputPriceConfigured {
		model.OutputPrice = convertCatalogScalar(model.OutputPrice, "output_price", rate, &changes)
	}
	if model.CachedInputPrice != 0 {
		model.CachedInputPrice = convertCatalogScalar(model.CachedInputPrice, "cached_input_price", rate, &changes)
	}
	model.CacheReadPrice = convertCatalogScalarPtr(model.CacheReadPrice, "cache_read_price", rate, &changes)
	model.CacheWritePrice = convertCatalogScalarPtr(model.CacheWritePrice, "cache_write_price", rate, &changes)
	model.CacheWrite5mPrice = convertCatalogScalarPtr(model.CacheWrite5mPrice, "cache_write_5m_price", rate, &changes)
	model.CacheWrite1hPrice = convertCatalogScalarPtr(model.CacheWrite1hPrice, "cache_write_1h_price", rate, &changes)

	trimmed := strings.TrimSpace(string(model.Pricing))
	if trimmed == "" || trimmed == "null" {
		return model, changes, nil, nil
	}
	var object map[string]interface{}
	if err := json.Unmarshal(model.Pricing, &object); err != nil {
		return model, nil, nil, fmt.Errorf("decode catalog pricing: %w", err)
	}
	originalSource, _ := object["source"].(map[string]interface{})
	rateAt := catalogConversionRateAt(originalSource)
	delete(object, "source")
	delete(object, "price_exchange_rate")
	delete(object, "price_currency_status")
	if err := convertStructuredPricingTree(object, priceCurrencyCNY, priceCurrencyUSD, rate, "", &changes); err != nil {
		return model, nil, nil, err
	}
	object["price_exchange_rate"] = rate.String()
	object["price_currency_status"] = priceCurrencyStatusConvertedFromCNY
	object["rate_source"] = catalogRateSourceConsoleSystem
	object["source"] = catalogConversionSource(rate, rateAt, amount)
	data, err := json.Marshal(object)
	if err != nil {
		return model, nil, nil, err
	}
	model.Pricing = data
	return model, changes, collectPublishedPricingUnits(object), nil
}

// convertStructuredPricingTree mirrors the Console conversion contract: every
// numeric price uses the nearest explicit currency object, and nested currency
// labels are rewritten to the canonical settlement currency.
func convertStructuredPricingTree(
	node interface{},
	inheritedCurrency, targetCurrency string,
	rate decimal.Decimal,
	path string,
	changes *[]CatalogPriceChange,
) error {
	switch typed := node.(type) {
	case []interface{}:
		for index, item := range typed {
			childPath := fmt.Sprintf("%s[%d]", path, index)
			if err := convertStructuredPricingTree(item, inheritedCurrency, targetCurrency, rate, childPath, changes); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		localCurrency, err := resolvePricingCurrency(typed, inheritedCurrency, targetCurrency)
		if err != nil {
			return err
		}
		unit := pricingUnit(typed)
		for _, key := range sortedJSONKeys(typed) {
			if key == "currency" || key == "source" {
				continue
			}
			child := typed[key]
			fieldPath := key
			if path != "" {
				fieldPath = path + "." + key
			}
			converted, handled, err := convertPricingEntry(key, child, localCurrency, targetCurrency, rate, fieldPath, unit, changes)
			if err != nil {
				return err
			}
			if handled {
				typed[key] = converted
				continue
			}
			if err := convertStructuredPricingTree(child, localCurrency, targetCurrency, rate, fieldPath, changes); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolvePricingCurrency applies the nearest explicit currency label and
// canonicalizes nested amount/rates objects that omitted one.
func resolvePricingCurrency(object map[string]interface{}, inheritedCurrency, targetCurrency string) (string, error) {
	localCurrency := inheritedCurrency
	if currency, ok := object["currency"].(string); ok && strings.TrimSpace(currency) != "" {
		normalized := strings.ToUpper(strings.TrimSpace(currency))
		if normalized != priceCurrencyUSD && normalized != priceCurrencyCNY {
			return "", fmt.Errorf("unsupported catalog pricing currency %q", currency)
		}
		localCurrency = normalized
		object["currency"] = targetCurrency
	}
	for _, key := range []string{"amount", "rates", "image_input_price_per_million"} {
		if _, present := object[key]; !present {
			continue
		}
		if _, hasCurrency := object["currency"]; !hasCurrency {
			object["currency"] = targetCurrency
		}
	}
	return localCurrency, nil
}

// convertPricingEntry converts one keyed price node. handled reports whether
// the key was a known price shape.
func convertPricingEntry(
	key string,
	child interface{},
	localCurrency, targetCurrency string,
	rate decimal.Decimal,
	fieldPath, unit string,
	changes *[]CatalogPriceChange,
) (interface{}, bool, error) {
	switch key {
	case "amount":
		text, ok := child.(string)
		if !ok {
			return child, false, nil
		}
		if strings.TrimSpace(text) == "" {
			return child, true, nil
		}
		parsed, err := decimal.NewFromString(strings.TrimSpace(text))
		if err != nil {
			return child, false, fmt.Errorf("decode catalog pricing amount %q: %w", text, err)
		}
		converted := convertCatalogDecimal(parsed, localCurrency, targetCurrency, rate)
		recordCatalogPriceChange(changes, fieldPath, unit, parsed, converted)
		return converted.String(), true, nil
	case "price":
		switch typedChild := child.(type) {
		case string:
			if strings.TrimSpace(typedChild) == "" {
				return child, true, nil
			}
			parsed, err := decimal.NewFromString(strings.TrimSpace(typedChild))
			if err != nil {
				return child, false, fmt.Errorf("decode catalog pricing price %q: %w", typedChild, err)
			}
			converted := convertCatalogDecimal(parsed, localCurrency, targetCurrency, rate)
			recordCatalogPriceChange(changes, fieldPath, unit, parsed, converted)
			return converted.String(), true, nil
		case float64:
			parsed := decimal.NewFromFloat(typedChild)
			converted := convertCatalogDecimal(parsed, localCurrency, targetCurrency, rate)
			recordCatalogPriceChange(changes, fieldPath, unit, parsed, converted)
			return converted.InexactFloat64(), true, nil
		}
		return child, false, nil
	}
	if _, ok := structuredPricingNumberKeys[key]; ok {
		number, ok := child.(float64)
		if !ok {
			return child, false, nil
		}
		parsed := decimal.NewFromFloat(number)
		converted := convertCatalogDecimal(parsed, localCurrency, targetCurrency, rate)
		recordCatalogPriceChange(changes, fieldPath, unit, parsed, converted)
		return converted.InexactFloat64(), true, nil
	}
	return child, false, nil
}

func recordCatalogPriceChange(changes *[]CatalogPriceChange, field, unit string, before, after decimal.Decimal) {
	*changes = append(*changes, CatalogPriceChange{Field: field, Unit: unit, Before: before.String(), After: after.String()})
}

func sortedJSONKeys(object map[string]interface{}) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func convertCatalogDecimal(value decimal.Decimal, fromCurrency, toCurrency string, rate decimal.Decimal) decimal.Decimal {
	if fromCurrency == toCurrency {
		return value
	}
	if fromCurrency == priceCurrencyUSD && toCurrency == priceCurrencyCNY {
		return value.Mul(rate)
	}
	if fromCurrency == priceCurrencyCNY && toCurrency == priceCurrencyUSD {
		return value.Div(rate)
	}
	return value
}

func convertCatalogScalar(value float64, field string, rate decimal.Decimal, changes *[]CatalogPriceChange) float64 {
	parsed := decimal.NewFromFloat(value)
	converted := parsed.Div(rate)
	*changes = append(*changes, CatalogPriceChange{Field: field, Before: parsed.String(), After: converted.String()})
	result, _ := converted.Float64()
	return result
}

func convertCatalogScalarPtr(value *float64, field string, rate decimal.Decimal, changes *[]CatalogPriceChange) *float64 {
	if value == nil {
		return nil
	}
	converted := convertCatalogScalar(*value, field, rate, changes)
	return &converted
}

// extractCatalogExchangeRate reads the conversion rate already carried by the
// snapshot pricing JSON. Pre never invents a rate; a missing rate stays
// fail-closed.
func extractCatalogExchangeRate(raw json.RawMessage) (decimal.Decimal, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return decimal.Zero, false
	}
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return decimal.Zero, false
	}
	if rate, ok := catalogDecimalValue(object["price_exchange_rate"]); ok && rate.IsPositive() {
		return rate, true
	}
	if source, ok := object["source"].(map[string]interface{}); ok {
		if rate, ok := catalogDecimalValue(source["rate"]); ok && rate.IsPositive() {
			return rate, true
		}
	}
	return decimal.Zero, false
}

func publishedPricingCurrencyStatus(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	status, _ := object["price_currency_status"].(string)
	return strings.TrimSpace(status)
}

func injectCatalogIdentityKeys(raw json.RawMessage, providerKey, vendorKey string) (json.RawMessage, error) {
	object := map[string]interface{}{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal(raw, &object); err != nil {
			return raw, fmt.Errorf("decode catalog pricing for identity keys: %w", err)
		}
	}
	object["identity"] = map[string]interface{}{
		"provider_key": providerKey,
		"vendor_key":   vendorKey,
	}
	data, err := json.Marshal(object)
	if err != nil {
		return raw, err
	}
	return data, nil
}

func catalogConversionSource(rate decimal.Decimal, rateAt, amount string) map[string]interface{} {
	if rateAt == "" {
		rateAt = time.Now().UTC().Format(time.RFC3339)
	}
	source := map[string]interface{}{
		"currency": priceCurrencyCNY,
		"rate":     rate.String(),
		"rate_at":  rateAt,
	}
	if strings.TrimSpace(amount) != "" {
		source["amount"] = amount
	}
	return source
}

func catalogConversionRateAt(source map[string]interface{}) string {
	if source == nil {
		return ""
	}
	rateAt, _ := source["rate_at"].(string)
	return strings.TrimSpace(rateAt)
}

// firstCatalogSourceAmount returns the first author price found in the
// snapshot so the audit source keeps the original CNY amount.
func firstCatalogSourceAmount(model PublishedModel) string {
	if model.InputPriceConfigured {
		return decimal.NewFromFloat(model.InputPrice).String()
	}
	if model.OutputPriceConfigured {
		return decimal.NewFromFloat(model.OutputPrice).String()
	}
	for _, candidate := range []*float64{model.CacheReadPrice, model.CacheWritePrice, model.CacheWrite5mPrice, model.CacheWrite1hPrice} {
		if candidate != nil {
			return decimal.NewFromFloat(*candidate).String()
		}
	}
	trimmed := strings.TrimSpace(string(model.Pricing))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal(model.Pricing, &value); err != nil {
		return ""
	}
	return firstStructuredPricingAmount(value)
}

func firstStructuredPricingAmount(value interface{}) string {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if amount := firstStructuredPricingAmount(item); amount != "" {
				return amount
			}
		}
	case map[string]interface{}:
		for _, key := range sortedJSONKeys(typed) {
			if key == "source" {
				continue
			}
			child := typed[key]
			if amount, ok := structuredPriceAmount(key, child); ok {
				return amount
			}
			if amount := firstStructuredPricingAmount(child); amount != "" {
				return amount
			}
		}
	}
	return ""
}

// structuredPriceAmount decodes a JSON value that carries a price for key.
func structuredPriceAmount(key string, child interface{}) (string, bool) {
	switch key {
	case "amount":
		if text, ok := child.(string); ok {
			if parsed, err := decimal.NewFromString(strings.TrimSpace(text)); err == nil {
				return parsed.String(), true
			}
		}
	case "price":
		if text, ok := child.(string); ok {
			if parsed, err := decimal.NewFromString(strings.TrimSpace(text)); err == nil {
				return parsed.String(), true
			}
		}
		if number, ok := child.(float64); ok {
			return decimal.NewFromFloat(number).String(), true
		}
	default:
		if _, ok := structuredPricingNumberKeys[key]; ok {
			if number, ok := child.(float64); ok {
				return decimal.NewFromFloat(number).String(), true
			}
		}
	}
	return "", false
}

func collectPublishedPricingUnits(value interface{}) []string {
	found := map[string]struct{}{}
	collectPricingUnits(value, found)
	units := make([]string, 0, len(found))
	for unit := range found {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}

func collectPricingUnits(value interface{}, found map[string]struct{}) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			collectPricingUnits(item, found)
		}
	case map[string]interface{}:
		for _, key := range []string{"unit", "base_unit", "billing_unit"} {
			if raw, ok := typed[key].(string); ok {
				if unit := strings.TrimSpace(raw); unit != "" {
					found[unit] = struct{}{}
				}
			}
		}
		for _, child := range typed {
			collectPricingUnits(child, found)
		}
	}
}

func pricingUnit(object map[string]interface{}) string {
	for _, key := range []string{"unit", "base_unit", "billing_unit"} {
		if raw, ok := object[key].(string); ok {
			if unit := strings.TrimSpace(raw); unit != "" {
				return unit
			}
		}
	}
	return ""
}

func catalogDecimalValue(value interface{}) (decimal.Decimal, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := decimal.NewFromString(strings.TrimSpace(typed))
		return parsed, err == nil
	case float64:
		return decimal.NewFromFloat(typed), true
	case json.Number:
		parsed, err := decimal.NewFromString(typed.String())
		return parsed, err == nil
	default:
		return decimal.Zero, false
	}
}
