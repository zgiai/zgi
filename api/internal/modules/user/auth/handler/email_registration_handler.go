package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type emailRegistrationService interface {
	SendCode(
		ctx context.Context,
		req auth_service.EmailRegistrationSendRequest,
		ipAddress string,
	) (*auth_service.EmailRegistrationSendResponse, error)
	VerifyCode(
		ctx context.Context,
		req auth_service.EmailRegistrationVerifyRequest,
	) (*auth_service.EmailRegistrationVerifyResponse, error)
	Finish(
		ctx context.Context,
		req auth_service.EmailRegistrationFinishRequest,
		ipAddress string,
	) (*shared_dto.LoginResponse, error)
}

type EmailRegistrationHandler struct {
	service emailRegistrationService
}

func NewEmailRegistrationHandler(service emailRegistrationService) *EmailRegistrationHandler {
	return &EmailRegistrationHandler{service: service}
}

func (h *EmailRegistrationHandler) SendCode(c *gin.Context) {
	var req auth_service.EmailRegistrationSendRequest
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

func (h *EmailRegistrationHandler) VerifyCode(c *gin.Context) {
	var req auth_service.EmailRegistrationVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.VerifyCode(c.Request.Context(), req)
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *EmailRegistrationHandler) Finish(c *gin.Context) {
	var req auth_service.EmailRegistrationFinishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.Finish(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondError(c, err)
		return
	}
	response.Success(c, gin.H{
		"result": "success",
		"data":   result,
	})
}

func (h *EmailRegistrationHandler) RegisterRoutes(v1 *gin.RouterGroup) {
	v1.POST("/register", h.SendCode)
	v1.POST("/register/validity", h.VerifyCode)
	v1.POST("/register/finish", h.Finish)
}

func (h *EmailRegistrationHandler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth_service.ErrEmailRegistrationDisabled):
		response.Fail(c, response.ErrRegisterNotAllowed)
	case errors.Is(err, auth_service.ErrEmailRegistrationAccountExists):
		response.Fail(c, response.ErrUserExists)
	case errors.Is(err, auth_service.ErrEmailRegistrationTokenInvalid):
		response.Fail(c, response.ErrTokenInvalid)
	case errors.Is(err, auth_service.ErrEmailRegistrationCodeInvalid):
		response.Fail(c, response.ErrInvalidCode)
	case errors.Is(err, auth_service.ErrEmailRegistrationRateLimited):
		response.Fail(c, response.ErrRateLimitExceeded)
	case errors.Is(err, auth_service.ErrEmailRegistrationSendFailed):
		response.Fail(c, response.ErrEmailSendFailed)
	case errors.Is(err, auth_service.ErrEmailRegistrationPasswordMismatch):
		response.Fail(c, response.ErrPasswordMismatch)
	case errors.Is(err, auth_service.ErrEmailRegistrationPasswordTooShort):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, auth_service.ErrEmailRegistrationAccountFrozen):
		response.Fail(c, response.ErrAccountFrozen)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}
