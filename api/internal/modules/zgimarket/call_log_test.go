package zgimarket

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestEveryMarketCallIsLoggedWithItsInputOutputAndCharge(t *testing.T) {
	fixture := newToolsFixture(t, true)
	fixture.market.items = true
	fixture.market.charges = map[string]map[string]any{
		"api-items-standard/v1/items/a": {"state": "charged", "amount_micros": 2000, "lines": []map[string]any{
			{"meter": "request", "unit_name": "", "quantity": 1, "amount_micros": 2000},
		}},
		"api-items-standard/v1/items/broke": {"state": "pending", "amount_micros": 0, "lines": []any{}},
	}
	ctx := context.Background()
	require.NoError(t, fixture.db.Exec(`ALTER TABLE agents ADD COLUMN name text`).Error)
	require.NoError(t, fixture.db.Exec(`UPDATE agents SET name = 'Weather agent'`).Error)
	status, answer := fixture.send(t, http.MethodPut, "tools/subjects/agent/"+fixture.agent.String(),
		`{"items":[{"listing_id":7},{"listing_id":20,"offering_slug":"items-standard"}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	listed, err := fixture.tools.SubjectTools(ctx, fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	require.Len(t, listed, 4)

	manager := tools.NewToolManager(nil)
	manager.SetMarketToolSource(fixture.tools)
	engine := tools.NewToolEngine(manager)
	invoke := func(tool SubjectTool, parameters map[string]interface{}) {
		_, _ = engine.Invoke(ctx, tools.InvokeRequest{ //nolint:errcheck // the log is what is checked
			ProviderType: tools.ToolProviderTypeMarket, ProviderID: tool.ProviderID, ToolName: tool.Name,
			TenantID: fixture.org.String(), Parameters: parameters,
		})
	}
	invoke(listed[0], map[string]interface{}{"topic": "rain"})
	invoke(listed[2], map[string]interface{}{"id": "a"})
	invoke(listed[2], map[string]interface{}{"id": "broke"})

	var logs []CallLog
	require.NoError(t, fixture.db.Order("created_at ASC").Find(&logs).Error)
	require.Len(t, logs, 3)
	mcp, api, broke := logs[0], logs[1], logs[2]
	assert.Equal(t, []any{ItemKindMCP, "wiki", "lookup", `{"topic":"rain"}`, "lookup:rain", CallSucceeded},
		[]any{mcp.ItemKind, mcp.ListingSlug, mcp.ToolName, mcp.Input, mcp.Output, mcp.Status})
	assert.Equal(t, "mcp-tools/call", mcp.RequestID, "the tool call's own request id, not the handshake's")
	assert.Equal(t, []any{SubjectAgent, fixture.agent, CallChargePending}, []any{mcp.SubjectType, mcp.SubjectID, mcp.ChargeState})
	assert.Equal(t, []any{ItemKindAPI, "items", "items-standard", CallSucceeded, "api-items-standard/v1/items/a"},
		[]any{api.ItemKind, api.ListingSlug, api.OfferingSlug, api.Status, api.RequestID})
	assert.Equal(t, CallToolError, broke.Status)
	assert.Contains(t, broke.Output, "402 BILLING.INSUFFICIENT_BALANCE")

	// Reading the logs asks the market what the pending calls came to, once they are a moment old.
	fixture.tools.now = func() time.Time { return time.Now().Add(time.Minute) }
	from, to := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10), strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	status, answer = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to, "")
	require.Equal(t, http.StatusOK, status, answer)
	page := answer["data"].(map[string]any)
	assert.Equal(t, float64(3), page["total"])
	summary := page["summary"].(map[string]any)
	assert.Equal(t, []any{float64(3), float64(2), float64(2000), float64(1)},
		[]any{summary["calls"], summary["succeeded"], summary["charge_micros"], summary["pending"]})
	rows := page["data"].([]any)
	byRequest := map[string]map[string]any{}
	for _, row := range rows {
		byRequest[row.(map[string]any)["request_id"].(string)] = row.(map[string]any)
	}
	assert.Equal(t, CallChargeNone, byRequest["mcp-tools/call"]["charge_state"])
	assert.Equal(t, "Weather agent", byRequest["mcp-tools/call"]["subject_name"])
	assert.Equal(t, CallChargeCharged, byRequest["api-items-standard/v1/items/a"]["charge_state"])
	assert.Len(t, byRequest["api-items-standard/v1/items/a"]["charge_lines"], 1)
	assert.Equal(t, CallChargePending, byRequest["api-items-standard/v1/items/broke"]["charge_state"])

	// A settled call is not asked for again.
	asked := len(fixture.market.calls)
	status, _ = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to+"&status=tool_error", "")
	require.Equal(t, http.StatusOK, status)
	assert.Len(t, fixture.market.calls, asked+1, "only the call still pending is asked for")

	for _, bad := range []string{"call-logs", "call-logs?from=" + to + "&to=" + from, "call-logs?from=0&to=" + to, "call-logs?from=" + from + "&to=" + to + "&status=lost"} {
		status, _ = fixture.send(t, http.MethodGet, bad, "")
		assert.Equal(t, http.StatusBadRequest, status, bad)
	}
}

func TestOnlyAdministratorsReadTheCallLogs(t *testing.T) {
	fixture := newToolsFixture(t, false)
	status, _ := fixture.send(t, http.MethodGet, "call-logs?from=0&to=100", "")
	assert.Equal(t, http.StatusForbidden, status)
}
