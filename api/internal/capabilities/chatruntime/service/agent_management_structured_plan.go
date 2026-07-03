package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const aiChatStructuredPlanVersion = "aichat.structured_plan.v1"

// AIChatStructuredPlan is a compact, model-visible description of the current
// turn strategy. It is advisory; execution still goes through planned tools and
// tool governance.
type AIChatStructuredPlan struct {
	SchemaVersion        string                      `json:"schema_version"`
	Domain               string                      `json:"domain"`
	Intent               string                      `json:"intent"`
	Target               string                      `json:"target,omitempty"`
	Operations           []AIChatStructuredOperation `json:"operations,omitempty"`
	RequiredToolSequence []AIChatTurnStrategyTool    `json:"required_tool_sequence,omitempty"`
	RequiresApproval     bool                        `json:"requires_approval"`
	ReadBeforeWrite      bool                        `json:"read_before_write,omitempty"`
	CompletionCriteria   []string                    `json:"completion_criteria,omitempty"`
	IfBlocked            string                      `json:"if_blocked,omitempty"`
	ValidationWarnings   []string                    `json:"validation_warnings,omitempty"`
}

type AIChatStructuredOperation struct {
	Action       string   `json:"action"`
	ResourceType string   `json:"resource_type"`
	ResourceName string   `json:"resource_name,omitempty"`
	ToolName     string   `json:"tool_name,omitempty"`
	Effect       string   `json:"effect,omitempty"`
	IfMissing    string   `json:"if_missing,omitempty"`
	Fields       []string `json:"fields,omitempty"`
}

func attachAgentManagementStructuredPlan(parts *chatRequestParts, strategy *AIChatTurnStrategy, query string) *AIChatTurnStrategy {
	if strategy == nil {
		return strategy
	}
	plan := agentManagementStructuredPlanFromStrategy(parts, strategy, query)
	if plan == nil {
		return strategy
	}
	strategy.StructuredPlan = plan
	strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria, plan.CompletionCriteria...)
	if plan.ReadBeforeWrite {
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"for Agent management changes, complete planned read/candidate lookup steps before the governed update or delete step",
		)
	}
	if plan.IfBlocked != "" {
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"if the structured plan is blocked, stop and report the exact missing evidence instead of claiming completion",
		)
	}
	return strategy
}

func agentManagementStructuredPlanFromStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy, query string) *AIChatStructuredPlan {
	if strategy == nil || len(strategy.PlannedTools) == 0 {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	tools := agentManagementStructuredPlanTools(strategy)
	if len(tools) == 0 {
		return nil
	}
	plan := &AIChatStructuredPlan{
		SchemaVersion:        aiChatStructuredPlanVersion,
		Domain:               "agent_management",
		Intent:               agentManagementStructuredIntent(query, tools),
		Target:               strings.TrimSpace(firstNonEmptyString(strategy.TargetPage, contextualTurnCurrentPage(parts))),
		Operations:           agentManagementStructuredOperations(query, tools),
		RequiredToolSequence: tools,
		RequiresApproval:     agentManagementStructuredPlanRequiresApproval(tools),
		ReadBeforeWrite:      agentManagementStructuredPlanReadBeforeWrite(tools),
		CompletionCriteria: []string{
			"execute only the structured Agent-management operations represented by required_tool_sequence",
			"after each mutation tool returns, verify completion from the tool result before the final answer",
			"if a requested candidate resource is missing, do not call the mutation tool; report the missing resource explicitly",
		},
		IfBlocked: "stop_and_report_actual_tool_result",
	}
	plan.ValidationWarnings = agentManagementStructuredPlanValidationWarnings(query, tools)
	return plan
}

func agentManagementStructuredPlanTools(strategy *AIChatTurnStrategy) []AIChatTurnStrategyTool {
	if strategy == nil {
		return nil
	}
	out := []AIChatTurnStrategyTool{}
	for _, tool := range strategy.PlannedTools {
		if !strings.EqualFold(strings.TrimSpace(tool.SkillID), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(tool.ToolName)
		if toolName == "" {
			continue
		}
		copied := tool
		copied.Arguments = mergeTurnStrategyToolArguments(nil, tool.Arguments)
		out = append(out, copied)
	}
	return out
}

func agentManagementStructuredIntent(query string, tools []AIChatTurnStrategyTool) string {
	switch {
	case agentManagementStructuredHasTool(tools, "delete_agents"):
		return "agent.batch_delete"
	case agentManagementStructuredHasTool(tools, "delete_agent"):
		return "agent.delete"
	case agentManagementStructuredHasTool(tools, "create_agent"):
		if agentManagementStructuredHasAnyTool(tools, "update_agent_config", "update_agent_identity") {
			return "agent.create_and_edit"
		}
		return "agent.create"
	case agentManagementStructuredHasTool(tools, "update_agent_config"):
		if agentManagementStructuredPlanHasBindingAction(query, tools) {
			return "agent.update_bindings"
		}
		return "agent.update_config"
	case agentManagementStructuredHasTool(tools, "update_agent_identity"):
		return "agent.update_identity"
	case agentManagementStructuredHasAnyCandidateLookup(tools):
		return "agent.inspect_candidates"
	case agentManagementReadRequested(query):
		return "agent.read"
	default:
		return "agent.inspect"
	}
}

func agentManagementStructuredOperations(query string, tools []AIChatTurnStrategyTool) []AIChatStructuredOperation {
	operations := []AIChatStructuredOperation{}
	add := func(operation AIChatStructuredOperation) {
		operation.Action = strings.TrimSpace(operation.Action)
		operation.ResourceType = strings.TrimSpace(operation.ResourceType)
		if operation.Action == "" || operation.ResourceType == "" {
			return
		}
		for _, existing := range operations {
			if existing.Action == operation.Action &&
				existing.ResourceType == operation.ResourceType &&
				existing.ToolName == operation.ToolName &&
				strings.EqualFold(existing.ResourceName, operation.ResourceName) {
				return
			}
		}
		operations = append(operations, operation)
	}

	for _, tool := range tools {
		toolName := strings.TrimSpace(tool.ToolName)
		switch toolName {
		case "create_agent":
			add(AIChatStructuredOperation{Action: "create", ResourceType: "agent", ToolName: toolName, Effect: "create"})
		case "delete_agent", "delete_agents":
			add(AIChatStructuredOperation{Action: "delete", ResourceType: "agent", ToolName: toolName, Effect: "delete"})
		case "get_agent", "list_agents":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "agent", ToolName: toolName, Effect: "read"})
		case "get_agent_config":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "agent_config", ToolName: toolName, Effect: "read"})
		case "update_agent_identity":
			add(AIChatStructuredOperation{Action: "update", ResourceType: "agent_identity", ToolName: toolName, Effect: "update"})
		case "list_available_models":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "model", ToolName: toolName, Effect: "read"})
		case "list_agent_skill_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "skill", ToolName: toolName, Effect: "read", IfMissing: "stop_and_report"})
		case "list_agent_knowledge_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "knowledge_base", ToolName: toolName, Effect: "read", IfMissing: "stop_and_report"})
		case "list_agent_database_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "database", ToolName: toolName, Effect: "read", IfMissing: "stop_and_report"})
		case "list_agent_database_tables":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "database_table", ToolName: toolName, Effect: "read", IfMissing: "stop_and_report"})
		case "list_agent_workflow_binding_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "workflow", ToolName: toolName, Effect: "read", IfMissing: "stop_and_report"})
		case "update_agent_config":
			operations = append(operations, agentManagementStructuredConfigOperations(query, tool, tools)...)
		}
	}
	return operations
}

func agentManagementStructuredConfigOperations(query string, tool AIChatTurnStrategyTool, tools []AIChatTurnStrategyTool) []AIChatStructuredOperation {
	actions := agentManagementStructuredBindingActions(query, tool, tools)
	fields := operationPlanNormalizedAgentConfigFieldsFromAny(tool.Arguments[operationPlanExpectedUpdatedFieldsKey])
	if len(fields) == 0 {
		fields = agentManagementExpectedConfigUpdateFields(query)
	}
	if len(actions) == 0 {
		return []AIChatStructuredOperation{{
			Action:       "update",
			ResourceType: "agent_config",
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       fields,
		}}
	}
	out := []AIChatStructuredOperation{}
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		action := operationPlanCanonicalAgentConfigBindingAction(actions[field])
		if action == "" {
			continue
		}
		out = append(out, AIChatStructuredOperation{
			Action:       action,
			ResourceType: agentManagementStructuredResourceTypeForConfigField(field),
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       []string{field},
			IfMissing:    "stop_and_report",
		})
	}
	if len(fields) > 0 {
		out = append(out, AIChatStructuredOperation{
			Action:       "update",
			ResourceType: "agent_config",
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       fields,
		})
	}
	if len(out) == 0 {
		out = append(out, AIChatStructuredOperation{Action: "update", ResourceType: "agent_config", ToolName: tool.ToolName, Effect: "update"})
	}
	return out
}

func agentManagementStructuredResourceTypeForConfigField(field string) string {
	switch operationPlanAgentConfigCanonicalField(field) {
	case "enabled_skill_ids":
		return "skill"
	case "knowledge_dataset_ids":
		return "knowledge_base"
	case "database_bindings":
		return "database_table"
	case "workflow_bindings":
		return "workflow"
	default:
		return "agent_config"
	}
}

func agentManagementStructuredPlanRequiresApproval(tools []AIChatTurnStrategyTool) bool {
	for _, tool := range tools {
		if skillLoopToolNameLooksAssetMutation(tool.ToolName) {
			return true
		}
	}
	return false
}

func agentManagementStructuredPlanReadBeforeWrite(tools []AIChatTurnStrategyTool) bool {
	seenRead := false
	for _, tool := range tools {
		if agentManagementStructuredToolIsRead(tool.ToolName) {
			seenRead = true
			continue
		}
		if seenRead && skillLoopToolNameLooksAssetMutation(tool.ToolName) {
			return true
		}
	}
	return false
}

func agentManagementStructuredPlanValidationWarnings(query string, tools []AIChatTurnStrategyTool) []string {
	warnings := []string{}
	if agentManagementCreateMentionIsExistingReferenceOnly(query) && agentManagementStructuredHasTool(tools, "create_agent") {
		warnings = appendUniqueStrings(warnings, "existing_agent_reference_should_not_plan_create_agent")
	}
	if agentManagementStructuredHasTool(tools, "update_agent_config") &&
		!agentManagementStructuredHasTool(tools, "get_agent_config") {
		warnings = appendUniqueStrings(warnings, "update_agent_config_without_prior_config_read")
	}
	for field, lookupTools := range map[string][]string{
		"enabled_skill_ids":     {"list_agent_skill_candidates"},
		"knowledge_dataset_ids": {"list_agent_knowledge_candidates"},
		"database_bindings":     {"list_agent_database_candidates", "list_agent_database_tables"},
		"workflow_bindings":     {"list_agent_workflow_binding_candidates"},
	} {
		action := agentManagementStructuredExpectedBindingAction(query, tools, field)
		if action == "" || action == "unbind" {
			continue
		}
		for _, lookupTool := range lookupTools {
			if !agentManagementStructuredHasTool(tools, lookupTool) {
				warnings = appendUniqueStrings(warnings, "binding_action_missing_candidate_lookup:"+field)
				break
			}
		}
	}
	return warnings
}

func agentManagementStructuredExpectedBindingAction(query string, tools []AIChatTurnStrategyTool, field string) string {
	for _, tool := range tools {
		if !strings.EqualFold(strings.TrimSpace(tool.ToolName), "update_agent_config") {
			continue
		}
		actions := agentManagementStructuredBindingActions(query, tool, tools)
		if action := operationPlanCanonicalAgentConfigBindingAction(actions[field]); action != "" {
			return action
		}
	}
	return ""
}

func agentManagementStructuredToolIsRead(toolName string) bool {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	return strings.HasPrefix(toolName, "get_") ||
		strings.HasPrefix(toolName, "list_") ||
		strings.Contains(toolName, "candidates")
}

func agentManagementStructuredPlanHasBindingAction(query string, tools []AIChatTurnStrategyTool) bool {
	for _, tool := range tools {
		if !strings.EqualFold(strings.TrimSpace(tool.ToolName), "update_agent_config") {
			continue
		}
		if len(agentManagementStructuredBindingActions(query, tool, tools)) > 0 {
			return true
		}
	}
	return false
}

func agentManagementStructuredBindingActions(query string, tool AIChatTurnStrategyTool, tools []AIChatTurnStrategyTool) map[string]string {
	actions := operationPlanAgentConfigBindingActionsFromAny(tool.Arguments[operationPlanExpectedBindingActionsKey])
	if len(actions) == 0 {
		actions = map[string]string{}
	} else {
		copied := map[string]string{}
		for field, action := range actions {
			if canonicalField := operationPlanAgentConfigCanonicalField(field); canonicalField != "" {
				copied[canonicalField] = operationPlanCanonicalAgentConfigBindingAction(action)
			}
		}
		actions = copied
	}
	query = strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(query)))
	if query == "" {
		return actions
	}
	defaultAction := agentBindingExpectedActionFromText(query)
	add := func(field string, enabled bool, markers []string) {
		if !enabled {
			return
		}
		field = operationPlanAgentConfigCanonicalField(field)
		if field == "" || operationPlanCanonicalAgentConfigBindingAction(actions[field]) != "" {
			return
		}
		action := agentBindingExpectedActionForResource(query, markers)
		if action == "" {
			action = defaultAction
		}
		if action == "" {
			action = "bind"
		}
		actions[field] = operationPlanCanonicalAgentConfigBindingAction(action)
	}

	add("enabled_skill_ids",
		agentManagementSkillBindingRequested(query) || agentManagementStructuredHasTool(tools, "list_agent_skill_candidates"),
		[]string{"skill", "\u6280\u80fd"},
	)
	requiredTools := requiredAgentBindingMutationTools(query)
	add("knowledge_dataset_ids",
		stringSliceContainsFold(requiredTools, "replace_agent_knowledge_bindings") ||
			agentManagementStructuredHasTool(tools, "list_agent_knowledge_candidates"),
		[]string{"knowledge", "\u77e5\u8bc6\u5e93"},
	)
	add("database_bindings",
		stringSliceContainsFold(requiredTools, "replace_agent_database_bindings") ||
			agentManagementStructuredHasTool(tools, "list_agent_database_candidates") ||
			agentManagementStructuredHasTool(tools, "list_agent_database_tables"),
		[]string{"database", "table", "\u6570\u636e\u5e93", "\u6570\u636e\u8868"},
	)
	add("workflow_bindings",
		stringSliceContainsFold(requiredTools, "replace_agent_workflow_bindings") ||
			agentManagementStructuredHasTool(tools, "list_agent_workflow_binding_candidates"),
		[]string{"workflow", "\u5de5\u4f5c\u6d41"},
	)
	if len(actions) == 0 {
		return nil
	}
	return actions
}

func agentManagementStructuredHasAnyCandidateLookup(tools []AIChatTurnStrategyTool) bool {
	return agentManagementStructuredHasAnyTool(tools,
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
	)
}

func agentManagementStructuredHasAnyTool(tools []AIChatTurnStrategyTool, toolNames ...string) bool {
	for _, toolName := range toolNames {
		if agentManagementStructuredHasTool(tools, toolName) {
			return true
		}
	}
	return false
}

func agentManagementStructuredHasTool(tools []AIChatTurnStrategyTool, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.ToolName), toolName) {
			return true
		}
	}
	return false
}
