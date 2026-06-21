package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultWorkflowConversationNamePrefix = "Conversation "
	defaultWorkflowConversationNameLayout = "2006-01-02 15:04:05"
	workflowConversationTitleTimeout      = 15 * time.Second
	workflowConversationTitleMaxTurns     = 3
)

type workflowConversationTitleParams struct {
	WorkspaceID    string
	OrganizationID string
	AgentID        string
	AccountID      uuid.UUID
	ConversationID uuid.UUID
	WebAppID       string
}

func newWorkflowConversationTitleGenerator(llmClient interface{}, graphFlowService *graphflow.Service) titlegen.Service {
	llm, ok := llmClient.(llmclient.LLMClient)
	if !ok || llm == nil || graphFlowService == nil || graphFlowService.DefaultModelSvc == nil {
		return nil
	}
	return titlegen.NewService(llm, graphFlowService.DefaultModelSvc)
}

func (s *WorkflowService) enqueueWebAppConversationTitleGeneration(ctx context.Context, params workflowConversationTitleParams) {
	if s == nil || s.advancedChatHandler == nil {
		return
	}
	if s.conversationTitleGen == nil {
		logger.WarnContext(ctx, "workflow conversation title generator is not configured", "conversation_id", params.ConversationID.String())
		return
	}
	if params.ConversationID == uuid.Nil || params.AccountID == uuid.Nil || strings.TrimSpace(params.WebAppID) == "" {
		return
	}

	go func() {
		titleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workflowConversationTitleTimeout)
		defer cancel()

		if err := s.generateWebAppConversationTitle(titleCtx, params); err != nil {
			logger.WarnContext(titleCtx, "failed to generate workflow conversation title", "conversation_id", params.ConversationID.String(), err)
		}
	}()
}

func (s *WorkflowService) generateWebAppConversationTitle(ctx context.Context, params workflowConversationTitleParams) error {
	conversationSvc := s.advancedChatHandler.conversationService
	messageSvc := s.advancedChatHandler.messageService
	if conversationSvc == nil || messageSvc == nil {
		return fmt.Errorf("conversation service is not configured")
	}

	agentID, err := uuid.Parse(strings.TrimSpace(params.AgentID))
	if err != nil || agentID == uuid.Nil {
		return fmt.Errorf("valid agent id is required for title generation")
	}

	conv, err := conversationSvc.GetConversationByIDAndAgent(ctx, params.ConversationID, agentID)
	if err != nil {
		return err
	}
	if conv == nil {
		return fmt.Errorf("conversation not found")
	}
	if !conversationBelongsToAccount(conv, params.AccountID) {
		return fmt.Errorf("conversation not found: %w", errWebAppConversationAccessDenied)
	}
	if !isDefaultWorkflowConversationName(conv.Name) {
		return nil
	}

	messages, err := messageSvc.GetConversationMessages(ctx, params.ConversationID)
	if err != nil {
		return err
	}
	titleMessages := buildWorkflowConversationTitleMessages(messages)
	if len(titleMessages) == 0 {
		return fmt.Errorf("conversation has no message content for title generation")
	}

	organizationID := strings.TrimSpace(params.OrganizationID)
	if organizationID == "" {
		organizationID = s.getOrganizationIDByWorkspace(ctx, params.WorkspaceID)
	}
	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil || organizationUUID == uuid.Nil {
		return fmt.Errorf("valid organization id is required for title generation")
	}

	var workspaceUUID *uuid.UUID
	if parsedWorkspaceID, err := uuid.Parse(strings.TrimSpace(params.WorkspaceID)); err == nil && parsedWorkspaceID != uuid.Nil {
		workspaceUUID = &parsedWorkspaceID
	}

	result, err := s.conversationTitleGen.Generate(ctx, titlegen.GenerateRequest{
		OrganizationID: organizationUUID,
		AccountID:      params.AccountID,
		WorkspaceID:    workspaceUUID,
		AppID:          strings.TrimSpace(params.AgentID),
		AppType:        string(InvokeFromWebApp),
		SessionID:      params.ConversationID.String(),
		ConversationID: params.ConversationID.String(),
		Messages:       titleMessages,
		FallbackTitle:  conv.Name,
	})
	if err != nil {
		return err
	}
	if result == nil || result.Source != titlegen.SourceModel {
		return fmt.Errorf("title generator returned fallback result")
	}

	nextName := strings.TrimSpace(result.Title)
	if nextName == "" {
		return fmt.Errorf("title generator returned empty title")
	}
	if isDefaultWorkflowConversationName(nextName) {
		return fmt.Errorf("title generator returned a default timestamp title")
	}

	updated, err := conversationSvc.UpdateConversationNameIfCurrent(ctx, params.ConversationID, conv.Name, nextName)
	if err != nil {
		return err
	}
	if !updated {
		logger.InfoContext(ctx, "workflow conversation title was not updated because name changed", "conversation_id", params.ConversationID.String())
	}
	return nil
}

func isDefaultWorkflowConversationName(name string) bool {
	timestamp := strings.TrimPrefix(strings.TrimSpace(name), defaultWorkflowConversationNamePrefix)
	if timestamp == strings.TrimSpace(name) || timestamp == "" {
		return false
	}
	_, err := time.Parse(defaultWorkflowConversationNameLayout, timestamp)
	return err == nil
}

func buildWorkflowConversationTitleMessages(messages []*conversation.AgentMessage) []titlegen.Message {
	titleMessages := make([]titlegen.Message, 0, workflowConversationTitleMaxTurns*2)
	turns := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		query := strings.TrimSpace(message.Query)
		answer := strings.TrimSpace(message.Answer)
		if query == "" && answer == "" {
			continue
		}
		if query != "" {
			titleMessages = append(titleMessages, titlegen.Message{Role: "user", Content: query})
		}
		if answer != "" {
			titleMessages = append(titleMessages, titlegen.Message{Role: "assistant", Content: answer})
		}
		turns++
		if turns >= workflowConversationTitleMaxTurns {
			break
		}
	}
	return titleMessages
}

func workflowConversationTitleReady(status string) bool {
	return strings.TrimSpace(status) != conversation.AgentMessageStatusRunning
}

func (h *WorkflowHandler) enqueueWebAppConversationTitleGeneration(ctx context.Context, systemInputs map[string]interface{}, requestInputs map[string]interface{}, messageData approvalConversationMessageData) {
	if h == nil || !workflowConversationTitleReady(messageData.Status) {
		return
	}
	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok || workflowService == nil {
		return
	}

	webAppID := ""
	if messageData.WebAppID != nil {
		webAppID = *messageData.WebAppID
	}

	workflowService.enqueueWebAppConversationTitleGeneration(ctx, workflowConversationTitleParams{
		WorkspaceID:    workflowTitleStringInput(systemInputs, requestInputs, "sys.workspace_id", "sys.tenant_id"),
		OrganizationID: workflowTitleStringInput(systemInputs, requestInputs, "sys.organization_id"),
		AgentID:        messageData.AgentID.String(),
		AccountID:      messageData.FromUserID,
		ConversationID: messageData.ConversationID,
		WebAppID:       webAppID,
	})
}

func workflowTitleStringInput(systemInputs map[string]interface{}, requestInputs map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := systemInputs[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := requestInputs[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
