package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPConnectionAndCapabilities(t *testing.T) {
	for _, stateless := range []bool{true, false} {
		t.Run(map[bool]string{true: "modern", false: "legacy"}[stateless], func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
			server.AddTool(&mcp.Tool{Name: "math.add", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "number"}}}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}, StructuredContent: map[string]any{"ok": true}}, nil
			})
			server.AddResource(&mcp.Resource{URI: "demo://readme", Name: "readme"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "demo://readme", MIMEType: "text/plain", Text: "hello"}}}, nil
			})
			server.AddPrompt(&mcp.Prompt{Name: "summarize"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: mcp.Role("user"), Content: &mcp.TextContent{Text: "summary"}}}}, nil
			})
			httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: stateless, JSONResponse: true}))
			defer httpServer.Close()
			client, err := NewClient(ServerConfig{ID: "test", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			conn, err := client.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			if !conn.Info().Capabilities.Tools || !conn.Info().Capabilities.Resources || !conn.Info().Capabilities.Prompts {
				t.Fatalf("capabilities: %#v", conn.Info())
			}
			page, err := conn.ListToolsPage(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 || page.Items[0].Name != "math.add" {
				t.Fatalf("tools: %#v", page)
			}
			result, err := conn.CallTool(ctx, ToolCallRequest{Name: "math.add", Arguments: json.RawMessage(`{"a":2}`)})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 1 || !result.StructuredContent.Present {
				t.Fatalf("tool result: %#v", result)
			}
			resources, err := conn.ListResourcesPage(ctx, nil)
			if err != nil || len(resources.Items) != 1 {
				t.Fatalf("resources: %#v %v", resources, err)
			}
			resource, err := conn.ReadResource(ctx, ResourceReadRequest{URI: "demo://readme"})
			if err != nil || len(resource.Contents) != 1 {
				t.Fatalf("resource: %#v %v", resource, err)
			}
			prompts, err := conn.ListPromptsPage(ctx, nil)
			if err != nil || len(prompts.Items) != 1 {
				t.Fatalf("prompts: %#v %v", prompts, err)
			}
			prompt, err := conn.GetPrompt(ctx, PromptGetRequest{Name: "summarize"})
			if err != nil || len(prompt.Messages) != 1 || prompt.Messages[0].Role != "user" {
				t.Fatalf("prompt: %#v %v", prompt, err)
			}
			catalog, err := conn.RefreshTools(ctx)
			if err != nil || len(catalog.Items) != 1 {
				t.Fatalf("catalog: %#v %v", catalog, err)
			}
		})
	}
}

func TestStdioHelper(t *testing.T) {
	if os.Getenv("MCP_TEST_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio-helper", Version: "1"}, nil)
	count := 0
	server.AddTool(&mcp.Tool{Name: "count", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		count++
		return &mcp.CallToolResult{StructuredContent: map[string]any{"count": count}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "large", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", 32<<10)}}}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestStdioConnectionReuseAndClose(t *testing.T) {
	t.Setenv("MCP_TEST_HELPER", "1")
	client, err := NewClient(ServerConfig{ID: "stdio", Transport: TransportStdio, Process: &ProcessSpec{Command: os.Args[0], Args: []string{"-test.run=^TestStdioHelper$"}, Env: map[string]string{"MCP_TEST_HELPER": "1"}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for want := 1; want <= 2; want++ {
		result, err := conn.CallTool(ctx, ToolCallRequest{Name: "count"})
		if err != nil {
			t.Fatal(err)
		}
		var value struct{ Count int }
		if err := json.Unmarshal(result.StructuredContent.JSON, &value); err != nil || value.Count != want {
			t.Fatalf("call %d: %#v %v", want, result, err)
		}
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.Done():
	default:
		t.Fatal("connection did not close")
	}
}

func TestStdioMessageLimit(t *testing.T) {
	t.Setenv("MCP_TEST_HELPER", "1")
	client, err := NewClient(ServerConfig{ID: "stdio-limit", Transport: TransportStdio, Process: &ProcessSpec{Command: os.Args[0], Args: []string{"-test.run=^TestStdioHelper$"}, Env: map[string]string{"MCP_TEST_HELPER": "1"}}, Limits: Limits{MaxMessageBytes: 16 << 10}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	_, err = conn.CallTool(t.Context(), ToolCallRequest{Name: "large"})
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrLimitExceeded {
		t.Fatalf("stdio limit error: %v", err)
	}
}

func TestStdioHandshakeLimit(t *testing.T) {
	t.Setenv("MCP_TEST_HELPER", "1")
	client, err := NewClient(ServerConfig{ID: "stdio-handshake-limit", Transport: TransportStdio, Process: &ProcessSpec{Command: os.Args[0], Args: []string{"-test.run=^TestStdioHelper$"}, Env: map[string]string{"MCP_TEST_HELPER": "1"}}, Limits: Limits{MaxMessageBytes: 64}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Connect(t.Context())
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrLimitExceeded {
		t.Fatalf("stdio handshake limit: %v", err)
	}
}
