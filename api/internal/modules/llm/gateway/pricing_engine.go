package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const pricingCreditsPerUSD = int64(1_000_000)

var pricingPerMillionDivisor = decimal.NewFromInt(1000000)

type PricingModelSource string

const (
	PricingModelSourceGlobal      PricingModelSource = "global"
	PricingModelSourceCustom      PricingModelSource = "custom"
	PricingModelSourcePassthrough PricingModelSource = "passthrough"
)

type PricingModelRef struct {
	ModelID   uuid.UUID
	Source    PricingModelSource
	Operation PricingOperation
}

type PricingQuote struct {
	InputUSD        decimal.Decimal
	OutputUSD       decimal.Decimal
	TotalUSD        decimal.Decimal
	InputCredits    int64
	OutputCredits   int64
	TotalCredits    int64
	PricingSource   PricingSource
	UsageSource     UsageSource
	RuleID          string
	PricingSnapshot datatypes.JSON

	InputTokenPriceUSDPer1M  decimal.Decimal
	OutputTokenPriceUSDPer1M decimal.Decimal
	InputTokenPriceResolved  bool
	OutputTokenPriceResolved bool
	InputRuleID              string
	OutputRuleID             string
}

type PricingEngine interface {
	QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error)
	QuoteImage(ctx context.Context, model PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error)
}

type pricingEngine struct {
	db          *gorm.DB
	columnCache sync.Map
}

type pricingModelRecord struct {
	ID                    uuid.UUID       `gorm:"column:id"`
	Provider              string          `gorm:"column:provider"`
	Name                  string          `gorm:"column:name"`
	InputPrice            decimal.Decimal `gorm:"column:input_price"`
	OutputPrice           decimal.Decimal `gorm:"column:output_price"`
	InputPriceConfigured  bool            `gorm:"column:input_price_configured"`
	OutputPriceConfigured bool            `gorm:"column:output_price_configured"`
	ImagePrices           datatypes.JSON  `gorm:"column:image_prices"`
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
			"input_price_configured":         model.InputPriceConfigured,
			"output_price_configured":        model.OutputPriceConfigured,
		})
		quote := newUSDQuote(inputUSD, outputUSD, PricingSourceUpstreamModelPrice, "", UsageSourceProviderUsage, snapshot)
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
		ModelID:   ref.ModelID,
		Source:    ref.Source,
		Operation: PricingOperationImage,
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
		prices, configured, err := configuredImagePricingRules(model)
		if err != nil {
			return PricingQuote{}, err
		}
		if configured {
			var matched llmmodel.PricingRule
			if matchImagePricingRule(&r, prices, &matched) {
				return quoteConfiguredImageRule(model, &r, matched, count)
			}
		}
	}

	return e.quoteImageWithFallback(ctx, ref, model, found, &r, count)
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
	config, err := LoadPricingFallbackConfig(ctx, e.db)
	if err != nil {
		return PricingQuote{}, err
	}
	if !config.Enabled {
		return PricingQuote{}, fmt.Errorf("missing token pricing and fallback pricing is disabled")
	}

	provider, modelName := pricingModelIdentity(model, found)
	var inputRule PricingFallbackRule
	var outputRule PricingFallbackRule
	var inputPrice decimal.Decimal
	var outputPrice decimal.Decimal
	var ruleIDs []string

	if requiredPrices.input {
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
	if requiredPrices.output {
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

	source := fallbackPricingSource(inputRule, outputRule)
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
		"input_rule_id":                  inputRule.ID,
		"output_rule_id":                 outputRule.ID,
		"input_rule_source":              inputRule.PricingSource,
		"output_rule_source":             outputRule.PricingSource,
	})

	quote := newUSDQuote(inputUSD, outputUSD, source, ruleID, UsageSourceProviderUsage, snapshot)
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

func (e *pricingEngine) quoteImageWithFallback(
	ctx context.Context,
	ref PricingModelRef,
	model *pricingModelRecord,
	found bool,
	req *adapter.ImageRequest,
	count int64,
) (PricingQuote, error) {
	config, err := LoadPricingFallbackConfig(ctx, e.db)
	if err != nil {
		return PricingQuote{}, err
	}
	if !config.Enabled {
		return PricingQuote{}, fmt.Errorf("missing image pricing and fallback pricing is disabled")
	}

	provider, modelName := pricingModelIdentity(model, found)
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
		totalUSD := decimal.NewFromFloat(matched.Price.Amount).Mul(decimal.NewFromInt(count))
		snapshot := configuredImagePricingSnapshot(model, req, matched, count, matched.Price.Amount, 0)
		return newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, matched.ID, UsageSourceRequestParameters, snapshot), nil
	}

	if matched.Price.Credits <= 0 {
		return PricingQuote{}, fmt.Errorf("image pricing rule %q has no price", matched.ID)
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

func configuredImagePricingSnapshot(model *pricingModelRecord, req *adapter.ImageRequest, rule llmmodel.PricingRule, count int64, usdPerImage float64, creditsPerImage int64) datatypes.JSON {
	provider, modelName := pricingModelIdentity(model, model != nil)
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
		return e.loadModelFromTable(ctx, "llm_models", ref.ModelID)
	case PricingModelSourceCustom:
		return e.loadModelFromTable(ctx, "llm_custom_models", ref.ModelID)
	default:
		return nil, false, fmt.Errorf("unknown pricing model source %q", ref.Source)
	}
}

func normalizePricingModelRef(ref PricingModelRef) PricingModelRef {
	if ref.Source == "" {
		ref.Source = PricingModelSourceGlobal
	}
	if ref.Operation == "" {
		ref.Operation = PricingOperationChat
	}
	return ref
}

func (e *pricingEngine) loadModelFromTable(ctx context.Context, table string, modelID uuid.UUID) (*pricingModelRecord, bool, error) {
	selects := []string{"id", "provider", "name", "input_price", "output_price"}
	if e.hasColumn(table, "input_price_configured") {
		selects = append(selects, "input_price_configured")
	} else {
		selects = append(selects, "false AS input_price_configured")
	}
	if e.hasColumn(table, "output_price_configured") {
		selects = append(selects, "output_price_configured")
	} else {
		selects = append(selects, "false AS output_price_configured")
	}
	if e.hasColumn(table, "image_prices") {
		selects = append(selects, "image_prices")
	} else {
		selects = append(selects, "'[]' AS image_prices")
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
	return &model, true, nil
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
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":                 quote.PricingSource,
		"usage_source":                   usageSource,
		"prompt_tokens":                  promptTokens,
		"completion_tokens":              completionTokens,
		"input_price_usd_per_1m_tokens":  quote.InputTokenPriceUSDPer1M.String(),
		"output_price_usd_per_1m_tokens": quote.OutputTokenPriceUSDPer1M.String(),
		"input_price_resolved":           quote.InputTokenPriceResolved,
		"output_price_resolved":          quote.OutputTokenPriceResolved,
		"input_rule_id":                  quote.InputRuleID,
		"output_rule_id":                 quote.OutputRuleID,
		"rule_id":                        quote.RuleID,
		"locked_pricing":                 true,
	})
	repriced := newUSDQuote(
		tokenUSD(quote.InputTokenPriceUSDPer1M, promptTokens),
		tokenUSD(quote.OutputTokenPriceUSDPer1M, completionTokens),
		quote.PricingSource,
		quote.RuleID,
		usageSource,
		snapshot,
	)
	return withTokenPricingBasis(
		repriced,
		quote.InputTokenPriceUSDPer1M,
		quote.OutputTokenPriceUSDPer1M,
		quote.InputTokenPriceResolved,
		quote.OutputTokenPriceResolved,
		quote.InputRuleID,
		quote.OutputRuleID,
	), nil
}

func creditsFromUSD(totalUSD decimal.Decimal) int64 {
	if !totalUSD.IsPositive() {
		return 0
	}
	return totalUSD.Mul(decimal.NewFromInt(pricingCreditsPerUSD)).Ceil().IntPart()
}

func matchImagePricingRule(req *adapter.ImageRequest, prices []llmmodel.PricingRule, matched *llmmodel.PricingRule) bool {
	if matched == nil {
		return false
	}
	for _, rule := range prices {
		if matchImageCondition(req, rule.Conditions) {
			*matched = rule
			return true
		}
	}
	return false
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
	return PricingFallbackRule{}, fmt.Errorf("missing token fallback pricing rule for operation %q meter %q", operation, meter)
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
	return PricingFallbackRule{}, fmt.Errorf("missing image fallback pricing rule for provider %q model %q", provider, modelName)
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

func pricingModelIdentity(model *pricingModelRecord, found bool) (string, string) {
	if !found || model == nil {
		return "", ""
	}
	return strings.TrimSpace(model.Provider), strings.TrimSpace(model.Name)
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

	rawInputCredits := inputUSD.Mul(decimal.NewFromInt(pricingCreditsPerUSD))
	rawOutputCredits := outputUSD.Mul(decimal.NewFromInt(pricingCreditsPerUSD))

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
