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
	"unicode/utf8"

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

var (
	agentProgressUUIDPattern           = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	incompleteAgentProgressListPattern = regexp.MustCompile(`(?i)([:：]\s*)?(\d+[\.\)、)]?|[-*•])\s*$`)
)

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
	pendingUserInput    map[string]interface{}
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
	evidenceContinuationRetryCount := 0
	finalizingProgressEmitted := false
	forcedToolChoiceForNextRound := interface{}(nil)
	skillToolCallCounts := map[string]int{}
	attemptedToolCalls := []SkillToolCallRef{}
	successfulToolCalls := []SkillToolCallRef{}
	successfulToolCallsByKey := map[string]SkillToolCallRef{}
	failedToolCallReasons := map[string]string{}
	skillUsed := false
	loadedSkills := initialLoadedSkillsForRun(req, resolved)
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
	if postVerificationConfigured {
		evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, nil)
		if feedback, shouldContinue := completionEvidenceContinuationSystemMessage(evidence, evidenceContinuationRetryCount, resolved); shouldContinue {
			evidenceContinuationRetryCount++
			messages = append(messages, feedback)
			forcedToolChoiceForNextRound = completionEvidenceContinuationToolChoice(evidence, loadedSkills, resolved)
		}
	}

	for round := 0; round < defaultMaxSkillPlanningRounds; round++ {
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = skills.MetaToolsForSkillState(resolved, loadedSkills)
		planningReq.ToolChoice = "auto"
		if forcedToolChoiceForNextRound != nil {
			planningReq.ToolChoice = forcedToolChoiceForNextRound
			forcedToolChoiceForNextRound = nil
		}

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
			evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
			if answer, ok := immediateCompletionEvidenceFastPathAnswer(evidence); ok {
				if planningResult.answerStreamed && text != "" {
					r.emitAnswerRetract(ctx, prepared, text, nil)
				}
				appendAnswerText(&answerBuilder, answer)
				r.emitAnswerChunk(ctx, prepared, answer, nil)
				logger.DebugContext(ctx, "aichat skill loop completed from immediate completion evidence fast path",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
				)
				return answerBuilder.String(), usage, nil
			}
			if feedback, shouldContinue := completionEvidenceContinuationSystemMessage(evidence, evidenceContinuationRetryCount, resolved); shouldContinue {
				evidenceContinuationRetryCount++
				if evidenceContinuationRetryCount <= defaultMaxCompletionVerificationRetries {
					if planningResult.answerStreamed && text != "" {
						r.emitAnswerRetract(ctx, prepared, text, nil)
					}
					messages = append(messages, feedback)
					forcedToolChoiceForNextRound = completionEvidenceContinuationToolChoice(evidence, loadedSkills, resolved)
					continue
				}
			}
			if !finalizingProgressEmitted && (toolCallCount > 0 || len(attemptedToolCalls) > 0 || len(successfulToolCalls) > 0) {
				if r.emitAgentProgress(ctx, prepared, completionVerificationFinalizingProgressText(prepared, evidence), nil) {
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
				decision := completionVerificationDecision{
					Status: completionVerificationStatusFailed,
					Reason: "\u6700\u7ec8\u7b54\u6848\u540e\u6821\u9a8c\u6682\u65f6\u4e0d\u53ef\u7528\uff0c\u56e0\u6b64\u4e0d\u80fd\u53ef\u9760\u786e\u8ba4\u672c\u8f6e\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002",
				}
				if answer, ok := completionEvidenceVerifiedFinalAnswer(req, successfulToolCalls, text); ok {
					text = answer
					decision.Status = completionVerificationStatusPass
					decision.Reason = "latest tool evidence satisfies the requested operation"
				} else {
					text = completionVerificationFallbackAnswer(decision, text)
				}
				notifyCompletionVerificationResult(req, decision, text)
			} else {
				switch decision.normalizedStatus() {
				case completionVerificationStatusPass:
					completionVerificationRetryCount = 0
					evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
					if answer, ok := FastPathPreferredFinalAnswerForCompletionEvidence(evidence, text); ok {
						text = answer
					} else if answer := strings.TrimSpace(decision.FinalAnswer); answer != "" {
						text = answer
					} else if strings.TrimSpace(text) == "" {
						if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
							text = answer
						}
					}
					notifyCompletionVerificationResult(req, decision, text)
				case completionVerificationStatusNeedsAction:
					if answer, ok := completionEvidenceVerifiedFinalAnswer(req, successfulToolCalls, text); ok {
						text = answer
						decision.Status = completionVerificationStatusPass
						decision.Reason = "latest tool evidence satisfies the requested operation"
						decision.MissingSteps = nil
						decision.NextActionHint = ""
						completionVerificationRetryCount = 0
						notifyCompletionVerificationResult(req, decision, text)
						break
					}
					completionVerificationRetryCount++
					if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
						if answer, ok := completionEvidenceVerifiedFinalAnswer(req, successfulToolCalls, text); ok {
							text = answer
							decision.Status = completionVerificationStatusPass
							decision.Reason = "latest tool evidence satisfies the requested operation"
							decision.MissingSteps = nil
							decision.NextActionHint = ""
						} else {
							text = completionVerificationFallbackAnswer(decision, text)
						}
						notifyCompletionVerificationResult(req, decision, text)
					} else {
						modelDecidesTools := completionEvidenceOperationPlanModelDecides(evidence)
						messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount, modelDecidesTools))
						if forced := completionVerificationFeedbackToolChoice(decision, loadedSkills, resolved, modelDecidesTools); forced != nil {
							forcedToolChoiceForNextRound = forced
						}
						continue
					}
				case completionVerificationStatusFailed, completionVerificationStatusAskUser:
					if replacement := strings.TrimSpace(decision.FinalAnswer); replacement != "" {
						text = replacement
						completionVerificationRetryCount = 0
						notifyCompletionVerificationResult(req, decision, text)
					} else {
						completionVerificationRetryCount++
						if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
							if answer, ok := completionEvidenceVerifiedFinalAnswer(req, successfulToolCalls, text); ok {
								text = answer
								decision.Status = completionVerificationStatusPass
								decision.Reason = "latest tool evidence satisfies the requested operation"
								decision.MissingSteps = nil
								decision.NextActionHint = ""
							} else {
								text = completionVerificationFallbackAnswer(decision, text)
							}
							notifyCompletionVerificationResult(req, decision, text)
						} else {
							modelDecidesTools := completionEvidenceOperationPlanModelDecides(evidence)
							messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount, modelDecidesTools))
							if forced := completionVerificationFeedbackToolChoice(decision, loadedSkills, resolved, modelDecidesTools); forced != nil {
								forcedToolChoiceForNextRound = forced
							}
							continue
						}
					}
				default:
					completionVerificationRetryCount++
					if completionVerificationRetryCount > defaultMaxCompletionVerificationRetries {
						if answer, ok := completionEvidenceVerifiedFinalAnswer(req, successfulToolCalls, text); ok {
							text = answer
							decision.Status = completionVerificationStatusPass
							decision.Reason = "latest tool evidence satisfies the requested operation"
							decision.MissingSteps = nil
							decision.NextActionHint = ""
						} else {
							text = completionVerificationFallbackAnswer(decision, text)
						}
						notifyCompletionVerificationResult(req, decision, text)
					} else {
						modelDecidesTools := completionEvidenceOperationPlanModelDecides(evidence)
						messages = append(messages, completionVerificationSystemMessage(decision, text, completionVerificationRetryCount, modelDecidesTools))
						if forced := completionVerificationFeedbackToolChoice(decision, loadedSkills, resolved, modelDecidesTools); forced != nil {
							forcedToolChoiceForNextRound = forced
						}
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
		roundDeferredSystemMessages := []adapter.Message{}
		var roundCompletionFeedback adapter.Message
		roundCompletionFeedbackQueued := false
		for _, call := range toolCalls {
			stepCount++
			callSkillID, callToolName, callToolArgs, failedCallKey := skillToolCallIdentityForCall(resolved, loadedSkills, call)
			callEvidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
			if redirected, ok := redirectDuplicateAgentMutationToPendingPostUpdateReadCall(call, callSkillID, callToolName, callEvidence, currentMetadataForRun(req), successfulToolCalls); ok {
				call = redirected
				callSkillID, callToolName, callToolArgs, failedCallKey = skillToolCallIdentityForCall(resolved, loadedSkills, call)
			}
			if answer, ok := redundantPostReadAgentConfigMutationAnswer(callSkillID, callToolName, callToolArgs, callEvidence); ok {
				appendAnswerText(&answerBuilder, answer)
				r.emitAnswerChunk(ctx, prepared, answer, nil)
				logger.DebugContext(ctx, "aichat skill loop completed before redundant post-read agent config mutation",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_id", callSkillID,
					"tool_name", callToolName,
				)
				return answerBuilder.String(), usage, nil
			}
			if fastPathAgentReadOnlyConfigToolName(callSkillID, callToolName) {
				if answer, ok := agentReadOnlyConfigFastPathAnswerFromEvidence(callEvidence); ok {
					appendAnswerText(&answerBuilder, answer)
					r.emitAnswerChunk(ctx, prepared, answer, nil)
					logger.DebugContext(ctx, "aichat skill loop completed before redundant read-only agent config call",
						"conversation_id", prepared.Conversation.ID.String(),
						"message_id", prepared.Message.ID.String(),
						"skill_id", callSkillID,
						"tool_name", callToolName,
					)
					return answerBuilder.String(), usage, nil
				}
			}
			if answer, ok := agentReadOnlyConfigFastPathAnswerBeforeRedundantLookup(callSkillID, callToolName, callEvidence); ok {
				appendAnswerText(&answerBuilder, answer)
				r.emitAnswerChunk(ctx, prepared, answer, nil)
				logger.DebugContext(ctx, "aichat skill loop completed before redundant read-only agent candidate lookup",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_id", callSkillID,
					"tool_name", callToolName,
				)
				return answerBuilder.String(), usage, nil
			}
			result := skillStepResult{}
			if failedCallKey != "" {
				if reason := failedToolCallReasons[failedCallKey]; strings.TrimSpace(reason) != "" {
					result = repeatedFailedToolCallRecoverableStep(call.ID, callSkillID, callToolName, callToolArgs, reason)
				}
			}
			if result.trace.Kind == "" {
				result = repeatedSuccessfulReadOnlyToolCallFeedbackStep(call.ID, callSkillID, callToolName, callToolArgs, successfulToolCallsByKey, successfulToolCalls, callEvidence, currentMetadataForRun(req))
			}
			if result.trace.Kind == "" {
				result = missingAgentTargetListAgentsTerminalStep(call.ID, callSkillID, callToolName, callToolArgs, successfulToolCalls, prepared.Query)
			}
			if result.trace.Kind == "" {
				result = r.handleProgressiveSkillCall(ctx, prepared, resolved, call, req.ExecutionContext, toolCallCount, skillToolCallCounts, loadedSkills, userInputGuardState{
					guard:               userInputGuard,
					toolCallGuard:       toolCallGuard,
					planToolGuard:       req.PlanToolGuard,
					argumentResolver:    req.ToolArgumentResolver,
					round:               round,
					skillUsed:           skillUsed,
					toolCallCount:       toolCallCount,
					attemptedToolCalls:  append([]SkillToolCallRef{}, attemptedToolCalls...),
					successfulToolCalls: append([]SkillToolCallRef{}, successfulToolCalls...),
				}, nil)
			}
			if strings.TrimSpace(result.trace.Kind) == "" {
				if result.usedSkill {
					skillUsed = true
				}
				if result.usedTool {
					toolCallCount++
					incrementSkillToolCallCount(skillToolCallCounts, result.trace.SkillID)
				}
				if result.toolMessage.Role != "" || result.toolMessage.ToolCallID != "" || result.toolMessage.Content != nil {
					messages = append(messages, result.toolMessage)
				}
				continue
			}
			traces = append(traces, result.trace)
			r.recordTrace(traces, result.trace)
			r.logSkillTrace(ctx, prepared, result.trace)
			if postVerificationConfigured && plannerFeedbackRequestsPendingMutation(result.trace) {
				evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
				if !roundCompletionFeedbackQueued {
					if feedback, shouldContinue := completionEvidenceContinuationSystemMessage(evidence, evidenceContinuationRetryCount, resolved); shouldContinue {
						evidenceContinuationRetryCount++
						roundCompletionFeedback = feedback
						roundCompletionFeedbackQueued = true
					}
				}
				if forced := completionEvidenceContinuationToolChoice(evidence, loadedSkills, resolved); forced != nil {
					forcedToolChoiceForNextRound = forced
				}
			}
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
					if failedCallKey != "" {
						successfulToolCallsByKey[failedCallKey] = SkillToolCallRef{
							SkillID:   strings.TrimSpace(callSkillID),
							ToolName:  strings.TrimSpace(callToolName),
							Arguments: copyStringAnyMap(callToolArgs),
							Result:    copyStringAnyMap(result.toolResult),
						}
					}
					finalAnswerGuardBlockCount = 0
					evidenceContinuationRetryCount = 0
				}
			}
			if postVerificationConfigured &&
				strings.EqualFold(strings.TrimSpace(result.trace.Kind), "skill_load") &&
				strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
				if forced := completionEvidenceContinuationToolChoice(completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls), loadedSkills, resolved); forced != nil {
					forcedToolChoiceForNextRound = forced
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
			fastPathEvidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
			if answer, ok := FastPathFinalAnswerForToolTraceWithEvidence(fastPathTraceWithToolResult(result.trace, result.toolResult), fastPathEvidence); ok {
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
			if answer, ok := FastPathFinalAnswerForCompletionEvidence(fastPathEvidence); ok {
				appendAnswerText(&answerBuilder, answer)
				r.emitAnswerChunk(ctx, prepared, answer, nil)
				logger.DebugContext(ctx, "aichat skill loop completed through accumulated evidence fast path",
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
			if result.pendingUserInput != nil {
				logger.DebugContext(ctx, "aichat skill planning requested user input",
					"conversation_id", prepared.Conversation.ID.String(),
					"message_id", prepared.Message.ID.String(),
					"skill_step_count", stepCount,
					"tool_call_count", toolCallCount,
				)
				return answerBuilder.String(), usage, &UserInputPendingError{Payload: result.pendingUserInput}
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
				roundDeferredSystemMessages = append(roundDeferredSystemMessages, message)
			}
			if postVerificationConfigured &&
				strings.EqualFold(strings.TrimSpace(result.trace.Kind), "tool_call") &&
				strings.EqualFold(strings.TrimSpace(result.trace.Status), "success") {
				evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successfulToolCalls)
				if !roundCompletionFeedbackQueued {
					if feedback, shouldContinue := completionEvidenceContinuationSystemMessage(evidence, evidenceContinuationRetryCount, resolved); shouldContinue {
						evidenceContinuationRetryCount++
						roundCompletionFeedback = feedback
						roundCompletionFeedbackQueued = true
						if forced := completionEvidenceContinuationToolChoice(evidence, loadedSkills, resolved); forced != nil {
							forcedToolChoiceForNextRound = forced
						}
					}
				} else if forced := completionEvidenceContinuationToolChoice(evidence, loadedSkills, resolved); forced != nil {
					forcedToolChoiceForNextRound = forced
				}
			}
		}
		if len(roundDeferredSystemMessages) > 0 {
			messages = append(messages, roundDeferredSystemMessages...)
		}
		if roundCompletionFeedbackQueued {
			messages = append(messages, roundCompletionFeedback)
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

func completionEvidenceContinuationSystemMessage(evidence map[string]interface{}, retryCount int, resolved *skills.ResolvedSkills) (adapter.Message, bool) {
	if feedback, ok := completionEvidenceContinuationAgentCreateSystemMessage(evidence, retryCount); ok {
		return feedback, true
	}
	return completionEvidenceContinuationPendingPlanStepSystemMessage(evidence, retryCount, resolved)
}

func completionEvidenceContinuationAgentCreateSystemMessage(evidence map[string]interface{}, retryCount int) (adapter.Message, bool) {
	progress := evidenceMapFromAny(evidence["agent_create_progress"])
	if len(progress) == 0 {
		return adapter.Message{}, false
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(progress["status"])))
	missingTargets := evidenceStringSliceFromAny(progress["missing_targets"])
	missingCount := firstNonNegativeInt(progress["missing_count"])
	if len(missingTargets) == 0 && missingCount <= 0 {
		return adapter.Message{}, false
	}
	if status != "" && status != "partial" && status != "missing" && status != "incomplete" && status != "needs_action" {
		return adapter.Message{}, false
	}
	payload := map[string]interface{}{
		"operation":         "agent.create",
		"missing_targets":   missingTargets,
		"missing_count":     firstPositiveInt(missingCount, len(missingTargets)),
		"completed_targets": evidenceStringSliceFromAny(progress["completed_targets"]),
	}
	if description := strings.TrimSpace(evidenceStringFromAny(progress["requested_description"])); description != "" {
		payload["requested_description"] = description
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte(fmt.Sprint(payload))
	}
	content := strings.Join([]string{
		fmt.Sprintf("Runtime execution evidence requires continued tool use before a final answer. Evidence-continuation retry %d of %d.", retryCount+1, defaultMaxCompletionVerificationRetries),
		"The current user goal is still missing one or more exact Agent creation targets.",
		"Call agent-management/create_agent once for each exact missing target name. Use requested_description if present and the user did not provide per-target descriptions.",
		"Do not answer as complete until successful create_agent tool evidence and the required frontend observation evidence exist for every requested target.",
		"Do not treat a similar existing visible Agent with a different exact name as satisfying a missing target.",
		"Agent creation progress JSON:\n" + string(payloadJSON),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func completionEvidenceContinuationPendingPlanStepSystemMessage(evidence map[string]interface{}, retryCount int, resolved *skills.ResolvedSkills) (adapter.Message, bool) {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	modelDecidesTools := completionEvidenceOperationPlanModelDecides(evidence)
	if modelDecidesTools {
		if _, ok := fastPathModelDecidesPendingAgentWorkStep(plan); !ok {
			if completionVerificationModelDecidesPlanHasOpenPhase(plan) {
				return completionEvidenceContinuationOpenModelDecidesPhaseSystemMessage(evidence, retryCount), true
			}
			return adapter.Message{}, false
		}
	}
	step, ok := completionVerificationPendingExecutablePlanStep(evidence)
	if !ok {
		return adapter.Message{}, false
	}
	action := completionVerificationPlanStepLabel(step)
	if strings.TrimSpace(action) == "" {
		return adapter.Message{}, false
	}
	if completionEvidenceContinuationShouldSkipPendingPlanStep(step, action) {
		return adapter.Message{}, false
	}
	if !completionEvidencePlanStepSkillResolved(step, resolved) {
		return adapter.Message{}, false
	}
	if modelDecidesTools {
		return completionEvidenceContinuationModelDecidesSystemMessage(evidence, step, retryCount), true
	}
	payloadJSON, err := json.Marshal(map[string]interface{}{
		"suggested_next_tool": action,
		"pending_tool_step":   step,
	})
	if err != nil {
		payloadJSON = []byte(fmt.Sprint(step))
	}
	content := strings.Join([]string{
		fmt.Sprintf("Runtime execution evidence requires continued tool use before a final answer. Evidence-continuation retry %d of %d.", retryCount+1, defaultMaxCompletionVerificationRetries),
		"The operation plan is an advisory strategy snapshot, not proof of completion. It currently has a pending phase with no successful evidence.",
		"If the current page context and available tools still support this action, load the suggested skill if needed and call the suggested tool. Resolve arguments from the latest user request, page context, and visible asset evidence.",
		"If the pending step is agent-management/update_agent_config, use pending_tool_step.config_goal as the user-facing configuration target and infer concrete update_agent_config arguments from the tool schema, latest user request, page context, and prior read results; extract the target field values from the latest user request and read result. If pending_tool_step also lists expected_updated_fields, treat them as verification hints rather than the only allowed fields. Do not repeat an identical get_agent_config call when it already succeeded earlier in this turn.",
		"If only load_skill is available for that skill, first call load_skill with the exact skill_id from suggested_next_tool, then call call_skill_tool with the exact skill_id/tool_name after the skill loads.",
		"Do not tell the user an approval card has been submitted unless a governed tool call actually returned a pending governance event. Governance approval cards are created by tool calls, not by natural-language progress text.",
		"If the action is impossible because context, permissions, or tool capability is missing, call the appropriate read/list tool when that can resolve the missing context; otherwise stop and explain the exact blocker truthfully.",
		"Do not answer as complete until successful tool evidence and any required page observation evidence exist for this pending phase.",
		"Pending plan step JSON:\n" + string(payloadJSON),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func completionEvidenceContinuationOpenModelDecidesPhaseSystemMessage(evidence map[string]interface{}, retryCount int) adapter.Message {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	payload := completionEvidenceContinuationOpenModelDecidesPhasePayload(plan)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte(fmt.Sprint(payload))
	}
	content := strings.Join([]string{
		fmt.Sprintf("Runtime execution evidence requires continued work before a final answer. Evidence-continuation retry %d of %d.", retryCount+1, defaultMaxCompletionVerificationRetries),
		"The operation strategy is phase-only: the current phase still lacks completion verification, but the model must choose the concrete next action from the currently available tool schemas and latest context.",
		"Use the original user goal, phase success criteria, current page context, prior tool results, turn_state, and visible asset evidence to decide the next action.",
		"Do not repeat an action when matching evidence already proves it succeeded.",
		"If the remaining work is impossible because context, permissions, or tool capability is missing, use available read/list/observe capabilities when they can resolve the blocker; otherwise stop and explain the exact blocker truthfully.",
		"Do not answer as complete until successful evidence supports the phase success criteria or a real blocker is proven.",
		"Open phase evidence JSON:\n" + string(payloadJSON),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}
}

func completionEvidenceContinuationOpenModelDecidesPhasePayload(plan map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{}
	for _, key := range []string{
		"original_user_goal",
		"intent",
		"planning_mode",
		"tool_choice_mode",
		"status",
		"pending_next_action",
		"phases",
		"success_criteria",
		"capability_goals",
	} {
		if value, ok := plan[key]; ok && completionEvidenceContinuationPayloadValuePresent(value) {
			payload[key] = promptEvidenceCopy(value)
		}
	}
	if goals := completionVerificationCapabilityGoalsForPrompt(plan["capability_goals"]); len(goals) > 0 {
		payload["capability_goals"] = goals
	}
	if len(payload) == 0 {
		payload["phase"] = "pending_user_visible_operation"
	}
	return payload
}

func completionEvidenceContinuationModelDecidesSystemMessage(evidence map[string]interface{}, step map[string]interface{}, retryCount int) adapter.Message {
	payload := map[string]interface{}{
		"pending_phase": completionEvidenceContinuationModelDecidesPhasePayload(step),
	}
	if goals := completionVerificationCapabilityGoalsForPrompt(evidenceMapFromAny(evidence["operation_plan"])["capability_goals"]); len(goals) > 0 {
		payload["capability_goals"] = goals
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte(fmt.Sprint(payload))
	}
	content := strings.Join([]string{
		fmt.Sprintf("Runtime execution evidence requires continued work before a final answer. Evidence-continuation retry %d of %d.", retryCount+1, defaultMaxCompletionVerificationRetries),
		"The operation strategy is phase-only: it describes the user-visible phase that is not yet proven complete, but the model must choose the concrete tool from the currently available tool schemas and latest context.",
		"Use current page context, prior tool results, turn_state, and visible asset evidence to decide the next action. Do not repeat an action when matching evidence already proves it succeeded.",
		"For Agent configuration work, apply the remaining user-requested changes with the available Agent management capability, then verify the refreshed configuration before the final answer.",
		"If the original user request already asks for those remaining Agent changes, do not ask the user whether to continue after a read-only check shows missing configuration; choose the next appropriate available tool and let governed tool calls request approval when needed.",
		"Do not tell the user an approval card has been submitted unless a governed tool call actually returned a pending governance event. Governance approval cards are created by tool calls, not by natural-language progress text.",
		"If the action is impossible because context, permissions, or tool capability is missing, use available read/list/observe capabilities when they can resolve the blocker; otherwise stop and explain the exact blocker truthfully.",
		"Do not answer as complete until successful tool evidence and any required page observation evidence exist for this pending phase.",
		"Pending phase evidence JSON:\n" + string(payloadJSON),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}
}

func completionEvidenceContinuationModelDecidesPhasePayload(step map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{}
	for _, key := range []string{
		"title",
		"phase",
		"status",
		"config_goal",
		"expected_updated_fields",
		"expected_binding_actions",
		"required_post_update_verification",
		"missing_updated_fields",
		"target",
		"targets",
		"resource",
		"resources",
		"goal",
	} {
		if value, ok := step[key]; ok && completionEvidenceContinuationPayloadValuePresent(value) {
			payload[key] = promptEvidenceCopy(value)
		}
	}
	if _, ok := payload["phase"]; !ok {
		payload["phase"] = "pending_user_visible_operation"
	}
	return payload
}

func completionEvidenceContinuationPayloadValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	if strings.TrimSpace(evidenceStringFromAny(value)) != "" {
		return true
	}
	if len(evidenceMapFromAny(value)) > 0 {
		return true
	}
	if len(evidenceSliceFromAny(value)) > 0 {
		return true
	}
	switch value.(type) {
	case bool, int, int64, float64, float32, json.Number:
		return true
	default:
		return false
	}
}

func completionEvidenceContinuationToolChoice(evidence map[string]interface{}, loadedSkills map[string]struct{}, resolved *skills.ResolvedSkills) interface{} {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if completionEvidenceOperationPlanModelDecides(evidence) {
		if _, ok := fastPathModelDecidesPendingAgentWorkStep(plan); !ok {
			return nil
		}
		return nil
	}
	step, ok := completionVerificationPendingExecutablePlanStep(evidence)
	if !ok {
		return nil
	}
	action := completionVerificationPlanStepLabel(step)
	if strings.TrimSpace(action) == "" {
		return nil
	}
	if completionEvidenceContinuationShouldSkipPendingPlanStep(step, action) {
		return nil
	}
	if !completionEvidencePlanStepSkillResolved(step, resolved) {
		return nil
	}
	skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])))
	toolName := strings.TrimSpace(evidenceStringFromAny(step["tool_name"]))
	if skillID == "" || toolName == "" {
		return nil
	}
	loaded := normalizedLoadedSkillSet(loadedSkills)
	if _, ok := loaded[skillID]; !ok {
		return forcedFunctionToolChoice(skills.MetaToolLoadSkill)
	}
	return forcedFunctionToolChoice(skills.MetaToolCallSkillTool)
}

func completionVerificationFeedbackToolChoice(decision completionVerificationDecision, loadedSkills map[string]struct{}, resolved *skills.ResolvedSkills, modelDecidesTools bool) interface{} {
	if modelDecidesTools {
		return nil
	}
	skillID, toolName, ok := completionVerificationRequiredSkillTool(decision)
	if !ok || strings.TrimSpace(skillID) == "" || strings.TrimSpace(toolName) == "" {
		return nil
	}
	if !resolvedSkillContains(resolved, skillID) {
		return nil
	}
	loaded := normalizedLoadedSkillSet(loadedSkills)
	if _, ok := loaded[strings.ToLower(strings.TrimSpace(skillID))]; !ok {
		return forcedFunctionToolChoice(skills.MetaToolLoadSkill)
	}
	return forcedFunctionToolChoice(skills.MetaToolCallSkillTool)
}

func resolvedSkillContains(resolved *skills.ResolvedSkills, skillID string) bool {
	if resolved == nil {
		return false
	}
	for _, resolvedSkillID := range resolved.SkillIDs() {
		if strings.EqualFold(strings.TrimSpace(resolvedSkillID), strings.TrimSpace(skillID)) {
			return true
		}
	}
	return false
}

func initialLoadedSkillsForRun(req RunRequest, resolved *skills.ResolvedSkills) map[string]struct{} {
	loaded := map[string]struct{}{}
	add := func(skillID string) {
		if canonical, ok := canonicalResolvedSkillID(resolved, skillID); ok {
			loaded[canonical] = struct{}{}
		}
	}
	metadata := currentMetadataForRun(req)
	for _, skillID := range evidenceStringSliceFromAny(metadata["loaded_skill_ids"]) {
		add(skillID)
	}
	for _, skillID := range evidenceStringSliceFromAny(metadata["loaded_skills"]) {
		add(skillID)
	}
	appendLoadedFromInvocations := func(invocations []map[string]interface{}) {
		for _, invocation := range invocations {
			kind := strings.TrimSpace(evidenceStringFromAny(invocation["kind"]))
			if !strings.EqualFold(kind, "skill_load") && !strings.EqualFold(kind, "tool_call") {
				continue
			}
			if !completionVerificationInvocationSucceeded(invocation) {
				continue
			}
			add(evidenceStringFromAny(invocation["skill_id"]))
		}
	}
	appendLoadedFromInvocations(evidenceMapsFromAny(metadata["skill_invocations"]))
	ledger := evidenceMapFromAny(metadata["execution_ledger"])
	appendLoadedFromInvocations(evidenceMapsFromAny(ledger["skill_invocations"]))
	appendLoadedFromInvocations(evidenceMapsFromAny(evidenceMapFromAny(ledger["summary"])["skill_invocations"]))
	return loaded
}

func canonicalResolvedSkillID(resolved *skills.ResolvedSkills, skillID string) (string, bool) {
	skillID = strings.TrimSpace(skillID)
	if skillID == "" || resolved == nil {
		return "", false
	}
	for _, resolvedSkillID := range resolved.SkillIDs() {
		if strings.EqualFold(strings.TrimSpace(resolvedSkillID), skillID) {
			return strings.TrimSpace(resolvedSkillID), true
		}
	}
	return "", false
}

func completionEvidenceVerifiedFinalAnswer(req RunRequest, successful []SkillToolCallRef, candidate string) (string, bool) {
	evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successful)
	if answer, ok := FastPathPreferredFinalAnswerForCompletionEvidence(evidence, candidate); ok {
		return answer, true
	}
	if answer, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		return answer, true
	}
	return "", false
}

func completionEvidenceForFastPathWithSuccessfulToolCalls(req RunRequest, successful []SkillToolCallRef) map[string]interface{} {
	evidence := completionEvidenceForFastPath(req)
	if len(successful) == 0 {
		return evidence
	}
	if evidence == nil {
		evidence = map[string]interface{}{}
	} else {
		evidence = copyStringAnyMap(evidence)
	}
	invocations := evidenceSliceFromAny(evidence["skill_invocations"])
	existingInvocations := map[string]struct{}{}
	for _, raw := range invocations {
		invocation := evidenceMapFromAny(raw)
		if len(invocation) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") {
			continue
		}
		signature := skillToolCallEvidenceSignature(
			stringFromAny(invocation["skill_id"]),
			stringFromAny(invocation["tool_name"]),
			evidenceMapFromAny(invocation["arguments"]),
			evidenceMapFromAny(invocation["result"]),
		)
		if signature != "" {
			existingInvocations[signature] = struct{}{}
		}
	}
	for _, call := range successful {
		skillID := strings.TrimSpace(call.SkillID)
		toolName := strings.TrimSpace(call.ToolName)
		if skillID == "" || toolName == "" {
			continue
		}
		signature := skillToolCallEvidenceSignature(skillID, toolName, call.Arguments, call.Result)
		if _, ok := existingInvocations[signature]; ok && signature != "" {
			continue
		}
		invocation := map[string]interface{}{
			"kind":      "tool_call",
			"status":    "success",
			"skill_id":  skillID,
			"tool_name": toolName,
		}
		if len(call.Arguments) > 0 {
			invocation["arguments"] = copyStringAnyMap(call.Arguments)
		}
		if len(call.Result) > 0 {
			invocation["result"] = copyStringAnyMap(call.Result)
		}
		invocations = append(invocations, invocation)
		if signature != "" {
			existingInvocations[signature] = struct{}{}
		}
	}
	if len(invocations) > 0 {
		evidence["skill_invocations"] = invocations
	}
	return evidence
}

func skillToolCallEvidenceSignature(skillID string, toolName string, arguments map[string]interface{}, result map[string]interface{}) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return ""
	}
	payload := map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
	}
	if len(arguments) > 0 {
		payload["arguments"] = arguments
	}
	if len(result) > 0 {
		payload["result"] = result
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return skillID + "/" + toolName
	}
	return string(data)
}

func completionEvidenceContinuationShouldSkipPendingPlanStep(step map[string]interface{}, action string) bool {
	if completionEvidencePlanStepIsRequiredPostUpdateAgentConfigRead(step) {
		return false
	}
	skillID := strings.TrimSpace(evidenceStringFromAny(step["skill_id"]))
	toolName := strings.TrimSpace(evidenceStringFromAny(step["tool_name"]))
	if completionEvidenceContinuationIsConsoleNavigateTool(skillID, toolName) {
		return true
	}
	return fastPathPendingActionIsRoutePostVerification(action)
}

func completionEvidenceContinuationIsConsoleNavigateTool(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillConsoleNavigator) &&
		strings.EqualFold(strings.TrimSpace(toolName), "navigate")
}

func completionEvidencePlanStepSkillResolved(step map[string]interface{}, resolved *skills.ResolvedSkills) bool {
	skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])))
	if skillID == "" {
		return false
	}
	for _, resolvedSkillID := range resolved.SkillIDs() {
		if strings.EqualFold(strings.TrimSpace(resolvedSkillID), skillID) {
			return true
		}
	}
	return false
}

func normalizedLoadedSkillSet(loadedSkills map[string]struct{}) map[string]struct{} {
	if len(loadedSkills) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(loadedSkills))
	for skillID := range loadedSkills {
		if normalized := strings.ToLower(strings.TrimSpace(skillID)); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func forcedFunctionToolChoice(name string) interface{} {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name": name,
		},
	}
}

func notifyCompletionVerificationResult(req RunRequest, decision completionVerificationDecision, finalAnswer string) {
	if req.OnCompletionVerification == nil {
		return
	}
	req.OnCompletionVerification(CompletionVerificationResult{
		Status:            decision.normalizedStatus(),
		Reason:            strings.TrimSpace(decision.Reason),
		MissingSteps:      append([]string(nil), decision.MissingSteps...),
		UnsupportedClaims: append([]string(nil), decision.UnsupportedClaims...),
		NextActionHint:    strings.TrimSpace(decision.NextActionHint),
		FinalAnswer:       strings.TrimSpace(finalAnswer),
	})
}

func evidenceStringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return dedupeStrings(typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(evidenceStringFromAny(item)); text != "" {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	case []map[string]interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(firstNonEmptyString(item["name"], item["agent_name"], item["title"])); text != "" {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	default:
		if text := strings.TrimSpace(evidenceStringFromAny(value)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func repeatedFailedToolCallKeyForCall(call adapter.ToolCall) (string, string, map[string]interface{}, string) {
	return skillToolCallIdentityForCall(nil, nil, call)
}

func skillToolCallIdentityForCall(resolved *skills.ResolvedSkills, loadedSkills map[string]struct{}, call adapter.ToolCall) (string, string, map[string]interface{}, string) {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		return "", "", nil, ""
	}
	if !strings.EqualFold(strings.TrimSpace(call.Function.Name), skills.MetaToolCallSkillTool) {
		toolName := strings.TrimSpace(call.Function.Name)
		if toolName == "" || isSkillMetaToolName(toolName) {
			return "", "", nil, ""
		}
		skillID, ok := uniqueLoadedSkillForToolName(resolved, loadedSkills, toolName)
		if !ok {
			return "", "", nil, ""
		}
		toolArgs := copyStringAnyMap(args)
		return skillID, toolName, toolArgs, failedToolCallKey(skillID, toolName, toolArgs)
	}
	skillID := normalizedSkillArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	return skillID, toolName, toolArgs, failedToolCallKey(skillID, toolName, toolArgs)
}

func redirectDuplicateAgentMutationToPendingPostUpdateReadCall(call adapter.ToolCall, skillID string, toolName string, evidence map[string]interface{}, metadata map[string]interface{}, successfulToolCalls []SkillToolCallRef) (adapter.ToolCall, bool) {
	if !isAgentManagementMutationTool(skillID, toolName) ||
		(!successfulAgentManagementMutationExists(successfulToolCalls) &&
			!successfulAgentManagementMutationExistsInEvidence(evidence) &&
			!successfulAgentManagementMutationExistsInEvidence(metadata)) {
		return adapter.ToolCall{}, false
	}
	step, ok := completionVerificationPendingExecutablePlanStep(evidence)
	if ok && !completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) {
		return adapter.ToolCall{}, false
	}
	if !ok {
		step, ok = pendingPostUpdateAgentReadStepFromMetadata(metadata)
		if !ok {
			return adapter.ToolCall{}, false
		}
	}
	pendingSkillID := strings.TrimSpace(evidenceStringFromAny(step["skill_id"]))
	pendingToolName := strings.TrimSpace(evidenceStringFromAny(step["tool_name"]))
	if !strings.EqualFold(pendingSkillID, skills.SkillAgentManagement) {
		return adapter.ToolCall{}, false
	}
	switch strings.ToLower(pendingToolName) {
	case "get_agent", "get_agent_config":
	default:
		return adapter.ToolCall{}, false
	}
	agentID := successfulAgentMutationAgentID(successfulToolCalls)
	if agentID == "" {
		agentID = successfulAgentMutationAgentIDFromEvidence(evidence)
	}
	if agentID == "" {
		agentID = successfulAgentMutationAgentIDFromEvidence(metadata)
	}
	if agentID == "" {
		return adapter.ToolCall{}, false
	}
	payload, err := json.Marshal(map[string]interface{}{
		"skill_id":  skills.SkillAgentManagement,
		"tool_name": pendingToolName,
		"arguments": map[string]interface{}{
			"agent_id": agentID,
		},
	})
	if err != nil {
		return adapter.ToolCall{}, false
	}
	call.Function.Name = skills.MetaToolCallSkillTool
	call.Function.Arguments = string(payload)
	return call, true
}

func currentMetadataForRun(req RunRequest) map[string]interface{} {
	if req.CurrentMetadata == nil {
		return nil
	}
	return copyStringAnyMap(req.CurrentMetadata())
}

func pendingPostUpdateAgentReadStepFromMetadata(metadata map[string]interface{}) (map[string]interface{}, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	plan := evidenceMapFromAny(metadata["operation_plan"])
	if len(plan) == 0 {
		return nil, false
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 || !completionEvidencePlanStepIsRequiredPostUpdateAgentRead(step) {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
		if status == "" {
			stepID := strings.TrimSpace(evidenceStringFromAny(step["id"]))
			status = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[stepID])))
		}
		switch status {
		case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
			continue
		default:
			return step, true
		}
	}
	return nil, false
}

func isAgentManagementMutationTool(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "update_agent_identity", "update_agent_config",
		"replace_agent_skill_bindings", "replace_agent_knowledge_bindings",
		"replace_agent_database_bindings", "replace_agent_workflow_bindings",
		"replace_agent_memory_slots":
		return true
	default:
		return false
	}
}

func successfulAgentManagementMutationExists(calls []SkillToolCallRef) bool {
	for _, call := range calls {
		if isAgentManagementMutationTool(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func successfulAgentManagementMutationExistsInEvidence(evidence map[string]interface{}) bool {
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		if isAgentManagementMutationTool(evidenceStringFromAny(invocation["skill_id"]), evidenceStringFromAny(invocation["tool_name"])) {
			return true
		}
	}
	return false
}

func successfulAgentMutationAgentID(calls []SkillToolCallRef) string {
	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if !isAgentManagementMutationTool(call.SkillID, call.ToolName) {
			continue
		}
		for _, value := range []interface{}{
			call.Result["agent_id"],
			call.Result["id"],
			call.Arguments["agent_id"],
			call.Arguments["id"],
		} {
			if text := strings.TrimSpace(evidenceStringFromAny(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

func successfulAgentMutationAgentIDFromEvidence(evidence map[string]interface{}) string {
	invocations := completionVerificationEvidenceInvocations(evidence)
	for i := len(invocations) - 1; i >= 0; i-- {
		invocation := invocations[i]
		if !completionVerificationInvocationSucceeded(invocation) ||
			!isAgentManagementMutationTool(evidenceStringFromAny(invocation["skill_id"]), evidenceStringFromAny(invocation["tool_name"])) {
			continue
		}
		args := evidenceMapFromAny(invocation["arguments"])
		result := evidenceMapFromAny(invocation["result"])
		if len(result) == 0 {
			result = evidenceMapFromAny(invocation["result_summary"])
		}
		for _, value := range []interface{}{
			result["agent_id"],
			result["id"],
			args["agent_id"],
			args["id"],
		} {
			if text := strings.TrimSpace(evidenceStringFromAny(value)); text != "" {
				return text
			}
		}
	}
	return ""
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

func repeatedSuccessfulReadOnlyToolCallFeedbackStep(callID string, skillID string, toolName string, args map[string]interface{}, successfulToolCallsByKey map[string]SkillToolCallRef, successfulToolCalls []SkillToolCallRef, evidence map[string]interface{}, metadata map[string]interface{}) skillStepResult {
	if !skillToolCallLooksReadOnly(skillID, toolName) {
		return skillStepResult{}
	}
	key := failedToolCallKey(skillID, toolName, args)
	if key == "" {
		return skillStepResult{}
	}
	previous, ok := successfulToolCallsByKey[key]
	if !ok {
		if candidatePrevious, candidateOK := previousSuccessfulAgentCandidateLookup(skillID, toolName, successfulToolCalls); candidateOK {
			if step, pendingOK := repeatedReadOnlyPendingAgentMutationStep(skillID, toolName, evidence, metadata); pendingOK {
				trace := plannerFeedbackTrace(skillID, toolName, nil)
				trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
				trace.Arguments["next_step"] = "call_pending_agent_mutation"
				trace.Arguments["reason"] = "same_candidate_lookup_already_found_usable_result_while_mutation_step_pending"
				trace.Arguments["required_next_tool"] = completionVerificationPlanStepLabel(step)
				return successfulSkillStep(trace, skills.ToolResultMessage(callID, repeatedSuccessfulReadOnlyPendingMutationPayload(skillID, toolName, candidatePrevious, step)), false, false)
			}
		}
		return skillStepResult{}
	}
	if repeatedReadOnlyToolShouldRunAfterMutation(skillID, toolName, args, successfulToolCalls) {
		return skillStepResult{}
	}
	if step, ok := repeatedReadOnlyPendingAgentMutationStep(skillID, toolName, evidence, metadata); ok {
		trace := plannerFeedbackTrace(skillID, toolName, nil)
		trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
		trace.Arguments["next_step"] = "call_pending_agent_mutation"
		trace.Arguments["reason"] = "same_read_only_tool_already_succeeded_while_mutation_step_pending"
		trace.Arguments["required_next_tool"] = completionVerificationPlanStepLabel(step)
		return successfulSkillStep(trace, skills.ToolResultMessage(callID, repeatedSuccessfulReadOnlyPendingMutationPayload(skillID, toolName, previous, step)), false, false)
	}
	trace := plannerFeedbackTrace(skillID, toolName, nil)
	trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	trace.Arguments["next_step"] = "answer_from_previous_result"
	trace.Arguments["reason"] = "same_read_only_tool_already_succeeded"
	return successfulSkillStep(trace, skills.ToolResultMessage(callID, repeatedSuccessfulReadOnlyToolCallPayload(skillID, toolName, previous)), false, false)
}

func missingAgentTargetListAgentsTerminalStep(callID string, skillID string, toolName string, args map[string]interface{}, successfulToolCalls []SkillToolCallRef, userQuery string) skillStepResult {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) ||
		!strings.EqualFold(strings.TrimSpace(toolName), "list_agents") {
		return skillStepResult{}
	}
	previousLookups := 0
	emptyLookups := 0
	broadLookups := 0
	for _, call := range successfulToolCalls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "list_agents") {
			continue
		}
		previousLookups++
		if !agentListAgentsArgsHasKeyword(call.Arguments) {
			broadLookups++
		}
		if agentListAgentsResultCount(call.Result) == 0 {
			emptyLookups++
		}
	}
	if previousLookups < 2 {
		return skillStepResult{}
	}
	if emptyLookups < 2 && !(emptyLookups >= 1 && broadLookups >= 1) {
		return skillStepResult{}
	}
	targetName := agentTargetNameFromQuery(userQuery)
	if targetName == "" {
		targetName = strings.TrimSpace(stringArg(args, "keyword"))
	}
	trace := plannerFeedbackTrace(skillID, toolName, nil)
	trace.Arguments = summarizeSkillToolArguments(skillID, toolName, args)
	trace.Arguments["next_step"] = "answer_missing_agent_target"
	trace.Arguments["reason"] = "agent_target_resolution_exhausted"
	trace.Arguments["previous_list_agents_calls"] = previousLookups
	trace.Arguments["empty_result_calls"] = emptyLookups
	if targetName != "" {
		trace.Arguments["target_name"] = targetName
	}
	payload := map[string]interface{}{
		"status":                     "completed",
		"advisory":                   "agent_target_resolution_exhausted",
		"skill_id":                   strings.TrimSpace(skillID),
		"tool_name":                  strings.TrimSpace(toolName),
		"target_name":                targetName,
		"previous_list_agents_calls": previousLookups,
		"empty_result_calls":         emptyLookups,
		"next_action":                "Stop calling list_agents in this turn. Answer from the existing target-resolution evidence; do not request governance approval and do not run mutation tools for an unresolved Agent target.",
	}
	result := terminalSkillStep(trace, skills.ToolResultMessage(callID, payload), true, false)
	result.answer = missingAgentTargetFinalAnswer(userQuery, targetName)
	return result
}

func agentListAgentsArgsHasKeyword(args map[string]interface{}) bool {
	if len(args) == 0 {
		return false
	}
	value, ok := args["keyword"]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case map[string]interface{}:
		return true
	default:
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
}

func agentListAgentsResultCount(result map[string]interface{}) int {
	for _, key := range []string{"agents_count", "count", "total", "target_count"} {
		if count, ok := intFromAny(result[key]); ok {
			return count
		}
	}
	if count := repeatedSuccessfulReadOnlyResultCollectionLength(result["agents"]); count >= 0 {
		return count
	}
	return -1
}

func agentTargetNameFromQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`名为\s*[「"“']?([^」"”'，。；、,\s]+)`),
		regexp.MustCompile(`名称为\s*[「"“']?([^」"”'，。；、,\s]+)`),
		regexp.MustCompile(`named\s+[「"“']?([^」"”'，。；、,\s]+)`),
		regexp.MustCompile(`called\s+[「"“']?([^」"”'，。；、,\s]+)`),
	} {
		if match := pattern.FindStringSubmatch(query); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	tokenPattern := regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9_.:-]{2,}\b`)
	matches := tokenPattern.FindAllString(query, -1)
	for _, match := range matches {
		upper := strings.ToUpper(match)
		if strings.Contains(upper, "AGENT") || strings.Contains(upper, "AICHAT") {
			return strings.TrimSpace(match)
		}
	}
	return ""
}

func missingAgentTargetFinalAnswer(userQuery string, targetName string) string {
	if containsCJK(userQuery) {
		if targetName != "" {
			return fmt.Sprintf("无法完成这次智能体操作：在当前工作空间中没有解析到名为「%s」的智能体。已根据已有搜索结果确认没有匹配目标，所以我没有发起审批，也没有执行修改或删除。", targetName)
		}
		return "无法完成这次智能体操作：已有搜索结果没有解析到明确的目标智能体，所以我没有发起审批，也没有执行修改或删除。"
	}
	if targetName != "" {
		return fmt.Sprintf("I couldn't complete this Agent operation because no Agent named %q was resolved in the current workspace. I did not request approval and did not modify or delete anything.", targetName)
	}
	return "I couldn't complete this Agent operation because the target Agent could not be resolved. I did not request approval and did not modify or delete anything."
}

func previousSuccessfulAgentCandidateLookup(skillID string, toolName string, successfulToolCalls []SkillToolCallRef) (SkillToolCallRef, bool) {
	if !agentCandidateLookupTool(skillID, toolName) {
		return SkillToolCallRef{}, false
	}
	for i := len(successfulToolCalls) - 1; i >= 0; i-- {
		call := successfulToolCalls[i]
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), strings.TrimSpace(skillID)) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), strings.TrimSpace(toolName)) {
			continue
		}
		if agentCandidateLookupResultUsable(call.Result) {
			return call, true
		}
	}
	return SkillToolCallRef{}, false
}

func agentCandidateLookupTool(skillID string, toolName string) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return true
	default:
		return false
	}
}

func agentCandidateLookupResultUsable(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	if status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["status"]))); status == "error" || status == "failed" {
		return false
	}
	for _, key := range []string{"count", "total", "target_count"} {
		if count, ok := intFromAny(result[key]); ok {
			return count > 0
		}
	}
	for _, key := range []string{"items", "agents", "skills", "knowledge_bases", "databases", "database_tables", "workflows", "models"} {
		if count := repeatedSuccessfulReadOnlyResultCollectionLength(result[key]); count >= 0 {
			return count > 0
		}
	}
	return true
}

func repeatedReadOnlyPendingAgentMutationStep(skillID string, toolName string, evidence map[string]interface{}, metadata map[string]interface{}) (map[string]interface{}, bool) {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return nil, false
	}
	if !skillToolCallLooksReadOnly(skillID, toolName) {
		return nil, false
	}
	for _, source := range []map[string]interface{}{evidence, metadata} {
		if step, ok := firstPendingAgentMutationPlanStep(source); ok {
			return step, true
		}
	}
	return nil, false
}

func firstPendingAgentMutationPlanStep(source map[string]interface{}) (map[string]interface{}, bool) {
	if len(source) == 0 {
		return nil, false
	}
	plan := evidenceMapFromAny(source["operation_plan"])
	if len(plan) == 0 {
		return nil, false
	}
	steps, _ := fastPathPendingExecutablePlanSteps(plan, 20)
	for _, step := range steps {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		if !isAgentManagementMutationTool(evidenceStringFromAny(step["skill_id"]), evidenceStringFromAny(step["tool_name"])) {
			continue
		}
		return copyStringAnyMap(step), true
	}
	return nil, false
}

func repeatedReadOnlyToolShouldRunAfterMutation(skillID string, toolName string, args map[string]interface{}, successfulToolCalls []SkillToolCallRef) bool {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "get_agent", "get_agent_config":
	default:
		return false
	}
	key := failedToolCallKey(skillID, toolName, args)
	if key == "" {
		return false
	}
	for i := len(successfulToolCalls) - 1; i >= 0; i-- {
		call := successfulToolCalls[i]
		if failedToolCallKey(call.SkillID, call.ToolName, call.Arguments) == key {
			return false
		}
		if isAgentManagementMutationTool(call.SkillID, call.ToolName) {
			return true
		}
	}
	return false
}

func skillToolCallLooksReadOnly(skillID string, toolName string) bool {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if skillID == "" || toolName == "" {
		return false
	}
	for _, prefix := range []string{"get_", "list_", "read_", "search_"} {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}
	return false
}

func repeatedSuccessfulReadOnlyToolCallPayload(skillID string, toolName string, previous SkillToolCallRef) map[string]interface{} {
	return map[string]interface{}{
		"status":                  "completed",
		"advisory":                "same_read_only_tool_already_succeeded",
		"skill_id":                strings.TrimSpace(skillID),
		"tool_name":               strings.TrimSpace(toolName),
		"message":                 "This read-only tool call with identical arguments already succeeded earlier in this turn.",
		"previous_result_summary": summarizeRepeatedSuccessfulReadOnlyResult(previous.Result),
		"next_action":             "Do not call the same read-only tool with identical arguments again. Answer from the previous tool result already present in the message history; if that result is empty, say there are no matching candidates.",
	}
}

func repeatedSuccessfulReadOnlyPendingMutationPayload(skillID string, toolName string, previous SkillToolCallRef, step map[string]interface{}) map[string]interface{} {
	required := completionVerificationPlanStepLabel(step)
	if required == "" {
		required = strings.TrimSpace(evidenceStringFromAny(step["id"]))
	}
	nextAction := strings.Join([]string{
		"Do not call the same read-only tool with identical arguments again.",
		"The current operation plan still has a pending asset-changing Agent step.",
		"Call the suggested pending tool next when the latest user request, page context, and previous read result still show that the mutation is needed.",
		"For Agent config updates, use the plan_step config_goal and the update_agent_config tool schema to infer concrete arguments and extract the target field values; do not wait for another read-only config call when the same read already succeeded.",
		"After the mutation succeeds, read the Agent configuration again only if verification is still required.",
	}, " ")
	if required != "" {
		nextAction = fmt.Sprintf("Suggested next tool: %s. %s", required, nextAction)
	}
	return map[string]interface{}{
		"status":                  "completed",
		"advisory":                "pending_mutation_step_after_repeated_read_only_tool",
		"skill_id":                strings.TrimSpace(skillID),
		"tool_name":               strings.TrimSpace(toolName),
		"suggested_next_tool":     required,
		"plan_step":               copyStringAnyMap(step),
		"message":                 "This read-only tool call already succeeded, but the operation plan still has an asset-changing Agent step pending.",
		"previous_result_summary": summarizeRepeatedSuccessfulReadOnlyResult(previous.Result),
		"next_action":             nextAction,
	}
}

func plannerFeedbackRequestsPendingMutation(trace skills.SkillTrace) bool {
	if !strings.EqualFold(strings.TrimSpace(trace.Kind), "planner_feedback") {
		return false
	}
	if got := strings.TrimSpace(evidenceStringFromAny(trace.Arguments["next_step"])); !strings.EqualFold(got, "call_pending_agent_mutation") {
		return false
	}
	required := strings.TrimSpace(evidenceStringFromAny(trace.Arguments["required_next_tool"]))
	if required == "" {
		required = strings.TrimSpace(evidenceStringFromAny(trace.Arguments["suggested_next_tool"]))
	}
	return required != ""
}

func summarizeRepeatedSuccessfulReadOnlyResult(result map[string]interface{}) map[string]interface{} {
	if len(result) == 0 {
		return nil
	}
	summary := map[string]interface{}{}
	for _, key := range []string{"status", "count", "total", "target_count", "success_count", "failed_count", "agent_id", "agent_name"} {
		if value, ok := result[key]; ok {
			summary[key] = value
		}
	}
	for _, key := range []string{"items", "agents", "skills", "knowledge_bases", "databases", "database_tables", "workflows", "models"} {
		if count := repeatedSuccessfulReadOnlyResultCollectionLength(result[key]); count >= 0 {
			summary[key+"_count"] = count
		}
	}
	if samples := repeatedSuccessfulReadOnlyCandidateSamples(result, 3); len(samples) > 0 {
		summary["candidate_samples"] = samples
	}
	if len(summary) == 0 {
		summary["available"] = true
	}
	return summary
}

func repeatedSuccessfulReadOnlyCandidateSamples(result map[string]interface{}, limit int) []map[string]interface{} {
	if len(result) == 0 || limit <= 0 {
		return nil
	}
	for _, key := range []string{
		"binding_candidates",
		"skills",
		"knowledge_bases",
		"databases",
		"database_tables",
		"tables",
		"workflows",
		"models",
		"items",
	} {
		records := evidenceMapsFromAny(result[key])
		if len(records) == 0 {
			continue
		}
		out := make([]map[string]interface{}, 0, min(len(records), limit))
		for _, record := range records {
			item := repeatedSuccessfulReadOnlyCandidateSample(record)
			if len(item) == 0 {
				continue
			}
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func repeatedSuccessfulReadOnlyCandidateSample(record map[string]interface{}) map[string]interface{} {
	if len(record) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	if id := firstNonEmptyString(record["id"], record["skill_id"], record["dataset_id"], record["knowledge_base_id"], record["data_source_id"], record["table_id"], record["workflow_id"], record["binding_id"]); id != "" {
		item["id"] = id
	}
	if name := firstNonEmptyString(record["name"], record["title"], record["label"], record["display_name"], record["dataset_name"], record["database_name"], record["table_name"], record["workflow_name"], record["model"]); name != "" {
		item["name"] = name
	}
	for _, key := range []string{"selected", "writable", "provider", "model"} {
		if value, ok := record[key]; ok && value != nil && value != "" {
			item[key] = value
		}
	}
	if binding := repeatedSuccessfulReadOnlyCandidateBinding(record["binding"]); len(binding) > 0 {
		item["binding"] = binding
	}
	return item
}

func repeatedSuccessfulReadOnlyCandidateBinding(value interface{}) map[string]interface{} {
	binding := evidenceMapFromAny(value)
	if len(binding) == 0 {
		return nil
	}
	item := map[string]interface{}{}
	for _, key := range []string{"data_source_id", "table_ids", "writable_table_ids", "agent_id", "workflow_id", "binding_id", "version_strategy", "version_uuid", "timeout_seconds"} {
		if value, ok := binding[key]; ok && value != nil && value != "" {
			item[key] = value
		}
	}
	return item
}

func repeatedSuccessfulReadOnlyResultCollectionLength(value interface{}) int {
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []map[string]interface{}:
		return len(typed)
	case []string:
		return len(typed)
	default:
		return -1
	}
}

func redundantPostReadAgentConfigMutationAnswer(skillID string, toolName string, args map[string]interface{}, evidence map[string]interface{}) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) ||
		!agentConfigMutationToolCanCloseFromPostRead(toolName) {
		return "", false
	}
	if !fastPathGoalRequestsAgentConfigPostRead(evidence) {
		return "", false
	}
	if !fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		return "", false
	}
	if !agentConfigRedundantMutationCoveredByEvidence(toolName, args, evidence) {
		return "", false
	}
	return agentConfigPostUpdateVerifiedFastPathAnswerFromEvidence(evidence)
}

func immediateCompletionEvidenceFastPathAnswer(evidence map[string]interface{}) (string, bool) {
	if !fastPathEvidenceHasSuccessfulAgentConfigUpdate(evidence) {
		return "", false
	}
	if !fastPathHasSuccessfulAgentConfigReadAfterUpdate(evidence) {
		return "", false
	}
	return FastPathFinalAnswerForCompletionEvidence(evidence)
}

func agentConfigMutationToolCanCloseFromPostRead(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "update_agent_config", "update_agent_identity":
		return true
	default:
		return false
	}
}

func agentConfigRedundantMutationCoveredByEvidence(toolName string, args map[string]interface{}, evidence map[string]interface{}) bool {
	trace, ok := fastPathLatestSuccessfulAgentConfigUpdateTrace(evidence)
	if !ok {
		return false
	}
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if !strings.EqualFold(strings.TrimSpace(trace.ToolName), toolName) {
		return false
	}
	if !agentConfigMutationTargetsSame(args, trace) {
		return false
	}
	switch toolName {
	case "update_agent_identity":
		return stringSetContainsAll(
			agentIdentityFieldsFromResult(trace.Result),
			agentIdentityFieldsFromArguments(args),
		)
	case "update_agent_config":
		return stringSetContainsAll(
			agentConfigFieldsFromResult(trace.Result),
			agentConfigFieldsFromArguments(args),
		)
	default:
		return false
	}
}

func agentConfigMutationTargetsSame(args map[string]interface{}, trace skills.SkillTrace) bool {
	pending := strings.TrimSpace(firstNonEmptyString(args["agent_id"], args["id"]))
	completed := strings.TrimSpace(firstNonEmptyString(
		trace.Arguments["agent_id"],
		trace.Arguments["id"],
		trace.Result["agent_id"],
		trace.Result["id"],
	))
	if completed == "" {
		if agent := payloadMap(trace.Result, "agent"); len(agent) > 0 {
			completed = strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"]))
		}
	}
	return pending == "" || completed == "" || pending == completed
}

func agentIdentityFieldsFromArguments(args map[string]interface{}) map[string]struct{} {
	fields := map[string]struct{}{}
	for _, field := range []string{"name", "description", "icon", "icon_type", "icon_background"} {
		if _, ok := args[field]; ok {
			fields[agentIdentityCanonicalField(field)] = struct{}{}
		}
	}
	return fields
}

func agentIdentityFieldsFromResult(result map[string]interface{}) map[string]struct{} {
	fields := map[string]struct{}{}
	for _, field := range sanitizedStringListArgumentValue(result["updated_fields"]) {
		if canonical := agentIdentityCanonicalField(field); canonical != "" {
			fields[canonical] = struct{}{}
		}
	}
	return fields
}

func agentIdentityCanonicalField(field string) string {
	switch strings.TrimSpace(field) {
	case "name":
		return "name"
	case "description":
		return "description"
	case "icon", "icon_type", "icon_background":
		return "icon"
	default:
		return ""
	}
}

func agentConfigFieldsFromArguments(args map[string]interface{}) map[string]struct{} {
	fields := map[string]struct{}{}
	for key := range args {
		if canonical := agentConfigCanonicalFieldFromArgument(key); canonical != "" {
			fields[canonical] = struct{}{}
		}
	}
	return fields
}

func agentConfigFieldsFromResult(result map[string]interface{}) map[string]struct{} {
	fields := map[string]struct{}{}
	for _, field := range sanitizedStringListArgumentValue(result["updated_fields"]) {
		if canonical := agentConfigCanonicalFieldFromResult(field); canonical != "" {
			fields[canonical] = struct{}{}
		}
	}
	for _, field := range sanitizedStringListArgumentValue(result["satisfied_fields"]) {
		if canonical := agentConfigCanonicalFieldFromResult(field); canonical != "" {
			fields[canonical] = struct{}{}
		}
	}
	for _, change := range agentConfigChangeMaps(result) {
		if canonical := agentConfigCanonicalFieldFromResult(firstNonEmptyString(change["field"], change["binding_kind"])); canonical != "" {
			fields[canonical] = struct{}{}
		}
	}
	return fields
}

func agentConfigCanonicalFieldFromArgument(field string) string {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return "system_prompt"
	case "model", "model_provider":
		return "model"
	case "model_parameters":
		return "model_parameters"
	case "enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids":
		return "enabled_skill_ids"
	case "agent_memory_enabled":
		return "agent_memory_enabled"
	case "file_upload_enabled":
		return "file_upload_enabled"
	case "home_title":
		return "home_title"
	case "input_placeholder":
		return "input_placeholder"
	case "theme_color":
		return "theme_color"
	case "suggested_questions":
		return "suggested_questions"
	case "knowledge_dataset_ids", "dataset_ids", "add_knowledge_dataset_ids", "remove_knowledge_dataset_ids":
		return "knowledge_dataset_ids"
	case "knowledge_retrieval_config":
		return "knowledge_retrieval_config"
	case "database_bindings", "add_database_bindings", "remove_database_bindings":
		return "database_bindings"
	case "workflow_bindings", "add_workflow_bindings", "remove_workflow_bindings":
		return "workflow_bindings"
	default:
		return ""
	}
}

func agentConfigCanonicalFieldFromResult(field string) string {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return "system_prompt"
	case "model", "model_provider":
		return "model"
	case "model_parameters":
		return "model_parameters"
	case "enabled_skill_ids", "agent_skill":
		return "enabled_skill_ids"
	case "agent_memory_enabled":
		return "agent_memory_enabled"
	case "file_upload_enabled":
		return "file_upload_enabled"
	case "home_title":
		return "home_title"
	case "input_placeholder":
		return "input_placeholder"
	case "theme_color":
		return "theme_color"
	case "suggested_questions":
		return "suggested_questions"
	case "knowledge_dataset_ids", "dataset_ids", "knowledge_base":
		return "knowledge_dataset_ids"
	case "knowledge_retrieval_config":
		return "knowledge_retrieval_config"
	case "database_bindings", "database_table":
		return "database_bindings"
	case "workflow_bindings", "workflow":
		return "workflow_bindings"
	default:
		return ""
	}
}

func stringSetContainsAll(have map[string]struct{}, want map[string]struct{}) bool {
	if len(want) == 0 {
		return false
	}
	for field := range want {
		if _, ok := have[field]; !ok {
			return false
		}
	}
	return true
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
	reason := "\u6267\u884c\u89c4\u5212\u8f6e\u6b21\u5df2\u8fbe\u5230\u4e0a\u9650\uff0c\u65e0\u6cd5\u786e\u8ba4\u672c\u8f6e\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002"
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
		result.Message = "The previous candidate final answer was blocked because the claimed outcome lacks successful skill/tool evidence in this turn. Continue from the latest evidence, call the next useful tool if it is still needed, and only then claim completion."
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
		result.Message = "The requested user clarification was blocked because runtime context already contains the information needed to continue. Continue from the latest evidence and use the next useful tool before asking the user."
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
		result.Message = "The requested skill tool call was blocked because it would run the task in the wrong order. Continue from the latest evidence and use the next useful tool first."
	}
	return result, true
}

func runToolArgumentResolver(resolver ToolArgumentResolver, req ToolCallGuardRequest) (map[string]interface{}, bool) {
	if resolver == nil {
		return nil, false
	}
	resolved, changed := resolver(req)
	if !changed {
		return nil, false
	}
	return copyStringAnyMap(resolved), true
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
	content := localizedAgentProgressText(preparedUserText(prepared), text)
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
	if prepared == nil {
		return ""
	}
	if prepared.LLMRequest != nil {
		for i := len(prepared.LLMRequest.Messages) - 1; i >= 0; i-- {
			message := prepared.LLMRequest.Messages[i]
			if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				continue
			}
			if text := strings.TrimSpace(messageContent(message.Content)); text != "" {
				return text
			}
		}
	}
	return strings.TrimSpace(prepared.Query)
}

func localizedAgentProgressText(userText string, text string) string {
	content := visibleAgentProgressText(text)
	if content == "" {
		return ""
	}
	if containsCJK(userText) && !containsCJK(content) {
		return "\u6211\u5148\u786e\u8ba4\u5f53\u524d\u4fe1\u606f\uff0c\u518d\u7ee7\u7eed\u5904\u7406\u3002"
	}
	return content
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
	if looksLikeIncompleteAgentProgress(content) {
		content = repairedIncompleteAgentProgressText(content)
		if content == "" {
			return ""
		}
	}
	return truncateRunes(content, agentProgressMaxRunes)
}

func repairedIncompleteAgentProgressText(text string) string {
	content := strings.TrimSpace(text)
	if content == "" || looksLikeInternalAgentProgress(content) || !looksLikeIncompleteAgentProgress(content) {
		return ""
	}
	if containsCJK(content) {
		return "\u6211\u6b63\u5728\u6839\u636e\u5df2\u5b8c\u6210\u7684\u7ed3\u679c\u7ee7\u7eed\u5904\u7406\u3002"
	}
	return "I am continuing from the completed results."
}

func looksLikeIncompleteAgentProgress(text string) bool {
	content := strings.TrimSpace(text)
	if content == "" {
		return false
	}
	if strings.HasSuffix(content, "\uff1a") || strings.HasSuffix(content, ":") {
		return true
	}
	if !incompleteAgentProgressListPattern.MatchString(content) {
		return false
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "step") ||
		strings.Contains(lower, "progress") ||
		strings.Contains(content, "\u6b65\u9aa4") ||
		strings.Contains(content, "\u8fdb\u5ea6") ||
		strings.Contains(content, "\u5b8c\u6210") ||
		strings.Contains(content, "\u5df2\u5b8c\u6210") ||
		strings.Contains(content, "\u4ee5\u4e0b")
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
		"submit_turn_state",
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
	for _, fragment := range []string{
		"i need to navigate",
		"i need to open",
		"i will navigate first",
		"i will open first",
		"first navigate",
		"first open the page",
		"\u6211\u9700\u8981\u5148\u5bfc\u822a",
		"\u6211\u9700\u8981\u5148\u6253\u5f00",
		"\u6211\u9700\u8981\u5148\u8fdb\u5165",
		"\u6211\u5148\u5bfc\u822a",
		"\u5148\u5bfc\u822a\u5230",
		"\u5148\u6253\u5f00\u9875\u9762",
		"\u5148\u8fdb\u5165\u9875\u9762",
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
		case '\n', '\u3002', '\uff1b', '\uff01', '\uff1f', '!', '?':
			return strings.TrimSpace(text[:index+len(string(char))])
		case '.':
			if periodLooksLikeNumberedListMarker(text, index) {
				continue
			}
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

func periodLooksLikeNumberedListMarker(text string, index int) bool {
	if index <= 0 || index >= len(text) || text[index] != '.' {
		return false
	}
	prev, ok := lastNonSpaceRune(text[:index])
	if !ok || !unicode.IsDigit(prev) {
		return false
	}
	start := index
	for start > 0 {
		char, size := utf8.DecodeLastRuneInString(text[:start])
		if char == utf8.RuneError && size == 0 {
			break
		}
		if !unicode.IsDigit(char) {
			break
		}
		start -= size
	}
	prefix := strings.TrimSpace(text[:start])
	if prefix != "" {
		last, ok := lastNonSpaceRune(prefix)
		if !ok || (last != ':' && last != '\uff1a') {
			return false
		}
	}
	rest := text[index+1:]
	if rest == "" {
		return true
	}
	for _, next := range rest {
		return unicode.IsSpace(next)
	}
	return false
}

func lastNonSpaceRune(text string) (rune, bool) {
	for i := len(text); i > 0; {
		char, size := utf8.DecodeLastRuneInString(text[:i])
		if char == utf8.RuneError && size == 0 {
			return 0, false
		}
		i -= size
		if !unicode.IsSpace(char) {
			return char, true
		}
	}
	return 0, false
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
		"All user-facing progress, reasoning, request_user_input text, submit_intermediate_answer text, and final answers must use the same language as the user's latest request. If the user writes in Chinese, progress text must be Chinese.",
		"Do not narrate every tool call, internal plan step, tool name, tool arguments, IDs, protocol details, or bookkeeping status.",
		"If you share progress or reasoning, frame it around the user's goal, current page evidence, and the next useful action; do not expose a rigid hidden checklist.",
		"Progress text must be one complete sentence. Do not end progress with a colon, list marker, or half-written numbered/bulleted list.",
		"Do not start every task by listing resources or navigating. If current page context, recent tool results, or visible resolved targets are enough, act from that evidence directly.",
		"Do not announce that you need to navigate, open, enter, or switch pages unless a visible console navigation tool is available and you are about to call it. If no navigation tool is available, say you will continue from current page evidence.",
		"When an additional system message contains preferred_route_action or suggested_next_tool, treat it as an advisory next phase, not as a reason to ignore fresh evidence. Load and call it when the current page context and prior tool/client-action evidence show it is still needed; do not repeat the same navigation or business tool after matching evidence already satisfies the step.",
		"Within one user request, do not reload a skill just because approval, navigation, refresh, or continuation resumed the loop. If the skill was already loaded and no newer instructions are needed, continue from the latest tool results, client-action evidence, and turn_state.",
		"After each skill/tool result, continue with the next necessary action or final answer. Summarize only user-relevant outcomes, not internal bookkeeping.",
		"If a tool call fails, explain the likely user-relevant cause, fix the arguments, and retry when possible.",
		"If a tool call fails, do not repeat the same tool with the same arguments. Re-plan from the error before retrying.",
		"For deterministic batch work, prefer one suitable business tool call that handles the batch coherently over many small repeated tool calls.",
		"Read-only tools may be grouped when useful, but call at most one side-effecting or governed mutation tool in a single assistant turn. Wait for its tool result or governance outcome, then continue with the next mutation in the following loop round.",
		"Do not claim that you saved, remembered, updated, deleted, sent, created, changed, or completed any external action unless the corresponding skill/tool call succeeded in this turn.",
		"Do not claim that a governance approval card has been submitted or is waiting unless a governed skill/tool call actually returned a pending governance event.",
		"If a save, update, delete, create, bind, unbind, publish, or navigation tool succeeded in this turn, describe the outcome as executed and verified from the tool/page evidence; do not say it was unnecessary or skipped just because the refreshed page already shows the requested state.",
		"Progress text sent together with tool calls is transient status text. Keep it short and do not place substantial user deliverables there.",
		"Long tasks may cross approvals, page navigation, page refresh, user confirmation, or continuation boundaries. Those boundaries can make implicit working memory unreliable even within the same user request.",
		"Before crossing a boundary or making later steps depend on a tool/page result, decide whether any exact value, summary, theme, selected target, model choice, prompt requirement, or verification fact must be reused. If yes, call submit_turn_state; use kind=working_fact/decision/verification with visibility=model_only for internal state, or kind=user_deliverable with visibility=user_visible when the reusable summary should also be shown to the user.",
		"Use submit_turn_state for internal working facts, decisions, assumptions, and verification state. Do not expose protocol names or JSON to the user; the recorded state is for continuing the same turn reliably.",
		"Do not record every detail. Record only facts that affect later tool arguments, naming, configuration, verification, or the final answer. For long documents, record a concise summary/theme and re-read if exact full text is needed.",
		"If you later need a value but did not record it and cannot see it in current tool/page evidence, re-read or re-observe it instead of guessing or using placeholders such as file content, read content, or 文件内容.",
		"submit_intermediate_answer is for substantial user-facing deliverables only; do not use it for progress, plans, tool status, internal reasoning, or protocol narration.",
		"Prefer submit_turn_state with kind=user_deliverable for new structured workflows; submit_intermediate_answer is a compatibility shortcut for a user-visible deliverable.",
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
