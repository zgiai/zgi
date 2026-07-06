package service

import (
	"context"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
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
			intent, err := s.classifyContextualAIChatTurnIntent(ctx, scope, conversation, config, parts)
			intentCh <- contextualPrepareIntentResult{intent: intent, err: err}
		}()
	}

	var memoryCh chan contextualPrepareMemoryResult
	if runMemory {
		memoryCh = make(chan contextualPrepareMemoryResult, 1)
		go func() {
			result, err := s.runUserMemoryPreflightDuringPrepare(ctx, scope, conversation, config, parts, llmRequest)
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
			return preflight, memoryResult.err
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
