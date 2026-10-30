package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type AIChatModelTurnIntent struct {
	Intent                   string              `json:"intent"`
	RawIntent                string              `json:"raw_intent,omitempty"`
	TaskType                 string              `json:"task_type,omitempty"`
	Outcomes                 []AIChatTurnOutcome `json:"outcomes,omitempty"`
	Phases                   []string            `json:"phases,omitempty"`
	EvidenceRequired         []string            `json:"evidence_required,omitempty"`
	RecommendedCapabilities  []string            `json:"recommended_capabilities,omitempty"`
	CompletionCriteria       []string            `json:"completion_criteria,omitempty"`
	NeedsExactAgentRuntime   bool                `json:"needs_exact_agent_runtime,omitempty"`
	CurrentContextMaySummary bool                `json:"current_context_may_be_summary,omitempty"`
	OpenCreatedAgentDetail   bool                `json:"open_created_agent_detail,omitempty"`
	TargetPage               string              `json:"target_page,omitempty"`
	TargetVisibleIndex       int                 `json:"target_visible_index,omitempty"`
	RouteRequired            *bool               `json:"route_required,omitempty"`
	AssetEffect              string              `json:"asset_effect,omitempty"`
	AssetRisk                string              `json:"asset_risk,omitempty"`
	Approval                 string              `json:"approval,omitempty"`
	Confidence               float64             `json:"confidence,omitempty"`
	LowConfidence            bool                `json:"low_confidence,omitempty"`
	Reason                   string              `json:"reason,omitempty"`
}

func normalizeModelTurnIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "answer_or_explain_console_context", "answer_or_explain_zgi_context", "answer", "explain", "page_qa", "assistant_capabilities":
		return "answer_or_explain_console_context"
	case "navigate_console_page", "navigation", "navigate", "route", "open_page":
		return "navigate_console_page"
	case "manage_agent_asset", "agent_management", "agent", "create_agent", "edit_agent", "delete_agent", "configure_agent":
		return "manage_agent_asset"
	case "read_visible_file_content", "read_file", "file_read", "summarize_file":
		return "read_visible_file_content"
	case "delete_visible_file", "delete_file", "file_delete":
		return "delete_visible_file"
	case "save_generated_file_to_file_management", "create_managed_file", "managed_file_create", "save_file_to_management":
		return "save_generated_file_to_file_management"
	case "generate_temporary_file_artifact", "generate_temporary_file", "temporary_file_generate", "generate_artifact":
		return "generate_temporary_file_artifact"
	case "continue_previous_task", "continue_previous_operation", "continue", "resume", "retry":
		return "continue_previous_task"
	default:
		return ""
	}
}

func contextualAIChatTurnStrategyFromModelIntent(parts *chatRequestParts, strategy *AIChatTurnStrategy, intent *AIChatModelTurnIntent) (*AIChatTurnStrategy, bool) {
	if parts == nil || strategy == nil || intent == nil || strings.TrimSpace(intent.Intent) == "" {
		return strategy, false
	}
	markAIChatTurnStrategySource(strategy, aiChatTurnStrategySourceModelIntent, modelTurnIntentSourceReason(intent))
	applyModelTurnIntentHints(parts, strategy, intent)
	if modelTurnIntentShouldUseGenericTaskContract(intent) {
		return contextualModelTurnTaskContractStrategy(parts, strategy, intent), true
	}
	switch intent.Intent {
	case "manage_agent_asset":
		if !canAcceptAgentModelTurnIntent(parts, intent) {
			return strategy, false
		}
		return contextualAgentManagementStrategy(parts, strategy), true
	case "navigate_console_page":
		return contextualNavigationStrategy(parts, strategy), true
	case "continue_previous_task":
		return contextualContinuationStrategy(parts, strategy), true
	case "save_generated_file_to_file_management":
		return contextualManagedFileCreateStrategy(parts, strategy), true
	case "generate_temporary_file_artifact":
		return contextualTemporaryFileGenerateStrategy(parts, strategy), true
	case "delete_visible_file":
		return contextualFileDeleteStrategy(parts, strategy), true
	case "read_visible_file_content":
		return contextualFileReadStrategy(parts, strategy), true
	case "answer_or_explain_console_context":
		strategy.Intent = intent.Intent
		strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
		}
		return strategy, true
	default:
		return strategy, false
	}
}

func modelTurnIntentShouldUseGenericTaskContract(intent *AIChatModelTurnIntent) bool {
	if intent == nil || !modelTurnIntentHasActionableTaskContract(intent) {
		return false
	}
	return intent.LowConfidence || strings.TrimSpace(intent.RawIntent) != ""
}

func modelTurnIntentHasActionableTaskContract(intent *AIChatModelTurnIntent) bool {
	if intent == nil {
		return false
	}
	if modelTurnIntentAssetEffectIsMutation(intent.AssetEffect) ||
		modelTurnIntentAssetEffectIsDelete(intent.AssetEffect) ||
		modelTurnIntentRequestsTemporaryFileArtifact(intent) ||
		intent.NeedsExactAgentRuntime ||
		intent.OpenCreatedAgentDetail {
		return true
	}
	if intent.RouteRequired != nil && *intent.RouteRequired {
		return true
	}
	for _, capability := range intent.RecommendedCapabilities {
		if modelTurnCapabilityHintRequiresSkillLoop(capability) {
			return true
		}
	}
	for _, outcome := range intent.Outcomes {
		for _, capability := range outcome.Capabilities {
			if modelTurnCapabilityHintRequiresSkillLoop(capability) {
				return true
			}
		}
	}
	return false
}

func modelTurnCapabilityHintRequiresSkillLoop(value string) bool {
	value = canonicalModelTurnCapabilityHint(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "agent.") ||
		strings.HasPrefix(value, "agent_") {
		return true
	}
	switch value {
	case "exact_agent_runtime",
		"visible_file_content",
		"page_navigation",
		"generated_artifact",
		"chart_artifact",
		"data_visualization_artifact",
		"visualization_artifact",
		"file_artifact",
		"document_artifact",
		"svg_artifact",
		"text_artifact",
		"pdf_artifact",
		"spreadsheet_artifact",
		"asset_mutation":
		return true
	default:
		return false
	}
}

func contextualModelTurnTaskContractStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy, intent *AIChatModelTurnIntent) *AIChatTurnStrategy {
	if strategy == nil {
		return strategy
	}
	strategy.Intent = "model_turn_contract"
	strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
	strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
		"complete the model-provided task phases before final answer",
		"use enabled tools from the latest evidence when a phase needs system data or asset mutation",
		"verify the final answer against successful tool, page, or client-action evidence",
	)
	strategy.ObservationPoints = appendUniqueStrings(strategy.ObservationPoints, intent.EvidenceRequired...)
	if intent.RouteRequired != nil && *intent.RouteRequired && skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
	}
	if modelTurnIntentHasRecommendedCapability(intent, "page_navigation") && skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
	}
	if modelTurnIntentNeedsAgentManagementSkill(intent) && skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillAgentManagement)
		strategy = appendAgentManagementModelDecidesGuidance(parts, strategy)
	}
	if modelTurnIntentHasRecommendedCapability(intent, "visible_file_content") && skillIDEnabled(parts.SkillIDs, skills.SkillFileReader) {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillFileReader)
	}
	if modelTurnIntentRequestsTemporaryFileArtifact(intent) {
		strategy.SupportingSkills = appendArtifactProducerSkills(strategy.SupportingSkills, parts)
	}
	return strategy
}

func modelTurnIntentNeedsAgentManagementSkill(intent *AIChatModelTurnIntent) bool {
	if intent == nil {
		return false
	}
	if intent.NeedsExactAgentRuntime || intent.OpenCreatedAgentDetail {
		return true
	}
	for _, capability := range intent.RecommendedCapabilities {
		canonical := canonicalModelTurnCapabilityHint(capability)
		if strings.HasPrefix(canonical, "agent.") ||
			strings.HasPrefix(canonical, "agent_") ||
			canonical == "exact_agent_runtime" {
			return true
		}
	}
	return false
}

func canAcceptAgentModelTurnIntent(parts *chatRequestParts, intent *AIChatModelTurnIntent) bool {
	if parts == nil || intent == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(intent.Intent), "manage_agent_asset") {
		return false
	}
	if skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
		return true
	}
	if !skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
		return false
	}
	return true
}

func modelTurnIntentSourceReason(intent *AIChatModelTurnIntent) string {
	if intent == nil {
		return ""
	}
	if reason := strings.TrimSpace(intent.Reason); reason != "" {
		if intent.LowConfidence {
			return truncateRunes("low_confidence: "+reason, 240)
		}
		return truncateRunes(reason, 240)
	}
	if intentValue := strings.TrimSpace(intent.Intent); intentValue != "" {
		if intent.LowConfidence {
			return "low_confidence_classified_as_" + intentValue
		}
		return "classified_as_" + intentValue
	}
	return ""
}

func modelTurnIntentTaskContract(intent *AIChatModelTurnIntent) map[string]interface{} {
	if intent == nil {
		return nil
	}
	contract := map[string]interface{}{
		"source":        "model_turn_contract",
		"intent_label":  strings.TrimSpace(intent.Intent),
		"confidence":    intent.Confidence,
		"compatibility": "intent_label_is_for_routing_compatibility_only",
	}
	if value := strings.TrimSpace(intent.RawIntent); value != "" {
		contract["raw_intent_label"] = value
	}
	if intent.LowConfidence {
		contract["low_confidence"] = true
	}
	if value := strings.TrimSpace(intent.TaskType); value != "" {
		contract["task_type"] = value
	}
	if len(intent.Phases) > 0 {
		contract["phases"] = append([]string(nil), intent.Phases...)
	}
	if len(intent.Outcomes) > 0 {
		contract["outcomes"] = mapsToInterfaceSlice(operationPlanTurnOutcomesToMaps(intent.Outcomes))
	}
	if len(intent.EvidenceRequired) > 0 {
		contract["evidence_required"] = append([]string(nil), intent.EvidenceRequired...)
	}
	if len(intent.RecommendedCapabilities) > 0 {
		contract["recommended_capabilities"] = append([]string(nil), intent.RecommendedCapabilities...)
	}
	if len(intent.CompletionCriteria) > 0 {
		contract["completion_criteria"] = append([]string(nil), intent.CompletionCriteria...)
	}
	if intent.NeedsExactAgentRuntime {
		contract["needs_exact_agent_runtime"] = true
	}
	if intent.CurrentContextMaySummary {
		contract["current_context_may_be_summary"] = true
	}
	if intent.OpenCreatedAgentDetail {
		contract["open_created_agent_detail"] = true
	}
	if value := strings.TrimSpace(intent.TargetPage); value != "" {
		contract["target_page"] = value
	}
	if intent.TargetVisibleIndex > 0 {
		contract["target_visible_index"] = intent.TargetVisibleIndex
	}
	if intent.RouteRequired != nil {
		contract["route_required"] = *intent.RouteRequired
	}
	if value := strings.TrimSpace(intent.AssetEffect); value != "" {
		contract["asset_effect"] = value
	}
	if value := strings.TrimSpace(intent.AssetRisk); value != "" {
		contract["asset_risk"] = value
	}
	if value := strings.TrimSpace(intent.Approval); value != "" {
		contract["approval"] = value
	}
	if value := strings.TrimSpace(intent.Reason); value != "" {
		contract["reason"] = value
	}
	return contract
}

func applyModelTurnIntentHints(parts *chatRequestParts, strategy *AIChatTurnStrategy, intent *AIChatModelTurnIntent) {
	if compatibilityIntent := strings.TrimSpace(intent.Intent); compatibilityIntent != "" {
		strategy.CompatibilityIntent = compatibilityIntent
	}
	if strings.TrimSpace(intent.TaskType) != "" {
		strategy.TaskType = strings.TrimSpace(intent.TaskType)
	}
	if len(intent.Outcomes) > 0 {
		strategy.Outcomes = append([]AIChatTurnOutcome(nil), intent.Outcomes...)
		for _, outcome := range intent.Outcomes {
			strategy.PhaseGoals = appendUniqueStrings(strategy.PhaseGoals, outcome.Goal)
		}
	}
	if len(intent.Phases) > 0 {
		strategy.PhaseGoals = appendUniqueStrings(strategy.PhaseGoals, intent.Phases...)
	}
	if len(intent.EvidenceRequired) > 0 {
		strategy.EvidenceRequired = appendUniqueStrings(strategy.EvidenceRequired, intent.EvidenceRequired...)
		strategy.ObservationPoints = appendUniqueStrings(strategy.ObservationPoints, intent.EvidenceRequired...)
	}
	if len(intent.RecommendedCapabilities) > 0 {
		strategy.RecommendedCapabilities = appendUniqueStrings(strategy.RecommendedCapabilities, intent.RecommendedCapabilities...)
	}
	if intent.LowConfidence {
		strategy.LowConfidence = true
	}
	if len(intent.CompletionCriteria) > 0 {
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria, intent.CompletionCriteria...)
	}
	if intent.NeedsExactAgentRuntime {
		strategy.NeedsExactAgentRuntime = true
		strategy.RecommendedCapabilities = appendUniqueStrings(strategy.RecommendedCapabilities, "exact_agent_runtime")
		strategy.ObservationPoints = appendUniqueStrings(strategy.ObservationPoints, "exact_agent_runtime_config")
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"when the user asks for actual Agent prompt or runtime configuration, use exact Agent runtime evidence if current page context may be summary-level",
		)
		if parts != nil && skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillAgentManagement)
		}
	}
	if intent.CurrentContextMaySummary {
		strategy.CurrentContextMaySummary = true
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"do not treat summary page context as complete evidence when the user asks for exact configuration",
		)
	}
	if intent.OpenCreatedAgentDetail {
		strategy.OpenCreatedAgentDetail = true
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria,
			"after create_agent succeeds, open the newly created Agent detail page before claiming the page is open",
		)
	}
	if strings.TrimSpace(intent.TargetPage) != "" {
		strategy.TargetPage = strings.TrimSpace(intent.TargetPage)
	}
	if intent.RouteRequired != nil {
		strategy.RouteRequired = *intent.RouteRequired
	}
	if strings.TrimSpace(intent.AssetEffect) != "" {
		strategy.AssetEffect = strings.TrimSpace(intent.AssetEffect)
	}
	if strings.TrimSpace(intent.AssetRisk) != "" {
		strategy.AssetRisk = strings.TrimSpace(intent.AssetRisk)
	}
	if strings.TrimSpace(intent.Approval) != "" {
		strategy.Approval = strings.TrimSpace(intent.Approval)
	}
}

func trimRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
