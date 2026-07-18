package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type phoneAuthService interface {
	CheckPhone(ctx context.Context, req auth_service.PhoneCheckRequest) (*auth_service.PhoneCheckResponse, error)
	SendCode(ctx context.Context, req auth_service.PhoneCodeSendRequest) (*auth_service.PhoneCodeSendResponse, error)
	VerifyCode(ctx context.Context, req auth_service.PhoneCodeVerifyRequest) (*auth_service.PhoneCodeVerifyResponse, error)
	RegisterByPhone(ctx context.Context, req auth_service.PhoneRegisterRequest, ipAddress string) (*dto.LoginResponse, error)
	LoginByPhone(ctx context.Context, req auth_service.PhoneLoginRequest, ipAddress string) (*dto.LoginResponse, error)
	LoginByPhonePassword(ctx context.Context, req auth_service.PhonePasswordLoginRequest, ipAddress string) (*dto.LoginResponse, error)
	ResetPasswordByPhone(ctx context.Context, req auth_service.PhoneResetPasswordRequest) error
}

type PhoneAuthHandler struct {
	service phoneAuthService
}

func NewPhoneAuthHandler(service phoneAuthService) *PhoneAuthHandler {
	return &PhoneAuthHandler{service: service}
}

func (h *PhoneAuthHandler) CheckPhone(c *gin.Context) {
	var req auth_service.PhoneCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.CheckPhone(c.Request.Context(), req)
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *PhoneAuthHandler) SendCode(c *gin.Context) {
	var req auth_service.PhoneCodeSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.SendCode(c.Request.Context(), req)
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *PhoneAuthHandler) VerifyCode(c *gin.Context) {
	var req auth_service.PhoneCodeVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.VerifyCode(c.Request.Context(), req)
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *PhoneAuthHandler) RegisterByPhone(c *gin.Context) {
	var req auth_service.PhoneRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.RegisterByPhone(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success", "data": result})
}

func (h *PhoneAuthHandler) LoginByPhone(c *gin.Context) {
	var req auth_service.PhoneLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.LoginByPhone(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success", "data": result})
}

func (h *PhoneAuthHandler) LoginByPhonePassword(c *gin.Context) {
	var req auth_service.PhonePasswordLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.service.LoginByPhonePassword(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success", "data": result})
}

func (h *PhoneAuthHandler) ResetPasswordByPhone(c *gin.Context) {
	var req auth_service.PhoneResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if err := h.service.ResetPasswordByPhone(c.Request.Context(), req); err != nil {
		h.respondPhoneAuthError(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *PhoneAuthHandler) RegisterRoutes(v1 *gin.RouterGroup) {
	phone := v1.Group("/phone")
	phone.POST("/check", h.CheckPhone)
	phone.POST("/code", h.SendCode)
	phone.POST("/code/verify", h.VerifyCode)
	phone.POST("/register", h.RegisterByPhone)
	phone.POST("/login", h.LoginByPhone)
	phone.POST("/password-login", h.LoginByPhonePassword)
	phone.POST("/reset-password", h.ResetPasswordByPhone)
}

func (h *PhoneAuthHandler) respondPhoneAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth_service.ErrPhoneRegistrationDisabled):
		response.Fail(c, response.ErrRegisterNotAllowed)
	case errors.Is(err, auth_service.ErrPhoneAccountExists):
		response.Fail(c, response.ErrUserExists)
	case errors.Is(err, auth_service.ErrPhoneAccountNotFound):
		response.Fail(c, response.ErrUserNotFound)
	case errors.Is(err, auth_service.ErrPhoneInvalid):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, auth_service.ErrPhoneTokenInvalid):
		response.Fail(c, response.ErrTokenInvalid)
	case errors.Is(err, auth_service.ErrPhoneCodeInvalid):
		response.Fail(c, response.ErrInvalidCode)
	case errors.Is(err, auth_service.ErrPhonePasswordMismatch):
		response.Fail(c, response.ErrEmailPasswordMismatch)
	case errors.Is(err, auth_service.ErrPhoneAccountInactive):
		response.Fail(c, response.ErrAccountFrozen)
	case errors.Is(err, auth_service.ErrPhoneSceneUnsupported):
		response.Fail(c, response.ErrInvalidParam)
	default:
		response.Fail(c, response.ErrThirdPartyService)
	}
}
