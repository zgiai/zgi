package mcpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProgressAndCatalogChangeEvents(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "events", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "progress", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if token := req.Params.GetProgressToken(); token != nil {
			_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: 1, Total: 1})
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "done"}}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer httpServer.Close()
	client, _ := NewClient(ServerConfig{ID: "events", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{EnableCatalogNotifications: true})
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.RefreshTools(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "progress"})
	if err != nil {
		t.Fatal(err)
	}
	server.AddTool(&mcp.Tool{Name: "new.tool", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	progress, changed := false, false
	deadline := time.After(5 * time.Second)
	for !progress || !changed {
		select {
		case event := <-conn.Events():
			if event.Kind == EventProgress && event.OperationID == result.OperationID {
				progress = true
			}
			if event.Kind == EventCatalogChanged && event.Category == "tools" {
				changed = true
			}
		case <-deadline:
			t.Fatalf("events missing: progress=%v change=%v", progress, changed)
		}
	}
	if !conn.ToolCatalog().Stale {
		t.Fatal("catalog was not marked stale")
	}
}
