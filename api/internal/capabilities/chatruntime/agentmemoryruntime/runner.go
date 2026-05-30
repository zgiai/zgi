package agentmemoryruntime

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/memoryplanner"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func RunPreflight(ctx context.Context, req PreflightRequest) PreflightResult {
	domain := agentMemoryPlannerDomain{req: req}
	common := memoryplanner.Run(ctx, memoryplanner.Request{
		LatestUserMessage: req.LatestUserMessage,
		LLMRequest:        req.LLMRequest,
		LLMClient:         req.LLMClient,
		AppContext:        req.AppContext,
		UseJSONMode:       req.UseJSONMode,
		MaxPlanningRounds: maxPlanningRounds,
		MaxRetries:        maxPlanningRetries,
	}, domain)
	return PreflightResult{
		Usage:           common.Usage,
		Messages:        common.Messages,
		Traces:          common.Traces,
		MetadataUpdates: common.MetadataUpdates,
	}
}

type agentMemoryPlannerDomain struct {
	req PreflightRequest
}

func (d agentMemoryPlannerDomain) PlannerMessages(baseMessages []adapter.Message) []adapter.Message {
	return PlannerMessages(d.req.State, d.req.LatestUserMessage, baseMessages)
}

func (d agentMemoryPlannerDomain) ParseDecision(raw string) (interface{}, error) {
	return ParseDecision(raw)
}

func (d agentMemoryPlannerDomain) IsNoop(decision interface{}) bool {
	typed, ok := decision.(Decision)
	return !ok || DecisionNoop(typed)
}

func (d agentMemoryPlannerDomain) NoopStatus(decision interface{}) string {
	return "success_none"
}

func (d agentMemoryPlannerDomain) PlannerSuccessStatus(decision interface{}) string {
	typed, ok := decision.(Decision)
	if !ok {
		return "success_none"
	}
	return PlannerSuccessStatus(typed)
}

func (d agentMemoryPlannerDomain) PlannerTrace(decision interface{}, status string, err error) skills.SkillTrace {
	typed, _ := decision.(Decision)
	return PlannerTrace(typed, status, err)
}

func (d agentMemoryPlannerDomain) ApplyDecision(ctx context.Context, decision interface{}) (map[string]interface{}, skills.SkillTrace, error) {
	typed, _ := decision.(Decision)
	return ApplyDecision(ctx, MutationRequest{
		MemoryService:    d.req.MemoryService,
		WorkspaceID:      d.req.WorkspaceID,
		AgentID:          d.req.AgentID,
		UserID:           d.req.UserID,
		UserScope:        d.req.UserScope,
		Slots:            d.req.State.EnabledSlots,
		MutationMetadata: d.req.MutationMetadata,
		OnToolCallStart:  d.req.OnToolCallStart,
		OnToolCallEnd:    d.req.OnToolCallEnd,
	}, typed)
}

func (d agentMemoryPlannerDomain) SuccessNote(decision interface{}, result map[string]interface{}) adapter.Message {
	typed, _ := decision.(Decision)
	return SuccessNote(typed, result)
}

func (d agentMemoryPlannerDomain) GuardNote(status string) adapter.Message {
	return GuardNote(status)
}

func (d agentMemoryPlannerDomain) MetadataUpdates(decision interface{}, plannerStatus string, result map[string]interface{}, mutationStatus string) map[string]interface{} {
	typed, _ := decision.(Decision)
	updates := map[string]interface{}{"planner_status": plannerStatus}
	if DecisionNoop(typed) {
		updates["planner_action"] = "none"
	} else {
		updates["planner_action"] = typed.Action
		updates["planner_key"] = typed.Key
	}
	if strings.TrimSpace(mutationStatus) != "" {
		updates["mutation_status"] = mutationStatus
	}
	if resultKey := StringResultValue(result, "key"); resultKey != "" {
		updates["mutation_key"] = resultKey
	} else if strings.TrimSpace(typed.Key) != "" && strings.TrimSpace(mutationStatus) != "" {
		updates["mutation_key"] = typed.Key
	}
	return updates
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
	return memoryplanner.MergeUsage(current, next)
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
