package v1

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	wecomintegration "github.com/zgiai/zgi/api/internal/modules/integrations/providers/collaboration/wecom"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

var wecomCallbackReceiverIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type startProviderInstallFlowRequest struct {
	IntegrationID    string                                  `json:"integration_id" binding:"required"`
	AuthMethodID     string                                  `json:"auth_method_id" binding:"required"`
	CredentialSource integrations.ConnectionCredentialSource `json:"credential_source"`
	ConnectionName   string                                  `json:"connection_name" binding:"required"`
	ReturnPath       string                                  `json:"return_path"`
}

func (handler *integrationHandler) startProviderInstallFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request startProviderInstallFlowRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	definition, method, ok := handler.providerInstallMethod(request.IntegrationID, request.AuthMethodID)
	if !ok {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeDisabled, "provider install method is unavailable", nil))
		return
	}
	if method.CredentialSource == integrations.ConnectionCredentialSourceOrganization && !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	if request.CredentialSource == "" {
		request.CredentialSource = method.CredentialSource
	}
	browserBindingDigest, err := ensureOAuthBrowserBindingCookie(c, handler.deps.WeComInstallCallbackURL)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	result, err := handler.deps.ProviderInstallFlows.Start(c.Request.Context(), integrations.ProviderInstallStartRequest{
		OrganizationID: organizationID, AccountID: accountID, BrowserBindingDigest: browserBindingDigest,
		IntegrationID: definition.ID, AuthMethodID: method.ID, CredentialSource: request.CredentialSource,
		ConnectionName: request.ConnectionName, RedirectURI: handler.deps.WeComInstallCallbackURL,
		ReturnPath: request.ReturnPath,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, result)
}

func (handler *integrationHandler) pollProviderInstallFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	view, err := handler.deps.ProviderInstallFlows.Poll(c.Request.Context(), c.Param("flow_id"), organizationID, accountID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, view)
}

func (handler *integrationHandler) cancelProviderInstallFlow(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	if err := handler.deps.ProviderInstallFlows.Cancel(c.Request.Context(), c.Param("flow_id"), organizationID, accountID); err != nil {
		integrationRouteError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, map[string]any{"status": integrations.ProviderInstallFlowCancelled})
}

func (handler *integrationHandler) wecomInstallCallback(c *gin.Context) {
	browserBindingDigest, _ := readOAuthBrowserBindingCookie(c, handler.deps.WeComInstallCallbackURL)
	result, err := handler.deps.ProviderInstallFlows.Callback(c.Request.Context(), c.Query("state"), browserBindingDigest, c.Query("auth_code"))
	if err != nil {
		logger.WarnContext(
			c.Request.Context(),
			"WeCom browser authorization could not be queued",
			wecomInstallCallbackDiagnosticFields(err),
		)
	}
	status := result.Status
	if status == "" {
		status = integrations.ProviderInstallFlowFailed
	}
	errorCode := result.ErrorCode
	if err != nil && errorCode == "" {
		errorCode = safeOAuthCallbackErrorCode(integrations.ErrorCode(err))
	}
	target, buildErr := providerInstallResultRedirectURL(handler.deps.WeComResultURL, status, errorCode)
	if buildErr != nil {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusBadRequest)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusFound, target)
}

func wecomInstallCallbackDiagnosticFields(err error) map[string]interface{} {
	return map[string]interface{}{
		"integration_id": wecomintegration.IntegrationID,
		"callback_kind":  "browser_return",
		"error_code":     safeOAuthCallbackErrorCode(integrations.ErrorCode(err)),
	}
}

func (handler *integrationHandler) wecomCallbackVerification(c *gin.Context) {
	plaintext, err := handler.deps.WeComCallbacks.VerifySuiteCallbackURL(
		c.Query("msg_signature"), c.Query("timestamp"), c.Query("nonce"), c.Query("echostr"),
	)
	if err != nil {
		logWeComCallbackVerificationFailure(c, "suite_command", err)
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusUnauthorized)
		return
	}
	writeWeComCallbackVerificationResponse(c, plaintext)
}

func (handler *integrationHandler) wecomDataCallbackVerification(c *gin.Context) {
	receiverID, ok := wecomCallbackReceiverID(c)
	if !ok {
		return
	}
	plaintext, err := handler.deps.WeComCallbacks.VerifyDataCallbackURL(
		receiverID, c.Query("msg_signature"), c.Query("timestamp"), c.Query("nonce"), c.Query("echostr"),
	)
	if err != nil {
		logWeComCallbackVerificationFailure(c, "data", err)
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusUnauthorized)
		return
	}
	writeWeComCallbackVerificationResponse(c, plaintext)
}

func writeWeComCallbackVerificationResponse(c *gin.Context, plaintext string) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, "%s", plaintext)
}

func logWeComCallbackVerificationFailure(c *gin.Context, callbackKind string, err error) {
	logger.WarnContext(
		c.Request.Context(),
		"WeCom callback URL verification failed",
		wecomCallbackVerificationDiagnosticFields(callbackKind, err),
	)
}

func wecomCallbackVerificationDiagnosticFields(callbackKind string, err error) map[string]interface{} {
	return map[string]interface{}{
		"callback_kind": callbackKind,
		"failure_stage": wecomintegration.CallbackFailureStage(err),
		"error_code":    integrations.ErrorCode(err),
	}
}

func (handler *integrationHandler) wecomSuiteEncryptedCallback(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (2<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 2<<20 {
		c.Status(http.StatusBadRequest)
		return
	}
	event, err := handler.deps.WeComCallbacks.ReceiveSuiteCallback(
		c.Request.Context(), c.Query("msg_signature"), c.Query("timestamp"), c.Query("nonce"), body,
	)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	switch event.InfoType {
	case "create_auth":
		if err := handler.deps.ProviderInstallFlows.EnqueueAuthorizationNotification(c.Request.Context(), wecomintegration.IntegrationID, event.State, event.AuthCode); err != nil {
			code := integrations.ErrorCode(err)
			logger.WarnContext(c.Request.Context(), "WeCom authorization notification could not be queued", map[string]interface{}{
				"info_type":  event.InfoType,
				"error_code": code,
			})
			// An unknown or expired state cannot safely be associated with a ZGI
			// organization. Acknowledge it to avoid a retry storm; infrastructure
			// failures remain retryable at the provider boundary.
			if code != integrations.ErrorCodeAuthInvalid {
				c.Status(http.StatusInternalServerError)
				return
			}
		}
	case "cancel_auth":
		if event.AuthCorpID != "" {
			if err := handler.deps.ProviderInstallFlows.DisableAuthorization(c.Request.Context(), wecomintegration.IntegrationID, handler.deps.WeComCallbacks.ProviderTenantHash(event.AuthCorpID), integrations.ErrorCodeReconnectRequired); err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
		}
	case "change_auth":
		if event.AuthCorpID != "" {
			if err := handler.deps.ProviderInstallFlows.MarkAuthorizationChanged(c.Request.Context(), wecomintegration.IntegrationID, handler.deps.WeComCallbacks.ProviderTenantHash(event.AuthCorpID)); err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
		}
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, "success")
}

func (handler *integrationHandler) wecomDataEncryptedCallback(c *gin.Context) {
	receiverID, ok := wecomCallbackReceiverID(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (2<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 2<<20 {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := handler.deps.WeComCallbacks.ReceiveDataCallback(
		receiverID, c.Query("msg_signature"), c.Query("timestamp"), c.Query("nonce"), body,
	); err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, "success")
}

func wecomCallbackReceiverID(c *gin.Context) (string, bool) {
	receiverID := strings.TrimSpace(c.Param("corp_id"))
	if !wecomCallbackReceiverIDPattern.MatchString(receiverID) {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusBadRequest)
		return "", false
	}
	return receiverID, true
}

func (handler *integrationHandler) providerInstallMethod(integrationID, authMethodID string) (integrations.ProviderDefinition, integrations.AuthMethodDefinition, bool) {
	definition, ok := handler.deps.Registry.ProviderDefinition(strings.ToLower(strings.TrimSpace(integrationID)))
	if !ok {
		return integrations.ProviderDefinition{}, integrations.AuthMethodDefinition{}, false
	}
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	for _, method := range definition.AuthMethods {
		if method.ID == authMethodID && method.Type == integrations.AuthMethodTypeProviderInstall && method.Available {
			return definition, method, true
		}
	}
	return integrations.ProviderDefinition{}, integrations.AuthMethodDefinition{}, false
}

func providerInstallResultRedirectURL(rawBaseURL string, status integrations.ProviderInstallFlowStatus, errorCode string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "provider install result URL is invalid", err)
	}
	query := target.Query()
	query.Set("install_status", string(status))
	if errorCode = safeOAuthCallbackErrorCode(errorCode); errorCode != "" {
		query.Set("error_code", errorCode)
	}
	target.RawQuery = query.Encode()
	target.Fragment = ""
	return target.String(), nil
}
