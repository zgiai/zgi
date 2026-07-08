package service

import (
	"sort"
	"strings"
)

func agentManagementExplicitConfigAssignmentRequested(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || !agentManagementMutationVerbRequested(query) || !agentManagementConfigCapabilityMarkerRequested(query) {
		return false
	}
	return containsAnySubstring(query, []string{
		"set ",
		" to ",
		"update ",
		"change ",
		"modify ",
		"replace ",
		"switch ",
		"enable ",
		"disable ",
		"\u8bbe\u7f6e",
		"\u8bbe\u4e3a",
		"\u8bbe\u6210",
		"\u914d\u7f6e\u6a21\u578b",
		"\u6a21\u578b\u914d\u7f6e",
		"\u6a21\u578b\u914d\u7f6e\u4e3a",
		"\u6a21\u578b\u914d\u6210",
		"\u914d\u7f6e\u4e3a",
		"\u914d\u7f6e\u6210",
		"\u5199\u597d\u63d0\u793a\u8bcd",
		"\u7f16\u5199\u63d0\u793a\u8bcd",
		"\u63d0\u793a\u8bcd\u9700\u8981",
		"\u4e0a\u4f20\u6587\u4ef6",
		"\u6539\u4e3a",
		"\u6539\u6210",
		"\u66f4\u65b0",
		"\u66f4\u6539",
		"\u66ff\u6362",
		"\u5207\u6362",
		"\u542f\u7528",
		"\u7981\u7528",
	})
}

func agentManagementExplicitIdentityAssignmentRequested(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || !agentManagementMutationVerbRequested(query) {
		return false
	}
	return containsPositiveAgentManagementResourceMarker(query, []string{
		"rename", "name", "description", "icon",
		"\u6539\u540d", "\u540d\u79f0", "\u540d\u5b57", "\u63cf\u8ff0", "\u56fe\u6807",
	})
}

func agentManagementExplicitNoMutationRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if containsAnySubstring(text, []string{
		"read-only",
		"readonly",
		"read only",
		"only read",
		"only inspect",
		"only view",
		"only navigate",
		"just read",
		"just inspect",
		"just view",
		"just navigate",
		"do not modify",
		"don't modify",
		"dont modify",
		"do not change",
		"don't change",
		"dont change",
		"do not update",
		"don't update",
		"dont update",
		"do not edit",
		"don't edit",
		"dont edit",
		"without modifying",
		"without changing",
		"without updating",
		"leave unchanged",
		"keep unchanged",
		"do not request approval",
		"don't request approval",
		"dont request approval",
		"do not ask for approval",
		"don't ask for approval",
		"dont ask for approval",
		"without approval",
		"no approval",
	}) {
		return true
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(text)
	return containsAnySubstring(compact, []string{
		"\u53ea\u8bfb",
		"\u53ea\u8bfb\u53d6",
		"\u53ea\u67e5\u770b",
		"\u53ea\u770b",
		"\u53ea\u6253\u5f00",
		"\u53ea\u5bfc\u822a",
		"\u53ea\u505a\u5bfc\u822a",
		"\u53ea\u505a\u67e5\u770b",
		"\u53ea\u505a\u8bfb\u53d6",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u7f16\u8f91",
		"\u4e0d\u8981\u7f16\u8f91",
		"\u4e0d\u8c03\u6574",
		"\u4e0d\u8981\u8c03\u6574",
		"\u4e0d\u53d8\u66f4",
		"\u4e0d\u8981\u53d8\u66f4",
		"\u4e0d\u8981\u4fee\u6539\u914d\u7f6e",
		"\u4e0d\u8981\u4fee\u6539\u4efb\u4f55\u914d\u7f6e",
		"\u4e0d\u4fee\u6539\u914d\u7f6e",
		"\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u8981\u5ba1\u6279",
		"\u4e0d\u5ba1\u6279",
		"\u65e0\u9700\u5ba1\u6279",
		"\u65e0\u9700\u4fee\u6539",
		"\u4fdd\u6301\u4e0d\u53d8",
		"\u4fdd\u6301\u539f\u6837",
	})
}

func agentManagementPersonaUpdateRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" || !agentManagementMutationVerbRequested(text) {
		return false
	}
	if !containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"persona", "role", "writer", "writing", "novel", "creator", "creative",
		"\u4eba\u8bbe", "\u89d2\u8272", "\u5199\u4f5c", "\u521b\u4f5c", "\u5c0f\u8bf4", "\u4f5c\u5bb6", "\u4f5c\u8005",
		"\u521b\u4f5c\u667a\u80fd\u4f53", "\u5199\u4f5c\u667a\u80fd\u4f53",
	})
}

func agentManagementConfigOnlyCapabilityRequested(query string, field string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" ||
		agentManagementExplicitNoMutationRequested(text) ||
		agentBindingStateReadOnlyQuestionRequested(text) {
		return false
	}
	descriptor, ok := agentManagementConfigOnlyCapabilityDescriptorForField(field)
	if !ok || !containsPositiveAgentManagementResourceMarker(text, descriptor.Markers) {
		return false
	}
	if !agentManagementMutationVerbRequested(text) &&
		containsAnySubstring(text, []string{"?", "\uff1f", "\u5417", "whether"}) &&
		!containsAnySubstring(text, []string{"let", "allow", "make", "\u8ba9", "\u4f7f"}) {
		return false
	}
	return agentManagementMutationVerbRequested(text) ||
		agentManagementCapabilityEnablePhraseRequested(text)
}

func agentManagementCapabilityEnablePhraseRequested(text string) bool {
	return containsAnySubstring(strings.ToLower(strings.TrimSpace(text)), agentManagementCapabilityEnablePhrases())
}

func agentManagementCapabilityEnablePhrases() []string {
	return []string{
		"let this agent",
		"let the agent",
		"let agent",
		"let current agent",
		"allow this agent",
		"allow the agent",
		"allow agent",
		"allow current agent",
		"make this agent",
		"make the agent",
		"make agent",
		"make current agent",
		"current agent can",
		"current agent could",
		"current agent should be able to",
		"current agent able to",
		"agent can",
		"agent could",
		"agent should be able to",
		"agent able to",
		"agent supports",
		"agent support",
		"\u8ba9 agent",
		"\u8ba9agent",
		"\u8ba9\u5f53\u524d agent",
		"\u8ba9\u5f53\u524dagent",
		"\u8ba9\u8fd9\u4e2a\u667a\u80fd\u4f53",
		"\u8ba9\u667a\u80fd\u4f53",
		"\u8ba9\u5b83",
		"\u8ba9\u5176",
		"\u4f7f agent",
		"\u4f7fagent",
		"\u4f7f\u5f53\u524d agent",
		"\u4f7f\u5f53\u524dagent",
		"\u4f7f\u8fd9\u4e2a\u667a\u80fd\u4f53",
		"\u4f7f\u667a\u80fd\u4f53",
		"\u4f7f\u5b83",
		"\u4f7f\u5176",
		"\u5f53\u524d agent \u80fd\u591f",
		"\u5f53\u524dagent\u80fd\u591f",
		"agent \u80fd\u591f",
		"agent\u80fd\u591f",
		"agent\u80fd",
		"agent\u53ef\u4ee5",
		"\u667a\u80fd\u4f53\u80fd",
		"\u667a\u80fd\u4f53\u53ef\u4ee5",
		"\u5b83\u80fd\u591f",
		"\u5b83\u80fd",
		"\u5b83\u53ef\u4ee5",
		"\u5176\u80fd\u591f",
		"\u5176\u80fd",
		"\u5176\u53ef\u4ee5",
	}
}

func agentManagementBroadEditableConfigRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(query)))
	if text == "" || !agentManagementMutationVerbRequested(text) {
		return false
	}
	if agentManagementExplicitNoMutationRequested(text) {
		return false
	}
	if !containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"all editable",
		"all configurable",
		"everything editable",
		"everything configurable",
		"everything you can edit",
		"everything you can modify",
		"all you can edit",
		"all you can modify",
		"as much as possible",
		"\u6240\u6709\u4f60\u80fd\u4fee\u6539",
		"\u6240\u6709\u53ef\u4fee\u6539",
		"\u6240\u6709\u80fd\u4fee\u6539",
		"\u6240\u6709\u4f60\u80fd\u7f16\u8f91",
		"\u6240\u6709\u53ef\u7f16\u8f91",
		"\u6240\u6709\u80fd\u7f16\u8f91",
		"\u6240\u6709\u53ef\u914d\u7f6e",
		"\u5168\u90e8\u53ef\u4fee\u6539",
		"\u5168\u90e8\u80fd\u4fee\u6539",
		"\u5168\u90e8\u53ef\u7f16\u8f91",
		"\u5168\u90e8\u53ef\u914d\u7f6e",
		"\u5c3d\u53ef\u80fd\u8fdb\u884c\u64cd\u4f5c",
		"\u5c3d\u53ef\u80fd\u4fee\u6539",
		"\u5c3d\u53ef\u80fd\u7f16\u8f91",
		"\u80fd\u6539\u7684\u90fd\u6539",
		"\u80fd\u4fee\u6539\u7684\u90fd\u4fee\u6539",
		"\u80fd\u7f16\u8f91\u7684\u90fd\u7f16\u8f91",
		"\u6240\u6709\u4fee\u6539\u9879",
		"\u6240\u6709\u53ef\u64cd\u4f5c",
		"\u53ef\u64cd\u4f5c\u7684\u90fd",
	})
}

func agentManagementSkillCapabilityRequested(query string) bool {
	return agentManagementSkillCapabilityCandidateQuery(query) != ""
}

func agentManagementSkillCapabilityCandidateQuery(query string) string {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" {
		return ""
	}
	if agentManagementSkillBindingNoop(text) ||
		agentManagementExplicitNoMutationRequested(text) ||
		agentManagementCapabilityStatusQuestionRequested(text) ||
		agentBindingStateReadOnlyQuestionRequested(text) {
		return ""
	}
	if candidateQuery := agentSkillBackedCapabilityCandidateQueryForText(text); candidateQuery != "" {
		return candidateQuery
	}
	for _, marker := range agentManagementCapabilityEnablePhrases() {
		if value := agentManagementCapabilityTailAfterMarker(text, marker); value != "" {
			return value
		}
	}
	if containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) {
		for _, marker := range []string{
			"enable capability",
			"enable capabilities",
			"capable of",
			"\u542f\u7528",
			"\u652f\u6301",
			"\u5177\u5907",
		} {
			if value := agentManagementCapabilityTailAfterMarker(text, marker); value != "" {
				return value
			}
		}
	}
	return ""
}

func agentManagementCapabilityTailAfterMarker(text string, marker string) string {
	marker = strings.TrimSpace(strings.ToLower(marker))
	if marker == "" {
		return ""
	}
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	tail := strings.TrimSpace(text[idx+len(marker):])
	return agentManagementCleanCapabilityCandidateQuery(tail)
}

func agentManagementCleanCapabilityCandidateQuery(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, prefix := range []string{
		"to ",
		"be able to ",
		"able to ",
		"can ",
		"could ",
		"should ",
		"support ",
		"supports ",
		"with ",
		"\u80fd\u591f",
		"\u80fd",
		"\u53ef\u4ee5",
		"\u652f\u6301",
		"\u5177\u5907",
		"\u8fdb\u884c",
	} {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
		}
	}
	text = strings.Trim(text, " \t\r\n:：,，.。;；")
	if text == "" {
		return ""
	}
	for _, stop := range []string{
		"\u3002", "\n", "\r",
		" then ", " and then ", " and use ", " and set ", " and switch ", " and replace ",
		"\u7136\u540e", "\u63a5\u7740", "\u4e4b\u540e", "\u5e76\u4f7f\u7528", "\u5e76\u91c7\u7528", "\u5e76\u9009\u7528", "\u5e76\u8bbe\u7f6e", "\u5e76\u5207\u6362", "\u5e76\u66ff\u6362",
	} {
		if idx := strings.Index(text, stop); idx > 0 {
			text = strings.TrimSpace(text[:idx])
		}
	}
	text = agentManagementSkillCapabilityQueryWithoutConfigOnlyParts(text)
	text = strings.Trim(text, " \t\r\n:：,，.。;；")
	if text == "" || text == "agent" || text == "\u667a\u80fd\u4f53" {
		return ""
	}
	if candidateQuery := agentSkillBackedCapabilityCandidateQueryForText(text); candidateQuery != "" {
		return candidateQuery
	}
	return truncateRunes(text, 80)
}

func agentManagementSkillCapabilityQueryWithoutConfigOnlyParts(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	markers := append([]string(nil), agentManagementConfigOnlyCapabilityMarkers()...)
	sort.SliceStable(markers, func(i, j int) bool {
		return len(markers[i]) > len(markers[j])
	})
	for _, marker := range markers {
		text = strings.ReplaceAll(text, marker, "")
	}
	text = strings.Trim(text, " \t\r\n:：,，.。;；、/\\|&+")
	for {
		trimmed := strings.TrimSpace(text)
		for _, suffix := range []string{
			"and",
			"or",
			"with",
			"capability",
			"capabilities",
			"\u548c",
			"\u4e0e",
			"\u53ca",
			"\u4ee5\u53ca",
			"\u5e76",
			"\u5e76\u4e14",
			"\u80fd\u529b",
		} {
			trimmed = strings.TrimSuffix(trimmed, suffix)
			trimmed = strings.TrimSpace(trimmed)
		}
		for _, prefix := range []string{
			"and",
			"or",
			"with",
			"\u548c",
			"\u4e0e",
			"\u53ca",
			"\u4ee5\u53ca",
			"\u5e76",
			"\u5e76\u4e14",
		} {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			trimmed = strings.TrimSpace(trimmed)
		}
		trimmed = strings.Trim(trimmed, " \t\r\n:：,，.。;；、/\\|&+")
		if trimmed == text {
			return trimmed
		}
		text = trimmed
	}
}

func agentManagementSkillBindingNoop(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" || !containsAnySubstring(text, []string{"skill", "\u6280\u80fd"}) {
		return false
	}
	if agentBindingNoBindScopeCoversResource(text, []string{"skill", "\u6280\u80fd"}, append(agentBindingNoopScopeMarkers(), agentBindingPreserveScopeMarkers()...)) {
		return true
	}
	return containsAnySubstring(text, []string{
		"do not add or remove", "don't add or remove", "dont add or remove", "without adding or removing",
		"do not add skill", "don't add skill", "dont add skill", "do not remove skill", "don't remove skill", "dont remove skill",
		"do not bind skill", "don't bind skill", "dont bind skill", "do not unbind skill", "don't unbind skill", "dont unbind skill",
		"do not bind or unbind", "don't bind or unbind", "dont bind or unbind",
		"do not delete skill", "don't delete skill", "dont delete skill", "do not change skill", "don't change skill", "dont change skill",
		"do not touch skill", "don't touch skill", "dont touch skill",
		"leave skill unchanged", "keep skill unchanged", "preserve skill", "retain skill",
		"\u4e0d\u8981\u6dfb\u52a0\u6216\u5220\u9664", "\u4e0d\u6dfb\u52a0\u6216\u5220\u9664", "\u65e0\u9700\u6dfb\u52a0\u6216\u5220\u9664",
		"\u4e0d\u8981\u6dfb\u52a0 skill", "\u4e0d\u6dfb\u52a0 skill", "\u65e0\u9700\u6dfb\u52a0 skill", "\u4e0d\u8981\u5220\u9664 skill", "\u4e0d\u5220\u9664 skill",
		"\u4e0d\u8981\u7ed1\u5b9a skill", "\u4e0d\u7ed1\u5b9a skill", "\u4e0d\u8981\u89e3\u7ed1 skill", "\u4e0d\u89e3\u7ed1 skill", "\u4e0d\u8981\u7ed1\u5b9a\u6216\u89e3\u7ed1 skill",
		"\u4e0d\u8981\u6dfb\u52a0\u6280\u80fd", "\u4e0d\u6dfb\u52a0\u6280\u80fd", "\u65e0\u9700\u6dfb\u52a0\u6280\u80fd", "\u4e0d\u8981\u5220\u9664\u6280\u80fd", "\u4e0d\u5220\u9664\u6280\u80fd",
		"\u4e0d\u8981\u7ed1\u5b9a\u6280\u80fd", "\u4e0d\u7ed1\u5b9a\u6280\u80fd", "\u4e0d\u8981\u89e3\u7ed1\u6280\u80fd", "\u4e0d\u89e3\u7ed1\u6280\u80fd", "\u4e0d\u8981\u7ed1\u5b9a\u6216\u89e3\u7ed1\u6280\u80fd",
		"\u4e0d\u8981\u4fee\u6539 skill", "\u4e0d\u6539 skill", "\u4fdd\u6301 skill", "\u4fdd\u7559 skill", "\u4e0d\u8981\u4fee\u6539\u6280\u80fd", "\u4e0d\u6539\u6280\u80fd", "\u4fdd\u6301\u6280\u80fd", "\u4fdd\u7559\u6280\u80fd",
		"\u4e0d\u8981\u52a8 skill", "\u522b\u52a8 skill", "\u4e0d\u8981\u52a8\u6280\u80fd", "\u522b\u52a8\u6280\u80fd",
	})
}
