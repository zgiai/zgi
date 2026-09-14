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
) error {
	if settlement == nil {
		if bc == nil || strings.TrimSpace(bc.OrganizationID) == "" || strings.TrimSpace(bc.AttemptID) == "" {
			return fmt.Errorf("missing billing identity for console settlement query")
		}
		var result *settlementQueryResponse
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			result, err = queryConsoleSettlement(ctx, s.consoleAPIBaseURL+"/billing/settlements/by-attempt", bc.OrganizationID, bc.AttemptID)
			if err == nil && result.Found && strings.EqualFold(result.SettlementStatus, "SETTLED") {
				break
			}
			if attempt < 2 {
				delay := time.Duration(100*(1<<attempt)) * time.Millisecond
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
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
		}
		if !strings.EqualFold(result.SettlementStatus, "SETTLED") {
			return fmt.Errorf("console settlement is not terminal: status=%s retryable=%t", result.SettlementStatus, result.Retryable)
		}
		if result.SettledCredits < 0 || result.RefundedCredits < 0 || (result.ReservedCredits > 0 && result.SettledCredits+result.RefundedCredits > result.ReservedCredits) {
			return fmt.Errorf("invalid console settlement amounts: reserved=%d settled=%d refunded=%d", result.ReservedCredits, result.SettledCredits, result.RefundedCredits)
		}
		settlement = &adapter.SettlementResult{SettlementID: result.SettlementID, OfficialPoints: result.SettledCredits, Status: result.SettlementStatus}
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
	applyPlatformSettlementCostSnapshot(bc, settlement)
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
	bc.Status = "success"
	bc.SettledAt = time.Now().UTC()
	applyPlatformSettlementCostSnapshot(bc, settlement)
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

func applyPlatformSettlementCostSnapshot(bc *BillingContext, settlement *adapter.SettlementResult) {
	if bc == nil || settlement == nil {
		return
	}
	values := map[string]interface{}{}
	if len(bc.PricingSnapshot) > 0 && string(bc.PricingSnapshot) != "null" {
		_ = json.Unmarshal(bc.PricingSnapshot, &values)
	}
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
	if value, ok := parsePositiveBillingDecimal(settlement.CNYPerUSD); ok {
		values["cny_per_usd"] = value.String()
		values["exchange_rate_source"] = "console_settlement"
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
