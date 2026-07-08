package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	modelTurnIntentMinimumConfidence = 0.5
	modelTurnIntentMaxTokens         = 5000
	modelTurnIntentPreviewRunes      = 500
)

type modelTurnIntentClassifierError struct {
	message string
	preview string
	err     error
}

func (e *modelTurnIntentClassifierError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *modelTurnIntentClassifierError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type AIChatModelTurnIntent struct {
	Intent                   string   `json:"intent"`
	RawIntent                string   `json:"raw_intent,omitempty"`
	TaskType                 string   `json:"task_type,omitempty"`
	Phases                   []string `json:"phases,omitempty"`
	EvidenceRequired         []string `json:"evidence_required,omitempty"`
	RecommendedCapabilities  []string `json:"recommended_capabilities,omitempty"`
	CompletionCriteria       []string `json:"completion_criteria,omitempty"`
	NeedsExactAgentRuntime   bool     `json:"needs_exact_agent_runtime,omitempty"`
	CurrentContextMaySummary bool     `json:"current_context_may_be_summary,omitempty"`
	OpenCreatedAgentDetail   bool     `json:"open_created_agent_detail,omitempty"`
	TargetPage               string   `json:"target_page,omitempty"`
	TargetVisibleIndex       int      `json:"target_visible_index,omitempty"`
	RouteRequired            *bool    `json:"route_required,omitempty"`
	AssetEffect              string   `json:"asset_effect,omitempty"`
	AssetRisk                string   `json:"asset_risk,omitempty"`
	Approval                 string   `json:"approval,omitempty"`
	Confidence               float64  `json:"confidence,omitempty"`
	LowConfidence            bool     `json:"low_confidence,omitempty"`
	Reason                   string   `json:"reason,omitempty"`
}

func (s *service) applyContextualAIChatModelTurnIntent(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, config RunConfig, parts *chatRequestParts) {
	if !s.shouldClassifyContextualAIChatTurnIntent(conversation, parts) {
		return
	}
	intent, err := s.classifyContextualAIChatTurnIntent(ctx, scope, conversation, config, parts)
	s.applyContextualAIChatModelTurnIntentResult(ctx, conversation, parts, intent, err)
}

func (s *service) shouldClassifyContextualAIChatTurnIntent(conversation *runtimemodel.Conversation, parts *chatRequestParts) bool {
	if s == nil || s.llmClient == nil || conversation == nil || parts == nil {
		return false
	}
	if parts.ModelTurnIntent != nil || strings.TrimSpace(parts.ModelTurnIntentError) != "" {
		return false
	}
	return isContextualAIChatSurface(parts.Surface) && chatPartsSkillsEnabled(parts)
}

func (s *service) applyContextualAIChatModelTurnIntentResult(ctx context.Context, conversation *runtimemodel.Conversation, parts *chatRequestParts, intent *AIChatModelTurnIntent, err error) {
	if parts == nil {
		return
	}
	if err != nil {
		parts.ModelTurnIntentError = err.Error()
		fields := []interface{}{
			"error", err.Error(),
		}
		if conversation != nil {
			fields = append([]interface{}{"conversation_id", conversation.ID.String()}, fields...)
		}
		if classifierErr, ok := err.(*modelTurnIntentClassifierError); ok && strings.TrimSpace(classifierErr.preview) != "" {
			fields = append(fields, "classifier_content_preview", classifierErr.preview)
		}
		logger.DebugContext(ctx, "aichat model turn intent classification skipped", fields...)
		return
	}
	parts.ModelTurnIntent = intent
}

func (s *service) classifyContextualAIChatTurnIntent(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, config RunConfig, parts *chatRequestParts) (*AIChatModelTurnIntent, error) {
	if strings.TrimSpace(parts.ModelName) == "" {
		return nil, fmt.Errorf("model is required")
	}
	payload := contextualTurnIntentClassifierPayload(parts)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	maxTokens := modelTurnIntentMaxTokens
	temperature := 0.0
	resp, err := s.llmClient.AppChat(ctx, newIntentClassifierAppContext(scope, conversation, config), &adapter.ChatRequest{
		Provider:       parts.Provider,
		Model:          parts.ModelName,
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
		Messages: []adapter.Message{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You create a lightweight semantic Turn Contract for one ZGI sidebar assistant turn. Return JSON only.",
					"This is not a tool script. Do not choose concrete tool names, tool arguments, or ordered tool calls.",
					"The intent field is a broad compatibility label only; the phases, evidence_required, recommended_capabilities, and completion_criteria are the real task contract.",
					"Pick exactly one broad compatibility intent label from:",
					"- answer_or_explain_zgi_context: answer questions about ZGI, the current page, or assistant capabilities without asset mutation.",
					"- navigate_console_page: the user mainly asks to open or switch to a ZGI console page.",
					"- manage_agent_asset: create, edit, delete, inspect, bind, unbind, configure, or verify Agents.",
					"- read_visible_file_content: read or summarize an existing visible/workspace file.",
					"- delete_visible_file: delete an existing visible/workspace file.",
					"- save_generated_file_to_file_management: save a generated/external artifact into File Management.",
					"- generate_temporary_file_artifact: generate an artifact only for the chat response, not File Management.",
					"- continue_previous_task: continue, retry, resume, or finish a previously paused operation.",
					"Also describe the user goal as phases and needed evidence. Phases are semantic checkpoints, not mandatory tool order.",
					"Use recommended_capabilities for capabilities the executor may need, such as exact_agent_runtime, visible_file_content, page_navigation, generated_artifact, or asset_mutation.",
					"For generated artifact turns, include chart_artifact for charts/graphs/data visualizations and file_artifact for ordinary documents, SVG/vector files, PDFs, spreadsheets, or text files.",
					"For Agent management turns, include canonical Agent capability IDs in recommended_capabilities when relevant: agent.model_selection, agent.system_prompt, agent.skill_backed_capability:<capability query>, agent.accept_uploaded_files, agent.memory, agent.knowledge_binding, agent.database_binding, agent.workflow_binding, agent.suggested_questions. Use :bind, :unbind, or :replace after binding capability IDs only when the user asks for that action.",
					"When the user refers to a visible current-page item by ordinal such as first, second, top, \u7b2c\u4e00\u4e2a, or \u7b2c\u4e8c\u4e2a, set target_visible_index to the 1-based visible index. Omit it when no visible ordinal is requested.",
					"For Agent creation turns, set open_created_agent_detail=true only when the user explicitly asks to open, enter, edit, configure, or inspect the newly created Agent detail page after creation.",
					"If the user asks for exact Agent prompt/config/runtime analysis and page context may be summary-level, set needs_exact_agent_runtime=true.",
					"Respond with keys: intent, task_type, phases, evidence_required, recommended_capabilities, completion_criteria, needs_exact_agent_runtime, current_context_may_be_summary, open_created_agent_detail, target_visible_index, confidence, reason, target_page, route_required, asset_effect, asset_risk, approval.",
					"Do not output skill IDs or tool names. Tool selection is handled later by the model from enabled tool schemas and latest evidence.",
					"Use confidence from 0 to 1. If unsure, still output the closest compatibility intent with confidence below 0.5 and make the task contract precise.",
				}, "\n"),
			},
			{Role: "user", Content: string(payloadJSON)},
		},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty classifier response")
	}
	intent, content, err := parseModelTurnIntentMessage(resp.Choices[0].Message)
	if err != nil {
		return nil, &modelTurnIntentClassifierError{
			message: err.Error(),
			preview: classifierContentPreview(content),
			err:     err,
		}
	}
	finalizeModelTurnIntent(intent)
	if intent.Intent == "" {
		return nil, fmt.Errorf("unsupported classifier intent")
	}
	return intent, nil
}

func finalizeModelTurnIntent(intent *AIChatModelTurnIntent) {
	if intent == nil {
		return
	}
	rawIntent := strings.TrimSpace(intent.Intent)
	normalizedIntent := normalizeModelTurnIntent(rawIntent)
	unsupportedIntent := rawIntent != "" && normalizedIntent == ""
	if normalizedIntent == "" {
		normalizedIntent = "answer_or_explain_zgi_context"
	}
	intent.Intent = normalizedIntent
	if rawIntent != "" && !strings.EqualFold(rawIntent, normalizedIntent) {
		intent.RawIntent = rawIntent
	}
	intent.TargetPage = strings.TrimSpace(intent.TargetPage)
	if intent.TargetVisibleIndex < 0 {
		intent.TargetVisibleIndex = 0
	}
	intent.TaskType = strings.TrimSpace(intent.TaskType)
	intent.Phases = normalizeModelTurnPlanStrings(intent.Phases, 8, 160)
	intent.EvidenceRequired = normalizeModelTurnPlanStrings(intent.EvidenceRequired, 10, 160)
	intent.RecommendedCapabilities = normalizeModelTurnPlanStrings(intent.RecommendedCapabilities, 10, 120)
	intent.CompletionCriteria = normalizeModelTurnPlanStrings(intent.CompletionCriteria, 8, 180)
	intent.AssetEffect = strings.TrimSpace(intent.AssetEffect)
	intent.AssetRisk = strings.TrimSpace(intent.AssetRisk)
	intent.Approval = strings.TrimSpace(intent.Approval)
	if intent.Confidence < 0 {
		intent.Confidence = 0
	}
	if intent.Confidence > 1 {
		intent.Confidence = 1
	}
	intent.LowConfidence = unsupportedIntent || intent.Confidence < modelTurnIntentMinimumConfidence
	intent.Reason = trimRunes(strings.TrimSpace(intent.Reason), 240)
	if unsupportedIntent && intent.Reason == "" {
		intent.Reason = "unsupported compatibility intent label; using task contract as the source of truth"
	}
}

func parseModelTurnIntentMessage(message adapter.Message) (*AIChatModelTurnIntent, string, error) {
	type candidate struct {
		content string
		source  string
	}
	finalContent := strings.TrimSpace(messageContentText(message.Content))
	reasoningContent := strings.TrimSpace(message.ReasoningContent)
	candidates := make([]candidate, 0, 2)
	if finalContent != "" {
		candidates = append(candidates, candidate{content: finalContent, source: "content"})
	}
	if reasoningContent != "" && classifierJSONText(reasoningContent) != "" {
		candidates = append(candidates, candidate{content: reasoningContent, source: "reasoning_content"})
	}
	if len(candidates) == 0 {
		if reasoningContent != "" {
			return nil, reasoningContent, fmt.Errorf("empty classifier content: reasoning content did not contain json")
		}
		return nil, "", fmt.Errorf("empty classifier content")
	}

	var firstErr error
	var firstContent string
	for _, item := range candidates {
		intent, err := parseModelTurnIntentContent(item.content)
		if err == nil {
			return intent, item.content, nil
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", item.source, err)
			firstContent = item.content
		}
	}
	return nil, firstContent, firstErr
}

func parseModelTurnIntentContent(content string) (*AIChatModelTurnIntent, error) {
	jsonText := classifierJSONText(content)
	if strings.TrimSpace(jsonText) == "" {
		return nil, fmt.Errorf("empty classifier content")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		return nil, fmt.Errorf("parse classifier json: %w", err)
	}
	intent := &AIChatModelTurnIntent{
		Intent:                   jsonRawString(raw["intent"]),
		TaskType:                 firstNonEmptyString(jsonRawString(raw["task_type"]), jsonRawString(raw["goal_type"])),
		Phases:                   jsonRawStringSlice(raw["phases"]),
		EvidenceRequired:         firstNonEmptyStringSlice(jsonRawStringSlice(raw["evidence_required"]), jsonRawStringSlice(raw["needed_evidence"])),
		RecommendedCapabilities:  firstNonEmptyStringSlice(jsonRawStringSlice(raw["recommended_capabilities"]), jsonRawStringSlice(raw["needed_capabilities"])),
		CompletionCriteria:       firstNonEmptyStringSlice(jsonRawStringSlice(raw["completion_criteria"]), jsonRawStringSlice(raw["success_criteria"])),
		NeedsExactAgentRuntime:   jsonRawBool(raw["needs_exact_agent_runtime"]),
		CurrentContextMaySummary: jsonRawBool(raw["current_context_may_be_summary"]),
		OpenCreatedAgentDetail:   jsonRawBool(raw["open_created_agent_detail"]),
		TargetPage:               jsonRawString(raw["target_page"]),
		TargetVisibleIndex: firstPositiveInt(
			jsonRawInt(raw["target_visible_index"]),
			jsonRawInt(raw["visible_index"]),
			jsonRawInt(raw["target_index"]),
		),
		RouteRequired: jsonRawBoolPtr(raw["route_required"]),
		AssetEffect:   jsonRawString(raw["asset_effect"]),
		AssetRisk:     jsonRawString(raw["asset_risk"]),
		Approval:      jsonRawApproval(raw["approval"]),
		Confidence:    jsonRawFloat64(raw["confidence"]),
		Reason:        jsonRawString(raw["reason"]),
	}
	return intent, nil
}

func classifierJSONText(content string) string {
	value := strings.TrimSpace(content)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "```") {
		lines := strings.Split(value, "\n")
		if len(lines) > 1 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				lines = lines[:len(lines)-1]
			}
			value = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if strings.HasPrefix(value, "{") {
		return value
	}
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(value[start : end+1])
	}
	return ""
}

func classifierContentPreview(content string) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	return trimRunes(value, modelTurnIntentPreviewRunes)
}

func jsonRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	var boolValue bool
	if err := json.Unmarshal(raw, &boolValue); err == nil {
		if boolValue {
			return "true"
		}
		return "false"
	}
	var numberValue float64
	if err := json.Unmarshal(raw, &numberValue); err == nil {
		return strconv.FormatFloat(numberValue, 'f', -1, 64)
	}
	return ""
}

func jsonRawApproval(raw json.RawMessage) string {
	value := strings.ToLower(strings.TrimSpace(jsonRawString(raw)))
	switch value {
	case "", "none", "false", "no", "not_required", "not required":
		return "none"
	case "true", "yes", "required", "ask", "approval_required", "requires_approval":
		return "required"
	default:
		return value
	}
}

func jsonRawBoolPtr(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return &value
	}
	text := strings.ToLower(strings.TrimSpace(jsonRawString(raw)))
	switch text {
	case "true", "yes", "required":
		value = true
		return &value
	case "false", "no", "none", "not_required":
		value = false
		return &value
	default:
		return nil
	}
}

func jsonRawBool(raw json.RawMessage) bool {
	if ptr := jsonRawBoolPtr(raw); ptr != nil {
		return *ptr
	}
	return false
}

func jsonRawStringSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	var interfaces []interface{}
	if err := json.Unmarshal(raw, &interfaces); err == nil {
		out := make([]string, 0, len(interfaces))
		for _, item := range interfaces {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	}
	if text := jsonRawString(raw); text != "" {
		return []string{text}
	}
	return nil
}

func normalizeModelTurnPlanStrings(values []string, limit int, maxRunes int) []string {
	if len(values) == 0 || limit <= 0 || maxRunes <= 0 {
		return nil
	}
	out := make([]string, 0, minInt(len(values), limit))
	for _, value := range values {
		value = trimRunes(strings.TrimSpace(value), maxRunes)
		if value == "" {
			continue
		}
		out = appendUniqueStrings(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func jsonRawInt(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var value int
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var numberValue float64
	if err := json.Unmarshal(raw, &numberValue); err == nil {
		return int(numberValue)
	}
	text := strings.TrimSpace(jsonRawString(raw))
	if text == "" {
		return 0
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return parsed
}

func jsonRawFloat64(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	text := strings.TrimSpace(jsonRawString(raw))
	if text == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func contextualTurnIntentClassifierPayload(parts *chatRequestParts) map[string]interface{} {
	payload := map[string]interface{}{
		"user_request": strings.TrimSpace(parts.Query),
		"surface":      normalizeAIChatSurface(parts.Surface),
		"current_page": contextualTurnCurrentPage(parts),
	}
	if parts.ContextControl != nil {
		payload["context_control"] = copyStringAnyMap(parts.ContextControl)
	}
	if parts.RuntimeContext != "" {
		payload["runtime_context"] = trimRunes(parts.RuntimeContext, 3000)
	}
	if parts.RawOperationContext != nil {
		payload["operation_context"] = compactClassifierMap(parts.RawOperationContext, 5000)
	}
	if len(parts.RecentGeneratedArtifacts) > 0 {
		payload["recent_generated_artifacts"] = trimRunes(fmt.Sprint(parts.RecentGeneratedArtifacts), 1000)
	}
	return payload
}

func compactClassifierMap(input map[string]interface{}, maxRunes int) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return copyStringAnyMap(input)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(trimRunes(string(raw), maxRunes)), &out); err == nil {
		return out
	}
	return map[string]interface{}{"json_preview": trimRunes(string(raw), maxRunes)}
}

func newIntentClassifierAppContext(scope Scope, conversation *runtimemodel.Conversation, config RunConfig) *llmclient.AppContext {
	appID := conversation.ID.String()
	if strings.TrimSpace(config.BillingAppID) != "" {
		appID = strings.TrimSpace(config.BillingAppID)
	}
	appType := runtimemodel.MessageBillingReasonSourceAIChat
	if strings.TrimSpace(config.BillingAppType) != "" {
		appType = strings.TrimSpace(config.BillingAppType)
	}
	ctx := &llmclient.AppContext{
		OrganizationID:     scope.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              appID,
		AppType:            appType,
		AccountID:          scope.AccountID.String(),
		SessionID:          conversation.ID.String(),
		ConversationID:     conversation.ID.String(),
	}
	if scope.WorkspaceID != nil {
		ctx.WorkspaceID = scope.WorkspaceID.String()
	} else if conversation.WorkspaceID != nil {
		ctx.WorkspaceID = conversation.WorkspaceID.String()
	}
	return ctx
}

func normalizeModelTurnIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "answer_or_explain_zgi_context", "answer", "explain", "page_qa", "assistant_capabilities":
		return "answer_or_explain_zgi_context"
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
	case "answer_or_explain_zgi_context":
		strategy.ToolChoiceMode = aiChatTurnToolChoiceModelDecides
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
		}
		return strategy, true
	default:
		return strategy, false
	}
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
	if strings.TrimSpace(intent.TaskType) != "" {
		strategy.TaskType = strings.TrimSpace(intent.TaskType)
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
