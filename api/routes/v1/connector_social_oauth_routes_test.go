package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialoauth"
	"github.com/zgiai/zgi/api/internal/util"
)

type oauthRouteStub struct {
	org, owner, connection, flow uuid.UUID
	provider, state, code, page  string
	denied                       bool
	err                          error
}

func (s *oauthRouteStub) Available(id string) bool { return id == "x-social" }
func (s *oauthRouteStub) Start(_ context.Context, o, a uuid.UUID, id string, c uuid.UUID) (socialoauth.Flow, error) {
	s.org = o
	s.owner = a
	s.connection = c
	s.provider = id
	return socialoauth.Flow{ID: s.flow, Status: "pending", AuthorizeURL: "https://x.com/i/oauth2/authorize?state=state-secret"}, s.err
}
func (s *oauthRouteStub) Complete(_ context.Context, p, state, code string, denied bool) (uuid.UUID, error) {
	s.provider = p
	s.state = state
	s.code = code
	s.denied = denied
	return s.flow, s.err
}
func (s *oauthRouteStub) Get(_ context.Context, o, a, id uuid.UUID) (socialoauth.Flow, error) {
	s.org = o
	s.owner = a
	s.flow = id
	return socialoauth.Flow{ID: id, Status: "ready"}, s.err
}
func (s *oauthRouteStub) SelectPage(_ context.Context, o, a, id uuid.UUID, page string) (socialoauth.Flow, error) {
	s.org = o
	s.owner = a
	s.flow = id
	s.page = page
	return socialoauth.Flow{ID: id, Status: "ready"}, s.err
}
func socialOAuthRouter(stub *oauthRouteStub, org, owner uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &connectorHandler{socialOAuth: stub, socialOAuthFrontendURL: "https://console.example"}
	h.registerSocialOAuthCallback(r.Group("/console/api"))
	g := r.Group("/console/api")
	g.Use(func(c *gin.Context) { util.SetOrganizationID(c, org.String()); c.Set("account_id", owner.String()) })
	h.registerSocialOAuth(g)
	return r
}
func TestSocialOAuthRoutesActorAndStrictInput(t *testing.T) {
	org, owner, flow, connection := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	s := &oauthRouteStub{flow: flow}
	r := socialOAuthRouter(s, org, owner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/console/api/connectors/x-social/oauth", strings.NewReader(`{"connection_id":"`+connection.String()+`"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || s.org != org || s.owner != owner || s.connection != connection {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge != 600 {
		t.Fatal("missing secure browser binding cookie")
	}
	callback := httptest.NewRequest("GET", "/console/api/connectors/x-social/oauth/callback?state=state-secret&error=access_denied&error_description=private-details", nil)
	callback.AddCookie(cookies[0])
	done := httptest.NewRecorder()
	r.ServeHTTP(done, callback)
	if done.Code != http.StatusSeeOther || !s.denied || strings.Contains(done.Body.String(), "private-details") {
		t.Fatal("denied callback did not safely return")
	}
	cleared := done.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge != -1 {
		t.Fatal("callback cookie not consumed")
	}
	for _, body := range []string{`{"redirect_url":"https://evil.example"}`, `{"connection_id":"bad"}`, `{"connection_id":"` + connection.String() + `"} {}`} {
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connectors/x-social/oauth", strings.NewReader(body)))
		if rec.Code < 400 {
			t.Fatalf("accepted %s", body)
		}
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connectors/oauth-flows/"+flow.String()+"/page", strings.NewReader(`{"page_id":"page1"}`)))
	if rec.Code != 200 || s.page != "page1" || s.org != org || s.owner != owner {
		t.Fatal("page selection lost actor")
	}
}
func TestSocialOAuthCallbackFixedRedirectAndNoSecrets(t *testing.T) {
	flow := uuid.New()
	s := &oauthRouteStub{flow: flow}
	r := socialOAuthRouter(s, uuid.Nil, uuid.Nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/api/connectors/x-social/oauth/callback?state=state-secret&code=code-secret&redirect_uri=https://evil.example", nil)
	req.AddCookie(socialOAuthBrowserCookie("x-social", "state-secret"))
	r.ServeHTTP(rec, req)
	u, e := url.Parse(rec.Header().Get("Location"))
	if e != nil || u.Host != "console.example" || u.Path != "/console/connectors/oauth" || u.Query().Get("flow_id") != flow.String() {
		t.Fatalf("unsafe redirect %s", rec.Header().Get("Location"))
	}
	if s.state != "state-secret" || s.code != "code-secret" {
		t.Fatal("callback not dispatched")
	}
	if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Header().Get("Location"), "secret") || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("secret callback output")
	}
	s.flow = uuid.Nil
	s.err = core.NewError(core.ErrorCodeFlowExpired, false, nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connectors/x-social/oauth/callback?state=bad&code=code-secret", nil))
	if rec.Code < 400 || strings.Contains(rec.Body.String(), "code-secret") {
		t.Fatal("invalid state accepted")
	}
}
func TestSocialOAuthCallbackRejectsAmbiguousQuery(t *testing.T) {
	s := &oauthRouteStub{flow: uuid.New()}
	r := socialOAuthRouter(s, uuid.New(), uuid.New())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connectors/x-social/oauth/callback?state=a&state=b&code=c", nil))
	if rec.Code < 400 || s.state != "" {
		t.Fatal("ambiguous state dispatched")
	}
}

func TestSocialOAuthCallbackRequiresInitiatingBrowser(t *testing.T) {
	s := &oauthRouteStub{flow: uuid.New()}
	r := socialOAuthRouter(s, uuid.New(), uuid.New())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connectors/x-social/oauth/callback?state=from-another-browser&code=code", nil))
	if rec.Code < 400 || s.state != "" {
		t.Fatal("callback accepted outside initiating browser")
	}
}

func (s *oauthRouteStub) CompleteInstallation(_ context.Context, o, a, id uuid.UUID) (socialoauth.Flow, error) {
	s.org = o
	s.owner = a
	s.flow = id
	return socialoauth.Flow{ID: id, ConnectorID: "github", Status: "ready"}, s.err
}
func TestGitHubOAuthRoutesAndBrowserBinding(t *testing.T) {
	org, owner, flow := uuid.New(), uuid.New(), uuid.New()
	s := &oauthRouteStub{flow: flow}
	r := socialOAuthRouter(s, org, owner)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/console/api/connectors/github/oauth", nil))
	if w.Code != 200 {
		t.Fatal("GitHub OAuth availability not registered", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/console/api/connectors/oauth-flows/"+flow.String()+"/installation", strings.NewReader(`{}`)))
	if w.Code != 200 || s.org != org || s.owner != owner {
		t.Fatal("installation resume not actor scoped", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/console/api/connectors/oauth-flows/"+flow.String()+"/installation", strings.NewReader(`{"installation_id":123}`)))
	if w.Code != 400 {
		t.Fatal("browser installation id trusted", w.Code)
	}
	req := httptest.NewRequest("GET", "/console/api/connectors/github/oauth/callback?state=state-secret&code=code-secret", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 303 {
		t.Fatal("callback accepted without browser cookie")
	}
	req = httptest.NewRequest("GET", "/console/api/connectors/github/oauth/callback?state=state-secret&code=code-secret", nil)
	req.AddCookie(socialOAuthBrowserCookie("github", "state-secret"))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 303 || s.provider != "github" || strings.Contains(w.Header().Get("Location"), "secret") {
		t.Fatal("GitHub callback failed", w.Code)
	}
}
