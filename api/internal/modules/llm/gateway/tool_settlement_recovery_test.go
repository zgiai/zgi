package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"gorm.io/datatypes"
)

func t4RemoteRecoveryFixture(t *testing.T) (*t9BillingFixture, *BillingContext, *RemoteBilling, *countingSettleQuotaClient) {
	t.Helper()
	f, bc := t4Fixture(t)
	bc.BillingLane, bc.UseSystemProvider, bc.ToolSettlement = UsageBillingLanePlatform, true, nil
	bc.PricingOperation = PricingOperationChat
	bc.PricingSnapshot = datatypes.JSON(`{"operation":"chat"}`)
	bc.ToolPostpaid = true
	if err := f.db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", f.apiKeyID).Update("quota_limit", nil).Error; err != nil {
		t.Fatal(err)
	}
	client := &countingSettleQuotaClient{}
	remote := &RemoteBilling{localService: f.service, grpcClient: client, reconcileWorkerID: "t4-recovery-test"}
	if err := remote.PreDeduct(context.Background(), bc); err != nil {
		t.Fatal(err)
	}
	return f, bc, remote, client
}

func t4AuthoritativeResult(bc *BillingContext, total, revision int64) *settlementQueryResponse {
	state := &toolSettlement{SchemaVersion: 1, BillingLane: "platform", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.DeductionID, UsageState: "settled", Revision: revision, TokenCredits: 100, ToolCredits: total - 100, TotalCredits: total, TokenUSD: "0.0001", ToolUSD: "0.004", TotalUSD: "0.0041", Prices: map[string]toolPriceCapture{"console-channel": {CNYPerUSD: "7.2"}}, EventIDs: []string{"event-1", "event-2", "event-3", "event-4"}}
	if total == 5100 {
		state.ToolUSD, state.TotalUSD = "0.005", "0.0051"
	}
	raw, _ := json.Marshal(state)
	snapshot := buildPricingSnapshot(map[string]interface{}{"cny_per_usd": "7.2", "tool_settlement": json.RawMessage(raw)})
	return &settlementQueryResponse{Found: true, OrganizationID: bc.OrganizationID, RequestID: bc.RequestID, AttemptID: bc.AttemptID, SettlementID: bc.DeductionID, DeductionID: bc.DeductionID, SettlementStatus: "SETTLED", PromptTokens: 80, CompletionTokens: 20, TotalTokens: 100, SettledCredits: total, ToolSettlement: raw, PricingSnapshot: json.RawMessage(snapshot)}
}

func TestT4PlatformFailureRecoveryWithoutFirstToolJSON(t *testing.T) {
	f, bc, remote, client := t4RemoteRecoveryFixture(t)
	ctx := context.Background()
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("recovery dispatched inference")
		}
		result := t4AuthoritativeResult(bc, 4100, 1)
		switch mode.Load() {
		case 0:
			result.ToolSettlement = nil
			result.PricingSnapshot = json.RawMessage(`{"cny_per_usd":"7.2"}`)
		case 2:
			result = t4AuthoritativeResult(bc, 5100, 2)
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	remote.consoleAPIBaseURL = server.URL
	var bill UsageBill
	if err := f.db.First(&bill, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if !isToolPostpaidSnapshot(bill.PricingSnapshot) || bill.Status != usageBillStatusPartial {
		t.Fatal("postpaid identity was not persisted before dispatch")
	}
	svc := &llmGatewayServiceImpl{billing: remote}
	if err := svc.queuePlatformSettlementRecovery(ctx, bc, adapter.ErrBillingSettleFailed); !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("%v", err)
	}
	// The Console money outcome alone is insufficient to finalize the tool bill.
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err == nil {
		t.Fatal("missing first tool JSON was accepted")
	}
	if err := f.db.First(&bill, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if bill.Status == usageBillStatusSuccess {
		t.Fatal("incomplete mirror became normal success")
	}
	f.assertAPIKeyQuota(t, 0, 100)
	mode.Store(1)
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertAPIKeyQuota(t, 4100, 100)
	mode.Store(2)
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertAPIKeyQuota(t, 5100, 100)
	f.assertWalletBalance(t, 200)
	finalBill, state := t4Bill(t, f, bc)
	if finalBill.TotalTokens != 100 || finalBill.PromptTokens != 80 || finalBill.CompletionTokens != 20 {
		t.Fatalf("authoritative tokens overwritten: %+v", finalBill)
	}
	if state.Revision != 2 || state.ToolCredits != 5000 || state.TokenCredits != 100 {
		t.Fatalf("%+v", state)
	}
	if client.settleCalls != 0 {
		t.Fatalf("funds RPC calls=%d", client.settleCalls)
	}
	var count int64
	if err := f.db.Model(&ChannelWalletTransaction{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("funds transactions=%d %v", count, err)
	}
	if err := f.db.Model(&toolusage.Event{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("tool events=%d %v", count, err)
	}
}

func TestT4InFlightRequestDowngradesAfterConsoleRollback(t *testing.T) {
	f, bc, remote, client := t4RemoteRecoveryFixture(t)
	result := &settlementQueryResponse{
		Found: true, OrganizationID: bc.OrganizationID, RequestID: bc.RequestID,
		AttemptID: bc.AttemptID, SettlementID: bc.DeductionID, DeductionID: bc.DeductionID,
		SettlementStatus: "SETTLED", ReservedCredits: 100, SettledCredits: 100,
		PromptTokens: 80, CompletionTokens: 20, TotalTokens: 100,
		PricingSnapshot: json.RawMessage(`{"cny_per_usd":"7.2"}`),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	remote.consoleAPIBaseURL = server.URL

	if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, nil); err != nil {
		t.Fatal(err)
	}
	if bc.ToolPostpaid || isToolPostpaidSnapshot(bc.PricingSnapshot) {
		t.Fatal("rolled-back Console left the in-flight request on the v2 contract")
	}
	f.assertAPIKeyQuota(t, 100, 100)
	if client.settleCalls != 0 {
		t.Fatal("legacy downgrade repeated the Console funds settlement RPC")
	}
}

func TestT4AuditedLegacyMirrorRepairDoesNotRechargeQuota(t *testing.T) {
	f, bc, remote, client := t4RemoteRecoveryFixture(t)
	ctx := context.Background()
	// Reproduce the old faulty finalization: amount/subject already applied,
	// but the tool JSON and postpaid marker were dropped.
	bc.ToolPostpaid = false
	bc.PricingSnapshot = datatypes.JSON(`{}`)
	bc.ActualCredits = 4100
	if err := f.db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"status": billingAttemptStatusSettled, "settlement_state": billingAttemptStatusSettled, "console_settlement_id": bc.DeductionID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&BillingAttemptEntry{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"actual_amount": 4100, "status": billingEntryStatusSettled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", f.apiKeyID).Update("used_quota", 4100).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.upsertUsageBill(ctx, f.db, bc, usageBillStatusSuccess, nil, nil); err != nil {
		t.Fatal(err)
	}
	f.assertAPIKeyQuota(t, 4100, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(t4AuthoritativeResult(bc, 4100, 1))
	}))
	defer server.Close()
	remote.consoleAPIBaseURL = server.URL
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := remote.reconcilePlatformToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertAPIKeyQuota(t, 4100, 100)
	f.assertWalletBalance(t, 200)
	_, state := t4Bill(t, f, bc)
	if state.TotalCredits != 4100 || state.Revision != 1 {
		t.Fatalf("%+v", state)
	}
	if client.settleCalls != 0 {
		t.Fatal("repair called funds settlement")
	}
}

func TestT4PlatformSSEFinanceFailureQueuesOnlyMirror(t *testing.T) {
	f, bc, remote, client := t4RemoteRecoveryFixture(t)
	svc := &llmGatewayServiceImpl{billing: remote, db: f.db, healthTracker: NewChannelHealthTracker(nil)}
	selection := &ProviderSelection{UseSystemProvider: true, BillingLane: UsageBillingLanePlatform, Model: llmmodel.LLMModel{ID: bc.ModelID, Model: bc.ModelName}}
	in := make(chan adapter.RawStreamEvent, 1)
	out := make(chan adapter.RawStreamEvent, 2)
	in <- adapter.RawStreamEvent{Event: "error", Data: json.RawMessage(`{"error_code":"billing.settle.failed","retryable":true,"error":{"code":"settlement_pending","message":"pending"}}`)}
	close(in)
	svc.handleNativeStreamBilling(context.Background(), nil, in, out, bc, selection, nil, 0, time.Now(), bc.ModelName, nil, "llm.native.stream", nativeUsageBodyFormatResponses)
	got := <-out
	if !got.Done || !errors.Is(got.Error, ErrBillingSettleFailed) || len(got.Data) != 0 {
		t.Fatalf("%+v", got)
	}
	var attempt BillingAttempt
	if err := f.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.SettlementState != billingAttemptStatusFailedRetryable || attempt.Status == billingAttemptStatusSettled {
		t.Fatalf("%+v", attempt)
	}
	if client.settleCalls != 0 {
		t.Fatal("finance failure released or re-settled funds")
	}
	f.assertWalletBalance(t, 200)
}

func TestT4PlatformPlainSuccessCannotBypassToolMirror(t *testing.T) {
	f, bc, remote, _ := t4RemoteRecoveryFixture(t)
	if err := remote.finalizeRemoteSettlement(context.Background(), bc); err == nil {
		t.Fatal("plain finalize swallowed pending tool mirror")
	}
	f.assertAPIKeyQuota(t, 0, 100)
}

func TestT4ExpiredZeroToolHoldIsMirroredWithoutSettlementRPC(t *testing.T) {
	f, bc, remote, client := t4RemoteRecoveryFixture(t)
	initial := &toolSettlement{SchemaVersion: 1, BillingLane: "platform", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.DeductionID, UsageState: "pending", Prices: map[string]toolPriceCapture{"console-channel": {PricingVersion: "frozen-before-failure"}}}
	raw, _ := json.Marshal(initial)
	result := &settlementQueryResponse{Found: true, OrganizationID: bc.OrganizationID, RequestID: bc.RequestID, AttemptID: bc.AttemptID, SettlementID: bc.DeductionID, DeductionID: bc.DeductionID, SettlementStatus: billingAttemptStatusRolledBack, ReservedCredits: bc.EstimatedCredits, RefundedCredits: bc.EstimatedCredits, ToolSettlement: raw, PricingSnapshot: json.RawMessage(buildPricingSnapshot(map[string]interface{}{"tool_settlement": json.RawMessage(raw)}))}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("release must only query authority")
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()
	remote.consoleAPIBaseURL = server.URL
	// Reproduce a request whose prior mirror retries were exhausted.
	if err := f.db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Updates(map[string]interface{}{"status": billingAttemptStatusPartial, "settlement_state": billingAttemptStatusFailedFinal}).Error; err != nil {
		t.Fatal(err)
	}
	if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, nil); err != nil {
		t.Fatal(err)
	}
	if err := remote.reconcilePlatformToolAttempt(context.Background(), bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	var attempt BillingAttempt
	if err := f.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusRolledBack || attempt.SettlementState != billingAttemptStatusRolledBack {
		t.Fatalf("release not terminal: %s/%s", attempt.Status, attempt.SettlementState)
	}
	var bill UsageBill
	f.db.First(&bill, "attempt_id = ?", bc.AttemptID)
	if bill.Status == usageBillStatusSuccess || bill.TotalPoints != 0 {
		t.Fatal("failed release became a charged/successful call")
	}
	f.assertWalletBalance(t, 200)
	f.assertAPIKeyQuota(t, 0, 100)
	if client.settleCalls != 0 {
		t.Fatal("release called funding RPC")
	}
	var count int64
	f.db.Model(&ChannelWalletTransaction{}).Count(&count)
	if count != 0 {
		t.Fatal("release mutated wallet funds")
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(bill.PricingSnapshot, &root)
	if !toolJSONEqual(root["tool_settlement"], raw) {
		t.Fatal("authority price snapshot was rewritten")
	}
}

func TestT4ReleasedToolHoldRejectsQuotesAmountsAndIdentityConflicts(t *testing.T) {
	for _, name := range []string{"retryable", "settled amount", "reserved amount", "quote", "completed revision", "request identity", "prior settlement"} {
		t.Run(name, func(t *testing.T) {
			f, bc, remote, client := t4RemoteRecoveryFixture(t)
			initial := map[string]interface{}{"schema_version": 1, "billing_lane": "platform", "attempt_id": bc.AttemptID, "request_id": bc.RequestID, "settlement_id": bc.DeductionID, "usage_state": "pending", "revision": 0, "token_credits": 0, "tool_credits": 0, "total_credits": 0}
			result := &settlementQueryResponse{Found: true, OrganizationID: bc.OrganizationID, RequestID: bc.RequestID, AttemptID: bc.AttemptID, SettlementID: bc.DeductionID, DeductionID: bc.DeductionID, SettlementStatus: billingAttemptStatusRolledBack}
			switch name {
			case "retryable":
				result.Retryable = true
			case "settled amount":
				result.SettledCredits = 1
			case "reserved amount":
				result.ReservedCredits = 1
			case "quote":
				initial["token_quote"] = map[string]interface{}{"actual_credits": 0}
			case "completed revision":
				initial["revision"] = 1
			case "request identity":
				initial["request_id"] = "different-request"
			case "prior settlement":
				f.db.Model(&BillingAttempt{}).Where("attempt_id = ?", bc.AttemptID).Update("status", billingAttemptStatusSettled)
			}
			raw, _ := json.Marshal(initial)
			result.ToolSettlement = raw
			result.PricingSnapshot = json.RawMessage(buildPricingSnapshot(map[string]interface{}{"tool_settlement": json.RawMessage(raw)}))
			var attempt BillingAttempt
			f.db.First(&attempt, "attempt_id = ?", bc.AttemptID)
			if err := remote.mirrorQueriedSettlement(context.Background(), &attempt, result); err == nil {
				t.Fatal("unsafe release accepted")
			}
			var after BillingAttempt
			f.db.First(&after, "attempt_id = ?", bc.AttemptID)
			if after.Status != attempt.Status {
				t.Fatal("rejected release changed attempt state")
			}
			f.assertWalletBalance(t, 200)
			f.assertAPIKeyQuota(t, 0, 100)
			if client.settleCalls != 0 {
				t.Fatal("rejected release called funding RPC")
			}
		})
	}
}
