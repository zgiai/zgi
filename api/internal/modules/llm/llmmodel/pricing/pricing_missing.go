package pricing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MissingPricingItem describes a required price that is not present.
type MissingPricingItem struct {
	UseCase   string `json:"use_case"`
	Operation string `json:"operation"`
	Meter     string `json:"meter"`
}

// PricingWriteOptions configures structured pricing normalization on write.
type PricingWriteOptions struct {
	UseCases     []string
	UserAuthored bool
}

type pricingRequirement struct {
	Operation string
	Meters    []string
	AnyMeter  bool
}

var useCaseAliases = map[string]string{
	"chat":              "text-chat",
	"embeddings":        "embedding",
	"text-embedding":    "embedding",
	"image-generation":  "image-gen",
	"image-edit":        "image-gen",
	"image-editing":     "image-gen",
	"video-generation":  "video-gen",
	"speech-generation": "text-to-speech",
	"tts":               "text-to-speech",
	"stt":               "speech-to-text",
	"transcription":     "speech-to-text",
	"music-generation":  "music-gen",
	"responses":         "moderation",
}

var useCaseRequirements = map[string]pricingRequirement{
	"text-chat":        {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"function-calling": {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"agent":            {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"vision":           {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"reasoning":        {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"realtime-audio":   {Operation: "chat", Meters: []string{"input_token", "output_token"}},
	"embedding":        {Operation: "embedding", Meters: []string{"input_token"}},
	"rerank":           {Operation: "rerank", Meters: []string{"input_token"}},
	"image-gen":        {Operation: "image_generation", Meters: []string{"image"}},
	"video-gen":        {Operation: "video_generation", AnyMeter: true},
	"speech-to-text":   {Operation: "transcription", Meters: []string{"input_audio_duration"}},
	"text-to-speech":   {Operation: "speech_generation", Meters: []string{"input_text"}},
	"music-gen":        {Operation: "music_generation", Meters: []string{"output_track"}},
}

// NormalizeUseCase maps backend aliases onto the canonical frontend use_case values.
func NormalizeUseCase(useCase string) string {
	key := strings.ToLower(strings.TrimSpace(useCase))
	if key == "" {
		return ""
	}
	if alias, ok := useCaseAliases[key]; ok {
		return alias
	}
	return key
}

// MissingPricingForUseCases returns objective missing pricing items for the given
// use_cases, ignoring configured flags.
func MissingPricingForUseCases(useCases []string, pricing json.RawMessage, scalars LegacyScalarPrices) []MissingPricingItem {
	return missingPricingForUseCases(useCases, pricing, scalars, false)
}

// MissingPricingForCreate enforces use_case requirements for new models. Empty
// use_cases default to text-chat so create cannot bypass token pricing.
func MissingPricingForCreate(useCases []string, pricing json.RawMessage, scalars LegacyScalarPrices) []MissingPricingItem {
	return missingPricingForUseCases(useCases, pricing, scalars, true)
}

func missingPricingForUseCases(useCases []string, pricing json.RawMessage, scalars LegacyScalarPrices, defaultWhenEmpty bool) []MissingPricingItem {
	normalized := normalizeUseCases(useCases)
	if len(normalized) == 0 && defaultWhenEmpty {
		normalized = []string{"text-chat"}
	}
	requirements := requiredPricingFromNormalizedUseCases(normalized)
	if len(requirements) == 0 {
		return nil
	}
	envelope := parsePricingEnvelope(pricing)
	missing := make([]MissingPricingItem, 0, len(requirements))
	for _, requirement := range requirements {
		if requirement.AnyMeter {
			if hasVideoPricing(envelope, scalars) {
				continue
			}
			missing = append(missing, MissingPricingItem{
				UseCase:   requirement.UseCase,
				Operation: requirement.Operation,
				Meter:     "video",
			})
			continue
		}
		for _, meter := range requirement.Meters {
			if hasOperationMeterPricing(envelope, scalars, requirement.Operation, meter) {
				continue
			}
			missing = append(missing, MissingPricingItem{
				UseCase:   requirement.UseCase,
				Operation: requirement.Operation,
				Meter:     meter,
			})
		}
	}
	return missing
}

// MissingPricingForOperations returns missing items limited to the requested
// operations while still evaluating against the full use_cases context.
func MissingPricingForOperations(operations map[string]bool, useCases []string, pricing json.RawMessage, scalars LegacyScalarPrices) []MissingPricingItem {
	if len(operations) == 0 {
		return nil
	}
	allMissing := MissingPricingForUseCases(useCases, pricing, scalars)
	if len(allMissing) == 0 {
		return nil
	}
	filtered := make([]MissingPricingItem, 0, len(allMissing))
	for _, item := range allMissing {
		if operations[item.Operation] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// PricingHasConfiguredRules reports whether pricing contains any configured=true rule.
func PricingHasConfiguredRules(pricing json.RawMessage) bool {
	envelope := parsePricingEnvelope(pricing)
	for _, rule := range envelope.Rules {
		if ruleConfigured(rule) {
			return true
		}
	}
	return false
}

// ConfiguredOperationsInPricing returns operations explicitly marked configured=true.
func ConfiguredOperationsInPricing(pricing json.RawMessage) map[string]bool {
	operations := make(map[string]bool)
	envelope := parsePricingEnvelope(pricing)
	for _, rule := range envelope.Rules {
		if !ruleConfigured(rule) {
			continue
		}
		operation := strings.TrimSpace(rule.Operation)
		if operation != "" {
			operations[operation] = true
		}
	}
	return operations
}

// TokenOperationsForUseCases returns chat/embedding/rerank operations implied by use_cases.
func TokenOperationsForUseCases(useCases []string) map[string]bool {
	operations := make(map[string]bool)
	for _, useCase := range normalizeUseCases(useCases) {
		switch useCase {
		case "text-chat", "function-calling", "agent", "vision", "reasoning", "realtime-audio":
			operations["chat"] = true
		case "embedding":
			operations["embedding"] = true
		case "rerank":
			operations["rerank"] = true
		}
	}
	if len(operations) == 0 {
		operations["chat"] = true
	}
	return operations
}

// FormatMissingPricingError formats missing items as a validation error.
func FormatMissingPricingError(items []MissingPricingItem) error {
	if len(items) == 0 {
		return nil
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Meter == "video" {
			parts = append(parts, fmt.Sprintf("%s requires %s pricing", item.UseCase, item.Operation))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s requires %s/%s pricing", item.UseCase, item.Operation, item.Meter))
	}
	return fmt.Errorf("missing pricing: %s", strings.Join(parts, "; "))
}

type requiredPricing struct {
	UseCase   string
	Operation string
	Meters    []string
	AnyMeter  bool
}

type pricingEnvelope struct {
	structuredPricing
	present bool
}

func normalizeUseCases(useCases []string) []string {
	if len(useCases) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(useCases))
	normalized := make([]string, 0, len(useCases))
	for _, useCase := range useCases {
		canonical := NormalizeUseCase(useCase)
		if canonical == "" {
			continue
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}
	return normalized
}

func requiredPricingFromNormalizedUseCases(normalized []string) []requiredPricing {
	if len(normalized) == 0 {
		return nil
	}
	seen := make(map[string]requiredPricing)
	for _, useCase := range normalized {
		requirement, ok := useCaseRequirements[useCase]
		if !ok {
			continue
		}
		key := requirement.Operation
		if requirement.AnyMeter {
			key += "|any"
		} else {
			key += "|" + strings.Join(requirement.Meters, ",")
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = requiredPricing{
			UseCase:   useCase,
			Operation: requirement.Operation,
			Meters:    append([]string{}, requirement.Meters...),
			AnyMeter:  requirement.AnyMeter,
		}
	}
	result := make([]requiredPricing, 0, len(seen))
	for _, requirement := range seen {
		result = append(result, requirement)
	}
	return result
}

func parsePricingEnvelope(raw json.RawMessage) pricingEnvelope {
	raw = NormalizeStructuredPricing(raw)
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return pricingEnvelope{}
	}
	var pricing structuredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return pricingEnvelope{}
	}
	return pricingEnvelope{structuredPricing: pricing, present: true}
}

func hasVideoPricing(envelope pricingEnvelope, _ LegacyScalarPrices) bool {
	if envelope.present {
		for _, rule := range envelope.Rules {
			if rule.Operation == "video_generation" && hasRulePrice(rule) {
				return true
			}
		}
		if envelope.VideoGeneration != nil && len(flattenVideoGenerationRates(envelope.VideoGeneration)) > 0 {
			return videoGenerationHasPrice(envelope.VideoGeneration)
		}
	}
	return false
}

func videoGenerationHasPrice(video *structuredVideoGenerationPricing) bool {
	for _, rate := range flattenVideoGenerationRates(video) {
		if rate.PricePerSecond != nil || rate.PricePerTask != nil || rate.PricePerMillionTokens != nil {
			return true
		}
	}
	return false
}

func hasOperationMeterPricing(envelope pricingEnvelope, scalars LegacyScalarPrices, operation, meter string) bool {
	if envelope.present {
		for _, rule := range envelope.Rules {
			if rule.Operation == operation && rule.Meter == meter && hasRulePrice(rule) {
				return true
			}
		}
	}
	return hasLegacyOperationMeterPricing(envelope, scalars, operation, meter)
}

func hasRulePrice(rule structuredRule) bool {
	_, ok := parsePricingDecimal(rule.Price)
	return ok
}

func hasLegacyOperationMeterPricing(envelope pricingEnvelope, scalars LegacyScalarPrices, operation, meter string) bool {
	switch operation {
	case "chat", "embedding", "rerank":
		return hasTokenMeterLegacyPricing(envelope, scalars, operation, meter)
	case "image_generation":
		if meter != "image" {
			return false
		}
		if envelope.present && envelope.PricePerImage != nil {
			return true
		}
		return false
	case "transcription", "speech_generation", "music_generation", "lyrics_generation":
		if !envelope.present {
			return false
		}
		contract, ok := meteredPricingContracts[operation]
		if !ok || contract.Meter != meter {
			return false
		}
		for _, metered := range envelope.Metered {
			if metered.Operation == operation && metered.Meter == meter && meteredHasPrice(metered) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func hasTokenMeterLegacyPricing(envelope pricingEnvelope, scalars LegacyScalarPrices, operation, meter string) bool {
	if envelope.present && len(envelope.TokenTiers) > 0 {
		tier := envelope.TokenTiers[0]
		switch meter {
		case "input_token":
			if tier.InputPricePerMillion != nil {
				return true
			}
		case "output_token":
			if tier.OutputPricePerMillion != nil {
				return true
			}
		case "cache_read_token":
			if tier.CachedInputPricePerMillion != nil {
				return true
			}
		case "cache_write_token":
			if tier.CacheWritePricePerMillion != nil {
				return true
			}
		case "cache_write_5m_token":
			if tier.CacheWrite5mPricePerMillion != nil {
				return true
			}
		case "cache_write_1h_token":
			if tier.CacheWrite1hPricePerMillion != nil {
				return true
			}
		}
	}
	switch meter {
	case "input_token":
		return scalars.InputPrice != nil
	case "output_token":
		return scalars.OutputPrice != nil
	case "cache_read_token":
		return scalars.CacheReadPrice != nil || scalars.CachedInputPrice != nil
	case "cache_write_token":
		return scalars.CacheWritePrice != nil
	case "cache_write_5m_token":
		return scalars.CacheWrite5mPrice != nil
	case "cache_write_1h_token":
		return scalars.CacheWrite1hPrice != nil
	default:
		return false
	}
}

func meteredHasPrice(metered structuredMeteredPricing) bool {
	var amount string
	if err := json.Unmarshal(metered.Price.Amount, &amount); err != nil {
		return false
	}
	_, ok := parsePricingDecimal(amount)
	return ok
}
