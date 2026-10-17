package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/millipoints"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Private-lane T1/T2 fixtures on the *real* Pre PostgreSQL service database.
//
// The earlier private coverage lived in the package's SQLite fixture. These
// cases put the same identities into the database the running Pre service
// connects to (public schema), so the service's own local recovery worker can be
// the thing that acts. The fixture never calls a recovery function: it either
// drives the ordinary Settle entry point (the release path) or parks a state and
// lets the worker sweep it.
//
// A dedicated attempt-id prefix ("rt2t1-"/"rt2t2-") lets the round clean up only
// rows it created.

const (
	prePGFrozenToolsJSON = `{"schema_version":1,"billing_lane":"private","pricing_version":"P1-frozen","tools":{"default":{"web_search":{"amount":"0.00003","currency":"USD","unit":"call","per_quantity":1}}}}`
	prePGTokenCredits    = int64(20)
	prePGTokenUSD        = "0.00002"
	prePGLateQuantity    = int64(2)
)

func prePGOpen(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("PRE_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("set PRE_PG_TEST_DSN to the isolated Pre service database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open Pre service database: %v", err)
	}
	var current string
	if err := db.Raw("SELECT current_schema()").Scan(&current).Error; err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if current != "public" {
		t.Fatalf("Pre fixture must run in the service's default schema, got %q", current)
	}
	return db
}

func prePGStatePath(t *testing.T, env string) string {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skip("set " + env + " to run this phase")
	}
	return path
}

// prePGIdentity is one isolated organization with its own route, channel wallet
// and API key, so no pre-existing wallet or key is touched by these cases.
type prePGIdentity struct {
	Label      string    `json:"label"`
	OrgID      uuid.UUID `json:"org_id"`
	RouteID    uuid.UUID `json:"route_id"`
	APIKeyID     uuid.UUID `json:"api_key_id"`
	ModelID      uuid.UUID `json:"model_id"`
	ProviderID   uuid.UUID `json:"provider_id"`
	CredentialID uuid.UUID `json:"credential_id"`
}

func prePGSeedIdentity(t *testing.T, db *gorm.DB, label string) prePGIdentity {
	t.Helper()
	id := prePGIdentity{
		Label: label, OrgID: uuid.New(), RouteID: uuid.New(),
		APIKeyID: uuid.New(), ModelID: uuid.New(), ProviderID: uuid.New(), CredentialID: uuid.New(),
	}
	name := "rt2-" + label + "-" + id.OrgID.String()[:8]
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed identity (%s): %v", label, err)
		}
	}
	mustExec(`INSERT INTO organizations (id, name, status, billing_display_currency, usd_to_cny_rate)
		VALUES (?, ?, 'active', 'USD', 7)`, id.OrgID, name)
	mustExec(`INSERT INTO llm_routes (id, organization_id, type, priority, weight, is_enabled, name, balance, user_credential_id)
		VALUES (?, ?, 'PRIVATE', 100, 1, false, ?, 100000, ?)`, id.RouteID, id.OrgID, name, id.CredentialID)
	mustExec(`INSERT INTO channel_wallets (channel_id, organization_id, balance, status)
		VALUES (?, ?, 100000, 'ACTIVE')`, id.RouteID, id.OrgID)
	mustExec(`INSERT INTO llm_organization_api_keys (id, organization_id, name, key, key_hash, status, used_quota, remain_quota, key_prefix, key_suffix)
		VALUES (?, ?, ?, ?, ?, 'active', 0, 100000, '', '')`,
		id.APIKeyID, id.OrgID, name, "sk-"+name, "hash-"+name)
	return id
}

// prePGToolContext builds the private billing context for a hosted-tool request:
// ToolPostpaid is the contract's zero-amount placeholder, so nothing is reserved
// up front and the settlement identity carries the frozen tool prices.
func prePGToolContext(t *testing.T, id prePGIdentity, attemptID, requestID, pricingVersion string) *BillingContext {
	t.Helper()
	raw := json.RawMessage(prePGFrozenToolsJSON)
	if pricingVersion != "" && pricingVersion != "P1-frozen" {
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode frozen tools: %v", err)
		}
		doc["pricing_version"] = pricingVersion
		rebuilt, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("rebuild frozen tools: %v", err)
		}
		raw = rebuilt
	}
	state := &toolSettlement{
		SchemaVersion: 1, BillingLane: "private", AttemptID: attemptID, RequestID: requestID,
		SettlementID: attemptID, UsageState: "pending",
		Requested: []string{toolusage.ToolWebSearch},
		Prices: map[string]toolPriceCapture{
			id.RouteID.String(): captureToolPrices(raw, id.RouteID.String(), "7.2", 1000, "charge"),
		},
		LineItems: []toolSettlementLine{}, EventIDs: []string{}, Adjustments: []toolSettlementAdjustment{},
	}
	channel := id.RouteID
	bc := &BillingContext{
		APIKeyID: id.APIKeyID.String(), OrganizationID: id.OrgID.String(),
		AttemptID: attemptID, RequestID: requestID,
		QuotaSubjectType: quotaSubjectTypeAPIKey, QuotaSubjectID: id.APIKeyID.String(),
		ChannelID: &channel, BillingLane: UsageBillingLanePrivate, UseSystemProvider: false,
		EstimatedCredits: 0, SubjectReservedCredits: 0, ToolPostpaid: true,
		ModelID: id.ModelID, ModelName: "gpt-4o", ProviderID: id.ProviderID, ProviderName: "openai",
		PricingOperation: PricingOperationChat, PricingSource: PricingSourceUpstreamModelPrice,
		UsageSource:     UsageSourceRequestParameters,
		PricingSnapshot: datatypes.JSON([]byte(`{"operation":"chat","meter":"token","base_unit":"1k_tokens"}`)),
		RequestCreatedAt: time.Now().Add(-time.Second),
		Status:           billingContextStatusSuccess,
	}
	putToolSettlement(bc, state)
	return bc
}

func prePGBill(t *testing.T, db *gorm.DB, attemptID string) (*UsageBill, *toolSettlement) {
	t.Helper()
	var bill UsageBill
	if err := db.Where("attempt_id = ?", attemptID).First(&bill).Error; err != nil {
		t.Fatalf("load usage bill %s: %v", attemptID, err)
	}
	state, err := decodeToolSettlement(bill.PricingSnapshot)
	if err != nil || state == nil {
		t.Fatalf("decode tool settlement for %s: state=%+v err=%v", attemptID, state, err)
	}
	return &bill, state
}

func prePGWalletBalance(t *testing.T, db *gorm.DB, id prePGIdentity) int64 {
	t.Helper()
	var wallet ChannelWallet
	if err := db.First(&wallet, "channel_id = ?", id.RouteID).Error; err != nil {
		t.Fatalf("load channel wallet: %v", err)
	}
	return int64(wallet.Balance)
}

func prePGAttempt(t *testing.T, db *gorm.DB, attemptID string) *BillingAttempt {
	t.Helper()
	var attempt BillingAttempt
	if err := db.Where("attempt_id = ?", attemptID).First(&attempt).Error; err != nil {
		t.Fatalf("load attempt %s: %v", attemptID, err)
	}
	return &attempt
}

func prePGWalletTxCount(t *testing.T, db *gorm.DB, attemptID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&ChannelWalletTransaction{}).Where("attempt_id = ?", attemptID).Count(&count).Error; err != nil {
		t.Fatalf("count wallet transactions: %v", err)
	}
	return count
}

// prePGFrozenVersion reads the pricing version the capture actually froze.
func prePGFrozenVersion(state *toolSettlement) string {
	for _, capture := range state.Prices {
		return capture.PricingVersion
	}
	return ""
}

// prePGCleanupAttempt removes only the fixture's own rows, so a re-run of a
// phase never collides with an earlier attempt. The attempt-id prefixes are
// owned by this file.
func prePGCleanupAttempt(t *testing.T, db *gorm.DB, attemptID string) {
	t.Helper()
	if !strings.HasPrefix(attemptID, "rt2t1-") && !strings.HasPrefix(attemptID, "rt2t2-") {
		t.Fatalf("refusing to clean up attempt %q: not ours", attemptID)
	}
	for _, stmt := range []string{
		`DELETE FROM channel_wallet_transactions WHERE attempt_id = ?`,
		`DELETE FROM billing_attempt_entries WHERE attempt_id = ?`,
		`DELETE FROM llm_usage_bills WHERE attempt_id = ?`,
		`DELETE FROM llm_tool_usage_events WHERE attempt_id = ?`,
		`DELETE FROM billing_attempts WHERE attempt_id = ?`,
	} {
		if err := db.Exec(stmt, attemptID).Error; err != nil {
			t.Fatalf("cleanup %s (%s): %v", attemptID, stmt, err)
		}
	}
}

// prePGExpectedToolCredits recomputes the frozen-price amount with plain decimal
// arithmetic. It reads only the captured price, never the settlement engine.
func prePGExpectedToolCredits(t *testing.T, capture toolPriceCapture, tool string, quantity int64) int64 {
	t.Helper()
	resolved := capture.Resolved[tool]
	amount, err := decimal.NewFromString(resolved.Amount)
	if err != nil {
		t.Fatalf("frozen amount %q: %v", resolved.Amount, err)
	}
	rate, err := decimal.NewFromString(capture.CNYPerUSD)
	if err != nil || !rate.IsPositive() {
		t.Fatalf("frozen rate %q: %v", capture.CNYPerUSD, err)
	}
	if resolved.PerQuantity <= 0 {
		t.Fatalf("frozen per_quantity %d", resolved.PerQuantity)
	}
	perQuantity := decimal.NewFromInt(resolved.PerQuantity)
	cost := amount.Mul(decimal.NewFromInt(quantity)).Div(perQuantity)
	if resolved.Currency == "CNY" {
		cost = cost.DivRound(rate, 36)
	}
	credits := cost.Mul(decimal.NewFromInt(capture.PointsPerUSD)).
		Mul(decimal.NewFromInt(millipoints.UnitsPerPoint)).Ceil()
	return credits.IntPart()
}

// ---------------------------------------------------------------------------
// T1 private, case A: a request that failed with no success fact goes through
// the ordinary release and must end as a failed zero-amount terminal record.
// ---------------------------------------------------------------------------

func TestT1PrivateZeroHoldReleaseIsFailedZeroTerminal(t *testing.T) {
	db := prePGOpen(t)
	// The service's own billing service, not a stub.
	svc := NewBillingService(db, nil, nil, nil)
	id := prePGSeedIdentity(t, db, "t1a")
	attemptID, requestID := "rt2t1-zerohold-release-a1", "rt2t1-zerohold-release-r1"
	prePGCleanupAttempt(t, db, attemptID)

	bc := prePGToolContext(t, id, attemptID, requestID, "P1-frozen")
	if err := svc.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct (zero hold): %v", err)
	}
	walletAfterHold := prePGWalletBalance(t, db, id)
	if walletAfterHold != 100000 {
		t.Fatalf("a postpaid zero hold must not reserve funds, wallet=%d", walletAfterHold)
	}
	if n := prePGWalletTxCount(t, db, attemptID); n != 0 {
		t.Fatalf("a zero hold must not write a funds transaction, got %d", n)
	}
	_, held := prePGBill(t, db, attemptID)
	if held.Revision != 0 || held.TokenFinalized {
		t.Fatalf("parked zero hold must be unquoted: revision=%d finalized=%v", held.Revision, held.TokenFinalized)
	}

	// The release travels on the same request context the hold was created with,
	// exactly like the request finalizer: the currency snapshot captured at
	// pre-deduction is already on it.
	capturedRate := bc.USDToCNYRate
	if !capturedRate.IsPositive() {
		t.Fatalf("pre-deduct must capture the organization rate")
	}
	bc.Status = "error"
	bc.ActualCredits = 0
	bc.TotalUSD = decimal.Zero
	if err := svc.Settle(t.Context(), bc); err != nil {
		t.Fatalf("release Settle(): %v", err)
	}

	attempt := prePGAttempt(t, db, attemptID)
	if attempt.Status != billingAttemptStatusRolledBack {
		t.Fatalf("released zero hold attempt status = %s, want %s", attempt.Status, billingAttemptStatusRolledBack)
	}
	bill, state := prePGBill(t, db, attemptID)
	if bill.Status != usageBillStatusFailed {
		t.Fatalf("released zero hold bill status = %s, want %s", bill.Status, usageBillStatusFailed)
	}
	if state.TokenCredits != 0 || state.ToolCredits != 0 || state.TotalCredits != 0 || state.UnsettledCredits != 0 {
		t.Fatalf("failed release must be zero money: %+v", state)
	}
	if !state.TokenFinalized || state.InvocationSucceeded {
		t.Fatalf("release must record a zero, unsuccessful intent: finalized=%v succeeded=%v",
			state.TokenFinalized, state.InvocationSucceeded)
	}
	if state.Revision != 1 {
		t.Fatalf("release must be revision 1, got %d", state.Revision)
	}
	// Audit retention: the captured price and the considered tool are still there.
	if prePGFrozenVersion(state) != "P1-frozen" || len(state.Prices) == 0 {
		t.Fatalf("frozen price audit lost: version=%q prices=%d", prePGFrozenVersion(state), len(state.Prices))
	}
	if len(state.LineItems) == 0 {
		t.Fatalf("the considered tool must remain auditable, line_items=%d", len(state.LineItems))
	}
	// No funds moved at all: nothing was reserved, nothing refunded, no debt.
	if got := prePGWalletBalance(t, db, id); got != walletAfterHold {
		t.Fatalf("wallet moved on a zero release: before=%d after=%d", walletAfterHold, got)
	}
	if n := prePGWalletTxCount(t, db, attemptID); n != 0 {
		t.Fatalf("zero release must not write a funds transaction, got %d", n)
	}
	if int64(bill.OfficialPoints) != 0 || int64(bill.PrivatePoints) != 0 || int64(bill.TotalPoints) != 0 {
		t.Fatalf("zero release must not bill points: official=%d private=%d total=%d",
			int64(bill.OfficialPoints), int64(bill.PrivatePoints), int64(bill.TotalPoints))
	}
	if bill.RemoteDeductionID != nil && *bill.RemoteDeductionID != "" {
		t.Fatalf("private traffic must not carry a Console deduction: %v", *bill.RemoteDeductionID)
	}
	var remoteEntries int64
	db.Model(&BillingAttemptEntry{}).Where("attempt_id = ? AND ledger_type = ?", attemptID, "remote").Count(&remoteEntries)
	if remoteEntries != 0 {
		t.Fatalf("private traffic must not write remote ledger entries, got %d", remoteEntries)
	}

	// A replay of the same release is idempotent and cannot move funds again.
	replay := prePGToolContext(t, id, attemptID, requestID, "P1-frozen")
	replay.USDToCNYRate = capturedRate
	replay.Status = "error"
	replay.ActualCredits = 0
	if err := svc.Settle(t.Context(), replay); err != nil {
		t.Fatalf("replayed release: %v", err)
	}
	_, afterReplay := prePGBill(t, db, attemptID)
	if afterReplay.Revision != 1 {
		t.Fatalf("replay changed the release revision: %d", afterReplay.Revision)
	}
	if got := prePGWalletBalance(t, db, id); got != walletAfterHold {
		t.Fatalf("replay moved the wallet: %d", got)
	}
	fmt.Printf("T1_PRIVATE_RELEASE_OK attempt=%s attempt_status=%s bill_status=%s revision=%d token=%d tool=%d total=%d wallet=%d wallet_tx=%d remote_id=%v\n",
		attemptID, attempt.Status, bill.Status, state.Revision, state.TokenCredits, state.ToolCredits,
		state.TotalCredits, prePGWalletBalance(t, db, id), prePGWalletTxCount(t, db, attemptID), bill.RemoteDeductionID)
}

// ---------------------------------------------------------------------------
// T1 private, case B: an unquoted zero hold left in flight (the request never
// completed) is handed to the *running service's* local recovery worker.
// ---------------------------------------------------------------------------

type prePGParked struct {
	Label        string    `json:"label"`
	OrgID        uuid.UUID `json:"org_id"`
	RouteID      uuid.UUID `json:"route_id"`
	APIKeyID     uuid.UUID `json:"api_key_id"`
	ModelID      uuid.UUID `json:"model_id"`
	ProviderID   uuid.UUID `json:"provider_id"`
	AttemptID    string    `json:"attempt_id"`
	RequestID    string    `json:"request_id"`
	WalletAtPark int64     `json:"wallet_at_park"`
	ParkedAt     string    `json:"parked_at"`
}

func TestT1PrivateWorkerPhase1ParksUnquotedZeroHold(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T1_STATE")
	svc := NewBillingService(db, nil, nil, nil)
	id := prePGSeedIdentity(t, db, "t1b")
	attemptID, requestID := "rt2t1-zerohold-inflight-a1", "rt2t1-zerohold-inflight-r1"
	prePGCleanupAttempt(t, db, attemptID)

	bc := prePGToolContext(t, id, attemptID, requestID, "P1-frozen")
	if err := svc.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct (in-flight zero hold): %v", err)
	}
	// The request never completed. Make the attempt old enough that the
	// service's own stale-pre-deduct sweeper considers it; only this fixture's
	// own row is backdated, no production timeout is changed.
	if err := db.Exec(`UPDATE billing_attempts SET updated_at = now() - interval '30 minutes' WHERE attempt_id = ?`, attemptID).Error; err != nil {
		t.Fatalf("backdate attempt: %v", err)
	}
	_, held := prePGBill(t, db, attemptID)
	if held.Revision != 0 || held.TokenFinalized || held.TotalCredits != 0 {
		t.Fatalf("in-flight zero hold must be unquoted and zero: %+v", held)
	}
	parked := prePGParked{
		Label: id.Label, OrgID: id.OrgID, RouteID: id.RouteID, APIKeyID: id.APIKeyID,
		ModelID: id.ModelID, ProviderID: id.ProviderID,
		AttemptID: attemptID, RequestID: requestID,
		WalletAtPark: prePGWalletBalance(t, db, id),
		ParkedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	blob, _ := json.MarshalIndent(parked, "", "  ")
	if err := os.WriteFile(statePath, blob, 0o600); err != nil {
		t.Fatalf("park state: %v", err)
	}
	fmt.Printf("T1_PRIVATE_PHASE1_PARKED attempt=%s attempt_status=%s revision=%d finalized=%v wallet=%d parked_at=%s\n",
		attemptID, prePGAttempt(t, db, attemptID).Status, held.Revision, held.TokenFinalized, parked.WalletAtPark, parked.ParkedAt)
}

func TestT1PrivateWorkerPhase2RetainsThenReleases(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T1_STATE")
	blob, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read parked state: %v", err)
	}
	var parked prePGParked
	if err := json.Unmarshal(blob, &parked); err != nil {
		t.Fatalf("decode parked state: %v", err)
	}
	id := prePGIdentity{Label: parked.Label, OrgID: parked.OrgID, RouteID: parked.RouteID,
		APIKeyID: parked.APIKeyID, ModelID: parked.ModelID, ProviderID: parked.ProviderID}

	// The service worker has swept at least twice by now. The unquoted zero hold
	// still has no completion evidence, so the design keeps the identity and the
	// audit instead of manufacturing a zero quote; nothing may have moved.
	attempt := prePGAttempt(t, db, parked.AttemptID)
	bill, state := prePGBill(t, db, parked.AttemptID)
	if attempt.Status != billingAttemptStatusPre {
		t.Fatalf("in-flight zero hold must not be terminalised by the worker: status=%s", attempt.Status)
	}
	if bill.Status != usageBillStatusPartial || state.Revision != 0 || state.TokenFinalized {
		t.Fatalf("worker changed the unquoted hold: bill=%s revision=%d finalized=%v",
			bill.Status, state.Revision, state.TokenFinalized)
	}
	if got := prePGWalletBalance(t, db, id); got != parked.WalletAtPark {
		t.Fatalf("retention moved funds: before=%d after=%d", parked.WalletAtPark, got)
	}
	if n := prePGWalletTxCount(t, db, parked.AttemptID); n != 0 {
		t.Fatalf("retention wrote a funds transaction: %d", n)
	}
	if prePGFrozenVersion(state) != "P1-frozen" || len(state.Prices) == 0 {
		t.Fatalf("retention lost the frozen price audit: version=%q prices=%d", prePGFrozenVersion(state), len(state.Prices))
	}

	// The retained identity is not a dead end: the ordinary release still turns
	// it into the failed zero-amount terminal state.
	svc := NewBillingService(db, nil, nil, nil)
	release := &BillingContext{
		OrganizationID: parked.OrgID.String(), AttemptID: parked.AttemptID, RequestID: parked.RequestID,
		BillingLane: UsageBillingLanePrivate, ToolPostpaid: true,
	}
	// Rebuild the request projections from the bill, as the service's own
	// recovery path does, so the frozen currency snapshot is reused.
	restoreToolBillContext(release, bill, attempt)
	release.Status = "error"
	release.ActualCredits = 0
	release.ToolPostpaid = true
	release.ToolSettlement = state
	if err := svc.Settle(t.Context(), release); err != nil {
		t.Fatalf("release retained zero hold: %v", err)
	}
	attempt = prePGAttempt(t, db, parked.AttemptID)
	bill, state = prePGBill(t, db, parked.AttemptID)
	if attempt.Status != billingAttemptStatusRolledBack || bill.Status != usageBillStatusFailed {
		t.Fatalf("release after retention = attempt %s / bill %s, want ROLLED_BACK / failed",
			attempt.Status, bill.Status)
	}
	if state.Revision != 1 || state.TokenCredits != 0 || state.ToolCredits != 0 || state.TotalCredits != 0 {
		t.Fatalf("released zero hold is not a zero terminal record: %+v", state)
	}
	if got := prePGWalletBalance(t, db, id); got != parked.WalletAtPark {
		t.Fatalf("release after retention moved funds: %d", got)
	}
	fmt.Printf("T1_PRIVATE_PHASE2_OK attempt=%s retained_then_status=%s bill=%s revision=%d token=%d tool=%d wallet=%d wallet_tx=%d\n",
		parked.AttemptID, attempt.Status, bill.Status, state.Revision, state.TokenCredits,
		state.ToolCredits, prePGWalletBalance(t, db, id), prePGWalletTxCount(t, db, parked.AttemptID))
}

// ---------------------------------------------------------------------------
// T2 private: token frozen at the first settle, late tool fact adds only the
// difference, and the running Pre worker is what performs the correction.
// ---------------------------------------------------------------------------

type prePGT2Parked struct {
	prePGParked
	FrozenVersion      string `json:"frozen_version"`
	TokenCredits       int64  `json:"token_credits"`
	WalletAfterT1      int64  `json:"wallet_after_token"`
	PendingEventID     string `json:"pending_event_id"`
	DedupeKey          string `json:"dedupe_key"`
	RevisionBeforeLate int64  `json:"revision_before_late_fact"`
	WorkerRevision     int64  `json:"worker_revision"`
	WorkerTotal        int64  `json:"worker_total"`
	WorkerWallet       int64  `json:"worker_wallet"`
}

func TestT2PrivatePhase1SettlesTokenOnly(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T2_STATE")
	svc := NewBillingService(db, nil, nil, nil)
	id := prePGSeedIdentity(t, db, "t2")
	attemptID, requestID := "rt2t2-late-delta-a1", "rt2t2-late-delta-r1"
	dedupeKey := "rt2t2-dedupe-" + attemptID
	prePGCleanupAttempt(t, db, attemptID)

	bc := prePGToolContext(t, id, attemptID, requestID, "P1-frozen")
	if err := svc.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct: %v", err)
	}
	// Only an observation exists at completion time: the tool has not reported yet.
	channel := id.RouteID
	observation := &toolusage.Event{
		OrganizationID: id.OrgID, RequestID: requestID, AttemptID: attemptID,
		EventID: "rt2t2-event-" + attemptID, BillingLane: "private",
		Tool: toolusage.ToolWebSearch, ProviderName: "openai", ChannelID: &channel,
		Quantity: 0, Unit: "call", Status: toolusage.StatusPending,
		UsageSource: toolusage.UsageSourceMissing, DedupeKey: dedupeKey,
	}
	if _, err := toolusage.NewRepository(db).Record(t.Context(), observation); err != nil {
		t.Fatalf("record pending observation: %v", err)
	}

	// Same request context as the hold, so the currency snapshot captured at
	// pre-deduction is reused exactly like the request finalizer does.
	bc.ActualCredits = prePGTokenCredits
	bc.TotalUSD = decimal.RequireFromString(prePGTokenUSD)
	if err := svc.Settle(t.Context(), bc); err != nil {
		t.Fatalf("token-only settle: %v", err)
	}
	bill, state := prePGBill(t, db, attemptID)
	if state.TokenCredits != prePGTokenCredits || state.ToolCredits != 0 || state.TotalCredits != prePGTokenCredits {
		t.Fatalf("token-only settle wrong: %+v", state)
	}
	if state.Revision != 1 || state.UsageState != "pending" {
		t.Fatalf("token-only settle must be revision 1 pending, got revision=%d state=%s", state.Revision, state.UsageState)
	}
	if int64(bill.PrivatePoints) != prePGTokenCredits {
		t.Fatalf("bill private points = %d, want %d", int64(bill.PrivatePoints), prePGTokenCredits)
	}
	walletAfter := prePGWalletBalance(t, db, id)
	if walletAfter != 100000-prePGTokenCredits {
		t.Fatalf("wallet after token-only settle = %d, want %d", walletAfter, 100000-prePGTokenCredits)
	}
	frozen := ""
	for _, capture := range state.Prices {
		frozen = capture.PricingVersion
	}
	if frozen == "" {
		t.Fatalf("the attempt must capture a pricing version")
	}
	parked := prePGT2Parked{
		prePGParked: prePGParked{
			Label: id.Label, OrgID: id.OrgID, RouteID: id.RouteID, APIKeyID: id.APIKeyID,
			ModelID: id.ModelID, ProviderID: id.ProviderID,
			AttemptID: attemptID, RequestID: requestID,
			WalletAtPark: walletAfter, ParkedAt: time.Now().UTC().Format(time.RFC3339),
		},
		FrozenVersion: frozen, TokenCredits: state.TokenCredits,
		WalletAfterT1: walletAfter, PendingEventID: observation.EventID, DedupeKey: dedupeKey,
	}
	blob, _ := json.MarshalIndent(parked, "", "  ")
	if err := os.WriteFile(statePath, blob, 0o600); err != nil {
		t.Fatalf("park state: %v", err)
	}
	fmt.Printf("T2_PRIVATE_PHASE1_OK attempt=%s token=%d tool=%d revision=%d wallet=%d frozen_version=%s\n",
		attemptID, state.TokenCredits, state.ToolCredits, state.Revision, walletAfter, frozen)
}

// TestT2PrivatePhase2DeliversLateFactOnly matures the same deduplicated fact and
// stops. It never settles or reconciles: the running service must do that.
func TestT2PrivatePhase2DeliversLateFactOnly(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T2_STATE")
	blob, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read parked state: %v", err)
	}
	var parked prePGT2Parked
	if err := json.Unmarshal(blob, &parked); err != nil {
		t.Fatalf("decode parked state: %v", err)
	}
	channel := parked.RouteID
	late := &toolusage.Event{
		OrganizationID: parked.OrgID, RequestID: parked.RequestID, AttemptID: parked.AttemptID,
		EventID: parked.PendingEventID, BillingLane: "private",
		Tool: toolusage.ToolWebSearch, ProviderName: "openai", ChannelID: &channel,
		Quantity: prePGLateQuantity, Unit: "call", Status: toolusage.StatusSucceeded,
		UsageSource: toolusage.UsageSourceProviderReported, DedupeKey: parked.DedupeKey,
	}
	_, before := prePGBill(t, db, parked.AttemptID)
	if before.TokenCredits != parked.TokenCredits || before.ToolCredits != 0 {
		t.Fatalf("the late fact must not be charged before its release: token=%d tool=%d",
			before.TokenCredits, before.ToolCredits)
	}
	stored, err := toolusage.NewRepository(db).Record(t.Context(), late)
	if err != nil {
		t.Fatalf("release late fact: %v", err)
	}
	if stored.Status != toolusage.StatusSucceeded || stored.Quantity != prePGLateQuantity {
		t.Fatalf("late fact not matured: status=%s quantity=%d", stored.Status, stored.Quantity)
	}
	_, after := prePGBill(t, db, parked.AttemptID)
	if after.ToolCredits != 0 {
		t.Fatalf("the release step itself must not charge a tool: %d", after.ToolCredits)
	}
	parked.RevisionBeforeLate = after.Revision
	blob2, _ := json.MarshalIndent(parked, "", "  ")
	if err := os.WriteFile(statePath, blob2, 0o600); err != nil {
		t.Fatalf("update parked state: %v", err)
	}
	fmt.Printf("T2_PRIVATE_PHASE2_FACT_RELEASED attempt=%s event=%s quantity=%d revision_before_worker=%d token=%d tool=%d at=%s\n",
		parked.AttemptID, stored.EventID, stored.Quantity, after.Revision,
		after.TokenCredits, after.ToolCredits, time.Now().UTC().Format(time.RFC3339))
}

func TestT2PrivatePhase3AssertsWorkerDelta(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T2_STATE")
	blob, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read parked state: %v", err)
	}
	var parked prePGT2Parked
	if err := json.Unmarshal(blob, &parked); err != nil {
		t.Fatalf("decode parked state: %v", err)
	}
	id := prePGIdentity{Label: parked.Label, OrgID: parked.OrgID, RouteID: parked.RouteID,
		APIKeyID: parked.APIKeyID, ModelID: parked.ModelID, ProviderID: parked.ProviderID}

	bill, state := prePGBill(t, db, parked.AttemptID)
	if state.Revision <= parked.RevisionBeforeLate {
		t.Fatalf("the service worker has not corrected the late fact yet: revision=%d (before=%d)",
			state.Revision, parked.RevisionBeforeLate)
	}
	// Independent decimal expectation from the frozen capture only.
	var capture toolPriceCapture
	for _, c := range state.Prices {
		capture = c
	}
	wantTool := prePGExpectedToolCredits(t, capture, toolusage.ToolWebSearch, prePGLateQuantity)
	if state.ToolCredits != wantTool {
		t.Fatalf("tool credits = %d, independent expectation = %d", state.ToolCredits, wantTool)
	}
	if state.TokenCredits != parked.TokenCredits {
		t.Fatalf("token must stay frozen at %d, got %d", parked.TokenCredits, state.TokenCredits)
	}
	if state.TotalCredits != parked.TokenCredits+wantTool {
		t.Fatalf("total = %d, want token+tool = %d", state.TotalCredits, parked.TokenCredits+wantTool)
	}
	if state.UsageState != "settled" {
		t.Fatalf("late correction must settle the usage state, got %s", state.UsageState)
	}
	if prePGFrozenVersion(state) != parked.FrozenVersion {
		t.Fatalf("frozen pricing version changed: %s -> %s", parked.FrozenVersion, prePGFrozenVersion(state))
	}
	for _, item := range state.LineItems {
		if item.PricingVersion != parked.FrozenVersion {
			t.Fatalf("late correction used version %q, want the frozen %q", item.PricingVersion, parked.FrozenVersion)
		}
	}
	if len(state.Adjustments) == 0 || state.Adjustments[len(state.Adjustments)-1].DeltaCredits != wantTool {
		t.Fatalf("the correction must be recorded as a tool-only adjustment: %+v", state.Adjustments)
	}
	// Only the difference moved on the wallet.
	wantWallet := parked.WalletAfterT1 - wantTool
	if got := prePGWalletBalance(t, db, id); got != wantWallet {
		t.Fatalf("wallet = %d, want %d (token already paid, plus the tool difference)", got, wantWallet)
	}
	if int64(bill.PrivatePoints) != parked.TokenCredits+wantTool || int64(bill.TotalPoints) != parked.TokenCredits+wantTool {
		t.Fatalf("bill points do not balance the wallet: private=%d total=%d", int64(bill.PrivatePoints), int64(bill.TotalPoints))
	}
	if int64(bill.OfficialPoints) != 0 {
		t.Fatalf("private traffic must not consume official points, got %d", int64(bill.OfficialPoints))
	}
	if bill.RemoteDeductionID != nil && *bill.RemoteDeductionID != "" {
		t.Fatalf("private traffic must not carry a Console deduction: %v", *bill.RemoteDeductionID)
	}
	attempt := prePGAttempt(t, db, parked.AttemptID)
	if attempt.Status != billingAttemptStatusSettled {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusSettled)
	}
	parked.WorkerRevision, parked.WorkerTotal = state.Revision, state.TotalCredits
	parked.WorkerWallet = prePGWalletBalance(t, db, id)
	blob2, _ := json.MarshalIndent(parked, "", "  ")
	if err := os.WriteFile(statePath, blob2, 0o600); err != nil {
		t.Fatalf("record worker result: %v", err)
	}
	fmt.Printf("T2_PRIVATE_PHASE3_OK attempt=%s revision=%d token=%d tool=%d total=%d frozen=%s wallet=%d bill_private_points=%d adjustments=%d\n",
		parked.AttemptID, state.Revision, state.TokenCredits, state.ToolCredits, state.TotalCredits,
		prePGFrozenVersion(state), prePGWalletBalance(t, db, id), int64(bill.PrivatePoints), len(state.Adjustments))
}

// TestT2PrivatePhase4Idempotent re-reads the state after further worker sweeps.
func TestT2PrivatePhase4Idempotent(t *testing.T) {
	db := prePGOpen(t)
	statePath := prePGStatePath(t, "PRE_PG_TEST_T2_STATE")
	blob, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read parked state: %v", err)
	}
	var parked prePGT2Parked
	if err := json.Unmarshal(blob, &parked); err != nil {
		t.Fatalf("decode parked state: %v", err)
	}
	id := prePGIdentity{Label: parked.Label, OrgID: parked.OrgID, RouteID: parked.RouteID,
		APIKeyID: parked.APIKeyID, ModelID: parked.ModelID, ProviderID: parked.ProviderID}
	bill, state := prePGBill(t, db, parked.AttemptID)
	if state.Revision != parked.WorkerRevision || state.TotalCredits != parked.WorkerTotal {
		t.Fatalf("repeated recovery changed the settlement: revision=%d (want %d) total=%d (want %d)",
			state.Revision, parked.WorkerRevision, state.TotalCredits, parked.WorkerTotal)
	}
	if int64(bill.PrivatePoints) != parked.WorkerTotal {
		t.Fatalf("repeated recovery changed the bill: private_points=%d want %d",
			int64(bill.PrivatePoints), parked.WorkerTotal)
	}
	if got := prePGWalletBalance(t, db, id); got != parked.WorkerWallet {
		t.Fatalf("repeated recovery moved the wallet: %d want %d", got, parked.WorkerWallet)
	}
	if n := prePGWalletTxCount(t, db, parked.AttemptID); n != 2 {
		t.Fatalf("expected exactly two wallet transactions (token, tool difference), got %d", n)
	}
	fmt.Printf("T2_PRIVATE_PHASE4_OK attempt=%s revision=%d total=%d wallet=%d wallet_tx=%d private_points=%d idempotent=true\n",
		parked.AttemptID, state.Revision, state.TotalCredits,
		prePGWalletBalance(t, db, id), prePGWalletTxCount(t, db, parked.AttemptID), int64(bill.PrivatePoints))
}
