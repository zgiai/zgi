package memory

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetMe(c *gin.Context) {
	accountID, ok := currentAccountID(c)
	if !ok {
		return
	}
	result, err := h.service.GetMe(c.Request.Context(), accountID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	accountID, ok := currentAccountID(c)
	if !ok {
		return
	}
	var req UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}
	result, err := h.service.SetEnabled(c.Request.Context(), accountID, req.Enabled)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateEntry(c *gin.Context) {
	accountID, ok := currentAccountID(c)
	if !ok {
		return
	}
	var req CreateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}
	entry, err := h.service.CreateEntry(c.Request.Context(), accountID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *Handler) UpdateEntry(c *gin.Context) {
	accountID, entryID, ok := currentAccountAndEntryID(c)
	if !ok {
		return
	}
	var req UpdateEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}
	entry, err := h.service.UpdateEntry(c.Request.Context(), accountID, entryID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *Handler) DeleteEntry(c *gin.Context) {
	accountID, entryID, ok := currentAccountAndEntryID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteEntry(c.Request.Context(), accountID, entryID); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func currentAccountAndEntryID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	accountID, ok := currentAccountID(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	entryID, err := uuid.Parse(strings.TrimSpace(c.Param("entry_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return uuid.Nil, uuid.Nil, false
	}
	return accountID, entryID, true
}

func currentAccountID(c *gin.Context) (uuid.UUID, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return uuid.Nil, false
	}
	return accountID, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrInvalidInput):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
	}
}
