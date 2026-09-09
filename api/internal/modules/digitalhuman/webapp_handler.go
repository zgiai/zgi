package digitalhuman

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
)

const (
	webAppAccessContext   = "digital_human_webapp_access"
	webAppIdentityContext = "digital_human_webapp_identity"
)

type webAppRequestIdentity struct {
	ID            uuid.UUID
	Authenticated bool
}

// RegisterWebAppRoutes exposes published DigitalHumans to the hosted landing
// page. Anonymous identities must use server-signed, webapp-bound sessions.
func (h *ExternalHandler) RegisterWebAppRoutes(router *gin.RouterGroup) {
	public := router.Group("/v1/digital-human-webapps/:web_app_id")
	public.GET("/config", h.GetWebAppConfig)
	public.POST("/session", h.CreateWebAppSession)

	protected := router.Group("/v1/digital-human-webapps/:web_app_id")
	protected.Use(h.authenticateWebApp)
	protected.POST("/chat", h.ChatWebApp)
	protected.GET("/runs/:run_id", h.GetWebAppRun)
	protected.GET("/runs/:run_id/events", h.StreamWebAppRunEvents)
	protected.POST("/runs/:run_id/stop", h.StopWebAppRun)
	protected.POST("/runs/:run_id/runtime-input", h.SubmitWebAppRunRuntimeInput)
	protected.GET("/conversations", h.ListWebAppConversations)
	protected.GET("/search", h.SearchWebAppConversations)
	protected.GET("/conversations/search", h.SearchWebAppConversations)
	protected.GET("/conversations/:conversation_id", h.GetWebAppConversation)
	protected.PATCH("/conversations/:conversation_id", h.UpdateWebAppConversation)
	protected.DELETE("/conversations/:conversation_id", h.DeleteWebAppConversation)
	protected.GET("/conversations/:conversation_id/messages", h.ListWebAppConversationMessages)
	protected.GET("/conversations/:conversation_id/events", h.StreamWebAppConversationEvents)
	protected.POST("/conversations/:conversation_id/stop", h.StopWebAppConversation)
	protected.POST("/conversations/:conversation_id/messages/:message_id/runtime-inputs", h.SubmitWebAppConversationRuntimeInput)
	protected.GET("/files/upload", h.GetWebAppUploadConfig)
	protected.POST("/files/upload", h.UploadWebAppFile)
}

func (h *ExternalHandler) GetWebAppConfig(c *gin.Context) {
	_, access, ok := h.resolveWebApp(c)
	if !ok {
		return
	}
	policy := presentationPolicy(access.Version, access.Definition)
	writeExternalData(c, http.StatusOK, WebAppConfigResponse{
		DigitalHumanID: access.Definition.ID.String(), WebAppID: access.Definition.WebAppID.String(),
		Name: access.Definition.Name, Description: access.Definition.Description,
		Icon: access.Definition.Icon, IconType: access.Definition.IconType, IconURL: access.Definition.IconURL,
		HomeTitle: policy.HomeTitle, OpeningStatement: policy.OpeningStatement,
		InputPlaceholder: policy.InputPlaceholder, ThemeColor: policy.ThemeColor,
		SuggestedQuestions:   nonNilStrings(policy.SuggestedQuestions),
		AnonymousChatEnabled: true, LoginRequired: false,
		FileUploadEnabled: true, FileUploadRequiresLogin: true,
		RelationshipMemoryEnabled: true, RuntimeInputEnabled: true,
		Version: access.Version.VersionNumber, VersionUUID: access.Version.ID.String(),
	})
}

func (h *ExternalHandler) CreateWebAppSession(c *gin.Context) {
	webAppID, _, ok := h.resolveWebApp(c)
	if !ok {
		return
	}
	if h.sessions == nil {
		h.writeInternalError(c)
		return
	}
	token, session, err := h.sessions.Issue(webAppID)
	if err != nil {
		h.writeInternalError(c)
		return
	}
	writeExternalData(c, http.StatusCreated, WebAppSessionResponse{
		SessionToken: token, ExpiresAt: session.ExpiresAt.Unix(),
		AnonymousEnabled: true, LoginRequired: false,
	})
}

func (h *ExternalHandler) authenticateWebApp(c *gin.Context) {
	webAppID, access, ok := h.resolveWebApp(c)
	if !ok {
		c.Abort()
		return
	}
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		h.writeAuthError(c, false)
		c.Abort()
		return
	}
	token := strings.TrimSpace(parts[1])
	identity := webAppRequestIdentity{}
	if h.sessions != nil {
		if session, err := h.sessions.Parse(token, webAppID); err == nil {
			identity.ID = session.SubjectID
		}
	}
	if identity.ID == uuid.Nil {
		accountID, err := jwtpkg.GetUserIDFromToken(token)
		if err != nil {
			h.writeAuthError(c, false)
			c.Abort()
			return
		}
		identity.ID, err = uuid.Parse(strings.TrimSpace(accountID))
		if err != nil || identity.ID == uuid.Nil {
			h.writeAuthError(c, false)
			c.Abort()
			return
		}
		identity.Authenticated = true
	}
	c.Set(webAppAccessContext, access)
	c.Set(webAppIdentityContext, identity)
	c.Next()
}

func (h *ExternalHandler) ChatWebApp(c *gin.Context) {
	scope, access, _, ok := h.webAppScope(c)
	if !ok {
		return
	}
	var request WebAppChatRequest
	if !h.base.decode(c, &request) {
		return
	}
	responseMode := strings.TrimSpace(request.ResponseMode)
	if responseMode != "" && responseMode != "streaming" {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.chat.response_mode")
		return
	}
	run, created, err := h.runtime.SubmitWebApp(c.Request.Context(), scope, access.Definition.ID, request.ManualInvocationRequest)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.chat")
		return
	}
	c.Header("X-ZGI-Run-ID", run.ID.String())
	c.Header("Access-Control-Expose-Headers", "X-ZGI-Run-ID")
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	_ = writer.write(externalRunEventID(run.ID, "accepted"), "run_accepted", gin.H{
		"run_id": run.ID.String(), "request_id": request.RequestID.String(), "status": run.Status, "created": created,
	})
	h.streamRun(c, writer, scope, access.Definition.ID, run.ID, SurfaceWebApp, "")
}

func (h *ExternalHandler) GetWebAppRun(c *gin.Context) {
	scope, access, _, runID, ok := h.webAppRunRequest(c)
	if !ok {
		return
	}
	run, err := h.ownedRunForSurface(c, scope, access.Definition.ID, runID, SurfaceWebApp)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.run.get")
		return
	}
	writeExternalData(c, http.StatusOK, externalRunResponse(run))
}

func (h *ExternalHandler) StreamWebAppRunEvents(c *gin.Context) {
	scope, access, _, runID, ok := h.webAppRunRequest(c)
	if !ok {
		return
	}
	if _, err := h.ownedRunForSurface(c, scope, access.Definition.ID, runID, SurfaceWebApp); err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.run.events")
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
	h.streamRun(c, writer, scope, access.Definition.ID, runID, SurfaceWebApp, afterID)
}

func (h *ExternalHandler) StopWebAppRun(c *gin.Context) {
	scope, access, _, runID, ok := h.webAppRunRequest(c)
	if !ok {
		return
	}
	if _, err := h.ownedRunForSurface(c, scope, access.Definition.ID, runID, SurfaceWebApp); err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.run.stop")
		return
	}
	run, err := h.runtime.CancelRun(c.Request.Context(), scope, access.Definition.ID, runID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.run.stop")
		return
	}
	writeExternalData(c, http.StatusOK, externalRunResponse(run))
}

func (h *ExternalHandler) SubmitWebAppRunRuntimeInput(c *gin.Context) {
	scope, access, _, runID, ok := h.webAppRunRequest(c)
	if !ok {
		return
	}
	var request WebAppRuntimeInputRequest
	if !h.base.decode(c, &request) {
		return
	}
	run, err := h.ownedRunForSurface(c, scope, access.Definition.ID, runID, SurfaceWebApp)
	if err != nil || run.ConversationID == nil || run.MessageID == nil {
		if err == nil {
			err = ErrRunConflict
		}
		h.writeApplicationError(c, err, "digital_human.webapp.run.runtime_input")
		return
	}
	clientInputID, err := uuid.Parse(strings.TrimSpace(request.ClientInputID))
	if err != nil || clientInputID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.run.runtime_input")
		return
	}
	result, err := h.runtime.SubmitRuntimeInput(c.Request.Context(), scope, access.Definition.ID, runtimeservice.SubmitRuntimeInputRequest{
		ConversationID: *run.ConversationID, TargetMessageID: *run.MessageID,
		ClientInputID: clientInputID, Content: request.Content,
	})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.run.runtime_input")
		return
	}
	writeExternalData(c, http.StatusOK, runtimedto.SubmitRuntimeInputResponse{
		Input: digitalHumanRuntimeInputResponse(result.Input), Disposition: string(result.Disposition), Created: result.Created,
	})
}

func (h *ExternalHandler) ListWebAppConversations(c *gin.Context) {
	scope, access, _, ok := h.webAppScope(c)
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, maxConversationPageSize)
	items, total, err := h.runtime.ListConversations(c.Request.Context(), scope, access.Definition.ID, page, limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.list")
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

func (h *ExternalHandler) SearchWebAppConversations(c *gin.Context) {
	scope, access, _, ok := h.webAppScope(c)
	if !ok {
		return
	}
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, 50)
	items, err := h.runtime.SearchConversations(c.Request.Context(), scope, access.Definition.ID, c.Query("query"), limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.search")
		return
	}
	responses := make([]runtimedto.SearchResultResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalHumanSearchResponse(item))
	}
	writeExternalData(c, http.StatusOK, responses)
}

func (h *ExternalHandler) GetWebAppConversation(c *gin.Context) {
	h.withWebAppConversation(c, "get", func(scope Scope, access *PublishedAccess, id uuid.UUID) (any, error) {
		conversation, err := h.runtime.GetConversation(c.Request.Context(), scope, access.Definition.ID, id)
		if err != nil {
			return nil, err
		}
		return digitalHumanConversationResponse(conversation), nil
	})
}

func (h *ExternalHandler) UpdateWebAppConversation(c *gin.Context) {
	scope, access, id, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	var request WebAppConversationUpdateRequest
	if !h.base.decode(c, &request) || request.Title == nil {
		if request.Title == nil && !c.IsAborted() {
			h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.conversation.update")
		}
		return
	}
	conversation, err := h.runtime.UpdateConversation(c.Request.Context(), scope, access.Definition.ID, id, runtimedto.UpdateConversationRequest{Title: request.Title})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.update")
		return
	}
	writeExternalData(c, http.StatusOK, digitalHumanConversationResponse(conversation))
}

func (h *ExternalHandler) DeleteWebAppConversation(c *gin.Context) {
	h.withWebAppConversation(c, "delete", func(scope Scope, access *PublishedAccess, id uuid.UUID) (any, error) {
		if err := h.runtime.DeleteConversation(c.Request.Context(), scope, access.Definition.ID, id); err != nil {
			return nil, err
		}
		return gin.H{"result": "success"}, nil
	})
}

func (h *ExternalHandler) ListWebAppConversationMessages(c *gin.Context) {
	scope, access, id, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultMessagePageSize, maxMessagePageSize)
	items, total, err := h.runtime.ListConversationMessages(c.Request.Context(), scope, access.Definition.ID, id, page, limit)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.messages")
		return
	}
	inputs, err := h.runtime.ListConversationRuntimeInputs(c.Request.Context(), scope, access.Definition.ID, id)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.runtime_inputs")
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

// StreamWebAppConversationEvents is the shared ChatRuntime UI compatibility
// surface. The public API remains run-oriented, while the first-party landing
// page can reuse the same recovery state machine as other ZGI chat surfaces.
func (h *ExternalHandler) StreamWebAppConversationEvents(c *gin.Context) {
	scope, access, conversationID, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Query("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.conversation.events.message")
		return
	}
	afterID := strings.TrimSpace(c.Query("after_id"))
	if afterID == "" {
		afterID = strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	err = h.runtime.StreamConversationEvents(
		c.Request.Context(), scope, access.Definition.ID, conversationID, messageID, afterID,
		func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		},
	)
	if err != nil && c.Request.Context().Err() == nil {
		_ = h.writeStreamError(writer, uuid.Nil, err)
	}
}

func (h *ExternalHandler) StopWebAppConversation(c *gin.Context) {
	h.withWebAppConversation(c, "stop", func(scope Scope, access *PublishedAccess, conversationID uuid.UUID) (any, error) {
		result, err := h.runtime.StopConversation(c.Request.Context(), scope, access.Definition.ID, conversationID)
		if err != nil {
			return nil, err
		}
		return digitalHumanStopConversationResponse(result), nil
	})
}

func (h *ExternalHandler) SubmitWebAppConversationRuntimeInput(c *gin.Context) {
	scope, access, conversationID, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.conversation.runtime_input.message")
		return
	}
	var request runtimedto.SubmitRuntimeInputRequest
	if !h.base.decode(c, &request) {
		return
	}
	clientInputID, err := uuid.Parse(strings.TrimSpace(request.ClientInputID))
	if err != nil || clientInputID == uuid.Nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.conversation.runtime_input.client_input")
		return
	}
	result, err := h.runtime.SubmitRuntimeInput(c.Request.Context(), scope, access.Definition.ID, runtimeservice.SubmitRuntimeInputRequest{
		ConversationID:  conversationID,
		TargetMessageID: messageID,
		ClientInputID:   clientInputID,
		Content:         request.Content,
	})
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation.runtime_input")
		return
	}
	writeExternalData(c, http.StatusOK, runtimedto.SubmitRuntimeInputResponse{
		Input:       digitalHumanRuntimeInputResponse(result.Input),
		Disposition: string(result.Disposition),
		Created:     result.Created,
	})
}

func (h *ExternalHandler) GetWebAppUploadConfig(c *gin.Context) {
	_, _, identity, ok := h.webAppScope(c)
	if !ok {
		return
	}
	if !identity.Authenticated {
		h.writeAuthError(c, true)
		return
	}
	writeExternalData(c, http.StatusOK, h.fileService.GetUploadConfig())
}

func (h *ExternalHandler) UploadWebAppFile(c *gin.Context) {
	scope, access, identity, ok := h.webAppScope(c)
	if !ok {
		return
	}
	if !identity.Authenticated {
		h.writeAuthError(c, false)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxExternalUploadBytes)
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		h.writeApplicationError(c, ErrInvalidRequest, "digital_human.webapp.file.request")
		return
	}
	reader, err := file.Open()
	if err != nil {
		h.writeInternalError(c)
		return
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		h.writeInternalError(c)
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
		scope.ActorID.String(), access.Definition.OrganizationID.String(), filemodel.CreatedByRoleAccount,
		&source, &workspaceID, true, false,
	)
	if err != nil {
		switch {
		case errors.Is(err, filemodel.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, ExternalErrorResponse{Code: "file.too_large", Message: "The file is too large."})
		case errors.Is(err, filemodel.ErrUnsupportedFileType):
			c.JSON(http.StatusUnsupportedMediaType, ExternalErrorResponse{Code: "file.type.unsupported", Message: "The file type is not supported."})
		default:
			h.writeInternalError(c)
		}
		return
	}
	writeExternalData(c, http.StatusCreated, ExternalFileResponse{
		ID: uploaded.ID, Name: uploaded.Name, Size: uploaded.Size,
		Extension: strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), "."),
		MimeType:  uploaded.MimeType, CreatedAt: uploaded.CreatedAt.Unix(),
	})
}

func (h *ExternalHandler) resolveWebApp(c *gin.Context) (uuid.UUID, *PublishedAccess, bool) {
	webAppID, err := uuid.Parse(strings.TrimSpace(c.Param("web_app_id")))
	if err != nil || webAppID == uuid.Nil {
		h.writeApplicationError(c, ErrNotFound, "digital_human.webapp.resolve")
		return uuid.Nil, nil, false
	}
	access, err := h.access.ResolveWebApp(c.Request.Context(), webAppID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.resolve")
		return uuid.Nil, nil, false
	}
	return webAppID, access, true
}

func (h *ExternalHandler) webAppScope(c *gin.Context) (Scope, *PublishedAccess, webAppRequestIdentity, bool) {
	accessValue, accessOK := c.Get(webAppAccessContext)
	identityValue, identityOK := c.Get(webAppIdentityContext)
	access, accessTypeOK := accessValue.(*PublishedAccess)
	identity, identityTypeOK := identityValue.(webAppRequestIdentity)
	if !accessOK || !identityOK || !accessTypeOK || !identityTypeOK || access == nil || access.Definition == nil || identity.ID == uuid.Nil {
		h.writeAuthError(c, false)
		return Scope{}, nil, webAppRequestIdentity{}, false
	}
	webAppID := access.Definition.WebAppID
	scope := Scope{
		OrganizationID: access.Definition.OrganizationID, WorkspaceID: access.Definition.WorkspaceID,
		ActorID: identity.ID, PrincipalType: string(invocation.PrincipalExternalUser), PrincipalID: identity.ID,
		WebAppID: &webAppID, Surface: SurfaceWebApp,
	}
	if identity.Authenticated {
		subject := relationshipmemory.AccountSubject(identity.ID)
		subjectID := subject.ID
		scope.PrincipalType = string(invocation.PrincipalAccount)
		scope.UserSubjectKind = string(subject.Kind)
		scope.UserSubjectID = &subjectID
	}
	return scope, access, identity, true
}

func (h *ExternalHandler) webAppRunRequest(c *gin.Context) (Scope, *PublishedAccess, webAppRequestIdentity, uuid.UUID, bool) {
	scope, access, identity, ok := h.webAppScope(c)
	if !ok {
		return Scope{}, nil, webAppRequestIdentity{}, uuid.Nil, false
	}
	runID, ok := h.runID(c)
	return scope, access, identity, runID, ok
}

func (h *ExternalHandler) webAppConversationRequest(c *gin.Context) (Scope, *PublishedAccess, uuid.UUID, bool) {
	scope, access, _, ok := h.webAppScope(c)
	if !ok {
		return Scope{}, nil, uuid.Nil, false
	}
	id, ok := h.conversationID(c)
	return scope, access, id, ok
}

func (h *ExternalHandler) withWebAppConversation(c *gin.Context, operation string, action func(Scope, *PublishedAccess, uuid.UUID) (any, error)) {
	scope, access, id, ok := h.webAppConversationRequest(c)
	if !ok {
		return
	}
	result, err := action(scope, access, id)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.webapp.conversation."+operation)
		return
	}
	writeExternalData(c, http.StatusOK, result)
}

type WebAppConfigResponse struct {
	DigitalHumanID            string   `json:"digital_human_id"`
	WebAppID                  string   `json:"web_app_id"`
	Name                      string   `json:"name"`
	Description               string   `json:"description"`
	Icon                      string   `json:"icon"`
	IconType                  string   `json:"icon_type"`
	IconURL                   string   `json:"icon_url"`
	HomeTitle                 string   `json:"home_title"`
	OpeningStatement          string   `json:"opening_statement"`
	InputPlaceholder          string   `json:"input_placeholder"`
	ThemeColor                string   `json:"theme_color"`
	SuggestedQuestions        []string `json:"suggested_questions"`
	AnonymousChatEnabled      bool     `json:"anonymous_chat_enabled"`
	LoginRequired             bool     `json:"login_required"`
	FileUploadEnabled         bool     `json:"file_upload_enabled"`
	FileUploadRequiresLogin   bool     `json:"file_upload_requires_login"`
	RelationshipMemoryEnabled bool     `json:"relationship_memory_enabled"`
	RuntimeInputEnabled       bool     `json:"runtime_input_enabled"`
	Version                   int      `json:"version"`
	VersionUUID               string   `json:"version_uuid"`
}

type WebAppSessionResponse struct {
	SessionToken     string `json:"session_token,omitempty"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
	AnonymousEnabled bool   `json:"anonymous_enabled"`
	LoginRequired    bool   `json:"login_required"`
}

type WebAppChatRequest struct {
	ManualInvocationRequest
	ResponseMode string `json:"response_mode,omitempty"`
}

type WebAppRuntimeInputRequest struct {
	ClientInputID string `json:"client_input_id"`
	Content       string `json:"content"`
}

type WebAppConversationUpdateRequest struct {
	Title *string `json:"title,omitempty"`
}
