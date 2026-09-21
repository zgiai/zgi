package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type platformMediaSettlementMirrorer interface {
	MirrorPlatformMediaSettlement(context.Context, *BillingContext, *adapter.SettlementResult) error
}

func (s *llmGatewayServiceImpl) mirrorPlatformMediaSettlement(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	selection *ProviderSelection,
	billingOrganizationID uuid.UUID,
	requestID, attemptID string,
	operation PricingOperation,
	streaming bool,
	startedAt time.Time,
	settlement *adapter.SettlementResult,
) error {
	mirrorer, ok := s.billing.(platformMediaSettlementMirrorer)
	if !ok {
		return nil
	}
	if settlement == nil {
		return fmt.Errorf("console returned no media settlement")
	}
	bc := s.createBillingContext(apiKey, nil, selection, getChannelID(selection), billingOrganizationID, 0, streaming, startedAt, requestID, attemptID)
	bc.PricingOperation = operation
	// Product surfaces declare their source on the context; a public Gateway
	// call carries none and resolves to "api". Without this the media mirror
	// persisted an empty source, which normalized to "unknown".
	bc.InvocationSource = resolveInvocationSource(ctx, nil)
	bc.RoutingStrategy = selection.RoutingStrategy
	bc.ResponseTime = time.Since(startedAt).Milliseconds()
	bc.DeductionID = settlement.SettlementID
	return mirrorer.MirrorPlatformMediaSettlement(ctx, bc, settlement)
}
