package digitalhuman

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

type digitalHumanTaskApprovalRequest struct {
	Type          string                 `json:"type"`
	User          string                 `json:"user"`
	ApprovalToken string                 `json:"approval_token"`
	Action        string                 `json:"action"`
	Inputs        map[string]interface{} `json:"inputs"`
}

func (h *RuntimeHandler) ContinueTaskApproval(c *gin.Context) {
	scope, digitalHumanID, conversationID, messageID, ok := h.taskApprovalIDs(c)
	if !ok {
		return
	}
	request, err := decodeTaskApprovalRequest(h.base, c)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.task.approval"))
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	if continuation, ok := h.service.(interface {
		WorkflowContinuationEnabled() bool
		ContinueWorkflowApproval(context.Context, Scope, uuid.UUID, uuid.UUID, uuid.UUID, runtimeservice.DigitalHumanTaskApprovalRequest, func(runtimeservice.StreamEvent) error) error
	}); ok && continuation.WorkflowContinuationEnabled() {
		err := continuation.ContinueWorkflowApproval(c.Request.Context(), scope, digitalHumanID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		})
		if !errors.Is(err, ErrDigitalHumanTaskContinuation) {
			if err != nil && c.Request.Context().Err() == nil {
				_ = writer.write("", "error", gin.H{"error": err.Error()})
			}
			return
		}
	}
	approvalService, ok := h.service.(interface {
		ContinueTaskApproval(context.Context, Scope, uuid.UUID, uuid.UUID, uuid.UUID, runtimeservice.DigitalHumanTaskApprovalRequest, func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error)
	})
	if !ok {
		_ = writer.write("", "error", gin.H{"error": "task approval unavailable"})
		return
	}
	result, err := approvalService.ContinueTaskApproval(c.Request.Context(), scope, digitalHumanID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
		return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
	})
	if err != nil {
		if c.Request.Context().Err() == nil {
			_ = writer.write("", "error", gin.H{"error": "task approval failed"})
		}
		return
	}
	_ = result
}

func (h *ExternalHandler) ContinueWebAppTaskApproval(c *gin.Context) {
	scope, access, conversationID, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	if scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil {
		h.writeAuthError(c, true)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.task.approval.message")
		return
	}
	request, err := decodeTaskApprovalRequest(h.base, c)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.task.approval")
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	if h.runtime != nil && h.runtime.WorkflowContinuationEnabled() {
		err := h.runtime.ContinueWorkflowApproval(c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		})
		if !errors.Is(err, ErrDigitalHumanTaskContinuation) {
			if err != nil && c.Request.Context().Err() == nil {
				_ = h.writeStreamError(writer, uuid.Nil, err)
			}
			return
		}
	}
	result, err := h.runtime.ContinueTaskApproval(c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
		return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
	})
	if err != nil {
		if c.Request.Context().Err() == nil {
			_ = h.writeStreamError(writer, uuid.Nil, err)
		}
		return
	}
	_ = result
}

// ContinueAPITaskApproval exposes the same durable task approval continuation
// to API callers. The user field is required so the API key can address the
// same stable UserSubject that created the conversation and proposal.
func (h *ExternalHandler) ContinueAPITaskApproval(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var raw digitalHumanTaskApprovalRequest
	if !h.base.decode(c, &raw) || strings.TrimSpace(raw.User) == "" {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.task.approval.user")
		return
	}
	scope, ok := h.apiUserScope(c, key, access, raw.User)
	if !ok {
		return
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil || conversationID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.task.approval.conversation")
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.task.approval.message")
		return
	}
	request, err := normalizeTaskApprovalRequest(raw)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.task.approval")
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	if h.runtime != nil && h.runtime.WorkflowContinuationEnabled() {
		err := h.runtime.ContinueWorkflowApproval(c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		})
		if !errors.Is(err, ErrDigitalHumanTaskContinuation) {
			if err != nil && c.Request.Context().Err() == nil {
				_ = h.writeStreamError(writer, uuid.Nil, err)
			}
			return
		}
	}
	result, err := h.runtime.ContinueTaskApproval(c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, request, func(event runtimeservice.StreamEvent) error {
		return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
	})
	if err != nil {
		if c.Request.Context().Err() == nil {
			_ = h.writeStreamError(writer, uuid.Nil, err)
		}
		return
	}
	_ = result
}

func decodeTaskApprovalRequest(base *Handler, c *gin.Context) (runtimeservice.DigitalHumanTaskApprovalRequest, error) {
	var request digitalHumanTaskApprovalRequest
	if base == nil || !base.decode(c, &request) {
		return runtimeservice.DigitalHumanTaskApprovalRequest{}, ErrInvalidRequest
	}
	return normalizeTaskApprovalRequest(request)
}

func normalizeTaskApprovalRequest(request digitalHumanTaskApprovalRequest) (runtimeservice.DigitalHumanTaskApprovalRequest, error) {
	if !strings.EqualFold(strings.TrimSpace(request.Type), "approval") && !strings.EqualFold(strings.TrimSpace(request.Type), "question_answer") {
		return runtimeservice.DigitalHumanTaskApprovalRequest{}, ErrInvalidRequest
	}
	if strings.EqualFold(strings.TrimSpace(request.Type), "approval") && strings.TrimSpace(request.ApprovalToken) == "" {
		return runtimeservice.DigitalHumanTaskApprovalRequest{}, ErrInvalidRequest
	}
	return runtimeservice.DigitalHumanTaskApprovalRequest{
		Type:          request.Type,
		ApprovalToken: strings.TrimSpace(request.ApprovalToken),
		Action:        strings.TrimSpace(request.Action),
		Inputs:        request.Inputs,
	}, nil
}

func (h *RuntimeHandler) taskApprovalIDs(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	// The generic console scope is intentionally transport-neutral when it is
	// built by Handler.scope.  Approval continuation, however, also applies
	// the task owner policy, which must distinguish a console account from an
	// external/channel caller.  Stamp the surface here so the continuation can
	// safely fall back to the authenticated account as the UserSubject.
	scope.Surface = SurfaceConsole
	scope.PrincipalType = string(invocation.PrincipalAccount)
	scope.PrincipalID = scope.ActorID
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil || conversationID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.task.approval.conversation"))
		return Scope{}, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.task.approval.message"))
		return Scope{}, uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return scope, digitalHumanID, conversationID, messageID, true
}
