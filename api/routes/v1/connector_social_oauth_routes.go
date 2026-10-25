package v1

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialoauth"
)

type connectorSocialOAuthService interface {
	Available(string) bool
	Start(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID) (socialoauth.Flow, error)
	Complete(context.Context, string, string, string, bool) (uuid.UUID, error)
	Get(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (socialoauth.Flow, error)
	CompleteInstallation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (socialoauth.Flow, error)
	SelectPage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (socialoauth.Flow, error)
}

func (h *connectorHandler) registerSocialOAuth(group *gin.RouterGroup) {
	group.GET("/connectors/:connectorId/oauth", h.socialOAuthAvailability)
	group.POST("/connectors/:connectorId/oauth", h.startSocialOAuth)
	group.GET("/connectors/oauth-flows/:flowId", h.getSocialOAuth)
	group.POST("/connectors/oauth-flows/:flowId/page", h.selectSocialOAuthPage)
	group.POST("/connectors/oauth-flows/:flowId/installation", h.completeOAuthInstallation)
}

// The provider cannot carry ZGI's JWT. Only a one-use, actor-bound state may
// authorize this callback; all status reads and Page selections require a JWT.
func (h *connectorHandler) registerSocialOAuthCallback(group *gin.RouterGroup) {
	group.GET("/connectors/:connectorId/oauth/callback", h.completeSocialOAuth)
}

func socialOAuthHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
}
func (h *connectorHandler) socialOAuthAvailability(c *gin.Context) {
	socialOAuthHeaders(c)
	if _, _, ok := connectorActor(c); !ok {
		return
	}
	if !supportsConnectorOAuth(c.Param("connectorId")) {
		connectorRouteError(c, core.NewError(core.ErrorCodeNotFound, false, nil))
		return
	}
	connectorSuccess(c, http.StatusOK, gin.H{"available": h.socialOAuth != nil && h.socialOAuth.Available(c.Param("connectorId"))})
}
func (h *connectorHandler) startSocialOAuth(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialOAuth == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct {
		ConnectionID string `json:"connection_id"`
	}
	if decodeConnectorJSON(c, &input) != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	var id uuid.UUID
	if input.ConnectionID != "" {
		var err error
		id, err = parseConnectorUUID(input.ConnectionID)
		if err != nil {
			connectorRouteError(c, err)
			return
		}
	}
	flow, err := h.socialOAuth.Start(c.Request.Context(), org, owner, c.Param("connectorId"), id)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	authorize, parseErr := url.Parse(flow.AuthorizeURL)
	if parseErr != nil || authorize.Query().Get("state") == "" {
		connectorRouteError(c, core.NewError(core.ErrorCodeInternal, false, nil))
		return
	}
	state := authorize.Query().Get("state")
	cookie := socialOAuthBrowserCookie(c.Param("connectorId"), state)
	cookie.Secure = strings.HasPrefix(h.socialOAuthFrontendURL, "https://")
	http.SetCookie(c.Writer, cookie)
	connectorSuccess(c, http.StatusOK, flow)
}
func (h *connectorHandler) getSocialOAuth(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialOAuth == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	flow, err := h.socialOAuth.Get(c.Request.Context(), org, owner, id)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, flow)
}
func (h *connectorHandler) selectSocialOAuthPage(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialOAuth == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct {
		PageID string `json:"page_id"`
	}
	if decodeConnectorJSON(c, &input) != nil || strings.TrimSpace(input.PageID) == "" || len(input.PageID) > 256 {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	flow, err := h.socialOAuth.SelectPage(c.Request.Context(), org, owner, id, input.PageID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, flow)
}
func (h *connectorHandler) completeSocialOAuth(c *gin.Context) {
	socialOAuthHeaders(c)
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	state, code := query.Get("state"), query.Get("code")
	denied := query.Get("error") != ""
	if err != nil || len(query["state"]) != 1 || len(query["code"]) > 1 || len(query["error"]) > 1 || state == "" || len(state) > 512 || len(code) > 8192 || (!denied && code == "") || h.socialOAuth == nil || !supportsConnectorOAuth(c.Param("connectorId")) {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	expectedCookie := socialOAuthBrowserCookie(c.Param("connectorId"), state)
	browserCookie, cookieErr := c.Request.Cookie(expectedCookie.Name)
	if cookieErr != nil || subtle.ConstantTimeCompare([]byte(browserCookie.Value), []byte(expectedCookie.Value)) != 1 {
		connectorRouteError(c, core.NewError(core.ErrorCodeFlowExpired, false, nil))
		return
	}
	expectedCookie.MaxAge = -1
	expectedCookie.Secure = strings.HasPrefix(h.socialOAuthFrontendURL, "https://")
	http.SetCookie(c.Writer, expectedCookie)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	id, completeErr := h.socialOAuth.Complete(ctx, c.Param("connectorId"), state, code, denied)
	if id == uuid.Nil {
		if completeErr == nil {
			completeErr = core.NewError(core.ErrorCodeFlowExpired, false, nil)
		}
		connectorRouteError(c, completeErr)
		return
	}
	// Only a configured origin is used. Request-provided redirect parameters and
	// upstream error text are never copied into the redirect or response.
	destination, err := url.Parse(h.socialOAuthFrontendURL)
	if err != nil || destination.Host == "" || destination.User != nil || (destination.Scheme != "https" && destination.Scheme != "http") {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	destination.Path = "/console/connectors/oauth"
	destination.RawQuery = url.Values{"flow_id": []string{id.String()}}.Encode()
	destination.Fragment = ""
	c.Redirect(http.StatusSeeOther, destination.String())
}

// The one-time state must return in the browser that initiated authorization.
// Separate names allow simultaneous authorizations without overwriting a flow.
func socialOAuthBrowserCookie(provider, state string) *http.Cookie {
	digest := sha256.Sum256([]byte(state))
	value := hex.EncodeToString(digest[:])
	return &http.Cookie{Name: "zgi_social_oauth_" + value[:16], Value: value, Path: "/console/api/connectors/" + provider + "/oauth/callback", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func supportsConnectorOAuth(id string) bool {
	return id == "github" || catalog.IsSocialMCPConnector(id)
}
func (h *connectorHandler) completeOAuthInstallation(c *gin.Context) {
	socialOAuthHeaders(c)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	if h.socialOAuth == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct{}
	if decodeConnectorJSON(c, &input) != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	flow, err := h.socialOAuth.CompleteInstallation(c.Request.Context(), org, owner, id)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, flow)
}
