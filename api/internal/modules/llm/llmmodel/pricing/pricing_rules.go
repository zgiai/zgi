package pricing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const pricingRulesSchemaVersion = 2

// LegacyScalarPrices carries the seven nullable model price columns used when
// synthesizing rules[] from legacy clients that still submit scalar fields.
type LegacyScalarPrices struct {
	InputPrice        *float64
	OutputPrice       *float64
	CachedInputPrice  *float64
	CacheReadPrice    *float64
	CacheWritePrice   *float64
	CacheWrite5mPrice *float64
	CacheWrite1hPrice *float64
}

type structuredRule struct {
	ID         string            `json:"id,omitempty"`
	Operation  string            `json:"operation"`
	Meter      string            `json:"meter"`
	Unit       string            `json:"unit"`
	Price      string            `json:"price"`
	Configured *bool             `json:"configured,omitempty"`
	Conditions map[string]string `json:"conditions,omitempty"`
}

var pricingRuleOperations = map[string]struct{}{
	"chat":              {},
	"embedding":         {},
	"rerank":            {},
	"image_generation":  {},
	"video_generation":  {},
	"speech_generation": {},
	"transcription":     {},
	"music_generation":  {},
	"lyrics_generation": {},
	"realtime_audio":    {},
}

var pricingRuleMeterUnits = map[string]map[string]string{
	"chat": {
		"input_token":          "per_1m_tokens",
		"output_token":         "per_1m_tokens",
		"cache_read_token":     "per_1m_tokens",
		"cache_write_token":    "per_1m_tokens",
		"cache_write_5m_token": "per_1m_tokens",
		"cache_write_1h_token": "per_1m_tokens",
	},
	"embedding": {"input_token": "per_1m_tokens"},
	"rerank":    {"input_token": "per_1m_tokens"},
	"image_generation": {
		"image":             "per_image",
		"input_text_token":  "per_1m_tokens",
		"input_image_token": "per_1m_tokens",
		"output_token":      "per_1m_tokens",
	},
	"video_generation": {
		"output_video_second": "second",
		"output_video_task":   "task",
		"output_video_token":  "million_video_tokens",
	},
	"transcription": {
		"input_audio_duration": "millisecond",
	},
	"speech_generation": {
		"input_text": "billed_character",
	},
	"music_generation": {
		"output_track": "track",
	},
	"lyrics_generation": {
		"request": "request",
	},
	"realtime_audio": {
		"input_token":      "per_1m_tokens",
		"output_token":     "per_1m_tokens",
		"cache_read_token": "per_1m_tokens",
	},
}

// SynthesizeRulesFromLegacyPricing builds rules[] from legacy projections and
// optional scalar columns when the inbound payload does not already include
// authoritative rules.
func SynthesizeRulesFromLegacyPricing(scalars LegacyScalarPrices, pricing json.RawMessage, opts PricingWriteOptions) json.RawMessage {
	pricing = bytes.TrimSpace(pricing)
	if len(pricing) == 0 || bytes.Equal(pricing, []byte("null")) {
		if !legacyScalarsHaveValues(scalars) {
			return pricing
		}
		pricing = json.RawMessage(`{"deployment_scope":"global"}`)
	}

	var envelope structuredPricing
	if err := json.Unmarshal(pricing, &envelope); err != nil {
		return pricing
	}
	if len(envelope.Rules) > 0 {
		return pricing
	}

	envelope.Rules = synthesizeRulesFromLegacyProjections(envelope, scalars, opts)
	if opts.UserAuthored {
		markRulesConfigured(envelope.Rules)
	}
	next, err := json.Marshal(envelope)
	if err != nil {
		return pricing
	}
	return next
}

// PrepareStructuredPricingForWrite normalizes inbound pricing, synthesizes rules[]
// when missing, and derives legacy projections before persisting.
func PrepareStructuredPricingForWrite(raw json.RawMessage, currency string, scalars LegacyScalarPrices, opts PricingWriteOptions) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		if !legacyScalarsHaveValues(scalars) {
			return raw
		}
		raw = json.RawMessage(`{"deployment_scope":"global"}`)
	}
	normalized := NormalizeStructuredPricing(raw)
	withRules := SynthesizeRulesFromLegacyPricing(scalars, normalized, opts)
	withRules = applyScalarOverridesToRules(withRules, opts)
	withRules = applyLegacyProjectionOverrides(withRules, opts)
	return DeriveLegacyProjections(withRules, currency)
}

// applyLegacyProjectionOverrides keeps the interactive price editors working
// when the payload already carries rules[]. The image/video/metered editors
// still submit the legacy projections, so a projection that differs from what
// rules[] implies is the user's edit and is folded back into rules[]. Machine
// writers keep rules[] authoritative.
func applyLegacyProjectionOverrides(raw json.RawMessage, opts PricingWriteOptions) json.RawMessage {
	if !opts.UserAuthored {
		return raw
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return raw
	}
	var pricing structuredPricing
	if err := json.Unmarshal(trimmed, &pricing); err != nil {
		return raw
	}
	currency := inferPricingCurrency(pricing)
	changed := false

	if pricing.PricePerImage != nil {
		derived := derivePricePerImageFromRules(pricing.Rules)
		if derived == nil || *derived != *pricing.PricePerImage {
			pricing.Rules = upsertImageDefaultRule(pricing.Rules, *pricing.PricePerImage)
			pricing.PricePerImage = nil
			changed = true
		}
	}

	if pricing.ImageTokenPricing != nil && !imageTokenPricingProjectionMatchesRules(pricing.ImageTokenPricing, pricing.Rules) {
		imageRules := synthesizeImageTokenRulesFromLegacy(*pricing.ImageTokenPricing)
		for index := range imageRules {
			imageRules[index].Configured = boolPtr(true)
		}
		pricing.Rules = append(filterOutImageTokenRules(pricing.Rules), imageRules...)
		changed = true
	}

	if pricing.VideoGeneration != nil && !videoProjectionMatchesRules(pricing.VideoGeneration, pricing.Rules, currency) {
		videoRules := synthesizeVideoRulesFromLegacy(pricing.VideoGeneration)
		for index := range videoRules {
			videoRules[index].Configured = boolPtr(true)
		}
		pricing.Rules = append(filterRulesByOperation(pricing.Rules, "video_generation"), videoRules...)
		changed = true
	}

	if len(pricing.Metered) > 0 && !meteredProjectionMatchesRules(pricing.Metered, pricing.Rules, currency) {
		operations := make(map[string]bool, len(pricing.Metered))
		meteredRules := make([]structuredRule, 0, len(pricing.Metered))
		for _, metered := range pricing.Metered {
			operations[metered.Operation] = true
			rule, ok := synthesizeMeteredRule(metered)
			if !ok {
				continue
			}
			rule.Configured = boolPtr(true)
			meteredRules = append(meteredRules, rule)
		}
		kept := make([]structuredRule, 0, len(pricing.Rules))
		for _, rule := range pricing.Rules {
			if operations[rule.Operation] {
				continue
			}
			kept = append(kept, rule)
		}
		pricing.Rules = append(kept, meteredRules...)
		changed = true
	}

	if !changed {
		return raw
	}
	if pricing.Version == 0 {
		pricing.Version = pricingRulesSchemaVersion
	}
	next, err := json.Marshal(pricing)
	if err != nil {
		return raw
	}
	return next
}

func upsertImageDefaultRule(rules []structuredRule, price float64) []structuredRule {
	next := make([]structuredRule, 0, len(rules)+1)
	for _, rule := range rules {
		if rule.Operation == "image_generation" && rule.Meter == "image" && len(rule.Conditions) == 0 {
			continue
		}
		next = append(next, rule)
	}
	return append(next, structuredRule{
		ID:         "image-default",
		Operation:  "image_generation",
		Meter:      "image",
		Unit:       "per_image",
		Price:      formatPricingDecimal(price),
		Configured: boolPtr(true),
	})
}

func filterRulesByOperation(rules []structuredRule, operation string) []structuredRule {
	kept := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == operation {
			continue
		}
		kept = append(kept, rule)
	}
	return kept
}

// videoProjectionMatchesRules reports whether the submitted video projection is
// already what rules[] implies (i.e. it was not edited by the user).
func videoProjectionMatchesRules(video *structuredVideoGenerationPricing, rules []structuredRule, currency string) bool {
	derived := deriveVideoGenerationFromRules(rules, currency)
	if derived == nil {
		return false
	}
	if strings.TrimSpace(derived.BillingUnit) != strings.TrimSpace(video.BillingUnit) {
		return false
	}
	return jsonValueEqual(flattenVideoGenerationRates(derived), flattenVideoGenerationRates(video))
}

// meteredProjectionMatchesRules reports whether the submitted metered projection
// is already what rules[] implies (i.e. it was not edited by the user).
func meteredProjectionMatchesRules(metered []structuredMeteredPricing, rules []structuredRule, currency string) bool {
	derived := preserveMeteredPerQuantity(deriveMeteredFromRules(rules, currency), metered)
	return jsonValueEqual(derived, metered)
}

func jsonValueEqual(left, right interface{}) bool {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return bytes.Equal(leftJSON, rightJSON)
}

// applyScalarOverridesToRules makes caller-provided scalar token prices
// authoritative for their chat token meter. Without this, a payload that
// already carries rules[] (or token_tiers, which normalize into rules[]) would
// silently ignore a scalar price edit coming from a legacy client.
func applyScalarOverridesToRules(raw json.RawMessage, opts PricingWriteOptions) json.RawMessage {
	if opts.ScalarOverrides == nil || !legacyScalarsHaveValues(*opts.ScalarOverrides) {
		return raw
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return raw
	}
	var pricing structuredPricing
	if err := json.Unmarshal(trimmed, &pricing); err != nil {
		return raw
	}
	operation := tokenOperationForUseCases(opts.UseCases)
	if operation == "" {
		operation = "chat"
	}
	cacheRead := opts.ScalarOverrides.CacheReadPrice
	if cacheRead == nil {
		cacheRead = opts.ScalarOverrides.CachedInputPrice
	}
	type scalarOverride struct {
		id    string
		meter string
		price *float64
	}
	overrides := []scalarOverride{
		{id: operation + "-input", meter: "input_token", price: opts.ScalarOverrides.InputPrice},
	}
	if operation == "chat" {
		overrides = append(overrides,
			scalarOverride{id: operation + "-output", meter: "output_token", price: opts.ScalarOverrides.OutputPrice},
			scalarOverride{id: operation + "-cache-read", meter: "cache_read_token", price: cacheRead},
			scalarOverride{id: operation + "-cache-write", meter: "cache_write_token", price: opts.ScalarOverrides.CacheWritePrice},
			scalarOverride{id: operation + "-cache-write-5m", meter: "cache_write_5m_token", price: opts.ScalarOverrides.CacheWrite5mPrice},
			scalarOverride{id: operation + "-cache-write-1h", meter: "cache_write_1h_token", price: opts.ScalarOverrides.CacheWrite1hPrice},
		)
	}
	for _, override := range overrides {
		if override.price == nil {
			continue
		}
		pricing.Rules = upsertTokenRule(pricing.Rules, structuredRule{
			ID:         override.id,
			Operation:  operation,
			Meter:      override.meter,
			Unit:       "per_1m_tokens",
			Price:      formatPricingDecimal(*override.price),
			Configured: boolPtr(true),
		})
	}
	if pricing.Version == 0 {
		pricing.Version = pricingRulesSchemaVersion
	}
	next, err := json.Marshal(pricing)
	if err != nil {
		return raw
	}
	return next
}

// upsertTokenRule replaces every rule for the same operation+meter (including
// conditional variants) with the explicit scalar rule.
func upsertTokenRule(rules []structuredRule, rule structuredRule) []structuredRule {
	next := make([]structuredRule, 0, len(rules)+1)
	for _, existing := range rules {
		if existing.Operation == rule.Operation && existing.Meter == rule.Meter {
			continue
		}
		next = append(next, existing)
	}
	return append(next, rule)
}

// DeriveScalarsFromPricing projects the seven nullable model price columns from
// structured pricing rules and legacy projections.
func DeriveScalarsFromPricing(raw json.RawMessage) LegacyScalarPrices {
	raw = DeriveLegacyProjections(NormalizeStructuredPricing(raw), "")
	envelope := parsePricingEnvelope(raw)
	scalars := LegacyScalarPrices{}
	if len(envelope.TokenTiers) > 0 {
		tier := envelope.TokenTiers[0]
		scalars.InputPrice = tier.InputPricePerMillion
		scalars.OutputPrice = tier.OutputPricePerMillion
		scalars.CachedInputPrice = tier.CachedInputPricePerMillion
		scalars.CacheReadPrice = tier.CachedInputPricePerMillion
		scalars.CacheWritePrice = tier.CacheWritePricePerMillion
		scalars.CacheWrite5mPrice = tier.CacheWrite5mPricePerMillion
		scalars.CacheWrite1hPrice = tier.CacheWrite1hPricePerMillion
	}
	for _, rule := range envelope.Rules {
		value, ok := parsePricingDecimal(rule.Price)
		if !ok {
			continue
		}
		price := floatPtr(value)
		switch rule.Operation {
		case "chat", "embedding", "rerank":
			switch rule.Meter {
			case "input_token":
				if scalars.InputPrice == nil {
					scalars.InputPrice = price
				}
			case "output_token":
				if scalars.OutputPrice == nil {
					scalars.OutputPrice = price
				}
			case "cache_read_token":
				if scalars.CacheReadPrice == nil {
					scalars.CacheReadPrice = price
				}
				if scalars.CachedInputPrice == nil {
					scalars.CachedInputPrice = price
				}
			case "cache_write_token":
				if scalars.CacheWritePrice == nil {
					scalars.CacheWritePrice = price
				}
			case "cache_write_5m_token":
				if scalars.CacheWrite5mPrice == nil {
					scalars.CacheWrite5mPrice = price
				}
			case "cache_write_1h_token":
				if scalars.CacheWrite1hPrice == nil {
					scalars.CacheWrite1hPrice = price
				}
			}
		}
	}
	return scalars
}

// PricingContainsImageRule reports whether pricing includes image generation via
// legacy projection or authoritative rules[].
func PricingContainsImageRule(raw json.RawMessage) bool {
	raw = NormalizeStructuredPricing(raw)
	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return false
	}
	if pricing.PricePerImage != nil {
		return true
	}
	for _, rule := range pricing.Rules {
		if rule.Operation == "image_generation" && rule.Meter == "image" {
			return true
		}
	}
	return false
}

// MergeOrganizationPricingOverride overlays tenant-specific rules onto model
// pricing without replacing the full pricing document.
func MergeOrganizationPricingOverride(basePricing, overridePricing json.RawMessage, currency string) json.RawMessage {
	overridePricing = bytes.TrimSpace(overridePricing)
	if len(overridePricing) == 0 || bytes.Equal(overridePricing, []byte("null")) {
		return basePricing
	}

	basePricing = NormalizeStructuredPricing(basePricing)
	var baseEnvelope structuredPricing
	if err := json.Unmarshal(basePricing, &baseEnvelope); err != nil {
		return basePricing
	}

	overrideRules := extractPricingOverrideRules(overridePricing)
	if len(overrideRules) == 0 {
		return basePricing
	}

	overrideByKey := make(map[string]structuredRule, len(overrideRules))
	overrideByID := make(map[string]structuredRule, len(overrideRules))
	for _, rule := range overrideRules {
		overrideByKey[pricingRuleMergeKey(rule)] = rule
		if id := strings.TrimSpace(rule.ID); id != "" {
			overrideByID[id] = rule
		}
	}

	merged := make([]structuredRule, 0, len(baseEnvelope.Rules)+len(overrideRules))
	matched := make(map[string]struct{}, len(overrideRules))
	for _, rule := range baseEnvelope.Rules {
		if override, ok := findPricingRuleOverride(rule, overrideByKey, overrideByID); ok {
			rule = applyPricingRuleOverride(rule, override)
			matched[pricingRuleMergeKey(override)] = struct{}{}
			if id := strings.TrimSpace(override.ID); id != "" {
				matched["id:"+id] = struct{}{}
			}
		}
		merged = append(merged, rule)
	}
	for _, override := range overrideRules {
		key := pricingRuleMergeKey(override)
		if _, ok := matched[key]; ok {
			continue
		}
		if id := strings.TrimSpace(override.ID); id != "" {
			if _, ok := matched["id:"+id]; ok {
				continue
			}
		}
		merged = append(merged, override)
	}

	baseEnvelope.Rules = dedupePricingRules(merged)
	mergedRaw, err := json.Marshal(baseEnvelope)
	if err != nil {
		return basePricing
	}
	return DeriveLegacyProjections(mergedRaw, currency)
}

// DeriveLegacyProjections projects rules[] into the legacy pricing fields that
// existing resolvers continue to read.
func DeriveLegacyProjections(raw json.RawMessage, currency string) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return raw
	}

	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return raw
	}
	if len(pricing.Rules) == 0 {
		return raw
	}
	applyDerivedLegacyProjections(&pricing, currency)
	next, err := json.Marshal(pricing)
	if err != nil {
		return raw
	}
	return next
}

func synthesizeRulesFromLegacyProjections(pricing structuredPricing, scalars LegacyScalarPrices, opts PricingWriteOptions) []structuredRule {
	rules := make([]structuredRule, 0, 16)
	tokenOperation := tokenOperationForUseCases(opts.UseCases)
	rules = append(rules, synthesizeTokenRulesFromTokenTiers(pricing.TokenTiers, tokenOperation)...)
	if pricing.PricePerImage != nil {
		rules = append(rules, structuredRule{
			ID:         "image-default",
			Operation:  "image_generation",
			Meter:      "image",
			Unit:       "per_image",
			Price:      formatPricingDecimal(*pricing.PricePerImage),
			Configured: boolPtr(false),
		})
	}
	if pricing.ImageTokenPricing != nil {
		rules = append(rules, synthesizeImageTokenRulesFromLegacy(*pricing.ImageTokenPricing)...)
	}
	if pricing.VideoGeneration != nil {
		rules = append(rules, synthesizeVideoRulesFromLegacy(pricing.VideoGeneration)...)
	}
	for _, metered := range pricing.Metered {
		if rule, ok := synthesizeMeteredRule(metered); ok {
			rules = append(rules, rule)
		}
	}
	rules = append(rules, synthesizeTokenRulesFromScalars(scalars, tokenOperation, rules)...)
	return dedupePricingRules(rules)
}

func tokenOperationForUseCases(useCases []string) string {
	normalized := normalizeUseCases(useCases)
	if len(normalized) == 0 {
		return "chat"
	}
	hasChat := false
	hasEmbedding := false
	hasRerank := false
	for _, useCase := range normalized {
		switch useCase {
		case "text-chat", "function-calling", "agent", "vision", "reasoning", "realtime-audio":
			hasChat = true
		case "embedding":
			hasEmbedding = true
		case "rerank":
			hasRerank = true
		}
	}
	if hasChat {
		return "chat"
	}
	if hasEmbedding {
		return "embedding"
	}
	if hasRerank {
		return "rerank"
	}
	return "chat"
}

func synthesizeTokenRulesFromTokenTiers(tiers []structuredPriceTier, operation string) []structuredRule {
	if len(tiers) == 0 {
		return nil
	}
	tier := tiers[0]
	rules := make([]structuredRule, 0, 6)
	appendTokenRule := func(id, meter string, price *float64) {
		if price == nil {
			return
		}
		rules = append(rules, structuredRule{
			ID:         id,
			Operation:  operation,
			Meter:      meter,
			Unit:       "per_1m_tokens",
			Price:      formatPricingDecimal(*price),
			Configured: boolPtr(false),
		})
	}
	prefix := operation
	appendTokenRule(prefix+"-input", "input_token", tier.InputPricePerMillion)
	if operation == "chat" {
		appendTokenRule(prefix+"-output", "output_token", tier.OutputPricePerMillion)
		appendTokenRule(prefix+"-cache-read", "cache_read_token", tier.CachedInputPricePerMillion)
		appendTokenRule(prefix+"-cache-write", "cache_write_token", tier.CacheWritePricePerMillion)
		appendTokenRule(prefix+"-cache-write-5m", "cache_write_5m_token", tier.CacheWrite5mPricePerMillion)
		appendTokenRule(prefix+"-cache-write-1h", "cache_write_1h_token", tier.CacheWrite1hPricePerMillion)
	}
	return rules
}

// synthesizeImageTokenRulesFromLegacy converts the image_token_pricing block
// into canonical image_generation token rules. Absent prices are skipped so the
// resolver fallback stays the single source of the input-tier mirroring.
func synthesizeImageTokenRulesFromLegacy(pricing structuredImageTokenPricing) []structuredRule {
	prices := []struct {
		id    string
		meter string
		price *float64
	}{
		{id: "image-token-text-input", meter: "input_text_token", price: pricing.TextInputPricePerMillion},
		{id: "image-token-image-input", meter: "input_image_token", price: pricing.ImageInputPricePerMillion},
		{id: "image-token-output", meter: "output_token", price: pricing.OutputPricePerMillion},
	}
	rules := make([]structuredRule, 0, len(prices))
	for _, entry := range prices {
		if entry.price == nil {
			continue
		}
		rules = append(rules, structuredRule{
			ID:         entry.id,
			Operation:  "image_generation",
			Meter:      entry.meter,
			Unit:       "per_1m_tokens",
			Price:      formatPricingDecimal(*entry.price),
			Configured: boolPtr(false),
		})
	}
	return rules
}

// deriveImageTokenPricingFromRules rebuilds the image_token_pricing projection
// from image_generation token rules. It returns nil when none are present.
func deriveImageTokenPricingFromRules(rules []structuredRule) *structuredImageTokenPricing {
	block := structuredImageTokenPricing{}
	found := false
	for _, rule := range rules {
		if rule.Operation != "image_generation" || !isImageTokenMeter(rule.Meter) {
			continue
		}
		value, ok := parsePricingDecimal(rule.Price)
		if !ok {
			continue
		}
		switch rule.Meter {
		case "input_text_token":
			block.TextInputPricePerMillion = floatPtr(value)
		case "input_image_token":
			block.ImageInputPricePerMillion = floatPtr(value)
		case "output_token":
			block.OutputPricePerMillion = floatPtr(value)
		}
		found = true
	}
	if !found {
		return nil
	}
	return &block
}

func imageTokenPricingHasPrice(pricing structuredImageTokenPricing) bool {
	return pricing.TextInputPricePerMillion != nil ||
		pricing.ImageInputPricePerMillion != nil ||
		pricing.OutputPricePerMillion != nil
}

func imageTokenPricingProjectionMatchesRules(block *structuredImageTokenPricing, rules []structuredRule) bool {
	if block == nil {
		return false
	}
	derived := deriveImageTokenPricingFromRules(rules)
	if derived == nil {
		return false
	}
	// Currency is informational: rules[] do not carry it, so compare only the
	// price fields to avoid needless rewrites on every authoring pass.
	candidate := *block
	candidate.Currency = ""
	return jsonValueEqual(derived, candidate)
}

func isImageTokenMeter(meter string) bool {
	switch meter {
	case "input_text_token", "input_image_token", "output_token":
		return true
	default:
		return false
	}
}

func filterOutImageTokenRules(rules []structuredRule) []structuredRule {
	kept := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == "image_generation" && isImageTokenMeter(rule.Meter) {
			continue
		}
		kept = append(kept, rule)
	}
	return kept
}

func synthesizeTokenRulesFromScalars(scalars LegacyScalarPrices, operation string, existing []structuredRule) []structuredRule {
	cacheRead := scalars.CacheReadPrice
	if cacheRead == nil {
		cacheRead = scalars.CachedInputPrice
	}
	rules := make([]structuredRule, 0, 6)
	appendIfMissing := func(id, meter string, price *float64) {
		if price == nil || hasTokenMeter(existing, operation, meter) || hasTokenMeter(rules, operation, meter) {
			return
		}
		rules = append(rules, structuredRule{
			ID:         id,
			Operation:  operation,
			Meter:      meter,
			Unit:       "per_1m_tokens",
			Price:      formatPricingDecimal(*price),
			Configured: boolPtr(false),
		})
	}
	prefix := operation
	appendIfMissing(prefix+"-input", "input_token", scalars.InputPrice)
	if operation == "chat" {
		appendIfMissing(prefix+"-output", "output_token", scalars.OutputPrice)
		appendIfMissing(prefix+"-cache-read", "cache_read_token", cacheRead)
		appendIfMissing(prefix+"-cache-write", "cache_write_token", scalars.CacheWritePrice)
		appendIfMissing(prefix+"-cache-write-5m", "cache_write_5m_token", scalars.CacheWrite5mPrice)
		appendIfMissing(prefix+"-cache-write-1h", "cache_write_1h_token", scalars.CacheWrite1hPrice)
	}
	return rules
}

func hasTokenMeter(rules []structuredRule, operation, meter string) bool {
	for _, rule := range rules {
		if rule.Operation == operation && rule.Meter == meter {
			return true
		}
	}
	return false
}

func legacyScalarsHaveValues(scalars LegacyScalarPrices) bool {
	return scalars.InputPrice != nil ||
		scalars.OutputPrice != nil ||
		scalars.CachedInputPrice != nil ||
		scalars.CacheReadPrice != nil ||
		scalars.CacheWritePrice != nil ||
		scalars.CacheWrite5mPrice != nil ||
		scalars.CacheWrite1hPrice != nil
}

func markRulesConfigured(rules []structuredRule) {
	for index := range rules {
		rules[index].Configured = boolPtr(true)
	}
}

func synthesizeVideoRulesFromLegacy(video *structuredVideoGenerationPricing) []structuredRule {
	if video == nil {
		return nil
	}
	rules := make([]structuredRule, 0, len(flattenVideoGenerationRates(video)))
	for index, rate := range flattenVideoGenerationRates(video) {
		meter, unit, price, ok := videoRateToRulePrice(video.BillingUnit, rate)
		if !ok {
			continue
		}
		conditions := videoRateConditions(rate)
		id := strings.TrimSpace(rate.ModelTier)
		if id == "" {
			id = fmt.Sprintf("video-rate-%d", index)
		}
		rules = append(rules, structuredRule{
			ID:         id,
			Operation:  "video_generation",
			Meter:      meter,
			Unit:       unit,
			Price:      price,
			Configured: boolPtr(false),
			Conditions: conditions,
		})
	}
	return rules
}

func videoRateToRulePrice(billingUnit string, rate structuredVideoGenerationRate) (meter, unit, price string, ok bool) {
	switch billingUnit {
	case "second":
		if rate.PricePerSecond == nil {
			return "", "", "", false
		}
		return "output_video_second", "second", formatPricingDecimal(*rate.PricePerSecond), true
	case "task":
		if rate.PricePerTask == nil {
			return "", "", "", false
		}
		return "output_video_task", "task", formatPricingDecimal(*rate.PricePerTask), true
	case "million_video_tokens":
		if rate.PricePerMillionTokens == nil {
			return "", "", "", false
		}
		return "output_video_token", "million_video_tokens", formatPricingDecimal(*rate.PricePerMillionTokens), true
	default:
		return "", "", "", false
	}
}

func videoRateConditions(rate structuredVideoGenerationRate) map[string]string {
	conditions := make(map[string]string)
	if value := strings.TrimSpace(rate.Resolution); value != "" {
		conditions["resolution"] = value
	}
	if rate.InputVideo != nil {
		conditions["input_video"] = strconv.FormatBool(*rate.InputVideo)
	}
	if rate.Audio != nil {
		conditions["audio"] = strconv.FormatBool(*rate.Audio)
	}
	if rate.DurationSeconds != nil {
		conditions["duration_seconds"] = formatPricingDecimal(*rate.DurationSeconds)
	}
	if len(conditions) == 0 {
		return nil
	}
	return conditions
}

func synthesizeMeteredRule(metered structuredMeteredPricing) (structuredRule, bool) {
	var amount string
	if err := json.Unmarshal(metered.Price.Amount, &amount); err != nil {
		return structuredRule{}, false
	}
	contract, ok := meteredPricingContracts[metered.Operation]
	if !ok || metered.Meter != contract.Meter || metered.BaseUnit != contract.BaseUnit {
		return structuredRule{}, false
	}
	conditions, err := decodeMeteredDimensions(metered.Dimensions)
	if err != nil {
		return structuredRule{}, false
	}
	return structuredRule{
		ID:         fmt.Sprintf("%s-%s", metered.Operation, metered.Meter),
		Operation:  metered.Operation,
		Meter:      metered.Meter,
		Unit:       metered.BaseUnit,
		Price:      amount,
		Configured: boolPtr(false),
		Conditions: conditions,
	}, true
}

func applyDerivedLegacyProjections(pricing *structuredPricing, currency string) {
	if pricing == nil || len(pricing.Rules) == 0 {
		return
	}
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = inferPricingCurrency(*pricing)
	}

	originalVideo := pricing.VideoGeneration
	originalMetered := pricing.Metered

	pricing.TokenTiers = deriveTokenTiersFromRules(pricing.Rules)
	pricing.PricePerImage = derivePricePerImageFromRules(pricing.Rules)
	pricing.ImageTokenPricing = deriveImageTokenPricingFromRules(pricing.Rules)
	pricing.VideoGeneration = deriveVideoGenerationFromRules(pricing.Rules, currency)
	if pricing.VideoGeneration != nil && originalVideo != nil {
		pricing.VideoGeneration.TokenEstimation = originalVideo.TokenEstimation
		pricing.VideoGeneration.EstimateSupported = originalVideo.EstimateSupported
		if len(pricing.VideoGeneration.Dimensions) == 0 {
			pricing.VideoGeneration.Dimensions = originalVideo.Dimensions
		}
	}
	pricing.Metered = preserveMeteredPerQuantity(
		deriveMeteredFromRules(pricing.Rules, currency),
		originalMetered,
	)

	if pricing.Version == 0 {
		pricing.Version = pricingRulesSchemaVersion
	}
}

func preserveMeteredPerQuantity(derived, original []structuredMeteredPricing) []structuredMeteredPricing {
	if len(derived) == 0 || len(original) == 0 {
		return derived
	}
	for index, entry := range derived {
		for _, candidate := range original {
			if entry.Operation != candidate.Operation || entry.Meter != candidate.Meter || entry.BaseUnit != candidate.BaseUnit {
				continue
			}
			if !bytes.Equal(bytes.TrimSpace(entry.Dimensions), bytes.TrimSpace(candidate.Dimensions)) {
				continue
			}
			var derivedAmount string
			var originalAmount string
			if err := json.Unmarshal(entry.Price.Amount, &derivedAmount); err != nil {
				break
			}
			if err := json.Unmarshal(candidate.Price.Amount, &originalAmount); err != nil {
				break
			}
			if derivedAmount != originalAmount {
				break
			}
			derived[index].Price.PerQuantity = candidate.Price.PerQuantity
			break
		}
	}
	return derived
}

func deriveTokenTiersFromRules(rules []structuredRule) []structuredPriceTier {
	chatRules := selectChatRulesForProjection(rules)
	if len(chatRules) == 0 {
		return nil
	}
	tier := structuredPriceTier{MinInputTokens: 0}
	for _, rule := range chatRules {
		value, ok := parsePricingDecimal(rule.Price)
		if !ok {
			continue
		}
		switch rule.Meter {
		case "input_token":
			tier.InputPricePerMillion = floatPtr(value)
		case "output_token":
			tier.OutputPricePerMillion = floatPtr(value)
		case "cache_read_token":
			tier.CachedInputPricePerMillion = floatPtr(value)
		case "cache_write_token":
			tier.CacheWritePricePerMillion = floatPtr(value)
		case "cache_write_5m_token":
			tier.CacheWrite5mPricePerMillion = floatPtr(value)
		case "cache_write_1h_token":
			tier.CacheWrite1hPricePerMillion = floatPtr(value)
		}
	}
	if tier.InputPricePerMillion == nil && tier.OutputPricePerMillion == nil &&
		tier.CachedInputPricePerMillion == nil && tier.CacheWritePricePerMillion == nil &&
		tier.CacheWrite5mPricePerMillion == nil && tier.CacheWrite1hPricePerMillion == nil {
		return nil
	}
	return []structuredPriceTier{tier}
}

func selectChatRulesForProjection(rules []structuredRule) []structuredRule {
	chatRules := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation != "chat" {
			continue
		}
		chatRules = append(chatRules, rule)
	}
	sort.SliceStable(chatRules, func(i, j int) bool {
		leftConfigured := ruleConfigured(chatRules[i])
		rightConfigured := ruleConfigured(chatRules[j])
		if leftConfigured != rightConfigured {
			return leftConfigured
		}
		return strings.Compare(chatRules[i].ID, chatRules[j].ID) < 0
	})
	return chatRules
}

func derivePricePerImageFromRules(rules []structuredRule) *float64 {
	imageRules := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == "image_generation" && rule.Meter == "image" {
			imageRules = append(imageRules, rule)
		}
	}
	if len(imageRules) == 0 {
		return nil
	}
	for _, rule := range imageRules {
		if len(rule.Conditions) == 0 {
			value, ok := parsePricingDecimal(rule.Price)
			if !ok {
				return nil
			}
			return floatPtr(value)
		}
	}
	bestScore := -1
	var best *structuredRule
	for index := range imageRules {
		rule := imageRules[index]
		score, ok := imageRuleScore(rule, map[string]string{
			"size":    "1024x1024",
			"quality": "standard",
		})
		if !ok || score <= bestScore {
			continue
		}
		bestScore = score
		best = &imageRules[index]
	}
	if best == nil {
		return nil
	}
	value, ok := parsePricingDecimal(best.Price)
	if !ok {
		return nil
	}
	return floatPtr(value)
}

func imageRuleScore(rule structuredRule, request map[string]string) (int, bool) {
	if len(rule.Conditions) == 0 {
		return 0, true
	}
	score := 0
	for key, expected := range rule.Conditions {
		actual, ok := request[key]
		if !ok {
			return 0, false
		}
		if !strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(expected)) {
			return 0, false
		}
		score++
	}
	return score, true
}

func deriveVideoGenerationFromRules(rules []structuredRule, currency string) *structuredVideoGenerationPricing {
	videoRules := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == "video_generation" {
			videoRules = append(videoRules, rule)
		}
	}
	if len(videoRules) == 0 {
		return nil
	}

	billingUnit := ""
	for _, rule := range videoRules {
		switch rule.Meter {
		case "output_video_second":
			billingUnit = "second"
		case "output_video_task":
			billingUnit = "task"
		case "output_video_token":
			billingUnit = "million_video_tokens"
		}
		if billingUnit != "" {
			break
		}
	}
	if billingUnit == "" {
		return nil
	}

	rates := make([]structuredVideoGenerationRate, 0, len(videoRules))
	dimensions := make(map[string]struct{})
	for index, rule := range videoRules {
		rate, ok := deriveVideoRateFromRule(rule, billingUnit, index)
		if !ok {
			continue
		}
		rates = append(rates, rate)
		for key := range rule.Conditions {
			dimensions[key] = struct{}{}
		}
	}
	if len(rates) == 0 {
		return nil
	}
	dimensionList := make([]string, 0, len(dimensions))
	for key := range dimensions {
		dimensionList = append(dimensionList, key)
	}
	sort.Strings(dimensionList)
	return &structuredVideoGenerationPricing{
		Currency:    currency,
		BillingUnit: billingUnit,
		Dimensions:  dimensionList,
		Rates:       rates,
	}
}

func deriveVideoRateFromRule(rule structuredRule, billingUnit string, index int) (structuredVideoGenerationRate, bool) {
	value, ok := parsePricingDecimal(rule.Price)
	if !ok {
		return structuredVideoGenerationRate{}, false
	}
	rate := structuredVideoGenerationRate{
		ID:        strings.TrimSpace(rule.ID),
		ModelTier: strings.TrimSpace(rule.ID),
	}
	if rate.ID == "" {
		rate.ID = fmt.Sprintf("video-rate-%d", index)
		rate.ModelTier = rate.ID
	}
	if resolution := strings.TrimSpace(rule.Conditions["resolution"]); resolution != "" {
		rate.Resolution = resolution
	}
	if inputVideo, ok := parseBoolCondition(rule.Conditions["input_video"]); ok {
		rate.InputVideo = boolPtr(inputVideo)
	}
	if audio, ok := parseBoolCondition(rule.Conditions["audio"]); ok {
		rate.Audio = boolPtr(audio)
	}
	if duration, ok := parsePricingDecimal(rule.Conditions["duration_seconds"]); ok {
		rate.DurationSeconds = floatPtr(duration)
	}
	switch billingUnit {
	case "second":
		rate.PricePerSecond = floatPtr(value)
	case "task":
		rate.PricePerTask = floatPtr(value)
	case "million_video_tokens":
		rate.PricePerMillionTokens = floatPtr(value)
	default:
		return structuredVideoGenerationRate{}, false
	}
	return rate, true
}

func deriveMeteredFromRules(rules []structuredRule, currency string) []structuredMeteredPricing {
	metered := make([]structuredMeteredPricing, 0, len(rules))
	for _, rule := range rules {
		contract, ok := meteredPricingContracts[rule.Operation]
		if !ok || rule.Meter != contract.Meter || rule.Unit != contract.BaseUnit {
			continue
		}
		dimensionsRaw, err := encodeMeteredDimensions(rule.Conditions)
		if err != nil {
			continue
		}
		amount, err := json.Marshal(rule.Price)
		if err != nil {
			continue
		}
		metered = append(metered, structuredMeteredPricing{
			Operation:  rule.Operation,
			Meter:      rule.Meter,
			BaseUnit:   rule.Unit,
			Dimensions: dimensionsRaw,
			Price: structuredMeteredPrice{
				Amount:      amount,
				Currency:    currency,
				PerQuantity: 1,
			},
		})
	}
	if len(metered) == 0 {
		return nil
	}
	return metered
}

func validatePricingRules(rules []structuredRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("invalid pricing: rules must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		prefix := fmt.Sprintf("invalid pricing: rules[%d]", index)
		operation := strings.TrimSpace(rule.Operation)
		if _, ok := pricingRuleOperations[operation]; !ok {
			return fmt.Errorf("%s.operation is invalid", prefix)
		}
		meter := strings.TrimSpace(rule.Meter)
		unit := strings.TrimSpace(rule.Unit)
		expectedUnit, ok := pricingRuleMeterUnits[operation][meter]
		if !ok || expectedUnit != unit {
			return fmt.Errorf("%s must use a valid operation/meter/unit combination", prefix)
		}
		if !nonNegativeDecimalPattern.MatchString(strings.TrimSpace(rule.Price)) {
			return fmt.Errorf("%s.price must be a non-negative decimal string", prefix)
		}
		for key, value := range rule.Conditions {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s.conditions must contain only non-empty string values", prefix)
			}
		}
		key := pricingRuleIdentity(rule)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s duplicates an existing rule for the same operation, meter, unit, and conditions", prefix)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func pricingRuleIdentity(rule structuredRule) string {
	conditions := make([]string, 0, len(rule.Conditions))
	for key, value := range rule.Conditions {
		conditions = append(conditions, key+"="+value)
	}
	sort.Strings(conditions)
	return strings.Join([]string{
		rule.Operation,
		rule.Meter,
		rule.Unit,
		strings.Join(conditions, "&"),
	}, "|")
}

func pricingRuleMergeKey(rule structuredRule) string {
	if id := strings.TrimSpace(rule.ID); id != "" {
		return "id:" + id
	}
	return "identity:" + pricingRuleIdentity(rule)
}

func extractPricingOverrideRules(raw json.RawMessage) []structuredRule {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var envelope structuredPricing
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Rules) > 0 {
		return envelope.Rules
	}
	var rulesOnly struct {
		Rules []structuredRule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &rulesOnly); err == nil && len(rulesOnly.Rules) > 0 {
		return rulesOnly.Rules
	}
	return nil
}

func findPricingRuleOverride(rule structuredRule, byKey, byID map[string]structuredRule) (structuredRule, bool) {
	if override, ok := byKey[pricingRuleMergeKey(rule)]; ok {
		return override, true
	}
	if id := strings.TrimSpace(rule.ID); id != "" {
		if override, ok := byID[id]; ok {
			return override, true
		}
	}
	return structuredRule{}, false
}

func applyPricingRuleOverride(base, override structuredRule) structuredRule {
	if strings.TrimSpace(override.Price) != "" {
		base.Price = strings.TrimSpace(override.Price)
	}
	if override.Configured != nil {
		base.Configured = override.Configured
	}
	return base
}

func hasVideoGenerationRule(rules []structuredRule) bool {
	for _, rule := range rules {
		if rule.Operation == "video_generation" {
			return true
		}
	}
	return false
}

func inferPricingCurrency(pricing structuredPricing) string {
	if pricing.VideoGeneration != nil && strings.TrimSpace(pricing.VideoGeneration.Currency) != "" {
		return strings.TrimSpace(pricing.VideoGeneration.Currency)
	}
	for _, metered := range pricing.Metered {
		if strings.TrimSpace(metered.Price.Currency) != "" {
			return strings.TrimSpace(metered.Price.Currency)
		}
	}
	return "USD"
}

func dedupePricingRules(rules []structuredRule) []structuredRule {
	if len(rules) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(rules))
	deduped := make([]structuredRule, 0, len(rules))
	for _, rule := range rules {
		key := pricingRuleIdentity(rule)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, rule)
	}
	return deduped
}

func encodeMeteredDimensions(conditions map[string]string) (json.RawMessage, error) {
	if len(conditions) == 0 {
		return nil, nil
	}
	return json.Marshal(conditions)
}

func formatPricingDecimal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func parsePricingDecimal(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func parseBoolCondition(value string) (bool, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	switch trimmed {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func ruleConfigured(rule structuredRule) bool {
	return rule.Configured != nil && *rule.Configured
}

func boolPtr(value bool) *bool {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}
