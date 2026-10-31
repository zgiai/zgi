package agents

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

type interruptedResumeRuntimeService struct {
	*webAppRuntimePermissionService
	called         bool
	conversationID uuid.UUID
	messageID      uuid.UUID
	err            error
}

func (s *interruptedResumeRuntimeService) RunConfiguredInterruptedTurnResume(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, conversationID, messageID uuid.UUID, _ func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	s.called = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastRunConfig = config
	s.conversationID = conversationID
	s.messageID = messageID
	return nil, s.err
}

func runAPIKeyInterruptedResume(t *testing.T, runtimeSvc *interruptedResumeRuntimeService, ids webAppRuntimePermissionIDs) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/resume?user=end-user-1", nil)
	c.Params = gin.Params{
		{Key: "conversation_id", Value: ids.conversationID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
	}
	setAPIKeyAgentTestContext(c, ids)
	handler.ResumeAPIKeyAgentRuntimeInterruptedTurn(c)
	return w
}

// An external client resumes an interrupted turn as the same external caller
// that started it (ADR 0042).
func TestResumeAPIKeyAgentRuntimeInterruptedTurnUsesPublishedExternalScope(t *testing.T) {
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(),
	}
	runtimeSvc := &interruptedResumeRuntimeService{webAppRuntimePermissionService: &webAppRuntimePermissionService{}}
	w := runAPIKeyInterruptedResume(t, runtimeSvc, ids)

	if !runtimeSvc.called || runtimeSvc.conversationID != ids.conversationID || runtimeSvc.messageID != ids.messageID {
		t.Fatalf("resume called = %v for %s/%s", runtimeSvc.called, runtimeSvc.conversationID, runtimeSvc.messageID)
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastRunConfig.SystemPromptVersion != "agent.published.v1" {
		t.Fatalf("SystemPromptVersion = %q, want agent.published.v1", runtimeSvc.lastRunConfig.SystemPromptVersion)
	}
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", w.Header().Get("Content-Type"))
	}
}

// A turn that cannot be resumed is reported with a stable code, and other
// errors are not exposed.
func TestResumeAPIKeyAgentRuntimeInterruptedTurnReportsNotResumable(t *testing.T) {
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(),
	}
	runtimeSvc := &interruptedResumeRuntimeService{
		webAppRuntimePermissionService: &webAppRuntimePermissionService{},
		err:                            fmt.Errorf("%w: was resumed once already", runtimeservice.ErrInterruptedTurnNotResumable),
	}
	body := runAPIKeyInterruptedResume(t, runtimeSvc, ids).Body.String()
	if !strings.Contains(body, agentInterruptedTurnNotResumableCode) {
		t.Fatalf("stream = %s, want the not-resumable code", body)
	}

	runtimeSvc.err = fmt.Errorf("redis password leaked by internal error")
	body = runAPIKeyInterruptedResume(t, runtimeSvc, ids).Body.String()
	if strings.Contains(body, "redis password") || !strings.Contains(body, `"code":399001`) {
		t.Fatalf("resume stream error was not sanitized: %s", body)
	}
}
