package agents

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestAgentSSEWriterHeartbeatIsTransportOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/stream", nil)
	w := newAgentSSEWriter(context)

	if err := w.writeHeartbeat(); err != nil {
		t.Fatalf("writeHeartbeat: %v", err)
	}
	if err := w.WriteEvent("event-1", "message_chunk", gin.H{"answer": "ok"}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	body := recorder.Body.String()
	if !strings.HasPrefix(body, ": heartbeat\n\n") {
		t.Fatalf("stream body = %q, want heartbeat comment prefix", body)
	}
	if strings.Contains(body, "event: heartbeat") {
		t.Fatalf("heartbeat must not be emitted as a business event: %q", body)
	}
	for _, expected := range []string{"id: event-1", "event: message_chunk", `"answer":"ok"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("stream body = %q, want %q", body, expected)
		}
	}
}

func TestSetupPreparedAgentSSESendsRuntimeIdentityBeforeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	ginContext.Request = httptest.NewRequest("GET", "/stream", nil).WithContext(requestContext)
	conversationID := uuid.New()
	messageID := uuid.New()

	setupPreparedAgentSSE(ginContext, &runtimeservice.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	})

	if got := recorder.Header().Get(agentSSEConversationIDHeader); got != conversationID.String() {
		t.Fatalf("%s = %q, want %q", agentSSEConversationIDHeader, got, conversationID.String())
	}
	if got := recorder.Header().Get(agentSSEMessageIDHeader); got != messageID.String() {
		t.Fatalf("%s = %q, want %q", agentSSEMessageIDHeader, got, messageID.String())
	}
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestWriteAgentChatEndRedactsPrivateMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	prepared := &runtimeservice.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
	}
	metadata := map[string]interface{}{
		"agent_transcript_version": 1,
		"agent_transcript":         []interface{}{map[string]interface{}{"role": "tool", "content": "private tool result"}},
		"model_invocations":        []interface{}{map[string]interface{}{"request": "private prompt"}},
		"skill_invocations":        []interface{}{map[string]interface{}{"kind": "tool_call", "tool_name": "search"}},
	}

	writeAgentChatEnd(ginContext, prepared, &runtimeservice.ChatResult{
		Status:   runtimemodel.MessageStatusCompleted,
		Metadata: metadata,
	})

	body := recorder.Body.String()
	for _, privateValue := range []string{"agent_transcript", "private tool result", "model_invocations\"", "private prompt"} {
		if strings.Contains(body, privateValue) {
			t.Fatalf("message_end exposed %q: %s", privateValue, body)
		}
	}
	for _, publicValue := range []string{"skill_invocations", "search", "model_invocations_redacted", "model_invocation_count"} {
		if !strings.Contains(body, publicValue) {
			t.Fatalf("message_end lost %q: %s", publicValue, body)
		}
	}
	if _, ok := metadata["agent_transcript"]; !ok {
		t.Fatalf("SSE redaction mutated durable metadata: %#v", metadata)
	}
}
