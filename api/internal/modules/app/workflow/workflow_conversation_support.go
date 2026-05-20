package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

func (h *WorkflowHandler) getDialogueCount(conversationID string) int {
	if conversationID == "" {
		return 1
	}

	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return 1
	}

	conv, err := h.advancedChatHandler.GetConversation(conversationUUID)
	if err != nil {
		return 1
	}

	return conv.DialogueCount + 1
}

// MinimalConversation represents a minimal conversation record for database insertion
type MinimalConversation struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID         string    `gorm:"type:uuid;not null" json:"app_id"`
	Mode          string    `gorm:"type:varchar(255);not null" json:"mode"`
	Name          string    `gorm:"type:varchar(255);not null" json:"name"`
	Status        string    `gorm:"type:varchar(255);not null;default:'normal'" json:"status"`
	FromSource    string    `gorm:"type:varchar(255);not null" json:"from_source"`
	FromAccountID *string   `gorm:"type:uuid" json:"from_account_id"`
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`
}

// TableName specifies table name for MinimalConversation
func (MinimalConversation) TableName() string {
	return "conversations"
}

// createConversationRecord creates a new conversation record in the database and returns its ID
func (h *WorkflowHandler) createConversationRecord(workspaceID, appID, accountID string) (string, error) {
	logger.Debug("creating workflow conversation record",
		zap.String("workspace_id", workspaceID),
		zap.String("agent_id", appID),
		zap.String("account_id", accountID),
	)

	// Use the new AdvancedChatWorkflowHandler to create conversation
	conversationID, err := h.advancedChatHandler.CreateConversationRecord(workspaceID, appID, accountID)
	if err != nil {
		logger.Error("failed to create workflow conversation record",
			zap.String("workspace_id", workspaceID),
			zap.String("agent_id", appID),
			zap.String("account_id", accountID),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create conversation record: %w", err)
	}

	logger.Debug("workflow conversation record created", zap.String("conversation_id", conversationID))
	return conversationID, nil
}

// createConversationRecordWithParams creates a new conversation record with additional parameters
func (h *WorkflowHandler) createConversationRecordWithParams(workspaceID, appID, accountID, fromSource, invokeFrom string, inputs map[string]interface{}, overrideModelConfigs *string, webAppID *string) (string, error) {
	logger.Debug("creating workflow conversation record with params",
		zap.String("workspace_id", workspaceID),
		zap.String("agent_id", appID),
		zap.String("account_id", accountID),
		zap.String("from_source", fromSource),
		zap.String("invoke_from", invokeFrom),
		zap.Bool("has_web_app_id", webAppID != nil),
	)

	// Use the new AdvancedChatWorkflowHandler to create conversation with params
	conversationID, err := h.advancedChatHandler.CreateConversationRecordWithParams(workspaceID, appID, accountID, fromSource, invokeFrom, inputs, overrideModelConfigs, webAppID)
	if err != nil {
		logger.Error("failed to create workflow conversation record with params",
			zap.String("workspace_id", workspaceID),
			zap.String("agent_id", appID),
			zap.String("account_id", accountID),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create conversation record: %w", err)
	}

	logger.Debug("workflow conversation record created", zap.String("conversation_id", conversationID))
	return conversationID, nil
}

func (h *WorkflowHandler) loadConversationHistory(conversationID string, maxRounds *int) ([]map[string]interface{}, error) {
	messages, err := h.advancedChatHandler.GetConversationMessages(uuid.MustParse(conversationID))
	if err != nil {
		return nil, err
	}

	var history []map[string]interface{}
	for _, msg := range messages {
		if msg.Query != "" {
			history = append(history, map[string]interface{}{
				"role":       "user",
				"content":    msg.Query,
				"message_id": msg.ID.String(),
			})
		}

		if msg.Answer != "" {
			history = append(history, map[string]interface{}{
				"role":       "assistant",
				"content":    msg.Answer,
				"message_id": msg.ID.String(),
			})
		}
	}

	if maxRounds != nil {
		if *maxRounds == 0 {
			logger.Debug("conversation history window is zero", zap.String("conversation_id", conversationID))
			return []map[string]interface{}{}, nil
		} else if *maxRounds > 0 {
			maxMessages := (*maxRounds) * 2
			if len(history) > maxMessages {
				history = history[len(history)-maxMessages:]
				logger.Debug("conversation history limited",
					zap.String("conversation_id", conversationID),
					zap.Int("history_window_size", *maxRounds),
					zap.Int("max_messages", maxMessages),
				)
			}
		}
	}

	logger.Debug("conversation history loaded",
		zap.String("conversation_id", conversationID),
		zap.Int("history_messages_count", len(history)),
	)
	return history, nil
}

func (h *WorkflowHandler) getLatestMessageID(conversationID string) (string, error) {
	messages, err := h.advancedChatHandler.GetConversationMessages(uuid.MustParse(conversationID))
	if err != nil {
		return "", err
	}

	if len(messages) == 0 {
		return "", nil
	}

	return messages[len(messages)-1].ID.String(), nil
}

// updateAgentWorkflowConfig updates the workflow_config for an agent during debugging
func (h *WorkflowHandler) updateAgentWorkflowConfig(ctx context.Context, agentID, conversationID string, historyWindowSize *int, inputs map[string]interface{}) error {
	// Get workflow service with agents repository
	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws.agentsRepo == nil {
		logger.Warn("Workflow service or agents repository not available for workflow_config update")
		return fmt.Errorf("workflow service not properly initialized")
	}

	// Get current agent
	agent, err := ws.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	// Parse existing workflow_config or create new one
	var workflowConfig map[string]interface{}
	if agent.WorkflowConfig != nil && *agent.WorkflowConfig != "" {
		if err := json.Unmarshal([]byte(*agent.WorkflowConfig), &workflowConfig); err != nil {
			logger.Warn("Failed to parse existing workflow_config, creating new one", "error", err)
			workflowConfig = make(map[string]interface{})
		}
	} else {
		workflowConfig = make(map[string]interface{})
	}

	// Update history_window_size if provided
	if historyWindowSize != nil {
		workflowConfig["history_window_size"] = *historyWindowSize
	} else if _, exists := workflowConfig["history_window_size"]; !exists {
		workflowConfig["history_window_size"] = 10 // default value
	}

	// Update conversation_id if provided
	if conversationID != "" {
		workflowConfig["conversation_id"] = conversationID
	} else if _, exists := workflowConfig["conversation_id"]; !exists {
		workflowConfig["conversation_id"] = ""
	}

	// Update variable_config from inputs
	if inputs != nil {
		// Extract variable_config from inputs or create from inputs
		variableConfig := make(map[string]interface{})

		// Map inputs to variable_config (excluding system variables)
		for key, value := range inputs {
			if !strings.HasPrefix(key, "sys.") && key != "conversation_params" {
				variableConfig[key] = value
			}
		}

		workflowConfig["variable_config"] = variableConfig
	} else if _, exists := workflowConfig["variable_config"]; !exists {
		workflowConfig["variable_config"] = make(map[string]interface{})
	}

	// Ensure workflow_id exists (preserve existing value or set empty)
	if _, exists := workflowConfig["workflow_id"]; !exists {
		workflowConfig["workflow_id"] = ""
	}

	// Marshal back to JSON
	configJSON, err := json.Marshal(workflowConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow_config: %w", err)
	}

	// Update agent's workflow_config
	if err := ws.agentsRepo.UpdateWorkflowConfig(ctx, agentID, string(configJSON)); err != nil {
		return fmt.Errorf("failed to update agent workflow_config: %w", err)
	}

	logger.Info("Updated agent workflow_config", "agentID", agentID, "conversationID", conversationID, "historyWindowSize", historyWindowSize)
	return nil
}
