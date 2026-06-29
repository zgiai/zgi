package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	agentspkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestAgentHistoryDispatchRequiresWorkflowLogsViewForBuilderHistory(t *testing.T) {
	agentID := uuid.New()
	workspaceID := uuid.New()
	endpoints := []struct {
		name   string
		method func(*AgentHistoryDispatchHandler, *gin.Context)
		path   string
		params gin.Params
	}{
		{
			name:   "conversations",
			method: (*AgentHistoryDispatchHandler).GetConversations,
			path:   "/agents/" + agentID.String() + "/conversations",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}},
		},
		{
			name:   "conversation detail",
			method: (*AgentHistoryDispatchHandler).GetConversationDetail,
			path:   "/agents/" + agentID.String() + "/conversations/" + uuid.NewString(),
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "conversation_id", Value: uuid.NewString()}},
		},
		{
			name:   "chat messages",
			method: (*AgentHistoryDispatchHandler).GetChatMessages,
			path:   "/agents/" + agentID.String() + "/chat-messages",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}},
		},
		{
			name:   "runtime logs",
			method: (*AgentHistoryDispatchHandler).GetRuntimeLogs,
			path:   "/agents/" + agentID.String() + "/runtime-logs",
			params: gin.Params{{Key: "agent_id", Value: agentID.String()}},
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			permissionChecker := &agentHistoryPermissionChecker{allowed: false}
			handler := NewAgentHistoryDispatchHandler(
				&agentHistoryAgentRepo{agent: &agentspkg.Agent{
					ID:         agentID,
					TenantID:   workspaceID,
					AgentsType: "WORKFLOW",
				}},
				&WorkflowHandler{
					agentWorkspaceResolver:     &agentHistoryWorkspaceResolver{workspaceID: workspaceID.String()},
					workspacePermissionChecker: permissionChecker,
				},
				nil,
				nil,
				nil,
			)
			ctx, recorder := newAgentHistoryPermissionContext(http.MethodGet, endpoint.path, endpoint.params)

			endpoint.method(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if !permissionChecker.checked {
				t.Fatalf("expected workflow.logs.view permission check")
			}
			if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowLogsView {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowLogsView)
			}
		})
	}
}

func TestAgentHistoryDispatchRuntimeLogsRequiresWorkflowLogsViewBeforeBindingRequest(t *testing.T) {
	agentID := uuid.New()
	workspaceID := uuid.New()
	permissionChecker := &agentHistoryPermissionChecker{allowed: false}
	handler := NewAgentHistoryDispatchHandler(
		&agentHistoryAgentRepo{agent: &agentspkg.Agent{
			ID:         agentID,
			TenantID:   workspaceID,
			AgentsType: "WORKFLOW",
		}},
		&WorkflowHandler{
			agentWorkspaceResolver:     &agentHistoryWorkspaceResolver{workspaceID: workspaceID.String()},
			workspacePermissionChecker: permissionChecker,
		},
		nil,
		nil,
		nil,
	)

	ctx, recorder := newAgentHistoryPermissionContext(
		http.MethodPost,
		"/agents/"+agentID.String()+"/runtime-logs",
		gin.Params{{Key: "agent_id", Value: agentID.String()}},
	)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID.String()+"/runtime-logs", strings.NewReader(`{"broken":`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.GetRuntimeLogs(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check before request binding")
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowLogsView {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowLogsView)
	}
}

func TestAgentHistoryDispatchRuntimeAgentRequiresAgentLogsView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()
	organizationID := uuid.New()

	endpoints := []struct {
		name       string
		method     func(*AgentHistoryDispatchHandler, *gin.Context)
		methodName string
		path       string
		params     gin.Params
		body       string
	}{
		{
			name:       "conversations",
			method:     (*AgentHistoryDispatchHandler).GetConversations,
			methodName: http.MethodGet,
			path:       "/agents/" + agentID.String() + "/conversations",
			params:     gin.Params{{Key: "agent_id", Value: agentID.String()}},
		},
		{
			name:       "conversation detail",
			method:     (*AgentHistoryDispatchHandler).GetConversationDetail,
			methodName: http.MethodGet,
			path:       "/agents/" + agentID.String() + "/conversations/" + uuid.NewString(),
			params:     gin.Params{{Key: "agent_id", Value: agentID.String()}, {Key: "conversation_id", Value: uuid.NewString()}},
		},
		{
			name:       "chat messages",
			method:     (*AgentHistoryDispatchHandler).GetChatMessages,
			methodName: http.MethodGet,
			path:       "/agents/" + agentID.String() + "/chat-messages",
			params:     gin.Params{{Key: "agent_id", Value: agentID.String()}},
		},
		{
			name:       "runtime logs",
			method:     (*AgentHistoryDispatchHandler).GetRuntimeLogs,
			methodName: http.MethodPost,
			path:       "/agents/" + agentID.String() + "/runtime-logs",
			params:     gin.Params{{Key: "agent_id", Value: agentID.String()}},
			body:       `{"broken":`,
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			service := newFakeRuntimeHistoryService()
			service.addConversation(&runtimemodel.Conversation{
				ID:             uuid.New(),
				OrganizationID: organizationID,
				WorkspaceID:    &workspaceID,
				AccountID:      accountID,
				CallerType:     runtimemodel.ConversationCallerAgent,
				CallerID:       &agentID,
				Source:         runtimemodel.ConversationSourceConsole,
			})
			permissionChecker := &agentHistoryPermissionChecker{allowed: false}
			handler := NewAgentHistoryDispatchHandler(
				&agentHistoryAgentRepo{agent: &agentspkg.Agent{
					ID:         agentID,
					TenantID:   workspaceID,
					AgentsType: "AGENT",
				}},
				&WorkflowHandler{workspacePermissionChecker: permissionChecker},
				nil,
				nil,
				service,
			)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(endpoint.methodName, endpoint.path, strings.NewReader(endpoint.body))
			ctx.Params = endpoint.params
			ctx.Set("account_id", accountID.String())
			util.SetOrganizationID(ctx, organizationID.String())

			endpoint.method(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if !permissionChecker.checked {
				t.Fatalf("expected agent.logs.view permission check")
			}
			if permissionChecker.lastPermission != workspace_model.WorkspacePermissionAgentLogsView {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionAgentLogsView)
			}
		})
	}
}

func TestAgentHistoryDispatchRuntimeConversationsAreCallerScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	visibleConversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             visibleConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	otherAgentID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      uuid.New(),
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/conversations", nil)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())

	handler.GetConversations(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["id"]; got != visibleConversationID.String() {
		t.Fatalf("conversation id = %#v, want %s", got, visibleConversationID.String())
	}
}

func TestAgentHistoryDispatchRuntimeConversationDetailRejectsOtherAgentConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	otherAgentID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/conversations/"+conversationID.String(), nil)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: conversationID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())

	handler.GetConversationDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentHistoryDispatchRuntimeChatMessagesRejectsOtherAccountConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      uuid.New(),
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: conversationID,
		Query:          "secret",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/chat-messages?conversation_id="+conversationID.String(), nil)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())

	handler.GetChatMessages(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentHistoryDispatchRuntimeLogsAreCallerScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	visibleConversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             visibleConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	visibleMessageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             visibleMessageID,
		ConversationID: visibleConversationID,
		Query:          "visible",
		Answer:         "allowed",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	otherAgentID := uuid.New()
	otherAgentConversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             otherAgentConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: otherAgentConversationID,
		Query:          "other agent",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	otherAccountConversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             otherAccountConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      uuid.New(),
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: otherAccountConversationID,
		Query:          "other account",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+ids.agentID.String()+"/runtime-logs", strings.NewReader(`{"page":1,"limit":20}`))
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())

	handler.GetRuntimeLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["message_id"]; got != visibleMessageID.String() {
		t.Fatalf("message id = %#v, want %s", got, visibleMessageID.String())
	}
}

func newAgentHistoryPermissionContext(method, target string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = params
	ctx.Set("account_id", "account-1")
	util.SetOrganizationID(ctx, "org-1")
	return ctx, recorder
}

type agentHistoryAgentRepo struct {
	agent *agentspkg.Agent
}

func (r *agentHistoryAgentRepo) GetByID(_ context.Context, id string) (*agentspkg.Agent, error) {
	if r.agent != nil && r.agent.ID.String() == id {
		return r.agent, nil
	}
	return nil, errWorkflowRunNotFoundOrDenied
}

type agentHistoryWorkspaceResolver struct {
	workspaceID string
}

func (r *agentHistoryWorkspaceResolver) GetAgentWorkspaceID(_ context.Context, _ string) (string, error) {
	return r.workspaceID, nil
}

type agentHistoryPermissionChecker struct {
	allowed        bool
	checked        bool
	lastPermission workspace_model.WorkspacePermissionCode
}

func (c *agentHistoryPermissionChecker) CheckWorkspacePermission(_ context.Context, _, _, _ string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastPermission = permissionCode
	return c.allowed, nil
}
