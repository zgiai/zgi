package service

import (
	"strings"
)

func agentManagementDeleteHasExplicitFollowupMutation(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	for _, phrase := range []string{
		"then edit", "then update", "then modify", "then create", "then add", "then bind", "then unbind", "then transform", "then convert",
		"and edit", "and update", "and modify", "and create", "and add", "and bind", "and unbind", "and transform", "and convert",
		"\u7136\u540e\u4fee\u6539", "\u7136\u540e\u7f16\u8f91", "\u7136\u540e\u66f4\u65b0", "\u7136\u540e\u521b\u5efa", "\u7136\u540e\u65b0\u5efa", "\u7136\u540e\u7ed1\u5b9a", "\u7136\u540e\u89e3\u7ed1", "\u7136\u540e\u6539\u9020",
		"\u518d\u4fee\u6539", "\u518d\u7f16\u8f91", "\u518d\u66f4\u65b0", "\u518d\u521b\u5efa", "\u518d\u65b0\u5efa", "\u518d\u7ed1\u5b9a", "\u518d\u89e3\u7ed1", "\u518d\u6539\u9020",
		"\u540c\u65f6\u4fee\u6539", "\u540c\u65f6\u7f16\u8f91", "\u540c\u65f6\u66f4\u65b0", "\u540c\u65f6\u521b\u5efa", "\u540c\u65f6\u65b0\u5efa", "\u540c\u65f6\u7ed1\u5b9a", "\u540c\u65f6\u89e3\u7ed1", "\u540c\u65f6\u6539\u9020",
		"\u5e76\u4fee\u6539", "\u5e76\u7f16\u8f91", "\u5e76\u66f4\u65b0", "\u5e76\u521b\u5efa", "\u5e76\u65b0\u5efa", "\u5e76\u7ed1\u5b9a", "\u5e76\u89e3\u7ed1", "\u5e76\u6539\u9020",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	for _, marker := range []string{"after deleting", "after delete", "after deletion", "\u5220\u9664\u540e", "\u5220\u5b8c\u540e"} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		tail := strings.TrimSpace(text[idx+len(marker):])
		if agentManagementFollowupMutationRequested(tail) {
			return true
		}
	}
	return false
}

func agentManagementFollowupMutationRequested(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return agentManagementCreateRequested(text) ||
		agentManagementMutationVerbRequested(text) ||
		agentBindingMutationRequested(text)
}

func agentManagementCreateMentionIsDeleteTargetReference(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripQuotedIntentPayloads(query)))
	if text == "" {
		return false
	}
	if !agentManagementDeleteRequested(text) || !agentManagementCreateMentionHasExistingReference(text) {
		return false
	}
	if agentManagementExplicitCreateCommandRequested(text) {
		return false
	}
	return true
}

func agentManagementIdentityUpdateRequested(query string) bool {
	query = agentManagementSecondaryIntentQuery(query)
	query = stripAgentManagementFinalAnswerInstruction(query)
	return strings.Contains(query, "update_agent_identity") ||
		agentManagementBroadEditableConfigRequested(query) ||
		agentManagementPersonaUpdateRequested(query) ||
		(agentManagementMutationVerbRequested(query) &&
			containsPositiveAgentManagementResourceMarker(query, []string{
				"rename", "agent name", "name to", "name as", "description", "icon",
				"\u6539\u540d", "\u540d\u79f0", "\u540d\u5b57", "\u63cf\u8ff0", "\u56fe\u6807",
			}))
}

func agentManagementSkillBindingRequested(query string) bool {
	query = agentManagementSecondaryIntentQuery(query)
	return strings.Contains(query, "list_agent_skill_candidates") ||
		agentManagementSkillBindingRequestedByIntent(query)
}

func agentManagementSkillBindingRequestedByIntent(query string) bool {
	if agentManagementSkillBindingNoop(query) {
		return false
	}
	if agentManagementSkillCapabilityRequested(query) {
		return true
	}
	if !agentBindingStateReadOnlyQuestionRequested(query) &&
		agentBindingSoftMutationRequested(query) &&
		containsAnySubstring(query, []string{"skill", "\u6280\u80fd"}) {
		return true
	}
	return (agentBindingMutationRequested(query) && containsAnySubstring(query, []string{"skill", "\u6280\u80fd"})) ||
		containsAnySubstring(query, []string{
			"agent skill", "skill binding", "enable skill", "disable skill", "add skill", "remove skill", "delete skill", "bind skill",
			"\u6dfb\u52a0 skill", "\u6dfb\u52a0\u8fd9\u4e2a skill", "\u65b0\u589e skill", "\u542f\u7528 skill", "\u7981\u7528 skill", "\u505c\u7528 skill", "\u7ed1\u5b9a skill", "\u79fb\u9664 skill", "\u5220\u9664 skill",
			"\u6dfb\u52a0\u6280\u80fd", "\u65b0\u589e\u6280\u80fd", "\u542f\u7528\u6280\u80fd", "\u7981\u7528\u6280\u80fd", "\u505c\u7528\u6280\u80fd", "\u7ed1\u5b9a\u6280\u80fd", "\u79fb\u9664\u6280\u80fd", "\u5220\u9664\u6280\u80fd",
		})
}

func agentBindingMutationRequested(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	if agentBindingExplicitCandidateSelectionMutationRequested(query) {
		return true
	}
	for _, marker := range []string{
		"\u89e3\u7ed1",
		"\u7981\u7528",
		"\u505c\u7528",
		"\u53d6\u6d88\u5173\u8054",
		"\u66ff\u6362",
		"\u6dfb\u52a0",
		"\u65b0\u589e",
		"\u5220\u9664",
		"\u79fb\u9664",
		"\u6e05\u7a7a",
		"unbind",
		"disable",
		"detach",
		"delete",
		"replace",
		"add",
		"remove",
		"clear",
	} {
		if containsUnnegatedAgentManagementMutationMarker(query, marker) {
			return true
		}
	}
	if agentBindingReadOnlyRequested(query) {
		return false
	}
	return agentBindingSoftMutationRequested(query)
}

func agentManagementBatchDeleteRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if !agentManagementDeleteRequested(text) {
		return false
	}
	text = stripQuotedIntentPayloads(text)
	return strings.Contains(text, "delete_agents") ||
		containsAnySubstring(text, []string{
			"delete agents",
			"remove agents",
			"batch",
			"multiple",
			"first ",
			"top ",
			"these agents",
			"selected agents",
			"\u6279\u91cf",
			"\u591a\u4e2a",
			"\u4e24\u4e2a",
			"\u4e09\u4e2a",
			"\u56db\u4e2a",
			"\u4e94\u4e2a",
			"\u51e0\u4e2a",
			"\u8fd9\u4e9b",
			"\u8fd9\u4e24\u4e2a",
			"\u8fd9\u4e09\u4e2a",
			"\u8fd9\u56db\u4e2a",
			"\u8fd9\u51e0\u4e2a",
			"\u5168\u90e8",
			"\u6240\u6709",
			"\u9009\u4e2d",
		})
}
