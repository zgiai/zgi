package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRawResultAndEmptyCursor(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "raw-test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "Case.Tool", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "placeholder"}}}, nil
	})
	base := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBytes, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(requestBytes))
		var request map[string]json.RawMessage
		_ = json.Unmarshal(requestBytes, &request)
		var method string
		_ = json.Unmarshal(request["method"], &method)
		if method != "tools/list" && method != "tools/call" {
			base.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		base.ServeHTTP(recorder, r)
		body := recorder.Body.Bytes()
		if recorder.Code == http.StatusOK && (method == "tools/list" || method == "tools/call") {
			var envelope map[string]json.RawMessage
			_ = json.Unmarshal(body, &envelope)
			if method == "tools/call" {
				envelope["result"] = json.RawMessage(`{"resultType":"complete","content":[{"type":"text","text":"hello","vendorField":7}],"structuredContent":null,"vendorResult":{"x":1}}`)
			} else {
				var params map[string]json.RawMessage
				_ = json.Unmarshal(request["params"], &params)
				if _, present := params["cursor"]; present {
					envelope["result"] = json.RawMessage(`{"resultType":"complete","tools":[{"name":"Other.tool","inputSchema":{"type":"object"}}]}`)
				} else {
					envelope["result"] = json.RawMessage(`{"resultType":"complete","tools":[{"name":"Case.Tool","inputSchema":{"type":"object","$defs":{"item":{"type":"string"}},"properties":{"x":{"$ref":"#/$defs/item"}}},"annotations":{"readOnlyHint":true},"vendorTool":9}],"nextCursor":""}`)
				}
			}
			body, _ = json.Marshal(envelope)
		}
		for key, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(body)
	}))
	defer httpServer.Close()
	client, err := NewClient(ServerConfig{ID: "raw", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	page, err := conn.ListToolsPage(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor == nil || *page.NextCursor != "" || len(page.Items) != 1 || page.Items[0].Name != "Case.Tool" {
		t.Fatalf("first page: %#v", page)
	}
	if !bytes.Contains(page.Items[0].InputSchema, []byte(`"$defs"`)) || !bytes.Contains(page.Items[0].Raw, []byte(`"vendorTool"`)) || !page.Items[0].Annotations.Present {
		t.Fatalf("tool metadata lost: %#v", page.Items[0])
	}
	second, err := conn.ListToolsPage(t.Context(), page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Name != "Other.tool" {
		t.Fatalf("empty cursor was not forwarded: %#v", second)
	}
	result, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "Case.Tool"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.StructuredContent.Present || string(result.StructuredContent.JSON) != "null" || !bytes.Contains(result.RawResult, []byte(`"vendorResult"`)) || len(result.Content) != 1 || !bytes.Contains(result.Content[0].Raw, []byte(`"vendorField"`)) {
		t.Fatalf("raw result lost: %#v", result)
	}
	catalog, err := conn.RefreshTools(t.Context())
	if err != nil || len(catalog.Items) != 2 {
		t.Fatalf("refresh: %#v %v", catalog, err)
	}
}
