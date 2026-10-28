package skillloop

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// Connector terminal evidence comes from the host's typed tool trace. Provider
// JSON, a tool name, or a completed model call cannot attest business success.
func connectorTerminalStateDecision(evidence map[string]interface{}, answer string) *terminalStateGuardDecision {
	states := map[string]string{}
	details := map[string]connectortools.InvocationEvidence{}
	for index, record := range terminalStateGuardInvocations(evidence) {
		proof, native := connectortools.Evidence(record)
		if !native {
			continue
		}
		key := fmt.Sprintf("unresolved:%d", index)
		state := proof.Status
		if proof.Operation.Valid() {
			key = proof.Key()
		}
		if previous, exists := states[key]; exists && previous != "pending" && previous != state {
			// Inconsistent terminal evidence is never resolved by choosing the
			// more optimistic record. Current native outcomes are immutable.
			state = "unknown"
		}
		states[key] = state
		if state != "successful" && state != "pending" && proof.Title != "" {
			proof.Status = state
			details[key] = proof
		}
	}
	counts := map[string]int{}
	for _, state := range states {
		counts[state]++
	}
	missing := 0
	for _, phase := range evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"]) {
		if len(connectorPhaseOperation(phase)) == 0 {
			continue
		}
		if _, matched := connectorPhaseEvidence(phase, evidence); !matched {
			missing++
		}
	}
	if counts["pending"] > 0 {
		return &terminalStateGuardDecision{Path: terminalStateGuardBlocked, Reason: "Connector confirmation is pending", Blockers: []string{"pending_protocol:connector_confirmation"}}
	}
	if missing > 0 && len(states) == counts["successful"] {
		return &terminalStateGuardDecision{Path: terminalStateGuardBlocked, Reason: "Required Connector phases have no native execution outcome", Blockers: []string{"connector_plan:missing_outcome"}}
	}
	counts["unavailable"] += missing
	if missing == 0 && len(states) == counts["successful"] {
		return nil
	}
	hint := evidenceStringFromAny(evidence["latest_user_request"]) + evidenceStringFromAny(evidence["user_request"]) + answer
	finalAnswer := connectorOutcomeAnswer(hint, counts)
	// Name every operation that did not succeed and why, in the order it ran,
	// so the fixed answer still tells the person what to fix.
	chinese := connectorAnswerIsChinese(hint)
	parts := []string{}
	for _, record := range terminalStateGuardInvocations(evidence) {
		proof, native := connectortools.Evidence(record)
		if !native {
			continue
		}
		if detail, exists := details[proof.Key()]; exists {
			parts = append(parts, connectorOutcomeDetail(detail, chinese))
			delete(details, proof.Key())
		}
	}
	if len(parts) > 0 {
		finalAnswer = strings.Join(parts, "\n") + "\n" + finalAnswer
	}
	// When every operation that did not succeed was cancelled by the person,
	// nothing ran that the model could misreport, so the facts are appended to
	// its own answer instead of replacing it: declining one step must not erase
	// the evidence and analysis gathered by every other step. A failed,
	// unknown, rejected, or in-progress outcome keeps the strict replacement.
	onlyCancelled := counts["cancelled"] > 0 && counts["successful"]+counts["cancelled"] == len(states) && missing == 0
	if candidate := strings.TrimSpace(answer); onlyCancelled && candidate != "" {
		finalAnswer = candidate + "\n\n" + finalAnswer
	}
	return &terminalStateGuardDecision{Path: terminalStateGuardAccepted, Reason: "Connector outcomes do not prove every operation succeeded", FinalAnswer: finalAnswer}
}

// Once dispatched, a non-success result ends unattended business execution.
// Argument validation before dispatch has no operation reference and can still
// use ordinary tool correction. Pending confirmation follows its own protocol.
func connectorTerminalStep(trace skills.SkillTrace, callID string, runtimeState map[string]interface{}) *skillStepResult {
	if !trace.ConnectorNative || trace.Result["operation"] == nil || trace.Result["state"] == "requires_confirmation" {
		return nil
	}
	current := map[string]interface{}{"kind": "tool_call", "runtime_source": "connector", "result": trace.Result}
	if connectorTerminalStateDecision(map[string]interface{}{"skill_invocations": []map[string]interface{}{current}}, "") == nil {
		return nil
	}
	evidence := make(map[string]interface{}, len(runtimeState)+1)
	for key, value := range runtimeState {
		evidence[key] = value
	}
	records := append([]map[string]interface{}{}, terminalStateGuardInvocations(runtimeState)...)
	evidence["skill_invocations"] = append(records, current)
	decision := connectorTerminalStateDecision(evidence, "")
	if decision == nil || decision.Path != terminalStateGuardAccepted {
		return nil
	}
	result := terminalSkillStep(trace, skills.ToolResultMessage(callID, trace.Result), true, true)
	result.toolResult, result.answer = trace.Result, decision.FinalAnswer
	return &result
}

func connectorAnswerIsChinese(hint string) bool {
	for _, r := range hint {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// connectorOutcomeDetail is one line for an operation that did not succeed:
// its title and the safe, localized reason of its error code, or the
// presentation summary when there is no code.
func connectorOutcomeDetail(proof connectortools.InvocationEvidence, chinese bool) string {
	locale := "en-US"
	if chinese {
		locale = "zh-Hans"
	}
	reason := ""
	if proof.ErrorCode != "" {
		reason = connectortools.SafeTerminalSummary(proof.Status, proof.ErrorCode, locale)
	}
	if reason == "" {
		reason = proof.Summary
	}
	if reason == "" {
		return proof.Title
	}
	return proof.Title + "：" + reason
}

func connectorOutcomeAnswer(hint string, counts map[string]int) string {
	chinese := connectorAnswerIsChinese(hint)
	if counts["rejected"] > 0 && counts["unknown"] == 0 && counts["in_progress"] == 0 {
		if chinese {
			return "有操作在提交前被拒绝，本轮已停止后续操作。"
		}
		return "An operation was rejected before submission. Further operations in this turn have stopped."
	}
	if counts["in_progress"] > 0 && counts["unknown"] == 0 {
		if chinese {
			return "已有操作仍在处理，请勿重复提交；本轮已停止后续操作。"
		}
		return "An existing operation is still processing. Do not submit it again; further operations have stopped."
	}
	parts := []string{}
	for _, item := range []struct{ state, english, chinese string }{
		{"successful", "succeeded", "成功"}, {"failed", "failed", "失败"}, {"unknown", "have an unknown result", "结果未知"}, {"cancelled", "were cancelled", "已取消"}, {"unavailable", "could not run", "未执行"}, {"rejected", "were rejected before submission", "提交前拒绝"}, {"in_progress", "are processing", "处理中"},
	} {
		if count := counts[item.state]; count > 0 {
			if chinese {
				parts = append(parts, fmt.Sprintf("%s %d 项", item.chinese, count))
			} else {
				parts = append(parts, fmt.Sprintf("%d %s", count, item.english))
			}
		}
	}
	if chinese {
		answer := "连接器操作结果：" + strings.Join(parts, "，") + "。"
		if counts["unknown"] > 0 {
			answer += "结果未知的操作不能视为成功；请先核查执行记录，再决定是否重试。"
		}
		return answer
	}
	answer := "Connector results: " + strings.Join(parts, ", ") + "."
	if counts["unknown"] > 0 {
		answer += " Unknown results do not establish success; check the execution records before deciding whether to retry."
	}
	return answer
}
