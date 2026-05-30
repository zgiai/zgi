package memoryplanner

import (
	"context"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func Run(ctx context.Context, req Request, domain Domain) Result {
	result := Result{MetadataUpdates: map[string]interface{}{}}
	baseMessages := []adapter.Message{}
	if req.LLMRequest != nil {
		baseMessages = append(baseMessages, req.LLMRequest.Messages...)
	}
	if strings.TrimSpace(req.LatestUserMessage) == "" {
		trace := domain.PlannerTrace(nil, "skipped_empty_query", nil)
		result.Traces = append(result.Traces, trace)
		result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(nil, "skipped_empty_query", nil, ""))
		result.Messages = appendMessages(baseMessages, domain.GuardNote("skipped_empty_query"))
		return result
	}
	if req.LLMClient == nil {
		err := fmt.Errorf("memory planner llm client is not configured")
		trace := domain.PlannerTrace(nil, "error_llm", err)
		result.Traces = append(result.Traces, trace)
		result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(nil, "error_llm", nil, ""))
		result.Messages = appendMessages(baseMessages, domain.GuardNote("error_llm"))
		return result
	}

	maxRounds := req.MaxPlanningRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxPlanningRounds
	}
	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxPlanningRetries
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	planningTimeout := req.PlanningTimeout
	if planningTimeout <= 0 {
		planningTimeout = DefaultPlanningTimeout
	}
	messages := domain.PlannerMessages(baseMessages)
	retries := 0

	for round := 0; round < maxRounds; round++ {
		_ = round
		planningReq := cloneChatRequest(req.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = nil
		planningReq.ToolChoice = nil
		temperature := 0.0
		planningReq.Temperature = &temperature
		planningReq.MaxTokens = &maxTokens
		if req.UseJSONMode {
			planningReq.ResponseFormat = &adapter.ResponseFormat{Type: "json_object"}
		} else {
			planningReq.ResponseFormat = nil
		}

		planningCtx, cancel := context.WithTimeout(ctx, planningTimeout)
		resp, err := req.LLMClient.AppChat(planningCtx, req.AppContext, planningReq)
		cancel()
		if err != nil {
			trace := domain.PlannerTrace(nil, "error_llm", err)
			result.Traces = append(result.Traces, trace)
			result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(nil, "error_llm", nil, ""))
			result.Messages = appendMessages(baseMessages, domain.GuardNote("error_llm"))
			return result
		}
		result.Usage = MergeUsage(result.Usage, responseUsage(resp))
		decision, err := domain.ParseDecision(messageTextFromResponse(firstResponseMessage(resp)))
		if err != nil {
			retries++
			trace := domain.PlannerTrace(nil, "error_parse", err)
			result.Traces = append(result.Traces, trace)
			if retries > maxRetries {
				result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(nil, "error_parse", nil, ""))
				result.Messages = appendMessages(baseMessages, domain.GuardNote("error_parse"))
				return result
			}
			messages = append(messages, adapter.Message{Role: "system", Content: "Return a valid memory decision JSON object only. Previous decision was invalid: " + err.Error()})
			continue
		}
		if domain.IsNoop(decision) {
			status := domain.NoopStatus(decision)
			if strings.TrimSpace(status) == "" {
				status = "success_none"
			}
			trace := domain.PlannerTrace(decision, status, nil)
			result.Traces = append(result.Traces, trace)
			result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(decision, status, nil, ""))
			result.Messages = appendMessages(baseMessages, domain.GuardNote(status))
			return result
		}
		plannerStatus := domain.PlannerSuccessStatus(decision)
		trace := domain.PlannerTrace(decision, plannerStatus, nil)
		result.Traces = append(result.Traces, trace)
		result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(decision, plannerStatus, nil, ""))

		mutationResult, mutationTrace, err := domain.ApplyDecision(ctx, decision)
		result.Traces = append(result.Traces, mutationTrace)
		if err != nil {
			status := strings.TrimSpace(mutationTrace.Status)
			if status == "" {
				status = "mutation_error"
			}
			result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(decision, plannerStatus, mutationResult, status))
			result.Messages = appendMessages(baseMessages, domain.GuardNote(status))
			return result
		}
		result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(decision, plannerStatus, mutationResult, "success"))
		result.Messages = appendMessages(baseMessages, domain.SuccessNote(decision, mutationResult))
		return result
	}
	result.MetadataUpdates = mergeMetadataUpdates(result.MetadataUpdates, domain.MetadataUpdates(nil, "error_round_limit", nil, ""))
	result.Messages = appendMessages(baseMessages, domain.GuardNote("error_round_limit"))
	return result
}

func appendMessages(messages []adapter.Message, next adapter.Message) []adapter.Message {
	out := append([]adapter.Message{}, messages...)
	return append(out, next)
}

func mergeMetadataUpdates(current map[string]interface{}, next map[string]interface{}) map[string]interface{} {
	if current == nil {
		current = map[string]interface{}{}
	}
	for key, value := range next {
		if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
			continue
		}
		current[key] = value
	}
	return current
}

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	clone := *source
	clone.Messages = append([]adapter.Message{}, source.Messages...)
	clone.Tools = append([]adapter.Tool{}, source.Tools...)
	clone.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		clone.AdditionalParameters = map[string]interface{}{}
		for key, value := range source.AdditionalParameters {
			clone.AdditionalParameters[key] = value
		}
	}
	if source.LogitBias != nil {
		clone.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			clone.LogitBias[key] = value
		}
	}
	return &clone
}

func responseUsage(resp *adapter.ChatResponse) *adapter.Usage {
	if resp == nil || resp.Usage == nil {
		return nil
	}
	usage := *resp.Usage
	return &usage
}

func MergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		copy := *next
		return &copy
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	return current
}

func firstResponseMessage(resp *adapter.ChatResponse) adapter.Message {
	if resp == nil || len(resp.Choices) == 0 {
		return adapter.Message{Role: "assistant"}
	}
	return resp.Choices[0].Message
}

func messageTextFromResponse(message adapter.Message) string {
	switch typed := message.Content.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
