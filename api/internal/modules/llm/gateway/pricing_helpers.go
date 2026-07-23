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
	quote, err := engine.QuoteTokens(ctx, model, promptTokens, completionTokens)
	if err != nil {
		return PricingQuote{}, wrapPricingNotConfiguredError(err, pricingErrorParamsFromModelRef(model))
	}
	return quote, nil
}

func (s *llmGatewayServiceImpl) quoteTokenPricingForSelection(
	ctx context.Context,
	selection *ProviderSelection,
	model PricingModelRef,
	promptTokens int,
	completionTokens int,
) (PricingQuote, error) {
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("failed to resolve billing lane for token pricing: %w", err)
	}
	if lane == UsageBillingLanePlatform {
		return PricingQuote{}, nil
	}
	return s.quoteTokenPricing(ctx, model, promptTokens, completionTokens)
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
	quote, err := engine.QuoteImage(ctx, model, req)
	if err != nil {
		return PricingQuote{}, wrapPricingNotConfiguredError(err, pricingErrorParamsFromModelRef(PricingModelRef{
			ModelID:        model.ModelID,
			OrganizationID: model.OrganizationID,
			Source:         model.Source,
			Operation:      PricingOperationImage,
			Provider:       model.Provider,
			Model:          model.Model,
		}))
	}
	return quote, nil
}

func (s *llmGatewayServiceImpl) quoteImagePricingForSelection(
	ctx context.Context,
	selection *ProviderSelection,
	model PricingModelRef,
	req *adapter.ImageRequest,
) (PricingQuote, error) {
	lane, err := usageBillingLaneFromContext(selection, nil)
	if err != nil {
		return PricingQuote{}, fmt.Errorf("failed to resolve billing lane for image pricing: %w", err)
	}
	if lane == UsageBillingLanePlatform {
		return PricingQuote{}, nil
	}
	return s.quoteImagePricing(ctx, model, req)
}

func pricingErrorParamsFromModelRef(model PricingModelRef) map[string]interface{} {
	model = normalizePricingModelRef(model)
	params := map[string]interface{}{
		"model_source": string(model.Source),
		"operation":    string(model.Operation),
	}
	if model.ModelID != uuid.Nil {
		params["model_id"] = model.ModelID.String()
	}
	if model.OrganizationID != uuid.Nil {
		params["organization_id"] = model.OrganizationID.String()
	}
	if model.Provider != "" {
		params["provider"] = model.Provider
	}
	if model.Model != "" {
		params["model"] = model.Model
	}
	return params
}

// PricingErrorParamsFromModelRef builds user-facing pricing error parameters.
func PricingErrorParamsFromModelRef(model PricingModelRef) map[string]interface{} {
	return pricingErrorParamsFromModelRef(model)
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
		ModelID:        selection.Model.ID,
		OrganizationID: selection.OrganizationID,
		Source:         selection.ModelSource,
		Provider:       provider,
		Model:          model,
	})
}

func pricingModelRefFromBillingContext(bc *BillingContext) PricingModelRef {
	if bc == nil {
		return PricingModelRef{Source: PricingModelSourceGlobal}
	}
	ref := PricingModelRef{
		ModelID:        bc.ModelID,
		OrganizationID: uuid.Nil,
		Source:         bc.ModelSource,
		Operation:      bc.PricingOperation,
		Provider:       bc.ProviderName,
		Model:          bc.ModelName,
	}
	if organizationID, err := uuid.Parse(strings.TrimSpace(bc.OrganizationID)); err == nil {
		ref.OrganizationID = organizationID
	}
	return normalizePricingModelRef(ref)
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
	if bc.UsageSource != UsageSourceEstimatedUsage && quote.UsageSource != "" {
		bc.UsageSource = quote.UsageSource
	}
	bc.PricingSnapshot = quote.PricingSnapshot
}

func lockTokenPricingQuote(bc *BillingContext, quote PricingQuote) {
	if bc == nil {
		return
	}
	locked := quote
	bc.LockedTokenQuote = &locked
}
