package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
)

type agentModelPrecheckServiceFake struct {
	interfaces.AgentsService
	draft      *dto.AgentDraftRuntimeConfigResponse
	published  *dto.AgentWebAppRuntimeConfigResponse
	capability *dto.AgentWebAppRuntimeCapabilityResponse
}

func (f *agentModelPrecheckServiceFake) RequireAgentManageAccess(context.Context, string, string) error {
	return nil
}

func (f *agentModelPrecheckServiceFake) GetAgentDraftRuntimeConfig(context.Context, string, string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	return f.draft, nil
}

func (f *agentModelPrecheckServiceFake) GetPublishedAgentWebAppConfig(context.Context, string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	return f.published, nil
}

func (f *agentModelPrecheckServiceFake) GetWebAppRuntimeCapability(context.Context, string, string, bool) (*dto.AgentWebAppRuntimeCapabilityResponse, error) {
	return f.capability, nil
}

type agentModelPrecheckerFake struct {
	result *llmclient.AppModelPrecheckResult
	err    error
	appCtx *llmclient.AppContext
	models []llmclient.AppModelRef
}

func (f *agentModelPrecheckerFake) PrecheckAppModels(
	_ context.Context,
	appCtx *llmclient.AppContext,
	models []llmclient.AppModelRef,
) (*llmclient.AppModelPrecheckResult, error) {
	f.appCtx = appCtx
	f.models = append([]llmclient.AppModelRef(nil), models...)
	return f.result, f.err
}

func TestPrecheckAgentDraftModelUsesSelectedModelAndAgentScope(t *testing.T) {
	accountID := uuid.NewString()
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	agentID := uuid.NewString()
	service := &agentModelPrecheckServiceFake{draft: &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     agentID,
		WorkspaceID: workspaceID,
	}}
	prechecker := &agentModelPrecheckerFake{result: &llmclient.AppModelPrecheckResult{
		Status: llmclient.AppModelPrecheckStatusWarning,
		Warnings: []llmclient.AppModelPrecheckWarning{{
			Kind:   llmclient.AppModelPrecheckWarningPrivateChannelUpstreamBalanceLow,
			Reason: "balance_low",
			Scope:  llmclient.AppModelPrecheckWarningScopeAll,
		}},
	}}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	handler.SetModelPrechecker(prechecker)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", accountID)
		util.SetOrganizationID(c, organizationID)
		c.Next()
	})
	router.POST("/agents/:agent_id/runtime/model-precheck", handler.PrecheckAgentDraftModel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/agents/"+agentID+"/runtime/model-precheck",
		bytes.NewBufferString(`{"provider":"deepseek","model":"deepseek-chat"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	responseBody := decodeAgentModelPrecheckResponse(t, recorder)
	if recorder.Code != http.StatusOK || responseBody.Code != "0" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if responseBody.Data.Status != llmclient.AppModelPrecheckStatusWarning || len(responseBody.Data.Warnings) != 1 {
		t.Fatalf("precheck = %#v, want one warning", responseBody.Data)
	}
	if len(prechecker.models) != 1 || prechecker.models[0].Provider != "deepseek" || prechecker.models[0].Model != "deepseek-chat" {
		t.Fatalf("models = %#v, want selected DeepSeek model", prechecker.models)
	}
	if prechecker.appCtx == nil ||
		prechecker.appCtx.OrganizationID != organizationID ||
		prechecker.appCtx.WorkspaceID != workspaceID ||
		prechecker.appCtx.AccountID != accountID ||
		prechecker.appCtx.AppID != agentID ||
		prechecker.appCtx.AppType != "agent" ||
		prechecker.appCtx.ModelUseCase != agentModelSelectionUseCase ||
		prechecker.appCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("app context = %#v, want draft agent billing scope", prechecker.appCtx)
	}
}

func TestPrecheckPublishedAgentModelUsesPublishedConfig(t *testing.T) {
	accountID := uuid.NewString()
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	agentID := uuid.NewString()
	webAppID := uuid.NewString()
	service := &agentModelPrecheckServiceFake{
		capability: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true},
		published: &dto.AgentWebAppRuntimeConfigResponse{
			AgentID:        agentID,
			WebAppID:       webAppID,
			WorkspaceID:    workspaceID,
			OrganizationID: organizationID,
			Config: dto.AgentConfigResponse{
				ModelProvider: "qwen",
				Model:         "qwen-plus",
			},
		},
	}
	prechecker := &agentModelPrecheckerFake{result: &llmclient.AppModelPrecheckResult{
		Status:   llmclient.AppModelPrecheckStatusOK,
		Warnings: []llmclient.AppModelPrecheckWarning{},
	}}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	handler.SetModelPrechecker(prechecker)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", accountID)
		c.Set("is_authenticated", true)
		c.Next()
	})
	router.POST("/webapps/:web_app_id/runtime/model-precheck", handler.PrecheckPublishedAgentModel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webapps/"+webAppID+"/runtime/model-precheck", nil)
	router.ServeHTTP(recorder, request)

	responseBody := decodeAgentModelPrecheckResponse(t, recorder)
	if recorder.Code != http.StatusOK || responseBody.Code != "0" || responseBody.Data.Status != llmclient.AppModelPrecheckStatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(prechecker.models) != 1 || prechecker.models[0].Provider != "qwen" || prechecker.models[0].Model != "qwen-plus" {
		t.Fatalf("models = %#v, want published Qwen model", prechecker.models)
	}
	if prechecker.appCtx == nil ||
		prechecker.appCtx.OrganizationID != organizationID ||
		prechecker.appCtx.WorkspaceID != workspaceID ||
		prechecker.appCtx.AccountID != accountID ||
		prechecker.appCtx.AppID != agentID ||
		prechecker.appCtx.AppType != "agent" ||
		prechecker.appCtx.ModelUseCase != agentModelSelectionUseCase {
		t.Fatalf("app context = %#v, want published agent billing scope", prechecker.appCtx)
	}
}

func TestPrecheckPublishedAgentModelDegradesFailureWithoutBlocking(t *testing.T) {
	service := &agentModelPrecheckServiceFake{
		capability: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true},
		published: &dto.AgentWebAppRuntimeConfigResponse{
			AgentID:        uuid.NewString(),
			WebAppID:       uuid.NewString(),
			WorkspaceID:    uuid.NewString(),
			OrganizationID: uuid.NewString(),
			Config: dto.AgentConfigResponse{
				ModelProvider: "deepseek",
				Model:         "deepseek-chat",
			},
		},
	}
	prechecker := &agentModelPrecheckerFake{err: errors.New("precheck unavailable")}
	handler := NewAgentsHandler(service, nil, nil, nil, nil)
	handler.SetModelPrechecker(prechecker)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("account_id", uuid.NewString())
		c.Set("is_authenticated", false)
		c.Next()
	})
	router.POST("/webapps/:web_app_id/runtime/model-precheck", handler.PrecheckPublishedAgentModel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webapps/"+service.published.WebAppID+"/runtime/model-precheck", nil)
	router.ServeHTTP(recorder, request)

	responseBody := decodeAgentModelPrecheckResponse(t, recorder)
	if recorder.Code != http.StatusOK || responseBody.Code != "0" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if responseBody.Data.Status != llmclient.AppModelPrecheckStatusUnknown || len(responseBody.Data.Warnings) != 0 {
		t.Fatalf("precheck = %#v, want unknown with no warning", responseBody.Data)
	}
}

type agentModelPrecheckTestResponse struct {
	Code string `json:"code"`
	Data struct {
		Status   llmclient.AppModelPrecheckStatus `json:"status"`
		Warnings []struct {
			Kind   llmclient.AppModelPrecheckWarningKind  `json:"kind"`
			Reason string                                 `json:"reason"`
			Scope  llmclient.AppModelPrecheckWarningScope `json:"scope"`
		} `json:"warnings"`
	} `json:"data"`
}

func decodeAgentModelPrecheckResponse(t *testing.T, recorder *httptest.ResponseRecorder) agentModelPrecheckTestResponse {
	t.Helper()
	var body agentModelPrecheckTestResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}
