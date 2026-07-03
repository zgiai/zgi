package skillloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	defaultMaxCompletionVerificationRetries = 2
	completionVerifierMaxTokens             = 1600

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

func completionVerificationPlanNeedsRuntimeEvidence(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["status"])))
	if status == "failed" || status == "error" {
		return true
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
	evidence := map[string]interface{}{}
	if req.CompletionEvidence != nil {
		for key, value := range req.CompletionEvidence() {
			evidence[key] = value
		}
	}
	if !completionVerificationShouldRun(evidence, attempted, successful, toolCallCount) {
		return completionVerificationDecision{Status: completionVerificationStatusPass}, nil, nil
	}
	if completionVerificationCandidateAnswerLeaksInternalPlan(candidateAnswer) {
		decision := completionVerificationInternalPlanLeakDecision(evidence)
		decision = completionVerificationAlignLanguage(evidence, decision)
		return decision, nil, nil
	}
	payload := map[string]interface{}{
		"candidate_answer":       strings.TrimSpace(candidateAnswer),
		"evidence":               evidence,
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
		decision.FinalAnswerGuidance = completionVerificationAgentConfigPostReadGuidance()
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
	out := []string{}
	for _, item := range evidenceSliceFromAny(value) {
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

func completionVerificationAgentConfigPostReadGuidance() string {
	return "Call agent-management/get_agent_config again after the successful Agent config update, then base the final answer on that fresh configuration result. Do not claim the requested post-update verification is complete until that read succeeds."
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

func completionVerificationSystemMessage(decision completionVerificationDecision, candidateAnswer string, retry int) adapter.Message {
	lines := []string{
		"Runtime completion verification feedback:",
		"The previous candidate final answer did not pass post-verification.",
	}
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if strings.EqualFold(strings.TrimSpace(decision.LanguageHint), "zh-Hans") {
		lines = append(lines, "Language: The user's original request is Chinese. Continue in Chinese and do not answer in English unless the user explicitly asks.")
	}
	if missing := strings.Join(cleanStringSlice(decision.MissingSteps), ", "); missing != "" {
		lines = append(lines, "Missing steps: "+missing)
	}
	if claims := strings.Join(cleanStringSlice(decision.UnsupportedClaims), ", "); claims != "" {
		lines = append(lines, "Unsupported claims: "+claims)
	}
	if hint := strings.TrimSpace(decision.NextActionHint); hint != "" {
		lines = append(lines, "Next action hint: "+hint)
	}
	if guidance := strings.TrimSpace(decision.FinalAnswerGuidance); guidance != "" {
		lines = append(lines, "Final answer guidance: "+guidance)
	}
	lines = append(lines, completionVerificationExecutableActionFeedback(decision)...)
	lines = append(lines,
		fmt.Sprintf("Post-verification retry %d of %d.", retry, defaultMaxCompletionVerificationRetries),
		"Continue only if the next action is safe and supported by the available tools. If the same tool with the same arguments already failed, do not repeat it; answer truthfully from the failure.",
	)
	if text := strings.TrimSpace(candidateAnswer); text != "" {
		lines = append(lines, "Candidate answer:\n"+text)
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func completionVerificationExecutableActionFeedback(decision completionVerificationDecision) []string {
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
