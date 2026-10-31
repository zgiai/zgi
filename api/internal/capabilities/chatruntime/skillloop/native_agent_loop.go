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

func nativeAgentToolsForRun(resolved *skills.ResolvedSkills, toolSet *skills.NativeToolSet, session *skills.NativeSkillSession, runtimeTools []RuntimeTool, omitPlanning bool, unified ...bool) []adapter.Tool {
	if len(unified) > 0 && unified[0] {
		return unifiedAgentToolsForRun(resolved, toolSet, session, runtimeTools, omitPlanning)
	}
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
		return append(tools, skills.NativeControlToolsForRun(
			resolved,
			current.ActiveSkillIDs,
			session.OfferedSkillIDs(),
			session.SearchAvailable(),
			!omitPlanning,
		)...)
	}
	if toolSet == nil {
		return append(runtimeDefinitions, skills.NativeControlToolsForRun(resolved, nil, nil, false, !omitPlanning)...)
	}
	tools := append([]adapter.Tool(nil), toolSet.ProviderTools...)
	tools = append(tools, runtimeDefinitions...)
	tools = append(tools, skills.NativeControlToolsForRun(resolved, toolSet.ActiveSkillIDs, nil, false, !omitPlanning)...)
	return tools
}

// unifiedAgentToolsForRun is the tool list of a run in unified mode (ADR
// 0044): resident tools, connector operations, runtime tools and protocol
// tools stay native, and the tools of activatable skills are reached through
// the unified tool. Activation changes none of it.
func unifiedAgentToolsForRun(resolved *skills.ResolvedSkills, toolSet *skills.NativeToolSet, session *skills.NativeSkillSession, runtimeTools []RuntimeTool, omitPlanning bool) []adapter.Tool {
	current := skills.NativeToolSet{}
	if session != nil {
		current = session.ToolSet()
	} else if toolSet != nil {
		current = *toolSet
	}
	tools := make([]adapter.Tool, 0, len(current.ProviderTools)+len(runtimeTools)+6)
	for _, tool := range current.ProviderTools {
		if binding, ok := current.ToolBindings[tool.Function.Name]; !ok || skills.UnifiedDispatchExposesTool(binding) {
			tools = append(tools, tool)
		}
	}
	if skills.HasNonResidentSkill(resolved) {
		tools = append(tools, skills.UnifiedSkillTool())
	}
	for _, runtimeTool := range runtimeTools {
		if strings.TrimSpace(runtimeTool.Definition.Function.Name) != "" && runtimeTool.Handler != nil {
			tools = append(tools, runtimeTool.Definition)
		}
	}
	// References are offered for every skill from the start, so activation
	// does not add read_skill_reference later in the turn.
	offered, search := []string(nil), false
	if session != nil {
		offered, search = session.OfferedSkillIDs(), session.SearchAvailable()
	}
	var everySkill []string
	if resolved != nil {
		everySkill = resolved.SkillIDs()
	}
	return append(tools, skills.NativeControlToolsForRun(resolved, everySkill, offered, search, !omitPlanning)...)
}

// translateUnifiedSkillCalls turns each unified call into the native call it
// names, keeping its ID, so the rest of the loop handles it as a direct call:
// governance, prefetching and traces are unchanged (ADR 0044). A call that
// names no tool stays unified and is answered with a recoverable error.
func translateUnifiedSkillCalls(calls []adapter.ToolCall, toolSet *skills.NativeToolSet, resolved *skills.ResolvedSkills) []adapter.ToolCall {
	out := make([]adapter.ToolCall, len(calls))
	for index, call := range calls {
		out[index] = call
		if strings.TrimSpace(call.Function.Name) != skills.UnifiedSkillToolName {
			continue
		}
		var args struct {
			Command string                 `json:"command"`
			Input   map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			continue
		}
		fields := strings.Fields(strings.ReplaceAll(args.Command, "/", " "))
		if len(fields) != 2 {
			continue
		}
		skillID, toolName := strings.ToLower(fields[0]), fields[1]
		name := unifiedProviderToolName(toolSet, resolved, skillID, toolName)
		if name == "" {
			continue
		}
		if args.Input == nil {
			args.Input = map[string]interface{}{}
		}
		encoded, err := json.Marshal(args.Input)
		if err != nil {
			continue
		}
		out[index].Function.Name = name
		out[index].Function.Arguments = string(encoded)
	}
	return out
}

// unifiedProviderToolName is the function name of a skill tool: its projected
// name when the skill is active, or the tool name of a candidate skill, which
// a direct call activates.
func unifiedProviderToolName(toolSet *skills.NativeToolSet, resolved *skills.ResolvedSkills, skillID string, toolName string) string {
	if toolSet != nil {
		for name, binding := range toolSet.ToolBindings {
			if strings.EqualFold(binding.SkillID, skillID) && binding.ToolName == toolName {
				return name
			}
		}
	}
	if resolved == nil {
		return ""
	}
	doc, ok := resolved.Get(skillID)
	if !ok {
		return ""
	}
	for _, definition := range doc.Tools {
		if strings.TrimSpace(definition.Name) == toolName {
			return toolName
		}
	}
	return ""
}

func unifiedSkillCallErrorStep(callID string) skillStepResult {
	err := fmt.Errorf("%w: zgi command must name an available skill and tool, such as \"file-generator generate_file\"", ErrInvalidInput)
	trace := plannerFeedbackTrace("", skills.UnifiedSkillToolName, err)
	trace.Arguments["reason_code"] = "unified_command_unknown"
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(
		err,
		"Activate the candidate skill to see its tools, then call zgi with command \"<skill_id> <tool_name>\" and input matching the tool's schema.",
	)), false, false)
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
	startedAt := time.Now()
	result := runtimeTool.Handler(ctx, call)
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "success"
		if result.Error != nil {
			status = "failed"
		}
	}
	trace := skills.SkillTrace{
		InvocationID: call.ID,
		Kind:         "tool_call",
		SkillID:      strings.TrimSpace(runtimeTool.SkillID),
		ToolName:     strings.TrimSpace(runtimeTool.Definition.Function.Name),
		Status:       status,
		Arguments:    result.Arguments,
		Result:       result.Result,
		DurationMS:   time.Since(startedAt).Milliseconds(),
	}
	if result.Error != nil {
		trace.Error = status
	}
	modelResult := result.Result
	if result.ModelResult != nil {
		modelResult = result.ModelResult
	}
	message := skills.ToolResultMessage(call.ID, modelResult)
	step := successfulSkillStep(trace, message, false, true)
	step.toolResult = result.Result
	step.artifacts = result.Artifacts
	step.events = result.Events
	step.pendingApproval = result.PendingApproval
	step.pendingGovernance = result.PendingGovernance
	step.pendingConnector = result.PendingConnector
	step.pendingQuestion = result.PendingQuestion
	step.pendingClientAction = result.PendingClientAction
	step.pendingUserInput = result.PendingUserInput
	if result.Recoverable {
		step = recoverableSkillStep(trace, message, false, true)
		step.toolResult = result.Result
		step.pendingApproval = result.PendingApproval
		step.pendingGovernance = result.PendingGovernance
		step.pendingConnector = result.PendingConnector
		step.pendingQuestion = result.PendingQuestion
		step.pendingClientAction = result.PendingClientAction
		step.pendingUserInput = result.PendingUserInput
		step.artifacts = result.Artifacts
		step.events = result.Events
	}
	return step
}

// runtimeToolEventPayload adds the conversation and message a runtime tool's
// event belongs to, without changing the tool's own map.
func runtimeToolEventPayload(prepared *PreparedChat, payload map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(payload)+2)
	for key, value := range payload {
		out[key] = value
	}
	if prepared != nil && prepared.Conversation != nil {
		out["conversation_id"] = prepared.Conversation.ID
	}
	if prepared != nil && prepared.Message != nil {
		out["message_id"] = prepared.Message.ID
	}
	return out
}

func nativeAgentLoopSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"## Skills",
		"Candidate skills are listed by name only; their instructions and business functions are available after activate_skills. Resident tools need no activation.",
		"Answer directly when the conversation already holds what the user needs, such as knowledge questions, explanations, translations, rewrites and summaries.",
		"Activate a skill only when the request needs an external read, a generated file, a change, or a page operation, and activate the smallest set that covers it. Use search_skills only when the directory says some skills were left out.",
		"An active script skill is already loaded: run its session_scripts package with shell_exec, using the package_sha256 it gives.",
		"## Tools",
		"Call business functions directly. Read-only calls may run together; make at most one call that changes something per response, then wait for its result.",
		"Never repeat a failed call with the same arguments. Use read_skill_reference only when an active skill asks for it, and request_user_input only when missing information blocks reliable progress.",
		"If update_plan is available and the request needs three or more steps or several deliverables, call update_plan with the steps before the first business call, then update it as each step finishes; the user follows the work through this checklist. If submit_turn_state is available, use it to keep exact working state across an approval, a question to the user, or another pause.",
		"## Progress notes",
		"For work with several stages, write one or two short sentences before the first business call: what you found or decided, and what the next step will produce. Add another only when evidence changes the plan, the stage changes, or you recover from an error.",
		"Skip notes for a single quick call, for repeated calls of the same kind, and for direct answers. A few notes per task are enough.",
		"## Answers",
		"When no more calls are needed, answer as ordinary assistant content. For complex work, give the result, what it is based on, and anything left unfinished.",
		"Never say an action succeeded without a matching successful tool result.",
		"## Language and privacy",
		"Write notes, questions and answers in the language the user asked for, in this conversation or in their saved preferences. Otherwise use the user's interface language when the context names it; writing in another language is not a request for it. Without either, use the language the user writes in.",
		"Never show internal details: skill or tool names, parameters, IDs, JSON, hidden reasoning or plans. Put file bodies and other function content only in the function arguments.",
	}, "\n")}
}

func nativeActiveSessionScriptsSystemMessage(resolved *skills.ResolvedSkills, activeSkillIDs []string, runtimeTools []RuntimeTool) (adapter.Message, bool) {
	if resolved == nil || !nativeRuntimeToolAvailable(runtimeTools, "shell_exec") {
		return adapter.Message{}, false
	}
	scripts := make([]map[string]interface{}, 0, len(activeSkillIDs))
	for _, skillID := range activeSkillIDs {
		if doc, ok := resolved.Get(skillID); ok {
			if script := portableSessionScriptPayload(doc); script != nil {
				if nativeRuntimeToolAvailable(runtimeTools, "workspace_prepare_dependencies") {
					script["dependency_preparation_tool"] = "workspace_prepare_dependencies"
				}
				scripts = append(scripts, script)
			}
		}
	}
	if len(scripts) == 0 {
		return adapter.Message{}, false
	}
	payload, err := json.Marshal(map[string]interface{}{
		"session_scripts": scripts,
		"next_action":     "These Skills are already active. Pass the matching skill_id and package_sha256 to shell_exec, then run the script from $ZGI_SKILL_DIR/scripts/. If a standard Skill reports a missing package, install it with the available pip/npm/apt-get command and retry; use workspace_prepare_dependencies only when explicitly exposed for a legacy Skill. Do not call load_skill or activate_skills for an already active Skill.",
	})
	if err != nil {
		return adapter.Message{}, false
	}
	return adapter.Message{Role: "system", Content: string(payload)}, true
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

// withSystemPromptAdditions appends guidance that stays the same from turn to
// turn, such as the native-loop rules, to the system prompt. There it is part
// of the prefix a provider caches; sent after the history, as it was, every
// turn's first request paid for it in full (ADR 0032). Without a leading text
// system prompt, the guidance becomes one.
func withSystemPromptAdditions(messages []adapter.Message, additions []adapter.Message) []adapter.Message {
	parts := make([]string, 0, len(additions))
	for _, addition := range additions {
		if text := strings.TrimSpace(messageContent(addition.Content)); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return messages
	}
	guidance := strings.Join(parts, "\n\n")
	if len(messages) > 0 && strings.EqualFold(messages[0].Role, "system") {
		if prompt, ok := messages[0].Content.(string); ok {
			out := append([]adapter.Message(nil), messages...)
			if prompt = strings.TrimSpace(prompt); prompt != "" {
				guidance = prompt + "\n\n" + guidance
			}
			out[0].Content = guidance
			return out
		}
	}
	return append([]adapter.Message{{Role: "system", Content: guidance}}, messages...)
}

// withoutRemindersInHistory leaves out guidance the conversation already
// holds verbatim, such as the instructions of a skill a previous turn
// activated: they are not sent again every turn (ADR 0043).
func withoutRemindersInHistory(messages []adapter.Message, additions []adapter.Message) []adapter.Message {
	out := make([]adapter.Message, 0, len(additions))
	for _, addition := range additions {
		text := strings.TrimSpace(messageContent(addition.Content))
		if text != "" && strings.EqualFold(strings.TrimSpace(addition.Role), "system") && messagesHoldReminder(messages, text) {
			continue
		}
		out = append(out, addition)
	}
	return out
}

func messagesHoldReminder(messages []adapter.Message, text string) bool {
	wrapped := adapter.SystemReminder(text)
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "user") && strings.Contains(messageContent(message.Content), wrapped) {
			return true
		}
	}
	return false
}

// placedTurnUserContent is the current question as the provider receives it:
// the last user message with the system messages after it merged in. It is
// reported only when nothing but system messages follows the question.
func placedTurnUserContent(messages []adapter.Message) (interface{}, bool) {
	last := -1
	for index := len(messages) - 1; index >= 0; index-- {
		role := strings.ToLower(strings.TrimSpace(messages[index].Role))
		if role == "system" {
			continue
		}
		if role == "user" && messages[index].ToolCallID == "" {
			last = index
		}
		break
	}
	if last <= 0 {
		return nil, false
	}
	placed := adapter.PlaceSystemMessages(append([]adapter.Message{{Role: "system", Content: "-"}}, messages[last:]...))
	if len(placed) != 2 {
		return nil, false
	}
	return placed[1].Content, true
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
	execution skills.ExecutionContext,
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
		doc, trace, err := r.SkillRuntime.LoadSkill(ctx, resolved, skillID, execution)
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
			"effective_version":  skillEffectiveVersion(doc),
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
	resolved *skills.ResolvedSkills,
	runtimeTools []RuntimeTool,
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
	if nativeRuntimeToolAvailable(runtimeTools, "shell_exec") {
		sessionScripts := make([]map[string]interface{}, 0)
		for _, skillID := range append(append([]string(nil), activation.ActivatedSkillIDs...), activation.AlreadyActiveSkillIDs...) {
			if doc, ok := resolved.Get(skillID); ok {
				if script := portableSessionScriptPayload(doc); script != nil {
					if nativeRuntimeToolAvailable(runtimeTools, "workspace_prepare_dependencies") {
						script["dependency_preparation_tool"] = "workspace_prepare_dependencies"
					}
					sessionScripts = append(sessionScripts, script)
				}
			}
		}
		if len(sessionScripts) > 0 {
			payload["session_scripts"] = sessionScripts
			payload["next_action"] = "Use shell_exec with skill_id and the activated Skill's package_sha256 to stage its script package, then run its script from $ZGI_SKILL_DIR/scripts/. Standard Skills do not need a ZGI manifest or dependency declaration: if Python reports ModuleNotFoundError, Node reports Cannot find module, or a command is missing, install the missing package with the available pip/npm/apt-get command in this isolated runtime and retry. Use workspace_prepare_dependencies only when it is explicitly exposed for a legacy declarative Skill."
		}
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

// activateNativeSkillForToolCall activates the skill of a business tool the
// model called without activating it first, typically a tool it used in an
// earlier turn of the conversation. Skills start inactive in every turn, and
// bouncing the call cost the turn a round. Activation goes through the
// session, so only a skill that is still a candidate for this turn and passes
// its checks becomes active; any other call keeps the not-active feedback.
func activateNativeSkillForToolCall(
	ctx context.Context,
	call adapter.ToolCall,
	session *skills.NativeSkillSession,
	resolved *skills.ResolvedSkills,
	toolSet *skills.NativeToolSet,
	loadedSkills map[string]struct{},
) (skills.NativeSkillActivationResult, bool) {
	name := strings.TrimSpace(call.Function.Name)
	if session == nil || name == "" || isSkillMetaToolName(name) || name == skills.MetaToolActivateSkills || name == skills.MetaToolSearchSkills {
		return skills.NativeSkillActivationResult{}, false
	}
	if toolSet != nil {
		if _, _, bound := nativeToolBindingForCall(toolSet, name); bound {
			return skills.NativeSkillActivationResult{}, false
		}
	}
	skillID, ok := uniqueResolvedSkillForToolName(resolved, name)
	if !ok {
		return skills.NativeSkillActivationResult{}, false
	}
	if _, active := loadedSkills[skillID]; active {
		return skills.NativeSkillActivationResult{}, false
	}
	activation := session.Activate(ctx, []string{skillID}, "tool_call")
	for _, activated := range activation.ActivatedSkillIDs {
		if strings.EqualFold(strings.TrimSpace(activated), skillID) {
			return activation, true
		}
	}
	return activation, false
}

func nativeRuntimeToolAvailable(runtimeTools []RuntimeTool, name string) bool {
	for _, runtimeTool := range runtimeTools {
		if runtimeTool.Handler != nil && strings.EqualFold(strings.TrimSpace(runtimeTool.Definition.Function.Name), name) {
			return true
		}
	}
	return false
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
