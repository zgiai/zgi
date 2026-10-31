package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FinalizePlatformProxySettlement mirrors a console-api proxy settlement locally.
func (s *RemoteBilling) FinalizePlatformProxySettlement(
	ctx context.Context,
	bc *BillingContext,
	settlement *adapter.SettlementResult,
) (retErr error) {
	defer func() {
		if retErr != nil && bc != nil && bc.ToolPostpaid {
			if err := s.markAttemptSettleFailed(ctx, bc, "TOOL_MIRROR_PENDING", retErr.Error()); err != nil {
				retErr = fmt.Errorf("tool mirror failed: %v; persist retry: %w", retErr, err)
			}
		}
	}()
	// Set only when the settlement came from a Console query, the one path that
	// reports the reservation the overage is measured against.
	queriedReservedCredits := int64(0)
	if settlement == nil || bc != nil && bc.ToolPostpaid {
		if bc == nil || strings.TrimSpace(bc.OrganizationID) == "" || strings.TrimSpace(bc.AttemptID) == "" {
			return fmt.Errorf("missing billing identity for console settlement query")
		}
		result, err := waitForConsoleSettlement(ctx, s.consoleAPIBaseURL+"/billing/settlements/by-attempt", bc.OrganizationID, bc.AttemptID)
		if err != nil {
			return fmt.Errorf("query console settlement: %w", err)
		}
		if !result.Found {
			return fmt.Errorf("console settlement not found for attempt %s", bc.AttemptID)
		}
		if !strings.EqualFold(result.AttemptID, "") && result.AttemptID != bc.AttemptID {
			return fmt.Errorf("console settlement attempt mismatch: got=%s want=%s", result.AttemptID, bc.AttemptID)
		}
		if !strings.EqualFold(result.OrganizationID, "") && !strings.EqualFold(result.OrganizationID, bc.OrganizationID) {
			return fmt.Errorf("console settlement organization mismatch: got=%s want=%s", result.OrganizationID, bc.OrganizationID)
		}
		if result.RequestID != "" && result.RequestID != bc.RequestID {
			return fmt.Errorf("console settlement request mismatch: got=%s want=%s", result.RequestID, bc.RequestID)
		}
		if result.ActualQuantity < 0 || result.EstimatedQuantity < 0 || (len(result.PricingSnapshot) > 0 && !json.Valid(result.PricingSnapshot)) {
			return fmt.Errorf("invalid console settlement usage or pricing snapshot")
		}
		if len(result.PricingSnapshot) > 0 && string(result.PricingSnapshot) != "null" {
			bc.PricingSnapshot = datatypes.JSON(result.PricingSnapshot)
			if bc.ToolPostpaid {
				markToolPostpaid(bc)
			}
		}
		projection, projectionErr := toolProjectionFromQuery(result)
		if projectionErr != nil {
			return projectionErr
		}
		if len(projection) > 0 && string(projection) != "null" && strings.EqualFold(result.SettlementStatus, billingAttemptStatusRolledBack) {
			var attempt BillingAttempt
			if err := s.localService.db.WithContext(ctx).Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
				return err
			}
			return s.mirrorQueriedSettlement(ctx, &attempt, result)
		}
		if len(projection) > 0 && string(projection) != "null" {
			if !strings.EqualFold(result.SettlementStatus, billingAttemptStatusSettled) {
				return fmt.Errorf("authoritative tool settlement is still pending")
			}
			var projected toolSettlement
			if json.Unmarshal(projection, &projected) != nil || projected.TotalCredits != result.SettledCredits {
				return fmt.Errorf("authoritative settlement amount mismatch")
			}
			applyPlatformSettlementTokenUsage(bc, settlementFromQueryResult(result))
			return s.mirrorPlatformTools(ctx, bc, projection, datatypes.JSON(result.PricingSnapshot))
		}
		if bc.ToolPostpaid {
			supported, capabilityErr := s.SupportsToolPostpaid(ctx)
			if capabilityErr != nil {
				return fmt.Errorf("verify billing capability after missing tool projection: %w", capabilityErr)
			}
			if supported {
				return fmt.Errorf("authoritative tool settlement JSON is still pending")
			}
			clearToolPostpaid(bc)
		}
		if strings.EqualFold(result.SettlementStatus, billingAttemptStatusRolledBack) && !result.Retryable {
			bc.Status, bc.ActualCredits = "error", 0
			return s.finalizeRemoteSettlement(ctx, bc)
		}
		if !strings.EqualFold(result.SettlementStatus, "SETTLED") {
			return fmt.Errorf("console settlement is not terminal: status=%s retryable=%t", result.SettlementStatus, result.Retryable)
		}
		if !consoleSettlementAmountsValid(result.ReservedCredits, result.SettledCredits, result.RefundedCredits) {
			return fmt.Errorf("invalid console settlement amounts: reserved=%d settled=%d refunded=%d", result.ReservedCredits, result.SettledCredits, result.RefundedCredits)
		}
		queriedReservedCredits = result.ReservedCredits
		settlement = settlementFromQueryResult(result)
		if result.DeductionID != "" && result.DeductionID != result.SettlementID {
			return fmt.Errorf("console settlement deduction mismatch: settlement_id=%s deduction_id=%s", result.SettlementID, result.DeductionID)
		}
	}
	if err := validatePlatformProxySettlement(bc, settlement); err != nil {
		if bc != nil && strings.TrimSpace(bc.AttemptID) != "" {
			_ = s.markAttemptSettleFailed(ctx, bc, "PROXY_SETTLE_INVALID_RESPONSE", err.Error())
		}
		return err
	}
	terminal, err := s.isAttemptTerminal(ctx, bc.AttemptID)
	if err != nil {
		return err
	}
	if terminal {
		return nil
	}

	bc.BillingLane = UsageBillingLanePlatform
	bc.UseSystemProvider = true
	bc.ActualCredits = settlement.OfficialPoints
	applyPlatformSettlementTokenUsage(bc, settlement)
	if err := applyPlatformSettlementCostSnapshot(bc, settlement); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "PROXY_SETTLE_CURRENCY_SNAPSHOT_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("apply platform settlement currency snapshot: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return fmt.Errorf("apply platform settlement currency snapshot: %w", err)
	}
	recordSettlementOverageSnapshot(bc, queriedReservedCredits, settlement.OfficialPoints)
	s.localService.applyRouteAttributionWithLookup(ctx, bc)
	finalizeMediaTimingSnapshot(bc)
	bc.SettledAt = time.Now().UTC()
	if bc.RequestCreatedAt.IsZero() {
		bc.RequestCreatedAt = time.Now().UTC()
	} else {
		bc.RequestCreatedAt = bc.RequestCreatedAt.UTC()
	}

	alreadyFinalized, err := s.prepareRemoteSettlement(ctx, bc)
	if err != nil {
		return fmt.Errorf("prepare proxy settle pending state failed: %w", err)
	}
	if alreadyFinalized {
		return nil
	}

	if err := s.finalizeRemoteSettlement(ctx, bc); err != nil {
		if markErr := s.markAttemptSettleFailed(ctx, bc, "PROXY_SETTLE_FINALIZE_FAILED", err.Error()); markErr != nil {
			return fmt.Errorf("finalize proxy settle failed: %v (additionally failed to mark partial: %w)", err, markErr)
		}
		return fmt.Errorf("finalize proxy settle failed: %w", err)
	}

	return nil
}

// MirrorPlatformMediaSettlement records a Console-owned metered media charge in
// the local attempt/bill ledger without reserving or settling funds a second
// time. Console already performed the authoritative metered deduction.
func (s *RemoteBilling) MirrorPlatformMediaSettlement(ctx context.Context, bc *BillingContext, settlement *adapter.SettlementResult) error {
	if bc == nil || settlement == nil || strings.TrimSpace(settlement.SettlementID) == "" {
		return fmt.Errorf("invalid platform media settlement")
	}
	terminal, err := s.isAttemptTerminal(ctx, bc.AttemptID)
	if err != nil || terminal {
		return err
	}
	bc.BillingLane = UsageBillingLanePlatform
	bc.UseSystemProvider = true
	bc.DeductionID = settlement.SettlementID
	bc.ActualCredits = settlement.OfficialPoints
	applyPlatformSettlementTokenUsage(bc, settlement)
	bc.Status = "success"
	bc.SettledAt = time.Now().UTC()
	if err := applyPlatformSettlementCostSnapshot(bc, settlement); err != nil {
		return fmt.Errorf("apply platform media currency snapshot: %w", err)
	}
	s.localService.applyRouteAttributionWithLookup(ctx, bc)
	finalizeMediaTimingSnapshot(bc)
	return s.localService.db.Transaction(func(tx *gorm.DB) error {
		if err := s.localService.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		invocation := "success"
		if err := s.localService.updateAttemptStatus(ctx, tx, bc, billingAttemptStatusSettled, &invocation, nil, nil); err != nil {
			return err
		}
		return s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusSuccess, nil, nil)
	})
}

// applyPlatformSettlementTokenUsage copies only an authoritative, non-empty
// Console usage snapshot. An empty/legacy settlement must not erase tokens
// already collected on a private or partially mirrored attempt.
func applyPlatformSettlementTokenUsage(bc *BillingContext, settlement *adapter.SettlementResult) {
	if bc == nil || settlement == nil {
		return
	}
	valid := settlement.PromptTokens > 0 || settlement.CompletionTokens > 0 || settlement.TotalTokens > 0 || settlement.CacheReadTokens > 0 || settlement.CacheWriteTokens > 0 || settlement.CacheWrite5mTokens > 0 || settlement.CacheWrite1hTokens > 0 || settlement.ReasoningOutputTokens > 0 || settlement.AudioInputTokens > 0 || settlement.AudioOutputTokens > 0
	if !valid {
		return
	}
	bc.PromptTokens = maxInt(settlement.PromptTokens, 0)
	bc.CompletionTokens = maxInt(settlement.CompletionTokens, 0)
	bc.CacheReadTokens = maxInt(settlement.CacheReadTokens, 0)
	bc.CacheWriteTokens = maxInt(settlement.CacheWriteTokens, 0)
	bc.CacheWrite5mTokens = maxInt(settlement.CacheWrite5mTokens, 0)
	bc.CacheWrite1hTokens = maxInt(settlement.CacheWrite1hTokens, 0)
	if bc.CacheWriteTokens == 0 {
		bc.CacheWriteTokens = bc.CacheWrite5mTokens + bc.CacheWrite1hTokens
	}
	bc.TotalTokens = maxInt(settlement.TotalTokens, 0)
	if bc.TotalTokens == 0 {
		bc.TotalTokens = bc.PromptTokens + bc.CacheReadTokens + bc.CacheWriteTokens + bc.CompletionTokens
	}
}

func maxInt(value, fallback int) int {
	if value < fallback {
		return fallback
	}
	return value
}

// applyPlatformSettlementCostSnapshot merges Console-recorded settlement facts
// and the canonical exchange-rate segment into the billing snapshot. Platform
// rows are authoritative in Console, so the rate is mirrored from the Console
// settlement (rate_source=console_system). A settlement that records no rate is
// fail-closed.
func applyPlatformSettlementCostSnapshot(bc *BillingContext, settlement *adapter.SettlementResult) error {
	if bc == nil || settlement == nil {
		return nil
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	changed := false
	if value, ok := parseNonNegativeBillingDecimal(settlement.TotalCostUSD); ok {
		bc.TotalUSD = value
		values["total_cost_usd"] = value.String()
		changed = true
	}
	if value, ok := parseNonNegativeBillingDecimal(settlement.TotalCostCNY); ok {
		values["total_cost_cny"] = value.String()
		changed = true
	}
	for key, raw := range map[string]string{
		"input_price_usd_per_1m_tokens":          settlement.InputPriceUSDPer1MTokens,
		"cache_read_price_usd_per_1m_tokens":     settlement.CacheReadPriceUSDPer1MTokens,
		"cache_write_price_usd_per_1m_tokens":    settlement.CacheWritePriceUSDPer1MTokens,
		"cache_write_5m_price_usd_per_1m_tokens": settlement.CacheWrite5mPriceUSDPer1MTokens,
		"cache_write_1h_price_usd_per_1m_tokens": settlement.CacheWrite1hPriceUSDPer1MTokens,
		"output_price_usd_per_1m_tokens":         settlement.OutputPriceUSDPer1MTokens,
		"input_cost_usd":                         settlement.InputCostUSD,
		"cache_read_cost_usd":                    settlement.CacheReadCostUSD,
		"cache_write_cost_usd":                   settlement.CacheWriteCostUSD,
		"cache_write_5m_cost_usd":                settlement.CacheWrite5mCostUSD,
		"cache_write_1h_cost_usd":                settlement.CacheWrite1hCostUSD,
		"output_cost_usd":                        settlement.OutputCostUSD,
	} {
		if value, ok := parseNonNegativeBillingDecimal(raw); ok {
			values[key] = value.String()
			changed = true
		}
	}
	for key, value := range map[string]string{
		"actual_channel_id":     settlement.ActualChannelID,
		"actual_channel_name":   settlement.ActualChannelName,
		"actual_provider":       settlement.ActualProvider,
		"actual_protocol":       settlement.ActualProtocol,
		"actual_upstream_model": settlement.ActualUpstreamModel,
	} {
		if value = strings.TrimSpace(value); value != "" {
			values[key] = value
			changed = true
		}
	}
	if changed {
		bc.PricingSnapshot = buildPricingSnapshot(values)
	}
	applyRouteAttributionSnapshot(bc, "")

	// Mirror Console's recorded-rate reader (ledger_currency.go): cny_per_usd is
	// the canonical USD->CNY rate; the local currency segment written by an
	// earlier mirror is next. The legacy metered CNY form records the rate in
	// exchange_rate, but a USD snapshot stores the identity rate there, so
	// exchange_rate is only read when the source currency is CNY. Never fall
	// back to a default.
	rate, ok := parsePositiveBillingDecimal(settlement.CNYPerUSD)
	if !ok {
		rate, ok = parsePositiveBillingDecimal(pricingSnapshotString(values, exchangeRateSnapshotRateKey))
	}
	if !ok && strings.EqualFold(pricingSnapshotString(values, consoleSettlementCurrencyKey), billingDisplayCurrencyCNY) {
		rate, ok = parsePositiveBillingDecimal(pricingSnapshotString(values, consoleSettlementExchangeRateKey))
	}
	if !ok {
		return fmt.Errorf("%w: console settlement recorded no USD to CNY rate", ErrExchangeRateUnavailable)
	}
	return mergeExchangeRateSnapshot(bc, rate, exchangeRateSourceConsoleSystem, time.Now().UTC())
}

// settlementOverageStrategyChargeDifference mirrors Console's
// ReservationOverageStrategyChargeDifference: an actual above the reservation
// is charged in full and the balance may go negative.
const settlementOverageStrategyChargeDifference = "charge_difference"

// consoleSettlementAmountsValid accepts a Console settlement that Pre can
// mirror. Console charges the difference when the actual exceeds the
// reservation, so an overage is valid as long as nothing was refunded; within
// the reservation, settled+refunded must still fit it. A reservation Console
// did not report (zero) leaves only the sign check.
func consoleSettlementAmountsValid(reserved, settled, refunded int64) bool {
	if settled < 0 || refunded < 0 {
		return false
	}
	if reserved <= 0 {
		return true
	}
	if settled > reserved {
		return refunded == 0
	}
	return settled+refunded <= reserved
}

// recordSettlementOverageSnapshot records an actual above the reservation in
// the settlement snapshot, with the key names Console's settle transaction
// metadata uses, so an overage is auditable from the usage bill alone.
func recordSettlementOverageSnapshot(bc *BillingContext, reserved, settled int64) {
	if bc == nil || reserved <= 0 || settled <= reserved {
		return
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	values["overage_credits"] = settled - reserved
	values["overage_strategy"] = settlementOverageStrategyChargeDifference
	bc.PricingSnapshot = buildPricingSnapshot(values)
}

func validatePlatformProxySettlement(bc *BillingContext, settlement *adapter.SettlementResult) error {
	if bc == nil {
		return fmt.Errorf("billing context is nil")
	}
	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil {
		return err
	}
	if usageLane != UsageBillingLanePlatform {
		return fmt.Errorf("proxy settlement requires platform lane, got %s (request_id=%s)", usageLane, bc.RequestID)
	}
	if strings.TrimSpace(bc.DeductionID) == "" {
		return fmt.Errorf("missing deduction_id for proxy settlement (request_id=%s)", bc.RequestID)
	}
	if strings.TrimSpace(bc.AttemptID) == "" {
		return fmt.Errorf("missing attempt_id for proxy settlement (request_id=%s)", bc.RequestID)
	}
	if settlement == nil {
		return fmt.Errorf("missing console proxy settlement result (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	if strings.TrimSpace(settlement.SettlementID) == "" {
		return fmt.Errorf("missing console proxy settlement_id (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	if strings.TrimSpace(settlement.SettlementID) != strings.TrimSpace(bc.DeductionID) {
		return fmt.Errorf("console proxy settlement_id mismatch (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	if !strings.EqualFold(settlement.Status, "settled") && !strings.EqualFold(settlement.Status, "success") {
		return fmt.Errorf("console proxy settlement status is not settled (request_id=%s attempt_id=%s status=%s)", bc.RequestID, bc.AttemptID, settlement.Status)
	}
	if billingContextStatusIsSuccess(bc.Status) && settlement.OfficialPoints <= 0 {
		return fmt.Errorf("console proxy settlement returned no official_points (request_id=%s attempt_id=%s)", bc.RequestID, bc.AttemptID)
	}
	return nil
}
