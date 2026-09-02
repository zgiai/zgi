package developeraccess

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func accountID(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.GetString("account_id"))
	if id == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}
	return id, true
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		response.Fail(c, response.ErrWorkspaceDenied)
	case errors.Is(err, ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrInvalid):
		response.Fail(c, response.ErrInvalidParams)
	case errors.Is(err, ErrAccessDisabled):
		response.FailWithMessage(c, response.ErrActionNotAllowed, "Developer API access is disabled for this workspace")
	case errors.Is(err, ErrApprovalNeeded):
		response.FailWithMessage(c, response.ErrActionNotAllowed, "Developer API access approval is required")
	case errors.Is(err, ErrConflict):
		response.FailWithMessage(c, response.ErrActionNotAllowed, "The requested operation conflicts with the current access state")
	case errors.Is(err, ErrQuotaExceeded):
		response.Fail(c, response.ErrOpenAIQuota)
	default:
		logger.ErrorContext(c.Request.Context(), "developer access request failed", "error", err)
		response.Fail(c, response.ErrSystemError)
	}
}

func (h *Handler) GetMe(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	result, err := h.service.GetMe(c.Request.Context(), c.Param("workspace_id"), account)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) GetPolicy(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	result, err := h.service.GetPolicy(c.Request.Context(), c.Param("workspace_id"), account)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) PutPolicy(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input PolicyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.PutPolicy(c.Request.Context(), c.Param("workspace_id"), account, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ListRequests(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	result, err := h.service.ListRequests(c.Request.Context(), c.Param("workspace_id"), account, c.Query("status"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateRequest(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input CreateRequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.CreateRequest(c.Request.Context(), c.Param("workspace_id"), account, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) CancelRequest(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	result, err := h.service.CancelRequest(c.Request.Context(), c.Param("workspace_id"), account, c.Param("request_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) review(c *gin.Context, approve bool) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input ReviewRequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.ReviewRequest(c.Request.Context(), c.Param("workspace_id"), account, c.Param("request_id"), approve, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ApproveRequest(c *gin.Context) { h.review(c, true) }
func (h *Handler) RejectRequest(c *gin.Context)  { h.review(c, false) }

func (h *Handler) ListKeys(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	result, err := h.service.ListKeys(c.Request.Context(), c.Param("workspace_id"), account)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateKey(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input CreateKeyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.CreateKey(c.Request.Context(), c.Param("workspace_id"), account, input)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) UpdateKey(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input UpdateKeyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.UpdateKey(c.Request.Context(), c.Param("workspace_id"), account, c.Param("key_id"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) setStatus(c *gin.Context, status string) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	reason := ""
	if status == "revoked" {
		var input RevokeKeyInput
		if err := c.ShouldBindJSON(&input); err != nil {
			response.Fail(c, response.ErrInvalidParams)
			return
		}
		reason = input.Reason
	}
	result, err := h.service.SetKeyStatus(c.Request.Context(), c.Param("workspace_id"), account, c.Param("key_id"), status, reason)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) DisableKey(c *gin.Context) { h.setStatus(c, "inactive") }
func (h *Handler) EnableKey(c *gin.Context)  { h.setStatus(c, "active") }
func (h *Handler) RevokeKey(c *gin.Context)  { h.setStatus(c, "revoked") }
