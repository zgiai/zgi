package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/billingsettings"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/ratingcore"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// NormalizedPointsPerUSD is the display conversion rate exposed to organization
// settings readers for fiat-to-points previews in the console UI.
func NormalizedPointsPerUSD() int64 {
	return billingsettings.Default().PointsPerUSD()
}

// localMilliPointsPerUSD is the local points_per_usd expressed in milli-points
// (points_per_usd x 1000), so USD -> milli-points stays exact. It replaces the
// retired compiled-in internal-unit constant and follows the catalog-synced
// points_per_usd instead.
func localMilliPointsPerUSD() int64 {
	return NormalizedPointsPerUSD() * millipoints.UnitsPerPoint
}

var (
	ErrPricingNotConfigured  = errors.New("model pricing is not configured")
	pricingPerMillionDivisor = decimal.NewFromInt(1000000)
)

type PricingModelSource string

const (
	PricingModelSourceGlobal      PricingModelSource = "global"
	PricingModelSourceCustom      PricingModelSource = "custom"
	PricingModelSourcePassthrough PricingModelSource = "passthrough"
)

type PricingModelRef struct {
	ModelID        uuid.UUID
	OrganizationID uuid.UUID
	Source         PricingModelSource
	Operation      PricingOperation
	Provider       string
	Model          string
}

type PricingQuote struct {
	InputUSD          decimal.Decimal
	CacheReadUSD      decimal.Decimal
	CacheWriteUSD     decimal.Decimal
	OutputUSD         decimal.Decimal
	TotalUSD          decimal.Decimal
	InputCredits      int64
	OutputCredits     int64
	TotalCredits      int64
	PricingSource     PricingSource
	UsageSource       UsageSource
	RuleID            string
	PricingSnapshot   datatypes.JSON
	ReservationPolicy string

	InputTokenPriceUSDPer1M   decimal.Decimal
	CacheReadPriceUSDPer1M    decimal.Decimal
	CacheWritePriceUSDPer1M   decimal.Decimal
	CacheWrite5mPriceUSDPer1M decimal.Decimal
	CacheWrite1hPriceUSDPer1M decimal.Decimal
	OutputTokenPriceUSDPer1M  decimal.Decimal
	InputTokenPriceResolved   bool
	CacheReadPriceResolved    bool
	CacheWritePriceResolved   bool
	CacheWrite5mPriceResolved bool
	CacheWrite1hPriceResolved bool
	OutputTokenPriceResolved  bool
	// CacheBillingSupported is derived from canonical pricing capability
	// metadata, never from legacy scalar cache columns.
	CacheBillingSupported *bool
	InputRuleID           string
	OutputRuleID          string

	MeteredOperation       PricingOperation
	MeteredMeter           string
	MeteredBaseUnit        string
	MeteredPriceUSDPerUnit decimal.Decimal
	MeteredPriceResolved   bool
}

// MeteredUsage identifies one non-token usage quantity against structured model pricing.
type MeteredUsage struct {
	Operation  PricingOperation
	Meter      string
	BaseUnit   string
	Quantity   int64
	Dimensions map[string]string
}

type TokenUsage struct {
	InputTokens        int
	CacheReadTokens    int
	CacheWriteTokens   int
	CacheWrite5mTokens int
	CacheWrite1hTokens int
	OutputTokens       int
	// AudioInputTokens / AudioOutputTokens carry the upstream realtime audio
	// token split (OpenAI input_tokens_details.audio_tokens /
	// output_tokens_details.audio_tokens). For realtime_audio they are
	// preferred over Input/OutputTokens; when absent the normal prompt /
	// completion counts are billed against the same realtime rules.
	AudioInputTokens  int
	AudioOutputTokens int
}

type PricingEngine interface {
	QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error)
	QuoteImage(ctx context.Context, model PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error)
	QuoteMetered(ctx context.Context, model PricingModelRef, usage MeteredUsage) (PricingQuote, error)
}

type pricingEngine struct {
	db          *gorm.DB
	columnCache sync.Map
}

type pricingModelRecord struct {
	ID                               uuid.UUID       `gorm:"column:id"`
	Provider                         string          `gorm:"column:provider"`
	Name                             string          `gorm:"column:name"`
	InputPrice                       decimal.Decimal `gorm:"column:input_price"`
	OutputPrice                      decimal.Decimal `gorm:"column:output_price"`
	CacheReadPrice                   decimal.Decimal `gorm:"column:cost_cache_read"`
	CacheWritePrice                  decimal.Decimal `gorm:"column:cost_cache_write"`
	CacheWrite5mPrice                decimal.Decimal `gorm:"column:cost_cache_write_5m"`
	CacheWrite1hPrice                decimal.Decimal `gorm:"column:cost_cache_write_1h"`
	InputPriceConfigured             bool            `gorm:"column:input_price_configured"`
	OutputPriceConfigured            bool            `gorm:"column:output_price_configured"`
	CacheReadPriceConfigured         bool            `gorm:"column:cache_read_price_configured"`
	CacheWritePriceConfigured        bool            `gorm:"column:cache_write_price_configured"`
	CacheWrite5mPriceConfigured      bool            `gorm:"column:cache_write_5m_price_configured"`
	CacheWrite1hPriceConfigured      bool            `gorm:"column:cache_write_1h_price_configured"`
	ImagePrices                      datatypes.JSON  `gorm:"column:image_prices"`
	Pricing                          datatypes.JSON  `gorm:"column:pricing"`
	CacheBillingSupported            bool            `gorm:"-"`
	InputPriceOrganizationOverride   bool            `gorm:"-"`
	OutputPriceOrganizationOverride  bool            `gorm:"-"`
	CacheReadOrganizationOverride    bool            `gorm:"-"`
	CacheWriteOrganizationOverride   bool            `gorm:"-"`
	CacheWrite5mOrganizationOverride bool            `gorm:"-"`
	CacheWrite1hOrganizationOverride bool            `gorm:"-"`
}

func NewPricingEngine(db *gorm.DB) PricingEngine {
	return &pricingEngine{db: db}
}

func (e *pricingEngine) QuoteTokens(ctx context.Context, ref PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error) {
	if promptTokens < 0 || completionTokens < 0 {
		return PricingQuote{}, fmt.Errorf("token count must be greater than or equal to zero")
	}
	ref = normalizePricingModelRef(ref)
	requiredPrices := tokenPricingRequirementForOperation(ref.Operation)
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}

	if ref.Operation == PricingOperationRealtimeAudio {
		return e.quoteRealtimeAudioTokens(ref, model, found, promptTokens, completionTokens)
	}

	if found && tokenPricesConfiguredForUsage(model, requiredPrices) {
		inputUSD := tokenUSD(model.InputPrice, promptTokens)
		outputUSD := tokenUSD(model.OutputPrice, completionTokens)
		snapshot := buildPricingSnapshot(map[string]interface{}{
			"pricing_source":                 PricingSourceUpstreamModelPrice,
			"usage_source":                   UsageSourceProviderUsage,
			"operation":                      ref.Operation,
			"model_id":                       model.ID.String(),
			"provider":                       strings.TrimSpace(model.Provider),
			"model":                          strings.TrimSpace(model.Name),
			"prompt_tokens":                  promptTokens,
			"completion_tokens":              completionTokens,
			"input_price_usd_per_1m_tokens":  model.InputPrice.String(),
			"output_price_usd_per_1m_tokens": model.OutputPrice.String(),
			"input_cost_usd":                 inputUSD.String(),
			"output_cost_usd":                outputUSD.String(),
			"input_price_source":             configuredPriceSource(model.InputPriceOrganizationOverride),
			"output_price_source":            configuredPriceSource(model.OutputPriceOrganizationOverride),
			"input_price_configured":         model.InputPriceConfigured,
			"output_price_configured":        model.OutputPriceConfigured,
		})
		quote := newUSDQuote(inputUSD, outputUSD, PricingSourceUpstreamModelPrice, "", UsageSourceProviderUsage, snapshot)
		quote.CacheBillingSupported = boolPtr(model.CacheBillingSupported)
		return withTokenPricingBasis(
			quote,
			model.InputPrice,
			model.OutputPrice,
			requiredPrices.input,
			requiredPrices.output,
			"",
			"",
		), nil
	}

	return e.quoteTokensWithFallback(ctx, ref, model, found, requiredPrices, promptTokens, completionTokens)
}

func (e *pricingEngine) QuoteTokenUsage(ctx context.Context, ref PricingModelRef, usage TokenUsage) (PricingQuote, error) {
	if usage.InputTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.CacheWrite5mTokens < 0 || usage.CacheWrite1hTokens < 0 || usage.OutputTokens < 0 || usage.AudioInputTokens < 0 || usage.AudioOutputTokens < 0 {
		return PricingQuote{}, fmt.Errorf("token count must be greater than or equal to zero")
	}
	usage = normalizeGatewayTokenUsage(usage)
	ref = normalizePricingModelRef(ref)
	if ref.Operation == PricingOperationRealtimeAudio {
		// Prefer upstream audio tokens; without them the normal prompt and
		// completion counts are billed against the same realtime_audio rules.
		usage = realtimeAudioTokenUsage(usage)
	}
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}
	// Cache usage is ignored unless the model's canonical pricing explicitly
	// declares cache meters. This makes historical scalar cache prices inert.
	if !found || model == nil || !model.CacheBillingSupported {
		usage.CacheReadTokens = 0
		usage.CacheWriteTokens = 0
		usage.CacheWrite5mTokens = 0
		usage.CacheWrite1hTokens = 0
	}
	quote, err := e.QuoteTokens(ctx, ref, usage.InputTokens, usage.OutputTokens)
	if err != nil {
		return PricingQuote{}, err
	}
	if usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 {
		return quote, nil
	}
	if !found || model == nil {
		return PricingQuote{}, fmt.Errorf("%w: cache token pricing requires a configured model", ErrPricingNotConfigured)
	}
	if usage.CacheReadTokens > 0 && !model.CacheReadPriceConfigured {
		return PricingQuote{}, fmt.Errorf("%w: cache read price is not configured", ErrPricingNotConfigured)
	}
	cacheWriteTokens := legacyCacheWriteTokens(usage)
	if cacheWriteTokens > 0 && !model.CacheWritePriceConfigured {
		return PricingQuote{}, fmt.Errorf("%w: cache write price is not configured", ErrPricingNotConfigured)
	}
	if usage.CacheWrite5mTokens > 0 && !cacheWrite5mPriceConfigured(model) {
		return PricingQuote{}, fmt.Errorf("%w: cache write 5m price is not configured", ErrPricingNotConfigured)
	}
	if usage.CacheWrite1hTokens > 0 && !cacheWrite1hPriceConfigured(model) {
		return PricingQuote{}, fmt.Errorf("%w: cache write 1h price is not configured", ErrPricingNotConfigured)
	}

	cacheWrite5mPrice, cacheWrite5mResolved, cacheWrite5mSource := resolveCacheWrite5mPrice(model)
	cacheWrite1hPrice, cacheWrite1hResolved, cacheWrite1hSource := resolveCacheWrite1hPrice(model)
	cacheReadUSD := tokenUSD(model.CacheReadPrice, usage.CacheReadTokens)
	cacheWriteLegacyUSD := tokenUSD(model.CacheWritePrice, cacheWriteTokens)
	cacheWrite5mUSD := tokenUSD(cacheWrite5mPrice, usage.CacheWrite5mTokens)
	cacheWrite1hUSD := tokenUSD(cacheWrite1hPrice, usage.CacheWrite1hTokens)
	cacheWriteUSD := cacheWriteLegacyUSD.Add(cacheWrite5mUSD).Add(cacheWrite1hUSD)
	inputUSD := quote.InputUSD.Add(cacheReadUSD).Add(cacheWriteUSD)
	snapshotValues := map[string]interface{}{}
	if len(quote.PricingSnapshot) > 0 {
		_ = json.Unmarshal(quote.PricingSnapshot, &snapshotValues)
	}
	snapshotValues["prompt_tokens"] = usage.InputTokens
	snapshotValues["cache_read_tokens"] = usage.CacheReadTokens
	snapshotValues["cache_write_tokens"] = usage.CacheWriteTokens
	snapshotValues["cache_write_legacy_tokens"] = cacheWriteTokens
	snapshotValues["cache_write_5m_tokens"] = usage.CacheWrite5mTokens
	snapshotValues["cache_write_1h_tokens"] = usage.CacheWrite1hTokens
	snapshotValues["completion_tokens"] = usage.OutputTokens
	snapshotValues["cache_read_price_usd_per_1m_tokens"] = model.CacheReadPrice.String()
	snapshotValues["cache_write_price_usd_per_1m_tokens"] = model.CacheWritePrice.String()
	snapshotValues["cache_write_5m_price_usd_per_1m_tokens"] = cacheWrite5mPrice.String()
	snapshotValues["cache_write_1h_price_usd_per_1m_tokens"] = cacheWrite1hPrice.String()
	snapshotValues["cache_read_price_source"] = cachePriceSource(model.CacheReadOrganizationOverride)
	snapshotValues["cache_write_price_source"] = cachePriceSource(model.CacheWriteOrganizationOverride)
	snapshotValues["cache_write_5m_price_source"] = cacheWrite5mSource
	snapshotValues["cache_write_1h_price_source"] = cacheWrite1hSource
	snapshotValues["input_cost_usd"] = quote.InputUSD.String()
	snapshotValues["cache_read_cost_usd"] = cacheReadUSD.String()
	snapshotValues["cache_write_cost_usd"] = cacheWriteUSD.String()
	snapshotValues["cache_write_legacy_cost_usd"] = cacheWriteLegacyUSD.String()
	snapshotValues["cache_write_5m_cost_usd"] = cacheWrite5mUSD.String()
	snapshotValues["cache_write_1h_cost_usd"] = cacheWrite1hUSD.String()
	snapshotValues["output_cost_usd"] = quote.OutputUSD.String()

	result := newUSDQuote(inputUSD, quote.OutputUSD, quote.PricingSource, quote.RuleID, quote.UsageSource, buildPricingSnapshot(snapshotValues))
	result.CacheReadUSD = cacheReadUSD
	result.CacheWriteUSD = cacheWriteUSD
	result.InputTokenPriceUSDPer1M = quote.InputTokenPriceUSDPer1M
	result.OutputTokenPriceUSDPer1M = quote.OutputTokenPriceUSDPer1M
	result.InputTokenPriceResolved = quote.InputTokenPriceResolved
	result.OutputTokenPriceResolved = quote.OutputTokenPriceResolved
	result.InputRuleID = quote.InputRuleID
	result.OutputRuleID = quote.OutputRuleID
	result.CacheReadPriceUSDPer1M = model.CacheReadPrice
	result.CacheWritePriceUSDPer1M = model.CacheWritePrice
	result.CacheWrite5mPriceUSDPer1M = cacheWrite5mPrice
	result.CacheWrite1hPriceUSDPer1M = cacheWrite1hPrice
	result.CacheReadPriceResolved = model.CacheReadPriceConfigured
	result.CacheWritePriceResolved = model.CacheWritePriceConfigured
	result.CacheWrite5mPriceResolved = cacheWrite5mResolved
	result.CacheWrite1hPriceResolved = cacheWrite1hResolved
	result.CacheBillingSupported = boolPtr(model.CacheBillingSupported)
	return result, nil
}

// quoteRealtimeAudioTokens bills a realtime audio request from the model's
// canonical realtime_audio rules (audio input/output tokens). The chat scalar
// columns represent text tokens and must not be used for audio billing.
func (e *pricingEngine) quoteRealtimeAudioTokens(
	ref PricingModelRef,
	model *pricingModelRecord,
	found bool,
	inputTokens int,
	outputTokens int,
) (PricingQuote, error) {
	if !found || model == nil {
		return PricingQuote{}, fmt.Errorf("%w: realtime audio pricing requires a configured model", ErrPricingNotConfigured)
	}
	inputPrice, outputPrice, ok, err := resolveRealtimeAudioTokenPrices(model.Pricing)
	if err != nil {
		return PricingQuote{}, err
	}
	if !ok {
		return PricingQuote{}, fmt.Errorf("%w: realtime audio token pricing is not configured", ErrPricingNotConfigured)
	}
	inputUSD := tokenUSD(inputPrice, inputTokens)
	outputUSD := tokenUSD(outputPrice, outputTokens)
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                 PricingSourceUpstreamModelPrice,
		"usage_source":                   UsageSourceProviderUsage,
		"operation":                      ref.Operation,
		"model_id":                       model.ID.String(),
		"provider":                       strings.TrimSpace(model.Provider),
		"model":                          strings.TrimSpace(model.Name),
		"prompt_tokens":                  inputTokens,
		"completion_tokens":              outputTokens,
		"input_price_usd_per_1m_tokens":  inputPrice.String(),
		"output_price_usd_per_1m_tokens": outputPrice.String(),
		"input_cost_usd":                 inputUSD.String(),
		"output_cost_usd":                outputUSD.String(),
	})
	quote := newUSDQuote(inputUSD, outputUSD, PricingSourceUpstreamModelPrice, "", UsageSourceProviderUsage, snapshot)
	return withTokenPricingBasis(quote, inputPrice, outputPrice, true, true, "", ""), nil
}

func resolveRealtimeAudioTokenPrices(raw datatypes.JSON) (decimal.Decimal, decimal.Decimal, bool, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return decimal.Zero, decimal.Zero, false, nil
	}
	var envelope struct {
		Rules []struct {
			Operation string          `json:"operation"`
			Meter     string          `json:"meter"`
			Price     json.RawMessage `json:"price"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return decimal.Zero, decimal.Zero, false, fmt.Errorf("%w: invalid realtime audio pricing: %v", ErrPricingNotConfigured, err)
	}
	operation := string(PricingOperationRealtimeAudio)
	var inputPrice, outputPrice decimal.Decimal
	var hasInput, hasOutput bool
	for _, rule := range envelope.Rules {
		if rule.Operation != operation {
			continue
		}
		amount, ok := parseRealtimeAudioRulePrice(rule.Price)
		if !ok {
			continue
		}
		switch rule.Meter {
		case string(PricingMeterInputToken):
			inputPrice, hasInput = amount, true
		case string(PricingMeterOutputToken):
			outputPrice, hasOutput = amount, true
		}
	}
	if !hasInput && !hasOutput {
		return decimal.Zero, decimal.Zero, false, nil
	}
	return inputPrice, outputPrice, true, nil
}

func parseRealtimeAudioRulePrice(raw json.RawMessage) (decimal.Decimal, bool) {
	if len(raw) == 0 {
		return decimal.Zero, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		amount, err := decimal.NewFromString(strings.TrimSpace(text))
		if err != nil || amount.IsNegative() {
			return decimal.Zero, false
		}
		return amount, true
	}
	// Explicit boundary: legacy pricing JSON can encode the amount as a JSON
	// number. Convert once here; every pricing computation afterward is decimal.
	var number float64
	if err := json.Unmarshal(raw, &number); err != nil || number < 0 {
		return decimal.Zero, false
	}
	return decimal.NewFromFloat(number), true
}

func normalizeGatewayTokenUsage(usage TokenUsage) TokenUsage {
	if usage.CacheWriteTokens == 0 {
		usage.CacheWriteTokens = usage.CacheWrite5mTokens + usage.CacheWrite1hTokens
	}
	return usage
}

// realtimeAudioTokenUsage prefers upstream audio_tokens when present and falls
// back to the prompt/completion token counts otherwise. Missing audio tokens
// must not fail closed: a modality-aware realtime price is still billed from
// the text-token split against the same realtime_audio rules. This mirrors the
// pure ratingcore fallback so Pre rates realtime audio identically.
func realtimeAudioTokenUsage(usage TokenUsage) TokenUsage {
	if usage.AudioInputTokens > 0 {
		usage.InputTokens = usage.AudioInputTokens
	}
	if usage.AudioOutputTokens > 0 {
		usage.OutputTokens = usage.AudioOutputTokens
	}
	return usage
}

func legacyCacheWriteTokens(usage TokenUsage) int {
	tokens := usage.CacheWriteTokens - usage.CacheWrite5mTokens - usage.CacheWrite1hTokens
	if tokens < 0 {
		return 0
	}
	return tokens
}

func cacheWrite5mPriceConfigured(model *pricingModelRecord) bool {
	if model == nil {
		return false
	}
	return model.CacheWrite5mPriceConfigured || model.CacheWritePriceConfigured
}

func cacheWrite1hPriceConfigured(model *pricingModelRecord) bool {
	if model == nil {
		return false
	}
	return model.CacheWrite1hPriceConfigured || model.CacheWritePriceConfigured
}

func resolveCacheWrite5mPrice(model *pricingModelRecord) (decimal.Decimal, bool, string) {
	if model == nil {
		return decimal.Zero, false, ""
	}
	if model.CacheWrite5mPriceConfigured {
		return model.CacheWrite5mPrice, true, cachePriceSource(model.CacheWrite5mOrganizationOverride)
	}
	if model.CacheWritePriceConfigured {
		return model.CacheWritePrice, true, cachePriceSource(model.CacheWriteOrganizationOverride)
	}
	return decimal.Zero, false, ""
}

func resolveCacheWrite1hPrice(model *pricingModelRecord) (decimal.Decimal, bool, string) {
	if model == nil {
		return decimal.Zero, false, ""
	}
	if model.CacheWrite1hPriceConfigured {
		return model.CacheWrite1hPrice, true, cachePriceSource(model.CacheWrite1hOrganizationOverride)
	}
	if model.CacheWritePriceConfigured {
		return model.CacheWritePrice, true, cachePriceSource(model.CacheWriteOrganizationOverride)
	}
	return decimal.Zero, false, ""
}

func cachePriceSource(organizationOverride bool) string {
	return configuredPriceSource(organizationOverride)
}

func configuredPriceSource(organizationOverride bool) string {
	if organizationOverride {
		return "organization_override"
	}
	return "synced_model"
}

type tokenPricingRequirement struct {
	input  bool
	output bool
}

func tokenPricingRequirementForOperation(operation PricingOperation) tokenPricingRequirement {
	switch operation {
	case PricingOperationEmbedding, PricingOperationRerank:
		return tokenPricingRequirement{input: true}
	default:
		return tokenPricingRequirement{input: true, output: true}
	}
}

func tokenPricesConfiguredForUsage(model *pricingModelRecord, requiredPrices tokenPricingRequirement) bool {
	if model == nil {
		return false
	}
	if requiredPrices.input && !model.InputPriceConfigured {
		return false
	}
	if requiredPrices.output && !model.OutputPriceConfigured {
		return false
	}
	return true
}

func (e *pricingEngine) QuoteImage(ctx context.Context, ref PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error) {
	if req == nil {
		return PricingQuote{}, fmt.Errorf("image pricing request is nil")
	}
	ref = normalizePricingModelRef(PricingModelRef{
		ModelID:        ref.ModelID,
		OrganizationID: ref.OrganizationID,
		Source:         ref.Source,
		Operation:      PricingOperationImage,
		Provider:       ref.Provider,
		Model:          ref.Model,
	})

	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}

	r := *req
	if strings.TrimSpace(r.Size) == "" {
		r.Size = "1024x1024"
	}
	if strings.TrimSpace(r.Quality) == "" {
		r.Quality = "standard"
	}

	count := int64(1)
	if req.N != nil && *req.N > 0 {
		count = int64(*req.N)
	}

	if found && model != nil {
		if quote, ok := quoteConfiguredOrganizationImageUnitPrice(model, &r, count); ok {
			return quote, nil
		}
		if quote, ok := quoteConfiguredStructuredImagePrice(model, &r, count); ok {
			return quote, nil
		}
		prices, configured, err := configuredImagePricingRules(model)
		if err != nil {
			return PricingQuote{}, err
		}
		if configured {
			matched, err := matchImagePricingRule(&r, prices)
			if err != nil {
				return PricingQuote{}, err
			}
			if matched != nil {
				return quoteConfiguredImageRule(model, &r, *matched, count)
			}
		}
		if !configured {
			if quote, ok := quoteConfiguredImageUnitPrice(model, &r, count); ok {
				return quote, nil
			}
		}
	}

	return e.quoteImageWithFallback(ctx, ref, model, found, &r, count)
}

// ImageTokenUsage is the upstream image token split used to bill the
// three-tier image_token_pricing block: text input, image input, and output.
type ImageTokenUsage struct {
	TextInputTokens  int
	ImageInputTokens int
	OutputTokens     int
}

// imageTokenPricingEngine is the optional pricing capability for token-metered
// image generation. It is deliberately separate from PricingEngine so fake
// engines used by unrelated tests keep the per-image path.
type imageTokenPricingEngine interface {
	QuoteImageTokenUsage(ctx context.Context, ref PricingModelRef, usage ImageTokenUsage) (PricingQuote, error)
}

// QuoteImageTokenUsage rates image generation from upstream token usage with the
// explicit three-tier image_token_pricing block. It never falls back to the flat
// per-image price: a model without the block is not token priced. The amount is
// produced by the shared ratingcore kernel so Pre and Console agree on T1.
func (e *pricingEngine) QuoteImageTokenUsage(ctx context.Context, ref PricingModelRef, usage ImageTokenUsage) (PricingQuote, error) {
	ref = normalizePricingModelRef(PricingModelRef{
		ModelID:        ref.ModelID,
		OrganizationID: ref.OrganizationID,
		Source:         ref.Source,
		Operation:      PricingOperationImage,
		Provider:       ref.Provider,
		Model:          ref.Model,
	})
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}
	if !found || model == nil {
		return PricingQuote{}, fmt.Errorf("%w: image token pricing requires a configured model", ErrPricingNotConfigured)
	}
	var pricing ratingcore.Pricing
	if err := json.Unmarshal(model.Pricing, &pricing); err != nil {
		return PricingQuote{}, fmt.Errorf("%w: invalid image token pricing: %v", ErrPricingNotConfigured, err)
	}
	if pricing.ImageTokenPricing == nil {
		return PricingQuote{}, fmt.Errorf("%w: image token pricing is not configured", ErrPricingNotConfigured)
	}
	operation := string(PricingOperationImage)
	inputUSD, err := ratingcore.Resolve(pricing, ratingcore.Usage{
		Operation:        operation,
		Meter:            "input_text_token",
		BaseUnit:         "per_1m_tokens",
		TextInputTokens:  int64(usage.TextInputTokens),
		InputImageTokens: int64(usage.ImageInputTokens),
	})
	if err != nil {
		return PricingQuote{}, fmt.Errorf("%w: %v", ErrPricingNotConfigured, err)
	}
	outputUSD, err := ratingcore.Resolve(pricing, ratingcore.Usage{
		Operation:    operation,
		Meter:        "output_token",
		BaseUnit:     "per_1m_tokens",
		OutputTokens: int64(usage.OutputTokens),
	})
	if err != nil {
		return PricingQuote{}, fmt.Errorf("%w: %v", ErrPricingNotConfigured, err)
	}
	tiers := pricing.ImageTokenPricing
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                PricingSourceUpstreamModelPrice,
		"usage_source":                  UsageSourceProviderUsage,
		"operation":                     PricingOperationImage,
		"model_id":                      model.ID.String(),
		"provider":                      strings.TrimSpace(model.Provider),
		"model":                         strings.TrimSpace(model.Name),
		"text_input_tokens":             usage.TextInputTokens,
		"image_input_tokens":            usage.ImageInputTokens,
		"output_tokens":                 usage.OutputTokens,
		"text_input_price_per_million":  tiers.TextInputPricePerMillion.String(),
		"image_input_price_per_million": tiers.ImageInputPricePerMillion.String(),
		"output_price_per_million":      tiers.OutputPricePerMillion.String(),
		"input_cost_usd":                inputUSD.String(),
		"output_cost_usd":               outputUSD.String(),
	})
	quote := newUSDQuote(inputUSD, outputUSD, PricingSourceUpstreamModelPrice, "pricing.image_token_pricing", UsageSourceProviderUsage, snapshot)
	quote.InputTokenPriceUSDPer1M = tiers.TextInputPricePerMillion
	quote.OutputTokenPriceUSDPer1M = tiers.OutputPricePerMillion
	quote.InputTokenPriceResolved = true
	quote.OutputTokenPriceResolved = true
	return quote, nil
}

func configuredImagePricingRules(model *pricingModelRecord) ([]llmmodel.PricingRule, bool, error) {
	if model == nil || len(model.ImagePrices) == 0 || string(model.ImagePrices) == "null" {
		return nil, false, nil
	}
	var prices []llmmodel.PricingRule
	if err := json.Unmarshal(model.ImagePrices, &prices); err != nil {
		return nil, false, fmt.Errorf("invalid image pricing: %w", err)
	}
	if len(prices) == 0 {
		return nil, false, nil
	}
	return prices, true, nil
}

func (e *pricingEngine) quoteTokensWithFallback(
	ctx context.Context,
	ref PricingModelRef,
	model *pricingModelRecord,
	found bool,
	requiredPrices tokenPricingRequirement,
	promptTokens int,
	completionTokens int,
) (PricingQuote, error) {
	config, err := LoadPricingFallbackConfig(ctx, e.db, ref.OrganizationID)
	if err != nil {
		return PricingQuote{}, err
	}
	if !config.Enabled {
		return PricingQuote{}, fmt.Errorf("%w: missing token pricing and fallback pricing is disabled", ErrPricingNotConfigured)
	}

	provider, modelName := pricingModelIdentity(ref, model, found)
	var inputRule PricingFallbackRule
	var outputRule PricingFallbackRule
	var inputPrice decimal.Decimal
	var outputPrice decimal.Decimal
	var ruleIDs []string
	inputFromModel := false
	outputFromModel := false

	if requiredPrices.input {
		if model != nil && model.InputPriceConfigured {
			inputPrice = model.InputPrice
			inputFromModel = true
		} else {
			inputRule, err = findTokenFallbackRule(config.EffectiveRules, ref.Operation, PricingMeterInputToken, provider, modelName)
			if err != nil {
				return PricingQuote{}, err
			}
			inputPrice, err = parseFallbackTokenPrice(inputRule)
			if err != nil {
				return PricingQuote{}, err
			}
			ruleIDs = append(ruleIDs, inputRule.ID)
		}
	}
	if requiredPrices.output {
		if model != nil && model.OutputPriceConfigured {
			outputPrice = model.OutputPrice
			outputFromModel = true
		} else {
			outputRule, err = findTokenFallbackRule(config.EffectiveRules, ref.Operation, PricingMeterOutputToken, provider, modelName)
			if err != nil {
				return PricingQuote{}, err
			}
			outputPrice, err = parseFallbackTokenPrice(outputRule)
			if err != nil {
				return PricingQuote{}, err
			}
			ruleIDs = append(ruleIDs, outputRule.ID)
		}
	}

	source := PricingSourceUpstreamModelPrice
	if len(ruleIDs) > 0 {
		source = fallbackPricingSource(inputRule, outputRule)
	}
	ruleID := strings.Join(ruleIDs, ",")
	inputUSD := tokenUSD(inputPrice, promptTokens)
	outputUSD := tokenUSD(outputPrice, completionTokens)
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                 source,
		"usage_source":                   UsageSourceProviderUsage,
		"operation":                      ref.Operation,
		"model_id":                       pricingModelID(model),
		"provider":                       provider,
		"model":                          modelName,
		"prompt_tokens":                  promptTokens,
		"completion_tokens":              completionTokens,
		"input_price_usd_per_1m_tokens":  inputPrice.String(),
		"output_price_usd_per_1m_tokens": outputPrice.String(),
		"input_cost_usd":                 inputUSD.String(),
		"output_cost_usd":                outputUSD.String(),
		"input_rule_id":                  inputRule.ID,
		"output_rule_id":                 outputRule.ID,
		"input_rule_source":              inputRule.PricingSource,
		"output_rule_source":             outputRule.PricingSource,
		"input_price_from_model":         inputFromModel,
		"output_price_from_model":        outputFromModel,
		"input_price_source":             resolvedTokenPriceSource(inputFromModel, modelPriceOrganizationOverride(model, true), inputRule.PricingSource),
		"output_price_source":            resolvedTokenPriceSource(outputFromModel, modelPriceOrganizationOverride(model, false), outputRule.PricingSource),
	})

	quote := newUSDQuote(inputUSD, outputUSD, source, ruleID, UsageSourceProviderUsage, snapshot)
	if model != nil {
		quote.CacheBillingSupported = boolPtr(model.CacheBillingSupported)
	}
	return withTokenPricingBasis(
		quote,
		inputPrice,
		outputPrice,
		requiredPrices.input,
		requiredPrices.output,
		inputRule.ID,
		outputRule.ID,
	), nil
}

func resolvedTokenPriceSource(fromModel bool, organizationOverride bool, fallbackSource PricingSource) string {
	if fromModel {
		return configuredPriceSource(organizationOverride)
	}
	return string(fallbackSource)
}

func modelPriceOrganizationOverride(model *pricingModelRecord, input bool) bool {
	if model == nil {
		return false
	}
	if input {
		return model.InputPriceOrganizationOverride
	}
	return model.OutputPriceOrganizationOverride
}

func (e *pricingEngine) quoteImageWithFallback(
	ctx context.Context,
	ref PricingModelRef,
	model *pricingModelRecord,
	found bool,
	req *adapter.ImageRequest,
	count int64,
) (PricingQuote, error) {
	config, err := LoadPricingFallbackConfig(ctx, e.db, ref.OrganizationID)
	if err != nil {
		return PricingQuote{}, err
	}
	if !config.Enabled {
		return PricingQuote{}, fmt.Errorf("%w: missing image pricing and fallback pricing is disabled", ErrPricingNotConfigured)
	}

	provider, modelName := pricingModelIdentity(ref, model, found)
	provider = canonicalImagePricingProvider(provider)
	if provider == "" || provider == "*" {
		provider = inferImagePricingProvider(req)
	}
	rule, err := findImageFallbackRule(config.EffectiveRules, provider, modelName, req)
	if err != nil {
		return PricingQuote{}, err
	}
	totalCredits := rule.Credits * count
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":    rule.PricingSource,
		"usage_source":      UsageSourceRequestParameters,
		"operation":         PricingOperationImage,
		"model_id":          pricingModelID(model),
		"provider":          provider,
		"model":             modelName,
		"request_model":     strings.TrimSpace(req.Model),
		"size":              strings.TrimSpace(req.Size),
		"quality":           strings.TrimSpace(req.Quality),
		"style":             strings.TrimSpace(req.Style),
		"image_count":       count,
		"credits_per_image": rule.Credits,
		"rule_id":           rule.ID,
	})
	return PricingQuote{
		OutputCredits:   totalCredits,
		TotalCredits:    totalCredits,
		PricingSource:   rule.PricingSource,
		UsageSource:     UsageSourceRequestParameters,
		RuleID:          rule.ID,
		PricingSnapshot: snapshot,
	}, nil
}

func quoteConfiguredImageRule(model *pricingModelRecord, req *adapter.ImageRequest, matched llmmodel.PricingRule, count int64) (PricingQuote, error) {
	if matched.Price.Amount > 0 {
		// Explicit boundary: the structured rule amount is a legacy JSON float.
		totalUSD := decimal.NewFromFloat(matched.Price.Amount).Mul(decimal.NewFromInt(count))
		snapshot := configuredImagePricingSnapshot(model, req, matched, count, matched.Price.Amount, 0)
		return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, matched.ID, UsageSourceRequestParameters, snapshot), nil
	}

	if matched.Price.Credits <= 0 {
		return PricingQuote{}, fmt.Errorf("%w: image pricing rule %q has no price", ErrPricingNotConfigured, matched.ID)
	}

	totalCredits := matched.Price.Credits * count
	snapshot := configuredImagePricingSnapshot(model, req, matched, count, 0, matched.Price.Credits)
	return PricingQuote{
		TotalCredits:    totalCredits,
		OutputCredits:   totalCredits,
		PricingSource:   PricingSourceUpstreamModelPrice,
		UsageSource:     UsageSourceRequestParameters,
		RuleID:          matched.ID,
		PricingSnapshot: snapshot,
	}, nil
}

func quoteConfiguredImageUnitPrice(model *pricingModelRecord, req *adapter.ImageRequest, count int64) (PricingQuote, bool) {
	price, column, ok := configuredImageUnitPrice(model)
	if !ok {
		return PricingQuote{}, false
	}
	totalUSD := price.Mul(decimal.NewFromInt(count))
	snapshot := configuredImageUnitPriceSnapshot(model, req, count, price, column)
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, "", UsageSourceRequestParameters, snapshot), true
}

// quoteConfiguredStructuredImagePrice reads the canonical model pricing
// snapshot written by Console. This bridges the newer price_per_image field
// to the legacy image_prices/output_price runtime projections.
func quoteConfiguredStructuredImagePrice(model *pricingModelRecord, req *adapter.ImageRequest, count int64) (PricingQuote, bool) {
	if model == nil || len(model.Pricing) == 0 || string(model.Pricing) == "null" {
		return PricingQuote{}, false
	}
	var pricing struct {
		PricePerImage *float64 `json:"price_per_image"`
	}
	if err := json.Unmarshal(model.Pricing, &pricing); err != nil || pricing.PricePerImage == nil || *pricing.PricePerImage < 0 {
		return PricingQuote{}, false
	}
	// Explicit boundary: price_per_image is projected from legacy JSON as a
	// float. Convert once here and keep the pricing arithmetic decimal.
	price := decimal.NewFromFloat(*pricing.PricePerImage)
	totalUSD := price.Mul(decimal.NewFromInt(count))
	provider, modelName := pricingModelIdentity(PricingModelRef{}, model, true)
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source": PricingSourceUpstreamModelPrice,
		"usage_source":   UsageSourceRequestParameters,
		"operation":      PricingOperationImage,
		"model_id":       pricingModelID(model),
		"provider":       provider,
		"model":          modelName,
		"request_model":  strings.TrimSpace(req.Model),
		"size":           strings.TrimSpace(req.Size),
		"quality":        strings.TrimSpace(req.Quality),
		"image_count":    count,
		"usd_per_image":  price.String(),
		"price_column":   "pricing.price_per_image",
	})
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, "pricing.price_per_image", UsageSourceRequestParameters, snapshot), true
}

func quoteConfiguredOrganizationImageUnitPrice(model *pricingModelRecord, req *adapter.ImageRequest, count int64) (PricingQuote, bool) {
	price, column, ok := configuredOrganizationImageUnitPrice(model)
	if !ok {
		return PricingQuote{}, false
	}
	totalUSD := price.Mul(decimal.NewFromInt(count))
	snapshot := configuredImageUnitPriceSnapshot(model, req, count, price, column)
	return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, "", UsageSourceRequestParameters, snapshot), true
}

func configuredOrganizationImageUnitPrice(model *pricingModelRecord) (decimal.Decimal, string, bool) {
	if model == nil {
		return decimal.Zero, "", false
	}
	if model.OutputPriceOrganizationOverride {
		return model.OutputPrice, "output_price_override", true
	}
	if model.InputPriceOrganizationOverride {
		return model.InputPrice, "input_price_override", true
	}
	return decimal.Zero, "", false
}

func configuredImageUnitPrice(model *pricingModelRecord) (decimal.Decimal, string, bool) {
	if model == nil {
		return decimal.Zero, "", false
	}
	if model.OutputPriceConfigured || model.OutputPrice.IsPositive() {
		return model.OutputPrice, "output_price", true
	}
	if model.InputPriceConfigured || model.InputPrice.IsPositive() {
		return model.InputPrice, "input_price", true
	}
	return decimal.Zero, "", false
}

func configuredImageUnitPriceSnapshot(model *pricingModelRecord, req *adapter.ImageRequest, count int64, usdPerImage decimal.Decimal, column string) datatypes.JSON {
	provider, modelName := pricingModelIdentity(PricingModelRef{}, model, model != nil)
	return buildPricingSnapshot(map[string]interface{}{
		"pricing_source":       PricingSourceUpstreamModelPrice,
		"usage_source":         UsageSourceRequestParameters,
		"operation":            PricingOperationImage,
		"model_id":             pricingModelID(model),
		"provider":             provider,
		"model":                modelName,
		"request_model":        strings.TrimSpace(req.Model),
		"size":                 strings.TrimSpace(req.Size),
		"quality":              strings.TrimSpace(req.Quality),
		"style":                strings.TrimSpace(req.Style),
		"image_count":          count,
		"usd_per_image":        usdPerImage.String(),
		"price_column":         column,
		"scalar_image_pricing": true,
	})
}

func configuredImagePricingSnapshot(model *pricingModelRecord, req *adapter.ImageRequest, rule llmmodel.PricingRule, count int64, usdPerImage float64, creditsPerImage int64) datatypes.JSON {
	provider, modelName := pricingModelIdentity(PricingModelRef{}, model, model != nil)
	return buildPricingSnapshot(map[string]interface{}{
		"pricing_source":    PricingSourceUpstreamModelPrice,
		"usage_source":      UsageSourceRequestParameters,
		"operation":         PricingOperationImage,
		"model_id":          pricingModelID(model),
		"provider":          provider,
		"model":             modelName,
		"request_model":     strings.TrimSpace(req.Model),
		"size":              strings.TrimSpace(req.Size),
		"quality":           strings.TrimSpace(req.Quality),
		"style":             strings.TrimSpace(req.Style),
		"image_count":       count,
		"rule_id":           rule.ID,
		"usd_per_image":     usdPerImage,
		"credits_per_image": creditsPerImage,
	})
}

func (e *pricingEngine) loadModel(ctx context.Context, ref PricingModelRef) (*pricingModelRecord, bool, error) {
	ref = normalizePricingModelRef(ref)
	if ref.ModelID == uuid.Nil || ref.Source == PricingModelSourcePassthrough {
		return nil, false, nil
	}

	switch ref.Source {
	case PricingModelSourceGlobal:
		record, found, err := e.loadModelFromTable(ctx, "llm_models", ref.ModelID)
		if err != nil || !found || ref.OrganizationID == uuid.Nil {
			return record, found, err
		}
		if err := e.applyOrganizationPriceOverride(ctx, record, ref.OrganizationID); err != nil {
			return nil, false, err
		}
		return record, found, nil
	case PricingModelSourceCustom:
		return e.loadModelFromTable(ctx, "llm_custom_models", ref.ModelID)
	default:
		return nil, false, fmt.Errorf("unknown pricing model source %q", ref.Source)
	}
}

type pricingModelConfigOverride struct {
	InputPriceOverride        *decimal.Decimal `gorm:"column:input_price_override"`
	OutputPriceOverride       *decimal.Decimal `gorm:"column:output_price_override"`
	CacheReadPriceOverride    *decimal.Decimal `gorm:"column:cache_read_price_override"`
	CacheWritePriceOverride   *decimal.Decimal `gorm:"column:cache_write_price_override"`
	CacheWrite5mPriceOverride *decimal.Decimal `gorm:"column:cache_write_5m_price_override"`
	CacheWrite1hPriceOverride *decimal.Decimal `gorm:"column:cache_write_1h_price_override"`
	PricingOverride           datatypes.JSON   `gorm:"column:pricing_override"`
}

func (e *pricingEngine) applyOrganizationPriceOverride(ctx context.Context, record *pricingModelRecord, organizationID uuid.UUID) error {
	if record == nil || organizationID == uuid.Nil {
		return nil
	}

	var cfg pricingModelConfigOverride
	err := e.db.WithContext(ctx).
		Table("llm_model_configs").
		Select(e.modelConfigOverrideColumns()).
		Where("organization_id = ? AND model_id = ? AND deleted_at IS NULL", organizationID, record.ID).
		First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingPricingTableError(err, "llm_model_configs") {
			return nil
		}
		return err
	}
	if cfg.InputPriceOverride != nil {
		record.InputPrice = *cfg.InputPriceOverride
		record.InputPriceOrganizationOverride = true
	}
	if cfg.OutputPriceOverride != nil {
		record.OutputPrice = *cfg.OutputPriceOverride
		record.OutputPriceOrganizationOverride = true
	}
	if cfg.CacheReadPriceOverride != nil {
		record.CacheReadPrice = *cfg.CacheReadPriceOverride
		record.CacheReadOrganizationOverride = true
	}
	if cfg.CacheWritePriceOverride != nil {
		record.CacheWritePrice = *cfg.CacheWritePriceOverride
		record.CacheWriteOrganizationOverride = true
	}
	if cfg.CacheWrite5mPriceOverride != nil {
		record.CacheWrite5mPrice = *cfg.CacheWrite5mPriceOverride
		record.CacheWrite5mOrganizationOverride = true
	}
	if cfg.CacheWrite1hPriceOverride != nil {
		record.CacheWrite1hPrice = *cfg.CacheWrite1hPriceOverride
		record.CacheWrite1hOrganizationOverride = true
	}
	if len(cfg.PricingOverride) > 0 && string(cfg.PricingOverride) != "null" {
		merged := modelpricing.MergeOrganizationPricingOverride(
			json.RawMessage(record.Pricing),
			json.RawMessage(cfg.PricingOverride),
			"",
		)
		record.Pricing = datatypes.JSON(merged)
	}
	applyResolvedConfiguredToRecord(record)
	return nil
}

func (e *pricingEngine) modelConfigOverrideColumns() string {
	columns := []string{"input_price_override", "output_price_override"}
	for _, column := range []string{"cache_read_price_override", "cache_write_price_override", "cache_write_5m_price_override", "cache_write_1h_price_override"} {
		if e.hasColumn("llm_model_configs", column) {
			columns = append(columns, column)
		} else {
			columns = append(columns, "NULL AS "+column)
		}
	}
	if e.hasColumn("llm_model_configs", "pricing_override") {
		columns = append(columns, "pricing_override")
	} else {
		columns = append(columns, "NULL AS pricing_override")
	}
	return strings.Join(columns, ", ")
}

func normalizePricingModelRef(ref PricingModelRef) PricingModelRef {
	if ref.Source == "" {
		ref.Source = PricingModelSourceGlobal
	}
	if ref.Operation == "" {
		ref.Operation = PricingOperationChat
	}
	ref.Provider = strings.TrimSpace(ref.Provider)
	ref.Model = strings.TrimSpace(ref.Model)
	return ref
}

func (e *pricingEngine) loadModelFromTable(ctx context.Context, table string, modelID uuid.UUID) (*pricingModelRecord, bool, error) {
	selects := []string{"id", "provider", "name", "input_price", "output_price"}
	if e.hasColumn(table, "image_prices") {
		selects = append(selects, "image_prices")
	} else {
		selects = append(selects, "'[]' AS image_prices")
	}
	for _, column := range []string{"cost_cache_read", "cost_cache_write", "cost_cache_write_5m", "cost_cache_write_1h"} {
		if e.hasColumn(table, column) {
			selects = append(selects, column)
		} else {
			selects = append(selects, "0 AS "+column)
		}
	}
	if e.hasColumn(table, "pricing") {
		selects = append(selects, "pricing")
	} else {
		selects = append(selects, "'{}' AS pricing")
	}

	var model pricingModelRecord
	err := e.db.WithContext(ctx).
		Table(table).
		Select(strings.Join(selects, ", ")).
		Where("id = ?", modelID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || isMissingPricingTableError(err, table) {
			return nil, false, nil
		}
		return nil, false, err
	}
	applyResolvedConfiguredToRecord(&model)
	return &model, true, nil
}

func applyResolvedConfiguredToRecord(record *pricingModelRecord) {
	if record == nil {
		return
	}
	overrides := modelpricing.OrganizationOverrides{
		Input:        record.InputPriceOrganizationOverride,
		Output:       record.OutputPriceOrganizationOverride,
		CacheRead:    record.CacheReadOrganizationOverride,
		CacheWrite:   record.CacheWriteOrganizationOverride,
		CacheWrite5m: record.CacheWrite5mOrganizationOverride,
		CacheWrite1h: record.CacheWrite1hOrganizationOverride,
	}
	resolved := modelpricing.ApplyOrganizationOverrides(modelpricing.ResolveConfigured([]byte(record.Pricing)), overrides)
	record.CacheBillingSupported = modelpricing.CacheBillingSupported([]byte(record.Pricing))
	record.InputPriceConfigured = resolved.Input
	record.OutputPriceConfigured = resolved.Output
	record.CacheReadPriceConfigured = resolved.CacheRead
	record.CacheWritePriceConfigured = resolved.CacheWrite
	record.CacheWrite5mPriceConfigured = resolved.CacheWrite5m
	record.CacheWrite1hPriceConfigured = resolved.CacheWrite1h
}

func (e *pricingEngine) hasColumn(table, column string) bool {
	if e == nil || e.db == nil {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(table)) + "." + strings.ToLower(strings.TrimSpace(column))
	if cached, ok := e.columnCache.Load(key); ok {
		value, _ := cached.(bool)
		return value
	}
	exists := e.db.Migrator().HasColumn(table, column)
	e.columnCache.Store(key, exists)
	return exists
}

func isMissingPricingTableError(err error, table string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	table = strings.ToLower(table)
	return strings.Contains(message, "no such table: "+table) ||
		strings.Contains(message, `relation "`+table+`" does not exist`) ||
		(strings.Contains(message, "table") && strings.Contains(message, table) && strings.Contains(message, "doesn't exist"))
}

func newUSDQuote(inputUSD, outputUSD decimal.Decimal, source PricingSource, ruleID string, usageSource UsageSource, snapshot datatypes.JSON) PricingQuote {
	totalUSD := inputUSD.Add(outputUSD)
	totalCredits := creditsFromUSD(totalUSD)
	inputCredits, outputCredits := splitCreditsByUSD(inputUSD, outputUSD, totalCredits)
	snapshot = enrichPricingSnapshotTotal(snapshot, totalUSD, totalCredits)

	return PricingQuote{
		InputUSD:        inputUSD,
		OutputUSD:       outputUSD,
		TotalUSD:        totalUSD,
		InputCredits:    inputCredits,
		OutputCredits:   outputCredits,
		TotalCredits:    totalCredits,
		PricingSource:   source,
		UsageSource:     usageSource,
		RuleID:          ruleID,
		PricingSnapshot: snapshot,
	}
}

func enrichPricingSnapshotTotal(snapshot datatypes.JSON, totalUSD decimal.Decimal, chargedCredits int64) datatypes.JSON {
	values := map[string]interface{}{}
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &values)
	}
	values["total_cost_usd"] = totalUSD.String()
	// Credit amounts in the snapshot are milli-points, the same unit as the
	// column, the wire and the in-process type.
	values["charged_credits"] = chargedCredits
	values["points_per_usd"] = NormalizedPointsPerUSD()
	return buildPricingSnapshot(values)
}

func withTokenPricingBasis(
	quote PricingQuote,
	inputPrice decimal.Decimal,
	outputPrice decimal.Decimal,
	inputResolved bool,
	outputResolved bool,
	inputRuleID string,
	outputRuleID string,
) PricingQuote {
	quote.InputTokenPriceUSDPer1M = inputPrice
	quote.OutputTokenPriceUSDPer1M = outputPrice
	quote.InputTokenPriceResolved = inputResolved
	quote.OutputTokenPriceResolved = outputResolved
	quote.InputRuleID = inputRuleID
	quote.OutputRuleID = outputRuleID
	return quote
}

func (e *pricingEngine) enrichTokenQuoteWithCachePricingBasis(
	ctx context.Context,
	ref PricingModelRef,
	quote PricingQuote,
) (PricingQuote, error) {
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}
	if !found || model == nil {
		return quote, nil
	}
	if model.CacheReadPriceConfigured {
		quote.CacheReadPriceUSDPer1M = model.CacheReadPrice
		quote.CacheReadPriceResolved = true
	}
	if model.CacheWritePriceConfigured {
		quote.CacheWritePriceUSDPer1M = model.CacheWritePrice
		quote.CacheWritePriceResolved = true
	}
	if cacheWrite5mPrice, resolved, _ := resolveCacheWrite5mPrice(model); resolved {
		quote.CacheWrite5mPriceUSDPer1M = cacheWrite5mPrice
		quote.CacheWrite5mPriceResolved = true
	}
	if cacheWrite1hPrice, resolved, _ := resolveCacheWrite1hPrice(model); resolved {
		quote.CacheWrite1hPriceUSDPer1M = cacheWrite1hPrice
		quote.CacheWrite1hPriceResolved = true
	}
	return quote, nil
}

func lockedTokenQuoteSupportsUsage(quote PricingQuote, usage TokenUsage) bool {
	usage = normalizeGatewayTokenUsage(usage)
	if usage.InputTokens > 0 && !quote.InputTokenPriceResolved {
		return false
	}
	if usage.OutputTokens > 0 && !quote.OutputTokenPriceResolved {
		return false
	}
	if usage.CacheReadTokens > 0 && !quote.CacheReadPriceResolved {
		return false
	}
	cacheWriteLegacy := legacyCacheWriteTokens(usage)
	if cacheWriteLegacy > 0 && !quote.CacheWritePriceResolved {
		return false
	}
	if usage.CacheWrite5mTokens > 0 && !quote.CacheWrite5mPriceResolved {
		return false
	}
	if usage.CacheWrite1hTokens > 0 && !quote.CacheWrite1hPriceResolved {
		return false
	}
	return true
}

func repriceLockedTokenUsage(quote PricingQuote, usage TokenUsage) (PricingQuote, error) {
	usage = normalizeGatewayTokenUsage(usage)
	if usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 && usage.CacheWrite5mTokens == 0 && usage.CacheWrite1hTokens == 0 {
		return repriceLockedTokenQuote(quote, usage.InputTokens, usage.OutputTokens)
	}
	if !lockedTokenQuoteSupportsUsage(quote, usage) {
		return PricingQuote{}, fmt.Errorf("locked token pricing does not cover usage components")
	}

	uncachedInputUSD := tokenUSD(quote.InputTokenPriceUSDPer1M, usage.InputTokens)
	cacheReadUSD := tokenUSD(quote.CacheReadPriceUSDPer1M, usage.CacheReadTokens)
	cacheWriteLegacy := legacyCacheWriteTokens(usage)
	cacheWriteLegacyUSD := tokenUSD(quote.CacheWritePriceUSDPer1M, cacheWriteLegacy)
	cacheWrite5mUSD := tokenUSD(quote.CacheWrite5mPriceUSDPer1M, usage.CacheWrite5mTokens)
	cacheWrite1hUSD := tokenUSD(quote.CacheWrite1hPriceUSDPer1M, usage.CacheWrite1hTokens)
	cacheWriteUSD := cacheWriteLegacyUSD.Add(cacheWrite5mUSD).Add(cacheWrite1hUSD)
	inputUSD := uncachedInputUSD.Add(cacheReadUSD).Add(cacheWriteUSD)
	outputUSD := tokenUSD(quote.OutputTokenPriceUSDPer1M, usage.OutputTokens)

	usageSource := quote.UsageSource
	if usageSource == "" {
		usageSource = UsageSourceProviderUsage
	}
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                         quote.PricingSource,
		"usage_source":                           usageSource,
		"prompt_tokens":                          usage.InputTokens,
		"cache_read_tokens":                      usage.CacheReadTokens,
		"cache_write_tokens":                     usage.CacheWriteTokens,
		"cache_write_legacy_tokens":              cacheWriteLegacy,
		"cache_write_5m_tokens":                  usage.CacheWrite5mTokens,
		"cache_write_1h_tokens":                  usage.CacheWrite1hTokens,
		"completion_tokens":                      usage.OutputTokens,
		"input_price_usd_per_1m_tokens":          quote.InputTokenPriceUSDPer1M.String(),
		"cache_read_price_usd_per_1m_tokens":     quote.CacheReadPriceUSDPer1M.String(),
		"cache_write_price_usd_per_1m_tokens":    quote.CacheWritePriceUSDPer1M.String(),
		"cache_write_5m_price_usd_per_1m_tokens": quote.CacheWrite5mPriceUSDPer1M.String(),
		"cache_write_1h_price_usd_per_1m_tokens": quote.CacheWrite1hPriceUSDPer1M.String(),
		"output_price_usd_per_1m_tokens":         quote.OutputTokenPriceUSDPer1M.String(),
		"input_cost_usd":                         uncachedInputUSD.String(),
		"cache_read_cost_usd":                    cacheReadUSD.String(),
		"cache_write_cost_usd":                   cacheWriteUSD.String(),
		"output_cost_usd":                        outputUSD.String(),
		"locked_pricing":                         true,
	})
	repriced := newUSDQuote(inputUSD, outputUSD, quote.PricingSource, quote.RuleID, usageSource, snapshot)
	repriced.CacheReadUSD = cacheReadUSD
	repriced.CacheWriteUSD = cacheWriteUSD
	repriced.InputTokenPriceUSDPer1M = quote.InputTokenPriceUSDPer1M
	repriced.OutputTokenPriceUSDPer1M = quote.OutputTokenPriceUSDPer1M
	repriced.CacheReadPriceUSDPer1M = quote.CacheReadPriceUSDPer1M
	repriced.CacheWritePriceUSDPer1M = quote.CacheWritePriceUSDPer1M
	repriced.CacheWrite5mPriceUSDPer1M = quote.CacheWrite5mPriceUSDPer1M
	repriced.CacheWrite1hPriceUSDPer1M = quote.CacheWrite1hPriceUSDPer1M
	repriced.InputTokenPriceResolved = quote.InputTokenPriceResolved
	repriced.OutputTokenPriceResolved = quote.OutputTokenPriceResolved
	repriced.CacheReadPriceResolved = quote.CacheReadPriceResolved
	repriced.CacheWritePriceResolved = quote.CacheWritePriceResolved
	repriced.CacheWrite5mPriceResolved = quote.CacheWrite5mPriceResolved
	repriced.CacheWrite1hPriceResolved = quote.CacheWrite1hPriceResolved
	repriced.CacheBillingSupported = quote.CacheBillingSupported
	repriced.InputRuleID = quote.InputRuleID
	repriced.OutputRuleID = quote.OutputRuleID
	return repriced, nil
}

func repriceLockedTokenQuote(quote PricingQuote, promptTokens, completionTokens int) (PricingQuote, error) {
	if promptTokens < 0 || completionTokens < 0 {
		return PricingQuote{}, fmt.Errorf("token count must be greater than or equal to zero")
	}
	if promptTokens > 0 && !quote.InputTokenPriceResolved {
		return PricingQuote{}, fmt.Errorf("locked token pricing is missing input price")
	}
	if completionTokens > 0 && !quote.OutputTokenPriceResolved {
		return PricingQuote{}, fmt.Errorf("locked token pricing is missing output price")
	}

	usageSource := quote.UsageSource
	if usageSource == "" {
		usageSource = UsageSourceProviderUsage
	}
	inputUSD := tokenUSD(quote.InputTokenPriceUSDPer1M, promptTokens)
	outputUSD := tokenUSD(quote.OutputTokenPriceUSDPer1M, completionTokens)
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                 quote.PricingSource,
		"usage_source":                   usageSource,
		"prompt_tokens":                  promptTokens,
		"completion_tokens":              completionTokens,
		"input_price_usd_per_1m_tokens":  quote.InputTokenPriceUSDPer1M.String(),
		"output_price_usd_per_1m_tokens": quote.OutputTokenPriceUSDPer1M.String(),
		"input_cost_usd":                 inputUSD.String(),
		"output_cost_usd":                outputUSD.String(),
		"input_price_resolved":           quote.InputTokenPriceResolved,
		"output_price_resolved":          quote.OutputTokenPriceResolved,
		"input_rule_id":                  quote.InputRuleID,
		"output_rule_id":                 quote.OutputRuleID,
		"rule_id":                        quote.RuleID,
		"locked_pricing":                 true,
	})
	repriced := newUSDQuote(
		inputUSD,
		outputUSD,
		quote.PricingSource,
		quote.RuleID,
		usageSource,
		snapshot,
	)
	repriced = withTokenPricingBasis(
		repriced,
		quote.InputTokenPriceUSDPer1M,
		quote.OutputTokenPriceUSDPer1M,
		quote.InputTokenPriceResolved,
		quote.OutputTokenPriceResolved,
		quote.InputRuleID,
		quote.OutputRuleID,
	)
	repriced.CacheBillingSupported = quote.CacheBillingSupported
	return repriced, nil
}

func creditsFromUSD(totalUSD decimal.Decimal) int64 {
	if !totalUSD.IsPositive() {
		return 0
	}
	return millipoints.FromUSD(totalUSD, NormalizedPointsPerUSD()).Int64()
}

func matchImagePricingRule(req *adapter.ImageRequest, prices []llmmodel.PricingRule) (*llmmodel.PricingRule, error) {
	bestScore := -1
	var best *llmmodel.PricingRule
	ambiguous := false
	for index := range prices {
		rule := prices[index]
		score, ok := scoreImagePricingRule(rule.Conditions, req)
		if !ok {
			continue
		}
		if score > bestScore {
			best = &prices[index]
			bestScore = score
			ambiguous = false
			continue
		}
		if score == bestScore && best != nil && rule.ID != best.ID {
			ambiguous = true
		}
	}
	if ambiguous {
		return nil, fmt.Errorf("%w: ambiguous image pricing rules", ErrPricingNotConfigured)
	}
	return best, nil
}

func scoreImagePricingRule(conditions map[string]interface{}, req *adapter.ImageRequest) (int, bool) {
	if len(conditions) == 0 {
		return 0, true
	}
	score := 0
	for key, expected := range conditions {
		switch key {
		case "size":
			if !matchStringOrArray(req.Size, expected) {
				return 0, false
			}
		case "quality":
			if !matchStringOrArray(req.Quality, expected) {
				return 0, false
			}
		case "style":
			if !matchStringOrArray(req.Style, expected) {
				return 0, false
			}
		default:
			return 0, false
		}
		score++
	}
	return score, true
}

func newOutputOnlyUSDQuote(totalUSD decimal.Decimal, source PricingSource, ruleID string, usageSource UsageSource, snapshot datatypes.JSON) PricingQuote {
	return newUSDQuote(decimal.Zero, totalUSD, source, ruleID, usageSource, snapshot)
}

func tokenUSD(price decimal.Decimal, tokens int) decimal.Decimal {
	if tokens <= 0 {
		return decimal.Zero
	}
	return price.Mul(decimal.NewFromInt(int64(tokens))).Div(pricingPerMillionDivisor)
}

func findTokenFallbackRule(rules []PricingFallbackRule, operation PricingOperation, meter PricingMeter, provider, modelName string) (PricingFallbackRule, error) {
	var best PricingFallbackRule
	var bestScore pricingFallbackRuleScore
	found := false
	for _, rule := range rules {
		if !pricingFallbackRuleEnabled(rule) {
			continue
		}
		if rule.Operation != operation || rule.Meter != meter {
			continue
		}
		score, ok := scoreTokenFallbackRule(rule, provider, modelName)
		if !ok {
			continue
		}
		if !found || score.betterThan(bestScore) {
			best = rule
			bestScore = score
			found = true
		}
	}
	if found {
		return best, nil
	}
	return PricingFallbackRule{}, fmt.Errorf("%w: missing token fallback pricing rule for operation %q meter %q", ErrPricingNotConfigured, operation, meter)
}

func scoreTokenFallbackRule(rule PricingFallbackRule, provider, modelName string) (pricingFallbackRuleScore, bool) {
	providerScore, ok := pricingPatternMatchScore(rule.Provider, provider)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	modelScore, ok := pricingPatternMatchScore(rule.Model, modelName)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	return pricingFallbackRuleScore{
		provider: providerScore,
		model:    modelScore,
		source:   pricingFallbackSourceScore(rule),
	}, true
}

func findImageFallbackRule(rules []PricingFallbackRule, provider, modelName string, req *adapter.ImageRequest) (PricingFallbackRule, error) {
	var best PricingFallbackRule
	var bestScore pricingFallbackRuleScore
	found := false
	for _, rule := range rules {
		if !pricingFallbackRuleEnabled(rule) {
			continue
		}
		if rule.Operation != PricingOperationImage || rule.Meter != PricingMeterImage {
			continue
		}
		score, ok := scoreImageFallbackRule(rule, provider, modelName, req)
		if !ok {
			continue
		}
		if !found || score.betterThan(bestScore) {
			best = rule
			bestScore = score
			found = true
		}
	}
	if found {
		return best, nil
	}
	return PricingFallbackRule{}, fmt.Errorf("%w: missing image fallback pricing rule for provider %q model %q", ErrPricingNotConfigured, provider, modelName)
}

type pricingFallbackRuleScore struct {
	provider int
	model    int
	size     int
	quality  int
	style    int
	source   int
}

func (s pricingFallbackRuleScore) betterThan(other pricingFallbackRuleScore) bool {
	if s.provider != other.provider {
		return s.provider > other.provider
	}
	if s.model != other.model {
		return s.model > other.model
	}
	if s.size != other.size {
		return s.size > other.size
	}
	if s.quality != other.quality {
		return s.quality > other.quality
	}
	if s.style != other.style {
		return s.style > other.style
	}
	return s.source > other.source
}

func scoreImageFallbackRule(rule PricingFallbackRule, provider, modelName string, req *adapter.ImageRequest) (pricingFallbackRuleScore, bool) {
	providerScore, ok := pricingPatternMatchScore(rule.Provider, provider)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	modelScore, ok := pricingPatternMatchScore(rule.Model, modelName)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	sizeScore, ok := pricingPatternMatchScore(rule.Size, req.Size)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	qualityScore, ok := pricingPatternMatchScore(rule.Quality, req.Quality)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	styleScore, ok := pricingPatternMatchScore(rule.Style, req.Style)
	if !ok {
		return pricingFallbackRuleScore{}, false
	}
	return pricingFallbackRuleScore{
		provider: providerScore,
		model:    modelScore,
		size:     sizeScore,
		quality:  qualityScore,
		style:    styleScore,
		source:   pricingFallbackSourceScore(rule),
	}, true
}

func pricingFallbackSourceScore(rule PricingFallbackRule) int {
	if rule.PricingSource == PricingSourceAdminFallback {
		return 1
	}
	return 0
}

func matchPricingPattern(pattern, value string) bool {
	_, ok := pricingPatternMatchScore(pattern, value)
	return ok
}

func pricingPatternMatchScore(pattern, value string) (int, bool) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" || pattern == "*" {
		return 0, true
	}
	if strings.HasSuffix(pattern, "*") {
		if strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
			return 1, true
		}
		return 0, false
	}
	if pattern == value {
		return 2, true
	}
	return 0, false
}

func fallbackPricingSource(inputRule, outputRule PricingFallbackRule) PricingSource {
	if inputRule.PricingSource == PricingSourceAdminFallback || outputRule.PricingSource == PricingSourceAdminFallback {
		return PricingSourceAdminFallback
	}
	return PricingSourceCodeDefaultFallback
}

func pricingModelIdentity(ref PricingModelRef, model *pricingModelRecord, found bool) (string, string) {
	provider := strings.TrimSpace(ref.Provider)
	modelName := strings.TrimSpace(ref.Model)
	if found && model != nil {
		if strings.TrimSpace(model.Provider) != "" {
			provider = strings.TrimSpace(model.Provider)
		}
		if strings.TrimSpace(model.Name) != "" {
			modelName = strings.TrimSpace(model.Name)
		}
	}
	return provider, modelName
}

func pricingModelID(model *pricingModelRecord) string {
	if model == nil || model.ID == uuid.Nil {
		return ""
	}
	return model.ID.String()
}

func inferImagePricingProvider(req *adapter.ImageRequest) string {
	if req == nil {
		return "*"
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	switch {
	case strings.Contains(model, "qwen"):
		return "qwen"
	case strings.Contains(model, "doubao"):
		return "doubao"
	case strings.Contains(model, "midjourney"):
		return "midjourney"
	case strings.Contains(model, "gemini"), strings.Contains(model, "imagen"), strings.Contains(model, "google"):
		return "gcp"
	case strings.Contains(model, "gpt"), strings.Contains(model, "dall-e"), strings.Contains(model, "openai"):
		return "openai"
	default:
		return "*"
	}
}

func canonicalImagePricingProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "dashscope", "alibaba", "aliyun":
		return "qwen"
	case "volcengine":
		return "doubao"
	case "gcp-imagen":
		return "gcp"
	case "mj-proxy":
		return "midjourney"
	default:
		return strings.TrimSpace(provider)
	}
}

func buildPricingSnapshot(payload interface{}) datatypes.JSON {
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(raw)
}

func splitCreditsByUSD(inputUSD, outputUSD decimal.Decimal, totalCredits int64) (int64, int64) {
	if totalCredits <= 0 {
		return 0, 0
	}
	if !inputUSD.IsPositive() {
		return 0, totalCredits
	}
	if !outputUSD.IsPositive() {
		return totalCredits, 0
	}

	multiplier := decimal.NewFromInt(localMilliPointsPerUSD())
	rawInputCredits := inputUSD.Mul(multiplier)
	rawOutputCredits := outputUSD.Mul(multiplier)

	inputFloor := rawInputCredits.Floor().IntPart()
	outputFloor := rawOutputCredits.Floor().IntPart()
	remaining := totalCredits - inputFloor - outputFloor
	if remaining <= 0 {
		return inputFloor, outputFloor
	}

	inputFraction := rawInputCredits.Sub(decimal.NewFromInt(inputFloor))
	outputFraction := rawOutputCredits.Sub(decimal.NewFromInt(outputFloor))

	inputCredits := inputFloor
	outputCredits := outputFloor
	for remaining > 0 {
		if outputFraction.GreaterThan(inputFraction) || outputFraction.Equal(inputFraction) {
			outputCredits++
			outputFraction = decimal.Zero
		} else {
			inputCredits++
			inputFraction = decimal.Zero
		}
		remaining--
	}

	return inputCredits, outputCredits
}
