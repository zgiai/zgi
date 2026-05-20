package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	ssoFeatureSocialOAuthLogin   = "social_oauth_login"
	ssoQueryCode                 = "code"
	ssoQueryState                = "state"
	ssoQueryReason               = "reason"
	ssoErrorDisabled             = "disabled"
	ssoErrorNotConfigured        = "not_configured"
	ssoErrorMissingCodeOrState   = "missing_code_or_state"
	ssoErrorInvalidState         = "invalid_state"
	ssoErrorExchangeFailed       = "exchange_failed"
	ssoErrorAccountResolveFailed = "account_resolution_failed"
	ssoErrorTicketIssueFailed    = "ticket_issue_failed"
	ssoReasonInvalidPhoneClaim   = "invalid_phone_claim"
	ssoReasonIdentityContact     = "identity_contact_required"
	ssoReasonSubjectClaim        = "subject_claim_required"
	ssoDisabledMessage           = "social oauth login is disabled"
	ssoNotConfiguredMessage      = "casdoor sso is not configured"
	ssoFrontendCallbackEnvKey    = "SSO_FRONTEND_CALLBACK_URL"
	ssoConsoleWebURLEnvKey       = "EMAIL_CONSOLE_WEB_URL"
	ssoFrontendCallbackPath      = "/sso/callback"
)

type ssoService interface {
	IssueSSOState(ctx context.Context) (string, error)
	ConsumeSSOState(ctx context.Context, state string) error
	ResolveOrCreateSSOAccount(ctx context.Context, identity *shared_dto.SSOIdentity) (*auth_model.Account, error)
	IssueSSOLoginTicket(ctx context.Context, account *auth_model.Account, sso *shared_dto.SSOProviderToken) (string, error)
	ConsumeSSOLoginTicket(ctx context.Context, ticket, ipAddress string) (*shared_dto.LoginResponse, error)
}

type casdoorOIDCClient interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (*shared_dto.SSOExchangeResult, error)
}

func (h *AuthHandler) StartCasdoorSSO(c *gin.Context) {
	if !h.isSSOEnabled() {
		response.FailWithMessage(c, response.ErrSystemError, ssoDisabledMessage)
		return
	}
	if h.ssoService == nil || h.casdoorClient == nil {
		response.FailWithMessage(c, response.ErrSystemError, ssoNotConfiguredMessage)
		return
	}

	state, err := h.ssoService.IssueSSOState(c.Request.Context())
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	redirectURL, err := h.casdoorClient.AuthorizationURL(c.Request.Context(), state)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) HandleCasdoorCallback(c *gin.Context) {
	if !h.isSSOEnabled() {
		h.redirectSSOError(c, ssoErrorDisabled, "")
		return
	}
	if h.ssoService == nil || h.casdoorClient == nil {
		h.redirectSSOError(c, ssoErrorNotConfigured, "")
		return
	}

	code := strings.TrimSpace(c.Query(ssoQueryCode))
	state := strings.TrimSpace(c.Query(ssoQueryState))
	if code == "" || state == "" {
		h.redirectSSOError(c, ssoErrorMissingCodeOrState, "")
		return
	}

	if err := h.ssoService.ConsumeSSOState(c.Request.Context(), state); err != nil {
		h.redirectSSOError(c, ssoErrorInvalidState, "")
		return
	}

	exchangeResult, err := h.casdoorClient.ExchangeCode(c.Request.Context(), code)
	if err != nil {
		h.redirectSSOError(c, ssoErrorExchangeFailed, classifySSOExchangeReason(err))
		return
	}
	if exchangeResult == nil || exchangeResult.Identity == nil {
		h.redirectSSOError(c, ssoErrorExchangeFailed, "")
		return
	}

	account, err := h.ssoService.ResolveOrCreateSSOAccount(c.Request.Context(), exchangeResult.Identity)
	if err != nil {
		h.redirectSSOError(c, ssoErrorAccountResolveFailed, "")
		return
	}

	ticket, err := h.ssoService.IssueSSOLoginTicket(c.Request.Context(), account, exchangeResult.Token)
	if err != nil {
		h.redirectSSOError(c, ssoErrorTicketIssueFailed, "")
		return
	}

	redirectURL, err := auth_service.BuildFrontendSSORedirect(h.ssoFrontendCallbackURL(), ticket, "", "")
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) ConsumeSSOLoginTicket(c *gin.Context) {
	if h.ssoService == nil {
		response.FailWithMessage(c, response.ErrSystemError, ssoNotConfiguredMessage)
		return
	}

	var req struct {
		Ticket string `json:"ticket" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	loginResponse, err := h.ssoService.ConsumeSSOLoginTicket(c.Request.Context(), req.Ticket, c.ClientIP())
	if err != nil {
		response.Fail(c, response.ErrTokenInvalid)
		return
	}

	data := gin.H{
		"access_token":  loginResponse.AccessToken,
		"refresh_token": loginResponse.RefreshToken,
		"account":       loginResponse.Account,
	}
	if loginResponse.SSO != nil {
		data["sso"] = loginResponse.SSO
	}

	response.Success(c, gin.H{
		"result": "success",
		"data":   data,
	})
}

func (h *AuthHandler) isSSOEnabled() bool {
	if h.featureService == nil {
		return true
	}
	return h.featureService.IsFeatureEnabled(ssoFeatureSocialOAuthLogin)
}

func (h *AuthHandler) redirectSSOError(c *gin.Context, errCode, reason string) {
	callbackURL := h.ssoFrontendCallbackURL()
	if callbackURL == "" {
		message := errCode
		if reason != "" {
			message += " " + ssoQueryReason + "=" + reason
		}
		response.FailWithMessage(c, response.ErrSystemError, message)
		return
	}

	redirectURL, err := auth_service.BuildFrontendSSORedirect(callbackURL, "", errCode, reason)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) ssoFrontendCallbackURL() string {
	cfg := config.Current()
	if callbackURL := strings.TrimSpace(cfg.Auth.SSO.FrontendCallbackURL); callbackURL != "" {
		return callbackURL
	}

	if consoleURL := strings.TrimRight(strings.TrimSpace(cfg.Console.WebURL), "/"); consoleURL != "" {
		return consoleURL + ssoFrontendCallbackPath
	}

	return ""
}

func classifySSOExchangeReason(err error) string {
	switch {
	case errors.Is(err, auth_service.ErrOIDCInvalidPhoneClaim):
		return ssoReasonInvalidPhoneClaim
	case errors.Is(err, auth_service.ErrOIDCIdentityContactAbsent):
		return ssoReasonIdentityContact
	case errors.Is(err, auth_service.ErrOIDCSubjectClaimRequired):
		return ssoReasonSubjectClaim
	default:
		return ""
	}
}
