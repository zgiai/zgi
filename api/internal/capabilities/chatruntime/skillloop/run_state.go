package skillloop

import (
	"context"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// runState is what one Run carries across its planning rounds: the
// conversation, the active skills, the call history the guards read and the
// answer built so far.
type runState struct {
	req      RunRequest
	prepared *PreparedChat
	resolved *skills.ResolvedSkills

	nativeToolSet *skills.NativeToolSet
	loadedSkills  map[string]struct{}
	toolPositions map[string]int
	messages      []adapter.Message
	traces        []skills.SkillTrace

	stepCount                   int
	toolCallCount               int
	skillToolCallCounts         map[string]int
	emptyFinalAnswerRetryCount  int
	retryFailureBudgets         *retryFailureBudgetState
	failedToolCallAttemptCounts map[string]int
	failedToolCallReasons       map[string]string
	successfulToolCalls         []SkillToolCallRef
	latestRecoverableTrace      skills.SkillTrace

	skillUsed                    bool
	maxSkillSteps                int
	terminalStateGuardConfigured bool

	answerBuilder    strings.Builder
	usage            *adapter.Usage
	nativeCommentary *nativeCommentaryState
}

// roundState is what one planning round collects while its tool calls run.
type roundState struct {
	hadRecoverableFailure  bool
	hadSuccess             bool
	failureClasses         map[retryFailureClass]bool
	failureProgress        map[retryFailureClass]bool
	stopBusinessLoop       bool
	stopBusinessLoopTrace  skills.SkillTrace
	deferredSystemMessages []adapter.Message
}

func newRoundState() *roundState {
	return &roundState{
		failureClasses:         map[retryFailureClass]bool{},
		failureProgress:        map[retryFailureClass]bool{},
		deferredSystemMessages: []adapter.Message{},
	}
}

// runOutcome ends a Run with its answer, usage and error.
type runOutcome struct {
	answer string
	usage  *adapter.Usage
	err    error
}

func (o *runOutcome) result() (string, *adapter.Usage, error) {
	return o.answer, o.usage, o.err
}

// stop ends the run with the answer built so far.
func (s *runState) stop(err error) *runOutcome {
	return &runOutcome{answer: s.answerBuilder.String(), usage: s.usage, err: err}
}

func (r *Runner) appendTrace(ctx context.Context, s *runState, trace skills.SkillTrace) {
	s.traces = append(s.traces, trace)
	r.recordTrace(s.traces, trace)
	r.logSkillTrace(ctx, s.prepared, trace)
}

// refreshNativeSkills takes the tool set and active skills from the native
// session after a call changed them.
func (r *Runner) refreshNativeSkills(s *runState) {
	current := s.req.NativeSkillSession.ToolSet()
	s.nativeToolSet = &current
	for _, skillID := range current.ActiveSkillIDs {
		s.loadedSkills[strings.ToLower(strings.TrimSpace(skillID))] = struct{}{}
	}
	r.diagnostics.loadedSkillIDs = append([]string(nil), current.ActiveSkillIDs...)
}

// runToolCalls runs one round's tool calls in order, then settles the round.
// A non-nil outcome ends the run.
func (r *Runner) runToolCalls(ctx context.Context, s *runState, round int, toolCalls []adapter.ToolCall) *runOutcome {
	rs := newRoundState()
	r.prefetchedCalls = r.prefetchReadOnlyCalls(ctx, s.req, s.resolved, s.nativeToolSet, s.loadedSkills, s.failedToolCallReasons, toolCalls)
	for callIndex, call := range toolCalls {
		result, failedCallKey := r.executeToolCall(ctx, s, rs, round, call)
		if eventErr := s.prepared.presentationEventError(); eventErr != nil {
			return s.stop(eventErr)
		}
		if outcome := r.recordToolCallResult(ctx, s, rs, result, failedCallKey, toolCalls[callIndex+1:]); outcome != nil {
			return outcome
		}
	}
	return r.finishToolRound(ctx, s, rs, round)
}

// executeToolCall activates the skill a native call belongs to and hands the
// call to the first handler that owns it. It also returns the key under which
// a failure of the call is remembered.
func (r *Runner) executeToolCall(ctx context.Context, s *runState, rs *roundState, round int, call adapter.ToolCall) (skillStepResult, string) {
	req, prepared, resolved := s.req, s.prepared, s.resolved
	s.stepCount++
	callEvidence := runtimeStateWithSuccessfulToolCalls(req, s.successfulToolCalls)
	executionCall := call
	runtimeTool, runtimeCall := runtimeToolForCall(req.RuntimeTools, call)
	if runtimeCall {
		// Runtime tools bypass the regular progressive-skill handler. Emit
		// the same lifecycle events so durable runs and the shared chat UI
		// expose their progress consistently with Agent tools.
		startPayload := skillCallStartPayload(
			prepared,
			call.ID,
			runtimeTool.SkillID,
			runtimeTool.Definition.Function.Name,
			nil,
		)
		if runtimeTool.ReadOnly {
			startPayload[skillCallReadOnlyKey] = true
		}
		r.emitEvent(prepared, EventSkillCallStart, startPayload)
		if err := prepared.presentationEventError(); err != nil {
			return unrecordedCallStep(call.ID, runtimeTool.SkillID, runtimeTool.Definition.Function.Name, err), ""
		}
	}
	if !runtimeCall {
		if activation, activated := activateNativeSkillForToolCall(ctx, call, req.NativeSkillSession, resolved, s.nativeToolSet, s.loadedSkills); activated {
			r.refreshNativeSkills(s)
			rs.deferredSystemMessages = append(rs.deferredSystemMessages, activation.InstructionMessages...)
		}
		executionCall = nativeExecutionCall(call, s.nativeToolSet)
	}
	callSkillID, callToolName, callToolArgs, failedCallKey := skillToolCallIdentityForCall(resolved, s.loadedSkills, executionCall)
	if runtimeCall {
		callSkillID = strings.TrimSpace(runtimeTool.SkillID)
		callToolName = strings.TrimSpace(runtimeTool.Definition.Function.Name)
		failedCallKey = ""
	}
	callEvidence[runtimeStateAllowPlanUpdateKey] = true
	callEvidence[runtimeStateAllowIntermediateAnswerKey] = !fileDeliveryRequiresArtifactOnly(req, callEvidence)
	callEvidence[runtimeStateNativeAgentLoopKey] = true
	callEvidence[runtimeStateKnowledgeRetrievalHistoryKey] = knowledgeRetrievalHistoryForToolCalls(s.successfulToolCalls)
	if isAgentKnowledgeRetrievalCall(callSkillID, callToolName) {
		callEvidence["knowledge_retrieval_round"] = s.skillToolCallCounts[callSkillID] + 1
		callEvidence["knowledge_retrieval_budget"] = maxBusinessToolCallsForSkill(resolved, callSkillID)
	}
	result := skillStepResult{}
	if req.NativeSkillSession != nil {
		if control, handled := r.handleNativeSessionControlCall(ctx, call, req.NativeSkillSession, resolved, req.RuntimeTools); handled {
			result = control.step
			if len(control.instructions) > 0 {
				rs.deferredSystemMessages = append(rs.deferredSystemMessages, control.instructions...)
			}
			r.refreshNativeSkills(s)
		}
	}
	if nativeForbiddenProtocolTool(call.Function.Name) {
		result = nativeForbiddenProtocolToolStep(call.ID, call.Function.Name)
	}
	if req.UnifiedSkillDispatch && strings.TrimSpace(call.Function.Name) == skills.UnifiedSkillToolName {
		result = unifiedSkillCallErrorStep(call.ID)
	}
	if result.trace.Kind == "" && runtimeCall {
		result = r.callRuntimeTool(ctx, call, runtimeTool)
		if strings.TrimSpace(result.trace.InvocationID) == "" {
			result.trace.InvocationID = call.ID
		}
		r.emitEvent(prepared, EventSkillCallEnd, skillCallEndPayload(prepared, result.trace))
		for _, artifact := range result.artifacts {
			r.recordArtifact(artifact)
			r.emitEvent(prepared, EventSkillArtifactCreated, artifact)
		}
		for _, event := range result.events {
			r.emitEvent(prepared, event.Type, runtimeToolEventPayload(prepared, event.Payload))
		}
	}
	if result.trace.Kind == "" {
		result = guardSkillToolCall(ctx, req, resolved, executionCall.ID, callSkillID, callToolName, callToolArgs, failedCallKey, s.failedToolCallReasons, s.failedToolCallAttemptCounts)
	}
	if result.trace.Kind == "" {
		result = r.handleProgressiveSkillCall(ctx, prepared, resolved, executionCall, req.ExecutionContext, s.toolCallCount, s.skillToolCallCounts, s.loadedSkills, callEvidence, round+1, nil)
	}
	return result, failedCallKey
}

// recordToolCallResult adds a call's result to the conversation and the call
// history. A non-nil outcome ends the run: a fatal failure, a pause for the
// user or an approval, or a terminal call. The calls after it in the round
// are answered as cancelled.
func (r *Runner) recordToolCallResult(ctx context.Context, s *runState, rs *roundState, result skillStepResult, failedCallKey string, rest []adapter.ToolCall) *runOutcome {
	prepared := s.prepared
	if strings.TrimSpace(result.trace.Kind) == "" {
		if result.usedSkill {
			s.skillUsed = true
		}
		if result.usedTool {
			s.toolCallCount++
			incrementSkillToolCallCount(s.skillToolCallCounts, result.trace.SkillID)
		}
		if result.toolMessage.Role != "" || result.toolMessage.ToolCallID != "" || result.toolMessage.Content != nil {
			s.messages = append(s.messages, result.toolMessage)
			if checkpointErr := r.checkpointContext(ctx, s.messages); checkpointErr != nil {
				return s.stop(checkpointErr)
			}
		}
		return nil
	}
	if result.recoverable {
		annotateRecoverableResult(s, rs, &result, failedCallKey)
	}
	r.appendTrace(ctx, s, result.trace)
	if result.recoverable &&
		failedCallKey != "" &&
		strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
		shouldRememberFailedToolCall(result.trace) {
		if _, alreadyRemembered := s.failedToolCallReasons[failedCallKey]; !alreadyRemembered {
			s.failedToolCallReasons[failedCallKey] = strings.TrimSpace(result.trace.Error)
			if s.failedToolCallReasons[failedCallKey] == "" {
				s.failedToolCallReasons[failedCallKey] = "previous tool call with the same arguments failed"
			}
		}
	}
	if result.recoverable {
		failureCategory := recoverableSkillFailureCategory(result.trace)
		failureClass := recoverableSkillFailureClass(result.trace)
		s.retryFailureBudgets.recordCall(failureClass, failureCategory, result.trace)
		rs.failureClasses[failureClass] = true
		if failureClass == retryFailureProviderBusiness {
			r.emitSkillError(ctx, prepared, result.trace)
		}
		rs.hadRecoverableFailure = true
		s.latestRecoverableTrace = result.trace
	} else {
		rs.hadSuccess = true
	}
	if result.fatalErr != nil {
		if !result.recoverable {
			r.emitSkillError(ctx, prepared, result.trace)
		}
		if result.toolMessage.Role != "" || result.toolMessage.ToolCallID != "" || result.toolMessage.Content != nil {
			s.messages = append(s.messages, result.toolMessage)
		}
		return r.pauseRound(ctx, s, rest, "the round stopped after a fatal sibling tool failure", result.fatalErr)
	}
	s.messages = append(s.messages, result.toolMessage)
	if projected, stats := projectMaterializedFileContent(s.messages, result.toolMessage.ToolCallID, result.toolResult); stats.removedRunes > 0 {
		s.messages = projected
		r.diagnostics.projectedRefs = appendUniqueProjectionRefs(r.diagnostics.projectedRefs, stats.refs...)
		r.diagnostics.projectedChars += stats.removedRunes
	}
	if checkpointErr := r.checkpointContext(ctx, s.messages); checkpointErr != nil {
		return s.stop(checkpointErr)
	}
	if result.usedSkill {
		s.skillUsed = true
	}
	if result.usedTool {
		s.toolCallCount++
		incrementSkillToolCallCount(s.skillToolCallCounts, result.trace.SkillID)
		if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
			strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
			s.successfulToolCalls = append(s.successfulToolCalls, SkillToolCallRef{
				ConnectorNative: result.trace.ConnectorNative,
				SkillID:         strings.TrimSpace(result.trace.SkillID),
				ToolName:        strings.TrimSpace(result.trace.ToolName),
				Arguments:       copyStringAnyMap(result.trace.Arguments),
				Result:          copyStringAnyMap(result.toolResult),
			})
		}
	}
	if result.pendingApproval != nil {
		return r.pauseRound(ctx, s, rest, "the round paused for approval before this sibling tool could run", &WorkflowApprovalPendingError{Payload: result.pendingApproval})
	}
	if result.pendingQuestion != nil {
		return r.pauseRound(ctx, s, rest, "the round paused for a workflow answer before this sibling tool could run", &WorkflowQuestionPendingError{Payload: result.pendingQuestion})
	}
	if result.pendingGovernance != nil {
		return r.pauseRound(ctx, s, rest, "the round paused for governance before this sibling tool could run", &ToolGovernancePendingError{Payload: result.pendingGovernance})
	}
	if result.pendingConnector != nil {
		return r.pauseRound(ctx, s, rest, "the round paused for Connector confirmation before this sibling tool could run", &ConnectorConfirmationPendingError{Payload: result.pendingConnector})
	}
	if result.pendingClientAction != nil {
		return r.pauseRound(ctx, s, rest, "the round paused for a client action before this sibling tool could run", &ClientActionPendingError{Payload: result.pendingClientAction})
	}
	if result.answer != "" {
		appendAnswerText(&s.answerBuilder, result.answer)
		if !result.answerStreamed {
			r.emitAnswerChunk(ctx, prepared, result.answer, nil)
		}
	}
	if result.pendingUserInput != nil {
		r.logUserInputRequested(ctx, s)
		return r.pauseRound(ctx, s, rest, "the round paused for user input before this sibling tool could run", &UserInputPendingError{Payload: result.pendingUserInput})
	}
	if result.terminal {
		r.logUserInputRequested(ctx, s)
		return r.pauseRound(ctx, s, rest, "the round ended before this sibling tool could run", nil)
	}
	if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "skill_load") &&
		strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
		r.diagnostics.loadedSkillIDs = appendUniqueProjectionRefs(r.diagnostics.loadedSkillIDs, result.trace.SkillID)
	}
	if message, ok := governedReadFileTargetSystemMessage(result.trace); ok {
		rs.deferredSystemMessages = append(rs.deferredSystemMessages, message)
	}
	if message, ok := nativeReferenceReadContinuationSystemMessage(result.trace); ok {
		rs.deferredSystemMessages = append(rs.deferredSystemMessages, message)
	}
	if result.stopBusinessLoop {
		rs.stopBusinessLoop = true
		rs.stopBusinessLoopTrace = result.trace
	}
	return nil
}

// annotateRecoverableResult records on a recoverable failure's trace how it
// is classified and budgeted, and whether it made progress over the previous
// failure of its class.
func annotateRecoverableResult(s *runState, rs *roundState, result *skillStepResult, failedCallKey string) {
	if result.trace.Arguments == nil {
		result.trace.Arguments = map[string]interface{}{}
	}
	result.trace.Arguments["failure_category"] = recoverableSkillFailureCategory(result.trace)
	failureClass := recoverableSkillFailureClass(result.trace)
	result.trace.Arguments["retry_failure_class"] = string(failureClass)
	budgetClass := recoverableSkillFailureBudget(result.trace)
	result.trace.Arguments["failure_budget"] = string(budgetClass)
	annotateRecoverableSkillFailure(
		&result.trace,
		failedCallKey,
		s.failedToolCallAttemptCounts,
		result.stopBusinessLoop,
	)
	classMetrics := s.retryFailureBudgets.metricsFor(failureClass)
	previous := skills.SkillTrace{}
	if classMetrics != nil {
		previous = classMetrics.latestTrace
	}
	if recoverableSkillFailureMadeProgress(previous, result.trace) {
		rs.failureProgress[failureClass] = true
	} else if failureClass == retryFailureProviderBusiness &&
		repeatedProviderValidationFailure(previous, result.trace) {
		// Permit one corrected retry after provider field-validation
		// feedback. If the provider reports the same structural failure
		// again, further argument variation is not evidence of progress.
		result.stopBusinessLoop = true
	}
}

func (r *Runner) logUserInputRequested(ctx context.Context, s *runState) {
	logger.DebugContext(ctx, "aichat skill planning requested user input",
		"conversation_id", s.prepared.Conversation.ID.String(),
		"message_id", s.prepared.Message.ID.String(),
		"skill_step_count", s.stepCount,
		"tool_call_count", s.toolCallCount,
	)
}

// pauseRound ends the run in the middle of a round: the calls not yet run
// are answered as cancelled for the given reason before the context is
// checkpointed.
func (r *Runner) pauseRound(ctx context.Context, s *runState, rest []adapter.ToolCall, reason string, err error) *runOutcome {
	s.messages = appendCancelledSiblingToolResults(s.messages, rest, reason)
	if checkpointErr := r.checkpointContext(ctx, s.messages); checkpointErr != nil {
		return s.stop(checkpointErr)
	}
	return s.stop(err)
}

// finishToolRound adds the round's deferred instructions, stops a business
// loop that repeated a failure without progress, and charges the round to
// the retry budgets.
func (r *Runner) finishToolRound(ctx context.Context, s *runState, rs *roundState, round int) *runOutcome {
	deferred := withoutRemindersInHistory(s.messages, rs.deferredSystemMessages)
	if len(deferred) > 0 {
		s.messages = append(s.messages, deferred...)
		if checkpointErr := r.checkpointContext(ctx, s.messages); checkpointErr != nil {
			return s.stop(checkpointErr)
		}
	}
	if rs.stopBusinessLoop && !rs.hadSuccess && !rs.failureProgress[recoverableSkillFailureClass(rs.stopBusinessLoopTrace)] {
		failureReason := "the model repeated the same failed business-tool call without changing its arguments"
		if strings.EqualFold(
			strings.TrimSpace(evidenceStringFromAny(rs.stopBusinessLoopTrace.Arguments["recovery_kind"])),
			"provider_validation",
		) {
			failureReason = "the provider rejected the corrected request with the same field-validation error"
		}
		return r.terminateForSafety(ctx, s, rs.stopBusinessLoopTrace, failureReason, round)
	}
	s.retryFailureBudgets.finishRound(rs.failureClasses, rs.failureProgress, rs.hadSuccess)
	if rs.hadRecoverableFailure {
		logger.DebugContext(ctx, "aichat skill recoverable failures observed",
			"conversation_id", s.prepared.Conversation.ID.String(),
			"message_id", s.prepared.Message.ID.String(),
			"retry_failure_budgets", s.retryFailureBudgets.snapshot(),
		)
		if class, metrics, reached := s.retryFailureBudgets.limitReached(); reached {
			return r.completeRetryFailureLimit(ctx, s, class, metrics, round)
		}
	}
	return nil
}

// terminateForSafety ends the run with an explanation of why the loop
// stopped before the requested operation completed.
func (r *Runner) terminateForSafety(ctx context.Context, s *runState, failure skills.SkillTrace, stopReason string, round int) *runOutcome {
	answer, explanationUsage, explanationErr := r.completeSkillLoopSafetyTermination(
		ctx,
		s.req,
		s.prepared,
		s.messages,
		s.traces,
		failure,
		s.successfulToolCalls,
		stopReason,
		round,
	)
	s.usage = mergeUsage(s.usage, explanationUsage)
	if explanationErr != nil {
		return s.stop(explanationErr)
	}
	return &runOutcome{answer: answer, usage: s.usage}
}

func (r *Runner) completeRetryFailureLimit(ctx context.Context, s *runState, class retryFailureClass, metrics *retryFailureBudgetMetrics, round int) *runOutcome {
	limitErr := fmt.Errorf("%w: too many %s failures", ErrInvalidInput, class)
	failure := skills.SkillTrace{}
	if metrics != nil {
		failure = metrics.latestTrace
	}
	if strings.TrimSpace(failure.Kind) == "" {
		failure = failedSkillTrace(string(class), "", limitErr)
		failure.Arguments = map[string]interface{}{}
	}
	reasonCode, stopReason, emitSkillError := retryFailureLimitDetails(class)
	trace := failedSkillTrace("tool_retry_guard", failure.ToolName, limitErr)
	trace.SkillID = failure.SkillID
	trace.Arguments = copyStringAnyMap(failure.Arguments)
	if trace.Arguments == nil {
		trace.Arguments = map[string]interface{}{}
	}
	trace.Arguments["reason_code"] = reasonCode
	trace.Arguments["retry_failure_class"] = string(class)
	trace.Arguments["retry_failure_class_counts"] = copyStringIntMap(s.retryFailureBudgets.classCallCounts())
	trace.Arguments["retry_failure_budgets"] = s.retryFailureBudgets.snapshot()
	trace.Arguments["termination_reason"] = "safe_retry_limit_reached"
	if providerMetrics := s.retryFailureBudgets.metricsFor(retryFailureProviderBusiness); providerMetrics != nil {
		trace.Arguments["failure_categories"] = copyStringIntMap(providerMetrics.categories)
	}
	if plannerMetrics := s.retryFailureBudgets.metricsFor(retryFailurePlannerSchema); plannerMetrics != nil {
		trace.Arguments["planner_feedback_categories"] = copyStringIntMap(plannerMetrics.categories)
	}
	r.appendTrace(ctx, s, trace)
	if emitSkillError {
		r.emitSkillError(ctx, s.prepared, trace)
	}
	return r.terminateForSafety(ctx, s, failure, stopReason, round)
}

// planningRoundsExhausted ends a run that used every planning round.
func (r *Runner) planningRoundsExhausted(ctx context.Context, s *runState) *runOutcome {
	err := fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
	trace := s.latestRecoverableTrace
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
	trace.Arguments["retry_failure_class_counts"] = copyStringIntMap(s.retryFailureBudgets.classCallCounts())
	trace.Arguments["retry_failure_budgets"] = s.retryFailureBudgets.snapshot()
	r.appendTrace(ctx, s, trace)
	r.emitSkillError(ctx, s.prepared, trace)
	return r.terminateForSafety(ctx, s, trace, "the planning loop reached its safety limit before the requested operation completed", defaultMaxSkillPlanningRounds)
}
