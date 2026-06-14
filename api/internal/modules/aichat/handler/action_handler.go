package handler

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *Handler) ListActionCapabilities(c *gin.Context) {
	scope, ok := h.actionScope(c)
	if !ok {
		return
	}
	capabilities, err := h.actionService.ListCapabilities(c.Request.Context(), scope)
	if err != nil {
		h.failAction(c, err)
		return
	}
	items := make([]actiondto.ActionCapabilityResponse, 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, actionCapabilityResponse(capability))
	}
	response.Success(c, items)
}

func (h *Handler) PlanAction(c *gin.Context) {
	scope, ok := h.actionScope(c)
	if !ok {
		return
	}
	var req actiondto.ActionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	view, err := h.actionService.PlanAction(c.Request.Context(), scope, req)
	if err != nil {
		h.failAction(c, err)
		return
	}
	response.Success(c, actionRunResponse(view))
}

func (h *Handler) GetAction(c *gin.Context) {
	scope, id, ok := h.actionScopedID(c)
	if !ok {
		return
	}
	view, err := h.actionService.GetActionRun(c.Request.Context(), scope, id)
	if err != nil {
		h.failAction(c, err)
		return
	}
	response.Success(c, actionRunResponse(view))
}

func (h *Handler) ConfirmAction(c *gin.Context) {
	scope, id, ok := h.actionScopedID(c)
	if !ok {
		return
	}
	var req actiondto.ConfirmActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	view, err := h.actionService.ConfirmAction(c.Request.Context(), scope, id, req)
	if err != nil {
		h.failAction(c, err)
		return
	}
	response.Success(c, actionRunResponse(view))
}

func (h *Handler) ExecuteAction(c *gin.Context) {
	scope, id, ok := h.actionScopedID(c)
	if !ok {
		return
	}
	var req actiondto.ExecuteActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	view, err := h.actionService.ExecuteAction(c.Request.Context(), scope, id, req)
	if err != nil {
		h.failAction(c, err)
		return
	}
	response.Success(c, actionRunResponse(view))
}

func (h *Handler) StreamActionEvents(c *gin.Context) {
	scope, id, ok := h.actionScopedID(c)
	if !ok {
		return
	}
	setupSSE(c)
	view, err := h.actionService.GetActionRun(c.Request.Context(), scope, id)
	if err != nil {
		_ = writeSSEEvent(c, "", "error", gin.H{"message": err.Error()})
		return
	}
	payload := actionRunResponse(view)
	_ = writeSSEEvent(c, "", "action_run_snapshot", payload)
	_ = writeSSEEvent(c, "", "action_run_end", gin.H{"action_run_id": payload.ID, "status": payload.Status})
}

func (h *Handler) actionScope(c *gin.Context) (actionservice.Scope, bool) {
	scope, ok := h.scope(c)
	if !ok {
		return actionservice.Scope{}, false
	}
	return actionservice.Scope{
		OrganizationID:  scope.OrganizationID,
		AccountID:       scope.AccountID,
		WorkspaceID:     scope.WorkspaceID,
		SkipAccessCheck: scope.SkipAccessCheck,
	}, true
}

func (h *Handler) actionScopedID(c *gin.Context) (actionservice.Scope, uuid.UUID, bool) {
	scope, ok := h.actionScope(c)
	if !ok {
		return actionservice.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return actionservice.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func (h *Handler) failAction(c *gin.Context, err error) {
	switch {
	case errors.Is(err, actionservice.ErrPermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, actionservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, actionservice.ErrInvalidInput), errors.Is(err, actionservice.ErrConfirmationRequired):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		logger.ErrorContext(c.Request.Context(), "aichat action runtime request failed", err)
		response.Fail(c, response.ErrSystemError)
	}
}

func actionCapabilityResponse(capability actionservice.CapabilityManifest) actiondto.ActionCapabilityResponse {
	return actiondto.ActionCapabilityResponse{
		ID:                   capability.ID,
		Domain:               capability.Domain,
		Action:               capability.Action,
		Name:                 capability.Name,
		Description:          capability.Description,
		Runtime:              capability.Runtime,
		AuthMode:             capability.AuthMode,
		RiskLevel:            capability.RiskLevel,
		RequiresConfirmation: capability.RequiresConfirmation,
		IdempotencyRequired:  capability.IdempotencyRequired,
		TokenTTLSeconds:      capability.TokenTTLSeconds,
		AllowedResources:     append([]string(nil), capability.AllowedResources...),
		Scopes:               append([]string(nil), capability.Scopes...),
	}
}

func actionRunResponse(view *actionservice.ActionRunView) actiondto.ActionRunResponse {
	if view == nil || view.Run == nil {
		return actiondto.ActionRunResponse{Steps: []actiondto.ActionStepResponse{}}
	}
	run := view.Run
	resp := actiondto.ActionRunResponse{
		ID:                   run.ID.String(),
		OrganizationID:       run.OrganizationID.String(),
		AccountID:            run.AccountID.String(),
		IdempotencyKey:       run.IdempotencyKey,
		Intent:               run.Intent,
		CapabilityID:         run.CapabilityID,
		Title:                run.Title,
		Summary:              run.Summary,
		Status:               run.Status,
		RiskLevel:            run.RiskLevel,
		RequiresConfirmation: run.RequiresConfirmation,
		ConfirmationStatus:   actionConfirmationStatus(run),
		ConfirmedAt:          unixPtr(run.ConfirmedAt),
		CanceledAt:           unixPtr(run.CanceledAt),
		Error:                run.Error,
		Resources:            nonNilMap(run.Resources),
		Arguments:            nonNilMap(run.Arguments),
		Ledger:               nonNilMap(run.Ledger),
		Metadata:             nonNilMap(run.Metadata),
		Steps:                actionStepResponses(view.Steps),
		CreatedAt:            run.CreatedAt.Unix(),
		UpdatedAt:            run.UpdatedAt.Unix(),
	}
	if run.WorkspaceID != nil {
		resp.WorkspaceID = stringPtr(run.WorkspaceID.String())
	}
	if run.ConversationID != nil {
		resp.ConversationID = stringPtr(run.ConversationID.String())
	}
	if run.MessageID != nil {
		resp.MessageID = stringPtr(run.MessageID.String())
	}
	if run.ConfirmedBy != nil {
		resp.ConfirmedBy = stringPtr(run.ConfirmedBy.String())
	}
	if view.Capability != nil && view.Capability.ID != "" {
		capability := actionCapabilityResponse(*view.Capability)
		resp.Capability = &capability
	}
	return resp
}

func actionStepResponses(steps []*actionmodel.ActionStep) []actiondto.ActionStepResponse {
	out := make([]actiondto.ActionStepResponse, 0, len(steps))
	for _, step := range steps {
		if step == nil {
			continue
		}
		out = append(out, actiondto.ActionStepResponse{
			ID:                   step.ID.String(),
			RunID:                step.RunID.String(),
			StepIndex:            step.StepIndex,
			StepKey:              step.StepKey,
			CapabilityID:         step.CapabilityID,
			Title:                step.Title,
			Status:               step.Status,
			RiskLevel:            step.RiskLevel,
			RequiresConfirmation: step.RequiresConfirmation,
			StartedAt:            unixPtr(step.StartedAt),
			CompletedAt:          unixPtr(step.CompletedAt),
			Error:                step.Error,
			Input:                nonNilMap(step.Input),
			Output:               nonNilMap(step.Output),
			Metadata:             nonNilMap(step.Metadata),
			CreatedAt:            step.CreatedAt.Unix(),
			UpdatedAt:            step.UpdatedAt.Unix(),
		})
	}
	return out
}

func actionConfirmationStatus(run *actionmodel.ActionRun) string {
	if run == nil {
		return "unknown"
	}
	if run.Status == actionmodel.ActionRunStatusCanceled {
		return "canceled"
	}
	if run.ConfirmedAt != nil {
		return "confirmed"
	}
	if run.RequiresConfirmation {
		return "pending"
	}
	return "not_required"
}

func unixPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	out := value.Unix()
	return &out
}

func nonNilMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	return input
}
