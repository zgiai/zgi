package skillloop

import (
	"context"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	onEvent func(Event) error,
) skillStepResult {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "fix the JSON arguments and retry the same tool call")), false, false)
	}
	switch call.Function.Name {
	case skills.MetaToolLoadSkill:
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
		if _, ok := loadedSkills[skillID]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), toolName, "skill must be loaded before calling its tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false)
		}
		if doc, ok := resolved.Get(skillID); ok && len(doc.Tools) == 0 {
			trace := blockedSkillGuardrailTrace(skillID, toolName, "skill does not provide callable tools")
			return successfulSkillStep(trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), true, false)
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
		return r.handleRequestUserInputCall(ctx, prepared, call.ID, args, onEvent)
	case skills.MetaToolIntermediateAnswer:
		return r.handleIntermediateAnswerCall(ctx, prepared, call.ID, args, onEvent)
	default:
		err := fmt.Errorf("%w: unsupported skill meta tool %s", ErrInvalidInput, call.Function.Name)
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(call.ID, recoverableErrorPayload(err, "use one of load_skill, request_user_input, read_skill_reference, call_skill_tool, or submit_intermediate_answer")), false, false)
	}
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
		err := fmt.Errorf("%w: intermediate answer content is required", ErrInvalidInput)
		trace := failedSkillTrace("intermediate_answer", "", err)
		return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableErrorPayload(err, "call submit_intermediate_answer again with non-empty content")), false, false)
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
	invocation.Trace.Arguments = argumentSummary
	if err != nil {
		return recoverableSkillStep(invocation.Trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, "fix the tool arguments based on the error and retry", skillID, toolName)), true, false)
	}
	invocation.Trace.Result = summarizeSkillToolResult(invocation.Trace.SkillID, invocation.Trace.ToolName, invocation.Messages)
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
	return successfulSkillStep(invocation.Trace, invocation.ToolMessage, true, true)
}

func isAgentWorkflowRunTool(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentWorkflow) &&
		strings.EqualFold(strings.TrimSpace(toolName), "run_agent_workflow")
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
