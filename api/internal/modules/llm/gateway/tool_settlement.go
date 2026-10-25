package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// These persistence projections implement the Console-owned settlement v1.
// They do not replace the shared pricing/event contracts or transport schema.
type toolSettlement struct {
	SchemaVersion       int                         `json:"schema_version"`
	BillingLane         string                      `json:"billing_lane"`
	AttemptID           string                      `json:"attempt_id"`
	RequestID           string                      `json:"request_id"`
	SettlementID        string                      `json:"settlement_id"`
	UsageState          string                      `json:"usage_state"`
	FreeReason          string                      `json:"free_reason"`
	PricingVersion      string                      `json:"pricing_version"`
	EventIDs            []string                    `json:"event_ids"`
	Revision            int64                       `json:"revision"`
	TokenCredits        int64                       `json:"token_credits"`
	ToolCredits         int64                       `json:"tool_credits"`
	TotalCredits        int64                       `json:"total_credits"`
	UnsettledCredits    int64                       `json:"unsettled_credits"`
	TokenUSD            string                      `json:"token_usd"`
	ToolUSD             string                      `json:"tool_usd"`
	TotalUSD            string                      `json:"total_usd"`
	Prices              map[string]toolPriceCapture `json:"prices"`
	LineItems           []toolSettlementLine        `json:"line_items"`
	Adjustments         []toolSettlementAdjustment  `json:"adjustments"`
	Requested           []string                    `json:"requested,omitempty"`
	Reused              map[string]string           `json:"reused,omitempty"`
	TokenFinalized      bool                        `json:"token_finalized,omitempty"`
	InvocationSucceeded bool                        `json:"invocation_succeeded,omitempty"`
}

type toolPriceCapture struct {
	PricingVersion string                       `json:"pricing_version"`
	PricingJSON    json.RawMessage              `json:"pricing_json"`
	CNYPerUSD      string                       `json:"cny_per_usd"`
	PointsPerUSD   int64                        `json:"points_per_usd"`
	Mode           string                       `json:"mode"`
	CapturedAt     time.Time                    `json:"captured_at"`
	Resolved       map[string]toolResolvedPrice `json:"resolved"`
}

type toolResolvedPrice struct {
	modelpricing.ToolPrice
	Source     string `json:"source"`
	FreeReason string `json:"free_reason,omitempty"`
}

type toolSettlementLine struct {
	EventID        string `json:"event_id"`
	Tool           string `json:"tool"`
	Quantity       int64  `json:"quantity"`
	Unit           string `json:"unit"`
	Amount         string `json:"amount"`
	PerQuantity    int64  `json:"per_quantity"`
	Currency       string `json:"currency"`
	Source         string `json:"source"`
	PricingVersion string `json:"pricing_version"`
	CostUSD        string `json:"cost_usd"`
	UsageState     string `json:"usage_state"`
	FreeReason     string `json:"free_reason"`
}

type toolSettlementAdjustment struct {
	Revision      int64     `json:"revision"`
	BeforeCredits int64     `json:"previous_credits"`
	AfterCredits  int64     `json:"total_credits"`
	DeltaCredits  int64     `json:"delta_credits"`
	EventIDs      []string  `json:"event_ids"`
	At            time.Time `json:"at"`
}

type requestedToolsKey struct{}

func withRequestedBillingTools(ctx context.Context, body []byte) context.Context {
	var payload struct {
		Tools []map[string]json.RawMessage `json:"tools"`
	}
	_ = json.Unmarshal(body, &payload)
	set := map[string]bool{}
	for _, t := range payload.Tools {
		var name string
		_ = json.Unmarshal(t["type"], &name)
		for key := range t {
			switch key {
			case "googleSearch", "google_search":
				name = toolusage.ToolWebSearch
			case "fileSearch", "file_search":
				name = toolusage.ToolFileSearch
			case "codeExecution", "code_execution":
				name = toolusage.ToolCodeInterpreter
			}
		}
		if strings.HasPrefix(name, "web_search") {
			name = toolusage.ToolWebSearch
		}
		if strings.HasPrefix(name, "code_execution") {
			name = toolusage.ToolCodeInterpreter
		}
		if name == toolusage.ToolWebSearch || name == toolusage.ToolFileSearch || name == toolusage.ToolCodeInterpreter {
			set[name] = true
		}
	}
	var names []string
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return context.WithValue(ctx, requestedToolsKey{}, names)
}

func decodeToolSettlement(raw datatypes.JSON) (*toolSettlement, error) {
	var root map[string]json.RawMessage
	if len(raw) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	v := root["tool_settlement"]
	if len(v) == 0 || string(v) == "null" {
		return nil, nil
	}
	var state toolSettlement
	if err := json.Unmarshal(v, &state); err != nil {
		return nil, err
	}
	if state.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported tool settlement version")
	}
	return &state, nil
}

func putToolSettlement(bc *BillingContext, state *toolSettlement) {
	bc.ToolSettlement = state
	values := pricingSnapshotMap(bc.PricingSnapshot)
	values["tool_settlement"] = state
	bc.PricingSnapshot = buildPricingSnapshot(values)
}

func toolChargeMode(channel string) string {
	if os.Getenv("LLM_TOOL_BILLING_MODE") == "charge" {
		for _, allowed := range strings.Split(os.Getenv("LLM_TOOL_BILLING_CHANNELS"), ",") {
			if channel != "" && strings.TrimSpace(allowed) == channel {
				return "charge"
			}
		}
	}
	return "observe"
}

func captureToolPrices(raw json.RawMessage, channel, rate string, points int64, mode string) toolPriceCapture {
	if !json.Valid(raw) {
		raw = json.RawMessage(`{}`)
	}
	capture := toolPriceCapture{PricingJSON: append(json.RawMessage(nil), raw...), CNYPerUSD: rate, PointsPerUSD: points, Mode: mode, CapturedAt: time.Now().UTC(), Resolved: map[string]toolResolvedPrice{}}
	for _, name := range []string{toolusage.ToolWebSearch, toolusage.ToolFileSearch, toolusage.ToolCodeInterpreter} {
		resolved, err := modelpricing.ResolveToolPrice(raw, "private", channel, name)
		price := toolResolvedPrice{ToolPrice: resolved.Price, Source: resolved.Source}
		if err != nil {
			price.FreeReason = "invalid_price"
		} else if resolved.Source == "free" {
			price.FreeReason = "missing_price"
		}
		capture.PricingVersion = resolved.PricingVersion
		capture.Resolved[name] = price
	}
	return capture
}

func (s *llmGatewayServiceImpl) prepareToolBilling(ctx context.Context, bc *BillingContext, selection *ProviderSelection) error {
	requested, _ := ctx.Value(requestedToolsKey{}).([]string)
	if len(requested) == 0 {
		return nil
	}
	if bc.UseSystemProvider {
		capabilities, ok := s.billing.(interface {
			SupportsToolPostpaid(context.Context) (bool, error)
		})
		if !ok {
			return nil
		}
		supported, err := capabilities.SupportsToolPostpaid(ctx)
		if err != nil {
			return fmt.Errorf("negotiate tool settlement capability: %w", err)
		}
		if !supported {
			return nil
		}
		bc.ToolPostpaid = true
		markToolPostpaid(bc)
		return nil
	}
	bc.ToolPostpaid = true
	if s.db == nil || bc.ChannelID == nil {
		return fmt.Errorf("missing private tool billing database or channel")
	}
	if err := captureOrganizationBillingCurrency(ctx, s.db, bc); err != nil {
		return err
	}
	engine := NewPricingEngine(s.db).(*pricingEngine)
	ref := pricingModelRefFromSelection(selection)
	organizationID, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid tool pricing organization: %w", err)
	}
	if ref.OrganizationID != uuid.Nil && ref.OrganizationID != organizationID {
		return fmt.Errorf("tool pricing organization mismatch")
	}
	if ref.ModelID != bc.ModelID {
		return fmt.Errorf("tool pricing model mismatch")
	}
	ref.OrganizationID = organizationID
	record, found, err := engine.loadModel(ctx, ref)
	if err != nil {
		return fmt.Errorf("load tool pricing: %w", err)
	}
	var raw json.RawMessage = json.RawMessage(`{}`)
	if found {
		raw = json.RawMessage(record.Pricing)
	}
	channel := bc.ChannelID.String()
	capture := captureToolPrices(raw, channel, bc.USDToCNYRate.String(), NormalizedPointsPerUSD(), toolChargeMode(channel))
	state := &toolSettlement{SchemaVersion: 1, BillingLane: "private", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.AttemptID, UsageState: "pending", Requested: requested, PricingVersion: capture.PricingVersion, Prices: map[string]toolPriceCapture{channel: capture}, LineItems: []toolSettlementLine{}, EventIDs: []string{}, Adjustments: []toolSettlementAdjustment{}, Reused: map[string]string{}}
	putToolSettlement(bc, state)
	return nil
}

func validateToolIdentity(state *toolSettlement, bc *BillingContext, lane string) error {
	if state == nil || state.SchemaVersion != 1 || state.BillingLane != lane || state.AttemptID != bc.AttemptID || state.RequestID != bc.RequestID || state.SettlementID == "" {
		return fmt.Errorf("tool settlement identity mismatch")
	}
	return nil
}

// checkToolDebt runs under the channel wallet lock acquired by pre-deduction.
// A historical token-only overdraft does not become a tool debt restriction.
func checkToolDebt(ctx context.Context, tx *gorm.DB, bc *BillingContext) error {
	if bc.ToolSettlement == nil {
		return nil
	}
	positive := false
	for _, capture := range bc.ToolSettlement.Prices {
		rate, rateErr := decimal.NewFromString(capture.CNYPerUSD)
		if capture.Mode != "charge" || capture.PointsPerUSD <= 0 || rateErr != nil || !rate.IsPositive() {
			continue
		}
		for _, name := range bc.ToolSettlement.Requested {
			p := capture.Resolved[name]
			amount, err := decimal.NewFromString(p.Amount)
			if err == nil && amount.IsPositive() && p.FreeReason == "" {
				positive = true
			}
		}
	}
	if !positive {
		return nil
	}
	var bills []UsageBill
	if err := tx.WithContext(ctx).Where("organization_id = ? AND billing_lane = ? AND CAST(pricing_snapshot AS TEXT) LIKE ?", bc.OrganizationID, "private", "%tool_settlement%").Find(&bills).Error; err != nil {
		return err
	}
	for _, bill := range bills {
		state, err := decodeToolSettlement(bill.PricingSnapshot)
		if err != nil {
			return err
		}
		if state == nil || state.UnsettledCredits <= 0 || bill.ChannelID == nil {
			continue
		}
		var wallet ChannelWallet
		if err := tx.WithContext(ctx).Where("organization_id = ? AND channel_id = ?", bc.OrganizationID, *bill.ChannelID).First(&wallet).Error; err != nil {
			return err
		}
		if wallet.Balance < 0 {
			return ErrInsufficientBalance
		}
	}
	return nil
}

func calculateToolSettlement(state *toolSettlement, events []toolusage.Event, failed bool) (*toolSettlement, error) {
	encoded, _ := json.Marshal(state)
	var result toolSettlement
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	result.LineItems = []toolSettlementLine{}
	result.EventIDs = []string{}
	result.UsageState, result.FreeReason = "free", "missing_usage"
	totalUSD, creditsExact := decimal.Zero, decimal.Zero
	seenTools := map[string]bool{}
	for _, event := range events {
		seenTools[event.Tool] = true
		channel := ""
		if event.ChannelID != nil {
			channel = event.ChannelID.String()
		}
		capture, captured := state.Prices[channel]
		p := capture.Resolved[event.Tool]
		line := toolSettlementLine{EventID: event.EventID, Tool: event.Tool, Quantity: event.Quantity, Unit: event.Unit, Amount: p.Amount, PerQuantity: p.PerQuantity, Currency: p.Currency, Source: p.Source, PricingVersion: capture.PricingVersion, CostUSD: "0", UsageState: "free"}
		switch {
		case event.Status == toolusage.StatusPending || event.Status == toolusage.StatusObserved:
			line.UsageState, line.FreeReason = "pending", "missing_usage"
		case event.Status == toolusage.StatusFailed || event.UsageSource == toolusage.UsageSourceMissing:
			var detail struct {
				FreeReason string `json:"free_reason"`
			}
			_ = json.Unmarshal(event.Details, &detail)
			line.FreeReason = "upstream_failed"
			if detail.FreeReason == "session_reused" {
				line.FreeReason = "session_reused"
			} else if detail.FreeReason == "usage_timeout" {
				line.FreeReason = "usage_timeout"
			} else if event.UsageSource == toolusage.UsageSourceMissing {
				line.FreeReason = "missing_usage"
			}
		case event.Quantity == 0:
			line.FreeReason = "zero_usage"
		case !captured:
			line.FreeReason = "missing_price"
		case p.FreeReason != "":
			line.FreeReason = p.FreeReason
		default:
			amount, err := decimal.NewFromString(p.Amount)
			rate, rateErr := decimal.NewFromString(capture.CNYPerUSD)
			switch {
			case err != nil || amount.IsNegative() || p.PerQuantity <= 0 || p.Unit != event.Unit:
				line.FreeReason = "invalid_price"
			case amount.IsZero():
				line.FreeReason = "zero_price"
			case capture.PointsPerUSD <= 0 || rateErr != nil || !rate.IsPositive():
				line.FreeReason = "missing_conversion"
			case p.Currency != "USD" && p.Currency != "CNY":
				line.FreeReason = "invalid_price"
			case capture.Mode != "charge":
				line.FreeReason = "observation_mode"
			default:
				cost := amount.Mul(decimal.NewFromInt(event.Quantity))
				if p.PerQuantity == 1000 {
					cost = cost.Shift(-3)
				} else if p.PerQuantity != 1 {
					return nil, fmt.Errorf("invalid tool price quantity")
				}
				if p.Currency == "CNY" {
					cost = cost.DivRound(rate, 36)
				}
				line.CostUSD, line.UsageState = cost.String(), "settled"
				totalUSD = totalUSD.Add(cost)
				creditsExact = creditsExact.Add(cost.Mul(decimal.NewFromInt(capture.PointsPerUSD)).Mul(decimal.NewFromInt(millipoints.UnitsPerPoint)))
			}
		}
		result.LineItems = append(result.LineItems, line)
		result.EventIDs = append(result.EventIDs, event.EventID)
	}
	for _, name := range state.Requested {
		if seenTools[name] {
			continue
		}
		line := toolSettlementLine{Tool: name, CostUSD: "0", UsageState: "pending", FreeReason: "missing_usage"}
		if failed {
			line.UsageState, line.FreeReason = "free", "upstream_failed"
		} else {
			wait := 300
			if seconds, err := strconv.Atoi(os.Getenv("LLM_TOOL_USAGE_WAIT_SECONDS")); err == nil && seconds > 0 && seconds <= 86400 {
				wait = seconds
			}
			for _, capture := range state.Prices {
				if !capture.CapturedAt.IsZero() && time.Since(capture.CapturedAt) >= time.Duration(wait)*time.Second {
					line.UsageState, line.FreeReason = "free", "usage_timeout"
					break
				}
			}
		}
		if id := state.Reused[name]; id != "" {
			line.EventID, line.UsageState, line.FreeReason = id, "free", "session_reused"
		}
		result.LineItems = append(result.LineItems, line)
	}
	result.FreeReason = ""
	for _, line := range result.LineItems {
		if line.UsageState == "pending" {
			result.UsageState = "pending"
		}
		if result.FreeReason == "" && line.FreeReason != "" {
			result.FreeReason = line.FreeReason
		}
	}
	rounded := creditsExact.Ceil()
	if rounded.GreaterThan(decimal.NewFromInt(math.MaxInt64-state.TokenCredits)) || state.TokenCredits < 0 {
		return nil, fmt.Errorf("tool settlement credits overflow")
	}
	result.ToolCredits = rounded.IntPart()
	result.TotalCredits = result.TokenCredits + result.ToolCredits
	if result.UsageState != "pending" && result.ToolCredits > 0 {
		result.UsageState, result.FreeReason = "settled", ""
	}
	result.ToolUSD = totalUSD.String()
	tokenUSD, err := decimal.NewFromString(state.TokenUSD)
	if err != nil || tokenUSD.IsNegative() {
		return nil, fmt.Errorf("invalid frozen token USD")
	}
	result.TotalUSD = tokenUSD.Add(totalUSD).String()
	return &result, nil
}

// settlePrivateTools owns the whole private request transaction. Revisions use
// only a tool delta, so a late fact never recharges or reprices tokens.
func (b *BillingService) settlePrivateTools(ctx context.Context, bc *BillingContext) (retErr error) {
	original := *bc
	defer func() {
		if retErr != nil {
			*bc = original
		}
	}()
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.OrganizationID.String() != bc.OrganizationID || attempt.RequestID != bc.RequestID || attempt.Lane != billingAttemptLaneLocal || bc.BillingLane != UsageBillingLanePrivate {
			return fmt.Errorf("private tool attempt identity mismatch")
		}
		var bill UsageBill
		if err := tx.Where("attempt_id = ? AND organization_id = ? AND billing_lane = ?", bc.AttemptID, bc.OrganizationID, "private").First(&bill).Error; err != nil {
			return err
		}
		stored, err := decodeToolSettlement(bill.PricingSnapshot)
		if err != nil {
			return err
		}
		if err := validateToolIdentity(stored, bc, "private"); err != nil {
			return err
		}
		if stored.Revision == 0 {
			if stored.TokenFinalized {
				restoreToolBillContext(bc, &bill, &attempt)
				bc.Status = "error"
				if stored.InvocationSucceeded {
					bc.Status = "success"
				}
			} else {
				return fmt.Errorf("tool token intent is not durably frozen")
			}
			if bc.ToolSettlement != nil {
				stored.Reused = bc.ToolSettlement.Reused
			}
		} else {
			if bc.ToolSettlement != nil && bc.ToolSettlement.Revision == 0 && billingContextStatusIsSuccess(bc.Status) && bc.ActualCredits != stored.TokenCredits {
				return fmt.Errorf("conflicting frozen token amount")
			}
			// Restore all token and identity projections from the first bill.
			restoreToolBillContext(bc, &bill, &attempt)
		}
		if stored.TokenFinalized && !stored.InvocationSucceeded {
			bc.Status = "error"
			if bill.ErrorCode != nil {
				bc.ErrorCode = *bill.ErrorCode
			}
			if bill.ErrorMessage != nil {
				bc.ErrorMessage = *bill.ErrorMessage
			}
		}
		events, err := toolusage.NewRepository(tx).ListByAttempt(ctx, attempt.OrganizationID, "private", bc.AttemptID)
		if err != nil {
			return err
		}
		next, err := calculateToolSettlement(stored, events, !billingContextStatusIsSuccess(bc.Status))
		if err != nil {
			return err
		}
		oldCredits := bill.TotalPoints.Int64()
		if stored.Revision == 0 {
			oldCredits = 0
		}
		delta := next.TotalCredits - oldCredits
		if delta < 0 {
			return fmt.Errorf("tool correction requires an authorized compensation")
		}
		if stored.Revision > 0 && reflect.DeepEqual(next.LineItems, stored.LineItems) && next.TotalCredits == stored.TotalCredits {
			putToolSettlement(bc, stored)
			bc.ActualCredits = stored.TotalCredits
			return nil
		}
		if bc.ChannelID == nil {
			return fmt.Errorf("private tool settlement missing channel")
		}
		wallet, err := b.getOrCreateChannelWalletForUpdate(ctx, tx, *bc.ChannelID, attempt.OrganizationID)
		if err != nil {
			return err
		}
		reserved := int64(0)
		if stored.Revision == 0 {
			reserved = bc.EstimatedCredits
		}
		change := reserved - delta
		if change < 0 && wallet.Balance.Int64() < math.MinInt64-change || change > 0 && wallet.Balance.Int64() > math.MaxInt64-change {
			return fmt.Errorf("wallet balance overflow")
		}
		after := wallet.Balance.Int64() + change
		if after == math.MinInt64 {
			return fmt.Errorf("wallet debt overflow")
		}
		next.UnsettledCredits = stored.UnsettledCredits
		if after < 0 {
			next.UnsettledCredits = -after
			next.UsageState = "unsettled"
		}
		if next.UnsettledCredits > 0 {
			next.UsageState = "unsettled"
		}
		next.Revision = stored.Revision + 1
		if stored.Revision > 0 {
			next.Adjustments = append(next.Adjustments, toolSettlementAdjustment{Revision: next.Revision, BeforeCredits: oldCredits, AfterCredits: next.TotalCredits, DeltaCredits: delta, EventIDs: next.EventIDs, At: time.Now().UTC()})
		}
		// Subject quota only consumes the new amount on a revision.
		subject := *bc
		subject.ActualCredits = delta
		if stored.Revision > 0 {
			subject.EstimatedCredits, subject.SubjectReservedCredits = 0, 0
		}
		if err := b.settleSubjectQuota(ctx, tx, &subject); err != nil {
			return err
		}
		if err := accumulateToolSubjectCharge(ctx, tx, bc, &subject, stored.Revision > 0); err != nil {
			return err
		}
		status := channelWalletStatusActive
		if after < 0 {
			status = channelWalletStatusDebt
		}
		if err := tx.Model(&ChannelWallet{}).Where("channel_id = ? AND organization_id = ?", wallet.ChannelID, wallet.OrganizationID).Updates(map[string]interface{}{"balance": after, "status": status, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := b.syncRouteBalanceSnapshot(ctx, tx, wallet.ChannelID, after); err != nil {
			return err
		}
		if change != 0 {
			kind := channelWalletTxTypeSettleAdjustment
			if change > 0 {
				kind = channelWalletTxTypeRefund
			}
			if err := b.createChannelWalletTransaction(ctx, tx, wallet.ChannelID, bc.AttemptID, kind, millipoints.MilliPoints(change), wallet.Balance, millipoints.MilliPoints(after), map[string]interface{}{"phase": billingPhaseSettle, "tool_settlement": next}); err != nil {
				return err
			}
		}
		bc.ActualCredits = next.TotalCredits
		bc.TotalUSD, _ = decimal.NewFromString(next.TotalUSD)
		bc.SettledAt = time.Now().UTC()
		putToolSettlement(bc, next)
		if stored.Revision == 0 {
			if err := applyLocalBillingCurrencySnapshot(bc); err != nil {
				return err
			}
		} else {
			values := pricingSnapshotMap(bc.PricingSnapshot)
			values["total_cost_usd"], values["total_cost_cny"] = next.TotalUSD, bc.TotalUSD.Mul(bc.USDToCNYRate).String()
			bc.PricingSnapshot = buildPricingSnapshot(values)
		}
		entryContext := *bc
		if next.TotalCredits > 0 {
			entryContext.Status = "success"
		}
		if err := b.updateAttemptEntriesAfterSettle(ctx, tx, &entryContext, billingLedgerTypeChannelWallet, wallet.ChannelID.String()); err != nil {
			return err
		}
		invocation := invocationResultFromBillingStatus(bc.Status)
		attemptStatus := billingAttemptStatusSettled
		if !billingContextStatusIsSuccess(bc.Status) && next.TotalCredits == 0 {
			attemptStatus = billingAttemptStatusRolledBack
		}
		if err := b.updateAttemptStatus(ctx, tx, bc, attemptStatus, &invocation, nil, nil); err != nil {
			return err
		}
		billStatus := usageBillStatusFromBillingContext(bc.Status)
		if next.TotalCredits > 0 {
			billStatus = usageBillStatusSuccess
		}
		return b.upsertUsageBill(ctx, tx, bc, billStatus, nil, nil)
	})
}

func restoreToolBillContext(bc *BillingContext, bill *UsageBill, attempt *BillingAttempt) {
	bc.BillingLane, bc.UseSystemProvider = bill.BillingLane, bill.UseSystemProvider
	bc.PricingSnapshot = append(datatypes.JSON(nil), bill.PricingSnapshot...)
	bc.ChannelID, bc.RouteID = bill.ChannelID, bill.RouteID
	bc.ModelID, bc.ModelName, bc.ProviderID, bc.ProviderName = bill.ModelID, bill.ModelName, bill.ProviderID, bill.ProviderName
	bc.PricingOperation = PricingOperation(bill.Operation)
	bc.PromptTokens, bc.CompletionTokens, bc.TotalTokens = int(bill.PromptTokens), int(bill.CompletionTokens), int(bill.TotalTokens)
	bc.CacheReadTokens, bc.CacheWriteTokens = int(bill.CacheReadTokens), int(bill.CacheWriteTokens)
	bc.AppID, bc.AppType, bc.AccountID = bill.AppID, bill.AppType, bill.AccountID
	bc.PricingSource, bc.UsageSource, bc.InvocationSource = bill.PricingSource, bill.UsageSource, bill.InvocationSource
	bc.RequestCreatedAt, bc.ResponseTime = bill.RequestCreatedAt, bill.ResponseTimeMS
	bc.Status = "success"
	if bill.Status == usageBillStatusFailed {
		bc.Status = "error"
	}
	bc.QuotaSubjectType, bc.QuotaSubjectID = attempt.QuotaSubjectType, attempt.QuotaSubjectID
	restoreBillingContextAttribution(bc, attempt)
	values := pricingSnapshotMap(bill.PricingSnapshot)
	bc.USDToCNYRate, _ = decimal.NewFromString(pricingSnapshotString(values, exchangeRateSnapshotRateKey))
	bc.DisplayCurrency = pricingSnapshotString(values, exchangeRateSnapshotDisplayCurrencyKey)
}

// reconcilePrivateToolAttempt is an explicit repair hook for one known T4
// attempt. Production reconciliation discovers work from BillingAttempt's due
// state; it must never sweep UsageBill history to find candidates.
func (b *BillingService) reconcilePrivateToolAttempt(ctx context.Context, attemptID string) error {
	if os.Getenv("LLM_TOOL_SETTLEMENT_RECONCILE_PAUSED") == "1" {
		return nil
	}
	var bill UsageBill
	if err := b.db.WithContext(ctx).Where("attempt_id = ? AND billing_lane = ?", attemptID, "private").First(&bill).Error; err != nil {
		return err
	}
	state, err := decodeToolSettlement(bill.PricingSnapshot)
	if err != nil {
		return err
	}
	if state == nil || state.Revision == 0 && !state.TokenFinalized {
		return nil
	}
	bc, err := b.buildLocalRecoveryBillingContext(ctx, attemptID)
	if err != nil {
		return err
	}
	bc.ToolPostpaid, bc.ToolSettlement = true, state
	bc.Status, bc.ActualCredits = "error", 0
	if state.InvocationSucceeded {
		bc.Status, bc.ActualCredits = "success", state.TokenCredits
	}
	return b.settlePrivateTools(ctx, bc)
}

// Persist the first completion quote before applying funds. If the funds
// transaction fails, a worker can retry the same intent after a restart.
func (b *BillingService) freezeToolTokenIntent(ctx context.Context, bc *BillingContext) error {
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt BillingAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_id = ?", bc.AttemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.OrganizationID.String() != bc.OrganizationID || attempt.RequestID != bc.RequestID || attempt.Lane != billingAttemptLaneLocal {
			return fmt.Errorf("tool intent identity mismatch")
		}
		var bill UsageBill
		if err := tx.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
			return err
		}
		state, err := decodeToolSettlement(bill.PricingSnapshot)
		if err != nil {
			return err
		}
		if err := validateToolIdentity(state, bc, "private"); err != nil {
			return err
		}
		if state.Revision > 0 {
			return nil
		}
		if state.TokenFinalized {
			if billingContextStatusIsSuccess(bc.Status) && bc.ActualCredits != state.TokenCredits {
				return fmt.Errorf("conflicting frozen token intent")
			}
			return nil
		}
		state.TokenCredits, state.TokenUSD = 0, "0"
		state.InvocationSucceeded = billingContextStatusIsSuccess(bc.Status)
		if state.InvocationSucceeded {
			state.TokenCredits, state.TokenUSD = bc.ActualCredits, bc.TotalUSD.String()
		}
		state.TokenFinalized = true
		state.TotalCredits, state.TotalUSD, state.ToolUSD = state.TokenCredits, state.TokenUSD, "0"
		if bc.ToolSettlement != nil {
			state.Reused = bc.ToolSettlement.Reused
		}
		copy := *bc
		putToolSettlement(&copy, state)
		if err := applyLocalBillingCurrencySnapshot(&copy); err != nil {
			return err
		}
		return b.upsertUsageBill(ctx, tx, &copy, usageBillStatusPartial, nil, nil)
	})
}

// Preserve cumulative hard-limited grant charges in the subject ledger while
// the fund entry records the full organization cost.
func accumulateToolSubjectCharge(ctx context.Context, tx *gorm.DB, target, delta *BillingContext, revision bool) error {
	if delta.QuotaChargedCredits == nil {
		return nil
	}
	charged := *delta.QuotaChargedCredits
	if revision {
		var entry BillingAttemptEntry
		if err := tx.WithContext(ctx).Where("attempt_id = ? AND entry_type = ?", target.AttemptID, billingEntryTypeSubject).First(&entry).Error; err != nil {
			return err
		}
		if entry.ActualAmount.Int64() > math.MaxInt64-charged {
			return fmt.Errorf("subject charge overflow")
		}
		charged += entry.ActualAmount.Int64()
	}
	target.QuotaChargedCredits = &charged
	return nil
}

// A rejected fresh request retains the existing failure audit. A concurrent or
// repeated attempt must never rewrite an already-owned ledger identity.
func (b *BillingService) recordToolPredeductFailure(ctx context.Context, bc *BillingContext, cause error) error {
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var original BillingAttempt
		if err := tx.Where("attempt_id = ?", bc.AttemptID).First(&original).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Use the same primary-key claim as the dispatch transaction.
		claim := BillingAttempt{AttemptID: bc.AttemptID, RequestID: bc.RequestID, Lane: billingAttemptLaneLocal, QuotaSubjectType: bc.QuotaSubjectType, QuotaSubjectID: bc.QuotaSubjectID, Status: billingAttemptStatusInit}
		if err := claim.OrganizationID.Scan(bc.OrganizationID); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "attempt_id"}}, DoNothing: true}).Create(&claim)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		copy := *bc
		copy.Status, copy.ActualCredits = "error", 0
		if err := b.upsertAttemptInit(ctx, tx, &copy); err != nil {
			return err
		}
		invocation, code, message := "error", "PREDEDUCT_FAILED", cause.Error()
		return b.updateAttemptStatus(ctx, tx, &copy, billingAttemptStatusPredeductFailed, &invocation, &code, &message)
	})
}
