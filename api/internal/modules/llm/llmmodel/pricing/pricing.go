package pricing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrPricingNotConfigured indicates that no model price is available for the
// requested operation and fail-closed billing must not proceed.
var ErrPricingNotConfigured = errors.New("model pricing is not configured")

var nonNegativeDecimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

type structuredPricing struct {
	Version         int                               `json:"version,omitempty"`
	PricingVersion  string                            `json:"pricing_version,omitempty"`
	CatalogOrigin   string                            `json:"catalog_origin,omitempty"`
	DeploymentScope string                            `json:"deployment_scope"`
	Rules           []structuredRule                  `json:"rules,omitempty"`
	TokenTiers      []structuredPriceTier             `json:"token_tiers,omitempty"`
	PricePerImage   *float64                          `json:"price_per_image,omitempty"`
	VideoGeneration *structuredVideoGenerationPricing `json:"video_generation,omitempty"`
	Metered         []structuredMeteredPricing        `json:"metered,omitempty"`
}

type structuredMeteredPricing struct {
	Operation  string                 `json:"operation"`
	Meter      string                 `json:"meter"`
	BaseUnit   string                 `json:"base_unit"`
	Price      structuredMeteredPrice `json:"price"`
	Dimensions json.RawMessage        `json:"dimensions,omitempty"`
}

type structuredMeteredPrice struct {
	Amount      json.RawMessage `json:"amount"`
	Currency    string          `json:"currency"`
	PerQuantity int64           `json:"per_quantity"`
}

type meteredPricingContract struct {
	Meter    string
	BaseUnit string
}

var meteredPricingContracts = map[string]meteredPricingContract{
	"transcription":     {Meter: "input_audio_duration", BaseUnit: "millisecond"},
	"speech_generation": {Meter: "input_text", BaseUnit: "billed_character"},
	"music_generation":  {Meter: "output_track", BaseUnit: "track"},
	"lyrics_generation": {Meter: "request", BaseUnit: "request"},
}

type structuredPriceTier struct {
	MinInputTokens              int      `json:"min_input_tokens"`
	MaxInputTokens              *int     `json:"max_input_tokens,omitempty"`
	InputPricePerMillion        *float64 `json:"input_price_per_million,omitempty"`
	CachedInputPricePerMillion  *float64 `json:"cached_input_price_per_million,omitempty"`
	CacheWritePricePerMillion   *float64 `json:"cache_write_price_per_million,omitempty"`
	CacheWrite5mPricePerMillion *float64 `json:"cache_write_5m_price_per_million,omitempty"`
	CacheWrite1hPricePerMillion *float64 `json:"cache_write_1h_price_per_million,omitempty"`
	OutputPricePerMillion       *float64 `json:"output_price_per_million,omitempty"`
	BatchInputPricePerMillion   *float64 `json:"batch_input_price_per_million,omitempty"`
	BatchOutputPricePerMillion  *float64 `json:"batch_output_price_per_million,omitempty"`
}

type structuredVideoGenerationPricing struct {
	Currency          string                          `json:"currency"`
	BillingUnit       string                          `json:"billing_unit"`
	EstimateSupported *bool                           `json:"estimate_supported,omitempty"`
	Dimensions        []string                        `json:"dimensions,omitempty"`
	Rates             []structuredVideoGenerationRate `json:"rates"`
	ResolutionRates   []structuredVideoResolutionRate `json:"resolution_rates,omitempty"`
	TokenEstimation   *structuredVideoTokenEstimation `json:"token_estimation,omitempty"`
}

type structuredVideoResolutionRate struct {
	Resolution string                          `json:"resolution"`
	Rates      []structuredVideoGenerationRate `json:"rates"`
}

type structuredVideoGenerationRate struct {
	ID                    string   `json:"id,omitempty"`
	ModelTier             string   `json:"model_tier,omitempty"`
	Mode                  string   `json:"mode,omitempty"`
	Resolution            string   `json:"resolution,omitempty"`
	DurationSeconds       *float64 `json:"duration_seconds,omitempty"`
	InputVideo            *bool    `json:"input_video,omitempty"`
	Audio                 *bool    `json:"audio,omitempty"`
	PricePerSecond        *float64 `json:"price_per_second,omitempty"`
	PricePerTask          *float64 `json:"price_per_task,omitempty"`
	PricePerMillionTokens *float64 `json:"price_per_million_tokens,omitempty"`
}

type structuredVideoTokenEstimation struct {
	Unit  string                             `json:"unit"`
	Rules []structuredVideoTokenEstimateRule `json:"rules"`
}

type structuredVideoTokenEstimateRule struct {
	Resolution      string   `json:"resolution,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	InputVideo      *bool    `json:"input_video,omitempty"`
	TokensPerSecond *float64 `json:"tokens_per_second"`
}

// TokenPricing is the non-batch token price selected for a concrete request size.
type TokenPricing struct {
	InputPricePerMillion        float64
	OutputPricePerMillion       float64
	CacheReadPricePerMillion    *float64
	CacheWritePricePerMillion   *float64
	CacheWrite5mPricePerMillion *float64
	CacheWrite1hPricePerMillion *float64
}

// MeteredPriceRequest identifies one non-token pricing dimension.
type MeteredPriceRequest struct {
	Operation  string
	Meter      string
	BaseUnit   string
	Dimensions map[string]string
}

// MeteredPrice is the immutable source-price fact selected for one operation.
type MeteredPrice struct {
	Operation   string
	Meter       string
	BaseUnit    string
	Amount      string
	Currency    string
	PerQuantity int64
	Dimensions  map[string]string
}

// VideoGenerationCostRequest describes the pricing dimensions of one video
// generation request.
type VideoGenerationCostRequest struct {
	Resolution      string
	DurationSeconds float64
	Count           int
	InputVideo      bool
	Audio           bool
	VideoTokens     int
}

// VideoGenerationCost is the resolved cost in the pricing currency.
type VideoGenerationCost struct {
	Currency        string
	BillingUnit     string
	UnitPrice       float64
	Quantity        float64
	SourceCost      float64
	RuleID          string
	VideoTokens     int
	Resolution      string
	DurationSeconds float64
	InputVideo      bool
	Audio           bool
}

// ImageGenerationPrice is the resolved per-image source price and rule identity.
type ImageGenerationPrice struct {
	UnitPrice      float64
	RuleID         string
	PricingVersion string
	Conditions     map[string]string
}

// ValidateStructuredPricing reports whether pricing is present and valid.
func ValidateStructuredPricing(raw json.RawMessage) (bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false, nil
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return true, fmt.Errorf("invalid pricing: %w", err)
	}
	hasRules := len(pricing.Rules) > 0
	hasTokenTiers := len(pricing.TokenTiers) > 0
	hasImagePrice := pricing.PricePerImage != nil
	hasVideoGeneration := pricing.VideoGeneration != nil
	hasMeteredPricing := len(pricing.Metered) > 0
	kindCount := 0
	for _, present := range []bool{hasTokenTiers, hasImagePrice, hasVideoGeneration, hasMeteredPricing} {
		if present {
			kindCount++
		}
	}
	if kindCount == 0 && !hasRules {
		return true, fmt.Errorf("invalid pricing: at least one of rules, token_tiers, price_per_image, video_generation, or metered is required")
	}
	if hasRules {
		if err := validatePricingRules(pricing.Rules); err != nil {
			return true, err
		}
	}
	// A model can expose multiple billable capabilities at once. The image
	// scalar is retained as a legacy projection, but it must be allowed to
	// coexist with token, video, and metered rules for multimodal models.
	if !hasVideoGeneration && !hasVideoGenerationRule(pricing.Rules) && strings.TrimSpace(pricing.DeploymentScope) == "" {
		return true, fmt.Errorf("invalid pricing: deployment_scope is required")
	}
	if hasImagePrice {
		if *pricing.PricePerImage < 0 {
			return true, fmt.Errorf("invalid pricing: price_per_image must be greater than or equal to 0")
		}
	}
	if hasVideoGeneration {
		if err := validateVideoGenerationPricing(pricing.VideoGeneration); err != nil {
			return true, err
		}
	}
	if hasMeteredPricing {
		if err := validateMeteredPricing(pricing.Metered); err != nil {
			return true, err
		}
	}

	if hasTokenTiers {
		expectedMin := 0
		for i, tier := range pricing.TokenTiers {
			if tier.MinInputTokens < 0 {
				return true, fmt.Errorf("invalid pricing: token_tiers[%d].min_input_tokens must be greater than or equal to 0", i)
			}
			if tier.MinInputTokens != expectedMin {
				return true, fmt.Errorf("invalid pricing: token_tiers must cover contiguous input token ranges")
			}
			if tier.MaxInputTokens != nil {
				if *tier.MaxInputTokens < tier.MinInputTokens {
					return true, fmt.Errorf("invalid pricing: token_tiers[%d].max_input_tokens must be greater than or equal to min_input_tokens", i)
				}
				expectedMin = *tier.MaxInputTokens + 1
			} else if i != len(pricing.TokenTiers)-1 {
				return true, fmt.Errorf("invalid pricing: only the final token tier may omit max_input_tokens")
			}
			prices := []*float64{
				tier.InputPricePerMillion,
				tier.CachedInputPricePerMillion,
				tier.CacheWritePricePerMillion,
				tier.CacheWrite5mPricePerMillion,
				tier.CacheWrite1hPricePerMillion,
				tier.OutputPricePerMillion,
				tier.BatchInputPricePerMillion,
				tier.BatchOutputPricePerMillion,
			}
			hasPrice := false
			for _, price := range prices {
				if price == nil {
					continue
				}
				hasPrice = true
				if *price < 0 {
					return true, fmt.Errorf("invalid pricing: token_tiers[%d] contains a negative price", i)
				}
			}
			if !hasPrice {
				return true, fmt.Errorf("invalid pricing: token_tiers[%d] contains no price", i)
			}
		}
	}
	return true, nil
}

func validateMeteredPricing(prices []structuredMeteredPricing) error {
	for i, price := range prices {
		prefix := fmt.Sprintf("invalid pricing: metered[%d]", i)
		contract, ok := meteredPricingContracts[price.Operation]
		if !ok {
			return fmt.Errorf("%s.operation must be transcription, speech_generation, music_generation, or lyrics_generation", prefix)
		}
		if price.Meter != contract.Meter || price.BaseUnit != contract.BaseUnit {
			return fmt.Errorf("%s must use %s/%s/%s", prefix, price.Operation, contract.Meter, contract.BaseUnit)
		}

		var amount string
		if err := json.Unmarshal(price.Price.Amount, &amount); err != nil || !nonNegativeDecimalPattern.MatchString(amount) {
			return fmt.Errorf("%s.price.amount must be a non-negative decimal string", prefix)
		}
		if !isUppercaseCurrencyCode(price.Price.Currency) {
			return fmt.Errorf("%s.price.currency must be a three-letter uppercase code", prefix)
		}
		if price.Price.PerQuantity <= 0 {
			return fmt.Errorf("%s.price.per_quantity must be a positive integer", prefix)
		}
		if len(price.Dimensions) > 0 {
			var dimensions map[string]string
			if bytes.Equal(bytes.TrimSpace(price.Dimensions), []byte("null")) || json.Unmarshal(price.Dimensions, &dimensions) != nil {
				return fmt.Errorf("%s.dimensions must contain only string values", prefix)
			}
		}
	}
	return nil
}

func isUppercaseCurrencyCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func validateVideoGenerationPricing(pricing *structuredVideoGenerationPricing) error {
	if pricing == nil {
		return fmt.Errorf("invalid pricing: video_generation is required")
	}
	if strings.TrimSpace(pricing.Currency) == "" {
		return fmt.Errorf("invalid pricing: video_generation.currency is required")
	}
	switch pricing.BillingUnit {
	case "second", "task", "million_video_tokens":
	default:
		return fmt.Errorf("invalid pricing: video_generation.billing_unit must be one of second, task, or million_video_tokens")
	}
	rates := flattenVideoGenerationRates(pricing)
	if len(rates) == 0 {
		return fmt.Errorf("invalid pricing: video_generation.rates must contain at least one rate")
	}
	for i, rate := range rates {
		prices := []*float64{
			rate.PricePerSecond,
			rate.PricePerTask,
			rate.PricePerMillionTokens,
		}
		hasAnyPrice := false
		for _, price := range prices {
			if price == nil {
				continue
			}
			hasAnyPrice = true
			if *price < 0 {
				return fmt.Errorf("invalid pricing: video_generation.rates[%d] contains a negative price", i)
			}
		}
		if !hasAnyPrice {
			return fmt.Errorf("invalid pricing: video_generation.rates[%d] contains no price", i)
		}
		switch pricing.BillingUnit {
		case "second":
			if rate.PricePerSecond == nil {
				return fmt.Errorf("invalid pricing: video_generation.rates[%d].price_per_second is required for second billing", i)
			}
		case "task":
			if rate.PricePerTask == nil {
				return fmt.Errorf("invalid pricing: video_generation.rates[%d].price_per_task is required for task billing", i)
			}
		case "million_video_tokens":
			if rate.PricePerMillionTokens == nil {
				return fmt.Errorf("invalid pricing: video_generation.rates[%d].price_per_million_tokens is required for million_video_tokens billing", i)
			}
		}
	}
	if pricing.TokenEstimation != nil {
		if pricing.TokenEstimation.Unit != "video_tokens" {
			return fmt.Errorf("invalid pricing: video_generation.token_estimation.unit must be video_tokens")
		}
		if len(pricing.TokenEstimation.Rules) == 0 {
			return fmt.Errorf("invalid pricing: video_generation.token_estimation.rules must contain at least one rule")
		}
		for i, rule := range pricing.TokenEstimation.Rules {
			if rule.TokensPerSecond == nil {
				return fmt.Errorf("invalid pricing: video_generation.token_estimation.rules[%d].tokens_per_second is required", i)
			}
			if *rule.TokensPerSecond < 0 {
				return fmt.Errorf("invalid pricing: video_generation.token_estimation.rules[%d].tokens_per_second must be greater than or equal to 0", i)
			}
		}
	}
	return nil
}

func flattenVideoGenerationRates(pricing *structuredVideoGenerationPricing) []structuredVideoGenerationRate {
	if pricing == nil {
		return nil
	}
	rates := make([]structuredVideoGenerationRate, 0, len(pricing.Rates))
	rates = append(rates, pricing.Rates...)
	for _, group := range pricing.ResolutionRates {
		resolution := strings.TrimSpace(group.Resolution)
		for _, rate := range group.Rates {
			if strings.TrimSpace(rate.Resolution) == "" && resolution != "" {
				rate.Resolution = resolution
			}
			rates = append(rates, rate)
		}
	}
	return rates
}

// NormalizeStructuredPricing converts accepted external pricing shapes into the
// canonical structure stored in llm_models.pricing.
func NormalizeStructuredPricing(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return raw
	}
	if normalized, ok := normalizeLegacyTokenPricing(raw); ok {
		raw = normalized
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return raw
	}
	if len(pricing.Rules) == 0 {
		pricing.Rules = synthesizeRulesFromLegacyProjections(pricing, LegacyScalarPrices{}, PricingWriteOptions{})
		if pricing.Version == 0 && len(pricing.Rules) > 0 {
			pricing.Version = pricingRulesSchemaVersion
		}
	}
	if pricing.VideoGeneration != nil && len(pricing.VideoGeneration.ResolutionRates) > 0 {
		pricing.VideoGeneration.Rates = flattenVideoGenerationRates(pricing.VideoGeneration)
		pricing.VideoGeneration.ResolutionRates = nil
	}
	next, err := json.Marshal(pricing)
	if err != nil {
		return raw
	}
	return next
}

// normalizeLegacyTokenPricing converts the v2 ModelMeta token_pricing shape
// into the canonical token_tiers shape consumed by billing. Older catalog
// entries encode prices as strings under token_pricing.chat; leaving that
// shape untouched makes the strict structured-pricing validator reject an
// otherwise valid token model before the legacy price columns can be used.
func normalizeLegacyTokenPricing(raw json.RawMessage) (json.RawMessage, bool) {
	var envelope struct {
		DeploymentScope string                     `json:"deployment_scope"`
		TokenPricing    map[string]json.RawMessage `json:"token_pricing"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.TokenPricing) == 0 {
		return raw, false
	}

	selected, ok := envelope.TokenPricing["chat"]
	if !ok {
		for _, candidate := range envelope.TokenPricing {
			selected = candidate
			break
		}
	}
	var prices map[string]json.RawMessage
	if err := json.Unmarshal(selected, &prices); err != nil {
		return raw, false
	}

	tier := map[string]interface{}{"min_input_tokens": 0}
	for source, target := range map[string]string{
		"input_price_per_million":          "input_price_per_million",
		"output_price_per_million":         "output_price_per_million",
		"cache_read_price_per_million":     "cached_input_price_per_million",
		"cached_input_price_per_million":   "cached_input_price_per_million",
		"cache_write_price_per_million":    "cache_write_price_per_million",
		"cache_write_5m_price_per_million": "cache_write_5m_price_per_million",
		"cache_write_1h_price_per_million": "cache_write_1h_price_per_million",
	} {
		value, exists := prices[source]
		if !exists {
			continue
		}
		parsed, err := parseLegacyPrice(value)
		if err != nil {
			return raw, false
		}
		tier[target] = parsed
	}
	if len(tier) == 1 {
		return raw, false
	}

	deploymentScope := strings.TrimSpace(envelope.DeploymentScope)
	if deploymentScope == "" {
		deploymentScope = "global"
	}
	canonical := map[string]interface{}{
		"deployment_scope": deploymentScope,
		"token_tiers":      []interface{}{tier},
	}
	next, err := json.Marshal(canonical)
	if err != nil {
		return raw, false
	}
	return next, true
}

func parseLegacyPrice(raw json.RawMessage) (float64, error) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.Float64()
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(text), 64)
}

// ResolveMeteredPrice selects the most specific metered price compatible with
// the request dimensions. An equally specific match is rejected as ambiguous.
func ResolveMeteredPrice(raw json.RawMessage, req MeteredPriceRequest) (MeteredPrice, bool, error) {
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		return MeteredPrice{}, present, err
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return MeteredPrice{}, true, fmt.Errorf("invalid pricing: %w", err)
	}
	if len(pricing.Metered) == 0 {
		return MeteredPrice{}, true, fmt.Errorf("metered is required for metered settlement")
	}

	operation := strings.TrimSpace(req.Operation)
	meter := strings.TrimSpace(req.Meter)
	baseUnit := strings.TrimSpace(req.BaseUnit)
	bestIndex := -1
	bestScore := -1
	ambiguous := false
	for i, candidate := range pricing.Metered {
		if candidate.Operation != operation || candidate.Meter != meter || candidate.BaseUnit != baseUnit {
			continue
		}
		dimensions, err := decodeMeteredDimensions(candidate.Dimensions)
		if err != nil {
			return MeteredPrice{}, true, err
		}
		if !meteredDimensionsMatch(dimensions, req.Dimensions) {
			continue
		}
		score := len(dimensions)
		if score > bestScore {
			bestIndex = i
			bestScore = score
			ambiguous = false
			continue
		}
		if score == bestScore {
			ambiguous = true
		}
	}
	if bestIndex < 0 {
		return MeteredPrice{}, true, fmt.Errorf("no metered pricing matches %s/%s/%s", operation, meter, baseUnit)
	}
	if ambiguous {
		return MeteredPrice{}, true, fmt.Errorf("ambiguous metered pricing matches %s/%s/%s", operation, meter, baseUnit)
	}

	selected := pricing.Metered[bestIndex]
	var amount string
	if err := json.Unmarshal(selected.Price.Amount, &amount); err != nil {
		return MeteredPrice{}, true, fmt.Errorf("decode metered price amount: %w", err)
	}
	dimensions, err := decodeMeteredDimensions(selected.Dimensions)
	if err != nil {
		return MeteredPrice{}, true, err
	}
	return MeteredPrice{
		Operation:   selected.Operation,
		Meter:       selected.Meter,
		BaseUnit:    selected.BaseUnit,
		Amount:      amount,
		Currency:    selected.Price.Currency,
		PerQuantity: selected.Price.PerQuantity,
		Dimensions:  dimensions,
	}, true, nil
}

func decodeMeteredDimensions(raw json.RawMessage) (map[string]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var dimensions map[string]string
	if err := json.Unmarshal(raw, &dimensions); err != nil {
		return nil, fmt.Errorf("decode metered pricing dimensions: %w", err)
	}
	return dimensions, nil
}

func meteredDimensionsMatch(priceDimensions, requestDimensions map[string]string) bool {
	for key, value := range priceDimensions {
		if requestDimensions[key] != value {
			return false
		}
	}
	return true
}

// ResolveTokenPricing selects the official token tier for the request input size.
func ResolveTokenPricing(raw json.RawMessage, inputTokens int) (TokenPricing, bool, error) {
	raw = NormalizeStructuredPricing(raw)
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		return TokenPricing{}, present, err
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return TokenPricing{}, true, fmt.Errorf("invalid pricing: %w", err)
	}
	if len(pricing.TokenTiers) == 0 {
		return TokenPricing{}, true, fmt.Errorf("token_tiers is required for token settlement")
	}
	for _, tier := range pricing.TokenTiers {
		if inputTokens < tier.MinInputTokens {
			continue
		}
		if tier.MaxInputTokens != nil && inputTokens > *tier.MaxInputTokens {
			continue
		}
		resolved := TokenPricing{}
		if tier.InputPricePerMillion != nil {
			resolved.InputPricePerMillion = *tier.InputPricePerMillion
		}
		if tier.OutputPricePerMillion != nil {
			resolved.OutputPricePerMillion = *tier.OutputPricePerMillion
		}
		resolved.CacheReadPricePerMillion = tier.CachedInputPricePerMillion
		resolved.CacheWritePricePerMillion = tier.CacheWritePricePerMillion
		resolved.CacheWrite5mPricePerMillion = tier.CacheWrite5mPricePerMillion
		resolved.CacheWrite1hPricePerMillion = tier.CacheWrite1hPricePerMillion
		return resolved, true, nil
	}
	return TokenPricing{}, true, fmt.Errorf("no token pricing tier matches %d input tokens", inputTokens)
}

// ImageGenerationPriceRequest carries request dimensions used to match image
// generation pricing rules.
type ImageGenerationPriceRequest struct {
	Size    string
	Quality string
	Style   string
}

// NormalizeImageGenerationPriceRequest applies the same default dimensions used
// by gateway and image settlement when callers omit optional fields.
func NormalizeImageGenerationPriceRequest(req ImageGenerationPriceRequest) ImageGenerationPriceRequest {
	if strings.TrimSpace(req.Size) == "" {
		req.Size = "1024x1024"
	}
	if strings.TrimSpace(req.Quality) == "" {
		req.Quality = "standard"
	}
	return req
}

func imageGenerationPriceConditions(req ImageGenerationPriceRequest) map[string]string {
	req = NormalizeImageGenerationPriceRequest(req)
	conditions := map[string]string{
		"size":    req.Size,
		"quality": req.Quality,
	}
	if strings.TrimSpace(req.Style) != "" {
		conditions["style"] = strings.TrimSpace(req.Style)
	}
	return conditions
}

// ResolveImageGenerationPrice selects the best image_generation rule for the
// request dimensions and falls back to legacy price_per_image when needed.
func ResolveImageGenerationPrice(raw json.RawMessage, req ImageGenerationPriceRequest) (float64, bool, error) {
	price, present, err := ResolveImageGenerationPriceDetail(raw, req)
	if err != nil || !present {
		return 0, present, err
	}
	return price.UnitPrice, true, nil
}

// ResolveImageGenerationPriceDetail selects the best image_generation rule and
// returns the matched rule identity alongside the unit price.
func ResolveImageGenerationPriceDetail(raw json.RawMessage, req ImageGenerationPriceRequest) (ImageGenerationPrice, bool, error) {
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		return ImageGenerationPrice{}, present, err
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return ImageGenerationPrice{}, true, fmt.Errorf("invalid pricing: %w", err)
	}

	conditions := imageGenerationPriceConditions(req)
	if price, ok, err := resolveImagePriceDetailFromRules(pricing.Rules, conditions); ok || err != nil {
		if ok {
			price.PricingVersion = strings.TrimSpace(pricing.PricingVersion)
		}
		return price, ok, err
	}
	if pricing.PricePerImage != nil {
		return ImageGenerationPrice{
			UnitPrice:      *pricing.PricePerImage,
			RuleID:         "pricing.price_per_image",
			PricingVersion: strings.TrimSpace(pricing.PricingVersion),
			Conditions:     conditions,
		}, true, nil
	}
	return ImageGenerationPrice{}, true, fmt.Errorf("%w: image pricing is not configured", ErrPricingNotConfigured)
}

func resolveImagePriceFromRules(rules []structuredRule, conditions map[string]string) (float64, bool, error) {
	price, ok, err := resolveImagePriceDetailFromRules(rules, conditions)
	if err != nil || !ok {
		return 0, ok, err
	}
	return price.UnitPrice, true, nil
}

func resolveImagePriceDetailFromRules(rules []structuredRule, conditions map[string]string) (ImageGenerationPrice, bool, error) {
	imageRules := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == "image_generation" && rule.Meter == "image" {
			imageRules = append(imageRules, rule)
		}
	}
	if len(imageRules) == 0 {
		return ImageGenerationPrice{}, false, nil
	}

	bestScore := -1
	var best *structuredRule
	ambiguous := false
	for index := range imageRules {
		rule := imageRules[index]
		score, ok := imageRuleScore(rule, conditions)
		if !ok {
			continue
		}
		if score > bestScore {
			best = &imageRules[index]
			bestScore = score
			ambiguous = false
			continue
		}
		if score == bestScore && best != nil && rule.ID != best.ID {
			ambiguous = true
		}
	}
	if ambiguous {
		return ImageGenerationPrice{}, true, fmt.Errorf("%w: ambiguous image pricing rules", ErrPricingNotConfigured)
	}
	if best == nil {
		return ImageGenerationPrice{}, false, nil
	}
	value, ok := parsePricingDecimal(best.Price)
	if !ok {
		return ImageGenerationPrice{}, true, fmt.Errorf("%w: image pricing rule %q has no price", ErrPricingNotConfigured, best.ID)
	}
	return ImageGenerationPrice{
		UnitPrice:  value,
		RuleID:     strings.TrimSpace(best.ID),
		Conditions: clonePricingConditions(best.Conditions),
	}, true, nil
}

func clonePricingConditions(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// ResolveImagePrice returns the official per-image price when configured.
func ResolveImagePrice(raw json.RawMessage) (float64, bool, error) {
	return ResolveImageGenerationPrice(raw, ImageGenerationPriceRequest{})
}

// ResolveVideoGenerationCost selects the best video-generation rate for a
// concrete request and returns the source-currency cost.
func ResolveVideoGenerationCost(raw json.RawMessage, req VideoGenerationCostRequest) (VideoGenerationCost, bool, error) {
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		return VideoGenerationCost{}, present, err
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return VideoGenerationCost{}, true, fmt.Errorf("invalid pricing: %w", err)
	}
	if pricing.VideoGeneration == nil {
		return VideoGenerationCost{}, true, fmt.Errorf("video_generation is required for video settlement")
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}
	rate, ok := selectVideoGenerationRate(flattenVideoGenerationRates(pricing.VideoGeneration), req)
	if !ok {
		return VideoGenerationCost{}, true, fmt.Errorf("no video generation pricing rate matches request")
	}

	result := VideoGenerationCost{
		Currency:        pricing.VideoGeneration.Currency,
		BillingUnit:     pricing.VideoGeneration.BillingUnit,
		RuleID:          videoGenerationRateID(rate),
		Resolution:      strings.TrimSpace(req.Resolution),
		DurationSeconds: req.DurationSeconds,
		InputVideo:      req.InputVideo,
		Audio:           req.Audio,
	}
	if result.Resolution == "" {
		result.Resolution = strings.TrimSpace(rate.Resolution)
	}
	if result.DurationSeconds <= 0 && rate.DurationSeconds != nil {
		result.DurationSeconds = *rate.DurationSeconds
	}

	switch pricing.VideoGeneration.BillingUnit {
	case "second":
		if rate.PricePerSecond == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_second is required for video settlement")
		}
		if result.DurationSeconds <= 0 {
			return VideoGenerationCost{}, true, fmt.Errorf("duration is required for per-second video settlement")
		}
		result.UnitPrice = *rate.PricePerSecond
		result.Quantity = result.DurationSeconds * float64(count)
	case "task":
		if rate.PricePerTask == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_task is required for video settlement")
		}
		result.UnitPrice = *rate.PricePerTask
		result.Quantity = float64(count)
	case "million_video_tokens":
		if rate.PricePerMillionTokens == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_million_tokens is required for video settlement")
		}
		videoTokens := req.VideoTokens
		if videoTokens <= 0 {
			videoTokens = estimateVideoTokens(pricing.VideoGeneration.TokenEstimation, req, result.DurationSeconds)
		}
		if videoTokens <= 0 {
			return VideoGenerationCost{}, true, fmt.Errorf("video tokens are required for million_video_tokens video settlement")
		}
		result.VideoTokens = videoTokens * count
		result.UnitPrice = *rate.PricePerMillionTokens
		result.Quantity = float64(result.VideoTokens) / 1_000_000
	default:
		return VideoGenerationCost{}, true, fmt.Errorf("unsupported video billing unit %q", pricing.VideoGeneration.BillingUnit)
	}

	result.SourceCost = result.UnitPrice * result.Quantity
	return result, true, nil
}

// ResolveVideoGenerationUnitPrice selects the best matching video rate and returns
// its unit price without requiring usage quantities such as video tokens.
func ResolveVideoGenerationUnitPrice(raw json.RawMessage, req VideoGenerationCostRequest) (VideoGenerationCost, bool, error) {
	present, err := ValidateStructuredPricing(raw)
	if !present || err != nil {
		return VideoGenerationCost{}, present, err
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return VideoGenerationCost{}, true, fmt.Errorf("invalid pricing: %w", err)
	}
	if pricing.VideoGeneration == nil {
		return VideoGenerationCost{}, true, fmt.Errorf("video_generation is required for video settlement")
	}

	rate, ok := selectVideoGenerationRate(flattenVideoGenerationRates(pricing.VideoGeneration), req)
	if !ok {
		return VideoGenerationCost{}, true, fmt.Errorf("no video generation pricing rate matches request")
	}

	result := VideoGenerationCost{
		Currency:        pricing.VideoGeneration.Currency,
		BillingUnit:     pricing.VideoGeneration.BillingUnit,
		RuleID:          videoGenerationRateID(rate),
		Resolution:      strings.TrimSpace(req.Resolution),
		DurationSeconds: req.DurationSeconds,
		InputVideo:      req.InputVideo,
		Audio:           req.Audio,
	}
	if result.Resolution == "" {
		result.Resolution = strings.TrimSpace(rate.Resolution)
	}

	switch pricing.VideoGeneration.BillingUnit {
	case "second":
		if rate.PricePerSecond == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_second is required for video settlement")
		}
		result.UnitPrice = *rate.PricePerSecond
	case "task":
		if rate.PricePerTask == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_task is required for video settlement")
		}
		result.UnitPrice = *rate.PricePerTask
	case "million_video_tokens":
		if rate.PricePerMillionTokens == nil {
			return VideoGenerationCost{}, true, fmt.Errorf("price_per_million_tokens is required for video settlement")
		}
		result.UnitPrice = *rate.PricePerMillionTokens
	default:
		return VideoGenerationCost{}, true, fmt.Errorf("unsupported video billing unit %q", pricing.VideoGeneration.BillingUnit)
	}

	return result, true, nil
}

func selectVideoGenerationRate(rates []structuredVideoGenerationRate, req VideoGenerationCostRequest) (structuredVideoGenerationRate, bool) {
	bestScore := -1
	var best structuredVideoGenerationRate
	for index, rate := range rates {
		if strings.TrimSpace(rate.ID) == "" {
			rate.ID = videoGenerationRateID(rate)
			if strings.TrimSpace(rate.ID) == "" {
				rate.ID = fmt.Sprintf("video-rate-%d", index)
			}
			rates[index] = rate
		}
		score, ok := videoGenerationRateScore(rate, req)
		if !ok {
			continue
		}
		if score > bestScore {
			best = rate
			bestScore = score
		}
	}
	return best, bestScore >= 0
}

func videoGenerationRateID(rate structuredVideoGenerationRate) string {
	if id := strings.TrimSpace(rate.ID); id != "" {
		return id
	}
	if tier := strings.TrimSpace(rate.ModelTier); tier != "" {
		return tier
	}
	return ""
}

func videoGenerationRateScore(rate structuredVideoGenerationRate, req VideoGenerationCostRequest) (int, bool) {
	score := 0
	requestedResolution := strings.TrimSpace(req.Resolution)
	if value := strings.TrimSpace(rate.Resolution); value != "" {
		if requestedResolution != "" && !strings.EqualFold(value, requestedResolution) {
			return 0, false
		}
		if requestedResolution != "" {
			score += 8
		} else {
			score++
		}
	}
	if rate.InputVideo != nil {
		if *rate.InputVideo != req.InputVideo {
			return 0, false
		}
		score += 4
	}
	if rate.Audio != nil {
		if *rate.Audio != req.Audio {
			return 0, false
		}
		score += 2
	}
	if rate.DurationSeconds != nil {
		if req.DurationSeconds > 0 && *rate.DurationSeconds != req.DurationSeconds {
			return 0, false
		}
		score++
	}
	return score, true
}

func estimateVideoTokens(estimation *structuredVideoTokenEstimation, req VideoGenerationCostRequest, durationSeconds float64) int {
	if estimation == nil || durationSeconds <= 0 {
		return 0
	}
	bestScore := -1
	var tokensPerSecond float64
	for _, rule := range estimation.Rules {
		if rule.TokensPerSecond == nil {
			continue
		}
		score := 0
		requestedResolution := strings.TrimSpace(req.Resolution)
		if value := strings.TrimSpace(rule.Resolution); value != "" {
			if requestedResolution != "" && !strings.EqualFold(value, requestedResolution) {
				continue
			}
			if requestedResolution != "" {
				score += 4
			} else {
				score++
			}
		}
		if rule.InputVideo != nil {
			if *rule.InputVideo != req.InputVideo {
				continue
			}
			score += 2
		}
		if rule.DurationSeconds != nil {
			if req.DurationSeconds > 0 && *rule.DurationSeconds != req.DurationSeconds {
				continue
			}
			score++
		}
		if score > bestScore {
			bestScore = score
			tokensPerSecond = *rule.TokensPerSecond
		}
	}
	if tokensPerSecond <= 0 {
		return 0
	}
	return int(tokensPerSecond * durationSeconds)
}
