package mcpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
)

type testTokenStore struct {
	mu    sync.Mutex
	saved OAuthTokens
	calls int
}

func (s *testTokenStore) Load(context.Context, string) (OAuthTokens, bool, error) {
	return OAuthTokens{}, false, nil
}
func (s *testTokenStore) Save(_ context.Context, _ string, value OAuthTokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = value
	s.calls++
	return nil
}
func (s *testTokenStore) Delete(context.Context, string) error { return nil }

type fixedTokenSource struct{ token *oauth2.Token }

func (s fixedTokenSource) Token() (*oauth2.Token, error) { return s.token, nil }

func TestOAuthRefreshPersistsChangedToken(t *testing.T) {
	store := &testTokenStore{}
	source := &savingTokenSource{source: fixedTokenSource{token: &oauth2.Token{AccessToken: "new", RefreshToken: "refresh"}}, store: store, scope: "opaque-host-scope", last: oauth2.Token{AccessToken: "old"}}
	if _, err := source.Token(); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.saved.Token.AccessToken != "new" {
		t.Fatalf("token refresh not saved: %#v", store.saved)
	}
	if _, err := source.Token(); err != nil || store.calls != 1 {
		t.Fatalf("unchanged token saved again: %v, %d", err, store.calls)
	}
}

func TestBearerCredentialOnEveryHTTPRequest(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "bearer", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "ping", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	var mu sync.Mutex
	requests := 0
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		requests++
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	client, err := NewClient(ServerConfig{ID: "bearer", Transport: TransportStreamableHTTP, Endpoint: httpServer.URL}, Options{HTTPAuth: HTTPAuthConfig{Mode: AuthBearer, Credentials: BearerToken("secret")}})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := client.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.ListToolsPage(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.CallTool(t.Context(), ToolCallRequest{Name: "ping"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	count := requests
	mu.Unlock()
	if count < 3 {
		t.Fatalf("only %d authenticated requests", count)
	}
}
