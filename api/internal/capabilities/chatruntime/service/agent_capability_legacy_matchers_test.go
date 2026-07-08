package service

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func agentManagementConfigFieldSemanticMarkers(field string) []string {
	descriptor, ok := agentManagementConfigFieldDescriptorForAlias(field)
	if !ok {
		return nil
	}
	return appendUniqueStrings(append([]string(nil), descriptor.explicitMarkers...), descriptor.semanticMarkers...)
}

func agentManagementConfigFieldSemanticMarkerRequested(text string, field string) bool {
	text = strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(text))))
	if text == "" {
		return false
	}
	return containsPositiveAgentManagementResourceMarker(text, agentManagementConfigFieldSemanticMarkers(field))
}

func agentManagementConfigCapabilityMarkers() []string {
	markers := []string{}
	for _, descriptor := range agentManagementConfigFieldDescriptors() {
		markers = appendUniqueStrings(markers, descriptor.explicitMarkers...)
		markers = appendUniqueStrings(markers, descriptor.semanticMarkers...)
	}
	markers = appendUniqueStrings(markers, agentManagementConfigOnlyCapabilityMarkers()...)
	return markers
}

func agentSkillBackedCapabilityCandidateQueryForText(text string) string {
	text = strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(text))))
	if text == "" {
		return ""
	}
	for _, descriptor := range agentSkillBackedCapabilityDescriptors() {
		if containsAnySubstring(text, descriptor.Markers) {
			return descriptor.CandidateQuery
		}
	}
	return ""
}

func agentManagementGenericBindingResourceStatusRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	return containsPositiveAgentManagementResourceMarker(text, []string{
		"binding resource",
		"binding resources",
		"bound resource",
		"bound resources",
		"associated resource",
		"associated resources",
		"resources",
		"resource",
		"\u7ed1\u5b9a\u8d44\u6e90",
		"\u5173\u8054\u8d44\u6e90",
		"\u5df2\u7ed1\u5b9a\u8d44\u6e90",
		"\u8d44\u6e90",
	})
}

func agentManagementConfigCapabilityStatusRequested(query string, field string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" || !agentManagementCapabilityStatusQuestionRequested(text) {
		return false
	}
	descriptor, ok := agentManagementConfigOnlyCapabilityDescriptorForField(field)
	if !ok {
		return false
	}
	return containsPositiveAgentManagementResourceMarker(text, descriptor.Markers)
}

func agentManagementConfigCapabilityMarkerRequested(query string) bool {
	query = strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if query == "" {
		return false
	}
	if len(agentManagementExplicitConfigFieldsFromText(query)) > 0 {
		return true
	}
	return containsPositiveAgentManagementResourceMarker(query, agentManagementConfigCapabilityMarkers())
}

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

func agentManagementCreateRequested(query string) bool {
	raw := strings.ToLower(strings.TrimSpace(query))
	if raw == "" {
		return false
	}
	if strings.Contains(raw, "create_agent") {
		return true
	}
	if agentManagementCreateMentionIsExistingReferenceOnly(raw) {
		return false
	}
	text := agentManagementCreateIntentText(raw)
	if text == "" || !containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) {
		return false
	}
	if containsAnySubstring(text, []string{"create agent", "create an agent", "create a new agent", "add agent", "add an agent", "add a new agent", "\u521b\u5efa\u667a\u80fd\u4f53", "\u65b0\u5efa\u667a\u80fd\u4f53"}) {
		return true
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:create|add|new)\s+(?:a\s+|an\s+|one\s+|[0-9]+\s+|two\s+|three\s+|four\s+|five\s+)?(?:temporary\s+|draft\s+|test\s+|new\s+)*agents?\b`),
	} {
		if pattern.MatchString(text) {
			return true
		}
	}
	for _, marker := range []string{"\u521b\u5efa", "\u65b0\u5efa", "\u65b0\u589e"} {
		if containsUnnegatedAgentManagementMutationMarker(text, marker) {
			return true
		}
	}
	return false
}

func agentManagementCreateMentionIsExistingReferenceOnly(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripQuotedIntentPayloads(query)))
	if text == "" || !agentManagementCreateMentionHasExistingReference(text) {
		return false
	}
	return !agentManagementExplicitCreateCommandRequested(text)
}

func agentManagementCreateMentionHasExistingReference(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return containsAnySubstring(text, []string{
		"just created", "newly created", "already created", "previously created", "created agent", "created agents",
		"\u521a\u521a\u521b\u5efa", "\u521a\u521b\u5efa", "\u521a\u624d\u521b\u5efa",
		"\u65b0\u521b\u5efa\u7684", "\u5df2\u521b\u5efa", "\u5df2\u7ecf\u521b\u5efa", "\u4e4b\u524d\u521b\u5efa", "\u524d\u9762\u521b\u5efa",
		"\u521a\u521a\u65b0\u5efa", "\u521a\u65b0\u5efa", "\u521a\u624d\u65b0\u5efa",
		"\u65b0\u5efa\u7684", "\u5df2\u65b0\u5efa", "\u5df2\u7ecf\u65b0\u5efa", "\u4e4b\u524d\u65b0\u5efa", "\u524d\u9762\u65b0\u5efa",
		"\u521b\u5efa\u7684\u8fd9", "\u521b\u5efa\u7684\u4e24", "\u521b\u5efa\u7684\u51e0", "\u521b\u5efa\u7684\u667a\u80fd\u4f53",
	})
}

func agentManagementExplicitCreateCommandRequested(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	if containsAnySubstring(text, []string{
		"please create", "help me create", "create agent", "create an agent", "create a new agent",
		"create one agent", "create two agents", "create three agents", "new agent", "add agent",
		"then create", "and create", "create another", "recreate",
		"\u8bf7\u521b\u5efa", "\u5e2e\u6211\u521b\u5efa", "\u5e2e\u5fd9\u521b\u5efa",
		"\u521b\u5efa\u4e00", "\u521b\u5efa\u4e24", "\u521b\u5efa 2", "\u521b\u5efa2", "\u521b\u5efa\u65b0", "\u521b\u5efa\u667a\u80fd\u4f53",
		"\u8bf7\u65b0\u5efa", "\u5e2e\u6211\u65b0\u5efa", "\u65b0\u5efa\u4e00", "\u65b0\u5efa\u4e24", "\u65b0\u5efa\u667a\u80fd\u4f53",
		"\u65b0\u589e\u4e00", "\u65b0\u589e\u667a\u80fd\u4f53",
		"\u7136\u540e\u521b\u5efa", "\u7136\u540e\u518d\u521b\u5efa", "\u518d\u521b\u5efa", "\u5e76\u521b\u5efa", "\u540c\u65f6\u521b\u5efa", "\u53e6\u5916\u521b\u5efa", "\u91cd\u65b0\u521b\u5efa",
		"\u7136\u540e\u65b0\u5efa", "\u7136\u540e\u518d\u65b0\u5efa", "\u518d\u65b0\u5efa", "\u5e76\u65b0\u5efa", "\u540c\u65f6\u65b0\u5efa", "\u53e6\u5916\u65b0\u5efa", "\u91cd\u65b0\u65b0\u5efa",
		"\u521b\u5efa\u6210\u529f\u540e", "\u521b\u5efa\u540e", "\u65b0\u5efa\u6210\u529f\u540e", "\u65b0\u5efa\u540e",
	}) {
		return true
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:create|add|new)\s+(?:a\s+|an\s+|one\s+|[0-9]+\s+|two\s+|three\s+|four\s+|five\s+)?(?:temporary\s+|draft\s+|test\s+|new\s+)*agents?\b`),
		regexp.MustCompile("(?:\u521b\u5efa|\u65b0\u5efa|\u65b0\u589e)\\s*[0-9]+\\s*(?:\u4e2a|\u4f4d|\\s)*(?:agent|agents|\u667a\u80fd\u4f53)"),
	} {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func agentManagementCreateIntentText(query string) string {
	text := stripAgentManagementCreateFieldPayloads(query)
	for _, marker := range []string{
		"name to", "rename to", "change name to", "set name to",
		"description to", "desc to", "change description to", "set description to",
		"icon to", "change icon to", "set icon to",
		"title to", "home title to", "homepage title to", "opening question to", "suggested question to",
		"\u540d\u79f0\u6539\u4e3a", "\u540d\u79f0\u6539\u6210", "\u540d\u5b57\u6539\u4e3a", "\u540d\u5b57\u6539\u6210",
		"\u63cf\u8ff0\u6539\u4e3a", "\u63cf\u8ff0\u6539\u6210", "\u63cf\u8ff0\u8bbe\u7f6e\u4e3a",
		"\u56fe\u6807\u6539\u4e3a", "\u56fe\u6807\u6539\u6210", "\u56fe\u6807\u8bbe\u7f6e\u4e3a",
		"\u9996\u9875\u6807\u9898\u6539\u4e3a", "\u9996\u9875\u6807\u9898\u8bbe\u7f6e\u4e3a", "\u5f00\u573a\u95ee\u9898\u6539\u4e3a", "\u5efa\u8bae\u95ee\u9898\u6539\u4e3a",
	} {
		text = removeAgentManagementCreateAssignmentPayload(text, marker)
	}
	return strings.ToLower(strings.TrimSpace(text))
}

func agentManagementDeleteRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if agentManagementDeleteMentionIsOnlyDescriptive(text) {
		return false
	}
	originalText := text
	text = stripQuotedIntentPayloads(text)
	return agentManagementExplicitDeleteRequestedInText(text, originalText)
}

func agentManagementExplicitDeleteRequestedInText(text string, originalText string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	originalText = strings.ToLower(strings.TrimSpace(originalText))
	if strings.Contains(text, "delete_agent") || strings.Contains(text, "delete_agents") {
		return true
	}
	if !containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) &&
		!containsAnySubstring(originalText, []string{"agent", "\u667a\u80fd\u4f53"}) {
		return false
	}
	for _, marker := range []string{"delete", "remove", "\u5220\u9664", "\u5220\u6389", "\u79fb\u9664", "\u6e05\u7406"} {
		if containsUnnegatedAgentManagementDeleteMarker(text, marker) {
			return true
		}
	}
	return false
}

func containsUnnegatedAgentManagementDeleteMarker(text string, marker string) bool {
	marker = strings.TrimSpace(strings.ToLower(marker))
	if text == "" || marker == "" {
		return false
	}
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], marker)
		if idx < 0 {
			return false
		}
		absoluteIdx := searchFrom + idx
		if !agentManagementDeleteMarkerEmbeddedInIdentifier(text, absoluteIdx, marker) &&
			!agentManagementMutationMarkerIsNavigationNoun(text, absoluteIdx, marker) &&
			!agentManagementMutationMarkerNegated(text, absoluteIdx) &&
			!agentManagementDeleteMarkerReferencesBindingMutation(text, absoluteIdx, marker) {
			return true
		}
		searchFrom = absoluteIdx + len(marker)
		if searchFrom >= len(text) {
			return false
		}
	}
}

func agentManagementDeleteMarkerEmbeddedInIdentifier(text string, markerStart int, marker string) bool {
	if text == "" || marker == "" || markerStart < 0 || markerStart+len(marker) > len(text) {
		return false
	}
	for _, r := range marker {
		if r > unicode.MaxASCII {
			return false
		}
	}
	isIdentifierByte := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_'
	}
	if markerStart > 0 && isIdentifierByte(text[markerStart-1]) {
		return true
	}
	after := markerStart + len(marker)
	return after < len(text) && isIdentifierByte(text[after])
}

func agentManagementDeleteMarkerReferencesBindingMutation(text string, markerStart int, marker string) bool {
	marker = strings.TrimSpace(strings.ToLower(marker))
	if text == "" || marker == "" || markerStart < 0 || markerStart+len(marker) > len(text) {
		return false
	}
	switch marker {
	case "remove":
		if strings.HasPrefix(text[markerStart:], "remove_") {
			return true
		}
	case "delete", "\u5220\u9664", "\u5220\u6389", "\u79fb\u9664", "\u6e05\u7406":
	default:
		return false
	}
	clause := agentManagementClauseAt(text, markerStart)
	if clause == "" {
		return false
	}
	if containsAnySubstring(clause, []string{"delete agent", "delete agents", "remove agent", "remove agents", "\u5220\u9664\u667a\u80fd\u4f53", "\u5220\u6389\u667a\u80fd\u4f53"}) {
		return false
	}
	hasBindingResource := containsAnySubstring(clause, []string{
		"skill", "knowledge", "database", "table", "workflow", "binding", "bindings",
		"\u6280\u80fd", "\u77e5\u8bc6\u5e93", "\u6570\u636e\u5e93", "\u6570\u636e\u8868", "\u5de5\u4f5c\u6d41", "\u7ed1\u5b9a", "\u5173\u8054",
	})
	hasBindingOperation := containsAnySubstring(clause, []string{
		"unbind", "detach", "disable", "remove_", "remove binding", "delete binding", "clear binding",
		"\u89e3\u7ed1", "\u53d6\u6d88\u5173\u8054", "\u79fb\u9664\u7ed1\u5b9a", "\u5220\u9664\u7ed1\u5b9a", "\u6e05\u7a7a\u7ed1\u5b9a",
	})
	return hasBindingResource && hasBindingOperation
}

func agentManagementClauseAt(text string, markerStart int) string {
	if markerStart < 0 || markerStart > len(text) {
		return ""
	}
	start := agentManagementClauseStart(text, markerStart)
	end := len(text)
	for _, separator := range []string{
		"\uff0c", "\u3002", "\uff1b", "\uff1a", "\uff01", "\uff1f",
		",", ".", ";", ":", "!", "?", "\n", "\r",
	} {
		if separator == "" {
			continue
		}
		if idx := strings.Index(text[markerStart:], separator); idx >= 0 && markerStart+idx < end {
			end = markerStart + idx
		}
	}
	if start > end {
		return ""
	}
	return strings.TrimSpace(text[start:end])
}

func agentManagementDeleteMentionIsOnlyDescriptive(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || !containsAnySubstring(text, []string{"agent", "\u667a\u80fd\u4f53"}) {
		return false
	}
	if agentManagementCreateDeleteMentionIsTestPurpose(text) {
		return true
	}
	if agentManagementExplicitDeleteRequestedInText(stripQuotedIntentPayloads(text), text) {
		return false
	}
	if containsAnySubstring(text, []string{"create", "new", "add", "\u521b\u5efa", "\u65b0\u5efa", "\u65b0\u589e"}) &&
		containsAnySubstring(text, []string{
			"description", "desc", "write", "named", "called",
			"\u63cf\u8ff0", "\u5199", "\u540d\u79f0", "\u53eb",
		}) &&
		containsAnySubstring(text, []string{
			"deletable", "can be deleted", "ok to delete", "safe to delete",
			"\u53ef\u5220\u9664", "\u53ef\u4ee5\u5220\u9664",
		}) {
		return true
	}
	return false
}

func agentManagementCreateDeleteMentionIsTestPurpose(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || !agentManagementCreateRequested(text) {
		return false
	}
	if agentManagementCreateThenDeleteRequested(text) {
		return false
	}
	return containsAnySubstring(text, []string{
		"batch delete regression",
		"batch deletion regression",
		"delete regression",
		"deletion regression",
		"batch delete test",
		"batch deletion test",
		"delete test object",
		"deletion test object",
		"delete smoke",
		"deletion smoke",
		"\u6279\u91cf\u5220\u9664\u56de\u5f52",
		"\u6279\u91cf\u5220\u9664\u6d4b\u8bd5",
		"\u5220\u9664\u56de\u5f52",
		"\u5220\u9664\u6d4b\u8bd5",
		"\u5220\u9664\u5192\u70df",
		"\u5220\u9664\u5bf9\u8c61",
		"\u7528\u4e8e\u6279\u91cf\u5220\u9664",
		"\u7528\u4e8e\u5220\u9664",
	})
}

func agentManagementCreateThenDeleteRequested(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, phrase := range []string{
		"after creating", "after creation", "after it is created", "after they are created", "once created",
		"then delete", "then remove", "and delete", "and remove",
		"\u521b\u5efa\u540e", "\u521b\u5efa\u6210\u529f\u540e", "\u65b0\u5efa\u540e", "\u65b0\u5efa\u6210\u529f\u540e", "\u5b8c\u6210\u540e",
		"\u7136\u540e\u5220\u9664", "\u7136\u540e\u5220\u6389", "\u7136\u540e\u6e05\u7406", "\u5e76\u5220\u9664", "\u5e76\u5220\u6389", "\u5e76\u6e05\u7406",
	} {
		idx := strings.Index(text, phrase)
		if idx < 0 {
			continue
		}
		tail := strings.TrimSpace(text[idx:])
		if agentManagementExplicitDeleteRequestedInText(stripQuotedIntentPayloads(tail), tail) {
			return true
		}
	}
	return false
}

func stripQuotedIntentPayloads(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var b strings.Builder
	var closing rune
	for _, r := range text {
		if closing != 0 {
			if r == closing {
				closing = 0
				b.WriteRune(' ')
			}
			continue
		}
		switch r {
		case '"':
			closing = '"'
			b.WriteRune(' ')
		case '\'':
			closing = '\''
			b.WriteRune(' ')
		case '\u201c':
			closing = '\u201d'
			b.WriteRune(' ')
		case '\u2018':
			closing = '\u2019'
			b.WriteRune(' ')
		case '\u300c':
			closing = '\u300d'
			b.WriteRune(' ')
		case '\u300e':
			closing = '\u300f'
			b.WriteRune(' ')
		case '\u300a':
			closing = '\u300b'
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func agentManagementSecondaryIntentQuery(query string) string {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return ""
	}
	if !agentManagementCreateRequested(text) {
		return text
	}
	return stripAgentManagementCreateFieldPayloads(text)
}

func stripAgentManagementCreateFieldPayloads(text string) string {
	text = strings.ToLower(strings.TrimSpace(stripQuotedIntentPayloads(text)))
	if text == "" {
		return ""
	}
	for _, marker := range []string{
		"name is", "name:", "named", "called",
		"description is", "description:", "desc:", "described as",
		"icon is", "icon:", "icon as", "use icon", "with icon",
		"\u540d\u79f0\u4e3a", "\u540d\u5b57\u4e3a", "\u540d\u4e3a", "\u53eb",
		"\u63cf\u8ff0\u4e3a", "\u63cf\u8ff0\u5199\u4e3a", "\u63cf\u8ff0\u90fd\u5199", "\u63cf\u8ff0\uff1a", "\u63cf\u8ff0:",
		"\u56fe\u6807\u4e3a", "\u56fe\u6807\u8bbe\u7f6e\u4e3a", "\u56fe\u6807\u4f7f\u7528", "\u4f7f\u7528\u56fe\u6807",
	} {
		text = removeAgentManagementCreateAssignmentPayload(text, marker)
	}
	return strings.TrimSpace(text)
}

func removeAgentManagementCreateAssignmentPayload(text string, marker string) string {
	marker = strings.ToLower(strings.TrimSpace(marker))
	if text == "" || marker == "" {
		return text
	}
	for {
		lower := strings.ToLower(text)
		idx := strings.Index(lower, marker)
		if idx < 0 {
			return strings.TrimSpace(text)
		}
		start := idx + len(marker)
		end := len(text)
		for _, stop := range []string{
			"\uff0c", "\u3002", "\uff1b", "\u3001", ";", ".", "\n", "\r",
			" then ", " and then ", " after ",
			"\u7136\u540e", "\u518d", "\u540c\u65f6", "\u5e76", "\u521b\u5efa\u540e", "\u5b8c\u6210\u540e",
		} {
			if stop == "" {
				continue
			}
			if relative := strings.Index(lower[start:], stop); relative >= 0 && start+relative < end {
				end = start + relative
			}
		}
		text = strings.TrimSpace(text[:idx] + " " + text[end:])
	}
}
