package developeraccess

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

var (
	legacyAccessDisabled = appcatalog.MustLegacyKey("llm.developer_access.disabled:403003")
	legacyApprovalNeeded = appcatalog.MustLegacyKey("llm.developer_access.approval_required:403003")
	legacyAccessConflict = appcatalog.MustLegacyKey("llm.developer_access.conflict:403003")
)

type Handler struct {
	service   *Service
	projector *apptransport.Projector
}

func NewHandler(service *Service, projector *apptransport.Projector) *Handler {
	return &Handler{service: service, projector: projector}
}

func accountID(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.GetString("account_id"))
	if id == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}
	return id, true
}

func (h *Handler) writeCatalogedLegacyError(c *gin.Context, err error, legacy appcatalog.LegacyKey) bool {
	if h.projector == nil {
		return false
	}
	message := h.projector.ProjectLegacyMessage(err, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")), legacy)
	if message.Resolution != apptransport.ResolutionMatched {
		return false
	}
	c.Header(apptransport.HeaderApplicationErrorCode, message.AppCode.String())
	response.FailWithMessage(c, response.ErrActionNotAllowed, message.Message)
	return true
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		response.Fail(c, response.ErrWorkspaceDenied)
	case errors.Is(err, ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrInvalid):
		response.Fail(c, response.ErrInvalidParams)
	case apperror.IsCode(err, llmerrors.AppCodeDeveloperAccessDisabled):
		if !h.writeCatalogedLegacyError(c, err, legacyAccessDisabled) {
			response.Fail(c, response.ErrActionNotAllowed)
		}
	case apperror.IsCode(err, llmerrors.AppCodeDeveloperApprovalNeeded):
		if !h.writeCatalogedLegacyError(c, err, legacyApprovalNeeded) {
			response.Fail(c, response.ErrActionNotAllowed)
		}
	case apperror.IsCode(err, llmerrors.AppCodeDeveloperAccessConflict):
		if !h.writeCatalogedLegacyError(c, err, legacyAccessConflict) {
			response.Fail(c, response.ErrActionNotAllowed)
		}
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ListAudit(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input AuditQuery
	if err := c.ShouldBindQuery(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.ListAudit(c.Request.Context(), c.Param("workspace_id"), account, input)
	if err != nil {
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
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
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) DisableKey(c *gin.Context) { h.setStatus(c, "inactive") }
func (h *Handler) EnableKey(c *gin.Context)  { h.setStatus(c, "active") }
func (h *Handler) RevokeKey(c *gin.Context)  { h.setStatus(c, "revoked") }

func (h *Handler) RotateKey(c *gin.Context) {
	account, ok := accountID(c)
	if !ok {
		return
	}
	var input RotateKeyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	result, err := h.service.RotateKey(c.Request.Context(), c.Param("workspace_id"), account, c.Param("key_id"), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}
