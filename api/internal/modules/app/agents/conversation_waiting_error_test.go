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
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
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

func TestExternalChatWaitingMessageFollowsTheRequestLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	waiting := &runtimeservice.ConversationWaitingError{Cause: runtimeservice.ErrConversationWaitingQuestion, MessageID: uuid.New(), Status: "waiting_question", RequestID: "req-1"}
	for locale, want := range map[string]string{
		"zh-Hans": "会话正在等待回答一个问题，请先回答再继续。",
		"en-US":   "The conversation is waiting for an answer to its question. Answer it to continue.",
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/chat", nil)
		c.Request.Header.Set("Accept-Language", locale)
		c.Set("invoke_from", string(runtimemodel.ConversationSourceExternalAPI))
		(&AgentsHandler{appErrorProjector: projector}).failRuntime(c, waiting)

		var body struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Data    map[string]string `json:"data"`
		}
		if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if recorder.Code != http.StatusConflict || body.Code != agentConversationWaitingCode || body.Message != want || body.Data["request_id"] != "req-1" {
			t.Fatalf("%s: status %d, body %+v", locale, recorder.Code, body)
		}
	}
}
