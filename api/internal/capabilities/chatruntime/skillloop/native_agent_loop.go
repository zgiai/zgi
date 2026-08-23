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

const planExpectedActionServerProjectionKey = "_server_projected_tool_name"
const planExpectedActionServerBindingFingerprintKey = "_server_projected_binding_fingerprint"

func nativeAgentToolsForRun(resolved *skills.ResolvedSkills, toolSet *skills.NativeToolSet, session *skills.NativeSkillSession, runtimeTools []RuntimeTool) []adapter.Tool {
	runtimeDefinitions := make([]adapter.Tool, 0, len(runtimeTools))
	for _, runtimeTool := range runtimeTools {
		if strings.TrimSpace(runtimeTool.Definition.Function.Name) != "" && runtimeTool.Handler != nil {
			runtimeDefinitions = append(runtimeDefinitions, runtimeTool.Definition)
		}
	}
	if session != nil {
		current := session.ToolSet()
		tools := append([]adapter.Tool(nil), current.ProviderTools...)
		tools = append(tools, runtimeDefinitions...)
		return append(tools, skills.NativeControlToolsForSession(
			resolved,
			current.ActiveSkillIDs,
			session.ExposedSkillIDs(),
			session.SearchAvailable(),
		)...)
	}
	if toolSet == nil {
		return append(runtimeDefinitions, skills.NativeControlToolsForSkills(resolved, nil)...)
	}
	tools := append([]adapter.Tool(nil), toolSet.ProviderTools...)
	tools = append(tools, runtimeDefinitions...)
	tools = append(tools, skills.NativeControlToolsForSkills(resolved, toolSet.ActiveSkillIDs)...)
	return tools
}

func runtimeToolForCall(runtimeTools []RuntimeTool, call adapter.ToolCall) (RuntimeTool, bool) {
	name := strings.TrimSpace(call.Function.Name)
	for _, runtimeTool := range runtimeTools {
		if runtimeTool.Handler != nil && strings.EqualFold(strings.TrimSpace(runtimeTool.Definition.Function.Name), name) {
			return runtimeTool, true
		}
	}
	return RuntimeTool{}, false
}

func handleRuntimeToolCall(ctx context.Context, call adapter.ToolCall, runtimeTool RuntimeTool) skillStepResult {
	result := runtimeTool.Handler(ctx, call)
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "success"
		if result.Error != nil {
			status = "failed"
		}
	}
	trace := skills.SkillTrace{
		Kind:      "tool_call",
		SkillID:   strings.TrimSpace(runtimeTool.SkillID),
		ToolName:  strings.TrimSpace(runtimeTool.Definition.Function.Name),
		Status:    status,
		Arguments: result.Arguments,
		Result:    result.Result,
	}
	if result.Error != nil {
		trace.Error = status
	}
	message := skills.ToolResultMessage(call.ID, result.Result)
	step := successfulSkillStep(trace, message, false, true)
	step.toolResult = result.Result
	if result.Recoverable {
		step = recoverableSkillStep(trace, message, false, true)
		step.toolResult = result.Result
	}
	return step
}

func nativeAgentLoopSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"Only active skills have complete instructions and visible business functions. Candidate skills are lightweight metadata until activated.",
		"Answer knowledge questions, explanations, translations, rewrites, summaries, and tasks fully supported by the current conversation context directly without activating a skill.",
		"Activate or search for skills only when the user outcome genuinely requires an external read, generation, mutation, governed action, or page operation. Never activate, search, update a plan, or submit state merely to inspect capabilities or prepare an answer.",
		"If a candidate skill is needed, call activate_skills with the smallest relevant set before doing the business work. Use search_skills only when the compact candidate directory omitted the needed capability.",
		"Skill discovery and activation are internal. Do not name, narrate, summarize, or expose that mechanism to the user. A useful user-facing process note may accompany the same turn when it describes the actual task stage rather than the internal preparation.",
		"When a business action is needed, call the exposed business function directly. Never call or invent load_skill, call_skill_tool, submit_intermediate_answer, or submit_final_answer.",
		"Before the first projected external Action, call update_plan earlier in the same tool-call response. Keep every required outcome phase and give each tool phase an expected_action whose tool_name is the exact exposed business function name; do not guess a hidden skill_id. Include stable target values when known, using target_arguments keyed by the exact business-argument path for nested targets. The runtime binds that alias to its server-owned integration, Action identity, and allowed target paths. A read prerequisite may run only after the final expected Action is in this ledger. If multiple phases call the same projected Action for the same target, pass that phase's exact plan_phase_id to each direct call.",
		"For complex or multi-step work, before the first business stage include one brief user-visible process note in ordinary assistant content before the business function call. Add another note only after important evidence arrives, when the work changes stage, when the approach changes, or when recovering from an error.",
		"A process note must use the language of the user's latest request and state the current judgment or evidence plus the result the next action will produce. Do not merely repeat the request.",
		"Keep each process note concise, normally one to four short sentences and about 60 to 180 estimated tokens. Never exceed 384 estimated tokens for one note. Across one user task, produce at most eight notes and about 1920 estimated tokens total.",
		"Do not add a process note for one fast business call, repeated calls of the same kind, or an ordinary direct answer. Internal control work alone is not worth announcing, but when the same stage has a useful task-level update, describe only the user-visible work and outcome without naming the control operation.",
		"Process notes must not reveal hidden reasoning, skill names, function or tool names, parameters, IDs, JSON, protocols, or hidden plans. Put artifact bodies and all content required by a function only in its arguments.",
		"If an active skill document mentions those legacy wrapper functions, this native-loop instruction overrides that wording; preserve the skill's business rules and call its exposed function instead.",
		"When no more tool calls are needed, provide the complete user-facing final answer as ordinary assistant content. For complex work, briefly state the completed result, the key basis for the work, and any material limitation or unfinished item.",
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
		"Keep the complete artifact body only in the function arguments. A brief process note is allowed only when the shared native-loop process-note policy calls for one.",
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
	call = nativeCanonicalizeProjectedActionPlanCall(call, toolSet)
	binding, _, ok := nativeToolBindingForCall(toolSet, call.Function.Name)
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
	arguments = nativeMaterializeProjectedActionDefaults(arguments, binding.DefaultArguments)
	arguments = nativeCanonicalizeProjectedActionArguments(arguments, binding.OptionalTargets)
	planPhaseID := ""
	if phaseArgument := strings.TrimSpace(binding.PlanPhaseArgument); phaseArgument != "" {
		planPhaseID = strings.TrimSpace(evidenceStringFromAny(arguments[phaseArgument]))
		delete(arguments, phaseArgument)
	}
	executionArguments := arguments
	if binding.ArgumentEnvelope != "" {
		executionArguments = make(map[string]interface{}, len(binding.FixedArguments)+1)
		for key, value := range binding.FixedArguments {
			executionArguments[key] = value
		}
		// The model-visible object is always nested after fixed values are
		// copied, so it cannot replace integration/action/revision metadata.
		executionArguments[binding.ArgumentEnvelope] = arguments
	} else if len(binding.FixedArguments) > 0 {
		executionArguments = make(map[string]interface{}, len(arguments)+len(binding.FixedArguments))
		for key, value := range arguments {
			executionArguments[key] = value
		}
		for key, value := range binding.FixedArguments {
			executionArguments[key] = value
		}
	}
	wrapper := map[string]interface{}{
		"skill_id":  binding.SkillID,
		"tool_name": binding.ToolName,
		"arguments": executionArguments,
	}
	if planPhaseID != "" {
		wrapper["plan_phase_id"] = planPhaseID
	}
	encoded, err := json.Marshal(wrapper)
	if err != nil {
		return call
	}
	call.Function.Name = skills.MetaToolCallSkillTool
	call.Function.Arguments = string(encoded)
	return call
}

func nativeCanonicalizeProjectedActionPlanCall(call adapter.ToolCall, toolSet *skills.NativeToolSet) adapter.ToolCall {
	if toolSet == nil || !strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolUpdatePlan) {
		return call
	}
	arguments := map[string]interface{}{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return call
	}
	phases := make([]interface{}, 0)
	for _, key := range []string{"plan", "phase_updates"} {
		if values, ok := arguments[key].([]interface{}); ok {
			phases = append(phases, values...)
		}
	}
	if len(phases) == 0 {
		return call
	}
	changed := false
	for _, rawPhase := range phases {
		phase, _ := rawPhase.(map[string]interface{})
		expected, _ := phase["expected_action"].(map[string]interface{})
		if len(expected) == 0 {
			continue
		}
		requestedToolName := strings.TrimSpace(evidenceStringFromAny(expected["tool_name"]))
		if strings.EqualFold(requestedToolName, "execute_action") {
			requestedToolName = strings.TrimSpace(evidenceStringFromAny(expected["projected_tool_name"]))
		}
		// This marker is server-owned. Strip any model copy before resolving the
		// current request's binding, but leave ordinary expected_action fields
		// untouched unless the selected name is a real projected external Action.
		if _, submittedServerMarker := expected[planExpectedActionServerProjectionKey]; submittedServerMarker {
			delete(expected, planExpectedActionServerProjectionKey)
			changed = true
		}
		if _, submittedBindingFingerprint := expected[planExpectedActionServerBindingFingerprintKey]; submittedBindingFingerprint {
			delete(expected, planExpectedActionServerBindingFingerprintKey)
			changed = true
		}
		if _, submittedProjectedName := expected["projected_tool_name"]; submittedProjectedName {
			delete(expected, "projected_tool_name")
			changed = true
		}
		binding, providerName, exists := nativeToolBindingForCall(toolSet, requestedToolName)
		integrationID, actionID, projected := nativeProjectedExternalActionIdentity(binding)
		if !exists || !projected || strings.TrimSpace(binding.BindingFingerprint) == "" {
			continue
		}
		submittedTargetArguments := copyStringAnyMap(evidenceMapFromAny(expected["target_arguments"]))
		delete(expected, "target_arguments")
		target := copyStringAnyMap(evidenceMapFromAny(expected["target"]))
		if target == nil {
			target = map[string]interface{}{}
		}
		// The alias only selects a server-created binding. Model values cannot
		// replace the integration or Action identity captured in that binding.
		target["integration_id"] = integrationID
		target["action_id"] = actionID
		expected["skill_id"] = skills.SkillExternalApps
		expected["tool_name"] = "execute_action"
		expected["target"] = target
		expected["projected_tool_name"] = providerName
		expected[planExpectedActionServerProjectionKey] = providerName
		expected[planExpectedActionServerBindingFingerprintKey] = strings.TrimSpace(binding.BindingFingerprint)
		targetArguments := map[string]interface{}{}
		for _, path := range binding.TargetArgumentPaths {
			value := operationPlanArgumentPathValue(target, path)
			if value == "" {
				value = strings.TrimSpace(evidenceStringFromAny(target[path]))
			}
			if value == "" {
				value = strings.TrimSpace(evidenceStringFromAny(submittedTargetArguments[path]))
			}
			if value == "" {
				value = operationPlanArgumentPathValue(binding.DefaultArguments, path)
			}
			if value == "" {
				value = strings.TrimSpace(evidenceStringFromAny(binding.DefaultArguments[path]))
			}
			if value != "" {
				targetArguments[path] = value
			}
		}
		targetArguments = nativeCanonicalizeProjectedActionArguments(targetArguments, binding.OptionalTargets)
		if len(targetArguments) > 0 {
			expected["target_arguments"] = targetArguments
		}
		changed = true
	}
	if !changed {
		return call
	}
	encoded, err := json.Marshal(arguments)
	if err == nil {
		call.Function.Arguments = string(encoded)
	}
	return call
}

func nativeMaterializeProjectedActionDefaults(
	arguments map[string]interface{},
	defaults map[string]interface{},
) map[string]interface{} {
	if len(defaults) == 0 {
		return arguments
	}
	out := make(map[string]interface{}, len(arguments)+len(defaults))
	for key, value := range defaults {
		out[key] = value
	}
	for key, value := range arguments {
		// Explicit model arguments retain normal JSON Schema semantics. The
		// server-owned default applies only when a property is omitted.
		out[key] = value
	}
	return out
}

func nativeCanonicalizeProjectedActionArguments(
	arguments map[string]interface{},
	optionalTargets []skills.NativeExternalActionOptionalTargetArgument,
) map[string]interface{} {
	if len(arguments) == 0 || len(optionalTargets) == 0 {
		return arguments
	}
	out := copyStringAnyMap(arguments)
	for _, target := range optionalTargets {
		if !target.DiscardWhenMatched {
			continue
		}
		path := strings.TrimSpace(target.Path)
		whenArgument := strings.TrimSpace(target.WhenArgument)
		if path == "" || whenArgument == "" {
			continue
		}
		actual, exists := out[whenArgument]
		if !exists || !nativeProjectedActionScalarEqual(actual, target.WhenEquals) {
			continue
		}
		// The selected conditional branch makes this alternate target
		// meaningless (for example recipient_id when recipient_type=self).
		// Strip it before plan matching, approval and provider execution so it
		// cannot split one real operation into several synthetic identities.
		delete(out, path)
	}
	return out
}

func nativeProjectedActionScalarEqual(left, right interface{}) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func nativeToolBindingForCall(toolSet *skills.NativeToolSet, name string) (skills.NativeToolBinding, string, bool) {
	if toolSet == nil {
		return skills.NativeToolBinding{}, "", false
	}
	name = strings.TrimSpace(name)
	if binding, ok := toolSet.ToolBindings[name]; ok {
		return binding, name, true
	}
	for providerName, binding := range toolSet.ToolBindings {
		if strings.EqualFold(strings.TrimSpace(providerName), name) {
			return binding, providerName, true
		}
	}
	return skills.NativeToolBinding{}, "", false
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
