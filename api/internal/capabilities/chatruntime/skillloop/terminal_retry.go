package skillloop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const terminalFinalAnswerMaxAttempts = 2

func (r *Runner) runTerminalOnlyPlanningWithRetry(
	ctx context.Context,
	prepared *PreparedChat,
	planningReq *adapter.ChatRequest,
	round int,
	onChunk func(string) error,
	terminalProtocol bool,
	terminalStreamingAllowed bool,
	suppressNaturalProgress bool,
) (planningResult, error) {
	retryReq := cloneChatRequest(planningReq)
	var totalUsage *adapter.Usage
	var lastResult planningResult
	var lastErr error

	for attempt := 0; attempt < terminalFinalAnswerMaxAttempts; attempt++ {
		result, err := r.runSkillPlanning(
			ctx,
			prepared,
			retryReq,
			round,
			onChunk,
			terminalProtocol,
			terminalStreamingAllowed,
			suppressNaturalProgress || attempt > 0,
		)
		totalUsage = mergeUsage(totalUsage, result.usage)
		result.usage = totalUsage
		lastResult = result
		lastErr = terminalOnlyPlanningResultError(result, err)
		if lastErr == nil {
			return result, nil
		}
		if !terminalFinalAnswerRetryAllowed(ctx, lastErr) {
			return lastResult, lastErr
		}
		if attempt+1 >= terminalFinalAnswerMaxAttempts {
			return lastResult, terminalFinalAnswerUnavailableError(lastErr)
		}

		logger.WarnContext(ctx, "chat runtime terminal answer retry",
			"message_id", prepared.Message.ID.String(),
			"provider", prepared.parts.Provider,
			"model", retryReq.Model,
			"attempt", attempt+1,
			"reason", lastErr.Error(),
		)
		retryReq = terminalOnlyRetryRequest(planningReq)
	}

	return lastResult, terminalFinalAnswerUnavailableError(lastErr)
}

func terminalOnlyPlanningResultError(result planningResult, callErr error) error {
	if callErr != nil {
		return callErr
	}
	if len(normalizeToolCalls(result.message.ToolCalls)) > 0 {
		return errors.New("terminal-only model returned an unexpected tool call")
	}
	if strings.TrimSpace(assistantMessageText(result.message)) == "" {
		return errors.New("terminal-only model returned no final answer")
	}
	return nil
}

func terminalFinalAnswerRetryAllowed(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var terminationErr *PlanningTerminationError
	if errors.As(err, &terminationErr) && terminationErr != nil && !terminationErr.Recoverable {
		return false
	}
	for _, nonRetryable := range []error{
		adapter.ErrInvalidConfig,
		adapter.ErrCapabilityUnsupported,
		adapter.ErrAuthFailed,
		adapter.ErrInsufficientBalance,
		adapter.ErrQuotaExhausted,
		adapter.ErrBillingUnavailable,
		adapter.ErrPlatformChannelUnavailable,
		adapter.ErrModelNotFound,
		adapter.ErrInvalidRequest,
		adapter.ErrContentPolicyViolation,
	} {
		if errors.Is(err, nonRetryable) {
			return false
		}
	}
	return true
}

func terminalOnlyRetryRequest(planningReq *adapter.ChatRequest) *adapter.ChatRequest {
	retryReq := cloneChatRequest(planningReq)
	retryReq.Messages = append(append([]adapter.Message{}, planningReq.Messages...), adapter.Message{
		Role: "system",
		Content: strings.Join([]string{
			"The previous terminal response failed to provide a usable final answer.",
			"Retry once using only the authoritative terminal evidence already provided.",
			"Return one complete natural-language final answer. Do not call tools, repeat operations, or leave the answer empty.",
		}, "\n"),
	})
	return retryReq
}

func terminalFinalAnswerUnavailableError(cause error) error {
	if cause == nil {
		return ErrFinalAnswerUnavailable
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return errors.Join(ErrFinalAnswerUnavailable, fmt.Errorf("terminal answer generation failed: %w", cause))
}
