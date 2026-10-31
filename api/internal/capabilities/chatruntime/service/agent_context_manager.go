package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (s *service) newAgentContextManager(ctx context.Context, prepared *PreparedChat) (*contextmgr.Manager, error) {
	if prepared == nil || prepared.Message == nil || prepared.LLMRequest == nil {
		return nil, fmt.Errorf("agent context manager input is incomplete")
	}
	if prepared.contextBudget == nil && runningUnderGoTest() {
		return nil, nil
	}
	var spec ModelSpec
	mainOutput := 0
	if prepared.contextBudget != nil {
		spec = prepared.contextBudget.Spec
		if prepared.contextBudget.Budget != nil {
			mainOutput = prepared.contextBudget.Budget.ReservedOutputTokens
		}
	} else {
		if s != nil && s.modelSpecResolver != nil {
			resolved, ok, err := s.modelSpecResolver.Resolve(ctx, prepared.Scope.OrganizationID, prepared.LLMRequest.Provider, prepared.LLMRequest.Model)
			if err != nil {
				logger.WarnContext(ctx, "failed to resolve Agent context model spec; using fallback context window", "error", err)
			} else if ok {
				spec = resolved
			}
		}
	}
	if spec.ContextWindow <= 0 {
		spec.ContextWindow = contextmgr.DefaultModelContextWindowTokens
	}
	if prepared.LLMRequest.MaxTokens != nil && *prepared.LLMRequest.MaxTokens > 0 {
		mainOutput = *prepared.LLMRequest.MaxTokens
	}
	runID := preparedAgentRunID(prepared)
	if prepared.Message.Metadata == nil {
		prepared.Message.Metadata = map[string]interface{}{}
	}
	prepared.Message.Metadata["agent_run_id"] = runID
	manager, err := contextmgr.New(contextmgr.Config{
		AgentRunID:              runID,
		ModelContextWindow:      spec.ContextWindow,
		MaxInputTokens:          spec.MaxInputTokens,
		MaxOutputTokens:         spec.MaxOutputTokens,
		DefaultMainOutputTokens: mainOutput,
		EstimateScale:           s.contextEstimateScale(ctx, prepared),
	}, &serviceContextCompactor{service: s, prepared: prepared}, nil, func(requestType string, round int, request *adapter.ChatRequest, decision contextmgr.Decision) {
		s.writeAgentContextPromptDumpBestEffort(context.Background(), prepared, requestType, round, request, &decision)
	}, func(ctx context.Context, transcript []adapter.Message) {
		// Best effort and never cancelled with the request: the point of a
		// checkpoint is that a turn interrupted mid-flight is still recoverable.
		s.persistTurnTranscriptEntries(context.WithoutCancel(ctx), prepared, transcript)
	})
	if err != nil {
		return nil, fmt.Errorf("create agent context manager: %w", err)
	}
	return manager, nil
}

func preparedAgentRunID(prepared *PreparedChat) string {
	if prepared != nil && prepared.Message != nil {
		if value := strings.TrimSpace(stringFromAny(prepared.Message.Metadata["agent_run_id"])); value != "" {
			return value
		}
		if value := strings.TrimSpace(stringFromAny(prepared.Message.Metadata["task_id"])); value != "" {
			return value
		}
		return prepared.Message.ID.String()
	}
	return ""
}

type serviceContextCompactor struct {
	service  *service
	prepared *PreparedChat
}

func (c *serviceContextCompactor) Compact(ctx context.Context, request *adapter.ChatRequest, call contextmgr.CompactCall) (string, *adapter.Usage, error) {
	if c == nil || c.service == nil || c.service.llmClient == nil || c.prepared == nil {
		return "", nil, errors.New("context compaction model client is unavailable")
	}
	// Persist the turn before summarising it: the summary boundary is the last
	// replayed entry of the compact input, so everything the summarizer is about
	// to receive has to be durable first.
	boundTranscript := c.service.persistTurnTranscriptEntries(ctx, c.prepared, call.Transcript)
	if len(boundTranscript) == len(call.Transcript) {
		copy(call.Transcript, boundTranscript)
	}
	startedAt := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, c.service.modelIdleTimeoutValue())
	defer cancel()
	response, err := c.service.llmClient.AppChat(callCtx, newBillingAppContext(c.prepared), request)
	if err != nil {
		c.persistInvocation(call, request, nil, nil, startedAt, err)
		return "", nil, err
	}
	if response == nil || len(response.Choices) == 0 {
		err = errors.New("context compaction returned no choices")
		c.persistInvocation(call, request, nil, nil, startedAt, err)
		return "", nil, err
	}
	choice := response.Choices[0]
	finishReason := strings.ToLower(strings.TrimSpace(choice.FinishReason))
	if finishReason == "length" || finishReason == "max_tokens" || finishReason == "content_filter" {
		err = fmt.Errorf("context compaction ended with %s", finishReason)
		c.persistInvocation(call, request, &choice.Message, response.Usage, startedAt, err)
		return "", response.Usage, err
	}
	if len(choice.Message.ToolCalls) > 0 || choice.Message.FunctionCall != nil {
		err = errors.New("context compaction attempted a tool call")
		c.persistInvocation(call, request, &choice.Message, response.Usage, startedAt, err)
		return "", response.Usage, err
	}
	summary := strings.TrimSpace(stringFromAny(choice.Message.Content))
	if summary == "" {
		err = errors.New("context compaction returned an empty summary")
		c.persistInvocation(call, request, &choice.Message, response.Usage, startedAt, err)
		return "", response.Usage, err
	}
	c.persistInvocation(call, request, &choice.Message, response.Usage, startedAt, nil)
	c.persistCompactionSummary(ctx, call, summary)
	return summary, response.Usage, nil
}

// persistCompactionSummary records one summary together with its exact boundary:
// the conversation position of the last replayed entry the summarizer received.
// The manager reads that position off the compact input it actually sent, so the
// boundary is stated rather than matched — it can neither over-claim (which
// would drop content) nor under-claim (which would replay it twice).
func (c *serviceContextCompactor) persistCompactionSummary(ctx context.Context, call contextmgr.CompactCall, summary string) {
	if c == nil || c.prepared == nil || c.prepared.Message == nil {
		return
	}
	if strings.TrimSpace(call.SummarizedEntryID) == "" {
		logger.WarnContext(ctx, "agent context summary has no entry boundary; keeping it in memory only",
			"message_id", c.prepared.Message.ID.String())
		return
	}
	c.service.persistAgentContextSummary(ctx, c.prepared, summary, call.SummarizedEntryID)
}

func (c *serviceContextCompactor) persistInvocation(call contextmgr.CompactCall, request *adapter.ChatRequest, response *adapter.Message, usage *adapter.Usage, startedAt time.Time, callErr error) {
	if c == nil || c.service == nil || c.prepared == nil {
		return
	}
	decision := call.Decision
	trace := skillloop.ModelInvocationTrace{
		Phase:                      call.Type,
		Round:                      call.APIRound,
		Streaming:                  false,
		StartedAt:                  startedAt,
		DurationMS:                 time.Since(startedAt).Milliseconds(),
		Request:                    request,
		Response:                   response,
		Usage:                      usage,
		StreamDoneReceived:         callErr == nil,
		TerminatedBy:               "response",
		AgentRunID:                 call.AgentRunID,
		ContextAPIRound:            decision.APIRound,
		ContextRequestType:         decision.RequestType,
		ContextDecision:            decision.Action,
		ContextModelWindowTokens:   decision.Budget.ModelContextWindow,
		ContextConfiguredWindowK:   decision.Budget.ConfiguredAgentWindowK,
		ContextEffectiveWindow:     decision.Budget.AgentContextWindow,
		ContextWindowClamped:       decision.Budget.AgentContextWindowClamped,
		ContextSoftLimit:           decision.Budget.SoftLimit,
		ContextHardLimit:           decision.Budget.HardLimit,
		ContextTargetTokens:        decision.Budget.TargetTokens,
		ContextFixedTokens:         decision.FixedRequestTokens,
		ContextCompressibleTokens:  decision.CompressibleTokens,
		ContextToolTokensBefore:    decision.ToolResultOriginalTokens,
		ContextToolTokensAfter:     decision.ToolResultProjectedTokens,
		ContextToolProjectionCount: decision.ToolProjectionCount,
		ContextLossyRecoveryRounds: decision.LossyRecoveryDroppedRounds,
		ContextCompactedThrough:    decision.CompactedThroughRound,
		ContextCompactionFailures:  decision.ConsecutiveCompactionFailures,
		BudgetPromptLimit:          decision.Budget.PromptBudget,
		BudgetEstimateScale:        decision.EstimateScale,
		EstimatedPromptTokens:      decision.FinalPromptTokens,
		PromptEstimator:            decision.Estimator,
		PromptComponentTokens:      decision.ComponentTokens,
	}
	if callErr != nil {
		trace.Error = callErr.Error()
	}
	c.service.persistModelInvocationBestEffort(context.Background(), c.prepared, trace)
}

// contextEstimateScale is the provider's prompt tokens per estimated token
// last recorded for this model in this conversation: on the message itself
// for a continuation, else on the turn it answers. A record made by another
// estimator does not count. It lets the first request of a turn use the
// budget the provider's usage showed, instead of the conservative estimate
// alone (ADR 0038).
func (s *service) contextEstimateScale(ctx context.Context, prepared *PreparedChat) float64 {
	if s == nil || prepared == nil || prepared.LLMRequest == nil {
		return 1
	}
	provider := strings.TrimSpace(prepared.LLMRequest.Provider)
	if provider == "" && prepared.parts != nil {
		provider = strings.TrimSpace(prepared.parts.Provider)
	}
	model := strings.TrimSpace(prepared.LLMRequest.Model)
	key := promptUsageCalibrationKey(provider, model)
	if key == "" || s.tokenEstimator == nil {
		return 1
	}
	tokenizer := s.tokenEstimator.EstimateText("x", model).Tokenizer
	scaleFrom := func(metadata map[string]interface{}) float64 {
		record := mapFromOperationContext(mapFromOperationContext(metadata["prompt_usage_calibration"])[key])
		if len(record) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromAny(record["tokenizer"])), tokenizer) {
			return 0
		}
		scale, _ := record["prompt_estimate_scale"].(float64)
		return scale
	}
	if prepared.Message != nil {
		if scale := scaleFrom(prepared.Message.Metadata); scale > 0 {
			return scale
		}
	}
	if prepared.ParentID == nil || s.repos == nil || s.repos.Message == nil {
		return 1
	}
	parent, err := s.repos.Message.GetScoped(ctx, *prepared.ParentID, prepared.Scope.OrganizationID, prepared.Scope.AccountID)
	if err != nil || parent == nil {
		return 1
	}
	if scale := scaleFrom(parent.Metadata); scale > 0 {
		return scale
	}
	return 1
}
