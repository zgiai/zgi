package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/workflowevents"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type userInputGuardState struct {
	guard               UserInputGuard
	toolCallGuard       ToolCallGuard
	planToolGuard       ToolCallGuard
	argumentResolver    ToolArgumentResolver
	round               int
	skillUsed           bool
	toolCallCount       int
	attemptedToolCalls  []SkillToolCallRef
	successfulToolCalls []SkillToolCallRef
}

func (r *Runner) handleProgressiveSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	call adapter.ToolCall,
	execCtx skills.ExecutionContext,
	currentToolCalls int,
	skillToolCallCounts map[string]int,
	loadedSkills map[string]struct{},
	userInputGuard userInputGuardState,
	onEvent func(Event) error,
) skillStepResult {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same tool call")), false, false)
	}
	if normalizedCall, normalizedArgs, ok := normalizeDirectLoadedSkillToolCall(resolved, loadedSkills, call, args); ok {
		call = normalizedCall
		args = normalizedArgs
	}
	switch call.Function.Name {
	case skills.MetaToolLoadSkill:
		skillID := normalizedSkillArg(args, "skill_id")
		if guardResult, blocked := runToolCallGuard(userInputGuard.planToolGuard, ToolCallGuardRequest{
			SkillID:             skillID,
			ToolName:            "",
			Round:               userInputGuard.round,
			SkillUsed:           userInputGuard.skillUsed,
			ToolCallCount:       userInputGuard.toolCallCount,
			AttemptedToolCalls:  append([]SkillToolCallRef{}, userInputGuard.attemptedToolCalls...),
			SuccessfulToolCalls: append([]SkillToolCallRef{}, userInputGuard.successfulToolCalls...),
		}); blocked {
			return planToolGuardRecoverableStep(call.ID, skillID, "", nil, guardResult)
		}
		return r.handleLoadSkillCall(ctx, prepared, resolved, call.ID, args, loadedSkills, onEvent)
	case skills.MetaToolReadSkillReference:
		if _, ok := loadedSkills[normalizedSkillArg(args, "skill_id")]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), "", "skill must be loaded before reading references")
			trace.SkillID = stringArg(args, "skill_id")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
		}
		return r.handleReadReferenceCall(ctx, prepared, resolved, call.ID, args, onEvent)
	case skills.MetaToolCallSkillTool:
		skillID := normalizedSkillArg(args, "skill_id")
		toolName := stringArg(args, "tool_name")
		toolArgs := mapArg(args, "arguments")
		if strings.EqualFold(toolName, skills.MetaToolRequestUserInput) {
			return r.handleRequestUserInputCall(ctx, prepared, call.ID, toolArgs, userInputGuard, onEvent)
		}
		if _, ok := loadedSkills[skillID]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), toolName, "skill must be loaded before calling its tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
		}
		if skills.RequiresPromptProfessionalizerPreflight(skillID, toolName) {
			if _, ok := loadedSkills[skills.SkillPromptProfessionalizer]; !ok {
				trace := blockedSkillGuardrailTrace(skillID, toolName, "prompt-professionalizer must be loaded before calling this professional generation tool")
				return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
			}
		}
		if doc, ok := resolved.Get(skillID); ok && len(doc.Tools) == 0 {
			trace := blockedSkillGuardrailTrace(skillID, toolName, "skill does not provide callable tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), true, false)
		}
		if resolvedArgs, ok := runToolArgumentResolver(userInputGuard.argumentResolver, ToolCallGuardRequest{
			SkillID:             skillID,
			ToolName:            toolName,
			Arguments:           copyStringAnyMap(toolArgs),
			Round:               userInputGuard.round,
			SkillUsed:           userInputGuard.skillUsed,
			ToolCallCount:       userInputGuard.toolCallCount,
			AttemptedToolCalls:  append([]SkillToolCallRef{}, userInputGuard.attemptedToolCalls...),
			SuccessfulToolCalls: append([]SkillToolCallRef{}, userInputGuard.successfulToolCalls...),
		}); ok {
			toolArgs = resolvedArgs
			args["arguments"] = toolArgs
		}
		if guardResult, blocked := runToolCallGuard(userInputGuard.planToolGuard, ToolCallGuardRequest{
			SkillID:             skillID,
			ToolName:            toolName,
			Arguments:           copyStringAnyMap(toolArgs),
			Round:               userInputGuard.round,
			SkillUsed:           userInputGuard.skillUsed,
			ToolCallCount:       userInputGuard.toolCallCount,
			AttemptedToolCalls:  append([]SkillToolCallRef{}, userInputGuard.attemptedToolCalls...),
			SuccessfulToolCalls: append([]SkillToolCallRef{}, userInputGuard.successfulToolCalls...),
		}); blocked {
			return planToolGuardRecoverableStep(call.ID, skillID, toolName, toolArgs, guardResult)
		}
		if guardResult, blocked := runToolCallGuard(userInputGuard.toolCallGuard, ToolCallGuardRequest{
			SkillID:             skillID,
			ToolName:            toolName,
			Arguments:           copyStringAnyMap(toolArgs),
			Round:               userInputGuard.round,
			SkillUsed:           userInputGuard.skillUsed,
			ToolCallCount:       userInputGuard.toolCallCount,
			AttemptedToolCalls:  append([]SkillToolCallRef{}, userInputGuard.attemptedToolCalls...),
			SuccessfulToolCalls: append([]SkillToolCallRef{}, userInputGuard.successfulToolCalls...),
		}); blocked {
			trace := toolCallGuardrailTrace(guardResult, skillID, toolName, toolArgs)
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, toolCallGuardrailPayload(guardResult, skillID, toolName)), false, false)
		}
		if currentToolCalls >= maxBusinessToolCalls(resolved) {
			err := fmt.Errorf("%w: too many skill tool calls", ErrInvalidInput)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return fatalSkillStep(trace, skills.ToolResultMessage(call.ID, errorPayload(err)), err)
		}
		if skillToolCallCounts[skillID] >= maxBusinessToolCallsForSkill(resolved, skillID) {
			err := fmt.Errorf("%w: too many skill tool calls for skill %s", ErrInvalidInput, skillID)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return fatalSkillStep(trace, skills.ToolResultMessage(call.ID, errorPayload(err)), err)
		}
		return r.handleCallSkillTool(ctx, prepared, resolved, call.ID, args, execCtx, onEvent)
	case skills.MetaToolRequestUserInput:
		return r.handleRequestUserInputCall(ctx, prepared, call.ID, args, userInputGuard, onEvent)
	case skills.MetaToolIntermediateAnswer:
		return r.handleIntermediateAnswerCall(ctx, prepared, call.ID, args, onEvent)
	default:
		if isInjectedContextPseudoToolName(call.Function.Name) {
			return injectedContextPseudoToolFeedbackStep(call.ID, call.Function.Name)
		}
		err := fmt.Errorf("%w: unsupported skill meta tool %s", ErrInvalidInput, call.Function.Name)
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "use one of load_skill, request_user_input, read_skill_reference, call_skill_tool, or submit_intermediate_answer")), false, false)
	}
}

func normalizeDirectLoadedSkillToolCall(
	resolved *skills.ResolvedSkills,
	loadedSkills map[string]struct{},
	call adapter.ToolCall,
	args map[string]interface{},
) (adapter.ToolCall, map[string]interface{}, bool) {
	toolName := strings.TrimSpace(call.Function.Name)
	if toolName == "" || isSkillMetaToolName(toolName) {
		return call, args, false
	}
	skillID, ok := uniqueLoadedSkillForToolName(resolved, loadedSkills, toolName)
	if !ok {
		return call, args, false
	}
	normalizedArgs := map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": copyStringAnyMap(args),
	}
	encoded, err := json.Marshal(normalizedArgs)
	if err != nil {
		return call, args, false
	}
	call.Function.Name = skills.MetaToolCallSkillTool
	call.Function.Arguments = string(encoded)
	return call, normalizedArgs, true
}

func isSkillMetaToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case skills.MetaToolLoadSkill,
		skills.MetaToolReadSkillReference,
		skills.MetaToolCallSkillTool,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolRequestUserInput:
		return true
	default:
		return false
	}
}

func isInjectedContextPseudoToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "get_current_page_context", "read_current_page_context", "observe_current_page_context":
		return true
	default:
		return false
	}
}

func uniqueLoadedSkillForToolName(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, toolName string) (string, bool) {
	if resolved == nil || len(loadedSkills) == 0 || strings.TrimSpace(toolName) == "" {
		return "", false
	}
	matchedSkillID := ""
	for _, doc := range resolved.Skills {
		skillID := strings.ToLower(strings.TrimSpace(doc.Metadata.ID))
		if skillID == "" {
			continue
		}
		if _, ok := loadedSkills[skillID]; !ok {
			continue
		}
		for _, tool := range doc.Tools {
			if !strings.EqualFold(strings.TrimSpace(tool.Name), toolName) {
				continue
			}
			if matchedSkillID != "" && !strings.EqualFold(matchedSkillID, skillID) {
				return "", false
			}
			matchedSkillID = skillID
		}
	}
	return matchedSkillID, matchedSkillID != ""
}

func planToolGuardRecoverableStep(callID string, skillID string, toolName string, args map[string]interface{}, result FinalAnswerGuardResult) skillStepResult {
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "tool is outside the current operation plan"
	}
	nextAction := strings.TrimSpace(result.SystemMessage)
	if nextAction == "" {
		nextAction = "continue with the pending tool from the current operation plan or answer from verified evidence"
	}
	if result.Advisory {
		trace := plannerFeedbackTrace(skillID, toolName, nil)
		if len(args) > 0 {
			trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
		}
		trace.Arguments["next_step"] = "continue_with_next_planned_step"
		trace.Arguments["advisory"] = "planner_feedback"
		return successfulSkillStep(trace, skills.ToolResultMessage(callID, plannerFeedbackAdvisoryPayload(message, nextAction, skillID, toolName)), false, false)
	}
	err := fmt.Errorf("%w: %s", ErrInvalidInput, message)
	trace := plannerFeedbackTrace(skillID, toolName, err)
	if len(args) > 0 {
		trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
		trace.Arguments["next_step"] = "continue_planning"
	}
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, skillID, toolName)), false, false)
}

func injectedContextPseudoToolFeedbackStep(callID string, toolName string) skillStepResult {
	err := fmt.Errorf("%w: current page context is already injected into the conversation, not exposed as a callable tool", ErrInvalidInput)
	trace := plannerFeedbackTrace("", toolName, err)
	trace.Arguments["next_step"] = "use_injected_context"
	nextAction := strings.Join([]string{
		"Do not call page context pseudo-tools.",
		"Use the current page context and visible resource evidence already provided in the conversation.",
		"If that evidence is insufficient, use an enabled read/list/search skill tool or ask the user a concise question.",
	}, " ")
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, nextAction)), false, false)
}

func (r *Runner) handleRequestUserInputCall(
	ctx context.Context,
	prepared *PreparedChat,
	callID string,
	args map[string]interface{},
	guardState userInputGuardState,
	onEvent func(Event) error,
) skillStepResult {
	questions, err := normalizeUserInputRequestArgs(args)
	if err != nil {
		trace := failedSkillTrace("user_input_request", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call request_user_input again with one to five non-empty questions and optional short options")), false, false)
	}
	visibleMessage := normalizeUserInputRequestMessage(args)
	if visibleMessage == "" {
		err := fmt.Errorf("%w: message is required", ErrInvalidInput)
		trace := failedSkillTrace("user_input_request", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call request_user_input again with a brief user-visible message and one to five questions")), false, false)
	}
	if guardResult, blocked := runUserInputGuard(guardState.guard, UserInputGuardRequest{
		Message:             visibleMessage,
		Questions:           cloneQuestionMaps(questions),
		Round:               guardState.round,
		SkillUsed:           guardState.skillUsed,
		ToolCallCount:       guardState.toolCallCount,
		AttemptedToolCalls:  append([]SkillToolCallRef{}, guardState.attemptedToolCalls...),
		SuccessfulToolCalls: append([]SkillToolCallRef{}, guardState.successfulToolCalls...),
	}); blocked {
		trace := userInputGuardrailTrace(guardResult)
		return successfulSkillStep(trace, skills.ToolResultMessage(callID, userInputGuardrailPayload(guardResult, visibleMessage, questions)), false, false)
	}
	firstQuestion := stringFromInterface(questions[0]["question"])
	trace := skills.SkillTrace{
		Kind:    "user_input_request",
		Message: firstQuestion,
		Status:  "success",
		Arguments: map[string]interface{}{
			"question_count": len(questions),
			"questions":      userInputQuestionSummaries(questions),
		},
	}
	if visibleMessage != "" {
		trace.Message = visibleMessage
	}
	r.emitEvent(EventUserInputRequested, userInputRequestPayload(prepared, callID, questions))
	logger.DebugContext(ctx, "aichat user input requested",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"question_count", len(questions),
	)
	result := terminalSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status":      "waiting_for_user",
		"instruction": "The question is visible to the user. Stop this turn and wait for the next user message.",
	}), false, false)
	result.answer = visibleMessage
	return result
}

func (r *Runner) handleIntermediateAnswerCall(
	ctx context.Context,
	prepared *PreparedChat,
	callID string,
	args map[string]interface{},
	onEvent func(Event) error,
) skillStepResult {
	content := strings.TrimSpace(stringArg(args, "content"))
	if content == "" {
		return successfulSkillStep(skills.SkillTrace{}, skills.ToolResultMessage(callID, map[string]interface{}{
			"status":      "skipped",
			"instruction": "No intermediate answer was shown because content was empty. Continue with the task and provide a normal final answer when ready.",
		}), false, false)
	}
	title := strings.TrimSpace(stringArg(args, "title"))
	answerID := strings.TrimSpace(stringArg(args, streamedIntermediateAnswerArg+"_id"))
	trace := skills.SkillTrace{
		Kind:    "intermediate_answer",
		Title:   title,
		Message: content,
		Status:  "success",
		Arguments: map[string]interface{}{
			"title": title,
		},
	}
	if boolArg(args, streamedIntermediateAnswerArg) {
		if answerID == "" {
			answerID = callID
		}
		r.emitEvent(EventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, "", 0, true, "success"))
	} else {
		r.emitIntermediateAnswer(ctx, prepared, callID, trace, onEvent)
	}
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "recorded",
		"instruction": strings.Join([]string{
			"The intermediate answer is visible to the user and saved in the run trace.",
			"Continue with any remaining tool callr.",
			"Your eventual user-facing reply must still be complete and self-contained; do not say see above.",
		}, " "),
	}), false, false)
}

func (r *Runner) emitIntermediateAnswer(
	ctx context.Context,
	prepared *PreparedChat,
	answerID string,
	trace skills.SkillTrace,
	onEvent func(Event) error,
) {
	chunks := splitIntermediateAnswerContent(trace.Message, intermediateAnswerChunkRunes)
	if len(chunks) == 0 {
		return
	}
	for index, chunk := range chunks {
		done := index == len(chunks)-1
		status := "streaming"
		if done {
			status = "success"
		}
		r.emitEvent(EventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, chunk, index, done, status))
	}
}

func splitIntermediateAnswerContent(content string, chunkRunes int) []string {
	if chunkRunes <= 0 {
		chunkRunes = intermediateAnswerChunkRunes
	}
	runes := []rune(content)
	if len(runes) <= chunkRunes {
		if content == "" {
			return nil
		}
		return []string{content}
	}

	chunks := make([]string, 0, (len(runes)/chunkRunes)+1)
	for start := 0; start < len(runes); {
		end := start + chunkRunes
		if end >= len(runes) {
			chunks = append(chunks, string(runes[start:]))
			break
		}

		split := end
		for i := end; i > start+chunkRunes/2; i-- {
			switch runes[i-1] {
			case '\n', ' ', '\t', '。', '，', '；', '！', '？', '.', ',', ';', '!', '?':
				split = i
				i = start
			}
		}
		if split <= start {
			split = end
		}
		chunks = append(chunks, string(runes[start:split]))
		start = split
	}
	return chunks
}

func (r *Runner) handleLoadSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	loadedSkills map[string]struct{},
	onEvent func(Event) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	if _, ok := resolved.Get(skillID); !ok {
		return unavailableSkillLoadFeedbackStep(callID, skillID)
	}
	r.emitEvent(EventSkillLoadStart, skillLoadPayload(prepared, skillID))
	doc, trace, err := r.SkillRuntime.LoadSkill(ctx, resolved, skillID)
	if err != nil {
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "choose an enabled skill_id from the exposed metadata and retry")), false, false)
	}
	loadedSkills[doc.Metadata.ID] = struct{}{}
	logger.DebugContext(ctx, "aichat skill loaded",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", doc.Metadata.ID,
	)
	r.emitEvent(EventSkillLoadEnd, skillLoadEndPayload(prepared, trace))
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, skillDocumentPayload(doc)), true, false)
}

func unavailableSkillLoadFeedbackStep(callID string, skillID string) skillStepResult {
	normalizedSkillID := strings.ToLower(strings.TrimSpace(skillID))
	err := fmt.Errorf("%w: skill %s is not enabled for this turn", ErrInvalidInput, normalizedSkillID)
	trace := plannerFeedbackTrace(normalizedSkillID, "", err)
	trace.Arguments["next_step"] = "choose_enabled_skill_or_continue_from_context"
	nextAction := "Choose an enabled skill_id from the exposed metadata. If the unavailable skill was console-navigator, continue from current page evidence unless navigation is explicitly required and available."
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, nextAction)), false, false)
}

func (r *Runner) handleReadReferenceCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	onEvent func(Event) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	path := stringArg(args, "path")
	content, trace, err := r.SkillRuntime.ReadReference(ctx, resolved, skillID, path)
	if err != nil {
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "use a reference path listed in the loaded SKILL.md and retry")), true, false)
	}
	logger.DebugContext(ctx, "aichat skill reference read",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", trace.SkillID,
		"path", path,
		"duration_ms", trace.DurationMS,
	)
	r.emitEvent(EventSkillReferenceRead, skillReferenceReadPayload(prepared, trace, path))
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"skill_id": skillID,
		"path":     path,
		"content":  content,
	}), true, false)
}

func (r *Runner) handleCallSkillTool(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	execCtx skills.ExecutionContext,
	onEvent func(Event) error,
) skillStepResult {
	skillID := stringArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	argumentSummary := summarizeSkillToolArguments(skillID, toolName, toolArgs)
	r.emitEvent(EventSkillCallStart, skillCallStartPayload(prepared, skillID, toolName, argumentSummary))
	if isAgentWorkflowRunTool(skillID, toolName) {
		ctx = workflowevents.WithEmitter(ctx, func(event workflowevents.Event) {
			if event.Type == "" {
				return
			}
			payload := event.Payload
			if payload == nil {
				payload = map[string]interface{}{}
			}
			if prepared != nil && prepared.Conversation != nil {
				payload["conversation_id"] = prepared.Conversation.ID.String()
			}
			if prepared != nil && prepared.Message != nil {
				payload["message_id"] = prepared.Message.ID.String()
			}
			r.emitEvent(event.Type, payload)
		})
	}
	invocation, err := r.SkillRuntime.CallSkillTool(ctx, resolved, skillID, toolName, toolArgs, execCtx, callID)
	if invocation == nil {
		if err == nil {
			err = fmt.Errorf("%w: skill tool returned no invocation result", ErrInvalidInput)
		}
		trace := failedSkillTrace("tool_call", toolName, err)
		trace.SkillID = skillID
		trace.Arguments = argumentSummary
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool_name or arguments and retry", skillID, toolName)), true, false)
	}
	if !traceHasGovernanceArgumentRewrite(invocation.Trace) {
		invocation.Trace.Arguments = argumentSummary
	}
	applyGovernedAssetArguments(&invocation.Trace)
	if invocation.Trace.Governance != nil {
		r.emitEvent(EventToolGovernanceDecision, toolGovernanceDecisionPayload(prepared, invocation.Trace))
	}
	if err != nil {
		return recoverableSkillStep(invocation.Trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool arguments based on the error and retry", skillID, toolName)), true, false)
	}
	if summary := summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages); len(summary) > 0 {
		invocation.Trace.Result = summary
	}
	guardToolResult := skillToolResultForGuard(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages, invocation.Trace.Result)
	logger.DebugContext(ctx, "aichat skill tool completed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", invocation.Trace.SkillID,
		"tool_name", invocation.Trace.ToolName,
		"duration_ms", invocation.Trace.DurationMS,
	)
	r.emitEvent(EventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace))
	for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
		r.recordArtifact(artifact)
		r.emitEvent(EventSkillArtifactCreated, artifact)
	}
	if payload := clientActionRequiredPayload(prepared, invocation.Trace, callID); len(payload) > 0 {
		r.emitEvent(EventClientActionRequired, payload)
		result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
		result.toolResult = guardToolResult
		result.pendingClientAction = payload
		return result
	}
	if isAgentWorkflowRunTool(invocation.Trace.SkillID, invocation.Trace.ToolName) {
		if payload := agentWorkflowResultPayload(invocation.Messages); len(payload) > 0 {
			if strings.EqualFold(stringFromInterface(payload["status"]), "pending_approval") {
				result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
				result.toolResult = guardToolResult
				result.pendingApproval = payload
				return result
			}
			if strings.EqualFold(stringFromInterface(payload["status"]), "pending_question") {
				result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
				result.toolResult = guardToolResult
				result.pendingQuestion = payload
				return result
			}
			if strings.EqualFold(stringFromInterface(payload["agent_type"]), "CONVERSATIONAL_WORKFLOW") &&
				strings.EqualFold(stringFromInterface(payload["status"]), "succeeded") {
				answer := strings.TrimSpace(stringFromInterface(payload["primary_output"]))
				if answer == "" {
					answer = "工作流已运行，但未返回可展示输出。workflow_run_id: " + stringFromInterface(payload["workflow_run_id"])
				}
				result := terminalSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
				result.toolResult = guardToolResult
				result.answer = answer
				return result
			}
		}
	}
	if toolGovernanceApprovalPending(invocation.Trace) {
		result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
		result.toolResult = guardToolResult
		result.pendingGovernance = toolGovernanceDecisionPayload(prepared, invocation.Trace)
		return result
	}
	result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
	result.toolResult = guardToolResult
	return result
}

func toolGovernanceApprovalPending(trace skills.SkillTrace) bool {
	return trace.Governance != nil &&
		trace.Governance.Status == toolgovernance.DecisionStatusNeedsApproval &&
		trace.Governance.RequiresApproval
}

func traceHasGovernanceArgumentRewrite(trace skills.SkillTrace) bool {
	if len(trace.Arguments) == 0 {
		return false
	}
	_, ok := trace.Arguments["governance_argument_rewrite"]
	return ok
}

func applyGovernedAssetArguments(trace *skills.SkillTrace) {
	if trace == nil || trace.Governance == nil {
		return
	}
	decision := trace.Governance
	if decision.Status != toolgovernance.DecisionStatusAllowed ||
		decision.Manifest.Effect != toolgovernance.EffectRead ||
		!strings.EqualFold(strings.TrimSpace(decision.Manifest.AssetType), "file") ||
		len(decision.Assets) != 1 {
		return
	}
	fileID := strings.TrimSpace(decision.Assets[0].ID)
	if fileID == "" {
		return
	}
	if trace.Arguments == nil {
		trace.Arguments = map[string]interface{}{}
	}
	previousID := strings.TrimSpace(stringFromInterface(trace.Arguments["file_id"]))
	trace.Arguments["file_id"] = fileID
	if previousID != "" && previousID != fileID {
		trace.Arguments["governance_argument_rewrite"] = map[string]interface{}{
			"reason":       "governed_asset_trace_alignment",
			"effect":       string(decision.Manifest.Effect),
			"asset_type":   decision.Manifest.AssetType,
			"from_file_id": previousID,
			"to_file_id":   fileID,
		}
	}
}

func isAgentWorkflowRunTool(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentWorkflow) &&
		strings.EqualFold(strings.TrimSpace(toolName), "run_agent_workflow")
}

func agentWorkflowResultPayload(messages []tools.ToolInvokeMessage) map[string]interface{} {
	for _, message := range messages {
		if message.Type == tools.ToolInvokeMessageTypeJSON && len(message.Data) > 0 {
			return message.Data
		}
	}
	return nil
}

func skillToolResultForGuard(skillID string, toolName string, messages []tools.ToolInvokeMessage, summary map[string]interface{}) map[string]interface{} {
	result := copyStringAnyMap(summary)
	if !isFileGeneratorTool(skillID, toolName) {
		return result
	}
	payload := firstJSONToolInvokePayload(messages)
	if len(payload) == 0 {
		return result
	}
	if result == nil {
		result = map[string]interface{}{}
	}
	if id := strings.TrimSpace(firstNonEmptyString(payload["tool_file_id"], payload["file_id"])); id != "" {
		result["tool_file_id"] = id
		result["file_id"] = id
	}
	for _, key := range []string{"filename", "format", "mime_type", "size", "target", "download_url"} {
		if value, ok := payload[key]; ok && value != nil && strings.TrimSpace(stringFromInterface(value)) != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func isFileGeneratorTool(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileGenerator) {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func firstJSONToolInvokePayload(messages []tools.ToolInvokeMessage) map[string]interface{} {
	for _, message := range messages {
		if message.Type == tools.ToolInvokeMessageTypeJSON && len(message.Data) > 0 {
			return message.Data
		}
	}
	return nil
}

func successfulSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
	}
}

func recoverableSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
		recoverable: true,
	}
}

func terminalSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, usedSkill bool, usedTool bool) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		usedSkill:   usedSkill,
		usedTool:    usedTool,
		terminal:    true,
	}
}

func fatalSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, err error) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		fatalErr:    err,
	}
}
