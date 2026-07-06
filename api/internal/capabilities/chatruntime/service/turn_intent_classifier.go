package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const modelTurnIntentMinimumConfidence = 0.5

type AIChatModelTurnIntent struct {
	Intent           string   `json:"intent"`
	TargetPage       string   `json:"target_page,omitempty"`
	RouteRequired    *bool    `json:"route_required,omitempty"`
	AssetEffect      string   `json:"asset_effect,omitempty"`
	AssetRisk        string   `json:"asset_risk,omitempty"`
	Approval         string   `json:"approval,omitempty"`
	PrimarySkills    []string `json:"primary_skills,omitempty"`
	SupportingSkills []string `json:"supporting_skills,omitempty"`
	Confidence       float64  `json:"confidence,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

func (s *service) applyContextualAIChatModelTurnIntent(ctx context.Context, scope Scope, conversation *runtimemodel.Conversation, config RunConfig, parts *chatRequestParts) {
	if s == nil || s.llmClient == nil || conversation == nil || parts == nil {
		return
	}
	if parts.ModelTurnIntent != nil || strings.TrimSpace(parts.ModelTurnIntentError) != "" {
		return
	}
	if !isContextualAIChatSurface(parts.Surface) || !chatPartsSkillsEnabled(parts) {
		return
	}
	intent, err := s.classifyContextualAIChatTurnIntent(ctx, scope, conversation, config, parts)
	if err != nil {
		parts.ModelTurnIntentError = err.Error()
		logger.DebugContext(ctx, "aichat model turn intent classification skipped",
			"conversation_id", conversation.ID.String(),
			"error", err.Error(),
		)
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
	maxTokens := 260
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
					"You classify one ZGI sidebar assistant turn. Return JSON only.",
					"Do not choose concrete tool arguments. Do not create an execution plan.",
					"Pick exactly one intent from:",
					"- answer_or_explain_zgi_context: answer questions about ZGI, the current page, or assistant capabilities without asset mutation.",
					"- navigate_console_page: the user mainly asks to open or switch to a ZGI console page.",
					"- manage_agent_asset: create, edit, delete, inspect, bind, unbind, configure, or verify Agents.",
					"- read_visible_file_content: read or summarize an existing visible/workspace file.",
					"- delete_visible_file: delete an existing visible/workspace file.",
					"- save_generated_file_to_file_management: save a generated/external artifact into File Management.",
					"- generate_temporary_file_artifact: generate an artifact only for the chat response, not File Management.",
					"- continue_previous_task: continue, retry, resume, or finish a previously paused operation.",
					"Respond with keys: intent, confidence, reason, target_page, route_required, asset_effect, asset_risk, approval, primary_skills, supporting_skills.",
					"Use confidence from 0 to 1. If unsure, choose the closest intent with confidence below 0.5.",
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
	content := strings.TrimSpace(messageContentText(resp.Choices[0].Message.Content))
	if content == "" {
		return nil, fmt.Errorf("empty classifier content")
	}
	var intent AIChatModelTurnIntent
	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		return nil, fmt.Errorf("parse classifier json: %w", err)
	}
	intent.Intent = normalizeModelTurnIntent(intent.Intent)
	if intent.Intent == "" {
		return nil, fmt.Errorf("unsupported classifier intent")
	}
	intent.TargetPage = strings.TrimSpace(intent.TargetPage)
	intent.AssetEffect = strings.TrimSpace(intent.AssetEffect)
	intent.AssetRisk = strings.TrimSpace(intent.AssetRisk)
	intent.Approval = strings.TrimSpace(intent.Approval)
	intent.PrimarySkills = normalizedSkillIDs(intent.PrimarySkills)
	intent.SupportingSkills = normalizedSkillIDs(intent.SupportingSkills)
	intent.Reason = trimRunes(strings.TrimSpace(intent.Reason), 240)
	if intent.Confidence < modelTurnIntentMinimumConfidence {
		return nil, fmt.Errorf("classifier confidence %.2f below %.2f for %s", intent.Confidence, modelTurnIntentMinimumConfidence, intent.Intent)
	}
	return &intent, nil
}

func contextualTurnIntentClassifierPayload(parts *chatRequestParts) map[string]interface{} {
	payload := map[string]interface{}{
		"user_request":   strings.TrimSpace(parts.Query),
		"surface":        normalizeAIChatSurface(parts.Surface),
		"current_page":   contextualTurnCurrentPage(parts),
		"enabled_skills": append([]string(nil), parts.SkillIDs...),
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
	applyModelTurnIntentHints(strategy, intent)
	switch intent.Intent {
	case "manage_agent_asset":
		if !skillIDEnabled(parts.SkillIDs, skills.SkillAgentManagement) {
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
		if skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator) {
			strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, skills.SkillConsoleNavigator)
		}
		return strategy, true
	default:
		return strategy, false
	}
}

func applyModelTurnIntentHints(strategy *AIChatTurnStrategy, intent *AIChatModelTurnIntent) {
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
	if len(intent.PrimarySkills) > 0 {
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, intent.PrimarySkills...)
	}
	if len(intent.SupportingSkills) > 0 {
		strategy.SupportingSkills = appendUniqueStrings(strategy.SupportingSkills, intent.SupportingSkills...)
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
