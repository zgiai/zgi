package service

import "strings"

const (
	agentCapabilitySkillBacked       = "agent.skill_backed_capability"
	agentCapabilityAcceptUploaded    = "agent.accept_uploaded_files"
	agentCapabilityModelSelection    = "agent.model_selection"
	agentCapabilitySystemPrompt      = "agent.system_prompt"
	agentCapabilityMemory            = "agent.memory"
	agentCapabilityKnowledgeBinding  = "agent.knowledge_binding"
	agentCapabilityDatabaseBinding   = "agent.database_binding"
	agentCapabilityWorkflowBinding   = "agent.workflow_binding"
	agentCapabilitySuggestedQuestion = "agent.suggested_questions"

	agentCapabilityActionInspect = "inspect"
	agentCapabilityActionEnable  = "enable"
	agentCapabilityActionUpdate  = "update"
	agentCapabilityActionBind    = "bind"
	agentCapabilityActionUnbind  = "unbind"
	agentCapabilityActionReplace = "replace"
)

// AIChatAgentCapabilityGoal describes the product-level Agent capability that
// the current turn is trying to achieve. It bridges user-facing language and the
// concrete Agent configuration evidence needed for the skill loop.
type AIChatAgentCapabilityGoal struct {
	CapabilityID           string            `json:"capability_id"`
	GoalAction             string            `json:"goal_action,omitempty"`
	DisplayName            string            `json:"display_name,omitempty"`
	Meaning                string            `json:"meaning,omitempty"`
	UserIntent             string            `json:"user_intent,omitempty"`
	RequiredConfigFields   []string          `json:"required_config_fields,omitempty"`
	RequiredBindingActions map[string]string `json:"required_binding_actions,omitempty"`
	CandidateTool          string            `json:"candidate_tool,omitempty"`
	CandidateQuery         string            `json:"candidate_query,omitempty"`
	CandidateUseCase       string            `json:"candidate_use_case,omitempty"`
	EnableBy               []string          `json:"enable_by,omitempty"`
	NotSufficient          []string          `json:"not_sufficient,omitempty"`
	VerifyBy               []string          `json:"verify_by,omitempty"`
}

type agentSkillBackedCapabilityDescriptor struct {
	DisplayName    string
	CandidateQuery string
	Markers        []string
}

type agentConfigOnlyCapabilityDescriptor struct {
	CapabilityID       string
	Field              string
	DisplayName        string
	GoalAction         string
	ContinuationEnable bool
	Markers            []string
	EnableVerify       []string
	InspectVerify      []string
}

type agentConfigFieldDescriptor struct {
	field                string
	aliases              []string
	explicitMarkers      []string
	semanticMarkers      []string
	bindingActionMarkers map[string][]string
}

func agentManagementConfigFieldDescriptors() []agentConfigFieldDescriptor {
	return []agentConfigFieldDescriptor{
		{
			field:           "system_prompt",
			aliases:         []string{"system_prompt"},
			explicitMarkers: []string{"system_prompt"},
			semanticMarkers: []string{"system prompt", "prompt", "\u7cfb\u7edf\u63d0\u793a\u8bcd", "\u63d0\u793a\u8bcd"},
		},
		{
			field:           "model",
			aliases:         []string{"model"},
			semanticMarkers: []string{"model", "\u6a21\u578b"},
		},
		{
			field:           "model_provider",
			aliases:         []string{"model_provider"},
			explicitMarkers: []string{"model_provider", "provider", "\u4f9b\u5e94\u5546"},
			semanticMarkers: []string{"provider", "\u4f9b\u5e94\u5546"},
		},
		{
			field:           "model_parameters",
			aliases:         []string{"model_parameters"},
			explicitMarkers: []string{"model_parameters"},
		},
		{
			field:           "enabled_skill_ids",
			aliases:         []string{"enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids", "agent_skill"},
			explicitMarkers: []string{"enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids", "agent_skill"},
			bindingActionMarkers: map[string][]string{
				"bind":    []string{"add_enabled_skill_ids"},
				"unbind":  []string{"remove_enabled_skill_ids"},
				"replace": []string{"enabled_skill_ids"},
			},
		},
		{
			field:           "agent_memory_enabled",
			aliases:         []string{"agent_memory_enabled"},
			explicitMarkers: []string{"agent_memory_enabled"},
			semanticMarkers: []string{"memory", "agent memory", "\u8bb0\u5fc6"},
		},
		{
			field:           "file_upload_enabled",
			aliases:         []string{"file_upload_enabled"},
			explicitMarkers: []string{"file_upload_enabled"},
			semanticMarkers: []string{"file upload", "\u6587\u4ef6\u4e0a\u4f20"},
		},
		{
			field:           "home_title",
			aliases:         []string{"home_title"},
			explicitMarkers: []string{"home_title"},
			semanticMarkers: []string{"home title", "\u9996\u9875"},
		},
		{
			field:           "input_placeholder",
			aliases:         []string{"input_placeholder"},
			explicitMarkers: []string{"input_placeholder"},
			semanticMarkers: []string{"placeholder", "\u5360\u4f4d"},
		},
		{
			field:           "theme_color",
			aliases:         []string{"theme_color"},
			explicitMarkers: []string{"theme_color"},
			semanticMarkers: []string{"theme", "display", "\u4e3b\u9898", "\u5c55\u793a"},
		},
		{
			field:           "suggested_questions",
			aliases:         []string{"suggested_questions"},
			explicitMarkers: []string{"suggested_questions"},
			semanticMarkers: []string{
				"opening question",
				"suggested question",
				"starter question",
				"example question",
				"\u5f00\u573a\u95ee\u9898",
				"\u5efa\u8bae\u95ee\u9898",
				"\u793a\u4f8b\u95ee\u9898",
				"\u5f15\u5bfc\u95ee\u9898",
			},
		},
		{
			field:           "knowledge_dataset_ids",
			aliases:         []string{"knowledge_dataset_ids", "dataset_ids", "add_knowledge_dataset_ids", "remove_knowledge_dataset_ids", "knowledge_base"},
			explicitMarkers: []string{"knowledge_dataset_ids", "dataset_ids", "add_knowledge_dataset_ids", "remove_knowledge_dataset_ids"},
			bindingActionMarkers: map[string][]string{
				"bind":    []string{"add_knowledge_dataset_ids"},
				"unbind":  []string{"remove_knowledge_dataset_ids"},
				"replace": []string{"knowledge_dataset_ids", "dataset_ids"},
			},
		},
		{
			field:           "knowledge_retrieval_config",
			aliases:         []string{"knowledge_retrieval_config"},
			explicitMarkers: []string{"knowledge_retrieval_config"},
		},
		{
			field:           "database_bindings",
			aliases:         []string{"database_bindings", "add_database_bindings", "remove_database_bindings", "database_table"},
			explicitMarkers: []string{"database_bindings", "add_database_bindings", "remove_database_bindings"},
			bindingActionMarkers: map[string][]string{
				"bind":    []string{"add_database_bindings"},
				"unbind":  []string{"remove_database_bindings"},
				"replace": []string{"database_bindings"},
			},
		},
		{
			field:           "workflow_bindings",
			aliases:         []string{"workflow_bindings", "add_workflow_bindings", "remove_workflow_bindings", "workflow"},
			explicitMarkers: []string{"workflow_bindings", "add_workflow_bindings", "remove_workflow_bindings"},
			bindingActionMarkers: map[string][]string{
				"bind":    []string{"add_workflow_bindings"},
				"unbind":  []string{"remove_workflow_bindings"},
				"replace": []string{"workflow_bindings"},
			},
		},
	}
}

func agentManagementConfigFieldDescriptorForAlias(value string) (agentConfigFieldDescriptor, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return agentConfigFieldDescriptor{}, false
	}
	for _, descriptor := range agentManagementConfigFieldDescriptors() {
		if strings.EqualFold(descriptor.field, value) {
			return descriptor, true
		}
		for _, alias := range descriptor.aliases {
			if strings.EqualFold(strings.TrimSpace(alias), value) {
				return descriptor, true
			}
		}
	}
	return agentConfigFieldDescriptor{}, false
}

func agentManagementExplicitConfigMarkerPresent(text string, marker string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	marker = strings.ToLower(strings.TrimSpace(marker))
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
		if !agentManagementResourceMarkerNegatedInClause(text, absoluteIdx) {
			return true
		}
		searchFrom = absoluteIdx + len(marker)
		if searchFrom >= len(text) {
			return false
		}
	}
}

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

func agentSkillBackedCapabilityDescriptors() []agentSkillBackedCapabilityDescriptor {
	return []agentSkillBackedCapabilityDescriptor{
		{
			DisplayName:    "file generation",
			CandidateQuery: "file generation",
			Markers: []string{
				"file generation",
				"generate file",
				"generate files",
				"create file",
				"create files",
				"generate pdf",
				"create pdf",
				"generate svg",
				"create svg",
				"\u751f\u6210\u6587\u4ef6",
				"\u6587\u4ef6\u751f\u6210",
				"\u521b\u5efa\u6587\u4ef6",
				"\u751f\u6210 pdf",
				"\u521b\u5efa pdf",
				"\u751f\u6210 svg",
				"\u521b\u5efa svg",
			},
		},
		{
			DisplayName:    "chart generation",
			CandidateQuery: "chart",
			Markers: []string{
				"chart",
				"charts",
				"diagram",
				"diagrams",
				"generate chart",
				"generate diagram",
				"\u751f\u6210\u56fe\u8868",
				"\u56fe\u8868\u751f\u6210",
				"\u56fe\u8868",
				"\u751f\u6210\u56fe",
			},
		},
		{
			DisplayName:    "image generation",
			CandidateQuery: "image",
			Markers: []string{
				"image",
				"images",
				"picture",
				"pictures",
				"generate image",
				"generate picture",
				"\u751f\u6210\u56fe\u7247",
				"\u56fe\u7247\u751f\u6210",
				"\u56fe\u7247",
			},
		},
	}
}

func agentManagementConfigOnlyCapabilityDescriptors() []agentConfigOnlyCapabilityDescriptor {
	return []agentConfigOnlyCapabilityDescriptor{
		{
			CapabilityID:       agentCapabilityAcceptUploaded,
			Field:              "file_upload_enabled",
			DisplayName:        "file upload",
			GoalAction:         agentCapabilityActionEnable,
			ContinuationEnable: true,
			Markers: []string{
				"file upload",
				"upload file",
				"upload files",
				"can upload file",
				"can upload files",
				"able to upload file",
				"able to upload files",
				"\u6587\u4ef6\u4e0a\u4f20",
				"\u4e0a\u4f20\u6587\u4ef6",
				"\u80fd\u4e0a\u4f20\u6587\u4ef6",
				"\u80fd\u591f\u4e0a\u4f20\u6587\u4ef6",
			},
			EnableVerify:  []string{"get_agent_config.file_upload_enabled is true"},
			InspectVerify: []string{"get_agent_config reports the current file_upload_enabled state"},
		},
		{
			CapabilityID:       agentCapabilityMemory,
			Field:              "agent_memory_enabled",
			DisplayName:        "agent memory",
			GoalAction:         agentCapabilityActionUpdate,
			ContinuationEnable: true,
			Markers: []string{
				"memory",
				"remember",
				"long-term memory",
				"agent memory",
				"memory capability",
				"can remember",
				"able to remember",
				"\u8bb0\u5fc6",
				"\u8bb0\u4f4f",
				"\u957f\u671f\u8bb0\u5fc6",
				"\u8bb0\u5fc6\u80fd\u529b",
				"\u80fd\u8bb0\u4f4f",
				"\u80fd\u591f\u8bb0\u4f4f",
			},
			EnableVerify:  []string{"get_agent_config returns the requested agent_memory_enabled state"},
			InspectVerify: []string{"get_agent_config reports the current agent_memory_enabled state"},
		},
	}
}

func agentManagementConfigOnlyCapabilityDescriptorForField(field string) (agentConfigOnlyCapabilityDescriptor, bool) {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return agentConfigOnlyCapabilityDescriptor{}, false
	}
	for _, descriptor := range agentManagementConfigOnlyCapabilityDescriptors() {
		if operationPlanAgentConfigCanonicalField(descriptor.Field) == field {
			return descriptor, true
		}
	}
	return agentConfigOnlyCapabilityDescriptor{}, false
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

func agentSkillBackedCapabilityDisplayName(candidateQuery string) string {
	candidateQuery = strings.ToLower(strings.TrimSpace(candidateQuery))
	if candidateQuery == "" {
		return "skill-backed capability"
	}
	for _, descriptor := range agentSkillBackedCapabilityDescriptors() {
		if strings.EqualFold(candidateQuery, descriptor.CandidateQuery) {
			return descriptor.DisplayName
		}
	}
	return "skill-backed capability"
}

func agentManagementConfigOnlyCapabilityMarkers() []string {
	out := []string{}
	for _, descriptor := range agentManagementConfigOnlyCapabilityDescriptors() {
		out = appendUniqueStrings(out, descriptor.Markers...)
	}
	return out
}

func agentManagementCapabilityStatusTargetMarkers() []string {
	markers := []string{
		"agent",
		"skill",
		"tool",
		"model",
		"provider",
		"\u667a\u80fd\u4f53",
		"\u6280\u80fd",
		"\u5de5\u5177",
		"\u6a21\u578b",
		"\u4f9b\u5e94\u5546",
		"\u80fd",
		"\u53ef\u4ee5",
		"\u652f\u6301",
	}
	markers = appendUniqueStrings(markers, agentManagementConfigOnlyCapabilityMarkers()...)
	for _, descriptor := range agentSkillBackedCapabilityDescriptors() {
		markers = appendUniqueStrings(markers, descriptor.Markers...)
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		markers = appendUniqueStrings(markers, descriptor.markers...)
	}
	return markers
}

func agentManagementCapabilityDefinitionsForPrompt() []map[string]interface{} {
	out := []map[string]interface{}{}
	addGoal := func(goal AIChatAgentCapabilityGoal) {
		records := agentCapabilityGoalsToMaps([]AIChatAgentCapabilityGoal{agentCapabilityGoalWithDefaults(goal)})
		if len(records) == 0 {
			return
		}
		out = append(out, records[0])
	}
	addGoal(AIChatAgentCapabilityGoal{
		CapabilityID:         agentCapabilityModelSelection,
		GoalAction:           agentCapabilityActionUpdate,
		DisplayName:          "model selection",
		RequiredConfigFields: []string{"model_provider", "model"},
		CandidateTool:        "list_available_models",
	})
	addGoal(AIChatAgentCapabilityGoal{
		CapabilityID:         agentCapabilitySystemPrompt,
		GoalAction:           agentCapabilityActionUpdate,
		DisplayName:          "system prompt",
		RequiredConfigFields: []string{"system_prompt"},
	})
	addGoal(AIChatAgentCapabilityGoal{
		CapabilityID:         agentCapabilitySuggestedQuestion,
		GoalAction:           agentCapabilityActionUpdate,
		DisplayName:          "suggested questions",
		RequiredConfigFields: []string{"suggested_questions"},
	})
	for _, descriptor := range agentManagementConfigOnlyCapabilityDescriptors() {
		addGoal(AIChatAgentCapabilityGoal{
			CapabilityID:         descriptor.CapabilityID,
			GoalAction:           descriptor.GoalAction,
			DisplayName:          descriptor.DisplayName,
			RequiredConfigFields: []string{descriptor.Field},
			VerifyBy:             append([]string(nil), descriptor.EnableVerify...),
		})
	}

	skillBacked := agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
		CapabilityID:           agentCapabilitySkillBacked,
		GoalAction:             agentCapabilityActionEnable,
		DisplayName:            "skill-backed capability",
		RequiredConfigFields:   []string{"enabled_skill_ids"},
		RequiredBindingActions: map[string]string{"enabled_skill_ids": "bind"},
		CandidateTool:          "list_agent_skill_candidates",
	})
	skillRecords := agentCapabilityGoalsToMaps([]AIChatAgentCapabilityGoal{skillBacked})
	if len(skillRecords) > 0 {
		examples := make([]map[string]string, 0, len(agentSkillBackedCapabilityDescriptors()))
		for _, descriptor := range agentSkillBackedCapabilityDescriptors() {
			examples = append(examples, map[string]string{
				"display_name":    descriptor.DisplayName,
				"candidate_query": descriptor.CandidateQuery,
			})
		}
		skillRecords[0]["examples"] = examples
		out = append(out, skillRecords[0])
	}

	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		goal := agentCapabilityGoalWithDefaults(AIChatAgentCapabilityGoal{
			CapabilityID:           descriptor.capabilityID,
			GoalAction:             agentCapabilityActionBind,
			DisplayName:            descriptor.displayName,
			RequiredBindingActions: map[string]string{descriptor.field: "bind"},
		})
		records := agentCapabilityGoalsToMaps([]AIChatAgentCapabilityGoal{goal})
		if len(records) == 0 {
			continue
		}
		records[0]["binding_kind"] = descriptor.bindingKind
		records[0]["candidate_tools"] = append([]string(nil), descriptor.candidateTools...)
		out = append(out, records[0])
	}
	return out
}

func agentManagementCapabilityGoalsFromModelIntent(intent *AIChatModelTurnIntent) []AIChatAgentCapabilityGoal {
	if intent == nil || len(intent.RecommendedCapabilities) == 0 {
		return nil
	}
	goals := []AIChatAgentCapabilityGoal{}
	for _, hint := range intent.RecommendedCapabilities {
		goal, ok := agentCapabilityGoalFromModelCapabilityHint(hint, intent)
		if !ok {
			continue
		}
		goals = appendAgentCapabilityGoals(goals, goal)
	}
	return goals
}

func agentCapabilityGoalFromModelCapabilityHint(hint string, intent *AIChatModelTurnIntent) (AIChatAgentCapabilityGoal, bool) {
	raw := strings.TrimSpace(hint)
	if raw == "" {
		return AIChatAgentCapabilityGoal{}, false
	}
	parts := strings.Split(raw, ":")
	for idx := range parts {
		parts[idx] = strings.TrimSpace(parts[idx])
	}
	capabilityID := canonicalAgentCapabilityIDHint(parts[0])
	if capabilityID == "" {
		return AIChatAgentCapabilityGoal{}, false
	}
	action := ""
	target := ""
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		if canonical := canonicalAgentCapabilityAction(part); canonical != "" && action == "" {
			action = canonical
			continue
		}
		if target == "" {
			target = part
		}
	}
	userIntent := modelIntentCapabilityUserIntent(intent)
	goal := AIChatAgentCapabilityGoal{
		CapabilityID: capabilityID,
		GoalAction:   action,
		UserIntent:   userIntent,
	}
	switch capabilityID {
	case agentCapabilityModelSelection:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionUpdate)
		goal.DisplayName = "model selection"
		goal.RequiredConfigFields = []string{"model_provider", "model"}
		goal.CandidateTool = "list_available_models"
		goal.CandidateUseCase = firstNonEmptyString(target, "agent_chat")
	case agentCapabilitySystemPrompt:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionUpdate)
		goal.DisplayName = "system prompt"
		goal.RequiredConfigFields = []string{"system_prompt"}
	case agentCapabilitySuggestedQuestion:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionUpdate)
		goal.DisplayName = "suggested_questions"
		goal.RequiredConfigFields = []string{"suggested_questions"}
	case agentCapabilityAcceptUploaded:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionEnable)
		goal.DisplayName = "file upload"
		goal.RequiredConfigFields = []string{"file_upload_enabled"}
	case agentCapabilityMemory:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionUpdate)
		goal.DisplayName = "agent memory"
		goal.RequiredConfigFields = []string{"agent_memory_enabled"}
	case agentCapabilitySkillBacked:
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionEnable)
		goal.DisplayName = agentSkillBackedCapabilityDisplayName(target)
		goal.RequiredConfigFields = []string{"enabled_skill_ids"}
		if target != "" {
			goal.CandidateTool = "list_agent_skill_candidates"
			goal.CandidateQuery = target
		}
		if canonicalAgentCapabilityAction(goal.GoalAction) != agentCapabilityActionInspect {
			if action := operationPlanCanonicalAgentConfigBindingAction(goal.GoalAction); action != "" {
				goal.RequiredBindingActions = map[string]string{"enabled_skill_ids": action}
			}
		}
	case agentCapabilityKnowledgeBinding, agentCapabilityDatabaseBinding, agentCapabilityWorkflowBinding:
		field := operationPlanAgentResourceBindingFieldForCapability(capabilityID)
		if field == "" {
			return AIChatAgentCapabilityGoal{}, false
		}
		goal.GoalAction = firstNonEmptyString(goal.GoalAction, agentCapabilityActionBind)
		goal.DisplayName = field
		goal.RequiredConfigFields = []string{field}
		if canonicalAgentCapabilityAction(goal.GoalAction) != agentCapabilityActionInspect {
			goal.RequiredBindingActions = map[string]string{field: operationPlanCanonicalAgentConfigBindingAction(goal.GoalAction)}
		}
	}
	goal.GoalAction = canonicalAgentCapabilityAction(goal.GoalAction)
	if goal.GoalAction == "" {
		goal.GoalAction = agentCapabilityActionUpdate
	}
	return agentCapabilityGoalWithDefaults(goal), true
}

func canonicalAgentCapabilityIDHint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	switch value {
	case agentCapabilityModelSelection, "agent.model", "agent.model_provider", "model", "model_selection", "llm_model":
		return agentCapabilityModelSelection
	case agentCapabilitySystemPrompt, "agent.prompt", "system_prompt", "prompt":
		return agentCapabilitySystemPrompt
	case agentCapabilitySkillBacked, "agent.skill", "agent.skills", "agent.skill_binding", "agent.skill_bindings", "agent.tool", "agent.tools", "skill", "skills", "skill_binding", "skill_bindings", "tool", "tools", "skill_backed_capability", "tool_backed_capability":
		return agentCapabilitySkillBacked
	case agentCapabilityAcceptUploaded, "agent.file_upload", "agent.uploaded_files", "file_upload", "accept_uploaded_files", "uploaded_files":
		return agentCapabilityAcceptUploaded
	case agentCapabilityMemory, "agent.agent_memory", "memory", "agent_memory":
		return agentCapabilityMemory
	case agentCapabilityKnowledgeBinding, "agent.knowledge", "knowledge", "knowledge_binding", "knowledge_base", "knowledge_dataset":
		return agentCapabilityKnowledgeBinding
	case agentCapabilityDatabaseBinding, "agent.database", "database", "database_binding", "database_table":
		return agentCapabilityDatabaseBinding
	case agentCapabilityWorkflowBinding, "agent.workflow", "workflow", "workflow_binding":
		return agentCapabilityWorkflowBinding
	case agentCapabilitySuggestedQuestion, "agent.suggested_question", "agent.opening_questions", "suggested_questions", "opening_questions", "starter_questions":
		return agentCapabilitySuggestedQuestion
	default:
		return ""
	}
}

func modelIntentCapabilityUserIntent(intent *AIChatModelTurnIntent) string {
	if intent == nil {
		return ""
	}
	if reason := strings.TrimSpace(intent.Reason); reason != "" {
		return truncateRunes(reason, 240)
	}
	if len(intent.Phases) > 0 {
		return truncateRunes(strings.Join(intent.Phases, "; "), 240)
	}
	return truncateRunes(strings.TrimSpace(intent.TaskType), 240)
}

type agentBindingCapabilityDescriptor struct {
	capabilityID   string
	field          string
	bindingKind    string
	displayName    string
	resourceName   string
	meaning        string
	resolveBy      string
	noopKey        string
	markers        []string
	candidateTools []string
	mutationTool   string
}

type agentBindingToolDescriptor struct {
	field       string
	bindingKind string
	toolName    string
}

func agentBindingToolDescriptors() []agentBindingToolDescriptor {
	out := []agentBindingToolDescriptor{{
		field:       "enabled_skill_ids",
		bindingKind: "agent_skill",
		toolName:    "replace_agent_skill_bindings",
	}}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		if descriptor.field == "" || descriptor.bindingKind == "" || descriptor.mutationTool == "" {
			continue
		}
		out = append(out, agentBindingToolDescriptor{
			field:       descriptor.field,
			bindingKind: descriptor.bindingKind,
			toolName:    descriptor.mutationTool,
		})
	}
	return out
}

func agentBindingToolDescriptorForTool(toolName string) (agentBindingToolDescriptor, bool) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return agentBindingToolDescriptor{}, false
	}
	for _, descriptor := range agentBindingToolDescriptors() {
		if strings.EqualFold(descriptor.toolName, toolName) {
			return descriptor, true
		}
	}
	return agentBindingToolDescriptor{}, false
}

func agentBindingToolDescriptorForField(field string) (agentBindingToolDescriptor, bool) {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return agentBindingToolDescriptor{}, false
	}
	for _, descriptor := range agentBindingToolDescriptors() {
		if operationPlanAgentConfigCanonicalField(descriptor.field) == field {
			return descriptor, true
		}
	}
	return agentBindingToolDescriptor{}, false
}

func agentManagementBindingCapabilityDescriptors() []agentBindingCapabilityDescriptor {
	return []agentBindingCapabilityDescriptor{
		{
			capabilityID:   agentCapabilityKnowledgeBinding,
			field:          "knowledge_dataset_ids",
			bindingKind:    "knowledge_base",
			displayName:    "knowledge base binding",
			resourceName:   "knowledge bases",
			meaning:        "Binds knowledge bases as Agent context; it does not create or edit knowledge bases.",
			resolveBy:      "resolve the target knowledge base IDs when not already known",
			noopKey:        "knowledge",
			markers:        []string{"knowledge", "knowledge base", "knowledge bases", "\u77e5\u8bc6\u5e93"},
			candidateTools: []string{"list_agent_knowledge_candidates"},
			mutationTool:   "replace_agent_knowledge_bindings",
		},
		{
			capabilityID:   agentCapabilityDatabaseBinding,
			field:          "database_bindings",
			bindingKind:    "database_table",
			displayName:    "database table binding",
			resourceName:   "database tables",
			meaning:        "Binds database tables for Agent data access; it does not create or edit database tables.",
			resolveBy:      "resolve the target database table bindings when not already known",
			noopKey:        "database",
			markers:        []string{"database", "database table", "database tables", "table", "tables", "\u6570\u636e\u5e93", "\u6570\u636e\u8868"},
			candidateTools: []string{"list_agent_database_candidates", "list_agent_database_tables"},
			mutationTool:   "replace_agent_database_bindings",
		},
		{
			capabilityID:   agentCapabilityWorkflowBinding,
			field:          "workflow_bindings",
			bindingKind:    "workflow",
			displayName:    "workflow binding",
			resourceName:   "workflows",
			meaning:        "Binds workflows so the Agent can call them; it does not publish or edit workflow definitions.",
			resolveBy:      "resolve the target workflow bindings when not already known",
			noopKey:        "workflow",
			markers:        []string{"workflow", "workflows", "\u5de5\u4f5c\u6d41"},
			candidateTools: []string{"list_agent_workflow_binding_candidates"},
			mutationTool:   "replace_agent_workflow_bindings",
		},
	}
}

func agentManagementBindingCapabilityDescriptorForField(field string) (agentBindingCapabilityDescriptor, bool) {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return agentBindingCapabilityDescriptor{}, false
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		if operationPlanAgentConfigCanonicalField(descriptor.field) == field {
			return descriptor, true
		}
	}
	return agentBindingCapabilityDescriptor{}, false
}

func agentManagementBindingCapabilityDescriptorForCapability(capabilityID string) (agentBindingCapabilityDescriptor, bool) {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return agentBindingCapabilityDescriptor{}, false
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		if strings.EqualFold(descriptor.capabilityID, capabilityID) {
			return descriptor, true
		}
	}
	return agentBindingCapabilityDescriptor{}, false
}

func agentManagementBindingCapabilityCandidateTools(field string) []string {
	descriptor, ok := agentManagementBindingCapabilityDescriptorForField(field)
	if !ok {
		return nil
	}
	return append([]string(nil), descriptor.candidateTools...)
}

func agentManagementBindingCapabilityResourceMarkers() []string {
	out := []string{}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		out = appendUniqueStrings(out, descriptor.markers...)
	}
	return out
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

func canonicalAgentCapabilityConfigFields(fields []string) []string {
	out := []string{}
	for _, field := range fields {
		if canonical := operationPlanAgentConfigCanonicalField(field); canonical != "" {
			out = appendUniqueStrings(out, canonical)
		}
	}
	return out
}

func canonicalAgentCapabilityBindingActions(actions map[string]string) map[string]string {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]string{}
	for field, action := range actions {
		canonicalField := operationPlanAgentConfigCanonicalField(field)
		canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
		if canonicalField == "" || canonicalAction == "" {
			continue
		}
		out[canonicalField] = canonicalAction
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func agentCapabilityGoalsExpectedConfigFields(goals []AIChatAgentCapabilityGoal) []string {
	fields := []string{}
	for _, goal := range goals {
		if !agentCapabilityGoalContributesExpectedConfigFields(goal) {
			continue
		}
		fields = appendUniqueStrings(fields, goal.RequiredConfigFields...)
	}
	return canonicalAgentCapabilityConfigFields(fields)
}

func agentCapabilityGoalContributesExpectedConfigFields(goal AIChatAgentCapabilityGoal) bool {
	if _, ok := agentManagementBindingCapabilityDescriptorForCapability(goal.CapabilityID); ok {
		return false
	}
	return true
}

func agentCapabilityGoalWithDefaults(goal AIChatAgentCapabilityGoal) AIChatAgentCapabilityGoal {
	switch goal.CapabilityID {
	case agentCapabilityModelSelection:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Selects the provider/model pair that powers the Agent at runtime.")
		if useCase := strings.TrimSpace(goal.CandidateUseCase); useCase != "" {
			goal.EnableBy = appendUniqueStrings(goal.EnableBy,
				"list_available_models with use_case="+useCase,
			)
		}
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			"resolve a valid provider/model pair",
			"update_agent_config model_provider and model together",
			"verify get_agent_config returns the selected pair",
		)
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"model_name_without_provider",
			"list_available_models_only",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config.model_provider and get_agent_config.model match the selected pair",
		)
	case agentCapabilitySystemPrompt:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Defines the Agent persona and behavioral instructions; it does not grant tools or data access.")
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			"update_agent_config system_prompt",
			"verify get_agent_config returns a matching system_prompt",
		)
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"read_current_config_only",
			"tool_binding_or_resource_access_claim",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config.system_prompt matches the requested role or instruction",
		)
	case agentCapabilityAcceptUploaded:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Allows the Agent chat experience to accept user-uploaded files; it does not generate files or access file-manager assets by itself.")
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			"update_agent_config file_upload_enabled=true",
			"verify get_agent_config.file_upload_enabled is true",
		)
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"system_prompt_only",
			"skill_binding_only",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config.file_upload_enabled is true",
		)
	case agentCapabilitySkillBacked:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Grants a tool-backed Agent capability only when a matching Skill is enabled in enabled_skill_ids.")
		switch canonicalAgentCapabilityAction(goal.GoalAction) {
		case agentCapabilityActionUnbind:
			goal.EnableBy = appendUniqueStrings(goal.EnableBy,
				"read get_agent_config.enabled_skill_ids before changing bindings when exact current bindings are needed",
				"update_agent_config enabled_skill_ids without the removed Skill",
				"verify get_agent_config.enabled_skill_ids no longer contains the removed Skill",
			)
			goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
				"get_agent_config.enabled_skill_ids no longer contains the removed Skill",
			)
		default:
			goal.EnableBy = appendUniqueStrings(goal.EnableBy,
				"list_agent_skill_candidates for the requested capability",
				"update_agent_config enabled_skill_ids with the selected Skill",
				"verify get_agent_config.enabled_skill_ids contains the selected Skill",
			)
			goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
				"get_agent_config.enabled_skill_ids contains the selected candidate Skill",
			)
		}
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"candidate_lookup_only",
			"natural_language_claim_only",
		)
	case agentCapabilityMemory:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Controls whether the Agent can use its memory feature; prompt text alone is not persistent memory.")
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			"update_agent_config agent_memory_enabled",
			"verify get_agent_config.agent_memory_enabled reports the requested state",
		)
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"system_prompt_only",
			"natural_language_claim_only",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config.agent_memory_enabled reports the requested state",
		)
	case agentCapabilitySuggestedQuestion:
		goal.Meaning = firstNonEmptyString(goal.Meaning, "Controls suggested starter questions shown to users; it does not change tools, model, or data access.")
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			"update_agent_config suggested_questions",
			"verify get_agent_config returns the requested suggested_questions",
		)
		goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
			"read_current_config_only",
			"natural_language_claim_only",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config.suggested_questions reflects the requested starter questions",
		)
	default:
		goal = agentCapabilityGoalWithBindingDefaults(goal)
	}
	return goal
}

func agentCapabilityGoalWithBindingDefaults(goal AIChatAgentCapabilityGoal) AIChatAgentCapabilityGoal {
	descriptor, ok := agentManagementBindingCapabilityDescriptorForCapability(goal.CapabilityID)
	if !ok {
		return goal
	}
	field := operationPlanAgentConfigCanonicalField(descriptor.field)
	goal.Meaning = firstNonEmptyString(goal.Meaning, descriptor.meaning)
	if field != "" {
		goal.EnableBy = appendUniqueStrings(goal.EnableBy,
			firstNonEmptyString(descriptor.resolveBy, "resolve the target "+firstNonEmptyString(descriptor.resourceName, descriptor.displayName, field)+" when not already known"),
			"update_agent_config "+field+" with the requested binding action",
			"verify get_agent_config."+field+" reflects the requested binding state",
		)
		goal.VerifyBy = appendUniqueStrings(goal.VerifyBy,
			"get_agent_config."+field+" reflects the requested binding state",
		)
	}
	goal.NotSufficient = appendUniqueStrings(goal.NotSufficient,
		"current_config_read_only",
		"candidate_lookup_only",
		"natural_language_claim_only",
	)
	return goal
}

func agentCapabilityGoalsExpectedBindingActions(goals []AIChatAgentCapabilityGoal) map[string]string {
	out := map[string]string{}
	for _, goal := range goals {
		out = mergeAgentCapabilityGoalExpectedBindingActions(out, goal)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeAgentCapabilityGoalExpectedBindingActions(out map[string]string, goal AIChatAgentCapabilityGoal) map[string]string {
	if canonicalAgentCapabilityAction(goal.GoalAction) == agentCapabilityActionInspect {
		return out
	}
	if out == nil {
		out = map[string]string{}
	}
	for field, action := range goal.RequiredBindingActions {
		canonicalField := operationPlanAgentConfigCanonicalField(field)
		canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
		if canonicalField == "" || canonicalAction == "" {
			continue
		}
		if operationPlanCanonicalAgentConfigBindingAction(out[canonicalField]) == "" {
			out[canonicalField] = canonicalAction
		}
	}
	return out
}

func canonicalAgentCapabilityAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case agentCapabilityActionInspect, "read", "query", "status":
		return agentCapabilityActionInspect
	case agentCapabilityActionEnable:
		return agentCapabilityActionEnable
	case agentCapabilityActionUpdate, "set", "configure":
		return agentCapabilityActionUpdate
	case agentCapabilityActionBind, "add", "associate":
		return agentCapabilityActionBind
	case agentCapabilityActionUnbind, "remove", "detach", "disable":
		return agentCapabilityActionUnbind
	case agentCapabilityActionReplace, "switch":
		return agentCapabilityActionReplace
	default:
		return ""
	}
}

func agentManagementCapabilityGoalsNeedSkillCandidateLookup(goals []AIChatAgentCapabilityGoal) bool {
	return agentManagementSkillCandidateQueryForCapabilityGoals(goals) != ""
}

func agentManagementSkillCandidateQueryForCapabilityGoals(goals []AIChatAgentCapabilityGoal) string {
	for _, goal := range goals {
		if !strings.EqualFold(strings.TrimSpace(goal.CandidateTool), "list_agent_skill_candidates") {
			continue
		}
		if candidateQuery := strings.TrimSpace(goal.CandidateQuery); candidateQuery != "" {
			return candidateQuery
		}
	}
	return ""
}

func agentManagementCapabilityGoalsNeedPostUpdateRead(goals []AIChatAgentCapabilityGoal) bool {
	for _, goal := range goals {
		if len(goal.RequiredConfigFields) > 0 || len(goal.RequiredBindingActions) > 0 {
			return true
		}
	}
	return false
}

func agentCapabilityGoalsRequireConfigMutation(goals []AIChatAgentCapabilityGoal) bool {
	for _, goal := range goals {
		if canonicalAgentCapabilityAction(goal.GoalAction) == agentCapabilityActionInspect {
			continue
		}
		if len(goal.RequiredConfigFields) > 0 || len(goal.RequiredBindingActions) > 0 {
			return true
		}
	}
	return false
}

func agentCapabilityGoalsAreExplicitReadOnly(goals []AIChatAgentCapabilityGoal) bool {
	if len(goals) == 0 {
		return false
	}
	hasInspectGoal := false
	for _, goal := range goals {
		switch canonicalAgentCapabilityAction(goal.GoalAction) {
		case agentCapabilityActionInspect:
			hasInspectGoal = true
		case "":
			if len(goal.RequiredConfigFields) > 0 || len(goal.RequiredBindingActions) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return hasInspectGoal
}

func appendAgentCapabilityGoals(current []AIChatAgentCapabilityGoal, additions ...AIChatAgentCapabilityGoal) []AIChatAgentCapabilityGoal {
	out := append([]AIChatAgentCapabilityGoal(nil), current...)
	for _, goal := range additions {
		goal.CapabilityID = strings.TrimSpace(goal.CapabilityID)
		if goal.CapabilityID == "" {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing.CapabilityID == goal.CapabilityID &&
				strings.EqualFold(existing.CandidateQuery, goal.CandidateQuery) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		out = append(out, goal)
	}
	return out
}

func agentCapabilityGoalSuccessCriteria(goals []AIChatAgentCapabilityGoal) []string {
	criteria := []string{}
	for _, goal := range goals {
		if goal.CapabilityID == "" {
			continue
		}
		if len(goal.VerifyBy) == 0 {
			criteria = appendUniqueStrings(criteria, "verify Agent capability goal before final answer: "+goal.CapabilityID)
			continue
		}
		for _, verify := range goal.VerifyBy {
			verify = strings.TrimSpace(verify)
			if verify != "" {
				criteria = appendUniqueStrings(criteria, "verify Agent capability goal "+goal.CapabilityID+": "+verify)
			}
		}
	}
	return criteria
}

func agentCapabilityGoalsToMaps(goals []AIChatAgentCapabilityGoal) []map[string]interface{} {
	if len(goals) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(goals))
	for _, goal := range goals {
		record := map[string]interface{}{}
		if value := strings.TrimSpace(goal.CapabilityID); value != "" {
			record["capability_id"] = value
		}
		if value := canonicalAgentCapabilityAction(goal.GoalAction); value != "" {
			record["goal_action"] = value
		}
		if value := strings.TrimSpace(goal.DisplayName); value != "" {
			record["display_name"] = value
		}
		if value := strings.TrimSpace(goal.Meaning); value != "" {
			record["meaning"] = value
		}
		if value := strings.TrimSpace(goal.UserIntent); value != "" {
			record["user_intent"] = value
		}
		if fields := canonicalAgentCapabilityConfigFields(goal.RequiredConfigFields); len(fields) > 0 {
			record["required_config_fields"] = fields
		}
		if actions := canonicalAgentCapabilityBindingActions(goal.RequiredBindingActions); len(actions) > 0 {
			record["required_binding_actions"] = actions
		}
		if value := strings.TrimSpace(goal.CandidateTool); value != "" {
			record["candidate_tool"] = value
		}
		if value := strings.TrimSpace(goal.CandidateQuery); value != "" {
			record["candidate_query"] = value
		}
		if value := strings.TrimSpace(goal.CandidateUseCase); value != "" {
			record["candidate_use_case"] = value
		}
		if values := compactStringSliceForPrompt(goal.EnableBy, 8, 180); len(values) > 0 {
			record["enable_by"] = values
		}
		if values := compactStringSliceForPrompt(goal.NotSufficient, 8, 120); len(values) > 0 {
			record["not_sufficient"] = values
		}
		if values := compactStringSliceForPrompt(goal.VerifyBy, 8, 180); len(values) > 0 {
			record["verify_by"] = values
		}
		if len(record) > 0 {
			out = append(out, record)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func agentCapabilityGoalsFromMaps(value interface{}) []AIChatAgentCapabilityGoal {
	records := mapSliceFromAny(value)
	if len(records) == 0 {
		return nil
	}
	var out []AIChatAgentCapabilityGoal
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		goal := AIChatAgentCapabilityGoal{
			CapabilityID:           stringFromAny(record["capability_id"]),
			GoalAction:             stringFromAny(record["goal_action"]),
			DisplayName:            stringFromAny(record["display_name"]),
			Meaning:                stringFromAny(record["meaning"]),
			UserIntent:             stringFromAny(record["user_intent"]),
			RequiredConfigFields:   stringSliceFromAny(record["required_config_fields"]),
			RequiredBindingActions: operationPlanAgentConfigBindingActionsFromAny(record["required_binding_actions"]),
			CandidateTool:          stringFromAny(record["candidate_tool"]),
			CandidateQuery:         stringFromAny(record["candidate_query"]),
			CandidateUseCase:       stringFromAny(record["candidate_use_case"]),
			EnableBy:               stringSliceFromAny(record["enable_by"]),
			NotSufficient:          stringSliceFromAny(record["not_sufficient"]),
			VerifyBy:               stringSliceFromAny(record["verify_by"]),
		}
		out = appendAgentCapabilityGoals(out, goal)
	}
	return out
}

func agentCapabilityGoalsFromOperationPlan(plan map[string]interface{}) []AIChatAgentCapabilityGoal {
	if len(plan) == 0 {
		return nil
	}
	if goals := agentCapabilityGoalsFromMaps(plan["capability_goals"]); len(goals) > 0 {
		return goals
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
		return agentCapabilityGoalsFromMaps(structured["capability_goals"])
	}
	return nil
}

func operationPlanCompactCapabilityGoals(value interface{}, limit int) []interface{} {
	if limit <= 0 {
		return nil
	}
	goals := mapSliceFromAny(value)
	if len(goals) == 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(limit, len(goals)))
	for _, goal := range goals {
		compact := map[string]interface{}{}
		for _, key := range []string{"capability_id", "goal_action", "display_name", "meaning", "user_intent", "candidate_query", "candidate_use_case"} {
			if value := strings.TrimSpace(stringFromAny(goal[key])); value != "" {
				compact[key] = compactForPrompt(value, 240)
			}
		}
		if fields := stringSliceFromAny(goal["required_config_fields"]); len(fields) > 0 {
			compact["required_config_fields"] = compactStringSliceForPrompt(fields, 8, 120)
		}
		if actions := mapFromOperationContext(goal["required_binding_actions"]); len(actions) > 0 {
			compact["required_binding_actions"] = actions
		}
		if values := stringSliceFromAny(goal["enable_by"]); len(values) > 0 {
			compact["enable_by"] = compactStringSliceForPrompt(values, 8, 180)
		}
		if values := stringSliceFromAny(goal["not_sufficient"]); len(values) > 0 {
			compact["not_sufficient"] = compactStringSliceForPrompt(values, 8, 120)
		}
		if values := stringSliceFromAny(goal["verify_by"]); len(values) > 0 {
			compact["verify_by"] = compactStringSliceForPrompt(values, 8, 180)
		}
		if len(compact) == 0 {
			continue
		}
		out = append(out, compact)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
