package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var meteredAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

type structuredMeteredPricing struct {
	Metered []structuredMeteredPrice `json:"metered"`
}

type structuredMeteredPrice struct {
	Operation  PricingOperation      `json:"operation"`
	Meter      string                `json:"meter"`
	BaseUnit   string                `json:"base_unit"`
	Price      structuredMeteredRate `json:"price"`
	Dimensions map[string]string     `json:"dimensions,omitempty"`
}

type structuredMeteredRate struct {
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	PerQuantity int64  `json:"per_quantity"`
}

func (e *pricingEngine) QuoteMetered(ctx context.Context, ref PricingModelRef, usage MeteredUsage) (PricingQuote, error) {
	usage.Operation = PricingOperation(strings.TrimSpace(string(usage.Operation)))
	usage.Meter = strings.TrimSpace(usage.Meter)
	usage.BaseUnit = strings.TrimSpace(usage.BaseUnit)
	if usage.Operation == "" || usage.Meter == "" || usage.BaseUnit == "" || usage.Quantity < 0 {
		return PricingQuote{}, fmt.Errorf("invalid metered usage")
	}
	ref.Operation = usage.Operation
	model, found, err := e.loadModel(ctx, ref)
	if err != nil {
		return PricingQuote{}, err
	}
	if !found || model == nil || len(model.Pricing) == 0 || string(model.Pricing) == "null" {
		return PricingQuote{}, fmt.Errorf("%w: missing metered model pricing", ErrPricingNotConfigured)
	}
	selected, err := resolveMeteredPrice(model.Pricing, usage)
	if err != nil {
		return PricingQuote{}, err
	}
	amountString := strings.TrimSpace(selected.Price.Amount)
	amount, err := decimal.NewFromString(amountString)
	if err != nil || !meteredAmountPattern.MatchString(amountString) || amount.IsNegative() || selected.Price.PerQuantity <= 0 {
		return PricingQuote{}, fmt.Errorf("%w: invalid metered price", ErrPricingNotConfigured)
	}
	priceUSD := amount
	currency := strings.TrimSpace(selected.Price.Currency)
	switch currency {
	case "USD":
	case "CNY":
		priceUSD = amount.Div(loadOrganizationUSDToCNYRate(ctx, e.db, ref.OrganizationID))
	default:
		return PricingQuote{}, fmt.Errorf("%w: unsupported metered pricing currency %q", ErrPricingNotConfigured, currency)
	}
	unitPriceUSD := priceUSD.Div(decimal.NewFromInt(selected.Price.PerQuantity))
	totalUSD := unitPriceUSD.Mul(decimal.NewFromInt(usage.Quantity))
	sourceCost := amount.Mul(decimal.NewFromInt(usage.Quantity)).Div(decimal.NewFromInt(selected.Price.PerQuantity))
	ruleID := strings.Join([]string{string(usage.Operation), usage.Meter, usage.BaseUnit}, "/")
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source":     PricingSourceUpstreamModelPrice,
		"usage_source":       UsageSourceRequestParameters,
		"operation":          usage.Operation,
		"meter":              usage.Meter,
		"base_unit":          usage.BaseUnit,
		"quantity":           usage.Quantity,
		"dimensions":         usage.Dimensions,
		"price_amount":       selected.Price.Amount,
		"price_currency":     currency,
		"price_per_quantity": selected.Price.PerQuantity,
		"source_cost":        sourceCost.String(),
		"total_usd":          totalUSD.String(),
	})
	quote := newOutputOnlyUSDQuote(totalUSD, PricingSourceUpstreamModelPrice, ruleID, UsageSourceRequestParameters, snapshot)
	return withMeteredPricingBasis(quote, usage, unitPriceUSD), nil
}

func resolveMeteredPrice(raw datatypes.JSON, usage MeteredUsage) (structuredMeteredPrice, error) {
	var pricing structuredMeteredPricing
	if err := json.Unmarshal(raw, &pricing); err != nil {
		return structuredMeteredPrice{}, fmt.Errorf("%w: invalid structured pricing: %v", ErrPricingNotConfigured, err)
	}
	bestIndex := -1
	bestScore := -1
	ambiguous := false
	for index, candidate := range pricing.Metered {
		if candidate.Operation != usage.Operation || candidate.Meter != usage.Meter || candidate.BaseUnit != usage.BaseUnit {
			continue
		}
		if !meteredDimensionsMatch(candidate.Dimensions, usage.Dimensions) {
			continue
		}
		score := len(candidate.Dimensions)
		if score > bestScore {
			bestIndex = index
			bestScore = score
			ambiguous = false
		} else if score == bestScore {
			ambiguous = true
		}
	}
	if bestIndex < 0 {
		return structuredMeteredPrice{}, fmt.Errorf("%w: no metered pricing matches %s/%s/%s", ErrPricingNotConfigured, usage.Operation, usage.Meter, usage.BaseUnit)
	}
	if ambiguous {
		return structuredMeteredPrice{}, fmt.Errorf("%w: ambiguous metered pricing matches %s/%s/%s", ErrPricingNotConfigured, usage.Operation, usage.Meter, usage.BaseUnit)
	}
	return pricing.Metered[bestIndex], nil
}

func meteredDimensionsMatch(priceDimensions, requestDimensions map[string]string) bool {
	for key, value := range priceDimensions {
		if requestDimensions[key] != value {
			return false
		}
	}
	return true
}

func loadOrganizationUSDToCNYRate(ctx context.Context, db *gorm.DB, organizationID uuid.UUID) decimal.Decimal {
	fallback := decimal.NewFromInt(7)
	if db == nil || organizationID == uuid.Nil {
		return fallback
	}
	var row struct {
		USDToCNYRate decimal.Decimal `gorm:"column:usd_to_cny_rate"`
	}
	err := db.WithContext(ctx).
		Table("organizations").
		Select("usd_to_cny_rate").
		Where("id = ?", organizationID).
		Take(&row).Error
	if err != nil || !row.USDToCNYRate.IsPositive() {
		return fallback
	}
	return row.USDToCNYRate
}

func withMeteredPricingBasis(quote PricingQuote, usage MeteredUsage, unitPriceUSD decimal.Decimal) PricingQuote {
	quote.MeteredOperation = usage.Operation
	quote.MeteredMeter = usage.Meter
	quote.MeteredBaseUnit = usage.BaseUnit
	quote.MeteredPriceUSDPerUnit = unitPriceUSD
	quote.MeteredPriceResolved = true
	return quote
}

func repriceLockedMeteredQuote(locked PricingQuote, usage MeteredUsage) (PricingQuote, error) {
	if usage.Quantity < 0 {
		return PricingQuote{}, fmt.Errorf("metered quantity must be greater than or equal to zero")
	}
	if !locked.MeteredPriceResolved || locked.MeteredPriceUSDPerUnit.IsNegative() {
		return PricingQuote{}, fmt.Errorf("locked metered pricing is missing its unit price")
	}
	if usage.Operation != locked.MeteredOperation || usage.Meter != locked.MeteredMeter || usage.BaseUnit != locked.MeteredBaseUnit {
		return PricingQuote{}, fmt.Errorf("locked metered pricing contract does not match actual usage")
	}
	usageSource := locked.UsageSource
	if usageSource == "" {
		usageSource = UsageSourceRequestParameters
	}
	snapshot := buildPricingSnapshot(map[string]interface{}{
		"pricing_source": locked.PricingSource,
		"usage_source":   usageSource,
		"operation":      usage.Operation,
		"meter":          usage.Meter,
		"base_unit":      usage.BaseUnit,
		"quantity":       usage.Quantity,
		"dimensions":     usage.Dimensions,
		"unit_price_usd": locked.MeteredPriceUSDPerUnit.String(),
		"rule_id":        locked.RuleID,
		"locked_pricing": true,
	})
	repriced := newOutputOnlyUSDQuote(
		locked.MeteredPriceUSDPerUnit.Mul(decimal.NewFromInt(usage.Quantity)),
		locked.PricingSource,
		locked.RuleID,
		usageSource,
		snapshot,
	)
	return withMeteredPricingBasis(repriced, usage, locked.MeteredPriceUSDPerUnit), nil
}
