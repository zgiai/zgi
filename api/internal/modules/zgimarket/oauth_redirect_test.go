package zgimarket

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testOAuthRedirect = "https://app.example.com/dashboard/market/oauth/callback"

func newRedirectProxy(t *testing.T, address string) (*gin.Engine, *[]seenRequest) {
	t.Helper()
	seen := []seenRequest{}
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test server
		seen = append(seen, seenRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		if strings.HasSuffix(r.URL.Path, "/oauth/callback") {
			w.Header().Set("Location", "https://app.example.com/done")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{}}`) //nolint:errcheck // test server
	}))
	t.Cleanup(console.Close)
	proxy := NewProxy(
		Config{ConsoleURL: console.URL, OAuthRedirectURL: address},
		func(c *gin.Context) string { return c.GetHeader("X-Test-Organization") },
		func(c *gin.Context) string { return c.GetHeader("X-Test-Account") },
		func(*gin.Context, string, string) (bool, bool, error) { return true, true, nil },
	)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return engine, &seen
}

func bodyOf(t *testing.T, raw string) map[string]any {
	t.Helper()
	members := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &members); err != nil {
		t.Fatalf("forwarded body %q: %v", raw, err)
	}
	return members
}

func TestAuthorizationIsStartedWithTheCallbackAddressOfThisInstance(t *testing.T) {
	engine, seen := newRedirectProxy(t, " "+testOAuthRedirect+" ")
	const base = "/workspaces/current/zgi-market/"
	asJSON := map[string]string{"Content-Type": "application/json"}

	// The browser cannot choose where the vendor sends it back.
	send(engine, http.MethodPost, base+"connections",
		`{"connector_slug":"drive","name":"team","return_url":"https://app.example.com/x","redirect_url":"https://evil.example/cb"}`, asJSON)
	// A reauthorization may have no body at all.
	send(engine, http.MethodPost, base+"connections/12/reauthorize", "", nil)
	send(engine, http.MethodPost, base+"connections/12/reauthorize", `{"return_url":"https://app.example.com/y"}`, asJSON)
	// Nothing else is touched.
	send(engine, http.MethodPost, base+"connections/12/disable", "", nil)
	send(engine, http.MethodPost, base+"subscriptions", `{"listing_id":7,"redirect_url":"kept"}`, asJSON)
	send(engine, http.MethodPost, base+"connections", `not json`, asJSON)
	if len(*seen) != 6 {
		t.Fatalf("console calls = %d", len(*seen))
	}

	created := bodyOf(t, (*seen)[0].body)
	if created["redirect_url"] != testOAuthRedirect || created["connector_slug"] != "drive" ||
		created["name"] != "team" || created["return_url"] != "https://app.example.com/x" || len(created) != 4 {
		t.Fatalf("create forwarded %v", created)
	}
	bare := (*seen)[1]
	if got := bodyOf(t, bare.body); got["redirect_url"] != testOAuthRedirect || len(got) != 1 ||
		bare.header.Get("Content-Type") != "application/json" {
		t.Fatalf("reauthorize without a body forwarded %q as %q", bare.body, bare.header.Get("Content-Type"))
	}
	if got := bodyOf(t, (*seen)[2].body); got["redirect_url"] != testOAuthRedirect || got["return_url"] != "https://app.example.com/y" {
		t.Fatalf("reauthorize forwarded %v", got)
	}
	if (*seen)[3].body != "" || (*seen)[4].body != `{"listing_id":7,"redirect_url":"kept"}` || (*seen)[5].body != "not json" {
		t.Fatalf("untouched requests were forwarded as %q, %q, %q", (*seen)[3].body, (*seen)[4].body, (*seen)[5].body)
	}
}

func TestCallbackNamesTheAddressTheBrowserArrivedAt(t *testing.T) {
	engine, seen := newRedirectProxy(t, testOAuthRedirect)
	recorder := send(engine, http.MethodGet,
		"/workspaces/current/zgi-market/connections/oauth/callback?state=s1&code=c1&redirect_url=https%3A%2F%2Fevil.example%2Fcb", "", nil)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "https://app.example.com/done") {
		t.Fatalf("callback answered %d %s", recorder.Code, recorder.Body.String())
	}
	query, err := url.ParseQuery((*seen)[0].query)
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("state") != "s1" || query.Get("code") != "c1" || len(query["redirect_url"]) != 1 ||
		query.Get("redirect_url") != testOAuthRedirect {
		t.Fatalf("callback forwarded %q", (*seen)[0].query)
	}
}

func TestWithoutACallbackAddressTheMarketDefaultIsLeftAlone(t *testing.T) {
	for _, address := range []string{
		"", "http://app.example.com/dashboard/market/oauth/callback", "https://user@app.example.com/cb",
		"https://app.example.com/cb?x=1", "https://app.example.com/cb#f", "/dashboard/market/oauth/callback",
	} {
		engine, seen := newRedirectProxy(t, address)
		const base = "/workspaces/current/zgi-market/"
		send(engine, http.MethodPost, base+"connections", `{"connector_slug":"drive","name":"team"}`, nil)
		send(engine, http.MethodPost, base+"connections", `{"connector_slug":"drive","name":"team","redirect_url":"https://evil.example/cb"}`, nil)
		send(engine, http.MethodPost, base+"connections/12/reauthorize", "", nil)
		send(engine, http.MethodGet, base+"connections/oauth/callback?state=s1&code=c1&redirect_url=x", "", nil)
		if (*seen)[0].body != `{"connector_slug":"drive","name":"team"}` {
			t.Fatalf("%q: a body that names no address was changed to %q", address, (*seen)[0].body)
		}
		if got := bodyOf(t, (*seen)[1].body); len(got) != 2 || got["redirect_url"] != nil {
			t.Fatalf("%q: the browser's address was forwarded: %v", address, got)
		}
		if (*seen)[2].body != "" {
			t.Fatalf("%q: reauthorize forwarded %q", address, (*seen)[2].body)
		}
		if query, _ := url.ParseQuery((*seen)[3].query); query.Has("redirect_url") || query.Get("state") != "s1" { //nolint:errcheck // asserted below
			t.Fatalf("%q: callback forwarded %q", address, (*seen)[3].query)
		}
	}
	for address, want := range map[string]bool{
		testOAuthRedirect: true, "http://localhost:3100/dashboard/market/oauth/callback": true,
		"http://127.0.0.1:3100/cb": false, "https://app.example.com/cb ": false,
	} {
		if ValidOAuthRedirectURL(address) != want {
			t.Fatalf("ValidOAuthRedirectURL(%q) != %v", address, want)
		}
	}
}
