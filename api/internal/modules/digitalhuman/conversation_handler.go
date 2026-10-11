package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

const (
	defaultConversationPageSize = 20
	defaultMessagePageSize      = 100
	maxConversationPageSize     = 100
	maxMessagePageSize          = 200
	digitalHumanSSEHeartbeat    = 15 * time.Second
)

func (h *RuntimeHandler) ListConversations(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, maxConversationPageSize)
	items, total, err := h.service.ListConversations(c.Request.Context(), scope, digitalHumanID, page, limit)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.list"))
		return
	}
	responses := make([]runtimedto.ConversationResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalHumanConversationResponse(item))
	}
	writeDigitalHumanData(c, http.StatusOK, runtimedto.ListResponse[runtimedto.ConversationResponse]{
		Data: responses, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total,
	})
}

func (h *RuntimeHandler) SearchConversations(c *gin.Context) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return
	}
	limit := positiveRuntimeQueryInt(c, "limit", defaultConversationPageSize, 50)
	items, err := h.service.SearchConversations(c.Request.Context(), scope, digitalHumanID, c.Query("query"), limit)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.search"))
		return
	}
	responses := make([]runtimedto.SearchResultResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, digitalHumanSearchResponse(item))
	}
	writeDigitalHumanData(c, http.StatusOK, responses)
}

func (h *RuntimeHandler) GetConversation(c *gin.Context) {
	h.withConversation(c, "get", func(scope Scope, digitalHumanID, conversationID uuid.UUID) (any, error) {
		conversation, err := h.service.GetConversation(c.Request.Context(), scope, digitalHumanID, conversationID)
		if err != nil {
			return nil, err
		}
		return digitalHumanConversationResponse(conversation), nil
	})
}

func (h *RuntimeHandler) UpdateConversation(c *gin.Context) {
	h.withConversation(c, "update", func(scope Scope, digitalHumanID, conversationID uuid.UUID) (any, error) {
		var request runtimedto.UpdateConversationRequest
		if !h.base.decode(c, &request) {
			return nil, nil
		}
		conversation, err := h.service.UpdateConversation(c.Request.Context(), scope, digitalHumanID, conversationID, request)
		if err != nil {
			return nil, err
		}
		return digitalHumanConversationResponse(conversation), nil
	})
}

func (h *RuntimeHandler) DeleteConversation(c *gin.Context) {
	h.withConversation(c, "delete", func(scope Scope, digitalHumanID, conversationID uuid.UUID) (any, error) {
		if err := h.service.DeleteConversation(c.Request.Context(), scope, digitalHumanID, conversationID); err != nil {
			return nil, err
		}
		return gin.H{"result": "success"}, nil
	})
}

func (h *RuntimeHandler) ListConversationMessages(c *gin.Context) {
	scope, digitalHumanID, conversationID, ok := h.conversationScope(c)
	if !ok {
		return
	}
	page := positiveRuntimeQueryInt(c, "page", 1, 1_000_000)
	limit := positiveRuntimeQueryInt(c, "limit", defaultMessagePageSize, maxMessagePageSize)
	items, total, err := h.service.ListConversationMessages(c.Request.Context(), scope, digitalHumanID, conversationID, page, limit)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.messages"))
		return
	}
	runtimeInputs, err := h.service.ListConversationRuntimeInputs(c.Request.Context(), scope, digitalHumanID, conversationID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.runtime_inputs"))
		return
	}
	inputsByMessage := digitalHumanRuntimeInputsByVisibleMessage(runtimeInputs)
	responses := make([]runtimedto.MessageResponse, 0, len(items))
	for _, item := range items {
		response := digitalHumanMessageResponse(item)
		response.RuntimeInputs = inputsByMessage[item.ID]
		if response.RuntimeInputs == nil {
			response.RuntimeInputs = []runtimedto.RuntimeInputResponse{}
		}
		responses = append(responses, response)
	}
	writeDigitalHumanData(c, http.StatusOK, runtimedto.ListResponse[runtimedto.MessageResponse]{
		Data: responses, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total,
	})
}

func (h *RuntimeHandler) SubmitRuntimeInput(c *gin.Context) {
	scope, digitalHumanID, conversationID, ok := h.conversationScope(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Param("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.conversation.runtime_input"))
		return
	}
	var request runtimedto.SubmitRuntimeInputRequest
	if !h.base.decode(c, &request) {
		return
	}
	clientInputID, err := uuid.Parse(strings.TrimSpace(request.ClientInputID))
	if err != nil || clientInputID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.conversation.runtime_input"))
		return
	}
	result, err := h.service.SubmitRuntimeInput(c.Request.Context(), scope, digitalHumanID, runtimeservice.SubmitRuntimeInputRequest{
		ConversationID: conversationID, TargetMessageID: messageID,
		ClientInputID: clientInputID, Content: request.Content,
	})
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.runtime_input"))
		return
	}
	writeDigitalHumanData(c, http.StatusOK, runtimedto.SubmitRuntimeInputResponse{
		Input: digitalHumanRuntimeInputResponse(result.Input), Disposition: string(result.Disposition), Created: result.Created,
	})
}

func (h *RuntimeHandler) StopConversation(c *gin.Context) {
	h.withConversation(c, "stop", func(scope Scope, digitalHumanID, conversationID uuid.UUID) (any, error) {
		result, err := h.service.StopConversation(c.Request.Context(), scope, digitalHumanID, conversationID)
		if err != nil {
			return nil, err
		}
		return digitalHumanStopConversationResponse(result), nil
	})
}

func (h *RuntimeHandler) StreamConversationEvents(c *gin.Context) {
	scope, digitalHumanID, conversationID, ok := h.conversationScope(c)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(c.Query("message_id")))
	if err != nil || messageID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.conversation.events"))
		return
	}
	if _, err := h.service.GetConversation(c.Request.Context(), scope, digitalHumanID, conversationID); err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation.events"))
		return
	}
	writer := newDigitalHumanSSEWriter(c)
	writer.start(c.Request.Context())
	err = h.service.StreamConversationEvents(
		c.Request.Context(), scope, digitalHumanID, conversationID, messageID, c.Query("after_id"),
		func(event runtimeservice.StreamEvent) error {
			return writer.write(event.ID, event.EventType, redactConversationValue(event.Payload))
		},
	)
	if err != nil {
		_ = writer.write("", "error", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"code":            AppCodeRunConflict.String(),
			"message":         "The conversation stream could not be continued.",
		})
	}
}

func (h *RuntimeHandler) withConversation(c *gin.Context, operation string, action func(Scope, uuid.UUID, uuid.UUID) (any, error)) {
	scope, digitalHumanID, conversationID, ok := h.conversationScope(c)
	if !ok {
		return
	}
	result, err := action(scope, digitalHumanID, conversationID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.conversation."+operation))
		return
	}
	if c.IsAborted() || result == nil {
		return
	}
	writeDigitalHumanData(c, http.StatusOK, result)
}

func (h *RuntimeHandler) conversationScope(c *gin.Context) (Scope, uuid.UUID, uuid.UUID, bool) {
	scope, digitalHumanID, ok := h.base.scopeAndID(c)
	if !ok {
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	conversationID, err := uuid.Parse(strings.TrimSpace(c.Param("conversation_id")))
	if err != nil || conversationID == uuid.Nil {
		h.base.writeError(c, applicationError(ErrInvalidRequest, "digital_human.conversation.request"))
		return Scope{}, uuid.Nil, uuid.Nil, false
	}
	return scope, digitalHumanID, conversationID, true
}

func positiveRuntimeQueryInt(c *gin.Context, name string, fallback, maximum int) int {
	value, err := strconv.Atoi(c.DefaultQuery(name, strconv.Itoa(fallback)))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func digitalHumanConversationResponse(conversation *runtimemodel.Conversation) runtimedto.ConversationResponse {
	response := runtimedto.ConversationResponse{
		ID: conversation.ID.String(), OrganizationID: conversation.OrganizationID.String(), AccountID: conversation.AccountID.String(),
		Title: conversation.Title, Status: conversation.Status, RuntimeStatus: conversation.RuntimeStatus,
		DialogueCount: conversation.DialogueCount, Source: conversation.Source,
		Metadata: redactConversationMap(conversation.Metadata), CreatedAt: conversation.CreatedAt.Unix(), UpdatedAt: conversation.UpdatedAt.Unix(),
	}
	if conversation.WorkspaceID != nil {
		value := conversation.WorkspaceID.String()
		response.WorkspaceID = &value
	}
	if conversation.CurrentLeafMessageID != nil {
		value := conversation.CurrentLeafMessageID.String()
		response.CurrentLeafMessageID = &value
	}
	if conversation.ActiveMessageID != nil {
		value := conversation.ActiveMessageID.String()
		response.ActiveMessageID = &value
	}
	return response
}

func digitalHumanMessageResponse(message *runtimemodel.Message) runtimedto.MessageResponse {
	response := runtimedto.MessageResponse{
		ID: message.ID.String(), ConversationID: message.ConversationID.String(), Query: message.Query,
		Answer: message.Answer, Status: message.Status, Error: message.Error,
		ModelProvider: message.ModelProvider, ModelName: message.ModelName,
		BillingReasonSource: message.BillingReasonSource, ModelParameters: redactConversationMap(message.ModelParameters),
		Metadata: redactConversationMap(message.Metadata), CreatedAt: message.CreatedAt.Unix(), UpdatedAt: message.UpdatedAt.Unix(),
	}
	if message.ParentID != nil {
		value := message.ParentID.String()
		response.ParentID = &value
	}
	if message.SourceMessageID != nil {
		value := message.SourceMessageID.String()
		response.SourceMessageID = &value
	}
	return response
}

func digitalHumanRuntimeInputsByVisibleMessage(inputs []*runtimemodel.RuntimeInput) map[uuid.UUID][]runtimedto.RuntimeInputResponse {
	result := make(map[uuid.UUID][]runtimedto.RuntimeInputResponse)
	for _, input := range inputs {
		if input == nil {
			continue
		}
		if input.ConsumedMessageID != nil && input.ConsumptionMode != nil && *input.ConsumptionMode == runtimemodel.RuntimeInputConsumptionFollowUp {
			continue
		}
		result[input.TargetMessageID] = append(result[input.TargetMessageID], digitalHumanRuntimeInputResponse(input))
	}
	return result
}

func digitalHumanRuntimeInputResponse(input *runtimemodel.RuntimeInput) runtimedto.RuntimeInputResponse {
	if input == nil {
		return runtimedto.RuntimeInputResponse{}
	}
	response := runtimedto.RuntimeInputResponse{
		ID: input.ID.String(), ClientInputID: input.ClientInputID.String(), Content: input.Content,
		Status: digitalHumanRuntimeInputUserStatus(input), TargetMessageID: input.TargetMessageID.String(),
		Sequence: input.Sequence, CreatedAt: input.CreatedAt.Unix(),
	}
	if input.ConsumedMessageID != nil {
		value := input.ConsumedMessageID.String()
		response.ConsumedMessageID = &value
	}
	return response
}

func digitalHumanRuntimeInputUserStatus(input *runtimemodel.RuntimeInput) string {
	if input == nil {
		return "failed"
	}
	switch input.Status {
	case runtimemodel.RuntimeInputStatusCanceled:
		return "canceled"
	case runtimemodel.RuntimeInputStatusFailed:
		return "failed"
	case runtimemodel.RuntimeInputStatusClaimed:
		if input.ConsumptionMode != nil && *input.ConsumptionMode == runtimemodel.RuntimeInputConsumptionFollowUp {
			return "queued_follow_up"
		}
	case runtimemodel.RuntimeInputStatusConsumed:
		if input.ConsumptionMode != nil && *input.ConsumptionMode == runtimemodel.RuntimeInputConsumptionFollowUp {
			return "applied_follow_up"
		}
		return "applied_current_run"
	}
	return "pending"
}

func digitalHumanSearchResponse(result *runtimeservice.SearchResult) runtimedto.SearchResultResponse {
	response := runtimedto.SearchResultResponse{
		Type: result.Type, ConversationID: result.ConversationID.String(), ConversationTitle: result.ConversationTitle,
		Snippet: result.Snippet, UpdatedAt: result.UpdatedAt.Unix(),
	}
	if result.MessageID != nil {
		value := result.MessageID.String()
		response.MessageID = &value
	}
	return response
}

func digitalHumanStopConversationResponse(result *runtimeservice.StopConversationResult) runtimedto.StopConversationResponse {
	response := runtimedto.StopConversationResponse{RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle, Status: runtimemodel.ConversationRuntimeStatusIdle}
	if result == nil || result.Conversation == nil {
		return response
	}
	response.ConversationID = result.Conversation.ID.String()
	response.RuntimeStatus = result.Conversation.RuntimeStatus
	response.Status = result.Conversation.RuntimeStatus
	if result.Message != nil {
		value := result.Message.ID.String()
		response.MessageID = &value
		response.Status = result.Message.Status
	}
	return response
}

func redactConversationMap(value map[string]interface{}) map[string]interface{} {
	redacted, ok := redactConversationValue(value).(map[string]interface{})
	if !ok || redacted == nil {
		return map[string]interface{}{}
	}
	return redacted
}

func redactConversationValue(value interface{}) interface{} {
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{}
	}
	var cloned interface{}
	if json.Unmarshal(encoded, &cloned) != nil {
		return map[string]interface{}{}
	}
	redactRunValue(cloned)
	return cloned
}

type digitalHumanSSEWriter struct {
	context *gin.Context
	mu      sync.Mutex
}

func newDigitalHumanSSEWriter(c *gin.Context) *digitalHumanSSEWriter {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.Flush()
	return &digitalHumanSSEWriter{context: c}
}

func (w *digitalHumanSSEWriter) start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(digitalHumanSSEHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.mu.Lock()
				_, err := fmt.Fprint(w.context.Writer, ": heartbeat\n\n")
				w.context.Writer.Flush()
				w.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
}

func (w *digitalHumanSSEWriter) write(id, event string, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	payload, err := json.Marshal(gin.H{"event": event, "data": data})
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w.context.Writer, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w.context.Writer, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	w.context.Writer.Flush()
	return nil
}
