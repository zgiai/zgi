package skillloop

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// FastPathFinalAnswerForToolTrace returns a user-visible final answer when a
// completed tool result is already sufficient evidence for this turn.
func FastPathFinalAnswerForToolTrace(trace skills.SkillTrace) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(trace.Kind), "tool_governance") {
		return "", false
	}
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return "", false
	}
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	switch {
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		switch toolName {
		case "delete_agent":
			return agentDeleteFastPathAnswer(trace.Result)
		case "delete_agents":
			return agentBatchDeleteFastPathAnswer(trace.Result)
		case "update_agent_identity":
			return agentIdentityUpdateFastPathAnswer(trace.Result)
		case "update_agent_config",
			"replace_agent_skill_bindings",
			"replace_agent_knowledge_bindings",
			"replace_agent_database_bindings",
			"replace_agent_workflow_bindings",
			"replace_agent_memory_slots":
			return agentConfigUpdateFastPathAnswer(trace.Result)
		}
	case strings.EqualFold(skillID, skills.SkillFileManager):
		switch toolName {
		case "save_file_to_management":
			return fileManagementSaveFastPathAnswer(trace.Result)
		case "delete_file":
			return fileManagementDeleteFastPathAnswer(trace.Result)
		}
	case strings.EqualFold(skillID, skills.SkillFileGenerator):
		switch toolName {
		case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
			return generatedArtifactFastPathAnswer(trace.Result, skillID, toolName)
		}
	case strings.EqualFold(skillID, skills.SkillChartGenerator):
		if toolName == "generate_chart" {
			return generatedArtifactFastPathAnswer(trace.Result, skillID, toolName)
		}
	default:
		return "", false
	}
	return "", false
}

// FastPathFinalAnswerForToolTraceWithEvidence keeps the fast path from ending
// a longer user turn when the operation plan still names a different pending
// action.
func FastPathFinalAnswerForToolTraceWithEvidence(trace skills.SkillTrace, evidence map[string]interface{}) (string, bool) {
	if answer, ok := agentCreateFastPathAnswerWithEvidence(trace, evidence); ok {
		return answer, true
	}
	if fastPathAgentConfigUpdateNeedsPostRead(trace, evidence) {
		return "", false
	}
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return "", false
	}
	if fastPathTraceIsSuccessfulAgentRead(trace) && fastPathGoalRequestsAgentConfigMutation(evidence) {
		return "", false
	}
	if answer, ok := agentCapabilityStatusFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := agentReadOnlyConfigFastPathAnswerWithEvidence(trace, evidence); ok {
		return answer, true
	}
	answer, ok := FastPathFinalAnswerForToolTrace(trace)
	if !ok {
		return "", false
	}
	if fastPathBlockedByPendingPlanAction(trace, evidence) {
		return "", false
	}
	return answer, true
}

func fastPathAgentConfigUpdateNeedsPostRead(trace skills.SkillTrace, evidence map[string]interface{}) bool {
	if !fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
		return false
	}
	if !fastPathEvidenceRequestsAgentPostUpdateRead(evidence) {
		return false
	}
	return !fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence)
}

func agentReadOnlyConfigFastPathAnswerWithEvidence(trace skills.SkillTrace, evidence map[string]interface{}) (string, bool) {
	if !fastPathTraceIsSuccessfulAgentRead(trace) {
		return "", false
	}
	if !fastPathGoalRequestsReadOnlyAgentConfig(evidence) {
		return "", false
	}
	if fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence) {
		return "", false
	}
	if fastPathReadOnlyAgentConfigBlockedByMutation(evidence) {
		return "", false
	}

	configResult, hasConfig := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent_config")
	if strings.EqualFold(strings.TrimSpace(trace.ToolName), "get_agent_config") {
		configResult = trace.Result
		hasConfig = len(configResult) > 0
	}
	if !hasConfig {
		return "", false
	}

	agentResult, hasAgent := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent")
	if strings.EqualFold(strings.TrimSpace(trace.ToolName), "get_agent") {
		agentResult = trace.Result
		hasAgent = len(agentResult) > 0
	}
	if !hasAgent && fastPathGoalRequestsAgentIdentityFields(evidence) {
		return "", false
	}
	if !hasAgent && fastPathPlanHasPendingAgentReadStep(evidence, "get_agent") {
		return "", false
	}
	return agentReadOnlyConfigFastPathAnswer(configResult, agentResult)
}

func agentReadOnlyConfigFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if !fastPathGoalRequestsReadOnlyAgentConfig(evidence) {
		return "", false
	}
	if fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence) {
		return "", false
	}
	if fastPathReadOnlyAgentConfigBlockedByMutation(evidence) {
		return "", false
	}
	configResult, hasConfig := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent_config")
	if !hasConfig {
		return "", false
	}
	agentResult, hasAgent := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent")
	if !hasAgent && fastPathGoalRequestsAgentIdentityFields(evidence) {
		return "", false
	}
	if !hasAgent && fastPathPlanHasPendingAgentReadStep(evidence, "get_agent") {
		return "", false
	}
	return agentReadOnlyConfigFastPathAnswer(configResult, agentResult)
}

func agentReadOnlyConfigSummaryFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if fastPathReadOnlyAgentConfigBlockedByMutation(evidence) {
		return "", false
	}
	configResult, hasConfig := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent_config")
	if !hasConfig {
		return "", false
	}
	agentResult, hasAgent := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent")
	if !hasAgent && fastPathGoalRequestsAgentIdentityFields(evidence) && !fastPathAgentConfigResultHasIdentityEvidence(configResult) {
		return "", false
	}
	if !hasAgent && fastPathPlanHasPendingAgentReadStep(evidence, "get_agent") {
		return "", false
	}
	return agentReadOnlyConfigFastPathAnswer(configResult, agentResult)
}

func fastPathAgentConfigResultHasIdentityEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	config := payloadMap(result, "config")
	agent := payloadMap(result, "agent")
	return strings.TrimSpace(firstNonEmptyString(
		result["agent_name"],
		result["name"],
		result["description"],
		result["agent_description"],
		config["agent_name"],
		config["name"],
		config["description"],
		agent["agent_name"],
		agent["name"],
		agent["description"],
	)) != ""
}

func fastPathTraceIsSuccessfulAgentRead(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(trace.ToolName)) {
	case "get_agent_config", "get_agent":
		return true
	default:
		return false
	}
}

func fastPathAgentReadOnlyConfigToolName(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "get_agent_config", "get_agent":
		return true
	default:
		return false
	}
}

func fastPathAgentReadOnlyConfigRedundantLookupToolName(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "list_agents",
		"list_available_models",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return true
	default:
		return false
	}
}

func fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	goal = fastPathRemoveNegatedCandidateLookupPhrases(goal)
	return containsAnyFastPathSubstring(goal, []string{
		"available model", "available models", "available skill", "available skills",
		"available knowledge", "available database", "available table", "available workflow",
		"candidate", "candidates", "selectable", "supported models", "model list",
		"resource list", "binding candidates", "bindable", "bindable resources",
		"\u53ef\u7528\u6a21\u578b", "\u53ef\u7528\u6280\u80fd", "\u53ef\u7528\u77e5\u8bc6\u5e93",
		"\u53ef\u7528\u6570\u636e\u5e93", "\u53ef\u7528\u8868", "\u53ef\u7528\u5de5\u4f5c\u6d41",
		"\u5019\u9009", "\u53ef\u9009", "\u652f\u6301\u7684\u6a21\u578b", "\u6a21\u578b\u5217\u8868",
		"\u8d44\u6e90\u5217\u8868", "\u53ef\u7ed1\u5b9a",
	})
}

func fastPathRemoveNegatedCandidateLookupPhrases(text string) string {
	replacer := strings.NewReplacer(
		"do not query available models", " ",
		"do not list available models", " ",
		"do not query candidate resources", " ",
		"do not list candidate resources", " ",
		"do not query candidates", " ",
		"do not list candidates", " ",
		"don't query available models", " ",
		"don't list available models", " ",
		"don't query candidate resources", " ",
		"don't list candidate resources", " ",
		"don't query candidates", " ",
		"don't list candidates", " ",
		"no candidate lookup", " ",
		"no candidate list", " ",
		"\u4e0d\u8981\u67e5\u8be2\u53ef\u7528\u6a21\u578b\u6216\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u8981\u67e5\u8be2\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u8981\u67e5\u8be2\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u8981\u67e5\u8be2\u5019\u9009", " ",
		"\u4e0d\u8981\u67e5\u770b\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u8981\u67e5\u770b\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u8981\u67e5\u770b\u5019\u9009", " ",
		"\u4e0d\u8981\u5217\u51fa\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u8981\u5217\u51fa\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u8981\u5217\u51fa\u5019\u9009", " ",
		"\u4e0d\u8981\u5217\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u8981\u5217\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u8981\u5217\u5019\u9009", " ",
		"\u4e0d\u67e5\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u67e5\u5019\u9009\u8d44\u6e90", " ",
		"\u4e0d\u67e5\u5019\u9009", " ",
		"\u65e0\u9700\u67e5\u8be2\u53ef\u7528\u6a21\u578b", " ",
		"\u65e0\u9700\u67e5\u8be2\u5019\u9009\u8d44\u6e90", " ",
		"\u65e0\u9700\u5217\u51fa\u5019\u9009", " ",
		"\u4e0d\u9700\u8981\u67e5\u8be2\u53ef\u7528\u6a21\u578b", " ",
		"\u4e0d\u9700\u8981\u67e5\u8be2\u5019\u9009\u8d44\u6e90", " ",
	)
	return replacer.Replace(text)
}

func agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup(skillID string, toolName string, evidence map[string]interface{}) (string, bool) {
	if !fastPathAgentReadOnlyConfigRedundantLookupToolName(skillID, toolName) {
		return "", false
	}
	if fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence) {
		return "", false
	}
	return agentReadOnlyConfigFastPathAnswerFromEvidence(evidence)
}

func fastPathGoalRequestsReadOnlyAgentConfig(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	requested, _ := fastPathReadOnlyAgentConfigIntent(goal)
	return requested
}

func fastPathGoalRequestsAgentConfigSummaryWithCandidates(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	return containsAnyFastPathSubstring(goal, []string{
		"basic info",
		"basic information",
		"runtime config",
		"runtime configuration",
		"current config",
		"current configuration",
		"editable item",
		"editable items",
		"editable field",
		"editable fields",
		"what can be edited",
		"\u57fa\u7840\u4fe1\u606f",
		"\u8fd0\u884c\u914d\u7f6e",
		"\u5f53\u524d\u914d\u7f6e",
		"\u914d\u7f6e\u9879",
		"\u53ef\u7f16\u8f91\u9879",
		"\u53ef\u7f16\u8f91\u9879\u76ee",
		"\u53ef\u4fee\u6539\u9879",
		"\u80fd\u4fee\u6539\u4ec0\u4e48",
		"\u80fd\u591f\u4fee\u6539\u4ec0\u4e48",
	})
}

func fastPathGoalRequestsAgentEditableSummary(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	return containsAnyFastPathSubstring(goal, []string{
		"editable item",
		"editable items",
		"editable field",
		"editable fields",
		"what can be edited",
		"\u53ef\u7f16\u8f91\u9879",
		"\u53ef\u7f16\u8f91\u9879\u76ee",
		"\u53ef\u4fee\u6539\u9879",
		"\u80fd\u4fee\u6539\u4ec0\u4e48",
		"\u80fd\u591f\u4fee\u6539\u4ec0\u4e48",
	})
}

func fastPathReadOnlyAgentConfigBlockedByMutation(evidence map[string]interface{}) bool {
	if fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence) {
		return true
	}
	requestedReadOnly, explicitReadOnly := fastPathReadOnlyAgentConfigIntent(fastPathReadOnlyAgentConfigGoalText(evidence))
	if requestedReadOnly && explicitReadOnly {
		return false
	}
	if fastPathGoalRequestsAgentConfigMutation(evidence) {
		return true
	}
	if !fastPathPlanHasPendingAgentMutationStep(evidence) {
		return false
	}
	return !(requestedReadOnly && explicitReadOnly)
}

func fastPathReadOnlyAgentConfigGoalText(evidence map[string]interface{}) string {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		evidence["latest_user_request"],
		evidence["user_request"],
		evidence["query"],
		evidence["original_user_goal"],
		evidence["user_goal"],
		plan["original_user_goal"],
		plan["user_goal"],
	)))
}

func fastPathGoalRequestsAgentConfigMutation(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	return fastPathTextRequestsAgentConfigMutation(goal)
}

func fastPathTextRequestsAgentConfigMutation(goal string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false
	}
	cleaned := fastPathRemoveReadOnlyStatePhrases(fastPathRemoveNegatedMutationPhrases(goal))
	return containsAnyFastPathSubstring(cleaned, []string{
		"edit", "update", "modify", "change", "set", "replace", "switch", "enable", "disable", "bind", "unbind", "create", "delete", "remove", "add", "publish",
		"\u7f16\u8f91", "\u66f4\u65b0", "\u4fee\u6539", "\u6539\u4e3a", "\u8bbe\u7f6e", "\u66ff\u6362", "\u5207\u6362", "\u542f\u7528", "\u5173\u95ed", "\u7981\u7528", "\u7ed1\u5b9a", "\u89e3\u7ed1", "\u521b\u5efa", "\u5220\u9664", "\u79fb\u9664", "\u6dfb\u52a0", "\u53d1\u5e03",
	})
}

func fastPathRemoveReadOnlyStatePhrases(text string) string {
	replacer := strings.NewReplacer(
		"currently bound", " ",
		"already bound", " ",
		"current bindings", " ",
		"current binding", " ",
		"bound resources", " ",
		"bound resource", " ",
		"binding count", " ",
		"bindings count", " ",
		"bound resource count", " ",
		"resource binding count", " ",
		"bindable", " ",
		"editable items", " ",
		"editable item", " ",
		"editable fields", " ",
		"editable field", " ",
		"editable", " ",
		"available to bind", " ",
		"available binding", " ",
		"available bindings", " ",
		"\u5f53\u524d\u5df2\u7ed1\u5b9a", " ",
		"\u5df2\u7ed1\u5b9a", " ",
		"\u5df2\u7ed1", " ",
		"\u7ed1\u5b9a\u7684", " ",
		"\u7ed1\u5b9a\u8d44\u6e90", " ",
		"\u8d44\u6e90\u7ed1\u5b9a", " ",
		"\u5f53\u524d\u7ed1\u5b9a\u6570\u91cf", " ",
		"\u7ed1\u5b9a\u6570\u91cf", " ",
		"\u7ed1\u5b9a\u8d44\u6e90\u6570\u91cf", " ",
		"\u53ef\u7ed1\u5b9a", " ",
		"\u53ef\u5173\u8054", " ",
		"\u53ef\u7528\u4e8e\u7ed1\u5b9a", " ",
		"\u53ef\u7f16\u8f91\u9879\u76ee", " ",
		"\u53ef\u7f16\u8f91\u9879", " ",
		"\u53ef\u7f16\u8f91", " ",
		"\u53ef\u4fee\u6539\u9879", " ",
		"\u53ef\u4fee\u6539", " ",
	)
	return replacer.Replace(text)
}

func fastPathRemoveNegatedMutationPhrases(text string) string {
	replacer := strings.NewReplacer(
		"do not modify, bind, unbind, create, or delete assets", " ",
		"do not modify, bind, unbind, create or delete assets", " ",
		"do not modify, bind, unbind, create, or delete", " ",
		"do not modify, bind, unbind, create or delete", " ",
		"do not edit", " ",
		"do not update", " ",
		"do not modify", " ",
		"do not change", " ",
		"do not set", " ",
		"do not replace", " ",
		"do not switch", " ",
		"do not enable", " ",
		"do not disable", " ",
		"do not bind", " ",
		"do not unbind", " ",
		"do not create", " ",
		"do not delete", " ",
		"do not remove", " ",
		"do not add", " ",
		"don't edit", " ",
		"don't update", " ",
		"don't modify", " ",
		"don't change", " ",
		"don't set", " ",
		"don't replace", " ",
		"don't switch", " ",
		"don't enable", " ",
		"don't disable", " ",
		"don't bind", " ",
		"don't unbind", " ",
		"don't create", " ",
		"don't delete", " ",
		"don't remove", " ",
		"don't add", " ",
		"\u4e0d\u8981\u7f16\u8f91", " ",
		"\u4e0d\u8981\u66f4\u65b0", " ",
		"\u4e0d\u8981\u4fee\u6539\u3001\u7ed1\u5b9a\u3001\u89e3\u7ed1\u3001\u521b\u5efa\u6216\u5220\u9664\u4efb\u4f55\u8d44\u4ea7", " ",
		"\u4e0d\u8981\u4fee\u6539\u3001\u7ed1\u5b9a\u3001\u89e3\u7ed1\u3001\u521b\u5efa\u6216\u5220\u9664", " ",
		"\u4e0d\u8981\u4fee\u6539\u3001\u7ed1\u5b9a\u3001\u89e3\u7ed1\u3001\u521b\u5efa\u3001\u5220\u9664", " ",
		"\u4e0d\u8981\u4fee\u6539", " ",
		"\u4e0d\u8981\u6539", " ",
		"\u4e0d\u8981\u8bbe\u7f6e", " ",
		"\u4e0d\u8981\u66ff\u6362", " ",
		"\u4e0d\u8981\u5207\u6362", " ",
		"\u4e0d\u8981\u542f\u7528", " ",
		"\u4e0d\u8981\u5173\u95ed", " ",
		"\u4e0d\u8981\u7981\u7528", " ",
		"\u4e0d\u8981\u7ed1\u5b9a", " ",
		"\u4e0d\u8981\u89e3\u7ed1", " ",
		"\u4e0d\u8981\u521b\u5efa", " ",
		"\u4e0d\u8981\u5220\u9664", " ",
		"\u4e0d\u8981\u79fb\u9664", " ",
		"\u4e0d\u8981\u6dfb\u52a0", " ",
		"\u4e0d\u7f16\u8f91", " ",
		"\u4e0d\u66f4\u65b0", " ",
		"\u4e0d\u4fee\u6539", " ",
		"\u4e0d\u6539", " ",
		"\u4e0d\u8bbe\u7f6e", " ",
		"\u4e0d\u66ff\u6362", " ",
		"\u4e0d\u5207\u6362", " ",
		"\u4e0d\u542f\u7528", " ",
		"\u4e0d\u5173\u95ed", " ",
		"\u4e0d\u7981\u7528", " ",
		"\u4e0d\u7ed1\u5b9a", " ",
		"\u4e0d\u89e3\u7ed1", " ",
		"\u4e0d\u521b\u5efa", " ",
		"\u4e0d\u5220\u9664", " ",
		"\u4e0d\u79fb\u9664", " ",
		"\u4e0d\u6dfb\u52a0", " ",
	)
	return replacer.Replace(text)
}

func fastPathReadOnlyAgentConfigIntent(goal string) (bool, bool) {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false, false
	}
	hasReadOnlyMarker := containsAnyFastPathSubstring(goal, []string{
		"read-only", "readonly", "only check", "just check", "only confirm", "do not modify", "do not edit", "don't modify", "don't edit",
		"\u53ea\u8bfb", "\u53ea\u68c0\u67e5", "\u53ea\u67e5\u770b", "\u53ea\u786e\u8ba4", "\u4e0d\u8981\u4fee\u6539", "\u4e0d\u8981\u7f16\u8f91", "\u4e0d\u4fee\u6539", "\u4e0d\u6539",
	})
	if fastPathTextRequestsAgentConfigMutation(goal) {
		return false, hasReadOnlyMarker
	}
	hasAgent := containsAnyFastPathSubstring(goal, []string{"agent", "\u667a\u80fd\u4f53"})
	hasConfig := containsAnyFastPathSubstring(goal, []string{
		"config", "configuration", "model", "provider", "prompt", "binding", "resource", "count", "name", "description",
		"\u914d\u7f6e", "\u6a21\u578b", "\u4f9b\u5e94\u5546", "\u63d0\u793a\u8bcd", "\u7ed1\u5b9a", "\u8d44\u6e90", "\u6570\u91cf", "\u540d\u79f0", "\u63cf\u8ff0",
	})
	hasRead := containsAnyFastPathSubstring(goal, []string{
		"read", "check", "inspect", "show", "view", "confirm",
		"\u8bfb", "\u68c0\u67e5", "\u67e5\u770b", "\u770b", "\u786e\u8ba4",
	})
	return hasAgent && hasConfig && hasRead, hasReadOnlyMarker
}

func fastPathGoalRequestsAgentIdentityFields(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	return containsAnyFastPathSubstring(goal, []string{
		"name", "description", "identity", "basic info", "basic information",
		"\u540d\u79f0", "\u63cf\u8ff0", "\u8eab\u4efd", "\u57fa\u7840\u4fe1\u606f", "\u8be6\u60c5",
	})
}

func fastPathPlanHasPendingAgentMutationStep(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	steps, _ := fastPathPendingExecutablePlanSteps(plan, 20)
	for _, step := range steps {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		if fastPathAgentManagementToolIsMutation(stringFromAny(step["tool_name"])) {
			return true
		}
	}
	pending := strings.ToLower(strings.TrimSpace(firstNonEmptyString(plan["pending_next_action"])))
	return fastPathPendingActionMentionsAgentMutation(pending)
}

func fastPathPlanHasPendingAgentReadStep(evidence map[string]interface{}, toolName string) bool {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return false
	}
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	steps, _ := fastPathPendingExecutablePlanSteps(plan, 20)
	for _, step := range steps {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), toolName) {
			return true
		}
	}
	return false
}

func fastPathAgentManagementToolIsMutation(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "create_agent", "delete_agent", "delete_agents", "update_agent_identity", "update_agent_config",
		"replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings",
		"replace_agent_workflow_bindings", "replace_agent_memory_slots":
		return true
	default:
		return false
	}
}

func fastPathPendingActionMentionsAgentMutation(pending string) bool {
	if pending == "" {
		return false
	}
	for _, toolName := range []string{
		"create_agent", "delete_agent", "delete_agents", "update_agent_identity", "update_agent_config",
		"replace_agent_skill_bindings", "replace_agent_knowledge_bindings", "replace_agent_database_bindings",
		"replace_agent_workflow_bindings", "replace_agent_memory_slots",
	} {
		if strings.Contains(pending, toolName) {
			return true
		}
	}
	return false
}

func fastPathLatestSuccessfulAgentReadResult(evidence map[string]interface{}, toolName string) (map[string]interface{}, bool) {
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return nil, false
	}
	invocations := fastPathInvocationSequence(evidence)
	for i := len(invocations) - 1; i >= 0; i-- {
		trace, ok := fastPathTraceFromInvocation(invocations[i])
		if !ok || !fastPathTraceIsSuccessfulAgentRead(trace) ||
			!strings.EqualFold(strings.TrimSpace(trace.ToolName), toolName) {
			continue
		}
		return trace.Result, true
	}
	results := fastPathLatestToolResultCandidates(evidence)
	for i := len(results) - 1; i >= 0; i-- {
		trace, ok := fastPathTraceFromToolResult(results[i])
		if !ok || !fastPathTraceIsSuccessfulAgentRead(trace) ||
			!strings.EqualFold(strings.TrimSpace(trace.ToolName), toolName) {
			continue
		}
		return trace.Result, true
	}
	return nil, false
}

func agentReadOnlyConfigFastPathAnswer(configResult map[string]interface{}, agentResult map[string]interface{}) (string, bool) {
	config := payloadMap(configResult, "config")
	if len(config) == 0 {
		config = configResult
	}
	if len(config) == 0 {
		return "", false
	}
	agent := payloadMap(agentResult, "agent")
	if len(agent) == 0 {
		agent = payloadMap(configResult, "agent")
	}
	details := make([]string, 0, 8)
	if name := strings.TrimSpace(firstNonEmptyString(agentResult["agent_name"], agent["name"], agent["agent_name"], configResult["agent_name"], config["agent_name"])); name != "" {
		details = append(details, "\u667a\u80fd\u4f53\uff1a"+name)
	}
	if description := fastPathTrimText(strings.TrimSpace(firstNonEmptyString(agentResult["agent_description"], agentResult["description"], agent["description"], configResult["description"], config["description"])), 80); description != "" {
		details = append(details, "\u63cf\u8ff0\uff1a"+description)
	}
	if model := agentReadOnlyConfigModelDetail(config); model != "" {
		details = append(details, model)
	}
	if prompt := fastPathTrimText(strings.TrimSpace(stringFromAny(config["system_prompt"])), 100); prompt != "" {
		details = append(details, "\u7cfb\u7edf\u63d0\u793a\u8bcd\uff1a"+prompt)
	}
	if title := fastPathTrimText(strings.TrimSpace(stringFromAny(config["home_title"])), 60); title != "" {
		details = append(details, "\u9996\u9875\u6807\u9898\uff1a"+title)
	}
	if placeholder := fastPathTrimText(strings.TrimSpace(stringFromAny(config["input_placeholder"])), 60); placeholder != "" {
		details = append(details, "\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\uff1a"+placeholder)
	}
	if theme := strings.TrimSpace(stringFromAny(config["theme_color"])); theme != "" {
		details = append(details, "\u4e3b\u9898\u8272\uff1a"+theme)
	}
	if questionCount := len(sanitizedStringListArgumentValue(config["suggested_questions"])); questionCount > 0 {
		details = append(details, fmt.Sprintf("\u5f00\u573a\u95ee\u9898\uff1a%d \u4e2a", questionCount))
	}
	details = append(details, fmt.Sprintf(
		"\u7ed1\u5b9a\u8d44\u6e90\uff1a\u6280\u80fd %d \u4e2a\uff0c\u77e5\u8bc6\u5e93 %d \u4e2a\uff0c\u6570\u636e\u5e93\u8868 %d \u4e2a\uff0c\u5de5\u4f5c\u6d41 %d \u4e2a",
		agentReadOnlyCollectionCount(configResult, config, "enabled_skill_ids", "enabled_skill_count"),
		agentReadOnlyCollectionCount(configResult, config, "knowledge_dataset_ids", "knowledge_dataset_count"),
		agentReadOnlyDatabaseTableCount(configResult, config),
		agentReadOnlyCollectionCount(configResult, config, "workflow_bindings", "workflow_binding_count"),
	))
	details = append(details,
		"\u8bb0\u5fc6\uff1a"+fastPathEnabledLabel(boolFromAny(firstNonEmptyValue(config["agent_memory_enabled"], config["use_memory"]))),
		"\u6587\u4ef6\u4e0a\u4f20\uff1a"+fastPathEnabledLabel(boolFromAny(config["file_upload_enabled"])),
	)
	if len(details) == 0 {
		return "", false
	}
	return "\u5f53\u524d\u667a\u80fd\u4f53\u914d\u7f6e\uff1a" + strings.Join(dedupeStrings(details), "\uff1b") + "\u3002", true
}

func agentReadOnlyConfigModelDetail(config map[string]interface{}) string {
	provider := strings.TrimSpace(firstNonEmptyString(config["model_provider"], config["provider"]))
	model := strings.TrimSpace(firstNonEmptyString(config["model"], config["model_name"]))
	switch {
	case provider != "" && model != "":
		return "\u6a21\u578b\uff1a" + provider + "/" + model
	case model != "":
		return "\u6a21\u578b\uff1a" + model
	case provider != "":
		return "\u6a21\u578b\u4f9b\u5e94\u5546\uff1a" + provider
	default:
		return ""
	}
}

func agentReadOnlyCollectionCount(result map[string]interface{}, config map[string]interface{}, collectionKey string, countKeys ...string) int {
	for _, key := range countKeys {
		if count, ok := intFromAny(firstNonEmptyValue(result[key], config[key])); ok && count >= 0 {
			return count
		}
	}
	if count := len(sanitizedStringListArgumentValue(config[collectionKey])); count > 0 {
		return count
	}
	return len(mapSliceFromAny(config[collectionKey]))
}

func agentReadOnlyDatabaseTableCount(result map[string]interface{}, config map[string]interface{}) int {
	for _, key := range []string{"database_table_count", "database_binding_table_count"} {
		if count, ok := intFromAny(firstNonEmptyValue(result[key], config[key])); ok && count >= 0 {
			return count
		}
	}
	count := 0
	for _, binding := range mapSliceFromAny(config["database_bindings"]) {
		tableIDs := sanitizedStringListArgumentValue(firstNonEmptyValue(binding["table_ids"], binding["tableIds"]))
		if len(tableIDs) > 0 {
			count += len(tableIDs)
			continue
		}
		count++
	}
	if count == 0 {
		if bindingCount, ok := intFromAny(firstNonEmptyValue(result["database_binding_count"], config["database_binding_count"])); ok && bindingCount >= 0 {
			return bindingCount
		}
	}
	return count
}

func fastPathEnabledLabel(enabled bool) string {
	if enabled {
		return "\u5f00\u542f"
	}
	return "\u5173\u95ed"
}

func fastPathTrimText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func fastPathGoalRequestsAgentPostUpdateRead(evidence map[string]interface{}) bool {
	return fastPathGoalRequestsAgentConfigPostRead(evidence) ||
		fastPathGoalRequestsAgentIdentityPostRead(evidence)
}

func fastPathGoalRequestsAgentConfigPostRead(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	goal := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		plan["original_user_goal"],
		plan["user_goal"],
		evidence["original_user_goal"],
		evidence["user_goal"],
		evidence["query"],
	)))
	if goal == "" {
		return false
	}
	if !strings.Contains(goal, "config") &&
		!strings.Contains(goal, "\u914d\u7f6e") &&
		!strings.Contains(goal, "binding") &&
		!strings.Contains(goal, "bind") &&
		!strings.Contains(goal, "unbind") &&
		!strings.Contains(goal, "\u7ed1\u5b9a") &&
		!strings.Contains(goal, "\u89e3\u7ed1") {
		return false
	}
	for _, marker := range []string{
		"read again",
		"read back",
		"re-read",
		"reread",
		"verify by reading",
		"read config again",
		"read the config again",
		"read config after",
		"verify config after",
		"after completion read",
		"after completing read",
		"\u518d\u6b21\u8bfb\u53d6",
		"\u91cd\u65b0\u8bfb\u53d6",
		"\u5b8c\u6210\u540e\u8bfb\u53d6",
		"\u5b8c\u6210\u540e\u518d\u6b21\u8bfb\u53d6",
		"\u5b8c\u6210\u540e\u91cd\u65b0\u8bfb\u53d6",
		"\u5b8c\u6210\u540e\u5fc5\u987b\u518d\u6b21\u8bfb\u53d6",
		"\u66f4\u65b0\u5b8c\u6210\u540e\u8bfb\u53d6",
		"\u66f4\u65b0\u5b8c\u6210\u540e\u518d\u6b21\u8bfb\u53d6",
		"\u66f4\u65b0\u5b8c\u6210\u540e\u5fc5\u987b\u518d\u6b21\u8bfb\u53d6",
		"\u8bfb\u53d6\u914d\u7f6e\u9a8c\u8bc1",
		"\u8bfb\u914d\u7f6e\u9a8c\u8bc1",
		"\u590d\u8bfb\u914d\u7f6e",
		"\u518d\u770b\u914d\u7f6e",
		"\u518d\u68c0\u67e5\u914d\u7f6e",
	} {
		if strings.Contains(goal, marker) {
			return true
		}
	}
	if fastPathGoalHasConfigPostReadShape(goal) {
		return true
	}
	return false
}

func fastPathGoalRequestsAgentIdentityPostRead(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if fastPathPlanHasAgentIdentityPostRead(plan) {
		return true
	}
	goal := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		plan["original_user_goal"],
		plan["user_goal"],
		evidence["original_user_goal"],
		evidence["user_goal"],
		evidence["query"],
	)))
	if goal == "" || !fastPathOriginalGoalRequestsAgentIdentityUpdate(map[string]interface{}{"original_user_goal": goal}) {
		return false
	}
	return containsAnyFastPathSubstring(goal, []string{
		"read again",
		"read back",
		"re-read",
		"reread",
		"verify",
		"check again",
		"confirm",
		"page",
		"top",
		"after update",
		"after updating",
		"\u91cd\u65b0\u8bfb\u53d6",
		"\u518d\u6b21\u8bfb\u53d6",
		"\u590d\u8bfb",
		"\u9a8c\u8bc1",
		"\u786e\u8ba4",
		"\u9875\u9762",
		"\u9876\u90e8",
		"\u751f\u6548",
		"\u5df2\u66f4\u65b0",
	})
}

func fastPathPlanHasAgentIdentityPostRead(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	seenIdentityUpdate := false
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
		switch toolName {
		case "update_agent_identity":
			seenIdentityUpdate = true
		case "get_agent", "get_agent_config":
			if seenIdentityUpdate || completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) {
				return true
			}
		}
	}
	if !seenIdentityUpdate {
		return false
	}
	pending := strings.ToLower(strings.TrimSpace(stringFromAny(plan["pending_next_action"])))
	return strings.Contains(pending, "agent-management/get_agent") ||
		strings.Contains(pending, "get_agent_config") ||
		strings.Contains(pending, "get_agent")
}

func fastPathGoalHasConfigPostReadShape(goal string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" || !strings.Contains(goal, "\u914d\u7f6e") && !strings.Contains(goal, "config") {
		return false
	}
	hasAfterMarker := containsAnyFastPathSubstring(goal, []string{
		"after completion", "after completing", "after update", "after updating", "when done",
		"\u5b8c\u6210\u540e", "\u66f4\u65b0\u5b8c\u6210\u540e", "\u4fee\u6539\u5b8c\u6210\u540e", "\u64cd\u4f5c\u5b8c\u6210\u540e", "\u4e4b\u540e",
	})
	if !hasAfterMarker {
		return false
	}
	hasReadAgain := containsAnyFastPathSubstring(goal, []string{
		"read again", "read back", "re-read", "reread", "verify", "check again", "inspect again",
		"\u518d\u6b21\u8bfb\u53d6", "\u91cd\u65b0\u8bfb\u53d6", "\u590d\u8bfb", "\u518d\u8bfb", "\u9a8c\u8bc1", "\u518d\u68c0\u67e5", "\u518d\u770b",
	})
	if !hasReadAgain {
		return false
	}
	return containsAnyFastPathSubstring(goal, []string{
		"read", "check", "inspect", "verify",
		"\u8bfb\u53d6", "\u68c0\u67e5", "\u67e5\u770b", "\u770b", "\u9a8c\u8bc1",
	})
}

func fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence map[string]interface{}) bool {
	seenUpdate := false
	awaitingReadAfterLatestUpdate := false
	latestUpdateAllowsGetAgentRead := false
	for _, invocation := range fastPathInvocationSequence(evidence) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
		if fastPathInvocationIsSuccessfulAgentConfigUpdate(invocation) {
			seenUpdate = true
			awaitingReadAfterLatestUpdate = true
			latestUpdateAllowsGetAgentRead = strings.EqualFold(toolName, "update_agent_identity")
			continue
		}
		if !awaitingReadAfterLatestUpdate || !fastPathInvocationSucceeded(invocation) {
			continue
		}
		if strings.EqualFold(toolName, "get_agent_config") {
			awaitingReadAfterLatestUpdate = false
			continue
		}
		if latestUpdateAllowsGetAgentRead && strings.EqualFold(toolName, "get_agent") {
			awaitingReadAfterLatestUpdate = false
			continue
		}
	}
	return seenUpdate && !awaitingReadAfterLatestUpdate
}

func fastPathInvocationIsSuccessfulAgentConfigUpdate(invocation map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])))
	switch status {
	case "success", "succeeded", "completed":
	default:
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(invocation["tool_name"]))) {
	case "update_agent_config", "update_agent_identity":
		return true
	default:
		return false
	}
}

func fastPathInvocationSequence(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	invocations := make([]map[string]interface{}, 0, 16)
	seen := map[string]struct{}{}
	appendInvocation := func(invocation map[string]interface{}, allowSameSourceRepeat bool) {
		if len(invocation) == 0 {
			return
		}
		signature := fastPathInvocationSignature(invocation)
		if signature != "" {
			if _, ok := seen[signature]; ok && !allowSameSourceRepeat {
				return
			}
			seen[signature] = struct{}{}
		}
		invocations = append(invocations, invocation)
	}
	appendInvocations := func(source map[string]interface{}) {
		sourceSeen := map[string]struct{}{}
		for _, invocation := range mapSliceFromAny(source["skill_invocations"]) {
			signature := fastPathInvocationSignature(invocation)
			allowSameSourceRepeat := false
			if signature != "" {
				if _, alreadySeen := seen[signature]; alreadySeen {
					if _, seenInSource := sourceSeen[signature]; !seenInSource {
						continue
					}
					allowSameSourceRepeat = true
				}
				sourceSeen[signature] = struct{}{}
			}
			appendInvocation(invocation, allowSameSourceRepeat)
		}
		for _, invocation := range mapSliceFromAny(source["invocations"]) {
			signature := fastPathInvocationSignature(invocation)
			allowSameSourceRepeat := false
			if signature != "" {
				if _, alreadySeen := seen[signature]; alreadySeen {
					if _, seenInSource := sourceSeen[signature]; !seenInSource {
						continue
					}
					allowSameSourceRepeat = true
				}
				sourceSeen[signature] = struct{}{}
			}
			appendInvocation(invocation, allowSameSourceRepeat)
		}
	}
	appendInvocations(evidence)
	executionSummary := evidenceMapFromAny(evidence["execution_summary"])
	appendInvocations(executionSummary)
	executionLedger := evidenceMapFromAny(evidence["execution_ledger"])
	appendInvocations(executionLedger)
	appendInvocations(evidenceMapFromAny(executionLedger["summary"]))
	return invocations
}

func fastPathInvocationSignature(invocation map[string]interface{}) string {
	if len(invocation) == 0 {
		return ""
	}
	runtimeID := strings.TrimSpace(firstNonEmptyString(invocation["runtime_id"], invocation["runtimeID"], invocation["id"]))
	if runtimeID != "" {
		return "runtime:" + runtimeID
	}
	payload := map[string]interface{}{}
	for _, key := range []string{"kind", "status", "skill_id", "tool_name", "arguments", "result", "tool_result"} {
		if value, ok := invocation[key]; ok {
			payload[key] = value
		}
	}
	if len(payload) == 0 {
		return ""
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprint(payload)
	}
	return string(encoded)
}

func fastPathInvocationSucceeded(invocation map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"])))
	switch status {
	case "success", "succeeded", "completed", "approved":
		return true
	default:
		return false
	}
}

// FastPathFinalAnswerForCompletionEvidence returns a final answer when the
// accumulated execution evidence is already enough to close the turn. This is
// used after client-side observations, where there may be no new tool trace for
// Runner to fast-path on.
func FastPathFinalAnswerForCompletionEvidence(evidence map[string]interface{}) (string, bool) {
	if fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		return "", false
	}
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return "", false
	}
	if answer, ok := agentMultiMutationFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := agentConfigPostUpdateVerifiedFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := agentCapabilityStatusFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := agentCandidateLookupFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := agentCreateFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := latestToolResultFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := latestClientActionFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	if answer, ok := generatedArtifactFastPathAnswerFromEvidence(evidence); ok {
		return answer, true
	}
	return "", false
}

// FastPathFinalAnswerForAgentMutationEvidence summarizes the completed Agent
// mutation evidence for a continuation when the just-executed governed tool is
// one part of a multi-step Agent edit.
func FastPathFinalAnswerForAgentMutationEvidence(evidence map[string]interface{}, current skills.SkillTrace) (string, bool) {
	if !fastPathTraceIsSuccessfulAgentConfigUpdate(current) {
		return "", false
	}
	return agentMultiMutationFastPathAnswerFromEvidence(evidence)
}

func agentMultiMutationFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if len(evidence) == 0 {
		return "", false
	}
	if fastPathHasPendingExecutablePlanStep(evidenceMapFromAny(evidence["operation_plan"])) {
		return "", false
	}

	traces := fastPathSuccessfulAgentMutationTraces(evidence)
	if len(traces) < 2 {
		return "", false
	}

	details := make([]string, 0, len(traces))
	seenDetails := map[string]struct{}{}
	agentName := ""
	for _, trace := range traces {
		if fastPathBlockedByPendingPlanAction(trace, evidence) {
			return "", false
		}
		if name := agentConfigResultAgentName(trace.Result); name != "" {
			agentName = name
		}
		detail := agentMutationFastPathDetail(trace)
		if detail == "" {
			continue
		}
		if _, ok := seenDetails[detail]; ok {
			continue
		}
		seenDetails[detail] = struct{}{}
		details = append(details, detail)
	}
	if len(details) < 2 {
		return "", false
	}

	target := "\u8be5\u667a\u80fd\u4f53"
	if agentName != "" {
		target = "\u667a\u80fd\u4f53\u300c" + agentName + "\u300d"
	}
	return "\u5df2\u5b8c\u6210" + target + "\u7684\u591a\u9879\u914d\u7f6e\u66f4\u65b0\uff1a\n- " + strings.Join(details, "\n- ") + "\u3002", true
}

func agentCapabilityStatusFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	goals := fastPathAgentCapabilityGoals(plan)
	if len(goals) == 0 || fastPathReadOnlyAgentConfigBlockedByMutation(evidence) {
		return "", false
	}
	configResult, hasConfig := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent_config")
	if !hasConfig {
		return "", false
	}

	lines := []string{}
	for _, goal := range goals {
		capabilityID := strings.ToLower(strings.TrimSpace(stringFromAny(goal["capability_id"])))
		switch capabilityID {
		case "agent.skill_backed_capability":
			line, ok := agentSkillBackedCapabilityStatusLine(evidence, goal, configResult)
			if !ok {
				return "", false
			}
			lines = append(lines, line)
		case "agent.accept_uploaded_files":
			line, ok := agentBooleanConfigCapabilityStatusLine(configResult, "file_upload_enabled", "\u6587\u4ef6\u4e0a\u4f20")
			if !ok {
				return "", false
			}
			lines = append(lines, line)
		case "agent.memory":
			line, ok := agentBooleanConfigCapabilityStatusLine(configResult, "agent_memory_enabled", "\u8bb0\u5fc6")
			if !ok {
				return "", false
			}
			lines = append(lines, line)
		default:
			continue
		}
	}
	if len(lines) == 0 {
		return "", false
	}

	agentName := agentCapabilityStatusAgentName(configResult)
	prefix := "\u5df2\u6839\u636e get_agent_config"
	if agentName != "" {
		prefix += "\u68c0\u67e5\u667a\u80fd\u4f53\u300c" + agentName + "\u300d"
	} else {
		prefix += "\u68c0\u67e5\u5f53\u524d\u667a\u80fd\u4f53"
	}
	return prefix + "\uff1a\n- " + strings.Join(dedupeStrings(lines), "\n- ") + "\n\n\u672c\u8f6e\u53ea\u8bfb\u68c0\u67e5\uff0c\u672a\u4fee\u6539\u914d\u7f6e\uff0c\u672a\u53d1\u8d77\u5ba1\u6279\u3002", true
}

func fastPathAgentCapabilityGoals(plan map[string]interface{}) []map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	goals := mapSliceFromAny(plan["capability_goals"])
	if len(goals) > 0 {
		return goals
	}
	structured := evidenceMapFromAny(plan["structured_plan"])
	return mapSliceFromAny(structured["capability_goals"])
}

func agentSkillBackedCapabilityStatusLine(evidence map[string]interface{}, goal map[string]interface{}, configResult map[string]interface{}) (string, bool) {
	results := fastPathSuccessfulAgentCandidateLookupResults(evidence)
	candidateResult, hasCandidateResult := results["list_agent_skill_candidates"]
	needsCandidate := strings.EqualFold(strings.TrimSpace(stringFromAny(goal["candidate_tool"])), "list_agent_skill_candidates") ||
		strings.TrimSpace(stringFromAny(goal["candidate_query"])) != ""
	if needsCandidate && !hasCandidateResult {
		return "", false
	}

	candidates := fastPathAgentSkillCandidates(candidateResult)
	enabledRefs, hasConcreteRefs := agentCapabilityEnabledSkillRefs(configResult)
	if len(candidates) == 0 {
		count := firstNonNegativeInt(candidateResult["count"], candidateResult["candidates_count"], candidateResult["total"])
		if hasCandidateResult && count == 0 {
			return "\u672a\u627e\u5230\u5339\u914d\u7684 Skill \u5019\u9009\uff0c\u56e0\u6b64\u65e0\u6cd5\u8bc1\u660e\u8be5\u6280\u80fd\u578b\u80fd\u529b\u53ef\u901a\u8fc7\u7ed1\u5b9a Skill \u5f00\u542f", true
		}
		if !hasConcreteRefs && agentCapabilityEnabledSkillCount(configResult) > 0 {
			return "\u5df2\u7ed1\u5b9a Skill\uff0c\u4f46\u5de5\u5177\u7ed3\u679c\u672a\u8fd4\u56de\u5177\u4f53 Skill ID\uff0c\u65e0\u6cd5\u786e\u8ba4\u8be5\u6280\u80fd\u578b\u80fd\u529b\u662f\u5426\u5df2\u5f00\u542f", true
		}
		return "\u5f53\u524d\u672a\u8fd4\u56de\u53ef\u7528\u7684 Skill \u5019\u9009\u8bc1\u636e\uff0c\u65e0\u6cd5\u786e\u8ba4\u8be5\u6280\u80fd\u578b\u80fd\u529b", true
	}

	for _, candidate := range candidates {
		if agentCapabilitySkillRefSetContains(enabledRefs, candidate.ID) ||
			agentCapabilitySkillRefSetContains(enabledRefs, candidate.Name) {
			return "\u5df2\u5177\u5907\u8be5\u6280\u80fd\u578b\u80fd\u529b\uff1a\u5df2\u542f\u7528 Skill\u300c" + candidate.Display() + "\u300d", true
		}
	}
	if hasConcreteRefs || agentCapabilityEnabledSkillCount(configResult) == 0 {
		return "\u5c1a\u672a\u5177\u5907\u8be5\u6280\u80fd\u578b\u80fd\u529b\uff1aget_agent_config \u672a\u663e\u793a\u5df2\u542f\u7528\u5019\u9009 Skill\u300c" + candidates[0].Display() + "\u300d\uff1b\u5982\u9700\u5f00\u542f\uff0c\u9700\u7ed1\u5b9a\u8be5 Skill", true
	}
	return "\u65e0\u6cd5\u786e\u8ba4\u8be5\u6280\u80fd\u578b\u80fd\u529b\u662f\u5426\u5df2\u5f00\u542f\uff1aget_agent_config \u53ea\u8fd4\u56de\u4e86 Skill \u6570\u91cf\uff0c\u672a\u8fd4\u56de\u5177\u4f53 ID\uff1b\u5019\u9009 Skill \u5305\u62ec\u300c" + candidates[0].Display() + "\u300d", true
}

type agentSkillCapabilityCandidate struct {
	ID   string
	Name string
}

func (candidate agentSkillCapabilityCandidate) Display() string {
	switch {
	case candidate.Name != "" && candidate.ID != "" && !strings.EqualFold(candidate.Name, candidate.ID):
		return candidate.Name + "\uff08" + candidate.ID + "\uff09"
	case candidate.Name != "":
		return candidate.Name
	default:
		return candidate.ID
	}
}

func fastPathAgentSkillCandidates(result map[string]interface{}) []agentSkillCapabilityCandidate {
	if len(result) == 0 {
		return nil
	}
	sources := []interface{}{
		result["candidate_samples"],
		result["binding_candidates"],
		result["candidates"],
		result["items"],
		result["skills"],
	}
	candidates := []agentSkillCapabilityCandidate{}
	seen := map[string]struct{}{}
	for _, source := range sources {
		for _, item := range mapSliceFromAny(source) {
			id := strings.TrimSpace(firstNonEmptyString(item["id"], item["skill_id"], item["tool_id"]))
			name := strings.TrimSpace(firstNonEmptyString(item["name"], item["label"], item["title"], item["display_name"], id))
			key := strings.ToLower(strings.TrimSpace(firstNonEmptyString(id, name)))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, agentSkillCapabilityCandidate{ID: id, Name: name})
		}
	}
	return candidates
}

func agentBooleanConfigCapabilityStatusLine(configResult map[string]interface{}, field string, label string) (string, bool) {
	config := payloadMap(configResult, "config")
	value, ok := firstPresentValue(configResult[field], config[field])
	if !ok {
		return "", false
	}
	if boolFromAny(value) {
		return label + "\uff1a\u5df2\u5f00\u542f", true
	}
	return label + "\uff1a\u672a\u5f00\u542f", true
}

func firstPresentValue(values ...interface{}) (interface{}, bool) {
	for _, value := range values {
		if value != nil {
			return value, true
		}
	}
	return nil, false
}

func agentCapabilityStatusAgentName(configResult map[string]interface{}) string {
	config := payloadMap(configResult, "config")
	agent := payloadMap(configResult, "agent")
	return strings.TrimSpace(firstNonEmptyString(
		configResult["agent_name"],
		configResult["name"],
		agent["agent_name"],
		agent["name"],
		config["agent_name"],
		config["name"],
	))
}

func agentCapabilityEnabledSkillRefs(configResult map[string]interface{}) (map[string]struct{}, bool) {
	refs := map[string]struct{}{}
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			refs[value] = struct{}{}
		}
	}
	config := payloadMap(configResult, "config")
	for _, value := range sanitizedStringListArgumentValue(firstNonEmptyValue(
		configResult["enabled_skill_ids"],
		configResult["skill_ids"],
		configResult["enabled_skill_refs"],
		config["enabled_skill_ids"],
		config["skill_ids"],
		config["enabled_skill_refs"],
	)) {
		add(value)
	}
	for _, key := range []string{"enabled_skills", "skills"} {
		for _, source := range []map[string]interface{}{configResult, config} {
			for _, item := range mapSliceFromAny(source[key]) {
				add(firstNonEmptyString(item["id"], item["skill_id"], item["tool_id"]))
				add(firstNonEmptyString(item["name"], item["label"], item["title"], item["display_name"]))
			}
		}
	}
	return refs, len(refs) > 0
}

func agentCapabilityEnabledSkillCount(configResult map[string]interface{}) int {
	config := payloadMap(configResult, "config")
	for _, key := range []string{"enabled_skill_count", "skill_count"} {
		if count := firstNonNegativeInt(configResult[key], config[key]); count >= 0 {
			return count
		}
	}
	refs, _ := agentCapabilityEnabledSkillRefs(configResult)
	return len(refs)
}

func agentCapabilitySkillRefSetContains(refs map[string]struct{}, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	_, ok := refs[value]
	return ok
}

func fastPathHasPendingExecutablePlanStep(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	pending, _ := fastPathPendingExecutablePlanSteps(plan, 1)
	return len(pending) > 0
}

func fastPathSuccessfulAgentMutationTraces(evidence map[string]interface{}) []skills.SkillTrace {
	traces := make([]skills.SkillTrace, 0, 4)
	seen := map[string]struct{}{}
	appendTrace := func(trace skills.SkillTrace) {
		if !fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
			return
		}
		key := fastPathAgentMutationTraceSignature(trace)
		if key != "" {
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		traces = append(traces, trace)
	}
	for _, invocation := range fastPathInvocationSequence(evidence) {
		trace, ok := fastPathTraceFromInvocation(invocation)
		if !ok {
			continue
		}
		appendTrace(trace)
	}
	for _, result := range fastPathLatestToolResultCandidates(evidence) {
		trace, ok := fastPathTraceFromToolResult(result)
		if !ok {
			continue
		}
		appendTrace(trace)
	}
	return traces
}

func fastPathAgentMutationTraceSignature(trace skills.SkillTrace) string {
	payload := map[string]interface{}{
		"skill_id":  trace.SkillID,
		"tool_name": trace.ToolName,
		"status":    trace.Status,
		"result":    trace.Result,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprint(payload)
	}
	return string(encoded)
}

func agentMutationFastPathDetail(trace skills.SkillTrace) string {
	switch strings.ToLower(strings.TrimSpace(trace.ToolName)) {
	case "update_agent_identity":
		fields := agentIdentityUpdatedFieldLabels(sanitizedStringListArgumentValue(trace.Result["updated_fields"]))
		if len(fields) == 0 {
			return ""
		}
		return "\u57fa\u7840\u4fe1\u606f\uff1a" + strings.Join(fields, "\u3001")
	case "update_agent_config":
		details := agentConfigUpdateDetails(trace.Result)
		if len(details) == 0 {
			return ""
		}
		return "\u8fd0\u884c\u914d\u7f6e\uff1a" + strings.Join(details, "\uff1b")
	default:
		return ""
	}
}

func agentCandidateLookupFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if !fastPathGoalExplicitlyRequestsAgentCandidateLookup(evidence) {
		return "", false
	}
	if fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence) {
		return "", false
	}
	if fastPathPlanHasPendingAgentMutationStep(evidence) && !fastPathGoalExplicitlyForbidsAgentMutation(evidence) {
		return "", false
	}
	results := fastPathSuccessfulAgentCandidateLookupResults(evidence)
	if len(results) == 0 {
		return "", false
	}
	configAnswer := ""
	if fastPathGoalRequestsAgentConfigSummaryWithCandidates(evidence) {
		if answer, ok := agentReadOnlyConfigSummaryFastPathAnswerFromEvidence(evidence); ok {
			configAnswer = answer
		} else {
			return "", false
		}
	}
	required := fastPathRequiredAgentCandidateLookupTools(evidence)
	if len(required) > 0 {
		for _, toolName := range required {
			if _, ok := results[toolName]; !ok {
				return "", false
			}
		}
	}

	sections := []string{}
	for _, item := range []struct {
		toolName string
		label    string
	}{
		{toolName: "list_agent_skill_candidates", label: "\u0053\u006b\u0069\u006c\u006c"},
		{toolName: "list_agent_knowledge_candidates", label: "\u77e5\u8bc6\u5e93"},
		{toolName: "list_agent_database_candidates", label: "\u6570\u636e\u5e93"},
		{toolName: "list_agent_database_tables", label: "\u6570\u636e\u5e93\u8868"},
		{toolName: "list_agent_workflow_binding_candidates", label: "\u5de5\u4f5c\u6d41"},
	} {
		result, ok := results[item.toolName]
		if !ok {
			continue
		}
		sections = append(sections, agentCandidateLookupFastPathSection(item.label, result))
	}
	if len(sections) == 0 {
		return "", false
	}
	parts := []string{"\u5df2\u5b8c\u6210\u53ea\u8bfb\u67e5\u8be2\uff0c\u672a\u4fee\u6539\u914d\u7f6e\uff0c\u672a\u53d1\u8d77\u5ba1\u6279\u3002"}
	if missing := fastPathMissingRequestedAgentCandidates(evidence, results); len(missing) > 0 {
		parts = append(parts, strings.Join(missing, "\n"))
	}
	if configAnswer != "" {
		parts = append(parts, configAnswer)
		if fastPathGoalRequestsAgentEditableSummary(evidence) {
			parts = append(parts, "\u53ef\u7f16\u8f91\u9879\u76ee\uff1a\u57fa\u7840\u4fe1\u606f\uff08\u540d\u79f0\u3001\u63cf\u8ff0\u3001\u56fe\u6807\uff09\uff0c\u8fd0\u884c\u914d\u7f6e\uff08\u6a21\u578b/provider\u3001\u7cfb\u7edf\u63d0\u793a\u8bcd\u3001\u5f00\u573a\u95ee\u9898\u3001\u9996\u9875\u6807\u9898\u3001\u4e3b\u9898/\u5c55\u793a\u914d\u7f6e\uff09\uff0c\u4ee5\u53ca Skill\u3001\u77e5\u8bc6\u5e93\u3001\u6570\u636e\u5e93\u8868\u548c\u5de5\u4f5c\u6d41\u7ed1\u5b9a\u3002")
		}
	}
	parts = append(parts, "\u53ef\u7ed1\u5b9a\u8d44\u6e90\u5019\u9009\uff1a\n"+strings.Join(sections, "\n"))
	return strings.Join(parts, "\n\n"), true
}

func fastPathMissingRequestedAgentCandidates(evidence map[string]interface{}, results map[string]map[string]interface{}) []string {
	goal := fastPathRawAgentGoalText(evidence)
	if goal == "" || len(results) == 0 {
		return nil
	}
	missing := []string{}
	for _, item := range []struct {
		toolName string
		label    string
		patterns []string
	}{
		{toolName: "list_agent_skill_candidates", label: "\u0053\u006b\u0069\u006c\u006c", patterns: []string{"Skill", "\u6280\u80fd"}},
		{toolName: "list_agent_knowledge_candidates", label: "\u77e5\u8bc6\u5e93", patterns: []string{"\u77e5\u8bc6\u5e93", `knowledge\s*base`}},
		{toolName: "list_agent_database_candidates", label: "\u6570\u636e\u5e93", patterns: []string{"\u6570\u636e\u5e93", `database`}},
		{toolName: "list_agent_database_tables", label: "\u6570\u636e\u5e93\u8868", patterns: []string{"\u6570\u636e\u5e93\u8868", "\u6570\u636e\u8868", `database\s*table`, `table`}},
		{toolName: "list_agent_workflow_binding_candidates", label: "\u5de5\u4f5c\u6d41", patterns: []string{"\u5de5\u4f5c\u6d41", `workflow`}},
	} {
		result, ok := results[item.toolName]
		if !ok {
			continue
		}
		names := fastPathRequestedAgentCandidateNames(goal, item.patterns)
		if len(names) == 0 {
			continue
		}
		candidateNames := fastPathCandidateLookupAllNames(result)
		if !fastPathCandidateLookupCanProveAbsence(result, candidateNames) {
			continue
		}
		candidateSet := map[string]struct{}{}
		for _, name := range candidateNames {
			if normalized := fastPathNormalizeCandidateName(name); normalized != "" {
				candidateSet[normalized] = struct{}{}
			}
		}
		for _, name := range names {
			normalized := fastPathNormalizeCandidateName(name)
			if normalized == "" {
				continue
			}
			if _, ok := candidateSet[normalized]; ok {
				continue
			}
			missing = append(missing, fmt.Sprintf("\u672a\u627e\u5230\u540d\u4e3a\u300c%s\u300d\u7684%s\uff1b\u56e0\u6b64\u672a\u4fee\u6539\u914d\u7f6e\uff0c\u672a\u53d1\u8d77\u5ba1\u6279\u3002", strings.TrimSpace(name), item.label))
		}
	}
	return dedupeStrings(missing)
}

func fastPathRawAgentGoalText(evidence map[string]interface{}) string {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	return strings.TrimSpace(firstNonEmptyString(
		evidence["latest_user_request"],
		evidence["user_request"],
		evidence["query"],
		evidence["original_user_goal"],
		evidence["user_goal"],
		plan["original_user_goal"],
		plan["user_goal"],
	))
}

func fastPathRequestedAgentCandidateNames(goal string, resourcePatterns []string) []string {
	goal = strings.TrimSpace(goal)
	if goal == "" || len(resourcePatterns) == 0 {
		return nil
	}
	resource := strings.Join(resourcePatterns, "|")
	patterns := []string{
		`(?i)(?:\x{540d}\x{4e3a}|\x{540d}\x{53eb}|\x{540d}\x{79f0}\x{4e3a}|\x{53eb})\s*[\x{300c}\x{201c}"']?([^\x{300c}\x{300d}\x{201c}\x{201d}"'\x{ff0c},\x{3002}\x{ff1b};\n]+?)[\x{300d}\x{201d}"']?\s*(?:\x{7684})?\s*(?:` + resource + `)`,
		`(?i)(?:named|called|name)\s+["']?([^"'\n,.;\x{ff0c}\x{3002}\x{ff1b}]{1,80}?)["']?\s+(?:` + resource + `)`,
		`(?i)(?:` + resource + `)\s+(?:named|called)\s+["']?([^"'\n,.;\x{ff0c}\x{3002}\x{ff1b}]{1,80}?)["']?`,
	}
	names := []string{}
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		for _, match := range re.FindAllStringSubmatch(goal, -1) {
			if len(match) < 2 {
				continue
			}
			name := fastPathCleanRequestedCandidateName(match[1])
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return dedupeStrings(names)
}

func fastPathCleanRequestedCandidateName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " \t\r\n\u300c\u300d\u201c\u201d\"'`\uff0c,\u3002.;\uff1b:\uff1a")
	return strings.TrimSpace(name)
}

func fastPathCandidateLookupCanProveAbsence(result map[string]interface{}, names []string) bool {
	count := firstNonNegativeInt(result["count"], result["candidates_count"], result["total"])
	if count == 0 {
		return true
	}
	return len(names) >= count
}

func fastPathGoalExplicitlyForbidsAgentMutation(evidence map[string]interface{}) bool {
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if goal == "" {
		return false
	}
	if requested, explicit := fastPathReadOnlyAgentConfigIntent(goal); requested && explicit {
		return true
	}
	return containsAnyFastPathSubstring(goal, []string{
		"do not request approval",
		"don't request approval",
		"dont request approval",
		"do not ask for approval",
		"don't ask for approval",
		"dont ask for approval",
		"without approval",
		"no approval",
		"do not modify",
		"don't modify",
		"dont modify",
		"do not edit",
		"don't edit",
		"dont edit",
		"do not update",
		"don't update",
		"dont update",
		"do not change",
		"don't change",
		"dont change",
		"\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u8981\u5ba1\u6279",
		"\u4e0d\u5ba1\u6279",
		"\u65e0\u9700\u5ba1\u6279",
		"\u4e0d\u8981\u4fee\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u7f16\u8f91",
		"\u4e0d\u7f16\u8f91",
		"\u4e0d\u8981\u66f4\u65b0",
		"\u4e0d\u66f4\u65b0",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u66f4\u6539",
	})
}

func fastPathSuccessfulAgentCandidateLookupResults(evidence map[string]interface{}) map[string]map[string]interface{} {
	results := map[string]map[string]interface{}{}
	record := func(trace skills.SkillTrace) {
		if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) {
			return
		}
		if strings.EqualFold(strings.TrimSpace(trace.Kind), "tool_governance") {
			return
		}
		status := strings.ToLower(strings.TrimSpace(trace.Status))
		if status != "success" && status != "succeeded" && status != "completed" {
			return
		}
		toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
		if !fastPathAgentCandidateLookupTool(toolName) {
			return
		}
		if len(trace.Result) == 0 {
			return
		}
		if !fastPathAgentCandidateLookupResultHasEvidence(trace.Result) {
			return
		}
		results[toolName] = copyFastPathResultMap(trace.Result)
	}
	for _, invocation := range fastPathInvocationSequence(evidence) {
		trace, ok := fastPathTraceFromInvocation(invocation)
		if ok {
			record(trace)
		}
	}
	for _, result := range fastPathLatestToolResultCandidates(evidence) {
		trace, ok := fastPathTraceFromToolResult(result)
		if ok {
			record(trace)
		}
	}
	return results
}

func fastPathAgentCandidateLookupResultHasEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	for _, key := range []string{
		"count",
		"candidates_count",
		"total",
		"candidate_samples",
		"binding_candidates",
		"candidates",
		"items",
		"skills",
		"knowledge_bases",
		"databases",
		"database_tables",
		"workflows",
	} {
		if value, ok := result[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func fastPathRequiredAgentCandidateLookupTools(evidence map[string]interface{}) []string {
	required := []string{}
	seen := map[string]struct{}{}
	add := func(toolName string) {
		toolName = strings.ToLower(strings.TrimSpace(toolName))
		if !fastPathAgentCandidateLookupTool(toolName) {
			return
		}
		if _, ok := seen[toolName]; ok {
			return
		}
		seen[toolName] = struct{}{}
		required = append(required, toolName)
	}
	plan := evidenceMapFromAny(evidence["operation_plan"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		add(stringFromAny(step["tool_name"]))
	}
	goal := fastPathReadOnlyAgentConfigGoalText(evidence)
	if containsAnyFastPathSubstring(goal, []string{"skill", "\u6280\u80fd"}) {
		add("list_agent_skill_candidates")
	}
	if strings.Contains(goal, "\u77e5\u8bc6\u5e93") || strings.Contains(goal, "knowledge") {
		add("list_agent_knowledge_candidates")
	}
	if strings.Contains(goal, "\u6570\u636e\u5e93\u8868") || strings.Contains(goal, "database table") || fastPathTextHasWord(goal, "table", "tables") || strings.Contains(goal, "\u6570\u636e\u8868") {
		add("list_agent_database_tables")
	} else if strings.Contains(goal, "\u6570\u636e\u5e93") || strings.Contains(goal, "database") {
		add("list_agent_database_candidates")
	}
	if strings.Contains(goal, "\u5de5\u4f5c\u6d41") || strings.Contains(goal, "workflow") {
		add("list_agent_workflow_binding_candidates")
	}
	if fastPathGoalRequestsGenericAgentBindableResourceSweep(goal) {
		add("list_agent_skill_candidates")
		add("list_agent_knowledge_candidates")
		add("list_agent_database_candidates")
		add("list_agent_database_tables")
		add("list_agent_workflow_binding_candidates")
	}
	return required
}

func fastPathTextHasWord(text string, words ...string) bool {
	if len(words) == 0 {
		return false
	}
	want := map[string]struct{}{}
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word != "" {
			want[word] = struct{}{}
		}
	}
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	}) {
		if _, ok := want[field]; ok {
			return true
		}
	}
	return false
}

func fastPathGoalRequestsGenericAgentBindableResourceSweep(goal string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	if goal == "" {
		return false
	}
	return containsAnyFastPathSubstring(goal, []string{
		"bindable resources",
		"available resources",
		"candidate resources",
		"resource candidates",
		"selectable resources",
		"resource list",
		"binding resources",
		"\u53ef\u7ed1\u5b9a\u8d44\u6e90",
		"\u53ef\u7ed1\u5b9a\u7684\u8d44\u6e90",
		"\u53ef\u5173\u8054\u8d44\u6e90",
		"\u53ef\u5173\u8054\u7684\u8d44\u6e90",
		"\u5019\u9009\u8d44\u6e90",
		"\u8d44\u6e90\u5019\u9009",
		"\u53ef\u9009\u8d44\u6e90",
		"\u8d44\u6e90\u5217\u8868",
	})
}

func fastPathAgentCandidateLookupTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return true
	default:
		return false
	}
}

func agentCandidateLookupFastPathSection(label string, result map[string]interface{}) string {
	count := firstNonNegativeInt(result["count"], result["candidates_count"], result["total"])
	samples := fastPathCandidateLookupSampleNames(result, 5)
	if count == 0 && len(samples) > 0 {
		count = len(samples)
	}
	line := fmt.Sprintf("- %s\uff1a%d \u4e2a", label, count)
	if len(samples) == 0 {
		return line + "\uff0c\u6682\u65e0\u5019\u9009"
	}
	return line + "\uff0c\u793a\u4f8b\uff1a" + strings.Join(samples, "\u3001")
}

func fastPathCandidateLookupSampleNames(result map[string]interface{}, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	names := fastPathCandidateLookupAllNames(result)
	if len(names) > limit {
		return names[:limit]
	}
	return names
}

func fastPathCandidateLookupAllNames(result map[string]interface{}) []string {
	sources := []interface{}{
		result["candidate_samples"],
		result["binding_candidates"],
		result["candidates"],
		result["items"],
		result["skills"],
		result["knowledge_bases"],
		result["databases"],
		result["database_tables"],
		result["workflows"],
	}
	names := []string{}
	for _, source := range sources {
		for _, item := range mapSliceFromAny(source) {
			name := strings.TrimSpace(firstNonEmptyString(
				item["name"],
				item["label"],
				item["title"],
				item["display_name"],
				item["id"],
				item["skill_id"],
				item["dataset_id"],
				item["data_source_id"],
				item["table_id"],
				item["binding_id"],
			))
			if name == "" {
				continue
			}
			names = append(names, name)
		}
	}
	return dedupeStrings(names)
}

func fastPathNormalizeCandidateName(text string) string {
	text = fastPathCleanRequestedCandidateName(text)
	text = strings.ToLower(text)
	return strings.Join(strings.Fields(text), " ")
}

// FastPathPreferredFinalAnswerForCompletionEvidence returns a narrow
// evidence-grounded replacement for model text when the user explicitly asked
// the turn to mutate an Agent config and then reread the config. In that case,
// the mutation result is the authoritative answer; the post-read proves the
// loop completed but should not let the model reinterpret the mutation as a
// no-op.
func FastPathPreferredFinalAnswerForCompletionEvidence(evidence map[string]interface{}, candidate string) (string, bool) {
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return "", false
	}
	answer, ok := agentConfigPostUpdateVerifiedFastPathAnswerFromEvidence(evidence)
	if !ok && !fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		answer, ok = latestAgentConfigUpdateFastPathAnswerFromEvidence(evidence)
	}
	if !ok || strings.TrimSpace(candidate) == answer {
		return "", false
	}
	return answer, true
}

func latestAgentConfigUpdateFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	trace, ok := fastPathLatestSuccessfulAgentConfigUpdateTrace(evidence)
	if !ok {
		return "", false
	}
	if fastPathBlockedByPendingPlanActionAfterPostUpdateRead(trace, evidence) ||
		fastPathBlockedByPendingPlanAction(trace, evidence) {
		return "", false
	}
	return agentConfigOrIdentityUpdateFastPathAnswer(trace)
}

func agentConfigPostUpdateVerifiedFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if !fastPathGoalRequestsAgentPostUpdateRead(evidence) {
		return "", false
	}
	if !fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		return "", false
	}
	if fastPathHasAgentConfigTargetMismatch(evidence) {
		return "", false
	}
	if trace, ok := fastPathLatestSuccessfulAgentConfigUpdateTrace(evidence); ok {
		answer, ok := agentConfigOrIdentityUpdateFastPathAnswer(trace)
		if ok {
			if fastPathBlockedByPendingPlanActionAfterPostUpdateRead(trace, evidence) {
				return "", false
			}
			return answer + "已在更新后重新读取配置并完成确认。", true
		}
	}
	for _, result := range fastPathLatestToolResultCandidates(evidence) {
		trace, ok := fastPathTraceFromToolResult(result)
		if !ok || !fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
			continue
		}
		answer, ok := agentConfigOrIdentityUpdateFastPathAnswer(trace)
		if !ok {
			continue
		}
		if fastPathBlockedByPendingPlanActionAfterPostUpdateRead(trace, evidence) {
			continue
		}
		return answer + "已在更新后重新读取配置并完成确认。", true
	}
	return "", false
}

func fastPathHasAgentConfigTargetMismatch(evidence map[string]interface{}) bool {
	if _, ok := fastPathLatestSuccessfulAgentConfigReadResultAfterUpdate(evidence); !ok {
		return false
	}
	return len(completionVerificationAgentConfigMismatches(evidence)) > 0
}

func fastPathLatestSuccessfulAgentConfigReadResultAfterUpdate(evidence map[string]interface{}) (map[string]interface{}, bool) {
	invocations := fastPathInvocationSequence(evidence)
	latestUpdateIndex := -1
	for index, invocation := range invocations {
		trace, ok := fastPathTraceFromInvocation(invocation)
		if !ok || !fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
			continue
		}
		latestUpdateIndex = index
	}
	if latestUpdateIndex < 0 {
		return nil, false
	}
	for index := len(invocations) - 1; index > latestUpdateIndex; index-- {
		invocation := invocations[index]
		if !fastPathInvocationSucceeded(invocation) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "get_agent_config") {
			continue
		}
		result := copyFastPathResultMap(evidenceMapFromAny(invocation["result"]))
		if len(result) == 0 {
			result = copyFastPathResultMap(evidenceMapFromAny(invocation["result_summary"]))
		}
		if len(result) == 0 {
			result = copyFastPathResultMap(evidenceMapFromAny(invocation["tool_result"]))
		}
		if len(result) == 0 {
			continue
		}
		return result, true
	}
	return nil, false
}

func agentConfigOrIdentityUpdateFastPathAnswer(trace skills.SkillTrace) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(trace.ToolName)) {
	case "update_agent_config":
		return agentConfigUpdateFastPathAnswer(trace.Result)
	case "update_agent_identity":
		return agentIdentityUpdateFastPathAnswer(trace.Result)
	default:
		return "", false
	}
}

func fastPathLatestSuccessfulAgentConfigUpdateTrace(evidence map[string]interface{}) (skills.SkillTrace, bool) {
	invocations := fastPathInvocationSequence(evidence)
	for i := len(invocations) - 1; i >= 0; i-- {
		trace, ok := fastPathTraceFromInvocation(invocations[i])
		if !ok || !fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
			continue
		}
		return trace, true
	}
	return skills.SkillTrace{}, false
}

func fastPathTraceFromInvocation(invocation map[string]interface{}) (skills.SkillTrace, bool) {
	if len(invocation) == 0 {
		return skills.SkillTrace{}, false
	}
	skillID := strings.TrimSpace(firstNonEmptyString(invocation["skill_id"], invocation["skillID"]))
	toolName := strings.TrimSpace(firstNonEmptyString(invocation["tool_name"], invocation["toolName"]))
	status := strings.TrimSpace(firstNonEmptyString(invocation["status"], invocation["result_status"]))
	if skillID == "" || toolName == "" || status == "" {
		return skills.SkillTrace{}, false
	}
	payload := copyFastPathResultMap(evidenceMapFromAny(invocation["result"]))
	if len(payload) == 0 {
		payload = copyFastPathResultMap(evidenceMapFromAny(invocation["tool_result"]))
	}
	if len(payload) == 0 {
		payload = copyFastPathResultMap(invocation)
	}
	if _, ok := payload["status"]; !ok {
		payload["status"] = status
	}
	return skills.SkillTrace{
		Kind:      strings.TrimSpace(firstNonEmptyString(invocation["kind"], "tool_call")),
		Status:    status,
		SkillID:   skillID,
		ToolName:  toolName,
		Result:    payload,
		Arguments: copyFastPathResultMap(evidenceMapFromAny(invocation["arguments"])),
	}, true
}

func fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence map[string]interface{}) bool {
	if fastPathPlanHasPendingPostUpdateAgentRead(evidenceMapFromAny(evidence["operation_plan"])) &&
		!fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		return true
	}
	if !fastPathEvidenceRequestsAgentPostUpdateRead(evidence) {
		return false
	}
	if !fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence) {
		return false
	}
	return !fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence)
}

func fastPathEvidenceRequestsAgentPostUpdateRead(evidence map[string]interface{}) bool {
	if fastPathPlanHasPendingPostUpdateAgentRead(evidenceMapFromAny(evidence["operation_plan"])) {
		return true
	}
	return fastPathGoalRequestsAgentPostUpdateRead(evidence)
}

func fastPathPlanHasPendingPostUpdateAgentRead(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 || !completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) {
			continue
		}
		id := strings.TrimSpace(evidenceStringFromAny(step["id"]))
		status := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		switch status {
		case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
			continue
		default:
			return true
		}
	}
	return false
}

func fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence map[string]interface{}) bool {
	for _, invocation := range fastPathInvocationSequence(evidence) {
		if !fastPathInvocationSucceeded(invocation) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		if fastPathInvocationIsSuccessfulAgentConfigUpdate(invocation) {
			return true
		}
	}
	for _, result := range fastPathLatestToolResultCandidates(evidence) {
		trace, ok := fastPathTraceFromToolResult(result)
		if !ok {
			continue
		}
		if fastPathTraceIsSuccessfulAgentConfigUpdate(trace) {
			return true
		}
	}
	return false
}

func completionEvidencePlanStepIsRequiredPostUpdateAgentConfigRead(step map[string]interface{}) bool {
	if !completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "get_agent_config") {
		return false
	}
	return true
}

func completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
		return false
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if !strings.EqualFold(toolName, "get_agent_config") &&
		!strings.EqualFold(toolName, "get_agent") {
		return false
	}
	if boolFromAny(step["required_post_update_verification"]) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(step["phase"])), "post_update_verification") {
		return true
	}
	id := strings.ToLower(strings.TrimSpace(stringFromAny(step["id"])))
	return strings.Contains(id, "get_agent_config#post_update") ||
		strings.Contains(id, "get_agent#post_update")
}

func completionEvidenceForFastPath(req RunRequest) map[string]interface{} {
	var evidence map[string]interface{}
	if req.CompletionEvidence != nil {
		evidence = req.CompletionEvidence()
	}
	if len(evidence) > 0 {
		evidence = copyStringAnyMap(evidence)
	} else {
		evidence = map[string]interface{}{}
	}
	evidence = mergeCurrentMetadataIntoFastPathEvidence(evidence, currentMetadataForRun(req))
	if text := strings.TrimSpace(latestUserRequestText(req)); text != "" {
		evidence["latest_user_request"] = text
		for _, key := range []string{"original_user_goal", "user_goal", "user_request", "query"} {
			if strings.TrimSpace(stringFromAny(evidence[key])) == "" {
				evidence[key] = text
			}
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return evidence
}

func latestUserRequestText(req RunRequest) string {
	if req.Prepared != nil {
		if text := strings.TrimSpace(req.Prepared.Query); text != "" {
			return text
		}
	}
	return latestUserMessageText(req)
}

func mergeCurrentMetadataIntoFastPathEvidence(evidence map[string]interface{}, metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return evidence
	}
	if evidence == nil {
		evidence = map[string]interface{}{}
	}
	for _, key := range []string{
		"operation_plan",
		"operation_result_summary",
		"execution_summary",
		"execution_ledger",
		"agent_create_progress",
		"generated_files",
		"client_actions",
	} {
		if _, exists := evidence[key]; exists {
			continue
		}
		if value, ok := metadata[key]; ok && value != nil {
			evidence[key] = value
		}
	}
	if metadataPlan := evidenceMapFromAny(metadata["operation_plan"]); len(metadataPlan) > 0 {
		evidencePlan := evidenceMapFromAny(evidence["operation_plan"])
		_, evidenceHasPendingMutation := firstPendingAgentMutationPlanStep(map[string]interface{}{"operation_plan": evidencePlan})
		_, metadataHasPendingMutation := firstPendingAgentMutationPlanStep(map[string]interface{}{"operation_plan": metadataPlan})
		if !evidenceHasPendingMutation && metadataHasPendingMutation {
			evidence["operation_plan"] = copyStringAnyMap(metadataPlan)
		}
	}
	return evidence
}

func latestUserMessageText(req RunRequest) string {
	if req.Prepared == nil || req.Prepared.LLMRequest == nil {
		return ""
	}
	messages := req.Prepared.LLMRequest.Messages
	for i := len(messages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			continue
		}
		if text := strings.TrimSpace(messageContent(messages[i].Content)); text != "" {
			return text
		}
	}
	return ""
}

func fastPathBlockedByPendingPlanAction(trace skills.SkillTrace, evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	pendingSteps, hasPlanSteps := fastPathPendingExecutablePlanSteps(plan, 8)
	if hasPlanSteps {
		for _, step := range pendingSteps {
			if fastPathPlanStepIsRoute(step) && fastPathPendingRouteStepHasPendingDependents(plan, step) {
				return true
			}
			if fastPathPendingPlanStepSatisfiedByTrace(trace, step, plan) {
				continue
			}
			if fastPathPendingActionBlocksTrace(trace, fastPathPlanStepAction(step)) {
				return true
			}
		}
		return false
	}

	pendingActions := []string{}
	if !hasPlanSteps {
		pending := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["pending_next_action"])))
		if pending != "" {
			pendingActions = append(pendingActions, pending)
		}
	}
	for _, pending := range pendingActions {
		if fastPathPendingActionBlocksTrace(trace, pending) {
			return true
		}
	}
	return false
}

func fastPathBlockedByPendingPlanActionAfterPostUpdateRead(trace skills.SkillTrace, evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	pendingSteps, hasPlanSteps := fastPathPendingExecutablePlanSteps(plan, 8)
	if hasPlanSteps {
		for _, step := range pendingSteps {
			if fastPathPendingPlanStepSatisfiedByTrace(trace, step, plan) {
				continue
			}
			if completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) && fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
				continue
			}
			if fastPathPendingActionBlocksTrace(trace, fastPathPlanStepAction(step)) {
				return true
			}
		}
		return false
	}

	pending := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["pending_next_action"])))
	if pending == "" {
		return false
	}
	if fastPathPendingActionIsAgentConfigRead(pending) && fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		return false
	}
	return fastPathPendingActionBlocksTrace(trace, pending)
}

func fastPathPendingActionBlocksTrace(trace skills.SkillTrace, pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" || pending == "none" || pending == "done" || pending == "completed" {
		return false
	}
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	skillID := strings.ToLower(strings.TrimSpace(trace.SkillID))
	if toolName == "" {
		return true
	}
	if pending == toolName || strings.Contains(pending, toolName) {
		return false
	}
	if skillID != "" && strings.Contains(pending, skillID+"/"+toolName) {
		return false
	}
	if fastPathTraceIsSuccessfulAgentConfigUpdate(trace) && fastPathPendingActionIsAgentConfigRead(pending) {
		return true
	}
	switch {
	case fastPathAuthoritativeMutation(trace) && fastPathPendingActionIsPostVerification(pending):
		return false
	case fastPathTraceIsConsoleNavigation(trace) && fastPathPendingActionIsRoutePostVerification(pending):
		return false
	case fastPathTraceIsTemporaryArtifactGeneration(trace) && fastPathPendingActionIsArtifactPostVerification(pending):
		return false
	}
	return true
}

func fastPathPendingActionIsAgentConfigRead(pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" {
		return false
	}
	return strings.Contains(pending, "agent-management/get_agent_config") ||
		strings.Contains(pending, "get_agent_config")
}

func fastPathPendingExecutablePlanActions(plan map[string]interface{}, limit int) ([]string, bool) {
	steps, hasPlanSteps := fastPathPendingExecutablePlanSteps(plan, limit)
	if len(steps) == 0 || limit <= 0 {
		return nil, hasPlanSteps
	}
	actions := make([]string, 0, len(steps))
	for _, step := range steps {
		action := fastPathPlanStepAction(step)
		if action == "" {
			continue
		}
		actions = append(actions, action)
	}
	return actions, hasPlanSteps
}

func fastPathPendingExecutablePlanSteps(plan map[string]interface{}, limit int) ([]map[string]interface{}, bool) {
	steps := mapSliceFromAny(plan["steps"])
	structuredSteps, hasStructuredSteps := fastPathPendingStructuredPlanSteps(plan, limit)
	hasPlanSteps := len(steps) > 0 || hasStructuredSteps
	if limit <= 0 {
		return nil, hasPlanSteps
	}
	pendingSteps := make([]map[string]interface{}, 0, limit)
	seen := map[string]struct{}{}
	appendStep := func(step map[string]interface{}) {
		if len(step) == 0 || len(pendingSteps) >= limit {
			return
		}
		key := fastPathPlanStepAction(step)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(firstNonEmptyString(step["id"], step["title"])))
		}
		if key != "" {
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		pendingSteps = append(pendingSteps, step)
	}
	for _, step := range steps {
		if !fastPathPlanStepBlocksCompletion(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], fastPathPlanStepStatusValue(plan["step_status"], id)))
		if status == "completed" || status == "failed" {
			continue
		}
		if fastPathPlanStepAction(step) == "" {
			continue
		}
		appendStep(step)
	}
	for _, step := range structuredSteps {
		appendStep(step)
	}
	return pendingSteps, hasPlanSteps
}

func fastPathPendingStructuredPlanSteps(plan map[string]interface{}, limit int) ([]map[string]interface{}, bool) {
	structured := evidenceMapFromAny(plan["structured_plan"])
	if len(structured) == 0 {
		return nil, false
	}
	operations := mapSliceFromAny(structured["operations"])
	source := operations
	hasStructuredSteps := len(operations) > 0
	if len(source) == 0 {
		source = mapSliceFromAny(structured["required_tool_sequence"])
		hasStructuredSteps = len(source) > 0
	}
	if len(source) == 0 || limit <= 0 {
		return nil, hasStructuredSteps
	}
	defaultSkillID := fastPathStructuredPlanDefaultSkillID(structured)
	pending := make([]map[string]interface{}, 0, limit)
	for index, item := range source {
		step := fastPathStructuredPlanItemAsStep(item, defaultSkillID, index)
		if !fastPathPlanStepBlocksCompletion(step) || fastPathPlanStepAction(step) == "" {
			continue
		}
		status := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], item["status"]))
		if status == "completed" || status == "failed" {
			continue
		}
		step["status"] = status
		pending = append(pending, step)
		if len(pending) >= limit {
			break
		}
	}
	return pending, hasStructuredSteps
}

func fastPathStructuredPlanItemAsStep(item map[string]interface{}, defaultSkillID string, index int) map[string]interface{} {
	if len(item) == 0 {
		return nil
	}
	skillID := strings.TrimSpace(firstNonEmptyString(item["skill_id"], defaultSkillID))
	toolName := strings.TrimSpace(stringFromAny(item["tool_name"]))
	action := strings.TrimSpace(stringFromAny(item["action"]))
	resourceType := strings.TrimSpace(stringFromAny(item["resource_type"]))
	id := strings.TrimSpace(firstNonEmptyString(item["id"], item["step_id"]))
	if id == "" {
		id = fastPathStructuredPlanSyntheticStepID(skillID, toolName, action, resourceType, index)
	}
	step := map[string]interface{}{
		"id":       id,
		"status":   strings.TrimSpace(stringFromAny(item["status"])),
		"skill_id": skillID,
	}
	if toolName != "" {
		step["tool_name"] = toolName
	}
	if action != "" {
		step["action"] = action
	}
	if resourceType != "" {
		step["resource_type"] = resourceType
	}
	for _, key := range []string{"title", "wait_for", "wait_for_step_id", "phase"} {
		if value := strings.TrimSpace(stringFromAny(item[key])); value != "" {
			step[key] = value
		}
	}
	return step
}

func fastPathStructuredPlanSyntheticStepID(skillID string, toolName string, action string, resourceType string, index int) string {
	switch {
	case skillID != "" && toolName != "":
		return fmt.Sprintf("structured:%s/%s#%d", strings.ToLower(skillID), strings.ToLower(toolName), index)
	case action != "" || resourceType != "":
		return fmt.Sprintf("structured:%s:%s#%d", strings.ToLower(action), strings.ToLower(resourceType), index)
	default:
		return fmt.Sprintf("structured:operation#%d", index)
	}
}

func fastPathStructuredPlanDefaultSkillID(structured map[string]interface{}) string {
	if len(structured) == 0 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(structured["domain"]))) {
	case "agent_management":
		return skills.SkillAgentManagement
	default:
		return ""
	}
}

func fastPathPendingPlanStepSatisfiedByTrace(trace skills.SkillTrace, step map[string]interface{}, plan map[string]interface{}) bool {
	if !fastPathPlanStepIsAgentCreate(step) {
		if fastPathTraceIsSuccessfulAgentConfigUpdate(trace) && fastPathPlanStepIsAgentIdentityUpdate(step) {
			return fastPathAgentIdentityStepIsStaleForConfigUpdate(plan)
		}
		return false
	}
	return false
}

func fastPathTraceIsSuccessfulAgentBatchDelete(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "delete_agents") {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return false
	}
	return fastPathAgentBatchDeleteHasCompleteEvidence(trace.Result)
}

func fastPathAgentBatchDeleteHasCompleteEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	group := payloadMap(result, "operation_group")
	items := mapSliceFromAny(firstNonEmptyValue(group["item_results"], result["item_results"]))
	if len(items) == 0 {
		return false
	}
	targetCount := firstPositiveInt(result["target_count"], group["target_count"], len(items))
	if targetCount <= 0 {
		return false
	}
	counted := 0
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(stringFromAny(item["status"]))) {
		case "succeeded", "success", "completed", "failed", "skipped", "rejected":
			counted++
		}
	}
	return counted >= targetCount
}

func fastPathTraceIsSuccessfulAgentConfigUpdate(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(trace.ToolName)) {
	case "update_agent_config":
		return len(agentConfigUpdateDetails(trace.Result)) > 0
	case "update_agent_identity":
		return fastPathAgentUpdateEffectIsSuccessful(trace.Result) &&
			len(agentIdentityUpdatedFieldLabels(sanitizedStringListArgumentValue(trace.Result["updated_fields"]))) > 0
	default:
		return false
	}
}

func fastPathAgentUpdateEffectIsSuccessful(result map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "" && status != "completed" && status != "success" && status != "succeeded" {
		return false
	}
	effect := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["effect"], result["operation"])))
	return effect == "" || effect == "updated" || effect == "update" || effect == "agent.update"
}

func fastPathPlanStepIsAgentIdentityUpdate(step map[string]interface{}) bool {
	return strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "update_agent_identity")
}

func containsAnyFastPathSubstring(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func fastPathAgentIdentityStepIsStaleForConfigUpdate(plan map[string]interface{}) bool {
	return !fastPathOriginalGoalRequestsAgentIdentityUpdate(plan)
}

func fastPathOriginalGoalRequestsAgentIdentityUpdate(plan map[string]interface{}) bool {
	goal := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		plan["original_user_goal"],
		plan["user_goal"],
		plan["objective"],
	)))
	if goal == "" {
		return false
	}
	goal = fastPathStripAgentIdentityPreservationPhrases(goal)
	for _, marker := range []string{
		"rename",
		"change name",
		"update name",
		"edit name",
		"set name",
		"change description",
		"update description",
		"edit description",
		"set description",
		"change icon",
		"update icon",
		"edit icon",
		"set icon",
		"all editable",
		"everything editable",
		"\u4fee\u6539\u540d\u79f0",
		"\u66f4\u65b0\u540d\u79f0",
		"\u8bbe\u7f6e\u540d\u79f0",
		"\u540d\u79f0\u6539\u4e3a",
		"\u540d\u79f0\u6539\u6210",
		"\u540d\u5b57\u6539\u4e3a",
		"\u540d\u5b57\u6539\u6210",
		"\u6539\u540d",
		"\u91cd\u547d\u540d",
		"\u4fee\u6539\u63cf\u8ff0",
		"\u66f4\u65b0\u63cf\u8ff0",
		"\u8bbe\u7f6e\u63cf\u8ff0",
		"\u63cf\u8ff0\u6539\u4e3a",
		"\u63cf\u8ff0\u6539\u6210",
		"\u4fee\u6539\u56fe\u6807",
		"\u66f4\u65b0\u56fe\u6807",
		"\u8bbe\u7f6e\u56fe\u6807",
		"\u56fe\u6807\u6539\u4e3a",
		"\u56fe\u6807\u6539\u6210",
		"\u4fee\u6539\u5934\u50cf",
		"\u66f4\u65b0\u5934\u50cf",
		"\u5934\u50cf\u6539\u4e3a",
		"\u5934\u50cf\u6539\u6210",
		"\u6240\u6709\u53ef\u4fee\u6539",
		"\u6240\u6709\u80fd\u4fee\u6539",
		"\u5168\u90e8\u53ef\u4fee\u6539",
	} {
		if strings.Contains(goal, marker) {
			return true
		}
	}
	return false
}

func fastPathStripAgentIdentityPreservationPhrases(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"do not change the name", " ",
		"do not change name", " ",
		"don't change the name", " ",
		"don't change name", " ",
		"keep the name unchanged", " ",
		"keep name unchanged", " ",
		"keep the name", " ",
		"keep name", " ",
		"do not rename", " ",
		"don't rename", " ",
		"do not change the description", " ",
		"do not change description", " ",
		"don't change the description", " ",
		"don't change description", " ",
		"keep the description unchanged", " ",
		"keep description unchanged", " ",
		"keep the description", " ",
		"keep description", " ",
		"do not change the icon", " ",
		"do not change icon", " ",
		"don't change the icon", " ",
		"don't change icon", " ",
		"keep the icon unchanged", " ",
		"keep icon unchanged", " ",
		"keep the icon", " ",
		"keep icon", " ",
		"\u4e0d\u6539\u540d\u79f0", " ",
		"\u4e0d\u4fee\u6539\u540d\u79f0", " ",
		"\u4e0d\u8981\u6539\u540d\u79f0", " ",
		"\u4e0d\u8981\u4fee\u6539\u540d\u79f0", " ",
		"\u4e0d\u7528\u6539\u540d\u79f0", " ",
		"\u4e0d\u7528\u4fee\u6539\u540d\u79f0", " ",
		"\u4e0d\u6539\u540d", " ",
		"\u4e0d\u8981\u6539\u540d", " ",
		"\u4e0d\u91cd\u547d\u540d", " ",
		"\u540d\u79f0\u4e0d\u53d8", " ",
		"\u540d\u5b57\u4e0d\u53d8", " ",
		"\u4fdd\u6301\u540d\u79f0", " ",
		"\u4fdd\u6301\u540d\u5b57", " ",
		"\u4e0d\u6539\u63cf\u8ff0", " ",
		"\u4e0d\u4fee\u6539\u63cf\u8ff0", " ",
		"\u4e0d\u8981\u6539\u63cf\u8ff0", " ",
		"\u4e0d\u8981\u4fee\u6539\u63cf\u8ff0", " ",
		"\u4e0d\u7528\u6539\u63cf\u8ff0", " ",
		"\u4e0d\u7528\u4fee\u6539\u63cf\u8ff0", " ",
		"\u63cf\u8ff0\u4e0d\u53d8", " ",
		"\u4fdd\u6301\u63cf\u8ff0", " ",
		"\u4e0d\u6539\u56fe\u6807", " ",
		"\u4e0d\u4fee\u6539\u56fe\u6807", " ",
		"\u4e0d\u8981\u6539\u56fe\u6807", " ",
		"\u4e0d\u8981\u4fee\u6539\u56fe\u6807", " ",
		"\u4e0d\u7528\u6539\u56fe\u6807", " ",
		"\u4e0d\u7528\u4fee\u6539\u56fe\u6807", " ",
		"\u56fe\u6807\u4e0d\u53d8", " ",
		"\u4fdd\u6301\u56fe\u6807", " ",
		"\u4e0d\u6539\u5934\u50cf", " ",
		"\u5934\u50cf\u4e0d\u53d8", " ",
		"\u4fdd\u6301\u5934\u50cf", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(goal)), " ")
}

func fastPathPlanStepIsAgentCreate(step map[string]interface{}) bool {
	return strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "create_agent")
}

func fastPathPlanStepStatusValue(statuses interface{}, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	switch typed := statuses.(type) {
	case map[string]interface{}:
		return stringFromAny(typed[id])
	case map[string]string:
		return typed[id]
	default:
		return ""
	}
}

func fastPathPlanStepBlocksCompletion(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	if fastPathPlanStepIsSkillLoadOnly(step) {
		return false
	}
	return true
}

func fastPathPlanStepIsSkillLoadOnly(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	return strings.HasPrefix(id, "skill:") &&
		strings.TrimSpace(stringFromAny(step["tool_name"])) == ""
}

func fastPathPlanStepAction(step map[string]interface{}) string {
	if len(step) == 0 {
		return ""
	}
	skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName != "" {
		if skillID != "" {
			return strings.ToLower(skillID + "/" + toolName)
		}
		return strings.ToLower(toolName)
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(step["id"], step["title"])))
}

func fastPathPlanStepIsRoute(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(stringFromAny(step["id"])))
	skillID := strings.ToLower(strings.TrimSpace(stringFromAny(step["skill_id"])))
	toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
	return strings.Contains(id, "route") ||
		strings.Contains(id, "navigate") ||
		skillID == skills.SkillConsoleNavigator ||
		toolName == "navigate" ||
		toolName == "route"
}

func fastPathPendingRouteStepHasPendingDependents(plan map[string]interface{}, routeStep map[string]interface{}) bool {
	routeID := strings.TrimSpace(stringFromAny(routeStep["id"]))
	if len(plan) == 0 || routeID == "" {
		return false
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if len(step) == 0 || strings.TrimSpace(stringFromAny(step["id"])) == routeID {
			continue
		}
		waitFor := strings.TrimSpace(firstNonEmptyString(step["wait_for"], step["wait_for_step_id"]))
		if waitFor != routeID {
			continue
		}
		if !fastPathPlanStepBlocksCompletion(step) || fastPathPlanStepAction(step) == "" {
			continue
		}
		stepID := strings.TrimSpace(stringFromAny(step["id"]))
		status := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], stepStatus[stepID]))
		if status == "completed" || status == "failed" {
			continue
		}
		return true
	}
	return false
}

func fastPathNormalizePlanStepStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "allowed", "approved", "completed":
		return "completed"
	case "failure", "error", "rejected", "denied", "failed":
		return "failed"
	case "running", "streaming", "in_progress", "waiting_approval", "pending":
		return "pending"
	default:
		return "pending"
	}
}

func fastPathAuthoritativeMutation(trace skills.SkillTrace) bool {
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	switch {
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		switch toolName {
		case "delete_agent", "delete_agents", "update_agent_identity", "update_agent_config",
			"replace_agent_skill_bindings", "replace_agent_knowledge_bindings",
			"replace_agent_database_bindings", "replace_agent_workflow_bindings",
			"replace_agent_memory_slots":
			return true
		}
	case strings.EqualFold(skillID, skills.SkillFileManager):
		return toolName == "save_file_to_management" || toolName == "delete_file"
	}
	return false
}

func fastPathTraceIsConsoleNavigation(trace skills.SkillTrace) bool {
	return strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillConsoleNavigator) &&
		strings.EqualFold(strings.TrimSpace(trace.ToolName), "navigate")
}

func fastPathTraceIsTemporaryArtifactGeneration(trace skills.SkillTrace) bool {
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	if strings.EqualFold(skillID, skills.SkillChartGenerator) {
		return toolName == "generate_chart"
	}
	if !strings.EqualFold(skillID, skills.SkillFileGenerator) {
		return false
	}
	switch toolName {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func agentCreateFastPathAnswerWithEvidence(trace skills.SkillTrace, evidence map[string]interface{}) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "create_agent") {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(trace.Status))
	if status != "success" && status != "succeeded" && status != "completed" {
		return "", false
	}
	if !agentCreateEvidenceObservationSucceeded(evidence) {
		return "", false
	}
	return agentCreateFastPathAnswerFromEvidence(evidence)
}

func agentCreateFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if !agentCreateEvidenceObservationSucceeded(evidence) {
		return "", false
	}
	results := agentCreateSuccessfulResultsFromEvidence(evidence)
	if len(results) == 0 {
		return "", false
	}
	expectedCount := agentCreateExpectedCountFromEvidence(evidence)
	if expectedCount > 0 && len(results) < expectedCount {
		return "", false
	}
	trace := skills.SkillTrace{
		Kind:     "client_action",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "create_agent",
		Status:   "succeeded",
	}
	if fastPathBlockedByPendingPlanAction(trace, evidence) {
		return "", false
	}

	names := make([]string, 0, len(results))
	for _, result := range results {
		if name := agentCreateResultName(result); name != "" {
			names = append(names, name)
		}
	}
	names = dedupeStrings(names)
	if len(names) == 0 {
		return "", false
	}
	return fmt.Sprintf("已创建 %d 个智能体%s。", len(names), formatNameList(names)), true
}

func agentCreateEvidenceObservationSucceeded(evidence map[string]interface{}) bool {
	for _, action := range evidenceClientActions(evidence) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(action["tool_name"])), "create_agent") {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(stringFromAny(action["status"])))
		if status == "succeeded" || status == "success" || status == "completed" {
			return true
		}
	}
	if len(agentCreateSuccessfulResultsFromEvidence(evidence)) == 0 {
		return false
	}
	if agentCreatePlanHasCompletedObservation(evidence) {
		return true
	}
	for _, action := range evidenceClientActions(evidence) {
		if agentCreateClientActionIsObservation(action) {
			return true
		}
	}
	return false
}

func agentCreatePlanHasCompletedObservation(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		ledger := evidenceMapFromAny(evidence["execution_ledger"])
		plan = evidenceMapFromAny(ledger["operation_plan"])
	}
	if len(plan) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(plan["status"])))
	if status == "completed" || status == "complete" || status == "done" {
		return true
	}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		id := strings.ToLower(strings.TrimSpace(firstNonEmptyString(step["id"], step["title"])))
		if !strings.Contains(id, "observe") && !strings.Contains(id, "route_loaded") && !strings.Contains(id, "asset_observation") {
			continue
		}
		stepStatus := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], fastPathPlanStepStatusValue(plan["step_status"], id)))
		if stepStatus == "completed" || stepStatus == "done" {
			return true
		}
	}
	return false
}

func agentCreateClientActionIsObservation(action map[string]interface{}) bool {
	if len(action) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(action["status"])))
	if status != "succeeded" && status != "success" && status != "completed" {
		return false
	}
	actionType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action["action_type"], action["event_type"])))
	reason := strings.ToLower(strings.TrimSpace(stringFromAny(action["reason"])))
	result := evidenceMapFromAny(action["result"])
	resultEvent := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["event_type"], result["action_type"])))
	if strings.Contains(reason, "created_agent") || strings.Contains(reason, "open_created_agent") {
		return true
	}
	for _, value := range []string{actionType, resultEvent} {
		if strings.Contains(value, "route_loaded") || strings.Contains(value, "asset_observed") || strings.Contains(value, "page_observed") {
			return true
		}
	}
	return false
}

func evidenceClientActions(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	if actions := mapSliceFromAny(evidence["client_actions"]); len(actions) > 0 {
		return actions
	}
	ledger := evidenceMapFromAny(evidence["execution_ledger"])
	return mapSliceFromAny(ledger["client_actions"])
}

func agentCreateSuccessfulResultsFromEvidence(evidence map[string]interface{}) []map[string]interface{} {
	invocations := evidenceSkillInvocations(evidence)
	if len(invocations) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0)
	seen := map[string]struct{}{}
	for _, invocation := range invocations {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "create_agent") ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
			continue
		}
		result := evidenceMapFromAny(invocation["result"])
		if !agentCreateResultSucceeded(result) {
			continue
		}
		key := firstNonEmptyString(result["agent_id"], result["id"], result["href"], result["agent_name"], result["name"])
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, result)
	}
	return out
}

func evidenceSkillInvocations(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	if invocations := mapSliceFromAny(evidence["skill_invocations"]); len(invocations) > 0 {
		return invocations
	}
	ledger := evidenceMapFromAny(evidence["execution_ledger"])
	return mapSliceFromAny(ledger["skill_invocations"])
}

func agentCreateResultSucceeded(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "" && status != "completed" && status != "success" && status != "succeeded" {
		return false
	}
	effect := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["effect"], result["operation"])))
	if effect != "" && effect != "created" && effect != "create" && effect != "agent.create" {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(result["agent_id"], result["id"], result["href"], result["agent_name"], result["name"])) != ""
}

func agentCreateResultName(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	if name := strings.TrimSpace(firstNonEmptyString(result["agent_name"], result["name"])); name != "" {
		return name
	}
	agent := payloadMap(result, "agent")
	return strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"]))
}

func agentCreateExpectedCountFromEvidence(evidence map[string]interface{}) int {
	text := strings.ToLower(strings.TrimSpace(firstNonEmptyString(evidence["user_request"])))
	if text == "" {
		plan := evidenceMapFromAny(evidence["operation_plan"])
		text = strings.ToLower(strings.TrimSpace(firstNonEmptyString(plan["original_user_goal"])))
	}
	if text == "" {
		return 0
	}
	if count := agentCreateExpectedNamedAgentCount(text); count > 1 {
		return count
	}
	for _, candidate := range []struct {
		count   int
		markers []string
	}{
		{10, []string{"10个", "10 个", "ten agents"}},
		{9, []string{"9个", "9 个", "nine agents", "九个", "九 个"}},
		{8, []string{"8个", "8 个", "eight agents", "八个", "八 个"}},
		{7, []string{"7个", "7 个", "seven agents", "七个", "七 个"}},
		{6, []string{"6个", "6 个", "six agents", "六个", "六 个"}},
		{5, []string{"5个", "5 个", "five agents", "五个", "五 个"}},
		{4, []string{"4个", "4 个", "four agents", "四个", "四 个"}},
		{3, []string{"3个", "3 个", "three agents", "三个", "三 个"}},
		{2, []string{"2个", "2 个", "two agents", "两个", "两 个"}},
		{1, []string{"1个", "1 个", "one agent", "一个", "一 个"}},
	} {
		for _, marker := range candidate.markers {
			if strings.Contains(text, marker) {
				return candidate.count
			}
		}
	}
	return 0
}

func agentCreateExpectedNamedAgentCount(text string) int {
	for _, marker := range []string{
		"agents named ",
		"agents called ",
		"agents titled ",
	} {
		index := strings.Index(text, marker)
		if index < 0 {
			continue
		}
		tail := strings.TrimSpace(text[index+len(marker):])
		if tail == "" {
			continue
		}
		return len(splitAgentCreateExpectedNames(tail))
	}
	return 0
}

func splitAgentCreateExpectedNames(text string) []string {
	replacer := strings.NewReplacer(
		"\uff0c", ",",
		"\u3001", ",",
		" and ", ",",
		" & ", ",",
		" plus ", ",",
		" then ", ",",
		" after ", ",",
	)
	normalized := replacer.Replace(strings.TrimSpace(text))
	parts := strings.Split(normalized, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		names = append(names, part)
	}
	return names
}

func fastPathPendingActionIsPostVerification(pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" {
		return false
	}
	for _, marker := range []string{
		"observe",
		"observation",
		"asset_observation",
		"page_refresh",
		"refresh",
		"route_loaded",
		"navigate",
		"console-navigator",
		"list_",
		"get_",
		"search",
		"read",
		"inspect",
		"generated_file_metadata",
		"generated_files",
		"message_file_card",
		"file card",
	} {
		if strings.Contains(pending, marker) {
			return true
		}
	}
	return false
}

func fastPathPendingActionIsRoutePostVerification(pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" {
		return false
	}
	for _, marker := range []string{
		"observe",
		"observation",
		"asset_observation",
		"page_refresh",
		"refresh",
		"route_loaded",
		"current_page",
		"inspect",
	} {
		if strings.Contains(pending, marker) {
			return true
		}
	}
	return false
}

func fastPathPendingActionIsArtifactPostVerification(pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" {
		return false
	}
	for _, marker := range []string{
		"generated_file_metadata",
		"generated_files",
		"message_file_card",
		"file card",
	} {
		if strings.Contains(pending, marker) {
			return true
		}
	}
	return false
}

func latestToolResultFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	for _, result := range fastPathLatestToolResultCandidates(evidence) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(result["kind"])), "tool_governance") {
			continue
		}
		trace, ok := fastPathTraceFromToolResult(result)
		if !ok {
			continue
		}
		if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
			return answer, true
		}
	}
	invocations := fastPathInvocationSequence(evidence)
	for i := len(invocations) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocations[i]["kind"])), "tool_governance") {
			continue
		}
		if !fastPathInvocationSucceeded(invocations[i]) {
			continue
		}
		trace, ok := fastPathTraceFromInvocation(invocations[i])
		if !ok {
			continue
		}
		if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(trace, evidence); ok {
			return answer, true
		}
	}
	return "", false
}

func generatedArtifactFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	for _, artifact := range fastPathGeneratedArtifactCandidates(evidence) {
		skillID := strings.TrimSpace(firstNonEmptyString(artifact["skill_id"], artifact["skillID"]))
		toolName := strings.TrimSpace(firstNonEmptyString(artifact["tool_name"], artifact["toolName"]))
		if skillID == "" {
			skillID = skills.SkillFileGenerator
		}
		if toolName == "" {
			toolName = "generate_file"
		}
		trace := skills.SkillTrace{
			Kind:     "tool_call",
			Status:   "success",
			SkillID:  skillID,
			ToolName: toolName,
			Result:   artifact,
		}
		if fastPathBlockedByPendingPlanAction(trace, evidence) {
			continue
		}
		if answer, ok := generatedArtifactFastPathAnswer(artifact, skillID, toolName); ok {
			return answer, true
		}
	}
	return "", false
}

func latestClientActionFastPathAnswerFromEvidence(evidence map[string]interface{}) (string, bool) {
	if len(evidenceMapFromAny(evidence["operation_plan"])) == 0 {
		return "", false
	}
	if fastPathAgentCreateStillMissing(evidence) {
		return "", false
	}
	for _, action := range fastPathClientActionCandidates(evidence) {
		trace, answer, ok := fastPathTraceAndAnswerFromClientAction(action)
		if !ok {
			continue
		}
		if fastPathBlockedByPendingPlanAction(trace, evidence) {
			continue
		}
		return answer, true
	}
	return "", false
}

func fastPathAgentCreateStillMissing(evidence map[string]interface{}) bool {
	progress := evidenceMapFromAny(evidence["agent_create_progress"])
	if len(progress) == 0 {
		ledger := evidenceMapFromAny(evidence["execution_ledger"])
		progress = evidenceMapFromAny(ledger["agent_create_progress"])
	}
	if len(progress) == 0 {
		return false
	}
	if status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(progress["status"]))); status == "completed" || status == "complete" || status == "done" {
		return false
	}
	if len(mapSliceFromAny(progress["missing_targets"])) > 0 {
		return true
	}
	if missing, ok := intFromAny(progress["missing_count"]); ok && missing > 0 {
		return true
	}
	requested, requestedOK := intFromAny(progress["requested_count"])
	completed, completedOK := intFromAny(progress["completed_count"])
	if !requestedOK || !completedOK {
		return false
	}
	return requested > 0 && completed < requested
}

func fastPathLatestToolResultCandidates(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	candidates := make([]map[string]interface{}, 0, 4)
	appendLatest := func(source map[string]interface{}) {
		if latest := evidenceMapFromAny(source["latest_tool_result"]); len(latest) > 0 {
			candidates = append(candidates, latest)
		}
	}
	appendPlanToolResult := func(source map[string]interface{}) {
		plan := evidenceMapFromAny(source["operation_plan"])
		if result := evidenceMapFromAny(plan["tool_result"]); len(result) > 0 {
			candidates = append(candidates, result)
		}
	}
	appendToolResults := func(source map[string]interface{}) {
		for _, result := range mapSliceFromAny(source["tool_results"]) {
			if len(result) > 0 {
				candidates = append(candidates, result)
			}
		}
	}
	appendPlanToolResult(evidence)
	appendLatest(evidenceMapFromAny(evidence["operation_result_summary"]))
	executionSummary := evidenceMapFromAny(evidence["execution_summary"])
	appendPlanToolResult(executionSummary)
	appendLatest(evidenceMapFromAny(executionSummary["operation_result_summary"]))
	appendToolResults(executionSummary)
	executionLedger := evidenceMapFromAny(evidence["execution_ledger"])
	appendPlanToolResult(executionLedger)
	appendLatest(evidenceMapFromAny(executionLedger["operation_result_summary"]))
	ledgerSummary := evidenceMapFromAny(executionLedger["summary"])
	appendPlanToolResult(ledgerSummary)
	appendToolResults(ledgerSummary)
	return candidates
}

func fastPathGeneratedArtifactCandidates(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	candidates := make([]map[string]interface{}, 0, 8)
	appendFiles := func(source map[string]interface{}) {
		files := mapSliceFromAny(source["generated_files"])
		for i := len(files) - 1; i >= 0; i-- {
			if len(files[i]) > 0 {
				candidates = append(candidates, files[i])
			}
		}
	}
	appendFiles(evidence)
	operationSummary := evidenceMapFromAny(evidence["operation_result_summary"])
	appendFiles(operationSummary)
	executionSummary := evidenceMapFromAny(evidence["execution_summary"])
	appendFiles(executionSummary)
	appendFiles(evidenceMapFromAny(executionSummary["operation_result_summary"]))
	executionLedger := evidenceMapFromAny(evidence["execution_ledger"])
	appendFiles(executionLedger)
	appendFiles(evidenceMapFromAny(executionLedger["summary"]))
	appendFiles(evidenceMapFromAny(executionLedger["operation_result_summary"]))
	return dedupeFastPathMaps(candidates, "tool_file_id", "file_id", "filename")
}

func fastPathClientActionCandidates(evidence map[string]interface{}) []map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	candidates := make([]map[string]interface{}, 0, 8)
	appendLatest := func(source map[string]interface{}) {
		if latest := evidenceMapFromAny(source["latest_client_action"]); len(latest) > 0 {
			candidates = append(candidates, latest)
		}
	}
	appendActions := func(source map[string]interface{}) {
		actions := mapSliceFromAny(source["client_actions"])
		for i := len(actions) - 1; i >= 0; i-- {
			if len(actions[i]) > 0 {
				candidates = append(candidates, actions[i])
			}
		}
	}
	operationSummary := evidenceMapFromAny(evidence["operation_result_summary"])
	appendLatest(operationSummary)
	appendActions(evidence)
	executionSummary := evidenceMapFromAny(evidence["execution_summary"])
	appendLatest(evidenceMapFromAny(executionSummary["operation_result_summary"]))
	appendActions(executionSummary)
	executionLedger := evidenceMapFromAny(evidence["execution_ledger"])
	appendLatest(evidenceMapFromAny(executionLedger["operation_result_summary"]))
	appendActions(executionLedger)
	appendActions(evidenceMapFromAny(executionLedger["summary"]))
	return dedupeFastPathMaps(candidates, "action_id", "runtime_id", "href", "loaded_href", "observed_path")
}

func fastPathTraceFromToolResult(result map[string]interface{}) (skills.SkillTrace, bool) {
	if len(result) == 0 {
		return skills.SkillTrace{}, false
	}
	skillID := strings.TrimSpace(firstNonEmptyString(result["skill_id"], result["skillID"]))
	toolName := strings.TrimSpace(firstNonEmptyString(result["tool_name"], result["toolName"]))
	status := strings.TrimSpace(firstNonEmptyString(result["status"], result["latest_tool_status"]))
	if skillID == "" || toolName == "" || status == "" {
		return skills.SkillTrace{}, false
	}
	payload := copyFastPathResultMap(evidenceMapFromAny(result["result_summary"]))
	if len(payload) == 0 {
		payload = copyFastPathResultMap(result)
	} else {
		mergeFastPathBatchEvidence(payload, result)
	}
	if _, ok := payload["status"]; !ok {
		payload["status"] = status
	}
	return skills.SkillTrace{
		Kind:     strings.TrimSpace(firstNonEmptyString(result["kind"], "tool_call")),
		Status:   status,
		SkillID:  skillID,
		ToolName: toolName,
		Result:   payload,
	}, true
}

func mergeFastPathBatchEvidence(target map[string]interface{}, source map[string]interface{}) {
	if len(target) == 0 || len(source) == 0 {
		return
	}
	for _, key := range []string{
		"operation_group",
		"item_results",
		"operation_type",
		"operation",
		"target_count",
		"deleted_count",
		"success_count",
		"failed_count",
	} {
		if _, exists := target[key]; exists {
			continue
		}
		if value, ok := source[key]; ok && value != nil {
			target[key] = value
		}
	}
}

func fastPathTraceAndAnswerFromClientAction(action map[string]interface{}) (skills.SkillTrace, string, bool) {
	if len(action) == 0 {
		return skills.SkillTrace{}, "", false
	}
	status := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action["status"], action["latest_client_action_status"])))
	if status != "succeeded" && status != "success" && status != "completed" {
		return skills.SkillTrace{}, "", false
	}
	result := evidenceMapFromAny(action["result"])
	skillID := strings.TrimSpace(firstNonEmptyString(action["skill_id"], action["skillID"]))
	toolName := strings.TrimSpace(firstNonEmptyString(action["tool_name"], action["toolName"]))
	actionType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action["action_type"], action["type"], action["event_type"])))
	if !strings.EqualFold(skillID, skills.SkillConsoleNavigator) &&
		!strings.EqualFold(toolName, "navigate") &&
		actionType != "route_navigation" {
		return skills.SkillTrace{}, "", false
	}
	href := strings.TrimSpace(firstNonEmptyString(
		action["href"],
		action["route"],
		action["target_page"],
		action["loaded_href"],
		action["observed_path"],
		result["href"],
		result["route"],
		result["target_page"],
		result["loaded_href"],
		result["observed_path"],
	))
	if href == "" {
		return skills.SkillTrace{}, "", false
	}
	label := strings.TrimSpace(firstNonEmptyString(action["label"], action["title"], result["label"], result["title"]))
	if label == "" {
		label = href
	}
	return skills.SkillTrace{
		Kind:     "client_action",
		Status:   "success",
		SkillID:  skills.SkillConsoleNavigator,
		ToolName: "navigate",
		Result: map[string]interface{}{
			"href":   href,
			"label":  label,
			"status": status,
		},
	}, fmt.Sprintf("\u5df2\u6253\u5f00\u300c%s\u300d\u9875\u9762\u3002", label), true
}

func generatedArtifactFastPathAnswer(result map[string]interface{}, skillID string, toolName string) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "" && status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	target := strings.ToLower(strings.TrimSpace(stringFromAny(result["target"])))
	if target == "managed_file" || target == "file_management" {
		return "", false
	}
	fileID := strings.TrimSpace(firstNonEmptyString(result["tool_file_id"], result["file_id"], result["id"], result["source_file_id"]))
	if fileID == "" &&
		strings.TrimSpace(firstNonEmptyString(result["download_url"], result["url"])) == "" {
		return "", false
	}
	filename := strings.TrimSpace(firstNonEmptyString(result["filename"], result["name"], result["file_name"], result["title"]))
	if filename == "" {
		return "", false
	}
	if strings.EqualFold(strings.TrimSpace(skillID), skills.SkillChartGenerator) ||
		strings.EqualFold(strings.TrimSpace(toolName), "generate_chart") {
		return fmt.Sprintf("\u56fe\u8868\u6587\u4ef6\u300c%s\u300d\u5df2\u751f\u6210\u3002", filename), true
	}
	return fmt.Sprintf("\u6587\u4ef6\u300c%s\u300d\u5df2\u751f\u6210\u3002", filename), true
}

func copyFastPathResultMap(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		switch key {
		case "kind", "skill_id", "skillID", "tool_name", "toolName", "invocation_id":
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func fastPathTraceWithToolResult(trace skills.SkillTrace, toolResult map[string]interface{}) skills.SkillTrace {
	if len(toolResult) == 0 {
		return trace
	}
	trace.Result = copyStringAnyMap(toolResult)
	return trace
}

func dedupeFastPathMaps(items []map[string]interface{}, keyFields ...string) []map[string]interface{} {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		keyParts := make([]string, 0, len(keyFields))
		for _, field := range keyFields {
			if value := strings.TrimSpace(firstNonEmptyString(item[field])); value != "" {
				keyParts = append(keyParts, field+"="+value)
			}
		}
		key := strings.Join(keyParts, "|")
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, item)
	}
	return out
}

func agentDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	effect := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["effect"], result["operation"])))
	if effect != "" && effect != "deleted" && effect != "delete" && effect != "agent.delete" {
		return "", false
	}
	name := agentDeleteItemName(result)
	if name == "" {
		name = "指定智能体"
	}
	return "已删除智能体：" + name + "。", true
}

func agentBatchDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "partial_failed" && status != "failed" {
		return "", false
	}
	group := payloadMap(result, "operation_group")
	if group == nil {
		group = map[string]interface{}{}
	}
	operation := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["operation_type"], group["operation"])))
	if operation != "" && operation != "agent.delete.batch" && operation != "agent.delete" {
		return "", false
	}

	items := mapSliceFromAny(firstNonEmptyValue(group["item_results"], result["item_results"]))
	targetCount := firstPositiveInt(result["target_count"], group["target_count"], len(items))
	if targetCount <= 0 {
		return "", false
	}
	deletedCount := firstNonNegativeInt(result["deleted_count"], group["success_count"], countAgentDeleteItems(items, "succeeded"))
	failedCount := firstNonNegativeInt(result["failed_count"], group["failed_count"], countAgentDeleteItems(items, "failed"))
	if deletedCount+failedCount == 0 && status == "completed" {
		deletedCount = targetCount
	}

	successNames := agentDeleteItemNames(items, "succeeded")
	failedItems := agentDeleteFailedItems(items)

	switch {
	case failedCount == 0:
		return fmt.Sprintf("已完成批量删除：成功删除 %d 个智能体%s。", deletedCount, formatNameList(successNames)), true
	case deletedCount == 0:
		detail := formatFailedItems(failedItems)
		if detail != "" {
			detail = "：" + detail
		}
		return fmt.Sprintf("批量删除未完成：%d 个智能体删除失败%s。", failedCount, detail), true
	default:
		successDetail := formatNameList(successNames)
		failedDetail := formatFailedItems(failedItems)
		if failedDetail != "" {
			failedDetail = "：" + failedDetail
		}
		return fmt.Sprintf("批量删除已结束：成功删除 %d 个智能体%s，%d 个删除失败%s。", deletedCount, successDetail, failedCount, failedDetail), true
	}
}

func agentConfigUpdateFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}

	agentName := agentConfigResultAgentName(result)
	details := agentConfigUpdateDetails(result)
	if len(details) == 0 {
		return "", false
	}

	target := "该智能体"
	if agentName != "" {
		target = "智能体「" + agentName + "」"
	}
	return fmt.Sprintf("已更新%s配置：%s。", target, strings.Join(details, "；")), true
}

func agentConfigResultAgentName(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	if name := strings.TrimSpace(firstNonEmptyString(result["agent_name"], result["name"])); name != "" {
		return name
	}
	agent := payloadMap(result, "agent")
	return strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"]))
}

func agentIdentityUpdateFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	effect := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["effect"], result["operation"])))
	if effect != "" && effect != "updated" && effect != "update" && effect != "agent.update" {
		return "", false
	}
	fields := agentIdentityUpdatedFieldLabels(sanitizedStringListArgumentValue(result["updated_fields"]))
	if len(fields) == 0 {
		return "", false
	}
	target := "该智能体"
	if agentName := agentConfigResultAgentName(result); agentName != "" {
		target = "智能体「" + agentName + "」"
	}
	return fmt.Sprintf("已更新%s的%s。", target, strings.Join(fields, "、")), true
}

func agentIdentityUpdatedFieldLabels(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	labels := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		label := agentIdentityFieldLabel(field)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	return labels
}

func agentIdentityFieldLabel(field string) string {
	switch strings.TrimSpace(field) {
	case "name":
		return "名称"
	case "description":
		return "描述"
	case "icon", "icon_type", "icon_background":
		return "图标"
	default:
		return ""
	}
}

func agentConfigUpdateDetails(result map[string]interface{}) []string {
	seenFields := map[string]struct{}{}
	details := make([]string, 0)
	if detail := agentConfigModelUpdateDetail(result); detail != "" {
		details = append(details, detail)
		seenFields["model"] = struct{}{}
		seenFields["model_provider"] = struct{}{}
	}
	for _, change := range agentConfigChangeMaps(result) {
		if field := strings.TrimSpace(stringFromAny(change["field"])); field != "" {
			seenFields[field] = struct{}{}
		}
		if detail := formatAgentConfigBindingChange(change); detail != "" {
			details = append(details, detail)
		}
	}
	for _, field := range sanitizedStringListArgumentValue(result["updated_fields"]) {
		if _, ok := seenFields[strings.TrimSpace(field)]; ok {
			continue
		}
		if agentConfigFieldCoveredByBindingChange(field, seenFields) {
			continue
		}
		if agentConfigBindingFieldRequiresChangeEvidence(field) {
			continue
		}
		if detail := agentConfigFieldUpdateDetail(field, result); detail != "" {
			details = append(details, detail)
			continue
		}
		if label := agentConfigFieldLabel(field); label != "" {
			details = append(details, "更新"+label)
		}
	}
	for _, detail := range agentConfigSatisfiedOnlyDetails(result, seenFields) {
		details = append(details, detail)
	}
	return dedupeStrings(details)
}

func agentConfigSatisfiedOnlyDetails(result map[string]interface{}, seenFields map[string]struct{}) []string {
	updatedSet := map[string]struct{}{}
	for _, field := range sanitizedStringListArgumentValue(result["updated_fields"]) {
		field = strings.TrimSpace(field)
		if field != "" {
			updatedSet[field] = struct{}{}
		}
	}
	details := []string{}
	for _, field := range sanitizedStringListArgumentValue(result["satisfied_fields"]) {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := updatedSet[field]; ok {
			continue
		}
		if _, ok := seenFields[field]; ok {
			continue
		}
		if agentConfigFieldCoveredByBindingChange(field, seenFields) {
			continue
		}
		if agentConfigBindingFieldRequiresChangeEvidence(field) {
			continue
		}
		if detail := agentConfigSatisfiedFieldDetail(field, result); detail != "" {
			details = append(details, detail)
		}
	}
	return details
}

func agentConfigSatisfiedFieldDetail(field string, result map[string]interface{}) string {
	detail := agentConfigFieldUpdateDetail(field, result)
	if detail == "" {
		if label := agentConfigFieldLabel(field); label != "" {
			return label + "\u5df2\u6ee1\u8db3"
		}
		return ""
	}
	label, value, ok := strings.Cut(detail, "\uff1a")
	if !ok || strings.TrimSpace(value) == "" {
		return detail + "\u5df2\u6ee1\u8db3"
	}
	return label + "\u5df2\u6ee1\u8db3\uff1a" + value
}

func agentConfigFieldUpdateDetail(field string, result map[string]interface{}) string {
	field = strings.TrimSpace(field)
	if field == "" || len(result) == 0 {
		return ""
	}
	config := payloadMap(result, "config")
	switch field {
	case "system_prompt":
		prompt := fastPathTrimText(strings.TrimSpace(firstNonEmptyString(result["system_prompt"], config["system_prompt"])), 80)
		if prompt == "" {
			return ""
		}
		return "\u7cfb\u7edf\u63d0\u793a\u8bcd\uff1a" + prompt
	case "agent_memory_enabled":
		return "\u8bb0\u5fc6\uff1a" + fastPathEnabledLabel(boolFromAny(firstNonEmptyValue(result["agent_memory_enabled"], config["agent_memory_enabled"], config["use_memory"])))
	case "file_upload_enabled":
		return "\u6587\u4ef6\u4e0a\u4f20\uff1a" + fastPathEnabledLabel(boolFromAny(firstNonEmptyValue(result["file_upload_enabled"], config["file_upload_enabled"])))
	case "home_title":
		title := strings.TrimSpace(firstNonEmptyString(result["home_title"], config["home_title"]))
		if title == "" {
			return ""
		}
		return "\u9996\u9875\u6807\u9898\uff1a" + title
	case "input_placeholder":
		placeholder := strings.TrimSpace(firstNonEmptyString(result["input_placeholder"], config["input_placeholder"]))
		if placeholder == "" {
			return ""
		}
		return "\u8f93\u5165\u6846\u5360\u4f4d\u6587\u6848\uff1a" + placeholder
	case "theme_color":
		theme := strings.TrimSpace(firstNonEmptyString(result["theme_color"], config["theme_color"]))
		if theme == "" {
			return ""
		}
		return "\u4e3b\u9898\u8272\uff1a" + theme
	case "suggested_questions":
		questions := sanitizedStringListArgumentValue(firstNonEmptyValue(result["suggested_questions"], config["suggested_questions"]))
		if len(questions) == 0 {
			return ""
		}
		visible := questions
		if len(visible) > 3 {
			visible = visible[:3]
		}
		return fmt.Sprintf("\u5f00\u573a\u95ee\u9898\uff1a%d \u4e2a\uff08%s\uff09", len(questions), strings.Join(visible, "\u3001"))
	default:
		return ""
	}
}

func agentConfigModelUpdateDetail(result map[string]interface{}) string {
	fields := sanitizedStringListArgumentValue(result["updated_fields"])
	if !agentConfigFieldsIncludeModel(fields) {
		return ""
	}
	config := payloadMap(result, "config")
	provider := strings.TrimSpace(firstNonEmptyString(result["model_provider"], config["model_provider"], config["provider"]))
	model := strings.TrimSpace(firstNonEmptyString(result["model"], config["model"], config["model_name"]))
	switch {
	case provider != "" && model != "":
		return "\u6a21\u578b\uff1a" + provider + "/" + model
	case model != "":
		return "\u6a21\u578b\uff1a" + model
	case provider != "":
		return "\u6a21\u578b\u4f9b\u5e94\u5546\uff1a" + provider
	default:
		return ""
	}
}

func agentConfigFieldsIncludeModel(fields []string) bool {
	for _, field := range fields {
		switch strings.TrimSpace(field) {
		case "model", "model_provider":
			return true
		}
	}
	return false
}

func fileManagementSaveFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	target := strings.ToLower(strings.TrimSpace(stringFromAny(result["target"])))
	if target != "" && target != "managed_file" && target != "file_management" {
		return "", false
	}
	fileID := strings.TrimSpace(firstNonEmptyString(result["file_id"], result["upload_file_id"], result["managed_file_id"], result["id"]))
	if fileID == "" {
		return "", false
	}
	filename := strings.TrimSpace(firstNonEmptyString(result["filename"], result["name"], result["file_name"]))
	if filename == "" {
		return "", false
	}
	return fmt.Sprintf("文件「%s」已保存到文件管理。", filename), true
}

func fileManagementDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "" && status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	if deleted := firstNonNegativeInt(result["deleted_count"]); deleted <= 0 && !boolFromAny(result["deleted"]) {
		return "", false
	}
	filename := strings.TrimSpace(firstNonEmptyString(
		result["filename"],
		result["name"],
		result["file_name"],
		result["file_title"],
	))
	if filename == "" {
		filename = "指定文件"
	}
	return fmt.Sprintf("已删除文件「%s」。", filename), true
}

func agentConfigChangeMaps(result map[string]interface{}) []map[string]interface{} {
	changes := mapSliceFromAny(result["binding_changes"])
	if len(changes) == 0 {
		changes = mapSliceFromAny(result["config_changes"])
	}
	if len(changes) == 0 {
		changes = mapSliceFromAny(result["binding_final_states"])
	}
	return changes
}

func formatAgentConfigBindingChange(change map[string]interface{}) string {
	if len(change) == 0 {
		return ""
	}
	kind := agentConfigBindingKindLabel(firstNonEmptyString(change["binding_kind"], change["field"]))
	action := strings.ToLower(strings.TrimSpace(stringFromAny(change["change_action"])))
	if kind == "" || action == "" {
		return ""
	}
	switch action {
	case "bind":
		return formatAgentConfigCountChange("绑定", kind, firstPositiveInt(change["resource_count"], change["added_resource_count"]), sanitizedStringListArgumentValue(firstNonEmptyValue(change["resource_names"], change["added_resource_names"])))
	case "unbind":
		return appendAgentConfigFinalResourceSummary(
			formatAgentConfigCountChange("解绑", kind, firstPositiveInt(change["resource_count"], change["removed_resource_count"]), sanitizedStringListArgumentValue(firstNonEmptyValue(change["resource_names"], change["removed_resource_names"]))),
			kind,
			change,
		)
	case "replace":
		added := firstNonNegativeInt(change["added_resource_count"])
		removed := firstNonNegativeInt(change["removed_resource_count"])
		if added == 0 && removed == 0 {
			return appendAgentConfigFinalResourceSummary(
				formatAgentConfigCountChange("替换", kind, firstPositiveInt(change["resource_count"]), sanitizedStringListArgumentValue(change["resource_names"])),
				kind,
				change,
			)
		}
		parts := make([]string, 0, 2)
		if added > 0 {
			parts = append(parts, fmt.Sprintf("新增 %d 个", added))
		}
		if removed > 0 {
			parts = append(parts, fmt.Sprintf("移除 %d 个", removed))
		}
		return appendAgentConfigFinalResourceSummary("替换"+kind+"（"+strings.Join(parts, "；")+"）", kind, change)
	case "update":
		if count := firstNonNegativeInt(change["final_resource_count"]); count > 0 {
			return fmt.Sprintf("更新%s（当前 %d 个）", kind, count)
		}
		return "更新" + kind
	case "satisfied":
		if finalSummary := formatAgentConfigFinalResourceSummary(kind, change); finalSummary != "" {
			return kind + "已满足：" + finalSummary
		}
		return kind + "已满足"
	default:
		return ""
	}
}

func appendAgentConfigFinalResourceSummary(summary string, kind string, change map[string]interface{}) string {
	finalSummary := formatAgentConfigFinalResourceSummary(kind, change)
	if finalSummary == "" {
		return summary
	}
	if summary == "" {
		return finalSummary
	}
	return summary + "；" + finalSummary
}

func formatAgentConfigFinalResourceSummary(kind string, change map[string]interface{}) string {
	if kind == "" || len(change) == 0 {
		return ""
	}
	names := sanitizedStringListArgumentValue(change["final_resource_names"])
	if len(names) > 0 {
		if count := firstPositiveInt(change["final_resource_count"]); count > 0 {
			return fmt.Sprintf("当前保留 %d 个%s%s", count, kind, formatNameList(names))
		}
		return "当前保留" + kind + formatNameList(names)
	}
	if value, ok := change["final_resource_count"]; ok {
		if count, ok := intFromAny(value); ok {
			if count > 0 {
				return fmt.Sprintf("当前保留 %d 个%s", count, kind)
			}
			return "当前未绑定" + kind
		}
	}
	return ""
}

func formatAgentConfigCountChange(action string, kind string, count int, names []string) string {
	if action == "" || kind == "" {
		return ""
	}
	if len(names) > 0 {
		if count > 0 {
			return fmt.Sprintf("%s %d 个%s%s", action, count, kind, formatNameList(names))
		}
		return action + kind + formatNameList(names)
	}
	if count > 0 {
		return fmt.Sprintf("%s %d 个%s", action, count, kind)
	}
	return ""
}

func agentConfigBindingKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agent_skill", "enabled_skill_ids":
		return "技能"
	case "knowledge_base", "knowledge_dataset_ids", "dataset_ids":
		return "知识库"
	case "database_table", "database_bindings":
		return "数据表"
	case "workflow", "workflow_bindings":
		return "工作流"
	case "multiple":
		return "资源绑定"
	default:
		return ""
	}
}

func agentConfigFieldCoveredByBindingChange(field string, seen map[string]struct{}) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return true
	}
	if _, ok := seen[field]; ok {
		return true
	}
	switch field {
	case "enabled_skill_ids":
		_, ok := seen["agent_skill"]
		return ok
	case "knowledge_dataset_ids", "dataset_ids":
		_, ok := seen["knowledge_base"]
		return ok
	case "database_bindings":
		_, ok := seen["database_table"]
		return ok
	case "workflow_bindings":
		_, ok := seen["workflow"]
		return ok
	default:
		return false
	}
}

func agentConfigBindingFieldRequiresChangeEvidence(field string) bool {
	switch strings.TrimSpace(field) {
	case "enabled_skill_ids", "knowledge_dataset_ids", "dataset_ids", "database_bindings", "workflow_bindings":
		return true
	default:
		return false
	}
}

func agentConfigFieldLabel(field string) string {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return "系统提示词"
	case "model", "model_provider":
		return "模型"
	case "model_parameters":
		return "模型参数"
	case "agent_memory_enabled":
		return "记忆开关"
	case "file_upload_enabled":
		return "文件上传开关"
	case "home_title":
		return "首页标题"
	case "input_placeholder":
		return "输入框占位文案"
	case "theme_color":
		return "主题色"
	case "suggested_questions":
		return "建议问题"
	case "knowledge_retrieval_config":
		return "知识库检索配置"
	case "enabled_skill_ids":
		return "技能"
	case "knowledge_dataset_ids", "dataset_ids":
		return "知识库"
	case "database_bindings":
		return "数据表绑定"
	case "workflow_bindings":
		return "工作流绑定"
	case "agent_memory_slots":
		return "记忆槽位"
	default:
		return ""
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func mapSliceFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, mapped)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case []map[string]interface{}:
			if len(typed) > 0 {
				return typed
			}
		case []interface{}:
			if len(typed) > 0 {
				return typed
			}
		default:
			if strings.TrimSpace(fmt.Sprint(value)) != "" {
				return value
			}
		}
	}
	return nil
}

func firstPositiveInt(values ...interface{}) int {
	for _, value := range values {
		if intValue, ok := intFromAny(value); ok && intValue > 0 {
			return intValue
		}
	}
	return 0
}

func firstNonNegativeInt(values ...interface{}) int {
	for _, value := range values {
		if intValue, ok := intFromAny(value); ok && intValue >= 0 {
			return intValue
		}
	}
	return 0
}

func boolFromAny(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "1", "y":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func intFromAny(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0, false
		}
		var out int
		if _, err := fmt.Sscanf(typed, "%d", &out); err != nil {
			return 0, false
		}
		return out, true
	default:
		return 0, false
	}
}

func countAgentDeleteItems(items []map[string]interface{}, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), status) {
			count++
		}
	}
	return count
}

func agentDeleteItemNames(items []map[string]interface{}, status string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if status != "" && !strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), status) {
			continue
		}
		if name := agentDeleteItemName(item); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func agentDeleteFailedItems(items []map[string]interface{}) []string {
	out := make([]string, 0)
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), "failed") {
			continue
		}
		name := agentDeleteItemName(item)
		if name == "" {
			index := 0
			if value, ok := intFromAny(item["index"]); ok {
				index = value + 1
			}
			if index > 0 {
				name = fmt.Sprintf("第 %d 个", index)
			} else {
				name = "某个智能体"
			}
		}
		if reason := strings.TrimSpace(stringFromAny(item["error"])); reason != "" {
			name += "（" + reason + "）"
		}
		out = append(out, name)
	}
	return out
}

func agentDeleteItemName(item map[string]interface{}) string {
	if len(item) == 0 {
		return ""
	}
	if name := strings.TrimSpace(firstNonEmptyString(item["agent_name"], item["name"])); name != "" {
		return name
	}
	agent := payloadMap(item, "agent")
	if name := strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"])); name != "" {
		return name
	}
	for _, source := range []interface{}{
		item["assets"],
		payloadMap(item, "governance")["assets"],
		payloadMap(item, "asset_operation_audit")["assets"],
	} {
		for _, asset := range mapSliceFromAny(source) {
			if name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["agent_name"], asset["title"])); name != "" {
				return name
			}
		}
	}
	return ""
}

func formatNameList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) > 5 {
		names = names[:5]
		return "（" + strings.Join(names, "、") + " 等）"
	}
	return "（" + strings.Join(names, "、") + "）"
}

func formatFailedItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > 5 {
		items = items[:5]
		return strings.Join(items, "、") + " 等"
	}
	return strings.Join(items, "、")
}
