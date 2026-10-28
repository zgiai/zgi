package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

type digitalHumanGovernanceRuntime interface {
	RunConfiguredToolGovernanceDecisionStream(context.Context, runtimeservice.Scope, runtimeservice.Caller, runtimeservice.RunConfig,
		uuid.UUID, uuid.UUID, string, runtimedto.ToolGovernanceDecisionRequest, func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error)
}

// ContinueToolGovernance restores the original sealed run, not the latest draft
// or a task approval. ChatRuntime claims the existing message and executes only
// the frozen business invocation; it never resubmits the original Shell.
func (s *RuntimeService) ContinueToolGovernance(ctx context.Context, scope Scope, app, conversationID, messageID uuid.UUID, correlation string,
	request runtimedto.ToolGovernanceDecisionRequest, onEvent func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	if s == nil || onEvent == nil || app == uuid.Nil || conversationID == uuid.Nil || messageID == uuid.Nil || strings.TrimSpace(correlation) == "" ||
		len(correlation) > 256 || (request.Action != "approve" && request.Action != "reject") ||
		(scope.Surface != SurfaceConsole && scope.Surface != SurfaceWebApp && scope.Surface != SurfaceAPI) {
		return nil, ErrInvalidRequest
	}
	lookup, ok := s.store.(digitalHumanContinuationRunLookup)
	if !ok {
		return nil, ErrRunConflict
	}
	runtime, ok := s.chat.(digitalHumanGovernanceRuntime)
	if !ok {
		return nil, ErrRunConflict
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, app, messageID)
	if err != nil {
		return nil, err
	}
	if !continuationRunOwnedByScope(run, scope, app) || run.ConversationID == nil || *run.ConversationID != conversationID || run.MessageID == nil || *run.MessageID != messageID {
		return nil, ErrConversationNotFound
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return nil, err
	}
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		return nil, err
	}
	s.attachAttachmentReader(&config, run, resolved, event, &scope)
	if request.Action == "approve" {
		if s.linux == nil {
			return nil, ErrRunConflict
		}
		config.RuntimeToolFactory, err = s.prepareContinuationLinux(ctx, run, resolved, event, scope)
		if err != nil {
			return nil, err
		}
		if config.RuntimeToolFactory == nil {
			return nil, ErrRunConflict
		}
	}
	_, chatScope, caller, err := s.conversationContext(scope, app)
	if err != nil {
		return nil, err
	}
	caller.RunID = cloneUUIDPointer(&run.ID)
	config.ConnectorRunID = run.ID.String()
	config.ConnectorInteractive = connectorInteractive(event)
	config.ConnectorRequesterID = connectorRequester(event)
	applyRunEndUserBilling(&config, run)
	result, err := runtime.RunConfiguredToolGovernanceDecisionStream(ctx, chatScope, caller, config, conversationID, messageID, strings.TrimSpace(correlation), request, onEvent)
	if err != nil {
		return result, err
	}
	if result != nil && result.Status == runtimemodel.MessageStatusCompleted && run.Status == RunStatusWaiting &&
		run.WaitingReason == runtimemodel.MessageStatusWaitingApproval && run.WaitingReceipt == nil && run.WaitingApprovalOrigin == platformCLIApprovalOrigin {
		// The repository verifies the completed message and original platform
		// owner again. Connector receipt-based approvals keep their own lifecycle.
		if err := s.store.CompleteRun(ctx, run.ID, result.Answer, executionTraceFromMetadata(result.Metadata), s.now()); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *RuntimeService) prepareContinuationLinux(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event, scope Scope) (func(*runtimeservice.PreparedChat) []skillloop.RuntimeTool, error) {
	if provider, ok := s.linux.(interface {
		PrepareContinuation(context.Context, *Run, *ResolvedExecution, invocation.Event, Scope) (func(*runtimeservice.PreparedChat) []skillloop.RuntimeTool, error)
	}); ok {
		return provider.PrepareContinuation(ctx, run, resolved, event, scope)
	}
	return s.linux.Prepare(ctx, run, resolved, event)
}

type digitalHumanGovernanceService interface {
	ContinueToolGovernance(context.Context, Scope, uuid.UUID, uuid.UUID, uuid.UUID, string, runtimedto.ToolGovernanceDecisionRequest, func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error)
}

// ContinueToolGovernance serves the authenticated console conversation route.
func (h *RuntimeHandler) ContinueToolGovernance(c *gin.Context) {
	scope, app, conversation, message, ok := h.userInputIDs(c)
	if !ok {
		return
	}
	var request runtimedto.ToolGovernanceDecisionRequest
	if !h.base.decode(c, &request) {
		return
	}
	service, ok := h.service.(digitalHumanGovernanceService)
	if !ok {
		h.base.writeError(c, applicationError(ErrRunConflict, "digital_human.governance.continue"))
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_, err := service.ContinueToolGovernance(c.Request.Context(), scope, app, conversation, message, c.Param("correlation_id"), request,
		func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, event.Payload)
		})
	if err != nil && !errors.Is(err, runtimeservice.ErrMessageStopped) && !runtimeservice.IsFinalizedStreamError(err) {
		_ = writer.write("", "error", gin.H{"error": "digital human tool governance continuation failed"})
	}
}

// ContinueWebAppToolGovernance requires the verified WebApp account subject.
func (h *ExternalHandler) ContinueWebAppToolGovernance(c *gin.Context) {
	scope, access, conversation, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	if scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil {
		h.writeAuthError(c, true)
		return
	}
	message, ok := parseUserInputMessageID(c, h.base, "digital_human.governance.message")
	if !ok {
		return
	}
	var request runtimedto.ToolGovernanceDecisionRequest
	if !h.base.decode(c, &request) {
		return
	}
	h.continueExternalToolGovernance(c, scope, access.Definition.ID, conversation, message, request)
}

// ContinueAPIToolGovernance uses the existing trusted API final-user contract.
func (h *ExternalHandler) ContinueAPIToolGovernance(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var raw struct {
		User   string `json:"user"`
		Action string `json:"action"`
	}
	if !h.base.decode(c, &raw) {
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
	message, ok := parseUserInputMessageID(c, h.base, "digital_human.governance.message")
	if !ok {
		return
	}
	h.continueExternalToolGovernance(c, scope, access.Definition.ID, conversation, message, runtimedto.ToolGovernanceDecisionRequest{Action: raw.Action})
}

func (h *ExternalHandler) continueExternalToolGovernance(c *gin.Context, scope Scope, app, conversation, message uuid.UUID, request runtimedto.ToolGovernanceDecisionRequest) {
	if strings.TrimSpace(c.Param("correlation_id")) == "" {
		h.writeApplicationError(c, fmt.Errorf("%w: missing correlation", ErrInvalidRequest), "digital_human.governance.continue")
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_, err := h.runtime.ContinueToolGovernance(c.Request.Context(), scope, app, conversation, message, c.Param("correlation_id"), request,
		func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		})
	if err != nil && c.Request.Context().Err() == nil {
		_ = h.writeStreamError(writer, uuid.Nil, err)
	}
}
