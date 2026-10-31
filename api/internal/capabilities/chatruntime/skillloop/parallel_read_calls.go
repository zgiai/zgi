package skillloop

import (
	"context"
	"strings"

	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// maxParallelReadCalls bounds the read-only calls of one round that run at
// the same time.
const maxParallelReadCalls = 4

// prefetchedCall is a read-only tool call started before its turn in the
// round came: a skill tool call, or a read-only runtime tool call.
type prefetchedCall struct {
	skillID, toolName string
	done              chan struct{}
	invocation        *skills.ToolInvocationResult
	err               error
	runtime           *skillStepResult
}

// prefetchReadOnlyCalls starts the read-only calls at the head of calls at
// once when there are at least two (ADR 0036): skill tool calls, and runtime
// tool calls whose tool is marked ReadOnly. The first call that is not such a
// read ends the group, so no read starts before a change the model asked for
// earlier; the round prefetches the next group when it reaches it. It returns
// the started calls and how many calls the group covered. The round still
// handles its
// calls one by one in the model's order, with every check, event, count and
// checkpoint as before; a call that was started here only waits for its
// result instead of running then. A call is started here only if the
// sequential pass would run it as it stands: an active skill, a tool that
// changes nothing, allowed by the organization's policy, and not held back
// for a plan update or an earlier failure. A started call that the pass then
// does not reach, because an earlier call paused the turn, is a read whose
// result is dropped.
func (r *Runner) prefetchReadOnlyCalls(
	ctx context.Context,
	req RunRequest,
	resolved *skills.ResolvedSkills,
	toolSet *skills.NativeToolSet,
	loadedSkills map[string]struct{},
	failedCalls map[string]string,
	calls []adapter.ToolCall,
) (map[string]*prefetchedCall, int) {
	type candidate struct {
		callID, skillID, toolName, planPhaseID string
		arguments                              map[string]interface{}
		call                                   adapter.ToolCall
		runtimeTool                            *RuntimeTool
	}
	candidates := make([]candidate, 0, len(calls))
	for _, call := range calls {
		if runtimeTool, runtimeCall := runtimeToolForCall(req.RuntimeTools, call); runtimeCall {
			// The round runs a runtime tool's handler as it stands, so a
			// read-only one can start now.
			if !runtimeTool.ReadOnly || strings.TrimSpace(call.ID) == "" {
				break
			}
			candidates = append(candidates, candidate{callID: call.ID, call: call, runtimeTool: &runtimeTool})
			continue
		}
		if r.SkillRuntime == nil {
			break
		}
		execution := nativeExecutionCall(call, toolSet)
		if strings.TrimSpace(execution.Function.Name) != skills.MetaToolCallSkillTool || strings.TrimSpace(call.ID) == "" {
			break
		}
		args, err := skills.ParseArguments(execution.Function.Arguments)
		if err != nil || boolArg(args, "_invalid_tool_arguments") {
			break
		}
		skillID := strings.TrimSpace(stringArg(args, "skill_id"))
		toolName := strings.TrimSpace(stringArg(args, "tool_name"))
		if _, active := loadedSkills[strings.ToLower(skillID)]; !active || isAgentWorkflowRunTool(skillID, toolName) {
			break
		}
		doc, ok := resolved.Get(skillID)
		if !ok || doc == nil || !skills.ToolIsReadOnly(*doc, toolName) || userInputPlanRevisionRequiredForTool(req, skillID, toolName) {
			break
		}
		toolArgs := mapArg(args, "arguments")
		if _, _, _, key := skillToolCallIdentityForCall(resolved, loadedSkills, execution); key != "" && strings.TrimSpace(failedCalls[key]) != "" {
			break
		}
		if req.AuthorizeSkillStep != nil {
			if allowed, err := req.AuthorizeSkillStep(ctx, skillID); err != nil || !allowed {
				break
			}
		}
		candidates = append(candidates, candidate{
			callID: call.ID, skillID: skillID, toolName: toolName,
			planPhaseID: strings.TrimSpace(stringArg(args, "plan_phase_id")), arguments: toolArgs,
		})
	}
	if len(candidates) < 2 {
		return nil, len(candidates)
	}
	prefetched := make(map[string]*prefetchedCall, len(candidates))
	slots := make(chan struct{}, maxParallelReadCalls)
	for _, item := range candidates {
		call := &prefetchedCall{skillID: item.skillID, toolName: item.toolName, done: make(chan struct{})}
		prefetched[item.callID] = call
		go func(item candidate, call *prefetchedCall) {
			defer close(call.done)
			slots <- struct{}{}
			defer func() { <-slots }()
			if item.runtimeTool != nil {
				result := handleRuntimeToolCall(ctx, item.call, *item.runtimeTool)
				call.runtime = &result
				return
			}
			callCtx := connectordispatch.WithActionPlanPhase(ctx, item.planPhaseID)
			call.invocation, call.err = r.SkillRuntime.CallSkillTool(callCtx, resolved, item.skillID, item.toolName, item.arguments, req.ExecutionContext, item.callID)
		}(item, call)
	}
	return prefetched, len(candidates)
}

// callSkillTool runs a skill tool call, or waits for it when the round
// already started it (prefetchReadOnlyCalls).
func (r *Runner) callSkillTool(
	ctx context.Context,
	resolved *skills.ResolvedSkills,
	skillID, toolName string,
	arguments map[string]interface{},
	execCtx skills.ExecutionContext,
	callID string,
) (*skills.ToolInvocationResult, error) {
	if call := r.prefetchedCalls[strings.TrimSpace(callID)]; call != nil &&
		strings.EqualFold(call.skillID, strings.TrimSpace(skillID)) && strings.EqualFold(call.toolName, strings.TrimSpace(toolName)) {
		delete(r.prefetchedCalls, strings.TrimSpace(callID))
		select {
		case <-call.done:
			return call.invocation, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.SkillRuntime.CallSkillTool(ctx, resolved, skillID, toolName, arguments, execCtx, callID)
}

// callRuntimeTool runs a runtime tool call, or waits for it when the round
// already started it (prefetchReadOnlyCalls).
func (r *Runner) callRuntimeTool(ctx context.Context, call adapter.ToolCall, runtimeTool RuntimeTool) skillStepResult {
	callID := strings.TrimSpace(call.ID)
	if prefetched := r.prefetchedCalls[callID]; prefetched != nil && prefetched.skillID == "" && prefetched.toolName == "" {
		delete(r.prefetchedCalls, callID)
		select {
		case <-prefetched.done:
			if prefetched.runtime != nil {
				return *prefetched.runtime
			}
		case <-ctx.Done():
		}
	}
	return handleRuntimeToolCall(ctx, call, runtimeTool)
}
