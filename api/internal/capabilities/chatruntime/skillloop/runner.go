package skillloop

import (
	"context"
	"crypto/sha256"
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
	runtimeStateAllowPlanUpdateKey                = "_skill_loop_allow_plan_update"
	runtimeStateAllowIntermediateAnswerKey        = "_skill_loop_allow_intermediate_answer"
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

type restoredSkillInstructionState struct {
	activeLoaded   map[string]struct{}
	reloadRequired []string
	restored       []string
	message        *adapter.Message
}

func metaToolsForRun(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, preferExplicitFinalAnswer bool, requireFinalPlanSnapshot bool) []adapter.Tool {
	tools := skills.MetaToolsForSkillStateWithOptions(resolved, loadedSkills, skills.MetaToolOptions{
		RequireFinalPlanSnapshot: requireFinalPlanSnapshot,
	})
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

func controlToolsForRound(input []adapter.Tool, allowPlanUpdate bool, allowIntermediateAnswer bool) []adapter.Tool {
	filtered := make([]adapter.Tool, 0, len(input))
	for _, tool := range input {
		name := strings.TrimSpace(tool.Function.Name)
		if !allowPlanUpdate && strings.EqualFold(name, skills.MetaToolUpdatePlan) {
			continue
		}
		if !allowIntermediateAnswer && strings.EqualFold(name, skills.MetaToolIntermediateAnswer) {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

func (r *Runner) Run(ctx context.Context, req RunRequest) (string, *adapter.Usage, error) {
	prepared := req.Prepared
	resolved := req.Resolved
	if r == nil {
		return "", nil, fmt.Errorf("%w: runner is not configured", ErrInvalidInput)
	}
	if r.LLMClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
	if prepared == nil || prepared.LLMRequest == nil {
		return "", nil, fmt.Errorf("%w: prepared chat is invalid", ErrInvalidInput)
	}
	if resolved == nil {
		resolved = &skills.ResolvedSkills{}
	}
	if len(resolved.Skills) == 0 && !req.ProtocolToolsOnly && !req.TerminalOnly {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}
	if len(resolved.Skills) > 0 && r.SkillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	preferExplicitFinalAnswer := req.PreferExplicitFinalAnswer && !req.TerminalOnly
	historicalLoadedSkills, restoreValidationTraces := validatedHistoricalLoadedSkillsForRun(ctx, req, resolved)
	restoredSkillState := restoredLoadedSkillInstructionStateForRun(resolved, historicalLoadedSkills, req.PreferredRestoredSkillID)
	if req.TerminalOnly {
		restoredSkillState = restoredSkillInstructionState{activeLoaded: map[string]struct{}{}}
	} else {
		restoreValidationTraces = append(restoreValidationTraces, restoredSkillAttemptTraces(currentMetadataForRun(req), resolved, restoredSkillState)...)
	}
	loadedSkills := restoredSkillState.activeLoaded

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	if req.TerminalOnly {
		messages = terminalOnlyProjectedMessages(prepared, currentMetadataForRun(req))
		messages = append([]adapter.Message{terminalOnlySystemMessage()}, messages...)
	} else {
		messages = append(messages, metadataMessage)
		if restoredSkillState.message != nil {
			messages = append(messages, *restoredSkillState.message)
		}
		messages = append(messages, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
		if req.ProtocolToolsOnly {
			messages = append(messages, protocolToolLoopSystemMessage(preferExplicitFinalAnswer))
		} else if req.LegacyToolChat {
			messages = append(messages, legacyToolChatSystemMessage())
		} else {
			messages = append(messages, agenticSkillLoopSystemMessage(preferExplicitFinalAnswer))
		}
	}
	traces := []skills.SkillTrace{}
	for _, trace := range restoreValidationTraces {
		traces = append(traces, trace)
		r.recordTrace(traces, trace)
	}
	metadataTrace := metadataExposedTrace(resolved.SkillIDs(), metadataStats)
	traces = append(traces, metadataTrace)
	r.recordTrace(traces, metadataTrace)
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
	emptyFinalAnswerRetryCount := 0
	skillToolCallCounts := map[string]int{}
	successfulToolCalls := []SkillToolCallRef{}
	failedToolCallReasons := map[string]string{}
	skillUsed := false
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	terminalStateGuardConfigured := req.RuntimeStateSnapshot != nil
	planRevisionRequired := userInputPlanRevisionPending(req)
	var answerBuilder strings.Builder
	var usage *adapter.Usage
	r.diagnostics = modelInvocationDiagnostics{
		restoredSkillIDs: append([]string(nil), restoredSkillState.restored...),
		continuationType: strings.TrimSpace(req.ContinuationType),
		terminalOnly:     req.TerminalOnly,
	}
	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		roundRuntimeState := map[string]interface{}{}
		terminalSubmissionAllowed := true
		if terminalStateGuardConfigured {
			roundRuntimeState = runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
			terminalSubmissionAllowed = terminalStateGuardCanStream(roundRuntimeState)
		}
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = metaToolsForRun(resolved, loadedSkills, preferExplicitFinalAnswer, false)
		allowPlanUpdate := planRevisionRequired || operationPlanModelRevisionRequired(req, roundRuntimeState)
		allowIntermediateAnswer := !fileDeliveryRequiresArtifactOnly(req, roundRuntimeState)
		planningReq.Tools = controlToolsForRound(
			planningReq.Tools,
			allowPlanUpdate,
			allowIntermediateAnswer,
		)
		if req.LegacyToolChat {
			planningReq.Tools = legacyToolChatTools(planningReq.Tools, len(restoredSkillState.reloadRequired) > 0)
		}
		if req.TerminalOnly {
			planningReq.Tools = nil
			planningReq.ToolChoice = nil
		} else {
			planningReq.ToolChoice = "auto"
		}
		r.diagnostics.activeSkillIDs = activeSkillIDsForDiagnostics(resolved, loadedSkills)
		r.requestBudget = planningRequestBudgetForRun(req)

		suppressNaturalProgress := req.SuppressInitialNaturalProgress && round == 0
		deferTerminalContent := preferExplicitFinalAnswer || req.TerminalOnly || !terminalSubmissionAllowed
		planningResult := planningResult{}
		var err error
		if req.TerminalOnly {
			planningResult, err = r.runSkillPlanning(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress)
		} else {
			planningResult, err = r.runSkillPlanningWithRetry(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress, req.PlanningOutputTokenLimit)
		}
		usage = mergeUsage(usage, planningResult.usage)
		if err != nil {
			if req.TerminalOnly {
				if fallback, ok := r.emitTerminalOnlyFallback(ctx, req, prepared, traces, roundRuntimeState, "model_error"); ok {
					appendAnswerText(&answerBuilder, fallback)
					return answerBuilder.String(), usage, nil
				}
			}
			var streamedErr *streamedFinalAnswerError
			if errors.As(err, &streamedErr) {
				appendAnswerText(&answerBuilder, strings.TrimSpace(streamedErr.answer))
			}
			return answerBuilder.String(), usage, err
		}
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		text := assistantMessageText(planningMessage)
		if req.TerminalOnly && len(toolCalls) > 0 {
			if fallback, ok := r.emitTerminalOnlyFallback(ctx, req, prepared, traces, roundRuntimeState, "unexpected_tool_call"); ok {
				appendAnswerText(&answerBuilder, fallback)
				return answerBuilder.String(), usage, nil
			}
			return answerBuilder.String(), usage, fmt.Errorf("%w: terminal-only model returned an unexpected tool call", ErrInvalidInput)
		}
		if req.TerminalOnly && strings.TrimSpace(text) == "" {
			if fallback, ok := r.emitTerminalOnlyFallback(ctx, req, prepared, traces, roundRuntimeState, "empty_response"); ok {
				appendAnswerText(&answerBuilder, fallback)
				return answerBuilder.String(), usage, nil
			}
			return answerBuilder.String(), usage, fmt.Errorf("%w: terminal-only model returned no final answer", ErrInvalidInput)
		}
		if text != "" && len(toolCalls) > 0 && !suppressNaturalProgress && !planningResult.naturalProgressStreamed && shouldEmitNaturalProgressForToolCalls(resolved, loadedSkills, toolCalls) {
			r.emitAgentProgress(ctx, prepared, text, nil)
		}
		if len(toolCalls) == 0 && terminalStateGuardConfigured {
			if strings.TrimSpace(text) == "" {
				if req.TerminalOnly {
					return answerBuilder.String(), usage, fmt.Errorf("%w: terminal-only model returned no final answer", ErrInvalidInput)
				}
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
			result.trace.Arguments["round"] = round + 1
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
			callEvidence[runtimeStateAllowPlanUpdateKey] = planRevisionRequired || operationPlanModelRevisionRequired(req, callEvidence)
			callEvidence[runtimeStateAllowIntermediateAnswerKey] = !fileDeliveryRequiresArtifactOnly(req, callEvidence)
			result := skillStepResult{}
			if userInputPlanRevisionRequiredForTool(req, callSkillID, callToolName) {
				result = pendingUserInputPlanRevisionStep(call.ID, callSkillID, callToolName, callToolArgs)
			}
			if result.trace.Kind == "" && req.AuthorizeSkillStep != nil && strings.TrimSpace(callSkillID) != "" {
				allowed, policyErr := req.AuthorizeSkillStep(ctx, callSkillID)
				if policyErr != nil || !allowed {
					result = unavailableSkillPolicyStep(call.ID, callSkillID, callToolName, callToolArgs, policyErr)
				}
			}
			if result.trace.Kind == "" && failedCallKey != "" {
				if reason := failedToolCallReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
					result = repeatedFailedToolCallRecoverableStep(call.ID, callSkillID, callToolName, callToolArgs, reason)
				}
			}
			if result.trace.Kind == "" {
				result = r.handleProgressiveSkillCall(ctx, prepared, resolved, call, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, callEvidence, round+1, nil)
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
				planRevisionRequired = true
			} else {
				roundHadSuccess = true
			}
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "plan_update") &&
				strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
				planRevisionRequired = false
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
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "skill_load") &&
				strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
				r.diagnostics.loadedSkillIDs = appendUniqueProjectionRefs(r.diagnostics.loadedSkillIDs, result.trace.SkillID)
			}
			messages = append(messages, result.toolMessage)
			if projected, stats := projectMaterializedFileContent(messages, result.toolMessage.ToolCallID, result.toolResult); stats.removedRunes > 0 {
				messages = projected
				r.diagnostics.projectedRefs = appendUniqueProjectionRefs(r.diagnostics.projectedRefs, stats.refs...)
				r.diagnostics.projectedChars += stats.removedRunes
			}
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
		if req.TerminalOnly {
			return answerBuilder.String(), usage, fmt.Errorf("%w: terminal-only model did not submit a final answer", ErrInvalidInput)
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

func (r *Runner) emitTerminalOnlyFallback(
	ctx context.Context,
	req RunRequest,
	prepared *PreparedChat,
	traces []skills.SkillTrace,
	runtimeState map[string]interface{},
	reason string,
) (string, bool) {
	answer, ok := terminalOnlyFallbackAnswer(prepared, currentMetadataForRun(req), runtimeState)
	if !ok || strings.TrimSpace(answer) == "" {
		return "", false
	}
	trace := skills.SkillTrace{
		Kind:    "final_answer",
		Title:   "Final answer",
		Message: answer,
		Status:  "success",
		Arguments: map[string]interface{}{
			"fallback":        true,
			"fallback_reason": strings.TrimSpace(reason),
		},
		Result: map[string]interface{}{
			"source": "runtime_evidence",
		},
	}
	traces = append(traces, trace)
	r.recordTrace(traces, trace)
	r.logSkillTrace(ctx, prepared, trace)
	r.emitAnswerChunk(ctx, prepared, answer, nil)

	decision := terminalStateGuardDecision{
		Path:        terminalStateGuardAccepted,
		Reason:      "completed runtime evidence supplied a deterministic terminal fallback",
		FinalAnswer: answer,
	}
	terminalStateGuardRecord(req, decision)
	if req.OnTerminalCompletion != nil {
		req.OnTerminalCompletion(TerminalCompletionResult{
			Status: "pass",
			Source: "runtime_evidence_fallback",
			Reason: decision.Reason,
		})
	}
	logger.WarnContext(ctx, "aichat terminal model degraded to completed runtime evidence",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"fallback_reason", strings.TrimSpace(reason),
	)
	return answer, true
}

func operationPlanModelRevisionRequired(req RunRequest, runtimeState map[string]interface{}) bool {
	if userInputPlanRevisionPending(req) {
		return true
	}
	plan := evidenceMapFromAny(runtimeState["operation_plan"])
	return strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(plan["plan_sync_status"])), "stale")
}

func fileDeliveryRequiresArtifactOnly(req RunRequest, runtimeState map[string]interface{}) bool {
	if latestUserExplicitlyRequestsInlineFileBody(req) {
		return false
	}
	plan := evidenceMapFromAny(runtimeState["operation_plan"])
	for _, phase := range evidenceMapsFromAny(plan["phases"]) {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		expected := evidenceMapFromAny(phase["expected_action"])
		skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(expected["skill_id"])))
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(expected["tool_name"])))
		if skillID == skills.SkillFileGenerator || toolName == "generate_file" {
			return true
		}
	}
	return false
}

func latestUserExplicitlyRequestsInlineFileBody(req RunRequest) bool {
	text := strings.ToLower(strings.TrimSpace(latestUserRequestText(req)))
	if text == "" {
		return false
	}
	for _, negative := range []string{"不要展示全文", "无需展示全文", "不需要展示全文", "只生成文件", "do not show the full", "don't show the full", "file only"} {
		if strings.Contains(text, negative) {
			return false
		}
	}
	for _, marker := range []string{
		"同时在聊天", "同时在对话", "在聊天中展示", "在对话中展示", "展示全文", "贴出全文", "同时展示", "正文也发",
		"show the full", "include the full", "paste the full", "in the chat", "inline copy", "also display",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func initialLoadedSkillsForRun(req RunRequest, resolved *skills.ResolvedSkills) map[string]struct{} {
	loaded := map[string]struct{}{}
	invalidDigests := invalidLoadedSkillDigests(currentMetadataForRun(req), resolved)
	add := func(skillID string) {
		if canonical, ok := canonicalResolvedSkillID(resolved, skillID); ok {
			if _, invalid := invalidDigests[canonical]; invalid {
				return
			}
			loaded[canonical] = struct{}{}
		}
	}
	if req.LegacyToolChat && resolved != nil {
		for _, skillID := range resolved.SkillIDs() {
			add(skillID)
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

func invalidLoadedSkillDigests(metadata map[string]interface{}, resolved *skills.ResolvedSkills) map[string]struct{} {
	invalid := map[string]struct{}{}
	if resolved == nil {
		return invalid
	}
	for _, record := range evidenceMapsFromAny(metadata["loaded_skill_state"]) {
		skillID, ok := canonicalResolvedSkillID(resolved, evidenceStringFromAny(record["skill_id"]))
		if !ok {
			continue
		}
		digest := strings.TrimSpace(evidenceStringFromAny(record["instruction_digest"]))
		if digest == "" {
			continue
		}
		doc, ok := resolved.Get(skillID)
		if !ok || doc == nil || digest != skillInstructionDigest(doc.Instructions) {
			invalid[skillID] = struct{}{}
		}
	}
	return invalid
}

func validatedHistoricalLoadedSkillsForRun(ctx context.Context, req RunRequest, resolved *skills.ResolvedSkills) (map[string]struct{}, []skills.SkillTrace) {
	loaded := initialLoadedSkillsForRun(req, resolved)
	metadata := currentMetadataForRun(req)
	traces := []skills.SkillTrace{}
	for _, record := range evidenceMapsFromAny(metadata["loaded_skill_state"]) {
		recordedSkillID := strings.TrimSpace(evidenceStringFromAny(record["skill_id"]))
		if recordedSkillID == "" {
			continue
		}
		canonical, ok := canonicalResolvedSkillID(resolved, recordedSkillID)
		if !ok {
			traces = append(traces, restoredSkillValidationTrace(recordedSkillID, "not_exposed_current_surface", record, "allowed", "not_applicable"))
			continue
		}
		doc, ok := resolved.Get(canonical)
		if !ok || doc == nil {
			delete(loaded, canonical)
			traces = append(traces, restoredSkillValidationTrace(canonical, "not_exposed_current_surface", record, "allowed", "not_applicable"))
			continue
		}
		currentVersion := skillInstructionDigest(doc.Instructions)
		recordedVersion := strings.TrimSpace(firstNonEmptyString(record["effective_version"], record["instruction_digest"]))
		if recordedVersion != "" && recordedVersion != currentVersion {
			delete(loaded, canonical)
			trace := restoredSkillValidationTrace(canonical, "version_changed", record, "allowed", "reload_required")
			trace.Arguments["effective_version"] = currentVersion
			traces = append(traces, trace)
			continue
		}
		if req.AuthorizeSkillStep != nil {
			allowed, err := req.AuthorizeSkillStep(ctx, canonical)
			if err != nil || !allowed {
				delete(loaded, canonical)
				accessStatus := "denied"
				if err != nil {
					accessStatus = "verification_failed"
				}
				trace := restoredSkillValidationTrace(canonical, "policy_denied", record, "denied", accessStatus)
				if err != nil {
					trace.Error = err.Error()
				}
				traces = append(traces, trace)
			}
		}
	}
	return loaded, traces
}

func restoredSkillValidationTrace(skillID string, outcome string, record map[string]interface{}, policyState string, accessStatus string) skills.SkillTrace {
	recordedVersion := strings.TrimSpace(firstNonEmptyString(record["effective_version"], record["instruction_digest"]))
	status := "blocked"
	switch strings.TrimSpace(outcome) {
	case "not_exposed_current_surface":
		status = "skipped"
	case "version_changed":
		status = "reload_required"
	}
	return skills.SkillTrace{
		Kind:     "skill_load_attempt",
		SkillID:  strings.TrimSpace(skillID),
		ToolName: skills.MetaToolLoadSkill,
		Status:   status,
		Arguments: map[string]interface{}{
			"runtime_id":         newSkillLoadAttemptRuntimeID(skillID),
			"created_at_ms":      time.Now().UnixMilli(),
			"requested_skill_id": strings.TrimSpace(skillID),
			"outcome":            strings.TrimSpace(outcome),
			"recorded_version":   recordedVersion,
			"policy_state":       strings.TrimSpace(policyState),
			"access_status":      strings.TrimSpace(accessStatus),
			"load_sequence":      firstNonNilValue(record["load_sequence"], record["loaded_sequence"]),
		},
	}
}

func restoredSkillAttemptTraces(metadata map[string]interface{}, resolved *skills.ResolvedSkills, state restoredSkillInstructionState) []skills.SkillTrace {
	if resolved == nil || (len(state.restored) == 0 && len(state.reloadRequired) == 0) {
		return nil
	}
	records := map[string]map[string]interface{}{}
	for _, record := range evidenceMapsFromAny(metadata["loaded_skill_state"]) {
		if canonical, ok := canonicalResolvedSkillID(resolved, evidenceStringFromAny(record["skill_id"])); ok {
			records[canonical] = record
		}
	}
	build := func(skillID string, status string, outcome string) skills.SkillTrace {
		record := records[skillID]
		doc, _ := resolved.Get(skillID)
		version := ""
		if doc != nil {
			version = skillInstructionDigest(doc.Instructions)
		}
		return skills.SkillTrace{
			Kind:     "skill_load_attempt",
			SkillID:  skillID,
			ToolName: skills.MetaToolLoadSkill,
			Status:   status,
			Arguments: map[string]interface{}{
				"runtime_id":         newSkillLoadAttemptRuntimeID(skillID),
				"created_at_ms":      time.Now().UnixMilli(),
				"requested_skill_id": skillID,
				"outcome":            outcome,
				"effective_version":  version,
				"policy_state":       "allowed",
				"access_status":      "authorized",
				"load_sequence":      firstNonNilValue(record["load_sequence"], record["loaded_sequence"]),
			},
		}
	}
	traces := make([]skills.SkillTrace, 0, len(state.restored)+len(state.reloadRequired))
	for _, skillID := range state.restored {
		traces = append(traces, build(skillID, "auto_restored", "auto_restored"))
	}
	for _, skillID := range state.reloadRequired {
		traces = append(traces, build(skillID, "reload_required", "restore_budget_exceeded"))
	}
	return traces
}

func firstNonNilValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func legacyToolChatTools(input []adapter.Tool, allowSkillReload bool) []adapter.Tool {
	allowed := map[string]struct{}{
		skills.MetaToolReadSkillReference: {},
		skills.MetaToolCallSkillTool:      {},
		skills.MetaToolRequestUserInput:   {},
	}
	if allowSkillReload {
		allowed[skills.MetaToolLoadSkill] = struct{}{}
	}
	out := make([]adapter.Tool, 0, len(input))
	for _, tool := range input {
		if _, ok := allowed[strings.TrimSpace(tool.Function.Name)]; ok {
			out = append(out, tool)
		}
	}
	return out
}

func restoredLoadedSkillInstructionState(resolved *skills.ResolvedSkills, historicalLoadedSkills map[string]struct{}) restoredSkillInstructionState {
	return restoredLoadedSkillInstructionStateForRun(resolved, historicalLoadedSkills, "")
}

func restoredLoadedSkillInstructionStateForRun(resolved *skills.ResolvedSkills, historicalLoadedSkills map[string]struct{}, preferredSkillID string) restoredSkillInstructionState {
	state := restoredSkillInstructionState{activeLoaded: map[string]struct{}{}}
	if resolved == nil || len(historicalLoadedSkills) == 0 {
		return state
	}
	sections := []string{
		"The following skill instructions were loaded earlier in this same user turn and remain active after navigation, approval, refresh, or continuation.",
		"Only skills whose complete instructions appear below are active. If a skill is listed as requiring reload, call load_skill before using its tools.",
	}
	remaining := restoredSkillInstructionsTotalBudgetChars
	preferred, _ := canonicalResolvedSkillID(resolved, preferredSkillID)
	ordered := append([]string(nil), resolved.SkillIDs()...)
	if preferred != "" {
		ordered = append([]string{preferred}, ordered...)
	}
	seen := map[string]struct{}{}
	for _, skillID := range ordered {
		canonical, ok := canonicalResolvedSkillID(resolved, skillID)
		if !ok {
			continue
		}
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		if _, ok := historicalLoadedSkills[canonical]; !ok {
			continue
		}
		doc, ok := resolved.Get(canonical)
		if !ok || doc == nil {
			continue
		}
		instructions := strings.TrimSpace(doc.Instructions)
		instructionRunes := len([]rune(instructions))
		preferredRestore := canonical == preferred
		if !preferredRestore && (instructionRunes > restoredSkillInstructionsPerSkillBudgetChars || instructionRunes > remaining) {
			state.reloadRequired = append(state.reloadRequired, canonical)
			continue
		}
		section := []string{"Restored skill: " + canonical}
		if description := strings.TrimSpace(doc.Metadata.Description); description != "" {
			section = append(section, "Description: "+description)
		}
		if whenToUse := strings.TrimSpace(doc.Metadata.WhenToUse); whenToUse != "" {
			section = append(section, "When to use: "+whenToUse)
		}
		if instructions != "" && !preferredRestore {
			remaining -= instructionRunes
		}
		if instructions != "" {
			section = append(section, "Instructions:\n"+instructions)
		}
		state.activeLoaded[canonical] = struct{}{}
		state.restored = append(state.restored, canonical)
		sections = append(sections, strings.Join(section, "\n"))
	}
	if len(state.reloadRequired) > 0 {
		sections = append(sections, "Skills requiring full reload before use: "+strings.Join(state.reloadRequired, ", "))
	}
	if len(sections) > 2 {
		state.message = &adapter.Message{Role: "system", Content: strings.Join(sections, "\n\n")}
	}
	return state
}

func skillInstructionDigest(instructions string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(instructions)))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func activeSkillIDsForDiagnostics(resolved *skills.ResolvedSkills, loaded map[string]struct{}) []string {
	if resolved == nil || len(loaded) == 0 {
		return nil
	}
	out := make([]string, 0, len(loaded))
	for _, skillID := range resolved.SkillIDs() {
		canonical, ok := canonicalResolvedSkillID(resolved, skillID)
		if !ok {
			continue
		}
		if _, ok := loaded[canonical]; ok {
			out = append(out, canonical)
		}
	}
	return out
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

func userInputPlanRevisionRequiredForTool(req RunRequest, skillID string, toolName string) bool {
	if req.LegacyToolChat {
		return false
	}
	return userInputPlanRevisionPending(req) && planRevisionRequiredForTool(skillID, toolName)
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

func unavailableSkillPolicyStep(callID string, skillID string, toolName string, args map[string]interface{}, policyErr error) skillStepResult {
	message := "skill is no longer enabled by the current organization policy"
	if policyErr != nil {
		message = "skill availability could not be verified against the current organization policy"
	}
	err := fmt.Errorf("%w: %s", ErrInvalidInput, message)
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Status = "blocked"
	trace.Arguments = summarizeSkillToolArguments(trace.SkillID, toolName, args)
	trace.Arguments["reason_code"] = "organization_skill_unavailable"
	nextAction := "Do not retry this skill in the current turn. Continue with another enabled skill, or answer truthfully that the requested operation was not executed."
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, trace.SkillID, toolName)), false, false)
}

func currentMetadataForRun(req RunRequest) map[string]interface{} {
	if req.CurrentMetadata == nil {
		return nil
	}
	return copyStringAnyMap(req.CurrentMetadata())
}

func runRequiresFinalPlanSnapshot(_ RunRequest) bool {
	return false
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
	planningReq = cloneChatRequest(planningReq)
	sourceMessages := cloneMessagesForProvider(planningReq.Messages)
	planningReq.Messages = adapter.NormalizeSystemMessages(sourceMessages)
	if err := r.applyFinalPlanningRequestBudget(planningReq, sourceMessages); err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:     "skill_planning",
			Round:     round,
			Request:   planningReq,
			StartedAt: time.Now(),
			Error:     err.Error(),
		})
		return planningResult{}, err
	}
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
	callCtx, cancel := context.WithTimeout(ctx, r.modelIdleTimeout())
	planningResp, err := r.LLMClient.AppChat(callCtx, r.AppContext, planningReq)
	callErr := callCtx.Err()
	cancel()
	if err != nil {
		if errors.Is(callErr, context.DeadlineExceeded) {
			err = ErrModelIdleTimeout
		}
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

func (r *Runner) runSkillPlanningWithRetry(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onChunk func(string) error,
	terminalProtocol bool,
	terminalStreamingAllowed bool,
	suppressNaturalProgress bool,
	outputTokenLimit int,
) (planningResult, error) {
	result, err := r.runSkillPlanning(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress)
	if err == nil {
		return result, nil
	}
	var streamedErr *streamedFinalAnswerError
	if errors.As(err, &streamedErr) && strings.TrimSpace(streamedErr.answer) != "" {
		return result, err
	}
	var terminationErr *PlanningTerminationError
	if !errors.As(err, &terminationErr) || terminationErr == nil || !terminationErr.Recoverable {
		return result, err
	}

	retryReq := cloneChatRequest(planningReq)
	retryReq.Messages = append(append([]adapter.Message{}, planningReq.Messages...), adapter.Message{
		Role:    "system",
		Content: "The previous planning response was truncated by its output limit. Retry once with exactly one complete protocol tool call or one concise final answer. Do not repeat completed operations or add long process narration.",
	})
	retryMaxTokens := planningRetryMaxTokens(planningReq.MaxTokens, outputTokenLimit)
	retryReq.MaxTokens = &retryMaxTokens
	logger.WarnContext(ctx, "chat runtime planning length retry",
		"message_id", prepared.Message.ID.String(),
		"provider", prepared.parts.Provider,
		"model", planningReq.Model,
		"finish_reason", terminationErr.Reason,
		"retry", 1,
		"max_tokens", retryMaxTokens,
	)
	retryResult, retryErr := r.runSkillPlanning(ctx, prepared, retryReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, true)
	retryResult.usage = mergeUsage(result.usage, retryResult.usage)
	if retryErr != nil {
		var secondTermination *PlanningTerminationError
		if errors.As(retryErr, &secondTermination) && secondTermination != nil && secondTermination.Recoverable {
			return retryResult, fmt.Errorf("planning_output_truncated: %w", retryErr)
		}
	}
	return retryResult, retryErr
}

func planningRetryMaxTokens(current *int, outputTokenLimit int) int {
	currentValue := 0
	if current != nil && *current > 0 {
		currentValue = *current
	}
	target := currentValue * 2
	if target < 8192 {
		target = 8192
	}
	if outputTokenLimit > 0 && target > outputTokenLimit {
		target = outputTokenLimit
	}
	if target <= 0 {
		return 8192
	}
	return target
}

func planningResponseFinishReason(response *adapter.ChatResponse) string {
	if response == nil || len(response.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(response.Choices[0].FinishReason)
}

func nonStreamingPlanningTerminationError(finishReason string) error {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "length", "max_tokens":
		return &PlanningTerminationError{Reason: strings.TrimSpace(finishReason), Recoverable: true}
	case "content_filter":
		return &PlanningTerminationError{Reason: strings.TrimSpace(finishReason)}
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
		"The operation_plan tracks independently verifiable user-visible outcomes, not individual tool calls. Tool loads, prerequisite reads, navigation requests, approvals, and other implementation details belong to the runtime action/effect ledger and do not require plan updates.",
		"Do not call update_plan after ordinary successful tool results. Use it only when the requested outcome structure changes, a failure invalidates the current route, or the user changes the goal. Prefer the outcomes form, preserve stable outcome IDs, and do not mark required outcomes completed or skipped without runtime evidence.",
		"plan_phase_id is optional correlation metadata. It never proves completion by itself. Omit it from prerequisite reads, inspections, skill loads, and helper calls; the runtime reconciles successful effects against outcome acceptance facts. expected_action is a legacy advisory hint, not permission to execute; governed mutations are separately frozen to their exact approved call.",
		"For call_skill_tool, set completion_intent=finalize_if_success only when that exact business action is the final remaining user-requested effect and every prerequisite read, artifact creation, save, and navigation has already completed. Otherwise omit it or use continue. This intent never bypasses governance and is ignored unless the frozen action succeeds and the runtime can close the remaining plan deterministically.",
		"Before submitting the final answer, reconcile the complete user request with the execution evidence. An advisory phase that is still marked open does not by itself require update_plan: if evidence proves the outcome, answer from that evidence; if an outcome is genuinely unfinished, continue the work or state truthfully that it was not completed. Never silently omit an open requested outcome.",
		"Verify the remaining outcomes and do not submit while you still intend to perform an open phase or an unverified user-visible action. Do not call update_plan only to make bookkeeping match successful evidence; the plan snapshot remains optional audit metadata.",
		"Treat user-visible actions such as opening or returning to a console page as real requested outcomes when the user asked for them. A backend read or mutation does not prove that the page changed. Perform the navigation and observe matching route/current_page_context evidence, or state truthfully that the page transition was not completed.",
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
		"Do not record every detail. Record only facts that affect later tool arguments, naming, configuration, verification, or the final answer. For long documents, use the generated or managed file reference, digest, and concise summary already recorded by the runtime. Re-read only when exact text is required and no authoritative file reference can be passed directly to the next tool.",
		"If you later need a value but did not record it and cannot see it in current tool/page evidence, re-read or re-observe it instead of guessing or using placeholders such as file content, read content, or 文件内容.",
		"submit_intermediate_answer is for substantial user-facing deliverables only; do not use it for progress, plans, tool status, internal reasoning, or protocol narration.",
		"Prefer submit_turn_state with kind=user_deliverable for new structured workflows; submit_intermediate_answer is a compatibility shortcut for a user-visible deliverable.",
		"If the current turn newly creates or substantially rewrites a user-facing deliverable before later tool/skill calls, call submit_intermediate_answer for that new deliverable before continuing, except when the requested destination is a generated or managed file and the user did not explicitly ask to see the full body in chat.",
		"Examples of new deliverables that should use submit_intermediate_answer when followed by more tool/skill calls: novel outlines, long-form drafts, plans, tables, code sketches, analysis sections, or generated content the user asked for.",
		"Do not call submit_intermediate_answer merely to repeat content that was already visible in an earlier assistant answer. For requests like exporting, saving, converting, or generating a file from existing content, pass the existing content directly to the file/tool call.",
		"For file-first work, generate the file directly and keep chat progress concise. Do not emit the same long body through submit_intermediate_answer and then repeat it in generate_file. Emit the full body in chat only when the user explicitly requests both an inline copy and a file.",
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
			"submit_final_answer is terminal. Do not combine it with business tools, request_user_input, or further actions. Verify the answer from the execution ledger and current evidence; the optional plan snapshot is audit metadata and must not trigger an extra bookkeeping round. If you did not call submit_intermediate_answer for a new requested deliverable, the answer field MUST include the deliverable in full, not a compressed summary.",
		)
	} else {
		instructions = append(instructions, "When no tool or skill call is needed, provide the complete user-facing reply as ordinary assistant content and end the turn.")
	}
	return adapter.Message{Role: "system", Content: strings.Join(instructions, "\n")}
}

func terminalOnlySystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"The approved or resumed operation has already completed the remaining bound plan phase.",
		"Use only the authoritative current-turn tool, approval, page, and operation-plan evidence supplied in context.",
		"No tools are available in this terminal response. Do not call or invent tools, load skills, update the plan, repeat completed work, or request more execution.",
		"Reply directly with one concise, self-contained completion message in the user's language. Mention only outcomes supported by the supplied evidence.",
	}, "\n")}
}

func legacyToolChatSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"Use the already available tools only when they are needed to answer the user's request.",
		"Treat successful tool results as execution evidence. Never claim an external action succeeded without a matching successful result.",
		"If a tool fails, do not repeat it with unchanged arguments; correct the request or explain the limitation.",
		"When no further tool call is needed, answer the user directly and end the turn.",
	}, "\n")}
}

func protocolToolLoopSystemMessage(preferExplicitFinalAnswer bool) adapter.Message {
	instructions := []string{
		"Use only the function tools exposed in the current request. No business skills or business tools are available; do not invent, load, or call any.",
		"All user-facing progress, request_user_input text, submit_intermediate_answer text, and final answers must use the same language as the user's latest request.",
		"When required information is missing or ambiguity blocks a reliable answer, call request_user_input with a brief user-visible message and one to five concise questions, then stop.",
		"Use update_plan, submit_turn_state, or submit_intermediate_answer only when their structured state is useful for the current request. Do not expose protocol names or bookkeeping to the user.",
	}
	if preferExplicitFinalAnswer {
		instructions = append(instructions, "When the answer is complete, call submit_final_answer with the complete user-facing reply as the only terminal action.")
	} else {
		instructions = append(instructions, "When no protocol tool is needed, provide the complete user-facing reply as ordinary assistant content and end the turn.")
	}
	return adapter.Message{Role: "system", Content: strings.Join(instructions, "\n")}
}

func AgenticSkillLoopSystemMessage() adapter.Message {
	return agenticSkillLoopSystemMessage(true)
}
