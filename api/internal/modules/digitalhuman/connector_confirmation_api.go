package digitalhuman

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// DecideAPIConnectorConfirmation lets the requester of an API run confirm or
// cancel the connector operation the run waits for (ADR 0007). The requester
// decides whether the operation uses their own credential or one its owner
// delegated to the DigitalHuman; the decision executes the operation, and a
// completed operation ends the run, as a channel reply does.
func (s *RuntimeService) DecideAPIConnectorConfirmation(ctx context.Context, scope Scope, app, conversationID, messageID uuid.UUID, confirm bool) (ChannelConnectorDecision, error) {
	if s == nil || app == uuid.Nil || conversationID == uuid.Nil || messageID == uuid.Nil ||
		scope.Surface != SurfaceAPI || scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil {
		return ChannelConnectorDecision{}, ErrInvalidRequest
	}
	lookup, ok := s.store.(digitalHumanContinuationRunLookup)
	if !ok || s.channelConnectors == nil {
		return ChannelConnectorDecision{}, ErrRunConflict
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, app, messageID)
	if err != nil {
		return ChannelConnectorDecision{}, err
	}
	if !continuationRunOwnedByScope(run, scope, app) || run.ConversationID == nil || *run.ConversationID != conversationID ||
		run.MessageID == nil || *run.MessageID != messageID || run.WaitingReceipt == nil || *run.WaitingReceipt == uuid.Nil ||
		run.Status != RunStatusWaiting {
		return ChannelConnectorDecision{}, ErrConversationNotFound
	}
	decision, err := s.channelConnectors.DecideChannelConnectorConfirmation(ctx, run.OrganizationID, *scope.UserSubjectID, *run.WaitingReceipt, confirm)
	if err != nil {
		return ChannelConnectorDecision{}, err
	}
	if decision.State == "completed" {
		if completeErr := s.store.CompleteRun(context.WithoutCancel(ctx), run.ID, strings.TrimSpace(decision.Summary), nil, s.now()); completeErr != nil && !errors.Is(completeErr, ErrRunConflict) {
			logger.WarnContext(ctx, "failed to complete digital human API connector run", "run_id", run.ID.String(), completeErr)
		}
	}
	return decision, nil
}

// ContinueAPIConnectorConfirmation confirms or cancels the connector
// operation an API run waits for.
func (h *ExternalHandler) ContinueAPIConnectorConfirmation(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var raw struct {
		User     string `json:"user"`
		Decision string `json:"decision"`
	}
	if !h.base.decode(c, &raw) {
		return
	}
	if raw.Decision != "confirmed" && raw.Decision != "cancelled" {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.connector_confirmation.decision")
		return
	}
	scope, ok := h.apiUserScope(c, key, access, raw.User)
	if !ok {
		return
	}
	conversation, ok := h.conversationID(c)
	if !ok {
		return
	}
	message, ok := parseUserInputMessageID(c, h.base, "digital_human.connector_confirmation.message")
	if !ok {
		return
	}
	decision, err := h.runtime.DecideAPIConnectorConfirmation(c.Request.Context(), scope, access.Definition.ID, conversation, message, raw.Decision == "confirmed")
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.connector_confirmation")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "0", "message": "success", "data": gin.H{
		"state": decision.State, "error_code": decision.ErrorCode, "title": decision.Title, "summary": decision.Summary,
	}})
}
