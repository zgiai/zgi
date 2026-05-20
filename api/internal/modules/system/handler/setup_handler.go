package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type SetupHandler struct {
	setupService *service.BootstrapService
}

// NewSetupHandler creates a setup handler backed by the shared bootstrap service.
func NewSetupHandler(setupService *service.BootstrapService) *SetupHandler {
	return &SetupHandler{
		setupService: setupService,
	}
}

// ============================================================================
// System feature handler - merged from setup_feature_handler.go
// ============================================================================

type FeatureHandler struct {
	featureService interfaces.FeatureService
}

// NewFeatureHandler creates a feature handler.
func NewFeatureHandler(featureService interfaces.FeatureService) *FeatureHandler {
	return &FeatureHandler{
		featureService: featureService,
	}
}

// GetSystemFeatures returns the current system feature set.
func (h *FeatureHandler) GetSystemFeatures(c *gin.Context) {
	features, err := h.featureService.GetSystemFeatures(c.Request.Context())
	if err != nil {
		response.Fail(c, response.ErrSystemFeatureGetFailed)
		return
	}

	responseData := model.NewSystemFeatureResponse(features)
	response.Success(c, responseData)
}

// ============================================================================
// System setup handler methods
// ============================================================================

// GetSetupStatus returns setup progress for the current edition.
func (h *SetupHandler) GetSetupStatus(c *gin.Context) {
	// Check if it's a self-hosted version
	edition := config.Current().Platform.Edition
	if edition == "SELF_HOSTED" {
		setupStatus, err := h.setupService.GetSetupStatus()
		if err != nil {
			response.Fail(c, response.ErrSetupStatusGetFailed)
			return
		}

		if setupStatus != nil {
			response.Success(c, model.SetupResponse{
				Step:    "finished",
				SetupAt: &setupStatus.SetupAt,
			})
			return
		}

		response.Success(c, model.SetupResponse{
			Step: "not_started",
		})
		return
	}

	// Non-self-hosted version, return completed directly
	response.Success(c, model.SetupResponse{
		Step: "finished",
	})
}

// OnlyEditionSelfHosted rejects setup calls outside self-hosted deployments.
func OnlyEditionSelfHosted() gin.HandlerFunc {
	return func(c *gin.Context) {
		edition := config.Current().Platform.Edition
		if edition != "SELF_HOSTED" {
			response.Fail(c, response.ErrSelfHostedOnly)
			c.Abort()
			return
		}
		c.Next()
	}
}

// ExtractRemoteIP returns the best-effort client IP from headers or the request.
func ExtractRemoteIP(c *gin.Context) string {
	// First try to get from X-Forwarded-For header
	ip := c.GetHeader("X-Forwarded-For")
	if ip != "" {
		// If there are multiple IPs, take the first one
		parts := strings.Split(ip, ",")
		ip = strings.TrimSpace(parts[0])
		return ip
	}

	// Try to get from X-Real-IP header
	ip = c.GetHeader("X-Real-IP")
	if ip != "" {
		return ip
	}

	// If none, use RemoteAddr
	return c.ClientIP()
}

// SetupSystem executes first-time setup for self-hosted deployments.
func (h *SetupHandler) SetupSystem(c *gin.Context) {
	var req model.SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrSetupRequestInvalid)
		return
	}

	ipAddress := ExtractRemoteIP(c)
	if err := h.setupService.Setup(c.Request.Context(), req.Email, req.Name, req.Password, ipAddress); err != nil {
		switch {
		case errors.Is(err, service.ErrAlreadySetup):
			response.Fail(c, response.ErrAlreadySetup)
		case errors.Is(err, service.ErrNotInitValidated):
			response.Fail(c, response.ErrNotInitValidated)
		case service.IsPasswordValidationError(err):
			response.Fail(c, response.ErrPasswordValidationFailed)
		default:
			response.Fail(c, response.ErrSystemSetupFailed)
		}
		return
	}

	response.Success(c, model.SetupResult{Result: "success"})
}
