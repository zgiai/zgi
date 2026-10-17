package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
)

// rt2WebSearchPayload is the real provider response shape the shared observer
// parses; it is declared locally because this package has its own copy of the
// toolusage machinery.
const rt2WebSearchPayload = `{"id":"resp_rt2","output":[{"id":"ws_rt2_1","type":"web_search_call","status":"completed"}],"tool_usage":{"web_search":{"num_requests":1}}}`

// RT-02 private lane. The earlier round folded A02/A04/A06/A08 into the platform
// table and left private untested. These cases drive the private lane through the
// shared observer/repository and the private settlement, and assert the two
// properties the review said were missing: the frozen price survives a late fact,
// and no authoritative Console record is produced for private traffic.

// TestRT02PrivateLatchHoldsFactOutsideAuthoritativeStore mirrors the platform
// latch on the private lane. The toolusage machinery is shared, only the owned
// lane differs, so this pins that the private lane really is the one under test.
func TestRT02PrivateLatchHoldsFactOutsideAuthoritativeStore(t *testing.T) {
	f, bc := t4Fixture(t)
	repo := toolusage.NewRepository(f.db)
	attempt := bc.AttemptID

	observer := toolusage.NewObserver(toolusage.Identity{
		OrganizationID: f.orgID, RequestID: bc.RequestID, AttemptID: attempt,
		Lane: toolusage.LanePrivate, Provider: "provider", ChannelID: f.channelID,
	}, []byte(`{"tools":[{"type":"web_search_preview"}]}`))
	observer.Observe([]byte(rt2WebSearchPayload))

	var before int64
	f.db.Model(&toolusage.Event{}).Where("attempt_id = ?", attempt).Count(&before)
	held, err := observer.Events(true, false)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var afterExtract int64
	f.db.Model(&toolusage.Event{}).Where("attempt_id = ?", attempt).Count(&afterExtract)
	if before != 0 || afterExtract != 0 {
		t.Fatalf("private fact must not be persisted before release: before=%d after=%d", before, afterExtract)
	}
	if len(held) == 0 || held[0].BillingLane != toolusage.LanePrivate {
		t.Fatalf("private observer must yield a private-lane fact: %+v", held)
	}
	for i := range held {
		if _, err := repo.Record(context.Background(), &held[i]); err != nil {
			t.Fatalf("release: %v", err)
		}
	}
	var stored toolusage.Event
	f.db.Where("attempt_id = ?", attempt).First(&stored)
	if stored.BillingLane != "private" || stored.Quantity != 1 {
		t.Fatalf("released private fact wrong: lane=%s quantity=%d", stored.BillingLane, stored.Quantity)
	}
	t.Logf("private latch OK: held -> 0 rows, released -> lane=%s quantity=%d", stored.BillingLane, stored.Quantity)
}

// TestRT02PrivateLateFactKeepsFrozenPriceAndChargesOnlyDelta is the private
// counterpart of the platform late-arrival case. The token settles while the tool
// fact is missing and timed out; the trusted fact then arrives and the private
// bill must add only the tool difference on the originally captured price.
func TestRT02PrivateLateFactKeepsFrozenPriceAndChargesOnlyDelta(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	attempt := bc.AttemptID
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}

	// T1: only a pending observation exists, token settles.
	event := t4Record(t, f, bc, toolusage.StatusPending, 0)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	_, afterToken := t4Bill(t, f, bc)
	if afterToken.TokenCredits != 20 || afterToken.ToolCredits != 0 {
		t.Fatalf("token-only private settle wrong: %+v", afterToken)
	}
	if afterToken.UsageState != "pending" {
		t.Fatalf("private settle with a missing fact must stay pending, got %s", afterToken.UsageState)
	}
	capturedVersion := ""
	for _, p := range afterToken.Prices {
		capturedVersion = p.PricingVersion
	}
	if capturedVersion == "" {
		t.Fatalf("the private attempt must capture a pricing version")
	}
	walletAfterToken := rt02PrivateWallet(t, f)

	// T2: the observation expires, then the trusted fact arrives late.
	if _, err := toolusage.NewRepository(f.db).ExpirePending(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("expire: %v", err)
	}
	event.Status = toolusage.StatusSucceeded
	event.Quantity = 2
	event.UsageSource = toolusage.UsageSourceProviderReported
	if _, err := toolusage.NewRepository(f.db).Record(ctx, event); err != nil {
		t.Fatalf("late fact: %v", err)
	}
	// Re-run the private settlement/recovery path.
	replay := *bc
	if err := f.service.Settle(ctx, &replay); err != nil {
		t.Fatalf("settle after late fact: %v", err)
	}

	bill, final := t4Bill(t, f, bc)
	if final.TokenCredits != 20 {
		t.Fatalf("token must stay frozen at 20 across the late fact, got %d", final.TokenCredits)
	}
	if final.ToolCredits <= 0 {
		t.Fatalf("the late private fact must add a tool charge, got %d", final.ToolCredits)
	}
	if final.TotalCredits != 20+final.ToolCredits {
		t.Fatalf("private total must be token + tool: %+v", final)
	}
	// Frozen price: the line item must still carry the originally captured version.
	for _, item := range final.LineItems {
		if item.PricingVersion != capturedVersion {
			t.Fatalf("late private charge used version %q, want the frozen %q",
				item.PricingVersion, capturedVersion)
		}
	}
	if final.Revision < 2 {
		t.Fatalf("the late correction must be recorded as a new revision, got %d", final.Revision)
	}

	// Private is billed locally. The frozen contract explicitly allows an attempt
	// to have several legitimate difference transactions ("允许同一attempt有多笔
	// 合法差额交易"), so the count is not pinned to one. What matters is that the
	// total adjustment equals token + tool exactly once and no stray debits exist.
	adjustments := f.countWalletTransactions(t, attempt, channelWalletTxTypeSettleAdjustment)
	if adjustments < 1 || adjustments > 2 {
		t.Fatalf("expected one or two legitimate adjustments (token, then delta), got %d", adjustments)
	}
	beforeDeduct := f.countWalletTransactions(t, attempt, channelWalletTxTypePreDeduct)
	if beforeDeduct != 1 {
		t.Fatalf("token reservation transactions=%d, want 1", beforeDeduct)
	}
	if got := rt02PrivateWallet(t, f); got != walletAfterToken-final.ToolCredits {
		t.Fatalf("private wallet %d, want %d (token already paid, plus tool %d)",
			got, walletAfterToken-final.ToolCredits, final.ToolCredits)
	}
	// Private settles locally: there must be no Console-side deduction for it.
	if bill.ID == uuid.Nil {
		t.Fatalf("private bill must exist")
	}
	var remoteEntries int64
	f.db.Model(&BillingAttemptEntry{}).Where("attempt_id = ? AND ledger_type = ?", attempt, "remote").
		Count(&remoteEntries)
	if remoteEntries != 0 {
		t.Fatalf("private traffic must not write remote (Console) ledger entries, got %d", remoteEntries)
	}
	t.Logf("private late fact OK: token=%d tool=%d total=%d revision=%d frozen_version=%s",
		final.TokenCredits, final.ToolCredits, final.TotalCredits, final.Revision, capturedVersion)
}

// rt02PrivateWallet reads the channel wallet the private lane debits.
func rt02PrivateWallet(t *testing.T, f *t9BillingFixture) int64 {
	t.Helper()
	var wallet ChannelWallet
	if err := f.db.First(&wallet, "channel_id = ?", f.channelID).Error; err != nil {
		t.Fatalf("load channel wallet: %v", err)
	}
	return int64(wallet.Balance)
}
