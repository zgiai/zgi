package agents

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/response"
)

func (h *AgentsHandler) GetAgentRuntimeExecutionFile(c *gin.Context) {
	// File access must not depend on runtime admission, Profile readiness or CLI
	// switches. Existing draft access still checks the account's live permission.
	scope, agentID, _, ok := h.agentDraftRuntimeAccess(c)
	if !ok {
		return
	}
	h.serveRuntimeExecutionFile(c, scope, runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole})
}

func (h *AgentsHandler) GetWebAppAgentRuntimeExecutionFile(c *gin.Context) {
	if !c.GetBool("is_authenticated") {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	scope, agentID, webAppID, _, ok := h.webAppAgentRuntimeAccess(c)
	if !ok {
		return
	}
	h.serveRuntimeExecutionFile(c, scope, runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID})
}

func (h *AgentsHandler) GetAPIKeyAgentRuntimeExecutionFile(c *gin.Context) {
	// Runtime Workspace credentials deliberately have no new protected-file or
	// execution route, even if this handler is called through a future adapter.
	if _, exists := c.Get("runtime_workspace_principal"); exists || c.GetString("runtime_workspace_id") != "" || c.GetString("runtime_subject_id") != "" {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	runtimeCtx, ok := h.apiKeyAgentUserRuntimeContext(c, c.Query("user"))
	if !ok || runtimeCtx.Caller.ID == nil {
		return
	}
	published, err := h.appService.GetPublishedAgentRuntimeConfig(c.Request.Context(), runtimeCtx.Caller.ID.String())
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	scope, err := scopeWithAgentResourceWorkspace(runtimeCtx.Scope, published.WorkspaceID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	h.serveRuntimeExecutionFile(c, scope, runtimeCtx.Caller)
}

func (h *AgentsHandler) serveRuntimeExecutionFile(c *gin.Context, scope runtimeservice.Scope, caller runtimeservice.Caller) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	conversation, err := h.chatRuntimeService.GetConversationByCaller(c.Request.Context(), scope, caller, conversationID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	if conversation == nil || conversation.ID != conversationID {
		response.Fail(c, response.ErrFileNotFound)
		return
	}
	access, ok := agentExecutionFileAccess(scope, caller, conversation)
	if !ok {
		response.Fail(c, response.ErrFileNotFound)
		return
	}
	tool_file.NewHTTPHandler(tool_file.GlobalToolFileManager).ServeExecutionFile(c, access)
}

func agentExecutionFileAccess(scope runtimeservice.Scope, caller runtimeservice.Caller, conversation *runtimemodel.Conversation) (tool_file.ExecutionFileAccess, bool) {
	workspace := scope.AgentResourceWorkspaceID()
	if workspace == nil || *workspace == uuid.Nil || caller.ID == nil || conversation == nil ||
		scope.RuntimeWorkspaceID != nil || scope.RuntimeSubjectID != nil ||
		conversation.DeletedAt != nil || conversation.OrganizationID != scope.OrganizationID || conversation.CallerType != runtimemodel.ConversationCallerAgent ||
		conversation.CallerID == nil || *conversation.CallerID != *caller.ID || conversation.Source != caller.Source {
		return tool_file.ExecutionFileAccess{}, false
	}
	subject, kind := scope.AccountID, "account"
	switch caller.Source {
	case runtimemodel.ConversationSourceConsole:
		if conversation.AccountID != subject {
			return tool_file.ExecutionFileAccess{}, false
		}
	case runtimemodel.ConversationSourceWebApp:
		if caller.SourceWebAppID == nil || conversation.SourceWebAppID == nil || *caller.SourceWebAppID != *conversation.SourceWebAppID || conversation.AccountID != subject {
			return tool_file.ExecutionFileAccess{}, false
		}
	case runtimemodel.ConversationSourceExternalAPI:
		if scope.ExternalUserID == nil || *scope.ExternalUserID == uuid.Nil || conversation.ExternalUserID == nil || *scope.ExternalUserID != *conversation.ExternalUserID {
			return tool_file.ExecutionFileAccess{}, false
		}
		subject, kind = *scope.ExternalUserID, "api_end_user"
	default:
		return tool_file.ExecutionFileAccess{}, false
	}
	return tool_file.ExecutionFileAccess{OrganizationID: scope.OrganizationID.String(), SubjectID: subject.String(),
		WorkspaceID: workspace.String(), ApplicationType: runtimemodel.ConversationCallerAgent, ApplicationID: caller.ID.String(),
		Entry: caller.Source, SubjectKind: kind, ConversationID: conversation.ID.String()}, true
}
