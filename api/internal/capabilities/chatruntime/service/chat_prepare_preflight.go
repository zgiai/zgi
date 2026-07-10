package service

import (
	"context"
	"errors"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	contextualPrepareIntentTimeout = 15 * time.Second
	contextualPrepareMemoryTimeout = 8 * time.Second
)

type contextualPreparePreflightResult struct {
	UserMemoryDone  bool
	UserMemoryUsage *adapter.Usage
}

type contextualPrepareIntentResult struct {
	intent *AIChatModelTurnIntent
	err    error
}

type contextualPrepareMemoryResult struct {
	result *userMemoryPreparePreflightResult
	err    error
}

func (s *service) runContextualPreparePreflights(
	ctx context.Context,
	scope Scope,
	conversation *runtimemodel.Conversation,
	config RunConfig,
	parts *chatRequestParts,
	llmRequest *adapter.ChatRequest,
) (*contextualPreparePreflightResult, error) {
	if parts == nil || !isContextualAIChatSurface(parts.Surface) {
		return nil, nil
	}

	runIntent := s.shouldClassifyContextualAIChatTurnIntent(conversation, parts)
	runMemory := s.shouldRunUserMemoryPreflightDuringPrepare(parts, llmRequest)
	if !runIntent && !runMemory {
		return nil, nil
	}

	var intentCh chan contextualPrepareIntentResult
	if runIntent {
		intentCh = make(chan contextualPrepareIntentResult, 1)
		go func() {
			preflightCtx, cancel := context.WithTimeout(ctx, contextualPrepareIntentTimeout)
			defer cancel()
			intent, err := s.classifyContextualAIChatTurnIntent(preflightCtx, scope, conversation, config, parts)
			intentCh <- contextualPrepareIntentResult{intent: intent, err: err}
		}()
	}

	var memoryCh chan contextualPrepareMemoryResult
	if runMemory {
		memoryCh = make(chan contextualPrepareMemoryResult, 1)
		go func() {
			preflightCtx, cancel := context.WithTimeout(ctx, contextualPrepareMemoryTimeout)
			defer cancel()
			result, err := s.runUserMemoryPreflightDuringPrepare(preflightCtx, scope, conversation, config, parts, llmRequest)
			memoryCh <- contextualPrepareMemoryResult{result: result, err: err}
		}()
	}

	preflight := &contextualPreparePreflightResult{}
	if intentCh != nil {
		intentResult := <-intentCh
		s.applyContextualAIChatModelTurnIntentResult(ctx, conversation, parts, intentResult.intent, intentResult.err)
	}
	if memoryCh != nil {
		memoryResult := <-memoryCh
		if memoryResult.err != nil {
			if ctx.Err() != nil {
				return preflight, memoryResult.err
			}
			if errors.Is(memoryResult.err, context.DeadlineExceeded) && ctx.Err() == nil {
				markUserMemoryPreflightTimeout(parts)
				return preflight, nil
			}
			markUserMemoryPreflightError(parts, memoryResult.err)
			return preflight, nil
		}
		preflight.UserMemoryDone = true
		if memoryResult.result != nil {
			preflight.UserMemoryUsage = memoryResult.result.Usage
			if memoryResult.result.ContextControl != nil {
				parts.ContextControl = memoryResult.result.ContextControl
			}
			if llmRequest != nil && memoryResult.result.Messages != nil {
				llmRequest.Messages = memoryResult.result.Messages
			}
		}
	}
	return preflight, nil
}

func markUserMemoryPreflightTimeout(parts *chatRequestParts) {
	if parts == nil {
		return
	}
	contextControl := copyStringAnyMap(parts.ContextControl)
	if contextControl == nil {
		contextControl = map[string]interface{}{}
	}
	userMemory := copyStringAnyMap(mapFromOperationContext(contextControl["user_memory"]))
	if userMemory == nil {
		userMemory = map[string]interface{}{}
	}
	userMemory["planner_status"] = "timeout_non_blocking"
	userMemory["planner_action"] = "none"
	contextControl["user_memory"] = userMemory
	parts.ContextControl = contextControl
}

func markUserMemoryPreflightError(parts *chatRequestParts, err error) {
	if parts == nil {
		return
	}
	contextControl := copyStringAnyMap(parts.ContextControl)
	if contextControl == nil {
		contextControl = map[string]interface{}{}
	}
	userMemory := copyStringAnyMap(mapFromOperationContext(contextControl["user_memory"]))
	if userMemory == nil {
		userMemory = map[string]interface{}{}
	}
	userMemory["planner_status"] = "error_non_blocking"
	userMemory["planner_action"] = "none"
	if err != nil {
		userMemory["planner_error"] = trimRunes(err.Error(), 500)
	}
	contextControl["user_memory"] = userMemory
	parts.ContextControl = contextControl
}
