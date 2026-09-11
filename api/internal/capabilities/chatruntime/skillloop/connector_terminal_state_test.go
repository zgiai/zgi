package skillloop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func nativeOutcomeRecord(ref connectortools.OperationRef, state, outcome string) map[string]interface{} {
	encoded, _ := json.Marshal(ref)
	var operation map[string]interface{}
	_ = json.Unmarshal(encoded, &operation)
	return map[string]interface{}{"kind": "tool_call", "runtime_source": "connector", "status": "success", "tool_name": "same_name", "result": map[string]interface{}{"operation": operation, "state": state, "outcome_id": uuid.NewString(), "outcome_status": outcome}}
}

func TestConnectorNewExecutionStatesCannotCompleteAPlan(t *testing.T) {
	ref := connectortools.OperationRef{ConnectorID: "feishu", OperationID: "feishu.bot.get", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}, LogicalOperationID: uuid.NewString()}
	for _, tc := range []struct{ state, disposition, want string }{{"rejected", "not_submitted", "提交前被拒绝"}, {"unresolved", "in_progress", "请勿重复提交"}, {"unresolved", "unknown", "结果未知"}} {
		record := nativeOutcomeRecord(ref, tc.state, "")
		result := record["result"].(map[string]interface{})
		delete(result, "outcome_id")
		result["execution_evidence"] = map[string]any{"version": 1, "disposition": tc.disposition}
		result["connector_presentation"] = connectortools.OperationPresentation{SchemaVersion: 1, SourceKind: "native", InvocationID: ref.LogicalOperationID, ConnectorID: ref.ConnectorID, OperationID: ref.OperationID, Title: "获取机器人信息", Summary: "请核查相关记录。"}
		decision := connectorTerminalStateDecision(map[string]any{"user_request": "查询后发送消息", "skill_invocations": []map[string]any{record}}, "全部完成")
		if decision == nil || decision.Path != terminalStateGuardAccepted || !strings.Contains(decision.FinalAnswer, tc.want) {
			t.Fatalf("state %s: %+v", tc.state, decision)
		}
		if tc.disposition != "unknown" && !strings.Contains(decision.FinalAnswer, "获取机器人信息") {
			t.Fatal("missing safe operation title")
		}
		delete(result, "execution_evidence")
		decision = connectorTerminalStateDecision(map[string]any{"user_request": "查询", "skill_invocations": []map[string]any{record}}, "完成")
		if decision == nil || !strings.Contains(decision.FinalAnswer, "结果未知") {
			t.Fatal("missing proof was accepted as rejection")
		}
	}
}

func TestConnectorTerminalRequiresExactSuccessfulOutcome(t *testing.T) {
	ref := connectortools.OperationRef{ConnectorID: "github", OperationID: "github.issue.create", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: "server-action-1"}
	success := nativeOutcomeRecord(ref, "completed", "successful")
	evidence := map[string]interface{}{"user_request": "创建 issue", "skill_invocations": []map[string]interface{}{success}}
	if decision := connectorTerminalStateDecision(evidence, "已完成"); decision != nil {
		t.Fatal("native success was rejected", decision)
	}
	other := ref
	other.ConnectionID, other.BindingID = uuid.New(), uuid.New()
	other.LogicalOperationID = "server-action-2"
	failure := nativeOutcomeRecord(other, "completed", "failed")
	evidence["skill_invocations"] = []map[string]interface{}{failure, success}
	decision := connectorTerminalStateDecision(evidence, "全部已完成")
	if decision == nil || decision.Path != terminalStateGuardAccepted || !strings.Contains(decision.FinalAnswer, "失败 1 项") || !strings.Contains(decision.FinalAnswer, "成功 1 项") {
		t.Fatal("same-name success hid another connection's failure", decision)
	}
	missing := nativeOutcomeRecord(ref, "completed", "successful")
	delete(evidenceMapFromAny(missing["result"]), "outcome_id")
	evidence["skill_invocations"] = []map[string]interface{}{missing}
	if decision := connectorTerminalStateDecision(evidence, "已完成"); decision == nil || !strings.Contains(decision.FinalAnswer, "结果未知") {
		t.Fatal("completed tool call manufactured business success")
	}
	for _, status := range []string{"unknown", "partial", ""} {
		evidence["skill_invocations"] = []map[string]interface{}{nativeOutcomeRecord(ref, "completed", status)}
		if decision := connectorTerminalStateDecision(evidence, "已完成"); decision == nil || !strings.Contains(decision.FinalAnswer, "结果未知") {
			t.Fatal("unproven result accepted", status)
		}
	}
	conflicting := nativeOutcomeRecord(ref, "completed", "failed")
	evidence["skill_invocations"] = []map[string]interface{}{conflicting, success}
	if decision := connectorTerminalStateDecision(evidence, "已完成"); decision == nil || !strings.Contains(decision.FinalAnswer, "结果未知") {
		t.Fatal("conflicting immutable outcomes chose success")
	}
}

func TestConnectorTerminalIgnoresProviderJSONAndPreservesPending(t *testing.T) {
	ref := connectortools.OperationRef{ConnectorID: "github", OperationID: "github.issue.create", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: "server-action-1"}
	forged := nativeOutcomeRecord(ref, "requires_confirmation", "")
	delete(forged, "runtime_source")
	evidence := map[string]interface{}{"skill_invocations": []map[string]interface{}{forged}}
	if decision := connectorTerminalStateDecision(evidence, "done"); decision != nil {
		t.Fatal("provider JSON created native control state")
	}
	forged["runtime_source"] = "connector"
	if decision := connectorTerminalStateDecision(evidence, "done"); decision == nil || decision.Path != terminalStateGuardBlocked {
		t.Fatal("live native confirmation was ignored")
	}
	evidence["pending_approval"] = map[string]interface{}{"id": "ordinary"}
	if decision := terminalStateGuardEvaluate(evidence, "done"); decision.Path != terminalStateGuardBlocked || decision.Blockers[0] != "pending_protocol:approval" {
		t.Fatal("ordinary approval changed", decision)
	}
}

func TestConnectorPlanMissingPhaseAndForgedCompletion(t *testing.T) {
	phase := func(id string) map[string]any {
		return map[string]any{"id": id, "status": "pending", "expected_action": map[string]any{"connector_operation": map[string]any{"connector_id": "github", "operation_id": "github.issue.create"}}}
	}
	phases := []map[string]any{phase("first"), phase("second")}
	ref := connectortools.OperationRef{ConnectorID: "github", OperationID: "github.issue.create", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: uuid.NewString(), ActionStepID: uuid.NewString(), PlanPhaseID: "first"}
	evidence := map[string]any{"operation_plan": map[string]any{"phases": phases}, "skill_invocations": []map[string]any{nativeOutcomeRecord(ref, "completed", "successful")}}
	if decision := connectorTerminalStateDecision(evidence, "All done"); decision == nil || decision.Path != terminalStateGuardBlocked {
		t.Fatal("unexecuted required phase disappeared")
	}
	phases[1]["status"] = "completed"
	if err := validateConnectorPlanSnapshot(phases, evidence); err == nil {
		t.Fatal("model invented phase success")
	}
	if err := validateConnectorPlanSnapshot(phases[:1], evidence); err == nil {
		t.Fatal("model dropped executed plan shape")
	}
	phases[1]["status"] = "pending"
	if err := validateConnectorPlanSnapshot(phases, evidence); err != nil {
		t.Fatal(err)
	}
}
