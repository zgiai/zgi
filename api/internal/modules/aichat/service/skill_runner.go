package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultMaxSkillPlanningRounds                 = 50
	defaultMaxSkillStepsPerTurn                   = 160
	defaultMaxBusinessToolCallsPerSkill           = 20
	defaultMaxRecoverableFailureRounds            = 12
	defaultMaxConsecutiveRecoverableFailureRounds = 5
	intermediateAnswerChunkRunes                  = 180
	streamedIntermediateAnswerArg                 = "_aichat_streamed_answer"
)

var skillPlanningFallbackProgressDelay = 800 * time.Millisecond

type skillStepResult struct {
	trace       skills.SkillTrace
	toolMessage adapter.Message
	answer      string
	usedSkill   bool
	usedTool    bool
	recoverable bool
	terminal    bool
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
	recoverableFailureRoundCount := 0
	consecutiveRecoverableFailureRounds := 0
	recoverableFailureCallCount := 0
	finalAnswerGuardBlockCount := 0
	skillToolCallCounts := map[string]int{}
	attemptedToolCalls := []skillToolCallRef{}
	successfulToolCalls := []skillToolCallRef{}
	skillUsed := false
	loadedSkills := map[string]struct{}{}
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	finalAnswerGuard := skillLoopFinalAnswerGuard(prepared)
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
		if len(toolCalls) == 0 {
			if guardResult, blocked := runFinalAnswerGuard(finalAnswerGuard, finalAnswerGuardRequest{
				Answer:              text,
				Round:               round,
				SkillUsed:           skillUsed,
				ToolCallCount:       toolCallCount,
				AttemptedToolCalls:  append([]skillToolCallRef{}, attemptedToolCalls...),
				SuccessfulToolCalls: append([]skillToolCallRef{}, successfulToolCalls...),
			}); blocked {
				finalAnswerGuardBlockCount++
				if planningResult.answerStreamed && text != "" {
					s.emitAnswerRetract(ctx, prepared, text, onEvent)
				}
				trace := finalAnswerGuardrailTrace(guardResult)
				traces = append(traces, trace)
				s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
				s.logSkillTrace(ctx, prepared, trace)
				if finalAnswerGuardBlockCount > defaultMaxConsecutiveRecoverableFailureRounds {
					err := fmt.Errorf("%w: final answer guard blocked too many consecutive replies", ErrInvalidInput)
					s.emitSkillError(ctx, prepared, failedSkillTrace("guardrail", guardResult.ToolName, err), onEvent)
					return answerBuilder.String(), usage, err
				}
				messages = append(messages, finalAnswerGuardSystemMessage(guardResult, text))
				continue
			}
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

		roundHadRecoverableFailure := false
		roundHadSuccess := false
		var lastRecoverableTrace skills.SkillTrace
		for _, call := range toolCalls {
			stepCount++
			result := s.handleProgressiveSkillCall(ctx, prepared, resolved, call, execCtx, toolCallCount, skillToolCallCounts, loadedSkills, onEvent)
			traces = append(traces, result.trace)
			s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
			s.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable {
				s.emitSkillError(ctx, prepared, result.trace, onEvent)
				roundHadRecoverableFailure = true
				lastRecoverableTrace = result.trace
				recoverableFailureCallCount++
			} else {
				roundHadSuccess = true
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
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") {
				attemptedToolCalls = append(attemptedToolCalls, skillToolCallRef{
					SkillID:   strings.TrimSpace(result.trace.SkillID),
					ToolName:  strings.TrimSpace(result.trace.ToolName),
					Arguments: copyStringAnyMap(result.trace.Arguments),
				})
			}
			if result.usedTool {
				toolCallCount++
				incrementSkillToolCallCount(skillToolCallCounts, result.trace.SkillID)
				if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
					strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
					successfulToolCalls = append(successfulToolCalls, skillToolCallRef{
						SkillID:   strings.TrimSpace(result.trace.SkillID),
						ToolName:  strings.TrimSpace(result.trace.ToolName),
						Arguments: copyStringAnyMap(result.trace.Arguments),
					})
					finalAnswerGuardBlockCount = 0
				}
			}
			if result.answer != "" {
				appendAnswerText(&answerBuilder, result.answer)
				s.emitAnswerChunk(ctx, prepared, result.answer, onEvent)
			}
			if result.terminal {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				return answerBuilder.String(), usage, nil
			}
			messages = append(messages, result.toolMessage)
		}
		if roundHadRecoverableFailure {
			recoverableFailureRoundCount++
			if !roundHadSuccess {
				consecutiveRecoverableFailureRounds++
			} else {
				consecutiveRecoverableFailureRounds = 0
			}
			logger.DebugContext(ctx, "aichat skill recoverable failures observed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"failure_round_count", recoverableFailureRoundCount,
				"consecutive_failure_rounds", consecutiveRecoverableFailureRounds,
				"failure_call_count", recoverableFailureCallCount,
			)
			if recoverableFailureRoundCount > defaultMaxRecoverableFailureRounds ||
				consecutiveRecoverableFailureRounds > defaultMaxConsecutiveRecoverableFailureRounds {
				err := fmt.Errorf("%w: too many failed skill calls", ErrInvalidInput)
				trace := failedSkillTrace(lastRecoverableTrace.Kind, lastRecoverableTrace.ToolName, err)
				trace.SkillID = lastRecoverableTrace.SkillID
				trace.Arguments = lastRecoverableTrace.Arguments
				s.emitSkillError(ctx, prepared, trace, onEvent)
				return answerBuilder.String(), usage, err
			}
		} else {
			consecutiveRecoverableFailureRounds = 0
		}
	}

	return answerBuilder.String(), usage, fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
}

func appendAnswerText(builder *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if builder.Len() > 0 {
		current := builder.String()
		if !strings.HasSuffix(current, "\n") {
			builder.WriteString("\n\n")
		}
	}
	builder.WriteString(text)
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

func (s *service) runSkillPlanningStream(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onEvent func(StreamEvent) error,
) (planningResult, bool, error) {
	streamReq := cloneChatRequest(planningReq)
	streamReq.Stream = true
	stream, fallbackProgressStreamed, err := s.openSkillPlanningStream(ctx, prepared, streamReq, onEvent)
	if err != nil {
		logger.WarnContext(ctx, "aichat skill planning stream unavailable, falling back to non-stream planning",
			"message_id", prepared.Message.ID.String(),
			"provider", prepared.parts.Provider,
			err,
		)
		return planningResult{}, false, nil
	}

	var contentBuilder strings.Builder
	var reasoningBuilder strings.Builder
	var usage *adapter.Usage
	toolCallsByIndex := map[int]*streamingToolCallState{}
	toolCallOrder := make([]int, 0)
	sawChunk := false
	sawToolCall := false
	answerStreamed := false
	naturalProgressStreamed := false
	toolPlanningProgressStreamed := false
	var speculativeAnswer strings.Builder
	fallbackTimer := time.NewTimer(skillPlanningFallbackProgressDelay)
	defer fallbackTimer.Stop()

	for {
		select {
		case <-fallbackTimer.C:
			if !answerStreamed && !naturalProgressStreamed && !toolPlanningProgressStreamed && !fallbackProgressStreamed {
				fallbackProgressStreamed = s.emitPlanningFallbackProgress(ctx, prepared, onEvent)
			}
			continue
		case response, ok := <-stream:
			if !ok {
				goto streamDone
			}

			if response.Error != nil {
				return planningResult{}, true, response.Error
			}
			usage = mergeUsage(usage, response.Usage)
			if response.Done {
				goto streamDone
			}
			if len(response.Choices) == 0 {
				continue
			}
			sawChunk = true
			for _, choice := range response.Choices {
				if reasoning := streamChoiceReasoningContent(choice); reasoning != "" {
					reasoningBuilder.WriteString(reasoning)
				}
				if text := streamChoiceText(choice); text != "" {
					contentBuilder.WriteString(text)
					if sawToolCall {
						s.emitAgentProgress(ctx, prepared, text, onEvent)
						naturalProgressStreamed = true
					} else {
						s.emitAnswerChunk(ctx, prepared, text, onEvent)
						speculativeAnswer.WriteString(text)
						answerStreamed = true
					}
				}
				for _, delta := range choice.Delta.ToolCalls {
					if !sawToolCall {
						sawToolCall = true
						if speculative := speculativeAnswer.String(); speculative != "" {
							s.emitAnswerRetract(ctx, prepared, speculative, onEvent)
						}
						if progress := strings.TrimSpace(contentBuilder.String()); progress != "" {
							s.emitAgentProgress(ctx, prepared, progress, onEvent)
							naturalProgressStreamed = true
						}
					}
					state := mergeStreamingToolCall(toolCallsByIndex, &toolCallOrder, delta)
					if state == nil {
						continue
					}
					if (!toolPlanningProgressStreamed || isStreamingBusinessToolCall(state)) && (!naturalProgressStreamed || isStreamingBusinessToolCall(state)) && s.emitStreamingToolPlanningProgress(ctx, prepared, state, onEvent) {
						toolPlanningProgressStreamed = true
					}
					s.emitStreamingIntermediateAnswerDelta(ctx, prepared, round, state, onEvent)
				}
			}
		}
	}

streamDone:
	if !sawChunk {
		return planningResult{}, false, nil
	}

	toolCalls := make([]adapter.ToolCall, 0, len(toolCallOrder))
	for _, index := range toolCallOrder {
		state := toolCallsByIndex[index]
		if state == nil {
			continue
		}
		call := state.call
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolIntermediateAnswer) && state.emittedContent != "" {
			call.Function.Arguments = markIntermediateAnswerArgumentsStreamed(call.Function.Arguments, streamingIntermediateAnswerID(prepared, round, call))
		}
		toolCalls = append(toolCalls, call)
	}

	return planningResult{
		message: adapter.Message{
			Role:             "assistant",
			Content:          contentBuilder.String(),
			ToolCalls:        toolCalls,
			ReasoningContent: reasoningBuilder.String(),
		},
		usage:            usage,
		answerStreamed:   answerStreamed && len(toolCalls) == 0,
		progressStreamed: naturalProgressStreamed || toolPlanningProgressStreamed || fallbackProgressStreamed,
	}, true, nil
}

type skillPlanningStreamOpenResult struct {
	stream <-chan adapter.StreamResponse
	err    error
}

func (s *service) openSkillPlanningStream(
	ctx context.Context,
	prepared *PreparedChat,
	streamReq *adapter.ChatRequest,
	onEvent func(StreamEvent) error,
) (<-chan adapter.StreamResponse, bool, error) {
	resultCh := make(chan skillPlanningStreamOpenResult, 1)
	go func() {
		stream, err := s.llmClient.AppChatStream(ctx, newBillingAppContext(prepared), streamReq)
		resultCh <- skillPlanningStreamOpenResult{stream: stream, err: err}
	}()

	timer := time.NewTimer(skillPlanningFallbackProgressDelay)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.stream, false, result.err
	case <-timer.C:
		fallbackProgressStreamed := s.emitPlanningFallbackProgress(ctx, prepared, onEvent)
		select {
		case result := <-resultCh:
			return result.stream, fallbackProgressStreamed, result.err
		case <-ctx.Done():
			return nil, fallbackProgressStreamed, ctx.Err()
		}
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

func shouldStreamSkillPlanning(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(prepared.parts.Provider))
	switch provider {
	case "openai", "openai-compatible", "deepseek", "openrouter", "zgi-cloud", "zgi_cloud", "dashscope",
		"aliyun", "claude", "anthropic", "moonshotai", "moonshotai-cn", "siliconflow", "agicto", "glm":
		return true
	default:
		return false
	}
}

func streamChoiceText(choice adapter.StreamChoice) string {
	switch typed := choice.Delta.Content.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func streamChoiceReasoningContent(choice adapter.StreamChoice) string {
	return choice.Delta.ReasoningContent
}

func mergeStreamingToolCall(states map[int]*streamingToolCallState, order *[]int, delta adapter.ToolCall) *streamingToolCallState {
	index := 0
	if delta.Index != nil {
		index = *delta.Index
	}
	state := states[index]
	if state == nil {
		callIndex := index
		state = &streamingToolCallState{
			call: adapter.ToolCall{
				Index: &callIndex,
				Type:  "function",
			},
		}
		states[index] = state
		*order = append(*order, index)
	}

	if strings.TrimSpace(delta.ID) != "" {
		state.call.ID = delta.ID
	}
	if strings.TrimSpace(delta.Type) != "" {
		state.call.Type = delta.Type
	}
	if delta.Index != nil {
		state.call.Index = delta.Index
	}
	if strings.TrimSpace(delta.Function.Name) != "" {
		state.call.Function.Name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		state.call.Function.Arguments += delta.Function.Arguments
	}
	return state
}

func isStreamingBusinessToolCall(state *streamingToolCallState) bool {
	return state != nil && strings.EqualFold(strings.TrimSpace(state.call.Function.Name), skills.MetaToolCallSkillTool)
}

func (s *service) emitStreamingIntermediateAnswerDelta(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	state *streamingToolCallState,
	onEvent func(StreamEvent) error,
) {
	if state == nil || !strings.EqualFold(strings.TrimSpace(state.call.Function.Name), skills.MetaToolIntermediateAnswer) {
		return
	}
	content, _ := partialJSONStringField(state.call.Function.Arguments, "content")
	if content == "" || len(content) <= len(state.emittedContent) || !strings.HasPrefix(content, state.emittedContent) {
		return
	}
	title, _ := partialJSONStringField(state.call.Function.Arguments, "title")
	delta := content[len(state.emittedContent):]
	state.emittedContent = content
	trace := skills.SkillTrace{
		Kind:    "intermediate_answer",
		Title:   strings.TrimSpace(title),
		Message: content,
		Status:  "running",
	}
	answerID := streamingIntermediateAnswerID(prepared, round, state.call)
	s.emitPreparedEvent(ctx, prepared, streamEventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, delta, 0, false, "streaming"), onEvent)
}

func (s *service) emitStreamingToolPlanningProgress(
	ctx context.Context,
	prepared *PreparedChat,
	state *streamingToolCallState,
	onEvent func(StreamEvent) error,
) bool {
	if state == nil {
		return false
	}
	metaToolName := strings.TrimSpace(state.call.Function.Name)
	if metaToolName == "" || strings.EqualFold(metaToolName, skills.MetaToolIntermediateAnswer) {
		return false
	}

	arguments := state.call.Function.Arguments
	argumentsChars := len([]rune(arguments))
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"phase":           "tool_planning",
		"meta_tool_name":  metaToolName,
		"arguments_chars": argumentsChars,
		"created_at":      time.Now().Unix(),
	}

	skillID := ""
	toolName := ""
	switch metaToolName {
	case skills.MetaToolCallSkillTool:
		skillID, _ = partialJSONStringField(arguments, "skill_id")
		toolName, _ = partialJSONStringField(arguments, "tool_name")
		skillID = strings.TrimSpace(skillID)
		toolName = strings.TrimSpace(toolName)
		if strings.TrimSpace(skillID) == "" || strings.TrimSpace(toolName) == "" {
			if argumentsChars < 256 {
				return false
			}
		}
		if skillID != "" {
			payload["skill_id"] = skillID
		}
		if toolName != "" {
			payload["tool_name"] = toolName
		}
	case skills.MetaToolLoadSkill, skills.MetaToolReadSkillReference:
		skillID, _ = partialJSONStringField(arguments, "skill_id")
		skillID = strings.TrimSpace(skillID)
		if strings.TrimSpace(skillID) == "" && argumentsChars < 128 {
			return false
		}
		if skillID != "" {
			payload["skill_id"] = skillID
		}
	default:
		if argumentsChars < 128 {
			return false
		}
	}

	if state.emittedPlanningProgress {
		if metaToolName != skills.MetaToolCallSkillTool {
			return false
		}
		if skillID == "" || toolName == "" {
			return false
		}
		if state.emittedPlanningSkillID == skillID && state.emittedPlanningToolName == toolName {
			return false
		}
	}

	state.emittedPlanningProgress = true
	state.emittedPlanningSkillID = skillID
	state.emittedPlanningToolName = toolName
	s.emitPreparedEvent(ctx, prepared, streamEventAgentProgress, payload, onEvent)
	return true
}

func (s *service) emitPlanningFallbackProgress(
	ctx context.Context,
	prepared *PreparedChat,
	onEvent func(StreamEvent) error,
) bool {
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"phase":           "planning",
		"created_at":      time.Now().Unix(),
	}
	s.emitPreparedEvent(ctx, prepared, streamEventAgentProgress, payload, onEvent)
	return true
}

func streamingIntermediateAnswerID(prepared *PreparedChat, round int, call adapter.ToolCall) string {
	if strings.TrimSpace(call.ID) != "" {
		return call.ID
	}
	index := 0
	if call.Index != nil {
		index = *call.Index
	}
	messageID := "message"
	if prepared != nil && prepared.Message != nil {
		messageID = prepared.Message.ID.String()
	}
	return fmt.Sprintf("intermediate-%s-%d-%d", messageID, round, index)
}

func markIntermediateAnswerArgumentsStreamed(arguments string, answerID string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return arguments
	}
	parsed[streamedIntermediateAnswerArg] = true
	parsed[streamedIntermediateAnswerArg+"_id"] = answerID
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return arguments
	}
	return string(encoded)
}

func (s *service) handleProgressiveSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	call adapter.ToolCall,
	execCtx skills.ExecutionContext,
	currentToolCalls int,
	skillToolCallCounts map[string]int,
	loadedSkills map[string]struct{},
	onEvent func(StreamEvent) error,
) skillStepResult {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same tool call")), false, false)
	}
	switch call.Function.Name {
	case skills.MetaToolLoadSkill:
		return s.handleLoadSkillCall(ctx, prepared, resolved, call.ID, args, loadedSkills, onEvent)
	case skills.MetaToolReadSkillReference:
		if _, ok := loadedSkills[normalizedSkillArg(args, "skill_id")]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), "", "skill must be loaded before reading references")
			trace.SkillID = stringArg(args, "skill_id")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
		}
		return s.handleReadReferenceCall(ctx, prepared, resolved, call.ID, args, onEvent)
	case skills.MetaToolCallSkillTool:
		skillID := normalizedSkillArg(args, "skill_id")
		toolName := stringArg(args, "tool_name")
		toolArgs := mapArg(args, "arguments")
		if _, ok := loadedSkills[skillID]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), toolName, "skill must be loaded before calling its tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
		}
		if doc, ok := resolved.Get(skillID); ok && len(doc.Tools) == 0 {
			trace := blockedSkillGuardrailTrace(skillID, toolName, "skill does not provide callable tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), true, false)
		}
		if currentToolCalls >= maxBusinessToolCalls(resolved) {
			err := fmt.Errorf("%w: too many skill tool calls", ErrInvalidInput)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return fatalSkillStep(trace, skills.ToolResultMessage(call.ID, errorPayload(err)), err)
		}
		if skillToolCallCounts[skillID] >= maxBusinessToolCallsForSkill(resolved, skillID) {
			err := fmt.Errorf("%w: too many skill tool calls for skill %s", ErrInvalidInput, skillID)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return fatalSkillStep(trace, skills.ToolResultMessage(call.ID, errorPayload(err)), err)
		}
		return s.handleCallSkillTool(ctx, prepared, resolved, call.ID, args, execCtx, onEvent)
	case skills.MetaToolRequestUserInput:
		return s.handleRequestUserInputCall(ctx, prepared, call.ID, args, onEvent)
	case skills.MetaToolIntermediateAnswer:
		return s.handleIntermediateAnswerCall(ctx, prepared, call.ID, args, onEvent)
	default:
		err := fmt.Errorf("%w: unsupported skill meta tool %s", ErrInvalidInput, call.Function.Name)
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "use one of load_skill, request_user_input, read_skill_reference, call_skill_tool, or submit_intermediate_answer")), false, false)
	}
}

func (s *service) handleRequestUserInputCall(
	ctx context.Context,
	prepared *PreparedChat,
	callID string,
	args map[string]interface{},
	onEvent func(StreamEvent) error,
) skillStepResult {
	questions, err := normalizeUserInputRequestArgs(args)
	if err != nil {
		trace := failedSkillTrace("user_input_request", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call request_user_input again with one to five non-empty questions and optional short options")), false, false)
	}
	visibleMessage := normalizeUserInputRequestMessage(args)
	if visibleMessage == "" {
		err := fmt.Errorf("%w: message is required", ErrInvalidInput)
		trace := failedSkillTrace("user_input_request", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call request_user_input again with a brief user-visible message and one to five questions")), false, false)
	}
	firstQuestion := userInputString(questions[0]["question"])
	trace := skills.SkillTrace{
		Kind:    "user_input_request",
		Message: firstQuestion,
		Status:  "success",
		Arguments: map[string]interface{}{
			"question_count": len(questions),
			"questions":      userInputQuestionSummaries(questions),
		},
	}
	if visibleMessage != "" {
		trace.Message = visibleMessage
	}
	payload := userInputRequestPayload(prepared, callID, questions)
	s.persistUserInputRequestBestEffort(ctx, prepared, payload)
	s.emitPreparedEvent(ctx, prepared, streamEventUserInputRequested, payload, onEvent)
	logger.DebugContext(ctx, "aichat user input requested",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"question_count", len(questions),
	)
	result := terminalSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status":      "waiting_for_user",
		"instruction": "The question is visible to the user. Stop this turn and wait for the next user message.",
	}), false, false)
	result.answer = visibleMessage
	return result
}

func (s *service) handleIntermediateAnswerCall(
	ctx context.Context,
	prepared *PreparedChat,
	callID string,
	args map[string]interface{},
	onEvent func(StreamEvent) error,
) skillStepResult {
	content := strings.TrimSpace(stringArg(args, "content"))
	if content == "" {
		err := fmt.Errorf("%w: intermediate answer content is required", ErrInvalidInput)
		trace := failedSkillTrace("intermediate_answer", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call submit_intermediate_answer again with non-empty content")), false, false)
	}
	title := strings.TrimSpace(stringArg(args, "title"))
	answerID := strings.TrimSpace(stringArg(args, streamedIntermediateAnswerArg+"_id"))
	trace := skills.SkillTrace{
		Kind:    "intermediate_answer",
		Title:   title,
		Message: content,
		Status:  "success",
		Arguments: map[string]interface{}{
			"title": title,
		},
	}
	if boolArg(args, streamedIntermediateAnswerArg) {
		if answerID == "" {
			answerID = callID
		}
		s.emitPreparedEvent(ctx, prepared, streamEventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, "", 0, true, "success"), onEvent)
	} else {
		s.emitIntermediateAnswer(ctx, prepared, callID, trace, onEvent)
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "recorded",
		"instruction": strings.Join([]string{
			"The intermediate answer is visible to the user and saved in the run trace.",
			"Continue with any remaining tool calls.",
			"Your eventual user-facing reply must still be complete and self-contained; do not say see above.",
		}, " "),
	}), false, false)
}

func (s *service) emitIntermediateAnswer(
	ctx context.Context,
	prepared *PreparedChat,
	answerID string,
	trace skills.SkillTrace,
	onEvent func(StreamEvent) error,
) {
	chunks := splitIntermediateAnswerContent(trace.Message, intermediateAnswerChunkRunes)
	if len(chunks) == 0 {
		return
	}
	for index, chunk := range chunks {
		done := index == len(chunks)-1
		status := "streaming"
		if done {
			status = "success"
		}
		s.emitPreparedEvent(ctx, prepared, streamEventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, chunk, index, done, status), onEvent)
	}
}

func splitIntermediateAnswerContent(content string, chunkRunes int) []string {
	if chunkRunes <= 0 {
		chunkRunes = intermediateAnswerChunkRunes
	}
	runes := []rune(content)
	if len(runes) <= chunkRunes {
		if content == "" {
			return nil
		}
		return []string{content}
	}

	chunks := make([]string, 0, (len(runes)/chunkRunes)+1)
	for start := 0; start < len(runes); {
		end := start + chunkRunes
		if end >= len(runes) {
			chunks = append(chunks, string(runes[start:]))
			break
		}

		split := end
		for i := end; i > start+chunkRunes/2; i-- {
			switch runes[i-1] {
			case '\n', ' ', '\t', '。', '，', '；', '！', '？', '.', ',', ';', '!', '?':
				split = i
				i = start
			}
		}
		if split <= start {
			split = end
		}
		chunks = append(chunks, string(runes[start:split]))
		start = split
	}
	return chunks
}

func (s *service) handleLoadSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	loadedSkills map[string]struct{},
	onEvent func(StreamEvent) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	s.emitPreparedEvent(ctx, prepared, streamEventSkillLoadStart, skillLoadPayload(prepared, skillID), onEvent)
	doc, trace, err := s.skillRuntime.LoadSkill(ctx, resolved, skillID)
	if err != nil {
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "choose an enabled skill_id from the exposed metadata and retry")), false, false)
	}
	loadedSkills[doc.Metadata.ID] = struct{}{}
	logger.DebugContext(ctx, "aichat skill loaded",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", doc.Metadata.ID,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillLoadEnd, skillLoadEndPayload(prepared, trace), onEvent)
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, skillDocumentPayload(doc)), true, false)
}

func (s *service) handleReadReferenceCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	onEvent func(StreamEvent) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	path := stringArg(args, "path")
	content, trace, err := s.skillRuntime.ReadReference(ctx, resolved, skillID, path)
	if err != nil {
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "use a reference path listed in the loaded SKILL.md and retry")), true, false)
	}
	logger.DebugContext(ctx, "aichat skill reference read",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", trace.SkillID,
		"path", path,
		"duration_ms", trace.DurationMS,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillReferenceRead, skillReferenceReadPayload(prepared, trace, path), onEvent)
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"skill_id": skillID,
		"path":     path,
		"content":  content,
	}), true, false)
}

func (s *service) handleCallSkillTool(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	execCtx skills.ExecutionContext,
	onEvent func(StreamEvent) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	argumentSummary := summarizeSkillToolArguments(skillID, toolName, toolArgs)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallStart, skillCallStartPayload(prepared, skillID, toolName, argumentSummary), onEvent)
	invocation, err := s.skillRuntime.CallSkillTool(ctx, resolved, skillID, toolName, toolArgs, execCtx, callID)
	if invocation == nil {
		if err == nil {
			err = fmt.Errorf("%w: skill tool returned no invocation result", ErrInvalidInput)
		}
		trace := failedSkillTrace("tool_call", toolName, err)
		trace.SkillID = skillID
		trace.Arguments = argumentSummary
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool_name or arguments and retry", skillID, toolName)), true, false)
	}
	invocation.Trace.Arguments = argumentSummary
	if invocation.Trace.Kind == "tool_governance" {
		s.emitPreparedEvent(ctx, prepared, streamEventToolGovernanceDecision, toolGovernanceDecisionPayload(prepared, invocation.Trace), onEvent)
	}
	if err != nil {
		return recoverableSkillStep(invocation.Trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool arguments based on the error and retry", skillID, toolName)), true, false)
	}
	invocation.Trace.Result = summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages)
	logger.DebugContext(ctx, "aichat skill tool completed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", invocation.Trace.SkillID,
		"tool_name", invocation.Trace.ToolName,
		"duration_ms", invocation.Trace.DurationMS,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace), onEvent)
	for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
		s.persistGeneratedArtifactBestEffort(ctx, prepared, artifact)
		s.emitPreparedEvent(ctx, prepared, streamEventSkillArtifactCreated, artifact, onEvent)
	}
	return successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
}

func successfulSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
	}
}

func recoverableSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
		recoverable: true,
	}
}

func terminalSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
		terminal:    true,
	}
}

func fatalSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, err error) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		fatalErr:    err,
	}
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	runtimeParameters := map[string]interface{}{
		"organization_id": prepared.Scope.OrganizationID.String(),
	}
	if workspaceID := preparedSkillWorkspaceID(prepared); workspaceID != "" {
		runtimeParameters["workspace_id"] = workspaceID
	}
	runtimeParameters = applySkillToolGovernanceRuntimeParameters(runtimeParameters, prepared)
	if prepared != nil && prepared.parts != nil && isConsoleFilesContext(prepared.parts) {
		runtimeParameters["console_files_page"] = true
		runtimeParameters["file_generation_default_target"] = "managed_file"
	}
	if visibleFiles := consoleFilesRuntimeVisibleFiles(prepared); len(visibleFiles) > 0 {
		runtimeParameters["console_files_visible_files"] = visibleFiles
	}
	return skills.ExecutionContext{
		OrganizationID:    prepared.Scope.OrganizationID.String(),
		UserID:            prepared.Scope.AccountID.String(),
		ConversationID:    prepared.Conversation.ID.String(),
		AppID:             prepared.Conversation.ID.String(),
		MessageID:         prepared.Message.ID.String(),
		InvokeFrom:        tools.ToolInvokeFromAIChat,
		RuntimeParameters: runtimeParameters,
	}
}

func preparedSkillWorkspaceID(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	if prepared.Scope.WorkspaceID != nil && *prepared.Scope.WorkspaceID != uuid.Nil {
		return prepared.Scope.WorkspaceID.String()
	}
	if prepared.Conversation != nil && prepared.Conversation.WorkspaceID != nil && *prepared.Conversation.WorkspaceID != uuid.Nil {
		return prepared.Conversation.WorkspaceID.String()
	}
	return ""
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
			"If a tool call fails, do not repeat the same tool with the same arguments. Re-plan from the error before retrying.",
			"For deterministic batch work, prefer one suitable business tool call that handles the batch coherently over many small repeated tool calls.",
			"Do not claim that you saved, remembered, updated, deleted, sent, created, changed, or completed any external action unless the corresponding skill/tool call succeeded in this turn.",
			"Progress text sent together with tool calls is transient status text. Keep it short and do not place substantial user deliverables there.",
			"If the current turn newly creates or substantially rewrites a user-facing deliverable before later tool/skill calls, call submit_intermediate_answer for that new deliverable before continuing.",
			"Examples of new deliverables that should use submit_intermediate_answer when followed by more tool/skill calls: novel outlines, long-form drafts, plans, tables, code sketches, analysis sections, or generated content the user asked for.",
			"Do not call submit_intermediate_answer merely to repeat content that was already visible in an earlier assistant answer. For requests like exporting, saving, converting, or generating a file from existing content, pass the existing content directly to the file/tool call.",
			"Do not skip submit_intermediate_answer by postponing or summarizing a new deliverable if the user explicitly asked for it as an intermediate phase.",
			"When required information is missing or ambiguity blocks reliable progress, call request_user_input with a brief user-visible message plus a questions array containing one to five concise questions, then stop. The message should explain what you checked, why input is needed, and what you will do next. Prefer one to three questions. Do not call any other tools in the same turn after request_user_input.",
			"When calling request_user_input, put the user-visible explanation only in the request_user_input message field. Do not also repeat that explanation in assistant text outside the tool call.",
			"Each request_user_input question should ask one decision point. Include options only when each option is a concrete, directly usable answer. Do not include vague options such as free choice, freestyle, not sure, depends, any, or other; omit options for open-ended questions because the user can type freely.",
			"Do not use request_user_input for information already confirmed in the conversation.",
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

func (s *service) persistSkillTracesBestEffort(ctx context.Context, prepared *PreparedChat, traces []skills.SkillTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := mergeSkillTraceMetadata(prepared.Message.Metadata, traces)
	prepared.Message.Metadata = metadata
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func (s *service) persistGeneratedArtifactBestEffort(ctx context.Context, prepared *PreparedChat, artifact map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(artifact) == 0 {
		return
	}
	metadata := mergeGeneratedArtifactMetadata(prepared.Message.Metadata, artifact)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat generated artifact metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeGeneratedArtifactMetadata(source map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
	return metadata
}

func mergeSkillTraceMetadata(source map[string]interface{}, traces []skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(traces) == 0 {
		return metadata
	}
	selected := make([]interface{}, 0)
	loaded := make([]interface{}, 0)
	toolsUsed := make([]interface{}, 0)
	invocations := make([]interface{}, 0, len(traces))
	selectedSeen := map[string]struct{}{}
	loadedSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	toolCallCount := 0
	guardrailCount := 0
	addConfiguredSkillIDs(metadata, selectedSeen, &selected)

	for _, trace := range traces {
		if trace.SkillID != "" {
			if _, exists := selectedSeen[trace.SkillID]; !exists {
				selectedSeen[trace.SkillID] = struct{}{}
				selected = append(selected, trace.SkillID)
			}
		}
		if trace.Kind == "skill_load" && trace.Status == "success" {
			if _, exists := loadedSeen[trace.SkillID]; trace.SkillID != "" && !exists {
				loadedSeen[trace.SkillID] = struct{}{}
				loaded = append(loaded, trace.SkillID)
			}
		}
		if trace.Kind == "tool_call" {
			toolCallCount++
			if _, exists := toolSeen[trace.ToolName]; trace.ToolName != "" && !exists {
				toolSeen[trace.ToolName] = struct{}{}
				toolsUsed = append(toolsUsed, trace.ToolName)
			}
		}
		if trace.Kind == "guardrail" {
			guardrailCount++
		}
		invocation := map[string]interface{}{
			"kind":        trace.Kind,
			"skill_id":    trace.SkillID,
			"tool_name":   trace.ToolName,
			"title":       trace.Title,
			"status":      trace.Status,
			"duration_ms": trace.DurationMS,
			"arguments":   trace.Arguments,
			"result":      trace.Result,
			"message":     trace.Message,
			"error":       trace.Error,
		}
		if trace.Governance != nil {
			invocation["governance"] = trace.Governance
		}
		invocations = append(invocations, invocation)
	}
	metadata["has_trace"] = true
	metadata["selected_skill_ids"] = selected
	metadata["loaded_skill_ids"] = loaded
	actionTraceCount := countSkillActionTraces(traces)
	metadata["skill_step_count"] = actionTraceCount
	metadata["skill_call_count"] = actionTraceCount
	metadata["tool_call_count"] = toolCallCount
	metadata["guardrail_count"] = guardrailCount
	metadata["skill_names"] = selected
	metadata["tool_names"] = toolsUsed
	metadata["skill_invocations"] = invocations
	return metadata
}

func countSkillActionTraces(traces []skills.SkillTrace) int {
	count := 0
	for _, trace := range traces {
		switch trace.Kind {
		case "skill_load", "reference_read", "tool_call", "tool_governance", "guardrail", "intermediate_answer", "user_input_request":
			count++
		}
	}
	return count
}

func addConfiguredSkillIDs(metadata map[string]interface{}, seen map[string]struct{}, out *[]interface{}) {
	value, ok := metadata["configured_skill_ids"]
	if !ok {
		return
	}
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		*out = append(*out, id)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
}

func generatedFilesFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...)
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if file, ok := item.(map[string]interface{}); ok {
				out = append(out, file)
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "download=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
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

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func boolArg(args map[string]interface{}, key string) bool {
	if args == nil {
		return false
	}
	value, ok := args[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func normalizedSkillArg(args map[string]interface{}, key string) string {
	return strings.ToLower(stringArg(args, key))
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}
	value, ok := args[key]
	if !ok || value == nil {
		return map[string]interface{}{}
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return map[string]interface{}{}
}

func partialJSONStringField(input string, field string) (string, bool) {
	start, ok := findJSONStringFieldValueStart(input, field)
	if !ok {
		return "", false
	}
	value, _, complete := decodePartialJSONString(input[start:])
	return value, complete
}

func findJSONStringFieldValueStart(input string, field string) (int, bool) {
	for i := 0; i < len(input); i++ {
		if input[i] != '"' {
			continue
		}
		keyStart := i
		key, keyEnd, complete := decodeJSONStringToken(input, keyStart)
		if !complete || key != field {
			continue
		}
		j := skipJSONWhitespace(input, keyEnd)
		if j >= len(input) || input[j] != ':' {
			continue
		}
		j = skipJSONWhitespace(input, j+1)
		if j < len(input) && input[j] == '"' {
			return j + 1, true
		}
	}
	return 0, false
}

func decodeJSONStringToken(input string, quoteStart int) (string, int, bool) {
	if quoteStart < 0 || quoteStart >= len(input) || input[quoteStart] != '"' {
		return "", quoteStart, false
	}
	value, consumed, complete := decodePartialJSONString(input[quoteStart+1:])
	return value, quoteStart + 1 + consumed, complete
}

func decodePartialJSONString(input string) (string, int, bool) {
	var builder strings.Builder
	for i := 0; i < len(input); i++ {
		ch := input[i]
		switch ch {
		case '"':
			return builder.String(), i + 1, true
		case '\\':
			if i+1 >= len(input) {
				return builder.String(), i, false
			}
			next := input[i+1]
			switch next {
			case '"', '\\', '/':
				builder.WriteByte(next)
				i++
			case 'b':
				builder.WriteByte('\b')
				i++
			case 'f':
				builder.WriteByte('\f')
				i++
			case 'n':
				builder.WriteByte('\n')
				i++
			case 'r':
				builder.WriteByte('\r')
				i++
			case 't':
				builder.WriteByte('\t')
				i++
			case 'u':
				if i+6 > len(input) {
					return builder.String(), i, false
				}
				value, err := strconv.ParseInt(input[i+2:i+6], 16, 32)
				if err != nil {
					return builder.String(), i, false
				}
				builder.WriteRune(rune(value))
				i += 5
			default:
				return builder.String(), i, false
			}
		default:
			builder.WriteByte(ch)
		}
	}
	return builder.String(), len(input), false
}

func skipJSONWhitespace(input string, index int) int {
	for index < len(input) {
		switch input[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}
