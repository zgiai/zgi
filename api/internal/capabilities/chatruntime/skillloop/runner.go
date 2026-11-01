package skillloop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ContextArtifactToolName remains reserved even on runtimes that do not expose
// the optional oversized-result receipt reader, so projected business Actions
// cannot claim a protocol name used by compatible runtimes.
const ContextArtifactToolName = "read_context_artifact"

const (
	defaultMaxSkillPlanningRounds                        = 50
	defaultMaxSkillStepsPerTurn                          = 160
	defaultMaxBusinessToolCallsPerSkill                  = 20
	defaultMaxRecoverableFailureRounds                   = 12
	defaultMaxConsecutiveRecoverableFailureRounds        = 5
	defaultMaxPlannerSchemaFailureRounds                 = 12
	defaultMaxConsecutivePlannerSchemaFailureRounds      = 5
	defaultMaxArgumentValidationFailureRounds            = 12
	defaultMaxConsecutiveArgumentValidationFailureRounds = 5
	defaultMaxProviderBusinessFailureRounds              = defaultMaxRecoverableFailureRounds
	defaultMaxConsecutiveProviderBusinessFailureRounds   = 5
	defaultMaxTerminalProtocolFailureRounds              = 12
	defaultMaxConsecutiveTerminalProtocolFailureRounds   = 5
	// Kept as a source-compatible alias for existing diagnostics and tests.
	defaultMaxConsecutivePlannerFeedbackRounds   = defaultMaxConsecutivePlannerSchemaFailureRounds
	intermediateAnswerChunkRunes                 = 180
	streamedIntermediateAnswerArg                = "_aichat_streamed_answer"
	streamedFinalAnswerArg                       = "_aichat_streamed_final_answer"
	runtimeStateAllowPlanUpdateKey               = "_skill_loop_allow_plan_update"
	runtimeStateAllowIntermediateAnswerKey       = "_skill_loop_allow_intermediate_answer"
	runtimeStateNativeAgentLoopKey               = "_native_agent_loop"
	userInputContinuationAnswered                = "answered"
	userInputContinuationReplan                  = "replan_after_user_input"
	restoredSkillInstructionsTotalBudgetChars    = 16000
	restoredSkillInstructionsPerSkillBudgetChars = 10000
	connectorOutcomeRecoveryReason               = "connector_outcomes_recovered"
)

type recoverableFailureBudgetClass string

const (
	recoverableFailureBudgetBusiness recoverableFailureBudgetClass = "business"
	recoverableFailureBudgetPlanner  recoverableFailureBudgetClass = "planner"
)

type retryFailureClass string

const (
	retryFailurePlannerSchema      retryFailureClass = "planner_schema"
	retryFailureArgumentValidation retryFailureClass = "argument_validation"
	retryFailureProviderBusiness   retryFailureClass = "provider_business"
	retryFailureTerminalProtocol   retryFailureClass = "terminal_protocol"
)

var retryFailureClasses = [...]retryFailureClass{
	retryFailureProviderBusiness,
	retryFailureArgumentValidation,
	retryFailurePlannerSchema,
	retryFailureTerminalProtocol,
}

type retryFailureBudgetLimit struct {
	maxRounds            int
	maxConsecutiveRounds int
}

type retryFailureBudgetMetrics struct {
	roundCount            int
	consecutiveRoundCount int
	callCount             int
	categories            map[string]int
	latestTrace           skills.SkillTrace
}

type retryFailureBudgetState struct {
	metrics map[retryFailureClass]*retryFailureBudgetMetrics
}

func newRetryFailureBudgetState() *retryFailureBudgetState {
	state := &retryFailureBudgetState{metrics: make(map[retryFailureClass]*retryFailureBudgetMetrics, len(retryFailureClasses))}
	for _, class := range retryFailureClasses {
		state.metrics[class] = &retryFailureBudgetMetrics{categories: map[string]int{}}
	}
	return state
}

func retryFailureBudgetLimitForClass(class retryFailureClass) retryFailureBudgetLimit {
	switch class {
	case retryFailurePlannerSchema:
		return retryFailureBudgetLimit{
			maxRounds:            defaultMaxPlannerSchemaFailureRounds,
			maxConsecutiveRounds: defaultMaxConsecutivePlannerSchemaFailureRounds,
		}
	case retryFailureArgumentValidation:
		return retryFailureBudgetLimit{
			maxRounds:            defaultMaxArgumentValidationFailureRounds,
			maxConsecutiveRounds: defaultMaxConsecutiveArgumentValidationFailureRounds,
		}
	case retryFailureTerminalProtocol:
		return retryFailureBudgetLimit{
			maxRounds:            defaultMaxTerminalProtocolFailureRounds,
			maxConsecutiveRounds: defaultMaxConsecutiveTerminalProtocolFailureRounds,
		}
	default:
		return retryFailureBudgetLimit{
			maxRounds:            defaultMaxProviderBusinessFailureRounds,
			maxConsecutiveRounds: defaultMaxConsecutiveProviderBusinessFailureRounds,
		}
	}
}

func (s *retryFailureBudgetState) metricsFor(class retryFailureClass) *retryFailureBudgetMetrics {
	if s == nil {
		return nil
	}
	if s.metrics == nil {
		s.metrics = map[retryFailureClass]*retryFailureBudgetMetrics{}
	}
	metrics := s.metrics[class]
	if metrics == nil {
		metrics = &retryFailureBudgetMetrics{categories: map[string]int{}}
		s.metrics[class] = metrics
	}
	if metrics.categories == nil {
		metrics.categories = map[string]int{}
	}
	return metrics
}

func (s *retryFailureBudgetState) recordCall(class retryFailureClass, category string, trace skills.SkillTrace) {
	metrics := s.metricsFor(class)
	if metrics == nil {
		return
	}
	metrics.callCount++
	category = strings.TrimSpace(category)
	if category == "" {
		category = "tool"
	}
	metrics.categories[category]++
	metrics.latestTrace = trace
}

func (s *retryFailureBudgetState) finishRound(
	observed map[retryFailureClass]bool,
	progress map[retryFailureClass]bool,
	roundHadSuccess bool,
) {
	for _, class := range retryFailureClasses {
		metrics := s.metricsFor(class)
		if metrics == nil {
			continue
		}
		if !observed[class] {
			metrics.consecutiveRoundCount = 0
			continue
		}
		metrics.roundCount++
		if roundHadSuccess || progress[class] {
			metrics.consecutiveRoundCount = 0
			continue
		}
		metrics.consecutiveRoundCount++
	}
}

func (s *retryFailureBudgetState) limitReached() (retryFailureClass, *retryFailureBudgetMetrics, bool) {
	for _, class := range retryFailureClasses {
		metrics := s.metricsFor(class)
		limit := retryFailureBudgetLimitForClass(class)
		if metrics != nil &&
			(metrics.roundCount >= limit.maxRounds || metrics.consecutiveRoundCount >= limit.maxConsecutiveRounds) {
			return class, metrics, true
		}
	}
	return "", nil, false
}

func (s *retryFailureBudgetState) classCallCounts() map[string]int {
	out := make(map[string]int, len(retryFailureClasses))
	for _, class := range retryFailureClasses {
		if metrics := s.metricsFor(class); metrics != nil && metrics.callCount > 0 {
			out[string(class)] = metrics.callCount
		}
	}
	return out
}

func (s *retryFailureBudgetState) snapshot() map[string]interface{} {
	out := make(map[string]interface{}, len(retryFailureClasses))
	for _, class := range retryFailureClasses {
		metrics := s.metricsFor(class)
		if metrics == nil {
			continue
		}
		limit := retryFailureBudgetLimitForClass(class)
		out[string(class)] = map[string]interface{}{
			"round_count":             metrics.roundCount,
			"consecutive_round_count": metrics.consecutiveRoundCount,
			"call_count":              metrics.callCount,
			"max_rounds":              limit.maxRounds,
			"max_consecutive_rounds":  limit.maxConsecutiveRounds,
			"failure_categories":      copyStringIntMap(metrics.categories),
		}
	}
	return out
}

func retryFailureLimitDetails(class retryFailureClass) (reasonCode string, stopReason string, emitSkillError bool) {
	switch class {
	case retryFailurePlannerSchema:
		return "planner_feedback_limit_reached", "the planner could not produce a valid next step before its retry limit was reached", false
	case retryFailureArgumentValidation:
		return "argument_validation_limit_reached", "tool arguments could not be validated before their independent retry limit was reached", false
	case retryFailureTerminalProtocol:
		return "terminal_protocol_limit_reached", "the final-answer protocol could not be reconciled before its independent retry limit was reached", false
	default:
		return "skill_tool_failure_limit_reached", "the Provider business operation continued to fail and its safe retry limit was reached", true
	}
}

func (r *Runner) completeSkillLoopSafetyTermination(
	ctx context.Context,
	req RunRequest,
	prepared *PreparedChat,
	messages []adapter.Message,
	traces []skills.SkillTrace,
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
	round int,
) (string, *adapter.Usage, error) {
	runtimeState := runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
	// Connector calls that all succeeded show the work that ran, not that the
	// task is done: a checklist with open steps says more was planned.
	if connectorExecutedOutcomesSatisfied(runtimeState) && len(planOpenSteps(r.planSteps)) == 0 {
		return r.completeConnectorOutcomeSuccess(
			ctx,
			req,
			prepared,
			messages,
			traces,
			runtimeState,
			stopReason,
			round,
		)
	}
	return r.completeBusinessToolFailure(
		ctx,
		req,
		prepared,
		traces,
		failure,
		successfulToolCalls,
		stopReason,
		round,
	)
}

func (r *Runner) completeConnectorOutcomeSuccess(
	ctx context.Context,
	req RunRequest,
	prepared *PreparedChat,
	messages []adapter.Message,
	traces []skills.SkillTrace,
	runtimeState map[string]interface{},
	stopReason string,
	round int,
) (string, *adapter.Usage, error) {
	request := cloneChatRequest(prepared.LLMRequest)
	request.Messages = append([]adapter.Message(nil), messages...)
	if evidence := terminalOnlyEvidenceSnapshot(runtimeState); len(evidence) > 0 {
		if encoded, err := json.Marshal(evidence); err == nil {
			request.Messages = append(request.Messages, adapter.Message{
				Role:    "system",
				Content: "Authoritative native Connector completion evidence JSON:\n" + string(encoded),
			})
		}
	}
	request.Messages = append(request.Messages, adapter.Message{
		Role: "system",
		Content: strings.Join([]string{
			"The runtime has verified that every required external outcome is supported by an exact native Connector execution.",
			"The loop stopped only because later planning or bookkeeping could not make further progress; that internal failure does not undo the completed provider operation.",
			"Write the normal final user-facing answer from the successful tool results already present in this conversation.",
			"Report only results supported by those tool results. Do not mention planning, bookkeeping, safety limits, schemas, or this recovery instruction.",
			"Do not call tools or claim any partial, failed, outcome_unknown, or still-pending operation succeeded.",
		}, "\n"),
	})
	request.Tools = nil
	request.ToolChoice = nil
	request.Functions = nil
	request.FunctionCall = nil
	request.ResponseFormat = nil
	request.StreamOptions = nil

	result, err := r.runTerminalOnlyPlanningWithRetry(
		ctx,
		prepared,
		request,
		round,
		req.OnChunk,
		true,
		false,
	)
	if err != nil {
		return "", result.usage, err
	}
	answer := strings.TrimSpace(terminalOnlyAnswerCandidate(result.message))
	if answer == "" {
		return "", result.usage, terminalFinalAnswerUnavailableError(errors.New("native Connector completion summary was empty"))
	}

	trace := skills.SkillTrace{
		Kind:    "final_answer",
		Title:   "Final answer",
		Message: answer,
		Status:  "success",
		Arguments: map[string]interface{}{
			"completion_reason": connectorOutcomeRecoveryReason,
			"round":             round,
		},
		Result: map[string]interface{}{
			"source": "connector_outcome_recovery",
		},
	}
	traces = append(traces, trace)
	r.recordTrace(traces, trace)
	r.logSkillTrace(ctx, prepared, trace)
	if !result.answerStreamed {
		r.emitAnswerChunk(ctx, prepared, answer, nil)
	}
	if req.OnTerminalCompletion != nil {
		req.OnTerminalCompletion(TerminalCompletionResult{
			Status:           "pass",
			Source:           "connector_outcome_recovery",
			Reason:           strings.TrimSpace(stopReason),
			CompletionReason: connectorOutcomeRecoveryReason,
		})
	}
	return answer, result.usage, nil
}

// guardSkillToolCall stops a skill tool call before it runs, with a
// recoverable result the model can act on: the skill is not authorized, or
// the same call already failed. It returns an empty step when the call may run.
func guardSkillToolCall(ctx context.Context, req RunRequest, resolved *skills.ResolvedSkills, callID string, skillID string, toolName string, args map[string]interface{}, failedCallKey string, failedReasons map[string]string, failedAttempts map[string]int) skillStepResult {
	if req.AuthorizeSkillStep != nil && strings.TrimSpace(skillID) != "" {
		if allowed, policyErr := req.AuthorizeSkillStep(ctx, skillID); policyErr != nil || !allowed {
			return unavailableSkillPolicyStep(callID, skillID, toolName, args, policyErr, resolved)
		}
	}
	if failedCallKey != "" {
		if reason := failedReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
			return repeatedFailedToolCallRecoverableStep(callID, skillID, toolName, args, reason, failedAttempts[failedCallKey] >= 2, resolved)
		}
	}
	return skillStepResult{}
}

// initialRunMessages builds the first request of a run: the terminal-only
// projection, or the native loop with its cached system prompt and turn
// guidance.
func initialRunMessages(req RunRequest, resolved *skills.ResolvedSkills, nativeToolSet *skills.NativeToolSet) []adapter.Message {
	prepared := req.Prepared
	if req.TerminalOnly {
		messages := terminalOnlyProjectedMessages(prepared, currentMetadataForRun(req))
		return append([]adapter.Message{terminalOnlySystemMessage()}, messages...)
	}
	return nativeInitialMessages(req, resolved, nativeToolSet, append([]adapter.Message{}, prepared.LLMRequest.Messages...))
}

// nativeInitialMessages adds the native loop's guidance: what stays the same
// from turn to turn goes into the system prompt, and the turn's own guidance
// follows the question (ADR 0032, ADR 0043).
func nativeInitialMessages(req RunRequest, resolved *skills.ResolvedSkills, nativeToolSet *skills.NativeToolSet, messages []adapter.Message) []adapter.Message {
	systemAdditions := validAdditionalSystemMessages(req.SystemPromptAdditions)
	rankedCatalog := false
	if req.NativeSkillSession != nil {
		if candidateMessage := req.NativeSkillSession.CatalogMessage(); strings.TrimSpace(messageContent(candidateMessage.Content)) != "" {
			// A directory that lists every candidate in a fixed order is the
			// same in every turn, so it belongs to the cached system prompt;
			// one ranked for this question stays with the turn.
			if req.NativeSkillSession.CatalogStable() {
				systemAdditions = append(systemAdditions, candidateMessage)
			} else {
				messages = append(messages, candidateMessage)
				rankedCatalog = true
			}
		}
	}
	tail := make([]adapter.Message, 0, 4)
	if nativeToolSet != nil {
		tail = append(tail, nativeToolSet.InstructionMessages...)
		if sessionScripts, ok := nativeActiveSessionScriptsSystemMessage(resolved, nativeToolSet.ActiveSkillIDs, req.RuntimeTools); ok {
			tail = append(tail, sessionScripts)
		}
	}
	tail = append(tail, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
	messages = append(messages, withoutRemindersInHistory(messages, tail)...)
	// A ranked directory changes with every question, so a turn that sends
	// one is not stored with it; its history would grow by a directory a turn.
	if req.OnTurnUserContent != nil && !rankedCatalog {
		if content, ok := placedTurnUserContent(messages); ok {
			req.OnTurnUserContent(content)
		}
	}
	return withSystemPromptAdditions(messages, append(systemAdditions, nativeAgentLoopSystemMessage()))
}

// planningToolsForRound is the tool list of one round before positions are
// kept. It stays the same through the run: a finished connector call does not
// take update_plan away, because the checklist is the model's (ADR 0049).
func (r *Runner) planningToolsForRound(req RunRequest, resolved *skills.ResolvedSkills, nativeToolSet *skills.NativeToolSet) []adapter.Tool {
	tools := nativeAgentToolsForRun(resolved, nativeToolSet, req.NativeSkillSession, req.RuntimeTools, req.OmitPlanningTools, req.UnifiedSkillDispatch)
	// Oversized tool results reach the model projected, with the rest
	// readable through this tool (ADR 0037). It is always offered, so
	// the tool list does not change when a projection first appears.
	return appendContextArtifactTool(tools, r.ContextManager != nil)
}

// runPlanningRound makes the model call of one round, with the native loop's
// retry and output-continuation policy.
func (r *Runner) runPlanningRound(ctx context.Context, req RunRequest, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, deferTerminalContent bool, terminalSubmissionAllowed bool, suppressNaturalProgress bool) (planningResult, error) {
	result, err := r.runNativeAgentRoundContinuing(ctx, prepared, planningReq, round, req.OnChunk, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress, req.PlanningOutputTokenLimit)
	return result, nativeAgentOutputError(err)
}

// connectorExecutedOutcomesSatisfied reports that the turn ran connector
// operations and every one reached a successful native outcome, with nothing
// left awaiting the user (ADR 0049: acceptance follows the operations that
// ran, not operations a plan declared).
func connectorExecutedOutcomesSatisfied(runtimeState map[string]interface{}) bool {
	if terminalStateGuardPendingProtocolBlocker(runtimeState) != "" || connectorTerminalStateDecision(runtimeState, "") != nil {
		return false
	}
	native := false
	for _, invocation := range terminalStateGuardInvocations(runtimeState) {
		evidence, ok := connectortools.Evidence(invocation)
		if !ok || !evidence.Operation.Valid() {
			continue
		}
		if evidence.Status != "successful" {
			return false
		}
		native = true
	}
	return native
}

type skillStepResult struct {
	trace               skills.SkillTrace
	toolMessage         adapter.Message
	toolResult          map[string]interface{}
	artifacts           []map[string]interface{}
	events              []RuntimeToolEvent
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
	pendingConnector    map[string]interface{}
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

// keepToolPositions orders tools by the round in which each first appeared,
// so a tool added during the turn, such as one from a newly active skill,
// goes after the tools already sent. Providers render tools before the
// messages, and an insertion earlier in the list would invalidate the cached
// prefix of everything after it (ADR 0032).
func keepToolPositions(tools []adapter.Tool, positions map[string]int) []adapter.Tool {
	if len(tools) == 0 || positions == nil {
		return tools
	}
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if _, seen := positions[name]; !seen {
			positions[name] = len(positions)
		}
	}
	ordered := append([]adapter.Tool(nil), tools...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return positions[strings.TrimSpace(ordered[i].Function.Name)] < positions[strings.TrimSpace(ordered[j].Function.Name)]
	})
	return ordered
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
	if len(resolved.Skills) > 0 && r.SkillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	nativeToolSet := req.NativeToolSet
	if req.NativeSkillSession != nil {
		current := req.NativeSkillSession.ToolSet()
		nativeToolSet = &current
	}
	s := &runState{
		req:                          req,
		prepared:                     prepared,
		resolved:                     resolved,
		nativeToolSet:                nativeToolSet,
		loadedSkills:                 map[string]struct{}{},
		toolPositions:                map[string]int{},
		messages:                     initialRunMessages(req, resolved, nativeToolSet),
		traces:                       []skills.SkillTrace{},
		retryFailureBudgets:          newRetryFailureBudgetState(),
		failedToolCallAttemptCounts:  map[string]int{},
		skillToolCallCounts:          map[string]int{},
		successfulToolCalls:          []SkillToolCallRef{},
		failedToolCallReasons:        map[string]string{},
		maxSkillSteps:                maxSkillStepsForTurn(resolved),
		terminalStateGuardConfigured: req.RuntimeStateSnapshot != nil,
		nativeCommentary:             newNativeCommentaryState(prepared.LLMRequest.Model, currentMetadataForRun(req)),
		planSteps:                    planChecklistFromMetadata(currentMetadataForRun(req)),
	}
	r.planSteps = s.planSteps
	if req.NativeSkillSession != nil {
		for _, skillID := range req.NativeSkillSession.ActiveSkillIDs() {
			s.loadedSkills[strings.ToLower(strings.TrimSpace(skillID))] = struct{}{}
		}
		for _, attempt := range req.NativeSkillSession.ActivationAttempts() {
			r.appendTrace(ctx, s, nativeSessionActivationAttemptTrace(attempt))
		}
	} else {
		loadedSkills, preloadErr := r.preloadNativeSkills(ctx, prepared, resolved, s.nativeToolSet, &s.traces, req.ExecutionContext)
		if preloadErr != nil {
			return "", nil, preloadErr
		}
		s.loadedSkills = loadedSkills
	}
	s.skillUsed = len(s.loadedSkills) > 0
	r.diagnostics = modelInvocationDiagnostics{
		activeSkillIDs:   activeSkillIDsForDiagnostics(resolved, s.loadedSkills),
		loadedSkillIDs:   activeSkillIDsForDiagnostics(resolved, s.loadedSkills),
		continuationType: strings.TrimSpace(req.ContinuationType),
		terminalOnly:     req.TerminalOnly,
	}
	platformServices := r.runtimePlatformServices(req, prepared, resolved, s.loadedSkills, func() *skills.NativeToolSet {
		if req.NativeSkillSession != nil {
			current := req.NativeSkillSession.ToolSet()
			return &current
		}
		return s.nativeToolSet
	}, &s.toolCallCount, s.skillToolCallCounts, &s.successfulToolCalls, &s.traces)
	for _, tool := range req.RuntimeTools {
		if tool.BindPlatform != nil {
			tool.BindPlatform(platformServices)
		}
	}
	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		if eventErr := prepared.presentationEventError(); eventErr != nil {
			return s.stop(eventErr).result()
		}
		if req.BeforeModelRound != nil {
			additional, err := req.BeforeModelRound(ctx, round)
			if err != nil {
				return s.stop(err).result()
			}
			if len(additional) > 0 {
				s.messages = append(s.messages, additional...)
			}
		}
		roundRuntimeState := map[string]interface{}{}
		terminalSubmissionAllowed := true
		if s.terminalStateGuardConfigured {
			roundRuntimeState = runtimeStateWithSuccessfulToolCalls(req, s.successfulToolCalls)
			terminalSubmissionAllowed = terminalStateGuardCanStream(roundRuntimeState)
		}
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = s.messages
		planningReq.Stream = false
		planningReq.Tools = r.planningToolsForRound(req, resolved, s.nativeToolSet)
		planningReq.Tools = keepToolPositions(planningReq.Tools, s.toolPositions)
		if req.TerminalOnly {
			planningReq.Tools = nil
			planningReq.ToolChoice = nil
		} else {
			planningReq.ToolChoice = "auto"
		}
		r.diagnostics.activeSkillIDs = activeSkillIDsForDiagnostics(resolved, s.loadedSkills)
		r.requestBudget = planningRequestBudgetForRun(req)

		suppressNaturalProgress := req.SuppressInitialNaturalProgress && round == 0
		deferTerminalContent := req.TerminalOnly || !terminalSubmissionAllowed
		planningResult, err := r.runPlanningRound(ctx, req, prepared, planningReq, round, deferTerminalContent, terminalSubmissionAllowed, suppressNaturalProgress)
		if req.AfterModelRound != nil {
			if hookErr := req.AfterModelRound(ctx, round, err == nil); hookErr != nil {
				if err != nil {
					return s.stop(errors.Join(err, hookErr)).result()
				}
				return s.stop(hookErr).result()
			}
		}
		s.usage = mergeUsage(s.usage, planningResult.usage)
		if len(planningResult.contextMessages) > 0 {
			s.messages = cloneMessagesForProvider(planningResult.contextMessages)
		}
		if eventErr := prepared.presentationEventError(); eventErr != nil {
			return s.stop(eventErr).result()
		}
		if err != nil {
			var streamedErr *streamedFinalAnswerError
			if errors.As(err, &streamedErr) {
				appendAnswerText(&s.answerBuilder, strings.TrimSpace(streamedErr.answer))
			}
			return s.stop(err).result()
		}
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		// The conversation keeps the model's own calls; the loop handles each
		// unified call as the native call it names (ADR 0044).
		modelToolCalls := toolCalls
		if req.UnifiedSkillDispatch {
			toolCalls = translateUnifiedSkillCalls(toolCalls, s.nativeToolSet, resolved)
		}
		text := assistantMessageText(planningMessage)
		if len(toolCalls) > 0 && strings.TrimSpace(text) != "" {
			candidate := planningResult.provisionalContent
			candidateStreamed := planningResult.provisionalStreamed
			if strings.TrimSpace(candidate) == "" && !suppressNaturalProgress && !deferTerminalContent {
				candidate = text
				r.emitAnswerChunk(ctx, prepared, candidate, nil)
				candidateStreamed = true
			}
			if candidateStreamed && strings.TrimSpace(candidate) != "" {
				decision := s.nativeCommentary.classify(candidate, toolCalls, s.nativeToolSet, req.RuntimeTools...)
				r.emitAnswerRetractWithDisposition(ctx, prepared, candidate, decision.disposition)
				r.logNativeCommentaryDecision(ctx, prepared, round, decision, planningResult.reasoningObserved)
				if eventErr := prepared.presentationEventError(); eventErr != nil {
					return s.stop(eventErr).result()
				}
			}
		}
		if len(toolCalls) == 0 && strings.TrimSpace(text) == "" {
			return s.stop(fmt.Errorf("%w: model returned neither a tool call nor assistant content", ErrAgentOutputTruncated)).result()
		}
		if req.TerminalOnly && len(toolCalls) > 0 {
			if checkpointErr := r.checkpointTerminalToolBatch(ctx, s.messages, planningMessage, toolCalls, "", adapter.Message{}, "terminal-only execution rejected an unexpected tool call"); checkpointErr != nil {
				return s.stop(checkpointErr).result()
			}
			return s.stop(errors.Join(
				ErrFinalAnswerUnavailable,
				errors.New("terminal-only model returned an unexpected tool call after retry"),
			)).result()
		}
		if req.TerminalOnly && strings.TrimSpace(text) == "" {
			return s.stop(errors.Join(
				ErrFinalAnswerUnavailable,
				errors.New("terminal-only model returned no final answer after retry"),
			)).result()
		}
		if len(toolCalls) == 0 && s.terminalStateGuardConfigured {
			if strings.TrimSpace(text) == "" {
				if req.TerminalOnly {
					return s.stop(errors.Join(
						ErrFinalAnswerUnavailable,
						errors.New("terminal-only model returned no final answer after retry"),
					)).result()
				}
				s.emptyFinalAnswerRetryCount++
				if s.emptyFinalAnswerRetryCount <= 1 {
					s.messages = append(s.messages, adapter.Message{
						Role:    "system",
						Content: "The previous assistant turn returned neither a tool call nor a user-visible answer. Continue from the latest context: call another tool if work remains, otherwise provide the final answer directly.",
					})
					continue
				}
				return s.stop(fmt.Errorf("%w: model returned no final answer", ErrInvalidInput)).result()
			}
			s.emptyFinalAnswerRetryCount = 0
			if r.remindOpenPlanSteps(ctx, s, text, planningResult.answerStreamed) {
				continue
			}
			guard := terminalStateGuardEvaluate(roundRuntimeState, text)
			terminalStateGuardRecord(req, guard)
			if guard.Path != terminalStateGuardAccepted {
				terminalStateGuardNotify(req, guard)
				return s.stop(terminalStateGuardError(guard)).result()
			}
			answer := strings.TrimSpace(firstNonEmptyString(guard.FinalAnswer, text))
			appendAnswerText(&s.answerBuilder, answer)
			if !planningResult.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, answer, nil)
			}
			terminalStateGuardNotify(req, guard)
			return s.stop(nil).result()
		}
		s.emptyFinalAnswerRetryCount = 0
		if len(toolCalls) == 0 && prepared.parts.SkillMode == "required" && !s.skillUsed {
			return s.stop(fmt.Errorf("%w: required skill was not used", ErrInvalidInput)).result()
		}
		if text != "" && len(toolCalls) == 0 && r.remindOpenPlanSteps(ctx, s, text, planningResult.answerStreamed) {
			continue
		}
		if text != "" && len(toolCalls) == 0 {
			appendAnswerText(&s.answerBuilder, text)
			if !planningResult.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, text, nil)
			}
		}
		if len(toolCalls) == 0 {
			logger.DebugContext(ctx, "aichat skill planning completed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"skill_step_count", s.stepCount,
				"tool_call_count", s.toolCallCount,
			)
			return s.stop(nil).result()
		}
		if s.stepCount+len(toolCalls) > s.maxSkillSteps {
			logger.WarnContext(ctx, "aichat skill step limit exceeded",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"current_step_count", s.stepCount,
				"requested_tool_calls", len(toolCalls),
				"max_steps", s.maxSkillSteps,
			)
			limitErr := fmt.Errorf("%w: too many skill steps", ErrInvalidInput)
			trace := failedSkillTrace("tool_retry_guard", "", limitErr)
			trace.Arguments = map[string]interface{}{
				"reason_code":   "skill_step_limit_exceeded",
				"current_steps": s.stepCount,
				"requested":     len(toolCalls),
				"max_steps":     s.maxSkillSteps,
			}
			r.appendTrace(ctx, s, trace)
			r.emitSkillError(ctx, prepared, trace)
			return r.terminateForSafety(ctx, s, trace, "the business-tool step safety limit was reached before the requested operation completed", round).result()
		}
		logger.DebugContext(ctx, "aichat skill planning requested tool calls",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"tool_call_count", len(toolCalls),
			"step_count", s.stepCount,
		)

		planningMessage.Role = "assistant"
		planningMessage.ToolCalls = modelToolCalls
		// Reasoning stays on tool-call rounds of this turn: thinking models
		// such as DeepSeek expect it back until the turn ends, and adapters
		// drop it for models that do not. Ordinary assistant content is kept
		// because it may hold plans and judgments the next round needs.
		s.messages = append(s.messages, planningMessage)

		if outcome := r.runToolCalls(ctx, s, round, toolCalls); outcome != nil {
			return outcome.result()
		}
	}
	return r.planningRoundsExhausted(ctx, s).result()
}

func skillInstructionDigest(instructions string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(instructions)))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func skillEffectiveVersion(doc *skills.SkillDocument) string {
	if doc == nil {
		return ""
	}
	if doc.Metadata.Source == skills.SkillSourceCustom && doc.Metadata.HasScripts {
		if digest, err := skills.SkillPackageSHA256(doc.Metadata.RootPath); err == nil {
			return "sha256:" + digest
		}
	}
	return skillInstructionDigest(doc.Instructions)
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

func unavailableSkillPolicyStep(callID string, skillID string, toolName string, args map[string]interface{}, policyErr error, resolved ...*skills.ResolvedSkills) skillStepResult {
	message := "skill is no longer enabled by the current organization policy"
	if policyErr != nil {
		message = "skill availability could not be verified against the current organization policy"
	}
	err := fmt.Errorf("%w: %s", ErrInvalidInput, message)
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Status = "blocked"
	trace.Arguments = summarizeSkillToolArgumentsForResolved(firstResolvedSkills(resolved), trace.SkillID, toolName, args)
	trace.Arguments["reason_code"] = "organization_skill_unavailable"
	nextAction := "Do not retry this skill in the current turn. Continue with another enabled skill, or answer truthfully that the requested operation was not executed."
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, trace.SkillID, toolName, resolved...)), false, false)
}

func currentMetadataForRun(req RunRequest) map[string]interface{} {
	if req.CurrentMetadata == nil {
		return nil
	}
	return copyStringAnyMap(req.CurrentMetadata())
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
	if strings.HasPrefix(reasonCode, "skill_tool_arguments_") ||
		strings.HasPrefix(reasonCode, "action_arguments_") {
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

func recoverableSkillFailureBudget(trace skills.SkillTrace) recoverableFailureBudgetClass {
	switch recoverableSkillFailureClass(trace) {
	case retryFailurePlannerSchema, retryFailureArgumentValidation, retryFailureTerminalProtocol:
		return recoverableFailureBudgetPlanner
	default:
		return recoverableFailureBudgetBusiness
	}
}

func recoverableSkillFailureClass(trace skills.SkillTrace) retryFailureClass {
	kind := strings.ToLower(strings.TrimSpace(trace.Kind))
	if kind == "terminal_protocol" || kind == "final_answer" {
		return retryFailureTerminalProtocol
	}
	providerRequestSent, providerRequestKnown := trace.Arguments["provider_request_sent"].(bool)
	if kind == "tool_call" && providerRequestKnown && providerRequestSent {
		return retryFailureProviderBusiness
	}
	switch kind {
	case "plan_update", "skill_load_attempt", "meta_tool":
		return retryFailurePlannerSchema
	case "planner_feedback":
		if recoverableSkillFailureCategory(trace) != "arguments" {
			return retryFailurePlannerSchema
		}
	}
	if recoverableSkillFailureCategory(trace) == "arguments" || (providerRequestKnown && !providerRequestSent) {
		return retryFailureArgumentValidation
	}
	if kind == "planner_feedback" {
		return retryFailurePlannerSchema
	}
	return retryFailureProviderBusiness
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
		"provider_error_code":  strings.ToLower(strings.TrimSpace(evidenceStringFromAny(trace.Arguments["provider_error_code"]))),
		"invalid_fields":       evidenceStringSliceFromAny(trace.Arguments["invalid_fields"]),
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
	if repeatedProviderValidationFailure(previous, current) {
		return false
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

func repeatedProviderValidationFailure(previous skills.SkillTrace, current skills.SkillTrace) bool {
	if !strings.EqualFold(
		strings.TrimSpace(evidenceStringFromAny(previous.Arguments["recovery_kind"])),
		"provider_validation",
	) || !strings.EqualFold(
		strings.TrimSpace(evidenceStringFromAny(current.Arguments["recovery_kind"])),
		"provider_validation",
	) {
		return false
	}
	previousCode := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(previous.Arguments["provider_error_code"])))
	currentCode := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(current.Arguments["provider_error_code"])))
	if previousCode == "" || previousCode != currentCode {
		return false
	}
	previousFields := evidenceStringSliceFromAny(previous.Arguments["invalid_fields"])
	currentFields := evidenceStringSliceFromAny(current.Arguments["invalid_fields"])
	if len(previousFields) != len(currentFields) {
		return false
	}
	for index := range previousFields {
		if !strings.EqualFold(strings.TrimSpace(previousFields[index]), strings.TrimSpace(currentFields[index])) {
			return false
		}
	}
	return true
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

func repeatedFailedToolCallRecoverableStep(callID string, skillID string, toolName string, args map[string]interface{}, reason string, stopBusinessLoop bool, resolved ...*skills.ResolvedSkills) skillStepResult {
	message := "same tool call with the same arguments already failed in this turn"
	if reason = strings.TrimSpace(reason); reason != "" {
		message += ": " + reason
	}
	retryAction := "Do not repeat the identical call. Change the arguments using the validation feedback before retrying."
	if stopBusinessLoop {
		retryAction = "stop retrying this business tool and explain the incomplete operation truthfully"
	}
	err := &skillToolArgumentsError{
		Code:              skillToolRetryNoProgressCode,
		SkillID:           strings.ToLower(strings.TrimSpace(skillID)),
		ToolName:          strings.TrimSpace(toolName),
		ExpectedType:      "object",
		ActualType:        "object",
		ExpectedArguments: skills.ExpectedSkillToolArgumentsForResolved(firstResolvedSkills(resolved), skillID, toolName),
		RetryAction:       retryAction,
		Cause:             fmt.Errorf("%w: %s", ErrInvalidInput, message),
	}
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Arguments = map[string]interface{}{}
	if len(args) > 0 {
		trace.Arguments = summarizeSkillToolArgumentsForResolved(firstResolvedSkills(resolved), skillID, toolName, args)
	}
	trace.Arguments["reason_code"] = skillToolRetryNoProgressCode
	result := recoverableSkillStep(
		trace,
		skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, err.RetryAction, skillID, toolName, resolved...)),
		false,
		false,
	)
	result.stopBusinessLoop = stopBusinessLoop
	return result
}

func firstResolvedSkills(resolved []*skills.ResolvedSkills) *skills.ResolvedSkills {
	if len(resolved) == 0 {
		return nil
	}
	return resolved[0]
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

// runModelToolRound calls the model for one round and retries a call that
// failed with a transient error and showed the user nothing.
func (r *Runner) runModelToolRound(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool, phase string) (planningResult, error) {
	var usage *adapter.Usage
	delays := r.modelRetryDelays()
	for attempt := 0; ; attempt++ {
		result, err := r.runModelToolRoundAttempt(ctx, prepared, planningReq, round, onChunk, terminalProtocol, terminalStreamingAllowed, suppressNaturalProgress, phase)
		usage = mergeUsage(usage, result.usage)
		result.usage = usage
		if err == nil || attempt >= len(delays) || !transientModelError(err) {
			return result, err
		}
		logger.WarnContext(ctx, "chat runtime retrying model call after a transient error",
			"message_id", prepared.Message.ID.String(),
			"phase", phase,
			"round", round,
			"attempt", attempt+1,
			"delay_ms", delays[attempt].Milliseconds(),
			err,
		)
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, err
		case <-timer.C:
		}
	}
}

func (r *Runner) runModelToolRoundAttempt(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, terminalProtocol bool, terminalStreamingAllowed bool, suppressNaturalProgress bool, phase string) (planningResult, error) {
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
		} else {
			err = connectionLost(err)
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
	r.emitReasoningChunk(prepared, message.ReasoningContent)
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
	// A restored checklist belongs to the request it was sent with; one a
	// retried or continued request still carries is dropped and restored
	// again below if the request still lacks the update_plan result.
	sourceMessages := withoutChecklistReminders(cloneMessagesForProvider(request.Messages))
	// The context manager places system messages for the provider; without
	// one, place them here (ADR 0032).
	request.Messages = sourceMessages
	request.Tools = providerCompatibleFunctionTools(request.Tools)
	r.contextDecision = nil
	// Reasoning from this turn's tool-call rounds goes back only to models
	// that expect it; other APIs may reject the field.
	restored := checklistMissingFromRequest(sourceMessages, r.planSteps)
	if r.ContextManager == nil {
		messages := sourceMessages
		if restored != nil {
			messages = append(messages, *restored)
		}
		request.Messages = adapter.MessagesForModel(request.Model, adapter.PlaceSystemMessages(messages))
		return request, r.applyFinalPlanningRequestBudget(request, sourceMessages)
	}
	// The checklist goes in before the context manager fits the request, so
	// it counts against the same budget as the rest (ADR 0049).
	if restored != nil {
		request.Messages = append(cloneMessagesForProvider(sourceMessages), *restored)
	}
	prepared, decision, err := r.ContextManager.PrepareBeforeModelCall(ctx, request)
	if err != nil {
		return request, err
	}
	if restored == nil {
		// The update_plan result was in the request, and fitting it may
		// have compacted the result away: fit again with the checklist.
		if restored = checklistMissingFromRequest(prepared.Messages, r.planSteps); restored != nil {
			request.Messages = append(cloneMessagesForProvider(sourceMessages), *restored)
			if prepared, decision, err = r.ContextManager.PrepareBeforeModelCall(ctx, request); err != nil {
				return request, err
			}
		}
	}
	prepared.Messages = adapter.MessagesForModel(prepared.Model, prepared.Messages)
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
	result.contextMessages = withoutChecklistReminders(cloneMessagesForProvider(request.Messages))
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
	result.contextMessages = withoutChecklistReminders(cloneMessagesForProvider(request.Messages))
	if r != nil && r.ContextManager != nil {
		result.usage = mergeUsage(r.ContextManager.ConsumeCompactionUsage(), result.usage)
	}
	return result
}

func (r *Runner) checkpointContext(ctx context.Context, messages []adapter.Message) error {
	if r == nil || r.ContextManager == nil {
		return nil
	}
	return r.ContextManager.ReplaceMessages(ctx, messages)
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

// emitReasoningChunk streams the model's reasoning to the client as a message
// event with an empty answer, the same contract as the direct chat stream. The
// reasoning is display-only: it is never replayed into the model context.
func (r *Runner) emitReasoningChunk(prepared *PreparedChat, reasoning string) {
	if reasoning == "" || prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return
	}
	r.emitEvent(prepared, EventMessage, map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"answer":            "",
		"reasoning_content": reasoning,
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

func terminalOnlySystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"The approved or resumed operation has already completed the remaining bound plan phase.",
		"Use only the authoritative current-turn tool, approval, page, and operation-plan evidence supplied in context.",
		"No tools are available in this terminal response. Do not call or invent tools, load skills, update the plan, repeat completed work, or request more execution.",
		"Reply directly with one concise, self-contained completion message in the user's language. Mention only outcomes supported by the supplied evidence.",
	}, "\n")}
}

// checklistMissingFromRequest returns the run's checklist as a system message
// when the request no longer holds the update_plan result that carries it,
// as after a compaction (ADR 0049).
func checklistMissingFromRequest(messages []adapter.Message, steps []map[string]interface{}) *adapter.Message {
	if len(steps) == 0 {
		return nil
	}
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") && toolResultHoldsChecklist(assistantMessageText(message), steps) {
			return nil
		}
	}
	var builder strings.Builder
	builder.WriteString(checklistReminderHeading)
	for _, step := range steps {
		builder.WriteString("\n- [" + evidenceStringFromAny(step["status"]) + "] " + evidenceStringFromAny(step["step"]))
	}
	return &adapter.Message{Role: "system", Content: builder.String()}
}

// checklistReminderHeading starts every restored checklist.
const checklistReminderHeading = "Your current checklist:"

// withoutChecklistReminders removes restored checklists, of any version, from
// messages. A restored checklist belongs to one request: the next round, or a
// retry of the same request, restores it again if it is still missing, so no
// request carries two copies and a newer checklist replaces an older one.
func withoutChecklistReminders(messages []adapter.Message) []adapter.Message {
	kept := messages[:0]
	for _, message := range messages {
		switch content := message.Content.(type) {
		case string:
			if strings.EqualFold(strings.TrimSpace(message.Role), "system") && strings.HasPrefix(strings.TrimSpace(content), checklistReminderHeading) {
				continue
			}
			// A system notice reaches the provider as a reminder in a user
			// message (ADR 0032).
			text := withoutChecklistReminderText(content)
			if text != content && strings.TrimSpace(text) == "" {
				continue
			}
			message.Content = text
		case []adapter.MessageContentPart:
			// Next to an image the reminder is a text part of its own.
			parts := make([]adapter.MessageContentPart, 0, len(content))
			for _, part := range content {
				if part.Type == "text" {
					text := withoutChecklistReminderText(part.Text)
					if text != part.Text && strings.TrimSpace(text) == "" {
						continue
					}
					part.Text = text
				}
				parts = append(parts, part)
			}
			if len(parts) == 0 {
				continue
			}
			message.Content = parts
		}
		kept = append(kept, message)
	}
	return kept
}

func withoutChecklistReminderText(text string) string {
	const opening = "<system-reminder>\n" + checklistReminderHeading
	const closing = "\n</system-reminder>"
	for {
		start := strings.Index(text, opening)
		if start < 0 {
			return text
		}
		length := strings.Index(text[start:], closing)
		if length < 0 {
			return text
		}
		before, after := text[:start], text[start+length+len(closing):]
		// Reminders merged into one message are separated by a blank line.
		if strings.HasSuffix(before, "\n\n") {
			before = strings.TrimSuffix(before, "\n\n")
		} else {
			after = strings.TrimPrefix(after, "\n\n")
		}
		text = before + after
	}
}

// toolResultHoldsChecklist reports that a tool result is the update_plan
// result for exactly these steps; an earlier version of the checklist does
// not count.
func toolResultHoldsChecklist(content string, steps []map[string]interface{}) bool {
	var result struct {
		Status string `json:"status"`
		Plan   []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if json.Unmarshal([]byte(content), &result) != nil || result.Status != "recorded" || len(result.Plan) != len(steps) {
		return false
	}
	for index, step := range result.Plan {
		if step.Step != evidenceStringFromAny(steps[index]["step"]) || step.Status != evidenceStringFromAny(steps[index]["status"]) {
			return false
		}
	}
	return true
}
