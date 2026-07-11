package skillloop

import (
	"context"
	"encoding/json"
	"errors"
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
	streamedFinalAnswerArg                        = "_aichat_streamed_final_answer"
	userInputContinuationAnswered                 = "answered"
	userInputContinuationReplan                   = "replan_after_user_input"
	restoredSkillInstructionsTotalBudgetChars     = 16000
	restoredSkillInstructionsPerSkillBudgetChars  = 10000
)

type skillStepResult struct {
	trace               skills.SkillTrace
	toolMessage         adapter.Message
	toolResult          map[string]interface{}
	answer              string
	answerStreamed      bool
	usedSkill           bool
	usedTool            bool
	recoverable         bool
	terminal            bool
	pendingApproval     map[string]interface{}
	pendingQuestion     map[string]interface{}
	pendingGovernance   map[string]interface{}
	pendingClientAction map[string]interface{}
	pendingUserInput    map[string]interface{}
	fatalErr            error
}

type planningResult struct {
	message                 adapter.Message
	usage                   *adapter.Usage
	answerStreamed          bool
	naturalProgressStreamed bool
}

type streamingToolCallState struct {
	call                    adapter.ToolCall
	emittedContent          string
	emittedFinalAnswer      string
	emittedPlanningProgress bool
	emittedPlanningSkillID  string
	emittedPlanningToolName string
}

func metaToolsForRun(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, preferExplicitFinalAnswer bool) []adapter.Tool {
	tools := skills.MetaToolsForSkillState(resolved, loadedSkills)
	if preferExplicitFinalAnswer {
		return tools
	}

	filtered := make([]adapter.Tool, 0, len(tools))
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Function.Name), skills.MetaToolFinalAnswer) {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
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
	preferExplicitFinalAnswer := req.PreferExplicitFinalAnswer
	loadedSkills := initialLoadedSkillsForRun(req, resolved)

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	messages = append(messages, metadataMessage)
	if restored := restoredLoadedSkillInstructionsMessage(resolved, loadedSkills); restored != nil {
		messages = append(messages, *restored)
	}
	messages = append(messages, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
	messages = append(messages, agenticSkillLoopSystemMessage(preferExplicitFinalAnswer))
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
	emptyFinalAnswerRetryCount := 0
	skillToolCallCounts := map[string]int{}
	attemptedToolCalls := []SkillToolCallRef{}
	successfulToolCalls := []SkillToolCallRef{}
	successfulToolCallsByKey := map[string]SkillToolCallRef{}
	failedToolCallReasons := map[string]string{}
	skillUsed := false
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	terminalStateGuardConfigured := req.RuntimeStateSnapshot != nil
	finalAnswerGuard := req.FinalAnswerGuard
	userInputGuard := req.UserInputGuard
	toolCallGuard := req.ToolCallGuard
	if terminalStateGuardConfigured {
		// The model owns tool selection and completion for agentic turns. Runtime authorization,
		// approval, and the terminal state guard enforce protocol and safety boundaries.
		// Keep the user-input guard so redundant clarification requests can still replan instead of
		// interrupting a task that already has enough evidence to continue.
		// Tool governance and backend authorization still enforce hard safety boundaries.
		finalAnswerGuard = nil
		toolCallGuard = nil
	}
	var answerBuilder strings.Builder
	var usage *adapter.Usage
	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = metaToolsForRun(resolved, loadedSkills, preferExplicitFinalAnswer)
		planningReq.ToolChoice = "auto"

		roundRuntimeState := map[string]interface{}{}
		terminalSubmissionAllowed := true
		if terminalStateGuardConfigured {
			roundRuntimeState = runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
			terminalSubmissionAllowed = terminalStateGuardCanStream(roundRuntimeState)
		}
		suppressNaturalProgress := req.SuppressInitialNaturalProgress && round == 0
		deferTerminalContent := preferExplicitFinalAnswer || !terminalSubmissionAllowed
		planningResult, err := r.runSkillPlanning(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress)
		usage = mergeUsage(usage, planningResult.usage)
		if err != nil {
			var streamedErr *streamedFinalAnswerError
			if errors.As(err, &streamedErr) {
				appendAnswerText(&answerBuilder, strings.TrimSpace(streamedErr.answer))
			}
			return answerBuilder.String(), usage, err
		}
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		text := assistantMessageText(planningMessage)
		if text != "" && len(toolCalls) > 0 && !suppressNaturalProgress && !planningResult.naturalProgressStreamed && shouldEmitNaturalProgressForToolCalls(resolved, loadedSkills, toolCalls) {
			r.emitAgentProgress(ctx, prepared, text, nil)
		}
		if len(toolCalls) == 0 {
			if guardResult, blocked := runFinalAnswerGuard(finalAnswerGuard, FinalAnswerGuardRequest{
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
		if len(toolCalls) == 0 && terminalStateGuardConfigured {
			if strings.TrimSpace(text) == "" {
				emptyFinalAnswerRetryCount++
				if emptyFinalAnswerRetryCount <= 1 {
					messages = append(messages, adapter.Message{
						Role:    "system",
						Content: "The previous assistant turn returned neither a tool call nor a user-visible answer. Continue from the latest context: call another tool if work remains, otherwise provide the final answer directly.",
					})
					continue
				}
				return answerBuilder.String(), usage, fmt.Errorf("%w: model returned no final answer", ErrInvalidInput)
			}
			emptyFinalAnswerRetryCount = 0
			guard := terminalStateGuardEvaluate(roundRuntimeState, text)
			terminalStateGuardRecord(req, guard)
			if guard.Path != terminalStateGuardAccepted {
				terminalStateGuardNotify(req, guard)
				return answerBuilder.String(), usage, terminalStateGuardError(guard)
			}
			answer := strings.TrimSpace(firstNonEmptyString(guard.FinalAnswer, text))
			appendAnswerText(&answerBuilder, answer)
			if !planningResult.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, answer, nil)
			}
			terminalStateGuardNotify(req, guard)
			return answerBuilder.String(), usage, nil
		}
		emptyFinalAnswerRetryCount = 0
		if call, ok := finalAnswerCall(toolCalls); ok && preferExplicitFinalAnswer {
			submission, parseErr := parseFinalAnswerSubmission(call, roundRuntimeState)
			if parseErr != nil {
				result := failedFinalAnswerSkillStep(call.ID, parseErr, "submit a complete final answer in a new user turn")
				traces = append(traces, result.trace)
				r.recordTrace(traces, result.trace)
				r.logSkillTrace(ctx, prepared, result.trace)
				if planningResult.answerStreamed {
					partialAnswer, _ := partialJSONStringField(call.Function.Arguments, "answer")
					appendAnswerText(&answerBuilder, strings.TrimSpace(partialAnswer))
				}
				return answerBuilder.String(), usage, parseErr
			}

			submission.streamed = submission.streamed || planningResult.answerStreamed
			guard := terminalStateGuardEvaluate(roundRuntimeState, submission.answer)
			terminalStateGuardRecord(req, guard)
			if guard.Path != terminalStateGuardAccepted {
				terminalStateGuardNotify(req, guard)
				return answerBuilder.String(), usage, terminalStateGuardError(guard)
			}

			result := finalAnswerSkillStep(call.ID, submission)
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			appendAnswerText(&answerBuilder, result.answer)
			if !result.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, result.answer, nil)
			}
			terminalStateGuardNotify(req, guard)
			logger.DebugContext(ctx, "aichat skill loop accepted explicit terminal answer",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"terminal_state_guard_path", string(guard.Path),
				"ignored_sibling_tool_calls", len(toolCalls)-1,
			)
			return answerBuilder.String(), usage, nil
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
		if preferExplicitFinalAnswer {
			// Process narration is user-visible progress, not durable model context.
			planningMessage.Content = ""
			planningMessage.ReasoningContent = ""
		}
		messages = append(messages, planningMessage)

		roundHadRecoverableFailure := false
		roundHadSuccess := false
		var lastRecoverableTrace skills.SkillTrace
		roundDeferredSystemMessages := []adapter.Message{}
		for _, call := range toolCalls {
			stepCount++
			callSkillID, callToolName, callToolArgs, failedCallKey := skillToolCallIdentityForCall(resolved, loadedSkills, call)
			callEvidence := runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
			result := skillStepResult{}
			if userInputPlanRevisionPending(req) && planRevisionRequiredForTool(callSkillID, callToolName) {
				result = pendingUserInputPlanRevisionStep(call.ID, callSkillID, callToolName, callToolArgs)
			}
			if result.trace.Kind == "" && failedCallKey != "" {
				if reason := failedToolCallReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
					result = repeatedFailedToolCallRecoverableStep(call.ID, callSkillID, callToolName, callToolArgs, reason)
				}
			}
			if result.trace.Kind == "" {
				result = repeatedSuccessfulReadOnlyToolCallFeedbackStep(call.ID, callSkillID, callToolName, callToolArgs, successfulToolCallsByKey, successfulToolCalls)
			}
			if result.trace.Kind == "" {
				result = r.handleProgressiveSkillCall(ctx, prepared, resolved, call, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, userInputGuardState{
					guard:               userInputGuard,
					toolCallGuard:       toolCallGuard,
					planToolGuard:       req.PlanToolGuard,
					argumentResolver:    req.ToolArgumentResolver,
					round:               round,
					skillUsed:           skillUsed,
					toolCallCount:       toolCallCount,
					attemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
					successfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
					completionEvidence:  callEvidence,
				}, nil)
			}
			if strings.TrimSpace(result.trace.Kind) == "" {
				if result.usedSkill {
					skillUsed = true
				}
				if result.usedTool {
					toolCallCount++
					incrementSkillToolCallCount(skillToolCallCounts, result.trace.SkillID)
				}
				if result.toolMessage.Role != "" || result.toolMessage.ToolCallID != "" || result.toolMessage.Content != nil {
					messages = append(messages, result.toolMessage)
				}
				continue
			}
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable && failedCallKey != "" && strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") {
				failedToolCallReasons[failedCallKey] = strings.TrimSpace(result.trace.Error)
				if failedToolCallReasons[failedCallKey] == "" {
					failedToolCallReasons[failedCallKey] = "previous tool call with the same arguments failed"
				}
			}
			if result.recoverable {
				if !internalPlannerFeedbackTrace(result.trace) {
					r.emitSkillError(ctx, prepared, result.trace)
				}
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
					Result:    copyStringAnyMap(result.toolResult),
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
						Result:    copyStringAnyMap(result.toolResult),
					})
					if failedCallKey != "" {
						successfulToolCallsByKey[failedCallKey] = SkillToolCallRef{
							SkillID:   strings.TrimSpace(callSkillID),
							ToolName:  strings.TrimSpace(callToolName),
							Arguments: copyStringAnyMap(callToolArgs),
							Result:    copyStringAnyMap(result.toolResult),
						}
					}
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
			if result.pendingClientAction != nil {
				return answerBuilder.String(), usage, &ClientActionPendingError{Payload: result.pendingClientAction}
			}
			if result.answer != "" {
				appendAnswerText(&answerBuilder, result.answer)
				r.emitAnswerChunk(ctx, prepared, result.answer, nil)
			}
			if result.pendingUserInput != nil {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				return answerBuilder.String(), usage, &UserInputPendingError{Payload: result.pendingUserInput}
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
				roundDeferredSystemMessages = append(roundDeferredSystemMessages, message)
			}
		}
		if len(roundDeferredSystemMessages) > 0 {
			messages = append(messages, roundDeferredSystemMessages...)
		}
		if preferExplicitFinalAnswer && !roundHadRecoverableFailure && terminalMetaCallsOnly(toolCalls) {
			messages = append(messages, adapter.Message{
				Role:    "system",
				Content: "The previous assistant turn only recorded internal state or plan progress. Continue the same user turn: call the next necessary business tool, request user input if blocked, or call submit_final_answer when the task is actually complete. Do not rely on ordinary assistant content from a meta-tool turn as the final answer.",
			})
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
				if !internalPlannerFeedbackTrace(lastRecoverableTrace) {
					r.emitSkillError(ctx, prepared, trace)
				}
				if terminalStateGuardConfigured {
					text := recoverableFailureFinalAnswer(lastRecoverableTrace, err)
					appendAnswerText(&answerBuilder, text)
					r.emitAnswerChunk(ctx, prepared, text, nil)
					return answerBuilder.String(), usage, nil
				}
				return answerBuilder.String(), usage, err
			}
		} else {
			consecutiveRecoverableFailureRounds = 0
		}
	}

	err := fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
	if terminalStateGuardConfigured {
		text := planningRoundsExhaustedFinalAnswer(err)
		appendAnswerText(&answerBuilder, text)
		r.emitAnswerChunk(ctx, prepared, text, nil)
		return answerBuilder.String(), usage, nil
	}
	return answerBuilder.String(), usage, err
}

func initialLoadedSkillsForRun(req RunRequest, resolved *skills.ResolvedSkills) map[string]struct{} {
	loaded := map[string]struct{}{}
	add := func(skillID string) {
		if canonical, ok := canonicalResolvedSkillID(resolved, skillID); ok {
			loaded[canonical] = struct{}{}
		}
	}
	metadata := currentMetadataForRun(req)
	for _, skillID := range evidenceStringSliceFromAny(metadata["loaded_skill_ids"]) {
		add(skillID)
	}
	for _, skillID := range evidenceStringSliceFromAny(metadata["loaded_skills"]) {
		add(skillID)
	}
	appendLoadedFromInvocations := func(invocations []map[string]interface{}) {
		for _, invocation := range invocations {
			kind := strings.TrimSpace(evidenceStringFromAny(invocation["kind"]))
			if !strings.EqualFold(kind, "skill_load") && !strings.EqualFold(kind, "tool_call") {
				continue
			}
			if !runtimeInvocationSucceeded(invocation) {
				continue
			}
			add(evidenceStringFromAny(invocation["skill_id"]))
		}
	}
	appendLoadedFromInvocations(evidenceMapsFromAny(metadata["skill_invocations"]))
	ledger := evidenceMapFromAny(metadata["execution_ledger"])
	appendLoadedFromInvocations(evidenceMapsFromAny(ledger["skill_invocations"]))
	appendLoadedFromInvocations(evidenceMapsFromAny(evidenceMapFromAny(ledger["summary"])["skill_invocations"]))
	return loaded
}

func restoredLoadedSkillInstructionsMessage(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}) *adapter.Message {
	if resolved == nil || len(loadedSkills) == 0 {
		return nil
	}
	sections := []string{
		"The following skill instructions were loaded earlier in this same user turn and remain active after navigation, approval, refresh, or continuation.",
		"Use them directly. Do not call load_skill for these skills again unless the runtime exposes that skill in load_skill.",
	}
	remaining := restoredSkillInstructionsTotalBudgetChars
	for _, skillID := range resolved.SkillIDs() {
		if remaining <= 0 {
			break
		}
		canonical, ok := canonicalResolvedSkillID(resolved, skillID)
		if !ok {
			continue
		}
		if _, ok := loadedSkills[canonical]; !ok {
			continue
		}
		doc, ok := resolved.Get(canonical)
		if !ok || doc == nil {
			continue
		}
		section := []string{"Restored skill: " + canonical}
		if description := strings.TrimSpace(doc.Metadata.Description); description != "" {
			section = append(section, "Description: "+description)
		}
		if whenToUse := strings.TrimSpace(doc.Metadata.WhenToUse); whenToUse != "" {
			section = append(section, "When to use: "+whenToUse)
		}
		if instructions := strings.TrimSpace(doc.Instructions); instructions != "" {
			budget := min(remaining, restoredSkillInstructionsPerSkillBudgetChars)
			instructions = compactRestoredSkillInstructions(instructions, budget)
			remaining -= len([]rune(instructions))
			section = append(section, "Instructions:\n"+instructions)
		}
		sections = append(sections, strings.Join(section, "\n"))
	}
	if len(sections) == 2 {
		return nil
	}
	return &adapter.Message{Role: "system", Content: strings.Join(sections, "\n\n")}
}

func compactRestoredSkillInstructions(instructions string, maxRunes int) string {
	instructions = strings.TrimSpace(instructions)
	if maxRunes <= 0 || instructions == "" {
		return ""
	}
	runes := []rune(instructions)
	if len(runes) <= maxRunes {
		return instructions
	}
	const marker = "\n\n[Detailed middle section omitted in continuation context. Use read_skill_reference or reload the skill only if those details are needed.]\n\n"
	markerRunes := []rune(marker)
	contentBudget := maxRunes - len(markerRunes)
	if contentBudget <= 0 {
		return strings.TrimSpace(string(runes[:maxRunes]))
	}
	headRunes := contentBudget * 2 / 3
	tailRunes := contentBudget - headRunes
	return strings.TrimSpace(string(runes[:headRunes])) + marker + strings.TrimSpace(string(runes[len(runes)-tailRunes:]))
}

func shouldEmitNaturalProgressForToolCalls(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, calls []adapter.ToolCall) bool {
	active := make(map[string]struct{}, len(loadedSkills))
	for skillID := range loadedSkills {
		active[skillID] = struct{}{}
	}
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		switch name {
		case skills.MetaToolLoadSkill:
			args, err := skills.ParseArguments(call.Function.Arguments)
			if err != nil {
				continue
			}
			if skillID, ok := canonicalResolvedSkillID(resolved, normalizedSkillArg(args, "skill_id")); ok {
				active[skillID] = struct{}{}
			}
		case skills.MetaToolCallSkillTool:
			args, err := skills.ParseArguments(call.Function.Arguments)
			if err != nil {
				continue
			}
			skillID, ok := canonicalResolvedSkillID(resolved, normalizedSkillArg(args, "skill_id"))
			toolName := stringArg(args, "tool_name")
			if !ok || isSkillMetaToolName(toolName) {
				continue
			}
			if _, ok := active[skillID]; ok && resolvedSkillProvidesTool(resolved, skillID, toolName) {
				return true
			}
		case skills.MetaToolReadSkillReference,
			skills.MetaToolRequestUserInput,
			skills.MetaToolTurnState,
			skills.MetaToolUpdatePlan,
			skills.MetaToolIntermediateAnswer,
			skills.MetaToolFinalAnswer:
			continue
		default:
			if _, ok := uniqueLoadedSkillForToolName(resolved, active, name); ok {
				return true
			}
		}
	}
	return false
}

func terminalMetaCallsOnly(calls []adapter.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		switch strings.ToLower(strings.TrimSpace(call.Function.Name)) {
		case skills.MetaToolTurnState, skills.MetaToolUpdatePlan:
			continue
		default:
			return false
		}
	}
	return true
}

func resolvedSkillProvidesTool(resolved *skills.ResolvedSkills, skillID string, toolName string) bool {
	doc, ok := resolved.Get(skillID)
	if !ok || doc == nil {
		return false
	}
	for _, tool := range doc.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

func canonicalResolvedSkillID(resolved *skills.ResolvedSkills, skillID string) (string, bool) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" || resolved == nil {
		return "", false
	}
	for _, resolvedSkillID := range resolved.SkillIDs() {
		if strings.EqualFold(strings.TrimSpace(resolvedSkillID), skillID) {
			return strings.TrimSpace(resolvedSkillID), true
		}
	}
	return "", false
}

func evidenceStringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return dedupeStrings(typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(evidenceStringFromAny(item)); text != "" {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	case []map[string]interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(firstNonEmptyString(item["name"], item["agent_name"], item["title"])); text != "" {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	default:
		if text := strings.TrimSpace(evidenceStringFromAny(value)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func repeatedFailedToolCallKeyForCall(call adapter.ToolCall) (string, string, map[string]interface{}, string) {
	return skillToolCallIdentityForCall(nil, nil, call)
}

func skillToolCallIdentityForCall(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, call adapter.ToolCall) (string, string, map[string]interface{}, string) {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		return "", "", nil, ""
	}
	if !strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolCallSkillTool) {
		toolName := strings.TrimSpace(call.Function.Name)
		if toolName == "" || isSkillMetaToolName(toolName) {
			return "", "", nil, ""
		}
		skillID, ok := uniqueLoadedSkillForToolName(resolved, loadedSkills, toolName)
		if !ok {
			return "", "", nil, ""
		}
		toolArgs := copyStringAnyMap(args)
		return skillID, toolName, toolArgs, failedToolCallKey(skillID, toolName, toolArgs)
	}
	skillID := normalizedSkillArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	return skillID, toolName, toolArgs, failedToolCallKey(skillID, toolName, toolArgs)
}

func userInputPlanRevisionPending(req RunRequest) bool {
	metadata := currentMetadataForRun(req)
	continuation := evidenceMapFromAny(metadata["user_input_continuation"])
	return strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(continuation["status"])), userInputContinuationAnswered) &&
		strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(continuation["next_action"])), userInputContinuationReplan)
}

func planRevisionRequiredForTool(skillID string, toolName string) bool {
	if strings.TrimSpace(skillID) == "" || strings.TrimSpace(toolName) == "" {
		return false
	}
	return !isSkillMetaToolName(toolName)
}

func pendingUserInputPlanRevisionStep(callID string, skillID string, toolName string, args map[string]interface{}) skillStepResult {
	err := fmt.Errorf("%w: update the current plan before calling a business tool after user clarification", ErrInvalidInput)
	trace := plannerFeedbackTrace(skillID, toolName, err)
	trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	trace.Arguments["next_step"] = skills.MetaToolUpdatePlan
	nextAction := "Revise the pending plan phases from the user's clarification with update_plan, then choose the next business tool from the revised plan. Do not repeat this business call before the plan update succeeds."
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, skillID, toolName)), false, false)
}

func currentMetadataForRun(req RunRequest) map[string]interface{} {
	if req.CurrentMetadata == nil {
		return nil
	}
	return copyStringAnyMap(req.CurrentMetadata())
}

func isAgentManagementMutationTool(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "update_agent_identity", "update_agent_config",
		"replace_agent_skill_bindings", "replace_agent_knowledge_bindings",
		"replace_agent_database_bindings", "replace_agent_workflow_bindings",
		"replace_agent_memory_slots":
		return true
	default:
		return false
	}
}

func failedToolCallKey(skillID string, toolName string, args map[string]interface{}) string {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if skillID == "" || toolName == "" {
		return ""
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = []byte(fmt.Sprint(args))
	}
	return skillID + "/" + toolName + ":" + string(encoded)
}

func repeatedFailedToolCallRecoverableStep(callID string, skillID string, toolName string, args map[string]interface{}, reason string) skillStepResult {
	message := "same tool call with the same arguments already failed in this turn"
	if reason = strings.TrimSpace(reason); reason != "" {
		message += ": " + reason
	}
	err := fmt.Errorf("%w: %s", ErrInvalidInput, message)
	trace := plannerFeedbackTrace(skillID, toolName, err)
	if len(args) > 0 {
		trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
		trace.Arguments["next_step"] = "continue_planning"
	}
	nextAction := strings.Join([]string{
		"Do not repeat the same tool with identical arguments.",
		"Change the arguments based on the previous error only if a safe retry is available.",
		"Otherwise answer truthfully from the failed tool result.",
	}, " ")
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, skillID, toolName)), false, false)
}

func repeatedSuccessfulReadOnlyToolCallFeedbackStep(callID string, skillID string, toolName string, args map[string]interface{}, successfulToolCallsByKey map[string]SkillToolCallRef, successfulToolCalls []SkillToolCallRef) skillStepResult {
	if !skillToolCallLooksReadOnly(skillID, toolName) {
		return skillStepResult{}
	}
	key := failedToolCallKey(skillID, toolName, args)
	if key == "" {
		return skillStepResult{}
	}
	previous, ok := successfulToolCallsByKey[key]
	if !ok {
		return skillStepResult{}
	}
	if repeatedReadOnlyToolShouldRunAfterMutation(skillID, toolName, args, successfulToolCalls) {
		return skillStepResult{}
	}
	trace := plannerFeedbackTrace(skillID, toolName, nil)
	trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	trace.Arguments["next_step"] = "answer_from_previous_result"
	trace.Arguments["reason"] = "same_read_only_tool_already_succeeded"
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, repeatedSuccessfulReadOnlyToolCallPayload(skillID, toolName, previous)), false, false)
}

func repeatedReadOnlyToolShouldRunAfterMutation(skillID string, toolName string, args map[string]interface{}, successfulToolCalls []SkillToolCallRef) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "get_agent", "get_agent_config":
	default:
		return false
	}
	key := failedToolCallKey(skillID, toolName, args)
	if key == "" {
		return false
	}
	for i := len(successfulToolCalls) - 1; i >= 0; i-- {
		call := successfulToolCalls[i]
		if failedToolCallKey(call.SkillID, call.ToolName, call.Arguments) == key {
			return false
		}
		if isAgentManagementMutationTool(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func skillToolCallLooksReadOnly(skillID string, toolName string) bool {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if skillID == "" || toolName == "" {
		return false
	}
	for _, prefix := range []string{"get_", "list_", "read_", "search_"} {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}
	return false
}

func repeatedSuccessfulReadOnlyToolCallPayload(skillID string, toolName string, previous SkillToolCallRef) map[string]interface{} {
	return map[string]interface{}{
		"status":                  "completed",
		"advisory":                "same_read_only_tool_already_succeeded",
		"skill_id":                strings.TrimSpace(skillID),
		"tool_name":               strings.TrimSpace(toolName),
		"message":                 "This read-only tool call with identical arguments already succeeded earlier in this turn.",
		"previous_result_summary": summarizeRepeatedSuccessfulReadOnlyResult(previous.Result),
		"next_action":             "Do not call the same read-only tool with identical arguments again. Answer from the previous tool result already present in the message history; if that result is empty, say there are no matching candidates.",
	}
}

func summarizeRepeatedSuccessfulReadOnlyResult(result map[string]interface{}) map[string]interface{} {
	if len(result) == 0 {
		return nil
	}
	summary := map[string]interface{}{}
	for _, key := range []string{"status", "count", "total", "target_count", "success_count", "failed_count", "agent_id", "agent_name"} {
		if value, ok := result[key]; ok {
			summary[key] = value
		}
	}
	for _, key := range []string{"items", "agents", "skills", "knowledge_bases", "databases", "database_tables", "workflows", "models"} {
		if count := repeatedSuccessfulReadOnlyResultCollectionLength(result[key]); count >= 0 {
			summary[key+"_count"] = count
		}
	}
	if samples := repeatedSuccessfulReadOnlyCandidateSamples(result, 3); len(samples) > 0 {
		summary["candidate_samples"] = samples
	}
	if len(summary) == 0 {
		summary["available"] = true
	}
	return summary
}

func repeatedSuccessfulReadOnlyCandidateSamples(result map[string]interface{}, limit int) []map[string]interface{} {
	if len(result) == 0 || limit <= 0 {
		return nil
	}
	for _, key := range []string{
		"binding_candidates",
		"skills",
		"knowledge_bases",
		"databases",
		"database_tables",
		"tables",
		"workflows",
		"models",
		"items",
	} {
		records := evidenceMapsFromAny(result[key])
		if len(records) == 0 {
			continue
		}
		out := make([]map[string]interface{}, 0, min(len(records), limit))
		for _, record := range records {
			item := repeatedSuccessfulReadOnlyCandidateSample(record)
			if len(item) == 0 {
				continue
			}
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func repeatedSuccessfulReadOnlyCandidateSample(record map[string]interface{}) map[string]interface{} {
	if len(record) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	if id := firstNonEmptyString(record["id"], record["skill_id"], record["dataset_id"], record["knowledge_base_id"], record["data_source_id"], record["table_id"], record["workflow_id"], record["binding_id"]); id != "" {
		item["id"] = id
	}
	if name := firstNonEmptyString(record["name"], record["title"], record["label"], record["display_name"], record["dataset_name"], record["database_name"], record["table_name"], record["workflow_name"], record["model"]); name != "" {
		item["name"] = name
	}
	for _, key := range []string{"selected", "writable", "provider", "model"} {
		if value, ok := record[key]; ok && value != nil && value != "" {
			item[key] = value
		}
	}
	if binding := repeatedSuccessfulReadOnlyCandidateBinding(record["binding"]); len(binding) > 0 {
		item["binding"] = binding
	}
	return item
}

func repeatedSuccessfulReadOnlyCandidateBinding(value interface{}) map[string]interface{} {
	binding := evidenceMapFromAny(value)
	if len(binding) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	for _, key := range []string{"data_source_id", "table_ids", "writable_table_ids", "agent_id", "workflow_id", "binding_id", "version_strategy", "version_uuid", "timeout_seconds"} {
		if value, ok := binding[key]; ok && value != nil && value != "" {
			item[key] = value
		}
	}
	return item
}

func repeatedSuccessfulReadOnlyResultCollectionLength(value interface{}) int {
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []map[string]interface{}:
		return len(typed)
	case []string:
		return len(typed)
	default:
		return -1
	}
}

func recoverableFailureFinalAnswer(trace skills.SkillTrace, err error) string {
	reason := strings.TrimSpace(trace.Error)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	step := strings.TrimSpace(trace.SkillID)
	if toolName := strings.TrimSpace(trace.ToolName); toolName != "" {
		if step != "" {
			step += "/"
		}
		step += toolName
	}
	return runtimeFailureAnswer(reason, step)
}

func planningRoundsExhaustedFinalAnswer(err error) string {
	reason := "\u6267\u884c\u89c4\u5212\u8f6e\u6b21\u5df2\u8fbe\u5230\u4e0a\u9650\uff0c\u65e0\u6cd5\u786e\u8ba4\u672c\u8f6e\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002"
	if err != nil {
		reason = reason + " " + err.Error()
	}
	return runtimeFailureAnswer(reason, "")
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
		result.Message = "The previous candidate final answer was blocked because the claimed outcome lacks successful skill/tool evidence in this turn. Continue from the latest evidence, call the next useful tool if it is still needed, and only then claim completion."
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
		result.Message = "The requested user clarification was blocked because runtime context already contains the information needed to continue. Continue from the latest evidence and use the next useful tool before asking the user."
	}
	return result, true
}

func runToolCallGuard(guard ToolCallGuard, req ToolCallGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The requested skill tool call was blocked because it would run the task in the wrong order. Continue from the latest evidence and use the next useful tool first."
	}
	return result, true
}

func runToolArgumentResolver(resolver ToolArgumentResolver, req ToolCallGuardRequest) (map[string]interface{}, bool) {
	if resolver == nil {
		return nil, false
	}
	resolved, changed := resolver(req)
	if !changed {
		return nil, false
	}
	return copyStringAnyMap(resolved), true
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

func toolCallGuardrailTrace(result FinalAnswerGuardResult, blockedSkillID string, blockedToolName string, blockedArguments map[string]interface{}) skills.SkillTrace {
	trace := finalAnswerGuardrailTrace(result)
	trace.Arguments = map[string]interface{}{
		"blocked_tool": strings.TrimSpace(blockedSkillID) + "/" + strings.TrimSpace(blockedToolName),
		"next_step":    "continue_planning",
	}
	if len(blockedArguments) > 0 {
		trace.Arguments["blocked_arguments"] = summarizeSkillToolArguments(blockedSkillID, blockedToolName, blockedArguments)
	}
	return trace
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
	feedback := strings.TrimSpace(result.SystemMessage)
	if feedback == "" {
		feedback = strings.TrimSpace(result.Message)
	}
	lines := []string{
		"Runtime guardrail feedback:",
		feedback,
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

func (r *Runner) runSkillPlanning(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool) (planningResult, error) {
	if shouldStreamSkillPlanning(prepared) {
		result, ok, err := r.runSkillPlanningStream(ctx, prepared, planningReq, round, nil, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress)
		if err != nil {
			return result, err
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
	finishReason := planningResponseFinishReason(planningResp)
	terminationErr := nonStreamingPlanningTerminationError(finishReason)
	r.recordModelInvocation(ModelInvocationTrace{
		Phase:              "skill_planning",
		Round:              round,
		Streaming:          false,
		StartedAt:          startedAt,
		DurationMS:         time.Since(startedAt).Milliseconds(),
		Request:            planningReq,
		Response:           &message,
		Usage:              usage,
		FinishReason:       finishReason,
		StreamDoneReceived: true,
		TerminatedBy:       "response",
		Error:              errorString(terminationErr),
	})
	if terminationErr != nil {
		return planningResult{}, terminationErr
	}
	return planningResult{message: message, usage: usage}, nil
}

func planningResponseFinishReason(response *adapter.ChatResponse) string {
	if response == nil || len(response.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(response.Choices[0].FinishReason)
}

func nonStreamingPlanningTerminationError(finishReason string) error {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "length", "max_tokens", "content_filter":
		return fmt.Errorf("skill planning response ended before a complete turn: finish_reason=%s", strings.TrimSpace(finishReason))
	default:
		return nil
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

func (r *Runner) emitAgentProgress(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) bool {
	content := localizedAgentProgressText(text)
	if content == "" {
		return false
	}
	r.emitEvent(EventAgentProgress, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         content,
		"created_at":      time.Now().Unix(),
	})
	return true
}

func localizedAgentProgressText(text string) string {
	return visibleAgentProgressText(text)
}

func visibleAgentProgressText(text string) string {
	return strings.TrimSpace(text)
}

func agenticSkillLoopSystemMessage(preferExplicitFinalAnswer bool) adapter.Message {
	instructions := []string{
		"When using skills or tools, you may provide concise user-facing progress when it helps the user understand a multi-step operation.",
		"Each progress update must describe only the newly reached judgment and the next useful action. It may contain multiple sentences or a short list when that is the clearest form. Do not restate progress already visible earlier in this turn.",
		"Do not acknowledge or restate the user's request or latest correction in progress text. Begin directly with the new evidence or next concrete action. If the tool call itself is the only new information, omit ordinary assistant content and call the tool directly.",
		"Progress emitted before a tool call must use planning language. Say an action is completed or a page has been reached only when the latest tool result or current_page_context proves it.",
		"Treat turn_start_context as immutable historical context. Read the current route and current visible assets only from current_page_context and the latest evidence.",
		"All user-facing progress, reasoning, request_user_input text, submit_intermediate_answer text, and final answers must use the same language as the user's latest request. If the user writes in Chinese, progress text must be Chinese.",
		"Do not narrate every tool call, internal plan step, tool name, tool arguments, IDs, protocol details, or bookkeeping status.",
		"If you share progress or reasoning, frame it around the user's goal, current page evidence, and the next useful action; do not expose a rigid hidden checklist.",
		"Finish each progress update before calling tools; do not leave a sentence or list item half-written.",
		"Do not start every task by listing resources or navigating. If current page context, recent tool results, or visible resolved targets are enough, act from that evidence directly.",
		"Do not announce that you need to navigate, open, enter, or switch pages unless a visible console navigation tool is available and you are about to call it. If no navigation tool is available, say you will continue from current page evidence.",
		"When an additional system message contains preferred_route_action or suggested_next_tool, treat it as an advisory next phase, not as a reason to ignore fresh evidence. Load and call it when the current page context and prior tool/client-action evidence show it is still needed; do not repeat the same navigation or business tool after matching evidence already satisfies the step.",
		"Within one user request, do not reload a skill just because approval, navigation, refresh, or continuation resumed the loop. If the skill was already loaded and no newer instructions are needed, continue from the latest tool results, client-action evidence, and turn_state.",
		"After each skill/tool result, continue with the next necessary action or final answer. Summarize only user-relevant outcomes, not internal bookkeeping.",
		"For a multi-phase task, keep the supplied plan current with update_plan. Preserve stable phase IDs and update the plan in the same response as the next business tool whenever possible. Include exact evidence_refs when they are readily available, but treat them as audit links and do not delay execution or finalization solely to repair a ref.",
		"If a tool call fails, explain the likely user-relevant cause, fix the arguments, and retry when possible.",
		"If a tool call fails, do not repeat the same tool with the same arguments. Re-plan from the error before retrying.",
		"For deterministic batch work, prefer one suitable business tool call that handles the batch coherently over many small repeated tool calls.",
		"Read-only tools may be grouped when useful, but call at most one side-effecting or governed mutation tool in a single assistant turn. Wait for its tool result or governance outcome, then continue with the next mutation in the following loop round.",
		"Do not claim that you saved, remembered, updated, deleted, sent, created, changed, or completed any external action unless the corresponding skill/tool call succeeded in this turn.",
		"Do not claim that a governance approval card has been submitted or is waiting unless a governed skill/tool call actually returned a pending governance event.",
		"If a save, update, delete, create, bind, unbind, publish, or navigation tool succeeded in this turn, describe the outcome as executed and verified from the tool/page evidence; do not say it was unnecessary or skipped just because the refreshed page already shows the requested state.",
		"Progress text sent together with tool calls is transient status text. Keep it short and do not place substantial user deliverables there.",
		"Long tasks may cross approvals, page navigation, page refresh, user confirmation, or continuation boundaries. Those boundaries can make implicit working memory unreliable even within the same user request.",
		"Before crossing a boundary or making later steps depend on a tool/page result, decide whether any exact value, summary, theme, selected target, model choice, prompt requirement, or verification fact must be reused. If yes, call submit_turn_state; use kind=working_fact/decision/verification with visibility=model_only for internal state, or kind=user_deliverable with visibility=user_visible when the reusable summary should also be shown to the user.",
		"Use submit_turn_state for internal working facts, decisions, assumptions, and verification state. Do not expose protocol names or JSON to the user; the recorded state is for continuing the same turn reliably.",
		"Do not record every detail. Record only facts that affect later tool arguments, naming, configuration, verification, or the final answer. For long documents, record a concise summary/theme and re-read if exact full text is needed.",
		"If you later need a value but did not record it and cannot see it in current tool/page evidence, re-read or re-observe it instead of guessing or using placeholders such as file content, read content, or 文件内容.",
		"submit_intermediate_answer is for substantial user-facing deliverables only; do not use it for progress, plans, tool status, internal reasoning, or protocol narration.",
		"Prefer submit_turn_state with kind=user_deliverable for new structured workflows; submit_intermediate_answer is a compatibility shortcut for a user-visible deliverable.",
		"If the current turn newly creates or substantially rewrites a user-facing deliverable before later tool/skill calls, call submit_intermediate_answer for that new deliverable before continuing.",
		"Examples of new deliverables that should use submit_intermediate_answer when followed by more tool/skill calls: novel outlines, long-form drafts, plans, tables, code sketches, analysis sections, or generated content the user asked for.",
		"Do not call submit_intermediate_answer merely to repeat content that was already visible in an earlier assistant answer. For requests like exporting, saving, converting, or generating a file from existing content, pass the existing content directly to the file/tool call.",
		"Do not skip submit_intermediate_answer by postponing or summarizing a new deliverable if the user explicitly asked for it as an intermediate phase.",
		"When required information is missing or ambiguity blocks reliable progress, call request_user_input with a brief user-visible message plus a questions array containing one to five concise questions, then stop. The message should explain what you checked, why input is needed, and what you will do next. Prefer one to three questions. Do not call any other tools in the same turn after request_user_input.",
		"Do not guess a revised business plan while the blocking clarification is still unanswered. After the clarification arrives, update the pending plan phases from that answer before the next business tool; update_plan and that next tool may be called in the same response, in that order.",
		"When calling request_user_input, put the user-visible explanation only in the request_user_input message field. Do not also repeat that explanation in assistant text outside the tool call.",
		"Each request_user_input question should ask one decision point. Include options only when each option is a concrete, directly usable answer. Do not include vague options such as free choice, freestyle, not sure, depends, any, or other; omit options for open-ended questions because the user can type freely.",
		"Do not use request_user_input for information already confirmed in the conversation.",
		"Do not label the user-facing reply with protocol wording such as Final Answer, final result, or their Chinese equivalents unless the user explicitly asks for that wording.",
		"When reusing existing conversation content, refer to it explicitly, for example as the previous outline or the current branch's draft; do not duplicate the full text unless the user asks to see it again.",
	}
	if preferExplicitFinalAnswer {
		instructions = append(instructions,
			"In this skill loop, ordinary assistant content is always transient process progress, never the terminal answer. The runtime may show the complete progress update but will not store it as final message content.",
			"When no more business tool, user input, state, or plan update calls are needed, call submit_final_answer with the complete, natural, self-contained user-facing reply. Do not write the final reply as ordinary assistant content.",
			"submit_final_answer is terminal. Do not combine it with business tools, request_user_input, or further actions. If a model-maintained plan exists, update it before finishing or include an optional final snapshot when useful; plan metadata never replaces the answer. Add evidence refs when readily available. If you did not call submit_intermediate_answer for a new requested deliverable, the answer field MUST include the deliverable in full, not a compressed summary.",
		)
	} else {
		instructions = append(instructions, "When no tool or skill call is needed, provide the complete user-facing reply as ordinary assistant content and end the turn.")
	}
	return adapter.Message{Role: "system", Content: strings.Join(instructions, "\n")}
}

func AgenticSkillLoopSystemMessage() adapter.Message {
	return agenticSkillLoopSystemMessage(true)
}
