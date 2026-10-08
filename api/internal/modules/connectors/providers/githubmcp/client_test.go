package githubmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

func testRemote(t *testing.T, includeRead bool) (*Client, func(), *[]string) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "github-fixture", Version: "1"}, nil)
	var mu sync.Mutex
	calls := []string{}
	add := func(name string, schema map[string]any, value map[string]any) {
		server.AddTool(&mcp.Tool{Name: name, InputSchema: schema}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			encoded, _ := json.Marshal(request.Params.Arguments)
			mu.Lock()
			calls = append(calls, name+":"+string(encoded))
			mu.Unlock()
			return &mcp.CallToolResult{StructuredContent: value}, nil
		})
	}
	add(toolGetMe, map[string]any{"type": "object"}, map[string]any{"id": 42, "login": "octocat"})
	add(toolListIssues, map[string]any{"type": "object", "required": []any{"owner", "repo", "state", "orderBy", "direction", "perPage"}, "properties": map[string]any{
		"owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "state": map[string]any{"const": "OPEN"}, "orderBy": map[string]any{"const": "UPDATED_AT"}, "direction": map[string]any{"const": "DESC"}, "perPage": map[string]any{"type": "integer"}, "fields": map[string]any{"type": "array"},
	}}, map[string]any{"issues": []any{
		map[string]any{"number": 12, "title": "Second", "state": "OPEN", "updated_at": "2026-10-08T12:00:00Z"},
		map[string]any{"number": 11, "title": "First", "state": "OPEN", "updated_at": "2026-10-08T13:00:00Z"},
	}, "totalCount": 2, "pageInfo": map[string]any{"hasNextPage": false}})
	if includeRead {
		add(toolIssueRead, map[string]any{"type": "object", "required": []any{"method", "owner", "repo", "issue_number"}, "properties": map[string]any{
			"method": map[string]any{"const": "get"}, "owner": map[string]any{"type": "string"}, "repo": map[string]any{"type": "string"}, "issue_number": map[string]any{"type": "integer"},
		}}, map[string]any{"method": "get", "issue": map[string]any{"number": 11, "title": "First", "body": "Issue details", "state": "OPEN", "updated_at": "2026-10-08T13:00:00Z", "user": map[string]any{"login": "reporter"}}})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MCP-Readonly") != "true" || r.Header.Get("X-MCP-Tools") != "get_me,list_issues,issue_read" {
			t.Errorf("missing GitHub MCP read-only restriction")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	client, err := newAt(httpServer.URL, nil)
	if err != nil {
		httpServer.Close()
		t.Fatal(err)
	}
	return client, httpServer.Close, &calls
}

func TestGitHubMCPVerifyAndReadIssues(t *testing.T) {
	client, closeServer, calls := testRemote(t, true)
	defer closeServer()
	verified, err := client.Verify(t.Context(), "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	if verified.AccountID != "42" || verified.DisplayName != "octocat" || !reflect.DeepEqual(verified.Operations, []string{operationList, operationGet}) {
		t.Fatalf("verification: %#v", verified)
	}
	list, err := client.Invoke(t.Context(), operationList, "fixture-token", map[string]any{"owner": "gin-gonic", "repo": "gin", "limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	issues := list.Value["issues"].([]any)
	first := issues[0].(map[string]any)
	if len(issues) != 2 || first["number"] != 11 || first["html_url"] != "https://github.com/gin-gonic/gin/issues/11" || list.Value["transport"] != "mcp" {
		t.Fatalf("issue list: %#v", list.Value)
	}
	read, err := client.Invoke(t.Context(), operationGet, "fixture-token", map[string]any{"owner": "gin-gonic", "repo": "gin", "issue_number": 11})
	if err != nil {
		t.Fatal(err)
	}
	issue := read.Value["issue"].(map[string]any)
	if issue["body"] != "Issue details" || issue["html_url"] != "https://github.com/gin-gonic/gin/issues/11" {
		t.Fatalf("issue detail: %#v", issue)
	}
	if len(*calls) != 3 {
		t.Fatalf("unexpected MCP calls: %#v", *calls)
	}
	var listArgs, readArgs map[string]any
	_ = json.Unmarshal([]byte((*calls)[1][len(toolListIssues)+1:]), &listArgs)
	_ = json.Unmarshal([]byte((*calls)[2][len(toolIssueRead)+1:]), &readArgs)
	if listArgs["state"] != "OPEN" || listArgs["orderBy"] != "UPDATED_AT" || listArgs["direction"] != "DESC" || listArgs["perPage"] != float64(5) || readArgs["method"] != "get" {
		t.Fatalf("unexpected remote arguments: list=%#v read=%#v", listArgs, readArgs)
	}
}

func TestGitHubMCPRejectsUnavailableToolAndInvalidOperation(t *testing.T) {
	client, closeServer, calls := testRemote(t, false)
	defer closeServer()
	if _, err := client.Verify(t.Context(), "fixture-token"); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInsufficientKey {
		t.Fatalf("missing issue_read must reject connection: %v", err)
	}
	if _, err := client.Invoke(t.Context(), operationGet, "fixture-token", map[string]any{"owner": "gin-gonic", "repo": "gin", "issue_number": 11}); err == nil || providers.FailureOf(err).Code != core.ErrorCodeConnectorUnavailable {
		t.Fatalf("missing issue_read must reject invocation: %v", err)
	}
	if _, err := client.Invoke(t.Context(), "issue_read", "fixture-token", map[string]any{"method": "create"}); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
		t.Fatalf("arbitrary MCP method must reject: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("unsafe tool invocation: %#v", *calls)
	}
}

func TestGitHubMCPRejectsInvalidPAT(t *testing.T) {
	client, closeServer, calls := testRemote(t, true)
	defer closeServer()
	if _, err := client.Verify(t.Context(), "wrong-token"); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidKey {
		t.Fatalf("invalid PAT result: %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("invalid PAT called MCP tool: %#v", *calls)
	}
}
