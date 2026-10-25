package agents

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

func TestExternalChatWaitingOnQuestionReturnsConflictWithContinuationIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	messageID := uuid.New()
	err := &runtimeservice.ConversationWaitingError{
		Cause:     runtimeservice.ErrConversationWaitingQuestion,
		MessageID: messageID,
		Status:    "waiting_question",
		RequestID: "req-1",
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/chat", nil)
	c.Set("invoke_from", string(runtimemodel.ConversationSourceExternalAPI))
	(&AgentsHandler{}).failRuntime(c, err)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	var body struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if body.Code != agentConversationWaitingCode || body.Data["status"] != "waiting_question" ||
		body.Data["message_id"] != messageID.String() || body.Data["request_id"] != "req-1" {
		t.Fatalf("body = %+v", body)
	}

	// The console keeps its existing envelope.
	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/console/api/agents/chat", nil)
	(&AgentsHandler{}).failRuntime(c, err)
	if recorder.Code == http.StatusConflict {
		t.Fatal("console chat changed to 409")
	}
}
