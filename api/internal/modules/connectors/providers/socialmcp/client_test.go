package socialmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

func fixture(t *testing.T, omit string, outcome map[string]any, toolError bool) (*Client, *int) {
	t.Helper()
	return fixtureForProvider(t, "x-social", omit, outcome, toolError)
}

func fixtureForProvider(t *testing.T, id, omit string, outcome map[string]any, toolError bool) (*Client, *int) {
	t.Helper()
	c, err := New(id, Options{})
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "social-fixture", Version: "1"}, nil)
	calls := new(int)
	var names []any
	for _, op := range c.definition.Operations {
		name := remoteName(c.definition.ID, op.ID)
		if name == omit || localEventOperation(op.ID) {
			continue
		}
		names = append(names, name)
		server.AddTool(&mcp.Tool{Name: name, InputSchema: op.InputSchema}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			*calls++
			if r.Params.Name == "account_get" {
				return &mcp.CallToolResult{StructuredContent: map[string]any{"id": "42", "name": "fixture", "operations": names}}, nil
			}
			if toolError {
				encoded, _ := json.Marshal(outcome)
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}, nil
			}
			return &mcp.CallToolResult{StructuredContent: outcome}, nil
		})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	c.open = func(ctx context.Context, _ string) (*mcpclient.Connection, func(), error) {
		client, err := mcpclient.NewClient(mcpclient.ServerConfig{ID: "fixture", Transport: mcpclient.TransportStreamableHTTP, Endpoint: httpServer.URL}, mcpclient.Options{})
		if err != nil {
			return nil, func() {}, err
		}
		connection, err := client.Connect(ctx)
		return connection, func() {
			if connection != nil {
				_ = connection.Close(context.Background())
			}
		}, err
	}
	return c, calls
}

func TestVerifyAndInvokeFixedSocialOperations(t *testing.T) {
	c, calls := fixture(t, "message_send", map[string]any{"id": "post-1", "text": "fixture-token"}, false)
	verified, err := c.Verify(t.Context(), "fixture-token")
	if err != nil || verified.AccountID != "42" {
		t.Fatalf("verify %#v: %v", verified, err)
	}
	for _, op := range verified.Operations {
		if op == "x-social.message.send" {
			t.Fatal("missing MCP tool advertised")
		}
	}
	result, err := c.Invoke(t.Context(), "x-social.post.create", "fixture-token", map[string]any{"text": "hello"})
	if err != nil || result.Value["provider"] != "x-social" || result.Value["transport"] != "mcp" {
		t.Fatalf("invoke %#v: %v", result, err)
	}
	encoded, _ := json.Marshal(result.Value)
	if strings.Contains(string(encoded), "fixture-token") {
		t.Fatal("credential leaked in result")
	}
	before := *calls
	if _, err = c.Invoke(t.Context(), "x-social.message.send", "fixture-token", map[string]any{"conversation_id": "c", "text": "hello"}); err == nil {
		t.Fatal("missing tool invoked")
	}
	if *calls != before {
		t.Fatal("unsupported operation dispatched")
	}
	if _, err = c.Invoke(t.Context(), "x-social.post.create", "fixture-token", map[string]any{"text": "hi", "command": "arbitrary"}); err == nil {
		t.Fatal("unknown input accepted")
	}
}

func TestAmbiguousWriteIsNotRetried(t *testing.T) {
	c, calls := fixture(t, "", map[string]any{"code": "outcome_unknown"}, true)
	_, err := c.Invoke(t.Context(), "x-social.post.create", "fixture-token", map[string]any{"text": "hello"})
	if err == nil || !providers.FailureOf(err).Unknown || *calls != 1 {
		t.Fatalf("outcome %v calls %d", err, *calls)
	}
}

func TestSocialConfigurationAndCredentialsFailClosed(t *testing.T) {
	for _, id := range []string{"x-social", "linkedin-social", "facebook-social"} {
		c, err := New(id, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Verify(t.Context(), "token"); err == nil || providers.FailureOf(err).Code != core.ErrorCodeConnectorUnavailable {
			t.Fatalf("unconfigured bridge: %v", err)
		}
		if _, err = c.Verify(t.Context(), "bad\ntoken"); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatalf("invalid token: %v", err)
		}
	}
	if _, err := New("arbitrary", Options{}); err == nil {
		t.Fatal("unknown connector accepted")
	}
	if _, err := New("x-social", Options{ScriptPath: "relative/script.mjs"}); err == nil {
		t.Fatal("relative deployment path accepted")
	}
	if _, err := New("x-social", Options{ScriptPath: "/tmp/server.mjs", NodeCommand: "sh -c node"}); err == nil {
		t.Fatal("shell command accepted")
	}
}

func TestLinkedInMessagesNeverDispatch(t *testing.T) {
	c, err := New("linkedin-social", Options{})
	if err != nil {
		t.Fatal(err)
	}
	c.open = func(context.Context, string) (*mcpclient.Connection, func(), error) {
		t.Fatal("unsupported company messaging dispatched")
		return nil, nil, nil
	}
	_, err = c.Invoke(t.Context(), "linkedin-social.message.send", "token", map[string]any{"conversation_id": "c", "text": "hi"})
	if err == nil || providers.FailureOf(err).Code != core.ErrorCodeInsufficientKey {
		t.Fatalf("unsupported message result: %v", err)
	}
}

func TestProcessContractNeverUsesCredentialArguments(t *testing.T) {
	c, err := New("facebook-social", Options{ScriptPath: "/opt/zgi/social-mcp/server.mjs"})
	if err != nil {
		t.Fatal(err)
	}
	spec, cleanup, err := c.processSpec("private-token")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if strings.Contains(strings.Join(spec.Args, " "), "private-token") || spec.Env["ZGI_SOCIAL_TOKEN"] != "private-token" || spec.Env["HOME"] == "" {
		t.Fatal("process credential boundary")
	}
	var operations []map[string]any
	if json.Unmarshal([]byte(spec.Env["ZGI_SOCIAL_CONTRACT"]), &operations) != nil || len(operations) != len(c.definition.Operations) {
		t.Fatal("missing native tool contracts")
	}
	for _, d := range catalog.SocialMCPDefinitions() {
		if !d.Supports(catalog.SurfaceDigitalHuman) {
			t.Fatal("missing binding surface")
		}
	}
}

// Exercise the packaged JS bridge through the actual Go stdio transport. No
// account tool is called, so this test never contacts a vendor or publishes.
func TestPackagedBridgeCatalogMatchesNativeContracts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node not installed")
	}
	script, err := filepath.Abs("../../../../../social-mcp/server.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(script), "node_modules/@modelcontextprotocol/sdk/package.json")); err != nil {
		t.Skip("install api/social-mcp dependencies to run cross-language test")
	}
	for _, id := range []string{"x-social", "linkedin-social", "facebook-social"} {
		t.Run(id, func(t *testing.T) {
			c, err := New(id, Options{NodeCommand: node, ScriptPath: script})
			if err != nil {
				t.Fatal(err)
			}
			connection, close, err := c.connect(t.Context(), "offline-fixture-token")
			if err != nil {
				t.Fatal(err)
			}
			defer close()
			tools, err := connection.RefreshTools(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := len(c.definition.Operations)
			if id == "linkedin-social" {
				want -= 3
			}
			if id == "facebook-social" {
				want -= 2
			}
			if len(tools.Items) != want {
				t.Fatalf("bridge tools %d, native %d", len(tools.Items), want)
			}
			for _, op := range c.definition.Operations {
				name := remoteName(id, op.ID)
				if c.unavailableMessaging(op.ID) || localEventOperation(op.ID) {
					if listed(tools.Items, name) {
						t.Fatal("unavailable or local tool advertised by bridge")
					}
					continue
				}
				if !listed(tools.Items, name) {
					t.Fatalf("missing native tool %s", name)
				}
			}
			if !toolExists(tools.Items, "account_get", map[string]any{}) {
				t.Fatal("identity schema mismatch")
			}
		})
	}
}
