package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/datatypes"
)

// mirrorReleasedToolHold closes only an authoritative, unquoted hold.
// The original tool JSON remains audit evidence; it is not a successful use.
func (s *RemoteBilling) mirrorReleasedToolHold(ctx context.Context, attempt *BillingAttempt, result *settlementQueryResponse, raw json.RawMessage) error {
	if !result.Found || !strings.EqualFold(result.SettlementStatus, billingAttemptStatusRolledBack) {
		return fmt.Errorf("tool hold release requires authoritative rollback")
	}
	if result.Retryable || result.SettledCredits != 0 || result.ReservedCredits < 0 || result.RefundedCredits != result.ReservedCredits {
		return fmt.Errorf("tool hold release is not a terminal full refund")
	}
	var state toolSettlement
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode released tool hold: %w", err)
	}
	bc := &BillingContext{OrganizationID: attempt.OrganizationID.String(), RequestID: attempt.RequestID, AttemptID: attempt.AttemptID, DeductionID: result.SettlementID, BillingLane: UsageBillingLanePlatform, UseSystemProvider: true, ToolPostpaid: true}
	if err := validateToolIdentity(&state, bc, "platform"); err != nil {
		return err
	}
	var audit map[string]json.RawMessage
	if err := json.Unmarshal(raw, &audit); err != nil {
		return err
	}
	quote := strings.TrimSpace(string(audit["token_quote"]))
	if state.Revision != 0 || state.TokenCredits != 0 || state.ToolCredits != 0 || state.TotalCredits != 0 || state.UnsettledCredits != 0 || state.TokenFinalized || (quote != "" && quote != "null") {
		return fmt.Errorf("cannot release a quoted or completed tool settlement")
	}
	for _, item := range state.LineItems {
		if item.Quantity > 0 {
			return fmt.Errorf("cannot release confirmed tool usage")
		}
	}
	if attempt.Status == billingAttemptStatusSettled {
		return fmt.Errorf("cannot release a settled platform mirror")
	}
	if attempt.ConsoleSettlementID != nil && *attempt.ConsoleSettlementID != "" && *attempt.ConsoleSettlementID != result.SettlementID {
		return fmt.Errorf("released tool settlement identity changed")
	}
	var entries []BillingAttemptEntry
	if err := s.localService.db.WithContext(ctx).Where("attempt_id = ?", attempt.AttemptID).Find(&entries).Error; err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ActualAmount != 0 {
			return fmt.Errorf("cannot release a locally charged tool hold")
		}
		if entry.EntryType == billingEntryTypeFund && entry.IdempotencyKey != nil && *entry.IdempotencyKey != "" && *entry.IdempotencyKey != result.SettlementID {
			return fmt.Errorf("released deduction id mismatch")
		}
		if entry.EntryType == billingEntryTypeSubject {
			bc.SubjectReservedCredits = entry.ReservedAmount.Int64()
		}
		if entry.EntryType == billingEntryTypeFund {
			bc.EstimatedCredits = entry.ReservedAmount.Int64()
		}
	}
	bc.QuotaSubjectType, bc.QuotaSubjectID = attempt.QuotaSubjectType, attempt.QuotaSubjectID
	if attempt.ModelID != nil {
		bc.ModelID = *attempt.ModelID
	}
	if attempt.ProviderID != nil {
		bc.ProviderID = *attempt.ProviderID
	}
	if err := s.restorePartialUsageBillContext(ctx, bc); err != nil {
		return err
	}
	if bc.OrganizationID != attempt.OrganizationID.String() || bc.RequestID != attempt.RequestID || bc.DeductionID != result.SettlementID {
		return fmt.Errorf("released tool bill identity mismatch")
	}
	restoreBillingContextAttribution(bc, attempt)
	if len(result.PricingSnapshot) > 0 && string(result.PricingSnapshot) != "null" {
		bc.PricingSnapshot = datatypes.JSON(result.PricingSnapshot)
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	values["tool_settlement"] = json.RawMessage(raw)
	bc.PricingSnapshot = buildPricingSnapshot(values)
	bc.Status, bc.ActualCredits = "error", 0
	bc.ErrorMessage = "Console released an unquoted tool hold"
	bc.UseSystemProvider, bc.BillingLane, bc.ToolPostpaid = true, UsageBillingLanePlatform, true
	return s.finalizeRemoteSettlement(ctx, bc)
}
