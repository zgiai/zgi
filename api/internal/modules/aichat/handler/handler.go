package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	aichatservice "github.com/zgiai/zgi/api/internal/modules/aichat/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	defaultConversationPageLimit = 20
	maxConversationPageLimit     = 100
	defaultMessagePageLimit      = 50
	maxMessagePageLimit          = 200
)

type Handler struct {
	service aichatservice.Service
}

func NewHandler(service aichatservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/aichat")
	group.GET("/skills", h.ListSkills)
	group.GET("/skills/config", h.GetSkillConfig)
	group.GET("/skills/:id", h.GetSkill)
	skillManagement := group.Group("/skills", middleware.EnterpriseAdminOrOwnerRequired())
	skillManagement.POST("/import", h.ImportSkill)
	skillManagement.POST("/import/preview", h.PreviewImportSkill)
	skillManagement.POST("/import/confirm", h.ConfirmImportSkill)
	skillManagement.PUT("/config", h.UpdateSkillConfig)
	skillManagement.DELETE("/:id", h.DeleteSkill)
	group.GET("/conversations", h.ListConversations)
	group.POST("/conversations", h.CreateConversation)
	group.GET("/conversations/:id", h.GetConversation)
	group.PATCH("/conversations/:id", h.UpdateConversation)
	group.DELETE("/conversations/:id", h.DeleteConversation)
	group.POST("/conversations/:id/stop", h.StopConversation)
	group.GET("/conversations/:id/events", h.StreamConversationEvents)
	group.GET("/conversations/:id/messages", h.ListMessages)
	group.DELETE("/messages/:id", h.DeleteMessage)
	group.POST("/messages/:id/stop", h.StopMessage)
	group.POST("/messages/:id/regenerate", h.RegenerateMessage)
	group.POST("/chat", h.Chat)
}

func (h *Handler) ListSkills(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	metadata, err := h.service.ListSkills(c.Request.Context(), scope)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]aichatdto.SkillResponse, 0, len(metadata))
	for _, item := range metadata {
		items = append(items, skillResponse(item))
	}
	response.Success(c, items)
}

func (h *Handler) GetSkill(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	skillID := strings.TrimSpace(c.Param("id"))
	if skillID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	metadata, err := h.service.GetSkill(c.Request.Context(), scope, skillID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillResponse(*metadata))
}

func (h *Handler) GetSkillConfig(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	config, err := h.service.GetSkillConfig(c.Request.Context(), scope)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillConfigResponse(config))
}

func (h *Handler) UpdateSkillConfig(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req aichatdto.UpdateSkillConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	config, err := h.service.UpdateSkillConfig(c.Request.Context(), scope, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillConfigResponse(config))
}

func (h *Handler) ImportSkill(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	metadata, err := h.service.ImportCustomSkill(c.Request.Context(), scope, fileHeader)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillResponse(*metadata))
}

func (h *Handler) PreviewImportSkill(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	preview, err := h.service.PreviewImportCustomSkill(c.Request.Context(), scope, fileHeader)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillImportPreviewResponse(preview))
}

func (h *Handler) ConfirmImportSkill(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req aichatdto.ConfirmImportSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	metadata, err := h.service.ConfirmCustomSkillImport(c.Request.Context(), scope, req.ImportID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillResponse(*metadata))
}

func (h *Handler) DeleteSkill(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	skillID := strings.TrimSpace(c.Param("id"))
	if skillID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := h.service.DeleteSkill(c.Request.Context(), scope, skillID); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *Handler) CreateConversation(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req aichatdto.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversation, err := h.service.CreateConversation(c.Request.Context(), scope, req.Title)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, conversationResponse(conversation))
}

func (h *Handler) ListConversations(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	page, limit := pagination(c, 1, defaultConversationPageLimit, maxConversationPageLimit)
	conversations, total, err := h.service.ListConversations(c.Request.Context(), scope, page, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]aichatdto.ConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, conversationResponse(conversation))
	}
	response.Success(c, aichatdto.ListResponse[aichatdto.ConversationResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

func (h *Handler) GetConversation(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	conversation, err := h.service.GetConversation(c.Request.Context(), scope, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, conversationResponse(conversation))
}

func (h *Handler) UpdateConversation(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	var req aichatdto.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversation, err := h.service.UpdateConversation(c.Request.Context(), scope, id, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, conversationResponse(conversation))
}

func (h *Handler) DeleteConversation(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteConversation(c.Request.Context(), scope, id); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *Handler) ListMessages(c *gin.Context) {
	scope, conversationID, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	page, limit := pagination(c, 1, defaultMessagePageLimit, maxMessagePageLimit)
	messages, total, err := h.service.ListMessages(c.Request.Context(), scope, conversationID, page, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]aichatdto.MessageResponse, 0, len(messages))
	for _, message := range messages {
		items = append(items, messageResponse(message))
	}
	response.Success(c, aichatdto.ListResponse[aichatdto.MessageResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

func (h *Handler) StreamConversationEvents(c *gin.Context) {
	scope, conversationID, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Query("message_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	setupSSE(c)
	err = h.service.StreamConversationEvents(c.Request.Context(), scope, conversationID, messageID, c.Query("after_id"), func(event aichatservice.StreamEvent) error {
		return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		logger.WarnContext(c.Request.Context(), "aichat event stream failed", "conversation_id", conversationID.String(), "message_id", messageID.String(), err)
		_ = writeSSEEvent(c, "", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"message":         err.Error(),
		})
	}
}

func (h *Handler) DeleteMessage(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteMessage(c.Request.Context(), scope, id); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

func (h *Handler) StopMessage(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	message, err := h.service.StopMessage(c.Request.Context(), scope, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, messageResponse(message))
}

func (h *Handler) StopConversation(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	result, err := h.service.StopConversation(c.Request.Context(), scope, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, stopConversationResponse(result))
}

func (h *Handler) RegenerateMessage(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	var req aichatdto.RegenerateMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	prepared, err := h.service.PrepareRootRegeneration(c.Request.Context(), scope, id, req)
	if err != nil {
		h.fail(c, err)
		return
	}

	setupSSE(c)
	writeChatStart(c, prepared)
	client := newSSEClientWriter(c)

	result, err := h.service.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return client.writeChunk(prepared, chunk)
	}, func(event aichatservice.StreamEvent) error {
		return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		if errors.Is(err, aichatservice.ErrMessageStopped) {
			writeChatStopped(c, prepared)
			return
		}
		writeChatError(c, prepared, err)
		return
	}
	writeChatEnd(c, prepared, result)
}

func (h *Handler) Chat(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req aichatdto.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	prepared, err := h.service.PrepareChat(c.Request.Context(), scope, req)
	if err != nil {
		h.fail(c, err)
		return
	}

	setupSSE(c)
	writeChatStart(c, prepared)
	client := newSSEClientWriter(c)

	result, err := h.service.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return client.writeChunk(prepared, chunk)
	}, func(event aichatservice.StreamEvent) error {
		return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		if errors.Is(err, aichatservice.ErrMessageStopped) {
			writeChatStopped(c, prepared)
			return
		}
		writeChatError(c, prepared, err)
		return
	}
	writeChatEnd(c, prepared, result)
}

func writeChatStart(c *gin.Context, prepared *aichatservice.PreparedChat) {
	_ = writeSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidStringPtr(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"replace":         prepared.ReplaceRoot,
		"created_at":      prepared.Message.CreatedAt.Unix(),
	})
}

func writeChatChunk(c *gin.Context, prepared *aichatservice.PreparedChat, chunk string) error {
	return writeSSE(c, "message", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          chunk,
	})
}

func writeChatError(c *gin.Context, prepared *aichatservice.PreparedChat, err error) {
	logger.WarnContext(c.Request.Context(), "aichat stream failed", "message_id", prepared.Message.ID.String(), err)
	_ = writeSSE(c, "error", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"message":         err.Error(),
	})
}

func writeChatEnd(c *gin.Context, prepared *aichatservice.PreparedChat, result *aichatservice.ChatResult) {
	_ = writeSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          aichatmodel.MessageStatusCompleted,
		"metadata": gin.H{
			"usage": result.Metadata["usage"],
		},
	})
}

func writeChatStopped(c *gin.Context, prepared *aichatservice.PreparedChat) {
	_ = writeSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          aichatmodel.MessageStatusStopped,
		"metadata": gin.H{
			"usage": gin.H{},
		},
	})
}

type sseClientWriter struct {
	c      *gin.Context
	closed bool
}

func newSSEClientWriter(c *gin.Context) *sseClientWriter {
	return &sseClientWriter{c: c}
}

func (w *sseClientWriter) writeChunk(prepared *aichatservice.PreparedChat, chunk string) error {
	if w.closed || w.c.Request.Context().Err() != nil {
		w.closed = true
		return nil
	}
	if err := writeChatChunk(w.c, prepared, chunk); err != nil {
		w.closed = true
		return nil
	}
	return nil
}

func (h *Handler) scope(c *gin.Context) (aichatservice.Scope, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return aichatservice.Scope{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return aichatservice.Scope{}, false
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err == nil {
			workspaceID = &parsed
		}
	}
	return aichatservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    workspaceID,
	}, true
}

func (h *Handler) scopedID(c *gin.Context, param string) (aichatservice.Scope, uuid.UUID, bool) {
	scope, ok := h.scope(c)
	if !ok {
		return aichatservice.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param(param)))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return aichatservice.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, aichatservice.ErrUnauthorized):
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, aichatservice.ErrPermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, aichatservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, aichatservice.ErrInvalidInput), errors.Is(err, aichatservice.ErrInvalidModelParam):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, aichatservice.ErrConversationRunning), errors.Is(err, aichatservice.ErrMessageReplaceNotAllowed):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		logger.ErrorContext(c.Request.Context(), "aichat request failed", err)
		response.Fail(c, response.ErrSystemError)
	}
}

func setupSSE(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Status(http.StatusOK)
	c.Writer.Flush()
}

func writeSSE(c *gin.Context, event string, data interface{}) error {
	return writeSSEEvent(c, "", event, data)
}

func writeSSEEvent(c *gin.Context, id string, event string, data interface{}) error {
	payload := gin.H{"event": event, "data": data}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) != "" {
		if _, err := fmt.Fprintf(c.Writer, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", encoded); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func pagination(c *gin.Context, defaultPage, defaultLimit, maxLimit int) (int, int) {
	page := parsePositiveInt(c.Query("page"), defaultPage)
	limit := parsePositiveInt(c.Query("limit"), defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func conversationResponse(conversation *aichatmodel.Conversation) aichatdto.ConversationResponse {
	resp := aichatdto.ConversationResponse{
		ID:             conversation.ID.String(),
		OrganizationID: conversation.OrganizationID.String(),
		AccountID:      conversation.AccountID.String(),
		Title:          conversation.Title,
		Status:         conversation.Status,
		RuntimeStatus:  conversation.RuntimeStatus,
		DialogueCount:  conversation.DialogueCount,
		Source:         conversation.Source,
		Metadata:       conversation.Metadata,
		CreatedAt:      conversation.CreatedAt.Unix(),
		UpdatedAt:      conversation.UpdatedAt.Unix(),
	}
	if resp.Metadata == nil {
		resp.Metadata = map[string]interface{}{}
	}
	if conversation.WorkspaceID != nil {
		resp.WorkspaceID = stringPtr(conversation.WorkspaceID.String())
	}
	if conversation.CurrentLeafMessageID != nil {
		resp.CurrentLeafMessageID = stringPtr(conversation.CurrentLeafMessageID.String())
	}
	if conversation.ActiveMessageID != nil {
		resp.ActiveMessageID = stringPtr(conversation.ActiveMessageID.String())
	}
	if conversation.SourceConversationID != nil {
		resp.SourceConversationID = stringPtr(conversation.SourceConversationID.String())
	}
	if conversation.SourceWebAppID != nil {
		resp.SourceWebAppID = stringPtr(conversation.SourceWebAppID.String())
	}
	return resp
}

func skillResponse(metadata skills.SkillDiscoveryMetadata) aichatdto.SkillResponse {
	return aichatdto.SkillResponse{
		SkillID:     metadata.ID,
		Source:      metadata.Source,
		Name:        metadata.Name,
		Description: metadata.Description,
		WhenToUse:   metadata.WhenToUse,
		Display: aichatdto.SkillDisplayResponse{
			Icon:        metadata.Display.Icon,
			Category:    metadata.Display.Category,
			Label:       metadata.Display.Label,
			Description: metadata.Display.Description,
			WhenToUse:   metadata.Display.WhenToUse,
			Tags:        metadata.Display.Tags,
		},
		RuntimeType:      metadata.RuntimeType,
		Enabled:          metadata.Enabled,
		HasTools:         metadata.HasTools,
		HasReferences:    metadata.HasReferences,
		HasScripts:       metadata.HasScripts,
		ScriptsSupported: metadata.ScriptsSupported,
		MaxCallsPerTurn:  metadata.MaxCallsPerTurn,
		TimeoutSeconds:   metadata.TimeoutSeconds,
		Status:           metadata.Status,
		ValidationError:  metadata.ValidationError,
	}
}

func skillImportPreviewResponse(preview *aichatservice.SkillImportPreview) aichatdto.ImportSkillPreviewResponse {
	if preview == nil {
		return aichatdto.ImportSkillPreviewResponse{
			Files:            []aichatdto.ImportSkillPreviewFile{},
			References:       []string{},
			Warnings:         []string{},
			ValidationErrors: []string{},
		}
	}
	files := make([]aichatdto.ImportSkillPreviewFile, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, aichatdto.ImportSkillPreviewFile{Path: file.Path, Size: file.Size})
	}
	var skill *aichatdto.SkillResponse
	if preview.Skill != nil {
		value := skillResponse(*preview.Skill)
		skill = &value
	}
	expiresAt := int64(0)
	if !preview.ExpiresAt.IsZero() {
		expiresAt = preview.ExpiresAt.Unix()
	}
	return aichatdto.ImportSkillPreviewResponse{
		ImportID:         preview.ImportID,
		ExpiresAt:        expiresAt,
		Skill:            skill,
		FileCount:        preview.FileCount,
		TotalSize:        preview.TotalSize,
		Files:            files,
		References:       copyStringSlice(preview.References),
		HasScripts:       preview.HasScripts,
		ScriptsSupported: preview.ScriptsSupported,
		Warnings:         copyStringSlice(preview.Warnings),
		ValidationErrors: copyStringSlice(preview.ValidationErrors),
		CanImport:        preview.CanImport,
	}
}

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func skillConfigResponse(config *aichatservice.SkillConfig) aichatdto.SkillConfigResponse {
	if config == nil {
		return aichatdto.SkillConfigResponse{EnabledSkillIDs: []string{}}
	}
	return aichatdto.SkillConfigResponse{EnabledSkillIDs: append([]string(nil), config.EnabledSkillIDs...)}
}

func messageResponse(message *aichatmodel.Message) aichatdto.MessageResponse {
	resp := aichatdto.MessageResponse{
		ID:                  message.ID.String(),
		ConversationID:      message.ConversationID.String(),
		Query:               message.Query,
		Answer:              message.Answer,
		Status:              message.Status,
		Error:               message.Error,
		ModelProvider:       message.ModelProvider,
		ModelName:           message.ModelName,
		BillingReasonSource: message.BillingReasonSource,
		ModelParameters:     message.ModelParameters,
		Metadata:            message.Metadata,
		CreatedAt:           message.CreatedAt.Unix(),
		UpdatedAt:           message.UpdatedAt.Unix(),
	}
	if message.ParentID != nil {
		resp.ParentID = stringPtr(message.ParentID.String())
	}
	if message.SourceMessageID != nil {
		resp.SourceMessageID = stringPtr(message.SourceMessageID.String())
	}
	return resp
}

func stopConversationResponse(result *aichatservice.StopConversationResult) aichatdto.StopConversationResponse {
	resp := aichatdto.StopConversationResponse{
		Status: aichatmodel.ConversationRuntimeStatusIdle,
	}
	if result == nil || result.Conversation == nil {
		return resp
	}
	resp.ConversationID = result.Conversation.ID.String()
	resp.RuntimeStatus = result.Conversation.RuntimeStatus
	resp.Status = result.Conversation.RuntimeStatus
	if result.Message != nil {
		resp.MessageID = stringPtr(result.Message.ID.String())
		resp.Status = result.Message.Status
	}
	return resp
}

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	return stringPtr(value.String())
}

func stringPtr(value string) *string {
	return &value
}
