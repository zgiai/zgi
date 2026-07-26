package approval

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service     *Service
	taskManager *queue.TaskManager
}

func NewHandler(service *Service, taskManager *queue.TaskManager) *Handler {
	return &Handler{
		service:     service,
		taskManager: taskManager,
	}
}

func (h *Handler) GetForm(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	payload, err := h.service.GetFormByToken(c.Request.Context(), token)
	if err != nil {
		h.handleApprovalError(c, err)
		return
	}
	response.Success(c, payload)
}

func (h *Handler) SubmitForm(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	var accountID *string
	if value := c.GetString("account_id"); value != "" {
		accountID = &value
	}

	submitResult, err := h.service.SubmitByTokenWithResume(c.Request.Context(), token, req, accountID, nil)
	if err != nil {
		h.handleApprovalError(c, err)
		return
	}

	form := submitResult.Form
	if submitResult.Outbox != nil && h.taskManager != nil {
		if err := DispatchResumeOutbox(c.Request.Context(), h.service, h.taskManager, submitResult.Outbox); err != nil {
			logger.WarnContext(c.Request.Context(), "approval resume queued for outbox retry", "form_id", form.ID, err)
		}
	}

	action := ""
	if form.SelectedActionID != nil {
		action = *form.SelectedActionID
	}
	response.Success(c, map[string]interface{}{
		"form_id":           form.ID,
		"workflow_run_id":   form.WorkflowRunID,
		"status":            form.Status,
		"action":            action,
		"resume_enqueued":   submitResult.ResumeState != "waiting",
		"resume_state":      submitResult.ResumeState,
		"event_cursor":      submitResult.EventCursor,
		"idempotent_replay": submitResult.IdempotentReplay,
	})
}

func (h *Handler) GetRunEvents(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	afterSequence, err := parseNonNegativeQueryInt(c, "after")
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	limit, err := parseNonNegativeQueryInt(c, "limit")
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	payload, err := h.service.ListRunEventsByToken(c.Request.Context(), token, afterSequence, limit)
	if err != nil {
		h.handleApprovalError(c, err)
		return
	}
	response.Success(c, payload)
}

func (h *Handler) handleApprovalError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrFormNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrFormAlreadySubmitted):
		response.FailWithMessage(c, response.ErrInvalidParam, ErrFormAlreadySubmitted.Error())
	case errors.Is(err, ErrFormExpired):
		response.FailWithMessage(c, response.ErrInvalidParam, ErrFormExpired.Error())
	default:
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	}
}

func parseNonNegativeQueryInt(c *gin.Context, key string) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}
