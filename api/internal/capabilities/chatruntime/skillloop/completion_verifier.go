package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	defaultMaxCompletionVerificationRetries = 2
	completionVerifierMaxTokens             = 1600
	operationPlanToolChoiceModelDecides     = "model_decides"

	completionVerificationStatusPass        = "pass"
	completionVerificationStatusNeedsAction = "needs_action"
	completionVerificationStatusFailed      = "failed"
	completionVerificationStatusAskUser     = "ask_user"

	completionVerificationFallbackAskUser = "\u6211\u8fd8\u9700\u8981\u66f4\u591a\u4fe1\u606f\u624d\u80fd\u53ef\u9760\u7ee7\u7eed\u3002"
	completionVerificationFallbackFailed  = "\u8fd9\u4e00\u6b65\u6ca1\u6709\u88ab\u5de5\u5177\u7ed3\u679c\u786e\u8ba4\u6210\u529f\u3002"
	completionVerificationFallbackUnknown = "\u6211\u8fd8\u4e0d\u80fd\u786e\u8ba4\u8fd9\u4e2a\u64cd\u4f5c\u5df2\u7ecf\u5b8c\u6210\u3002"
	completionVerificationJoinSeparator   = "\u3001"
)

type completionVerificationDecision struct {
	Status              string   `json:"status"`
	Reason              string   `json:"reason"`
	MissingSteps        []string `json:"missing_steps"`
	UnsupportedClaims   []string `json:"unsupported_claims"`
	NextActionHint      string   `json:"next_action_hint"`
	FinalAnswer         string   `json:"final_answer"`
	FinalAnswerGuidance string   `json:"final_answer_guidance"`
	LanguageHint        string   `json:"-"`
}

// ReconcileCompletionVerificationResultWithEvidence applies deterministic
// evidence checks after the model verifier returns. It only clears Agent config
// verification needs_action results when the latest tool evidence now proves the
// requested config state, so real tool failures and unrelated verifier findings
// are still preserved.
func ReconcileCompletionVerificationResultWithEvidence(evidence map[string]interface{}, result CompletionVerificationResult) CompletionVerificationResult {
	if _, ok := FastPathFinalAnswerForCompletionEvidence(evidence); ok {
		result.Status = completionVerificationStatusPass
		result.Reason = strings.TrimSpace(firstNonEmptyString(
			result.Reason,
			"latest tool evidence satisfies the requested operation",
		))
		result.MissingSteps = nil
		result.UnsupportedClaims = nil
		result.NextActionHint = ""
		result.FinalAnswer = ""
		return result
	}
	decision := completionVerificationReconcileDecisionWithEvidence(evidence, completionVerificationDecision{
		Status:            result.Status,
		Reason:            result.Reason,
		MissingSteps:      result.MissingSteps,
		UnsupportedClaims: result.UnsupportedClaims,
		NextActionHint:    result.NextActionHint,
		FinalAnswer:       result.FinalAnswer,
	})
	if decision.normalizedStatus() != completionVerificationStatusPass || strings.EqualFold(strings.TrimSpace(result.Status), strings.TrimSpace(decision.Status)) {
		return result
	}
	result.Status = completionVerificationStatusPass
	result.Reason = strings.TrimSpace(firstNonEmptyString(decision.Reason, result.Reason, "latest Agent config evidence satisfied requested state"))
	result.MissingSteps = nil
	result.UnsupportedClaims = nil
	result.NextActionHint = ""
	result.FinalAnswer = ""
	return result
}

func completionVerificationReconcileDecisionWithEvidence(evidence map[string]interface{}, decision completionVerificationDecision) completionVerificationDecision {
	status := decision.normalizedStatus()
	if status != completionVerificationStatusNeedsAction || !completionVerificationDecisionLooksLikeAgentConfigNeedsAction(decision) {
		return decision
	}
	optimistic := completionVerificationApplyPlanOverride(evidence, completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: "latest evidence satisfies requested Agent config verification",
	})
	if optimistic.normalizedStatus() != completionVerificationStatusPass {
		return decision
	}
	return completionVerificationDecision{
		Status: completionVerificationStatusPass,
		Reason: strings.TrimSpace(firstNonEmptyString(
			decision.Reason,
			"latest Agent config evidence satisfied requested state",
		)),
	}
}

func completionVerificationDecisionLooksLikeAgentConfigNeedsAction(decision completionVerificationDecision) bool {
	values := append([]string{}, decision.MissingSteps...)
	values = append(values, decision.NextActionHint, decision.Reason, decision.FinalAnswerGuidance)
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if strings.Contains(value, "agent-management/get_agent_config") ||
			strings.Contains(value, "agent-management/update_agent_config") ||
			strings.Contains(value, "agent config") ||
			strings.Contains(value, "enabled_skill_ids") ||
			strings.Contains(value, "file_upload_enabled") ||
			strings.Contains(value, "model_provider") {
			return true
		}
	}
	return false
}

func (d completionVerificationDecision) normalizedStatus() string {
	switch strings.ToLower(strings.TrimSpace(d.Status)) {
	case completionVerificationStatusPass:
		return completionVerificationStatusPass
	case completionVerificationStatusNeedsAction, "action", "continue", "retry":
		return completionVerificationStatusNeedsAction
	case completionVerificationStatusAskUser, "ask", "question":
		return completionVerificationStatusAskUser
	case completionVerificationStatusFailed, "fail", "failure", "error":
		return completionVerificationStatusFailed
	default:
		return completionVerificationStatusFailed
	}
}

func completionVerificationShouldRun(evidence map[string]interface{}, attempted []SkillToolCallRef, successful []SkillToolCallRef, toolCallCount int) bool {
	if toolCallCount > 0 || len(attempted) > 0 || len(successful) > 0 {
		return true
	}
	if len(evidence) == 0 {
		return false
	}
	if plan := evidenceMapFromAny(evidence["operation_plan"]); len(plan) > 0 {
		if completionVerificationPlanNeedsRuntimeEvidence(plan) {
			return true
		}
	}
	if invocations := evidenceSliceFromAny(evidence["skill_invocations"]); len(invocations) > 0 {
		return true
	}
	if artifacts := evidenceSliceFromAny(evidence["generated_files"]); len(artifacts) > 0 {
		return true
	}
	if completionVerificationEvidenceValuePresent(evidence["operation_ledger"]) ||
		completionVerificationEvidenceValuePresent(evidence["client_actions"]) ||
		completionVerificationEvidenceValuePresent(evidence["tool_governance"]) {
		return true
	}
	if ledger := evidenceMapFromAny(evidence["execution_ledger"]); completionVerificationLedgerHasFacts(ledger) {
		return true
	}
	return false
}

func completionEvidenceOperationPlanModelDecides(evidence map[string]interface{}) bool {
	if len(evidence) == 0 {
		return false
	}
	return operationPlanModelDecidesTools(evidenceMapFromAny(evidence["operation_plan"]))
}

func operationPlanModelDecidesTools(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(plan["tool_choice_mode"])), operationPlanToolChoiceModelDecides)
}

func completionVerificationEvidenceForPrompt(evidence map[string]interface{}) map[string]interface{} {
	if len(evidence) == 0 {
		return nil
	}
	copied := promptEvidenceCopy(evidence)
	out := evidenceMapFromAny(copied)
	if len(out) == 0 {
		return nil
	}
	if plan := evidenceMapFromAny(out["operation_plan"]); len(plan) > 0 {
		out["operation_plan"] = completionVerificationOperationPlanForPrompt(plan)
	}
	if summary := evidenceMapFromAny(out["execution_summary"]); len(summary) > 0 {
		if plan := evidenceMapFromAny(summary["operation_plan"]); len(plan) > 0 {
			summary["operation_plan"] = completionVerificationOperationPlanForPrompt(plan)
		}
	}
	if ledger := evidenceMapFromAny(out["execution_ledger"]); len(ledger) > 0 {
		if summary := evidenceMapFromAny(ledger["summary"]); len(summary) > 0 {
			if plan := evidenceMapFromAny(summary["operation_plan"]); len(plan) > 0 {
				summary["operation_plan"] = completionVerificationOperationPlanForPrompt(plan)
			}
		}
	}
	return out
}

func completionVerificationOperationPlanForPrompt(plan map[string]interface{}) map[string]interface{} {
	if len(plan) == 0 || !operationPlanModelDecidesTools(plan) {
		return plan
	}
	out := copyStringAnyMap(plan)
	delete(out, "steps")
	delete(out, "step_status")
	delete(out, "structured_plan")
	delete(out, "completed_steps")
	delete(out, "failed_steps")
	if goals := completionVerificationCapabilityGoalsForPrompt(out["capability_goals"]); len(goals) > 0 {
		out["capability_goals"] = goals
	} else {
		delete(out, "capability_goals")
	}
	if state := evidenceMapFromAny(out["strategy_state"]); len(state) > 0 {
		state = copyStringAnyMap(state)
		delete(state, "plan_steps")
		delete(state, "structured_plan")
		if goals := completionVerificationCapabilityGoalsForPrompt(state["capability_goals"]); len(goals) > 0 {
			state["capability_goals"] = goals
		} else {
			delete(state, "capability_goals")
		}
		out["strategy_state"] = state
	}
	if strings.TrimSpace(evidenceStringFromAny(out["planning_mode"])) == "" {
		out["planning_mode"] = "phase_only_model_decides"
	}
	return out
}

func completionVerificationCapabilityGoalsForPrompt(value interface{}) []interface{} {
	goals := evidenceMapsFromAny(value)
	if len(goals) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(goals))
	for _, goal := range goals {
		if len(goal) == 0 {
			continue
		}
		item := copyStringAnyMap(goal)
		delete(item, "candidate_tool")
		out = append(out, item)
	}
	return out
}

func promptEvidenceCopy(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = promptEvidenceCopy(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = promptEvidenceCopy(item)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, promptEvidenceCopy(item))
		}
		return out
	default:
		return value
	}
}

func completionVerificationPlanNeedsRuntimeEvidence(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["status"])))
	if status == "failed" || status == "error" {
		return true
	}
	if fastPathPlanHasPendingPostUpdateAgentRead(plan) {
		return true
	}
	if _, ok := fastPathModelDecidesPendingAgentWorkStep(plan); ok {
		return true
	}
	if operationPlanModelDecidesTools(plan) {
		return false
	}
	pending := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["pending_next_action"])))
	switch pending {
	case "", "none", "no_action", "noop", "answer", "final_answer", "respond", "respond_to_user":
	default:
		return true
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		if completionVerificationPlanStepNeedsRuntimeEvidence(step, stepStatus) {
			return true
		}
	}
	return false
}

func completionVerificationPlanStepNeedsRuntimeEvidence(step map[string]interface{}, stepStatus map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
	id := strings.TrimSpace(evidenceStringFromAny(step["id"]))
	if status == "" && id != "" {
		status = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[id])))
	}
	switch status {
	case "failed", "error":
		return true
	case "completed", "complete", "success", "succeeded", "skipped", "not_applicable":
		return false
	}
	if strings.TrimSpace(evidenceStringFromAny(step["skill_id"])) != "" &&
		strings.TrimSpace(evidenceStringFromAny(step["tool_name"])) != "" {
		return true
	}
	if strings.TrimSpace(evidenceStringFromAny(step["wait_for"])) != "" {
		return true
	}
	if strings.TrimSpace(evidenceStringFromAny(step["client_action"])) != "" ||
		strings.TrimSpace(evidenceStringFromAny(step["action_type"])) != "" {
		return true
	}
	lowerID := strings.ToLower(id)
	return strings.HasPrefix(lowerID, "tool:") ||
		strings.HasPrefix(lowerID, "route:") ||
		strings.HasPrefix(lowerID, "wait:") ||
		strings.HasPrefix(lowerID, "client_action:")
}

func completionVerificationLedgerHasFacts(ledger map[string]interface{}) bool {
	if len(ledger) == 0 {
		return false
	}
	for _, key := range []string{"operation_ledger", "skill_invocations", "generated_files", "client_actions", "tool_governance", "summary"} {
		if completionVerificationEvidenceValuePresent(ledger[key]) {
			return true
		}
	}
	return false
}

func completionVerificationEvidenceValuePresent(value interface{}) bool {
	if len(evidenceMapFromAny(value)) > 0 {
		return true
	}
	return len(evidenceSliceFromAny(value)) > 0
}

func (r *Runner) runCompletionVerifier(
	ctx context.Context,
	prepared *PreparedChat,
	req RunRequest,
	candidateAnswer string,
	round int,
	attempted []SkillToolCallRef,
	successful []SkillToolCallRef,
	toolCallCount int,
) (completionVerificationDecision, *adapter.Usage, error) {
	if r == nil || r.LLMClient == nil || prepared == nil || prepared.LLMRequest == nil {
		return completionVerificationDecision{Status: completionVerificationStatusPass}, nil, nil
	}
	evidence := completionEvidenceForFastPathWithSuccessfulToolCalls(req, successful)
	if evidence == nil {
		evidence = map[string]interface{}{}
	}
	if !completionVerificationShouldRun(evidence, attempted, successful, toolCallCount) {
		return completionVerificationDecision{Status: completionVerificationStatusPass}, nil, nil
	}
	if completionVerificationCandidateAnswerLeaksInternalPlan(candidateAnswer) {
		decision := completionVerificationInternalPlanLeakDecision(evidence)
		decision = completionVerificationAlignLanguage(evidence, decision)
		return decision, nil, nil
	}
	promptEvidence := completionVerificationEvidenceForPrompt(evidence)
	payload := map[string]interface{}{
		"candidate_answer":       strings.TrimSpace(candidateAnswer),
		"evidence":               promptEvidence,
		"attempted_tool_calls":   skillToolCallRefsForVerifier(attempted),
		"successful_tool_calls":  skillToolCallRefsForVerifier(successful),
		"tool_call_count":        toolCallCount,
		"verification_contract":  completionVerificationContract(),
		"max_retries_after_fail": defaultMaxCompletionVerificationRetries,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return completionVerificationDecision{}, nil, fmt.Errorf("marshal completion verifier payload: %w", err)
	}
	var usage *adapter.Usage
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		strictRetry := attempt > 0
		verificationReq := completionVerificationRequest(prepared.LLMRequest, string(payloadJSON), strictRetry)
		startedAt := time.Now()
		resp, err := r.LLMClient.AppChat(ctx, r.AppContext, verificationReq)
		trace := ModelInvocationTrace{
			Phase:      "completion_verifier",
			Round:      round,
			Streaming:  false,
			StartedAt:  startedAt,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Request:    verificationReq,
		}
		if err != nil {
			trace.Error = err.Error()
			r.recordModelInvocation(trace)
			return completionVerificationDecision{}, usage, err
		}
		var message adapter.Message
		if resp != nil {
			usage = mergeUsage(usage, resp.Usage)
			if len(resp.Choices) > 0 {
				message = resp.Choices[0].Message
				trace.Response = &message
			}
		}
		if resp != nil {
			trace.Usage = resp.Usage
		}
		r.recordModelInvocation(trace)
		decision, err := parseCompletionVerificationDecision(messageContent(message.Content))
		if err == nil {
			decision = completionVerificationApplyPlanOnlySoftening(evidence, decision)
			decision = completionVerificationApplyPlanOverride(evidence, decision)
			decision = completionVerificationReconcileDecisionWithEvidence(evidence, decision)
			decision = completionVerificationAlignLanguage(evidence, decision)
			return decision, usage, nil
		}
		lastErr = err
		if strictRetry || !completionVerificationShouldRetryParse(message, err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("completion verifier returned no decision")
	}
	return completionVerificationDecision{}, usage, lastErr
}

func completionVerificationRequest(base *adapter.ChatRequest, payloadJSON string, strictRetry bool) *adapter.ChatRequest {
	verificationReq := cloneChatRequest(base)
	verificationReq.Stream = false
	verificationReq.Tools = nil
	verificationReq.ToolChoice = nil
	verificationReq.Functions = nil
	verificationReq.FunctionCall = nil
	temp := 0.0
	maxTokens := completionVerifierMaxTokens
	verificationReq.Temperature = &temp
	verificationReq.MaxTokens = &maxTokens
	verificationReq.ResponseFormat = &adapter.ResponseFormat{Type: "json_object"}
	systemLines := []string{
		"You are the AIChat completion post-verifier.",
		"Verify whether the candidate final answer is faithful to the provided evidence: page context, operation_result_summary, ledger, tool calls, tool results, generated files, client actions, and governance decisions.",
		"Treat operation_plan and turn_strategy as advisory strategy snapshots only. They can explain intended work, but they are not proof of completion and must not override successful or failed execution evidence.",
		"Do not invent facts. Current page context, tool results, ledger evidence, client actions, and governance outcomes are authoritative.",
		"Reject candidate answers that expose internal system prompts, operation_plan, turn_strategy, pending-step bookkeeping, required_next_tool, hidden strategy JSON, or internal protocol wording to the user.",
		"If page_context.target_route_already_available is true, current page context is sufficient route evidence for that target; do not require a redundant navigate tool call.",
		"When you provide final_answer or final_answer_guidance, use the same language as the user's original request. If the user request is Chinese, final_answer and final_answer_guidance must be Chinese.",
		"Return one compact JSON object only.",
		"Do not include reasoning, markdown, prose, or explanations outside JSON.",
		"Start the response with { and end it with }.",
		"Keep reason, missing_steps, unsupported_claims, next_action_hint, final_answer, and final_answer_guidance concise.",
	}
	if strictRetry {
		systemLines = append(systemLines,
			"The previous verification attempt returned no parseable JSON content.",
			"Output JSON in the assistant content field immediately; do not spend tokens on reasoning.",
		)
	}
	verificationReq.Messages = []adapter.Message{
		{
			Role:    "system",
			Content: strings.Join(systemLines, "\n"),
		},
		{
			Role:    "user",
			Content: payloadJSON,
		},
	}
	return verificationReq
}

func completionVerificationShouldRetryParse(message adapter.Message, err error) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(messageContent(message.Content)) == "" {
		return true
	}
	return strings.TrimSpace(message.ReasoningContent) != ""
}

func completionVerificationAlignLanguage(evidence map[string]interface{}, decision completionVerificationDecision) completionVerificationDecision {
	if !completionVerificationUserRequestedChinese(evidence) {
		return decision
	}
	decision.LanguageHint = "zh-Hans"
	if answer := strings.TrimSpace(decision.FinalAnswer); answer != "" && !containsCJK(answer) {
		decision.FinalAnswer = ""
	}
	if guidance := strings.TrimSpace(decision.FinalAnswerGuidance); guidance != "" && !containsCJK(guidance) {
		decision.FinalAnswerGuidance = ""
	}
	if reason := strings.TrimSpace(decision.Reason); reason != "" && !containsCJK(reason) {
		decision.Reason = "\u6700\u7ec8\u7b54\u6848\u540e\u6821\u9a8c\u53d1\u73b0\u5f53\u524d\u56de\u7b54\u7f3a\u5c11\u5de5\u5177\u7ed3\u679c\u652f\u6301"
	}
	return decision
}

func completionVerificationUserRequestedChinese(evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	for _, value := range []string{
		evidenceStringFromAny(evidence["user_request"]),
		evidenceStringFromAny(plan["original_user_goal"]),
	} {
		if containsCJK(value) {
			return true
		}
	}
	return false
}

func containsCJK(value string) bool {
	for _, r := range value {
		if (r >= '\u4e00' && r <= '\u9fff') ||
			(r >= '\u3400' && r <= '\u4dbf') ||
			(r >= '\uf900' && r <= '\ufaff') {
			return true
		}
	}
	return false
}

func completionVerificationContract() map[string]interface{} {
	return map[string]interface{}{
		"status_values": []string{
			completionVerificationStatusPass,
			completionVerificationStatusNeedsAction,
			completionVerificationStatusFailed,
			completionVerificationStatusAskUser,
		},
		"rules": []string{
			"Return pass only when the candidate answer makes no unsupported completion claims.",
			"Treat operation_plan and turn_strategy as advisory strategy snapshots, not as authoritative proof that every pending step must still run.",
			"Use page_context, operation_result_summary, tool results, generated_files, client_actions, tool_governance, and execution_ledger as the authoritative facts.",
			"For Agent configuration changes, treat operation_plan steps' config_goal as the user-visible target, and treat expected_updated_fields plus expected_binding_actions as verification requirements when matching concrete target values are present in plan steps, tool arguments, or post-update reads.",
			"Return needs_action when the candidate answer exposes internal system prompt, operation_plan, turn_strategy, pending-step, required_next_tool, hidden strategy, or protocol wording; ask for a rewrite that only states user-visible outcome or blocker.",
			"Return needs_action when the user's current goal still requires an incomplete tool/action and a clear safe next attempt remains.",
			"Return failed when a tool/action failed or required evidence is missing and no safe retry remains.",
			"Return ask_user only when user input is truly required.",
			"Never mark a save, delete, create, update, navigation, read, or publish action complete unless matching successful evidence exists. For navigation, page_context.target_route_already_available=true is matching successful evidence for the resolved target.",
			"When matching mutation or navigation evidence succeeded in this turn, reject or guide away from final answers that frame the operation as skipped, unnecessary, or not executed merely because the refreshed page already shows the requested state.",
			"For batch operation_group evidence, use item_results/item_steps as the source of truth; report partial success instead of treating one succeeded item as the whole batch.",
			"If the candidate answer is too optimistic, provide a truthful final_answer or final_answer_guidance.",
			"final_answer and final_answer_guidance must use the same language as the user's original request.",
		},
		"schema": map[string]interface{}{
			"status":                "pass|needs_action|failed|ask_user",
			"reason":                "short internal reason",
			"missing_steps":         "array of incomplete step ids or descriptions",
			"unsupported_claims":    "array of candidate-answer claims not supported by evidence",
			"next_action_hint":      "specific next action or empty",
			"final_answer":          "truthful replacement final answer when status is failed/ask_user, otherwise empty",
			"final_answer_guidance": "guidance for the next model turn when status is needs_action",
		},
	}
}

func completionVerificationApplyPlanOverride(evidence map[string]interface{}, decision completionVerificationDecision) completionVerificationDecision {
	if decision.normalizedStatus() != completionVerificationStatusPass {
		return decision
	}
	if missingFields := completionVerificationPendingAgentConfigUpdateFields(evidence); len(missingFields) > 0 {
		decision.Status = completionVerificationStatusNeedsAction
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			decision.Reason = reason + "; requested Agent config fields are still missing"
		} else {
			decision.Reason = "requested Agent config fields are still missing"
		}
		missingStep := "agent-management/update_agent_config missing fields: " + strings.Join(missingFields, ", ")
		decision.MissingSteps = append(cleanStringSlice(decision.MissingSteps), missingStep)
		decision.NextActionHint = "agent-management/update_agent_config"
		decision.FinalAnswer = ""
		decision.FinalAnswerGuidance = completionVerificationAgentConfigMissingFieldsGuidance(missingFields)
		return decision
	}
	if fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		decision.Status = completionVerificationStatusNeedsAction
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			decision.Reason = reason + "; requested post-update Agent config read is still missing"
		} else {
			decision.Reason = "requested post-update Agent config read is still missing"
		}
		decision.MissingSteps = append(cleanStringSlice(decision.MissingSteps), "agent-management/get_agent_config post-update verification")
		decision.NextActionHint = "agent-management/get_agent_config"
		decision.FinalAnswer = ""
		decision.FinalAnswerGuidance = completionVerificationAgentConfigPostReadGuidance(completionEvidenceOperationPlanModelDecides(evidence))
		return decision
	}
	if mismatches := completionVerificationAgentConfigMismatches(evidence); len(mismatches) > 0 {
		decision.Status = completionVerificationStatusNeedsAction
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			decision.Reason = reason + "; requested Agent config state was not verified"
		} else {
			decision.Reason = "requested Agent config state was not verified"
		}
		for _, mismatch := range mismatches {
			decision.MissingSteps = append(cleanStringSlice(decision.MissingSteps), mismatch)
		}
		if completionVerificationAgentConfigNeedsPostRead(mismatches) {
			decision.NextActionHint = "agent-management/get_agent_config"
		} else {
			decision.NextActionHint = "agent-management/update_agent_config"
		}
		decision.FinalAnswer = ""
		decision.FinalAnswerGuidance = completionVerificationAgentConfigMismatchGuidance(mismatches, completionEvidenceOperationPlanModelDecides(evidence))
		return decision
	}
	if failedStepLabel, ok := completionVerificationFailedOperationPlanStepLabel(evidence); ok && completionVerificationHasFailedEvidenceForPlanStep(evidence, failedStepLabel) {
		decision.Status = completionVerificationStatusFailed
		failedReason := "operation plan failed"
		if failedStepLabel != "" {
			failedReason = "operation plan failed at " + failedStepLabel
		}
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			decision.Reason = reason + "; " + failedReason
		} else {
			decision.Reason = failedReason
		}
		if len(cleanStringSlice(decision.UnsupportedClaims)) == 0 {
			decision.UnsupportedClaims = []string{"candidate answer passed despite a failed operation plan"}
		} else {
			decision.UnsupportedClaims = cleanStringSlice(decision.UnsupportedClaims)
		}
		decision.FinalAnswer = completionVerificationFailedPlanFinalAnswer(failedStepLabel, completionVerificationFailureDetailForStep(evidence, failedStepLabel))
		return decision
	}
	return decision
}

func completionVerificationPendingAgentConfigUpdateFields(evidence map[string]interface{}) []string {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	if operationPlanModelDecidesTools(plan) {
		return nil
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		missing := completionVerificationStringSlice(step["missing_updated_fields"])
		if len(missing) == 0 {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
		if status == "" {
			if id := strings.TrimSpace(evidenceStringFromAny(step["id"])); id != "" {
				status = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[id])))
			}
		}
		switch status {
		case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
			continue
		}
		return missing
	}
	return nil
}

func completionVerificationStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "[") {
			var decoded []interface{}
			if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
				return completionVerificationStringSlice(decoded)
			}
			var decodedStrings []string
			if err := json.Unmarshal([]byte(trimmed), &decodedStrings); err == nil {
				return cleanStringSlice(decodedStrings)
			}
		}
		parts := strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == ';' || r == '，' || r == '；' || r == '\n' || r == '\t'
		})
		return cleanStringSlice(parts)
	case []string:
		return cleanStringSlice(typed)
	}
	out := []string{}
	for _, item := range evidenceSliceFromAny(value) {
		if len(evidenceMapFromAny(item)) > 0 {
			continue
		}
		text := strings.TrimSpace(evidenceStringFromAny(item))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return cleanStringSlice(out)
}

func completionVerificationAgentConfigMissingFieldsGuidance(fields []string) string {
	if len(fields) == 0 {
		return "Call agent-management/update_agent_config again with the remaining requested Agent config fields, then verify with get_agent_config."
	}
	return "Call agent-management/update_agent_config again and include these remaining requested fields: " + strings.Join(fields, ", ") + ". Then verify with get_agent_config before the final answer."
}

func completionVerificationApplyPlanOnlySoftening(evidence map[string]interface{}, decision completionVerificationDecision) completionVerificationDecision {
	status := decision.normalizedStatus()
	if status != completionVerificationStatusNeedsAction && status != completionVerificationStatusFailed {
		return decision
	}
	if strings.TrimSpace(decision.FinalAnswer) != "" {
		return decision
	}
	if completionVerificationHasFailedEvidence(evidence) || !completionVerificationHasSuccessfulEvidence(evidence) {
		return decision
	}
	if completionVerificationHasUnsatisfiedManagedFileSave(evidence) {
		return decision
	}
	if len(completionVerificationPendingAgentConfigUpdateFields(evidence)) > 0 {
		return decision
	}
	if fastPathCompletionEvidenceNeedsAgentConfigPostRead(evidence) {
		return decision
	}
	if len(completionVerificationAgentConfigMismatches(evidence)) > 0 {
		return decision
	}
	if !completionVerificationDecisionIsPlanOnly(evidence, decision) {
		return decision
	}
	decision.Status = completionVerificationStatusPass
	decision.MissingSteps = nil
	decision.UnsupportedClaims = nil
	decision.NextActionHint = ""
	decision.FinalAnswerGuidance = ""
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		decision.Reason = reason + "; plan-only verifier decision softened because successful evidence exists"
	} else {
		decision.Reason = "plan-only verifier decision softened because successful evidence exists"
	}
	return decision
}

func completionVerificationAgentConfigPostReadGuidance(modelDecidesTools bool) string {
	if modelDecidesTools {
		return "Use the available Agent management capabilities to verify the refreshed Agent configuration before the final answer. Do not claim the requested post-update verification is complete until fresh configuration evidence succeeds."
	}
	return "Call agent-management/get_agent_config again after the successful Agent config update, then base the final answer on that fresh configuration result. Do not claim the requested post-update verification is complete until that read succeeds."
}

type completionVerificationAgentConfigBindingExpectation struct {
	Field   string
	Action  string
	Targets []string
}

func completionVerificationAgentConfigMismatches(evidence map[string]interface{}) []string {
	return cleanStringSlice(append(
		completionVerificationAgentConfigFieldMismatches(evidence),
		completionVerificationAgentConfigBindingMismatches(evidence)...,
	))
}

func completionVerificationAgentConfigFieldExpectations(evidence map[string]interface{}) map[string]map[string]interface{} {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	expectations := map[string]map[string]interface{}{}
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		if completionVerificationPlanStepClearlyTerminalWithoutSuccess(plan, step) {
			continue
		}
		fields := completionVerificationCanonicalAgentConfigFields(step["expected_updated_fields"])
		if len(fields) == 0 {
			continue
		}
		for _, source := range []map[string]interface{}{step, evidenceMapFromAny(step["arguments"])} {
			completionVerificationMergeAgentConfigFieldTargets(expectations, fields, source)
		}
	}
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "update_agent_config") {
			continue
		}
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		args := completionVerificationInvocationArguments(invocation)
		if len(args) == 0 {
			continue
		}
		fields := completionVerificationCanonicalAgentConfigFields(args["expected_updated_fields"])
		if len(fields) == 0 {
			fields = completionVerificationCanonicalAgentConfigFieldsFromArguments(args)
		}
		completionVerificationMergeAgentConfigFieldTargets(expectations, fields, args)
	}
	if len(expectations) == 0 {
		return nil
	}
	return expectations
}

func completionVerificationMergeAgentConfigFieldTargets(expectations map[string]map[string]interface{}, fields []string, source map[string]interface{}) {
	if len(source) == 0 || len(fields) == 0 {
		return
	}
	for _, field := range fields {
		target := completionVerificationAgentConfigFieldTarget(field, source)
		if len(target) == 0 {
			continue
		}
		current := expectations[field]
		if current == nil {
			current = map[string]interface{}{}
			expectations[field] = current
		}
		for key, value := range target {
			current[key] = value
		}
	}
}

func completionVerificationAgentConfigFieldTarget(field string, source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	switch field {
	case "model":
		if hint := completionVerificationAgentModelHint(source); hint != "" {
			out["model_hint"] = hint
		}
		if completionVerificationMapHasKey(source, "model_provider") {
			out["model_provider"] = source["model_provider"]
		} else if completionVerificationMapHasKey(source, "provider") {
			out["model_provider"] = source["provider"]
		}
		if completionVerificationMapHasKey(source, "model") {
			out["model"] = source["model"]
		} else if completionVerificationMapHasKey(source, "model_name") {
			out["model"] = source["model_name"]
		}
	case "system_prompt", "agent_memory_enabled", "file_upload_enabled", "home_title", "input_placeholder", "theme_color", "suggested_questions":
		if completionVerificationMapHasKey(source, field) {
			out["value"] = source[field]
		}
	}
	return out
}

func completionVerificationAgentModelHint(source map[string]interface{}) string {
	for _, key := range []string{"model_query", "model_hint", "target_model", "desired_model", "requested_model"} {
		if value := strings.TrimSpace(evidenceStringFromAny(source[key])); value != "" {
			return value
		}
	}
	return completionVerificationExtractAgentModelHint(evidenceStringFromAny(source["config_goal"]))
}

func completionVerificationExtractAgentModelHint(goal string) string {
	if strings.TrimSpace(goal) == "" {
		return ""
	}
	lower := strings.ToLower(goal)
	markers := []string{
		"模型配置为",
		"模型设置为",
		"模型改为",
		"模型换成",
		"模型使用",
		"模型用",
		"使用模型",
		"model configured as",
		"model set to",
		"model should be",
		"model is",
		"model:",
		"model",
	}
	bestIndex := -1
	bestMarker := ""
	for _, marker := range markers {
		if index := strings.Index(lower, marker); index >= 0 && (bestIndex < 0 || index < bestIndex) {
			bestIndex = index
			bestMarker = marker
		}
	}
	if bestIndex < 0 {
		return ""
	}
	start := bestIndex + len(bestMarker)
	if start > len(goal) {
		return ""
	}
	rest := completionVerificationTrimAgentModelHintPrefix(goal[start:])
	if rest == "" {
		return ""
	}
	stop := len(rest)
	for index, r := range rest {
		switch r {
		case '，', '。', '；', ';', ',', '.', '\n', '\r':
			stop = index
			goto done
		}
	}
done:
	hint := strings.TrimSpace(rest[:stop])
	if len([]rune(hint)) > 80 {
		hint = string([]rune(hint)[:80])
	}
	return strings.Trim(hint, " \t\r\n\"'`“”‘’")
}

func completionVerificationTrimAgentModelHintPrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, " \t\r\n:：")
	for {
		trimmed := strings.TrimSpace(value)
		lower := strings.ToLower(trimmed)
		next := trimmed
		for _, prefix := range []string{"配置为", "设置为", "改为", "换成", "使用", "为", "是", "成", "到", "to ", "as ", "use ", "using "} {
			if strings.HasPrefix(lower, prefix) {
				next = strings.TrimSpace(trimmed[len(prefix):])
				break
			}
		}
		if next == trimmed {
			return trimmed
		}
		value = next
	}
}

func completionVerificationCanonicalAgentConfigFields(value interface{}) []string {
	fields := completionVerificationStringSlice(value)
	out := []string{}
	for _, field := range fields {
		if canonical := completionVerificationCanonicalAgentConfigField(field); canonical != "" {
			out = append(out, canonical)
		}
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationCanonicalAgentConfigFieldsFromArguments(args map[string]interface{}) []string {
	if len(args) == 0 {
		return nil
	}
	out := []string{}
	for key := range args {
		if canonical := completionVerificationCanonicalAgentConfigField(key); canonical != "" {
			out = append(out, canonical)
		}
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationCanonicalAgentConfigField(field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "model", "model_provider", "provider":
		return "model"
	case "system_prompt":
		return "system_prompt"
	case "agent_memory_enabled", "use_memory":
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
	case "enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids", "agent_skill", "skill", "skills":
		return "enabled_skill_ids"
	case "knowledge_dataset_ids", "dataset_ids", "add_knowledge_dataset_ids", "remove_knowledge_dataset_ids", "knowledge_base":
		return "knowledge_dataset_ids"
	case "database_bindings", "add_database_bindings", "remove_database_bindings", "database_table":
		return "database_bindings"
	case "workflow_bindings", "add_workflow_bindings", "remove_workflow_bindings", "workflow":
		return "workflow_bindings"
	default:
		return ""
	}
}

func completionVerificationAgentConfigFieldMismatches(evidence map[string]interface{}) []string {
	expectations := completionVerificationAgentConfigFieldExpectations(evidence)
	if len(expectations) == 0 {
		return nil
	}
	configRead, ok := completionVerificationLatestAgentConfigReadResult(evidence)
	if !ok {
		return []string{"agent-management/get_agent_config post-update verification missing"}
	}
	config := completionVerificationAgentConfigMap(configRead)
	mismatches := []string{}
	for field, target := range expectations {
		switch field {
		case "model":
			if hint := strings.TrimSpace(evidenceStringFromAny(target["model_hint"])); hint != "" &&
				!completionVerificationAgentModelActualMatchesHint(configRead, hint) {
				mismatches = append(mismatches, "agent-management/get_agent_config model mismatch: requested model hint "+hint+" was not verified")
			}
			provider := strings.TrimSpace(evidenceStringFromAny(target["model_provider"]))
			model := strings.TrimSpace(evidenceStringFromAny(target["model"]))
			if provider != "" {
				actualProvider := strings.TrimSpace(firstNonEmptyString(config["model_provider"], config["provider"], configRead["model_provider"], configRead["provider"]))
				if !strings.EqualFold(actualProvider, provider) {
					mismatches = append(mismatches, "agent-management/get_agent_config model_provider mismatch: want "+provider+", got "+firstNonEmptyDisplay(actualProvider, "<empty>"))
				}
			}
			if model != "" {
				actualModel := strings.TrimSpace(firstNonEmptyString(config["model"], config["model_name"], configRead["model"], configRead["model_name"]))
				if !strings.EqualFold(actualModel, model) {
					mismatches = append(mismatches, "agent-management/get_agent_config model mismatch: want "+model+", got "+firstNonEmptyDisplay(actualModel, "<empty>"))
				}
			}
		case "system_prompt", "home_title", "input_placeholder", "theme_color":
			want := strings.TrimSpace(evidenceStringFromAny(target["value"]))
			if want == "" {
				continue
			}
			actual := strings.TrimSpace(firstNonEmptyString(config[field], configRead[field]))
			if !completionVerificationAgentConfigTextMatches(want, actual) {
				mismatches = append(mismatches, "agent-management/get_agent_config "+field+" mismatch")
			}
		case "suggested_questions":
			want := completionVerificationStringSlice(target["value"])
			if len(want) == 0 {
				continue
			}
			actual := completionVerificationStringSlice(firstNonEmptyValue(config[field], configRead[field]))
			if !completionVerificationStringSlicesEqual(want, actual) {
				mismatches = append(mismatches, "agent-management/get_agent_config suggested_questions mismatch")
			}
		case "agent_memory_enabled", "file_upload_enabled":
			want, ok := completionVerificationBoolFromAny(target["value"])
			if !ok {
				continue
			}
			actual, ok := completionVerificationBoolFromAny(firstNonEmptyValue(config[field], configRead[field]))
			if !ok || actual != want {
				mismatches = append(mismatches, "agent-management/get_agent_config "+field+" mismatch: want "+strconv.FormatBool(want))
			}
		}
	}
	return cleanStringSlice(mismatches)
}

func completionVerificationAgentModelActualMatchesHint(configRead map[string]interface{}, hint string) bool {
	tokens := completionVerificationAgentModelHintTokens(hint)
	if len(tokens) == 0 {
		return true
	}
	config := completionVerificationAgentConfigMap(configRead)
	actual := completionVerificationCompactAgentModelText(strings.Join([]string{
		evidenceStringFromAny(config["model_provider"]),
		evidenceStringFromAny(config["provider"]),
		evidenceStringFromAny(config["model"]),
		evidenceStringFromAny(config["model_name"]),
		evidenceStringFromAny(config["display_name"]),
		evidenceStringFromAny(configRead["model_provider"]),
		evidenceStringFromAny(configRead["provider"]),
		evidenceStringFromAny(configRead["model"]),
		evidenceStringFromAny(configRead["model_name"]),
		evidenceStringFromAny(configRead["display_name"]),
	}, " "))
	if actual == "" {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(actual, token) {
			return false
		}
	}
	return true
}

func completionVerificationAgentModelHintTokens(hint string) []string {
	normalized := strings.ToLower(strings.TrimSpace(hint))
	if normalized == "" {
		return nil
	}
	tokens := []string{}
	seen := map[string]struct{}{}
	for _, raw := range strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		token := completionVerificationCompactAgentModelText(raw)
		if token == "" || completionVerificationAgentModelHintTokenIsGeneric(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	if compact := completionVerificationCompactAgentModelText(normalized); compact != "" && len(tokens) == 0 {
		tokens = append(tokens, compact)
	}
	return tokens
}

func completionVerificationCompactAgentModelText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func completionVerificationAgentModelHintTokenIsGeneric(token string) bool {
	switch token {
	case "model", "llm", "ai", "chat", "text", "use", "using", "to", "as", "is", "the":
		return true
	default:
		return false
	}
}

func completionVerificationAgentConfigBindingMismatches(evidence map[string]interface{}) []string {
	expectations := completionVerificationAgentConfigBindingExpectations(evidence)
	if len(expectations) == 0 {
		return nil
	}
	configRead, ok := completionVerificationLatestAgentConfigReadResult(evidence)
	if !ok {
		return []string{"agent-management/get_agent_config post-update verification missing"}
	}
	mismatches := []string{}
	for _, expectation := range expectations {
		if len(expectation.Targets) == 0 {
			continue
		}
		actualIDs := completionVerificationAgentConfigCollectionIDs(configRead, expectation.Field)
		switch expectation.Action {
		case "bind", "add", "replace":
			for _, target := range expectation.Targets {
				if !completionVerificationAgentConfigBindingTargetSatisfied(evidence, actualIDs, expectation.Field, target) {
					mismatches = append(mismatches, "agent-management/get_agent_config "+expectation.Field+" missing bound target: "+target)
				}
			}
			if expectation.Action == "replace" {
				for _, extra := range completionVerificationUnexpectedAgentConfigBindingTargets(evidence, expectation.Field, actualIDs, expectation.Targets) {
					mismatches = append(mismatches, "agent-management/get_agent_config "+expectation.Field+" contains unexpected target after replace: "+extra)
				}
			}
		case "unbind", "remove":
			for _, target := range expectation.Targets {
				if completionVerificationAgentConfigBindingTargetSatisfied(evidence, actualIDs, expectation.Field, target) {
					mismatches = append(mismatches, "agent-management/get_agent_config "+expectation.Field+" still contains removed target: "+target)
				}
			}
		}
	}
	return cleanStringSlice(mismatches)
}

func completionVerificationAgentConfigBindingTargetSatisfied(evidence map[string]interface{}, actual []string, field string, target string) bool {
	for _, alias := range completionVerificationAgentConfigBindingTargetAliases(evidence, field, target) {
		if completionVerificationStringSliceContainsFold(actual, alias) {
			return true
		}
	}
	return false
}

func completionVerificationAgentConfigBindingTargetAliases(evidence map[string]interface{}, field string, target string) []string {
	aliases := []string{target}
	if strings.EqualFold(strings.TrimSpace(field), "enabled_skill_ids") {
		for _, candidate := range fastPathAgentSkillCandidates(fastPathSuccessfulAgentCandidateLookupResults(evidence)["list_agent_skill_candidates"]) {
			refs := cleanStringSlice([]string{candidate.ID, candidate.Name})
			for _, ref := range refs {
				if strings.EqualFold(strings.TrimSpace(ref), strings.TrimSpace(target)) {
					aliases = append(aliases, refs...)
					break
				}
			}
		}
	}
	return cleanStringSlice(dedupeStrings(aliases))
}

func completionVerificationUnexpectedAgentConfigBindingTargets(evidence map[string]interface{}, field string, actual []string, expected []string) []string {
	actual = cleanStringSlice(dedupeStrings(actual))
	expected = cleanStringSlice(dedupeStrings(expected))
	if len(actual) == 0 || len(expected) == 0 {
		return nil
	}
	expectedAliases := []string{}
	for _, target := range expected {
		expectedAliases = append(expectedAliases, completionVerificationAgentConfigBindingTargetAliases(evidence, field, target)...)
	}
	out := []string{}
	for _, item := range actual {
		if !completionVerificationStringSliceContainsFold(expectedAliases, item) {
			out = append(out, item)
		}
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationAgentConfigNeedsPostRead(mismatches []string) bool {
	for _, mismatch := range mismatches {
		if strings.Contains(strings.ToLower(strings.TrimSpace(mismatch)), "post-update verification missing") {
			return true
		}
	}
	return false
}

func completionVerificationAgentConfigMismatchGuidance(mismatches []string, modelDecidesTools bool) string {
	if len(mismatches) == 0 {
		if modelDecidesTools {
			return "Verify the requested Agent config state with the available Agent management capabilities before the final answer."
		}
		return "Verify the requested Agent config state with get_agent_config before the final answer."
	}
	if modelDecidesTools {
		return "The refreshed Agent configuration evidence did not confirm the requested state: " + strings.Join(completionVerificationModelDecidesPublicTexts(mismatches), "; ") + ". If the change is still needed, choose the appropriate available Agent management capability to apply the concrete target values, then verify the refreshed configuration before the final answer."
	}
	return "The post-update Agent config read did not confirm the requested state: " + strings.Join(mismatches, "; ") + ". If the change is still needed, call agent-management/update_agent_config with the concrete target values, then call get_agent_config again and base the final answer on that result."
}

func completionVerificationAgentConfigBindingExpectations(evidence map[string]interface{}) []completionVerificationAgentConfigBindingExpectation {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return nil
	}
	expectations := []completionVerificationAgentConfigBindingExpectation{}
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		if completionVerificationPlanStepClearlyTerminalWithoutSuccess(plan, step) {
			continue
		}
		actions := completionVerificationAgentConfigBindingActionsFromAny(step["expected_binding_actions"])
		for field, action := range actions {
			field = completionVerificationCanonicalAgentConfigBindingField(field)
			action = completionVerificationCanonicalAgentConfigBindingAction(action)
			if field == "" || action == "" {
				continue
			}
			targets := completionVerificationAgentConfigBindingTargets(evidence, step, field, action)
			expectations = append(expectations, completionVerificationAgentConfigBindingExpectation{
				Field:   field,
				Action:  action,
				Targets: targets,
			})
		}
	}
	return completionVerificationMergeAgentConfigBindingExpectations(expectations)
}

func completionVerificationPlanStepClearlyTerminalWithoutSuccess(plan map[string]interface{}, step map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
	if status == "" {
		stepStatus := evidenceMapFromAny(plan["step_status"])
		if id := strings.TrimSpace(evidenceStringFromAny(step["id"])); id != "" {
			status = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[id])))
		}
	}
	switch status {
	case "failed", "error", "skipped", "not_applicable", "rejected":
		return true
	default:
		return false
	}
}

func completionVerificationAgentConfigBindingActionsFromAny(value interface{}) map[string]string {
	out := map[string]string{}
	if typed := evidenceMapFromAny(value); len(typed) > 0 {
		for rawField, rawAction := range typed {
			field := completionVerificationCanonicalAgentConfigBindingField(rawField)
			action := completionVerificationCanonicalAgentConfigBindingAction(evidenceStringFromAny(rawAction))
			if field != "" && action != "" {
				out[field] = action
			}
		}
		return out
	}
	text := strings.TrimSpace(evidenceStringFromAny(value))
	if text == "" {
		return out
	}
	for _, part := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || r == '\n' || r == '\t'
	}) {
		field, action, ok := strings.Cut(part, ":")
		if !ok {
			field, action, ok = strings.Cut(part, "=")
		}
		if !ok {
			continue
		}
		canonicalField := completionVerificationCanonicalAgentConfigBindingField(field)
		canonicalAction := completionVerificationCanonicalAgentConfigBindingAction(action)
		if canonicalField != "" && canonicalAction != "" {
			out[canonicalField] = canonicalAction
		}
	}
	return out
}

func completionVerificationCanonicalAgentConfigBindingField(field string) string {
	switch completionVerificationCanonicalAgentConfigField(field) {
	case "enabled_skill_ids":
		return "enabled_skill_ids"
	case "knowledge_dataset_ids":
		return "knowledge_dataset_ids"
	case "database_bindings":
		return "database_bindings"
	case "workflow_bindings":
		return "workflow_bindings"
	default:
		return ""
	}
}

func completionVerificationCanonicalAgentConfigBindingAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "bind", "add", "append":
		return "bind"
	case "unbind", "remove", "delete":
		return "unbind"
	case "replace", "set":
		return "replace"
	default:
		return ""
	}
}

func completionVerificationAgentConfigBindingTargets(evidence map[string]interface{}, step map[string]interface{}, field string, action string) []string {
	targets := []string{}
	targets = completionVerificationAppendAgentConfigBindingTargets(targets, evidenceMapFromAny(step["arguments"]), field, action)
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "update_agent_config") {
			continue
		}
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		targets = completionVerificationAppendAgentConfigBindingTargets(targets, completionVerificationInvocationArguments(invocation), field, action)
	}
	return cleanStringSlice(dedupeStrings(targets))
}

func completionVerificationAppendAgentConfigBindingTargets(targets []string, source map[string]interface{}, field string, action string) []string {
	if len(source) == 0 {
		return targets
	}
	switch field {
	case "enabled_skill_ids":
		targets = append(targets, completionVerificationStringSlice(source["candidate_skill_id"])...)
		targets = append(targets, completionVerificationStringSlice(source["target_skill_id"])...)
		targets = append(targets, completionVerificationStringSlice(source["agent_skill_id"])...)
		switch action {
		case "bind":
			targets = append(targets, completionVerificationStringSlice(source["add_enabled_skill_ids"])...)
			targets = append(targets, completionVerificationStringSlice(source["enabled_skill_ids"])...)
		case "unbind":
			targets = append(targets, completionVerificationStringSlice(source["remove_enabled_skill_ids"])...)
		case "replace":
			targets = append(targets, completionVerificationStringSlice(source["enabled_skill_ids"])...)
		}
	case "knowledge_dataset_ids":
		targets = append(targets, completionVerificationStringSlice(source["candidate_knowledge_dataset_id"])...)
		targets = append(targets, completionVerificationStringSlice(source["knowledge_dataset_id"])...)
		targets = append(targets, completionVerificationStringSlice(source["dataset_id"])...)
		switch action {
		case "bind":
			targets = append(targets, completionVerificationStringSlice(source["add_knowledge_dataset_ids"])...)
			targets = append(targets, completionVerificationStringSlice(source["knowledge_dataset_ids"])...)
			targets = append(targets, completionVerificationStringSlice(source["dataset_ids"])...)
		case "unbind":
			targets = append(targets, completionVerificationStringSlice(source["remove_knowledge_dataset_ids"])...)
		case "replace":
			targets = append(targets, completionVerificationStringSlice(source["knowledge_dataset_ids"])...)
			targets = append(targets, completionVerificationStringSlice(source["dataset_ids"])...)
		}
	case "database_bindings":
		switch action {
		case "bind":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["add_database_bindings"])...)
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["database_bindings"])...)
		case "unbind":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["remove_database_bindings"])...)
		case "replace":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["database_bindings"])...)
		}
	case "workflow_bindings":
		switch action {
		case "bind":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["add_workflow_bindings"])...)
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["workflow_bindings"])...)
		case "unbind":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["remove_workflow_bindings"])...)
		case "replace":
			targets = append(targets, completionVerificationAgentConfigBindingIDs(source["workflow_bindings"])...)
		}
	}
	return targets
}

func completionVerificationInvocationArguments(invocation map[string]interface{}) map[string]interface{} {
	for _, key := range []string{"arguments", "args", "input", "tool_arguments"} {
		if args := evidenceMapFromAny(invocation[key]); len(args) > 0 {
			return args
		}
	}
	return nil
}

func completionVerificationLatestAgentConfigReadResult(evidence map[string]interface{}) (map[string]interface{}, bool) {
	if result, ok := completionVerificationLatestAgentConfigReadResultAfterUpdate(evidence); ok {
		return result, true
	}
	if result, ok := fastPathLatestSuccessfulAgentConfigReadResultAfterUpdate(evidence); ok {
		return result, true
	}
	if result, ok := fastPathLatestSuccessfulAgentReadResult(evidence, "get_agent_config"); ok {
		return result, true
	}
	invocations := completionVerificationEvidenceInvocations(evidence)
	for i := len(invocations) - 1; i >= 0; i-- {
		invocation := invocations[i]
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "get_agent_config") {
			continue
		}
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		result := evidenceMapFromAny(invocation["result"])
		if len(result) == 0 {
			result = evidenceMapFromAny(invocation["result_summary"])
		}
		if len(result) == 0 {
			continue
		}
		return result, true
	}
	if result, ok := completionVerificationLatestAgentConfigUpdateResultWithConfigEvidence(evidence); ok {
		return result, true
	}
	return nil, false
}

func completionVerificationLatestAgentConfigUpdateResultWithConfigEvidence(evidence map[string]interface{}) (map[string]interface{}, bool) {
	var latest map[string]interface{}
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])), "update_agent_config") {
			continue
		}
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		result := completionVerificationInvocationResult(invocation)
		if completionVerificationAgentConfigUpdateResultHasConfigEvidence(result) {
			latest = result
		} else {
			latest = nil
		}
	}
	if len(latest) == 0 {
		return nil, false
	}
	return latest, true
}

func completionVerificationHasAgentConfigUpdateResultEvidence(evidence map[string]interface{}) bool {
	_, ok := completionVerificationLatestAgentConfigUpdateResultWithConfigEvidence(evidence)
	return ok
}

func completionVerificationInvocationResult(invocation map[string]interface{}) map[string]interface{} {
	for _, key := range []string{"result", "result_summary", "tool_result"} {
		if result := evidenceMapFromAny(invocation[key]); len(result) > 0 {
			return result
		}
	}
	return nil
}

func completionVerificationAgentConfigUpdateResultHasConfigEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	fields := cleanStringSlice(append(
		completionVerificationCanonicalAgentConfigFields(result["updated_fields"]),
		completionVerificationCanonicalAgentConfigFields(result["satisfied_fields"])...,
	))
	if len(fields) == 0 {
		for _, field := range []string{
			"model", "system_prompt", "agent_memory_enabled", "file_upload_enabled",
			"home_title", "input_placeholder", "theme_color", "suggested_questions",
			"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings",
		} {
			if completionVerificationAgentConfigResultHasFieldEvidence(result, field) {
				return true
			}
		}
		return false
	}
	for _, field := range fields {
		if completionVerificationAgentConfigResultHasFieldEvidence(result, field) {
			return true
		}
	}
	return false
}

func completionVerificationAgentConfigResultHasFieldEvidence(result map[string]interface{}, field string) bool {
	if len(result) == 0 {
		return false
	}
	config := completionVerificationAgentConfigMap(result)
	hasInResultOrConfig := func(key string) bool {
		return completionVerificationMapHasKey(result, key) || completionVerificationMapHasKey(config, key)
	}
	switch completionVerificationCanonicalAgentConfigField(field) {
	case "model":
		return hasInResultOrConfig("model") || hasInResultOrConfig("model_name") ||
			hasInResultOrConfig("model_provider") || hasInResultOrConfig("provider")
	case "system_prompt", "agent_memory_enabled", "file_upload_enabled", "home_title", "input_placeholder", "theme_color", "suggested_questions":
		return hasInResultOrConfig(field)
	case "enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings":
		return completionVerificationAgentConfigResultHasCollectionFieldEvidence(result, field)
	default:
		return false
	}
}

func completionVerificationAgentConfigResultHasCollectionFieldEvidence(result map[string]interface{}, field string) bool {
	if len(result) == 0 {
		return false
	}
	sources := []map[string]interface{}{result}
	if config := evidenceMapFromAny(result["config"]); len(config) > 0 {
		sources = append(sources, config)
	}
	if agent := evidenceMapFromAny(result["agent"]); len(agent) > 0 {
		sources = append(sources, agent)
		if config := evidenceMapFromAny(agent["config"]); len(config) > 0 {
			sources = append(sources, config)
		}
	}
	keys := []string{field}
	switch completionVerificationCanonicalAgentConfigField(field) {
	case "enabled_skill_ids":
		keys = append(keys, "skill_ids", "agent_skill_ids", "enabled_skill_refs", "enabled_skills", "skills", "agent_skills")
	case "knowledge_dataset_ids":
		keys = append(keys, "dataset_ids", "knowledge_base_ids", "knowledge_bases")
	case "database_bindings":
		keys = append(keys, "database_table_ids", "table_ids")
	case "workflow_bindings":
		keys = append(keys, "workflow_ids")
	}
	for _, source := range sources {
		for _, key := range keys {
			if completionVerificationMapHasKey(source, key) {
				return true
			}
		}
	}
	for _, key := range []string{"binding_changes", "binding_final_states"} {
		for _, raw := range evidenceSliceFromAny(result[key]) {
			item := evidenceMapFromAny(raw)
			if len(item) == 0 {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(item["field"])), field) {
				return true
			}
		}
	}
	return false
}

func completionVerificationLatestAgentConfigReadResultAfterUpdate(evidence map[string]interface{}) (map[string]interface{}, bool) {
	scan := func(invocations []map[string]interface{}) (map[string]interface{}, bool) {
		seenUpdate := false
		var latest map[string]interface{}
		for _, invocation := range invocations {
			if !strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) {
				continue
			}
			toolName := strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"]))
			switch {
			case strings.EqualFold(toolName, "update_agent_config") && completionVerificationInvocationSucceeded(invocation):
				seenUpdate = true
				latest = nil
			case seenUpdate && strings.EqualFold(toolName, "get_agent_config") && completionVerificationInvocationSucceeded(invocation):
				result := evidenceMapFromAny(invocation["result"])
				if len(result) == 0 {
					result = evidenceMapFromAny(invocation["result_summary"])
				}
				if len(result) == 0 {
					result = evidenceMapFromAny(invocation["tool_result"])
				}
				if len(result) > 0 {
					latest = result
				}
			}
		}
		if len(latest) == 0 {
			return nil, false
		}
		return latest, true
	}
	if result, ok := scan(evidenceMapsFromAny(evidence["skill_invocations"])); ok {
		return result, true
	}
	if ledger := evidenceMapFromAny(evidence["execution_ledger"]); len(ledger) > 0 {
		if result, ok := scan(evidenceMapsFromAny(ledger["skill_invocations"])); ok {
			return result, true
		}
		if result, ok := scan(evidenceMapsFromAny(evidenceMapFromAny(ledger["summary"])["skill_invocations"])); ok {
			return result, true
		}
	}
	return nil, false
}

func completionVerificationAgentConfigCollectionIDs(result map[string]interface{}, collectionKey string) []string {
	if len(result) == 0 {
		return nil
	}
	out := []string{}
	appendFrom := func(value interface{}) {
		for _, item := range completionVerificationStringSlice(value) {
			out = append(out, item)
		}
		for _, raw := range evidenceSliceFromAny(value) {
			item := evidenceMapFromAny(raw)
			if len(item) == 0 {
				continue
			}
			out = append(out, completionVerificationAgentConfigBindingIDsForField(collectionKey, item)...)
		}
	}
	sources := []map[string]interface{}{result}
	if config := evidenceMapFromAny(result["config"]); len(config) > 0 {
		sources = append(sources, config)
	}
	if agent := evidenceMapFromAny(result["agent"]); len(agent) > 0 {
		sources = append(sources, agent)
		if config := evidenceMapFromAny(agent["config"]); len(config) > 0 {
			sources = append(sources, config)
		}
	}
	for _, source := range sources {
		appendFrom(source[collectionKey])
		switch collectionKey {
		case "enabled_skill_ids":
			appendFrom(source["skill_ids"])
			appendFrom(source["agent_skill_ids"])
			appendFrom(source["enabled_skill_refs"])
			appendFrom(source["enabled_skills"])
			appendFrom(source["skills"])
			appendFrom(source["agent_skills"])
		case "knowledge_dataset_ids":
			appendFrom(source["dataset_ids"])
			appendFrom(source["knowledge_base_ids"])
			appendFrom(source["knowledge_bases"])
		case "database_bindings":
			appendFrom(source["database_table_ids"])
			appendFrom(source["table_ids"])
			out = append(out, completionVerificationAgentConfigBindingIDsForField(collectionKey, source["database_bindings"])...)
		case "workflow_bindings":
			appendFrom(source["workflow_ids"])
			out = append(out, completionVerificationAgentConfigBindingIDsForField(collectionKey, source["workflow_bindings"])...)
		}
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationAgentConfigMap(result map[string]interface{}) map[string]interface{} {
	if len(result) == 0 {
		return nil
	}
	if config := evidenceMapFromAny(result["config"]); len(config) > 0 {
		return config
	}
	agent := evidenceMapFromAny(result["agent"])
	if config := evidenceMapFromAny(agent["config"]); len(config) > 0 {
		return config
	}
	return result
}

func completionVerificationAgentConfigBindingIDs(value interface{}) []string {
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			var decoded interface{}
			if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
				return completionVerificationAgentConfigBindingIDs(decoded)
			}
		}
	}
	out := []string{}
	out = append(out, completionVerificationStringSlice(value)...)
	for _, raw := range evidenceSliceFromAny(value) {
		item := evidenceMapFromAny(raw)
		if len(item) == 0 {
			continue
		}
		out = append(out,
			completionVerificationStringSlice(item["id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["binding_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["resource_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["knowledge_dataset_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["dataset_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["database_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["data_source_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["table_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["table_ids"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["workflow_id"])...,
		)
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationAgentConfigBindingIDsForField(field string, value interface{}) []string {
	field = strings.TrimSpace(field)
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			var decoded interface{}
			if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
				return completionVerificationAgentConfigBindingIDsForField(field, decoded)
			}
		}
	}
	out := completionVerificationStringSlice(value)
	for _, raw := range evidenceSliceFromAny(value) {
		item := evidenceMapFromAny(raw)
		if len(item) == 0 {
			continue
		}
		out = append(out, completionVerificationAgentConfigBindingIDsForField(field, item)...)
	}
	item := evidenceMapFromAny(value)
	if len(item) == 0 {
		return cleanStringSlice(dedupeStrings(out))
	}
	switch field {
	case "enabled_skill_ids":
		out = append(out,
			completionVerificationStringSlice(item["skill_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["name"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["label"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["title"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["display_name"])...,
		)
	case "knowledge_dataset_ids":
		out = append(out,
			completionVerificationStringSlice(item["knowledge_dataset_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["dataset_id"])...,
		)
		if len(out) == 0 {
			out = append(out,
				completionVerificationStringSlice(item["id"])...,
			)
		}
	case "database_bindings":
		out = append(out,
			completionVerificationStringSlice(item["data_source_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["database_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["table_id"])...,
		)
		out = append(out,
			completionVerificationStringSlice(item["table_ids"])...,
		)
	case "workflow_bindings":
		out = append(out,
			completionVerificationStringSlice(item["workflow_id"])...,
		)
		if len(out) == 0 {
			out = append(out,
				completionVerificationStringSlice(item["id"])...,
			)
		}
	default:
		out = append(out, completionVerificationAgentConfigBindingIDs(value)...)
	}
	return cleanStringSlice(dedupeStrings(out))
}

func completionVerificationMapHasKey(source map[string]interface{}, key string) bool {
	if len(source) == 0 {
		return false
	}
	_, ok := source[key]
	return ok
}

func completionVerificationBoolFromAny(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(typed))
		switch trimmed {
		case "true", "1", "yes", "enabled", "on":
			return true, true
		case "false", "0", "no", "disabled", "off":
			return false, true
		default:
			return false, false
		}
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case float64:
		return typed != 0, true
	default:
		return false, false
	}
}

func firstNonEmptyDisplay(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func completionVerificationMergeAgentConfigBindingExpectations(expectations []completionVerificationAgentConfigBindingExpectation) []completionVerificationAgentConfigBindingExpectation {
	if len(expectations) == 0 {
		return nil
	}
	byKey := map[string]int{}
	out := make([]completionVerificationAgentConfigBindingExpectation, 0, len(expectations))
	for _, expectation := range expectations {
		if expectation.Field == "" || expectation.Action == "" {
			continue
		}
		key := expectation.Field + "\x00" + expectation.Action
		if idx, ok := byKey[key]; ok {
			out[idx].Targets = cleanStringSlice(dedupeStrings(append(out[idx].Targets, expectation.Targets...)))
			continue
		}
		expectation.Targets = cleanStringSlice(dedupeStrings(expectation.Targets))
		byKey[key] = len(out)
		out = append(out, expectation)
	}
	return out
}

func completionVerificationStringSliceContainsFold(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func completionVerificationStringSlicesEqual(want []string, actual []string) bool {
	want = cleanStringSlice(want)
	actual = cleanStringSlice(actual)
	if len(want) != len(actual) {
		return false
	}
	for idx := range want {
		if strings.TrimSpace(want[idx]) != strings.TrimSpace(actual[idx]) {
			return false
		}
	}
	return true
}

func completionVerificationAgentConfigTextMatches(want string, actual string) bool {
	want = strings.TrimSpace(want)
	actual = strings.TrimSpace(actual)
	if want == "" {
		return true
	}
	if actual == want {
		return true
	}
	if completionVerificationTextEvidenceLooksTruncated(want, actual) &&
		completionVerificationCommonPrefixRuneLen(want, actual) >= 20 {
		return true
	}
	for _, prefix := range []string{
		completionVerificationTrimTextEvidencePrefix(want),
		completionVerificationTrimTextEvidencePrefix(actual),
	} {
		if len([]rune(prefix)) < 20 {
			continue
		}
		if strings.HasPrefix(want, prefix) && strings.HasPrefix(actual, prefix) {
			return true
		}
	}
	return false
}

func completionVerificationTrimTextEvidencePrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, ".…\uFFFD?")
	return strings.TrimSpace(value)
}

func completionVerificationTextEvidenceLooksTruncated(a string, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	hasMarker := func(value string) bool {
		return strings.HasSuffix(value, "...") ||
			strings.HasSuffix(value, "..") ||
			strings.HasSuffix(value, "…") ||
			strings.HasSuffix(value, "\uFFFD") ||
			strings.HasSuffix(value, "?")
	}
	if hasMarker(a) || hasMarker(b) {
		return true
	}
	shorter, longer := len([]rune(a)), len([]rune(b))
	if shorter > longer {
		shorter, longer = longer, shorter
	}
	return shorter >= 20 && longer >= shorter*2
}

func completionVerificationCommonPrefixRuneLen(a string, b string) int {
	ar := []rune(strings.TrimSpace(a))
	br := []rune(strings.TrimSpace(b))
	limit := len(ar)
	if len(br) < limit {
		limit = len(br)
	}
	for i := 0; i < limit; i++ {
		if ar[i] != br[i] {
			return i
		}
	}
	return limit
}

func completionVerificationHasFailedEvidence(evidence map[string]interface{}) bool {
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if completionVerificationInvocationFailed(invocation) {
			return true
		}
	}
	return false
}

func completionVerificationHasFailedEvidenceForPlanStep(evidence map[string]interface{}, failedStepLabel string) bool {
	invocations := completionVerificationEvidenceInvocations(evidence)
	if len(invocations) == 0 {
		return false
	}
	failedStepLabel = strings.ToLower(strings.TrimSpace(failedStepLabel))
	for _, invocation := range invocations {
		if !completionVerificationInvocationFailed(invocation) {
			continue
		}
		if failedStepLabel == "" {
			return true
		}
		invocationLabel := strings.ToLower(strings.TrimSpace(completionVerificationInvocationLabel(invocation)))
		if invocationLabel != "" && invocationLabel == failedStepLabel {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["action_type"])), "route_navigation") &&
			(strings.HasPrefix(failedStepLabel, "route:") || strings.Contains(failedStepLabel, "/console/") || !strings.Contains(failedStepLabel, "/")) {
			return true
		}
	}
	return false
}

func completionVerificationHasSuccessfulEvidence(evidence map[string]interface{}) bool {
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if completionVerificationInvocationSucceeded(invocation) {
			return true
		}
	}
	return len(evidenceSliceFromAny(evidence["generated_files"])) > 0
}

func completionVerificationInvocationSucceeded(invocation map[string]interface{}) bool {
	result := evidenceMapFromAny(invocation["result"])
	if len(result) == 0 {
		result = evidenceMapFromAny(invocation["result_summary"])
	}
	if completionVerificationOperationGroupHasFailedItems(result) {
		return false
	}
	if len(result) > 0 {
		resultStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(result["status"], result["result_status"]))))
		switch resultStatus {
		case "error", "failed", "partial_failed", "partially_failed":
			return false
		}
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["status"])))
	switch status {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	}
	if len(result) == 0 {
		return false
	}
	resultStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(result["status"], result["result_status"]))))
	switch resultStatus {
	case "success", "succeeded", "completed", "allowed", "approved":
		return true
	default:
		return false
	}
}

func completionVerificationDecisionIsPlanOnly(evidence map[string]interface{}, decision completionVerificationDecision) bool {
	texts := []string{decision.Reason, decision.NextActionHint, decision.FinalAnswerGuidance}
	texts = append(texts, decision.MissingSteps...)
	if !completionVerificationAnyPlanOnlyText(texts...) {
		return false
	}
	for _, claim := range cleanStringSlice(decision.UnsupportedClaims) {
		if !completionVerificationPlanOnlyText(claim) {
			return false
		}
	}
	for _, step := range cleanStringSlice(decision.MissingSteps) {
		if completionVerificationPlanOnlyText(step) {
			continue
		}
		if completionVerificationMissingStepHasSuccessfulEvidence(evidence, step) {
			continue
		}
		return false
	}
	return true
}

func completionVerificationAnyPlanOnlyText(values ...string) bool {
	for _, value := range values {
		if completionVerificationPlanOnlyText(value) {
			return true
		}
	}
	return false
}

func completionVerificationPlanOnlyText(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"operation_plan",
		"operation plan",
		"turn_strategy",
		"turn strategy",
		"pending executable",
		"pending step",
		"plan step",
		"planned step",
		"required_next_tool",
		"still pending",
		"incomplete plan",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func completionVerificationCandidateAnswerLeaksInternalPlan(candidateAnswer string) bool {
	lower := strings.ToLower(strings.TrimSpace(candidateAnswer))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"operation_plan",
		"operation plan",
		"turn_strategy",
		"turn strategy",
		"required_next_tool",
		"pending executable",
		"pending step",
		"system prompt",
		"system message",
		"hidden strategy",
		"strategy json",
		"internal plan",
		"internal strategy",
		"\u5185\u90e8\u8ba1\u5212",
		"\u5185\u90e8\u7b56\u7565",
		"\u9690\u85cf\u8ba1\u5212",
		"\u9690\u85cf\u7b56\u7565",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return completionVerificationAnswerMentionsInternalSystemPrompt(lower)
}

func completionVerificationAnswerMentionsInternalSystemPrompt(lower string) bool {
	if strings.Contains(lower, "system prompt") {
		return true
	}
	marker := "\u7cfb\u7edf\u63d0\u793a"
	for start := 0; ; {
		idx := strings.Index(lower[start:], marker)
		if idx < 0 {
			return false
		}
		absolute := start + idx
		if !completionVerificationIsUserVisibleChineseSystemPromptField(lower[absolute:]) {
			return true
		}
		start = absolute + len(marker)
	}
}

func completionVerificationIsUserVisibleChineseSystemPromptField(text string) bool {
	for _, allowedPrefix := range []string{
		"\u7cfb\u7edf\u63d0\u793a\u8bcd",
		"\u7cfb\u7edf\u63d0\u793a\u8bed",
		"\u7cfb\u7edf\u63d0\u793a\u5b57\u6bb5",
		"\u7cfb\u7edf\u63d0\u793a\u914d\u7f6e",
	} {
		if strings.HasPrefix(text, allowedPrefix) {
			return true
		}
	}
	return false
}

func completionVerificationInternalPlanLeakDecision(evidence map[string]interface{}) completionVerificationDecision {
	guidance := "Rewrite the final answer using only user-visible operation outcome or blocker evidence. Do not mention system prompts, hidden plans, operation_plan, turn_strategy, required_next_tool, pending steps, protocol details, or internal strategy."
	if completionVerificationUserRequestedChinese(evidence) {
		guidance = "\u8bf7\u91cd\u5199\u6700\u7ec8\u7b54\u590d\uff1a\u53ea\u8bf4\u660e\u7528\u6237\u80fd\u7406\u89e3\u7684\u64cd\u4f5c\u7ed3\u679c\u6216\u963b\u585e\u539f\u56e0\uff0c\u4e0d\u8981\u63d0\u5230\u7cfb\u7edf\u63d0\u793a\u3001operation_plan\u3001turn_strategy\u3001required_next_tool\u3001pending step\u3001\u5185\u90e8\u8ba1\u5212\u6216\u5de5\u5177\u534f\u8bae\u3002"
	}
	return completionVerificationDecision{
		Status:              completionVerificationStatusNeedsAction,
		Reason:              "candidate answer exposed internal planning or system instruction wording",
		UnsupportedClaims:   []string{"internal planning or system instruction wording leaked to the user"},
		FinalAnswerGuidance: guidance,
	}
}

func completionVerificationHasUnsatisfiedManagedFileSave(evidence map[string]interface{}) bool {
	if len(evidence) == 0 {
		return false
	}
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 || !completionVerificationPlanMentionsPendingManagedFileSave(plan) {
		return false
	}
	return !completionVerificationHasManagedFileSaveEvidence(evidence)
}

func completionVerificationPlanMentionsPendingManagedFileSave(plan map[string]interface{}) bool {
	pending := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["pending_next_action"])))
	if completionVerificationTextMentionsManagedFileSave(pending) {
		return true
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		id := strings.TrimSpace(evidenceStringFromAny(step["id"]))
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
		if status == "" && id != "" {
			status = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[id])))
		}
		switch status {
		case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
			continue
		}
		skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["skill_id"])))
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["tool_name"])))
		if skillID == skills.SkillFileManager && toolName == "save_file_to_management" {
			return true
		}
		if completionVerificationTextMentionsManagedFileSave(id) ||
			completionVerificationTextMentionsManagedFileSave(evidenceStringFromAny(step["title"])) {
			return true
		}
	}
	return false
}

func completionVerificationTextMentionsManagedFileSave(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "save_file_to_management") ||
		strings.Contains(lower, "save_remaining_generated_files") ||
		(strings.Contains(lower, "file_management") && strings.Contains(lower, "save")) ||
		(strings.Contains(lower, "managed_file") && strings.Contains(lower, "save"))
}

func completionVerificationHasManagedFileSaveEvidence(evidence map[string]interface{}) bool {
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])))
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])))
		if skillID == skills.SkillFileManager && toolName == "save_file_to_management" {
			return true
		}
		result := evidenceMapFromAny(invocation["result"])
		if len(result) == 0 {
			result = evidenceMapFromAny(invocation["result_summary"])
		}
		if completionVerificationResultIsManagedFileSave(result) {
			return true
		}
	}
	for _, file := range evidenceMapsFromAny(evidence["generated_files"]) {
		if completionVerificationGeneratedFileIsManaged(file) {
			return true
		}
	}
	if ledger := evidenceMapFromAny(evidence["execution_ledger"]); len(ledger) > 0 {
		for _, file := range evidenceMapsFromAny(ledger["generated_files"]) {
			if completionVerificationGeneratedFileIsManaged(file) {
				return true
			}
		}
	}
	return false
}

func completionVerificationResultIsManagedFileSave(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(result["target"])))
	if target != "" && target != "managed_file" && target != "file_management" {
		return false
	}
	return strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(
		result["managed_file_id"],
		result["upload_file_id"],
		result["file_id"],
		result["id"],
	))) != ""
}

func completionVerificationGeneratedFileIsManaged(file map[string]interface{}) bool {
	if len(file) == 0 {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(file["target"])))
	if target == "managed_file" || target == "file_management" {
		return true
	}
	return strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(
		file["managed_file_id"],
		file["upload_file_id"],
		file["managed_filename"],
	))) != ""
}

func completionVerificationMissingStepHasSuccessfulEvidence(evidence map[string]interface{}, missingStep string) bool {
	normalized := strings.ToLower(strings.TrimSpace(missingStep))
	if normalized == "" {
		return true
	}
	for _, invocation := range completionVerificationEvidenceInvocations(evidence) {
		if !completionVerificationInvocationSucceeded(invocation) {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(completionVerificationInvocationLabel(invocation)))
		if label != "" && strings.Contains(normalized, label) {
			return true
		}
		skillID := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"])))
		toolName := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"])))
		if skillID != "" && toolName != "" && strings.Contains(normalized, skillID) && strings.Contains(normalized, toolName) {
			return true
		}
		if strings.HasPrefix(normalized, "route:") &&
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(invocation["action_type"])), "route_navigation") {
			return true
		}
	}
	return false
}

func completionVerificationFailedOperationPlanStepLabel(evidence map[string]interface{}) (string, bool) {
	if len(evidence) == 0 {
		return "", false
	}
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["status"])))
	if status != "failed" {
		return "", false
	}
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		stepState := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
		if stepState != "failed" {
			continue
		}
		label := completionVerificationPlanStepLabel(step)
		if label != "" {
			return label, true
		}
		break
	}
	return "", true
}

func completionVerificationFailedPlanFinalAnswer(failedStepLabel string, failureDetail string) string {
	failedStepLabel = strings.TrimSpace(failedStepLabel)
	failureDetail = strings.TrimSpace(failureDetail)
	lines := []string{completionVerificationFallbackFailed}
	if failedStepLabel == "" {
		lines = append(lines, "\u6267\u884c\u8ba1\u5212\u5df2\u6807\u8bb0\u4e3a\u5931\u8d25\u3002")
	} else {
		lines = append(lines, "\u5931\u8d25\u6b65\u9aa4\uff1a"+failedStepLabel+"\u3002")
	}
	if failureDetail != "" {
		lines = append(lines, "\u5931\u8d25\u539f\u56e0\uff1a"+failureDetail+"\u3002")
	}
	return strings.Join(lines, "\n")
}

func completionVerificationFailureDetailForStep(evidence map[string]interface{}, failedStepLabel string) string {
	failedStepLabel = strings.ToLower(strings.TrimSpace(failedStepLabel))
	if len(evidence) == 0 {
		return ""
	}
	invocations := completionVerificationEvidenceInvocations(evidence)
	for _, invocation := range invocations {
		if !completionVerificationInvocationFailed(invocation) {
			continue
		}
		if failedStepLabel != "" && !strings.EqualFold(completionVerificationInvocationLabel(invocation), failedStepLabel) {
			continue
		}
		if detail := completionVerificationInvocationFailureDetail(invocation); detail != "" {
			return detail
		}
	}
	if failedStepLabel == "" {
		return ""
	}
	for _, invocation := range invocations {
		if !completionVerificationInvocationFailed(invocation) {
			continue
		}
		if detail := completionVerificationInvocationFailureDetail(invocation); detail != "" {
			return detail
		}
	}
	return ""
}

func completionVerificationEvidenceInvocations(evidence map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	out = append(out, evidenceMapsFromAny(evidence["skill_invocations"])...)
	out = append(out, evidenceMapsFromAny(evidence["client_actions"])...)
	out = append(out, evidenceMapsFromAny(evidence["tool_governance"])...)
	if operationSummary := evidenceMapFromAny(evidence["operation_result_summary"]); len(operationSummary) > 0 {
		out = append(out, completionVerificationOperationSummaryInvocations(operationSummary)...)
	}
	if summary := evidenceMapFromAny(evidence["execution_summary"]); len(summary) > 0 {
		out = append(out, evidenceMapsFromAny(summary["tool_results"])...)
		out = append(out, evidenceMapsFromAny(summary["client_actions"])...)
		if operationSummary := evidenceMapFromAny(summary["operation_result_summary"]); len(operationSummary) > 0 {
			out = append(out, completionVerificationOperationSummaryInvocations(operationSummary)...)
		}
	}
	if ledger := evidenceMapFromAny(evidence["execution_ledger"]); len(ledger) > 0 {
		out = append(out, evidenceMapsFromAny(ledger["skill_invocations"])...)
		out = append(out, evidenceMapsFromAny(ledger["client_actions"])...)
		out = append(out, evidenceMapsFromAny(ledger["tool_governance"])...)
		if operationSummary := evidenceMapFromAny(ledger["operation_result_summary"]); len(operationSummary) > 0 {
			out = append(out, completionVerificationOperationSummaryInvocations(operationSummary)...)
		}
		if summary := evidenceMapFromAny(ledger["summary"]); len(summary) > 0 {
			out = append(out, evidenceMapsFromAny(summary["tool_results"])...)
			out = append(out, evidenceMapsFromAny(summary["client_actions"])...)
			if operationSummary := evidenceMapFromAny(summary["operation_result_summary"]); len(operationSummary) > 0 {
				out = append(out, completionVerificationOperationSummaryInvocations(operationSummary)...)
			}
		}
	}
	return out
}

func completionVerificationOperationSummaryInvocations(summary map[string]interface{}) []map[string]interface{} {
	if len(summary) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, 2)
	if latest := evidenceMapFromAny(summary["latest_tool_result"]); len(latest) > 0 {
		out = append(out, latest)
	}
	if group := evidenceMapFromAny(summary["operation_group"]); len(group) > 0 {
		invocation := map[string]interface{}{
			"kind":      "operation_result_summary",
			"status":    firstNonEmptyEvidence(summary["status"], group["status"]),
			"skill_id":  summary["skill_id"],
			"tool_name": summary["tool_name"],
			"result_summary": map[string]interface{}{
				"operation_group": group,
			},
		}
		out = append(out, invocation)
	}
	return out
}

func completionVerificationInvocationFailed(invocation map[string]interface{}) bool {
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(invocation["status"])))
	if status == "error" || status == "failed" || status == "blocked" || status == "rejected" {
		return true
	}
	result := evidenceMapFromAny(invocation["result"])
	if len(result) == 0 {
		result = evidenceMapFromAny(invocation["result_summary"])
		if len(result) == 0 {
			return false
		}
	}
	if completionVerificationOperationGroupHasFailedItems(result) {
		return true
	}
	resultStatus := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(result["status"], result["result_status"]))))
	if resultStatus == "error" || resultStatus == "failed" || resultStatus == "partial_failed" || resultStatus == "partially_failed" {
		return true
	}
	return strings.TrimSpace(evidenceStringFromAny(result["error"])) != "" ||
		strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(result["content_status"])), "error")
}

func completionVerificationInvocationLabel(invocation map[string]interface{}) string {
	skillID := strings.TrimSpace(evidenceStringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(evidenceStringFromAny(invocation["tool_name"]))
	if skillID != "" && toolName != "" {
		return skillID + "/" + toolName
	}
	if id := strings.TrimSpace(evidenceStringFromAny(invocation["id"])); id != "" {
		return id
	}
	return strings.TrimSpace(evidenceStringFromAny(invocation["runtime_id"]))
}

func completionVerificationInvocationFailureDetail(invocation map[string]interface{}) string {
	result := evidenceMapFromAny(invocation["result"])
	resultSummary := evidenceMapFromAny(invocation["result_summary"])
	for _, value := range []interface{}{
		invocation["error"],
		result["error"],
		result["content_error"],
		result["error_code"],
		result["message"],
		resultSummary["error"],
		resultSummary["content_error"],
		resultSummary["error_code"],
		resultSummary["message"],
		completionVerificationOperationGroupFailureDetail(result),
		completionVerificationOperationGroupFailureDetail(resultSummary),
		invocation["message"],
	} {
		if detail := trimCompletionVerificationDetail(evidenceStringFromAny(value)); detail != "" {
			return detail
		}
	}
	return ""
}

func completionVerificationOperationGroupHasFailedItems(result map[string]interface{}) bool {
	return completionVerificationOperationGroupFailureDetail(result) != ""
}

func completionVerificationOperationGroupFailureDetail(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	if count := completionVerificationNumericEvidence(result["failed_count"]); count > 0 {
		return fmt.Sprintf("%d item(s) failed", count)
	}
	items := evidenceMapsFromAny(result["item_results"])
	if len(items) == 0 {
		group := evidenceMapFromAny(result["operation_group"])
		if count := completionVerificationNumericEvidence(group["failed_count"]); count > 0 {
			return fmt.Sprintf("%d item(s) failed", count)
		}
		items = evidenceMapsFromAny(group["item_results"])
	}
	for _, item := range items {
		status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(item["status"])))
		switch status {
		case "failed", "error", "blocked", "rejected":
			name := strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(
				item["agent_name"],
				item["name"],
				item["asset_name"],
				item["resource_name"],
				item["id"],
				item["agent_id"],
			)))
			reason := strings.TrimSpace(evidenceStringFromAny(firstNonEmptyEvidence(item["error"], item["message"], item["reason"])))
			if name != "" && reason != "" {
				return name + ": " + reason
			}
			if name != "" {
				return name + " failed"
			}
			if reason != "" {
				return reason
			}
			return "batch item failed"
		}
	}
	return ""
}

func completionVerificationNumericEvidence(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
		floatValue, err := typed.Float64()
		if err == nil {
			return int(floatValue)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func trimCompletionVerificationDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240]) + "..."
	}
	return value
}

func firstNonEmptyEvidence(values ...interface{}) interface{} {
	for _, value := range values {
		if strings.TrimSpace(evidenceStringFromAny(value)) != "" {
			return value
		}
	}
	return nil
}

func completionVerificationPendingExecutablePlanStep(evidence map[string]interface{}) (map[string]interface{}, bool) {
	if len(evidence) == 0 {
		return nil, false
	}
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return nil, false
	}
	modelDecidesTools := operationPlanModelDecidesTools(plan)
	modelDecidesStep, hasModelDecidesStep := fastPathModelDecidesPendingAgentWorkStep(plan)
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["status"])))
	switch status {
	case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
		return nil, false
	}
	stepStatus := evidenceMapFromAny(plan["step_status"])
	for _, raw := range evidenceSliceFromAny(plan["steps"]) {
		step := evidenceMapFromAny(raw)
		if len(step) == 0 {
			continue
		}
		skillID := strings.TrimSpace(evidenceStringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(evidenceStringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" {
			continue
		}
		id := strings.TrimSpace(evidenceStringFromAny(step["id"]))
		stepState := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(step["status"])))
		if stepState == "" && id != "" {
			stepState = strings.ToLower(strings.TrimSpace(evidenceStringFromAny(stepStatus[id])))
		}
		switch stepState {
		case "completed", "complete", "success", "succeeded", "failed", "error", "skipped", "not_applicable":
			continue
		}
		if modelDecidesTools {
			if !hasModelDecidesStep {
				continue
			}
			if fastPathPlanStepAction(step) != fastPathPlanStepAction(modelDecidesStep) {
				continue
			}
		}
		return step, true
	}
	return nil, false
}

func completionVerificationPlanStepLabel(step map[string]interface{}) string {
	skillID := strings.TrimSpace(evidenceStringFromAny(step["skill_id"]))
	toolName := strings.TrimSpace(evidenceStringFromAny(step["tool_name"]))
	if skillID != "" && toolName != "" {
		return skillID + "/" + toolName
	}
	if title := strings.TrimSpace(evidenceStringFromAny(step["title"])); title != "" {
		return title
	}
	return strings.TrimSpace(evidenceStringFromAny(step["id"]))
}

func parseCompletionVerificationDecision(raw string) (completionVerificationDecision, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return completionVerificationDecision{}, fmt.Errorf("completion verifier returned empty response")
	}
	raw = trimJSONCodeFence(raw)
	var decision completionVerificationDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return completionVerificationDecision{}, fmt.Errorf("parse completion verifier response: %w", err)
	}
	if decision.normalizedStatus() == "" {
		return completionVerificationDecision{}, fmt.Errorf("completion verifier status is empty")
	}
	return decision, nil
}

func trimJSONCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```JSON")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

func completionVerificationSystemMessage(decision completionVerificationDecision, candidateAnswer string, retry int, modelDecidesArgs ...bool) adapter.Message {
	modelDecidesTools := false
	if len(modelDecidesArgs) > 0 {
		modelDecidesTools = modelDecidesArgs[0]
	}
	displayDecision := decision
	if modelDecidesTools {
		displayDecision = completionVerificationDecisionForModelDecidesFeedback(decision)
	}
	lines := []string{
		"Runtime completion verification feedback:",
		"The previous candidate final answer did not pass post-verification.",
	}
	if reason := strings.TrimSpace(displayDecision.Reason); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if strings.EqualFold(strings.TrimSpace(displayDecision.LanguageHint), "zh-Hans") {
		lines = append(lines, "Language: The user's original request is Chinese. Continue in Chinese and do not answer in English unless the user explicitly asks.")
	}
	if missing := strings.Join(cleanStringSlice(displayDecision.MissingSteps), ", "); missing != "" {
		lines = append(lines, "Missing steps: "+missing)
	}
	if claims := strings.Join(cleanStringSlice(displayDecision.UnsupportedClaims), ", "); claims != "" {
		lines = append(lines, "Unsupported claims: "+claims)
	}
	if hint := strings.TrimSpace(displayDecision.NextActionHint); hint != "" {
		lines = append(lines, "Next action hint: "+hint)
	}
	if guidance := strings.TrimSpace(displayDecision.FinalAnswerGuidance); guidance != "" {
		lines = append(lines, "Final answer guidance: "+guidance)
	}
	lines = append(lines, completionVerificationExecutableActionFeedback(decision, modelDecidesTools)...)
	lines = append(lines,
		fmt.Sprintf("Post-verification retry %d of %d.", retry, defaultMaxCompletionVerificationRetries),
		"Continue only if the next action is safe and supported by the available tools. If the same tool with the same arguments already failed, do not repeat it; answer truthfully from the failure.",
	)
	if text := strings.TrimSpace(candidateAnswer); text != "" {
		lines = append(lines, "Candidate answer:\n"+text)
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func completionVerificationDecisionForModelDecidesFeedback(decision completionVerificationDecision) completionVerificationDecision {
	decision.Reason = completionVerificationModelDecidesPublicText(decision.Reason)
	decision.MissingSteps = completionVerificationModelDecidesPublicTexts(decision.MissingSteps)
	decision.UnsupportedClaims = completionVerificationModelDecidesPublicTexts(decision.UnsupportedClaims)
	decision.NextActionHint = completionVerificationModelDecidesPublicText(decision.NextActionHint)
	decision.FinalAnswerGuidance = completionVerificationModelDecidesPublicText(decision.FinalAnswerGuidance)
	return decision
}

func completionVerificationModelDecidesPublicTexts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = completionVerificationModelDecidesPublicText(value); value != "" {
			out = append(out, value)
		}
	}
	return dedupeStrings(out)
}

func completionVerificationModelDecidesPublicText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacements := []struct {
		old string
		new string
	}{
		{"agent-management/update_agent_config", "Agent configuration update evidence"},
		{"agent-management/get_agent_config", "fresh Agent configuration read evidence"},
		{"file-manager/save_file_to_management", "file management save evidence"},
		{"file-manager/delete_file", "file deletion evidence"},
		{"update_agent_config", "Agent configuration update evidence"},
		{"get_agent_config", "fresh Agent configuration read evidence"},
		{"save_file_to_management", "file management save evidence"},
		{"delete_file", "file deletion evidence"},
		{"Delete resolved file", "file deletion evidence"},
		{"delete resolved file", "file deletion evidence"},
	}
	for _, replacement := range replacements {
		value = strings.ReplaceAll(value, replacement.old, replacement.new)
	}
	return strings.TrimSpace(value)
}

func completionVerificationExecutableActionFeedback(decision completionVerificationDecision, modelDecidesTools bool) []string {
	if modelDecidesTools {
		return completionVerificationModelDecidesExecutableActionFeedback(decision)
	}
	text := strings.ToLower(strings.Join(append(append([]string{}, decision.MissingSteps...), decision.NextActionHint, decision.FinalAnswerGuidance, decision.Reason), "\n"))
	if skillID, toolName, ok := completionVerificationRequiredSkillTool(decision); ok {
		switch {
		case strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "update_agent_config"):
			return []string{
				"Required next tool: call agent-management/update_agent_config for the remaining requested Agent configuration changes.",
				"The next business tool call must be update_agent_config; do not call get_agent_config before this missing update_agent_config call succeeds.",
				"If one part of the Agent update already succeeded, preserve that evidence and update only the missing fields from the user's original request.",
				"After update_agent_config succeeds, call agent-management/get_agent_config to verify the refreshed Agent configuration before the final answer.",
				"Do not produce another final answer until the requested Agent configuration update succeeds, fails, or is rejected by governance.",
			}
		case strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "get_agent_config"):
			return []string{
				"Required next tool: call agent-management/get_agent_config to verify the current Agent configuration after the update.",
				"Use the fresh configuration result as the source of truth for the final answer.",
				"Do not produce another final answer until get_agent_config succeeds or fails.",
			}
		}
	}
	switch {
	case strings.Contains(text, "file-manager/delete_file") ||
		strings.Contains(text, "delete_file") ||
		strings.Contains(text, "delete resolved file"):
		return []string{
			"Required next tool: call file-manager/delete_file with the resolved file_id from current page context, resolved targets, or operation plan evidence.",
			"Tool governance owns the approval card; do not ask for a separate natural-language confirmation before calling the governed delete tool.",
			"Do not produce another final answer until file-manager/delete_file succeeds, fails, or is rejected by governance.",
		}
	case strings.Contains(text, "file-manager/save_file_to_management") ||
		strings.Contains(text, "save_file_to_management"):
		return []string{
			"Required next tool: call file-manager/save_file_to_management with the already generated artifact or supplied URL.",
			"Do not regenerate an existing artifact unless the prior generation failed or the user requested different content.",
			"Do not produce another final answer until file-manager/save_file_to_management succeeds, fails, or is rejected by governance.",
		}
	default:
		return nil
	}
}

func completionVerificationModelDecidesExecutableActionFeedback(decision completionVerificationDecision) []string {
	if skillID, toolName, ok := completionVerificationRequiredSkillTool(decision); ok {
		switch {
		case strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "update_agent_config"):
			return []string{
				"The remaining user-requested Agent configuration changes still need successful tool evidence.",
				"Choose the appropriate available Agent management capability from the current tool schemas, apply only the missing concrete target values from the user's request and latest evidence, then verify the refreshed configuration before the final answer.",
				"Do not produce another final answer until the requested Agent configuration update succeeds, fails, or is rejected by governance.",
			}
		case strings.EqualFold(skillID, skills.SkillAgentManagement) && strings.EqualFold(toolName, "get_agent_config"):
			return []string{
				"Fresh Agent configuration verification evidence is still missing.",
				"Choose the appropriate available read or observation capability to verify the current Agent configuration, then use that fresh result as the source of truth for the final answer.",
				"Do not produce another final answer until the verification succeeds or fails.",
			}
		case strings.EqualFold(skillID, skills.SkillFileManager) && strings.EqualFold(toolName, "delete_file"):
			return []string{
				"The requested file deletion still needs matching successful tool evidence.",
				"Use the available file-management capability only after resolving the exact target from current page context, prior tool results, or turn state. Governance owns any required approval card; do not ask for a separate natural-language confirmation.",
				"Do not produce another final answer until the file deletion succeeds, fails, or is rejected by governance.",
			}
		case strings.EqualFold(skillID, skills.SkillFileManager) && strings.EqualFold(toolName, "save_file_to_management"):
			return []string{
				"The requested save into file management still needs matching successful tool evidence.",
				"Use the already generated artifact or supplied URL when available; do not regenerate an existing artifact unless the prior generation failed or the user requested different content.",
				"Do not produce another final answer until the save succeeds, fails, or is rejected by governance.",
			}
		}
	}
	return nil
}

func completionVerificationRequiredSkillTool(decision completionVerificationDecision) (string, string, bool) {
	text := strings.ToLower(strings.Join(append(append([]string{}, decision.MissingSteps...), decision.NextActionHint, decision.FinalAnswerGuidance, decision.Reason), "\n"))
	if text == "" {
		return "", "", false
	}
	if strings.Contains(text, "agent-management/update_agent_config") ||
		strings.Contains(text, "update_agent_config") {
		return skills.SkillAgentManagement, "update_agent_config", true
	}
	if strings.Contains(text, "agent-management/get_agent_config") ||
		strings.Contains(text, "get_agent_config") {
		return skills.SkillAgentManagement, "get_agent_config", true
	}
	if strings.Contains(text, "file-manager/save_file_to_management") ||
		strings.Contains(text, "save_file_to_management") {
		return skills.SkillFileManager, "save_file_to_management", true
	}
	if strings.Contains(text, "file-manager/delete_file") ||
		strings.Contains(text, "delete_file") ||
		strings.Contains(text, "delete resolved file") {
		return skills.SkillFileManager, "delete_file", true
	}
	return "", "", false
}

func completionVerificationFallbackAnswer(decision completionVerificationDecision, candidateAnswer string) string {
	if answer := strings.TrimSpace(decision.FinalAnswer); answer != "" {
		return answer
	}
	parts := []string{}
	switch decision.normalizedStatus() {
	case completionVerificationStatusAskUser:
		parts = append(parts, completionVerificationFallbackAskUser)
	case completionVerificationStatusFailed:
		parts = append(parts, completionVerificationFallbackFailed)
	default:
		parts = append(parts, completionVerificationFallbackUnknown)
	}
	if reason := completionVerificationPublicReason(decision.Reason); reason != "" {
		parts = append(parts, "\u539f\u56e0\uff1a"+reason+"\u3002")
	}
	if missing := strings.Join(completionVerificationPublicMissingSteps(decision.MissingSteps), completionVerificationJoinSeparator); missing != "" {
		parts = append(parts, "\u7f3a\u5c11\u7684\u5b8c\u6210\u8bc1\u636e\uff1a"+missing+"\u3002")
	}
	if claims := strings.Join(completionVerificationPublicUnsupportedClaims(decision.UnsupportedClaims), completionVerificationJoinSeparator); claims != "" {
		parts = append(parts, "\u5019\u9009\u7b54\u590d\u4e2d\u6709\u672a\u88ab\u5de5\u5177\u7ed3\u679c\u652f\u6301\u7684\u8bf4\u6cd5\uff1a"+claims+"\u3002")
	}
	if len(parts) == 0 {
		return strings.TrimSpace(candidateAnswer)
	}
	return strings.Join(parts, "\n")
}

func completionVerificationPublicUnsupportedClaims(claims []string) []string {
	cleaned := cleanStringSlice(claims)
	out := make([]string, 0, len(cleaned))
	for _, claim := range cleaned {
		lower := strings.ToLower(strings.TrimSpace(claim))
		if completionVerificationCandidateAnswerLeaksInternalPlan(lower) ||
			strings.Contains(lower, "internal planning") ||
			strings.Contains(lower, "system instruction") ||
			strings.Contains(lower, "candidate answer") ||
			strings.Contains(lower, "unsupported claim") {
			continue
		}
		out = append(out, claim)
	}
	return dedupeStrings(out)
}

func completionVerificationPublicMissingSteps(steps []string) []string {
	cleaned := cleanStringSlice(steps)
	out := make([]string, 0, len(cleaned))
	for _, step := range cleaned {
		lower := strings.ToLower(strings.TrimSpace(step))
		switch {
		case strings.Contains(lower, "delete resolved file"):
			out = append(out, "\u6587\u4ef6\u5220\u9664\u7ed3\u679c")
		case strings.Contains(lower, "agent-management/update_agent_config") ||
			strings.Contains(lower, "update_agent_config"):
			out = append(out, "\u667a\u80fd\u4f53\u914d\u7f6e\u66f4\u65b0\u7ed3\u679c")
		case strings.Contains(lower, "agent-management/get_agent_config") ||
			strings.Contains(lower, "get_agent_config"):
			out = append(out, "\u66f4\u65b0\u540e\u7684\u667a\u80fd\u4f53\u914d\u7f6e\u8bfb\u53d6\u7ed3\u679c")
		default:
			out = append(out, step)
		}
	}
	return dedupeStrings(out)
}

func completionVerificationPublicReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	lower := strings.ToLower(reason)
	internalMarkers := []string{
		"operation_plan",
		"operation plan",
		"turn_strategy",
		"turn strategy",
		"pending executable",
		"pending step",
		"required_next_tool",
		"completion verifier",
		"post-verification",
		"candidate answer",
		"unsupported claim",
		"verification contract",
		"system prompt",
		"system message",
		"internal plan",
		"internal strategy",
		"\u7cfb\u7edf\u63d0\u793a",
		"\u5185\u90e8\u8ba1\u5212",
	}
	for _, marker := range internalMarkers {
		if strings.Contains(lower, marker) {
			return ""
		}
	}
	return trimCompletionVerificationDetail(strings.TrimSuffix(reason, "\u3002"))
}

func skillToolCallRefsForVerifier(refs []SkillToolCallRef) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(refs))
	for _, ref := range refs {
		item := map[string]interface{}{
			"skill_id":  strings.TrimSpace(ref.SkillID),
			"tool_name": strings.TrimSpace(ref.ToolName),
		}
		if len(ref.Arguments) > 0 {
			item["arguments"] = ref.Arguments
		}
		if len(ref.Result) > 0 {
			item["result"] = ref.Result
		}
		out = append(out, item)
	}
	return out
}

func cleanStringSlice(input []string) []string {
	out := make([]string, 0, len(input))
	for _, item := range input {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func evidenceMapFromAny(value interface{}) map[string]interface{} {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func evidenceSliceFromAny(value interface{}) []interface{} {
	if typed, ok := value.([]interface{}); ok {
		return typed
	}
	if typed, ok := value.([]map[string]interface{}); ok {
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	}
	return nil
}

func evidenceMapsFromAny(value interface{}) []map[string]interface{} {
	if typed, ok := value.([]map[string]interface{}); ok {
		return typed
	}
	values := evidenceSliceFromAny(value)
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		if item := evidenceMapFromAny(value); len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func evidenceStringFromAny(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
