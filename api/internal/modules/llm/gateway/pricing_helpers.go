package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// Platform reservations include a bounded prompt margin because provider
	// tokenizers can count protocol framing differently from the local encoder.
	platformPromptReservationMinimumSlack = 16
	platformPromptReservationPercent      = 10

	// Reasoning providers may report hidden reasoning tokens beyond a small
	// requested max_tokens value. These floors keep pre-deduction conservative
	// without reserving a model's entire output window.
	reasoningPromptReservationFloor     = 64
	reasoningCompletionReservationFloor = 512
	reasoningCompletionReservationCap   = 4096
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
		authority, ok := s.billing.(interface {
			QuotePlatformTokenPricing(context.Context, PricingModelRef, int, int) (PricingQuote, error)
		})
		if !ok {
			return PricingQuote{}, fmt.Errorf("platform token pricing authority is not configured")
		}
		promptTokens, completionTokens = conservativePlatformReservationTokens(selection, promptTokens, completionTokens)
		return authority.QuotePlatformTokenPricing(ctx, model, promptTokens, completionTokens)
	}
	promptTokens, completionTokens = conservativeReasoningReservationTokens(selection, promptTokens, completionTokens)
	quote, err := s.quoteTokenPricing(ctx, model, promptTokens, completionTokens)
	if err != nil {
		return PricingQuote{}, err
	}
	return s.enrichPrivateTokenQuoteForSettlement(ctx, model, quote)
}

type privateTokenQuoteEnricher interface {
	enrichTokenQuoteWithCachePricingBasis(context.Context, PricingModelRef, PricingQuote) (PricingQuote, error)
}

func (s *llmGatewayServiceImpl) enrichPrivateTokenQuoteForSettlement(
	ctx context.Context,
	model PricingModelRef,
	quote PricingQuote,
) (PricingQuote, error) {
	engine := s.pricing()
	enricher, ok := engine.(privateTokenQuoteEnricher)
	if !ok {
		return quote, nil
	}
	return enricher.enrichTokenQuoteWithCachePricingBasis(ctx, model, quote)
}

func conservativePlatformReservationTokens(selection *ProviderSelection, promptTokens, completionTokens int) (int, int) {
	promptTokens, completionTokens = conservativeReasoningReservationTokens(selection, promptTokens, completionTokens)
	if promptTokens <= 0 {
		return promptTokens, completionTokens
	}
	percentageSlack := (promptTokens*platformPromptReservationPercent + 99) / 100
	slack := max(platformPromptReservationMinimumSlack, percentageSlack)
	return promptTokens + slack, completionTokens
}

func conservativeReasoningReservationTokens(selection *ProviderSelection, promptTokens, completionTokens int) (int, int) {
	if selection == nil || !selection.Model.SupportsReasoning {
		return promptTokens, completionTokens
	}
	if promptTokens > 0 && promptTokens < reasoningPromptReservationFloor {
		promptTokens = reasoningPromptReservationFloor
	}
	completionTokens = conservativeReasoningCompletionTokens(selection.Model.MaxOutputTokens, completionTokens)
	return promptTokens, completionTokens
}

func conservativeReasoningCompletionTokens(maxOutputTokens, completionTokens int) int {
	if completionTokens < 0 {
		completionTokens = 0
	}
	reserved := max(completionTokens, reasoningCompletionReservationFloor)
	if maxOutputTokens > 0 {
		catalogBound := min(maxOutputTokens, reasoningCompletionReservationCap)
		reserved = max(reserved, catalogBound)
	}
	return reserved
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

type cacheTokenPricingEngine interface {
	QuoteTokenUsage(context.Context, PricingModelRef, TokenUsage) (PricingQuote, error)
}

func (s *llmGatewayServiceImpl) quoteTokenPricingForSettlementUsage(
	ctx context.Context,
	bc *BillingContext,
	model PricingModelRef,
	usage TokenUsage,
) (PricingQuote, error) {
	usage = normalizeGatewayTokenUsage(usage)
	if bc != nil && bc.LockedTokenQuote != nil && lockedTokenQuoteSupportsUsage(*bc.LockedTokenQuote, usage) {
		return repriceLockedTokenUsage(*bc.LockedTokenQuote, usage)
	}
	if usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 && usage.CacheWrite5mTokens == 0 && usage.CacheWrite1hTokens == 0 {
		return s.quoteTokenPricingForSettlement(ctx, bc, model, usage.InputTokens, usage.OutputTokens)
	}
	engine := s.pricing()
	cacheEngine, ok := engine.(cacheTokenPricingEngine)
	if !ok {
		return PricingQuote{}, fmt.Errorf("pricing engine does not support cache token pricing")
	}
	quote, err := cacheEngine.QuoteTokenUsage(ctx, model, usage)
	if err != nil {
		return PricingQuote{}, wrapPricingNotConfiguredError(err, pricingErrorParamsFromModelRef(model))
	}
	return quote, nil
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
		// Cloud/official image prices are owned by Console.  Do not consult the
		// Pre pricing engine and do not manufacture a uniform image estimate:
		// the remote proxy performs the authoritative deduction and returns the
		// settlement facts after the upstream call.
		return PricingQuote{
			PricingSource: PricingSourcePlatformRemoteAuthority,
			UsageSource:   UsageSourceRequestParameters,
		}, nil
	}
	return s.quoteImagePricing(ctx, model, req)
}

func fallbackPlatformTokenPredeductQuote(promptTokens, completionTokens int) PricingQuote {
	const estimateUSDPerMillionTokens = int64(10)
	unit := decimal.NewFromInt(estimateUSDPerMillionTokens).Div(decimal.NewFromInt(1_000_000))
	inputUSD := decimal.NewFromInt(int64(max(promptTokens, 0))).Mul(unit)
	outputUSD := decimal.NewFromInt(int64(max(completionTokens, 0))).Mul(unit)
	return newUSDQuote(inputUSD, outputUSD, PricingSourcePlatformPredeductEstimate, "platform.uniform.max-token-estimate", UsageSourceEstimatedUsage, buildPricingSnapshot(map[string]interface{}{
		"pricing_source":             PricingSourcePlatformPredeductEstimate,
		"estimate_usd_per_1m_tokens": estimateUSDPerMillionTokens,
	}))
}

func fallbackPlatformImagePredeductQuote(req *adapter.ImageRequest) PricingQuote {
	count := 1
	if req != nil && req.N != nil && *req.N > 0 {
		count = *req.N
	}
	return newOutputOnlyUSDQuote(decimal.NewFromInt(int64(count)), PricingSourcePlatformPredeductEstimate, "platform.uniform.image-estimate", UsageSourceEstimatedUsage, buildPricingSnapshot(map[string]interface{}{
		"pricing_source":         PricingSourcePlatformPredeductEstimate,
		"estimate_usd_per_image": 1,
		"image_count":            count,
	}))
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
	modelID := selection.Model.ID
	// Official/platform selections are represented by a route model and do
	// not carry the catalog model ID in selection.Model.ID.  The catalog
	// identity is hydrated separately so pricing lookup must use it; otherwise
	// QuoteImage/QuoteTokens fall back as if pricing were not configured.
	if selection.UseSystemProvider && selection.ModelVendorModelID != uuid.Nil {
		modelID = selection.ModelVendorModelID
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
		ModelID:        modelID,
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
	if len(quote.PricingSnapshot) > 0 && string(quote.PricingSnapshot) != "null" {
		bc.PricingSnapshot = quote.PricingSnapshot
	}
}

func pricingQuoteHasTokenLock(quote PricingQuote) bool {
	return quote.InputTokenPriceResolved || quote.OutputTokenPriceResolved
}

func lockTokenPricingQuote(bc *BillingContext, quote PricingQuote) {
	if bc == nil {
		return
	}
	locked := quote
	bc.LockedTokenQuote = &locked
}
