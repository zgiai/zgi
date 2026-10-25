package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// T1 platform lane, Pre side: mirror the authoritative release of an unquoted
// zero hold.
//
// The authority is the real expired Console deduction this round produced in
// zgi_tool_rt2_20261015 (attempt t1-zerohold-a1, org bbf9d4a9-..., deduction
// 4e239f0f-..., settlement_status ROLLED_BACK, all amounts zero, revision 0, no
// token quote). This fixture parks the mirrored attempt in the real Pre database
// and then lets the *running Pre service's* platform reconcile worker query
// Console and mirror the release through mirrorReleasedToolHold. The test never
// calls the mirror itself.

const (
	t1MirrorAttemptID  = "t1-zerohold-a1"
	t1MirrorRequestID  = "t1-zerohold-r1"
	t1MirrorOrgID      = "bbf9d4a9-bf8b-4ffe-a677-b289d3bfb21b"
	t1MirrorSettlement = "4e239f0f-6b1b-4cc5-b1de-232af1fcef5b"
)

type t1MirrorState struct {
	AttemptID string    `json:"attempt_id"`
	RequestID string    `json:"request_id"`
	OrgID     uuid.UUID `json:"org_id"`
	RouteID   uuid.UUID `json:"route_id"`
	BillID    uuid.UUID `json:"bill_id"`
	ParkedAt  string    `json:"parked_at"`
}

func t1MirrorBillSnapshot() datatypes.JSON {
	state := map[string]any{
		"schema_version": 1, "billing_lane": "platform", "attempt_id": t1MirrorAttemptID,
		"request_id": t1MirrorRequestID, "settlement_id": t1MirrorSettlement,
		"usage_state": "pending", "free_reason": "", "pricing_version": "before",
		"revision": 0, "token_credits": 0, "tool_credits": 0, "total_credits": 0,
		"unsettled_credits": 0, "prices": map[string]any{}, "line_items": []any{},
	}
	blob, _ := json.Marshal(map[string]any{"tool_postpaid": true, "tool_settlement": state})
	return datatypes.JSON(blob)
}

// t1MirrorDelete removes only this fixture's own Pre rows.
func t1MirrorDelete(t *testing.T, db *gorm.DB) {
	t.Helper()
	if t1MirrorAttemptID != "t1-zerohold-a1" {
		t.Fatalf("refusing to clean up an attempt this fixture does not own")
	}
	for _, stmt := range []string{
		`DELETE FROM channel_wallet_transactions WHERE attempt_id = ?`,
		`DELETE FROM billing_attempt_entries WHERE attempt_id = ?`,
		`DELETE FROM llm_usage_bills WHERE attempt_id = ?`,
		`DELETE FROM billing_attempts WHERE attempt_id = ?`,
	} {
		if err := db.Exec(stmt, t1MirrorAttemptID).Error; err != nil {
			t.Fatalf("cleanup %s: %v", stmt, err)
		}
	}
}

func TestT1PlatformMirrorPhase1ParksRemoteAttempt(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T1M_STATE")
	t1MirrorDelete(t, db)

	orgID := uuid.MustParse(t1MirrorOrgID)
	routeID := uuid.New()
	modelID := uuid.New()
	provID := uuid.New()
	now := time.Now().UTC()

	attempt := &BillingAttempt{
		AttemptID: t1MirrorAttemptID, RequestID: t1MirrorRequestID, OrganizationID: orgID,
		Lane: billingAttemptLaneRemote, Status: billingAttemptStatusPartial,
		QuotaSubjectType: quotaSubjectTypeOrganization, QuotaSubjectID: orgID.String(),
		InvocationSource: InvocationSource("api"), AuthMethod: "legacy_api_key",
		SettlementState: "PENDING", SettlementVersion: 1,
		RouteID: &routeID, ModelID: &modelID, ProviderID: &provID,
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("park remote attempt: %v", err)
	}
	entries := []BillingAttemptEntry{
		{AttemptID: t1MirrorAttemptID, EntryType: billingEntryTypeSubject, LedgerType: "key_quota",
			LedgerRefID: orgID.String(), Status: "PENDING", CreatedAt: now, UpdatedAt: now},
		{AttemptID: t1MirrorAttemptID, EntryType: billingEntryTypeFund, LedgerType: billingLedgerTypeOrgFunds,
			LedgerRefID: orgID.String(), Status: "PENDING", CreatedAt: now, UpdatedAt: now},
	}
	for i := range entries {
		if err := db.Create(&entries[i]).Error; err != nil {
			t.Fatalf("park entry: %v", err)
		}
	}
	quotaSubject := quotaSubjectTypeOrganization
	bill := &UsageBill{
		ID: uuid.New(), AttemptID: t1MirrorAttemptID, RequestID: t1MirrorRequestID,
		OrganizationID: t1MirrorOrgID, APIKeyID: uuid.NewString(),
		ModelID: modelID, ModelName: "gpt-4o", ProviderID: provID, ProviderName: "openai",
		RouteID: &routeID, ChannelID: &routeID, BillingLane: UsageBillingLanePlatform,
		UseSystemProvider: true, Status: usageBillStatusPartial,
		QuotaSubjectType: &quotaSubject, QuotaSubjectID: &[]string{orgID.String()}[0],
		PricingSnapshot: t1MirrorBillSnapshot(), InvocationSource: InvocationSource("api"),
		RequestCreatedAt: now.Add(-time.Minute), SettledAt: now.Add(-time.Minute),
	}
	if err := db.Create(bill).Error; err != nil {
		t.Fatalf("park platform bill: %v", err)
	}

	parked := t1MirrorState{AttemptID: t1MirrorAttemptID, RequestID: t1MirrorRequestID, OrgID: orgID,
		RouteID: routeID, BillID: bill.ID, ParkedAt: time.Now().UTC().Format(time.RFC3339)}
	blob, _ := json.MarshalIndent(parked, "", "  ")
	if err := os.WriteFile(statePath, blob, 0o600); err != nil {
		t.Fatalf("park state: %v", err)
	}
	fmt.Printf("T1_MIRROR_PHASE1_PARKED attempt=%s org=%s lane=remote status=%s bill=%s parked_at=%s\n",
		t1MirrorAttemptID, t1MirrorOrgID, attempt.Status, bill.ID, parked.ParkedAt)
}

func TestT1PlatformMirrorPhase2AssertsRelease(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T1M_STATE")
	blob, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read parked state: %v", err)
	}
	var parked t1MirrorState
	if err := json.Unmarshal(blob, &parked); err != nil {
		t.Fatalf("decode parked state: %v", err)
	}

	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", parked.AttemptID).First(&attempt).Error; err != nil {
		t.Fatalf("load attempt: %v", err)
	}
	if attempt.Status != billingAttemptStatusRolledBack {
		t.Fatalf("mirrored attempt status = %s, want %s", attempt.Status, billingAttemptStatusRolledBack)
	}
	if attempt.SettlementState != billingAttemptStatusRolledBack {
		t.Fatalf("mirrored settlement_state = %s, want %s", attempt.SettlementState, billingAttemptStatusRolledBack)
	}
	if attempt.ConsoleSettlementID == nil || *attempt.ConsoleSettlementID != t1MirrorSettlement {
		t.Fatalf("console settlement id = %v, want %s", attempt.ConsoleSettlementID, t1MirrorSettlement)
	}

	// No funds: both entries settle at zero and no wallet transaction is written.
	var entries []BillingAttemptEntry
	if err := db.Where("attempt_id = ?", parked.AttemptID).Find(&entries).Error; err != nil {
		t.Fatalf("load entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected the two parked entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.ActualAmount != 0 || e.ReservedAmount != 0 || e.RefundedAmount != 0 {
			t.Fatalf("a released zero hold must not move funds: %+v", e)
		}
	}
	var walletTx int64
	db.Model(&ChannelWalletTransaction{}).Where("attempt_id = ?", parked.AttemptID).Count(&walletTx)
	if walletTx != 0 {
		t.Fatalf("released zero hold wrote %d wallet transactions", walletTx)
	}

	// Audit: the original tool JSON survives, still unquoted and at revision 0.
	bill, state := prePGBill(t, db, parked.AttemptID)
	if state.Revision != 0 || state.TokenCredits != 0 || state.ToolCredits != 0 ||
		state.TotalCredits != 0 || state.TokenFinalized {
		t.Fatalf("released hold was rewritten: %+v", state)
	}
	if bill.Status != usageBillStatusFailed {
		t.Fatalf("released bill status = %s, want %s", bill.Status, usageBillStatusFailed)
	}
	if int64(bill.OfficialPoints) != 0 || int64(bill.PrivatePoints) != 0 || int64(bill.TotalPoints) != 0 {
		t.Fatalf("released hold billed points: official=%d private=%d total=%d",
			bill.OfficialPoints, bill.PrivatePoints, bill.TotalPoints)
	}
	errMsg := ""
	if bill.ErrorMessage != nil {
		errMsg = *bill.ErrorMessage
	}
	fmt.Printf("T1_MIRROR_PHASE2_OK attempt=%s status=%s settlement_state=%s console_settlement_id=%s entries_zero=true wallet_tx=0 bill_status=%s revision=%d token=%d tool=%d error=%q\n",
		parked.AttemptID, attempt.Status, attempt.SettlementState, *attempt.ConsoleSettlementID,
		bill.Status, state.Revision, state.TokenCredits, state.ToolCredits, errMsg)
}
