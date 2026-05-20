package workflowtest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/stretchr/testify/require"
)

type handlerWorkflowServiceStub struct {
	interfaces.WorkflowService
	workspaceID string
}

func (s *handlerWorkflowServiceStub) GetAgentWorkspaceID(ctx context.Context, agentID string) (string, error) {
	return s.workspaceID, nil
}

type handlerLLMClientStub struct {
	llmclient.LLMClient
	appCtx *llmclient.AppContext
	req    *adapter.ChatRequest
}

func (s *handlerLLMClientStub) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	s.appCtx = appCtx
	s.req = req
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Content: `{"scenarios":[{"name":"订单查询","description":"用户查询订单状态"}],"assignments":[]}`},
		}},
	}, nil
}

func TestHandlerGetSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := setupTestService(t)
	handler := NewHandler(service, nil)
	agentID := uuid.NewString()

	router := gin.New()
	router.GET("/agents/:agent_id/workflow-tests/settings", handler.GetSettings)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/workflow-tests/settings", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code string  `json:"code"`
		Data Setting `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "0", body.Code)
	require.Equal(t, agentID, body.Data.AgentID)
	require.Equal(t, DefaultJudgePromptTemplate, body.Data.JudgePromptTemplate)
}

func TestHandlerRecognizeScenariosUsesAgentWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := setupTestService(t)
	agentID := uuid.NewString()
	workspaceID := uuid.NewString()
	llm := &handlerLLMClientStub{}
	handler := NewHandler(service, &handlerWorkflowServiceStub{workspaceID: workspaceID}, llm)

	router := gin.New()
	router.POST("/agents/:agent_id/workflow-tests/scenarios/recognize", func(c *gin.Context) {
		c.Set("account_id", "account-1")
		handler.RecognizeScenarios(c)
	})

	body := []byte(`{"context":"基本llm","model":{"provider":"openai","name":"chatgpt-4o-latest"}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/workflow-tests/scenarios/recognize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, llm.appCtx)
	require.Equal(t, workspaceID, llm.appCtx.WorkspaceID)
	require.Equal(t, llmclient.BillingSubjectTypeWorkspace, llm.appCtx.BillingSubjectType)
	require.NotNil(t, llm.req)
	require.Equal(t, "openai", llm.req.Provider)
	require.Equal(t, "chatgpt-4o-latest", llm.req.Model)
}
