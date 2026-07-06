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
) (planningResult, bool, error) {
	streamReq := cloneChatRequest(planningReq)
	streamReq.Stream = true
	startedAt := time.Now()
	stream, fallbackProgressStreamed, err := r.openSkillPlanningStream(ctx, prepared, streamReq, onEvent)
	if err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:      "skill_planning",
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
	sawToolCall := false
	answerStreamed := false
	naturalProgressStreamed := false
	toolPlanningProgressStreamed := false
	var speculativeAnswer strings.Builder
	fallbackTimer := time.NewTimer(r.fallbackDelay())
	defer fallbackTimer.Stop()

	for {
		select {
		case <-fallbackTimer.C:
			if !answerStreamed && !naturalProgressStreamed && !toolPlanningProgressStreamed && !fallbackProgressStreamed {
				fallbackProgressStreamed = r.emitPlanningFallbackProgress(ctx, prepared, onEvent)
			}
			continue
		case response, ok := <-stream:
			if !ok {
				goto streamDone
			}

			usage = mergeUsage(usage, response.Usage)
			if response.Error != nil {
				r.recordModelInvocation(ModelInvocationTrace{
					Phase:      "skill_planning",
					Round:      round,
					Streaming:  true,
					StartedAt:  startedAt,
					DurationMS: time.Since(startedAt).Milliseconds(),
					Request:    streamReq,
					Usage:      usage,
					Error:      response.Error.Error(),
				})
				return planningResult{}, true, response.Error
			}
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
						if !naturalProgressStreamed && r.emitAgentProgress(ctx, prepared, text, onEvent) {
							naturalProgressStreamed = true
						}
					} else {
						r.emitAnswerChunk(ctx, prepared, text, onEvent)
						speculativeAnswer.WriteString(text)
						answerStreamed = true
					}
				}
				for _, delta := range choice.Delta.ToolCalls {
					if !sawToolCall {
						sawToolCall = true
						if speculative := speculativeAnswer.String(); speculative != "" {
							r.emitAnswerRetract(ctx, prepared, speculative, onEvent)
						}
						if progress := strings.TrimSpace(contentBuilder.String()); progress != "" {
							if r.emitAgentProgress(ctx, prepared, progress, onEvent) {
								naturalProgressStreamed = true
							}
						}
					}
					state := mergeStreamingToolCall(toolCallsByIndex, &toolCallOrder, delta)
					if state == nil {
						continue
					}
					if (!toolPlanningProgressStreamed || isStreamingBusinessToolCall(state)) && (!naturalProgressStreamed || isStreamingBusinessToolCall(state)) && r.emitStreamingToolPlanningProgress(ctx, prepared, state, onEvent) {
						toolPlanningProgressStreamed = true
					}
					r.emitStreamingIntermediateAnswerDelta(ctx, prepared, round, state, onEvent)
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
	message := adapter.Message{
		Role:             "assistant",
		Content:          contentBuilder.String(),
		ToolCalls:        toolCalls,
		ReasoningContent: reasoningBuilder.String(),
	}
	r.recordModelInvocation(ModelInvocationTrace{
		Phase:      "skill_planning",
		Round:      round,
		Streaming:  true,
		StartedAt:  startedAt,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Request:    streamReq,
		Response:   &message,
		Usage:      usage,
	})

	return planningResult{
		message:          message,
		usage:            usage,
		answerStreamed:   answerStreamed && len(toolCalls) == 0,
		progressStreamed: naturalProgressStreamed || toolPlanningProgressStreamed || fallbackProgressStreamed,
	}, true, nil
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
) (<-chan adapter.StreamResponse, bool, error) {
	resultCh := make(chan skillPlanningStreamOpenResult, 1)
	go func() {
		stream, err := r.LLMClient.AppChatStream(ctx, r.AppContext, streamReq)
		resultCh <- skillPlanningStreamOpenResult{stream: stream, err: err}
	}()

	timer := time.NewTimer(r.fallbackDelay())
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.stream, false, result.err
	case <-timer.C:
		fallbackProgressStreamed := r.emitPlanningFallbackProgress(ctx, prepared, onEvent)
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
	r.emitEvent(EventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, delta, 0, false, "streaming"))
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
	r.emitEvent(EventAgentProgress, payload)
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
	r.emitEvent(EventAgentProgress, payload)
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
