package digitalhuman

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func (h *RuntimeHandler) GetExecutionFile(c *gin.Context) {
	scope, digitalHumanID, conversationID, ok := h.conversationScope(c)
	if !ok {
		return
	}
	// This route is behind account authentication, unlike tasks and channel
	// invocations. Construct its explicit subject from that verified account.
	scope.Surface, scope.UserSubjectKind, scope.UserSubjectID = SurfaceConsole, "account", &scope.ActorID
	if h.executionFileApplications == nil {
		h.base.writeError(c, applicationError(ErrNotFound, "digital_human.execution_file.application"))
		return
	}
	detail, err := h.executionFileApplications.Get(c.Request.Context(), scope, digitalHumanID)
	if err != nil || detail == nil || detail.Definition == nil || detail.Definition.ID != digitalHumanID ||
		detail.Definition.OrganizationID != scope.OrganizationID || detail.Definition.WorkspaceID != scope.WorkspaceID || detail.Definition.DeletedAt != nil {
		h.base.writeError(c, applicationError(ErrNotFound, "digital_human.execution_file.application"))
		return
	}
	conversation, err := h.service.GetConversation(c.Request.Context(), scope, digitalHumanID, conversationID)
	if err != nil {
		h.base.writeError(c, applicationError(err, "digital_human.execution_file.conversation"))
		return
	}
	serveDigitalHumanExecutionFile(c, h.base, scope, digitalHumanID, conversationID, conversation)
}

func (h *ExternalHandler) GetAPIExecutionFile(c *gin.Context) {
	scope, access, conversationID, ok := h.apiConversationRequest(c, userFromQuery(c))
	if !ok {
		return
	}
	conversation, err := h.runtime.GetConversation(c.Request.Context(), scope, access.Definition.ID, conversationID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.execution_file.conversation")
		return
	}
	serveDigitalHumanExecutionFile(c, h.base, scope, access.Definition.ID, conversationID, conversation)
}

func (h *ExternalHandler) GetWebAppExecutionFile(c *gin.Context) {
	scope, access, identity, ok := h.webAppScope(c)
	if !ok {
		return
	}
	if !identity.Authenticated {
		h.writeAuthError(c, false)
		return
	}
	conversationID, ok := h.conversationID(c)
	if !ok {
		return
	}
	conversation, err := h.runtime.GetConversation(c.Request.Context(), scope, access.Definition.ID, conversationID)
	if err != nil {
		h.writeApplicationError(c, err, "digital_human.execution_file.conversation")
		return
	}
	serveDigitalHumanExecutionFile(c, h.base, scope, access.Definition.ID, conversationID, conversation)
}

func serveDigitalHumanExecutionFile(c *gin.Context, handler *Handler, scope Scope, digitalHumanID, conversationID uuid.UUID, conversation *runtimemodel.Conversation) {
	access, ok := digitalHumanExecutionFileAccess(scope, digitalHumanID, conversation)
	if !ok || conversation.ID != conversationID || tool_file.GlobalToolFileManager == nil {
		handler.writeError(c, apperror.New(appcatalog.CodeResourceNotFound, apperror.WithOperation("digital_human.execution_file.owner")))
		return
	}
	data, file, err := tool_file.GlobalToolFileManager.GetExecutionFileBinary(c.Request.Context(), access, c.Param("file_id"))
	if err != nil {
		handler.writeError(c, apperror.Wrap(err, appcatalog.CodeResourceNotFound, apperror.WithOperation("digital_human.execution_file.read")))
		return
	}
	tool_file.WriteExecutionFile(c, file, data)
}

func digitalHumanExecutionFileAccess(scope Scope, digitalHumanID uuid.UUID, conversation *runtimemodel.Conversation) (tool_file.ExecutionFileAccess, bool) {
	if scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil || scope.ActorID != *scope.UserSubjectID ||
		conversation == nil || conversation.DeletedAt != nil || conversation.OrganizationID != scope.OrganizationID ||
		conversation.CallerType != runtimemodel.ConversationCallerDigitalHuman || conversation.CallerID == nil || *conversation.CallerID != digitalHumanID ||
		conversation.AccountID != *scope.UserSubjectID || conversation.WorkspaceID == nil || *conversation.WorkspaceID != scope.WorkspaceID {
		return tool_file.ExecutionFileAccess{}, false
	}
	var entry string
	switch scope.Surface {
	case SurfaceConsole:
		entry = "console"
		if scope.UserSubjectKind != "account" {
			return tool_file.ExecutionFileAccess{}, false
		}
	case SurfaceWebApp:
		entry = "webapp"
		if scope.UserSubjectKind != "account" || scope.WebAppID == nil || conversation.SourceWebAppID == nil || *scope.WebAppID != *conversation.SourceWebAppID {
			return tool_file.ExecutionFileAccess{}, false
		}
	case SurfaceAPI:
		entry = "external-api"
		if scope.UserSubjectKind != "api_end_user" {
			return tool_file.ExecutionFileAccess{}, false
		}
	default:
		return tool_file.ExecutionFileAccess{}, false
	}
	if conversation.Source != entry {
		return tool_file.ExecutionFileAccess{}, false
	}
	return tool_file.ExecutionFileAccess{OrganizationID: scope.OrganizationID.String(), SubjectID: scope.UserSubjectID.String(), WorkspaceID: scope.WorkspaceID.String(),
		ApplicationType: "digital_human", ApplicationID: digitalHumanID.String(), Entry: entry, SubjectKind: scope.UserSubjectKind, ConversationID: conversation.ID.String()}, true
}
