package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (r *Runner) runSkillPlanningStream(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onEvent func(Event) error,
	terminalProtocol bool,
	terminalStreamingAllowed bool,
	suppressNaturalProgress bool,
) (planningResult, bool, error) {
	return r.runModelToolRoundStream(ctx, prepared, planningReq, round, onEvent, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, "skill_planning", nil)
}

func (r *Runner) runModelToolRoundStream(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onEvent func(Event) error,
	terminalProtocol bool,
	terminalStreamingAllowed bool,
	suppressNaturalProgress bool,
	phase string,
	modelProgress *modelProgressTracker,
) (planningResult, bool, error) {
	streamReq := cloneChatRequest(planningReq)
	streamReq.Stream = true
	startedAt := time.Now()
	stream, fallbackProgressStreamed, err := r.openSkillPlanningStream(ctx, prepared, streamReq, onEvent, phase == "agent_tool_loop")
	if err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:      phase,
			Round:      round,
			Streaming:  true,
			StartedAt:  startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Request:    streamReq,
			Error:      err.Error(),
		})
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
	answerStreamed := false
	naturalAnswerStreamed := false
	toolPlanningProgressStreamed := false
	finishReason := ""
	streamDoneReceived := false
	terminatedBy := ""
	var fallbackTimer *time.Timer
	var fallbackTimerC <-chan time.Time
	if phase != "agent_tool_loop" {
		fallbackTimer = time.NewTimer(r.fallbackDelay())
		fallbackTimerC = fallbackTimer.C
		defer fallbackTimer.Stop()
	}
	idleTimer := time.NewTimer(r.modelIdleTimeout())
	defer idleTimer.Stop()

	for {
		select {
		case <-idleTimer.C:
			err := ErrModelIdleTimeout
			r.recordModelInvocation(ModelInvocationTrace{
				Phase:      phase,
				Round:      round,
				Streaming:  true,
				StartedAt:  startedAt,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Request:    streamReq,
				Usage:      usage,
				Error:      err.Error(),
			})
			logger.WarnContext(ctx, "chat runtime model idle timeout",
				"message_id", prepared.Message.ID.String(),
				"provider", prepared.parts.Provider,
				"model", streamReq.Model,
				"phase", phase+"_stream",
			)
			return planningResult{usage: usage}, true, wrapStreamedFinalAnswerError(err, streamedFinalAnswerFromStates(toolCallsByIndex, toolCallOrder))
		case <-fallbackTimerC:
			if !answerStreamed && !naturalAnswerStreamed && !toolPlanningProgressStreamed && !fallbackProgressStreamed {
				fallbackProgressStreamed = r.emitPlanningFallbackProgress(ctx, prepared, onEvent)
			}
			continue
		case response, ok := <-stream:
			resetPlanningIdleTimer(idleTimer, r.modelIdleTimeout())
			if !ok {
				terminatedBy = "channel_closed"
				goto streamDone
			}

			usage = mergeStreamUsageSnapshot(usage, response.Usage)
			if response.Error != nil {
				r.recordModelInvocation(ModelInvocationTrace{
					Phase:      phase,
					Round:      round,
					Streaming:  true,
					StartedAt:  startedAt,
					DurationMS: time.Since(startedAt).Milliseconds(),
					Request:    streamReq,
					Usage:      usage,
					Error:      response.Error.Error(),
				})
				return planningResult{usage: usage}, true, wrapStreamedFinalAnswerError(response.Error, streamedFinalAnswerFromStates(toolCallsByIndex, toolCallOrder))
			}
			if len(response.Choices) == 0 {
				if response.Done {
					streamDoneReceived = true
					terminatedBy = "done"
					goto streamDone
				}
				continue
			}
			sawChunk = true
			for _, choice := range response.Choices {
				if reason := strings.TrimSpace(choice.FinishReason); reason != "" {
					finishReason = reason
					if terminatedBy == "" {
						terminatedBy = "finish_reason"
					}
				}
				if reasoning := streamChoiceReasoningContent(choice); reasoning != "" {
					reasoningBuilder.WriteString(reasoning)
					modelProgress.ObserveReasoningDelta()
				}
				if text := streamChoiceText(choice); text != "" {
					contentBuilder.WriteString(text)
					if !terminalProtocol && !suppressNaturalProgress && phase != "agent_tool_loop" {
						r.emitAnswerChunk(ctx, prepared, text, onEvent)
						answerStreamed = true
						naturalAnswerStreamed = true
					}
				}
				if len(choice.Delta.ToolCalls) > 0 {
					modelProgress.ObserveToolCallDelta()
				}
				for _, delta := range choice.Delta.ToolCalls {
					state := mergeStreamingToolCall(toolCallsByIndex, &toolCallOrder, delta)
					if state == nil {
						continue
					}
					if phase != "agent_tool_loop" &&
						(!toolPlanningProgressStreamed || isStreamingBusinessToolCall(state)) &&
						r.emitStreamingToolPlanningProgress(ctx, prepared, state, onEvent) {
						toolPlanningProgressStreamed = true
					}
					r.emitStreamingIntermediateAnswerDelta(ctx, prepared, round, state, onEvent)
					if terminalProtocol && terminalStreamingAllowed && len(toolCallsByIndex) == 1 {
						r.emitStreamingFinalAnswerDelta(ctx, prepared, state, onEvent)
					}
				}
			}
			if response.Done {
				streamDoneReceived = true
				terminatedBy = "done"
				goto streamDone
			}
		}
	}

streamDone:
	if !sawChunk {
		if phase == "agent_tool_loop" {
			err := fmt.Errorf("agent tool loop stream ended without a terminal signal: terminated_by=%s", strings.TrimSpace(terminatedBy))
			r.recordModelInvocation(ModelInvocationTrace{
				Phase:              phase,
				Round:              round,
				Streaming:          true,
				StartedAt:          startedAt,
				DurationMS:         time.Since(startedAt).Milliseconds(),
				Request:            streamReq,
				Usage:              usage,
				FinishReason:       finishReason,
				StreamDoneReceived: streamDoneReceived,
				TerminatedBy:       terminatedBy,
				Error:              err.Error(),
			})
			return planningResult{usage: usage}, true, err
		}
		return planningResult{}, false, nil
	}
	toolCalls := make([]adapter.ToolCall, 0, len(toolCallOrder))
	finalAnswerStreamDiverged := false
	for _, index := range toolCallOrder {
		state := toolCallsByIndex[index]
		if state == nil {
			continue
		}
		call := state.call
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolIntermediateAnswer) && state.emittedContent != "" {
			call.Function.Arguments = markIntermediateAnswerArgumentsStreamed(call.Function.Arguments, streamingIntermediateAnswerID(prepared, round, call))
		}
		if strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolFinalAnswer) && state.emittedFinalAnswer != "" {
			answer, complete := partialJSONStringField(call.Function.Arguments, "answer")
			if complete && answer == state.emittedFinalAnswer {
				call.Function.Arguments = markFinalAnswerArgumentsStreamed(call.Function.Arguments)
			} else if complete {
				finalAnswerStreamDiverged = true
			}
			answerStreamed = true
		}
		toolCalls = append(toolCalls, call)
	}
	if phase == "agent_tool_loop" && !terminalProtocol && !suppressNaturalProgress && len(toolCalls) == 0 {
		if content := contentBuilder.String(); content != "" {
			modelProgress.Stop()
			r.emitAnswerChunk(ctx, prepared, content, onEvent)
			answerStreamed = true
		}
	}
	if !terminalProtocol && len(toolCalls) > 0 && naturalAnswerStreamed {
		r.emitAnswerRetract(ctx, prepared, contentBuilder.String())
	}
	message := adapter.Message{
		Role:             "assistant",
		Content:          contentBuilder.String(),
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningBuilder.String(),
	}
	terminationErr := skillPlanningStreamTerminationError(finishReason, streamDoneReceived, terminatedBy)
	if terminationErr == nil && finalAnswerStreamDiverged {
		terminationErr = fmt.Errorf("streamed final answer diverged from completed tool arguments")
	}
	trace := ModelInvocationTrace{
		Phase:              phase,
		Round:              round,
		Streaming:          true,
		StartedAt:          startedAt,
		DurationMS:         time.Since(startedAt).Milliseconds(),
		Request:            streamReq,
		Response:           &message,
		Usage:              usage,
		FinishReason:       finishReason,
		StreamDoneReceived: streamDoneReceived,
		TerminatedBy:       terminatedBy,
	}
	if terminationErr != nil {
		trace.Error = terminationErr.Error()
	}
	r.recordModelInvocation(trace)
	if terminationErr != nil {
		return planningResult{
			message:               message,
			usage:                 usage,
			answerStreamed:        answerStreamed && (terminalProtocol || len(toolCalls) == 0),
			naturalAnswerStreamed: naturalAnswerStreamed && len(toolCalls) > 0,
		}, true, wrapStreamedFinalAnswerError(terminationErr, streamedFinalAnswerFromStates(toolCallsByIndex, toolCallOrder))
	}

	return planningResult{
		message:               message,
		usage:                 usage,
		answerStreamed:        answerStreamed && (terminalProtocol || len(toolCalls) == 0),
		naturalAnswerStreamed: naturalAnswerStreamed && len(toolCalls) > 0,
	}, true, nil
}

func resetPlanningIdleTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

type streamedFinalAnswerError struct {
	err    error
	answer string
}

func (e *streamedFinalAnswerError) Error() string {
	return e.err.Error()
}

func (e *streamedFinalAnswerError) Unwrap() error {
	return e.err
}

func wrapStreamedFinalAnswerError(err error, answer string) error {
	if err == nil || strings.TrimSpace(answer) == "" {
		return err
	}
	return &streamedFinalAnswerError{err: err, answer: answer}
}

func streamedFinalAnswerFromStates(states map[int]*streamingToolCallState, order []int) string {
	for _, index := range order {
		state := states[index]
		if state != nil && strings.EqualFold(strings.TrimSpace(state.call.Function.Name), skills.MetaToolFinalAnswer) {
			return state.emittedFinalAnswer
		}
	}
	return ""
}

func skillPlanningStreamTerminationError(finishReason string, streamDoneReceived bool, terminatedBy string) error {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "length", "max_tokens":
		return &PlanningTerminationError{Reason: strings.TrimSpace(finishReason), Recoverable: true, Streaming: true}
	case "content_filter":
		return &PlanningTerminationError{Reason: strings.TrimSpace(finishReason), Streaming: true}
	}
	if streamDoneReceived || strings.TrimSpace(finishReason) != "" {
		return nil
	}
	return fmt.Errorf("skill planning stream ended without a terminal signal: terminated_by=%s", strings.TrimSpace(terminatedBy))
}

type skillPlanningStreamOpenResult struct {
	stream <-chan adapter.StreamResponse
	err    error
}

func (r *Runner) openSkillPlanningStream(
	ctx context.Context,
	prepared *PreparedChat,
	streamReq *adapter.ChatRequest,
	onEvent func(Event) error,
	suppressFallback bool,
) (<-chan adapter.StreamResponse, bool, error) {
	resultCh := make(chan skillPlanningStreamOpenResult, 1)
	callCtx, cancel := context.WithCancel(ctx)
	go func() {
		stream, err := r.LLMClient.AppChatStream(callCtx, r.AppContext, streamReq)
		resultCh <- skillPlanningStreamOpenResult{stream: stream, err: err}
	}()

	var timer *time.Timer
	var timerC <-chan time.Time
	if !suppressFallback {
		timer = time.NewTimer(r.fallbackDelay())
		timerC = timer.C
		defer timer.Stop()
	}
	idleTimer := time.NewTimer(r.modelIdleTimeout())
	defer idleTimer.Stop()

	select {
	case result := <-resultCh:
		if result.err != nil {
			cancel()
		}
		return result.stream, false, result.err
	case <-idleTimer.C:
		cancel()
		return nil, false, ErrModelIdleTimeout
	case <-timerC:
		fallbackProgressStreamed := r.emitPlanningFallbackProgress(ctx, prepared, onEvent)
		select {
		case result := <-resultCh:
			if result.err != nil {
				cancel()
			}
			return result.stream, fallbackProgressStreamed, result.err
		case <-idleTimer.C:
			cancel()
			return nil, fallbackProgressStreamed, ErrModelIdleTimeout
		case <-ctx.Done():
			cancel()
			return nil, fallbackProgressStreamed, ctx.Err()
		}
	case <-ctx.Done():
		cancel()
		return nil, false, ctx.Err()
	}
}

func shouldStreamSkillPlanning(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	if prepared.LLMRequest != nil && isQwQModel(prepared.LLMRequest.Model) {
		return true
	}
	provider := strings.ToLower(strings.TrimSpace(prepared.parts.Provider))
	switch provider {
	case "openai", "openai-compatible", "deepseek", "openrouter", "zgi-cloud", "zgi_cloud", "dashscope",
		"aliyun", "qwen", "claude", "anthropic", "moonshotai", "moonshotai-cn", "siliconflow", "agicto", "glm":
		return true
	default:
		return false
	}
}

func isQwQModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = strings.TrimSpace(model[idx+1:])
	}
	return strings.HasPrefix(model, "qwq")
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

func (r *Runner) emitStreamingIntermediateAnswerDelta(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	state *streamingToolCallState,
	onEvent func(Event) error,
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
	r.emitEvent(prepared, EventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, delta, 0, false, "streaming"))
}

func (r *Runner) emitStreamingFinalAnswerDelta(
	ctx context.Context,
	prepared *PreparedChat,
	state *streamingToolCallState,
	onEvent func(Event) error,
) {
	if state == nil || !strings.EqualFold(strings.TrimSpace(state.call.Function.Name), skills.MetaToolFinalAnswer) {
		return
	}
	answer, _ := partialJSONStringField(state.call.Function.Arguments, "answer")
	if answer == "" || len(answer) <= len(state.emittedFinalAnswer) || !strings.HasPrefix(answer, state.emittedFinalAnswer) {
		return
	}
	delta := answer[len(state.emittedFinalAnswer):]
	state.emittedFinalAnswer = answer
	r.emitAnswerChunk(ctx, prepared, delta, onEvent)
}

func (r *Runner) emitStreamingToolPlanningProgress(
	ctx context.Context,
	prepared *PreparedChat,
	state *streamingToolCallState,
	onEvent func(Event) error,
) bool {
	if state == nil {
		return false
	}
	metaToolName := strings.TrimSpace(state.call.Function.Name)
	if metaToolName == "" || strings.EqualFold(metaToolName, skills.MetaToolIntermediateAnswer) || strings.EqualFold(metaToolName, skills.MetaToolFinalAnswer) {
		return false
	}
	if metaToolName == skills.MetaToolActivateSkills || metaToolName == skills.MetaToolSearchSkills {
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
	r.emitEvent(prepared, EventAgentProgress, payload)
	return true
}

func (r *Runner) emitPlanningFallbackProgress(
	ctx context.Context,
	prepared *PreparedChat,
	onEvent func(Event) error,
) bool {
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"phase":           "planning",
		"created_at":      time.Now().Unix(),
	}
	r.emitEvent(prepared, EventAgentProgress, payload)
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

func markFinalAnswerArgumentsStreamed(arguments string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return arguments
	}
	parsed[streamedFinalAnswerArg] = true
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return arguments
	}
	return string(encoded)
}
