package agentmemoryruntime

import (
	"context"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func RunPreflight(ctx context.Context, req PreflightRequest) PreflightResult {
	result := PreflightResult{MetadataUpdates: map[string]interface{}{}}
	if !ShouldRunDecision(req.LatestUserMessage) {
		trace := PlannerTrace(Decision{Action: "none", Reason: "empty latest user message"}, "skipped_empty_query", nil)
		result.Traces = append(result.Traces, trace)
		result.MetadataUpdates["planner_status"] = "skipped_empty_query"
		result.MetadataUpdates["planner_action"] = "none"
		result.Messages = appendMessages(req.LLMRequest.Messages, GuardNote("skipped_empty_query"))
		return result
	}

	baseMessages := append([]adapter.Message{}, req.LLMRequest.Messages...)
	messages := PlannerMessages(req.State, req.LatestUserMessage, baseMessages)
	retries := 0

	for round := 0; round < maxPlanningRounds; round++ {
		_ = round
		planningReq := cloneChatRequest(req.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = nil
		planningReq.ToolChoice = nil
		temperature := 0.0
		planningReq.Temperature = &temperature
		if req.UseJSONMode {
			planningReq.ResponseFormat = &adapter.ResponseFormat{Type: "json_object"}
		} else {
			planningReq.ResponseFormat = nil
		}
		maxTokens := 700
		planningReq.MaxTokens = &maxTokens

		resp, err := req.LLMClient.AppChat(ctx, req.AppContext, planningReq)
		if err != nil {
			trace := PlannerTrace(Decision{Action: "none", Reason: "planner llm error"}, "error_llm", err)
			result.Traces = append(result.Traces, trace)
			result.MetadataUpdates["planner_status"] = "error_llm"
			result.MetadataUpdates["planner_action"] = "none"
			result.Messages = appendMessages(baseMessages, GuardNote("error_llm"))
			return result
		}
		result.Usage = mergeUsage(result.Usage, responseUsage(resp))
		message := firstResponseMessage(resp)
		decision, err := ParseDecision(messageTextFromResponse(message))
		if err != nil {
			retries++
			trace := PlannerTrace(Decision{Action: "none", Reason: "planner returned invalid JSON"}, "error_parse", err)
			result.Traces = append(result.Traces, trace)
			if retries > maxPlanningRetries {
				result.MetadataUpdates["planner_status"] = "error_parse"
				result.MetadataUpdates["planner_action"] = "none"
				result.Messages = appendMessages(baseMessages, GuardNote("error_parse"))
				break
			}
			messages = append(messages, DecisionRetryMessage(err))
			continue
		}
		if DecisionNoop(decision) {
			result.Traces = append(result.Traces, PlannerTrace(decision, "success_none", nil))
			result.MetadataUpdates["planner_status"] = "success_none"
			result.MetadataUpdates["planner_action"] = "none"
			result.Messages = appendMessages(baseMessages, GuardNote("success_none"))
			break
		}
		plannerStatus := PlannerSuccessStatus(decision)
		result.Traces = append(result.Traces, PlannerTrace(decision, plannerStatus, nil))
		result.MetadataUpdates["planner_status"] = plannerStatus
		result.MetadataUpdates["planner_action"] = decision.Action
		result.MetadataUpdates["planner_key"] = decision.Key

		mutationResult, trace, err := ApplyDecision(ctx, MutationRequest{
			MemoryService:    req.MemoryService,
			WorkspaceID:      req.WorkspaceID,
			AgentID:          req.AgentID,
			UserID:           req.UserID,
			UserScope:        req.UserScope,
			Slots:            req.State.EnabledSlots,
			MutationMetadata: req.MutationMetadata,
			OnToolCallStart:  req.OnToolCallStart,
			OnToolCallEnd:    req.OnToolCallEnd,
		}, decision)
		result.Traces = append(result.Traces, trace)
		if err != nil {
			result.MetadataUpdates["mutation_status"] = trace.Status
			result.MetadataUpdates["mutation_key"] = decision.Key
			result.Messages = appendMessages(baseMessages, GuardNote(trace.Status))
			break
		}
		finalMessages := append([]adapter.Message{}, baseMessages...)
		finalMessages = append(finalMessages, SuccessNote(decision, mutationResult))
		result.Messages = finalMessages
		result.MetadataUpdates["mutation_status"] = "success"
		result.MetadataUpdates["mutation_key"] = StringResultValue(mutationResult, "key")
		break
	}
	return result
}

func appendMessages(messages []adapter.Message, next adapter.Message) []adapter.Message {
	out := append([]adapter.Message{}, messages...)
	return append(out, next)
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

func mergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
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

func TraceSkillIDs(traces []skills.SkillTrace) []string {
	ids := make([]string, 0, len(traces))
	for _, trace := range traces {
		if strings.TrimSpace(trace.SkillID) != "" {
			ids = append(ids, trace.SkillID)
		}
	}
	return ids
}
