package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestWebAppAgentRuntimeEventsRequireCallerScopedConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/events?message_id="+ids.messageID.String(), nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.StreamWebAppAgentRuntimeEvents(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called")
	}
	if runtimeSvc.streamCalled {
		t.Fatalf("StreamConversationEvents should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeEventsRequireCallerScopedConversationBeforeMessageIDValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/events?message_id=not-a-uuid", nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.StreamWebAppAgentRuntimeEvents(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before message_id validation")
	}
	if runtimeSvc.streamForCallerCalled {
		t.Fatalf("StreamConversationEventsForCaller should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeUpdateConversationRequiresCallerScopedConversationBeforeBindingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String(), bytes.NewBufferString(`{`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.UpdateWebAppAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before binding request")
	}
	if runtimeSvc.updateConversationCalled {
		t.Fatalf("UpdateConversation should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeDeleteConversationRequiresCallerScopedConversationBeforeDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String(), nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.DeleteWebAppAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before delete")
	}
	if runtimeSvc.deleteConversationCalled {
		t.Fatalf("DeleteConversation should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeListMessagesRequiresCallerScopedConversationBeforeList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/messages?page=1&limit=10", nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.ListWebAppAgentRuntimeMessages(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before listing messages")
	}
	if runtimeSvc.listMessagesCalled {
		t.Fatalf("ListMessages should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeStopConversationRequiresCallerScopedConversationBeforeStop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)
	continuationRunner := &fakeWorkflowContinuationRunner{}
	handler.SetWorkflowContinuationRunner(continuationRunner)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/stop", nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.StopWebAppAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before stop")
	}
	if runtimeSvc.stopConversationCalled {
		t.Fatalf("StopConversation should not be called after caller-scoped conversation denial")
	}
	if continuationRunner.stopCalled {
		t.Fatalf("StopWorkflowContinuation should not be called after caller-scoped conversation denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestWebAppAgentRuntimeEventsPassCallerToStreamService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/events?message_id="+ids.messageID.String(), nil)
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.StreamWebAppAgentRuntimeEvents(c)

	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called")
	}
	if !runtimeSvc.streamForCallerCalled {
		t.Fatalf("StreamConversationEventsForCaller was not called")
	}
	if runtimeSvc.streamCalled {
		t.Fatalf("legacy StreamConversationEvents should not be called for agent runtime events")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
}

func TestWebAppAgentRuntimeContinuationRequiresCallerScopedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{beginContinuationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/workflow-continuation", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.ContinueWebAppAgentRuntimeWorkflowApproval(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.beginContinuationCalled {
		t.Fatalf("BeginWorkflowApprovalContinuation was not called")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("content type = %q, should not switch to event stream after caller-scoped continuation denial", got)
	}
}

func TestWebAppAgentRuntimeRegenerationRequiresCallerScopedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{prepareRegenerationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newWebAppRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webapps/"+ids.webAppID.String()+"/runtime/messages/"+ids.messageID.String()+"/regenerate", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "web_app_id", Value: ids.webAppID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("is_authenticated", true)

	handler.RegenerateWebAppAgentRuntimeMessage(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.prepareRegenerationCalled {
		t.Fatalf("PrepareConfiguredRootRegeneration was not called")
	}
	if runtimeSvc.runPreparedStreamCalled {
		t.Fatalf("RunPreparedStream should not be called after caller-scoped regeneration denial")
	}
	assertWebAppRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("content type = %q, should not switch to event stream after caller-scoped regeneration denial", got)
	}
}

func TestAgentRuntimeEventsPassCallerToStreamService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/events?message_id="+ids.messageID.String(), nil)
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.StreamAgentRuntimeEvents(c)

	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called")
	}
	if !runtimeSvc.streamForCallerCalled {
		t.Fatalf("StreamConversationEventsForCaller was not called")
	}
	if runtimeSvc.streamCalled {
		t.Fatalf("legacy StreamConversationEvents should not be called for agent runtime events")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
}

func TestAgentRuntimeEventsRequireCallerScopedConversationBeforeMessageIDValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/events?message_id=not-a-uuid", nil)
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.StreamAgentRuntimeEvents(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before message_id validation")
	}
	if runtimeSvc.streamForCallerCalled {
		t.Fatalf("StreamConversationEventsForCaller should not be called after caller-scoped conversation denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestAgentRuntimeUpdateConversationRequiresCallerScopedConversationBeforeBindingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String(), bytes.NewBufferString(`{`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.UpdateAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before binding request")
	}
	if runtimeSvc.updateConversationCalled {
		t.Fatalf("UpdateConversation should not be called after caller-scoped conversation denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestAgentRuntimeDeleteConversationRequiresCallerScopedConversationBeforeDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String(), nil)
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.DeleteAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before delete")
	}
	if runtimeSvc.deleteConversationCalled {
		t.Fatalf("DeleteConversation should not be called after caller-scoped conversation denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestAgentRuntimeListMessagesRequiresCallerScopedConversationBeforeList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/messages?page=1&limit=10", nil)
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.ListAgentRuntimeMessages(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before listing messages")
	}
	if runtimeSvc.listMessagesCalled {
		t.Fatalf("ListMessages should not be called after caller-scoped conversation denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestAgentRuntimeStopConversationRequiresCallerScopedConversationBeforeStop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)
	continuationRunner := &fakeWorkflowContinuationRunner{}
	handler.SetWorkflowContinuationRunner(continuationRunner)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/stop", nil)
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.StopAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.getConversationCalled {
		t.Fatalf("GetConversationByCaller was not called before stop")
	}
	if runtimeSvc.stopConversationCalled {
		t.Fatalf("StopConversation should not be called after caller-scoped conversation denial")
	}
	if continuationRunner.stopCalled {
		t.Fatalf("StopWorkflowContinuation should not be called after caller-scoped conversation denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
}

func TestAgentRuntimeContinuationRequiresCallerScopedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{beginContinuationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/apps/"+ids.agentID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/workflow-continuation", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "conversation_id", Value: ids.conversationID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.ContinueAgentRuntimeWorkflowApproval(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.beginContinuationCalled {
		t.Fatalf("BeginWorkflowApprovalContinuation was not called")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastConversationID != ids.conversationID {
		t.Fatalf("conversation id = %s, want %s", runtimeSvc.lastConversationID, ids.conversationID)
	}
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("content type = %q, should not switch to event stream after caller-scoped continuation denial", got)
	}
}

func TestAgentRuntimeRegenerationRequiresCallerScopedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{prepareRegenerationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/apps/"+ids.agentID.String()+"/runtime/messages/"+ids.messageID.String()+"/regenerate", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
	}
	c.Set("account_id", ids.accountID.String())
	c.Set("organization_id", ids.organizationID.String())

	handler.RegenerateAgentRuntimeMessage(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	requireRuntimeResponseCode(t, w, response.ErrNotFound)
	if !runtimeSvc.prepareRegenerationCalled {
		t.Fatalf("PrepareConfiguredRootRegeneration was not called")
	}
	if runtimeSvc.runPreparedStreamCalled {
		t.Fatalf("RunPreparedStream should not be called after caller-scoped regeneration denial")
	}
	assertAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastMessageID != ids.messageID {
		t.Fatalf("message id = %s, want %s", runtimeSvc.lastMessageID, ids.messageID)
	}
	if got := w.Header().Get("Content-Type"); got == "text/event-stream" {
		t.Fatalf("content type = %q, should not switch to event stream after caller-scoped regeneration denial", got)
	}
}

type webAppRuntimePermissionIDs struct {
	organizationID uuid.UUID
	workspaceID    uuid.UUID
	accountID      uuid.UUID
	agentID        uuid.UUID
	webAppID       uuid.UUID
	conversationID uuid.UUID
	messageID      uuid.UUID
}

type webAppRuntimePermissionAppService struct {
	AgentsService
	ids webAppRuntimePermissionIDs
}

func newWebAppRuntimePermissionAppService(ids webAppRuntimePermissionIDs) *webAppRuntimePermissionAppService {
	return &webAppRuntimePermissionAppService{ids: ids}
}

func (s *webAppRuntimePermissionAppService) GetPublishedAgentWebAppConfig(_ context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	if webAppID != s.ids.webAppID.String() {
		return nil, runtimeservice.ErrNotFound
	}
	return &dto.AgentWebAppRuntimeConfigResponse{
		AgentID:        s.ids.agentID.String(),
		WebAppID:       s.ids.webAppID.String(),
		WorkspaceID:    s.ids.workspaceID.String(),
		OrganizationID: s.ids.organizationID.String(),
		AgentType:      "AGENT",
		Version:        "v1",
		VersionUUID:    uuid.NewString(),
		Config: dto.AgentConfigResponse{
			AgentID:         s.ids.agentID.String(),
			ModelParameters: map[string]interface{}{},
		},
	}, nil
}

func (s *webAppRuntimePermissionAppService) GetWebAppRuntimeCapability(_ context.Context, webAppID, accountID string, authenticated bool) (*dto.AgentWebAppRuntimeCapabilityResponse, error) {
	if webAppID != s.ids.webAppID.String() || accountID != s.ids.accountID.String() {
		return nil, runtimeservice.ErrNotFound
	}
	return &dto.AgentWebAppRuntimeCapabilityResponse{
		AgentID:        s.ids.agentID.String(),
		WebAppID:       s.ids.webAppID.String(),
		WorkspaceID:    s.ids.workspaceID.String(),
		OrganizationID: s.ids.organizationID.String(),
		Allowed:        true,
		Reason:         string(runtimeauth.RuntimeAccessAllowedAccountGrant),
		VersionUUID:    uuid.NewString(),
	}, nil
}

type agentRuntimePermissionAppService struct {
	AgentsService
	ids webAppRuntimePermissionIDs
}

func newAgentRuntimePermissionAppService(ids webAppRuntimePermissionIDs) *agentRuntimePermissionAppService {
	return &agentRuntimePermissionAppService{ids: ids}
}

func (s *agentRuntimePermissionAppService) GetAgentDraftRuntimeConfig(_ context.Context, agentID, accountID string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	if agentID != s.ids.agentID.String() || accountID != s.ids.accountID.String() {
		return nil, runtimeservice.ErrNotFound
	}
	return &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     s.ids.agentID.String(),
		WorkspaceID: s.ids.workspaceID.String(),
		Config: dto.AgentConfigResponse{
			AgentID:         s.ids.agentID.String(),
			ModelParameters: map[string]interface{}{},
		},
	}, nil
}

type webAppRuntimePermissionService struct {
	runtimeservice.Service

	getConversationErr        error
	beginContinuationErr      error
	prepareRegenerationErr    error
	getConversationCalled     bool
	updateConversationCalled  bool
	deleteConversationCalled  bool
	listMessagesCalled        bool
	stopConversationCalled    bool
	streamCalled              bool
	streamForCallerCalled     bool
	beginContinuationCalled   bool
	prepareRegenerationCalled bool
	runPreparedStreamCalled   bool
	lastScope                 runtimeservice.Scope
	lastCaller                runtimeservice.Caller
	lastConversationID        uuid.UUID
	lastMessageID             uuid.UUID
}

func (s *webAppRuntimePermissionService) GetConversationByCaller(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimemodel.Conversation, error) {
	s.getConversationCalled = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastConversationID = id
	if s.getConversationErr != nil {
		return nil, s.getConversationErr
	}
	return &runtimemodel.Conversation{ID: id}, nil
}

func (s *webAppRuntimePermissionService) DeleteConversation(_ context.Context, scope runtimeservice.Scope, id uuid.UUID) error {
	s.deleteConversationCalled = true
	s.lastScope = scope
	s.lastConversationID = id
	return nil
}

func (s *webAppRuntimePermissionService) DeleteConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) error {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return err
	}
	return s.DeleteConversation(ctx, scope, id)
}

func (s *webAppRuntimePermissionService) ListMessages(_ context.Context, scope runtimeservice.Scope, conversationID uuid.UUID, _ int, _ int) ([]*runtimemodel.Message, int64, error) {
	s.listMessagesCalled = true
	s.lastScope = scope
	s.lastConversationID = conversationID
	return nil, 0, nil
}

func (s *webAppRuntimePermissionService) ListConversationMessagesByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID uuid.UUID, _ int, _ int) ([]*runtimemodel.Message, int64, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, conversationID); err != nil {
		return nil, 0, err
	}
	s.listMessagesCalled = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastConversationID = conversationID
	return nil, 0, nil
}

func (s *webAppRuntimePermissionService) StopConversation(_ context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimeservice.StopConversationResult, error) {
	s.stopConversationCalled = true
	s.lastScope = scope
	s.lastConversationID = id
	return &runtimeservice.StopConversationResult{
		Conversation: &runtimemodel.Conversation{
			ID:            id,
			RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle,
		},
	}, nil
}

func (s *webAppRuntimePermissionService) StopConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimeservice.StopConversationResult, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return nil, err
	}
	return s.StopConversation(ctx, scope, id)
}

func (s *webAppRuntimePermissionService) UpdateConversation(_ context.Context, scope runtimeservice.Scope, id uuid.UUID, _ runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	s.updateConversationCalled = true
	s.lastScope = scope
	s.lastConversationID = id
	return &runtimemodel.Conversation{ID: id}, nil
}

func (s *webAppRuntimePermissionService) UpdateConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	if _, err := s.GetConversationByCaller(ctx, scope, caller, id); err != nil {
		return nil, err
	}
	return s.UpdateConversation(ctx, scope, id, req)
}

func (s *webAppRuntimePermissionService) StreamConversationEvents(_ context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, _ string, _ func(runtimeservice.StreamEvent) error) error {
	s.streamCalled = true
	s.lastScope = scope
	s.lastConversationID = conversationID
	s.lastMessageID = messageID
	return nil
}

func (s *webAppRuntimePermissionService) StreamConversationEventsForCaller(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID, messageID uuid.UUID, _ string, _ func(runtimeservice.StreamEvent) error) error {
	s.streamForCallerCalled = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastConversationID = conversationID
	s.lastMessageID = messageID
	return nil
}

func (s *webAppRuntimePermissionService) BeginWorkflowApprovalContinuation(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, conversationID, messageID uuid.UUID) (*runtimeservice.WorkflowApprovalContinuation, error) {
	s.beginContinuationCalled = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastConversationID = conversationID
	s.lastMessageID = messageID
	if s.beginContinuationErr != nil {
		return nil, s.beginContinuationErr
	}
	return &runtimeservice.WorkflowApprovalContinuation{ConversationID: conversationID, MessageID: messageID}, nil
}

func (s *webAppRuntimePermissionService) PrepareConfiguredRootRegeneration(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, _ runtimeservice.RunConfig, id uuid.UUID, _ runtimedto.RegenerateMessageRequest) (*runtimeservice.PreparedChat, error) {
	s.prepareRegenerationCalled = true
	s.lastScope = scope
	s.lastCaller = caller
	s.lastMessageID = id
	if s.prepareRegenerationErr != nil {
		return nil, s.prepareRegenerationErr
	}
	return nil, nil
}

func (s *webAppRuntimePermissionService) RunPreparedStream(_ context.Context, _ *runtimeservice.PreparedChat, _ func(string) error, _ ...func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	s.runPreparedStreamCalled = true
	return nil, nil
}

type fakeWorkflowContinuationRunner struct {
	stopCalled bool
}

func (r *fakeWorkflowContinuationRunner) ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error {
	return nil
}

func (r *fakeWorkflowContinuationRunner) ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error {
	return nil
}

func (r *fakeWorkflowContinuationRunner) StopWorkflowContinuation(context.Context, string, string) error {
	r.stopCalled = true
	return nil
}

func assertWebAppRuntimeCallerScope(t *testing.T, scope runtimeservice.Scope, caller runtimeservice.Caller, ids webAppRuntimePermissionIDs) {
	t.Helper()

	if scope.OrganizationID != ids.organizationID {
		t.Fatalf("scope organization = %s, want %s", scope.OrganizationID, ids.organizationID)
	}
	if scope.AccountID != ids.accountID {
		t.Fatalf("scope account = %s, want %s", scope.AccountID, ids.accountID)
	}
	if scope.WorkspaceID == nil || *scope.WorkspaceID != ids.workspaceID {
		t.Fatalf("scope workspace = %v, want %s", scope.WorkspaceID, ids.workspaceID)
	}
	if !scope.SkipAccessCheck {
		t.Fatalf("scope SkipAccessCheck = false, want true for published webapp runtime")
	}
	if caller.Type != runtimemodel.ConversationCallerAgent {
		t.Fatalf("caller type = %q, want %q", caller.Type, runtimemodel.ConversationCallerAgent)
	}
	if caller.Source != runtimemodel.ConversationSourceWebApp {
		t.Fatalf("caller source = %q, want %q", caller.Source, runtimemodel.ConversationSourceWebApp)
	}
	if caller.ID == nil || *caller.ID != ids.agentID {
		t.Fatalf("caller agent = %v, want %s", caller.ID, ids.agentID)
	}
	if caller.SourceWebAppID == nil || *caller.SourceWebAppID != ids.webAppID {
		t.Fatalf("caller webapp = %v, want %s", caller.SourceWebAppID, ids.webAppID)
	}
}

func assertAgentRuntimeCallerScope(t *testing.T, scope runtimeservice.Scope, caller runtimeservice.Caller, ids webAppRuntimePermissionIDs) {
	t.Helper()

	if scope.OrganizationID != ids.organizationID {
		t.Fatalf("scope organization = %s, want %s", scope.OrganizationID, ids.organizationID)
	}
	if scope.AccountID != ids.accountID {
		t.Fatalf("scope account = %s, want %s", scope.AccountID, ids.accountID)
	}
	if scope.WorkspaceID == nil || *scope.WorkspaceID != ids.workspaceID {
		t.Fatalf("scope workspace = %v, want %s", scope.WorkspaceID, ids.workspaceID)
	}
	if scope.SkipAccessCheck {
		t.Fatalf("scope SkipAccessCheck = true, want false for console agent runtime")
	}
	if caller.Type != runtimemodel.ConversationCallerAgent {
		t.Fatalf("caller type = %q, want %q", caller.Type, runtimemodel.ConversationCallerAgent)
	}
	if caller.Source != runtimemodel.ConversationSourceConsole {
		t.Fatalf("caller source = %q, want %q", caller.Source, runtimemodel.ConversationSourceConsole)
	}
	if caller.ID == nil || *caller.ID != ids.agentID {
		t.Fatalf("caller agent = %v, want %s", caller.ID, ids.agentID)
	}
	if caller.SourceWebAppID != nil {
		t.Fatalf("caller webapp = %v, want nil for console agent runtime", caller.SourceWebAppID)
	}
}

func requireRuntimeResponseCode(t *testing.T, w *httptest.ResponseRecorder, err response.ErrorCode) {
	t.Helper()

	var body map[string]interface{}
	if decodeErr := json.Unmarshal(w.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode response body: %v; body=%s", decodeErr, w.Body.String())
	}
	want := strconv.Itoa(err.Code)
	if got := body["code"]; got != want {
		t.Fatalf("response code = %#v, want %q; body=%s", got, want, w.Body.String())
	}
}
