package service

import (
	"strings"
	"unicode"
)

// agentManagementCapabilityGoalsForQuery is kept test-only while runtime
// planning migrates to model-provided capability goals.
func agentManagementCapabilityGoalsForQuery(query string) []AIChatAgentCapabilityGoal {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" {
		return nil
	}
	goals := []AIChatAgentCapabilityGoal{}
	add := func(goal AIChatAgentCapabilityGoal) {
		goal.CapabilityID = strings.TrimSpace(goal.CapabilityID)
		if goal.CapabilityID == "" {
			return
		}
		goal.GoalAction = canonicalAgentCapabilityAction(goal.GoalAction)
		goal.DisplayName = strings.TrimSpace(goal.DisplayName)
		goal.Meaning = strings.TrimSpace(goal.Meaning)
		goal.UserIntent = strings.TrimSpace(goal.UserIntent)
		goal.CandidateTool = strings.TrimSpace(goal.CandidateTool)
		goal.CandidateQuery = strings.TrimSpace(goal.CandidateQuery)
		goal.CandidateUseCase = strings.TrimSpace(goal.CandidateUseCase)
		goal.RequiredConfigFields = canonicalAgentCapabilityConfigFields(goal.RequiredConfigFields)
		goal.RequiredBindingActions = canonicalAgentCapabilityBindingActions(goal.RequiredBindingActions)
		goal = agentCapabilityGoalWithDefaults(goal)
		for idx, existing := range goals {
			if existing.CapabilityID == goal.CapabilityID &&
				strings.EqualFold(existing.CandidateQuery, goal.CandidateQuery) {
				if canonicalAgentCapabilityAction(existing.GoalAction) == agentCapabilityActionInspect &&
					canonicalAgentCapabilityAction(goal.GoalAction) != agentCapabilityActionInspect {
					goals[idx] = goal
				}
				return
			}
		}
		goals = append(goals, goal)
	}

	if agentManagementModelSelectionRequested(text) {
		add(AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilityModelSelection,
			GoalAction:           agentCapabilityActionUpdate,
			DisplayName:          "model selection",
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{"model_provider", "model"},
			CandidateTool:        "list_available_models",
			CandidateUseCase:     agentManagementModelUseCase(text),
			VerifyBy:             []string{"get_agent_config shows the selected provider/model pair"},
		})
	}
	if agentManagementPersonaUpdateRequested(text) ||
		(agentManagementMutationVerbRequested(text) && agentManagementConfigFieldSemanticMarkerRequested(text, "system_prompt")) {
		add(AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilitySystemPrompt,
			GoalAction:           agentCapabilityActionUpdate,
			DisplayName:          "system prompt",
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{"system_prompt"},
			VerifyBy:             []string{"get_agent_config returns a system_prompt consistent with the requested role or instruction"},
		})
	}
	explicitConfigFields := agentManagementExplicitConfigFieldsFromText(text)
	for _, descriptor := range agentManagementConfigOnlyCapabilityDescriptors() {
		field := operationPlanAgentConfigCanonicalField(descriptor.Field)
		if field == "" {
			continue
		}
		statusRequested := agentManagementConfigCapabilityStatusRequested(text, field)
		mutationRequested := agentManagementConfigOnlyCapabilityRequested(text, field)
		if !mutationRequested && !statusRequested && stringSliceContainsFold(explicitConfigFields, field) {
			mutationRequested = agentManagementMutationVerbRequested(text) ||
				agentManagementCapabilityEnablePhraseRequested(text)
		}
		if !mutationRequested && !statusRequested {
			continue
		}
		action := canonicalAgentCapabilityAction(descriptor.GoalAction)
		if action == "" {
			action = agentCapabilityActionEnable
		}
		verifyBy := append([]string(nil), descriptor.EnableVerify...)
		if statusRequested {
			action = agentCapabilityActionInspect
			verifyBy = append([]string(nil), descriptor.InspectVerify...)
		}
		add(AIChatAgentCapabilityGoal{
			CapabilityID:         descriptor.CapabilityID,
			GoalAction:           action,
			DisplayName:          descriptor.DisplayName,
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{field},
			VerifyBy:             verifyBy,
		})
	}
	skillCapabilityQuery := agentManagementSkillCapabilityCandidateQuery(text)
	statusCapabilityQuery := ""
	if skillCapabilityQuery == "" {
		statusCapabilityQuery = agentManagementCapabilityStatusCandidateQueryForText(text)
	}
	candidateQuery := firstNonEmptyString(skillCapabilityQuery, statusCapabilityQuery)
	if candidateQuery != "" {
		goal := AIChatAgentCapabilityGoal{
			CapabilityID: agentCapabilitySkillBacked,
			GoalAction:   agentCapabilityActionEnable,
			DisplayName:  agentSkillBackedCapabilityDisplayName(candidateQuery),
			UserIntent:   truncateRunes(text, 240),
			RequiredConfigFields: []string{
				"enabled_skill_ids",
			},
			CandidateTool:  "list_agent_skill_candidates",
			CandidateQuery: candidateQuery,
			NotSufficient: []string{
				"system_prompt_only",
				"file_upload_enabled_only",
			},
			VerifyBy: []string{"get_agent_config.enabled_skill_ids contains a selected candidate skill id"},
		}
		if statusCapabilityQuery != "" {
			goal.GoalAction = agentCapabilityActionInspect
			goal.VerifyBy = []string{"get_agent_config.enabled_skill_ids and candidate lookup report whether a matching Skill is enabled"}
		} else {
			goal.RequiredBindingActions = map[string]string{
				"enabled_skill_ids": "bind",
			}
		}
		add(goal)
	}
	if agentManagementSkillBindingRequested(text) {
		action := operationPlanCanonicalAgentConfigBindingAction(
			agentBindingExpectedActionForResource(text, []string{"skill", "\u6280\u80fd"}),
		)
		if action == "" {
			action = operationPlanCanonicalAgentConfigBindingAction(agentBindingExpectedActionFromText(text))
		}
		if action == "" {
			action = "bind"
		}
		goalAction := agentCapabilityActionBind
		switch action {
		case "unbind":
			goalAction = agentCapabilityActionUnbind
		case "replace":
			goalAction = agentCapabilityActionReplace
		}
		goal := AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilitySkillBacked,
			GoalAction:           goalAction,
			DisplayName:          "skill binding",
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{"enabled_skill_ids"},
			RequiredBindingActions: map[string]string{
				"enabled_skill_ids": action,
			},
			VerifyBy: []string{"get_agent_config.enabled_skill_ids reflects the requested Skill binding change"},
		}
		if agentManagementSkillBindingCandidateLookupNeeded(text) {
			goal.CandidateTool = "list_agent_skill_candidates"
			goal.CandidateQuery = agentManagementSkillCandidateQuery(text)
		}
		add(goal)
	}
	if stringSliceContainsFold(explicitConfigFields, "suggested_questions") ||
		(agentManagementMutationVerbRequested(text) && agentManagementConfigFieldSemanticMarkerRequested(text, "suggested_questions")) {
		add(AIChatAgentCapabilityGoal{
			CapabilityID:         agentCapabilitySuggestedQuestion,
			GoalAction:           agentCapabilityActionUpdate,
			DisplayName:          "suggested_questions",
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{"suggested_questions"},
			VerifyBy:             []string{"get_agent_config returns the requested suggested_questions state"},
		})
	}
	for _, goal := range agentManagementReadOnlyBindingCapabilityGoals(text) {
		add(goal)
	}
	for _, toolName := range requiredAgentBindingMutationTools(text) {
		field := agentBindingRequirementField(toolName)
		if field == "" {
			continue
		}
		action := operationPlanCanonicalAgentConfigBindingAction(agentBindingExpectedActionFromText(text))
		if action == "" {
			action = "bind"
		}
		capabilityID := operationPlanAgentResourceBindingCapabilityForField(field)
		if capabilityID == "" {
			continue
		}
		add(AIChatAgentCapabilityGoal{
			CapabilityID:         capabilityID,
			GoalAction:           canonicalAgentCapabilityAction(action),
			DisplayName:          field,
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{field},
			RequiredBindingActions: map[string]string{
				field: action,
			},
			VerifyBy: []string{"get_agent_config returns the requested " + field + " binding state"},
		})
	}
	if len(goals) == 0 {
		return nil
	}
	return goals
}

func agentManagementCapabilityStatusCandidateQueryForText(text string) string {
	if !agentManagementCapabilityStatusQuestionRequested(text) {
		return ""
	}
	if candidateQuery := agentSkillBackedCapabilityCandidateQueryForText(text); candidateQuery != "" {
		return candidateQuery
	}
	if containsAnySubstring(text, []string{"skill", "tool", "\u6280\u80fd", "\u5de5\u5177"}) {
		return strings.TrimSpace(truncateRunes(text, 80))
	}
	return ""
}

func agentManagementSkillCandidateQuery(query string) string {
	text := strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query)))
	if text == "" {
		return ""
	}
	if value := agentManagementExplicitCandidateQueryValue(text); value != "" {
		return value
	}
	if value := agentManagementSkillCapabilityCandidateQuery(text); value != "" {
		return value
	}
	return agentManagementQuotedResourceNameAfterMarkers(text, []string{"skill", "\u6280\u80fd"})
}

func agentManagementExplicitCandidateQueryValue(text string) string {
	lower := strings.ToLower(text)
	anchor := "list_agent_skill_candidates"
	anchorIndex := strings.Index(lower, anchor)
	if anchorIndex < 0 {
		return ""
	}
	tail := text[anchorIndex+len(anchor):]
	queryIndex := strings.Index(strings.ToLower(tail), "query")
	if queryIndex < 0 {
		return ""
	}
	return trimAgentManagementCandidateQueryValue(tail[queryIndex+len("query"):])
}

func trimAgentManagementCandidateQueryValue(value string) string {
	runes := []rune(strings.TrimSpace(value))
	start := 0
	for start < len(runes) {
		r := runes[start]
		if unicode.IsSpace(r) || strings.ContainsRune(":=\u540d\u4e3a\u7528\u4e3a\u662f\uff1a\uff0c,;'\u201c\u201d\"\u300c\u300d`", r) {
			start++
			continue
		}
		break
	}
	runes = runes[start:]
	end := 0
	for end < len(runes) {
		r := runes[end]
		if r == '\n' || r == '\r' || strings.ContainsRune(",.;\uff0c\u3002\uff1b\u3001", r) {
			break
		}
		end++
		if end >= 80 {
			break
		}
	}
	return strings.Trim(strings.TrimSpace(string(runes[:end])), "`\"'\u201c\u201d\u300c\u300d")
}

func agentManagementQuotedResourceNameAfterMarkers(text string, markers []string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lower := strings.ToLower(text)
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" {
			continue
		}
		searchFrom := 0
		for {
			index := strings.Index(lower[searchFrom:], marker)
			if index < 0 {
				break
			}
			index += searchFrom
			tail := text[index+len(marker):]
			if value := firstQuotedValue(tail); value != "" {
				return value
			}
			searchFrom = index + len(marker)
			if searchFrom >= len(lower) {
				break
			}
		}
	}
	return ""
}

func firstQuotedValue(text string) string {
	pairs := []struct {
		open  rune
		close rune
	}{
		{'\u300c', '\u300d'},
		{'\u300a', '\u300b'},
		{'\u201c', '\u201d'},
		{'"', '"'},
		{'\'', '\''},
		{'`', '`'},
	}
	runes := []rune(text)
	for _, pair := range pairs {
		start := -1
		for i, r := range runes {
			if i > 80 {
				break
			}
			if r == pair.open {
				start = i + 1
				break
			}
		}
		if start < 0 {
			continue
		}
		for i := start; i < len(runes); i++ {
			if runes[i] == pair.close {
				return strings.TrimSpace(string(runes[start:i]))
			}
		}
	}
	return ""
}

func agentManagementSkillBindingCandidateLookupNeeded(query string) bool {
	if !agentManagementSkillBindingRequested(query) {
		return false
	}
	action := operationPlanCanonicalAgentConfigBindingAction(
		agentBindingExpectedActionForResource(query, []string{"skill", "\u6280\u80fd"}),
	)
	if action == "unbind" && agentManagementSkillUnbindUsesCurrentBindingSet(query) {
		return false
	}
	return true
}

func agentManagementSkillUnbindUsesCurrentBindingSet(query string) bool {
	return agentManagementResourceUnbindUsesCurrentBindingSet(query, []string{"skill", "\u6280\u80fd"})
}

func agentManagementResourceUnbindUsesCurrentBindingSet(query string, resourceMarkers []string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" {
		return false
	}
	if action := operationPlanCanonicalAgentConfigBindingAction(agentBindingExpectedActionForResource(text, resourceMarkers)); action != "unbind" {
		globalAction := operationPlanCanonicalAgentConfigBindingAction(agentBindingExpectedActionFromText(text))
		if globalAction != "unbind" || !containsAnySubstring(text, resourceMarkers) {
			return false
		}
	}
	return containsAnySubstring(text, []string{
		"all skill", "all skills", "every skill", "all binding", "all bindings",
		"current knowledge", "current database", "current table", "current workflow", "from the current agent",
		"current binding", "current bindings", "existing binding", "existing bindings",
		"\u5168\u90e8", "\u6240\u6709", "\u90fd",
		"\u5f53\u524d\u7ed1\u5b9a", "\u5f53\u524d\u5df2\u7ed1\u5b9a", "\u73b0\u6709\u7ed1\u5b9a", "\u5df2\u6709\u7ed1\u5b9a",
	})
}

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

func agentManagementConfigReadRequested(query string) bool {
	query = agentManagementSecondaryIntentQuery(query)
	if agentManagementDefaultConfigReferenceOnly(query) {
		return false
	}
	if agentManagementCapabilityStatusQuestionRequested(query) {
		return true
	}
	return strings.Contains(query, "get_agent_config") ||
		agentManagementBindingReadRequested(query) ||
		containsPositiveAgentManagementResourceMarker(query, []string{
			"agent config", "agent configuration", "config", "configuration", "current model", "model name", "provider",
			"\u667a\u80fd\u4f53\u914d\u7f6e", "\u914d\u7f6e", "\u5f53\u524d\u6a21\u578b", "\u6a21\u578b\u540d\u79f0", "\u4f9b\u5e94\u5546",
		})
}

func agentManagementDefaultConfigReferenceOnly(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementCreateFieldPayloads(query)))
	if text == "" {
		return false
	}
	if !containsAnySubstring(text, []string{
		"default model", "default config", "default configuration", "use default", "keep default",
		"\u9ed8\u8ba4\u6a21\u578b", "\u9ed8\u8ba4\u914d\u7f6e", "\u9ed8\u8ba4\u7684\u6a21\u578b", "\u9ed8\u8ba4\u7684\u914d\u7f6e",
		"\u4f7f\u7528\u9ed8\u8ba4", "\u53ef\u4f7f\u7528\u9ed8\u8ba4", "\u4fdd\u6301\u9ed8\u8ba4",
	}) {
		return false
	}
	if containsAnySubstring(text, []string{
		"tell me", "show", "view", "inspect", "read", "check", "current", "what", "which",
		"\u544a\u8bc9", "\u67e5\u770b", "\u8bfb\u53d6", "\u68c0\u67e5", "\u5c55\u793a", "\u5f53\u524d", "\u662f\u4ec0\u4e48", "\u600e\u4e48\u6837", "\u6709\u54ea\u4e9b",
	}) {
		return false
	}
	if containsAnySubstring(text, []string{
		"set model", "switch model", "replace model", "change model", "set provider", "switch provider",
		"\u8bbe\u7f6e\u6a21\u578b", "\u5207\u6362\u6a21\u578b", "\u66ff\u6362\u6a21\u578b", "\u6539\u6210\u9ed8\u8ba4\u6a21\u578b",
		"\u8bbe\u7f6e\u4e3a\u9ed8\u8ba4\u6a21\u578b", "\u6539\u4e3a\u9ed8\u8ba4\u6a21\u578b",
	}) {
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

func agentManagementModelSelectionRequested(query string) bool {
	query = agentManagementSecondaryIntentQuery(query)
	if strings.Contains(query, "list_available_models") {
		return true
	}
	if containsPositiveAgentManagementResourceMarker(query, []string{
		"configure model",
		"model configured as",
		"model configured to",
		"\u914d\u7f6e\u6a21\u578b",
		"\u6a21\u578b\u914d\u7f6e",
		"\u6a21\u578b\u914d\u7f6e\u4e3a",
		"\u6a21\u578b\u914d\u6210",
	}) {
		return true
	}
	if agentManagementModelSelectionResourceMutationRequested(query) {
		return true
	}
	if agentManagementNamedModelSelectionRequested(query) {
		return true
	}
	if !agentManagementMutationVerbRequested(query) {
		return false
	}
	return containsPositiveAgentManagementResourceMarker(query, []string{
		"set model",
		"switch model",
		"replace model",
		"change model",
		"update model",
		"modify model",
		"use model",
		"model to",
		"model:",
		"set provider",
		"switch provider",
		"replace provider",
		"change provider",
		"update provider",
		"modify provider",
		"provider to",
		"provider:",
		"\u8bbe\u7f6e\u6a21\u578b",
		"\u5207\u6362\u6a21\u578b",
		"\u66ff\u6362\u6a21\u578b",
		"\u66f4\u6362\u6a21\u578b",
		"\u4fee\u6539\u6a21\u578b",
		"\u66f4\u65b0\u6a21\u578b",
		"\u4f7f\u7528\u6a21\u578b",
		"\u6a21\u578b\u4f7f\u7528",
		"\u6a21\u578b\u7528",
		"\u6a21\u578b\u6539\u4e3a",
		"\u6a21\u578b\u8bbe\u4e3a",
		"\u6a21\u578b\u5207\u6362",
		"\u6a21\u578b\u66ff\u6362",
		"\u6a21\u578b\u66f4\u6362",
		"\u8bbe\u7f6e provider",
		"\u5207\u6362 provider",
		"\u66ff\u6362 provider",
		"\u66f4\u6362 provider",
		"provider \u6539\u4e3a",
		"provider \u8bbe\u4e3a",
		"\u4f9b\u5e94\u5546\u6539\u4e3a",
		"\u4f9b\u5e94\u5546\u8bbe\u4e3a",
	})
}

func agentManagementNamedModelSelectionRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" || !agentManagementModelIdentifierMentioned(text) {
		return false
	}
	return agentManagementModelSelectionClauseRequestsMutation(text)
}

func agentManagementModelIdentifierMentioned(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	return agentManagementModelIdentifierPattern.MatchString(text)
}

func agentManagementModelSelectionResourceMutationRequested(query string) bool {
	text := strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(query)))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"model",
		"provider",
		"use_case",
		"use case",
		"\u6a21\u578b",
		"\u4f9b\u5e94\u5546",
	} {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" {
			continue
		}
		searchFrom := 0
		for {
			idx := strings.Index(text[searchFrom:], marker)
			if idx < 0 {
				break
			}
			absoluteIdx := searchFrom + idx
			clause := agentManagementClauseAt(text, absoluteIdx)
			if !agentManagementResourceMarkerNegatedInClause(text, absoluteIdx) &&
				agentManagementModelSelectionClauseRequestsMutation(clause) {
				return true
			}
			searchFrom = absoluteIdx + len(marker)
			if searchFrom >= len(text) {
				break
			}
		}
	}
	return false
}

func agentManagementModelSelectionClauseRequestsMutation(clause string) bool {
	clause = strings.ToLower(strings.TrimSpace(clause))
	if clause == "" {
		return false
	}
	if containsAnySubstring(clause, []string{
		"what model",
		"which model",
		"current model",
		"currently using",
		"using now",
		"model is it using",
		"default model",
		"use default model",
		"using default model",
		"\u5f53\u524d\u6a21\u578b",
		"\u73b0\u5728\u6a21\u578b",
		"\u9ed8\u8ba4\u6a21\u578b",
		"\u4f7f\u7528\u9ed8\u8ba4\u6a21\u578b",
		"\u4f7f\u7528\u7684\u6a21\u578b",
		"\u6a21\u578b\u662f\u4ec0\u4e48",
		"\u7528\u7684\u6a21\u578b",
	}) {
		return false
	}
	if agentManagementMutationVerbRequested(clause) {
		return true
	}
	return containsAnySubstring(clause, []string{
		"use ",
		"using ",
		"use_case",
		"use case",
		"\u4f7f\u7528",
		"\u91c7\u7528",
		"\u9009\u7528",
		"\u7528",
	})
}

func agentManagementModelUseCase(query string) string {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" {
		return "text-chat"
	}
	if value := canonicalAgentManagementModelUseCase(agentManagementExplicitModelUseCaseValue(text)); value != "" {
		return value
	}
	switch {
	case containsAnySubstring(text, []string{"function-calling", "function calling", "tool calling", "tool-call", "tool call", "\u5de5\u5177\u8c03\u7528", "\u51fd\u6570\u8c03\u7528"}):
		return "function-calling"
	case containsAnySubstring(text, []string{"vision", "visual", "image input", "image understanding", "\u89c6\u89c9", "\u56fe\u50cf\u7406\u89e3", "\u8bfb\u56fe", "\u770b\u56fe"}):
		return "vision"
	case containsAnySubstring(text, []string{"reasoning", "reasoner", "deep thinking", "complex reasoning", "\u63a8\u7406", "\u6df1\u5ea6\u601d\u8003", "\u590d\u6742\u63a8\u7406"}):
		return "reasoning"
	default:
		return "text-chat"
	}
}

func agentManagementExplicitModelUseCaseValue(text string) string {
	for _, marker := range []string{"use_case=", "use_case:", "use_case ", "use case=", "use case:", "use case "} {
		idx := strings.Index(text, marker)
		if idx < 0 {
			continue
		}
		value := strings.TrimSpace(text[idx+len(marker):])
		value = strings.TrimLeft(value, " =:\uff1a")
		if value == "" {
			continue
		}
		return firstModelUseCaseToken(value)
	}
	return ""
}

func firstModelUseCaseToken(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			return b.String()
		}
	}
	return b.String()
}

func canonicalAgentManagementModelUseCase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text-chat", "text_chat", "chat", "default":
		return "text-chat"
	case "reasoning", "reasoner":
		return "reasoning"
	case "vision", "visual":
		return "vision"
	case "function-calling", "function_calling", "function", "tool-calling", "tool_calling":
		return "function-calling"
	default:
		return ""
	}
}

func isAgentManagementIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if consoleAgentsManagePattern.MatchString(text) || containsAgentManagementToolMention(text) {
		return true
	}
	agentTerms := []string{"agent", "\u667a\u80fd\u4f53"}
	if !containsAnySubstring(text, agentTerms) {
		return false
	}
	if agentManagementConfigUpdateRequested(text) || agentManagementSkillBindingRequested(text) {
		return true
	}
	operationTerms := []string{
		"create", "new", "add", "edit", "update", "rename", "delete", "remove", "config", "configure", "prompt", "model", "icon", "description",
		"bind", "unbind", "enable", "disable", "detach", "clear",
		"\u521b\u5efa", "\u65b0\u5efa", "\u6dfb\u52a0", "\u7f16\u8f91", "\u4fee\u6539", "\u66f4\u65b0", "\u6539\u540d", "\u5220\u9664", "\u5220\u6389",
		"\u914d\u7f6e", "\u63d0\u793a\u8bcd", "\u6a21\u578b", "\u56fe\u6807", "\u63cf\u8ff0",
		"\u7ed1\u5b9a", "\u89e3\u7ed1", "\u542f\u7528", "\u7981\u7528", "\u505c\u7528", "\u79fb\u9664", "\u6e05\u7a7a",
	}
	return agentManagementOperationNearAgent(text, agentTerms, operationTerms)
}

func containsAgentManagementToolMention(text string) bool {
	for _, marker := range []string{
		"list_agents",
		"get_agent",
		"create_agent",
		"update_agent_identity",
		"delete_agent",
		"delete_agents",
		"get_agent_config",
		"update_agent_config",
		"replace_agent_memory_slots",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
		"list_available_models",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func agentManagementOperationNearAgent(text string, agentTerms []string, operationTerms []string) bool {
	const maxAgentOperationDistance = 48
	for _, agentTerm := range agentTerms {
		for _, agentPos := range allStringIndexes(text, agentTerm) {
			agentEnd := agentPos + len(agentTerm)
			for _, operationTerm := range operationTerms {
				for _, operationPos := range allStringIndexes(text, operationTerm) {
					distance := agentPos - operationPos
					if distance < 0 {
						distance = -distance
					}
					if distance <= maxAgentOperationDistance {
						if operationPos > agentPos && agentReferenceAttributeCrossesClauseBoundary(text, agentEnd, operationPos) {
							continue
						}
						return true
					}
				}
			}
		}
	}
	return false
}

func agentReferenceAttributeCrossesClauseBoundary(text string, agentEnd int, operationPos int) bool {
	if text == "" || agentEnd < 0 || operationPos <= agentEnd || agentEnd > len(text) || operationPos > len(text) {
		return false
	}
	suffix := text[agentEnd:]
	if !agentReferenceHasAttributeSuffix(suffix) {
		return false
	}
	between := text[agentEnd:operationPos]
	return containsAnySubstring(between, []string{";", ",", ".", "\uff1b", "\uff0c", "\u3002", "\u3001"})
}

func agentReferenceHasAttributeSuffix(suffix string) bool {
	for _, marker := range []string{
		"\u540d\u79f0",
		"\u540d\u5b57",
		"id",
		"\u7f16\u53f7",
		"\u914d\u7f6e",
		"\u5185\u5bb9",
		"\u63cf\u8ff0",
		"\u7ed3\u679c",
		"\u8f93\u51fa",
		"\u7ed1\u5b9a",
		"\u6570\u91cf",
		"\u72b6\u6001",
	} {
		if strings.HasPrefix(suffix, marker) {
			return true
		}
	}
	return false
}

func agentManagementExplicitMutationOverridesReadOnlyConfigCheck(query string) bool {
	query = strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(query)))
	if query == "" {
		return false
	}
	if strings.Contains(query, "update_agent_config") ||
		strings.Contains(query, "update_agent_identity") ||
		strings.Contains(query, "create_agent") ||
		strings.Contains(query, "delete_agent") ||
		agentManagementCreateRequested(query) ||
		agentManagementDeleteRequested(query) ||
		agentManagementExplicitConfigAssignmentRequested(query) ||
		agentManagementExplicitIdentityAssignmentRequested(query) ||
		agentManagementFileUploadConfigCapabilityRequested(query) ||
		agentManagementSkillBindingRequested(query) ||
		len(requiredAgentBindingMutationTools(query)) > 0 {
		return true
	}
	return false
}

func agentManagementFileUploadConfigCapabilityRequested(query string) bool {
	return agentManagementConfigOnlyCapabilityRequested(query, "file_upload_enabled")
}

func agentManagementConfigUpdateRequested(query string) bool {
	query = agentManagementSecondaryIntentQuery(query)
	return strings.Contains(query, "update_agent_config") ||
		agentManagementExplicitConfigAssignmentRequested(query) ||
		agentManagementBroadEditableConfigRequested(query) ||
		agentManagementPersonaUpdateRequested(query) ||
		agentManagementFileUploadConfigCapabilityRequested(query) ||
		agentManagementSkillBindingRequested(query) ||
		len(requiredAgentBindingMutationTools(query)) > 0 ||
		(agentManagementMutationVerbRequested(query) && agentManagementConfigCapabilityMarkerRequested(query))
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

func requiredAgentBindingMutationTools(query string) []string {
	normalized := strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(query)))
	if normalized == "" || !agentBindingMutationRequested(normalized) {
		return nil
	}
	required := make([]string, 0, 3)
	addIfMentioned := func(toolName string, resourceMarkers []string) {
		if strings.Contains(normalized, strings.ToLower(toolName)) {
			required = appendUniqueStrings(required, toolName)
			return
		}
		for _, marker := range resourceMarkers {
			if marker != "" && strings.Contains(normalized, marker) {
				required = appendUniqueStrings(required, toolName)
				return
			}
		}
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		if descriptor.mutationTool == "" || agentBindingResourceNoop(normalized, descriptor.noopKey) {
			continue
		}
		addIfMentioned(agentBindingUpdateConfigRequirement(descriptor.field), descriptor.markers)
	}
	return required
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

func agentManagementReadOnlyBindingCapabilityGoals(query string) []AIChatAgentCapabilityGoal {
	text := strings.ToLower(strings.TrimSpace(stripAgentManagementFinalAnswerInstruction(agentManagementSecondaryIntentQuery(query))))
	if text == "" ||
		!agentBindingStateReadOnlyQuestionRequested(text) ||
		agentBindingMutationRequested(text) {
		return nil
	}
	descriptors := agentManagementBindingCapabilityDescriptors()
	selected := make([]agentBindingCapabilityDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if containsPositiveAgentManagementResourceMarker(text, descriptor.markers) {
			selected = append(selected, descriptor)
		}
	}
	if len(selected) == 0 && agentManagementGenericBindingResourceStatusRequested(text) {
		selected = descriptors
	}
	if len(selected) == 0 {
		return nil
	}
	out := make([]AIChatAgentCapabilityGoal, 0, len(selected))
	for _, descriptor := range selected {
		out = append(out, AIChatAgentCapabilityGoal{
			CapabilityID:         descriptor.capabilityID,
			GoalAction:           agentCapabilityActionInspect,
			DisplayName:          descriptor.displayName,
			UserIntent:           truncateRunes(text, 240),
			RequiredConfigFields: []string{descriptor.field},
			VerifyBy:             []string{"get_agent_config reports the current " + descriptor.field + " binding state"},
		})
	}
	return out
}

func agentManagementExplicitReadOnlyConfigCheck(query string) bool {
	query = strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(query)))
	if query == "" {
		return false
	}
	if agentManagementExplicitMutationOverridesReadOnlyConfigCheck(query) {
		return false
	}
	hasReadOnlyMarker := containsAnySubstring(query, []string{
		"read-only",
		"readonly",
		"read only",
		"only read",
		"inspect",
		"check",
		"view",
		"\u53ea\u8bfb",
		"\u53ea\u8bfb\u53d6",
		"\u53ea\u68c0\u67e5",
		"\u4ec5\u68c0\u67e5",
		"\u53ea\u786e\u8ba4",
		"\u4ec5\u786e\u8ba4",
		"\u67e5\u770b",
		"\u68c0\u67e5",
	})
	if !hasReadOnlyMarker {
		return false
	}
	if agentManagementCandidateLookupExplicitlyNegated(query) {
		return true
	}
	return containsAnySubstring(query, []string{
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
		"without modifying",
		"without changing",
		"no modification",
		"no config changes",
		"do not request approval",
		"don't request approval",
		"dont request approval",
		"do not ask for approval",
		"don't ask for approval",
		"dont ask for approval",
		"without approval",
		"no approval",
		"\u4e0d\u8981\u4fee\u6539",
		"\u4e0d\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u8981\u7f16\u8f91",
		"\u4e0d\u7f16\u8f91",
		"\u4e0d\u8981\u66f4\u65b0",
		"\u4e0d\u66f4\u65b0",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u6539",
		"\u4e0d\u6539",
		"\u4e0d\u8981\u53d8\u66f4",
		"\u4e0d\u53d8\u66f4",
		"\u4e0d\u8981\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u53d1\u8d77\u5ba1\u6279",
		"\u4e0d\u8981\u5ba1\u6279",
		"\u4e0d\u5ba1\u6279",
		"\u65e0\u9700\u5ba1\u6279",
	})
}

func agentManagementMemoryConfigCapabilityRequested(query string) bool {
	return agentManagementConfigOnlyCapabilityRequested(query, "agent_memory_enabled")
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

func agentManagementReadOnlyBindingCandidateTools(query string) []string {
	query = strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(query)))
	if query == "" || !agentManagementBindingCandidateReadRequested(query) {
		return nil
	}
	tools := []string{}
	add := func(toolName string) {
		tools = appendUniqueStrings(tools, toolName)
	}
	if agentManagementBindingCandidateGenericResourceRequested(query) {
		add("list_agent_skill_candidates")
		for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
			for _, toolName := range descriptor.candidateTools {
				add(toolName)
			}
		}
		return tools
	}
	if agentManagementBindingCandidateReadResourceRequested(query, []string{"skill", "\u6280\u80fd"}) {
		add("list_agent_skill_candidates")
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		if !agentManagementBindingCandidateReadResourceRequested(query, descriptor.markers) {
			continue
		}
		for _, toolName := range descriptor.candidateTools {
			add(toolName)
		}
	}
	return tools
}

func agentManagementBindingCandidateReadRequested(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	for _, scope := range agentManagementBindingCandidateReadScopes(query) {
		if containsAnySubstring(scope, agentManagementBindingCandidateReadMarkers()) {
			return true
		}
	}
	return false
}

func agentManagementBindingCandidateReadMarkers() []string {
	markers := []string{
		"skill", "binding", "bindings", "resource", "resources",
		"\u6280\u80fd", "\u7ed1\u5b9a", "\u5173\u8054", "\u8d44\u6e90",
	}
	return appendUniqueStrings(markers, agentManagementBindingCapabilityResourceMarkers()...)
}

func agentManagementBindingCandidateReadResourceRequested(query string, resourceMarkers []string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len(resourceMarkers) == 0 {
		return false
	}
	for _, scope := range agentManagementBindingCandidateReadScopes(query) {
		if agentManagementCandidateScopeContainsPositiveResource(scope, resourceMarkers) {
			return true
		}
	}
	if agentManagementBindingCandidateGenericResourceRequested(query) &&
		containsPositiveAgentManagementResourceMarker(query, resourceMarkers) {
		return true
	}
	return false
}

func agentManagementBindingCandidateReadScopes(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	markers := []string{
		"candidate", "candidates", "available binding", "available bindings", "available to bind", "bindable",
		"\u5019\u9009", "\u53ef\u7ed1\u5b9a", "\u53ef\u5173\u8054", "\u53ef\u7528\u4e8e\u7ed1\u5b9a", "\u53ef\u9009",
	}
	scopes := []string{}
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" {
			continue
		}
		searchFrom := 0
		for {
			idx := strings.Index(query[searchFrom:], marker)
			if idx < 0 {
				break
			}
			markerStart := searchFrom + idx
			start := agentManagementCandidateScopeStart(query, markerStart)
			end := agentBindingNoBindScopeEnd(query, markerStart)
			if start < end {
				scopes = appendUniqueStrings(scopes, strings.TrimSpace(query[start:end]))
			}
			searchFrom = markerStart + len(marker)
		}
	}
	return scopes
}

func agentManagementBindingCandidateGenericResourceRequested(query string) bool {
	for _, scope := range agentManagementBindingCandidateReadScopes(query) {
		if containsAnySubstring(scope, []string{
			"resource", "resources", "binding resource", "binding resources",
			"\u8d44\u6e90", "\u7ed1\u5b9a\u8d44\u6e90", "\u5173\u8054\u8d44\u6e90",
		}) {
			return true
		}
	}
	return false
}
