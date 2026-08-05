package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func nativeAgentToolsForRun(resolved *skills.ResolvedSkills, toolSet *skills.NativeToolSet, session *skills.NativeSkillSession) []adapter.Tool {
	if session != nil {
		current := session.ToolSet()
		tools := append([]adapter.Tool(nil), current.ProviderTools...)
		return append(tools, skills.NativeControlToolsForSession(
			resolved,
			current.ActiveSkillIDs,
			session.ExposedSkillIDs(),
			session.SearchAvailable(),
		)...)
	}
	if toolSet == nil {
		return skills.NativeControlToolsForSkills(resolved, nil)
	}
	tools := append([]adapter.Tool(nil), toolSet.ProviderTools...)
	tools = append(tools, skills.NativeControlToolsForSkills(resolved, toolSet.ActiveSkillIDs)...)
	return tools
}

func nativeAgentLoopSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"Only active skills have complete instructions and visible business functions. Candidate skills are lightweight metadata until activated.",
		"If a candidate skill is needed, call activate_skills with the smallest relevant set before doing the business work. Use search_skills only when the compact candidate directory omitted the needed capability.",
		"Skill discovery and activation are internal. Do not narrate, summarize, or expose them to the user.",
		"When a business action is needed, call the exposed business function directly. Never call or invent load_skill, call_skill_tool, submit_intermediate_answer, or submit_final_answer.",
		"Every assistant turn that calls any tool must contain tool calls only: leave ordinary assistant content empty. Do not draft, analyze, narrate, or reproduce the requested deliverable beside or before a tool call. Put content needed by the tool only in its arguments.",
		"If an active skill document mentions those legacy wrapper functions, this native-loop instruction overrides that wording; preserve the skill's business rules and call its exposed function instead.",
		"When no more tool calls are needed, provide the complete user-facing final answer as ordinary assistant content.",
		"Use update_plan only for short outcome or step status changes, never for detailed reasoning. Use submit_turn_state when exact working state must survive approval, navigation, refresh, user input, or another continuation boundary.",
		"Use read_skill_reference only when an active skill's instructions require a reference. Use request_user_input only when missing information blocks reliable progress.",
		"All user-visible progress, questions, and final answers must use the language of the user's latest request. Never expose internal protocol details, hidden reasoning, tool aliases, IDs, or bookkeeping.",
		"Never claim an external action succeeded without a matching successful tool result. Do not repeat a failed tool call with unchanged arguments.",
		"Read-only calls may be grouped when useful, but call at most one side-effecting or governed mutation in one assistant turn, then wait for its result.",
	}, "\n")}
}

func nativeReferenceReadContinuationSystemMessage(trace skills.SkillTrace) (adapter.Message, bool) {
	if !strings.EqualFold(strings.TrimSpace(trace.Kind), "reference_read") ||
		!strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return adapter.Message{}, false
	}
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"The requested skill reference is now available in the immediately preceding tool result.",
		"If the user request and source data are sufficient, call the relevant business function now instead of drafting the deliverable in ordinary assistant content.",
		"For a tool-calling turn, leave ordinary assistant content empty and place the complete artifact body only in the function arguments.",
	}, "\n")}, true
}

// NativeAgentLoopSystemMessage exposes the native-loop prompt for the service's
// final context-budget estimate without duplicating its contents.
func NativeAgentLoopSystemMessage() adapter.Message {
	return nativeAgentLoopSystemMessage()
}

func (r *Runner) preloadNativeSkills(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	toolSet *skills.NativeToolSet,
	traces *[]skills.SkillTrace,
) (map[string]struct{}, error) {
	loaded := map[string]struct{}{}
	if toolSet == nil {
		return loaded, nil
	}
	for _, skipped := range toolSet.SkippedSkills {
		trace := skills.SkillTrace{
			Kind:    "skill_load_attempt",
			SkillID: skipped.SkillID,
			Status:  "skipped",
			Arguments: map[string]interface{}{
				"source":          "runtime_preload",
				"outcome":         "skipped",
				"reason_code":     skipped.Reason,
				"tool_name":       skipped.ToolName,
				"budget":          skipped.Budget,
				"required":        skipped.Required,
				"budget_tokens":   skipped.BudgetTokens,
				"required_tokens": skipped.RequiredTokens,
			},
			Error: skipped.Detail,
		}
		*traces = append(*traces, trace)
		r.recordTrace(*traces, trace)
		r.logSkillTrace(ctx, prepared, trace)
	}
	for _, skillID := range toolSet.ActiveSkillIDs {
		startedAt := time.Now()
		doc, trace, err := r.SkillRuntime.LoadSkill(ctx, resolved, skillID)
		trace.DurationMS = time.Since(startedAt).Milliseconds()
		if trace.Arguments == nil {
			trace.Arguments = map[string]interface{}{}
		}
		trace.Arguments["source"] = "runtime_preload"
		if err != nil {
			trace.Status = "error"
			trace.Error = err.Error()
			*traces = append(*traces, trace)
			r.recordTrace(*traces, trace)
			r.logSkillTrace(ctx, prepared, trace)
			return loaded, err
		}
		trace.Result = map[string]interface{}{
			"source":             "runtime_preload",
			"outcome":            "preloaded",
			"instruction_digest": skillInstructionDigest(doc.Instructions),
			"instruction_chars":  len([]rune(strings.TrimSpace(doc.Instructions))),
			"effective_version":  skillInstructionDigest(doc.Instructions),
			"policy_state":       "allowed",
			"access_status":      "authorized",
		}
		loaded[strings.ToLower(strings.TrimSpace(doc.Metadata.ID))] = struct{}{}
		*traces = append(*traces, trace)
		r.recordTrace(*traces, trace)
		r.logSkillTrace(ctx, prepared, trace)
	}
	return loaded, nil
}

type nativeSessionControlResult struct {
	step         skillStepResult
	instructions []adapter.Message
}

func nativeSessionActivationAttemptTrace(attempt skills.NativeSkillActivationAttempt) skills.SkillTrace {
	status := "skipped"
	if attempt.Outcome == "activated" || attempt.Outcome == "already_active" {
		status = "success"
	}
	return skills.SkillTrace{
		Kind:    "skill_load_attempt",
		SkillID: strings.TrimSpace(attempt.SkillID),
		Status:  status,
		Arguments: map[string]interface{}{
			"source":      strings.TrimSpace(attempt.Source),
			"outcome":     strings.TrimSpace(attempt.Outcome),
			"reason_code": strings.TrimSpace(attempt.Reason),
		},
		Error: strings.TrimSpace(attempt.Detail),
	}
}

func (r *Runner) handleNativeSessionControlCall(
	ctx context.Context,
	call adapter.ToolCall,
	session *skills.NativeSkillSession,
) (nativeSessionControlResult, bool) {
	name := strings.TrimSpace(call.Function.Name)
	if name != skills.MetaToolActivateSkills && name != skills.MetaToolSearchSkills {
		return nativeSessionControlResult{}, false
	}
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := invalidToolArgumentsFeedbackTrace(name, err, nil)
		return nativeSessionControlResult{step: recoverableSkillStep(
			trace,
			skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same control tool")),
			false,
			false,
		)}, true
	}
	if session == nil {
		err := fmt.Errorf("%w: native skill session is not configured", ErrInvalidInput)
		trace := plannerFeedbackTrace("", name, err)
		return nativeSessionControlResult{step: recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "provide a final answer using the currently available capabilities")), false, false)}, true
	}
	if name == skills.MetaToolSearchSkills {
		query := stringArg(args, "query")
		if query == "" {
			err := fmt.Errorf("%w: search_skills query is required", ErrInvalidInput)
			trace := plannerFeedbackTrace("", name, err)
			return nativeSessionControlResult{step: recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "retry search_skills with a concise capability query")), false, false)}, true
		}
		matches := session.Search(query, nativeIntegerArg(args, "limit"))
		payload := map[string]interface{}{
			"status":      "success",
			"candidates":  matches,
			"next_action": "activate the smallest relevant candidate set, or answer directly if no skill is needed",
		}
		trace := skills.SkillTrace{
			Kind:     "skill_load_attempt",
			ToolName: skills.MetaToolSearchSkills,
			Status:   "success",
			Arguments: map[string]interface{}{
				"source":  "runtime_activation",
				"outcome": "searched",
				"query":   query,
			},
			Result: map[string]interface{}{"candidate_count": len(matches)},
		}
		return nativeSessionControlResult{step: successfulSkillStep(trace, skills.ToolResultMessage(call.ID, payload), false, false)}, true
	}

	requested := nativeStringSliceArg(args, "skill_ids")
	if len(requested) == 0 || len(requested) > skills.MaxNativeSkillActivationBatch {
		err := fmt.Errorf("%w: activate_skills requires one to three skill ids", ErrInvalidInput)
		trace := plannerFeedbackTrace("", name, err)
		return nativeSessionControlResult{step: recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "retry with one to three candidate skill IDs")), false, false)}, true
	}
	activation := session.Activate(ctx, requested, "runtime_activation")
	payload := map[string]interface{}{
		"status":                   "success",
		"activated_skill_ids":      activation.ActivatedSkillIDs,
		"already_active_skill_ids": activation.AlreadyActiveSkillIDs,
		"skipped_skills":           activation.SkippedSkills,
		"next_action":              "call an activated business function directly, or activate another candidate only if the task still requires it",
	}
	trace := skills.SkillTrace{
		Kind:     "skill_load_attempt",
		ToolName: skills.MetaToolActivateSkills,
		Status:   "success",
		Arguments: map[string]interface{}{
			"source":    "runtime_activation",
			"outcome":   "activation_requested",
			"skill_ids": requested,
		},
		Result: map[string]interface{}{
			"activated_skill_ids":      activation.ActivatedSkillIDs,
			"already_active_skill_ids": activation.AlreadyActiveSkillIDs,
			"skipped_skills":           activation.SkippedSkills,
		},
	}
	step := successfulSkillStep(trace, skills.ToolResultMessage(call.ID, payload), false, false)
	if len(activation.ActivatedSkillIDs) == 0 && len(activation.AlreadyActiveSkillIDs) == 0 {
		trace.Status = "blocked"
		trace.Error = "no requested skill could be activated"
		step = recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, payload), false, false)
	}
	return nativeSessionControlResult{step: step, instructions: activation.InstructionMessages}, true
}

func nativeStringSliceArg(args map[string]interface{}, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	out := make([]string, 0)
	switch values := raw.(type) {
	case []interface{}:
		for _, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
	case []string:
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func nativeIntegerArg(args map[string]interface{}, key string) int {
	value, ok := args[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func nativeExecutionCall(call adapter.ToolCall, toolSet *skills.NativeToolSet) adapter.ToolCall {
	if toolSet == nil {
		return call
	}
	binding, ok := toolSet.ToolBindings[strings.TrimSpace(call.Function.Name)]
	if !ok {
		return call
	}
	arguments := map[string]interface{}{}
	if strings.TrimSpace(call.Function.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			// Preserve malformed JSON so the existing argument feedback path handles it.
			return call
		}
	}
	wrapper := map[string]interface{}{
		"skill_id":  binding.SkillID,
		"tool_name": binding.ToolName,
		"arguments": arguments,
	}
	encoded, err := json.Marshal(wrapper)
	if err != nil {
		return call
	}
	call.Function.Name = skills.MetaToolCallSkillTool
	call.Function.Arguments = string(encoded)
	return call
}

func nativeExecutionCalls(calls []adapter.ToolCall, toolSet *skills.NativeToolSet) []adapter.ToolCall {
	out := make([]adapter.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, nativeExecutionCall(call, toolSet))
	}
	return out
}

func nativeSessionControlCallsOnly(calls []adapter.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		switch strings.TrimSpace(call.Function.Name) {
		case skills.MetaToolActivateSkills, skills.MetaToolSearchSkills:
			continue
		default:
			return false
		}
	}
	return true
}

func nativeForbiddenProtocolTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case skills.MetaToolLoadSkill,
		skills.MetaToolCallSkillTool,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolFinalAnswer:
		return true
	default:
		return false
	}
}

func nativeForbiddenProtocolToolStep(callID string, toolName string) skillStepResult {
	err := fmt.Errorf("%w: legacy protocol tool %s is not available in the native agent loop", ErrInvalidInput, strings.TrimSpace(toolName))
	trace := plannerFeedbackTrace("", toolName, err)
	trace.Arguments["reason_code"] = "native_protocol_tool_unavailable"
	trace.Arguments["next_step"] = "call_exposed_business_tool_directly"
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(
		err,
		"Call one of the business functions exposed in this request directly, or provide the final answer as ordinary assistant content.",
	)), false, false)
}

func nativeAgentOutputError(err error) error {
	if err == nil {
		return nil
	}
	var termination *PlanningTerminationError
	if errors.As(err, &termination) && termination != nil {
		switch strings.ToLower(strings.TrimSpace(termination.Reason)) {
		case "length", "max_tokens":
			return fmt.Errorf("%w: %v", ErrAgentOutputTruncated, err)
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "ended without a terminal signal") {
		return fmt.Errorf("%w: %v", ErrAgentOutputTruncated, err)
	}
	return err
}
