package graph_engine

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func (e *WorkflowEngine) getGraphInitParams() entities.GraphInitParams {
	workspaceID := ""
	organizationID := ""
	billingSubjectType := ""
	appID := ""
	workflowType := entities.WorkflowTypeWorkflow
	workflowID := ""
	userID := ""
	userFrom := entities.UserFromAccount
	invokeFrom := entities.InvokeFromServiceAPI
	callDepth := 0
	graphConfig := make(map[string]interface{})

	if e.runtimeState != nil {
		if e.runtimeState.VariablePool != nil {
			if sysVars := e.runtimeState.VariablePool.SystemVariables; sysVars != nil {
				sysVarsDict := sysVars.ToDict()

				if workspaceIDVal, ok := sysVarsDict["workspace_id"]; ok {
					if str, ok := workspaceIDVal.(string); ok {
						workspaceID = str
					}
				}
				if organizationIDVal, ok := sysVarsDict["organization_id"]; ok {
					if str, ok := organizationIDVal.(string); ok {
						organizationID = str
					}
				}
				if billingSubjectTypeVal, ok := sysVarsDict["billing_subject_type"]; ok {
					if str, ok := billingSubjectTypeVal.(string); ok {
						billingSubjectType = str
					}
				}

				if agentIDVal, ok := sysVarsDict["agent_id"]; ok {
					if str, ok := agentIDVal.(string); ok {
						appID = str
					}
				}
				if workflowIDVal, ok := sysVarsDict["workflow_id"]; ok {
					if str, ok := workflowIDVal.(string); ok {
						workflowID = str
					}
				}
				if userIDVal, ok := sysVarsDict["user_id"]; ok {
					if str, ok := userIDVal.(string); ok {
						userID = str
					}
				}

				if wftVal, ok := sysVarsDict["workflow_type"]; ok {
					if str, ok := wftVal.(string); ok {
						switch str {
						case "chat", "advanced-chat", "CONVERSATION_WORKFLOW":
							workflowType = entities.WorkflowTypeChat
						default:
							workflowType = entities.WorkflowTypeWorkflow
						}
					}
				}
			}
		}
	}

	if e.graph != nil {
		if e.graph.Config != nil {
			graphConfig = e.graph.Config
		}
	}

	return entities.GraphInitParams{
		TenantID:           workspaceID, // Legacy mirror for old workflow paths.
		WorkspaceID:        workspaceID,
		OrganizationID:     organizationID,
		BillingSubjectType: billingSubjectType,
		AppID:              appID,
		WorkflowType:       workflowType,
		WorkflowID:         workflowID,
		UserFrom:           userFrom,
		UserID:             userID,
		GraphConfig:        graphConfig,
		InvokeFrom:         invokeFrom,
		CallDepth:          callDepth,
	}
}
