package usermemoryruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/memoryplanner"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func RunPreflight(ctx context.Context, req PreflightRequest) PreflightResult {
	domain := userMemoryPlannerDomain{req: req}
	common := memoryplanner.Run(ctx, memoryplanner.Request{
		LatestUserMessage: req.LatestUserMessage,
		LLMRequest:        req.LLMRequest,
		LLMClient:         req.LLMClient,
		AppContext:        req.AppContext,
		UseJSONMode:       req.UseJSONMode,
	}, domain)
	return PreflightResult{
		Usage:           common.Usage,
		Messages:        common.Messages,
		Traces:          common.Traces,
		MetadataUpdates: common.MetadataUpdates,
	}
}

type userMemoryPlannerDomain struct {
	req PreflightRequest
}

func (d userMemoryPlannerDomain) PlannerMessages(baseMessages []adapter.Message) []adapter.Message {
	return PlannerMessages(d.req.State, d.req.LatestUserMessage, baseMessages)
}

func (d userMemoryPlannerDomain) ParseDecision(raw string) (interface{}, error) {
	return ParseDecision(raw)
}

func (d userMemoryPlannerDomain) IsNoop(decision interface{}) bool {
	typed, ok := decision.(Decision)
	return !ok || DecisionNoop(typed)
}

func (d userMemoryPlannerDomain) NoopStatus(decision interface{}) string {
	typed, ok := decision.(Decision)
	if ok && typed.Action == ActionAskConfirmation {
		return "requires_confirmation"
	}
	return "success_none"
}

func (d userMemoryPlannerDomain) PlannerSuccessStatus(decision interface{}) string {
	typed, ok := decision.(Decision)
	if !ok {
		return "success_none"
	}
	return PlannerSuccessStatus(typed)
}

func (d userMemoryPlannerDomain) PlannerTrace(decision interface{}, status string, err error) skills.SkillTrace {
	typed, _ := decision.(Decision)
	return PlannerTrace(typed, status, err)
}

func (d userMemoryPlannerDomain) ApplyDecision(ctx context.Context, decision interface{}) (map[string]interface{}, skills.SkillTrace, error) {
	typed, _ := decision.(Decision)
	return ApplyDecision(ctx, d.req, typed)
}

func (d userMemoryPlannerDomain) SuccessNote(decision interface{}, result map[string]interface{}) adapter.Message {
	typed, _ := decision.(Decision)
	return SuccessNote(typed, result)
}

func (d userMemoryPlannerDomain) GuardNote(status string) adapter.Message {
	return GuardNote(status)
}

func (d userMemoryPlannerDomain) MetadataUpdates(decision interface{}, plannerStatus string, result map[string]interface{}, mutationStatus string) map[string]interface{} {
	typed, _ := decision.(Decision)
	updates := map[string]interface{}{"planner_status": plannerStatus}
	if strings.TrimSpace(typed.Action) == "" {
		updates["planner_action"] = ActionNone
	} else {
		updates["planner_action"] = typed.Action
	}
	if strings.TrimSpace(typed.EntryID) != "" {
		updates["planner_entry_id"] = typed.EntryID
	}
	if strings.TrimSpace(mutationStatus) != "" {
		updates["mutation_status"] = mutationStatus
	}
	if entryID := StringResultValue(result, "entry_id"); entryID != "" {
		updates["mutation_entry_id"] = entryID
	}
	if mutationError := StringResultValue(result, "error"); mutationError != "" {
		updates["mutation_error"] = truncateRunes(mutationError, 240)
	}
	return updates
}

func SuccessNote(decision Decision, result map[string]interface{}) adapter.Message {
	action := strings.TrimSpace(decision.Action)
	if resultAction := StringResultValue(result, "action"); resultAction != "" {
		action = resultAction
	}
	content := fmt.Sprintf("Internal user memory note: user memory %s succeeded this turn. The final answer may briefly confirm this memory change if relevant. Do not mention tools, planner, or internal memory process.", action)
	return adapter.Message{Role: "system", Content: content}
}

func GuardNote(status string) adapter.Message {
	if strings.TrimSpace(status) == "requires_confirmation" {
		return adapter.Message{Role: "system", Content: "Internal user memory note: memory was not changed because confirmation is required. Ask a brief clarification question before changing saved memory. Do not say anything was remembered, saved, updated, deleted, or forgotten."}
	}
	return adapter.Message{Role: "system", Content: "Internal user memory note: no user memory mutation succeeded in this turn (status: " + strings.TrimSpace(status) + "). The final answer must not say memory was remembered, recorded, saved, updated, deleted, cleared, or forgotten."}
}

func StringResultValue(result map[string]interface{}, key string) string {
	if len(result) == 0 {
		return ""
	}
	value, ok := result[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
