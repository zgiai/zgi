package service

import (
	"context"
	"fmt"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func (p *PreparedChat) skillsEnabled() bool {
	if p == nil || p.parts == nil {
		return false
	}
	return p.parts.SkillMode != skillModeDisabled && len(p.parts.SkillIDs) > 0
}

func (s *service) runPreparedSkillStream(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onChunk func(string) error,
	onEvent func(StreamEvent) error,
) (string, *adapter.Usage, error) {
	if s.skillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.llmClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
	custom, err := s.customSkillCatalogEntries(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return "", nil, err
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkillsWithCustom(ctx, prepared.parts.SkillIDs, custom)
	if err != nil {
		return "", nil, err
	}
	if len(resolved.Skills) == 0 {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	timeline := newProcessTimelineRecorder(ctx, persistCtx, s, prepared, onEvent)
	runner := &skillloop.Runner{
		LLMClient:    s.llmClient,
		SkillRuntime: s.skillRuntime,
		AppContext:   newBillingAppContext(prepared),
		OnEvent: func(event skillloop.Event) error {
			if event.Type == skillloop.EventUserInputRequested {
				s.persistUserInputRequestBestEffort(persistCtx, prepared, event.Payload)
			}
			timeline.RecordEvent(event.Type, event.Payload)
			return nil
		},
		OnTrace: func(traces []skills.SkillTrace, trace skills.SkillTrace) {
			timeline.RecordTrace(traces, trace)
		},
		OnArtifact: func(artifact map[string]interface{}) {
			s.persistGeneratedArtifactBestEffort(ctx, prepared, artifact)
		},
	}
	return runner.Run(ctx, skillloop.RunRequest{
		Prepared: skillloop.NewPreparedChat(
			prepared.Conversation.ID.String(),
			prepared.Message.ID.String(),
			prepared.parts.Provider,
			prepared.parts.SkillMode,
			prepared.LLMRequest,
		),
		Resolved:         resolved,
		ExecutionContext: s.skillExecutionContext(prepared),
		OnChunk:          onChunk,
	})
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	appID := prepared.Conversation.ID.String()
	if strings.TrimSpace(prepared.RunConfig.BillingAppID) != "" {
		appID = strings.TrimSpace(prepared.RunConfig.BillingAppID)
	}
	invokeFrom := tools.ToolInvokeFromAIChat
	if normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent {
		invokeFrom = tools.ToolInvokeFromAgent
	}
	return skills.ExecutionContext{
		OrganizationID:    prepared.Scope.OrganizationID.String(),
		UserID:            prepared.Scope.AccountID.String(),
		ConversationID:    prepared.Conversation.ID.String(),
		AppID:             appID,
		MessageID:         prepared.Message.ID.String(),
		InvokeFrom:        invokeFrom,
		RuntimeParameters: skillRuntimeParametersForPrepared(prepared),
	}
}

func skillRuntimeParameters(scope Scope, config RunConfig) map[string]interface{} {
	return runtimeCapabilityConfigFromRunConfig(config).RuntimeParameters(scope, config.BillingAppType)
}

func skillRuntimeParametersForPrepared(prepared *PreparedChat) map[string]interface{} {
	params := skillRuntimeParameters(prepared.Scope, prepared.RunConfig)
	if history := workflowConversationHistoryFromPrepared(prepared); len(history) > 0 {
		params["workflow_context"] = map[string]interface{}{
			"conversation_history": history,
		}
	}
	return params
}

func workflowConversationHistoryFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.LLMRequest == nil || len(prepared.LLMRequest.Messages) == 0 {
		return nil
	}
	messages := prepared.LLMRequest.Messages
	lastUserIndex := -1
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if strings.EqualFold(strings.TrimSpace(messages[idx].Role), "user") {
			lastUserIndex = idx
			break
		}
	}
	out := make([]map[string]interface{}, 0, len(messages))
	for idx, message := range messages {
		if idx == lastUserIndex {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(messageContentText(message.Content))
		if content == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func messageContentText(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []adapter.MessageContentPart:
		var builder strings.Builder
		for _, part := range typed {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
		return builder.String()
	case []interface{}:
		var builder strings.Builder
		for _, raw := range typed {
			part, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(part["text"]))
			if text == "" || text == "<nil>" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
		return builder.String()
	default:
		return ""
	}
}

func copyAgentDatabaseBindings(input []AgentDatabaseBinding) []AgentDatabaseBinding {
	out := make([]AgentDatabaseBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.DataSourceID) == "" || len(binding.TableIDs) == 0 {
			continue
		}
		out = append(out, AgentDatabaseBinding{
			DataSourceID:     strings.TrimSpace(binding.DataSourceID),
			TableIDs:         append([]string(nil), binding.TableIDs...),
			WritableTableIDs: append([]string(nil), binding.WritableTableIDs...),
		})
	}
	return out
}

func copyAgentWorkflowBindings(input []AgentWorkflowBinding) []AgentWorkflowBinding {
	out := make([]AgentWorkflowBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.BindingID) == "" || strings.TrimSpace(binding.AgentID) == "" || strings.TrimSpace(binding.WorkflowID) == "" {
			continue
		}
		out = append(out, AgentWorkflowBinding{
			BindingID:       strings.TrimSpace(binding.BindingID),
			Label:           strings.TrimSpace(binding.Label),
			Description:     strings.TrimSpace(binding.Description),
			AgentID:         strings.TrimSpace(binding.AgentID),
			WorkflowID:      strings.TrimSpace(binding.WorkflowID),
			AgentType:       strings.TrimSpace(binding.AgentType),
			VersionStrategy: strings.TrimSpace(binding.VersionStrategy),
			VersionUUID:     strings.TrimSpace(binding.VersionUUID),
			TimeoutSeconds:  binding.TimeoutSeconds,
			StartInputs:     copyAgentWorkflowStartInputs(binding.StartInputs),
			RequiredInputs:  append([]string(nil), binding.RequiredInputs...),
			DefaultInputKey: strings.TrimSpace(binding.DefaultInputKey),
		})
	}
	return out
}

func copyAgentWorkflowStartInputs(input []AgentWorkflowStartInput) []AgentWorkflowStartInput {
	out := make([]AgentWorkflowStartInput, 0, len(input))
	for _, item := range input {
		variable := strings.TrimSpace(item.Variable)
		if variable == "" {
			continue
		}
		out = append(out, AgentWorkflowStartInput{
			Variable: variable,
			Label:    strings.TrimSpace(item.Label),
			Type:     strings.TrimSpace(item.Type),
			Required: item.Required,
		})
	}
	return out
}

func mergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		return &cloned
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	return current
}

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	cloned := *source
	cloned.Messages = append([]adapter.Message{}, source.Messages...)
	cloned.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		cloned.AdditionalParameters = copyStringAnyMap(source.AdditionalParameters)
	}
	if source.LogitBias != nil {
		cloned.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			cloned.LogitBias[key] = value
		}
	}
	return &cloned
}

func agenticSkillLoopSystemMessage() adapter.Message {
	return skillloop.AgenticSkillLoopSystemMessage()
}
