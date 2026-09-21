package ratingcore

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Rule is one canonical-USD price rule inside a pricing JSON document.
type Rule struct {
	ID         string          `json:"id"`
	Operation  string          `json:"operation"`
	Meter      string          `json:"meter"`
	Unit       string          `json:"unit"`
	Price      decimal.Decimal `json:"price"`
	Configured *bool           `json:"configured,omitempty"`
}

// ImageTokenPricing is the three-tier image-by-token price book. All three
// tiers are canonical USD per million tokens.
type ImageTokenPricing struct {
	Currency                  string          `json:"currency"`
	TextInputPricePerMillion  decimal.Decimal `json:"text_input_price_per_million"`
	ImageInputPricePerMillion decimal.Decimal `json:"image_input_price_per_million"`
	OutputPricePerMillion     decimal.Decimal `json:"output_price_per_million"`
}

// VideoRate is one rate row. Exactly one price field applies to the selected
// base unit; the others stay nil.
type VideoRate struct {
	ID                    string           `json:"id"`
	ModelTier             string           `json:"model_tier"`
	Resolution            string           `json:"resolution"`
	PricePerSecond        *decimal.Decimal `json:"price_per_second"`
	PricePerTask          *decimal.Decimal `json:"price_per_task"`
	PricePerMillionTokens *decimal.Decimal `json:"price_per_million_tokens"`
}

// VideoGenerationPricing groups the video rate rows.
type VideoGenerationPricing struct {
	Rates []VideoRate `json:"rates"`
}

// MeteredPrice is the amount/currency/per_quantity triple of a metered rule.
type MeteredPrice struct {
	Amount      decimal.Decimal `json:"amount"`
	Currency    string          `json:"currency"`
	PerQuantity int64           `json:"per_quantity"`
}

// MeteredRule is one transcription / speech / music / lyrics price entry.
type MeteredRule struct {
	Operation string       `json:"operation"`
	Meter     string       `json:"meter"`
	BaseUnit  string       `json:"base_unit"`
	Price     MeteredPrice `json:"price"`
}

// PriceSource is the author's original amount/currency/rate, retained for
// audit only. It never participates in rating.
type PriceSource struct {
	Amount   decimal.Decimal `json:"amount"`
	Currency string          `json:"currency"`
	Rate     string          `json:"rate"`
	RateAt   string          `json:"rate_at"`
}

// Pricing is the canonical-USD price book. Rules are the primary shape;
// image_token_pricing / video_generation / metered are the typed projections
// used by the corresponding operations.
type Pricing struct {
	Currency            string                  `json:"currency"`
	Rules               []Rule                  `json:"rules"`
	PricePerImage       *decimal.Decimal        `json:"price_per_image"`
	ImageTokenPricing   *ImageTokenPricing      `json:"image_token_pricing"`
	VideoGeneration     *VideoGenerationPricing `json:"video_generation"`
	Metered             []MeteredRule           `json:"metered"`
	Source              *PriceSource            `json:"source"`
	PriceExchangeRate   string                  `json:"price_exchange_rate,omitempty"`
	PriceCurrencyStatus string                  `json:"price_currency_status,omitempty"`
	PricingVersion      string                  `json:"pricing_version,omitempty"`
}

// Dimensions selects the applicable video rate.
type Dimensions struct {
	Resolution string `json:"resolution"`
	ModelTier  string `json:"model_tier"`
}

// Usage is one request's metered quantities.
type Usage struct {
	Operation string `json:"operation"`
	Meter     string `json:"meter"`
	BaseUnit  string `json:"base_unit"`

	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	CacheWrite5m     int64 `json:"cache_write_5m_token"`
	CacheWrite1h     int64 `json:"cache_write_1h_token"`

	AudioInputTokens  int64 `json:"audio_input_tokens"`
	AudioOutputTokens int64 `json:"audio_output_tokens"`

	TextInputTokens  int64 `json:"text_input_tokens"`
	InputImageTokens int64 `json:"input_image_tokens"`

	Quantity   int64      `json:"quantity"`
	Dimensions Dimensions `json:"dimensions"`
}

// Snapshot is the required {rate, rate_source, rate_at, display_currency}
// record written beside every pre-deduct, settlement, and usage bill.
type Snapshot struct {
	Rate            string `json:"rate"`
	RateSource      string `json:"rate_source"`
	RateAt          string `json:"rate_at"`
	DisplayCurrency string `json:"display_currency"`
}

// Request is one rating input: a price book, a usage fact, and the exchange
// rate snapshot context.
type Request struct {
	Pricing Pricing `json:"pricing"`
	Usage   Usage   `json:"usage"`

	// Rate is the current/recorded USD-to-display-currency rate. It is
	// required and must be positive; a missing rate is fail-closed.
	Rate *decimal.Decimal `json:"rate"`
	// RateSource is one of console_system | channel_rate | pre_org |
	// payment_gateway | wallet.
	RateSource string `json:"rate_source"`
	// RateAt is when the rate was observed.
	RateAt string `json:"rate_at"`
	// DisplayCurrency defaults to USD when empty.
	DisplayCurrency string `json:"display_currency"`
}

// Result is the rated amount. USD is the canonical decimal value; PointsMP is
// the single integer accounting assertion.
type Result struct {
	USD         decimal.Decimal
	PointsMP    int64
	TotalPoints decimal.Decimal
	Snapshot    Snapshot
}

// Quote rates one request: resolve the canonical USD amount, round up to
// milli-points exactly once, and attach the required snapshot.
func Quote(request Request) (Result, error) {
	if request.Rate == nil || !request.Rate.IsPositive() {
		return Result{}, exchangeRateError("a positive exchange rate is required")
	}
	usd, err := Resolve(request.Pricing, request.Usage)
	if err != nil {
		return Result{}, err
	}
	if usd.IsNegative() {
		return Result{}, invalidRequestError("rated amount cannot be negative")
	}
	scaled := usd.Mul(decimalMillion).Ceil()
	if !scaled.LessThanOrEqual(decimalMaxInt64) {
		return Result{}, invalidRequestError("amount overflows milli-points")
	}
	pointsMP := scaled.IntPart()
	return Result{
		USD:         usd,
		PointsMP:    pointsMP,
		TotalPoints: MilliPointsToPoints(pointsMP),
		Snapshot: Snapshot{
			Rate:            request.Rate.String(),
			RateSource:      strings.TrimSpace(request.RateSource),
			RateAt:          strings.TrimSpace(request.RateAt),
			DisplayCurrency: DisplayCurrency(request.DisplayCurrency),
		},
	}, nil
}

// Resolve returns the canonical USD amount for a price book and usage fact,
// without applying milli-point rounding. It is the take-price engine: unknown
// operation/meter/unit combinations fail closed.
func Resolve(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	operation := strings.TrimSpace(usage.Operation)
	if operation == "" {
		return decimal.Zero, invalidRequestError("operation is required")
	}
	if hasNegativeUsage(usage) {
		return decimal.Zero, invalidRequestError("usage cannot be negative")
	}

	switch operation {
	case "chat", "embedding", "rerank", "realtime_audio":
		return resolveTokenMeters(pricing, usage)
	case "image_generation":
		if isImageTokenUsage(usage) {
			return resolveImageTokenPricing(pricing, usage)
		}
		return resolvePerImage(pricing, usage)
	case "video_generation":
		return resolveVideo(pricing, usage)
	case "transcription", "speech_generation", "music_generation", "lyrics_generation":
		return resolveMetered(pricing, usage)
	default:
		return decimal.Zero, pricingError("unsupported operation " + operation)
	}
}

// DisplayCurrency normalizes a presentment currency, defaulting to USD.
func DisplayCurrency(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return "USD"
	}
	return normalized
}

// ConvertToUSD converts an author-entered amount to canonical USD exactly once.
// USD passes through; CNY requires a positive USD-to-CNY rate; any other
// currency is fail-closed rather than silently treated as USD.
func ConvertToUSD(amount decimal.Decimal, currency string, usdToCurrencyRate *decimal.Decimal) (decimal.Decimal, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "USD":
		return amount, nil
	case "CNY":
		if usdToCurrencyRate == nil || !usdToCurrencyRate.IsPositive() {
			return decimal.Zero, exchangeRateError("a positive USD to CNY rate is required")
		}
		return amount.Div(*usdToCurrencyRate), nil
	default:
		return decimal.Zero, exchangeRateError("unsupported pricing currency")
	}
}

// ConvertFromUSD converts canonical USD back to a presentment currency. It is
// the exact inverse of ConvertToUSD and is the only place a display amount is
// derived; display conversion never writes back to storage.
func ConvertFromUSD(usd decimal.Decimal, currency string, usdToCurrencyRate *decimal.Decimal) (decimal.Decimal, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "USD":
		return usd, nil
	case "CNY":
		if usdToCurrencyRate == nil || !usdToCurrencyRate.IsPositive() {
			return decimal.Zero, exchangeRateError("a positive USD to CNY rate is required")
		}
		return usd.Mul(*usdToCurrencyRate), nil
	default:
		return decimal.Zero, exchangeRateError("unsupported display currency")
	}
}

func hasNegativeUsage(usage Usage) bool {
	return usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 ||
		usage.CacheWriteTokens < 0 || usage.CacheWrite5m < 0 || usage.CacheWrite1h < 0 ||
		usage.AudioInputTokens < 0 || usage.AudioOutputTokens < 0 ||
		usage.TextInputTokens < 0 || usage.InputImageTokens < 0 || usage.Quantity < 0
}

// resolveTokenMeters sums every active token meter. realtime_audio prefers
// audio_tokens and falls back to prompt/completion tokens on the same rules.
func resolveTokenMeters(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	if usage.Operation == "realtime_audio" {
		if usage.AudioInputTokens > 0 {
			inputTokens = usage.AudioInputTokens
		}
		if usage.AudioOutputTokens > 0 {
			outputTokens = usage.AudioOutputTokens
		}
	}

	type meterQuantity struct {
		meter string
		value int64
	}
	candidates := []meterQuantity{
		{meter: "input_token", value: inputTokens},
		{meter: "output_token", value: outputTokens},
		{meter: "cache_read_token", value: usage.CacheReadTokens},
		{meter: "cache_write_token", value: usage.CacheWriteTokens},
		{meter: "cache_write_5m_token", value: usage.CacheWrite5m},
		{meter: "cache_write_1h_token", value: usage.CacheWrite1h},
	}

	total := decimal.Zero
	active := 0
	for _, candidate := range candidates {
		if candidate.value == 0 {
			continue
		}
		if usage.Meter != "" && usage.Meter != candidate.meter {
			continue
		}
		if !knownMeter(usage.Operation, candidate.meter, "per_1m_tokens") {
			return decimal.Zero, invalidRequestError("unknown meter " + candidate.meter)
		}
		price, ok := findRulePrice(pricing.Rules, usage.Operation, candidate.meter, "per_1m_tokens")
		if !ok {
			return decimal.Zero, pricingError("pricing is not configured for " + usage.Operation + "/" + candidate.meter)
		}
		total = total.Add(price.Mul(decimal.NewFromInt(candidate.value)).Div(decimalMillion))
		active++
	}
	if active == 0 {
		// Explicit single-meter usage must still be a registered combination.
		if usage.Meter != "" && !knownMeter(usage.Operation, usage.Meter, "per_1m_tokens") {
			return decimal.Zero, invalidRequestError("unknown meter " + usage.Meter)
		}
		return decimal.Zero, nil
	}
	return total, nil
}

func isImageTokenUsage(usage Usage) bool {
	switch usage.Meter {
	case "input_text_token", "input_image_token":
		return true
	case "output_token":
		return usage.Operation == "image_generation" && usage.BaseUnit == "per_1m_tokens"
	}
	if usage.BaseUnit == "per_1m_tokens" {
		return true
	}
	return usage.TextInputTokens != 0 || usage.InputImageTokens != 0
}

// resolveImageTokenPricing rates the three tiers. A request that carries image
// token usage must never fall back to per-image pricing.
func resolveImageTokenPricing(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	if pricing.ImageTokenPricing == nil {
		return decimal.Zero, pricingError("image token pricing is not configured")
	}
	tiers := pricing.ImageTokenPricing
	if tiers.TextInputPricePerMillion.IsNegative() || tiers.ImageInputPricePerMillion.IsNegative() || tiers.OutputPricePerMillion.IsNegative() {
		return decimal.Zero, pricingError("image token pricing is invalid")
	}

	textInput := usage.TextInputTokens
	imageInput := usage.InputImageTokens
	output := usage.OutputTokens
	// A single-meter fact may carry its quantity in the generic field.
	if usage.Quantity > 0 {
		switch usage.Meter {
		case "input_text_token":
			textInput = usage.Quantity
		case "input_image_token":
			imageInput = usage.Quantity
		case "output_token":
			output = usage.Quantity
		}
	}

	total := tiers.TextInputPricePerMillion.Mul(decimal.NewFromInt(textInput)).
		Add(tiers.ImageInputPricePerMillion.Mul(decimal.NewFromInt(imageInput))).
		Add(tiers.OutputPricePerMillion.Mul(decimal.NewFromInt(output)))
	return total.Div(decimalMillion), nil
}

func resolvePerImage(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	price, ok := findRulePrice(pricing.Rules, "image_generation", "image", "per_image")
	if !ok {
		if pricing.PricePerImage != nil && !pricing.PricePerImage.IsNegative() {
			price, ok = *pricing.PricePerImage, true
		}
	}
	if !ok {
		return decimal.Zero, pricingError("image per-image pricing is not configured")
	}
	quantity := usage.Quantity
	if quantity == 0 {
		quantity = 1
	}
	return price.Mul(decimal.NewFromInt(quantity)), nil
}

func resolveVideo(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	if pricing.VideoGeneration == nil {
		return decimal.Zero, pricingError("video pricing is not configured")
	}
	rate, ok := selectVideoRate(pricing.VideoGeneration.Rates, usage.Dimensions)
	if !ok {
		return decimal.Zero, pricingError("video pricing rate is not configured")
	}
	quantity := usage.Quantity
	switch usage.BaseUnit {
	case "second":
		if rate.PricePerSecond == nil {
			return decimal.Zero, pricingError("video second price is not configured")
		}
		return rate.PricePerSecond.Mul(decimal.NewFromInt(quantity)), nil
	case "task":
		if rate.PricePerTask == nil {
			return decimal.Zero, pricingError("video task price is not configured")
		}
		return rate.PricePerTask.Mul(decimal.NewFromInt(quantity)), nil
	case "million_video_tokens":
		if rate.PricePerMillionTokens == nil {
			return decimal.Zero, pricingError("video token price is not configured")
		}
		return rate.PricePerMillionTokens.Mul(decimal.NewFromInt(quantity)).Div(decimalMillion), nil
	default:
		return decimal.Zero, pricingError("unsupported video base unit " + usage.BaseUnit)
	}
}

// selectVideoRate picks the most specific rate that does not conflict with a
// requested dimension. A rate row with an empty dimension is generic; the
// request's empty dimension matches any rate. This keeps a row declared as
// model_tier="720p" selectable by a request that only carries resolution=720p.
func selectVideoRate(rates []VideoRate, dimensions Dimensions) (VideoRate, bool) {
	best := -1
	bestScore := -1
	for index, rate := range rates {
		if rate.Resolution != "" && dimensions.Resolution != "" && rate.Resolution != dimensions.Resolution {
			continue
		}
		if rate.ModelTier != "" && dimensions.ModelTier != "" && rate.ModelTier != dimensions.ModelTier {
			continue
		}
		score := 0
		if rate.Resolution != "" && dimensions.Resolution != "" && rate.Resolution == dimensions.Resolution {
			score++
		}
		if rate.ModelTier != "" && dimensions.ModelTier != "" && rate.ModelTier == dimensions.ModelTier {
			score++
		}
		if score > bestScore {
			best = index
			bestScore = score
		}
	}
	if best < 0 {
		return VideoRate{}, false
	}
	return rates[best], true
}

func resolveMetered(pricing Pricing, usage Usage) (decimal.Decimal, error) {
	if !knownMeter(usage.Operation, usage.Meter, usage.BaseUnit) {
		return decimal.Zero, invalidRequestError("unknown metered combination")
	}
	var matched *MeteredRule
	for index := range pricing.Metered {
		rule := pricing.Metered[index]
		if rule.Operation == usage.Operation && rule.Meter == usage.Meter && rule.BaseUnit == usage.BaseUnit {
			if matched != nil {
				return decimal.Zero, pricingError("metered pricing is ambiguous")
			}
			matched = &pricing.Metered[index]
		}
	}
	if matched == nil {
		return decimal.Zero, pricingError("metered pricing is not configured")
	}
	if matched.Price.PerQuantity <= 0 {
		return decimal.Zero, pricingError("metered per_quantity must be a positive integer")
	}
	return matched.Price.Amount.Mul(decimal.NewFromInt(usage.Quantity)).Div(decimal.NewFromInt(matched.Price.PerQuantity)), nil
}

func findRulePrice(rules []Rule, operation, meter, baseUnit string) (decimal.Decimal, bool) {
	for _, rule := range rules {
		if rule.Operation != operation || rule.Meter != meter || rule.Unit != baseUnit {
			continue
		}
		if rule.Configured != nil && !*rule.Configured {
			continue
		}
		if rule.Price.IsNegative() {
			continue
		}
		return rule.Price, true
	}
	return decimal.Zero, false
}
