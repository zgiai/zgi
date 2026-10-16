package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func t4Fixture(t *testing.T) (*t9BillingFixture, *BillingContext) {
	t.Helper()
	f := newT9BillingFixture(t)
	sqlDB, _ := f.db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := f.db.AutoMigrate(&toolusage.Event{}); err != nil {
		t.Fatal(err)
	}
	bc := f.billingContext("t4-"+uuid.NewString(), 20)
	bc.ToolPostpaid = true
	bc.Status = "success"
	raw := json.RawMessage(`{"schema_version":1,"billing_lane":"private","pricing_version":"old","tools":{"default":{"web_search":{"amount":"0.00003","currency":"USD","unit":"call","per_quantity":1},"code_interpreter":{"amount":"0.00004","currency":"USD","unit":"session","per_quantity":1}}}}`)
	state := &toolSettlement{SchemaVersion: 1, BillingLane: "private", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.AttemptID, UsageState: "pending", Requested: []string{"web_search"}, Prices: map[string]toolPriceCapture{f.channelID.String(): captureToolPrices(raw, f.channelID.String(), "7.2", 1000, "charge")}, LineItems: []toolSettlementLine{}, EventIDs: []string{}, Adjustments: []toolSettlementAdjustment{}}
	putToolSettlement(bc, state)
	return f, bc
}

func t4Record(t *testing.T, f *t9BillingFixture, bc *BillingContext, status string, quantity int64) *toolusage.Event {
	t.Helper()
	source := toolusage.UsageSourceProviderReported
	if status == toolusage.StatusPending {
		source = toolusage.UsageSourceMissing
	}
	e := &toolusage.Event{OrganizationID: f.orgID, RequestID: bc.RequestID, AttemptID: bc.AttemptID, EventID: "event-" + bc.AttemptID, BillingLane: "private", Tool: toolusage.ToolWebSearch, ProviderName: "test", ChannelID: &f.channelID, Quantity: quantity, Unit: "call", Status: status, UsageSource: source, DedupeKey: "dedupe-" + bc.AttemptID}
	stored, err := toolusage.NewRepository(f.db).Record(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func t4Bill(t *testing.T, f *t9BillingFixture, bc *BillingContext) (*UsageBill, *toolSettlement) {
	t.Helper()
	var bill UsageBill
	if err := f.db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatal(err)
	}
	state, err := decodeToolSettlement(bill.PricingSnapshot)
	if err != nil || state == nil {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	return &bill, state
}

func TestT4PrivateMergedSettlementAndConcurrentReplay(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	if n := f.countWalletTransactions(t, bc.AttemptID, channelWalletTxTypePreDeduct); n != 1 {
		t.Fatalf("token reservation transactions=%d, want 1", n)
	}
	t4Record(t, f, bc, toolusage.StatusSucceeded, 2)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 120)
	f.assertAPIKeyQuota(t, 80, 20)
	_, state := t4Bill(t, f, bc)
	if state.TokenCredits != 20 || state.ToolCredits != 60 || state.TotalCredits != 80 || state.Revision != 1 || state.UsageState != "settled" {
		t.Fatalf("%+v", state)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); copy := *bc; errs <- f.service.Settle(ctx, &copy) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.assertWalletBalance(t, 120)
	f.assertAPIKeyQuota(t, 80, 20)
	if n := f.countWalletTransactions(t, bc.AttemptID, channelWalletTxTypeSettleAdjustment); n != 1 {
		t.Fatalf("transactions=%d", n)
	}
	// A failed/zero callback is a release request, never a refund of success.
	bc.Status, bc.ActualCredits = "error", 0
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 120)
	if err := f.service.PreDeduct(ctx, bc); err == nil {
		t.Fatal("repeated prededuct must not reset entries")
	}
	_, after := t4Bill(t, f, bc)
	if after.Revision != 1 {
		t.Fatal("replay changed revision")
	}
}

func TestT4LateUsageKeepsFrozenPricesAndTokens(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	e := t4Record(t, f, bc, toolusage.StatusPending, 0)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	_, state := t4Bill(t, f, bc)
	if state.UsageState != "pending" || state.TotalCredits != 20 {
		t.Fatalf("%+v", state)
	}
	if _, err := toolusage.NewRepository(f.db).ExpirePending(ctx, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	_, state = t4Bill(t, f, bc)
	if state.FreeReason != "usage_timeout" || state.TokenCredits != 20 {
		t.Fatalf("%+v", state)
	}
	e.Quantity, e.Status, e.UsageSource = 2, toolusage.StatusSucceeded, toolusage.UsageSourceProviderReported
	if _, err := toolusage.NewRepository(f.db).Record(ctx, e); err != nil {
		t.Fatal(err)
	}
	// The worker has only the durable snapshot; no current model table exists.
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 120)
	f.assertAPIKeyQuota(t, 80, 20)
	_, state = t4Bill(t, f, bc)
	if state.TokenCredits != 20 || state.ToolCredits != 60 || state.Revision != 3 || len(state.Adjustments) != 2 || state.Adjustments[1].DeltaCredits != 60 {
		t.Fatalf("%+v", state)
	}
	if state.Prices[f.channelID.String()].PricingVersion != "old" {
		t.Fatal("price was not frozen")
	}
}

func TestT4FreeReasonsAndAggregateRounding(t *testing.T) {
	for _, tc := range []struct {
		name, amount, mode, rate, currency, wantReason string
		qty, per, want                                 int64
	}{
		{"aggregate", "0.0004", "charge", "7", "USD", "", 1, 1000, 1},
		{"observe", "1", "observe", "7", "USD", "observation_mode", 1, 1, 0},
		{"zero", "0", "charge", "7", "USD", "zero_price", 1, 1, 0},
		{"conversion", "1", "charge", "0", "CNY", "missing_conversion", 1, 1, 0},
		{"cny", "0.000007", "charge", "7", "CNY", "", 1, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, bc := t4Fixture(t)
			cap := bc.ToolSettlement.Prices[f.channelID.String()]
			cap.Mode, cap.CNYPerUSD = tc.mode, tc.rate
			p := cap.Resolved["web_search"]
			p.Amount, p.Currency, p.PerQuantity = tc.amount, tc.currency, tc.per
			cap.Resolved["web_search"] = p
			bc.ToolSettlement.Prices[f.channelID.String()] = cap
			bc.ToolSettlement.TokenUSD = "0"
			e := toolusage.Event{EventID: "one", Tool: "web_search", ChannelID: &f.channelID, Quantity: tc.qty, Unit: "call", Status: toolusage.StatusSucceeded, UsageSource: toolusage.UsageSourceProviderReported}
			e2 := e
			e2.EventID = "two"
			state, err := calculateToolSettlement(bc.ToolSettlement, []toolusage.Event{e, e2}, false)
			if err != nil || state.ToolCredits != tc.want || state.FreeReason != tc.wantReason {
				t.Fatalf("%+v %v", state, err)
			}
		})
	}
	for _, raw := range []string{`{}`, `{"tools":{"channels":{"c": {"web_search":null}},"default":{"web_search":{"amount":"1","currency":"USD","unit":"call","per_quantity":1}}}}`, `{"tools":{"default":{"web_search":{"amount":"1","currency":"USD","unit":"unknown","per_quantity":1}}}}`} {
		cap := captureToolPrices(json.RawMessage(raw), "c", "7", 1000, "charge")
		if cap.Resolved["web_search"].FreeReason == "" {
			t.Fatal("invalid/missing/null tool price must be free")
		}
	}
}

func TestT4DebtRestrictionAndRecharge(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	t4Record(t, f, bc, toolusage.StatusSucceeded, 10)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, -120)
	_, state := t4Bill(t, f, bc)
	if state.UsageState != "unsettled" || state.UnsettledCredits != 120 {
		t.Fatalf("%+v", state)
	}
	if err := checkToolDebt(ctx, f.db, bc); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err=%v", err)
	}
	cap := bc.ToolSettlement.Prices[f.channelID.String()]
	cap.Mode = "observe"
	bc.ToolSettlement.Prices[f.channelID.String()] = cap
	if err := checkToolDebt(ctx, f.db, bc); err != nil {
		t.Fatal("free tools restricted", err)
	}
	cap.Mode = "charge"
	bc.ToolSettlement.Prices[f.channelID.String()] = cap
	// Existing channel wallet adjustment restores ACTIVE once funds cover debt.
	if err := f.db.Model(&ChannelWallet{}).Where("channel_id = ?", f.channelID).Updates(map[string]interface{}{"balance": 0, "status": "ACTIVE"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := checkToolDebt(ctx, f.db, bc); err != nil {
		t.Fatal("recharge did not restore", err)
	}
	_, state = t4Bill(t, f, bc)
	if state.UnsettledCredits != 120 {
		t.Fatal("historical debt was rewritten")
	}
}

func TestT4AtomicFailureAndIdentityIsolation(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	t4Record(t, f, bc, toolusage.StatusSucceeded, 2)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.db.Exec(`CREATE TRIGGER fail_t4_bill BEFORE UPDATE ON llm_usage_bills WHEN json_extract(NEW.pricing_snapshot,'$.tool_settlement.revision') > 0 BEGIN SELECT RAISE(ABORT, 'injected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.Settle(ctx, bc); err == nil {
		t.Fatal("expected rollback")
	}
	// The failed settlement transaction must leave the original token hold
	// intact so recovery can finish without redispatching upstream work.
	f.assertWalletBalance(t, 180)
	f.assertAPIKeyQuota(t, 0, 0)
	if err := f.db.Exec("DROP TRIGGER fail_t4_bill").Error; err != nil {
		t.Fatal(err)
	}
	if bc.ActualCredits != 20 || bc.ToolSettlement.Revision != 0 {
		t.Fatal("failed transaction mutated caller pricing")
	}
	_, intent := t4Bill(t, f, bc)
	if !intent.TokenFinalized || intent.TokenCredits != 20 {
		t.Fatal("completion intent was not preserved")
	}
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 120)
	bc.OrganizationID = uuid.NewString()
	if err := f.service.Settle(ctx, bc); err == nil {
		t.Fatal("cross-org attempt accepted")
	}
	bc.OrganizationID = f.orgID.String()
	bc.BillingLane = UsageBillingLanePlatform
	if err := f.service.settlePrivateTools(ctx, bc); err == nil {
		t.Fatal("cross-lane attempt accepted")
	}
}

func TestT4FailedReleaseKeepsSuccessfulFacts(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	t4Record(t, f, bc, toolusage.StatusSucceeded, 2)
	bc.Status = "error"
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 140)
	_, state := t4Bill(t, f, bc)
	if state.ToolCredits != 60 || state.TokenCredits != 0 {
		t.Fatalf("%+v", state)
	}
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 140)
}

func TestT4PrivateConcurrentRequestsReserveTokenFundsAndWholeSubjectQuota(t *testing.T) {
	f, first := t4Fixture(t)
	second := *first
	second.AttemptID, second.RequestID = "t4-concurrent-second", "t4-concurrent-second"
	secondState := *first.ToolSettlement
	secondState.AttemptID, secondState.RequestID, secondState.SettlementID = second.AttemptID, second.RequestID, second.AttemptID
	second.ToolSettlement = &secondState
	putToolSettlement(&second, &secondState)

	contexts := []*BillingContext{first, &second}
	errs := make(chan error, len(contexts))
	var wg sync.WaitGroup
	for _, bc := range contexts {
		bc := bc
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- f.service.PreDeduct(context.Background(), bc)
		}()
	}
	wg.Wait()
	close(errs)
	successes, rejected := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInsufficientQuota):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("successes=%d rejected=%d", successes, rejected)
	}
	f.assertAPIKeyQuota(t, 0, 0)
	f.assertWalletBalance(t, 180)
}

func TestT4RecoveryLoadsDurableSnapshot(t *testing.T) {
	for _, knownQuote := range []bool{false, true} {
		t.Run(fmt.Sprint(knownQuote), func(t *testing.T) {
			f, bc := t4Fixture(t)
			ctx := context.Background()
			if err := f.service.PreDeduct(ctx, bc); err != nil {
				t.Fatal(err)
			}
			t4Record(t, f, bc, toolusage.StatusSucceeded, 2)
			if knownQuote {
				bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
				if err := f.service.freezeToolTokenIntent(ctx, bc); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.service.recoverLocalPredeductAttempt(ctx, bc.AttemptID); err != nil {
				t.Fatal(err)
			}
			want := int64(180)
			if knownQuote {
				want = 120
			}
			f.assertWalletBalance(t, want)
			bill, state := t4Bill(t, f, bc)
			if !knownQuote {
				if state.Revision != 0 || state.TokenFinalized || state.UsageState != "pending" {
					t.Fatal("missing quote was treated as zero usage")
				}
				bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
				if err := f.service.Settle(ctx, bc); err != nil {
					t.Fatal(err)
				}
				f.assertWalletBalance(t, 120)
				bill, state = t4Bill(t, f, bc)
			}
			if state.ToolCredits != 60 || bill.ModelName != bc.ModelName || state.TokenCredits != 20 {
				t.Fatalf("name=%s state=%+v", bill.ModelName, state)
			}
		})
	}
}

func TestT4ObserverAuditsReusedCodeSession(t *testing.T) {
	f, bc := t4Fixture(t)
	svc := &llmGatewayServiceImpl{db: f.db}
	for i := 0; i < 2; i++ {
		attempt := fmt.Sprintf("reuse-%d", i)
		o := toolusage.NewObserver(toolusage.Identity{OrganizationID: f.orgID, RequestID: attempt, AttemptID: attempt, Lane: "private", Provider: "test", ChannelID: f.channelID}, []byte(`{"tools":[{"type":"code_interpreter"}]}`))
		o.Observe([]byte(fmt.Sprintf(`{"id":"r%d","output":[{"id":"ci%d","type":"code_interpreter_call","status":"completed","container_id":"same-session"}]}`, i, i)))
		svc.recordPrivateToolObserver(context.Background(), o, false, bc)
	}
	events, err := toolusage.NewRepository(f.db).ListByAttempt(context.Background(), f.orgID, "private", "reuse-1")
	if err != nil || len(events) != 1 || events[0].Quantity != 0 || events[0].Status != toolusage.StatusFailed {
		t.Fatalf("%+v %v", events, err)
	}
	var details map[string]interface{}
	_ = json.Unmarshal(events[0].Details, &details)
	if details["free_reason"] != "session_reused" || details["related_event_id"] == nil {
		t.Fatalf("%+v", details)
	}
}

func TestT4CaptureBeforeDispatchUsesActualModelAndChannel(t *testing.T) {
	f, bc := t4Fixture(t)
	if err := f.db.Exec("CREATE TABLE llm_custom_models (id text PRIMARY KEY, provider text, name text, input_price decimal, output_price decimal, pricing json)").Error; err != nil {
		t.Fatal(err)
	}
	pricing := string(bc.ToolSettlement.Prices[f.channelID.String()].PricingJSON)
	if err := f.db.Exec("INSERT INTO llm_custom_models VALUES (?, ?, ?, 0, 0, ?)", bc.ModelID, bc.ProviderName, bc.ModelName, pricing).Error; err != nil {
		t.Fatal(err)
	}
	selection := &ProviderSelection{Model: llmmodel.LLMModel{ID: bc.ModelID, Model: bc.ModelName}, ModelSource: PricingModelSourceCustom, OrganizationID: f.orgID}
	svc := &llmGatewayServiceImpl{db: f.db}
	bc.ToolSettlement = nil
	bc.EstimatedCredits = 99
	ctx := withRequestedBillingTools(context.Background(), []byte(`{"tools":[{"type":"web_search_preview"}]}`))
	t.Setenv("LLM_TOOL_BILLING_MODE", "charge")
	t.Setenv("LLM_TOOL_BILLING_CHANNELS", f.channelID.String())
	if err := svc.prepareToolBilling(ctx, bc, selection); err != nil {
		t.Fatal(err)
	}
	if !bc.ToolPostpaid || bc.EstimatedCredits != 99 || bc.ToolSettlement.Prices[f.channelID.String()].Mode != "charge" {
		t.Fatalf("%+v", bc)
	}
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE llm_custom_models SET pricing = '{}' WHERE id = ?", bc.ModelID).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_TOOL_BILLING_MODE", "observe")
	t4Record(t, f, bc, toolusage.StatusSucceeded, 1)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 150)
}

type t4QuotaRPC struct{ recordingQuotaRPCClient }

func (c *t4QuotaRPC) SettleQuota(context.Context, *pb.SettleQuotaRequest, ...grpc.CallOption) (*pb.SettleQuotaResponse, error) {
	return &pb.SettleQuotaResponse{Success: true, ToolSettlementJson: `{"schema_version":1,"billing_lane":"platform"}`}, nil
}

type t4CapabilityQuotaClient struct {
	failingSettleQuotaClient
	supported bool
	err       error
}

func (c t4CapabilityQuotaClient) GetBillingCapabilities(context.Context) (bool, error) {
	return c.supported, c.err
}

func TestToolPostpaidRequiresConsoleCapabilityAndPreservesTokenQuote(t *testing.T) {
	ctx := withRequestedBillingTools(context.Background(), []byte(`{"tools":[{"googleSearch":{}}]}`))
	for _, supported := range []bool{false, true} {
		bc := &BillingContext{UseSystemProvider: true, BillingLane: UsageBillingLanePlatform, EstimatedCredits: 73}
		remote := &RemoteBilling{grpcClient: t4CapabilityQuotaClient{supported: supported}}
		svc := &llmGatewayServiceImpl{billing: remote}
		if err := svc.prepareToolBilling(ctx, bc, nil); err != nil {
			t.Fatal(err)
		}
		if bc.ToolPostpaid != supported || bc.EstimatedCredits != 73 {
			t.Fatalf("supported=%t context=%+v", supported, bc)
		}
	}
	bc := &BillingContext{UseSystemProvider: true, BillingLane: UsageBillingLanePlatform, EstimatedCredits: 73}
	remote := &RemoteBilling{grpcClient: t4CapabilityQuotaClient{err: errors.New("capability transport failed")}}
	if err := (&llmGatewayServiceImpl{billing: remote}).prepareToolBilling(ctx, bc, nil); err == nil || bc.ToolPostpaid {
		t.Fatalf("capability transport failure was treated as a compatible downgrade: context=%+v err=%v", bc, err)
	}
}

func TestRequestedBillingToolsRecognizesGeminiNativeDeclarations(t *testing.T) {
	ctx := withRequestedBillingTools(context.Background(), []byte(`{"tools":[{"googleSearch":{}},{"fileSearch":{}},{"codeExecution":{}}]}`))
	got, _ := ctx.Value(requestedToolsKey{}).([]string)
	if strings.Join(got, ",") != "code_interpreter,file_search,web_search" {
		t.Fatalf("tools=%v", got)
	}
}
func TestT4RPCProjectionPreservesTokenReservation(t *testing.T) {
	rpc := &t4QuotaRPC{}
	client := &QuotaClient{client: rpc}
	if _, err := client.PreDeductQuota(context.Background(), &PreDeductQuotaRequest{ToolPostpaid: true, EstimatedCredits: 73}); err != nil {
		t.Fatal(err)
	}
	if !rpc.request.ToolPostpaid || rpc.request.EstimatedCredits != 73 {
		t.Fatalf("%+v", rpc.request)
	}
	resp, err := client.SettleQuota(context.Background(), &SettleQuotaRequest{})
	if err != nil || resp.ToolSettlementJSON != `{"schema_version":1,"billing_lane":"platform"}` {
		t.Fatalf("%+v %v", resp, err)
	}
}

func TestT4PausePreservesPendingAndLateFacts(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	e := t4Record(t, f, bc, toolusage.StatusPending, 0)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	e.Quantity, e.Status, e.UsageSource = 2, toolusage.StatusSucceeded, toolusage.UsageSourceProviderReported
	if _, err := toolusage.NewRepository(f.db).Record(ctx, e); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLM_TOOL_SETTLEMENT_RECONCILE_PAUSED", "1")
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 180)
	t.Setenv("LLM_TOOL_SETTLEMENT_RECONCILE_PAUSED", "0")
	if err := f.service.reconcilePrivateToolAttempt(ctx, bc.AttemptID); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 120)
}

func TestT4NoPersistedFactTimesOutFree(t *testing.T) {
	f, bc := t4Fixture(t)
	state := bc.ToolSettlement
	state.TokenUSD = "0"
	cap := state.Prices[f.channelID.String()]
	cap.CapturedAt = time.Now().Add(-10 * time.Minute)
	state.Prices[f.channelID.String()] = cap
	next, err := calculateToolSettlement(state, nil, false)
	if err != nil || next.UsageState != "free" || next.FreeReason != "usage_timeout" {
		t.Fatalf("%+v %v", next, err)
	}
}

func TestT4RejectedKeyRetainsFailureAudit(t *testing.T) {
	for _, reason := range []string{"disabled", "exhausted"} {
		t.Run(reason, func(t *testing.T) {
			f, bc := t4Fixture(t)
			changes := map[string]interface{}{"status": "disabled"}
			if reason == "exhausted" {
				changes = map[string]interface{}{"remain_quota": 0}
			}
			if err := f.db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", f.apiKeyID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.service.PreDeduct(context.Background(), bc); err == nil {
				t.Fatal("inactive or exhausted key accepted")
			}
			var attempt BillingAttempt
			if err := f.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.Status != billingAttemptStatusPredeductFailed {
				t.Fatalf("status=%s", attempt.Status)
			}
			f.assertWalletBalance(t, 200)
		})
	}
}

func TestT4PlatformPartialMirrorRetriesSameRevision(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	bc.BillingLane, bc.UseSystemProvider, bc.ToolSettlement = UsageBillingLanePlatform, true, nil
	bc.DeductionID = "console-deduction"
	state := &toolSettlement{SchemaVersion: 1, BillingLane: "platform", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.DeductionID, UsageState: "settled", Revision: 1, TokenCredits: 20, ToolCredits: 30, TotalCredits: 50, TokenUSD: "0.00002", ToolUSD: "0.00003", TotalUSD: "0.00005", Prices: map[string]toolPriceCapture{"console-channel": {CNYPerUSD: "7.2"}}}
	raw, _ := json.Marshal(state)
	values := map[string]interface{}{"tool_settlement": json.RawMessage(raw), "cny_per_usd": "7.2", "new_unknown_field": map[string]interface{}{"integer": json.Number("9007199254740993")}}
	bc.PricingSnapshot = buildPricingSnapshot(values)
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		if err := f.service.upsertAttemptInit(ctx, tx, bc); err != nil {
			return err
		}
		return f.service.upsertUsageBill(ctx, tx, bc, usageBillStatusPartial, nil, nil)
	}); err != nil {
		t.Fatal(err)
	}
	var attempt BillingAttempt
	if err := f.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	remote := &RemoteBilling{localService: f.service}
	result := &settlementQueryResponse{Found: true, OrganizationID: bc.OrganizationID, RequestID: bc.RequestID, AttemptID: bc.AttemptID, SettlementID: bc.DeductionID, DeductionID: bc.DeductionID, SettlementStatus: "SETTLED", SettledCredits: 50, ToolSettlement: raw, PricingSnapshot: json.RawMessage(bc.PricingSnapshot)}
	if err := remote.mirrorQueriedSettlement(ctx, &attempt, result); err != nil {
		t.Fatal(err)
	}
	if err := remote.mirrorQueriedSettlement(ctx, &attempt, result); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 200)
	f.assertAPIKeyQuota(t, 50, 50)
	var bill UsageBill
	if err := f.db.First(&bill, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(bill.PricingSnapshot, &root)
	if !toolJSONEqual(root["tool_settlement"], raw) {
		t.Fatal("authoritative object changed")
	}
}

func TestT4FailureAndSessionReuseAreFree(t *testing.T) {
	for _, reused := range []bool{false, true} {
		t.Run(fmt.Sprint(reused), func(t *testing.T) {
			f, bc := t4Fixture(t)
			ctx := context.Background()
			bc.ToolSettlement.Requested = []string{"code_interpreter"}
			putToolSettlement(bc, bc.ToolSettlement)
			if err := f.service.PreDeduct(ctx, bc); err != nil {
				t.Fatal(err)
			}
			if reused {
				bc.ToolSettlement.Reused = map[string]string{"code_interpreter": "original-event"}
			} else {
				bc.Status = "error"
			}
			if err := f.service.Settle(ctx, bc); err != nil {
				t.Fatal(err)
			}
			_, state := t4Bill(t, f, bc)
			want := "upstream_failed"
			if reused {
				want = "session_reused"
			}
			if state.FreeReason != want || state.ToolCredits != 0 {
				t.Fatalf("%+v", state)
			}
			f.assertWalletBalance(t, 200)
		})
	}
}

func TestT4PlatformMirrorRevisionsNeverDeductFunds(t *testing.T) {
	f, bc := t4Fixture(t)
	ctx := context.Background()
	bc.BillingLane, bc.UseSystemProvider, bc.ToolSettlement = UsageBillingLanePlatform, true, nil
	bc.DeductionID = "console-deduction"
	if err := f.db.Transaction(func(tx *gorm.DB) error {
		var key apikeymodel.TenantAPIKey
		if err := tx.Where("id = ?", f.apiKeyID).First(&key).Error; err != nil {
			return err
		}
		if err := f.service.preDeductSubjectQuota(ctx, tx, bc, &key); err != nil {
			return err
		}
		return f.service.upsertAttemptInit(ctx, tx, bc)
	}); err != nil {
		t.Fatal(err)
	}
	remote := &RemoteBilling{localService: f.service}
	state := &toolSettlement{SchemaVersion: 1, BillingLane: "platform", AttemptID: bc.AttemptID, RequestID: bc.RequestID, SettlementID: bc.DeductionID, UsageState: "free", Revision: 1, TokenUSD: "0", ToolUSD: "0", TotalUSD: "0"}
	raw, _ := json.Marshal(state)
	snapshot := datatypes.JSON(`{"rate":"7.2","rate_source":"console_system","display_currency":"USD"}`)
	if err := remote.mirrorPlatformTools(ctx, bc, raw, snapshot); err != nil {
		t.Fatal(err)
	}
	state.TokenCredits, state.ToolCredits, state.TotalCredits, state.TokenUSD, state.ToolUSD, state.TotalUSD, state.UsageState, state.Revision = 20, 60, 80, "0.00002", "0.00006", "0.00008", "settled", 2
	// Tokens are immutable even across revisions.
	raw, _ = json.Marshal(state)
	if err := remote.mirrorPlatformTools(ctx, bc, raw, snapshot); err == nil {
		t.Fatal("changed tokens accepted")
	}
	state.TokenCredits, state.TotalCredits, state.TokenUSD, state.TotalUSD = 0, 60, "0", "0.00006"
	raw, _ = json.Marshal(state)
	if err := remote.mirrorPlatformTools(ctx, bc, raw, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := remote.mirrorPlatformTools(ctx, bc, raw, snapshot); err != nil {
		t.Fatal(err)
	}
	f.assertWalletBalance(t, 200)
	f.assertAPIKeyQuota(t, 60, 40)
	var count int64
	if err := f.db.Model(&ChannelWalletTransaction{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("fund transactions=%d err=%v", count, err)
	}
	if err := f.db.Model(&toolusage.Event{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("events=%d err=%v", count, err)
	}
	var bill UsageBill
	if err := f.db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatal(err)
	}
	if bill.TotalPoints != 60 || bill.OfficialPoints != 60 || bill.PrivatePoints != 0 {
		t.Fatalf("%+v", bill)
	}
	state.Revision = 1
	raw, _ = json.Marshal(state)
	if err := remote.mirrorPlatformTools(ctx, bc, raw, snapshot); err != nil {
		t.Fatal(err)
	}
	f.assertAPIKeyQuota(t, 60, 40)
}
