package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const pricingCreditsPerUSD = int64(1_000_000)

var pricingPerMillionDivisor = decimal.NewFromInt(1000000)
var pricingDefaultInputPricePerMillion = decimal.NewFromInt(1)
var pricingDefaultOutputPricePerMillion = decimal.NewFromInt(2)

type PricingSource string

const (
	PricingSourceUSDPrice                 PricingSource = "usd_price"
	PricingSourceDefaultUSDFallback       PricingSource = "default_usd_fallback"
	PricingSourceDefaultImageRuleFallback PricingSource = "default_image_rule_fallback"
)

type PricingModelSource string

const (
	PricingModelSourceGlobal      PricingModelSource = "global"
	PricingModelSourceCustom      PricingModelSource = "custom"
	PricingModelSourcePassthrough PricingModelSource = "passthrough"
)

type PricingModelRef struct {
	ModelID uuid.UUID
	Source  PricingModelSource
}

type PricingQuote struct {
	InputUSD      decimal.Decimal
	OutputUSD     decimal.Decimal
	TotalUSD      decimal.Decimal
	InputCredits  int64
	OutputCredits int64
	TotalCredits  int64
	Source        PricingSource
}

type PricingEngine interface {
	QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error)
	QuoteImage(ctx context.Context, model PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error)
}

type pricingEngine struct {
	db *gorm.DB
}

type pricingModelRecord struct {
	ID          uuid.UUID       `gorm:"column:id"`
	Provider    string          `gorm:"column:provider"`
	InputPrice  decimal.Decimal `gorm:"column:input_price"`
	OutputPrice decimal.Decimal `gorm:"column:output_price"`
}

func NewPricingEngine(db *gorm.DB) PricingEngine {
	return &pricingEngine{db: db}
}

func (e *pricingEngine) QuoteTokens(ctx context.Context, ref PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error) {
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}

	inputUSD := decimal.Zero
	outputUSD := decimal.Zero
	source := PricingSourceUSDPrice

	if promptTokens > 0 {
		inputPrice := pricingDefaultInputPricePerMillion
		if found && model != nil && !model.InputPrice.IsZero() {
			inputPrice = model.InputPrice
		} else {
			source = PricingSourceDefaultUSDFallback
		}
		inputUSD = inputPrice.
			Mul(decimal.NewFromInt(int64(promptTokens))).
			Div(pricingPerMillionDivisor)
	}

	if completionTokens > 0 {
		outputPrice := pricingDefaultOutputPricePerMillion
		if found && model != nil && !model.OutputPrice.IsZero() {
			outputPrice = model.OutputPrice
		} else {
			source = PricingSourceDefaultUSDFallback
		}
		outputUSD = outputPrice.
			Mul(decimal.NewFromInt(int64(completionTokens))).
			Div(pricingPerMillionDivisor)
	}

	return newUSDQuote(inputUSD, outputUSD, source), nil
}

func (e *pricingEngine) QuoteImage(ctx context.Context, ref PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error) {
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
		totalUnitUSD := model.InputPrice.Add(model.OutputPrice)
		if !totalUnitUSD.IsZero() {
			totalUSD := totalUnitUSD.Mul(decimal.NewFromInt(count))
			return newOutputOnlyUSDQuote(totalUSD, PricingSourceUSDPrice), nil
		}
	}

	provider := inferImagePricingProvider(req, model, found)
	defaultPrices := defaultImagePricingRules(provider)

	var matched llmmodel.PricingRule
	if !matchImagePricingRule(&r, defaultPrices, &matched) {
		return PricingQuote{}, nil
	}

	if matched.Price.Amount > 0 {
		totalUSD := decimal.NewFromFloat(matched.Price.Amount).Mul(decimal.NewFromInt(count))
		return newOutputOnlyUSDQuote(totalUSD, PricingSourceDefaultImageRuleFallback), nil
	}

	if matched.Price.Credits <= 0 {
		return PricingQuote{}, nil
	}

	totalCredits := matched.Price.Credits * count
	return PricingQuote{
		TotalCredits:  totalCredits,
		OutputCredits: totalCredits,
		Source:        PricingSourceDefaultImageRuleFallback,
	}, nil
}

func inferImagePricingProvider(req *adapter.ImageRequest, model *pricingModelRecord, found bool) string {
	if found && model != nil {
		return model.Provider
	}
	if req == nil {
		return ""
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" {
		return ""
	}
	if provider, _, ok := strings.Cut(modelName, "/"); ok {
		return provider
	}
	return ""
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
	return ref
}

func (e *pricingEngine) loadModelFromTable(ctx context.Context, table string, modelID uuid.UUID) (*pricingModelRecord, bool, error) {
	var model pricingModelRecord
	err := e.db.WithContext(ctx).
		Table(table).
		Select("id, provider, input_price, output_price").
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

func newUSDQuote(inputUSD, outputUSD decimal.Decimal, source PricingSource) PricingQuote {
	totalUSD := inputUSD.Add(outputUSD)
	totalCredits := creditsFromUSD(totalUSD)
	inputCredits, outputCredits := splitCreditsByUSD(inputUSD, outputUSD, totalCredits)

	return PricingQuote{
		InputUSD:      inputUSD,
		OutputUSD:     outputUSD,
		TotalUSD:      totalUSD,
		InputCredits:  inputCredits,
		OutputCredits: outputCredits,
		TotalCredits:  totalCredits,
		Source:        source,
	}
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

func newOutputOnlyUSDQuote(totalUSD decimal.Decimal, source PricingSource) PricingQuote {
	return newUSDQuote(decimal.Zero, totalUSD, source)
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
