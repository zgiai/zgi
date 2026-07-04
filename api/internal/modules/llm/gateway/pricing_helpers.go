package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func (s *llmGatewayServiceImpl) pricing() PricingEngine {
	if s.pricingEngine != nil {
		return s.pricingEngine
	}
	if s.db == nil {
		return nil
	}
	s.pricingEngine = NewPricingEngine(s.db)
	return s.pricingEngine
}

func (s *llmGatewayServiceImpl) quoteTokenPricing(
	ctx context.Context,
	model PricingModelRef,
	promptTokens int,
	completionTokens int,
) (PricingQuote, error) {
	engine := s.pricing()
	if engine == nil {
		return PricingQuote{}, fmt.Errorf("pricing engine is not configured")
	}
	return engine.QuoteTokens(ctx, model, promptTokens, completionTokens)
}

func (s *llmGatewayServiceImpl) quoteTokenPricingForSettlement(
	ctx context.Context,
	bc *BillingContext,
	model PricingModelRef,
	promptTokens int,
	completionTokens int,
) (PricingQuote, error) {
	if bc != nil && bc.LockedTokenQuote != nil {
		return repriceLockedTokenQuote(*bc.LockedTokenQuote, promptTokens, completionTokens)
	}
	return s.quoteTokenPricing(ctx, model, promptTokens, completionTokens)
}

func (s *llmGatewayServiceImpl) quoteImagePricing(
	ctx context.Context,
	model PricingModelRef,
	req *adapter.ImageRequest,
) (PricingQuote, error) {
	engine := s.pricing()
	if engine == nil {
		return PricingQuote{}, fmt.Errorf("pricing engine is not configured")
	}
	return engine.QuoteImage(ctx, model, req)
}

func pricingModelRefFromSelection(selection *ProviderSelection) PricingModelRef {
	if selection == nil {
		return PricingModelRef{Source: PricingModelSourceGlobal}
	}
	provider := strings.TrimSpace(selection.Model.Provider)
	if provider == "" {
		provider = strings.TrimSpace(selection.ChannelProvider)
	}
	if provider == "" {
		provider = strings.TrimSpace(selection.Provider.Provider)
	}
	model := strings.TrimSpace(selection.Model.Model)
	if model == "" {
		model = strings.TrimSpace(selection.Model.ModelName)
	}
	return normalizePricingModelRef(PricingModelRef{
		OrganizationID: selection.OrganizationID,
		ModelID:        selection.Model.ID,
		Source:         selection.ModelSource,
		Provider:       provider,
		Model:          model,
	})
}

func pricingModelRefFromBillingContext(bc *BillingContext) PricingModelRef {
	if bc == nil {
		return PricingModelRef{Source: PricingModelSourceGlobal}
	}
	var organizationID uuid.UUID
	if parsed, err := uuid.Parse(strings.TrimSpace(bc.OrganizationID)); err == nil {
		organizationID = parsed
	}
	return normalizePricingModelRef(PricingModelRef{
		OrganizationID: organizationID,
		ModelID:        bc.ModelID,
		Source:         bc.ModelSource,
		Operation:      bc.PricingOperation,
		Provider:       bc.ProviderName,
		Model:          bc.ModelName,
	})
}

func applyPricingQuoteToBillingContext(bc *BillingContext, quote PricingQuote) {
	if bc == nil {
		return
	}

	bc.InputUSD = quote.InputUSD
	bc.OutputUSD = quote.OutputUSD
	bc.TotalUSD = quote.TotalUSD
	bc.InputCost = decimal.NewFromInt(quote.InputCredits)
	bc.OutputCost = decimal.NewFromInt(quote.OutputCredits)
	bc.TotalCost = decimal.NewFromInt(quote.TotalCredits)
	bc.PricingSource = quote.PricingSource
	bc.UsageSource = quote.UsageSource
	bc.PricingSnapshot = quote.PricingSnapshot
}

func lockTokenPricingQuote(bc *BillingContext, quote PricingQuote) {
	if bc == nil {
		return
	}
	locked := quote
	bc.LockedTokenQuote = &locked
}
