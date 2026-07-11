package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
)

var (
	errWebAppConversationInvalidID      = errors.New("invalid web app conversation id")
	errWebAppConversationInvalidAgent   = errors.New("invalid web app conversation agent id")
	errWebAppConversationInvalidAccount = errors.New("invalid web app conversation account id")
	errWebAppConversationNotFound       = errors.New("web app conversation not found")
	errWebAppConversationAccessDenied   = errors.New("web app conversation access denied")
	errWebAppConversationServiceMissing = errors.New("web app conversation service missing")
)

type webAppConversationAccessService interface {
	GetConversationByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*conversation.AgentConversation, error)
}

func validateWebAppConversationAccess(ctx context.Context, service webAppConversationAccessService, conversationID, agentID, accountID string) error {
	_, _, err := loadWebAppConversationForCaller(ctx, service, conversationID, agentID, accountID)
	return err
}

func loadWebAppConversationForCaller(ctx context.Context, service webAppConversationAccessService, conversationID, agentID, accountID string) (uuid.UUID, *conversation.AgentConversation, error) {
	if service == nil {
		return uuid.Nil, nil, errWebAppConversationServiceMissing
	}

	conversationUUID, err := uuid.Parse(strings.TrimSpace(conversationID))
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("%w: %v", errWebAppConversationInvalidID, err)
	}

	agentUUID, err := uuid.Parse(strings.TrimSpace(agentID))
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("%w: %v", errWebAppConversationInvalidAgent, err)
	}

	accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("%w: %v", errWebAppConversationInvalidAccount, err)
	}

	conv, err := service.GetConversationByIDAndAgent(ctx, conversationUUID, agentUUID)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("%w: %v", errWebAppConversationNotFound, err)
	}
	if !conversationBelongsToAccount(conv, accountUUID) {
		return uuid.Nil, nil, errWebAppConversationAccessDenied
	}

	return conversationUUID, conv, nil
}

func validateWorkflowInputConversationAccess(ctx context.Context, service webAppConversationAccessService, inputs map[string]interface{}, agentID, accountID string, keys ...string) error {
	conversationID := workflowInputConversationID(inputs, keys...)
	if conversationID == "" {
		return nil
	}
	return validateWebAppConversationAccess(ctx, service, conversationID, agentID, accountID)
}

func promoteWorkflowInputConversationIDToSystemInput(inputs map[string]interface{}) {
	if len(inputs) == 0 {
		return
	}
	if workflowInputConversationID(inputs, "sys.conversation_id") != "" {
		return
	}
	if conversationID := workflowInputConversationID(inputs, "conversation_id"); conversationID != "" {
		inputs["sys.conversation_id"] = conversationID
	}
}

func workflowInputConversationID(inputs map[string]interface{}, keys ...string) string {
	if len(inputs) == 0 {
		return ""
	}
	if len(keys) == 0 {
		keys = []string{"sys.conversation_id", "conversation_id"}
	}

	for _, key := range keys {
		raw, exists := inputs[key]
		if !exists || raw == nil {
			continue
		}
		if value, ok := raw.(string); ok {
			return strings.TrimSpace(value)
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func webAppConversationAccessServiceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errWebAppConversationInvalidID),
		errors.Is(err, errWebAppConversationInvalidAgent),
		errors.Is(err, errWebAppConversationInvalidAccount):
		return fmt.Errorf("invalid web app conversation context: %w", err)
	case errors.Is(err, errWebAppConversationNotFound),
		errors.Is(err, errWebAppConversationAccessDenied):
		return fmt.Errorf("conversation not found: %w", err)
	default:
		return fmt.Errorf("validate web app conversation access: %w", err)
	}
}
