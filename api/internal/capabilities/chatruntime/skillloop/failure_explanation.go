package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const toolFailureExplainedCompletionReason = "tool_failure_explained"

type toolFailureExplanationEvidence struct {
	StopReason          string                 `json:"stop_reason"`
	FailedOperation     toolFailureOperation   `json:"failed_operation"`
	CompletedOperations []toolFailureOperation `json:"completed_operations,omitempty"`
	PartialSuccess      bool                   `json:"partial_success,omitempty"`
	MissingFields       []string               `json:"missing_fields,omitempty"`
	FailureStage        string                 `json:"failure_stage,omitempty"`
	ReasonCode          string                 `json:"reason_code,omitempty"`
	IntegrationID       string                 `json:"integration_id,omitempty"`
	ActionID            string                 `json:"action_id,omitempty"`
	ProviderRequestSent *bool                  `json:"provider_request_sent,omitempty"`
	Recoverable         bool                   `json:"recoverable,omitempty"`
	RecoveryAction      string                 `json:"recovery_action,omitempty"`
	InternalError       string                 `json:"internal_error,omitempty"`
}

type toolFailureOperation struct {
	SkillID       string `json:"skill_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	IntegrationID string `json:"integration_id,omitempty"`
	ActionID      string `json:"action_id,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	Status        string `json:"status,omitempty"`
}

func (r *Runner) completeBusinessToolFailure(
	ctx context.Context,
	req RunRequest,
	prepared *PreparedChat,
	traces []skills.SkillTrace,
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
	round int,
) (string, *adapter.Usage, error) {
	answer, usage, err := r.runToolFailureExplanation(
		ctx,
		req,
		prepared,
		failure,
		successfulToolCalls,
		stopReason,
		round,
	)
	if err != nil {
		return "", usage, err
	}

	trace := skills.SkillTrace{
		Kind:    "final_answer",
		Title:   "Final answer",
		Message: answer,
		Status:  "success",
		Arguments: map[string]interface{}{
			"completion_reason": toolFailureExplainedCompletionReason,
		},
		Result: map[string]interface{}{
			"source": "model",
		},
	}
	traces = append(traces, trace)
	r.recordTrace(traces, trace)
	r.logSkillTrace(ctx, prepared, trace)
	r.emitAnswerChunk(ctx, prepared, answer, nil)
	if req.OnTerminalCompletion != nil {
		req.OnTerminalCompletion(TerminalCompletionResult{
			Status:           "blocked",
			Source:           "model_tool_failure_explanation",
			Reason:           strings.TrimSpace(stopReason),
			CompletionReason: toolFailureExplainedCompletionReason,
		})
	}
	return answer, usage, nil
}

func (r *Runner) runToolFailureExplanation(
	ctx context.Context,
	req RunRequest,
	prepared *PreparedChat,
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
	round int,
) (string, *adapter.Usage, error) {
	runtimeState := runtimeStateWithSuccessfulToolCalls(req, successfulToolCalls)
	evidence := buildToolFailureExplanationEvidenceWithRuntimeState(
		failure,
		successfulToolCalls,
		stopReason,
		runtimeState,
	)
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return "", nil, fmt.Errorf("marshal tool failure evidence: %w", err)
	}

	explanationRequest := cloneChatRequest(prepared.LLMRequest)
	explanationRequest.Messages = cloneMessagesForProvider(prepared.LLMRequest.Messages)
	explanationRequest.Messages = append(explanationRequest.Messages, adapter.Message{
		Role:    "system",
		Content: toolFailureExplanationSystemPrompt(evidence, string(encodedEvidence)),
	})
	explanationRequest.Tools = nil
	explanationRequest.ToolChoice = nil
	explanationRequest.Functions = nil
	explanationRequest.FunctionCall = nil
	explanationRequest.ResponseFormat = nil
	explanationRequest.Stream = false
	explanationRequest.StreamOptions = nil

	var usage *adapter.Usage
	for attempt := 0; attempt < 2; attempt++ {
		request := cloneChatRequest(explanationRequest)
		if attempt > 0 {
			retryInstruction := "The previous response was empty or attempted a tool call. Reply once with concise natural-language failure guidance only."
			if evidence.PartialSuccess {
				retryInstruction = "The previous response was empty or attempted a tool call. Reply once with a concise natural-language partial-success summary that separately identifies the completed and uncompleted operations."
			}
			request.Messages = append(request.Messages, adapter.Message{
				Role:    "system",
				Content: retryInstruction,
			})
		}
		sourceMessages := cloneMessagesForProvider(request.Messages)
		request.Messages = adapter.NormalizeSystemMessages(sourceMessages)
		r.requestBudget = planningRequestBudgetForRun(req)
		if budgetErr := r.applyFinalPlanningRequestBudget(request, sourceMessages); budgetErr != nil {
			r.recordModelInvocation(ModelInvocationTrace{
				Phase:     "tool_failure_explanation",
				Round:     round,
				Request:   request,
				StartedAt: time.Now(),
				Error:     budgetErr.Error(),
			})
			return "", usage, budgetErr
		}

		startedAt := time.Now()
		callCtx, cancel := context.WithTimeout(ctx, r.modelIdleTimeout())
		response, callErr := r.LLMClient.AppChat(callCtx, r.AppContext, request)
		contextErr := callCtx.Err()
		cancel()
		if callErr != nil {
			if errors.Is(contextErr, context.DeadlineExceeded) {
				callErr = ErrModelIdleTimeout
			}
			r.recordModelInvocation(ModelInvocationTrace{
				Phase:      "tool_failure_explanation",
				Round:      round,
				Streaming:  false,
				StartedAt:  startedAt,
				DurationMS: time.Since(startedAt).Milliseconds(),
				Request:    request,
				Error:      callErr.Error(),
			})
			return "", usage, callErr
		}

		message := firstPlanningMessage(response)
		responseUsage := planningRespUsage(response)
		usage = mergeUsage(usage, responseUsage)
		finishReason := planningResponseFinishReason(response)
		terminationErr := nonStreamingPlanningTerminationError(finishReason)
		answer := strings.TrimSpace(assistantMessageText(message))
		if terminationErr == nil && (answer == "" || len(normalizeToolCalls(message.ToolCalls)) > 0) {
			terminationErr = fmt.Errorf("%w: tool failure explanation returned no usable natural-language answer", ErrInvalidInput)
		}
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:              "tool_failure_explanation",
			Round:              round,
			Streaming:          false,
			StartedAt:          startedAt,
			DurationMS:         time.Since(startedAt).Milliseconds(),
			Request:            request,
			Response:           &message,
			Usage:              responseUsage,
			FinishReason:       finishReason,
			StreamDoneReceived: true,
			TerminatedBy:       "response",
			Error:              errorString(terminationErr),
		})
		if terminationErr == nil {
			return answer, usage, nil
		}
		if attempt == 1 {
			return "", usage, terminationErr
		}
	}
	return "", usage, fmt.Errorf("%w: tool failure explanation was not generated", ErrInvalidInput)
}

func toolFailureExplanationSystemPrompt(evidence toolFailureExplanationEvidence, encodedEvidence string) string {
	instructions := []string{}
	if evidence.PartialSuccess {
		instructions = append(instructions,
			"The request was only partially completed: the evidence contains one or more trustworthy completed business operations and a separate operation that did not complete.",
			"Write the final user-facing answer now, using only the user request and the internal evidence below.",
			"Explicitly distinguish every operation in completed_operations from failed_operation, so the user can tell what succeeded and what remains uncompleted.",
			"Preserve the confirmed successes. Never say that the whole request failed, and never deny or cast doubt on an operation listed in completed_operations.",
			"Do not infer success for any operation not listed in completed_operations. An outcome_unknown, unconfirmed, partial, or failed external operation is not a success.",
		)
	} else {
		instructions = append(instructions,
			"The requested operation could not be completed by the available business tool.",
			"Write the final user-facing answer now, using only the user request and the internal evidence below.",
			"State truthfully that the operation did not complete; never claim that a file, asset, update, or other side effect succeeded.",
		)
	}
	instructions = append(instructions,
		"Explain the reason in language the user can understand and suggest a practical next step, such as retrying, choosing another format, or supplying required information.",
		"When the evidence names an integration action, that action exists. Never claim that the integration lacks the capability merely because its arguments failed validation.",
		"When provider_request_sent is false, say that validation failed before any request was sent to the external provider.",
		"Do not expose schemas, call IDs, stack traces, raw internal errors, credentials, or implementation details.",
		"Do not call or propose calling tools in this response. Return natural-language answer text only.",
		"Internal evidence (not for verbatim disclosure): "+encodedEvidence,
	)
	return strings.Join(instructions, "\n")
}

func buildToolFailureExplanationEvidence(
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
) toolFailureExplanationEvidence {
	return buildToolFailureExplanationEvidenceWithRuntimeState(failure, successfulToolCalls, stopReason, nil)
}

func buildToolFailureExplanationEvidenceWithRuntimeState(
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
	runtimeState map[string]interface{},
) toolFailureExplanationEvidence {
	evidence := toolFailureExplanationEvidence{
		StopReason: strings.TrimSpace(stopReason),
		FailedOperation: toolFailureOperation{
			SkillID:   strings.TrimSpace(failure.SkillID),
			ToolName:  strings.TrimSpace(failure.ToolName),
			ErrorCode: strings.TrimSpace(evidenceStringFromAny(failure.Arguments["reason_code"])),
			Status:    strings.TrimSpace(failure.Status),
		},
		InternalError: truncateFailureEvidence(failure.Error, 1600),
	}
	evidence.FailureStage = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["failure_stage"]))
	evidence.ReasonCode = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["reason_code"]))
	evidence.IntegrationID = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["integration_id"]))
	evidence.ActionID = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["action_id"]))
	if evidence.IntegrationID == "" || evidence.ActionID == "" {
		integrationID, actionID := terminalStateGuardExternalActionIdentity(map[string]interface{}{
			"arguments": failure.Arguments,
			"result":    failure.Result,
		})
		if evidence.IntegrationID == "" {
			evidence.IntegrationID = integrationID
		}
		if evidence.ActionID == "" {
			evidence.ActionID = actionID
		}
	}
	evidence.FailedOperation.IntegrationID = evidence.IntegrationID
	evidence.FailedOperation.ActionID = evidence.ActionID
	if providerRequestSent, ok := failure.Arguments["provider_request_sent"].(bool); ok {
		evidence.ProviderRequestSent = &providerRequestSent
	}
	evidence.Recoverable, _ = failure.Arguments["recoverable"].(bool)
	evidence.RecoveryAction = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["recovery_action"]))
	evidence.MissingFields = evidenceStringSliceFromAny(failure.Arguments["missing_fields"])
	evidence.CompletedOperations = toolFailureCompletedOperations(successfulToolCalls, runtimeState)
	evidence.PartialSuccess = len(evidence.CompletedOperations) > 0
	return evidence
}

func toolFailureCompletedOperations(
	successfulToolCalls []SkillToolCallRef,
	runtimeState map[string]interface{},
) []toolFailureOperation {
	completed := make([]toolFailureOperation, 0)
	seen := map[string]struct{}{}
	appendOperation := func(operation toolFailureOperation) {
		key := strings.ToLower(strings.Join([]string{
			strings.TrimSpace(operation.SkillID),
			strings.TrimSpace(operation.ToolName),
			strings.TrimSpace(operation.IntegrationID),
			strings.TrimSpace(operation.ActionID),
			strings.TrimSpace(operation.Status),
		}, "\x00"))
		if key == "\x00\x00\x00\x00" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		completed = append(completed, operation)
	}

	if runtimeState != nil {
		for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"]) {
			if !strings.EqualFold(
				strings.TrimSpace(evidenceStringFromAny(phase[operationPlanPhaseSourceKey])),
				operationPlanPhaseSourceServerOutcome,
			) || !operationPlanPhaseHasServerAuthority(phase, runtimeState) ||
				!operationPlanPhaseRequiredByServer(phase, runtimeState) {
				continue
			}
			if _, external := projectedExternalActionKeyFromExpected(evidenceMapFromAny(phase["expected_action"])); !external {
				continue
			}
			if operation, confirmed := toolFailureCompletedServerOutcome(phase, runtimeState); confirmed {
				appendOperation(operation)
			}
		}
	}

	for _, call := range successfulToolCalls {
		if strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillExternalApps) &&
			strings.EqualFold(strings.TrimSpace(call.ToolName), "execute_action") &&
			runtimeState != nil {
			// In the runtime path, standalone external calls are implementation
			// evidence only. A required server Outcome is the user-visible success
			// contract; in particular, a successful preparation query must not become
			// a completed operation by itself.
			continue
		}
		if operation, trusted := trustedToolFailureCompletedOperation(call); trusted {
			appendOperation(operation)
		}
	}
	return completed
}

func toolFailureCompletedServerOutcome(
	phase map[string]interface{},
	runtimeState map[string]interface{},
) (toolFailureOperation, bool) {
	expected := evidenceMapFromAny(phase["expected_action"])
	actionKey, external := projectedExternalActionKeyFromExpected(expected)
	if !external || actionKey == "" || projectedExpectedActionServerBindingIssue(expected, runtimeState) != "" {
		return toolFailureOperation{}, false
	}
	parts := strings.SplitN(actionKey, ":", 2)
	candidate := projectedExternalActionCandidateByFingerprint(
		runtimeState,
		strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey])),
	)
	if len(parts) != 2 || len(candidate) == 0 ||
		strings.TrimSpace(evidenceStringFromAny(candidate[operationPlanServerProjectedConnectionBindingKey])) !=
			strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedConnectionBindingKey])) ||
		!toolFailureServerOutcomeProviderConfirmed(phase, expected, actionKey, runtimeState) {
		return toolFailureOperation{}, false
	}
	return toolFailureOperation{
		SkillID:       skills.SkillExternalApps,
		ToolName:      "execute_action",
		IntegrationID: parts[0],
		ActionID:      parts[1],
		Status:        "provider_confirmed_success",
	}, true
}

func toolFailureServerOutcomeProviderConfirmed(
	phase map[string]interface{},
	expected map[string]interface{},
	actionKey string,
	runtimeState map[string]interface{},
) bool {
	phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
	ledgerEpoch := strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedLedgerEpochKey]))
	bindingFingerprint := strings.TrimSpace(evidenceStringFromAny(expected[planExpectedActionServerBindingFingerprintKey]))
	connectionBinding := strings.TrimSpace(evidenceStringFromAny(phase[operationPlanServerProjectedConnectionBindingKey]))
	if phaseID == "" || ledgerEpoch == "" || bindingFingerprint == "" || connectionBinding == "" {
		return false
	}
	expectedTarget := terminalStateGuardExpectedActionTargetKey(expected)
	matched := false
	confirmed := false
	for _, invocation := range terminalStateGuardExternalInvocations(runtimeState) {
		if !terminalStateGuardInvocationIsExecutionEvidence(invocation) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillExternalApps) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "execute_action") ||
			terminalStateGuardExternalActionKey(invocation) != actionKey ||
			!strings.EqualFold(terminalStateGuardExternalPlanPhaseID(invocation), phaseID) ||
			terminalStateGuardExternalLedgerEpoch(invocation) != ledgerEpoch {
			continue
		}
		arguments := evidenceMapFromAny(invocation["arguments"])
		if strings.TrimSpace(evidenceStringFromAny(arguments[planExpectedActionServerBindingFingerprintKey])) != bindingFingerprint ||
			strings.TrimSpace(evidenceStringFromAny(arguments[operationPlanServerProjectedConnectionBindingKey])) != connectionBinding {
			continue
		}
		if invocationTarget := terminalStateGuardExternalActionTargetKey(invocation); expectedTarget != "" && invocationTarget != expectedTarget {
			continue
		}
		matched = true
		confirmed = toolFailureInvocationExplicitProviderConfirmed(invocation) &&
			terminalStateGuardExternalActionProviderConfirmed(invocation)
	}
	return matched && confirmed
}

func toolFailureInvocationExplicitProviderConfirmed(invocation map[string]interface{}) bool {
	seen := false
	for _, source := range []map[string]interface{}{
		invocation,
		evidenceMapFromAny(invocation["result"]),
		evidenceMapFromAny(invocation["result_summary"]),
	} {
		raw, exists := source["provider_success_confirmed"]
		if !exists {
			continue
		}
		confirmed, ok := raw.(bool)
		if !ok || !confirmed {
			return false
		}
		seen = true
	}
	return seen
}

func trustedToolFailureCompletedOperation(call SkillToolCallRef) (toolFailureOperation, bool) {
	skillID := strings.TrimSpace(call.SkillID)
	toolName := strings.TrimSpace(call.ToolName)
	if skillID == "" || toolName == "" {
		return toolFailureOperation{}, false
	}
	operation := toolFailureOperation{SkillID: skillID, ToolName: toolName, Status: "success"}
	if !strings.EqualFold(skillID, skills.SkillExternalApps) || !strings.EqualFold(toolName, "execute_action") {
		return operation, true
	}

	record := map[string]interface{}{
		"kind":      "tool_call",
		"status":    "success",
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": call.Arguments,
		"result":    call.Result,
	}
	integrationID, actionID := terminalStateGuardExternalActionIdentity(record)
	providerConfirmed, hasProviderConfirmation := call.Result["provider_success_confirmed"].(bool)
	if integrationID == "" || actionID == "" || !hasProviderConfirmation || !providerConfirmed ||
		!terminalStateGuardExternalActionProviderConfirmed(record) {
		return toolFailureOperation{}, false
	}
	operation.IntegrationID = integrationID
	operation.ActionID = actionID
	operation.Status = "provider_confirmed_success"
	return operation, true
}

func truncateFailureEvidence(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}
