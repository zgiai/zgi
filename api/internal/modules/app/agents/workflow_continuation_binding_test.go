package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
)

func TestWorkflowContinuationUnavailableBindingDeniedAcrossAgentSurfaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, surface := range []string{"draft", "webapp", "api"} {
		for _, continuationType := range []string{"approval", "question_answer"} {
			t.Run(surface+"/"+continuationType, func(t *testing.T) {
				ids := webAppRuntimePermissionIDs{
					organizationID: uuid.New(),
					workspaceID:    uuid.New(),
					accountID:      uuid.New(),
					agentID:        uuid.New(),
					webAppID:       uuid.New(),
					conversationID: uuid.New(),
					messageID:      uuid.New(),
				}
				runtimeSvc := &webAppRuntimePermissionService{
					enforceContinuationBinding: true,
					continuationBindingID:      "removed-binding",
					continuationAgentID:        uuid.NewString(),
				}
				appService := newAgentRuntimePermissionAppService(ids)
				if surface == "webapp" {
					appService = nil
				}
				var handler *AgentsHandler
				if surface == "webapp" {
					handler = NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)
				} else {
					handler = NewAgentsHandler(appService, nil, nil, nil, nil, runtimeSvc)
				}
				runner := &deniedWorkflowContinuationRunner{}
				handler.SetWorkflowContinuationRunner(runner)

				body := `{"type":"approval","action":"approve","approval_token":"token-1"}`
				if continuationType == "question_answer" {
					body = `{"type":"question_answer","inputs":{"query":"answer"}}`
				}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, "/workflow-continuation", bytes.NewBufferString(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Params = gin.Params{
					{Key: "agent_id", Value: ids.agentID.String()},
					{Key: "web_app_id", Value: ids.webAppID.String()},
					{Key: "conversation_id", Value: ids.conversationID.String()},
					{Key: "message_id", Value: ids.messageID.String()},
				}
				c.Set("account_id", ids.accountID.String())
				c.Set("organization_id", ids.organizationID.String())
				c.Set("workspace_id", ids.workspaceID.String())
				c.Set("agent_id", ids.agentID.String())
				c.Set("is_authenticated", true)

				switch surface {
				case "draft":
					handler.ContinueAgentRuntimeWorkflowApproval(c)
				case "webapp":
					handler.ContinueWebAppAgentRuntimeWorkflowApproval(c)
				case "api":
					handler.ContinueAPIKeyAgentRuntimeWorkflowContinuation(c)
				}

				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
				}
				var responseBody map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &responseBody); err != nil {
					t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
				}
				if responseBody["code"] != agentWorkflowBindingUnavailableCode {
					t.Fatalf("response code = %#v, want %q", responseBody["code"], agentWorkflowBindingUnavailableCode)
				}
				data, _ := responseBody["data"].(map[string]interface{})
				if data["status"] != "denied" || data["reason"] != "unavailable" {
					t.Fatalf("response data = %#v, want stable denied/unavailable", data)
				}
				if !runtimeSvc.beginContinuationCalled {
					t.Fatal("BeginWorkflowApprovalContinuation was not called")
				}
				if len(runtimeSvc.lastRunConfig.WorkflowBindings) != 0 {
					t.Fatalf("reloaded workflow bindings = %#v, want removed binding absent", runtimeSvc.lastRunConfig.WorkflowBindings)
				}
				if runner.resumeApprovalCalled || runner.resumeQuestionCalled {
					t.Fatalf("workflow resumed after binding denial: approval=%v question=%v", runner.resumeApprovalCalled, runner.resumeQuestionCalled)
				}
				if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
					t.Fatalf("content type = %q, should not start SSE after binding denial", got)
				}
			})
		}
	}
}

type deniedWorkflowContinuationRunner struct {
	resumeApprovalCalled bool
	resumeQuestionCalled bool
}

func (r *deniedWorkflowContinuationRunner) ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error {
	r.resumeApprovalCalled = true
	return nil
}

func (r *deniedWorkflowContinuationRunner) ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error {
	r.resumeQuestionCalled = true
	return nil
}

func (*deniedWorkflowContinuationRunner) StopWorkflowContinuation(context.Context, string, string) error {
	return nil
}

var _ interface {
	ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error
	ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error
	StopWorkflowContinuation(context.Context, string, string) error
} = (*deniedWorkflowContinuationRunner)(nil)

var _ runtimeservice.Service = (*webAppRuntimePermissionService)(nil)
