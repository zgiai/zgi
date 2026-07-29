package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type emailCodeLoginService interface {
	SendCode(ctx context.Context, req auth_service.EmailCodeLoginSendRequest, ipAddress string) (*auth_service.EmailCodeLoginSendResponse, error)
	VerifyAndLogin(ctx context.Context, req auth_service.EmailCodeLoginVerifyRequest, ipAddress string) (*dto.LoginResponse, error)
}

type EmailCodeLoginHandler struct {
	service emailCodeLoginService
}

func NewEmailCodeLoginHandler(service emailCodeLoginService) *EmailCodeLoginHandler {
	return &EmailCodeLoginHandler{service: service}
}

func (h *EmailCodeLoginHandler) SendCode(c *gin.Context) {
	var req auth_service.EmailCodeLoginSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	result, err := h.service.SendCode(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *EmailCodeLoginHandler) VerifyAndLogin(c *gin.Context) {
	var req auth_service.EmailCodeLoginVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	result, err := h.service.VerifyAndLogin(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success", "data": result})
}

func (h *EmailCodeLoginHandler) RegisterRoutes(v1 *gin.RouterGroup) {
	v1.POST("/login/by-email", h.SendCode)
	v1.POST("/login/by-email/code", h.VerifyAndLogin)
}

func (h *EmailCodeLoginHandler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth_service.ErrEmailCodeLoginDisabled):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, auth_service.ErrEmailCodeLoginAccountMissing):
		response.Fail(c, response.ErrAccountNotFound)
	case errors.Is(err, auth_service.ErrEmailCodeLoginTokenInvalid):
		response.Fail(c, response.ErrTokenInvalid)
	case errors.Is(err, auth_service.ErrEmailCodeLoginCodeInvalid):
		response.Fail(c, response.ErrInvalidCode)
	case errors.Is(err, auth_service.ErrEmailCodeLoginRateLimited):
		response.Fail(c, response.ErrRateLimitExceeded)
	case errors.Is(err, auth_service.ErrEmailCodeLoginSendFailed):
		response.Fail(c, response.ErrEmailSendFailed)
	case errors.Is(err, auth_service.ErrEmailCodeLoginAccountBlocked):
		response.Fail(c, response.ErrAccountFrozen)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}
