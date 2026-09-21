package digitalhuman

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type RuntimeHandlerService interface {
	SubmitManual(context.Context, Scope, uuid.UUID, ManualInvocationRequest) (*Run, bool, error)
	GetRun(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error)
	GetRunRuntimeLog(context.Context, Scope, uuid.UUID, uuid.UUID) (*RunRuntimeLog, error)
	ListRuns(context.Context, Scope, uuid.UUID, ListQuery) (*RunListResult, error)
	CancelRun(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error)
	ListResourceCandidates(context.Context, Scope, uuid.UUID, ResourceCandidateQuery) (*ResourceCandidateList, error)
	ListSkillCandidates(context.Context, Scope) (*ResourceCandidateList, error)
	ListConnectorCandidates(context.Context, Scope, ResourceCandidateQuery) (*ResourceCandidateList, error)
	ListConversations(context.Context, Scope, uuid.UUID, int, int) ([]*runtimemodel.Conversation, int64, error)
	SearchConversations(context.Context, Scope, uuid.UUID, string, int) ([]*runtimeservice.SearchResult, error)
	GetConversation(context.Context, Scope, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error)
	UpdateConversation(context.Context, Scope, uuid.UUID, uuid.UUID, runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error)
	DeleteConversation(context.Context, Scope, uuid.UUID, uuid.UUID) error
	ListConversationMessages(context.Context, Scope, uuid.UUID, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error)
	ListConversationRuntimeInputs(context.Context, Scope, uuid.UUID, uuid.UUID) ([]*runtimemodel.RuntimeInput, error)
	SubmitRuntimeInput(context.Context, Scope, uuid.UUID, runtimeservice.SubmitRuntimeInputRequest) (*runtimeservice.SubmitRuntimeInputResult, error)
	StopConversation(context.Context, Scope, uuid.UUID, uuid.UUID) (*runtimeservice.StopConversationResult, error)
	StreamConversationEvents(context.Context, Scope, uuid.UUID, uuid.UUID, uuid.UUID, string, func(runtimeservice.StreamEvent) error) error
}

type runtimeRegenerationService interface {
	SubmitRegeneration(context.Context, Scope, uuid.UUID, RegenerateInvocationRequest) (*Run, bool, error)
}

type RuntimeHandler struct {
	service RuntimeHandlerService
	base    *Handler
}

func NewRuntimeHandler(service RuntimeHandlerService, projector *apptransport.Projector) *RuntimeHandler {
	if service == nil {
		panic("DigitalHuman runtime handler requires service")
	}
	if projector == nil {
		panic("DigitalHuman runtime handler requires application error projector")
	}
	return &RuntimeHandler{service: service, base: &Handler{projector: projector}}
}

func (h *RuntimeHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Registered beside the id-scoped group: creation needs the bindable skills
	// and owner Connector connections before a digital human exists.
	router.GET("/digital-human-skill-candidates", h.ListSkillCandidates)
	router.GET("/digital-human-connector-candidates", h.ListConnectorCandidates)
	group := router.Group("/digital-humans/:id")
	group.POST("/invocations", h.SubmitManual)
	group.GET("/draft/binding-candidates", h.ListResourceCandidates)
	group.GET("/runs", h.ListRuns)
	group.GET("/runs/:run_id", h.GetRun)
	group.GET("/runs/:run_id/runtime-log", h.GetRunRuntimeLog)
	group.POST("/runs/:run_id/cancel", h.CancelRun)
	group.GET("/runtime/conversations", h.ListConversations)
	group.GET("/runtime/search", h.SearchConversations)
	group.GET("/runtime/conversations/:conversation_id", h.GetConversation)
	group.PATCH("/runtime/conversations/:conversation_id", h.UpdateConversation)
	group.DELETE("/runtime/conversations/:conversation_id", h.DeleteConversation)
	group.GET("/runtime/conversations/:conversation_id/messages", h.ListConversationMessages)
	group.POST("/runtime/conversations/:conversation_id/messages/:message_id/runtime-inputs", h.SubmitRuntimeInput)
	group.POST("/runtime/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue", h.ContinueUserInput)
	group.POST("/runtime/conversations/:conversation_id/stop", h.StopConversation)
	group.GET("/runtime/conversations/:conversation_id/events", h.StreamConversationEvents)
	group.POST("/runtime/conversations/:conversation_id/messages/:message_id/workflow-continuation", h.ContinueTaskApproval)
	group.POST("/runtime/messages/:message_id/regenerate", h.RegenerateMessage)
}

func (h *RuntimeHandler) RegenerateMessage(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.message.regenerate"))
		return
	}
	var request runtimedto.RegenerateMessageRequest
	if !h.base.decode(c, &request) {
		return
	}
	service, ok := h.service.(runtimeRegenerationService)
	if !ok {
		h.base.writeError(c, applicationError(ErrRunConflict, "digital_human.message.regenerate"))
		return
	}
	run, _, err := service.SubmitRegeneration(c.Request.Context(), scope, digitalHumanID, RegenerateInvocationRequest{
		MessageID: messageID, Query: request.Query, Surface: request.Surface,
		RuntimeContext: request.RuntimeContext, OperationContext: request.OperationContext,
		Model: request.Model, Provider: request.Provider, Parameters: request.Parameters, UseMemory: request.UseMemory,
	})
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.message.regenerate"))
		return
	}
	c.Header("X-ZGI-Run-ID", run.ID.String())
	c.Header("Access-Control-Expose-Headers", "X-ZGI-Run-ID")
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	if err := streamRegeneratedRun(c, h.service, writer, scope, digitalHumanID, run.ID); err != nil && c.Request.Context().Err() == nil {
		_ = writer.write("", "error", gin.H{"error": "digital human message replay failed"})
	}
}

func streamRegeneratedRun(c *gin.Context, service RuntimeHandlerService, writer *digitalHumanSSEWriter, scope Scope, digitalHumanID, runID uuid.UUID) error {
	for {
		run, err := service.GetRun(c.Request.Context(), scope, digitalHumanID, runID)
		if err != nil {
			return err
		}
		if run.ConversationID != nil && run.MessageID != nil {
			err = service.StreamConversationEvents(c.Request.Context(), scope, digitalHumanID, *run.ConversationID, *run.MessageID, "", func(event runtimeservice.StreamEvent) error {
				return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
			})
			return err
		}
		if isTerminalRunStatus(run.Status) {
			return fmt.Errorf("regenerated message did not start")
		}
		select {
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ListResourceCandidates returns safe, version-pinned choices for the draft editor.
func (h *RuntimeHandler) ListResourceCandidates(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	limit, ok := candidateLimit(c)
	if !ok {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.bindings.candidates"))
		return
	}
	result, err := h.service.ListResourceCandidates(c.Request.Context(), scope, digitalHumanID, ResourceCandidateQuery{
		Type:             resourcebindings.BindingType(strings.TrimSpace(c.Query("type"))),
		ParentResourceID: c.Query("parent_resource_id"),
		Search:           c.Query("query"),
		Limit:            limit,
	})
	if err != nil {
		h.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// candidateLimit parses the shared optional "limit" query parameter. An absent
// value keeps the service default; a present non-integer value is rejected the
// same way on every candidate surface. Range clamping stays in the service so
// the per-type maximum has one owner.
func candidateLimit(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// ListSkillCandidates returns the skills a new digital human may bind, so the
// creation flow offers the same choices the draft editor offers.
func (h *RuntimeHandler) ListSkillCandidates(c *gin.Context) {
	scope, ok := h.base.scope(c)
	if !ok {
		return
	}
	result, err := h.service.ListSkillCandidates(c.Request.Context(), scope)
	if err != nil {
		h.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

// ListConnectorCandidates returns the owner Connector connections a new digital
// human may attach, so the creation flow offers the same choices the draft
// editor offers without needing a draft id. It accepts the shared query/limit
// parameters, but the Connector binding type is forced server-side and a
// caller-supplied "type" is ignored.
func (h *RuntimeHandler) ListConnectorCandidates(c *gin.Context) {
	scope, ok := h.base.scope(c)
	if !ok {
		return
	}
	limit, ok := candidateLimit(c)
	if !ok {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.bindings.candidates"))
		return
	}
	result, err := h.service.ListConnectorCandidates(c.Request.Context(), scope, ResourceCandidateQuery{
		Type:             resourcebindings.BindingTypeConnector,
		ParentResourceID: c.Query("parent_resource_id"),
		Search:           c.Query("query"),
		Limit:            limit,
	})
	if err != nil {
		h.base.writeError(c, err)
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *RuntimeHandler) SubmitManual(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	var request ManualInvocationRequest
	if !h.base.decode(c, &request) {
		return
	}
	run, created, err := h.service.SubmitManual(c.Request.Context(), scope, digitalHumanID, request)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.submit"))
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeDigitalHumanData(c, status, gin.H{"run": publicRun(run), "created": created})
}

func (h *RuntimeHandler) ListRuns(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	query, err := parseListQuery(c)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.list"))
		return
	}
	result, err := h.service.ListRuns(c.Request.Context(), scope, digitalHumanID, query)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.list"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, publicRunList(result))
}

func (h *RuntimeHandler) GetRun(c *gin.Context) {
	scope, digitalHumanID, runID, ok := h.scopeAndRunID(c)
	if !ok {
		return
	}
	run, err := h.service.GetRun(c.Request.Context(), scope, digitalHumanID, runID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.get"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, publicRun(run))
}

func (h *RuntimeHandler) GetRunRuntimeLog(c *gin.Context) {
	scope, digitalHumanID, runID, ok := h.scopeAndRunID(c)
	if !ok {
		return
	}
	result, err := h.service.GetRunRuntimeLog(c.Request.Context(), scope, digitalHumanID, runID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.runtime_log"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *RuntimeHandler) CancelRun(c *gin.Context) {
	scope, digitalHumanID, runID, ok := h.scopeAndRunID(c)
	if !ok {
		return
	}
	run, err := h.service.CancelRun(c.Request.Context(), scope, digitalHumanID, runID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.run.cancel"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, RunCancelResult{Run: publicRun(run)})
}

func (h *RuntimeHandler) scopeAndRunID(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	runID, err := uuid.Parse(strings.TrimSpace(c.Param("run_id")))
	if err != nil || runID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.run.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, digitalHumanID, runID, true
}
