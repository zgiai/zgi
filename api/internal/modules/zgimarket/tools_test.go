package zgimarket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const testMarketSecret = "zgi_sk_test-binding-secret"

// fakeMarket answers the console proxy calls the market tools make.
type fakeMarket struct {
	mu       sync.Mutex
	calls    []string
	bindings map[uint64]map[string]any
	nextID   uint64
}

func (m *fakeMarket) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, consoleProxyPath)
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test server
		m.calls = append(m.calls, r.Method+" "+path+" "+string(body))
		data := func(status int, value any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": value}) //nolint:errcheck // test server
		}
		switch {
		case r.Method == http.MethodGet && path == "subscriptions":
			data(http.StatusOK, []map[string]any{
				{"id": 11, "listing_id": 7, "listing_kind": "mcp", "listing_slug": "wiki", "status": "active"},
				{"id": 12, "listing_id": 8, "listing_kind": "api", "listing_slug": "echo", "status": "active"},
				{"id": 13, "listing_id": 9, "listing_kind": "mcp", "listing_slug": "gone", "status": "cancelled"},
			})
		case r.Method == http.MethodGet && path == "listings/mcp/wiki":
			data(http.StatusOK, map[string]any{
				"id": 7, "card": map[string]any{"title": map[string]string{"zh-CN": "百科", "en-US": "Wiki"}},
				"mcp": map[string]any{"tools": []map[string]any{
					{"name": "lookup", "description": map[string]string{"en-US": "Look up"}, "risk_level": "read"},
					{"name": "edit", "description": map[string]string{"en-US": "Edit"}, "risk_level": "write"},
				}},
				"required_connectors": []map[string]any{},
			})
		case r.Method == http.MethodGet && path == "connections":
			data(http.StatusOK, []map[string]any{{"id": 5, "connector_slug": "drive", "name": "Team drive", "status": "connected"}})
		case r.Method == http.MethodPost && path == "bindings":
			m.nextID++
			var request map[string]any
			require.NoError(t, json.Unmarshal(body, &request))
			m.bindings[m.nextID] = request
			data(http.StatusCreated, map[string]any{"binding": map[string]any{"id": m.nextID, "version": 1}, "secret_key": testMarketSecret})
		case r.Method == http.MethodPut && strings.HasPrefix(path, "bindings/") && strings.HasSuffix(path, "/items"):
			data(http.StatusOK, map[string]any{"version": 2})
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "bindings/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":404}`) //nolint:errcheck // test server
		}
	}
}

// fakeGateway is a minimal MCP server at /v1/mcp/{slug} that records the credential it was called with.
type fakeGateway struct {
	mu          sync.Mutex
	credentials []string
	called      []string
}

func (g *fakeGateway) handler(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.credentials = append(g.credentials, r.Header.Get("Authorization"))
	g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+testMarketSecret || r.URL.Path != "/v1/mcp/wiki" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request) //nolint:errcheck // test server
	if len(request.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch request.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "fake", "version": "1"}}
	case "tools/list":
		result = map[string]any{"tools": []map[string]any{
			{"name": "lookup", "description": "Look up a topic", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"topic": map[string]any{"type": "string"}}, "required": []string{"topic"}}},
			{"name": "edit", "description": "Edit a page", "inputSchema": map[string]any{"type": "object"}},
		}}
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(request.Params, &params) //nolint:errcheck // test server
		g.mu.Lock()
		g.called = append(g.called, params.Name)
		g.mu.Unlock()
		result = map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%s:%v", params.Name, params.Arguments["topic"])}}}
	default:
		result = map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}) //nolint:errcheck // test server
}

type toolsFixture struct {
	db      *gorm.DB
	market  *fakeMarket
	gateway *fakeGateway
	tools   *Tools
	engine  *gin.Engine
	org     uuid.UUID
	agent   uuid.UUID
	human   uuid.UUID
}

func newToolsFixture(t *testing.T, admin bool) *toolsFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ToolBinding{}))
	require.NoError(t, db.Exec(`CREATE TABLE workspaces (id text, organization_id text)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE agents (id text, tenant_id text, deleted_at datetime)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE digital_humans (id text, organization_id text, deleted_at datetime)`).Error)
	fixture := &toolsFixture{db: db, market: &fakeMarket{bindings: map[uint64]map[string]any{}}, gateway: &fakeGateway{},
		org: uuid.New(), agent: uuid.New(), human: uuid.New()}
	workspace := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO workspaces VALUES (?, ?)`, workspace.String(), fixture.org.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents VALUES (?, ?, NULL)`, fixture.agent.String(), workspace.String()).Error)
	require.NoError(t, db.Exec(`INSERT INTO digital_humans VALUES (?, ?, NULL)`, fixture.human.String(), fixture.org.String()).Error)

	console := httptest.NewServer(fixture.market.handler(t))
	t.Cleanup(console.Close)
	gatewayServer := httptest.NewServer(http.HandlerFunc(fixture.gateway.handler))
	t.Cleanup(gatewayServer.Close)

	sealer, err := credentials.NewStore("k1", map[string]string{"k1": strings.Repeat("k", 32)})
	require.NoError(t, err)
	fixture.tools, err = NewTools(db, sealer, &GatewayConfig{URL: gatewayServer.URL, AllowPrivateTargets: true})
	require.NoError(t, err)
	proxy := NewProxy(
		Config{ConsoleURL: console.URL},
		func(*gin.Context) string { return fixture.org.String() },
		func(*gin.Context) string { return uuid.NewSHA1(uuid.Nil, []byte("account")).String() },
		func(*gin.Context, string, string) (bool, bool, error) { return true, admin, nil },
	).WithTools(fixture.tools)
	gin.SetMode(gin.TestMode)
	fixture.engine = gin.New()
	fixture.engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return fixture
}

func (f *toolsFixture) send(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, "/workspaces/current/zgi-market/"+path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded) //nolint:errcheck // asserted by the caller
	return recorder.Code, decoded
}

func TestToolCandidatesAreTheActivatedMCPListingsAndTheConnections(t *testing.T) {
	fixture := newToolsFixture(t, true)
	status, answer := fixture.send(t, http.MethodGet, "tools/candidates", "")
	require.Equal(t, http.StatusOK, status, answer)
	data := answer["data"].(map[string]any)
	listings := data["listings"].([]any)
	require.Len(t, listings, 1, "an api listing and a cancelled subscription are not candidates")
	listing := listings[0].(map[string]any)
	assert.Equal(t, "wiki", listing["slug"])
	assert.EqualValues(t, 11, listing["subscription_id"])
	assert.Len(t, listing["tools"], 2)
	assert.Len(t, data["connections"], 1)
}

func TestBindingAnAgentCreatesAMarketBindingAndSealsItsCredential(t *testing.T) {
	fixture := newToolsFixture(t, true)
	path := "tools/subjects/agent/" + fixture.agent.String()

	status, answer := fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":7,"connection_id":5}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	items := answer["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "wiki", items[0].(map[string]any)["slug"])

	created := fixture.market.bindings[1]
	assert.Equal(t, "agent", created["consumer_type"])
	assert.Equal(t, "agent:"+fixture.agent.String(), created["consumer_ref"])
	assert.Equal(t, []any{map[string]any{"subscription_id": float64(11), "connection_id": float64(5)}}, created["items"])

	var stored ToolBinding
	require.NoError(t, fixture.db.First(&stored).Error)
	assert.NotContains(t, stored.SecretCiphertext, testMarketSecret, "the credential is kept sealed")
	assert.Equal(t, []ToolInfo{{Name: "lookup", RiskLevel: "read"}, {Name: "edit", RiskLevel: "write"}}, stored.Items[0].Tools)

	// The runtime lists the tools at the gateway with the binding's credential.
	listed, err := fixture.tools.SubjectTools(context.Background(), fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, "agent:"+fixture.agent.String()+":wiki", listed[0].ProviderID)
	assert.Equal(t, "lookup", listed[0].Name)
	assert.Equal(t, "read", listed[0].RiskLevel)
	assert.Equal(t, "write", listed[1].RiskLevel)
	assert.Equal(t, []any{"topic"}, listed[0].InputSchema["required"])

	// A tool call goes to the gateway through the tool manager.
	manager := tools.NewToolManager(nil)
	manager.SetMarketToolSource(fixture.tools)
	engine := tools.NewToolEngine(manager)
	result, err := engine.Invoke(context.Background(), tools.InvokeRequest{
		ProviderType: tools.ToolProviderTypeMarket, ProviderID: listed[0].ProviderID, ToolName: "lookup",
		TenantID: fixture.org.String(), Parameters: map[string]interface{}{"topic": "go"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, "lookup:go", result.Messages[0].Text)
	assert.Equal(t, []string{"lookup"}, fixture.gateway.called)

	// Another organization cannot reach the subject's tools by naming its provider.
	_, err = engine.Invoke(context.Background(), tools.InvokeRequest{
		ProviderType: tools.ToolProviderTypeMarket, ProviderID: listed[0].ProviderID, ToolName: "lookup",
		TenantID: uuid.NewString(), Parameters: map[string]interface{}{"topic": "go"},
	})
	assert.Error(t, err)
	assert.Equal(t, []string{"lookup"}, fixture.gateway.called)

	// Replacing the listings replaces the items of the same market binding.
	status, answer = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":7}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	require.NoError(t, fixture.db.First(&stored).Error)
	assert.Equal(t, 2, stored.MarketVersion)
	assert.Nil(t, stored.Items[0].ConnectionID)

	// Unbinding the last listing deletes the market binding and the stored credential.
	status, _ = fixture.send(t, http.MethodPut, path, `{"items":[]}`)
	require.Equal(t, http.StatusOK, status)
	var count int64
	require.NoError(t, fixture.db.Model(&ToolBinding{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Contains(t, fixture.market.calls[len(fixture.market.calls)-1], "DELETE bindings/1")
	listed, err = fixture.tools.SubjectTools(context.Background(), fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestBindingADigitalHumanAndTheRefusals(t *testing.T) {
	fixture := newToolsFixture(t, true)
	status, _ := fixture.send(t, http.MethodPut, "tools/subjects/digital_human/"+fixture.human.String(), `{"items":[{"listing_id":7}]}`)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "digital_human", fixture.market.bindings[1]["consumer_type"])
	assert.Equal(t, "dh:"+fixture.human.String(), fixture.market.bindings[1]["consumer_ref"])

	status, _ = fixture.send(t, http.MethodPut, "tools/subjects/agent/"+uuid.NewString(), `{"items":[{"listing_id":7}]}`)
	assert.Equal(t, http.StatusNotFound, status, "an agent of another organization")
	status, _ = fixture.send(t, http.MethodPut, "tools/subjects/agent/"+fixture.agent.String(), `{"items":[{"listing_id":8}]}`)
	assert.Equal(t, http.StatusBadRequest, status, "an api listing is not an MCP server")
	status, _ = fixture.send(t, http.MethodPut, "tools/subjects/robot/"+fixture.agent.String(), `{"items":[]}`)
	assert.Equal(t, http.StatusNotFound, status)

	member := newToolsFixture(t, false)
	status, _ = member.send(t, http.MethodPut, "tools/subjects/agent/"+member.agent.String(), `{"items":[{"listing_id":7}]}`)
	assert.Equal(t, http.StatusForbidden, status, "a member reads but does not bind")
	status, _ = member.send(t, http.MethodGet, "tools/subjects/agent/"+member.agent.String(), "")
	assert.Equal(t, http.StatusOK, status)
}

func TestTheOAuthCallbackIsAnsweredWithThePageToOpen(t *testing.T) {
	engine, _ := newTestProxy(t, true, true)
	recorder := send(engine, http.MethodGet, "/workspaces/current/zgi-market/connections/oauth/callback?state=s&code=c", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"redirect":"https://app.example.com/done"`)
	assert.Empty(t, recorder.Header().Get("Location"), "the page reads the target from the body")
}

func TestTheGatewayMustBeHTTPSOutsideLocalDevelopment(t *testing.T) {
	_, err := NewTools(nil, nil, &GatewayConfig{URL: "http://market.example.com"})
	assert.Error(t, err)
	_, err = NewTools(nil, nil, &GatewayConfig{URL: "https://user@market.example.com"})
	assert.Error(t, err)
	client, err := newGatewayClient(GatewayConfig{URL: "https://127.0.0.1:1"})
	require.NoError(t, err)
	_, err = client.ListTools(context.Background(), "wiki", testMarketSecret)
	assert.Error(t, err, "a gateway that resolves to a loopback address is refused")
}
