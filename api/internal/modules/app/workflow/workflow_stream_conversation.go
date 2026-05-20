package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

type workflowStreamSystemInputParams struct {
	WorkspaceID        string
	AppID              string
	AccountID          string
	WorkflowID         string
	BillingSubjectType string
	ProcessedFiles     interface{}
	Inputs             map[string]interface{}
}

func (h *WorkflowHandler) prepareWorkflowStreamSystemInputs(ctx context.Context, w http.ResponseWriter, params workflowStreamSystemInputParams) (map[string]interface{}, bool) {
	systemInputs := map[string]interface{}{
		"sys.files":           params.ProcessedFiles,
		"sys.user_id":         params.AccountID,
		"sys.agent_id":        params.AppID,
		"sys.workflow_id":     params.WorkflowID,
		"sys.workflow_run_id": "",
		"sys.tenant_id":       params.WorkspaceID,
		"sys.workspace_id":    params.WorkspaceID,
	}
	if params.BillingSubjectType != "" {
		systemInputs["sys.billing_subject_type"] = params.BillingSubjectType
	}
	if organizationID := resolveRunOrganizationID(ctx, h.enterpriseService, params.WorkspaceID, params.Inputs); organizationID != "" {
		systemInputs["sys.organization_id"] = organizationID
	}

	if params.Inputs == nil {
		return h.prepareDefaultWorkflowStreamConversation(ctx, w, params, systemInputs)
	}

	logger.DebugContext(ctx, "workflow stream request inputs received",
		zap.Int("input_count", len(params.Inputs)),
	)
	if convID, exists := params.Inputs["sys.conversation_id"]; exists && convID != "" {
		systemInputs["sys.conversation_id"] = convID
		logger.DebugContext(ctx, "workflow stream using existing conversation",
			zap.Any("conversation_id", convID),
		)
	} else if convID, exists := params.Inputs["conversation_id"]; exists && convID != "" {
		systemInputs["sys.conversation_id"] = convID
		logger.DebugContext(ctx, "workflow stream using existing conversation",
			zap.Any("conversation_id", convID),
		)
	} else {
		if ok := h.createWorkflowStreamConversation(ctx, w, params, systemInputs); !ok {
			return nil, false
		}
	}

	if query, exists := params.Inputs["sys.query"]; exists {
		systemInputs["sys.query"] = query
	}
	if dialogueCount, exists := params.Inputs["sys.dialogue_count"]; exists {
		systemInputs["sys.dialogue_count"] = dialogueCount
	} else {
		systemInputs["sys.dialogue_count"] = 1
	}
	if conversationHistory, exists := params.Inputs["sys.conversation_history"]; exists {
		systemInputs["sys.conversation_history"] = conversationHistory
		logger.DebugContext(ctx, "workflow stream added conversation history",
			zap.Int("history_messages_count", conversationHistoryCount(conversationHistory)),
		)
	}
	if parentMessageID, exists := params.Inputs["sys.parent_message_id"]; exists {
		systemInputs["sys.parent_message_id"] = parentMessageID
		logger.DebugContext(ctx, "workflow stream added parent message id",
			zap.Any("parent_message_id", parentMessageID),
		)
	}

	return systemInputs, true
}

func (h *WorkflowHandler) createWorkflowStreamConversation(ctx context.Context, w http.ResponseWriter, params workflowStreamSystemInputParams, systemInputs map[string]interface{}) bool {
	fromSource := "account"
	invokeFrom := "workflow"
	var overrideModelConfigs *string

	if convParams, ok := params.Inputs["conversation_params"].(map[string]interface{}); ok {
		if fs, ok := convParams["from_source"].(string); ok {
			fromSource = fs
		}
		if iv, ok := convParams["invoke_from"].(string); ok {
			invokeFrom = iv
		}
		if omc, ok := convParams["override_model_configs"].(string); ok {
			overrideModelConfigs = &omc
		}
	}

	var webAppID *string
	if waid, exists := params.Inputs["sys.web_app_id"].(string); exists && waid != "" {
		webAppID = &waid
	}

	newConversationID, err := h.createConversationRecordWithParams(params.WorkspaceID, params.AppID, params.AccountID, fromSource, invokeFrom, params.Inputs, overrideModelConfigs, webAppID)
	if err != nil {
		logger.CriticalContext(ctx, "failed to create conversation record", "agent_id", params.AppID, "workspace_id", params.WorkspaceID, err)
		h.sendSSEError(ctx, w, fmt.Sprintf("Failed to create conversation: %v", err))
		return false
	}
	systemInputs["sys.conversation_id"] = newConversationID
	logger.DebugContext(ctx, "workflow stream created conversation",
		zap.String("conversation_id", newConversationID),
	)
	return true
}

func (h *WorkflowHandler) prepareDefaultWorkflowStreamConversation(ctx context.Context, w http.ResponseWriter, params workflowStreamSystemInputParams, systemInputs map[string]interface{}) (map[string]interface{}, bool) {
	logger.DebugContext(ctx, "workflow stream request inputs are nil")

	newConversationID, err := h.createConversationRecord(params.WorkspaceID, params.AppID, params.AccountID)
	if err != nil {
		logger.CriticalContext(ctx, "failed to create default conversation record", "agent_id", params.AppID, "workspace_id", params.WorkspaceID, err)
		h.sendSSEError(ctx, w, fmt.Sprintf("Failed to create conversation: %v", err))
		return nil, false
	}
	systemInputs["sys.conversation_id"] = newConversationID
	systemInputs["sys.dialogue_count"] = 1
	logger.DebugContext(ctx, "workflow stream created default conversation",
		zap.String("conversation_id", newConversationID),
		zap.Int("dialogue_count", 1),
	)
	return systemInputs, true
}

func (h *WorkflowHandler) updateConversationWorkflowConfigAsync(ctx context.Context, appID, workflowID, runType string) {
	if runType != "CONVERSATION_WORKFLOW" {
		return
	}

	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws.agentsRepo == nil {
		return
	}

	go func() {
		agent, err := ws.agentsRepo.GetByID(context.Background(), appID)
		if err != nil || agent == nil {
			return
		}

		workflowConfig := make(map[string]interface{})
		if agent.WorkflowConfig != nil && *agent.WorkflowConfig != "" {
			if err := json.Unmarshal([]byte(*agent.WorkflowConfig), &workflowConfig); err != nil {
				workflowConfig = make(map[string]interface{})
			}
		}

		workflowConfig["workflow_id"] = workflowID
		if _, exists := workflowConfig["history_window_size"]; !exists {
			workflowConfig["history_window_size"] = 10
		}
		if _, exists := workflowConfig["variable_config"]; !exists {
			workflowConfig["variable_config"] = make(map[string]interface{})
		}
		if _, exists := workflowConfig["conversation_id"]; !exists {
			workflowConfig["conversation_id"] = ""
		}

		configJSON, err := json.Marshal(workflowConfig)
		if err != nil {
			return
		}
		configStr := string(configJSON)
		agent.WorkflowConfig = &configStr
		if updateErr := ws.agentsRepo.Update(context.Background(), agent); updateErr != nil {
			logger.ErrorContext(ctx, "failed to update agent workflow config", "agent_id", appID, updateErr)
			return
		}
		logger.Info("Updated agent workflow_config during workflow run", "agentID", appID, "workflowID", workflowID)
	}()
}
