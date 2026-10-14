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
	"time"

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
	// items adds an activated http API offering and a model offering to the subscriptions.
	items      bool
	deleted    []string
	failDelete bool
	// charges answers calls/charges by request id; a request id it lacks is answered none.
	charges map[string]map[string]any
}

// itemsListing is the API listing "items" as the market answers it, for a member and in public.
func itemsListing() map[string]any {
	return map[string]any{
		"id": 20, "card": map[string]any{"title": map[string]string{"zh-CN": "条目", "en-US": "Items"}},
		"offerings": []map[string]any{
			{"id": 31, "slug": "items-standard", "protocol_family": "http", "status": "active", "operations": []map[string]any{
				{"method": "GET", "path": "/v1/items/{id}", "input_schema": map[string]any{
					"type": "object", "description": "Read one item", "required": []string{"id"},
					"properties": map[string]any{"id": map[string]any{"type": "string"}, "scale": map[string]any{"type": "number"}},
				}},
				{"method": "POST", "path": "/v1/items", "input_schema": map[string]any{"type": "object"}},
				{"method": "GET", "path": "/v1/plain"},
			}},
			{"id": 32, "slug": "items-chat", "protocol_family": "openai_chat", "status": "active"},
		},
		"required_connectors": []map[string]any{},
	}
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
			subscriptions := []map[string]any{
				{"id": 11, "listing_id": 7, "listing_kind": "mcp", "listing_slug": "wiki", "status": "active"},
				{"id": 12, "listing_id": 8, "listing_kind": "api", "listing_slug": "echo", "status": "active"},
				{"id": 13, "listing_id": 9, "listing_kind": "mcp", "listing_slug": "gone", "status": "cancelled"},
			}
			if m.items {
				subscriptions = append(subscriptions,
					map[string]any{"id": 21, "listing_id": 20, "listing_kind": "api", "listing_slug": "items", "offering_slug": "items-standard", "status": "active"},
					map[string]any{"id": 22, "listing_id": 20, "listing_kind": "api", "listing_slug": "items", "offering_slug": "items-chat", "status": "active"},
				)
			}
			data(http.StatusOK, subscriptions)
		case r.Method == http.MethodGet && path == "listings/api/items" && m.items:
			data(http.StatusOK, itemsListing())
		case r.Method == http.MethodGet && path == "listings/mcp/wiki":
			data(http.StatusOK, map[string]any{
				"id": 7, "card": map[string]any{"title": map[string]string{"zh-CN": "百科", "en-US": "Wiki"}},
				"mcp": map[string]any{"tools": []map[string]any{
					{"name": "lookup", "description": map[string]string{"en-US": "Look up"}, "risk_level": "read"},
					{"name": "edit", "description": map[string]string{"en-US": "Edit"}, "risk_level": "write"},
				}},
				"required_connectors": []map[string]any{},
			})
		case r.Method == http.MethodGet && path == "calls/charges":
			answers := []map[string]any{}
			for _, id := range strings.Split(r.URL.Query().Get("request_ids"), ",") {
				charge, ok := m.charges[id]
				if !ok {
					charge = map[string]any{"state": "none", "amount_micros": 0, "lines": []any{}}
				}
				charge["request_id"] = id
				answers = append(answers, charge)
			}
			data(http.StatusOK, map[string]any{"charges": answers})
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
			if m.failDelete {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			m.deleted = append(m.deleted, path)
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
	// requests are the offering calls it received, as "METHOD path?query body".
	requests []string
	// listing replaces the public answer for the API listing "items" when set.
	listing map[string]any
}

func (g *fakeGateway) handler(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.credentials = append(g.credentials, r.Header.Get("Authorization"))
	g.mu.Unlock()
	// The public catalog is served at the gateway's origin and takes no credential.
	if r.Method == http.MethodGet && r.URL.Path == "/v1/market/listings/api/items" {
		g.mu.Lock()
		listing := g.listing
		g.mu.Unlock()
		if listing == nil {
			listing = itemsListing()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": listing}) //nolint:errcheck // test server
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/gateway/") {
		w.Header().Set(requestIDHeader, "api-"+strings.TrimPrefix(r.URL.Path, "/v1/gateway/"))
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test server
		g.mu.Lock()
		g.requests = append(g.requests, strings.TrimSpace(r.Method+" "+r.URL.RequestURI()+" "+string(body)))
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Header.Get("Authorization") != "Bearer "+testMarketSecret:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":401,"error_code":"MARKET_ACCESS.CREDENTIAL_INVALID","message":"Credential is invalid"}`) //nolint:errcheck // test server
		case strings.HasSuffix(r.URL.Path, "/v1/items/broke"):
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = io.WriteString(w, `{"code":402,"error_code":"BILLING.INSUFFICIENT_BALANCE","message":"The balance is too low"}`) //nolint:errcheck // test server
		default:
			_, _ = io.WriteString(w, `{"item":"`+r.URL.Path+`","scale":"`+r.URL.Query().Get("scale")+`"}`) //nolint:errcheck // test server
		}
		return
	}
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
	// Every POST is named; the tool call's own name is what a call log keeps.
	w.Header().Set(requestIDHeader, "mcp-"+request.Method)
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
	require.NoError(t, db.AutoMigrate(&ToolBinding{}, &CallLog{}))
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

	// A name the runtime prefixed with the listing, because two listings share it, reaches the
	// vendor's tool.
	result, err = engine.Invoke(context.Background(), tools.InvokeRequest{
		ProviderType: tools.ToolProviderTypeMarket, ProviderID: listed[0].ProviderID, ToolName: ToolNamePrefix("wiki") + "lookup",
		TenantID: fixture.org.String(), Parameters: map[string]interface{}{"topic": "rust"},
	})
	require.NoError(t, err)
	assert.Equal(t, "lookup:rust", result.Messages[0].Text)
	fixture.gateway.called = fixture.gateway.called[:1]

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

func TestOperationToolNameNamesTheOfferingAndTheRequestWithinTheLimit(t *testing.T) {
	assert.Equal(t, "amap_web_service_standard__get_v3_geocode_geo", OperationToolName("amap-web-service-standard", "GET", "/v3/geocode/geo"))
	assert.Equal(t, "items_standard__post_v1_items_id_notes", OperationToolName("items-standard", "post", "/v1/items/{id}/notes"))
	assert.Equal(t, "echo__get", OperationToolName("echo", "GET", "/"))

	long := OperationToolName("a-very-long-offering-slug-of-a-vendor-standard", "DELETE", "/v1/organizations/{organization}/projects/{project}")
	other := OperationToolName("a-very-long-offering-slug-of-a-vendor-standard", "DELETE", "/v1/organizations/{organization}/projects/{project}/keys")
	for _, name := range []string{long, other} {
		assert.LessOrEqual(t, len(name), 64)
		assert.True(t, toolNamePattern.MatchString(name), name)
	}
	assert.NotEqual(t, long, other, "two long names never meet")
	assert.Equal(t, long, OperationToolName("a-very-long-offering-slug-of-a-vendor-standard", "DELETE", "/v1/organizations/{organization}/projects/{project}"), "the name is stable")
}

func TestOperationsAreNamedInAsFewWordsAsTellThemApart(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	offering := func(slug string, operations ...marketOperation) marketOffering {
		return marketOffering{Slug: slug, ProtocolFamily: "http", Operations: operations}
	}
	post := func(path string) marketOperation {
		return marketOperation{Method: "POST", Path: path, InputSchema: schema}
	}
	get := func(path string) marketOperation {
		return marketOperation{Method: "GET", Path: path, InputSchema: schema}
	}

	// One operation of the listing's only offering: the listing's slug says it all.
	single := offering("zgi-public-holiday-standard", post("/v1/public-holiday-queries"))
	assert.Equal(t, map[string]string{"POST /v1/public-holiday-queries": "zgi_public_holiday"},
		operationToolNames("zgi-public-holiday", single, true))
	// A listing with several offerings names the offering, so its offerings never share a name.
	assert.Equal(t, map[string]string{"POST /v1/public-holiday-queries": "zgi_public_holiday_standard"},
		operationToolNames("zgi-public-holiday", single, false))

	// Several operations: the method and the last literal segment of the path.
	maps := offering("amap-web-service-standard", get("/v3/geocode/geo"), get("/v3/geocode/regeo"), get("/v3/place/text"), get("/v3/weather/weatherInfo"))
	assert.Equal(t, map[string]string{
		"GET /v3/geocode/geo": "amap_web_service__get_geo", "GET /v3/geocode/regeo": "amap_web_service__get_regeo",
		"GET /v3/place/text": "amap_web_service__get_text", "GET /v3/weather/weatherInfo": "amap_web_service__get_weatherinfo",
	}, operationToolNames("amap-web-service", maps, true))
	// A path parameter is not a word; the method tells a read from a removal of the same thing.
	assert.Equal(t, map[string]string{"GET /v1/items/{id}": "items__get_items", "DELETE /v1/items/{id}": "items__delete_items"},
		operationToolNames("items", offering("items-standard", get("/v1/items/{id}"), marketOperation{Method: "DELETE", Path: "/v1/items/{id}", InputSchema: schema}), true))

	// Two operations whose last segments are the same: every operation names its whole path.
	clash := offering("crm-standard", get("/v1/users/{id}/notes"), get("/v1/orders/{id}/notes"), get("/v1/users"))
	assert.Equal(t, map[string]string{
		"GET /v1/users/{id}/notes": "crm__get_v1_users_id_notes", "GET /v1/orders/{id}/notes": "crm__get_v1_orders_id_notes",
		"GET /v1/users": "crm__get_v1_users",
	}, operationToolNames("crm", clash, true))

	// The names do not depend on which operations have a schema, are within the limit, and are
	// the same every time.
	mixed := offering("items-standard", get("/v1/items/{id}"), marketOperation{Method: "GET", Path: "/v1/plain"}, marketOperation{Method: "TRACE", Path: "/v1/items"})
	assert.Equal(t, map[string]string{"GET /v1/items/{id}": "items__get_items", "GET /v1/plain": "items__get_plain"},
		operationToolNames("items", mixed, true))
	long := offering("a-very-long-offering-slug-of-a-vendor-of-many-words-standard",
		get("/v1/organizations/{organization}/projects"), get("/v1/organizations/{organization}/project-archives"))
	names := operationToolNames("a-very-long-listing-slug-of-a-vendor-of-many-words-and-more-words", long, true)
	require.Len(t, names, 2)
	seen := map[string]bool{}
	for _, name := range names {
		assert.LessOrEqual(t, len(name), 64)
		assert.True(t, toolNamePattern.MatchString(name), name)
		assert.False(t, seen[name], "two long names never meet")
		seen[name] = true
	}
	assert.Equal(t, names, operationToolNames("a-very-long-listing-slug-of-a-vendor-of-many-words-and-more-words", long, true))
}

func TestAShortNameThatIsTakenGivesWayToTheFullName(t *testing.T) {
	// A vendor's MCP tool is called `lookup`, and so is the only operation of the listing `lookup`.
	taken := map[string]bool{"lookup": true}
	operation := BoundOperation{Key: "POST /v1/queries", Name: "lookup", Method: "POST", Path: "/v1/queries"}
	name, free := freeOperationName(taken, "lookup-standard", operation)
	assert.Equal(t, []any{"lookup_standard__post_v1_queries", true}, []any{name, free})
	assert.True(t, taken[name], "the name is taken from then on")
	// Listed again, as a repeated offering would be, both names are taken: it is left out.
	_, free = freeOperationName(taken, "lookup-standard", operation)
	assert.False(t, free)
	// A name nobody has is used as it is.
	name, free = freeOperationName(taken, "items-standard", BoundOperation{Name: "items", Method: "GET", Path: "/v1/items"})
	assert.Equal(t, []any{"items", true}, []any{name, free})
}

func TestOperationArgumentsBecomeThePathTheQueryOrTheBody(t *testing.T) {
	arguments := map[string]any{
		"id": "a/b", "scale": float64(2), "deep": true, "tags": []any{"x", "y z", float64(3)},
		"filter": map[string]any{"k": "v"}, "skip": nil,
	}
	request, err := buildOperationRequest(http.MethodGet, "/v1/items/{id}", arguments)
	require.NoError(t, err)
	assert.Equal(t, "/v1/items/a%2Fb", request.Path, "a path argument is escaped and leaves the rest")
	assert.Nil(t, request.Body)
	assert.Equal(t, "deep=true&filter=%7B%22k%22%3A%22v%22%7D&scale=2&tags=x&tags=y+z&tags=3", request.Query.Encode())
	assert.Len(t, arguments, 6, "the caller's arguments are not changed")

	request, err = buildOperationRequest(http.MethodPost, "/v1/items/{id}/notes", map[string]any{"id": float64(7), "text": "hi", "tags": []any{"x"}})
	require.NoError(t, err)
	assert.Equal(t, "/v1/items/7/notes", request.Path)
	assert.Empty(t, request.Query)
	assert.JSONEq(t, `{"text":"hi","tags":["x"]}`, string(request.Body))

	request, err = buildOperationRequest(http.MethodDelete, "/v1/items/{id}", map[string]any{"id": "9", "force": false})
	require.NoError(t, err)
	assert.Equal(t, "force=false", request.Query.Encode())

	for name, bad := range map[string]map[string]any{
		"a missing path argument": {"scale": 1},
		"an empty path argument":  {"id": ""},
		"a path argument object":  {"id": map[string]any{"x": 1}},
	} {
		_, err := buildOperationRequest(http.MethodGet, "/v1/items/{id}", bad)
		assert.ErrorIs(t, err, errArguments, name)
	}
}

func TestAnOperationIsAToolOnlyWithAnObjectInputSchema(t *testing.T) {
	offering := marketOffering{Slug: "items-standard", ProtocolFamily: "http", Operations: []marketOperation{
		{Method: "GET", Path: "/v1/items/{id}", InputSchema: json.RawMessage(`{"type":"object","description":"Read one item","properties":{"id":{"type":"string"}}}`)},
		{Method: "post", Path: "/v1/items", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Method: "GET", Path: "/v1/plain"},
		{Method: "GET", Path: "/v1/list", InputSchema: json.RawMessage(`{"type":"array"}`)},
		{Method: "GET", Path: "/v1/items/{id}", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Method: "TRACE", Path: "/v1/items"},
	}}
	operations := candidateOperations("items", offering, true, "Items")
	require.Len(t, operations, 4, "a repeated operation and an unknown method are dropped")
	assert.Equal(t, CandidateOperation{
		Key: "GET /v1/items/{id}", Name: "items__get_items", Method: "GET", Path: "/v1/items/{id}",
		Description: "Read one item", RiskLevel: RiskRead, Bindable: true, InputSchema: offering.Operations[0].InputSchema,
	}, operations[0])
	assert.Equal(t, RiskWrite, operations[1].RiskLevel, "every method but GET writes")
	assert.Equal(t, "POST /v1/items (Items)", operations[1].Description)
	assert.Equal(t, []any{false, ReasonNoInputSchema}, []any{operations[2].Bindable, operations[2].Reason})
	assert.Equal(t, []any{false, ReasonInputSchemaNotObject}, []any{operations[3].Bindable, operations[3].Reason})

	candidate := ToolCandidate{Kind: ItemKindAPI, OfferingSlug: "items-standard", Operations: operations}
	bound, all, err := boundOperations(candidate, nil)
	require.NoError(t, err)
	assert.True(t, all)
	assert.Len(t, bound, 2, "no selection is every bindable operation")
	bound, all, err = boundOperations(candidate, []string{"POST /v1/items"})
	require.NoError(t, err)
	assert.False(t, all)
	require.Len(t, bound, 1)
	assert.Equal(t, "items__post_items", bound[0].Name)
	_, _, err = boundOperations(candidate, []string{"GET /v1/plain"})
	assert.ErrorIs(t, err, ErrListingNotBindable, "an operation without a schema cannot be selected")
	_, _, err = boundOperations(candidate, []string{"GET /v1/unknown"})
	assert.ErrorIs(t, err, ErrListingNotBindable)
}

func TestAnOperationTakesTheRiskLevelTheMarketDeclares(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	operations := candidateOperations("lookup", marketOffering{Slug: "lookup-standard", ProtocolFamily: "http", Operations: []marketOperation{
		// A lookup served by POST: the market says it reads, the method alone would say it writes.
		{Method: "POST", Path: "/v1/holiday-queries", RiskLevel: "read", InputSchema: schema},
		{Method: "GET", Path: "/v1/purge", RiskLevel: " Destructive ", InputSchema: schema},
		{Method: "POST", Path: "/v1/orders", InputSchema: schema},
		{Method: "POST", Path: "/v1/odd", RiskLevel: "harmless", InputSchema: schema},
	}}, true, "")
	require.Len(t, operations, 4)
	assert.Equal(t, RiskRead, operations[0].RiskLevel)
	assert.Equal(t, RiskDestructive, operations[1].RiskLevel, "a declared level wins over a GET")
	assert.Equal(t, RiskWrite, operations[2].RiskLevel, "without a declaration the method decides")
	assert.Equal(t, RiskWrite, operations[3].RiskLevel, "an unknown level is not trusted")
}

func TestAToolResultIsCutWithAMarkerAndAFailureCarriesTheMarketCode(t *testing.T) {
	short := `{"value":42}`
	assert.Equal(t, short, capResult(short))
	long := strings.Repeat("好", maxToolResultChars)
	cut := capResult(long)
	assert.LessOrEqual(t, len(cut), maxToolResultChars)
	assert.True(t, strings.HasSuffix(cut, truncatedMarker))
	assert.True(t, strings.HasPrefix(long, strings.TrimSuffix(cut, truncatedMarker)), "the cut does not split a character")

	assert.Equal(t, "ZGI Market answered 402 BILLING.INSUFFICIENT_BALANCE: The balance is too low",
		marketFailure(402, []byte(`{"code":402,"error_code":"BILLING.INSUFFICIENT_BALANCE","message":"The balance is too low"}`)))
	assert.Equal(t, "ZGI Market answered 502: bad gateway", marketFailure(502, []byte("bad gateway")))
	assert.Equal(t, "ZGI Market answered 500", marketFailure(500, nil))
}

func TestAnAPIOfferingIsBoundAndItsOperationsAreCalledAtTheGateway(t *testing.T) {
	fixture := newToolsFixture(t, true)
	fixture.market.items = true
	ctx := context.Background()

	// The http offering is a candidate with its operations; the model offering is not listed.
	status, answer := fixture.send(t, http.MethodGet, "tools/candidates", "")
	require.Equal(t, http.StatusOK, status, answer)
	listings := answer["data"].(map[string]any)["listings"].([]any)
	require.Len(t, listings, 2)
	offering := listings[1].(map[string]any)
	assert.Equal(t, []any{"api", "items", "items-standard", float64(21)},
		[]any{offering["kind"], offering["slug"], offering["offering_slug"], offering["subscription_id"]})
	operations := offering["operations"].([]any)
	require.Len(t, operations, 3)
	assert.Equal(t, ReasonNoInputSchema, operations[2].(map[string]any)["reason"])
	assert.Len(t, offering["tools"], 2, "the bindable operations are its tools")

	// Bind the MCP server and every operation of the offering to the agent.
	path := "tools/subjects/agent/" + fixture.agent.String()
	status, answer = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":7},{"listing_id":20,"offering_slug":"items-standard"}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	items := answer["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 2)
	bound := items[1].(map[string]any)
	assert.Equal(t, []any{"api", "items-standard", true}, []any{bound["kind"], bound["offering_slug"], bound["all_operations"]})
	assert.NotContains(t, fmt.Sprint(bound["operations"]), "input_schema", "a page is not sent the schemas")
	assert.Equal(t, []any{map[string]any{"subscription_id": float64(11)}, map[string]any{"subscription_id": float64(21)}},
		fixture.market.bindings[1]["items"], "an offering is one more item of the same market binding")

	listed, err := fixture.tools.SubjectTools(ctx, fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	require.Len(t, listed, 4)
	read, write := listed[2], listed[3]
	assert.Equal(t, "agent:"+fixture.agent.String()+":api.items-standard", read.ProviderID)
	assert.Equal(t, []any{"items__get_items", "Read one item", RiskRead}, []any{read.Name, read.Description, read.RiskLevel})
	assert.Equal(t, []any{"id"}, read.InputSchema["required"])
	assert.Equal(t, []any{"items__post_items", RiskWrite}, []any{write.Name, write.RiskLevel})

	manager := tools.NewToolManager(nil)
	manager.SetMarketToolSource(fixture.tools)
	engine := tools.NewToolEngine(manager)
	invoke := func(tool string, parameters map[string]interface{}) (*tools.InvokeResult, error) {
		return engine.Invoke(ctx, tools.InvokeRequest{
			ProviderType: tools.ToolProviderTypeMarket, ProviderID: read.ProviderID, ToolName: tool,
			TenantID: fixture.org.String(), Parameters: parameters,
		})
	}
	// A GET: the path argument fills the path, the rest is the query, the credential is the binding's.
	result, err := invoke(read.Name, map[string]interface{}{"id": "a b", "scale": float64(2)})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.JSONEq(t, `{"item":"/v1/gateway/items-standard/v1/items/a b","scale":"2"}`, result.Messages[0].Text)
	assert.Equal(t, "GET /v1/gateway/items-standard/v1/items/a%20b?scale=2", fixture.gateway.requests[0])
	// Another method sends the arguments as a JSON body.
	_, err = invoke(write.Name, map[string]interface{}{"name": "n"})
	require.NoError(t, err)
	assert.Equal(t, `POST /v1/gateway/items-standard/v1/items {"name":"n"}`, fixture.gateway.requests[1])
	// Arguments that do not fit the path reach no gateway.
	_, err = invoke(read.Name, map[string]interface{}{"scale": float64(2)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `the path needs the argument "id"`)
	assert.Len(t, fixture.gateway.requests, 2)
	// An answer outside 2xx is a tool error with the market's code.
	_, err = invoke(read.Name, map[string]interface{}{"id": "broke"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "402 BILLING.INSUFFICIENT_BALANCE")
	// The full name of an operation finds it too: a conversation begun before the short names holds it.
	result, err = invoke("items_standard__get_v1_items_id", map[string]interface{}{"id": "x"})
	require.NoError(t, err)
	require.True(t, result.Success)
	// A tool the offering does not have, and another organization, reach nothing.
	for _, unknown := range []string{"items__get_plain", "items_standard__get_v1_plain"} {
		_, err = invoke(unknown, nil)
		assert.Error(t, err, unknown)
	}
	_, err = engine.Invoke(ctx, tools.InvokeRequest{
		ProviderType: tools.ToolProviderTypeMarket, ProviderID: read.ProviderID, ToolName: read.Name,
		TenantID: uuid.NewString(), Parameters: map[string]interface{}{"id": "x"},
	})
	assert.Error(t, err)
	assert.Len(t, fixture.gateway.requests, 4)

	// A new version of the listing is followed: the operation it drops is no tool any more, and
	// the one it adds is, because every operation was bound.
	next := itemsListing()
	next["offerings"].([]map[string]any)[0]["operations"] = []map[string]any{
		{"method": "GET", "path": "/v1/items/{id}", "input_schema": map[string]any{"type": "object"}},
		{"method": "DELETE", "path": "/v1/items/{id}", "input_schema": map[string]any{"type": "object"}},
	}
	fixture.gateway.mu.Lock()
	fixture.gateway.listing = next
	fixture.gateway.mu.Unlock()
	fixture.tools.now = func() time.Time { return time.Now().Add(2 * toolCatalogTTL) }
	listed, err = fixture.tools.SubjectTools(ctx, fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range listed {
		names = append(names, tool.Name)
	}
	assert.Equal(t, []string{"lookup", "edit", "items__get_items", "items__delete_items"}, names)

	// A chosen subset stays a subset; an operation without a schema cannot be chosen.
	status, answer = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":20,"offering_slug":"items-standard","operations":["GET /v1/items/{id}"]}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	var stored ToolBinding
	require.NoError(t, fixture.db.First(&stored).Error)
	require.Len(t, stored.Items, 1)
	assert.False(t, stored.Items[0].AllOperations)
	listed, err = fixture.tools.SubjectTools(ctx, fixture.org, SubjectAgent, fixture.agent)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "items__get_items", listed[0].Name, "choosing fewer operations renames nothing")
	status, _ = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":20,"offering_slug":"items-standard","operations":["GET /v1/plain"]}]}`)
	assert.Equal(t, http.StatusBadRequest, status)
	status, _ = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":20,"offering_slug":"items-chat"}]}`)
	assert.Equal(t, http.StatusBadRequest, status, "a model offering is not bindable")
	status, _ = fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":20,"offering_slug":"items-standard"},{"listing_id":20,"offering_slug":"items-standard"}]}`)
	assert.Equal(t, http.StatusBadRequest, status, "one offering is bound once")
}
