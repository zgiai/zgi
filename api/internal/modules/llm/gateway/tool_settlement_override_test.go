package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
)

func t4GlobalModelFixture(t *testing.T) (*t9BillingFixture, *BillingContext, *ProviderSelection) {
	t.Helper()
	f, bc := t4Fixture(t)
	if err := f.db.Exec(`CREATE TABLE llm_models (id text PRIMARY KEY, name text, provider text, input_price decimal, output_price decimal, pricing json)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TABLE llm_model_configs (organization_id text, model_id text, input_price_override decimal, output_price_override decimal, pricing_override json, deleted_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	base := `{"version":2,"schema_version":1,"billing_lane":"platform","pricing_version":"platform-base","rules":[{"id":"chat-in","operation":"chat","meter":"input_token","unit":"per_1m_tokens","price":"1","configured":true}],"tools":{"default":{"web_search":{"amount":"99","currency":"USD","unit":"call","per_quantity":1}}}}`
	if err := f.db.Exec("INSERT INTO llm_models VALUES (?, ?, ?, 1, 2, ?)", bc.ModelID, bc.ModelName, bc.ProviderName, base).Error; err != nil {
		t.Fatal(err)
	}
	selection := &ProviderSelection{OrganizationID: f.orgID, Model: llmmodel.LLMModel{ID: bc.ModelID, Model: bc.ModelName, Provider: bc.ProviderName}, ModelSource: PricingModelSourceGlobal, BillingLane: UsageBillingLanePrivate, RouteID: f.channelID}
	return f, bc, selection
}

func TestT4GlobalModelTenantToolsCapture(t *testing.T) {
	for _, tc := range []struct{ name, channel, wantSource, want string }{
		{"default", "", "default", "0.001"},
		{"channel", `{"web_search":{"amount":"0.002","currency":"USD","unit":"call","per_quantity":1}}`, "channel", "0.002"},
		{"null", `{"web_search":null}`, "free", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, bc, selection := t4GlobalModelFixture(t)
			tools := map[string]interface{}{"default": map[string]interface{}{"web_search": map[string]interface{}{"amount": "0.001", "currency": "USD", "unit": "call", "per_quantity": 1}, "code_interpreter": map[string]interface{}{"amount": "0.003", "currency": "USD", "unit": "session", "per_quantity": 1}}}
			if tc.channel != "" {
				var channel interface{}
				_ = json.Unmarshal([]byte(tc.channel), &channel)
				tools["channels"] = map[string]interface{}{f.channelID.String(): channel}
			}
			override, _ := json.Marshal(map[string]interface{}{"schema_version": 1, "billing_lane": "private", "pricing_version": "tenant-v1", "tools": tools})
			if err := f.db.Exec("INSERT INTO llm_model_configs VALUES (?, ?, NULL, NULL, ?, NULL)", f.orgID, bc.ModelID, string(override)).Error; err != nil {
				t.Fatal(err)
			}
			otherOrg := uuid.New()
			if err := f.db.Exec("INSERT INTO llm_model_configs VALUES (?, ?, NULL, NULL, ?, NULL)", otherOrg, bc.ModelID, `{"schema_version":1,"billing_lane":"private","pricing_version":"other","tools":{"default":{"web_search":{"amount":"7","currency":"USD","unit":"call","per_quantity":1}}}}`).Error; err != nil {
				t.Fatal(err)
			}
			bc.ToolSettlement = nil
			t.Setenv("LLM_TOOL_BILLING_MODE", "charge")
			t.Setenv("LLM_TOOL_BILLING_CHANNELS", f.channelID.String())
			ctx := withRequestedBillingTools(context.Background(), []byte(`{"tools":[{"type":"web_search"},{"type":"code_interpreter"}]}`))
			svc := &llmGatewayServiceImpl{db: f.db}
			if err := svc.prepareToolBilling(ctx, bc, selection); err != nil {
				t.Fatal(err)
			}
			capture := bc.ToolSettlement.Prices[f.channelID.String()]
			p := capture.Resolved["web_search"]
			if capture.PricingVersion != "tenant-v1" || p.Amount != tc.want || p.Source != tc.wantSource || capture.Resolved["code_interpreter"].Amount != "0.003" {
				t.Fatalf("%+v", capture)
			}
			if pricingModelRefFromSelection(selection).ModelID != bc.ModelID {
				t.Fatal("supplier model id was changed")
			}
			selection.OrganizationID = otherOrg
			if err := svc.prepareToolBilling(ctx, bc, selection); err == nil {
				t.Fatal("cross-tenant selection accepted")
			}
			selection.OrganizationID = uuid.Nil
			if err := svc.prepareToolBilling(ctx, bc, selection); err != nil {
				t.Fatal("missing selection organization did not use authenticated owner", err)
			}
		})
	}
}

func TestT4GlobalTenantOverrideFreezesOldRequest(t *testing.T) {
	f, bc, selection := t4GlobalModelFixture(t)
	ctx := context.Background()
	if err := f.db.Model(&ChannelWallet{}).Where("channel_id = ?", f.channelID).Update("balance", 100000).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", f.apiKeyID).Update("quota_limit", nil).Error; err != nil {
		t.Fatal(err)
	}
	override := func(version, amount string) string {
		return fmt.Sprintf(`{"schema_version":1,"billing_lane":"private","pricing_version":%q,"tools":{"default":{"web_search":{"amount":%q,"currency":"USD","unit":"call","per_quantity":1}}}}`, version, amount)
	}
	if err := f.db.Exec("INSERT INTO llm_model_configs VALUES (?, ?, NULL, NULL, ?, NULL)", f.orgID, bc.ModelID, override("tenant-v1", "0.001")).Error; err != nil {
		t.Fatal(err)
	}
	bc.ToolSettlement = nil
	t.Setenv("LLM_TOOL_BILLING_MODE", "charge")
	t.Setenv("LLM_TOOL_BILLING_CHANNELS", f.channelID.String())
	svc := &llmGatewayServiceImpl{db: f.db}
	requestCtx := withRequestedBillingTools(ctx, []byte(`{"tools":[{"type":"web_search"}]}`))
	if err := svc.prepareToolBilling(requestCtx, bc, selection); err != nil {
		t.Fatal(err)
	}
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	e := t4Record(t, f, bc, toolusage.StatusPending, 0)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE llm_model_configs SET pricing_override = ? WHERE organization_id = ? AND model_id = ?", override("tenant-v2", "0.002"), f.orgID, bc.ModelID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := toolusage.NewRepository(f.db).UpdateFact(ctx, f.orgID, "private", e.DedupeKey, 1, toolusage.StatusSucceeded, toolusage.UsageSourceProviderReported, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ReconcilePrivateTools(ctx); err != nil {
		t.Fatal(err)
	}
	_, old := t4Bill(t, f, bc)
	if old.ToolCredits != 1000 || old.Prices[f.channelID.String()].PricingVersion != "tenant-v1" {
		t.Fatalf("%+v", old)
	}
	next := *bc
	next.AttemptID, next.RequestID = "next-attempt", "next-request"
	next.ToolSettlement = nil
	if err := svc.prepareToolBilling(requestCtx, &next, selection); err != nil {
		t.Fatal(err)
	}
	if next.ToolSettlement.Prices[f.channelID.String()].PricingVersion != "tenant-v2" || next.ToolSettlement.Prices[f.channelID.String()].Resolved["web_search"].Amount != "0.002" {
		t.Fatal("new request did not capture new tenant version")
	}
	f.assertWalletBalance(t, 98980)
}

func TestT4TenantOverrideFixDoesNotRepriceCompletedFreeBill(t *testing.T) {
	f, bc, selection := t4GlobalModelFixture(t)
	ctx := context.Background()
	// Reproduce the previously captured token-only pricing document.
	bc.ToolSettlement.Prices[f.channelID.String()] = captureToolPrices(json.RawMessage(`{"rules":[]}`), f.channelID.String(), "7.2", 1000, "charge")
	putToolSettlement(bc, bc.ToolSettlement)
	if err := f.service.PreDeduct(ctx, bc); err != nil {
		t.Fatal(err)
	}
	t4Record(t, f, bc, toolusage.StatusSucceeded, 1)
	bc.ActualCredits, bc.TotalUSD = 20, decimal.RequireFromString("0.00002")
	if err := f.service.Settle(ctx, bc); err != nil {
		t.Fatal(err)
	}
	oldBill, oldState := t4Bill(t, f, bc)
	if oldState.ToolCredits != 0 || oldState.FreeReason != "missing_price" {
		t.Fatalf("%+v", oldState)
	}
	before := string(oldBill.PricingSnapshot)
	if err := f.db.Exec("INSERT INTO llm_model_configs VALUES (?, ?, NULL, NULL, ?, NULL)", f.orgID, bc.ModelID, `{"schema_version":1,"billing_lane":"private","pricing_version":"fixed-v1","tools":{"default":{"web_search":{"amount":"0.001","currency":"USD","unit":"call","per_quantity":1}}}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.ReconcilePrivateTools(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := t4Bill(t, f, bc)
	if string(after.PricingSnapshot) != before {
		t.Fatal("completed free price snapshot was rewritten")
	}
	f.assertWalletBalance(t, 180)
	t.Setenv("LLM_TOOL_BILLING_MODE", "charge")
	t.Setenv("LLM_TOOL_BILLING_CHANNELS", f.channelID.String())
	next := *bc
	next.AttemptID, next.RequestID = "fixed-attempt", "fixed-request"
	next.ToolSettlement = nil
	svc := &llmGatewayServiceImpl{db: f.db}
	if err := svc.prepareToolBilling(withRequestedBillingTools(ctx, []byte(`{"tools":[{"type":"web_search"}]}`)), &next, selection); err != nil {
		t.Fatal(err)
	}
	if next.ToolSettlement.Prices[f.channelID.String()].Resolved["web_search"].Amount != "0.001" {
		t.Fatal("new request did not use corrected tenant override")
	}
}
