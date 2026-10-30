package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/workflowevents"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (r *Runner) handleProgressiveSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	call adapter.ToolCall,
	execCtx skills.ExecutionContext,
	currentToolCalls int,
	skillToolCallCounts map[string]int,
	loadedSkills map[string]struct{},
	runtimeState map[string]interface{},
	round int,
	onEvent func(Event) error,
) skillStepResult {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := invalidToolArgumentsFeedbackTrace(call.Function.Name, err, nil)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same tool call")), false, false)
	}
	if boolArg(args, "_invalid_tool_arguments") {
		err := fmt.Errorf("%w: tool arguments were not valid JSON", ErrInvalidInput)
		trace := invalidToolArgumentsFeedbackTrace(call.Function.Name, err, args)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, stringArg(args, "next_action"))), false, false)
	}
	// Tool visibility is scoped to the current skill session. A model may carry
	// a previously visible tool name into a new user turn; the native loop
	// activates the skill through the session first when it is still a
	// candidate (activateNativeSkillForToolCall), and any other inactive skill
	// is not restored from conversation history.
	if !isSkillMetaToolName(call.Function.Name) {
		if skillID, ok := uniqueResolvedSkillForToolName(resolved, call.Function.Name); ok {
			if _, active := loadedSkills[skillID]; !active {
				err := fmt.Errorf("%w: skill %s is not active", ErrInvalidInput, skillID)
				trace := failedSkillTrace("tool_call", call.Function.Name, err)
				trace.SkillID = skillID
				trace.Status = "blocked"
				trace.Arguments = summarizeSkillToolArgumentsForResolved(resolved, skillID, call.Function.Name, args)
				trace.Arguments["reason_code"] = "skill_not_active"
				nextAction := fmt.Sprintf("call activate_skills with skill_ids [\"%s\"], then retry %s", skillID, call.Function.Name)
				payload := recoverableSkillToolErrorPayload(err, nextAction, skillID, call.Function.Name, resolved)
				payload["error_code"] = "skill_not_active"
				payload["skill_id"] = skillID
				payload["tool_name"] = call.Function.Name
				return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, payload), false, false)
			}
		}
	}
	if normalizedCall, normalizedArgs, ok := normalizeDirectLoadedSkillToolCall(resolved, loadedSkills, call, args); ok {
		call = normalizedCall
		args = normalizedArgs
	}
	switch call.Function.Name {
	case contextArtifactToolName:
		return r.handleReadContextArtifactCall(ctx, call.ID, args)
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
		toolArgs, argumentsErr := normalizeSkillToolArguments(args, skillID, toolName)
		if argumentsErr != nil {
			return invalidSkillToolArgumentsStep(call.ID, skillID, toolName, args["arguments"], argumentsErr)
		}
		args["arguments"] = toolArgs
		if strings.EqualFold(toolName, skills.MetaToolRequestUserInput) {
			return r.handleRequestUserInputCall(ctx, prepared, call.ID, toolArgs, onEvent)
		}
		if strings.EqualFold(toolName, skills.MetaToolTurnState) {
			return r.handleTurnStateCall(ctx, prepared, call.ID, toolArgs, onEvent)
		}
		if strings.EqualFold(toolName, skills.MetaToolUpdatePlan) {
			if !runtimeControlToolAllowed(runtimeState, runtimeStateAllowPlanUpdateKey) {
				return skippedControlToolStep(call.ID, skills.MetaToolUpdatePlan, "continue with the current outcome contract; runtime effects are reconciled automatically")
			}
			return r.handleUpdatePlanCall(call.ID, toolArgs, runtimeState, round)
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
		if isAgentKnowledgeRetrievalCall(skillID, toolName) && skillToolCallCounts[skillID] >= maxBusinessToolCallsForSkill(resolved, skillID) {
			return knowledgeRetrievalBudgetStep(call.ID, skillID, toolName, toolArgs, skillToolCallCounts[skillID]+1, maxBusinessToolCallsForSkill(resolved, skillID))
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
		planPhaseID, enforcePlanPhase, phaseErr := resolveOperationPlanPhaseForSkillCall(
			runtimeState,
			strings.TrimSpace(stringArg(args, "plan_phase_id")),
			skillID,
			toolName,
			toolArgs,
		)
		if phaseErr != nil {
			return operationPlanPhaseMismatchStep(call.ID, skillID, toolName, toolArgs, phaseErr)
		}
		if enforcePlanPhase && planPhaseID != "" {
			args["plan_phase_id"] = planPhaseID
		} else {
			delete(args, "plan_phase_id")
		}
		callExecCtx := execCtx
		if isAgentKnowledgeRetrievalCall(skillID, toolName) {
			callExecCtx.RuntimeParameters = copyStringAnyMap(execCtx.RuntimeParameters)
			if callExecCtx.RuntimeParameters == nil {
				callExecCtx.RuntimeParameters = map[string]interface{}{}
			}
			callExecCtx.RuntimeParameters["knowledge_retrieval_round"] = runtimeKnowledgeRetrievalRound(runtimeState, skillID)
			callExecCtx.RuntimeParameters["knowledge_retrieval_budget"] = runtimeKnowledgeRetrievalBudget(runtimeState, skillID)
			callExecCtx.RuntimeParameters["knowledge_retrieval_history"] = runtimeState[runtimeStateKnowledgeRetrievalHistoryKey]
		}
		return r.handleCallSkillTool(ctx, prepared, resolved, call.ID, args, callExecCtx, runtimeState, onEvent)
	case skills.MetaToolRequestUserInput:
		return r.handleRequestUserInputCall(ctx, prepared, call.ID, args, onEvent)
	case skills.MetaToolTurnState:
		return r.handleTurnStateCall(ctx, prepared, call.ID, args, onEvent)
	case skills.MetaToolUpdatePlan:
		if !runtimeControlToolAllowed(runtimeState, runtimeStateAllowPlanUpdateKey) {
			return skippedControlToolStep(call.ID, skills.MetaToolUpdatePlan, "continue with the current outcome contract; runtime effects are reconciled automatically")
		}
		return r.handleUpdatePlanCall(call.ID, args, runtimeState, round)
	default:
		if isInjectedContextPseudoToolName(call.Function.Name) {
			return injectedContextPseudoToolFeedbackStep(call.ID, call.Function.Name)
		}
		err := fmt.Errorf("%w: unsupported skill meta tool %s", ErrInvalidInput, call.Function.Name)
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		nextAction := "call one of the business functions exposed in this request directly, use an available control tool, or provide the final answer as ordinary assistant content"
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, nextAction)), false, false)
	}
}

func invalidToolArgumentsFeedbackTrace(toolName string, err error, args map[string]interface{}) skills.SkillTrace {
	trace := plannerFeedbackTrace("", toolName, err)
	trace.Arguments["code"] = "invalid_tool_arguments"
	trace.Arguments["failure_category"] = "arguments"
	trace.Arguments["next_step"] = "retry_same_tool_with_valid_json"
	if preview := strings.TrimSpace(stringArg(args, "raw_preview")); preview != "" {
		trace.Arguments["raw_preview"] = preview
	}
	return trace
}

func runtimeControlToolAllowed(runtimeState map[string]interface{}, key string) bool {
	if runtimeState == nil {
		return true
	}
	value, exists := runtimeState[key]
	if !exists {
		return true
	}
	allowed, ok := value.(bool)
	return !ok || allowed
}

func isAgentKnowledgeRetrievalCall(skillID, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentKnowledge) &&
		strings.EqualFold(strings.TrimSpace(toolName), "retrieve_agent_knowledge")
}

func runtimeKnowledgeRetrievalRound(runtimeState map[string]interface{}, _ string) int {
	if runtimeState != nil {
		if value, ok := runtimeState["knowledge_retrieval_round"].(int); ok && value > 0 {
			return value
		}
		if history := runtimeState[runtimeStateKnowledgeRetrievalHistoryKey]; history != nil {
			if calls, ok := history.([]map[string]interface{}); ok && len(calls) > 0 {
				return len(calls) + 1
			}
		}
	}
	return 1
}

func runtimeKnowledgeRetrievalBudget(runtimeState map[string]interface{}, _ string) int {
	if runtimeState != nil {
		if value, ok := runtimeState["knowledge_retrieval_budget"].(int); ok && value > 0 {
			return value
		}
	}
	return defaultMaxBusinessToolCallsPerSkill
}

func knowledgeRetrievalBudgetStep(callID, skillID, toolName string, args map[string]interface{}, round int, budget int) skillStepResult {
	trace := plannerFeedbackTrace(skillID, toolName, nil)
	trace.Kind = "tool_call"
	trace.Status = "blocked"
	trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	trace.Arguments["reason_code"] = "retrieval_round_limit"
	trace.Arguments["next_step"] = "answer from the evidence already retrieved; do not call retrieve_agent_knowledge again in this execution"
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"query":               stringArg(args, "query"),
		"status":              "blocked",
		"result_count":        0,
		"retriever_resources": []interface{}{},
		"warnings":            []string{"retrieval execution budget reached; answer from the evidence already retrieved"},
		"control": map[string]interface{}{
			"continuation_allowed": false,
			"remaining_rounds":     0,
			"round":                round,
			"new_evidence_count":   0,
			"stop_reason":          "retrieval_round_limit",
		},
	}), true, false)
}

func skippedControlToolStep(callID string, toolName string, nextAction string) skillStepResult {
	trace := plannerFeedbackTrace("", toolName, nil)
	trace.Error = "control tool is not required for the current execution segment"
	trace.Arguments["call_id"] = strings.TrimSpace(callID)
	trace.Arguments["reason_code"] = "control_tool_not_required"
	trace.Arguments["next_step"] = "continue_planning"
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, plannerFeedbackAdvisoryPayload(
		trace.Error,
		nextAction,
		"",
		toolName,
	)), false, false)
}

func invalidSkillToolArgumentsStep(
	callID string,
	skillID string,
	toolName string,
	rawArguments interface{},
	err error,
) skillStepResult {
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.SkillID = strings.ToLower(strings.TrimSpace(skillID))
	trace.Arguments = rawSkillToolArgumentsFingerprint(rawArguments)
	trace.Arguments["failure_category"] = "arguments"
	trace.Arguments["provider_request_sent"] = false
	var argumentErr *skillToolArgumentsError
	if errors.As(err, &argumentErr) && argumentErr != nil {
		trace.Arguments["reason_code"] = argumentErr.Code
		trace.Arguments["missing_fields"] = append([]string(nil), argumentErr.MissingFields...)
		trace.Arguments["expected_type"] = argumentErr.ExpectedType
		trace.Arguments["actual_type"] = argumentErr.ActualType
	}
	return recoverableSkillStep(
		trace,
		skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(
			err,
			"correct the arguments using the structured error and retry once",
			skillID,
			toolName,
		)),
		false,
		false,
	)
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
		skills.MetaToolTurnState,
		skills.MetaToolUpdatePlan,
		skills.MetaToolIntermediateAnswer,
		skills.MetaToolRequestUserInput,
		skills.MetaToolFinalAnswer,
		contextArtifactToolName:
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

func uniqueResolvedSkillForToolName(resolved *skills.ResolvedSkills, toolName string) (string, bool) {
	if resolved == nil || strings.TrimSpace(toolName) == "" {
		return "", false
	}
	matched := ""
	for _, doc := range resolved.Skills {
		for _, tool := range doc.Tools {
			if !strings.EqualFold(strings.TrimSpace(tool.Name), strings.TrimSpace(toolName)) {
				continue
			}
			skillID := strings.ToLower(strings.TrimSpace(doc.Metadata.ID))
			if skillID == "" {
				continue
			}
			if matched != "" && matched != skillID {
				return "", false
			}
			matched = skillID
		}
	}
	return matched, matched != ""
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
	pendingPayload := userInputRequestPayload(prepared, callID, visibleMessage, questions)
	r.emitEvent(prepared, EventUserInputRequested, pendingPayload)
	logger.DebugContext(ctx, "aichat user input requested",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"question_count", len(questions),
	)
	result := terminalSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status":      "waiting_for_user",
		"instruction": "The question is visible to the user. Stop this turn and wait for the next user message.",
	}), false, false)
	result.pendingUserInput = pendingPayload
	return result
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
		r.emitEvent(prepared, EventIntermediateAnswer, intermediateAnswerPayload(prepared, trace, answerID, chunk, index, done, status))
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
	r.emitEvent(prepared, EventSkillReferenceRead, skillReferenceReadPayload(prepared, trace, path))
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
	runtimeState map[string]interface{},
	onEvent func(Event) error,
) skillStepResult {
	delegateOutput := &agentWorkflowDelegateOutput{}
	skillID := stringArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	planPhaseID := strings.TrimSpace(stringArg(args, "plan_phase_id"))
	completionIntent := normalizeSkillToolCompletionIntent(stringArg(args, "completion_intent"))
	planTarget := operationPlanSkillCallTarget(toolArgs)
	argumentSummary := summarizeSkillToolArgumentsForResolved(resolved, skillID, toolName, toolArgs)
	if len(planTarget) > 0 {
		argumentSummary["operation_plan_target"] = copyStringAnyMap(planTarget)
	}
	if planPhaseID != "" {
		argumentSummary["plan_phase_id"] = planPhaseID
	}
	if completionIntent != "" {
		argumentSummary["completion_intent"] = completionIntent
	}
	r.emitEvent(prepared, EventSkillCallStart, skillCallStartPayload(prepared, callID, skillID, toolName, argumentSummary))
	if err := prepared.presentationEventError(); err != nil {
		return unrecordedCallStep(callID, skillID, toolName, err)
	}
	if isAgentWorkflowRunTool(skillID, toolName) {
		execCtx.RuntimeParameters = copyStringAnyMap(execCtx.RuntimeParameters)
		if execCtx.RuntimeParameters == nil {
			execCtx.RuntimeParameters = map[string]interface{}{}
		}
		parentMessageID := strings.TrimSpace(execCtx.MessageID)
		if parentMessageID == "" && prepared != nil && prepared.Message != nil {
			parentMessageID = prepared.Message.ID.String()
		}
		execCtx.RuntimeParameters["workflow_parent_tool_call_id"] = strings.TrimSpace(callID)
		execCtx.RuntimeParameters["workflow_parent_message_id"] = parentMessageID
		execCtx.RuntimeParameters["workflow_parent_conversation_id"] = strings.TrimSpace(execCtx.ConversationID)
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
			if event.Sequence > 0 {
				payload["sequence"] = event.Sequence
			}
			if event.SchemaVersion > 0 {
				payload["schema_version"] = event.SchemaVersion
			}
			if event.PayloadVersion > 0 {
				payload["payload_version"] = event.PayloadVersion
			}
			if event.ExecutionID != "" {
				payload["execution_id"] = event.ExecutionID
			}
			if event.PauseID != "" {
				payload["pause_id"] = event.PauseID
			}
			if event.PauseGeneration > 0 {
				payload["pause_generation"] = event.PauseGeneration
			}
			if event.Type == EventMessage && agentWorkflowInvocationIsDelegate(payload) {
				if chunk, _ := payload["answer"].(string); chunk != "" {
					payload["presentation_role"] = agentWorkflowDelegatePresentationRole
					delegateOutput.append(chunk)
				}
			}
			r.emitEvent(prepared, event.Type, payload)
		})
	}
	ctx = connectordispatch.WithActionPlanPhase(ctx, planPhaseID)
	invocation, err := r.callSkillTool(ctx, resolved, skillID, toolName, toolArgs, execCtx, callID)
	if invocation == nil {
		r.discardAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput)
		if err == nil {
			err = fmt.Errorf("%w: skill tool returned no invocation result", ErrInvalidInput)
		}
		trace := failedSkillTrace("tool_call", toolName, err)
		trace.InvocationID = strings.TrimSpace(callID)
		trace.SkillID = skillID
		trace.Arguments = argumentSummary
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool_name or arguments and retry", skillID, toolName, resolved)), true, false)
	}
	if invocation.Trace.ConnectorNative {
		invocation.Trace.Result = summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages)
		if terminal := connectorTerminalStep(invocation.Trace, callID, runtimeState); terminal != nil {
			r.emitEvent(prepared, EventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace))
			return *terminal
		}
	}
	if err != nil {
		r.discardAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput)
		// CallSkillTool owns the safe trace summary for validation and provider
		// failures. In particular, data-egress tools redact their arguments at
		// that boundary; do not replace the redacted summary with call inputs.
		if len(invocation.Trace.Arguments) == 0 {
			invocation.Trace.Arguments = argumentSummary
		}
		if planPhaseID != "" {
			invocation.Trace.Arguments["plan_phase_id"] = planPhaseID
		}
		if completionIntent != "" {
			invocation.Trace.Arguments["completion_intent"] = completionIntent
		}
		applyPublicToolErrorRecoveryTrace(&invocation.Trace, err)
		if invocation.Trace.ConnectorNative {
			invocation.Trace.Result = summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages)
		}
		if blockedResult := summarizeSkillToolResult(
			invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages,
		); strings.EqualFold(strings.TrimSpace(stringFromInterface(blockedResult["state"])), "blocked") {
			// This is a server-authored terminal control result, not provider
			// output. Surface it once and end the unattended run so the model
			// cannot automatically retry a consequential operation.
			invocation.Trace.Result = blockedResult
			if connectorNeedsPerson(blockedResult) {
				// A change no one was present to approve ran nothing and
				// cannot succeed on a retry: the model finishes its report.
				r.emitEvent(prepared, EventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace))
				return recoverableSkillStep(invocation.Trace, skills.ToolResultMessage(callID, connectorNeedsPersonResult(blockedResult)), true, true)
			}
			toolMessage := skills.ToolResultMessage(callID, blockedResult)
			r.emitEvent(prepared, EventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace))
			terminal := terminalSkillStep(invocation.Trace, toolMessage, true, true)
			terminal.toolResult = blockedResult
			return terminal
		}
		if invocation.Trace.Governance != nil {
			r.emitEvent(prepared, EventToolGovernanceDecision, toolGovernanceDecisionPayload(prepared, invocation.Trace))
		}
		errorPayload := recoverableSkillToolErrorPayload(err, "fix the tool arguments based on the error and retry", skillID, toolName, resolved)
		if code := strings.TrimSpace(invocation.Trace.ErrorCode); code != "" {
			// The model can reason about a stable failure category without
			// receiving provider-specific messages or wrapped upstream causes.
			errorPayload["error"] = code
			errorPayload["error_code"] = code
		}
		if toolName == skills.SkillScriptToolRun && invocation.Trace.Result["exit_code"] != nil && len(invocation.Messages) > 0 {
			// A failed script can still produce authorized diagnostic output.
			// Preserve it without treating the execution or its artifacts as
			// successful completion evidence. Other provider errors retain their
			// existing public error/redaction boundary.
			errorPayload["execution_output"] = invocation.Messages
			for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
				artifact = enrichGeneratedArtifactContentMetadata(artifact, invocation.Trace)
				r.recordArtifact(artifact)
				r.emitEvent(prepared, EventSkillArtifactCreated, artifact)
			}
		}
		return recoverableSkillStep(invocation.Trace, skills.ToolResultMessage(callID, errorPayload), true, false)
	}
	if !traceHasGovernanceArgumentRewrite(invocation.Trace) {
		invocation.Trace.Arguments = argumentSummary
	}
	if planPhaseID != "" {
		if invocation.Trace.Arguments == nil {
			invocation.Trace.Arguments = map[string]interface{}{}
		}
		invocation.Trace.Arguments["plan_phase_id"] = planPhaseID
	}
	if completionIntent != "" {
		if invocation.Trace.Arguments == nil {
			invocation.Trace.Arguments = map[string]interface{}{}
		}
		invocation.Trace.Arguments["completion_intent"] = completionIntent
	}
	applyGovernedAssetArguments(&invocation.Trace)
	if len(planTarget) > 0 {
		if invocation.Trace.Arguments == nil {
			invocation.Trace.Arguments = map[string]interface{}{}
		}
		if governedFileID, ok := invocation.Trace.Arguments["file_id"].(string); ok && strings.TrimSpace(governedFileID) != "" {
			planTarget["file_id"] = strings.TrimSpace(governedFileID)
		}
		invocation.Trace.Arguments["operation_plan_target"] = planTarget
	}
	if invocation.Trace.Governance != nil {
		r.emitEvent(prepared, EventToolGovernanceDecision, toolGovernanceDecisionPayload(prepared, invocation.Trace))
	}
	if summary := summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages); len(summary) > 0 {
		invocation.Trace.Result = summary
	}
	applyStateHandoffAdvisoryToToolMessage(invocation)
	applyPageContextInvalidationAdvisory(invocation)
	guardToolResult := skillToolResultForGuard(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages, invocation.Trace.Result)
	if isAgentKnowledgeRetrievalCall(invocation.Trace.SkillID, invocation.Trace.ToolName) {
		if payload := firstJSONToolInvokePayload(invocation.Messages); len(payload) > 0 {
			guardToolResult = copyStringAnyMap(payload)
		}
	}
	logger.DebugContext(ctx, "aichat skill tool completed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", invocation.Trace.SkillID,
		"tool_name", invocation.Trace.ToolName,
		"duration_ms", invocation.Trace.DurationMS,
	)
	r.emitEvent(prepared, EventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace))
	for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
		artifact = enrichGeneratedArtifactContentMetadata(artifact, invocation.Trace)
		r.recordArtifact(artifact)
		r.emitEvent(prepared, EventSkillArtifactCreated, artifact)
	}
	if artifact := managedFileArtifactFromSaveResult(invocation.Trace, invocation.Messages); len(artifact) > 0 {
		// Saving a temporary artifact must update continuation metadata without
		// emitting a second public file card for the same user-visible file.
		r.recordArtifact(artifact)
	}
	if invocation.ConnectorConfirmation != nil {
		payload := connectorConfirmationRequiredPayload(prepared, invocation.ConnectorConfirmation)
		payload["skill_id"] = invocation.Trace.SkillID
		payload["tool_name"] = invocation.Trace.ToolName
		r.emitEvent(prepared, EventConnectorConfirmation, payload)
		// Connector confirmation uses the durable waiting-client-action host
		// state, while its action type remains reserved for the one-shot opaque
		// receipt endpoint. It never becomes a reusable governance grant.
		return connectorConfirmationPendingStep(
			invocation.Trace, invocation.ToolMessage, guardToolResult, payload,
		)
	}
	if payload := clientActionRequiredPayload(prepared, invocation.Trace, callID); len(payload) > 0 {
		r.emitEvent(prepared, EventClientActionRequired, payload)
		result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
		result.toolResult = guardToolResult
		if clientActionRequiresModelContinuation(payload) {
			result.pendingClientAction = payload
		}
		return result
	}
	if isAgentWorkflowRunTool(invocation.Trace.SkillID, invocation.Trace.ToolName) {
		if payload := agentWorkflowResultPayload(invocation.Messages); len(payload) > 0 {
			if strings.EqualFold(stringFromInterface(payload["status"]), "pending_approval") {
				r.discardAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput)
				result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
				result.toolResult = guardToolResult
				result.pendingApproval = payload
				return result
			}
			if strings.EqualFold(stringFromInterface(payload["status"]), "pending_question") {
				r.discardAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput)
				result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
				result.toolResult = guardToolResult
				result.pendingQuestion = payload
				return result
			}
			if agentWorkflowInvocationIsDelegate(payload) &&
				strings.EqualFold(stringFromInterface(payload["status"]), "succeeded") {
				answer, _ := payload["primary_output"].(string)
				if strings.TrimSpace(answer) != "" {
					result := terminalSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
					result.toolResult = guardToolResult
					result.answer = answer
					result.answerStreamed = r.reconcileAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput, answer)
					return result
				}
			}
		}
		r.discardAgentWorkflowDelegateOutput(ctx, prepared, delegateOutput)
	}
	if toolGovernanceApprovalPending(invocation.Trace) {
		result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
		result.toolResult = guardToolResult
		result.pendingGovernance = toolGovernanceDecisionPayload(prepared, invocation.Trace)
		if planPhaseID != "" {
			result.pendingGovernance["plan_phase_id"] = planPhaseID
		}
		if completionIntent != "" {
			result.pendingGovernance["completion_intent"] = completionIntent
		}
		return result
	}
	result := successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
	result.toolResult = guardToolResult
	return result
}

func agentWorkflowInvocationIsDelegate(payload map[string]interface{}) bool {
	mode := strings.TrimSpace(stringFromInterface(payload["invocation_mode"]))
	if mode != "" {
		return strings.EqualFold(mode, "agent_conversation_delegate")
	}
	return strings.EqualFold(stringFromInterface(payload["agent_type"]), "CONVERSATIONAL_WORKFLOW")
}

const agentWorkflowDelegatePresentationRole = "final_output"

type agentWorkflowDelegateOutput struct {
	mu   sync.Mutex
	text strings.Builder
}

func (o *agentWorkflowDelegateOutput) append(chunk string) {
	if o == nil || chunk == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.text.WriteString(chunk)
}

func (o *agentWorkflowDelegateOutput) snapshot() string {
	if o == nil {
		return ""
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

func (o *agentWorkflowDelegateOutput) take() string {
	if o == nil {
		return ""
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	text := o.text.String()
	o.text.Reset()
	return text
}

func (r *Runner) reconcileAgentWorkflowDelegateOutput(ctx context.Context, prepared *PreparedChat, output *agentWorkflowDelegateOutput, answer string) bool {
	streamed := output.snapshot()
	if streamed == "" {
		return false
	}
	if streamed == answer {
		return true
	}
	if strings.HasPrefix(answer, streamed) {
		suffix := strings.TrimPrefix(answer, streamed)
		if suffix != "" {
			output.append(suffix)
			r.emitEvent(prepared, EventMessage, map[string]interface{}{
				"conversation_id":   prepared.Conversation.ID.String(),
				"message_id":        prepared.Message.ID.String(),
				"answer":            suffix,
				"presentation_role": agentWorkflowDelegatePresentationRole,
			})
		}
		return true
	}

	r.discardAgentWorkflowDelegateOutput(ctx, prepared, output)
	return false
}

func (r *Runner) discardAgentWorkflowDelegateOutput(_ context.Context, prepared *PreparedChat, output *agentWorkflowDelegateOutput) {
	content := output.take()
	if content == "" || prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return
	}
	r.emitEvent(prepared, EventMessageRetract, map[string]interface{}{
		"conversation_id":          prepared.Conversation.ID.String(),
		"message_id":               prepared.Message.ID.String(),
		"content":                  content,
		"length":                   utf16CodeUnitLength(content),
		"created_at":               time.Now().Unix(),
		"presentation_disposition": "discard",
		"presentation_role":        agentWorkflowDelegatePresentationRole,
	})
}

func normalizeSkillToolCompletionIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "finalize_if_success":
		return "finalize_if_success"
	case "continue":
		return "continue"
	default:
		return ""
	}
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

// skillCallReadOnlyKey marks the start of a call whose tool is read-only, so a
// turn that stops before the call's result can repeat it (ADR 0035).
const skillCallReadOnlyKey = "read_only"

// unrecordedCallStep ends a run before a call whose start could not be
// recorded. Had the call run, a process that stops before its result could
// not tell later whether it changed something (ADR 0035).
func unrecordedCallStep(callID, skillID, toolName string, err error) skillStepResult {
	trace := failedSkillTrace("tool_call", toolName, err)
	trace.InvocationID = strings.TrimSpace(callID)
	trace.SkillID = skillID
	return fatalSkillStep(trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"status": "not_run",
		"error":  "the call was not run because its start could not be recorded",
	}), err)
}

func fatalSkillStep(trace skills.SkillTrace, toolMessage adapter.Message, err error) skillStepResult {
	return skillStepResult{
		trace:       trace,
		toolMessage: toolMessage,
		fatalErr:    err,
	}
}
