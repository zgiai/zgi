package zgimarket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type seenRequest struct {
	method, path, query, body string
	header                    http.Header
}

func newTestProxy(t *testing.T, member, admin bool) (*gin.Engine, *[]seenRequest) {
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
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"code":0,"data":{"ok":true}}`) //nolint:errcheck // test server
	}))
	t.Cleanup(console.Close)

	proxy := NewProxy(
		Config{ConsoleURL: console.URL + "/", InternalKey: "shared-key"},
		func(c *gin.Context) string { return c.GetHeader("X-Test-Organization") },
		func(c *gin.Context) string { return c.GetHeader("X-Test-Account") },
		func(*gin.Context, string, string) (bool, bool, error) { return member, admin, nil },
	)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return engine, &seen
}

func send(engine *gin.Engine, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("X-Test-Organization", "org-1")
	request.Header.Set("X-Test-Account", "account-1")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestForwardNamesTheCallerItselfAndReturnsTheAnswerUnchanged(t *testing.T) {
	engine, seen := newTestProxy(t, true, true)
	recorder := send(engine, http.MethodPost, "/workspaces/current/zgi-market/subscriptions?source=pre", `{"listing_id":7}`, map[string]string{
		"Content-Type": "application/json",
		// A browser cannot name another organization, account, or role.
		"X-ZGI-Billing-Organization-ID": "org-other", "X-ZGI-Account-ID": "account-other", "X-ZGI-Organization-Role": "owner",
		"Authorization": "Bearer user-session",
	})
	if recorder.Code != http.StatusCreated || recorder.Body.String() != `{"code":0,"data":{"ok":true}}` {
		t.Fatalf("answer = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(*seen) != 1 {
		t.Fatalf("console calls = %d", len(*seen))
	}
	call := (*seen)[0]
	if call.path != "/v1/internal/zgi-market/proxy/subscriptions" || call.query != "source=pre" || call.body != `{"listing_id":7}` {
		t.Fatalf("forwarded %s %s?%s %s", call.method, call.path, call.query, call.body)
	}
	for name, want := range map[string]string{
		"X-ZGI-Billing-Organization-ID": "org-1", "X-ZGI-Account-ID": "account-1", "X-ZGI-Organization-Role": "admin",
		"X-ZGI-Internal-Secret": "shared-key",
	} {
		if got := call.header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if call.header.Get("Authorization") != "" {
		t.Error("the member's session was passed on")
	}
	if call.header.Get("X-Internal-Signature") == "" {
		t.Error("the request to the console is not signed")
	}

	// A plain member is named as one, and the market's redirect after the OAuth callback is
	// handed back as the page to open.
	engine, seen = newTestProxy(t, true, false)
	recorder = send(engine, http.MethodGet, "/workspaces/current/zgi-market/connections/oauth/callback?state=s", "", nil)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "https://app.example.com/done") {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Body.String())
	}
	if role := (*seen)[0].header.Get("X-ZGI-Organization-Role"); role != "member" {
		t.Errorf("role = %q", role)
	}
}

func TestForwardRefusesBeforeAskingTheConsole(t *testing.T) {
	engine, seen := newTestProxy(t, true, false)
	for _, target := range []string{
		"/workspaces/current/zgi-market/wallet", "/workspaces/current/zgi-market/", "/workspaces/current/zgi-market/listings/../wallet",
		"/workspaces/current/zgi-market/recharge-orders", "/workspaces/current/zgi-market/operator/market/listings",
	} {
		if recorder := send(engine, http.MethodGet, target, "", nil); recorder.Code == http.StatusCreated {
			t.Errorf("%s was forwarded", target)
		}
	}
	// Someone who is not a member of the organization, and a request without an organization.
	outsider, outsiderSeen := newTestProxy(t, false, false)
	send(outsider, http.MethodGet, "/workspaces/current/zgi-market/listings", "", nil)
	send(engine, http.MethodGet, "/workspaces/current/zgi-market/listings", "", map[string]string{"X-Test-Organization": ""})
	if len(*seen) != 0 || len(*outsiderSeen) != 0 {
		t.Fatalf("console was asked %d and %d times", len(*seen), len(*outsiderSeen))
	}

	unconfigured := NewProxy(Config{}, func(*gin.Context) string { return "org-1" }, func(*gin.Context) string { return "a" },
		func(*gin.Context, string, string) (bool, bool, error) { return true, false, nil })
	plain := gin.New()
	plain.Any("/workspaces/current/zgi-market/*path", unconfigured.Forward)
	if recorder := send(plain, http.MethodGet, "/workspaces/current/zgi-market/listings", "", nil); recorder.Code < 400 {
		t.Errorf("an unconfigured proxy answered %d", recorder.Code)
	}
}

func TestForwardReachesTheCallRecordsOfTheOrganization(t *testing.T) {
	engine, seen := newTestProxy(t, true, false)
	recorder := send(engine, http.MethodGet, "/workspaces/current/zgi-market/calls?outcome=refused", "", nil)
	if recorder.Code != http.StatusCreated || len(*seen) != 1 {
		t.Fatalf("answer = %d, console calls = %d", recorder.Code, len(*seen))
	}
	if call := (*seen)[0]; call.path != "/v1/internal/zgi-market/proxy/calls" || call.query != "outcome=refused" {
		t.Fatalf("forwarded %s?%s", call.path, call.query)
	}
}

func TestMetaIsAnsweredHereForAMemberOnly(t *testing.T) {
	console := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the console was asked for the meta")
	}))
	t.Cleanup(console.Close)
	gin.SetMode(gin.TestMode)
	build := func(member, admin bool, gateway string) *gin.Engine {
		proxy := NewProxy(
			Config{ConsoleURL: console.URL, GatewayURL: gateway},
			func(c *gin.Context) string { return c.GetHeader("X-Test-Organization") },
			func(c *gin.Context) string { return c.GetHeader("X-Test-Account") },
			func(*gin.Context, string, string) (bool, bool, error) { return member, admin, nil },
		)
		engine := gin.New()
		engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
		return engine
	}
	const target = "/workspaces/current/zgi-market/meta"

	recorder := send(build(true, true, "https://market.example.com"), http.MethodGet, target, "", nil)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"gateway_url":"https://market.example.com"`) ||
		!strings.Contains(recorder.Body.String(), `"admin":true`) {
		t.Fatalf("administrator: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = send(build(true, false, ""), http.MethodGet, target, "", nil)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"gateway_url":""`) ||
		!strings.Contains(recorder.Body.String(), `"admin":false`) {
		t.Fatalf("member without a gateway: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder = send(build(false, false, "https://market.example.com"), http.MethodGet, target, "", nil); recorder.Code == http.StatusOK ||
		strings.Contains(recorder.Body.String(), "market.example.com") {
		t.Fatalf("an outsider read the meta: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder = send(build(true, true, ""), http.MethodPost, target, "{}", nil); recorder.Code == http.StatusOK {
		t.Fatalf("a write to the meta answered %d", recorder.Code)
	}
}

func TestForwardLetsABrowserReadASkillManifestAndNothingElseUnderSkills(t *testing.T) {
	engine, seen := newTestProxy(t, true, false)
	recorder := send(engine, http.MethodGet, "/workspaces/current/zgi-market/skills/csv-summary/manifest", "", nil)
	if recorder.Code != http.StatusCreated || len(*seen) != 1 || (*seen)[0].path != "/v1/internal/zgi-market/proxy/skills/csv-summary/manifest" {
		t.Fatalf("answer = %d, console calls = %v", recorder.Code, *seen)
	}
	for _, refused := range []struct{ method, path string }{
		{http.MethodGet, "/workspaces/current/zgi-market/skills/csv-summary/package"},
		{http.MethodPost, "/workspaces/current/zgi-market/skills/csv-summary/manifest"},
		{http.MethodGet, "/workspaces/current/zgi-market/skills/csv-summary"},
		{http.MethodGet, "/workspaces/current/zgi-market/skills/CSV/manifest"},
		{http.MethodGet, "/workspaces/current/zgi-market/skills/a/b/manifest"},
	} {
		if recorder := send(engine, refused.method, refused.path, "", nil); recorder.Code == http.StatusCreated {
			t.Errorf("%s %s was forwarded", refused.method, refused.path)
		}
	}
	if len(*seen) != 1 {
		t.Fatalf("console calls = %d", len(*seen))
	}
}
