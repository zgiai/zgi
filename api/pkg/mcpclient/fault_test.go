package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolIsErrorAndCancellationUncertainty(t *testing.T) {
	started := make(chan struct{})
	server := mcp.NewServer(&mcp.Implementation{Name: "faults", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "fail", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "remote failure"}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "slow", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "faults", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	failed, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "fail"})
	if err != nil || !failed.IsError || len(failed.Content) != 1 {
		t.Fatalf("tool isError: %#v %v", failed, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := conn.CallTool(ctx, ToolCallRequest{Name: "slow"}); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow tool was not reached")
	}
	cancel()
	select {
	case err := <-result:
		var classified *MCPError
		if !errors.As(err, &classified) || classified.Kind != ErrCancelled || !classified.MayHaveExecuted {
			t.Fatalf("cancelled call classification: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not finish")
	}
}

func TestHTTPAuthenticationAndResponseLimit(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer unauthorized.Close()
	client, _ := NewClient(ServerConfig{ID: "auth", Transport: TransportStreamableHTTP, Endpoint: unauthorized.URL}, Options{})
	_, err := client.Connect(t.Context())
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrAuthRequired {
		t.Fatalf("expected auth_required, got %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "large", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "large", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", 32<<10)}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	client, _ = NewClient(ServerConfig{ID: "limited", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL, Limits: Limits{MaxMessageBytes: 16 << 10}}, Options{})
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	_, err = conn.CallTool(t.Context(), ToolCallRequest{Name: "large"})
	if !errors.As(err, &classified) || classified.Kind != ErrLimitExceeded {
		t.Fatalf("expected limit_exceeded, got %v, cause %v", err, classified.Cause)
	}
}

func TestMalformedJSONIsProtocolError(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{`))
	}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "malformed", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
	_, err := client.Connect(t.Context())
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrProtocol {
		t.Fatalf("malformed response: %v", err)
	}
}

func TestRecognizedModernDiscoveryErrorDoesNotSendLegacyInitialize(t *testing.T) {
	var legacyRequests atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Method == "initialize" {
			legacyRequests.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Protocol-Version", "2026-07-28")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": jsonrpc.CodeInvalidParams, "message": "invalid discovery"}})
	}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "modern-error", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
	_, err := client.Connect(t.Context())
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrProtocol || legacyRequests.Load() != 0 {
		t.Fatalf("modern error fallback: %v, initialize calls=%d", err, legacyRequests.Load())
	}
}

func TestHTTPSSEResponse(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "sse", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "sse", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	result, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "ping"})
	if err != nil || len(result.Content) != 1 {
		t.Fatalf("SSE result: %#v %v", result, err)
	}
}

func TestConcurrencyLimitRejectsUnsentCall(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := mcp.NewServer(&mcp.Implementation{Name: "limit", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "wait", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return &mcp.CallToolResult{}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "limit", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL, Limits: Limits{MaxConcurrentRequests: 1}}, Options{})
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	firstDone := make(chan error, 1)
	go func() { _, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "wait"}); firstDone <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first call never reached server")
	}
	secondCtx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, err = conn.CallTool(secondCtx, ToolCallRequest{Name: "wait"})
	var classified *MCPError
	if !errors.As(err, &classified) || classified.Kind != ErrTimeout || classified.MayHaveExecuted {
		t.Fatalf("queued call classification: %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}
