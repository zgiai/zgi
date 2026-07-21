package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
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
	defaultSearchLimit           = 20
	maxSearchLimit               = 50

	skillConfigUpdateStatusApplied              = "applied"
	skillConfigUpdateStatusConfirmationRequired = "confirmation_required"
)

type skillConfigAppliedResult struct {
	Status          string   `json:"status"`
	Applied         bool     `json:"applied"`
	EnabledSkillIDs []string `json:"enabled_skill_ids"`
}

type skillConfigConfirmationRequiredResult struct {
	Status  string               `json:"status"`
	Applied bool                 `json:"applied"`
	Impact  agentbindings.Impact `json:"impact"`
}

type Handler struct {
	service runtimeservice.Service
}

func NewHandler(service runtimeservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/aichat")
	group.GET("/skills", h.ListSkills)
	group.GET("/skills/config", h.GetSkillConfig)
	group.GET("/skill-preferences/me", h.GetMySkillPreference)
	group.PUT("/skill-preferences/me", h.UpdateMySkillPreference)
	group.GET("/skills/:id", h.GetSkill)
	skillManagement := group.Group("/skills", middleware.EnterpriseAdminOrOwnerRequired())
	skillManagement.POST("/import/preview", h.PreviewImportSkill)
	skillManagement.POST("/import/confirm", h.ConfirmImportSkill)
	skillManagement.DELETE("/import/preview/:import_id", h.CancelImportSkillPreview)
	skillManagement.PUT("/config", h.UpdateSkillConfig)
	skillManagement.GET("/:id/delete-impact", h.PreviewSkillDeleteImpact)
	skillManagement.DELETE("/:id", h.DeleteSkill)
	group.GET("/conversations", h.ListConversations)
	group.GET("/search", h.Search)
	group.POST("/conversations", h.CreateConversation)
	group.GET("/conversations/:id", h.GetConversation)
	group.PATCH("/conversations/:id", h.UpdateConversation)
	group.DELETE("/conversations/:id", h.DeleteConversation)
	group.POST("/conversations/:id/stop", h.StopConversation)
	group.GET("/conversations/:id/events", h.StreamConversationEvents)
	group.GET("/conversations/:id/messages", h.ListMessages)
	group.GET("/conversations/:id/asset-operation-audits", h.ListAssetOperationAudits)
	group.DELETE("/messages/:id", h.DeleteMessage)
	group.POST("/messages/:id/stop", h.StopMessage)
	group.POST("/messages/:id/regenerate", h.RegenerateMessage)
	group.POST("/conversations/:id/messages/:message_id/tool-governance/:correlation_id", h.SubmitToolGovernanceDecision)
	group.POST("/conversations/:id/messages/:message_id/tool-governance/:correlation_id/continue", h.ContinueToolGovernanceDecision)
	group.POST("/conversations/:id/messages/:message_id/client-actions/:action_id/continue", h.ContinueClientAction)
	group.POST("/conversations/:id/messages/:message_id/user-input/:request_id/continue", h.ContinueUserInput)
	group.POST("/work-chat/chat", h.WorkChat)
	group.POST("/contextual/chat", h.ContextualChat)
	// Keep the original route as a work-chat compatibility alias. Surface is
	// fixed by the endpoint and is never trusted from the request body.
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
	items := make([]runtimedto.SkillResponse, 0, len(metadata))
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
	var req runtimedto.UpdateSkillConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	config, err := h.service.UpdateSkillConfig(c.Request.Context(), scope, req)
	if err != nil {
		var conflict *agentbindings.ConflictError
		if errors.As(err, &conflict) && conflict != nil {
			response.Success(c, skillConfigConfirmationRequiredResult{
				Status:  skillConfigUpdateStatusConfirmationRequired,
				Applied: false,
				Impact:  conflict.Impact,
			})
			return
		}
		h.fail(c, err)
		return
	}
	response.Success(c, skillConfigAppliedUpdateResponse(config))
}

func (h *Handler) GetMySkillPreference(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	preference, err := h.service.GetAccountSkillPreference(c.Request.Context(), scope, runtimemodel.ConversationCallerAIChat)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, accountSkillPreferenceResponse(preference))
}

func (h *Handler) UpdateMySkillPreference(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req runtimedto.UpdateAccountSkillPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	preference, err := h.service.UpdateAccountSkillPreference(c.Request.Context(), scope, runtimemodel.ConversationCallerAIChat, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, accountSkillPreferenceResponse(preference))
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
	var req runtimedto.ConfirmImportSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	metadata, err := h.service.ConfirmCustomSkillImport(c.Request.Context(), scope, req.ImportID, req.OverwriteConfirmed)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, skillResponse(*metadata))
}

func (h *Handler) CancelImportSkillPreview(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	importID := strings.TrimSpace(c.Param("import_id"))
	if importID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := h.service.CancelCustomSkillImportPreview(c.Request.Context(), scope, importID); err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"canceled": true})
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
	if err := h.service.DeleteSkill(
		c.Request.Context(),
		scope,
		skillID,
		c.Query("agent_binding_action"),
		c.Query("impact_token"),
	); err != nil {
		if util.WriteAgentBindingConflict(c, err) {
			return
		}
		h.fail(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

type skillDeleteImpactPreviewer interface {
	PreviewSkillDeleteImpact(ctx context.Context, scope runtimeservice.Scope, skillID string) (*agentbindings.Impact, error)
}

func (h *Handler) PreviewSkillDeleteImpact(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	skillID := strings.TrimSpace(c.Param("id"))
	if skillID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	previewer, ok := h.service.(skillDeleteImpactPreviewer)
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}
	impact, err := previewer.PreviewSkillDeleteImpact(c.Request.Context(), scope, skillID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, impact)
}

func (h *Handler) CreateConversation(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req runtimedto.CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversationType, ok := h.conversationType(c, req.ConversationType)
	if !ok {
		return
	}
	conversation, err := h.service.CreateConversationForCaller(c.Request.Context(), scope, runtimeservice.Caller{
		Type:             runtimemodel.ConversationCallerAIChat,
		ConversationType: conversationType,
	}, req.Title)
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
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	var conversations []*runtimemodel.Conversation
	var total int64
	var err error
	if conversationType == runtimemodel.ConversationTypeImage {
		conversations, total, err = h.service.ListConversationsByCaller(c.Request.Context(), scope, runtimeservice.Caller{
			Type:             runtimemodel.ConversationCallerAIChat,
			ConversationType: conversationType,
		}, page, limit)
	} else {
		conversations, total, err = h.service.ListConversationsBySurface(c.Request.Context(), scope, c.Query("surface"), page, limit)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]runtimedto.ConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, conversationResponse(conversation))
	}
	response.Success(c, runtimedto.ListResponse[runtimedto.ConversationResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

func (h *Handler) Search(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	limit := parsePositiveInt(c.Query("limit"), defaultSearchLimit)
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	var results []*runtimeservice.SearchResult
	var err error
	if conversationType == runtimemodel.ConversationTypeImage {
		results, err = h.service.SearchByCaller(c.Request.Context(), scope, runtimeservice.Caller{
			Type:             runtimemodel.ConversationCallerAIChat,
			ConversationType: conversationType,
		}, c.Query("query"), limit)
	} else {
		results, err = h.service.SearchBySurface(c.Request.Context(), scope, c.Query("surface"), c.Query("query"), limit)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]runtimedto.SearchResultResponse, 0, len(results))
	for _, item := range results {
		items = append(items, searchResultResponse(item))
	}
	response.Success(c, items)
}

func (h *Handler) GetConversation(c *gin.Context) {
	scope, id, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	conversation, err := h.service.GetConversationByCaller(c.Request.Context(), scope, runtimeservice.Caller{
		Type:             runtimemodel.ConversationCallerAIChat,
		ConversationType: conversationType,
	}, id)
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
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	var req runtimedto.UpdateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversation, err := h.service.UpdateConversationByCaller(c.Request.Context(), scope, runtimeservice.Caller{
		Type:             runtimemodel.ConversationCallerAIChat,
		ConversationType: conversationType,
	}, id, req)
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
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	if err := h.service.DeleteConversationByCaller(c.Request.Context(), scope, runtimeservice.Caller{
		Type:             runtimemodel.ConversationCallerAIChat,
		ConversationType: conversationType,
	}, id); err != nil {
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
	conversationType, ok := h.conversationType(c, c.Query("conversation_type"))
	if !ok {
		return
	}
	page, limit := pagination(c, 1, defaultMessagePageLimit, maxMessagePageLimit)
	messages, total, err := h.service.ListConversationMessagesByCaller(c.Request.Context(), scope, runtimeservice.Caller{
		Type:             runtimemodel.ConversationCallerAIChat,
		ConversationType: conversationType,
	}, conversationID, page, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]runtimedto.MessageResponse, 0, len(messages))
	for _, message := range messages {
		items = append(items, messageResponse(message))
	}
	response.Success(c, runtimedto.ListResponse[runtimedto.MessageResponse]{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	})
}

type assetOperationAuditLister interface {
	ListAssetOperationAudits(ctx context.Context, scope runtimeservice.Scope, conversationID uuid.UUID, page, limit int) ([]runtimeservice.AssetOperationAuditRecord, int64, error)
}

func (h *Handler) ListAssetOperationAudits(c *gin.Context) {
	scope, conversationID, ok := h.scopedID(c, "id")
	if !ok {
		return
	}
	lister, ok := h.service.(assetOperationAuditLister)
	if !ok {
		h.fail(c, fmt.Errorf("%w: asset operation audit service is not configured", runtimeservice.ErrInvalidInput))
		return
	}
	page, limit := pagination(c, 1, defaultMessagePageLimit, maxMessagePageLimit)
	items, total, err := lister.ListAssetOperationAudits(c.Request.Context(), scope, conversationID, page, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, runtimedto.ListResponse[runtimeservice.AssetOperationAuditRecord]{
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
	err = h.service.StreamConversationEvents(c.Request.Context(), scope, conversationID, messageID, c.Query("after_id"), func(event runtimeservice.StreamEvent) error {
		return writeStreamEvent(c, event)
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
	var req runtimedto.RegenerateMessageRequest
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
	}, func(event runtimeservice.StreamEvent) error {
		return writeStreamEvent(c, event)
	})
	if err != nil {
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			writeChatStopped(c, prepared)
			return
		}
		if runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		writeChatError(c, prepared, err)
		return
	}
	writeChatEnd(c, prepared, result)
}

func (h *Handler) Chat(c *gin.Context) {
	h.chatForSurface(c, runtimedto.RuntimeSurfaceWorkChat)
}

func (h *Handler) WorkChat(c *gin.Context) {
	h.chatForSurface(c, runtimedto.RuntimeSurfaceWorkChat)
}

func (h *Handler) ContextualChat(c *gin.Context) {
	h.chatForSurface(c, runtimedto.RuntimeSurfaceContextualSidebar)
}

func (h *Handler) chatForSurface(c *gin.Context, surface string) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req runtimedto.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	req.Surface = surface

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
	}, func(event runtimeservice.StreamEvent) error {
		return writeStreamEvent(c, event)
	})
	if err != nil {
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			writeChatStopped(c, prepared)
			return
		}
		if runtimeservice.IsFinalizedStreamError(err) {
			return
		}
		writeChatError(c, prepared, err)
		return
	}
	writeChatEnd(c, prepared, result)
}

func writeChatStart(c *gin.Context, prepared *runtimeservice.PreparedChat) {
	_ = writeSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidStringPtr(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"replace":         prepared.ReplaceRoot,
		"created_at":      prepared.Message.CreatedAt.Unix(),
		"created_at_ms":   prepared.Message.CreatedAt.UnixMilli(),
	})
}

func writeChatChunk(c *gin.Context, prepared *runtimeservice.PreparedChat, chunk string) error {
	now := time.Now()
	return writeSSE(c, "message", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          chunk,
		"created_at":      now.Unix(),
		"created_at_ms":   now.UnixMilli(),
	})
}

func writeChatError(c *gin.Context, prepared *runtimeservice.PreparedChat, err error) {
	logger.WarnContext(c.Request.Context(), "aichat stream failed", "message_id", prepared.Message.ID.String(), err)
	_ = writeSSE(c, "error", runtimeservice.BuildStreamErrorPayload(prepared, err))
}

func writeChatEnd(c *gin.Context, prepared *runtimeservice.PreparedChat, result *runtimeservice.ChatResult) {
	metadata := map[string]interface{}{}
	status := runtimemodel.MessageStatusCompleted
	eventID := ""
	if result != nil && result.Metadata != nil {
		metadata = messageMetadataResponse(result.Metadata)
	}
	if result != nil && strings.TrimSpace(result.Status) != "" {
		status = strings.TrimSpace(result.Status)
	}
	if result != nil {
		eventID = strings.TrimSpace(result.MessageEndEventID)
	}
	_ = writeSSEEvent(c, eventID, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          status,
		"metadata":        metadata,
	})
}

func writeChatStopped(c *gin.Context, prepared *runtimeservice.PreparedChat) {
	_ = writeSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          runtimemodel.MessageStatusStopped,
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

func (w *sseClientWriter) writeChunk(prepared *runtimeservice.PreparedChat, chunk string) error {
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

func (h *Handler) scope(c *gin.Context) (runtimeservice.Scope, bool) {
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return runtimeservice.Scope{}, false
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return runtimeservice.Scope{}, false
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err == nil {
			workspaceID = &parsed
		}
	}
	return runtimeservice.Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    workspaceID,
	}, true
}

func (h *Handler) scopedID(c *gin.Context, param string) (runtimeservice.Scope, uuid.UUID, bool) {
	scope, ok := h.scope(c)
	if !ok {
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param(param)))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return runtimeservice.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimeservice.ErrUnauthorized):
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, runtimeservice.ErrPermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, runtimeservice.ErrNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, runtimeservice.ErrInvalidInput), errors.Is(err, runtimeservice.ErrInvalidModelParam):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, runtimeservice.ErrConversationRunning),
		errors.Is(err, runtimeservice.ErrConversationWaitingApproval),
		errors.Is(err, runtimeservice.ErrConversationWaitingQuestion),
		errors.Is(err, runtimeservice.ErrConversationWaitingAction),
		errors.Is(err, runtimeservice.ErrMessageReplaceNotAllowed):
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
	writer := newRuntimeSSEWriter(c)
	c.Set(runtimeSSEWriterContextKey, writer)
	writer.StartHeartbeat(c.Request.Context())
}

func writeSSE(c *gin.Context, event string, data interface{}) error {
	return writeSSEEvent(c, "", event, data)
}

func writeStreamEvent(c *gin.Context, event runtimeservice.StreamEvent) error {
	return writeSSEEvent(c, event.ID, event.EventType, event.Payload)
}

func writeSSEEvent(c *gin.Context, id string, event string, data interface{}) error {
	if value, ok := c.Get(runtimeSSEWriterContextKey); ok {
		if writer, writerOK := value.(*runtimeSSEWriter); writerOK && writer != nil {
			return writer.WriteEvent(id, event, data)
		}
	}
	return writeSSERaw(c, id, event, data)
}

func writeSSERaw(c *gin.Context, id string, event string, data interface{}) error {
	payload := gin.H{"event": event, "data": enrichSSEPayload(id, data)}
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

func enrichSSEPayload(id string, data interface{}) interface{} {
	payload, ok := cloneSSEPayload(data)
	if !ok {
		return data
	}
	id = strings.TrimSpace(id)
	if id != "" {
		payload["event_id"] = id
	}
	if createdAtMS, sequence := redisStreamIDParts(id); createdAtMS > 0 {
		payload["created_at_ms"] = createdAtMS
		payload["created_at"] = createdAtMS / 1000
		payload["sequence"] = sequence
		return payload
	}
	if _, ok := payload["created_at_ms"]; !ok {
		if createdAt := int64FromInterface(payload["created_at"]); createdAt > 0 {
			payload["created_at_ms"] = createdAt * 1000
		}
	}
	return payload
}

func cloneSSEPayload(data interface{}) (gin.H, bool) {
	switch typed := data.(type) {
	case gin.H:
		cloned := make(gin.H, len(typed)+4)
		for key, value := range typed {
			cloned[key] = value
		}
		return cloned, true
	case map[string]interface{}:
		cloned := make(gin.H, len(typed)+4)
		for key, value := range typed {
			cloned[key] = value
		}
		return cloned, true
	default:
		return nil, false
	}
}

func redisStreamIDParts(id string) (int64, int64) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, 0
	}
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	createdAtMS, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || createdAtMS <= 0 {
		return 0, 0
	}
	sequenceRaw := parts[1]
	if index := strings.Index(sequenceRaw, ":"); index >= 0 {
		sequenceRaw = sequenceRaw[:index]
	}
	sequence, err := strconv.ParseInt(sequenceRaw, 10, 64)
	if err != nil || sequence < 0 {
		sequence = 0
	}
	return createdAtMS, sequence
}

func int64FromInterface(value interface{}) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
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

func (h *Handler) conversationType(c *gin.Context, raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return runtimemodel.ConversationTypeChat, true
	}
	switch value {
	case runtimemodel.ConversationTypeChat, runtimemodel.ConversationTypeImage:
		return value, true
	default:
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
}

func conversationResponse(conversation *runtimemodel.Conversation) runtimedto.ConversationResponse {
	resp := runtimedto.ConversationResponse{
		ID:               conversation.ID.String(),
		OrganizationID:   conversation.OrganizationID.String(),
		AccountID:        conversation.AccountID.String(),
		Title:            conversation.Title,
		Status:           conversation.Status,
		RuntimeStatus:    conversation.RuntimeStatus,
		ConversationType: conversation.ConversationType,
		DialogueCount:    conversation.DialogueCount,
		Source:           conversation.Source,
		Metadata:         conversation.Metadata,
		CreatedAt:        conversation.CreatedAt.Unix(),
		UpdatedAt:        conversation.UpdatedAt.Unix(),
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

func skillResponse(metadata skills.SkillDiscoveryMetadata) runtimedto.SkillResponse {
	return runtimedto.SkillResponse{
		SkillID:     metadata.ID,
		Source:      metadata.Source,
		Name:        metadata.Name,
		Description: metadata.Description,
		WhenToUse:   metadata.WhenToUse,
		Display: runtimedto.SkillDisplayResponse{
			Icon:        metadata.Display.Icon,
			Category:    metadata.Display.Category,
			Scenarios:   metadata.Display.Scenarios,
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
		SupportedCallers: metadata.SupportedCallers,
		RequiredConfig:   metadata.RequiredConfig,
		Exposure:         skillExposureResponse(skills.SkillExposureForMetadata(metadata)),
	}
}

func skillExposureResponse(profile skills.SkillExposureProfile) runtimedto.SkillExposureResponse {
	return runtimedto.SkillExposureResponse{
		Category:            profile.Category,
		UserSelectable:      profile.UserSelectable,
		RuntimeManaged:      profile.RuntimeManaged,
		SystemAsset:         profile.SystemAsset,
		PageContextRequired: profile.PageContextRequired,
		GovernanceRisk:      profile.GovernanceRisk,
	}
}

func skillImportPreviewResponse(preview *runtimeservice.SkillImportPreview) runtimedto.ImportSkillPreviewResponse {
	if preview == nil {
		return runtimedto.ImportSkillPreviewResponse{
			Files:            []runtimedto.ImportSkillPreviewFile{},
			References:       []string{},
			Warnings:         []string{},
			ValidationErrors: []string{},
		}
	}
	files := make([]runtimedto.ImportSkillPreviewFile, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, runtimedto.ImportSkillPreviewFile{Path: file.Path, Size: file.Size})
	}
	var skill *runtimedto.SkillResponse
	if preview.Skill != nil {
		value := skillResponse(*preview.Skill)
		skill = &value
	}
	var existingSkill *runtimedto.ExistingSkillResponse
	if preview.ExistingSkill != nil {
		existingSkill = &runtimedto.ExistingSkillResponse{
			SkillID: preview.ExistingSkill.SkillID,
			Name:    preview.ExistingSkill.Name,
		}
		if !preview.ExistingSkill.UpdatedAt.IsZero() {
			existingSkill.UpdatedAt = preview.ExistingSkill.UpdatedAt.Unix()
		}
	}
	expiresAt := int64(0)
	if !preview.ExpiresAt.IsZero() {
		expiresAt = preview.ExpiresAt.Unix()
	}
	return runtimedto.ImportSkillPreviewResponse{
		ImportID:         preview.ImportID,
		ExpiresAt:        expiresAt,
		Skill:            skill,
		WillOverwrite:    preview.WillOverwrite,
		ExistingSkill:    existingSkill,
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

func skillConfigResponse(config *runtimeservice.SkillConfig) runtimedto.SkillConfigResponse {
	if config == nil {
		return runtimedto.SkillConfigResponse{EnabledSkillIDs: []string{}}
	}
	return runtimedto.SkillConfigResponse{EnabledSkillIDs: append([]string(nil), config.EnabledSkillIDs...)}
}

func skillConfigAppliedUpdateResponse(config *runtimeservice.SkillConfig) skillConfigAppliedResult {
	result := skillConfigAppliedResult{
		Status:          skillConfigUpdateStatusApplied,
		Applied:         true,
		EnabledSkillIDs: []string{},
	}
	if config != nil {
		result.EnabledSkillIDs = append([]string(nil), config.EnabledSkillIDs...)
	}
	return result
}

func accountSkillPreferenceResponse(pref *runtimeservice.AccountSkillPreference) runtimedto.AccountSkillPreferenceResponse {
	if pref == nil {
		return runtimedto.AccountSkillPreferenceResponse{EnabledSkillIDs: []string{}, Defaulted: true}
	}
	return runtimedto.AccountSkillPreferenceResponse{
		EnabledSkillIDs: append([]string(nil), pref.EnabledSkillIDs...),
		Defaulted:       pref.Defaulted,
	}
}

func messageResponse(message *runtimemodel.Message) runtimedto.MessageResponse {
	resp := runtimedto.MessageResponse{
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
		Metadata:            messageMetadataResponse(message.Metadata),
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

func messageMetadataResponse(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return metadata
	}
	out := make(map[string]interface{}, len(metadata))
	redactedModelInvocations := false
	modelInvocationCount := 0
	for key, value := range metadata {
		if key == "model_invocations" {
			redactedModelInvocations = true
			modelInvocationCount = modelInvocationMetadataCount(value)
			continue
		}
		if key == "model_invocations_redacted" {
			continue
		}
		if key == "skill_invocations" {
			filtered, changed := messageSkillInvocationsResponse(value)
			if changed {
				if len(filtered) > 0 {
					out[key] = filtered
				}
				continue
			}
		}
		out[key] = value
	}
	if redactedModelInvocations {
		out["model_invocations_redacted"] = true
		if modelInvocationCount > 0 {
			out["model_invocation_count"] = modelInvocationCount
		}
	}
	return out
}

func messageSkillInvocationsResponse(value interface{}) ([]interface{}, bool) {
	items, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]map[string]interface{}); ok {
			items = make([]interface{}, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		}
	}
	if len(items) == 0 {
		return nil, false
	}
	out := make([]interface{}, 0, len(items))
	changed := false
	for _, item := range items {
		invocation, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		if messageMetadataFinalAnswerInvocation(invocation) {
			changed = true
			continue
		}
		out = append(out, item)
	}
	return out, changed
}

func messageMetadataFinalAnswerInvocation(invocation map[string]interface{}) bool {
	return strings.EqualFold(strings.TrimSpace(messageMetadataString(invocation["kind"])), "final_answer") ||
		strings.EqualFold(strings.TrimSpace(messageMetadataString(invocation["tool_name"])), skills.MetaToolFinalAnswer)
}

func messageMetadataString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func modelInvocationMetadataCount(value interface{}) int {
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []map[string]interface{}:
		return len(typed)
	default:
		return 0
	}
}

func searchResultResponse(result *runtimeservice.SearchResult) runtimedto.SearchResultResponse {
	resp := runtimedto.SearchResultResponse{
		Type:              result.Type,
		ConversationID:    result.ConversationID.String(),
		ConversationTitle: result.ConversationTitle,
		Snippet:           result.Snippet,
		UpdatedAt:         result.UpdatedAt.Unix(),
	}
	if result.MessageID != nil {
		resp.MessageID = stringPtr(result.MessageID.String())
	}
	return resp
}

func stopConversationResponse(result *runtimeservice.StopConversationResult) runtimedto.StopConversationResponse {
	resp := runtimedto.StopConversationResponse{
		Status: runtimemodel.ConversationRuntimeStatusIdle,
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
