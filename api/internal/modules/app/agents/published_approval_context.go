package agents

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
)

func (h *AgentsHandler) restorePublishedPlatformApproval(c *gin.Context, current *agentRuntimeContext) bool {
	conversation, ok := uuidParam(c, "conversation_id")
	if !ok {
		return false
	}
	message, ok := uuidParam(c, "message_id")
	if !ok {
		return false
	}
	keyID, _ := uuid.Parse(c.GetString("api_key_id"))
	config, err := h.publishedApprovalRunConfig(c.Request.Context(), *current, conversation, message, c.Param("correlation_id"), keyID)
	if err != nil {
		h.failRuntime(c, publishedExecutionDenied(err))
		return false
	}
	current.RunConfig = config
	return true
}

func (h *AgentsHandler) publishedApprovalRunConfig(ctx context.Context, current agentRuntimeContext, conversationID, messageID uuid.UUID, correlation string, keyID uuid.UUID) (runtimeservice.RunConfig, error) {
	denied := execution.ErrInvalidIdentity
	if h.chatRuntimeService == nil || current.Caller.ID == nil || correlation == "" || strings.TrimSpace(correlation) != correlation || len(correlation) > 256 {
		return runtimeservice.RunConfig{}, denied
	}
	message, conversation, err := h.chatRuntimeService.GetMessageByCaller(ctx, current.Scope, current.Caller, messageID)
	if err != nil {
		return runtimeservice.RunConfig{}, err
	}
	origin, err := publishedApprovalOrigin(current, conversation, message, conversationID, messageID, correlation)
	if err != nil {
		return runtimeservice.RunConfig{}, err
	}
	reader, ok := h.appService.(interface {
		getPublishedAgentRuntimeConfigAtVersion(context.Context, uuid.UUID, uuid.UUID) (*dto.AgentWebAppRuntimeConfigResponse, error)
	})
	if !ok {
		return runtimeservice.RunConfig{}, denied
	}
	publication, err := uuid.Parse(origin.PublicationRef)
	if err != nil {
		return runtimeservice.RunConfig{}, denied
	}
	published, err := reader.getPublishedAgentRuntimeConfigAtVersion(ctx, *current.Caller.ID, publication)
	if err != nil {
		return runtimeservice.RunConfig{}, err
	}
	if published == nil || published.VersionUUID != origin.PublicationRef || published.Config.ExecutionRuntime == nil || !published.Config.ExecutionRuntime.Enabled {
		return runtimeservice.RunConfig{}, denied
	}
	if _, err := publishedExecutionAccessFor(current.Scope, current.Caller, published, current.Caller.Source == runtimemodel.ConversationSourceWebApp); err != nil {
		return runtimeservice.RunConfig{}, err
	}
	memoryScope := "account"
	if current.Caller.Source == runtimemodel.ConversationSourceExternalAPI {
		memoryScope = "end_user"
	}
	config, err := h.agentRunConfig(ctx, current.Scope, published.AgentID, "agent.published."+published.Version, published.Config, memoryScope)
	if err != nil {
		return runtimeservice.RunConfig{}, err
	}
	config.RuntimeInputPolicy = current.RunConfig.RuntimeInputPolicy
	if err := h.attachPublishedExecutionTools(ctx, &config, current.Scope, current.Caller, published, current.Caller.Source == runtimemodel.ConversationSourceWebApp, keyID, true); err != nil {
		return runtimeservice.RunConfig{}, err
	}
	if config.RuntimeToolFactory == nil {
		return runtimeservice.RunConfig{}, denied
	}
	return config, nil
}

// Read only the authoritative sealed call after caller-scoped message lookup.
// No client field can select a publication, owner, application or frozen payload.
func publishedApprovalOrigin(current agentRuntimeContext, conversation *runtimemodel.Conversation, message *runtimemodel.Message, conversationID, messageID uuid.UUID, correlation string) (*toolgovernance.ExecutionOrigin, error) {
	denied := execution.ErrInvalidIdentity
	if conversation == nil || message == nil || conversation.ID != conversationID || message.ID != messageID || message.ConversationID != conversationID || message.DeletedAt != nil ||
		current.Scope.WorkspaceID == nil || conversation.WorkspaceID == nil || *current.Scope.WorkspaceID != *conversation.WorkspaceID || conversation.AccountID != current.Scope.AccountID {
		return nil, denied
	}
	owner, ok := agentExecutionFileAccess(current.Scope, current.Caller, conversation)
	if !ok {
		return nil, denied
	}
	var origin *toolgovernance.ExecutionOrigin
	events, _ := message.Metadata["tool_governance_decisions"].([]interface{})
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event["correlation_id"] != correlation {
			continue
		}
		approval, _ := event["approval_event"].(map[string]interface{})
		frozen, found, err := toolgovernance.FrozenInvocationFromAny(approval["frozen_invocation"])
		if origin != nil || err != nil || !found || frozen.CorrelationID != correlation || !toolgovernance.FrozenInvocationHashMatches(frozen) || frozen.ExecutionOrigin == nil {
			return nil, denied
		}
		o := frozen.ExecutionOrigin
		if o.Validate() != nil || o.ApplicationType != "agent" || o.ApplicationID != owner.ApplicationID || o.OrganizationID != owner.OrganizationID || o.WorkspaceID != owner.WorkspaceID ||
			o.Entry != owner.Entry || o.SubjectID != owner.SubjectID || o.SubjectKind != owner.SubjectKind || o.ConversationID != conversationID.String() || o.MessageID != messageID.String() {
			return nil, denied
		}
		origin = o
	}
	if origin == nil {
		return nil, denied
	}
	return origin, nil
}
