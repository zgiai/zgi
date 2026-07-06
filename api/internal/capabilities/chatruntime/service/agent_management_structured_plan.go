package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	aiChatStructuredPlanVersion              = "aichat.structured_plan.v1"
	aiChatStructuredCreatedAgentsOutputAlias = "created_agents"
	aiChatStructuredFirstCreatedAgentIDExpr  = "$created_agents[index=0].agent_id"
)

// AIChatStructuredPlan is a compact, model-visible description of the current
// turn strategy. It is advisory: execution choices remain with the model and
// every side-effecting tool still goes through normal tool governance.
type AIChatStructuredPlan struct {
	SchemaVersion        string                      `json:"schema_version"`
	Domain               string                      `json:"domain"`
	Intent               string                      `json:"intent"`
	Target               string                      `json:"target,omitempty"`
	Operations           []AIChatStructuredOperation `json:"operations,omitempty"`
	RequiredToolSequence []AIChatTurnStrategyTool    `json:"required_tool_sequence,omitempty"`
	CapabilityGoals      []AIChatAgentCapabilityGoal `json:"capability_goals,omitempty"`
	RequiresApproval     bool                        `json:"requires_approval"`
	ReadBeforeWrite      bool                        `json:"read_before_write,omitempty"`
	CompletionCriteria   []string                    `json:"completion_criteria,omitempty"`
	IfBlocked            string                      `json:"if_blocked,omitempty"`
	ValidationWarnings   []string                    `json:"validation_warnings,omitempty"`
}

type AIChatStructuredOperation struct {
	Action             string            `json:"action"`
	ResourceType       string            `json:"resource_type"`
	ResourceName       string            `json:"resource_name,omitempty"`
	SkillID            string            `json:"skill_id,omitempty"`
	ToolName           string            `json:"tool_name,omitempty"`
	Effect             string            `json:"effect,omitempty"`
	IfMissing          string            `json:"if_missing,omitempty"`
	Fields             []string          `json:"fields,omitempty"`
	Goal               string            `json:"goal,omitempty"`
	OutputAlias        string            `json:"output_alias,omitempty"`
	Arguments          map[string]string `json:"arguments,omitempty"`
	ArgsBinding        map[string]string `json:"args_binding,omitempty"`
	Status             string            `json:"status,omitempty"`
	LastInvocationID   string            `json:"last_invocation_id,omitempty"`
	LastInvocationKind string            `json:"last_invocation_kind,omitempty"`
	Error              string            `json:"error,omitempty"`
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

func attachContextualSidebarStructuredPlan(parts *chatRequestParts, strategy *AIChatTurnStrategy, query string) *AIChatTurnStrategy {
	if strategy == nil || strategy.StructuredPlan != nil {
		return strategy
	}
	if parts == nil || !isContextualAIChatSurface(parts.Surface) {
		return strategy
	}
	plan := contextualSidebarStructuredPlanFromStrategy(parts, strategy, query)
	if plan == nil {
		return strategy
	}
	strategy.StructuredPlan = plan
	strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria, plan.CompletionCriteria...)
	if plan.IfBlocked != "" {
		strategy.Avoid = appendUniqueStrings(strategy.Avoid,
			"if the structured sidebar plan is blocked, continue from actual tool/page evidence or report the blocker instead of claiming completion",
		)
	}
	return strategy
}

func contextualSidebarStructuredPlanFromStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy, query string) *AIChatStructuredPlan {
	if strategy == nil {
		return nil
	}
	tools := contextualSidebarStructuredPlanTools(strategy)
	if len(tools) == 0 {
		return nil
	}
	plan := &AIChatStructuredPlan{
		SchemaVersion:        aiChatStructuredPlanVersion,
		Domain:               contextualSidebarStructuredPlanDomain(strategy, tools),
		Intent:               contextualSidebarStructuredIntent(strategy, query, tools),
		Target:               strings.TrimSpace(firstNonEmptyString(strategy.TargetPage, contextualTurnCurrentPage(parts))),
		Operations:           contextualSidebarStructuredOperations(tools),
		RequiredToolSequence: tools,
		RequiresApproval:     contextualSidebarStructuredPlanRequiresApproval(tools),
		ReadBeforeWrite:      contextualSidebarStructuredPlanReadBeforeWrite(tools),
		CompletionCriteria: []string{
			"execute or intentionally skip every pending structured sidebar operation before the final answer",
			"base the final answer on matching tool, page, or client-action evidence instead of only the initial plan",
			"if a later structured operation is blocked, report the exact completed and blocked operations",
		},
		IfBlocked: "continue_from_actual_evidence_or_report_blocker",
	}
	if len(plan.Operations) == 0 {
		return nil
	}
	if len(strategy.CapabilityGoals) > 0 {
		plan.CapabilityGoals = append([]AIChatAgentCapabilityGoal(nil), strategy.CapabilityGoals...)
	}
	return plan
}

func contextualSidebarStructuredPlanTools(strategy *AIChatTurnStrategy) []AIChatTurnStrategyTool {
	if strategy == nil {
		return nil
	}
	out := []AIChatTurnStrategyTool{}
	add := func(tool AIChatTurnStrategyTool) {
		skillID := strings.TrimSpace(tool.SkillID)
		toolName := strings.TrimSpace(tool.ToolName)
		if skillID == "" || toolName == "" {
			return
		}
		copied := tool
		copied.SkillID = skillID
		copied.ToolName = toolName
		copied.Arguments = mergeTurnStrategyToolArguments(nil, tool.Arguments)
		copied.ArgsBinding = mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)
		out = append(out, copied)
	}
	if strategy.RequiredNextTool != nil {
		add(*strategy.RequiredNextTool)
	}
	for _, tool := range strategy.PlannedTools {
		add(tool)
	}
	return out
}

func contextualSidebarStructuredPlanDomain(strategy *AIChatTurnStrategy, tools []AIChatTurnStrategyTool) string {
	if strategy != nil {
		switch strings.TrimSpace(strategy.Intent) {
		case "navigate_console_page":
			return "console_navigation"
		case "create_managed_file", "generate_temporary_file", "delete_file", "read_file":
			return "file_management"
		}
	}
	for _, tool := range tools {
		switch strings.TrimSpace(tool.SkillID) {
		case skills.SkillFileManager, skills.SkillFileGenerator, skills.SkillFileReader:
			return "file_management"
		case skills.SkillConsoleNavigator:
			return "console_navigation"
		}
	}
	return "contextual_sidebar"
}

func contextualSidebarStructuredIntent(strategy *AIChatTurnStrategy, query string, tools []AIChatTurnStrategyTool) string {
	if strategy != nil && strings.TrimSpace(strategy.Intent) != "" {
		return strings.TrimSpace(strategy.Intent)
	}
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.SkillID), skills.SkillConsoleNavigator) &&
			strings.EqualFold(strings.TrimSpace(tool.ToolName), "navigate") {
			return "navigate_console_page"
		}
	}
	return strings.TrimSpace(firstNonEmptyString(query, "contextual_sidebar_task"))
}

func contextualSidebarStructuredOperations(tools []AIChatTurnStrategyTool) []AIChatStructuredOperation {
	operations := []AIChatStructuredOperation{}
	add := func(operation AIChatStructuredOperation) {
		operation.SkillID = strings.TrimSpace(operation.SkillID)
		operation.ToolName = strings.TrimSpace(operation.ToolName)
		operation.Action = strings.TrimSpace(operation.Action)
		operation.ResourceType = strings.TrimSpace(operation.ResourceType)
		if operation.SkillID == "" || operation.ToolName == "" || operation.Action == "" || operation.ResourceType == "" {
			return
		}
		for _, existing := range operations {
			if strings.EqualFold(existing.SkillID, operation.SkillID) &&
				strings.EqualFold(existing.ToolName, operation.ToolName) &&
				existing.Action == operation.Action &&
				existing.ResourceType == operation.ResourceType &&
				strings.EqualFold(existing.ResourceName, operation.ResourceName) {
				return
			}
		}
		operations = append(operations, operation)
	}
	for _, tool := range tools {
		skillID := strings.TrimSpace(tool.SkillID)
		toolName := strings.TrimSpace(tool.ToolName)
		action, resourceType, effect := contextualSidebarStructuredOperationShape(skillID, toolName)
		add(AIChatStructuredOperation{
			Action:       action,
			ResourceType: resourceType,
			SkillID:      skillID,
			ToolName:     toolName,
			Effect:       effect,
		})
	}
	return operations
}

func contextualSidebarStructuredOperationShape(skillID string, toolName string) (string, string, string) {
	switch {
	case isConsoleNavigatorNavigateTool(skillID, toolName):
		return "navigate", "page", "navigate"
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return "create", "temporary_file", "create"
	case isFileManagerSaveToolCall(skillID, toolName):
		return "create", "managed_file", "create"
	case isFileManagerDeleteToolCall(skillID, toolName):
		return "delete", "file", "delete"
	case strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileReader):
		return "read", "file", "read"
	default:
		if skillLoopToolNameLooksAssetMutation(toolName) {
			return "execute", "asset", "update"
		}
		return "read", "context", "read"
	}
}

func contextualSidebarStructuredPlanRequiresApproval(tools []AIChatTurnStrategyTool) bool {
	for _, tool := range tools {
		if contextualSidebarStructuredToolRequiresApproval(tool.SkillID, tool.ToolName) {
			return true
		}
	}
	return false
}

func contextualSidebarStructuredToolRequiresApproval(skillID string, toolName string) bool {
	switch {
	case isConsoleNavigatorNavigateTool(skillID, toolName):
		return false
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return false
	case isFileManagerSaveToolCall(skillID, toolName), isFileManagerDeleteToolCall(skillID, toolName):
		return true
	default:
		return skillLoopToolNameLooksAssetMutation(toolName)
	}
}

func contextualSidebarStructuredPlanReadBeforeWrite(tools []AIChatTurnStrategyTool) bool {
	seenRead := false
	for _, tool := range tools {
		if skillLoopToolNameLooksReadOnly(tool.ToolName) ||
			isConsoleNavigatorNavigateTool(tool.SkillID, tool.ToolName) {
			seenRead = true
			continue
		}
		if seenRead && skillLoopToolNameLooksAssetMutation(tool.ToolName) {
			return true
		}
	}
	return false
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
		Intent:               agentManagementStructuredIntent(query, tools, strategy.CapabilityGoals),
		Target:               strings.TrimSpace(firstNonEmptyString(strategy.TargetPage, contextualTurnCurrentPage(parts))),
		Operations:           agentManagementStructuredOperations(query, tools, strategy.CapabilityGoals),
		RequiredToolSequence: tools,
		CapabilityGoals:      append([]AIChatAgentCapabilityGoal(nil), strategy.CapabilityGoals...),
		RequiresApproval:     agentManagementStructuredPlanRequiresApproval(tools),
		ReadBeforeWrite:      agentManagementStructuredPlanReadBeforeWrite(tools),
		CompletionCriteria: []string{
			"execute only the structured Agent-management operations relevant to the user's request",
			"after each mutation tool returns, verify completion from the tool result before the final answer",
			"if a requested candidate resource is missing, do not call the mutation tool; report the missing resource explicitly",
		},
		IfBlocked: "stop_and_report_actual_tool_result",
	}
	plan.ValidationWarnings = agentManagementStructuredPlanValidationWarnings(query, tools, strategy.CapabilityGoals)
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
		copied.ArgsBinding = mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)
		out = append(out, copied)
	}
	return agentManagementToolSequenceWithCreateBindings(out)
}

func agentManagementToolSequenceWithCreateBindings(tools []AIChatTurnStrategyTool) []AIChatTurnStrategyTool {
	if len(tools) == 0 || !agentManagementStructuredHasTool(tools, "create_agent") {
		return tools
	}
	out := append([]AIChatTurnStrategyTool(nil), tools...)
	createSeen := false
	createStepID := ""
	for idx := range out {
		toolName := strings.TrimSpace(out[idx].ToolName)
		if strings.EqualFold(toolName, "create_agent") {
			if strings.TrimSpace(out[idx].OutputAlias) == "" {
				out[idx].OutputAlias = aiChatStructuredCreatedAgentsOutputAlias
			}
			createStepID = aiChatTurnStrategyToolStepID(out[idx])
			createSeen = true
			continue
		}
		if !createSeen || !agentManagementToolRequiresSingleAgentTarget(toolName) {
			continue
		}
		if out[idx].ArgsBinding == nil {
			out[idx].ArgsBinding = map[string]string{}
		}
		if strings.TrimSpace(out[idx].ArgsBinding["agent_id"]) == "" {
			out[idx].ArgsBinding["agent_id"] = aiChatStructuredFirstCreatedAgentIDExpr
		}
		if strings.TrimSpace(out[idx].WaitForStepID) == "" {
			out[idx].WaitForStepID = createStepID
		}
	}
	return out
}

func agentManagementStructuredIntent(query string, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) string {
	switch {
	case agentManagementStructuredHasTool(tools, "delete_agents") &&
		agentManagementStructuredHasTool(tools, "create_agent") &&
		agentManagementStructuredHasAnyTool(tools, "update_agent_config", "update_agent_identity"):
		return "agent.batch_delete_then_create_and_edit"
	case agentManagementStructuredHasTool(tools, "delete_agents") &&
		agentManagementStructuredHasTool(tools, "create_agent"):
		return "agent.batch_delete_then_create"
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
		if agentManagementStructuredPlanHasBindingAction(query, tools, goals) {
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

func agentManagementStructuredOperations(query string, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) []AIChatStructuredOperation {
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
		skillID := strings.TrimSpace(tool.SkillID)
		if skillID == "" {
			skillID = skills.SkillAgentManagement
		}
		switch toolName {
		case "create_agent":
			add(AIChatStructuredOperation{Action: "create", ResourceType: "agent", SkillID: skillID, ToolName: toolName, Effect: "create", OutputAlias: strings.TrimSpace(tool.OutputAlias), Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "delete_agent", "delete_agents":
			add(AIChatStructuredOperation{Action: "delete", ResourceType: "agent", SkillID: skillID, ToolName: toolName, Effect: "delete", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "get_agent", "list_agents":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "agent", SkillID: skillID, ToolName: toolName, Effect: "read", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "get_agent_config":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "agent_config", SkillID: skillID, ToolName: toolName, Effect: "read", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "update_agent_identity":
			add(AIChatStructuredOperation{Action: "update", ResourceType: "agent_identity", SkillID: skillID, ToolName: toolName, Effect: "update", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "list_available_models":
			add(AIChatStructuredOperation{Action: "read", ResourceType: "model", SkillID: skillID, ToolName: toolName, Effect: "read", Arguments: agentManagementStructuredOperationArguments(tool)})
		case "list_agent_skill_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "skill", SkillID: skillID, ToolName: toolName, Effect: "read", IfMissing: "stop_and_report", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "list_agent_knowledge_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "knowledge_base", SkillID: skillID, ToolName: toolName, Effect: "read", IfMissing: "stop_and_report", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "list_agent_database_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "database", SkillID: skillID, ToolName: toolName, Effect: "read", IfMissing: "stop_and_report", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "list_agent_database_tables":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "database_table", SkillID: skillID, ToolName: toolName, Effect: "read", IfMissing: "stop_and_report", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "list_agent_workflow_binding_candidates":
			add(AIChatStructuredOperation{Action: "read_candidates", ResourceType: "workflow", SkillID: skillID, ToolName: toolName, Effect: "read", IfMissing: "stop_and_report", Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
		case "update_agent_config":
			operations = append(operations, agentManagementStructuredConfigOperations(query, tool, tools, goals)...)
		}
	}
	return operations
}

func agentManagementStructuredOperationArguments(tool AIChatTurnStrategyTool) map[string]string {
	args := mergeTurnStrategyToolArguments(nil, tool.Arguments)
	if len(args) == 0 {
		return nil
	}
	for _, key := range []string{
		operationPlanExpectedUpdatedFieldsKey,
		operationPlanExpectedBindingActionsKey,
		operationPlanConfigGoalKey,
	} {
		delete(args, key)
	}
	if len(args) == 0 {
		return nil
	}
	return args
}

func agentManagementStructuredConfigOperations(query string, tool AIChatTurnStrategyTool, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) []AIChatStructuredOperation {
	actions := agentManagementStructuredBindingActions(query, tool, tools, goals)
	fields := operationPlanNormalizedAgentConfigFieldsFromAny(tool.Arguments[operationPlanExpectedUpdatedFieldsKey])
	if len(fields) == 0 {
		fields = agentCapabilityGoalsExpectedConfigFields(goals)
	}
	if len(fields) == 0 {
		fields = agentManagementExpectedConfigUpdateFields(query)
	}
	configGoal := strings.TrimSpace(firstNonEmptyString(tool.Arguments[operationPlanConfigGoalKey], agentManagementConfigGoal(query)))
	if len(actions) == 0 {
		return []AIChatStructuredOperation{{
			Action:       "update",
			ResourceType: "agent_config",
			SkillID:      firstNonEmptyString(tool.SkillID, skills.SkillAgentManagement),
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       fields,
			Goal:         configGoal,
			Arguments:    agentManagementStructuredOperationArguments(tool),
			ArgsBinding:  mergeTurnStrategyToolArguments(nil, tool.ArgsBinding),
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
			SkillID:      firstNonEmptyString(tool.SkillID, skills.SkillAgentManagement),
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       []string{field},
			IfMissing:    "stop_and_report",
			Arguments:    agentManagementStructuredOperationArguments(tool),
			ArgsBinding:  mergeTurnStrategyToolArguments(nil, tool.ArgsBinding),
		})
	}
	if len(fields) > 0 {
		out = append(out, AIChatStructuredOperation{
			Action:       "update",
			ResourceType: "agent_config",
			SkillID:      firstNonEmptyString(tool.SkillID, skills.SkillAgentManagement),
			ToolName:     tool.ToolName,
			Effect:       "update",
			Fields:       fields,
			Goal:         configGoal,
			Arguments:    agentManagementStructuredOperationArguments(tool),
			ArgsBinding:  mergeTurnStrategyToolArguments(nil, tool.ArgsBinding),
		})
	}
	if len(out) == 0 {
		out = append(out, AIChatStructuredOperation{Action: "update", ResourceType: "agent_config", SkillID: firstNonEmptyString(tool.SkillID, skills.SkillAgentManagement), ToolName: tool.ToolName, Effect: "update", Goal: configGoal, Arguments: agentManagementStructuredOperationArguments(tool), ArgsBinding: mergeTurnStrategyToolArguments(nil, tool.ArgsBinding)})
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

func agentManagementStructuredPlanValidationWarnings(query string, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) []string {
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
		action := agentManagementStructuredExpectedBindingAction(query, tools, field, goals)
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

func agentManagementStructuredExpectedBindingAction(query string, tools []AIChatTurnStrategyTool, field string, goals []AIChatAgentCapabilityGoal) string {
	for _, tool := range tools {
		if !strings.EqualFold(strings.TrimSpace(tool.ToolName), "update_agent_config") {
			continue
		}
		actions := agentManagementStructuredBindingActions(query, tool, tools, goals)
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

func agentManagementStructuredPlanHasBindingAction(query string, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) bool {
	for _, tool := range tools {
		if !strings.EqualFold(strings.TrimSpace(tool.ToolName), "update_agent_config") {
			continue
		}
		if len(agentManagementStructuredBindingActions(query, tool, tools, goals)) > 0 {
			return true
		}
	}
	return false
}

func agentManagementStructuredBindingActions(query string, tool AIChatTurnStrategyTool, tools []AIChatTurnStrategyTool, goals []AIChatAgentCapabilityGoal) map[string]string {
	actions := operationPlanAgentConfigBindingActionsFromAny(tool.Arguments[operationPlanExpectedBindingActionsKey])
	if len(actions) == 0 {
		actions = agentCapabilityGoalsExpectedBindingActions(goals)
	}
	if len(actions) == 0 {
		actions = agentManagementExpectedConfigBindingActions(query)
	} else {
		copied := map[string]string{}
		for field, action := range actions {
			if canonicalField := operationPlanAgentConfigCanonicalField(field); canonicalField != "" {
				copied[canonicalField] = operationPlanCanonicalAgentConfigBindingAction(action)
			}
		}
		actions = copied
	}
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
