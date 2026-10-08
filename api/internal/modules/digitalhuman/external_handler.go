package digitalhuman

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/externalapikey"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const (
	externalAPIKeyContext   = "digital_human_api_key"
	externalAccessContext   = "digital_human_published_access"
	externalRunPollInterval = 250 * time.Millisecond
	maxExternalUploadBytes  = chatfile.MaxRequestBytes
	externalRunEventPrefix  = "run:"
)

// ExternalHandler exposes DigitalHuman's published, durable runtime without
// sharing the legacy Agent API-key identity context.
type ExternalHandler struct {
	runtime     *RuntimeService
	access      *AccessService
	tasks       TaskHandlerService
	fileService interfaces.FileService
	base        *Handler
	sessions    *WebAppSessionSigner
	pollEvery   time.Duration
}

func NewExternalHandler(runtime *RuntimeService, access *AccessService, tasks TaskHandlerService, fileService interfaces.FileService, projector *apptransport.Projector, sessions *WebAppSessionSigner) *ExternalHandler {
	if runtime == nil || access == nil || tasks == nil || fileService == nil || projector == nil {
		panic("digital human external handler requires runtime, access, tasks, file service, and projector")
	}
	return &ExternalHandler{
		runtime: runtime, access: access, tasks: tasks, fileService: fileService,
		base: &Handler{projector: projector}, sessions: sessions, pollEvery: externalRunPollInterval,
	}
}

// RegisterAPIRoutes registers the DigitalHuman-specific Bearer-authenticated
// contract before the legacy /api/v1 Agent middleware is installed.
func (h *ExternalHandler) RegisterAPIRoutes(router *gin.RouterGroup) {
	group := router.Group("/v1/digital-humans")
	group.Use(h.authenticateAPIKey)
	group.GET("/config", h.GetConfig)
	group.POST("/chat", h.Chat)
	group.GET("/runs/:run_id", h.GetRun)
	group.GET("/runs/:run_id/events", h.StreamRunEvents)
	group.POST("/runs/:run_id/stop", h.StopRun)
	group.POST("/runs/:run_id/runtime-input", h.SubmitRunRuntimeInput)
	group.GET("/conversations", h.ListConversations)
	group.GET("/conversations/search", h.SearchConversations)
	group.GET("/conversations/:conversation_id", h.GetConversation)
	group.PATCH("/conversations/:conversation_id", h.UpdateConversation)
	group.DELETE("/conversations/:conversation_id", h.DeleteConversation)
	group.GET("/conversations/:conversation_id/messages", h.ListConversationMessages)
	group.GET("/conversations/:conversation_id/files/:file_id", h.GetAPIExecutionFile)
	group.POST("/conversations/:conversation_id/messages/:message_id/regenerate", h.RegenerateAPIMessage)
	group.POST("/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue", h.ContinueAPIUserInput)
	group.POST("/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue", h.ContinueAPIToolGovernance)
	group.POST("/conversations/:conversation_id/messages/:message_id/token-budget/:request_id/continue", h.ContinueAPITaskTokenBudget)
	group.POST("/conversations/:conversation_id/messages/:message_id/workflow-continuation", h.ContinueAPITaskApproval)
	group.POST("/files/upload", h.UploadFile)
	group.GET("/tasks", h.ListAPITasks)
	group.POST("/tasks", h.CreateAPITask)
	group.GET("/tasks/:task_id", h.GetAPITask)
	group.PATCH("/tasks/:task_id", h.UpdateAPITask)
	group.POST("/tasks/:task_id/actions/:action", h.ApplyAPITaskAction)
	group.GET("/tasks/:task_id/runs", h.ListAPITaskRuns)
	group.GET("/tasks/:task_id/activities", h.ListAPITaskActivities)
}

func (h *ExternalHandler) authenticateAPIKey(c *gin.Context) {
	raw, err := externalapikey.Extract(c.GetHeader("Authorization"), c.GetHeader("X-API-Key"))
	if err != nil {
		h.writeAuthError(c, false)
		c.Abort()
		return
	}
	key, access, err := h.access.ResolveAPIKey(c.Request.Context(), raw)
	if err != nil {
		h.writeAuthError(c, errors.Is(err, ErrPublishInvalid))
		c.Abort()
		return
	}
	c.Set(externalAPIKeyContext, key)
	c.Set(externalAccessContext, access)
	c.Next()
}

func (h *ExternalHandler) GetConfig(c *gin.Context) {
	_, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	policy := presentationPolicy(access.Version, access.Definition)
	writeExternalData(c, http.StatusOK, ExternalConfigResponse{
		FileUploadPolicy: chatfile.Clone(access.Version.FileUploadPolicy),
		DigitalHumanID:   access.Definition.ID.String(),
		WebAppID:         access.Definition.WebAppID.String(),
		Name:             access.Definition.Name, Description: access.Definition.Description,
		Icon: access.Definition.Icon, IconType: access.Definition.IconType, IconURL: access.Definition.IconURL,
		HomeTitle: policy.HomeTitle, OpeningStatement: policy.OpeningStatement,
		InputPlaceholder:   policy.InputPlaceholder,
		SuggestedQuestions: nonNilStrings(policy.SuggestedQuestions),
		FileUploadEnabled:  true, RelationshipMemoryEnabled: true,
		MemoryRequiresUser: true, ExternalUserRequired: true,
		RuntimeInputEnabled: true, RegenerateEnabled: true,
		LandingPageEnabled: access.Settings.WebAppEnabled, APIEnabled: access.Settings.APIEnabled,
		Version: strconv.Itoa(access.Version.VersionNumber), VersionUUID: access.Version.ID.String(),
	})
}

func (h *ExternalHandler) Chat(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalChatRequest
	if !h.base.decode(c, &request) {
		return
	}
	responseMode := strings.TrimSpace(request.ResponseMode)
	if responseMode != "" && responseMode != "streaming" {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.chat.response_mode")
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	if err := h.access.ValidateExternalFiles(c.Request.Context(), scope, access.Definition.ID, request.FileIDs, SurfaceAPI); err != nil {
		h.writeApplicationError(c, err, "digital_human.external.chat.files")
		return
	}
	run, created, err := h.runtime.SubmitAPI(c.Request.Context(), scope, access.Definition.ID, request.ManualInvocationRequest)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.chat")
		return
	}
	c.Header("X-ZGI-Run-ID", run.ID.String())
	c.Header("Access-Control-Expose-Headers", "X-ZGI-Run-ID")
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_ = writer.write(externalRunEventID(run.ID, "accepted"), "run_accepted", gin.H{
		"run_id": run.ID.String(), "request_id": request.RequestID.String(),
		"status": run.Status, "created": created,
	})
	h.streamRun(c, writer, scope, access.Definition.ID, run.ID, SurfaceAPI, "")
}

func (h *ExternalHandler) GetRun(c *gin.Context) {
	scope, access, runID, ok := h.apiRunRequest(c, userFromQuery(c))
	if !ok {
		return
	}
	run, err := h.ownedRun(c, scope, access.Definition.ID, runID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.run.get")
		return
	}
	writeExternalData(c, http.StatusOK, externalRunResponse(run))
}

func (h *ExternalHandler) StreamRunEvents(c *gin.Context) {
	scope, access, runID, ok := h.apiRunRequest(c, userFromQuery(c))
	if !ok {
		return
	}
	if _, err := h.ownedRun(c, scope, access.Definition.ID, runID); err != nil {
		h.writeApplicationError(c, err, "digital_human.external.run.events")
		return
	}
	afterID := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	if afterID == "" {
		afterID = strings.TrimSpace(c.Query("after_id"))
	}
	if strings.HasPrefix(afterID, externalRunEventPrefix) {
		afterID = ""
	}
	c.Header("X-ZGI-Run-ID", runID.String())
	c.Header("Access-Control-Expose-Headers", "X-ZGI-Run-ID")
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	h.streamRun(c, writer, scope, access.Definition.ID, runID, SurfaceAPI, afterID)
}

func (h *ExternalHandler) StopRun(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalUserRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	runID, ok := h.runID(c)
	if !ok {
		return
	}
	if _, err := h.ownedRun(c, scope, access.Definition.ID, runID); err != nil {
		h.writeApplicationError(c, err, "digital_human.external.run.stop")
		return
	}
	run, err := h.runtime.CancelRun(c.Request.Context(), scope, access.Definition.ID, runID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.run.stop")
		return
	}
	writeExternalData(c, http.StatusOK, externalRunResponse(run))
}

func (h *ExternalHandler) SubmitRunRuntimeInput(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalRuntimeInputRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	runID, ok := h.runID(c)
	if !ok {
		return
	}
	run, err := h.ownedRun(c, scope, access.Definition.ID, runID)
	if err != nil || run.ConversationID == nil || run.MessageID == nil {
		if err == nil {
			err = ErrRunConflict
		}
		h.writeApplicationError(c, err, "digital_human.external.run.runtime_input")
		return
	}
	clientInputID, err := uuid.Parse(strings.TrimSpace(request.ClientInputID))
	if err != nil || clientInputID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.run.runtime_input")
		return
	}
	result, err := h.runtime.SubmitRuntimeInput(c.Request.Context(), scope, access.Definition.ID, runtimeservice.SubmitRuntimeInputRequest{
		ConversationID: *run.ConversationID, TargetMessageID: *run.MessageID,
		ClientInputID: clientInputID, Content: request.Content,
	})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.run.runtime_input")
		return
	}
	writeExternalData(c, http.StatusOK, runtimedto.SubmitRuntimeInputResponse{
		Input: digitalHumanRuntimeInputResponse(result.Input), Disposition: string(result.Disposition), Created: result.Created,
	})
}

func (h *ExternalHandler) ListConversations(c *gin.Context) {
	scope, access, ok := h.apiQueryScope(c)
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, maxConversationPageSize)
	items, total, err := h.runtime.ListConversations(c.Request.Context(), scope, access.Definition.ID, page, limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation.list")
		return
	}
	responses := make([]runtimedto.ConversationResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalHumanConversationResponse(item))
	}
	writeExternalData(c, http.StatusOK, runtimedto.ListResponse[runtimedto.ConversationResponse]{
		Data: responses, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total,
	})
}

func (h *ExternalHandler) SearchConversations(c *gin.Context) {
	scope, access, ok := h.apiQueryScope(c)
	if !ok {
		return
	}
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, 50)
	items, err := h.runtime.SearchConversations(c.Request.Context(), scope, access.Definition.ID, c.Query("query"), limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation.search")
		return
	}
	responses := make([]runtimedto.SearchResultResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalHumanSearchResponse(item))
	}
	writeExternalData(c, http.StatusOK, responses)
}

func (h *ExternalHandler) GetConversation(c *gin.Context) {
	h.withAPIConversation(c, userFromQuery(c), "get", func(scope Scope, access *PublishedAccess, id uuid.UUID) (any, error) {
		conversation, err := h.runtime.GetConversation(c.Request.Context(), scope, access.Definition.ID, id)
		if err != nil {
			return nil, err
		}
		return digitalHumanConversationResponse(conversation), nil
	})
}

func (h *ExternalHandler) UpdateConversation(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var request ExternalConversationUpdateRequest
	if !h.base.decode(c, &request) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, request.User)
	if !ok {
		return
	}
	id, ok := h.conversationID(c)
	if !ok {
		return
	}
	if request.Title == nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.conversation.update")
		return
	}
	conversation, err := h.runtime.UpdateConversation(c.Request.Context(), scope, access.Definition.ID, id, runtimedto.UpdateConversationRequest{Title: request.Title})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation.update")
		return
	}
	writeExternalData(c, http.StatusOK, digitalHumanConversationResponse(conversation))
}

func (h *ExternalHandler) DeleteConversation(c *gin.Context) {
	h.withAPIConversation(c, userFromQuery(c), "delete", func(scope Scope, access *PublishedAccess, id uuid.UUID) (any, error) {
		if err := h.runtime.DeleteConversation(c.Request.Context(), scope, access.Definition.ID, id); err != nil {
			return nil, err
		}
		return gin.H{"result": "success"}, nil
	})
}

func (h *ExternalHandler) ListConversationMessages(c *gin.Context) {
	scope, access, id, ok := h.apiConversationRequest(c, userFromQuery(c))
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultMessagePageSize, maxMessagePageSize)
	items, total, err := h.runtime.ListConversationMessages(c.Request.Context(), scope, access.Definition.ID, id, page, limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation.messages")
		return
	}
	inputs, err := h.runtime.ListConversationRuntimeInputs(c.Request.Context(), scope, access.Definition.ID, id)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation.runtime_inputs")
		return
	}
	inputsByMessage := digitalHumanRuntimeInputsByVisibleMessage(inputs)
	responses := make([]runtimedto.MessageResponse, 0, len(items))
	for _, item := range items {
		response := digitalHumanMessageResponse(item)
		response.RuntimeInputs = inputsByMessage[item.ID]
		if response.RuntimeInputs == nil {
			response.RuntimeInputs = []runtimedto.RuntimeInputResponse{}
		}
		responses = append(responses, response)
	}
	writeExternalData(c, http.StatusOK, runtimedto.ListResponse[runtimedto.MessageResponse]{
		Data: responses, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total,
	})
}

// RegenerateAPIMessage replays a root user turn through the same durable run
// boundary used by /chat. The API user remains the data owner; the API key is
// only the authenticated principal.
func (h *ExternalHandler) RegenerateAPIMessage(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	var raw struct {
		User string `json:"user"`
		runtimedto.RegenerateMessageRequest
	}
	if !h.base.decode(c, &raw) {
		return
	}
	scope, ok := h.apiUserScope(c, key, access, raw.User)
	if !ok {
		return
	}
	conversationID, ok := h.conversationID(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.message.regenerate")
		return
	}
	run, _, err := h.runtime.SubmitRegeneration(c.Request.Context(), scope, access.Definition.ID, RegenerateInvocationRequest{
		MessageID: messageID, ConversationID: &conversationID, Query: raw.Query, Surface: SurfaceAPI,
		RuntimeContext: raw.RuntimeContext, OperationContext: raw.OperationContext,
		Model: raw.Model, Provider: raw.Provider, Parameters: raw.Parameters, UseMemory: raw.UseMemory,
	})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.message.regenerate")
		return
	}
	c.Header("X-ZGI-Run-ID", run.ID.String())
	c.Header("Access-Control-Expose-Headers", "X-ZGI-Run-ID")
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_ = writer.write(externalRunEventID(run.ID, "accepted"), "run_accepted", gin.H{
		"run_id": run.ID.String(), "status": run.Status,
	})
	h.streamRun(c, writer, scope, access.Definition.ID, run.ID, SurfaceAPI, "")
}

func (h *ExternalHandler) UploadFile(c *gin.Context) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxExternalUploadBytes)
	user := c.PostForm("user")
	scope, ok := h.apiUserScope(c, key, access, user)
	if !ok {
		return
	}
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.file.request")
		return
	}
	contentReader, err := file.Open()
	if err != nil {
		h.writeInternalError(c)
		return
	}
	defer contentReader.Close()
	policy := effectiveFileUploadPolicy(access.Version, h.fileService.GetUploadConfig())
	if policy.ValidateFile(chatfile.Extension(file.Filename), file.Size) != nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.file.policy")
		return
	}
	content, err := io.ReadAll(io.LimitReader(contentReader, chatfile.MaxTurnBytes+1))
	if err != nil {
		h.writeInternalError(c)
		return
	}
	if policy.ValidateFile(chatfile.Extension(file.Filename), int64(len(content))) != nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.file.policy")
		return
	}
	mimeType := strings.TrimSpace(file.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = http.DetectContentType(content)
	}
	source := interfaces.FileSourceWorkflow
	workspaceID := access.Definition.WorkspaceID.String()
	uploaded, err := h.fileService.UploadFile(
		c.Request.Context(), file.Filename, content, mimeType,
		scope.ActorID.String(), access.Definition.OrganizationID.String(), filemodel.CreatedByRoleEndUser,
		&source, &workspaceID, true, false,
	)
	if err != nil {
		switch {
		case errors.Is(err, filemodel.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, ExternalErrorResponse{Code: "file.too_large", Message: "The file is too large.", Retryable: false})
		case errors.Is(err, filemodel.ErrUnsupportedFileType):
			c.JSON(http.StatusUnsupportedMediaType, ExternalErrorResponse{Code: "file.type.unsupported", Message: "The file type is not supported.", Retryable: false})
		default:
			h.writeInternalError(c)
		}
		return
	}
	uploadedID, err := uuid.Parse(strings.TrimSpace(uploaded.ID))
	if err != nil || uploadedID == uuid.Nil || h.access.BindExternalFile(c.Request.Context(), scope, access.Definition.ID, uploadedID, SurfaceAPI) != nil {
		_ = h.fileService.DeleteFiles(c.Request.Context(), []string{uploaded.ID})
		h.writeInternalError(c)
		return
	}
	writeExternalData(c, http.StatusCreated, ExternalFileResponse{
		ID: uploaded.ID, Name: uploaded.Name, Size: uploaded.Size,
		Extension: strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), "."),
		MimeType:  uploaded.MimeType, CreatedAt: uploaded.CreatedAt.Unix(),
	})
}

func (h *ExternalHandler) streamRun(c *gin.Context, writer *digitalHumanSSEWriter, scope Scope, digitalHumanID, runID uuid.UUID, surface, afterID string) {
	run, err := h.waitForRunMessage(c, scope, digitalHumanID, runID, surface)
	if err != nil {
		_ = h.writeStreamError(writer, runID, err)
		_ = writer.write(externalRunEventID(runID, "end"), "run_end", gin.H{"run_id": runID.String(), "status": RunStatusFailed})
		return
	}
	if run.ConversationID != nil && run.MessageID != nil {
		err = h.runtime.StreamConversationEvents(c.Request.Context(), scope, digitalHumanID, *run.ConversationID, *run.MessageID, afterID, func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		})
		if err != nil {
			_ = h.writeStreamError(writer, runID, err)
		}
		run, err = h.waitForRunStreamBoundary(c, scope, digitalHumanID, runID, surface)
		if err != nil && c.Request.Context().Err() == nil {
			_ = h.writeStreamError(writer, runID, err)
		}
	}
	if run == nil {
		return
	}
	if run.Status == RunStatusFailed || run.Status == RunStatusUnknown {
		_ = h.writeStreamError(writer, runID, ErrRunConflict)
	}
	_ = writer.write(externalRunEventID(runID, "end"), "run_end", gin.H{
		"run_id": run.ID.String(), "status": run.Status,
		"conversation_id": uuidPtrString(run.ConversationID), "message_id": uuidPtrString(run.MessageID),
	})
}

func (h *ExternalHandler) waitForRunMessage(c *gin.Context, scope Scope, digitalHumanID, runID uuid.UUID, surface string) (*Run, error) {
	for {
		run, err := h.ownedRunForSurface(c, scope, digitalHumanID, runID, surface)
		if err != nil {
			return nil, err
		}
		if run.ConversationID != nil && run.MessageID != nil {
			return run, nil
		}
		if isTerminalRunStatus(run.Status) {
			return run, nil
		}
		if err := waitExternalPoll(c, h.pollEvery); err != nil {
			return nil, err
		}
	}
}

// A waiting run is resumable, not terminal. Its current message stream must
// nevertheless finish so clients can submit an approval or other continuation.
func (h *ExternalHandler) waitForRunStreamBoundary(c *gin.Context, scope Scope, digitalHumanID, runID uuid.UUID, surface string) (*Run, error) {
	for {
		run, err := h.ownedRunForSurface(c, scope, digitalHumanID, runID, surface)
		if err != nil || isTerminalRunStatus(run.Status) || run.Status == RunStatusWaiting {
			return run, err
		}
		if err := waitExternalPoll(c, h.pollEvery); err != nil {
			return run, err
		}
	}
}

func waitExternalPoll(c *gin.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	case <-timer.C:
		return nil
	}
}

func (h *ExternalHandler) writeStreamError(writer *digitalHumanSSEWriter, runID uuid.UUID, err error) error {
	projected := h.base.projector.Project(applicationError(err, "digital_human.external.stream"), "").Presentation
	return writer.write("", "error", ExternalStreamError{
		RunID: runID.String(), Code: projected.Code.String(), Message: projected.Message, Retryable: projected.Retryable,
	})
}

func (h *ExternalHandler) apiRunRequest(c *gin.Context, user string) (Scope, *PublishedAccess, uuid.UUID, bool) {
	scope, access, ok := h.apiScope(c, user)
	if !ok {
		return Scope{}, nil, uuid.Nil, false
	}
	runID, ok := h.runID(c)
	return scope, access, runID, ok
}

func (h *ExternalHandler) apiConversationRequest(c *gin.Context, user string) (Scope, *PublishedAccess, uuid.UUID, bool) {
	scope, access, ok := h.apiScope(c, user)
	if !ok {
		return Scope{}, nil, uuid.Nil, false
	}
	id, ok := h.conversationID(c)
	return scope, access, id, ok
}

func (h *ExternalHandler) apiQueryScope(c *gin.Context) (Scope, *PublishedAccess, bool) {
	return h.apiScope(c, userFromQuery(c))
}

func (h *ExternalHandler) apiScope(c *gin.Context, user string) (Scope, *PublishedAccess, bool) {
	key, access, ok := h.apiAccess(c)
	if !ok {
		return Scope{}, nil, false
	}
	scope, ok := h.apiUserScope(c, key, access, user)
	return scope, access, ok
}

func (h *ExternalHandler) apiUserScope(c *gin.Context, key *APIKey, access *PublishedAccess, user string) (Scope, bool) {
	if key == nil || access == nil || access.Definition == nil {
		h.writeAuthError(c, false)
		return Scope{}, false
	}
	subject, err := usersubject.DigitalHumanAPIEndUserSubject(access.Definition.WorkspaceID, access.Definition.ID, user)
	if err != nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.user")
		return Scope{}, false
	}
	subjectID := subject.ID
	return Scope{
		OrganizationID: access.Definition.OrganizationID, WorkspaceID: access.Definition.WorkspaceID,
		ActorID: subjectID, PrincipalType: string(invocation.PrincipalService), PrincipalID: key.ID,
		UserSubjectKind: string(subject.Kind), UserSubjectID: &subjectID, Surface: SurfaceAPI,
	}, true
}

func (h *ExternalHandler) apiAccess(c *gin.Context) (*APIKey, *PublishedAccess, bool) {
	keyValue, keyOK := c.Get(externalAPIKeyContext)
	accessValue, accessOK := c.Get(externalAccessContext)
	key, keyTypeOK := keyValue.(*APIKey)
	access, accessTypeOK := accessValue.(*PublishedAccess)
	if !keyOK || !accessOK || !keyTypeOK || !accessTypeOK || key == nil || access == nil || access.Definition == nil || access.Version == nil {
		h.writeAuthError(c, false)
		return nil, nil, false
	}
	return key, access, true
}

func (h *ExternalHandler) ownedRun(c *gin.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	return h.ownedRunForSurface(c, scope, digitalHumanID, runID, SurfaceAPI)
}

func (h *ExternalHandler) ownedRunForSurface(c *gin.Context, scope Scope, digitalHumanID, runID uuid.UUID, surface string) (*Run, error) {
	run, err := h.runtime.GetRun(c.Request.Context(), scope, digitalHumanID, runID)
	if err != nil {
		return nil, err
	}
	if !externalRunOwnedByScope(run, scope, surface) {
		return nil, ErrRunNotFound
	}
	return run, nil
}

func externalRunOwnedByScope(run *Run, scope Scope, surface string) bool {
	if run == nil || run.Surface != surface {
		return false
	}
	if scope.UserSubjectID != nil && *scope.UserSubjectID != uuid.Nil {
		return run.UserSubjectID != nil && *run.UserSubjectID == *scope.UserSubjectID && run.UserSubjectKind == scope.UserSubjectKind
	}
	return surface == SurfaceWebApp && run.UserSubjectID == nil && run.PrincipalID != nil &&
		*run.PrincipalID == scope.PrincipalID
}

func (h *ExternalHandler) withAPIConversation(c *gin.Context, user, operation string, action func(Scope, *PublishedAccess, uuid.UUID) (any, error)) {
	scope, access, id, ok := h.apiConversationRequest(c, user)
	if !ok {
		return
	}
	result, err := action(scope, access, id)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.external.conversation."+operation)
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

func (h *ExternalHandler) runID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("run_id")))
	if err != nil || id == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.run.id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ExternalHandler) conversationID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil || id == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.external.conversation.id")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ExternalHandler) writeApplicationError(c *gin.Context, err error, operation string) {
	h.base.writeError(c, applicationError(err, operation))
}

func (h *ExternalHandler) writeAuthError(c *gin.Context, forbidden bool) {
	code := appcatalog.CodeAuthenticationRequired
	if forbidden {
		code = appcatalog.CodePermissionDenied
	}
	h.base.writeError(c, apperror.New(code, apperror.WithOperation("digital_human.external.authenticate")))
}

func (h *ExternalHandler) writeInternalError(c *gin.Context) {
	h.base.writeError(c, errors.New("digital human external request failed"))
}

func writeExternalData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"code": "0", "message": "success", "data": data})
}

func userFromQuery(c *gin.Context) string { return c.Query("user") }

func externalRunEventID(runID uuid.UUID, phase string) string {
	return externalRunEventPrefix + runID.String() + ":" + phase
}

func isTerminalRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCanceled, RunStatusUnknown:
		return true
	default:
		return false
	}
}

func uuidPtrString(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func externalRunResponse(run *Run) ExternalRunResponse {
	if run == nil {
		return ExternalRunResponse{}
	}
	return ExternalRunResponse{
		RunID: run.ID.String(), Status: run.Status,
		ConversationID: uuidPtrString(run.ConversationID), MessageID: uuidPtrString(run.MessageID),
		ErrorCode: run.ErrorCode, ErrorMessage: safeExternalRunErrorMessage(run),
		CreatedAt: run.CreatedAt.Unix(), UpdatedAt: run.UpdatedAt.Unix(),
	}
}

func safeExternalRunErrorMessage(run *Run) string {
	if run == nil || (run.Status != RunStatusFailed && run.Status != RunStatusUnknown) {
		return ""
	}
	return "The DigitalHuman run could not be completed."
}

type ExternalConfigResponse struct {
	FileUploadPolicy          *chatfile.Policy `json:"file_upload_policy,omitempty"`
	DigitalHumanID            string           `json:"digital_human_id"`
	WebAppID                  string           `json:"web_app_id"`
	Name                      string           `json:"name"`
	Description               string           `json:"description"`
	Icon                      string           `json:"icon"`
	IconType                  string           `json:"icon_type"`
	IconURL                   string           `json:"icon_url"`
	HomeTitle                 string           `json:"home_title"`
	OpeningStatement          string           `json:"opening_statement"`
	InputPlaceholder          string           `json:"input_placeholder"`
	SuggestedQuestions        []string         `json:"suggested_questions"`
	FileUploadEnabled         bool             `json:"file_upload_enabled"`
	RelationshipMemoryEnabled bool             `json:"relationship_memory_enabled"`
	MemoryRequiresUser        bool             `json:"memory_requires_user"`
	ExternalUserRequired      bool             `json:"external_user_required"`
	RuntimeInputEnabled       bool             `json:"runtime_input_enabled"`
	RegenerateEnabled         bool             `json:"regenerate_enabled"`
	LandingPageEnabled        bool             `json:"landing_page_enabled"`
	APIEnabled                bool             `json:"api_enabled"`
	Version                   string           `json:"version"`
	VersionUUID               string           `json:"version_uuid"`
}

type ExternalChatRequest struct {
	ManualInvocationRequest
	User         string `json:"user"`
	ResponseMode string `json:"response_mode,omitempty"`
}

type ExternalUserRequest struct {
	User string `json:"user"`
}

type ExternalRuntimeInputRequest struct {
	User          string `json:"user"`
	ClientInputID string `json:"client_input_id"`
	Content       string `json:"content"`
}

type ExternalConversationUpdateRequest struct {
	User  string  `json:"user"`
	Title *string `json:"title,omitempty"`
}

type ExternalRunResponse struct {
	RunID          string    `json:"run_id"`
	Status         RunStatus `json:"status"`
	ConversationID string    `json:"conversation_id,omitempty"`
	MessageID      string    `json:"message_id,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      int64     `json:"created_at"`
	UpdatedAt      int64     `json:"updated_at"`
}

type ExternalFileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Extension string `json:"extension"`
	MimeType  string `json:"mime_type"`
	CreatedAt int64  `json:"created_at"`
}

type ExternalErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type ExternalStreamError struct {
	RunID     string `json:"run_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
