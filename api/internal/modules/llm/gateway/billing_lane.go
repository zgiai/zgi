package gateway

import (
	"fmt"
	"strings"
)

// BillingLane represents which billing backend should be used for a request.
type BillingLane string

const (
	BillingLaneRemote BillingLane = "remote"
	BillingLaneLocal  BillingLane = "local"
)

// UsageBillingLane represents who owns billing settlement for a request.
type UsageBillingLane string

const (
	UsageBillingLanePlatform UsageBillingLane = "platform"
	UsageBillingLanePrivate  UsageBillingLane = "private"
)

// BillingDecision captures lane selection and key context for observability.
type BillingDecision struct {
	Lane              BillingLane
	UsageLane         UsageBillingLane
	UseSystemProvider bool
	RouteID           string
	RequestID         string
}

func (s *llmGatewayServiceImpl) resolveBillingDecision(
	providerSelection *ProviderSelection,
	billingCtx *BillingContext,
) (BillingDecision, error) {
	usageLane, err := usageBillingLaneFromContext(providerSelection, billingCtx)
	if err != nil {
		return BillingDecision{}, err
	}
	useSystemProvider := usageBillingLaneUsesSystemProvider(usageLane)

	decision := BillingDecision{
		UsageLane:         usageLane,
		UseSystemProvider: useSystemProvider,
		RouteID:           routeIDFromSelection(providerSelection),
		RequestID:         requestIDString(billingCtx),
	}
	if decision.RouteID == "" && billingCtx != nil {
		decision.RouteID = routeIDString(billingCtx.ChannelID)
	}

	if useSystemProvider {
		decision.Lane = BillingLaneRemote
	} else {
		decision.Lane = BillingLaneLocal
	}

	// Enforce strict lane consistency in CLOUD mode.
	if s.consoleProvider != nil && s.consoleProvider.GetMode() == "CLOUD" {
		switch decision.Lane {
		case BillingLaneRemote:
			// In CLOUD mode, remote lane must never point to local billing.
			if s.localBilling != nil && s.billing == s.localBilling {
				return decision, fmt.Errorf(
					"%w: request_id=%s route_id=%s use_system_provider=%t expected_lane=%s actual_lane=%s",
					ErrBillingLaneMismatch,
					decision.RequestID,
					decision.RouteID,
					decision.UseSystemProvider,
					BillingLaneRemote,
					BillingLaneLocal,
				)
			}
		case BillingLaneLocal:
			// Private lane must always have a dedicated local billing backend.
			if s.localBilling == nil {
				return decision, fmt.Errorf(
					"%w: request_id=%s route_id=%s use_system_provider=%t expected_lane=%s actual_lane=%s",
					ErrBillingLaneMismatch,
					decision.RequestID,
					decision.RouteID,
					decision.UseSystemProvider,
					BillingLaneLocal,
					"unknown",
				)
			}
		}
	}

	return decision, nil
}

func (s *llmGatewayServiceImpl) billingProviderForDecision(decision BillingDecision) BillingProvider {
	if decision.Lane == BillingLaneLocal && s.localBilling != nil {
		return s.localBilling
	}
	return s.billing
}

func usageBillingLaneFromSystemProvider(useSystemProvider bool) UsageBillingLane {
	if useSystemProvider {
		return UsageBillingLanePlatform
	}
	return UsageBillingLanePrivate
}

func usageBillingLaneUsesSystemProvider(lane UsageBillingLane) bool {
	return lane == UsageBillingLanePlatform
}

func normalizeUsageBillingLane(lane UsageBillingLane, useSystemProvider bool) (UsageBillingLane, error) {
	trimmed := UsageBillingLane(strings.TrimSpace(string(lane)))
	switch trimmed {
	case "":
		return usageBillingLaneFromSystemProvider(useSystemProvider), nil
	case UsageBillingLanePlatform, UsageBillingLanePrivate:
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported billing lane %q", trimmed)
	}
}

func usageBillingLaneFromContext(providerSelection *ProviderSelection, billingCtx *BillingContext) (UsageBillingLane, error) {
	if providerSelection != nil {
		return normalizeUsageBillingLane(providerSelection.BillingLane, providerSelection.UseSystemProvider)
	}
	if billingCtx != nil {
		return normalizeUsageBillingLane(billingCtx.BillingLane, billingCtx.UseSystemProvider)
	}
	return UsageBillingLanePrivate, nil
}

func normalizeBillingContextUsageLane(bc *BillingContext) (UsageBillingLane, error) {
	if bc == nil {
		return "", fmt.Errorf("billing context is nil")
	}

	lane, err := normalizeUsageBillingLane(bc.BillingLane, bc.UseSystemProvider)
	if err != nil {
		return "", err
	}
	bc.BillingLane = lane
	bc.UseSystemProvider = usageBillingLaneUsesSystemProvider(lane)
	return lane, nil
}

func billingLaneString(billingCtx *BillingContext, useSystemProvider bool) string {
	if billingCtx != nil {
		lane, err := normalizeUsageBillingLane(billingCtx.BillingLane, billingCtx.UseSystemProvider)
		if err == nil {
			return string(lane)
		}
	}
	return string(usageBillingLaneFromSystemProvider(useSystemProvider))
}
