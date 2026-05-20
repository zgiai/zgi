package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/dto"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

// GDPRHandler handles GDPR-related HTTP requests
type GDPRHandler struct {
	service service.GDPRService
}

// NewGDPRHandler creates a new GDPR handler
func NewGDPRHandler(service service.GDPRService) *GDPRHandler {
	return &GDPRHandler{service: service}
}

// ============================================================================
// Data Export
// ============================================================================

// ExportMyData exports the current user's data
// POST /console/api/gdpr/export
func (h *GDPRHandler) ExportMyData(c *gin.Context) {
	accountID, exists := c.Get("account_id")
	if !exists {
		response.FailWithMessage(c, response.ErrUnauthorized, "unauthorized")
		return
	}

	accID, err := uuid.Parse(accountID.(string))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "invalid account ID")
		return
	}

	var req dto.ExportDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body - export current user's data
		req.AccountID = accID
	}

	// Regular users can only export their own data
	if req.AccountID != accID {
		response.FailWithMessage(c, response.ErrAPIKeyPermissionDenied, "can only export your own data")
		return
	}

	result, err := h.service.ExportUserData(c.Request.Context(), accID, &req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ============================================================================
// Data Erasure
// ============================================================================

// EraseMyData erases the current user's data
// POST /console/api/gdpr/erase
func (h *GDPRHandler) EraseMyData(c *gin.Context) {
	accountID, exists := c.Get("account_id")
	if !exists {
		response.FailWithMessage(c, response.ErrUnauthorized, "unauthorized")
		return
	}

	accID, err := uuid.Parse(accountID.(string))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "invalid account ID")
		return
	}

	var req dto.EraseDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}

	// Regular users can only erase their own data
	req.AccountID = accID

	result, err := h.service.EraseUserData(c.Request.Context(), accID, &req)
	if err != nil {
		if err == service.ErrInvalidConfirmation {
			response.FailWithMessage(c, response.ErrInvalidParams, "confirmation key must match your email")
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// ============================================================================
// Consent Management
// ============================================================================

// GetMyConsents gets the current user's consent status
// GET /console/api/gdpr/consents
func (h *GDPRHandler) GetMyConsents(c *gin.Context) {
	accountID, exists := c.Get("account_id")
	if !exists {
		response.FailWithMessage(c, response.ErrUnauthorized, "unauthorized")
		return
	}

	accID, err := uuid.Parse(accountID.(string))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "invalid account ID")
		return
	}

	result, err := h.service.GetConsentStatus(c.Request.Context(), accID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// UpdateMyConsent updates the current user's consent
// PUT /console/api/gdpr/consents
func (h *GDPRHandler) UpdateMyConsent(c *gin.Context) {
	accountID, exists := c.Get("account_id")
	if !exists {
		response.FailWithMessage(c, response.ErrUnauthorized, "unauthorized")
		return
	}

	accID, err := uuid.Parse(accountID.(string))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "invalid account ID")
		return
	}

	var req dto.UpdateConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	if err := h.service.UpdateConsent(c.Request.Context(), accID, &req, ipAddress, userAgent); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"message": "consent updated"})
}
