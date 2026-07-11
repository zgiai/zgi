package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	agentspkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestAgentRuntimeRunsReturnsOnlyNewWebAppMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	webAppConversationID := uuid.New()
	consoleConversationID := uuid.New()
	legacyWebAppConversationID := uuid.New()
	webAppID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             webAppConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "webapp",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             consoleConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "console",
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             legacyWebAppConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "legacy webapp",
		Source:         runtimemodel.ConversationSourceWebApp,
	})
	webAppMessageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             webAppMessageID,
		ConversationID: webAppConversationID,
		Query:          "hello",
		Answer:         "world",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		Metadata: map[string]interface{}{
			"elapsed_time_ms":       123.0,
			"system_prompt_version": "agent.v1",
			"usage":                 map[string]interface{}{"total_tokens": 9.0},
			"model_invocations": []interface{}{
				map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 3.0}},
				map[string]interface{}{"total_tokens": 4.0},
			},
			"skill_step_count": 1.0,
		},
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: consoleConversationID,
		Query:          "console",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: legacyWebAppConversationID,
		Query:          "legacy",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs?triggered_from=web-app", nil)

	handler.GetRuntimeRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["id"]; got != webAppMessageID.String() {
		t.Fatalf("id = %#v, want %s", got, webAppMessageID.String())
	}
	if got := items[0]["source"]; got != runtimemodel.ConversationSourceWebApp {
		t.Fatalf("source = %#v, want %s", got, runtimemodel.ConversationSourceWebApp)
	}
	if got := items[0]["query"]; got != "hello" {
		t.Fatalf("query = %#v, want hello", got)
	}
	if got := items[0]["answer_preview"]; got != "world" {
		t.Fatalf("answer_preview = %#v, want world", got)
	}
	if got := items[0]["total_tokens"]; got != float64(7) {
		t.Fatalf("total_tokens = %#v, want sum of model invocations 7", got)
	}
}

func TestAgentRuntimeRunsIncludesWebAppMessagesFromEndUserAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	endUserAccountID := uuid.New()
	webAppID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      endUserAccountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "webapp end user",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "from visitor",
		Answer:         "visible to manager",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs?triggered_from=web-app", nil)

	handler.GetRuntimeRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 || items[0]["id"] != messageID.String() {
		t.Fatalf("items = %#v, want end-user webapp message %s", items, messageID.String())
	}
}

func TestAgentRuntimeRunsCanFilterByConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	firstConversationID := uuid.New()
	secondConversationID := uuid.New()
	consoleConversationID := uuid.New()
	webAppID := uuid.New()
	for _, conversation := range []*runtimemodel.Conversation{
		{
			ID:             firstConversationID,
			OrganizationID: ids.organizationID,
			WorkspaceID:    &ids.workspaceID,
			AccountID:      ids.accountID,
			CallerType:     runtimemodel.ConversationCallerAgent,
			CallerID:       &ids.agentID,
			Source:         runtimemodel.ConversationSourceWebApp,
			SourceWebAppID: &webAppID,
		},
		{
			ID:             secondConversationID,
			OrganizationID: ids.organizationID,
			WorkspaceID:    &ids.workspaceID,
			AccountID:      ids.accountID,
			CallerType:     runtimemodel.ConversationCallerAgent,
			CallerID:       &ids.agentID,
			Source:         runtimemodel.ConversationSourceWebApp,
			SourceWebAppID: &webAppID,
		},
		{
			ID:             consoleConversationID,
			OrganizationID: ids.organizationID,
			WorkspaceID:    &ids.workspaceID,
			AccountID:      ids.accountID,
			CallerType:     runtimemodel.ConversationCallerAgent,
			CallerID:       &ids.agentID,
			Source:         runtimemodel.ConversationSourceConsole,
		},
	} {
		service.addConversation(conversation)
	}
	firstMessageID := uuid.New()
	secondMessageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             firstMessageID,
		ConversationID: firstConversationID,
		Query:          "first question",
		Answer:         "first answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             secondMessageID,
		ConversationID: secondConversationID,
		Query:          "second question",
		Answer:         "second answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: consoleConversationID,
		Query:          "console question",
		Answer:         "console answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs?triggered_from=web-app&conversation_id="+secondConversationID.String(), nil)

	handler.GetRuntimeRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["id"]; got != secondMessageID.String() {
		t.Fatalf("id = %#v, want %s", got, secondMessageID.String())
	}
	if got := items[0]["conversation_id"]; got != secondConversationID.String() {
		t.Fatalf("conversation_id = %#v, want %s", got, secondConversationID.String())
	}
	if got := items[0]["query"]; got != "second question" {
		t.Fatalf("query = %#v, want second question", got)
	}
}

func TestAgentRuntimeRunsCanFilterConsoleAndKeyword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	webAppConversationID := uuid.New()
	consoleConversationID := uuid.New()
	webAppID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             webAppConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             consoleConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceConsole,
	})
	consoleMessageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: webAppConversationID,
		Query:          "deploy plan",
		Answer:         "webapp answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             consoleMessageID,
		ConversationID: consoleConversationID,
		Query:          "debug request",
		Answer:         "contains special reply",
		Status:         runtimemodel.MessageStatusCompleted,
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: consoleConversationID,
		Query:          "another debug",
		Answer:         "ordinary answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs?source=console&q=special", nil)

	handler.GetRuntimeRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["id"]; got != consoleMessageID.String() {
		t.Fatalf("id = %#v, want %s", got, consoleMessageID.String())
	}
	if got := items[0]["source"]; got != runtimemodel.ConversationSourceConsole {
		t.Fatalf("source = %#v, want %s", got, runtimemodel.ConversationSourceConsole)
	}
}

func TestAgentRuntimeLogRoutesRequireAgentViewPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := runtimeHistoryIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	messageID := uuid.New()
	service := newFakeRuntimeHistoryService()
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewAgentRuntimeLogsHandler(
		&fakeRuntimeAgentRepo{agent: &agentspkg.Agent{
			ID:         ids.agentID,
			TenantID:   ids.workspaceID,
			Name:       "agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		}},
		service,
		permissionChecker,
	)

	endpoints := []struct {
		name   string
		path   string
		params gin.Params
		call   func(*gin.Context)
	}{
		{
			name:   "runtime runs",
			path:   "/agents/" + ids.agentID.String() + "/runtime-runs",
			params: gin.Params{{Key: "agent_id", Value: ids.agentID.String()}},
			call:   handler.GetRuntimeRuns,
		},
		{
			name: "runtime run detail",
			path: "/agents/" + ids.agentID.String() + "/runtime-runs/" + messageID.String(),
			params: gin.Params{
				{Key: "agent_id", Value: ids.agentID.String()},
				{Key: "message_id", Value: messageID.String()},
			},
			call: handler.GetRuntimeRunDetail,
		},
		{
			name: "runtime run steps",
			path: "/agents/" + ids.agentID.String() + "/runtime-runs/" + messageID.String() + "/steps",
			params: gin.Params{
				{Key: "agent_id", Value: ids.agentID.String()},
				{Key: "message_id", Value: messageID.String()},
			},
			call: handler.GetRuntimeRunSteps,
		},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			permissionChecker.checked = false
			service.listRuntimeLogFiltersCalled = false
			service.getMessageRuntimeLogCalled = false

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Params = endpoint.params
			ctx.Set("account_id", ids.accountID.String())
			util.SetOrganizationID(ctx, ids.organizationID.String())
			ctx.Request = httptest.NewRequest(http.MethodGet, endpoint.path, nil)

			endpoint.call(ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if !permissionChecker.checked {
				t.Fatalf("expected workflow.logs.view permission check")
			}
			if service.listRuntimeLogFiltersCalled || service.getMessageRuntimeLogCalled {
				t.Fatalf("runtime service should not be called after missing workflow.logs.view denial")
			}
		})
	}
}

func TestAgentRuntimeRunsRequiresAgentViewBeforeBindingQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := runtimeHistoryIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	service := newFakeRuntimeHistoryService()
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewAgentRuntimeLogsHandler(
		&fakeRuntimeAgentRepo{agent: &agentspkg.Agent{
			ID:         ids.agentID,
			TenantID:   ids.workspaceID,
			Name:       "agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		}},
		service,
		permissionChecker,
	)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs?limit=not-an-int", nil)

	handler.GetRuntimeRuns(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check before query binding")
	}
	if service.listRuntimeLogFiltersCalled {
		t.Fatalf("runtime service should not be called after missing workflow.logs.view denial")
	}
}

func TestAgentRuntimeRunDetailRejectsOtherAgentMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	otherAgentID := uuid.New()
	webAppID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Title:          "other",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "secret",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "message_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs/"+messageID.String(), nil)

	handler.GetRuntimeRunDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentRuntimeRunStepsRejectsOtherAgentMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	otherAgentID := uuid.New()
	webAppID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Title:          "other",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "secret",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "message_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs/"+messageID.String()+"/steps", nil)

	handler.GetRuntimeRunSteps(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentRuntimeRunDetailAllowsWebAppMessageFromEndUserAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeLogsHandler(t)

	webAppID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      uuid.New(),
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "webapp visitor",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "visitor query",
		Answer:         "visitor answer",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "message_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/runtime-runs/"+messageID.String(), nil)

	handler.GetRuntimeRunDetail(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["id"] != messageID.String() || data["query"] != "visitor query" {
		t.Fatalf("detail = %#v, want visitor message", data)
	}
}

func TestAgentRuntimeStepsFromSkillInvocationsAndFinalAnswer(t *testing.T) {
	messageID := uuid.New()
	createdAt := time.Unix(1700000000, 0)
	updatedAt := createdAt.Add(2 * time.Second)
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: uuid.New(),
		Query:          "calculate",
		Answer:         "4",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		Metadata: map[string]interface{}{
			"usage": map[string]interface{}{"total_tokens": 12.0},
			"model_invocations": []interface{}{
				map[string]interface{}{
					"kind":        "model_call",
					"phase":       "skill_planning",
					"round":       0.0,
					"status":      "success",
					"duration_ms": 40.0,
					"request": map[string]interface{}{"model": "gpt-test", "messages": []interface{}{
						map[string]interface{}{"role": "system", "content": "visible user prompt\n\nhidden agent runtime prompt"},
						map[string]interface{}{"role": "system", "content": "hidden skill loop prompt"},
						map[string]interface{}{"role": "user", "content": "calculate"},
						map[string]interface{}{
							"role":         "tool",
							"tool_call_id": "call-load-skill",
							"content":      `{"skill_id":"calculator","instructions":"hidden skill instructions","tools":["calculate"],"token":"secret-token"}`,
						},
						map[string]interface{}{
							"role": "assistant",
							"tool_calls": []interface{}{
								map[string]interface{}{
									"id": "call-sensitive",
									"function": map[string]interface{}{
										"name":      "call_skill_tool",
										"arguments": `{"query":"select 1","password":"secret","headers":{"Authorization":"Bearer abc"}}`,
									},
								},
							},
						},
					}},
					"response":           map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": "call-1"}}}},
					"usage":              map[string]interface{}{"prompt_tokens": 8.0, "completion_tokens": 4.0, "total_tokens": 12.0},
					"prompt_tokens":      8.0,
					"completion_tokens":  4.0,
					"total_tokens":       12.0,
					"user_system_prompt": "visible user prompt",
					"runtime_id":         "model_call:skill_planning:0:1",
					"created_at":         1700000000.0,
				},
				map[string]interface{}{
					"kind":        "model_call",
					"phase":       "skill_planning",
					"round":       1.0,
					"status":      "success",
					"duration_ms": 30.0,
					"request": map[string]interface{}{"model": "gpt-test", "messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "calculate"},
					}},
					"response":   map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "done"}},
					"runtime_id": "model_call:skill_planning:1:2",
					"created_at": 1700000000.0,
				},
			},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":        "tool_call",
					"skill_id":    "calculator",
					"tool_name":   "calculate",
					"status":      "success",
					"duration_ms": 25.0,
					"arguments": map[string]interface{}{
						"expression": "2+2",
						"api_key":    "secret-key",
						"headers":    map[string]interface{}{"Authorization": "Bearer abc"},
					},
					"result":     map[string]interface{}{"value": 4.0, "instructions": "hidden skill instructions", "token": "secret-token"},
					"runtime_id": "tool_call:calculator:calculate#1",
					"created_at": 1700000000.0,
				},
			},
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	items := buildAgentRuntimeSteps(message)

	if len(items) != 4 {
		t.Fatalf("items len = %d, want 4", len(items))
	}
	if items[0].Type != "model_call" || items[0].Title != "Model call: skill_planning" {
		t.Fatalf("first item = %#v, want skill planning model call", items[0])
	}
	if request, ok := items[0].Input.(map[string]interface{}); !ok || request["model"] != "gpt-test" {
		t.Fatalf("first item input = %#v, want model request", items[0].Input)
	}
	request := items[0].Input.(map[string]interface{})
	messages := request["messages"].([]interface{})
	if len(messages) != 4 {
		t.Fatalf("first item messages = %#v, want user system prompt plus user message", messages)
	}
	if messages[0].(map[string]interface{})["role"] != "system" || messages[0].(map[string]interface{})["content"] != "visible user prompt" {
		t.Fatalf("first system message = %#v, want visible user prompt only", messages[0])
	}
	if messages[1].(map[string]interface{})["role"] != "user" {
		t.Fatalf("second message = %#v, want user message", messages[1])
	}
	toolMessageContent := messages[2].(map[string]interface{})["content"].(string)
	var toolMessage map[string]interface{}
	if err := json.Unmarshal([]byte(toolMessageContent), &toolMessage); err != nil {
		t.Fatalf("tool message content json = %v", err)
	}
	if toolMessage["instructions"] != agentRuntimeHiddenInstructionsPlaceholder || toolMessage["token"] != "[REDACTED]" {
		t.Fatalf("tool message content = %#v, want instructions placeholder and token redacted", toolMessage)
	}
	assistantToolCall := messages[3].(map[string]interface{})["tool_calls"].([]interface{})[0].(map[string]interface{})
	toolCallFn := assistantToolCall["function"].(map[string]interface{})
	var toolCallArgs map[string]interface{}
	if err := json.Unmarshal([]byte(toolCallFn["arguments"].(string)), &toolCallArgs); err != nil {
		t.Fatalf("tool call arguments json = %v", err)
	}
	if toolCallArgs["password"] != "[REDACTED]" || toolCallArgs["query"] != "select 1" {
		t.Fatalf("tool call arguments = %#v, want sensitive fields redacted only", toolCallArgs)
	}
	headers := toolCallArgs["headers"].(map[string]interface{})
	if headers["Authorization"] != "[REDACTED]" {
		t.Fatalf("tool call headers = %#v, want authorization redacted", headers)
	}
	rawEvent := items[0].Process["raw_event"].(map[string]interface{})
	rawRequest := rawEvent["request"].(map[string]interface{})
	rawMessages := rawRequest["messages"].([]interface{})
	if len(rawMessages) != 4 || rawMessages[0].(map[string]interface{})["content"] != "visible user prompt" {
		t.Fatalf("raw_event messages = %#v, want user system prompt only", rawMessages)
	}
	rawToolMessageContent := rawMessages[2].(map[string]interface{})["content"].(string)
	var rawToolMessage map[string]interface{}
	if err := json.Unmarshal([]byte(rawToolMessageContent), &rawToolMessage); err != nil {
		t.Fatalf("raw tool message content json = %v", err)
	}
	if rawToolMessage["instructions"] != agentRuntimeHiddenInstructionsPlaceholder || rawToolMessage["token"] != "[REDACTED]" {
		t.Fatalf("raw tool message content = %#v, want instructions placeholder and token redacted", rawToolMessage)
	}
	if items[0].Process["total_tokens"] != 12.0 || items[0].Process["prompt_tokens"] != 8.0 || items[0].Process["completion_tokens"] != 4.0 {
		t.Fatalf("first item token process = %#v, want per-call token usage", items[0].Process)
	}
	if items[1].Type != "tool_call" || items[1].Title != "calculator / calculate" {
		t.Fatalf("second item = %#v, want calculator tool call", items[1])
	}
	if items[1].Status != "succeeded" || items[1].ElapsedTime != 25 {
		t.Fatalf("second item status/elapsed = %s/%v, want succeeded/25", items[1].Status, items[1].ElapsedTime)
	}
	toolInput := items[1].Input.(map[string]interface{})
	if toolInput["api_key"] != "[REDACTED]" || toolInput["expression"] != "2+2" {
		t.Fatalf("second item input = %#v, want sensitive fields redacted only", toolInput)
	}
	toolInputHeaders := toolInput["headers"].(map[string]interface{})
	if toolInputHeaders["Authorization"] != "[REDACTED]" {
		t.Fatalf("second item input headers = %#v, want authorization redacted", toolInputHeaders)
	}
	if raw, ok := items[1].Process["raw_event"].(map[string]interface{}); !ok || raw["runtime_id"] != "tool_call:calculator:calculate#1" {
		t.Fatalf("second item raw_event = %#v, want persisted runtime event", items[1].Process["raw_event"])
	}
	rawTool := items[1].Process["raw_event"].(map[string]interface{})
	rawArgs := rawTool["arguments"].(map[string]interface{})
	if rawArgs["api_key"] != "[REDACTED]" {
		t.Fatalf("second item raw_event arguments = %#v, want sensitive fields redacted", rawArgs)
	}
	output := items[1].Output.(map[string]interface{})
	result := output["result"].(map[string]interface{})
	if result["instructions"] != agentRuntimeHiddenInstructionsPlaceholder || result["token"] != "[REDACTED]" {
		t.Fatalf("second item output = %#v, want instructions placeholder and token redacted", output)
	}
	if items[2].Type != "model_call" || items[2].Title != "Model call: skill_planning" {
		t.Fatalf("third item = %#v, want second model call after tool event", items[2])
	}
	if items[3].Type != "model_answer" || items[3].Output == nil {
		t.Fatalf("final item = %#v, want model answer step", items[3])
	}
}

func TestAgentRuntimeStepsWithoutTraceKeepsFinalAnswer(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "hello",
		Answer:         "world",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		CreatedAt:      time.Unix(1700000000, 0),
		UpdatedAt:      time.Unix(1700000001, 0),
	}

	items := buildAgentRuntimeSteps(message)

	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].Type != "model_answer" {
		t.Fatalf("first type = %q, want model_answer", items[0].Type)
	}
}

func TestAgentRuntimeStepsIncludeWorkflowRunEvents(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "run workflow",
		Answer:         "done",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		Metadata: map[string]interface{}{
			"workflow_runs": []interface{}{
				map[string]interface{}{
					"workflow_run_id": "run-1",
					"workflow_id":     "workflow-1",
					"status":          "pending_approval",
					"created_at":      1700000000.0,
					"elapsed_time":    1500.0,
					"inputs":          map[string]interface{}{"query": "run workflow", "api_key": "secret"},
					"outputs":         map[string]interface{}{"answer": "workflow output"},
					"approval": map[string]interface{}{
						"approval_form_id": "form-1",
						"approval_url":     "https://example.test/approve",
						"approval_form":    map[string]interface{}{"title": "Approve"},
					},
					"approval_result": map[string]interface{}{
						"approval_form_id": "form-1",
						"action":           "approve",
						"action_label":     "Approve",
						"inputs":           map[string]interface{}{"comment": "ok"},
					},
					"question_answer": map[string]interface{}{
						"answer": "workflow output",
					},
					"nodes": []interface{}{
						map[string]interface{}{
							"node_id":      "node-1",
							"node_type":    "llm",
							"title":        "Generate",
							"status":       "succeeded",
							"elapsed_time": 500.0,
							"inputs":       map[string]interface{}{"prompt": "hello"},
							"outputs":      map[string]interface{}{"text": "world", "token": "secret-token"},
						},
					},
				},
			},
		},
		CreatedAt: time.Unix(1700000000, 0),
		UpdatedAt: time.Unix(1700000002, 0),
	}

	items := buildAgentRuntimeSteps(message)

	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if items[0].Type != "workflow_run" || items[0].Title != "Workflow run: workflow-1" {
		t.Fatalf("first item = %#v, want workflow run event", items[0])
	}
	runInput := items[0].Input.(map[string]interface{})
	if runInput["api_key"] != "[REDACTED]" || runInput["query"] != "run workflow" {
		t.Fatalf("workflow run input = %#v, want sensitive fields redacted only", runInput)
	}
	process := items[0].Process
	nodes := process["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("workflow run process nodes = %#v, want one node", nodes)
	}
	node := nodes[0].(map[string]interface{})
	if node["node_id"] != "node-1" || node["node_type"] != "llm" {
		t.Fatalf("workflow run node = %#v, want persisted node detail", node)
	}
	nodeOutput := node["outputs"].(map[string]interface{})
	if nodeOutput["token"] != "[REDACTED]" || nodeOutput["text"] != "world" {
		t.Fatalf("workflow node output = %#v, want sensitive fields redacted only", nodeOutput)
	}
	approvals := process["approvals"].([]interface{})
	if len(approvals) != 1 {
		t.Fatalf("workflow run process approvals = %#v, want one approval lifecycle", approvals)
	}
	approval := approvals[0].(map[string]interface{})
	if approval["status"] != "succeeded" || approval["approval_form_id"] != "form-1" {
		t.Fatalf("workflow approval lifecycle = %#v, want succeeded form-1", approval)
	}
	if _, ok := process["question_answers"]; ok {
		t.Fatalf("workflow run process question_answers = %#v, want no fake question event", process["question_answers"])
	}
	if items[1].Type != "model_answer" {
		t.Fatalf("final item = %#v, want model answer", items[1])
	}
}

func TestAgentRuntimeWorkflowRunsNoLongerServeAgentMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _, ids := setupRuntimeHistoryHandler(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/workflow-runs?triggered_from=web-app", nil)

	handler.GetWorkflowRuns(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentRuntimeWorkflowRunDetailNoLongerServesAgentMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	conversationID := uuid.New()
	webAppID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "runtime question",
		Answer:         "runtime answer",
		Status:         runtimemodel.MessageStatusCompleted,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "run_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/workflow-runs/"+messageID.String(), nil)

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentRuntimeWorkflowRunNodeExecutionsNoLongerServeAgentMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	conversationID := uuid.New()
	webAppID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "runtime question",
		Answer:         "runtime answer",
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":      "tool_call",
					"skill_id":  "calculator",
					"tool_name": "calculate",
				},
			},
		},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "run_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/workflow-runs/"+messageID.String()+"/node-executions", nil)

	handler.GetWorkflowRunNodeExecutions(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

type runtimeHistoryIDs struct {
	organizationID uuid.UUID
	workspaceID    uuid.UUID
	accountID      uuid.UUID
	agentID        uuid.UUID
}

func setupRuntimeHistoryHandler(t *testing.T) (*AgentHistoryDispatchHandler, *fakeRuntimeHistoryService, runtimeHistoryIDs) {
	t.Helper()
	ids := runtimeHistoryIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	service := newFakeRuntimeHistoryService()
	handler := NewAgentHistoryDispatchHandler(
		&fakeRuntimeAgentRepo{agent: &agentspkg.Agent{
			ID:         ids.agentID,
			TenantID:   ids.workspaceID,
			Name:       "agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		}},
		&WorkflowHandler{
			workspacePermissionChecker: &fakeRuntimeLogWorkspacePermissionChecker{allowed: true},
		},
		nil,
		nil,
		service,
	)
	return handler, service, ids
}

func setupRuntimeLogsHandler(t *testing.T) (*AgentRuntimeLogsHandler, *fakeRuntimeHistoryService, runtimeHistoryIDs) {
	t.Helper()
	ids := runtimeHistoryIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	service := newFakeRuntimeHistoryService()
	handler := NewAgentRuntimeLogsHandler(
		&fakeRuntimeAgentRepo{agent: &agentspkg.Agent{
			ID:         ids.agentID,
			TenantID:   ids.workspaceID,
			Name:       "agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		}},
		service,
		&fakeRuntimeLogWorkspacePermissionChecker{allowed: true},
	)
	return handler, service, ids
}

func responseDataArray(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	payload, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data type = %T, want object: %s", resp["data"], string(body))
	}
	rawItems, ok := payload["data"].([]interface{})
	if !ok {
		t.Fatalf("payload data type = %T, want array: %#v", payload["data"], payload)
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("item type = %T, want object", raw)
		}
		items = append(items, item)
	}
	return items
}

type fakeRuntimeAgentRepo struct {
	agent *agentspkg.Agent
}

func (r *fakeRuntimeAgentRepo) GetByID(ctx context.Context, id string) (*agentspkg.Agent, error) {
	if r.agent != nil && r.agent.ID.String() == id {
		return r.agent, nil
	}
	return nil, fmt.Errorf("agent not found")
}

type fakeRuntimeHistoryService struct {
	runtimeservice.Service
	conversations               map[uuid.UUID]*runtimemodel.Conversation
	messages                    []*runtimemodel.Message
	listRuntimeLogFiltersCalled bool
	getMessageRuntimeLogCalled  bool
}

func newFakeRuntimeHistoryService() *fakeRuntimeHistoryService {
	return &fakeRuntimeHistoryService{
		conversations: map[uuid.UUID]*runtimemodel.Conversation{},
		messages:      []*runtimemodel.Message{},
	}
}

func (s *fakeRuntimeHistoryService) addConversation(conversation *runtimemodel.Conversation) {
	if conversation.Status == "" {
		conversation.Status = runtimemodel.ConversationStatusNormal
	}
	if conversation.RuntimeStatus == "" {
		conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	}
	if conversation.Metadata == nil {
		conversation.Metadata = map[string]interface{}{}
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = time.Unix(1700000000, 0)
	}
	if conversation.UpdatedAt.IsZero() {
		conversation.UpdatedAt = conversation.CreatedAt
	}
	s.conversations[conversation.ID] = conversation
}

func (s *fakeRuntimeHistoryService) addMessage(message *runtimemodel.Message) {
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Unix(1700000000, 0)
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt.Add(time.Second)
	}
	s.messages = append(s.messages, message)
}

func (s *fakeRuntimeHistoryService) ListMessagesByCallerSource(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, source string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	var filtered []*runtimemodel.Message
	for _, message := range s.messages {
		conversation := s.conversations[message.ConversationID]
		if !fakeConversationMatches(scope, caller, conversation) {
			continue
		}
		if source != "" && conversation.Source != source {
			continue
		}
		if source == runtimemodel.ConversationSourceWebApp && conversation.SourceWebAppID == nil {
			continue
		}
		filtered = append(filtered, message)
	}
	return paginateRuntimeMessages(filtered, page, limit)
}

func (s *fakeRuntimeHistoryService) ListMessagesByCallerLogFilters(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	var filtered []*runtimemodel.Message
	keyword := strings.ToLower(strings.TrimSpace(queryText))
	for _, message := range s.messages {
		conversation := s.conversations[message.ConversationID]
		if !fakeConversationMatches(scope, caller, conversation) {
			continue
		}
		if source != "" && conversation.Source != source {
			continue
		}
		if source == runtimemodel.ConversationSourceWebApp && conversation.SourceWebAppID == nil {
			continue
		}
		if conversationID != nil && message.ConversationID != *conversationID {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(message.Query), keyword) && !strings.Contains(strings.ToLower(message.Answer), keyword) {
			continue
		}
		filtered = append(filtered, message)
	}
	return paginateRuntimeMessages(filtered, page, limit)
}

func (s *fakeRuntimeHistoryService) ListMessagesByCallerRuntimeLogFilters(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, source string, conversationID *uuid.UUID, queryText string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	s.listRuntimeLogFiltersCalled = true
	var filtered []*runtimemodel.Message
	keyword := strings.ToLower(strings.TrimSpace(queryText))
	for _, message := range s.messages {
		conversation := s.conversations[message.ConversationID]
		if !fakeRuntimeLogConversationMatches(scope, caller, conversation, source) {
			continue
		}
		if conversationID != nil && message.ConversationID != *conversationID {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(message.Query), keyword) && !strings.Contains(strings.ToLower(message.Answer), keyword) {
			continue
		}
		filtered = append(filtered, message)
	}
	return paginateRuntimeMessages(filtered, page, limit)
}

func (s *fakeRuntimeHistoryService) GetMessageByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	for _, message := range s.messages {
		if message.ID != id {
			continue
		}
		conversation := s.conversations[message.ConversationID]
		if !fakeConversationMatches(scope, caller, conversation) {
			return nil, nil, runtimeservice.ErrNotFound
		}
		return message, conversation, nil
	}
	return nil, nil, runtimeservice.ErrNotFound
}

func (s *fakeRuntimeHistoryService) GetMessageByCallerRuntimeLog(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID, source string) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	s.getMessageRuntimeLogCalled = true
	for _, message := range s.messages {
		if message.ID != id {
			continue
		}
		conversation := s.conversations[message.ConversationID]
		if !fakeRuntimeLogConversationMatches(scope, caller, conversation, source) {
			return nil, nil, runtimeservice.ErrNotFound
		}
		return message, conversation, nil
	}
	return nil, nil, runtimeservice.ErrNotFound
}

func fakeConversationMatches(scope runtimeservice.Scope, caller runtimeservice.Caller, conversation *runtimemodel.Conversation) bool {
	if conversation == nil {
		return false
	}
	if conversation.OrganizationID != scope.OrganizationID || conversation.AccountID != scope.AccountID {
		return false
	}
	if conversation.CallerType != caller.Type {
		return false
	}
	if caller.ID == nil {
		return conversation.CallerID == nil
	}
	return conversation.CallerID != nil && *conversation.CallerID == *caller.ID
}

func fakeRuntimeLogConversationMatches(scope runtimeservice.Scope, caller runtimeservice.Caller, conversation *runtimemodel.Conversation, source string) bool {
	if conversation == nil {
		return false
	}
	if conversation.OrganizationID != scope.OrganizationID {
		return false
	}
	if scope.WorkspaceID != nil && (conversation.WorkspaceID == nil || *conversation.WorkspaceID != *scope.WorkspaceID) {
		return false
	}
	if conversation.CallerType != caller.Type {
		return false
	}
	if caller.ID == nil {
		if conversation.CallerID != nil {
			return false
		}
	} else if conversation.CallerID == nil || *conversation.CallerID != *caller.ID {
		return false
	}
	switch strings.TrimSpace(source) {
	case runtimemodel.ConversationSourceWebApp:
		return conversation.Source == runtimemodel.ConversationSourceWebApp &&
			conversation.SourceWebAppID != nil &&
			*conversation.SourceWebAppID != uuid.Nil
	case runtimemodel.ConversationSourceConsole:
		return conversation.Source == runtimemodel.ConversationSourceConsole &&
			conversation.AccountID == scope.AccountID
	case "":
		return conversation.AccountID == scope.AccountID
	default:
		return conversation.Source == source && conversation.AccountID == scope.AccountID
	}
}

func (s *fakeRuntimeHistoryService) CreateConversation(ctx context.Context, scope runtimeservice.Scope, title string) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CreateConversationForCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, title string) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListConversations(ctx context.Context, scope runtimeservice.Scope, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListConversationsByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	var filtered []*runtimemodel.Conversation
	for _, conversation := range s.conversations {
		if fakeConversationMatches(scope, caller, conversation) {
			filtered = append(filtered, conversation)
		}
	}
	total := int64(len(filtered))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = len(filtered)
		if limit < 1 {
			limit = 1
		}
	}
	start := (page - 1) * limit
	if start >= len(filtered) {
		return []*runtimemodel.Conversation{}, total, nil
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (s *fakeRuntimeHistoryService) ListConversationsBySurface(ctx context.Context, scope runtimeservice.Scope, surface string, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) Search(ctx context.Context, scope runtimeservice.Scope, query string, limit int) ([]*runtimeservice.SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) SearchBySurface(ctx context.Context, scope runtimeservice.Scope, surface string, query string, limit int) ([]*runtimeservice.SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) SearchByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, query string, limit int) ([]*runtimeservice.SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimemodel.Conversation, error) {
	conversation := s.conversations[id]
	if !fakeConversationMatches(scope, caller, conversation) {
		return nil, runtimeservice.ErrNotFound
	}
	return conversation, nil
}

func (s *fakeRuntimeHistoryService) UpdateConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) DeleteConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) DeleteConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListMessages(ctx context.Context, scope runtimeservice.Scope, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	var filtered []*runtimemodel.Message
	for _, message := range s.messages {
		if message.ConversationID != conversationID {
			continue
		}
		conversation := s.conversations[message.ConversationID]
		if conversation == nil || conversation.OrganizationID != scope.OrganizationID || conversation.AccountID != scope.AccountID {
			continue
		}
		filtered = append(filtered, message)
	}
	return paginateRuntimeMessages(filtered, page, limit)
}

func (s *fakeRuntimeHistoryService) ListConversationMessagesByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, conversationID); err != nil {
		return nil, 0, err
	}
	return s.ListMessages(ctx, scope, conversationID, page, limit)
}

func (s *fakeRuntimeHistoryService) ListMessagesByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, page, limit int) ([]*runtimemodel.Message, int64, error) {
	return s.ListMessagesByCallerSource(ctx, scope, caller, "", page, limit)
}

func paginateRuntimeMessages(messages []*runtimemodel.Message, page, limit int) ([]*runtimemodel.Message, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	start := (page - 1) * limit
	total := int64(len(messages))
	if start >= len(messages) {
		return []*runtimemodel.Message{}, total, nil
	}
	end := start + limit
	if end > len(messages) {
		end = len(messages)
	}
	return messages[start:end], total, nil
}

func (s *fakeRuntimeHistoryService) DeleteMessage(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StopMessage(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimemodel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StopConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimeservice.StopConversationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StopConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimeservice.StopConversationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareChat(ctx context.Context, scope runtimeservice.Scope, req runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareConfiguredChat(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, req runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareRootRegeneration(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareConfiguredRootRegeneration(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) RunPreparedStream(ctx context.Context, prepared *runtimeservice.PreparedChat, onChunk func(string) error, onEvent ...func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StreamConversationEvents(ctx context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, afterID string, onEvent func(runtimeservice.StreamEvent) error) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StreamConversationEventsForCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID, messageID uuid.UUID, afterID string, onEvent func(runtimeservice.StreamEvent) error) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) SubmitToolGovernanceDecision(ctx context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, correlationID string, req runtimedto.ToolGovernanceDecisionRequest) (*runtimedto.ToolGovernanceDecisionResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) RunToolGovernanceDecisionStream(ctx context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, correlationID string, req runtimedto.ToolGovernanceDecisionRequest, onEvent func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) RunClientActionContinuationStream(ctx context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, actionID string, req runtimedto.ClientActionResultRequest, onEvent func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) BeginWorkflowApprovalContinuation(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID, messageID uuid.UUID) (*runtimeservice.WorkflowApprovalContinuation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) RecordWorkflowApprovalContinuationEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*runtimeservice.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) AppendWorkflowApprovalContinuationStreamEvent(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*runtimeservice.StreamEvent, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateWorkflowApprovalContinuationStatus(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PauseWorkflowApprovalContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) SummarizeWorkflowApprovalContinuation(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req runtimeservice.WorkflowContinuationSummaryRequest, onEvent func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CompleteWorkflowApprovalContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, answer string, status string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) FailWorkflowApprovalContinuation(ctx context.Context, continuation *runtimeservice.WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListSkills(ctx context.Context, scope runtimeservice.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetSkill(ctx context.Context, scope runtimeservice.Scope, skillID string) (*skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetSkillConfig(ctx context.Context, scope runtimeservice.Scope) (*runtimeservice.SkillConfig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateSkillConfig(ctx context.Context, scope runtimeservice.Scope, req runtimedto.UpdateSkillConfigRequest) (*runtimeservice.SkillConfig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetAccountSkillPreference(ctx context.Context, scope runtimeservice.Scope, callerType string) (*runtimeservice.AccountSkillPreference, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateAccountSkillPreference(ctx context.Context, scope runtimeservice.Scope, callerType string, req runtimedto.UpdateAccountSkillPreferenceRequest) (*runtimeservice.AccountSkillPreference, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PreviewImportCustomSkill(ctx context.Context, scope runtimeservice.Scope, fileHeader *multipart.FileHeader) (*runtimeservice.SkillImportPreview, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ConfirmCustomSkillImport(ctx context.Context, scope runtimeservice.Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CancelCustomSkillImportPreview(ctx context.Context, scope runtimeservice.Scope, importID string) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) DeleteSkill(ctx context.Context, scope runtimeservice.Scope, skillID string) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CleanupStaleActiveMessages(ctx context.Context) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) MigrateWebAppConversation(ctx context.Context, scope runtimeservice.Scope, sourceConversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}
