package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultMaxSkillPlanningRounds            = 50
	defaultMaxSkillStepsPerTurn              = 160
	defaultMaxBusinessToolCallsPerSkill      = 20
	defaultMaxRecoverableSkillFailures       = 20
	defaultMaxConsecutiveRecoverableFailures = 5
	intermediateAnswerChunkRunes             = 180
	streamedIntermediateAnswerArg            = "_aichat_streamed_answer"
)

var skillPlanningFallbackProgressDelay = 800 * time.Millisecond

type skillStepResult struct {
	trace       skills.SkillTrace
	toolMessage adapter.Message
	usedSkill   bool
	usedTool    bool
	recoverable bool
	fatalErr    error
}

type planningResult struct {
	message          adapter.Message
	usage            *adapter.Usage
	answerStreamed   bool
	progressStreamed bool
}

type streamingToolCallState struct {
	call                    adapter.ToolCall
	emittedContent          string
	emittedPlanningProgress bool
	emittedPlanningSkillID  string
	emittedPlanningToolName string
}

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

	execCtx := s.skillExecutionContext(prepared)
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

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	messages = append(messages, metadataMessage)
	messages = append(messages, agenticSkillLoopSystemMessage())
	traces := []skills.SkillTrace{metadataExposedTrace(resolved.SkillIDs(), metadataStats)}
	s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
	logger.DebugContext(ctx, "aichat skill metadata exposed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_ids", resolved.SkillIDs(),
		"skill_mode", prepared.parts.SkillMode,
	)

	stepCount := 0
	toolCallCount := 0
	recoverableFailureCount := 0
	consecutiveRecoverableFailures := 0
	skillToolCallCounts := map[string]int{}
	skillUsed := false
	loadedSkills := map[string]struct{}{}
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	var answerBuilder strings.Builder
	var usage *adapter.Usage

	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = skills.MetaToolsForSkillState(resolved, loadedSkills)
		planningReq.ToolChoice = "auto"

		planningResult, err := s.runSkillPlanning(ctx, prepared, planningReq, round, onEvent)
		if err != nil {
			return answerBuilder.String(), usage, err
		}
		usage = mergeUsage(usage, planningResult.usage)
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		text := assistantMessageText(planningMessage)
		if text != "" && len(toolCalls) > 0 && !planningResult.progressStreamed {
			s.emitAgentProgress(ctx, prepared, text, onEvent)
		}
		if len(toolCalls) == 0 && prepared.parts.SkillMode == skillModeRequired && !skillUsed {
			return answerBuilder.String(), usage, fmt.Errorf("%w: required skill was not used", ErrInvalidInput)
		}
		if text != "" && len(toolCalls) == 0 {
			answerBuilder.WriteString(text)
			if !planningResult.answerStreamed {
				s.emitAnswerChunk(ctx, prepared, text, onEvent)
			}
		}
		if len(toolCalls) == 0 {
			logger.DebugContext(ctx, "aichat skill planning completed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"skill_step_count", stepCount,
				"tool_call_count", toolCallCount,
			)
			return answerBuilder.String(), usage, nil
		}
		if stepCount+len(toolCalls) > maxSkillSteps {
			logger.WarnContext(ctx, "aichat skill step limit exceeded",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"current_step_count", stepCount,
				"requested_tool_calls", len(toolCalls),
				"max_steps", maxSkillSteps,
			)
			return answerBuilder.String(), usage, fmt.Errorf("%w: too many skill steps", ErrInvalidInput)
		}
		logger.DebugContext(ctx, "aichat skill planning requested tool calls",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"tool_call_count", len(toolCalls),
			"step_count", stepCount,
		)

		planningMessage.Role = "assistant"
		planningMessage.ToolCalls = toolCalls
		messages = append(messages, planningMessage)

		for _, call := range toolCalls {
			stepCount++
			result := s.handleProgressiveSkillCall(ctx, prepared, resolved, call, execCtx, toolCallCount, skillToolCallCounts, loadedSkills, onEvent)
			traces = append(traces, result.trace)
			s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
			s.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable {
				s.emitSkillError(ctx, prepared, result.trace, onEvent)
				recoverableFailureCount++
				consecutiveRecoverableFailures++
				if recoverableFailureCount > defaultMaxRecoverableSkillFailures ||
					consecutiveRecoverableFailures > defaultMaxConsecutiveRecoverableFailures {
					err := fmt.Errorf("%w: too many failed skill calls", ErrInvalidInput)
					trace := failedSkillTrace(result.trace.Kind, result.trace.ToolName, err)
					trace.SkillID = result.trace.SkillID
					trace.Arguments = result.trace.Arguments
					s.emitSkillError(ctx, prepared, trace, onEvent)
					return answerBuilder.String(), usage, err
				}
			} else {
				consecutiveRecoverableFailures = 0
			}
			if result.fatalErr != nil {
				if !result.recoverable {
					s.emitSkillError(ctx, prepared, result.trace, onEvent)
				}
				return answerBuilder.String(), usage, result.fatalErr
			}
			if result.usedSkill {
				skillUsed = true
			}
			if result.usedTool {
				toolCallCount++
				incrementSkillToolCallCount(skillToolCallCounts, result.trace.SkillID)
			}
			messages = append(messages, result.toolMessage)
		}
	}

	return answerBuilder.String(), usage, fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
}

func (s *service) runSkillPlanning(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onEvent func(StreamEvent) error,
) (planningResult, error) {
	if shouldStreamSkillPlanning(prepared) {
		result, ok, err := s.runSkillPlanningStream(ctx, prepared, planningReq, round, onEvent)
		if err != nil {
			return planningResult{}, err
		}
		if ok {
			return result, nil
		}
	}

	planningReq.Stream = false
	planningResp, err := s.llmClient.AppChat(ctx, newBillingAppContext(prepared), planningReq)
	if err != nil {
		return planningResult{}, err
	}
	return planningResult{
		message: firstPlanningMessage(planningResp),
		usage:   planningRespUsage(planningResp),
	}, nil
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	tenantID := prepared.Scope.OrganizationID.String()
	if prepared.Scope.WorkspaceID != nil {
		tenantID = prepared.Scope.WorkspaceID.String()
	}
	appID := prepared.Conversation.ID.String()
	if strings.TrimSpace(prepared.RunConfig.BillingAppID) != "" {
		appID = strings.TrimSpace(prepared.RunConfig.BillingAppID)
	}
	invokeFrom := tools.ToolInvokeFromAIChat
	if normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent {
		invokeFrom = tools.ToolInvokeFromAgent
	}
	return skills.ExecutionContext{
		TenantID:          tenantID,
		UserID:            prepared.Scope.AccountID.String(),
		ConversationID:    prepared.Conversation.ID.String(),
		AppID:             appID,
		MessageID:         prepared.Message.ID.String(),
		InvokeFrom:        invokeFrom,
		RuntimeParameters: skillRuntimeParameters(prepared.Scope, prepared.RunConfig),
	}
}

func skillRuntimeParameters(scope Scope, config RunConfig) map[string]interface{} {
	params := map[string]interface{}{
		"organization_id": scope.OrganizationID.String(),
	}
	if scope.WorkspaceID != nil {
		params["workspace_id"] = scope.WorkspaceID.String()
	}
	if len(config.KnowledgeDatasetIDs) > 0 {
		params["knowledge_dataset_ids"] = append([]string(nil), config.KnowledgeDatasetIDs...)
	}
	if len(config.KnowledgeRetrievalConfig) > 0 {
		params["knowledge_retrieval_config"] = copyStringAnyMap(config.KnowledgeRetrievalConfig)
	}
	if strings.EqualFold(strings.TrimSpace(config.BillingAppType), runtimemodel.ConversationCallerAgent) && strings.TrimSpace(config.BillingAppID) != "" {
		params["agent_id"] = strings.TrimSpace(config.BillingAppID)
	}
	if config.AgentMemoryEnabled {
		params["agent_memory_enabled"] = true
		params["agent_memory_slots"] = enabledAgentMemorySlots(config.AgentMemorySlots)
		if userScope := strings.TrimSpace(config.AgentMemoryUserScope); userScope != "" {
			params["user_scope"] = userScope
		}
	}
	return params
}

func (s *service) emitAnswerChunk(ctx context.Context, prepared *PreparedChat, text string, onEvent func(StreamEvent) error) {
	if text == "" {
		return
	}
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          text,
	}
	s.emitPreparedEvent(ctx, prepared, streamEventMessage, payload, onEvent)
}

func (s *service) emitAnswerRetract(ctx context.Context, prepared *PreparedChat, text string, onEvent func(StreamEvent) error) {
	if text == "" {
		return
	}
	s.emitPreparedEvent(ctx, prepared, streamEventMessageRetract, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         text,
		"length":          utf16CodeUnitLength(text),
		"created_at":      time.Now().Unix(),
	}, onEvent)
}

func utf16CodeUnitLength(text string) int {
	return len(utf16.Encode([]rune(text)))
}

func (s *service) emitAgentProgress(ctx context.Context, prepared *PreparedChat, text string, onEvent func(StreamEvent) error) {
	content := strings.TrimSpace(text)
	if content == "" {
		return
	}
	s.emitPreparedEvent(ctx, prepared, streamEventAgentProgress, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         content,
		"created_at":      time.Now().Unix(),
	}, onEvent)
}

func agenticSkillLoopSystemMessage() adapter.Message {
	return adapter.Message{
		Role: "system",
		Content: strings.Join([]string{
			"When using skills or tools, briefly explain your next action to the user before calling a skill/tool.",
			"After each skill/tool result, summarize what happened. If a tool call fails, explain the likely cause, fix the arguments, and retry when possible.",
			"Do not claim that you saved, remembered, updated, deleted, sent, created, changed, or completed any external action unless the corresponding skill/tool call succeeded in this turn.",
			"Progress text sent together with tool calls is transient status text. Keep it short and do not place substantial user deliverables there.",
			"If the current turn newly creates or substantially rewrites a user-facing deliverable before later tool/skill calls, call submit_intermediate_answer for that new deliverable before continuing.",
			"Examples of new deliverables that should use submit_intermediate_answer when followed by more tool/skill calls: novel outlines, long-form drafts, plans, tables, code sketches, analysis sections, or generated content the user asked for.",
			"Do not call submit_intermediate_answer merely to repeat content that was already visible in an earlier assistant answer. For requests like exporting, saving, converting, or generating a file from existing content, pass the existing content directly to the file/tool call.",
			"Do not skip submit_intermediate_answer by postponing or summarizing a new deliverable if the user explicitly asked for it as an intermediate phase.",
			"When no more tool or skill calls are needed, send a natural user-facing reply that is complete and self-contained. If you did not call submit_intermediate_answer for a new requested deliverable, that reply MUST include the deliverable in full, not a compressed summary.",
			"Do not label the user-facing reply with protocol wording such as Final Answer, final result, or their Chinese equivalents unless the user explicitly asks for that wording.",
			"When reusing existing conversation content, refer to it explicitly, for example as the previous outline or the current branch's draft; do not duplicate the full text unless the user asks to see it again.",
		}, "\n"),
	}
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

func planningRespUsage(resp *adapter.ChatResponse) *adapter.Usage {
	if resp == nil {
		return nil
	}
	return resp.Usage
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

func firstPlanningMessage(resp *adapter.ChatResponse) adapter.Message {
	if resp == nil || len(resp.Choices) == 0 {
		return adapter.Message{Role: "assistant"}
	}
	message := resp.Choices[0].Message
	if strings.TrimSpace(message.Role) == "" {
		message.Role = "assistant"
	}
	return message
}

func assistantMessageText(message adapter.Message) string {
	switch typed := message.Content.(type) {
	case string:
		return typed
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func normalizeToolCalls(calls []adapter.ToolCall) []adapter.ToolCall {
	out := make([]adapter.ToolCall, 0, len(calls))
	for idx, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("call_%d", idx+1)
		}
		if strings.TrimSpace(call.Type) == "" {
			call.Type = "function"
		}
		index := idx
		if call.Index == nil {
			call.Index = &index
		}
		out = append(out, call)
	}
	return out
}

func maxBusinessToolCalls(resolved *skills.ResolvedSkills) int {
	if resolved == nil || len(resolved.Skills) == 0 {
		return defaultMaxBusinessToolCallsPerSkill
	}
	total := 0
	for _, doc := range resolved.Skills {
		if doc.Metadata.MaxCallsPerTurn <= 0 {
			total += defaultMaxBusinessToolCallsPerSkill
			continue
		}
		total += doc.Metadata.MaxCallsPerTurn
	}
	if total <= 0 {
		return defaultMaxBusinessToolCallsPerSkill
	}
	return total
}

func maxBusinessToolCallsForSkill(resolved *skills.ResolvedSkills, skillID string) int {
	if resolved == nil {
		return defaultMaxBusinessToolCallsPerSkill
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	for _, doc := range resolved.Skills {
		if strings.ToLower(strings.TrimSpace(doc.Metadata.ID)) != skillID {
			continue
		}
		if doc.Metadata.MaxCallsPerTurn > 0 {
			return doc.Metadata.MaxCallsPerTurn
		}
		return defaultMaxBusinessToolCallsPerSkill
	}
	return defaultMaxBusinessToolCallsPerSkill
}

func incrementSkillToolCallCount(counts map[string]int, skillID string) {
	if counts == nil {
		return
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return
	}
	counts[skillID]++
}

func maxSkillStepsForTurn(resolved *skills.ResolvedSkills) int {
	limit := maxBusinessToolCalls(resolved)
	if resolved != nil {
		limit += len(resolved.Skills) * 2
	}
	if limit < defaultMaxSkillStepsPerTurn {
		return defaultMaxSkillStepsPerTurn
	}
	return limit
}
