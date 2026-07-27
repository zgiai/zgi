package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type oauthRecoveryRouteRepository struct {
	summary        integrations.OAuthRecoveryAdminSummary
	acknowledged   bool
	organizationID uuid.UUID
	operationRef   string
	actorID        uuid.UUID
	resolutionCode string
}

func (repository *oauthRecoveryRouteRepository) OAuthRecoverySummary(
	_ context.Context,
	organizationID uuid.UUID,
	_ int,
) (integrations.OAuthRecoveryAdminSummary, error) {
	repository.organizationID = organizationID
	return repository.summary, nil
}

func (repository *oauthRecoveryRouteRepository) AcknowledgeOAuthRecovery(
	_ context.Context,
	organizationID uuid.UUID,
	operationRef string,
	actorID uuid.UUID,
	resolutionCode string,
) error {
	repository.acknowledged = true
	repository.organizationID = organizationID
	repository.operationRef = operationRef
	repository.actorID = actorID
	repository.resolutionCode = resolutionCode
	return nil
}

func TestOAuthResultRedirectURLContainsOnlySafeResultFields(t *testing.T) {
	target, err := oauthResultRedirectURL(
		"https://console.example.com/console/integrations/oauth/result?theme=dark#ignored",
		"V0u0V1qHib3hpmh8v_1mXFlNyWhw2xB5",
		integrations.OAuthFlowSucceeded,
		"",
	)
	if err != nil {
		t.Fatalf("build OAuth result redirect: %v", err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse OAuth result redirect: %v", err)
	}
	if parsed.Fragment != "" {
		t.Fatalf("OAuth result redirect must remove fragments, got %q", parsed.Fragment)
	}
	if parsed.Query().Get("flow") == "" || parsed.Query().Get("status") != string(integrations.OAuthFlowSucceeded) {
		t.Fatalf("OAuth result redirect is missing safe state: %s", target)
	}
	if parsed.Query().Get("theme") != "dark" {
		t.Fatalf("configured result URL query should be retained: %s", target)
	}
	for _, forbidden := range []string{"code", "state", "access_token", "refresh_token", "client_secret", "connection_id"} {
		if _, exists := parsed.Query()[forbidden]; exists {
			t.Fatalf("OAuth result redirect leaked %s: %s", forbidden, target)
		}
	}
}

func TestOAuthRecoverySummaryRouteReturnsOnlySafeRemediationMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID := uuid.New(), uuid.New()
	repository := &oauthRecoveryRouteRepository{summary: integrations.OAuthRecoveryAdminSummary{
		PendingRevocations: 1, ManualActionRequired: 1, UnresolvedDeadLetters: 1,
		RemediationOperations: []integrations.OAuthRecoveryRemediationItem{{
			OperationRef: "revoke-safe-reference", IntegrationID: "feishu",
			AuthMethodID: "user_oauth", ReasonCode: "manual_provider_revocation_required",
			CreatedAt: time.Now().UTC(), FailedAt: time.Now().UTC(),
		}},
	}}
	handler := &integrationHandler{deps: IntegrationRouteDeps{OAuthRecovery: repository}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/oauth-recovery", nil)
	ctx.Set("organization_id", organizationID.String())
	ctx.Set("account_id", accountID.String())
	handler.oauthRecoverySummary(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("oauthRecoverySummary() status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"access_token", "refresh_token", "client_secret", "encrypted_credentials", "connection_id"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("oauthRecoverySummary() leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "manual_provider_revocation_required") ||
		repository.organizationID != organizationID {
		t.Fatalf("oauthRecoverySummary() body = %s, organization = %s", body, repository.organizationID)
	}
}

func TestOAuthRecoveryAcknowledgementRouteUsesAuthenticatedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID := uuid.New(), uuid.New()
	repository := &oauthRecoveryRouteRepository{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{OAuthRecovery: repository}}
	body, _ := json.Marshal(map[string]string{
		"resolution_code": integrations.OAuthRecoveryResolutionProviderAccessRemoved,
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/console/api/integrations/oauth-recovery/revoke-safe-reference/acknowledge",
		bytes.NewReader(body),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "operation_ref", Value: "revoke-safe-reference"}}
	ctx.Set("organization_id", organizationID.String())
	ctx.Set("account_id", accountID.String())
	handler.acknowledgeOAuthRecovery(ctx)

	if recorder.Code != http.StatusOK || !repository.acknowledged {
		t.Fatalf("acknowledgeOAuthRecovery() status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if repository.organizationID != organizationID || repository.actorID != accountID ||
		repository.operationRef != "revoke-safe-reference" ||
		repository.resolutionCode != integrations.OAuthRecoveryResolutionProviderAccessRemoved {
		t.Fatalf("acknowledgement context = %#v", repository)
	}
}

func TestOAuthResultRedirectURLNormalizesUnsafeProviderError(t *testing.T) {
	target, err := oauthResultRedirectURL(
		"http://localhost:3000/console/integrations/oauth/result",
		"",
		integrations.OAuthFlowFailed,
		"provider_error_with_sensitive_details",
	)
	if err != nil {
		t.Fatalf("build OAuth result redirect: %v", err)
	}
	if strings.Contains(target, "provider_error_with_sensitive_details") {
		t.Fatalf("OAuth result redirect reflected provider error details: %s", target)
	}
	parsed, _ := url.Parse(target)
	if parsed.Query().Get("error_code") != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("unexpected safe error code: %s", target)
	}
}

func TestOAuthResultRedirectURLRejectsRelativeTarget(t *testing.T) {
	if _, err := oauthResultRedirectURL("/console/integrations/oauth/result", "opaque", integrations.OAuthFlowFailed, ""); err == nil {
		t.Fatal("expected relative OAuth result URL to be rejected")
	}
}

func TestOAuthProviderSetupURLPrefersOAuthApplicationConsole(t *testing.T) {
	method := integrations.AuthMethodDefinition{
		OAuth: &integrations.OAuthMethodMetadata{
			ProviderSetupURL: "https://console.example.com/oauth/apps",
		},
	}
	if got := oauthProviderSetupURL(method, "https://docs.example.com"); got != "https://console.example.com/oauth/apps" {
		t.Fatalf("oauthProviderSetupURL() = %q", got)
	}
	if got := oauthProviderSetupURL(integrations.AuthMethodDefinition{}, "https://docs.example.com"); got != "https://docs.example.com" {
		t.Fatalf("oauthProviderSetupURL() fallback = %q", got)
	}
}

func TestOAuthBrowserBindingCookieSupportsConcurrentFlowsAndRejectsOtherBrowser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	callbackURL := "https://api.example.com/api/integrations/oauth/callback"

	browserAFirstRecorder := httptest.NewRecorder()
	browserAFirst, _ := gin.CreateTestContext(browserAFirstRecorder)
	browserAFirst.Request = httptest.NewRequest(http.MethodPost, "https://api.example.com/api/integrations/oauth/flows", nil)
	firstDigest, err := ensureOAuthBrowserBindingCookie(browserAFirst, callbackURL)
	if err != nil {
		t.Fatalf("create Browser A binding: %v", err)
	}
	firstCookies := browserAFirstRecorder.Result().Cookies()
	if len(firstCookies) != 1 {
		t.Fatalf("Browser A Set-Cookie count = %d", len(firstCookies))
	}
	bindingCookie := firstCookies[0]
	if bindingCookie.Name != oauthBrowserBindingSecureCookieName || !bindingCookie.HttpOnly ||
		!bindingCookie.Secure || bindingCookie.Path != "/" || bindingCookie.Domain != "" ||
		bindingCookie.MaxAge <= 0 || bindingCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe HTTPS OAuth binding cookie = %#v", bindingCookie)
	}
	if strings.Contains(browserAFirstRecorder.Body.String(), bindingCookie.Value) {
		t.Fatal("OAuth browser binding leaked into the response body")
	}

	browserASecondRecorder := httptest.NewRecorder()
	browserASecond, _ := gin.CreateTestContext(browserASecondRecorder)
	browserASecond.Request = httptest.NewRequest(http.MethodPost, "https://api.example.com/api/integrations/oauth/flows", nil)
	browserASecond.Request.AddCookie(bindingCookie)
	secondDigest, err := ensureOAuthBrowserBindingCookie(browserASecond, callbackURL)
	if err != nil {
		t.Fatalf("reuse Browser A binding: %v", err)
	}
	if secondDigest != firstDigest {
		t.Fatalf("concurrent Browser A flows received different bindings")
	}

	browserBRecorder := httptest.NewRecorder()
	browserB, _ := gin.CreateTestContext(browserBRecorder)
	browserB.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/integrations/oauth/callback", nil)
	if _, err := readOAuthBrowserBindingCookie(browserB, callbackURL); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("Browser B callback binding error = %v", err)
	}
	browserBCreatedDigest, err := ensureOAuthBrowserBindingCookie(browserB, callbackURL)
	if err != nil {
		t.Fatalf("create Browser B binding: %v", err)
	}
	if browserBCreatedDigest == firstDigest {
		t.Fatal("different browsers received the same OAuth binding")
	}

	browserACallbackRecorder := httptest.NewRecorder()
	browserACallback, _ := gin.CreateTestContext(browserACallbackRecorder)
	browserACallback.Request = httptest.NewRequest(http.MethodGet, "https://api.example.com/api/integrations/oauth/callback", nil)
	browserACallback.Request.AddCookie(bindingCookie)
	callbackDigest, err := readOAuthBrowserBindingCookie(browserACallback, callbackURL)
	if err != nil || callbackDigest != firstDigest {
		t.Fatalf("Browser A callback digest = %q, %v", callbackDigest, err)
	}
}

func TestOAuthBrowserBindingUsesClearlyLocalCookieForLoopbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "http://localhost:2670/api/integrations/oauth/flows", nil)
	if _, err := ensureOAuthBrowserBindingCookie(context, "http://localhost:2670/api/integrations/oauth/callback"); err != nil {
		t.Fatalf("create loopback OAuth binding: %v", err)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("loopback Set-Cookie count = %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != oauthBrowserBindingLocalCookieName || cookie.Secure || !cookie.HttpOnly ||
		cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge <= 0 ||
		cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe loopback OAuth binding cookie = %#v", cookie)
	}
	if strings.HasPrefix(cookie.Name, "__Host-") {
		t.Fatalf("loopback HTTP cookie must not use __Host- prefix: %s", cookie.Name)
	}
}

func TestOAuthBrowserBindingRejectsNonLoopbackHTTPCallback(t *testing.T) {
	if _, err := resolveOAuthBrowserBindingCookieSpec("http://api.example.com/integrations/oauth/callback"); integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
		t.Fatalf("non-loopback HTTP callback error = %v", err)
	}
}
