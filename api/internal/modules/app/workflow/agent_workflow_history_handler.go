package workflow

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// AgentWorkflowHistoryHandler serves agent-centric workflow history endpoints.
type AgentWorkflowHistoryHandler struct {
	conversationService conversationHistoryAccessService
	messageService      conversationMessageQueryService
}

func NewAgentWorkflowHistoryHandler(
	conversationService conversationHistoryAccessService,
	messageService conversationMessageQueryService,
) *AgentWorkflowHistoryHandler {
	return &AgentWorkflowHistoryHandler{
		conversationService: conversationService,
		messageService:      messageService,
	}
}

// GetConversations handles GET /agents/:agent_id/conversations
func (h *AgentWorkflowHistoryHandler) GetConversations(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	req := AgentConversationListRequest{
		Page:             parsePositiveInt(c.Query("page"), 1),
		Limit:            parsePositiveInt(c.Query("limit"), 20),
		Keyword:          c.Query("keyword"),
		AnnotationStatus: c.DefaultQuery("annotation_status", "all"),
		SortBy:           c.DefaultQuery("sort_by", "created_at"),
		InvokeFrom:       parseMultiValueQuery(c.QueryArray("invoke_from"), string(InvokeFromWebApp)),
		Start:            c.Query("start"),
		End:              c.Query("end"),
	}

	start, err := parseOptionalTime(req.Start)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	end, err := parseOptionalTime(req.End)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	offset := (req.Page - 1) * req.Limit
	filter := conversation.AgentConversationHistoryFilter{
		AgentID:     agentUUID,
		Keyword:     req.Keyword,
		SortBy:      req.SortBy,
		Start:       start,
		End:         end,
		InvokeFroms: req.InvokeFrom,
		Limit:       req.Limit,
		Offset:      offset,
	}

	conversations, total, err := h.conversationService.GetConversationHistoryByAgent(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to get agent conversations", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	items := make([]AgentConversationListItem, 0, len(conversations))
	for _, conv := range conversations {
		items = append(items, buildAgentConversationListItem(conv))
	}

	hasMore := int64(req.Page*req.Limit) < total
	response.Success(c, AgentConversationListResponse{
		Page:    req.Page,
		Limit:   req.Limit,
		Total:   total,
		HasMore: hasMore,
		Data:    items,
	})
}

// GetConversationDetail handles GET /agents/:agent_id/conversations/:conversation_id
func (h *AgentWorkflowHistoryHandler) GetConversationDetail(c *gin.Context) {
	agentID := c.Param("agent_id")
	conversationID := c.Param("conversation_id")
	if agentID == "" || conversationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	conv, err := h.conversationService.GetConversationByIDAndAgent(c.Request.Context(), conversationUUID, agentUUID)
	if err != nil {
		logger.Error("Conversation does not belong to agent", err)
		response.Fail(c, response.ErrConversationNotFound)
		return
	}

	response.Success(c, buildAgentConversationDetailResponse(conv))
}

// GetChatMessages handles GET /agents/:agent_id/chat-messages?conversation_id=:conversation_id
func (h *AgentWorkflowHistoryHandler) GetChatMessages(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	conversationID := c.Query("conversation_id")
	if conversationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if _, err := h.conversationService.GetConversationByIDAndAgent(c.Request.Context(), conversationUUID, agentUUID); err != nil {
		logger.Error("Conversation does not belong to agent", err)
		response.Fail(c, response.ErrConversationNotFound)
		return
	}

	page := parsePositiveInt(c.Query("page"), 1)
	limit := parsePositiveInt(c.Query("limit"), 20)
	offset := (page - 1) * limit

	messages, total, err := h.messageService.GetMessagesByConversation(c.Request.Context(), conversationUUID, limit, offset)
	if err != nil {
		logger.Error("Failed to get agent chat messages", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, buildChatMessagesResponseWithContext(c.Request.Context(), messages, total, page, limit))
}

func buildAgentConversationListItem(conv *conversation.AgentConversation) AgentConversationListItem {
	return AgentConversationListItem{
		ID:                 conv.ID.String(),
		Status:             conv.Status,
		FromSource:         conv.FromSource,
		InvokeFrom:         conv.InvokeFrom,
		FromEndUserID:      uuidPointerToString(conv.FromEndUserID),
		FromAccountID:      uuidPointerToString(conv.FromAccountID),
		FromAccountName:    nil,
		Name:               conv.Name,
		Summary:            conv.Summary,
		ReadAt:             timePointerToUnix(conv.ReadAt),
		CreatedAt:          conv.CreatedAt.Unix(),
		UpdatedAt:          conv.UpdatedAt.Unix(),
		Annotated:          false,
		ModelConfig:        defaultAgentConversationModelConfig(),
		MessageCount:       conv.DialogueCount,
		UserFeedbackStats:  defaultAgentConversationStats(),
		AdminFeedbackStats: defaultAgentConversationStats(),
	}
}

func buildAgentConversationDetailResponse(conv *conversation.AgentConversation) AgentConversationDetailResponse {
	return AgentConversationDetailResponse{
		ID:                 conv.ID.String(),
		Status:             conv.Status,
		FromSource:         conv.FromSource,
		InvokeFrom:         conv.InvokeFrom,
		FromEndUserID:      uuidPointerToString(conv.FromEndUserID),
		FromAccountID:      uuidPointerToString(conv.FromAccountID),
		FromAccountName:    nil,
		Name:               conv.Name,
		Summary:            conv.Summary,
		ReadAt:             timePointerToUnix(conv.ReadAt),
		CreatedAt:          conv.CreatedAt.Unix(),
		UpdatedAt:          conv.UpdatedAt.Unix(),
		Annotated:          false,
		Introduction:       conv.Introduction,
		ModelConfig:        defaultAgentConversationModelConfig(),
		MessageCount:       conv.DialogueCount,
		UserFeedbackStats:  defaultAgentConversationStats(),
		AdminFeedbackStats: defaultAgentConversationStats(),
	}
}

func uuidPointerToString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	s := value.String()
	return &s
}

func timePointerToUnix(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	unix := value.Unix()
	return &unix
}

func defaultAgentConversationModelConfig() map[string]interface{} {
	return map[string]interface{}{
		"model":      nil,
		"pre_prompt": nil,
	}
}

func defaultAgentConversationStats() map[string]int64 {
	return map[string]int64{
		"like":    0,
		"dislike": 0,
	}
}
