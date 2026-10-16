package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func toolProjectionFromQuery(result *settlementQueryResponse) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if len(result.PricingSnapshot) > 0 {
		if err := json.Unmarshal(result.PricingSnapshot, &root); err != nil {
			return nil, err
		}
	}
	projection := result.ToolSettlement
	if len(projection) == 0 || string(projection) == "null" {
		projection = root["tool_settlement"]
	}
	if len(projection) > 0 && len(root["tool_settlement"]) > 0 {
		if !toolJSONEqual(projection, root["tool_settlement"]) {
			return nil, fmt.Errorf("conflicting authoritative tool projections")
		}
	}
	return projection, nil
}

// mirrorPlatformTools never invokes a billing RPC, a tool observer or a fund
// deduction. Only the local quota projection advances by the authoritative delta.
func (s *RemoteBilling) mirrorPlatformTools(ctx context.Context, bc *BillingContext, raw json.RawMessage, snapshot datatypes.JSON) error {
	var state toolSettlement
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode authoritative tool settlement: %w", err)
	}
	if err := validateToolIdentity(&state, bc, "platform"); err != nil {
		return err
	}
	if state.Revision < 1 || state.TokenCredits < 0 || state.ToolCredits < 0 || state.UnsettledCredits < 0 || state.TotalCredits < state.TokenCredits || state.TotalCredits-state.TokenCredits != state.ToolCredits {
		return fmt.Errorf("invalid authoritative tool amounts or revision")
	}
	if bc.DeductionID != "" && bc.DeductionID != state.SettlementID {
		return fmt.Errorf("authoritative tool settlement id mismatch")
	}
	switch state.UsageState {
	case "pending", "settled", "free", "unsettled":
	default:
		return fmt.Errorf("invalid authoritative tool usage state")
	}
	totalUSD, err := decimal.NewFromString(state.TotalUSD)
	if err != nil || totalUSD.IsNegative() {
		return fmt.Errorf("invalid authoritative total USD")
	}
	tokenUSD, tokenErr := decimal.NewFromString(state.TokenUSD)
	toolUSD, toolErr := decimal.NewFromString(state.ToolUSD)
	if tokenErr != nil || toolErr != nil || tokenUSD.IsNegative() || toolUSD.IsNegative() || !tokenUSD.Add(toolUSD).Equal(totalUSD) {
		return fmt.Errorf("inconsistent authoritative USD components")
	}
	incomingUsage := &adapter.SettlementResult{PromptTokens: bc.PromptTokens, CompletionTokens: bc.CompletionTokens, TotalTokens: bc.TotalTokens, CacheReadTokens: bc.CacheReadTokens, CacheWriteTokens: bc.CacheWriteTokens, CacheWrite5mTokens: bc.CacheWrite5mTokens, CacheWrite1hTokens: bc.CacheWrite1hTokens}
	return s.localService.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Lane != billingAttemptLaneRemote || attempt.OrganizationID.String() != bc.OrganizationID || attempt.RequestID != bc.RequestID {
			return fmt.Errorf("platform mirror identity mismatch")
		}
		if attempt.ConsoleSettlementID != nil && *attempt.ConsoleSettlementID != "" && *attempt.ConsoleSettlementID != state.SettlementID {
			return fmt.Errorf("platform settlement id changed")
		}
		var previous UsageBill
		findErr := tx.Where("attempt_id = ?", bc.AttemptID).First(&previous).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		oldCredits := int64(0)
		if findErr == nil {
			if previous.OrganizationID != bc.OrganizationID || previous.BillingLane != UsageBillingLanePlatform {
				return fmt.Errorf("platform bill identity mismatch")
			}
			old, err := decodeToolSettlement(previous.PricingSnapshot)
			if err != nil {
				return err
			}
			if old != nil {
				if state.Revision < old.Revision {
					return nil
				}
				if state.Revision == old.Revision {
					var oldRoot map[string]json.RawMessage
					_ = json.Unmarshal(previous.PricingSnapshot, &oldRoot)
					if !toolJSONEqual(raw, oldRoot["tool_settlement"]) {
						return fmt.Errorf("conflicting authoritative settlement revision")
					}
					if billingAttemptStatusIsFinalized(attempt.Status) {
						bc.ActualCredits = old.TotalCredits
						return nil
					}
				}
				if state.TokenCredits != old.TokenCredits {
					return fmt.Errorf("authoritative token amount changed")
				}
			}
			if billingAttemptStatusIsFinalized(attempt.Status) {
				oldCredits = previous.TotalPoints.Int64()
			}
			if bc.ModelID == previous.ModelID {
				restoreToolBillContext(bc, &previous, &attempt)
				applyPlatformSettlementTokenUsage(bc, incomingUsage)
			}
		}
		delta := state.TotalCredits - oldCredits
		if delta < 0 {
			return fmt.Errorf("authoritative refund requires compensation")
		}
		if len(snapshot) > 0 && string(snapshot) != "null" {
			bc.PricingSnapshot = append(datatypes.JSON(nil), snapshot...)
		}
		if bc.ModelID == [16]byte{} && attempt.ModelID != nil {
			bc.ModelID = *attempt.ModelID
		}
		if bc.ProviderID == [16]byte{} && attempt.ProviderID != nil {
			bc.ProviderID = *attempt.ProviderID
		}
		bc.QuotaSubjectType, bc.QuotaSubjectID = attempt.QuotaSubjectType, attempt.QuotaSubjectID
		restoreBillingContextAttribution(bc, &attempt)
		bc.BillingLane, bc.UseSystemProvider = UsageBillingLanePlatform, true
		bc.DeductionID, bc.Status, bc.ActualCredits, bc.TotalUSD = state.SettlementID, "success", state.TotalCredits, totalUSD
		bc.SettledAt = time.Now().UTC()
		if bc.RequestCreatedAt.IsZero() {
			bc.RequestCreatedAt = attempt.CreatedAt
		}
		if err := applyPlatformSettlementCostSnapshot(bc, settlementFromToolSnapshot(&state, bc.PricingSnapshot)); err != nil {
			return err
		}
		// Preserve the raw JSON, including fields unknown to this Pre version.
		values := pricingSnapshotMap(bc.PricingSnapshot)
		values["tool_settlement"] = json.RawMessage(raw)
		bc.PricingSnapshot = buildPricingSnapshot(values)
		subject := *bc
		subject.ActualCredits = delta
		if billingAttemptStatusIsFinalized(attempt.Status) {
			subject.EstimatedCredits, subject.SubjectReservedCredits = 0, 0
		}
		if err := s.localService.settleSubjectQuota(ctx, tx, &subject); err != nil {
			return err
		}
		if err := accumulateToolSubjectCharge(ctx, tx, bc, &subject, billingAttemptStatusIsFinalized(attempt.Status)); err != nil {
			return err
		}
		if err := s.localService.updateAttemptEntriesAfterSettle(ctx, tx, bc, billingLedgerTypeOrgFunds, bc.OrganizationID); err != nil {
			return err
		}
		if err := tx.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"status": billingAttemptStatusSettled, "settlement_state": billingAttemptStatusSettled, "console_settlement_id": state.SettlementID, "console_settlement_status": billingAttemptStatusSettled, "settlement_version": state.Revision, "next_reconcile_at": nil, "settlement_next_retry_at": nil, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return s.localService.upsertUsageBill(ctx, tx, bc, usageBillStatusSuccess, nil, nil)
	})
}

func settlementFromToolSnapshot(state *toolSettlement, raw datatypes.JSON) *adapter.SettlementResult {
	values := pricingSnapshotMap(raw)
	rate := firstNonEmptyString(pricingSnapshotString(values, "cny_per_usd"), pricingSnapshotString(values, exchangeRateSnapshotRateKey))
	for _, capture := range state.Prices {
		if value, err := decimal.NewFromString(capture.CNYPerUSD); err == nil && value.IsPositive() {
			rate = capture.CNYPerUSD
			break
		}
	}
	return &adapter.SettlementResult{SettlementID: state.SettlementID, Status: "settled", OfficialPoints: state.TotalCredits, TotalCostUSD: state.TotalUSD, CNYPerUSD: rate}
}

func toolJSONEqual(a, b []byte) bool {
	var left, right interface{}
	d := json.NewDecoder(bytes.NewReader(a))
	d.UseNumber()
	if err := d.Decode(&left); err != nil {
		return false
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&right) != nil {
		return false
	}
	left, leftOK := normalizeToolJSONNumbers(left)
	right, rightOK := normalizeToolJSONNumbers(right)
	return leftOK && rightOK && reflect.DeepEqual(left, right)
}

// PostgreSQL JSONB rewrites numeric notation. Compare exact decimal values,
// preserving strings, unknown fields and integers beyond float64 precision.
type toolJSONNumber struct {
	Coefficient string
	Exponent    int64
}

func normalizeToolJSONNumbers(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case json.Number:
		number, err := decimal.NewFromString(string(v))
		if err != nil {
			return nil, false
		}
		coefficient := number.Coefficient().String()
		if coefficient == "0" {
			return toolJSONNumber{Coefficient: "0"}, true
		}
		trimmed := strings.TrimRight(coefficient, "0")
		return toolJSONNumber{Coefficient: trimmed, Exponent: int64(number.Exponent()) + int64(len(coefficient)-len(trimmed))}, true
	case map[string]interface{}:
		for key, item := range v {
			normalized, ok := normalizeToolJSONNumbers(item)
			if !ok {
				return nil, false
			}
			v[key] = normalized
		}
	case []interface{}:
		for i, item := range v {
			normalized, ok := normalizeToolJSONNumbers(item)
			if !ok {
				return nil, false
			}
			v[i] = normalized
		}
	}
	return value, true
}

func markToolPostpaid(bc *BillingContext) {
	values := pricingSnapshotMap(bc.PricingSnapshot)
	values["tool_postpaid"] = true
	values["tool_protocol_version"] = 2
	bc.PricingSnapshot = buildPricingSnapshot(values)
}

func clearToolPostpaid(bc *BillingContext) {
	values := pricingSnapshotMap(bc.PricingSnapshot)
	delete(values, "tool_postpaid")
	delete(values, "tool_protocol_version")
	delete(values, "tool_settlement")
	bc.PricingSnapshot = buildPricingSnapshot(values)
	bc.ToolPostpaid = false
}

func isToolPostpaidSnapshot(raw datatypes.JSON) bool {
	var root struct {
		Postpaid bool            `json:"tool_postpaid"`
		Tool     json.RawMessage `json:"tool_settlement"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return false
	}
	return root.Postpaid || len(root.Tool) > 0 && string(root.Tool) != "null"
}

func (s *RemoteBilling) reconcilePlatformToolAttempt(ctx context.Context, attemptID string) error {
	var attempt BillingAttempt
	if err := s.localService.db.WithContext(ctx).Where("attempt_id = ? AND lane = ?", attemptID, billingAttemptLaneRemote).First(&attempt).Error; err != nil {
		return err
	}
	result, err := queryConsoleSettlement(ctx, strings.TrimRight(s.consoleAPIBaseURL, "/")+"/billing/settlements/by-attempt", attempt.OrganizationID.String(), attempt.AttemptID)
	if err != nil {
		return err
	}
	projection, err := toolProjectionFromQuery(result)
	if err != nil {
		return err
	}
	if !result.Found || len(projection) == 0 || string(projection) == "null" {
		return fmt.Errorf("audited tool mirror repair has no authoritative tool result")
	}
	return s.mirrorQueriedSettlement(ctx, &attempt, result)
}

// Preserve the attempt for an authoritative query without treating a completed
// inference's finance failure as a new provider invocation or a zero release.
func (s *llmGatewayServiceImpl) queuePlatformSettlementRecovery(ctx context.Context, bc *BillingContext, cause error) error {
	recoveryCtx, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if bc.ToolPostpaid {
		markToolPostpaid(bc)
	}
	remote, ok := s.billing.(*RemoteBilling)
	if !ok {
		return wrapBillingSettleError(cause, bc, true, routeIDString(bc.ChannelID))
	}
	if err := remote.markAttemptSettleFailed(recoveryCtx, bc, "PLATFORM_SETTLEMENT_PENDING", cause.Error()); err != nil {
		return wrapBillingSettleError(fmt.Errorf("queue authoritative settlement recovery: %w", err), bc, true, routeIDString(bc.ChannelID))
	}
	return wrapBillingSettleError(cause, bc, true, routeIDString(bc.ChannelID))
}
