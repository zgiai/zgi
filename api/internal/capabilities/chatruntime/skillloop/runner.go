package skillloop

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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

type skillStepResult struct {
	trace             skills.SkillTrace
	toolMessage       adapter.Message
	answer            string
	usedSkill         bool
	usedTool          bool
	recoverable       bool
	terminal          bool
	pendingApproval   map[string]interface{}
	pendingQuestion   map[string]interface{}
	pendingGovernance map[string]interface{}
	fatalErr          error
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

func (r *Runner) Run(ctx context.Context, req RunRequest) (string, *adapter.Usage, error) {
	prepared := req.Prepared
	resolved := req.Resolved
	if r == nil || r.SkillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if r.LLMClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
	if prepared == nil || prepared.LLMRequest == nil {
		return "", nil, fmt.Errorf("%w: prepared chat is invalid", ErrInvalidInput)
	}
	if resolved == nil || len(resolved.Skills) == 0 {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	messages = append(messages, metadataMessage)
	messages = append(messages, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
	messages = append(messages, agenticSkillLoopSystemMessage())
	traces := []skills.SkillTrace{metadataExposedTrace(resolved.SkillIDs(), metadataStats)}
	r.recordTrace(traces, traces[0])
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
	attemptedToolCalls := []SkillToolCallRef{}
	successfulToolCalls := []SkillToolCallRef{}
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

		planningResult, err := r.runSkillPlanning(ctx, prepared, planningReq, round, req.OnChunk)
		if err != nil {
			return answerBuilder.String(), usage, err
		}
		usage = mergeUsage(usage, planningResult.usage)
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		text := assistantMessageText(planningMessage)
		if text != "" && len(toolCalls) > 0 && !planningResult.progressStreamed {
			r.emitAgentProgress(ctx, prepared, text, nil)
		}
		if len(toolCalls) == 0 {
			if guardResult, blocked := runFinalAnswerGuard(req.FinalAnswerGuard, FinalAnswerGuardRequest{
				Answer:              text,
				Round:               round,
				SkillUsed:           skillUsed,
				ToolCallCount:       toolCallCount,
				AttemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
				SuccessfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
			}); blocked {
				finalAnswerGuardBlockCount++
				if planningResult.answerStreamed && text != "" {
					r.emitAnswerRetract(ctx, prepared, text, nil)
				}
				trace := finalAnswerGuardrailTrace(guardResult)
				traces = append(traces, trace)
				r.recordTrace(traces, trace)
				r.logSkillTrace(ctx, prepared, trace)
				if finalAnswerGuardBlockCount > defaultMaxConsecutiveRecoverableFailureRounds {
					err := fmt.Errorf("%w: final answer guard blocked too many consecutive replies", ErrInvalidInput)
					r.emitSkillError(ctx, prepared, failedSkillTrace("guardrail", guardResult.ToolName, err))
					return answerBuilder.String(), usage, err
				}
				messages = append(messages, finalAnswerGuardSystemMessage(guardResult, text))
				continue
			}
		}
		if len(toolCalls) == 0 && prepared.parts.SkillMode == "required" && !skillUsed {
			return answerBuilder.String(), usage, fmt.Errorf("%w: required skill was not used", ErrInvalidInput)
		}
		if text != "" && len(toolCalls) == 0 {
			answerBuilder.WriteString(text)
			if !planningResult.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, text, nil)
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
			result := r.handleProgressiveSkillCall(ctx, prepared, resolved, call, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, userInputGuardState{
				guard:               req.UserInputGuard,
				round:               round,
				skillUsed:           skillUsed,
				toolCallCount:       toolCallCount,
				attemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
				successfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
			}, nil)
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable {
				r.emitSkillError(ctx, prepared, result.trace)
				roundHadRecoverableFailure = true
				lastRecoverableTrace = result.trace
				recoverableFailureCallCount++
			} else {
				roundHadSuccess = true
			}
			if result.fatalErr != nil {
				if !result.recoverable {
					r.emitSkillError(ctx, prepared, result.trace)
				}
				return answerBuilder.String(), usage, result.fatalErr
			}
			if result.usedSkill {
				skillUsed = true
			}
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") {
				attemptedToolCalls = append(attemptedToolCalls, SkillToolCallRef{
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
					successfulToolCalls = append(successfulToolCalls, SkillToolCallRef{
						SkillID:   strings.TrimSpace(result.trace.SkillID),
						ToolName:  strings.TrimSpace(result.trace.ToolName),
						Arguments: copyStringAnyMap(result.trace.Arguments),
					})
					finalAnswerGuardBlockCount = 0
				}
			}
			if result.pendingApproval != nil {
				return answerBuilder.String(), usage, &WorkflowApprovalPendingError{Payload: result.pendingApproval}
			}
			if result.pendingQuestion != nil {
				return answerBuilder.String(), usage, &WorkflowQuestionPendingError{Payload: result.pendingQuestion}
			}
			if result.pendingGovernance != nil {
				return answerBuilder.String(), usage, &ToolGovernancePendingError{Payload: result.pendingGovernance}
			}
			if result.answer != "" {
				appendAnswerText(&answerBuilder, result.answer)
				r.emitAnswerChunk(ctx, prepared, result.answer, nil)
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
			if message, ok := governedReadFileTargetSystemMessage(result.trace); ok {
				messages = append(messages, message)
			}
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
				r.emitSkillError(ctx, prepared, trace)
				return answerBuilder.String(), usage, err
			}
		} else {
			consecutiveRecoverableFailureRounds = 0
		}
	}

	return answerBuilder.String(), usage, fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
}

func validAdditionalSystemMessages(input []adapter.Message) []adapter.Message {
	out := make([]adapter.Message, 0, len(input))
	for _, message := range input {
		content := strings.TrimSpace(messageContent(message.Content))
		if content == "" {
			continue
		}
		message.Role = "system"
		message.Content = content
		message.ToolCalls = nil
		out = append(out, message)
	}
	return out
}

func governedReadFileTargetSystemMessage(trace skills.SkillTrace) (adapter.Message, bool) {
	if trace.Governance == nil ||
		!strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillFileReader) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "read_file") {
		return adapter.Message{}, false
	}
	decision := trace.Governance
	if decision.Status != toolgovernance.DecisionStatusAllowed ||
		decision.Manifest.Effect != toolgovernance.EffectRead ||
		!strings.EqualFold(strings.TrimSpace(decision.Manifest.AssetType), "file") {
		return adapter.Message{}, false
	}
	assets := decision.ExpectedAssets
	if len(assets) == 0 {
		assets = decision.Assets
	}
	if len(assets) != 1 {
		return adapter.Message{}, false
	}
	fileID := strings.TrimSpace(assets[0].ID)
	fileName := strings.TrimSpace(assets[0].Name)
	if fileID == "" && fileName == "" {
		return adapter.Message{}, false
	}
	target := fileName
	if target == "" {
		target = fileID
	}
	content := strings.Join([]string{
		"Authoritative files-page target feedback:",
		fmt.Sprintf("The tool result above is for the resolved file target %q.", target),
		"Use that resolved file name and the returned file content as the only source for the final answer.",
		"Any earlier assistant progress text, assistant tool-call arguments, or visible-file ordinal interpretation that named a different file is incorrect for this turn.",
		"Do not mention this correction, internal resolution, governance, redirects, caches, mismatched IDs, or internal file IDs in the final answer. Simply answer the user's request from the resolved file content.",
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func runFinalAnswerGuard(guard FinalAnswerGuard, req FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The previous candidate final answer was blocked because a required skill/tool call has not succeeded in this turn. Continue planning and call the required skill/tool before claiming completion."
	}
	return result, true
}

func runUserInputGuard(guard UserInputGuard, req UserInputGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The requested user clarification was blocked because runtime context already contains the information needed to continue. Continue planning and call the required skill/tool before asking the user."
	}
	return result, true
}

func finalAnswerGuardrailTrace(result FinalAnswerGuardResult) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(result.SkillID),
		ToolName: strings.TrimSpace(result.ToolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(result.Message),
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}
}

func userInputGuardrailTrace(result FinalAnswerGuardResult) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(result.SkillID),
		ToolName: strings.TrimSpace(result.ToolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(result.Message),
		Arguments: map[string]interface{}{
			"blocked_tool": "request_user_input",
			"next_step":    "continue_planning",
		},
	}
}

func finalAnswerGuardSystemMessage(result FinalAnswerGuardResult, candidateAnswer string) adapter.Message {
	lines := []string{
		"Runtime guardrail feedback:",
		strings.TrimSpace(result.Message),
	}
	if text := strings.TrimSpace(candidateAnswer); text != "" {
		lines = append(lines, "Blocked candidate answer:\n"+text)
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func messageContent(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
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

func (r *Runner) runSkillPlanning(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error) (planningResult, error) {
	if shouldStreamSkillPlanning(prepared) {
		result, ok, err := r.runSkillPlanningStream(ctx, prepared, planningReq, round, nil)
		if err != nil {
			return planningResult{}, err
		}
		if ok {
			return result, nil
		}
	}

	planningReq.Stream = false
	startedAt := time.Now()
	planningResp, err := r.LLMClient.AppChat(ctx, r.AppContext, planningReq)
	if err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:      "skill_planning",
			Round:      round,
			Streaming:  false,
			StartedAt:  startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Request:    planningReq,
			Error:      err.Error(),
		})
		return planningResult{}, err
	}
	message := firstPlanningMessage(planningResp)
	usage := planningRespUsage(planningResp)
	r.recordModelInvocation(ModelInvocationTrace{
		Phase:      "skill_planning",
		Round:      round,
		Streaming:  false,
		StartedAt:  startedAt,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Request:    planningReq,
		Response:   &message,
		Usage:      usage,
	})
	return planningResult{message: message, usage: usage}, nil
}

func (r *Runner) emitAnswerChunk(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) {
	if text == "" {
		return
	}
	r.emitEvent(EventMessage, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          text,
	})
}

func (r *Runner) emitAnswerRetract(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) {
	if text == "" {
		return
	}
	r.emitEvent(EventMessageRetract, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         text,
		"length":          utf16CodeUnitLength(text),
		"created_at":      time.Now().Unix(),
	})
}

func utf16CodeUnitLength(text string) int {
	return len(utf16.Encode([]rune(text)))
}

func (r *Runner) emitAgentProgress(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) {
	content := strings.TrimSpace(text)
	if content == "" {
		return
	}
	r.emitEvent(EventAgentProgress, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         content,
		"created_at":      time.Now().Unix(),
	})
}

func agenticSkillLoopSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
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
	}, "\n")}
}

func AgenticSkillLoopSystemMessage() adapter.Message {
	return agenticSkillLoopSystemMessage()
}
