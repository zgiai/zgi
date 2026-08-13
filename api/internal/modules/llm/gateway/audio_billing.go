package gateway

import (
	"context"
	"fmt"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	meterInputText          = "input_text"
	meterInputAudioDuration = "input_audio_duration"
	baseUnitBilledCharacter = "billed_character"
	baseUnitMillisecond     = "millisecond"
)

func (s *llmGatewayServiceImpl) quoteMeteredPricing(
	ctx context.Context,
	selection *ProviderSelection,
	usage MeteredUsage,
) (PricingQuote, error) {
	engine := s.pricing()
	if engine == nil {
		return PricingQuote{}, fmt.Errorf("pricing engine is not configured")
	}
	ref := pricingModelRefFromSelection(selection)
	ref.Operation = usage.Operation
	quote, err := engine.QuoteMetered(ctx, ref, usage)
	if err != nil {
		return PricingQuote{}, wrapPricingNotConfiguredError(err, pricingErrorParamsFromModelRef(ref))
	}
	return quote, nil
}

func (s *llmGatewayServiceImpl) settlePrivateMeteredSuccess(
	ctx context.Context,
	billingCtx *BillingContext,
	selection *ProviderSelection,
	quote PricingQuote,
	responseTime int64,
) error {
	decision, err := s.resolveBillingDecision(selection, billingCtx)
	if err != nil {
		return wrapBillingLaneMismatchError(err)
	}
	if decision.UseSystemProvider {
		return fmt.Errorf("%w: private metered settlement selected the platform lane", ErrBillingLaneMismatch)
	}
	billingCtx.ActualCredits = quote.TotalCredits
	applyPricingQuoteToBillingContext(billingCtx, quote)
	billingCtx.Status = "success"
	billingCtx.ResponseTime = responseTime
	settlementCtx, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if err := s.billingProviderForDecision(decision).Settle(settlementCtx, billingCtx); err != nil {
		return wrapBillingSettleError(err, billingCtx, false, decision.RouteID)
	}
	return nil
}

func audioCapabilityError(operation, model string) error {
	return fmt.Errorf("%w: no %s adapter is available for model %q", adapter.ErrCapabilityUnsupported, operation, model)
}
