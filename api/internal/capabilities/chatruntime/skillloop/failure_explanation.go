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
	SkillID   string `json:"skill_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Status    string `json:"status,omitempty"`
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
	evidence := buildToolFailureExplanationEvidence(failure, successfulToolCalls, stopReason)
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return "", nil, fmt.Errorf("marshal tool failure evidence: %w", err)
	}

	explanationRequest := cloneChatRequest(prepared.LLMRequest)
	explanationRequest.Messages = cloneMessagesForProvider(prepared.LLMRequest.Messages)
	if r.ContextManager != nil {
		if state := r.ContextManager.State(); len(state.Messages) > 0 {
			explanationRequest.Messages = cloneMessagesForProvider(state.Messages)
		}
	}
	explanationRequest.Messages = append(explanationRequest.Messages, adapter.Message{
		Role: "system",
		Content: strings.Join([]string{
			"The requested operation could not be completed by the available business tool.",
			"Write the final user-facing answer now, using only the user request and the internal evidence below.",
			"State truthfully that the operation did not complete; never claim that a file, asset, update, or other side effect succeeded.",
			"Explain the reason in language the user can understand and suggest a practical next step, such as retrying, choosing another format, or supplying required information.",
			"When the evidence names an integration action, that action exists. Never claim that the integration lacks the capability merely because its arguments failed validation.",
			"When provider_request_sent is false, say that validation failed before any request was sent to the external provider.",
			"Do not expose schemas, call IDs, stack traces, raw internal errors, credentials, or implementation details.",
			"Do not call or propose calling tools in this response. Return natural-language answer text only.",
			"Internal evidence (not for verbatim disclosure): " + string(encodedEvidence),
		}, "\n"),
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
			request.Messages = append(request.Messages, adapter.Message{
				Role:    "system",
				Content: "The previous response was empty or attempted a tool call. Reply once with concise natural-language failure guidance only.",
			})
		}
		r.requestBudget = planningRequestBudgetForRun(req)
		request, budgetErr := r.prepareContextRequest(ctx, request)
		if budgetErr != nil {
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
		r.recordModelRequest("tool_failure_explanation", round, request)
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
		mainResponseUsage := planningRespUsage(response)
		contextResult, contextErr := r.finishContextRequest(ctx, request, planningResult{message: message, usage: mainResponseUsage})
		if contextErr != nil {
			return "", mergeUsage(usage, contextResult.usage), contextErr
		}
		usage = mergeUsage(usage, contextResult.usage)
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
			Usage:              mainResponseUsage,
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

func buildToolFailureExplanationEvidence(
	failure skills.SkillTrace,
	successfulToolCalls []SkillToolCallRef,
	stopReason string,
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
	if providerRequestSent, ok := failure.Arguments["provider_request_sent"].(bool); ok {
		evidence.ProviderRequestSent = &providerRequestSent
	}
	evidence.Recoverable, _ = failure.Arguments["recoverable"].(bool)
	evidence.RecoveryAction = strings.TrimSpace(evidenceStringFromAny(failure.Arguments["recovery_action"]))
	evidence.MissingFields = evidenceStringSliceFromAny(failure.Arguments["missing_fields"])
	for _, call := range successfulToolCalls {
		evidence.CompletedOperations = append(evidence.CompletedOperations, toolFailureOperation{
			SkillID:  strings.TrimSpace(call.SkillID),
			ToolName: strings.TrimSpace(call.ToolName),
			Status:   "success",
		})
	}
	return evidence
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
