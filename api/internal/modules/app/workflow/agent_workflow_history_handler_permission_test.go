package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
)

func TestAgentWorkflowHistoryChatMessagesRejectsOtherAgentConversationBeforeMessageQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	agentID := uuid.New()
	conversationID := uuid.New()
	conversationSvc := &agentWorkflowHistoryConversationService{
		err: errors.New("conversation not found"),
	}
	messageSvc := &agentWorkflowHistoryMessageService{}
	handler := NewAgentWorkflowHistoryHandler(conversationSvc, messageSvc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+agentID.String()+"/chat-messages?conversation_id="+conversationID.String(), nil)
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID.String()}}

	handler.GetChatMessages(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if !conversationSvc.called {
		t.Fatalf("GetConversationByIDAndAgent should be called before message query")
	}
	if conversationSvc.lastConversationID != conversationID || conversationSvc.lastAgentID != agentID {
		t.Fatalf("conversation scope = conversation:%s agent:%s, want conversation:%s agent:%s",
			conversationSvc.lastConversationID, conversationSvc.lastAgentID, conversationID, agentID)
	}
	if messageSvc.called {
		t.Fatalf("GetMessagesByConversation should not be called after conversation scope denial")
	}
}

type agentWorkflowHistoryConversationService struct {
	called             bool
	lastConversationID uuid.UUID
	lastAgentID        uuid.UUID
	err                error
}

func (s *agentWorkflowHistoryConversationService) GetConversationByIDAndAgent(_ context.Context, id, agentID uuid.UUID) (*conversation.AgentConversation, error) {
	s.called = true
	s.lastConversationID = id
	s.lastAgentID = agentID
	if s.err != nil {
		return nil, s.err
	}
	return &conversation.AgentConversation{
		ID:      id,
		AgentID: agentID,
	}, nil
}

func (s *agentWorkflowHistoryConversationService) GetConversationHistoryByAgent(_ context.Context, filter conversation.AgentConversationHistoryFilter) ([]*conversation.AgentConversation, int64, error) {
	return []*conversation.AgentConversation{
		{
			ID:      uuid.New(),
			AgentID: filter.AgentID,
		},
	}, 1, nil
}

type agentWorkflowHistoryMessageService struct {
	called bool
}

func (s *agentWorkflowHistoryMessageService) GetMessagesByConversation(_ context.Context, _ uuid.UUID, _, _ int) ([]*conversation.AgentMessage, int64, error) {
	s.called = true
	return []*conversation.AgentMessage{}, 0, nil
}
