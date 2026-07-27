package v1

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	oauthBrowserBindingSecureCookieName = "__Host-zgi_oauth_browser_binding"
	oauthBrowserBindingLocalCookieName  = "zgi_oauth_browser_binding_local_only"
	oauthBrowserBindingCookieLifetime   = 30 * time.Minute
)

type startIntegrationOAuthFlowRequest struct {
	IntegrationID      string                                  `json:"integration_id" binding:"required"`
	AuthMethodID       string                                  `json:"auth_method_id" binding:"required"`
	CredentialSource   integrations.ConnectionCredentialSource `json:"credential_source"`
	Intent             integrations.OAuthFlowIntent            `json:"intent"`
	ConnectionName     string                                  `json:"connection_name"`
	ConnectionID       *uuid.UUID                              `json:"connection_id"`
	RequestedActionIDs []string                                `json:"requested_action_ids" binding:"max=128"`
	ReturnPath         string                                  `json:"return_path"`
}

type putIntegrationOAuthClientConfigRequest struct {
	Revision     int            `json:"revision"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Config       map[string]any `json:"config"`
}

type acknowledgeIntegrationOAuthRecoveryRequest struct {
	ResolutionCode string `json:"resolution_code" binding:"required"`
}

type integrationOAuthClientConfigResponse struct {
	integrations.OAuthClientConfigView
	CallbackURL      string `json:"callback_url"`
	ProviderSetupURL string `json:"provider_setup_url,omitempty"`
}

func (handler *integrationHandler) startOAuthFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request startIntegrationOAuthFlowRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	definition, method, ok := handler.oauthMethod(request.IntegrationID, request.AuthMethodID)
	if !ok {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration OAuth auth method is unsupported", nil))
		return
	}
	if method.CredentialSource == integrations.ConnectionCredentialSourceOrganization &&
		!middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	if request.CredentialSource == "" {
		request.CredentialSource = method.CredentialSource
	}
	browserBindingDigest, err := ensureOAuthBrowserBindingCookie(c, handler.deps.OAuthCallbackURL)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	result, err := handler.deps.OAuthFlows.Start(c.Request.Context(), integrations.OAuthFlowStartRequest{
		OrganizationID: organizationID, AccountID: accountID,
		BrowserBindingDigest: browserBindingDigest,
		IntegrationID:        definition.ID, AuthMethodID: method.ID,
		CredentialSource: request.CredentialSource, Intent: request.Intent,
		ConnectionName: request.ConnectionName, ConnectionID: request.ConnectionID,
		RequestedActionIDs: request.RequestedActionIDs,
		RedirectURI:        handler.deps.OAuthCallbackURL, ReturnPath: request.ReturnPath,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}

func (handler *integrationHandler) pollOAuthFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	view, err := handler.deps.OAuthFlows.Poll(c.Request.Context(), c.Param("flow_id"), organizationID, accountID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, view)
}

func (handler *integrationHandler) cancelOAuthFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	if err := handler.deps.OAuthFlows.Cancel(c.Request.Context(), c.Param("flow_id"), organizationID, accountID); err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, map[string]any{"status": integrations.OAuthFlowCancelled})
}

func (handler *integrationHandler) oauthCallback(c *gin.Context) {
	// Never reflect provider error descriptions, codes, authorization codes, or
	// state into the browser URL. The result page receives only an opaque flow
	// reference and a bounded status/error category.
	browserBindingDigest, _ := readOAuthBrowserBindingCookie(c, handler.deps.OAuthCallbackURL)
	callback := integrations.OAuthCallbackRequest{
		State: c.Query("state"), BrowserBindingDigest: browserBindingDigest, Code: c.Query("code"),
		ProviderError:            c.Query("error"),
		ProviderErrorDescription: c.Query("error_description"),
	}
	result, err := handler.deps.OAuthFlows.Callback(c.Request.Context(), callback)
	status := result.Status
	if status == "" {
		status = integrations.OAuthFlowFailed
	}
	errorCode := result.ErrorCode
	if err != nil && errorCode == "" {
		errorCode = safeOAuthCallbackErrorCode(integrations.ErrorCode(err))
	}
	target, buildErr := oauthResultRedirectURL(handler.deps.OAuthResultURL, result.FlowID, status, errorCode)
	if buildErr != nil {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		c.Status(http.StatusBadRequest)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusFound, target)
}

type oauthBrowserBindingCookieSpec struct {
	name   string
	secure bool
}

func ensureOAuthBrowserBindingCookie(c *gin.Context, callbackURL string) (string, error) {
	if c == nil {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "integration OAuth browser binding is unavailable", nil)
	}
	spec, err := resolveOAuthBrowserBindingCookieSpec(callbackURL)
	if err != nil {
		return "", err
	}
	rawBinding, cookieErr := c.Cookie(spec.name)
	digest, digestErr := integrations.OAuthBrowserBindingDigest(rawBinding)
	if cookieErr != nil || digestErr != nil {
		randomValue := make([]byte, 32)
		if _, err := rand.Read(randomValue); err != nil {
			return "", integrations.NewError(integrations.ErrorCodeUpstream, "integration OAuth browser binding could not be generated", err)
		}
		rawBinding = base64.RawURLEncoding.EncodeToString(randomValue)
		digest, err = integrations.OAuthBrowserBindingDigest(rawBinding)
		if err != nil {
			return "", err
		}
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: spec.name, Value: rawBinding, Path: "/", HttpOnly: true, Secure: spec.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(oauthBrowserBindingCookieLifetime.Seconds()),
		Expires: time.Now().UTC().Add(oauthBrowserBindingCookieLifetime),
	})
	return digest, nil
}

func readOAuthBrowserBindingCookie(c *gin.Context, callbackURL string) (string, error) {
	if c == nil {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "integration OAuth browser binding is unavailable", nil)
	}
	spec, err := resolveOAuthBrowserBindingCookieSpec(callbackURL)
	if err != nil {
		return "", err
	}
	rawBinding, err := c.Cookie(spec.name)
	if err != nil {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "integration OAuth browser binding is unavailable", nil)
	}
	return integrations.OAuthBrowserBindingDigest(rawBinding)
}

func resolveOAuthBrowserBindingCookieSpec(callbackURL string) (oauthBrowserBindingCookieSpec, error) {
	parsed, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return oauthBrowserBindingCookieSpec{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration OAuth callback URL is invalid", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return oauthBrowserBindingCookieSpec{name: oauthBrowserBindingSecureCookieName, secure: true}, nil
	case "http":
		hostname := strings.ToLower(parsed.Hostname())
		address := net.ParseIP(hostname)
		if hostname == "localhost" || (address != nil && address.IsLoopback()) {
			return oauthBrowserBindingCookieSpec{name: oauthBrowserBindingLocalCookieName}, nil
		}
	}
	return oauthBrowserBindingCookieSpec{}, integrations.NewError(
		integrations.ErrorCodeInvalidInput,
		"integration OAuth callback URL scheme is unsupported for browser binding",
		nil,
	)
}

func (handler *integrationHandler) getOAuthClientConfig(c *gin.Context) {
	organizationID, _, ok := integrationActor(c)
	if !ok {
		return
	}
	definition, method, ok := handler.oauthMethod(c.Param("integration_id"), c.Param("auth_method_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	view, err := handler.deps.OAuthClients.GetView(c.Request.Context(), integrations.OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: definition.ID,
		DriverID: definition.DriverID, AuthMethodID: method.ID,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, integrationOAuthClientConfigResponse{
		OAuthClientConfigView: view, CallbackURL: handler.deps.OAuthCallbackURL,
		ProviderSetupURL: oauthProviderSetupURL(method, definition.DocumentationURL),
	})
}

func (handler *integrationHandler) getOAuthClientConfigImpact(c *gin.Context) {
	organizationID, _, ok := integrationActor(c)
	if !ok {
		return
	}
	definition, method, ok := handler.oauthMethod(c.Param("integration_id"), c.Param("auth_method_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	impact, err := handler.deps.OAuthClients.Impact(c.Request.Context(), integrations.OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: definition.ID,
		DriverID: definition.DriverID, AuthMethodID: method.ID,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, impact)
}

func (handler *integrationHandler) putOAuthClientConfig(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	definition, method, ok := handler.oauthMethod(c.Param("integration_id"), c.Param("auth_method_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	var request putIntegrationOAuthClientConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	view, err := handler.deps.OAuthClients.Put(c.Request.Context(), integrations.PutOAuthClientConfigRequest{
		OrganizationID: organizationID, IntegrationID: definition.ID, DriverID: definition.DriverID,
		AuthMethodID: method.ID, ClientID: request.ClientID, ClientSecret: request.ClientSecret,
		Config: request.Config, ActorID: &accountID, Revision: request.Revision,
	})
	request.ClientID = ""
	request.ClientSecret = ""
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, integrationOAuthClientConfigResponse{
		OAuthClientConfigView: view, CallbackURL: handler.deps.OAuthCallbackURL,
		ProviderSetupURL: oauthProviderSetupURL(method, definition.DocumentationURL),
	})
}

func (handler *integrationHandler) deleteOAuthClientConfig(c *gin.Context) {
	organizationID, _, ok := integrationActor(c)
	if !ok {
		return
	}
	definition, method, ok := handler.oauthMethod(c.Param("integration_id"), c.Param("auth_method_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	if err := handler.deps.OAuthClients.Delete(c.Request.Context(), integrations.OAuthClientResolveRequest{
		OrganizationID: organizationID, IntegrationID: definition.ID,
		DriverID: definition.DriverID, AuthMethodID: method.ID,
	}); err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, map[string]any{"deleted": true})
}

func (handler *integrationHandler) oauthRecoverySummary(c *gin.Context) {
	organizationID, _, ok := integrationActor(c)
	if !ok {
		return
	}
	summary, err := handler.deps.OAuthRecovery.OAuthRecoverySummary(
		c.Request.Context(),
		organizationID,
		50,
	)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, summary)
}

func (handler *integrationHandler) acknowledgeOAuthRecovery(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request acknowledgeIntegrationOAuthRecoveryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if err := handler.deps.OAuthRecovery.AcknowledgeOAuthRecovery(
		c.Request.Context(),
		organizationID,
		c.Param("operation_ref"),
		accountID,
		request.ResolutionCode,
	); err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, map[string]any{"acknowledged": true})
}

func (handler *integrationHandler) oauthMethod(integrationID, authMethodID string) (integrations.ProviderDefinition, integrations.AuthMethodDefinition, bool) {
	definition, ok := handler.deps.Registry.ProviderDefinition(strings.ToLower(strings.TrimSpace(integrationID)))
	if !ok {
		return integrations.ProviderDefinition{}, integrations.AuthMethodDefinition{}, false
	}
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	for _, method := range definition.AuthMethods {
		if method.ID == authMethodID && method.Type == integrations.AuthMethodTypeOAuth2 && method.Available && method.OAuth != nil {
			return definition, method, true
		}
	}
	return integrations.ProviderDefinition{}, integrations.AuthMethodDefinition{}, false
}

func oauthProviderSetupURL(method integrations.AuthMethodDefinition, fallback string) string {
	if method.OAuth != nil && strings.TrimSpace(method.OAuth.ProviderSetupURL) != "" {
		return strings.TrimSpace(method.OAuth.ProviderSetupURL)
	}
	return strings.TrimSpace(fallback)
}

func oauthResultRedirectURL(rawBaseURL, flowID string, status integrations.OAuthFlowStatus, errorCode string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "integration OAuth result URL is invalid", err)
	}
	query := target.Query()
	if strings.TrimSpace(flowID) != "" {
		query.Set("flow", strings.TrimSpace(flowID))
	}
	query.Set("status", string(status))
	if errorCode = safeOAuthCallbackErrorCode(errorCode); errorCode != "" {
		query.Set("error_code", errorCode)
	}
	target.RawQuery = query.Encode()
	target.Fragment = ""
	return target.String(), nil
}

func safeOAuthCallbackErrorCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case integrations.ErrorCodeAccessDenied, integrations.ErrorCodeAuthInvalid,
		integrations.ErrorCodeConnectionConflict, integrations.ErrorCodeConnectionInvalid,
		integrations.ErrorCodeDisabled, integrations.ErrorCodeInsufficientScope,
		integrations.ErrorCodeReconnectRequired, integrations.ErrorCodeResponseInvalid,
		integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream:
		return strings.ToLower(strings.TrimSpace(code))
	default:
		if strings.TrimSpace(code) != "" {
			return integrations.ErrorCodeAuthInvalid
		}
		return ""
	}
}
