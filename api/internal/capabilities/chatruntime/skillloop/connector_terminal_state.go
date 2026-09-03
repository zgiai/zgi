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
	return &terminalStateGuardDecision{Path: terminalStateGuardAccepted, Reason: "Connector outcomes do not prove every operation succeeded", FinalAnswer: connectorOutcomeAnswer(hint, counts)}
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

func connectorOutcomeAnswer(hint string, counts map[string]int) string {
	chinese := false
	for _, r := range hint {
		if unicode.Is(unicode.Han, r) {
			chinese = true
			break
		}
	}
	parts := []string{}
	for _, item := range []struct{ state, english, chinese string }{
		{"successful", "succeeded", "成功"}, {"failed", "failed", "失败"}, {"unknown", "have an unknown result", "结果未知"}, {"cancelled", "were cancelled", "已取消"}, {"unavailable", "could not run", "未执行"},
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
