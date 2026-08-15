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

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
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
	runtimeStateNativeAgentLoopKey                = "_native_agent_loop"
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
	stopBusinessLoop    bool
	fatalErr            error
}

type planningResult struct {
	message               adapter.Message
	usage                 *adapter.Usage
	contextMessages       []adapter.Message
	answerStreamed        bool
	naturalAnswerStreamed bool
	provisionalContent    string
	provisionalStreamed   bool
	reasoningObserved     bool
}

type modelToolRoundCallResult struct {
	response *adapter.ChatResponse
	err      error
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

type reactiveCompactAttemptContextKey struct{}

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
	prepared.resetPresentationEventError()
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
	nativeToolSet := req.NativeToolSet
	if req.NativeSkillSession != nil {
		current := req.NativeSkillSession.ToolSet()
		nativeToolSet = &current
	}
	historicalLoadedSkills, restoreValidationTraces := validatedHistoricalLoadedSkillsForRun(ctx, req, resolved)
	restoredSkillState := restoredLoadedSkillInstructionStateForRun(resolved, historicalLoadedSkills, req.PreferredRestoredSkillID)
	if req.NativeAgentLoop {
		restoreValidationTraces = nil
		restoredSkillState = restoredSkillInstructionState{activeLoaded: map[string]struct{}{}}
	}
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
	} else if req.NativeAgentLoop {
		if req.NativeSkillSession != nil {
			if candidateMessage := req.NativeSkillSession.CatalogMessage(); strings.TrimSpace(messageContent(candidateMessage.Content)) != "" {
				messages = append(messages, candidateMessage)
			}
		}
		if nativeToolSet != nil {
			messages = append(messages, nativeToolSet.InstructionMessages...)
		}
		messages = append(messages, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
		messages = append(messages, nativeAgentLoopSystemMessage())
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
	if req.NativeAgentLoop {
		if req.NativeSkillSession != nil {
			loadedSkills = map[string]struct{}{}
			for _, skillID := range req.NativeSkillSession.ActiveSkillIDs() {
				loadedSkills[strings.ToLower(strings.TrimSpace(skillID))] = struct{}{}
			}
			for _, attempt := range req.NativeSkillSession.ActivationAttempts() {
				trace := nativeSessionActivationAttemptTrace(attempt)
				traces = append(traces, trace)
				r.recordTrace(traces, trace)
				r.logSkillTrace(ctx, prepared, trace)
			}
		} else {
			var preloadErr error
			loadedSkills, preloadErr = r.preloadNativeSkills(ctx, prepared, resolved, nativeToolSet, &traces)
			if preloadErr != nil {
				return "", nil, preloadErr
			}
		}
	} else {
		metadataTrace := metadataExposedTrace(resolved.SkillIDs(), metadataStats)
		traces = append(traces, metadataTrace)
		r.recordTrace(traces, metadataTrace)
		logger.DebugContext(ctx, "aichat skill metadata exposed",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"skill_ids", resolved.SkillIDs(),
			"skill_mode", prepared.parts.SkillMode,
		)
	}

	stepCount := 0
	toolCallCount := 0
	recoverableFailureRoundCount := 0
	consecutiveRecoverableFailureRounds := 0
	recoverableFailureCallCount := 0
	recoverableFailureCounts := map[string]int{}
	failedToolCallAttemptCounts := map[string]int{}
	emptyFinalAnswerRetryCount := 0
	skillToolCallCounts := map[string]int{}
	successfulToolCalls := []SkillToolCallRef{}
	failedToolCallReasons := map[string]string{}
	var latestRecoverableTrace skills.SkillTrace
	skillUsed := req.NativeAgentLoop && len(loadedSkills) > 0
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	terminalStateGuardConfigured := req.RuntimeStateSnapshot != nil
	planRevisionRequired := userInputPlanRevisionPending(req)
	var answerBuilder strings.Builder
	var usage *adapter.Usage
	var nativeCommentary *nativeCommentaryState
	if req.NativeAgentLoop {
		nativeCommentary = newNativeCommentaryState(prepared.LLMRequest.Model, currentMetadataForRun(req))
	}
	r.diagnostics = modelInvocationDiagnostics{
		activeSkillIDs:   activeSkillIDsForDiagnostics(resolved, loadedSkills),
		loadedSkillIDs:   activeSkillIDsForDiagnostics(resolved, loadedSkills),
		restoredSkillIDs: append([]string(nil), restoredSkillState.restored...),
		continuationType: strings.TrimSpace(req.ContinuationType),
		terminalOnly:     req.TerminalOnly,
	}
	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		if eventErr := prepared.presentationEventError(); eventErr != nil {
			return answerBuilder.String(), usage, eventErr
		}
		roundRuntimeState := map[string]interface{}{}
		terminalSubmissionAllowed := true
		if terminalStateGuardConfigured {
			roundRuntimeState = runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
			terminalSubmissionAllowed = terminalStateGuardCanStream(roundRuntimeState)
		}
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		if req.NativeAgentLoop {
			planningReq.Tools = nativeAgentToolsForRun(resolved, nativeToolSet, req.NativeSkillSession)
		} else {
			planningReq.Tools = metaToolsForRun(resolved, loadedSkills, preferExplicitFinalAnswer, false)
		}
		planningReq.Tools = appendContextArtifactTool(planningReq.Tools, r.ContextManager != nil)
		allowPlanUpdate := planRevisionRequired || operationPlanModelRevisionRequired(req, roundRuntimeState)
		allowIntermediateAnswer := !fileDeliveryRequiresArtifactOnly(req, roundRuntimeState)
		if !req.NativeAgentLoop {
			planningReq.Tools = controlToolsForRound(
				planningReq.Tools,
				allowPlanUpdate,
				allowIntermediateAnswer,
			)
		}
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
		if req.NativeAgentLoop {
			planningResult, err = r.runNativeAgentRound(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress)
			err = nativeAgentOutputError(err)
		} else if req.TerminalOnly {
			planningResult, err = r.runTerminalOnlyPlanningWithRetry(
				ctx,
				prepared,
				planningReq,
				round,
				req.OnChunk,
				deferTerminalContent,
				suppressNaturalProgress,
			)
		} else {
			planningResult, err = r.runSkillPlanningWithRetry(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress, req.PlanningOutputTokenLimit)
		}
		usage = mergeUsage(usage, planningResult.usage)
		if len(planningResult.contextMessages) > 0 {
			messages = cloneMessagesForProvider(planningResult.contextMessages)
		}
		if eventErr := prepared.presentationEventError(); eventErr != nil {
			return answerBuilder.String(), usage, eventErr
		}
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
		if req.NativeAgentLoop && len(toolCalls) > 0 && strings.TrimSpace(text) != "" {
			candidate := planningResult.provisionalContent
			candidateStreamed := planningResult.provisionalStreamed
			if strings.TrimSpace(candidate) == "" && !suppressNaturalProgress && !deferTerminalContent {
				candidate = text
				r.emitAnswerChunk(ctx, prepared, candidate, nil)
				candidateStreamed = true
			}
			if candidateStreamed && strings.TrimSpace(candidate) != "" {
				decision := nativeCommentary.classify(candidate, toolCalls, nativeToolSet)
				r.emitAnswerRetractWithDisposition(ctx, prepared, candidate, decision.disposition)
				r.logNativeCommentaryDecision(ctx, prepared, round, decision, planningResult.reasoningObserved)
				if eventErr := prepared.presentationEventError(); eventErr != nil {
					return answerBuilder.String(), usage, eventErr
				}
			}
		}
		if req.NativeAgentLoop && len(toolCalls) == 0 && strings.TrimSpace(text) == "" {
			return answerBuilder.String(), usage, fmt.Errorf("%w: model returned neither a tool call nor assistant content", ErrAgentOutputTruncated)
		}
		if req.TerminalOnly && len(toolCalls) > 0 {
			if checkpointErr := r.checkpointTerminalToolBatch(ctx, messages, planningMessage, toolCalls, "", adapter.Message{}, "terminal-only execution rejected an unexpected tool call"); checkpointErr != nil {
				return answerBuilder.String(), usage, checkpointErr
			}
			return answerBuilder.String(), usage, errors.Join(
				ErrFinalAnswerUnavailable,
				errors.New("terminal-only model returned an unexpected tool call after retry"),
			)
		}
		if req.TerminalOnly && strings.TrimSpace(text) == "" {
			return answerBuilder.String(), usage, errors.Join(
				ErrFinalAnswerUnavailable,
				errors.New("terminal-only model returned no final answer after retry"),
			)
		}
		if text != "" && len(toolCalls) > 0 && !suppressNaturalProgress && !req.NativeAgentLoop {
			if !planningResult.naturalAnswerStreamed &&
				!nativeSessionControlCallsOnly(toolCalls) &&
				shouldEmitNaturalProgressForToolCalls(resolved, loadedSkills, nativeExecutionCalls(toolCalls, nativeToolSet)) {
				r.emitAnswerChunk(ctx, prepared, text, nil)
				r.emitAnswerRetract(ctx, prepared, text)
			}
		}
		if len(toolCalls) == 0 && terminalStateGuardConfigured {
			if strings.TrimSpace(text) == "" {
				if req.TerminalOnly {
					return answerBuilder.String(), usage, errors.Join(
						ErrFinalAnswerUnavailable,
						errors.New("terminal-only model returned no final answer after retry"),
					)
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
				if checkpointErr := r.checkpointTerminalToolBatch(ctx, messages, planningMessage, toolCalls, call.ID, result.toolMessage, "the round ended after submit_final_answer was rejected"); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, parseErr
			}

			submission.streamed = submission.streamed || planningResult.answerStreamed
			guard := terminalStateGuardEvaluate(roundRuntimeState, submission.answer)
			if guard.Path != terminalStateGuardAccepted {
				guardErr := terminalStateGuardError(guard)
				result := failedFinalAnswerSkillStep(call.ID, guardErr, "resolve the terminal-state blockers before submitting a final answer")
				if checkpointErr := r.checkpointTerminalToolBatch(ctx, messages, planningMessage, toolCalls, call.ID, result.toolMessage, "the round ended after submit_final_answer failed terminal-state validation"); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
			}
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
			if checkpointErr := r.checkpointTerminalToolBatch(ctx, messages, planningMessage, toolCalls, call.ID, result.toolMessage, "the round ended after submit_final_answer was accepted"); checkpointErr != nil {
				return answerBuilder.String(), usage, checkpointErr
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
			appendAnswerText(&answerBuilder, text)
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
			limitErr := fmt.Errorf("%w: too many skill steps", ErrInvalidInput)
			trace := failedSkillTrace("tool_retry_guard", "", limitErr)
			trace.Arguments = map[string]interface{}{
				"reason_code":   "skill_step_limit_exceeded",
				"current_steps": stepCount,
				"requested":     len(toolCalls),
				"max_steps":     maxSkillSteps,
			}
			traces = append(traces, trace)
			r.recordTrace(traces, trace)
			r.logSkillTrace(ctx, prepared, trace)
			r.emitSkillError(ctx, prepared, trace)
			planningMessage.Role = "assistant"
			planningMessage.ToolCalls = toolCalls
			planningMessage.ReasoningContent = ""
			messages = append(messages, planningMessage)
			messages = appendCancelledSiblingToolResults(messages, toolCalls, "the Agent tool-step safety limit was reached before this tool could run")
			if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
				return answerBuilder.String(), usage, checkpointErr
			}
			answer, explanationUsage, explanationErr := r.completeBusinessToolFailure(
				ctx,
				req,
				prepared,
				traces,
				trace,
				successfulToolCalls,
				"the business-tool step safety limit was reached before the requested operation completed",
				round,
			)
			usage = mergeUsage(usage, explanationUsage)
			if explanationErr != nil {
				return answerBuilder.String(), usage, explanationErr
			}
			return answer, usage, nil
		}
		logger.DebugContext(ctx, "aichat skill planning requested tool calls",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"tool_call_count", len(toolCalls),
			"step_count", stepCount,
		)

		planningMessage.Role = "assistant"
		planningMessage.ToolCalls = toolCalls
		// Internal reasoning is never replayed. Ordinary assistant content is
		// retained even on tool-call turns because it may contain model-visible
		// plans, constraints, and judgments needed by the next API round.
		planningMessage.ReasoningContent = ""
		messages = append(messages, planningMessage)

		roundHadRecoverableFailure := false
		roundHadSuccess := false
		roundMadeFailureProgress := false
		var lastRecoverableTrace skills.SkillTrace
		stopBusinessLoop := false
		var stopBusinessLoopTrace skills.SkillTrace
		roundDeferredSystemMessages := []adapter.Message{}
		for callIndex, call := range toolCalls {
			stepCount++
			executionCall := call
			if req.NativeAgentLoop {
				executionCall = nativeExecutionCall(call, nativeToolSet)
			}
			callSkillID, callToolName, callToolArgs, failedCallKey := skillToolCallIdentityForCall(resolved, loadedSkills, executionCall)
			callEvidence := runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
			callEvidence[runtimeStateAllowPlanUpdateKey] = req.NativeAgentLoop || planRevisionRequired || operationPlanModelRevisionRequired(req, callEvidence)
			callEvidence[runtimeStateAllowIntermediateAnswerKey] = !fileDeliveryRequiresArtifactOnly(req, callEvidence)
			callEvidence[runtimeStateNativeAgentLoopKey] = req.NativeAgentLoop
			result := skillStepResult{}
			if req.NativeAgentLoop && req.NativeSkillSession != nil {
				if control, handled := r.handleNativeSessionControlCall(ctx, call, req.NativeSkillSession); handled {
					result = control.step
					if len(control.instructions) > 0 {
						roundDeferredSystemMessages = append(roundDeferredSystemMessages, control.instructions...)
					}
					current := req.NativeSkillSession.ToolSet()
					nativeToolSet = &current
					for _, skillID := range current.ActiveSkillIDs {
						loadedSkills[strings.ToLower(strings.TrimSpace(skillID))] = struct{}{}
					}
					r.diagnostics.loadedSkillIDs = append([]string(nil), current.ActiveSkillIDs...)
				}
			}
			if req.NativeAgentLoop && nativeForbiddenProtocolTool(call.Function.Name) {
				result = nativeForbiddenProtocolToolStep(call.ID, call.Function.Name)
			}
			if result.trace.Kind == "" && userInputPlanRevisionRequiredForTool(req, callSkillID, callToolName) {
				result = pendingUserInputPlanRevisionStep(executionCall.ID, callSkillID, callToolName, callToolArgs)
			}
			if result.trace.Kind == "" && req.AuthorizeSkillStep != nil && strings.TrimSpace(callSkillID) != "" {
				allowed, policyErr := req.AuthorizeSkillStep(ctx, callSkillID)
				if policyErr != nil || !allowed {
					result = unavailableSkillPolicyStep(executionCall.ID, callSkillID, callToolName, callToolArgs, policyErr)
				}
			}
			if result.trace.Kind == "" && failedCallKey != "" {
				if reason := failedToolCallReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
					result = repeatedFailedToolCallRecoverableStep(executionCall.ID, callSkillID, callToolName, callToolArgs, reason)
				}
			}
			if result.trace.Kind == "" {
				result = r.handleProgressiveSkillCall(ctx, prepared, resolved, executionCall, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, callEvidence, round+1, nil)
			}
			if eventErr := prepared.presentationEventError(); eventErr != nil {
				return answerBuilder.String(), usage, eventErr
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
					if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
						return answerBuilder.String(), usage, checkpointErr
					}
				}
				continue
			}
			if result.recoverable {
				if result.trace.Arguments == nil {
					result.trace.Arguments = map[string]interface{}{}
				}
				result.trace.Arguments["failure_category"] = recoverableSkillFailureCategory(result.trace)
				annotateRecoverableSkillFailure(
					&result.trace,
					failedCallKey,
					failedToolCallAttemptCounts,
					result.stopBusinessLoop,
				)
				if recoverableSkillFailureMadeProgress(latestRecoverableTrace, result.trace) {
					roundMadeFailureProgress = true
				}
			}
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable &&
				failedCallKey != "" &&
				strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
				shouldRememberFailedToolCall(result.trace) {
				failedToolCallReasons[failedCallKey] = strings.TrimSpace(result.trace.Error)
				if failedToolCallReasons[failedCallKey] == "" {
					failedToolCallReasons[failedCallKey] = "previous tool call with the same arguments failed"
				}
			}
			if result.recoverable {
				failureCategory := recoverableSkillFailureCategory(result.trace)
				recoverableFailureCounts[failureCategory]++
				if !internalPlannerFeedbackTrace(result.trace) {
					r.emitSkillError(ctx, prepared, result.trace)
				}
				roundHadRecoverableFailure = true
				lastRecoverableTrace = result.trace
				latestRecoverableTrace = result.trace
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
				if result.toolMessage.Role != "" || result.toolMessage.ToolCallID != "" || result.toolMessage.Content != nil {
					messages = append(messages, result.toolMessage)
				}
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round stopped after a fatal sibling tool failure")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, result.fatalErr
			}
			messages = append(messages, result.toolMessage)
			if projected, stats := projectMaterializedFileContent(messages, result.toolMessage.ToolCallID, result.toolResult); stats.removedRunes > 0 {
				messages = projected
				r.diagnostics.projectedRefs = appendUniqueProjectionRefs(r.diagnostics.projectedRefs, stats.refs...)
				r.diagnostics.projectedChars += stats.removedRunes
			}
			if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
				return answerBuilder.String(), usage, checkpointErr
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
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round paused for approval before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, &WorkflowApprovalPendingError{Payload: result.pendingApproval}
			}
			if result.pendingQuestion != nil {
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round paused for a workflow answer before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, &WorkflowQuestionPendingError{Payload: result.pendingQuestion}
			}
			if result.pendingGovernance != nil {
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round paused for governance before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, &ToolGovernancePendingError{Payload: result.pendingGovernance}
			}
			if result.pendingClientAction != nil {
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round paused for a client action before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, &ClientActionPendingError{Payload: result.pendingClientAction}
			}
			if result.answer != "" {
				appendAnswerText(&answerBuilder, result.answer)
				if !result.answerStreamed {
					r.emitAnswerChunk(ctx, prepared, result.answer, nil)
				}
			}
			if result.pendingUserInput != nil {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round paused for user input before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, &UserInputPendingError{Payload: result.pendingUserInput}
			}
			if result.terminal {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				messages = appendCancelledSiblingToolResults(messages, toolCalls[callIndex+1:], "the round ended before this sibling tool could run")
				if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
					return answerBuilder.String(), usage, checkpointErr
				}
				return answerBuilder.String(), usage, nil
			}
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "skill_load") &&
				strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
				r.diagnostics.loadedSkillIDs = appendUniqueProjectionRefs(r.diagnostics.loadedSkillIDs, result.trace.SkillID)
			}
			if message, ok := governedReadFileTargetSystemMessage(result.trace); ok {
				roundDeferredSystemMessages = append(roundDeferredSystemMessages, message)
			}
			if req.NativeAgentLoop {
				if message, ok := nativeReferenceReadContinuationSystemMessage(result.trace); ok {
					roundDeferredSystemMessages = append(roundDeferredSystemMessages, message)
				}
			}
			if result.stopBusinessLoop {
				stopBusinessLoop = true
				stopBusinessLoopTrace = result.trace
			}
		}
		if len(roundDeferredSystemMessages) > 0 {
			messages = append(messages, roundDeferredSystemMessages...)
			if checkpointErr := r.checkpointContext(ctx, messages); checkpointErr != nil {
				return answerBuilder.String(), usage, checkpointErr
			}
		}
		if stopBusinessLoop && !roundHadSuccess && !roundMadeFailureProgress {
			answer, explanationUsage, explanationErr := r.completeBusinessToolFailure(
				ctx,
				req,
				prepared,
				traces,
				stopBusinessLoopTrace,
				successfulToolCalls,
				"the model repeated the same failed business-tool call without changing its arguments",
				round,
			)
			usage = mergeUsage(usage, explanationUsage)
			if explanationErr != nil {
				return answerBuilder.String(), usage, explanationErr
			}
			return answer, usage, nil
		}
		if preferExplicitFinalAnswer && !roundHadRecoverableFailure && terminalMetaCallsOnly(toolCalls) {
			messages = append(messages, adapter.Message{
				Role:    "system",
				Content: "The previous assistant turn only recorded internal state or plan progress. Continue the same user turn: call the next necessary business tool, request user input if blocked, or call submit_final_answer when the task is actually complete. Do not rely on ordinary assistant content from a meta-tool turn as the final answer.",
			})
		}
		if roundHadRecoverableFailure {
			recoverableFailureRoundCount++
			if !roundHadSuccess && !roundMadeFailureProgress {
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
				"failure_categories", recoverableFailureCounts,
				"failure_progress", roundMadeFailureProgress,
			)
			if recoverableFailureRoundCount >= defaultMaxRecoverableFailureRounds ||
				consecutiveRecoverableFailureRounds >= defaultMaxConsecutiveRecoverableFailureRounds {
				err := fmt.Errorf("%w: too many failed skill calls", ErrInvalidInput)
				trace := failedSkillTrace("tool_retry_guard", lastRecoverableTrace.ToolName, err)
				trace.SkillID = lastRecoverableTrace.SkillID
				trace.Arguments = copyStringAnyMap(lastRecoverableTrace.Arguments)
				if trace.Arguments == nil {
					trace.Arguments = map[string]interface{}{}
				}
				trace.Arguments["reason_code"] = "skill_tool_failure_limit_reached"
				trace.Arguments["failure_categories"] = copyStringIntMap(recoverableFailureCounts)
				trace.Arguments["termination_reason"] = "safe_retry_limit_reached"
				traces = append(traces, trace)
				r.recordTrace(traces, trace)
				r.logSkillTrace(ctx, prepared, trace)
				if !internalPlannerFeedbackTrace(lastRecoverableTrace) {
					r.emitSkillError(ctx, prepared, trace)
				}
				answer, explanationUsage, explanationErr := r.completeBusinessToolFailure(
					ctx,
					req,
					prepared,
					traces,
					lastRecoverableTrace,
					successfulToolCalls,
					"the business tool continued to fail and the safe retry limit was reached",
					round,
				)
				usage = mergeUsage(usage, explanationUsage)
				if explanationErr != nil {
					return answerBuilder.String(), usage, explanationErr
				}
				return answer, usage, nil
			}
		} else {
			consecutiveRecoverableFailureRounds = 0
		}
		if req.TerminalOnly {
			return answerBuilder.String(), usage, errors.Join(
				ErrFinalAnswerUnavailable,
				errors.New("terminal-only model did not submit a final answer after retry"),
			)
		}
	}

	err := fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
	trace := latestRecoverableTrace
	if strings.TrimSpace(trace.Kind) == "" {
		trace = failedSkillTrace("tool_retry_guard", "", err)
		trace.Arguments = map[string]interface{}{}
	}
	trace.Kind = "tool_retry_guard"
	trace.Status = "error"
	trace.Error = err.Error()
	if trace.Arguments == nil {
		trace.Arguments = map[string]interface{}{}
	}
	trace.Arguments["reason_code"] = "skill_planning_rounds_exhausted"
	traces = append(traces, trace)
	r.recordTrace(traces, trace)
	r.logSkillTrace(ctx, prepared, trace)
	r.emitSkillError(ctx, prepared, trace)
	answer, explanationUsage, explanationErr := r.completeBusinessToolFailure(
		ctx,
		req,
		prepared,
		traces,
		trace,
		successfulToolCalls,
		"the planning loop reached its safety limit before the requested operation completed",
		defaultMaxSkillPlanningRounds,
	)
	usage = mergeUsage(usage, explanationUsage)
	if explanationErr != nil {
		return answerBuilder.String(), usage, explanationErr
	}
	return answer, usage, nil
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
		contextArtifactToolName:           {},
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
	return businessToolCallCountForCalls(resolved, loadedSkills, calls) > 0
}

func businessToolCallCountForCalls(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, calls []adapter.ToolCall) int {
	active := make(map[string]struct{}, len(loadedSkills))
	for skillID := range loadedSkills {
		active[skillID] = struct{}{}
	}
	count := 0
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
				count++
			}
		case skills.MetaToolReadSkillReference,
			contextArtifactToolName,
			skills.MetaToolRequestUserInput,
			skills.MetaToolTurnState,
			skills.MetaToolUpdatePlan,
			skills.MetaToolIntermediateAnswer,
			skills.MetaToolFinalAnswer:
			continue
		default:
			if _, ok := uniqueLoadedSkillForToolName(resolved, active, name); ok {
				count++
			}
		}
	}
	return count
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
	toolArgs, argumentsErr := normalizeSkillToolArguments(args, skillID, toolName)
	if argumentsErr != nil {
		toolArgs = rawSkillToolArgumentsFingerprint(args["arguments"])
	}
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

func recoverableSkillFailureCategory(trace skills.SkillTrace) string {
	switch category := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(trace.Arguments["failure_category"]))); category {
	case "arguments", "permission", "transient", "tool":
		return category
	}
	reasonCode := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(trace.Arguments["reason_code"])))
	if strings.HasPrefix(reasonCode, "skill_tool_arguments_") {
		return "arguments"
	}
	if strings.Contains(reasonCode, "permission") ||
		strings.Contains(reasonCode, "denied") ||
		strings.Contains(reasonCode, "unauthorized") ||
		strings.Contains(reasonCode, "forbidden") ||
		strings.Contains(reasonCode, "not_preauthorized") {
		return "permission"
	}

	message := strings.ToLower(strings.TrimSpace(trace.Error))
	for _, fragment := range []string{
		"timeout",
		"timed out",
		"temporary",
		"connection reset",
		"connection refused",
		"network",
		"unexpected eof",
		"service unavailable",
		"tls handshake",
		"too many requests",
		"rate limit",
	} {
		if strings.Contains(message, fragment) {
			return "transient"
		}
	}
	for _, fragment := range []string{
		"permission denied",
		"access denied",
		"unauthorized",
		"forbidden",
		"not preauthorized",
	} {
		if strings.Contains(message, fragment) {
			return "permission"
		}
	}
	return "tool"
}

func shouldRememberFailedToolCall(trace skills.SkillTrace) bool {
	return recoverableSkillFailureCategory(trace) != "transient"
}

func annotateRecoverableSkillFailure(
	trace *skills.SkillTrace,
	failedCallKey string,
	attemptCounts map[string]int,
	stopBusinessLoop bool,
) {
	if trace == nil {
		return
	}
	if trace.Arguments == nil {
		trace.Arguments = map[string]interface{}{}
	}

	argumentFingerprint := digestDiagnosticValue(failedCallKey)
	if argumentFingerprint != "" {
		trace.Arguments["argument_fingerprint"] = argumentFingerprint
		if attemptCounts != nil {
			attemptCounts[failedCallKey]++
			trace.Arguments["retry_count"] = attemptCounts[failedCallKey]
		}
	}
	if failureFingerprint := recoverableSkillFailureFingerprint(*trace, argumentFingerprint); failureFingerprint != "" {
		trace.Arguments["failure_fingerprint"] = failureFingerprint
	}
	if stopBusinessLoop {
		trace.Arguments["termination_reason"] = "retry_no_progress"
	}
}

func recoverableSkillFailureFingerprint(trace skills.SkillTrace, argumentFingerprint string) string {
	payload := map[string]interface{}{
		"skill_id":             strings.ToLower(strings.TrimSpace(trace.SkillID)),
		"tool_name":            strings.ToLower(strings.TrimSpace(trace.ToolName)),
		"reason_code":          strings.ToLower(strings.TrimSpace(evidenceStringFromAny(trace.Arguments["reason_code"]))),
		"missing_fields":       evidenceStringSliceFromAny(trace.Arguments["missing_fields"]),
		"argument_fingerprint": strings.TrimSpace(argumentFingerprint),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return digestDiagnosticBytes(encoded)
}

func recoverableSkillFailureMadeProgress(previous skills.SkillTrace, current skills.SkillTrace) bool {
	if strings.TrimSpace(previous.Kind) == "" || strings.TrimSpace(current.Kind) == "" {
		return false
	}
	if recoverableSkillFailureCategory(previous) == "transient" ||
		recoverableSkillFailureCategory(current) == "transient" {
		return false
	}

	previousSkillID := strings.ToLower(strings.TrimSpace(previous.SkillID))
	currentSkillID := strings.ToLower(strings.TrimSpace(current.SkillID))
	previousToolName := strings.ToLower(strings.TrimSpace(previous.ToolName))
	currentToolName := strings.ToLower(strings.TrimSpace(current.ToolName))
	if previousSkillID != currentSkillID || previousToolName != currentToolName {
		return currentSkillID != "" || currentToolName != ""
	}

	previousArgumentFingerprint := strings.TrimSpace(evidenceStringFromAny(previous.Arguments["argument_fingerprint"]))
	currentArgumentFingerprint := strings.TrimSpace(evidenceStringFromAny(current.Arguments["argument_fingerprint"]))
	if previousArgumentFingerprint != "" &&
		currentArgumentFingerprint != "" &&
		previousArgumentFingerprint != currentArgumentFingerprint {
		return true
	}

	previousMissingFields := evidenceStringSliceFromAny(previous.Arguments["missing_fields"])
	currentMissingFields := evidenceStringSliceFromAny(current.Arguments["missing_fields"])
	if len(previousMissingFields) > 0 && len(currentMissingFields) < len(previousMissingFields) {
		return true
	}

	previousActualType := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(previous.Arguments["actual_type"])))
	currentActualType := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(current.Arguments["actual_type"])))
	return previousActualType != "" &&
		previousActualType != "object" &&
		currentActualType == "object"
}

func digestDiagnosticValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return digestDiagnosticBytes([]byte(value))
}

func digestDiagnosticBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func copyStringIntMap(input map[string]int) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func repeatedFailedToolCallRecoverableStep(callID string, skillID string, toolName string, args map[string]interface{}, reason string) skillStepResult {
	message := "same tool call with the same arguments already failed in this turn"
	if reason = strings.TrimSpace(reason); reason != "" {
		message += ": " + reason
	}
	err := &skillToolArgumentsError{
		Code:              skillToolRetryNoProgressCode,
		SkillID:           strings.ToLower(strings.TrimSpace(skillID)),
		ToolName:          strings.TrimSpace(toolName),
		ExpectedType:      "object",
		ActualType:        "object",
		ExpectedArguments: skills.ExpectedSkillToolArguments(skillID, toolName),
		RetryAction:       "stop retrying this business tool and explain the incomplete operation truthfully",
		Cause:             fmt.Errorf("%w: %s", ErrInvalidInput, message),
	}
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Arguments = map[string]interface{}{}
	if len(args) > 0 {
		trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	}
	trace.Arguments["reason_code"] = skillToolRetryNoProgressCode
	result := recoverableSkillStep(
		trace,
		skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, err.RetryAction, skillID, toolName)),
		false,
		false,
	)
	result.stopBusinessLoop = true
	return result
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

func appendCancelledSiblingToolResults(messages []adapter.Message, calls []adapter.ToolCall, reason string) []adapter.Message {
	for _, call := range calls {
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			continue
		}
		messages = append(messages, skills.ToolResultMessage(callID, map[string]interface{}{
			"status":      "cancelled",
			"tool_name":   strings.TrimSpace(call.Function.Name),
			"reason":      strings.TrimSpace(reason),
			"instruction": "The tool was not executed. Decide whether it is still needed after the paused run resumes.",
		}))
	}
	return messages
}

func (r *Runner) checkpointTerminalToolBatch(ctx context.Context, messages []adapter.Message, assistant adapter.Message, calls []adapter.ToolCall, completedCallID string, completedResult adapter.Message, reason string) error {
	if r == nil || r.ContextManager == nil {
		return nil
	}
	assistant.Role = "assistant"
	assistant.ToolCalls = calls
	assistant.ReasoningContent = ""
	completed := append(cloneMessagesForProvider(messages), assistant)
	completedCallID = strings.TrimSpace(completedCallID)
	for _, call := range calls {
		if completedCallID != "" && strings.TrimSpace(call.ID) == completedCallID {
			completed = append(completed, completedResult)
			continue
		}
		completed = appendCancelledSiblingToolResults(completed, []adapter.ToolCall{call}, reason)
	}
	return r.checkpointContext(ctx, completed)
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
	return r.runModelToolRound(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, "skill_planning")
}

func (r *Runner) runNativeAgentRound(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool) (planningResult, error) {
	return r.runModelToolRound(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, "agent_tool_loop")
}

func (r *Runner) runModelToolRound(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool, phase string) (planningResult, error) {
	var err error
	planningReq, err = r.prepareContextRequest(ctx, planningReq)
	if err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:     phase,
			Round:     round,
			Request:   planningReq,
			StartedAt: time.Now(),
			Error:     err.Error(),
		})
		return planningResult{}, err
	}
	var progress *modelProgressTracker
	if phase == "agent_tool_loop" {
		progress = r.startModelProgressTracker(ctx, prepared, round, planningReq.Model, planningReq.Messages)
	}
	defer progress.Stop()
	if shouldStreamSkillPlanning(prepared) {
		result, ok, streamErr := r.runModelToolRoundStream(ctx, prepared, planningReq, round, nil, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, phase, progress)
		if streamErr != nil {
			result = r.finishFailedContextRequest(planningReq, result)
			if retried, retryResult, retryErr := r.retryAfterPromptTooLong(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, phase, streamErr); retried {
				retryResult.usage = mergeUsage(result.usage, retryResult.usage)
				return retryResult, retryErr
			}
			return result, streamErr
		}
		if ok {
			return r.finishContextRequest(ctx, planningReq, result)
		}
		// Opening the stream was an upstream attempt. Re-run the complete
		// preflight before the non-streaming fallback call as well.
		planningReq, err = r.prepareContextRequest(ctx, planningReq)
		if err != nil {
			return planningResult{}, err
		}
	}

	planningReq.Stream = false
	startedAt := time.Now()
	r.recordModelRequest(phase, round, planningReq)
	callCtx, cancel := context.WithTimeout(ctx, r.modelIdleTimeout())
	resultCh := make(chan modelToolRoundCallResult, 1)
	go func() {
		response, err := r.LLMClient.AppChat(callCtx, r.AppContext, planningReq)
		resultCh <- modelToolRoundCallResult{response: response, err: err}
	}()
	var callResult modelToolRoundCallResult
	select {
	case callResult = <-resultCh:
	case <-callCtx.Done():
		callResult.err = callCtx.Err()
	}
	callErr := callCtx.Err()
	cancel()
	planningResp, err := callResult.response, callResult.err
	if err != nil {
		if errors.Is(callErr, context.DeadlineExceeded) {
			err = ErrModelIdleTimeout
		}
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:      phase,
			Round:      round,
			Streaming:  false,
			StartedAt:  startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Request:    planningReq,
			Error:      err.Error(),
		})
		result := r.finishFailedContextRequest(planningReq, planningResult{})
		if retried, retryResult, retryErr := r.retryAfterPromptTooLong(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, phase, err); retried {
			retryResult.usage = mergeUsage(result.usage, retryResult.usage)
			return retryResult, retryErr
		}
		return result, err
	}
	message := firstPlanningMessage(planningResp)
	usage := planningRespUsage(planningResp)
	finishReason := planningResponseFinishReason(planningResp)
	terminationErr := nonStreamingPlanningTerminationError(finishReason)
	r.recordModelInvocation(ModelInvocationTrace{
		Phase:              phase,
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
		result, observeErr := r.finishContextRequest(ctx, planningReq, planningResult{
			message:           message,
			usage:             usage,
			reasoningObserved: strings.TrimSpace(message.ReasoningContent) != "",
		})
		if observeErr != nil {
			return result, observeErr
		}
		return result, terminationErr
	}
	return r.finishContextRequest(ctx, planningReq, planningResult{
		message:           message,
		usage:             usage,
		reasoningObserved: strings.TrimSpace(message.ReasoningContent) != "",
	})
}

func (r *Runner) prepareContextRequest(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatRequest, error) {
	request = cloneChatRequest(request)
	sourceMessages := cloneMessagesForProvider(request.Messages)
	request.Messages = adapter.NormalizeSystemMessages(sourceMessages)
	r.contextDecision = nil
	if r.ContextManager == nil {
		return request, r.applyFinalPlanningRequestBudget(request, sourceMessages)
	}
	prepared, decision, err := r.ContextManager.PrepareBeforeModelCall(ctx, request)
	if err != nil {
		return request, err
	}
	r.contextDecision = &decision
	r.diagnostics.requestBudget = planningRequestBudgetDiagnostics{
		safeContextLimit:     decision.Budget.HardLimit,
		promptBudget:         decision.Budget.PromptBudget,
		originalPromptTokens: decision.BeforeTokens,
		finalPromptTokens:    decision.FinalPromptTokens,
		estimateScale:        decision.EstimateScale,
	}
	return prepared, nil
}

func (r *Runner) finishContextRequest(ctx context.Context, request *adapter.ChatRequest, result planningResult) (planningResult, error) {
	result.contextMessages = cloneMessagesForProvider(request.Messages)
	if r.ContextManager == nil {
		return result, nil
	}
	mainModelUsage := result.usage
	result.usage = mergeUsage(r.ContextManager.ConsumeCompactionUsage(), result.usage)
	if result.message.Role == "" && result.message.Content == nil && len(result.message.ToolCalls) == 0 {
		return result, nil
	}
	if err := r.ContextManager.ObserveModelResponse(ctx, result.message, mainModelUsage); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runner) finishFailedContextRequest(request *adapter.ChatRequest, result planningResult) planningResult {
	result.contextMessages = cloneMessagesForProvider(request.Messages)
	if r != nil && r.ContextManager != nil {
		result.usage = mergeUsage(r.ContextManager.ConsumeCompactionUsage(), result.usage)
	}
	return result
}

func (r *Runner) checkpointContext(ctx context.Context, messages []adapter.Message) error {
	if r == nil || r.ContextManager == nil {
		return nil
	}
	return r.ContextManager.ReplaceMessagesAndCheckpoint(ctx, messages)
}

func (r *Runner) retryAfterPromptTooLong(
	ctx context.Context,
	prepared *PreparedChat,
	request *adapter.ChatRequest,
	round int,
	onChunk func(string) error,
	terminalProtocol bool,
	terminalStreamingAllowed bool,
	suppressNaturalProgress bool,
	phase string,
	callErr error,
) (bool, planningResult, error) {
	if r == nil || r.ContextManager == nil || !isPromptTooLongError(callErr) {
		return false, planningResult{}, nil
	}
	if attempted, _ := ctx.Value(reactiveCompactAttemptContextKey{}).(bool); attempted {
		return true, planningResult{}, fmt.Errorf("%w: model request remained too large after reactive compaction: %v", contextmgr.ErrContextExhausted, callErr)
	}
	reactiveRequest, decision, compactErr := r.ContextManager.PrepareReactiveCompact(ctx, request)
	if compactErr != nil {
		return true, planningResult{}, errors.Join(callErr, fmt.Errorf("%w: reactive context compaction failed: %v", contextmgr.ErrContextExhausted, compactErr))
	}
	r.contextDecision = &decision
	retryCtx := context.WithValue(ctx, reactiveCompactAttemptContextKey{}, true)
	result, err := r.runModelToolRound(retryCtx, prepared, reactiveRequest, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, phase)
	return true, result, err
}

func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"prompt_too_long",
		"context_length_exceeded",
		"maximum context length",
		"context window exceeded",
		"input is too long",
		"too many input tokens",
		"request too large for model",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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

	currentMaxTokens := planningReq.MaxTokens
	if effectiveMaxTokens := r.diagnostics.requestBudget.effectiveMaxTokens; effectiveMaxTokens > 0 {
		currentMaxTokens = &effectiveMaxTokens
	}
	retryMaxTokens := planningRetryMaxTokens(currentMaxTokens, outputTokenLimit)

	retryReq := cloneChatRequest(planningReq)
	retryReq.Messages = append(append([]adapter.Message{}, planningReq.Messages...), adapter.Message{
		Role:    "system",
		Content: "The previous planning response was truncated by its output limit. Retry once with exactly one complete protocol tool call or one concise final answer. Do not repeat completed operations or add long process narration.",
	})
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
	r.emitEvent(prepared, EventMessage, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          text,
	})
}

func (r *Runner) emitAnswerRetract(ctx context.Context, prepared *PreparedChat, text string) {
	r.emitAnswerRetractWithDisposition(ctx, prepared, text, "")
}

func (r *Runner) emitAnswerRetractWithDisposition(ctx context.Context, prepared *PreparedChat, text string, disposition string) {
	if text == "" {
		return
	}
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         text,
		"length":          utf16CodeUnitLength(text),
		"created_at":      time.Now().Unix(),
	}
	if disposition = strings.TrimSpace(disposition); disposition != "" {
		payload["presentation_disposition"] = disposition
	}
	r.emitEvent(prepared, EventMessageRetract, payload)
}

func utf16CodeUnitLength(text string) int {
	return len(utf16.Encode([]rune(text)))
}

func (r *Runner) emitAgentProgress(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) bool {
	content := localizedAgentProgressText(text)
	if content == "" {
		return false
	}
	r.emitEvent(prepared, EventAgentProgress, map[string]interface{}{
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
