package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultMaxSkillPlanningRounds                 = 50
	defaultMaxSkillStepsPerTurn                   = 160
	defaultMaxBusinessToolCallsPerSkill           = 20
	defaultMaxRecoverableFailureRounds            = 12
	defaultMaxConsecutiveRecoverableFailureRounds = 5
	intermediateAnswerChunkRunes                  = 180
	agentProgressMaxRunes                         = 96
	streamedIntermediateAnswerArg                 = "_aichat_streamed_answer"
)

var agentProgressUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

type skillStepResult struct {
	trace               skills.SkillTrace
	toolMessage         adapter.Message
	toolResult          map[string]interface{}
	answer              string
	usedSkill           bool
	usedTool            bool
	recoverable         bool
	terminal            bool
	pendingApproval     map[string]interface{}
	pendingQuestion     map[string]interface{}
	pendingGovernance   map[string]interface{}
	pendingClientAction map[string]interface{}
	fatalErr            error
}

type planningResult struct {
	message          adapter.Message
	usage            *adapter.Usage
	answerStreamed   bool
	progressStreamed bool
}

type streamingToolCallState struct {
	call                    adapter.ToolCall
	emittedContent          string
	emittedPlanningProgress bool
	emittedPlanningSkillID  string
	emittedPlanningToolName string
}

func (r *Runner) Run(ctx context.Context, req RunRequest) (string, *adapter.Usage, error) {
	prepared := req.Prepared
	resolved := req.Resolved
	if r == nil || r.SkillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if r.LLMClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
	if prepared == nil || prepared.LLMRequest == nil {
		return "", nil, fmt.Errorf("%w: prepared chat is invalid", ErrInvalidInput)
	}
	if resolved == nil || len(resolved.Skills) == 0 {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	messages = append(messages, metadataMessage)
	messages = append(messages, validAdditionalSystemMessages(req.AdditionalSystemMessages)...)
	messages = append(messages, agenticSkillLoopSystemMessage())
	traces := []skills.SkillTrace{metadataExposedTrace(resolved.SkillIDs(), metadataStats)}
	r.recordTrace(traces, traces[0])
	logger.DebugContext(ctx, "aichat skill metadata exposed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_ids", resolved.SkillIDs(),
		"skill_mode", prepared.parts.SkillMode,
	)

	stepCount := 0
	toolCallCount := 0
	recoverableFailureRoundCount := 0
	consecutiveRecoverableFailureRounds := 0
	recoverableFailureCallCount := 0
	finalAnswerGuardBlockCount := 0
	completionVerificationRetryCount := 0
	finalizingProgressEmitted := false
	skillToolCallCounts := map[string]int{}
	attemptedToolCalls := []SkillToolCallRef{}
	successfulToolCalls := []SkillToolCallRef{}
	failedToolCallReasons := map[string]string{}
	skillUsed := false
	loadedSkills := map[string]struct{}{}
	maxSkillSteps := maxSkillStepsForTurn(resolved)
	postVerificationConfigured := req.CompletionEvidence != nil
	finalAnswerGuard := req.FinalAnswerGuard
	userInputGuard := req.UserInputGuard
	toolCallGuard := req.ToolCallGuard
	if postVerificationConfigured {
		// Model post verification replaces legacy answer/tool-alignment guardrails for agentic turns.
		// Keep the user-input guard so redundant clarification requests can still replan instead of
		// interrupting a task that already has enough evidence to continue.
		// Tool governance and backend authorization still enforce hard safety boundaries.
		finalAnswerGuard = nil
		toolCallGuard = nil
	}
	suppressFinalAnswerStream := false
	if postVerificationConfigured {
		suppressFinalAnswerStream = completionVerificationShouldRun(req.CompletionEvidence(), nil, nil, 0)
	}
	var answerBuilder strings.Builder
	var usage *adapter.Usage

	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = skills.MetaToolsForSkillState(resolved, loadedSkills)
		planningReq.ToolChoice = "auto"

		planningResult, err := r.runSkillPlanning(ctx, prepared, planningReq, round, req.OnChunk, suppressFinalAnswerStream)
		if err != nil {
			return answerBuilder.String(), usage, err
		}
		usage = mergeUsage(usage, planningResult.usage)
		planningMessage := planningResult.message
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		text := assistantMessageText(planningMessage)
		if text != "" && len(toolCalls) > 0 && !planningResult.progressStreamed {
			r.emitAgentProgress(ctx, prepared, text, nil)
		}
		if len(toolCalls) == 0 {
			if guardResult, blocked := runFinalAnswerGuard(finalAnswerGuard, FinalAnswerGuardRequest{
				Answer:              text,
				Round:               round,
				SkillUsed:           skillUsed,
				ToolCallCount:       toolCallCount,
				AttemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
				SuccessfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
			}); blocked {
				finalAnswerGuardBlockCount++
				if planningResult.answerStreamed && text != "" {
					r.emitAnswerRetract(ctx, prepared, text, nil)
				}
				trace := finalAnswerGuardrailTrace(guardResult)
				traces = append(traces, trace)
				r.recordTrace(traces, trace)
				r.logSkillTrace(ctx, prepared, trace)
				if finalAnswerGuardBlockCount > defaultMaxConsecutiveRecoverableFailureRounds {
					err := fmt.Errorf("%w: final answer guard blocked too many consecutive replies", ErrInvalidInput)
					r.emitSkillError(ctx, prepared, failedSkillTrace("guardrail", guardResult.ToolName, err))
					return answerBuilder.String(), usage, err
				}
				messages = append(messages, finalAnswerGuardSystemMessage(guardResult, text))
				continue
			}
		}
		if len(toolCalls) == 0 && postVerificationConfigured {
			if !finalizingProgressEmitted && (toolCallCount > 0 || len(attemptedToolCalls) > 0 || len(successfulToolCalls) > 0) {
				if r.emitAgentProgress(ctx, prepared, completionVerificationFinalizingProgressText(prepared, completionEvidenceForFastPath(req)), nil) {
					finalizingProgressEmitted = true
				}
			}
			decision, verifierUsage, err := r.runCompletionVerifier(ctx, prepared, req, text, round, attemptedToolCalls, successfulToolCalls, toolCallCount)
			usage = mergeUsage(usage, verifierUsage)
			if err != nil {
				logger.WarnContext(ctx, "aichat completion verifier failed; using conservative fallback answer",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					err,
				)
				text = completionVerificationFallbackAnswer(completionVerificationDecision{
					Status: completionVerificationStatusFailed,
					Reason: "\u6700\u7ec8\u7b54\u6848\u540e\u6821\u9a8c\u6682\u65f6\u4e0d\u53ef\u7528\uff0c\u56e0\u6b64\u4e0d\u80fd\u53ef\u9760\u786e\u8ba4\u672c\u8f6e\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002",
				}, text)
			} else {
				switch decision.normalizedStatus() {
				case completionVerificationStatusPass:
					completionVerificationRetryCount = 0
				case completionVerificationStatusNeedsAction:
					completionVerificationRetryCount++
					if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
						text = completionVerificationFallbackAnswer(decision, text)
					} else {
						messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount))
						continue
					}
				case completionVerificationStatusFailed, completionVerificationStatusAskUser:
					if replacement := strings.TrimSpace(decision.FinalAnswer); replacement != "" {
						text = replacement
						completionVerificationRetryCount = 0
					} else {
						completionVerificationRetryCount++
						if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
							text = completionVerificationFallbackAnswer(decision, text)
						} else {
							messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount))
							continue
						}
					}
				default:
					completionVerificationRetryCount++
					if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
						text = completionVerificationFallbackAnswer(decision, text)
					} else {
						messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount))
						continue
					}
				}
			}
		}
		if len(toolCalls) == 0 && prepared.parts.SkillMode == "required" && !skillUsed {
			return answerBuilder.String(), usage, fmt.Errorf("%w: required skill was not used", ErrInvalidInput)
		}
		if text != "" && len(toolCalls) == 0 {
			answerBuilder.WriteString(text)
			if !planningResult.answerStreamed {
				r.emitAnswerChunk(ctx, prepared, text, nil)
			}
		}
		if len(toolCalls) == 0 {
			logger.DebugContext(ctx, "aichat skill planning completed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"skill_step_count", stepCount,
				"tool_call_count", toolCallCount,
			)
			return answerBuilder.String(), usage, nil
		}
		if stepCount+len(toolCalls) > maxSkillSteps {
			logger.WarnContext(ctx, "aichat skill step limit exceeded",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"current_step_count", stepCount,
				"requested_tool_calls", len(toolCalls),
				"max_steps", maxSkillSteps,
			)
			return answerBuilder.String(), usage, fmt.Errorf("%w: too many skill steps", ErrInvalidInput)
		}
		logger.DebugContext(ctx, "aichat skill planning requested tool calls",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"tool_call_count", len(toolCalls),
			"step_count", stepCount,
		)

		planningMessage.Role = "assistant"
		planningMessage.ToolCalls = toolCalls
		messages = append(messages, planningMessage)

		roundHadRecoverableFailure := false
		roundHadSuccess := false
		var lastRecoverableTrace skills.SkillTrace
		for _, call := range toolCalls {
			stepCount++
			callSkillID, callToolName, callToolArgs, failedCallKey := repeatedFailedToolCallKeyForCall(call)
			result := skillStepResult{}
			if failedCallKey != "" {
				if reason := failedToolCallReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
					result = repeatedFailedToolCallRecoverableStep(call.ID, callSkillID, callToolName, callToolArgs, reason)
				}
			}
			if result.trace.Kind == "" {
				result = r.handleProgressiveSkillCall(ctx, prepared, resolved, call, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, userInputGuardState{
					guard:               userInputGuard,
					toolCallGuard:       toolCallGuard,
					planToolGuard:       req.PlanToolGuard,
					round:               round,
					skillUsed:           skillUsed,
					toolCallCount:       toolCallCount,
					attemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
					successfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
				}, nil)
			}
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			if result.recoverable && failedCallKey != "" && strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") {
				failedToolCallReasons[failedCallKey] = strings.TrimSpace(result.trace.Error)
				if failedToolCallReasons[failedCallKey] == "" {
					failedToolCallReasons[failedCallKey] = "previous tool call with the same arguments failed"
				}
			}
			if result.recoverable {
				if !internalPlannerFeedbackTrace(result.trace) {
					r.emitSkillError(ctx, prepared, result.trace)
				}
				roundHadRecoverableFailure = true
				lastRecoverableTrace = result.trace
				recoverableFailureCallCount++
			} else {
				roundHadSuccess = true
			}
			if result.fatalErr != nil {
				if !result.recoverable {
					r.emitSkillError(ctx, prepared, result.trace)
				}
				return answerBuilder.String(), usage, result.fatalErr
			}
			if result.usedSkill {
				skillUsed = true
			}
			if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") {
				attemptedToolCalls = append(attemptedToolCalls, SkillToolCallRef{
					SkillID:   strings.TrimSpace(result.trace.SkillID),
					ToolName:  strings.TrimSpace(result.trace.ToolName),
					Arguments: copyStringAnyMap(result.trace.Arguments),
					Result:    copyStringAnyMap(result.toolResult),
				})
			}
			if result.usedTool {
				toolCallCount++
				incrementSkillToolCallCount(skillToolCallCounts, result.trace.SkillID)
				if strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
					strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
					successfulToolCalls = append(successfulToolCalls, SkillToolCallRef{
						SkillID:   strings.TrimSpace(result.trace.SkillID),
						ToolName:  strings.TrimSpace(result.trace.ToolName),
						Arguments: copyStringAnyMap(result.trace.Arguments),
						Result:    copyStringAnyMap(result.toolResult),
					})
					finalAnswerGuardBlockCount = 0
				}
			}
			if result.pendingApproval != nil {
				return answerBuilder.String(), usage, &WorkflowApprovalPendingError{Payload: result.pendingApproval}
			}
			if result.pendingQuestion != nil {
				return answerBuilder.String(), usage, &WorkflowQuestionPendingError{Payload: result.pendingQuestion}
			}
			if result.pendingGovernance != nil {
				return answerBuilder.String(), usage, &ToolGovernancePendingError{Payload: result.pendingGovernance}
			}
			if result.pendingClientAction != nil {
				return answerBuilder.String(), usage, &ClientActionPendingError{Payload: result.pendingClientAction}
			}
			if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(result.trace, completionEvidenceForFastPath(req)); ok {
				appendAnswerText(&answerBuilder, answer)
				r.emitAnswerChunk(ctx, prepared, answer, nil)
				logger.DebugContext(ctx, "aichat skill loop completed through tool result fast path",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_id", result.trace.SkillID,
					"tool_name", result.trace.ToolName,
				)
				return answerBuilder.String(), usage, nil
			}
			if result.answer != "" {
				appendAnswerText(&answerBuilder, result.answer)
				r.emitAnswerChunk(ctx, prepared, result.answer, nil)
			}
			if result.terminal {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				return answerBuilder.String(), usage, nil
			}
			messages = append(messages, result.toolMessage)
			if message, ok := governedReadFileTargetSystemMessage(result.trace); ok {
				messages = append(messages, message)
			}
		}
		if roundHadRecoverableFailure {
			recoverableFailureRoundCount++
			if !roundHadSuccess {
				consecutiveRecoverableFailureRounds++
			} else {
				consecutiveRecoverableFailureRounds = 0
			}
			logger.DebugContext(ctx, "aichat skill recoverable failures observed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"failure_round_count", recoverableFailureRoundCount,
				"consecutive_failure_rounds", consecutiveRecoverableFailureRounds,
				"failure_call_count", recoverableFailureCallCount,
			)
			if recoverableFailureRoundCount > defaultMaxRecoverableFailureRounds ||
				consecutiveRecoverableFailureRounds > defaultMaxConsecutiveRecoverableFailureRounds {
				err := fmt.Errorf("%w: too many failed skill calls", ErrInvalidInput)
				trace := failedSkillTrace(lastRecoverableTrace.Kind, lastRecoverableTrace.ToolName, err)
				trace.SkillID = lastRecoverableTrace.SkillID
				trace.Arguments = lastRecoverableTrace.Arguments
				if !internalPlannerFeedbackTrace(lastRecoverableTrace) {
					r.emitSkillError(ctx, prepared, trace)
				}
				if postVerificationConfigured {
					text := recoverableFailureFinalAnswer(lastRecoverableTrace, err)
					appendAnswerText(&answerBuilder, text)
					r.emitAnswerChunk(ctx, prepared, text, nil)
					return answerBuilder.String(), usage, nil
				}
				return answerBuilder.String(), usage, err
			}
		} else {
			consecutiveRecoverableFailureRounds = 0
		}
	}

	err := fmt.Errorf("%w: too many skill planning rounds", ErrInvalidInput)
	if postVerificationConfigured {
		text := planningRoundsExhaustedFinalAnswer(err)
		appendAnswerText(&answerBuilder, text)
		r.emitAnswerChunk(ctx, prepared, text, nil)
		return answerBuilder.String(), usage, nil
	}
	return answerBuilder.String(), usage, err
}

func repeatedFailedToolCallKeyForCall(call adapter.ToolCall) (string, string, map[string]interface{}, string) {
	if !strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolCallSkillTool) {
		return "", "", nil, ""
	}
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		return "", "", nil, ""
	}
	skillID := normalizedSkillArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	return skillID, toolName, toolArgs, failedToolCallKey(skillID, toolName, toolArgs)
}

func failedToolCallKey(skillID string, toolName string, args map[string]interface{}) string {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if skillID == "" || toolName == "" {
		return ""
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		encoded = []byte(fmt.Sprint(args))
	}
	return skillID + "/" + toolName + ":" + string(encoded)
}

func repeatedFailedToolCallRecoverableStep(callID string, skillID string, toolName string, args map[string]interface{}, reason string) skillStepResult {
	message := "same tool call with the same arguments already failed in this turn"
	if reason = strings.TrimSpace(reason); reason != "" {
		message += ": " + reason
	}
	err := fmt.Errorf("%w: %s", ErrInvalidInput, message)
	trace := plannerFeedbackTrace(skillID, toolName, err)
	if len(args) > 0 {
		trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
		trace.Arguments["next_step"] = "continue_planning"
	}
	nextAction := strings.Join([]string{
		"Do not repeat the same tool with identical arguments.",
		"Change the arguments based on the previous error only if a safe retry is available.",
		"Otherwise answer truthfully from the failed tool result.",
	}, " ")
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, recoverableSkillToolErrorPayload(err, nextAction, skillID, toolName)), false, false)
}

func recoverableFailureFinalAnswer(trace skills.SkillTrace, err error) string {
	reason := strings.TrimSpace(trace.Error)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	step := strings.TrimSpace(trace.SkillID)
	if toolName := strings.TrimSpace(trace.ToolName); toolName != "" {
		if step != "" {
			step += "/"
		}
		step += toolName
	}
	decision := completionVerificationDecision{
		Status: completionVerificationStatusFailed,
		Reason: reason,
	}
	if step != "" {
		decision.MissingSteps = []string{step}
	}
	return completionVerificationFallbackAnswer(decision, "")
}

func planningRoundsExhaustedFinalAnswer(err error) string {
	reason := "执行规划轮次已达到上限，无法确认本轮操作已经完成。"
	if err != nil {
		reason = reason + " " + err.Error()
	}
	return completionVerificationFallbackAnswer(completionVerificationDecision{
		Status: completionVerificationStatusFailed,
		Reason: reason,
	}, "")
}

func validAdditionalSystemMessages(input []adapter.Message) []adapter.Message {
	out := make([]adapter.Message, 0, len(input))
	for _, message := range input {
		content := strings.TrimSpace(messageContent(message.Content))
		if content == "" {
			continue
		}
		message.Role = "system"
		message.Content = content
		message.ToolCalls = nil
		out = append(out, message)
	}
	return out
}

func governedReadFileTargetSystemMessage(trace skills.SkillTrace) (adapter.Message, bool) {
	if trace.Governance == nil ||
		!strings.EqualFold(strings.TrimSpace(trace.SkillID), skills.SkillFileReader) ||
		!strings.EqualFold(strings.TrimSpace(trace.ToolName), "read_file") {
		return adapter.Message{}, false
	}
	decision := trace.Governance
	if decision.Status != toolgovernance.DecisionStatusAllowed ||
		decision.Manifest.Effect != toolgovernance.EffectRead ||
		!strings.EqualFold(strings.TrimSpace(decision.Manifest.AssetType), "file") {
		return adapter.Message{}, false
	}
	assets := decision.ExpectedAssets
	if len(assets) == 0 {
		assets = decision.Assets
	}
	if len(assets) != 1 {
		return adapter.Message{}, false
	}
	fileID := strings.TrimSpace(assets[0].ID)
	fileName := strings.TrimSpace(assets[0].Name)
	if fileID == "" && fileName == "" {
		return adapter.Message{}, false
	}
	target := fileName
	if target == "" {
		target = fileID
	}
	content := strings.Join([]string{
		"Authoritative files-page target feedback:",
		fmt.Sprintf("The tool result above is for the resolved file target %q.", target),
		"Use that resolved file name and the returned file content as the only source for the final answer.",
		"Any earlier assistant progress text, assistant tool-call arguments, or visible-file ordinal interpretation that named a different file is incorrect for this turn.",
		"Do not mention this correction, internal resolution, governance, redirects, caches, mismatched IDs, or internal file IDs in the final answer. Simply answer the user's request from the resolved file content.",
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func runFinalAnswerGuard(guard FinalAnswerGuard, req FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The previous candidate final answer was blocked because a required skill/tool call has not succeeded in this turn. Continue planning and call the required skill/tool before claiming completion."
	}
	return result, true
}

func runUserInputGuard(guard UserInputGuard, req UserInputGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The requested user clarification was blocked because runtime context already contains the information needed to continue. Continue planning and call the required skill/tool before asking the user."
	}
	return result, true
}

func runToolCallGuard(guard ToolCallGuard, req ToolCallGuardRequest) (FinalAnswerGuardResult, bool) {
	if guard == nil {
		return FinalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return FinalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The requested skill tool call was blocked because it would run the task in the wrong order. Continue planning and call the required skill/tool first."
	}
	return result, true
}

func finalAnswerGuardrailTrace(result FinalAnswerGuardResult) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(result.SkillID),
		ToolName: strings.TrimSpace(result.ToolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(result.Message),
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}
}

func toolCallGuardrailTrace(result FinalAnswerGuardResult, blockedSkillID string, blockedToolName string, blockedArguments map[string]interface{}) skills.SkillTrace {
	trace := finalAnswerGuardrailTrace(result)
	trace.Arguments = map[string]interface{}{
		"blocked_tool": strings.TrimSpace(blockedSkillID) + "/" + strings.TrimSpace(blockedToolName),
		"next_step":    "continue_planning",
	}
	if len(blockedArguments) > 0 {
		trace.Arguments["blocked_arguments"] = summarizeSkillToolArguments(blockedSkillID, blockedToolName, blockedArguments)
	}
	return trace
}

func userInputGuardrailTrace(result FinalAnswerGuardResult) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(result.SkillID),
		ToolName: strings.TrimSpace(result.ToolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(result.Message),
		Arguments: map[string]interface{}{
			"blocked_tool": "request_user_input",
			"next_step":    "continue_planning",
		},
	}
}

func finalAnswerGuardSystemMessage(result FinalAnswerGuardResult, candidateAnswer string) adapter.Message {
	feedback := strings.TrimSpace(result.SystemMessage)
	if feedback == "" {
		feedback = strings.TrimSpace(result.Message)
	}
	lines := []string{
		"Runtime guardrail feedback:",
		feedback,
	}
	if text := strings.TrimSpace(candidateAnswer); text != "" {
		lines = append(lines, "Blocked candidate answer:\n"+text)
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func messageContent(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func appendAnswerText(builder *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if builder.Len() > 0 {
		current := builder.String()
		if !strings.HasSuffix(current, "\n") {
			builder.WriteString("\n\n")
		}
	}
	builder.WriteString(text)
}

func (r *Runner) runSkillPlanning(ctx context.Context, prepared *PreparedChat, planningReq *adapter.ChatRequest, round int, onChunk func(string) error, suppressFinalAnswerStream bool) (planningResult, error) {
	if !suppressFinalAnswerStream && shouldStreamSkillPlanning(prepared) {
		result, ok, err := r.runSkillPlanningStream(ctx, prepared, planningReq, round, nil)
		if err != nil {
			return planningResult{}, err
		}
		if ok {
			return result, nil
		}
	}

	planningReq.Stream = false
	startedAt := time.Now()
	planningResp, err := r.LLMClient.AppChat(ctx, r.AppContext, planningReq)
	if err != nil {
		r.recordModelInvocation(ModelInvocationTrace{
			Phase:      "skill_planning",
			Round:      round,
			Streaming:  false,
			StartedAt:  startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Request:    planningReq,
			Error:      err.Error(),
		})
		return planningResult{}, err
	}
	message := firstPlanningMessage(planningResp)
	usage := planningRespUsage(planningResp)
	r.recordModelInvocation(ModelInvocationTrace{
		Phase:      "skill_planning",
		Round:      round,
		Streaming:  false,
		StartedAt:  startedAt,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Request:    planningReq,
		Response:   &message,
		Usage:      usage,
	})
	return planningResult{message: message, usage: usage}, nil
}

func (r *Runner) emitAnswerChunk(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) {
	if text == "" {
		return
	}
	r.emitEvent(EventMessage, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"answer":          text,
	})
}

func (r *Runner) emitAnswerRetract(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) {
	if text == "" {
		return
	}
	r.emitEvent(EventMessageRetract, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         text,
		"length":          utf16CodeUnitLength(text),
		"created_at":      time.Now().Unix(),
	})
}

func utf16CodeUnitLength(text string) int {
	return len(utf16.Encode([]rune(text)))
}

func (r *Runner) emitAgentProgress(ctx context.Context, prepared *PreparedChat, text string, _ func(Event) error) bool {
	content := visibleAgentProgressText(text)
	if content == "" {
		return false
	}
	r.emitEvent(EventAgentProgress, map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"content":         content,
		"created_at":      time.Now().Unix(),
	})
	return true
}

func completionVerificationFinalizingProgressText(prepared *PreparedChat, evidence map[string]interface{}) string {
	if text := completionVerificationOperationProgressText(prepared, evidence); text != "" {
		return text
	}
	if containsCJK(preparedUserText(prepared)) {
		return "\u6b63\u5728\u6839\u636e\u5de5\u5177\u7ed3\u679c\u6574\u7406\u56de\u590d..."
	}
	return "Reviewing the tool results before the final reply..."
}

func completionVerificationOperationProgressText(prepared *PreparedChat, evidence map[string]interface{}) string {
	summary := completionVerificationProgressOperationSummary(evidence)
	if len(summary) == 0 {
		return ""
	}
	chinese := containsCJK(preparedUserText(prepared))
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(summary["status"])))
	operation := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(summary["operation"])))
	assetType := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(summary["asset_type"])))
	effect := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(summary["effect"])))
	successCount := firstPositiveInt(
		completionVerificationNumericEvidence(summary["success_count"]),
		completionVerificationNumericEvidence(summary["deleted_count"]),
	)
	targetCount := completionVerificationNumericEvidence(summary["target_count"])
	failedCount := completionVerificationNumericEvidence(summary["failed_count"])
	generatedFileCount := completionVerificationNumericEvidence(summary["generated_file_count"])

	if chinese {
		switch {
		case failedCount > 0 && successCount > 0:
			return fmt.Sprintf("\u5df2\u5b8c\u6210 %d \u9879\u64cd\u4f5c\uff0c%d \u9879\u5931\u8d25\uff0c\u6b63\u5728\u6574\u7406\u7ed3\u679c...", successCount, failedCount)
		case isDeleteOperation(operation, effect) && assetType == "agent" && successCount > 0:
			return fmt.Sprintf("\u5df2\u5220\u9664 %d \u4e2a\u667a\u80fd\u4f53\uff0c\u6b63\u5728\u786e\u8ba4\u7ed3\u679c...", successCount)
		case isDeleteOperation(operation, effect) && targetCount > 0:
			return fmt.Sprintf("\u5df2\u5904\u7406 %d \u4e2a\u5220\u9664\u76ee\u6807\uff0c\u6b63\u5728\u6574\u7406\u7ed3\u679c...", targetCount)
		case generatedFileCount > 0:
			return "\u6587\u4ef6\u5df2\u751f\u6210\uff0c\u6b63\u5728\u6574\u7406\u7ed3\u679c..."
		case status == "success" || status == "succeeded" || status == "completed":
			return "\u5de5\u5177\u5df2\u6267\u884c\uff0c\u6b63\u5728\u6574\u7406\u7ed3\u679c..."
		}
		return ""
	}

	switch {
	case failedCount > 0 && successCount > 0:
		return fmt.Sprintf("Completed %d item(s), %d failed; reviewing the result...", successCount, failedCount)
	case isDeleteOperation(operation, effect) && assetType == "agent" && successCount > 0:
		return fmt.Sprintf("Deleted %d agent(s); confirming the result...", successCount)
	case isDeleteOperation(operation, effect) && targetCount > 0:
		return fmt.Sprintf("Processed %d delete target(s); reviewing the result...", targetCount)
	case generatedFileCount > 0:
		return "File generated; reviewing the result..."
	case status == "success" || status == "succeeded" || status == "completed":
		return "Tool completed; reviewing the result..."
	default:
		return ""
	}
}

func completionVerificationProgressOperationSummary(evidence map[string]interface{}) map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	if summary := evidenceMapFromAny(evidence["operation_result_summary"]); len(summary) > 0 {
		return summary
	}
	if summary := evidenceMapFromAny(evidence["execution_summary"]); len(summary) > 0 {
		if operationSummary := evidenceMapFromAny(summary["operation_result_summary"]); len(operationSummary) > 0 {
			return operationSummary
		}
	}
	if ledger := evidenceMapFromAny(evidence["execution_ledger"]); len(ledger) > 0 {
		if operationSummary := evidenceMapFromAny(ledger["operation_result_summary"]); len(operationSummary) > 0 {
			return operationSummary
		}
		if summary := evidenceMapFromAny(ledger["summary"]); len(summary) > 0 {
			if operationSummary := evidenceMapFromAny(summary["operation_result_summary"]); len(operationSummary) > 0 {
				return operationSummary
			}
		}
	}
	return nil
}

func isDeleteOperation(operation string, effect string) bool {
	return strings.Contains(operation, "delete") || strings.EqualFold(effect, "delete")
}

func preparedUserText(prepared *PreparedChat) string {
	if prepared == nil || prepared.LLMRequest == nil {
		return ""
	}
	for i := len(prepared.LLMRequest.Messages) - 1; i >= 0; i-- {
		message := prepared.LLMRequest.Messages[i]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			continue
		}
		if text := strings.TrimSpace(messageContent(message.Content)); text != "" {
			return text
		}
	}
	return ""
}

func visibleAgentProgressText(text string) string {
	content := strings.TrimSpace(text)
	if content == "" {
		return ""
	}
	content = strings.Join(strings.Fields(content), " ")
	content = firstAgentProgressSentence(content)
	if content == "" || looksLikeInternalAgentProgress(content) {
		return ""
	}
	return truncateRunes(content, agentProgressMaxRunes)
}

func looksLikeInternalAgentProgress(text string) bool {
	content := strings.TrimSpace(text)
	if content == "" {
		return false
	}
	lower := strings.ToLower(content)
	if agentProgressUUIDPattern.MatchString(content) ||
		strings.Contains(content, "id:") ||
		strings.Contains(content, "(id") ||
		strings.Contains(content, "\uff08id") {
		return true
	}
	for _, fragment := range []string{
		"list_agents",
		"get_agent",
		"get_agent_config",
		"update_agent",
		"delete_agent",
		"delete_agents",
		"load_skill",
		"read_skill_reference",
		"call_skill_tool",
		"submit_intermediate_answer",
		"request_user_input",
		"operation_plan",
		"required_next_tool",
		"tool_call",
		"runtime_id",
		"correlation_id",
		"message_id",
		"conversation_id",
		"skill_step",
		"tool_call_count",
	} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func firstAgentProgressSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for index, char := range text {
		switch char {
		case '\n', '。', '！', '？', '!', '?':
			return strings.TrimSpace(text[:index+len(string(char))])
		case '.':
			rest := text[index+1:]
			if rest == "" {
				return strings.TrimSpace(text[:index+1])
			}
			for _, next := range rest {
				if unicode.IsSpace(next) {
					return strings.TrimSpace(text[:index+1])
				}
				break
			}
		}
	}
	return text
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func agenticSkillLoopSystemMessage() adapter.Message {
	return adapter.Message{Role: "system", Content: strings.Join([]string{
		"When using skills or tools, you may provide at most one brief, high-level user-facing progress sentence when it helps the user understand a multi-step operation.",
		"Do not narrate every tool call, internal plan step, tool name, tool arguments, IDs, protocol details, or bookkeeping status.",
		"When an additional system message contains required_next_tool, treat it as an important planned next step, not as a reason to ignore fresh evidence. Load and call it when the current page context and prior tool/client-action evidence show it is still needed; do not repeat the same navigation or business tool after matching evidence already satisfies the step.",
		"After each skill/tool result, continue with the next necessary action or final answer. Summarize only user-relevant outcomes, not internal bookkeeping.",
		"If a tool call fails, explain the likely user-relevant cause, fix the arguments, and retry when possible.",
		"If a tool call fails, do not repeat the same tool with the same arguments. Re-plan from the error before retrying.",
		"For deterministic batch work, prefer one suitable business tool call that handles the batch coherently over many small repeated tool calls.",
		"Do not claim that you saved, remembered, updated, deleted, sent, created, changed, or completed any external action unless the corresponding skill/tool call succeeded in this turn.",
		"Progress text sent together with tool calls is transient status text. Keep it short and do not place substantial user deliverables there.",
		"submit_intermediate_answer is for substantial user-facing deliverables only; do not use it for progress, plans, tool status, internal reasoning, or protocol narration.",
		"If the current turn newly creates or substantially rewrites a user-facing deliverable before later tool/skill calls, call submit_intermediate_answer for that new deliverable before continuing.",
		"Examples of new deliverables that should use submit_intermediate_answer when followed by more tool/skill calls: novel outlines, long-form drafts, plans, tables, code sketches, analysis sections, or generated content the user asked for.",
		"Do not call submit_intermediate_answer merely to repeat content that was already visible in an earlier assistant answer. For requests like exporting, saving, converting, or generating a file from existing content, pass the existing content directly to the file/tool call.",
		"Do not skip submit_intermediate_answer by postponing or summarizing a new deliverable if the user explicitly asked for it as an intermediate phase.",
		"When required information is missing or ambiguity blocks reliable progress, call request_user_input with a brief user-visible message plus a questions array containing one to five concise questions, then stop. The message should explain what you checked, why input is needed, and what you will do next. Prefer one to three questions. Do not call any other tools in the same turn after request_user_input.",
		"When calling request_user_input, put the user-visible explanation only in the request_user_input message field. Do not also repeat that explanation in assistant text outside the tool call.",
		"Each request_user_input question should ask one decision point. Include options only when each option is a concrete, directly usable answer. Do not include vague options such as free choice, freestyle, not sure, depends, any, or other; omit options for open-ended questions because the user can type freely.",
		"Do not use request_user_input for information already confirmed in the conversation.",
		"When no more tool or skill calls are needed, send a natural user-facing reply that is complete and self-contained. If you did not call submit_intermediate_answer for a new requested deliverable, that reply MUST include the deliverable in full, not a compressed summary.",
		"Do not label the user-facing reply with protocol wording such as Final Answer, final result, or their Chinese equivalents unless the user explicitly asks for that wording.",
		"When reusing existing conversation content, refer to it explicitly, for example as the previous outline or the current branch's draft; do not duplicate the full text unless the user asks to see it again.",
	}, "\n")}
}

func AgenticSkillLoopSystemMessage() adapter.Message {
	return agenticSkillLoopSystemMessage()
}
