package zgimarket

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
	account := uuid.New()
	require.NoError(t, fixture.db.Exec(`CREATE TABLE accounts (id text, name text)`).Error)
	require.NoError(t, fixture.db.Exec(`INSERT INTO accounts VALUES (?, 'Ada')`, account.String()).Error)
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
			UserID: account.String(), ConversationID: "conv-1", MessageID: "msg-1", AppID: fixture.agent.String(),
			RuntimeParameters: map[string]interface{}{"agent_runtime_source": "console"},
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
	assert.Equal(t, []string{"console", account.String(), "conv-1", "msg-1", fixture.agent.String()},
		[]string{mcp.Source, mcp.UserID, mcp.ConversationID, mcp.MessageID, mcp.AppID}, "where the call was made from")
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
	assert.Equal(t, "Ada", byRequest["mcp-tools/call"]["user_name"])
	status, answer = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to+"&source=webapp", "")
	require.Equal(t, http.StatusOK, status, answer)
	assert.Equal(t, float64(0), answer["data"].(map[string]any)["total"], "no call came from a published app")
	assert.Equal(t, CallChargeCharged, byRequest["api-items-standard/v1/items/a"]["charge_state"])
	assert.Len(t, byRequest["api-items-standard/v1/items/a"]["charge_lines"], 1)
	assert.Equal(t, CallChargePending, byRequest["api-items-standard/v1/items/broke"]["charge_state"])

	// A settled call is not asked for again.
	chargeReads := func() int {
		count := 0
		for _, call := range fixture.market.calls {
			if strings.HasPrefix(call, "GET calls/charges") {
				count++
			}
		}
		return count
	}
	asked := chargeReads()
	status, _ = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to+"&status=tool_error", "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, asked+1, chargeReads(), "only the call still pending is asked for")

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

func TestCallsMadeWithACredentialOutsideThisInstanceAreReadFromTheMarket(t *testing.T) {
	fixture := newToolsFixture(t, true)
	now := time.Now()
	old := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	str := func(value string) *string { return &value }
	require.NoError(t, fixture.db.Create(&CallLog{
		ID: uuid.New(), OrganizationID: fixture.org, SubjectType: SubjectAgent, SubjectID: fixture.agent,
		ItemKind: ItemKindMCP, ListingSlug: "wiki", ToolName: "lookup", Status: CallSucceeded,
		RequestID: "req-made-here", ChargeState: CallChargeCharged, ChargeLines: []CallChargeLine{}, CreatedAt: now.Add(-time.Hour),
	}).Error)
	fixture.market.records = []map[string]any{
		{"id": 1, "request_id": "req-init", "route": "mcp", "operation": "initialize", "target": "wiki", "outcome": "succeeded", "occurred_at": old},
		{"id": 2, "request_id": "req-made-here", "route": "mcp", "operation": "tools/call", "tool_name": "lookup", "target": "wiki", "outcome": "succeeded", "occurred_at": old},
		{"id": 3, "request_id": "req-script", "route": "api", "method": "GET", "operation": "/v1/items/7", "credential_id": 9,
			"target": "items-standard", "outcome": "succeeded", "duration_ms": 40, "metering_event_ids": []int{5}, "occurred_at": old},
		{"id": 4, "request_id": "req-broke", "route": "api", "method": "GET", "operation": "/v1/items/8", "credential_id": 9,
			"target": "items-standard", "outcome": "refused", "error_code": "BILLING.INSUFFICIENT_BALANCE", "occurred_at": old},
		{"id": 5, "request_id": "req-binding", "route": "mcp", "operation": "tools/call", "tool_name": "lookup", "credential_id": 3,
			"consumer_type": str(SubjectAgent), "consumer_ref": str("agent:" + fixture.agent.String()), "target": "wiki", "outcome": "succeeded", "occurred_at": old},
		{"id": 6, "request_id": "req-young", "route": "api", "operation": "/v1/items/9", "credential_id": 9, "target": "items-standard",
			"outcome": "succeeded", "occurred_at": now.UTC().Format(time.RFC3339)},
	}
	fixture.market.charges = map[string]map[string]any{
		"req-script": {"state": "charged", "amount_micros": 1000, "lines": []map[string]any{{"meter": "request", "quantity": 1, "amount_micros": 1000}}},
	}
	from, to := strconv.FormatInt(now.Add(-2*time.Hour).Unix(), 10), strconv.FormatInt(now.Add(time.Hour).Unix(), 10)
	status, answer := fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to, "")
	require.Equal(t, http.StatusOK, status, answer)

	var logs []CallLog
	require.NoError(t, fixture.db.Order("request_id").Find(&logs).Error)
	byRequest := map[string]CallLog{}
	for _, row := range logs {
		byRequest[row.RequestID] = row
	}
	assert.Len(t, logs, 4, "the handshake, the call made here and the young record are not added")
	script := byRequest["req-script"]
	assert.Equal(t, []any{SubjectCredential, SourceCredential, ItemKindAPI, "items-standard", "GET /v1/items/7", CallSucceeded, uint64(9), "Ops script", CallChargeCharged, int64(1000), ""},
		[]any{script.SubjectType, script.Source, script.ItemKind, script.OfferingSlug, script.ToolName, script.Status, script.CredentialID, script.CredentialName, script.ChargeState, script.ChargeMicros, script.Input},
		"the charge of a call read from the market is settled like any other")
	assert.Equal(t, []any{CallRefused, "BILLING.INSUFFICIENT_BALANCE", CallChargeNone},
		[]any{byRequest["req-broke"].Status, byRequest["req-broke"].ErrorMessage, byRequest["req-broke"].ChargeState})
	assert.Equal(t, []any{SubjectAgent, fixture.agent}, []any{byRequest["req-binding"].SubjectType, byRequest["req-binding"].SubjectID},
		"a binding's credential used elsewhere still names its agent")
	var cursor CallLogCursor
	require.NoError(t, fixture.db.First(&cursor).Error)
	assert.Equal(t, uint64(5), cursor.AfterID, "the cursor stops before the record that is too young")

	// Read again later: nothing is added twice, and the record that has aged is.
	fixture.tools.now = func() time.Time { return now.Add(time.Minute) }
	status, _ = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to+"&subject_type=credential", "")
	require.Equal(t, http.StatusOK, status)
	var count int64
	require.NoError(t, fixture.db.Model(&CallLog{}).Count(&count).Error)
	assert.Equal(t, int64(5), count)
	require.NoError(t, fixture.db.First(&cursor).Error)
	assert.Equal(t, uint64(6), cursor.AfterID)

	// Another page read at once does not ask the market for its records again.
	asked := func() int {
		fixture.market.mu.Lock()
		defer fixture.market.mu.Unlock()
		count := 0
		for _, call := range fixture.market.calls {
			if strings.HasPrefix(call, "GET calls ") {
				count++
			}
		}
		return count
	}
	before := asked()
	fixture.tools.now = func() time.Time { return now.Add(time.Minute + 5*time.Second) }
	status, _ = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to+"&page=2", "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, before, asked(), "the records were read moments ago")
	fixture.tools.now = func() time.Time { return now.Add(2 * time.Minute) }
	status, _ = fixture.send(t, http.MethodGet, "call-logs?from="+from+"&to="+to, "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, before+1, asked(), "a later read asks again")
}
