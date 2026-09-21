package service

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	confirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
)

func connectorPlanFixture() (map[string]any, map[string]any, connectortools.OperationRef) {
	ref := connectortools.OperationRef{ConnectorID: "github", OperationID: "github.issue.create", BindingID: uuid.New(), ConnectionID: uuid.New(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}, LogicalOperationID: uuid.NewString(), ActionStepID: uuid.NewString(), PlanPhaseID: "first"}
	phases := []map[string]any{}
	for _, id := range []string{"first", "second"} {
		phases = append(phases, map[string]any{"id": id, "status": "pending", "expected_action": map[string]any{"connector_operation": map[string]any{"connector_id": "github", "operation_id": "github.issue.create"}}})
	}
	plan := map[string]any{"phases": mapsToInterfaceSlice(phases)}
	invocation := map[string]any{"id": uuid.NewString(), "kind": "tool_call", "runtime_source": "connector", "status": "success", "arguments": map[string]any{"plan_phase_id": "second"}, "result": map[string]any{"operation": ref, "state": "completed", "outcome_id": uuid.NewString(), "outcome_status": "successful"}}
	return plan, invocation, ref
}

func TestConnectorPlanUsesOutcomeAndServerPhase(t *testing.T) {
	plan, invocation, _ := connectorPlanFixture()
	if operationPlanStatusFromInvocation(invocation) != "completed" || !operationPlanCompleteInvocationPhase(plan, invocation, "completed") {
		t.Fatal("native success not reconciled")
	}
	phases := mapSliceFromAny(plan["phases"])
	if phases[0]["status"] != "completed" || phases[1]["status"] == "completed" {
		t.Fatal("model phase label overrode server association")
	}
	result := mapFromOperationContext(invocation["result"])
	for _, status := range []string{"failed", "unknown", ""} {
		result["outcome_status"] = status
		if operationPlanStatusFromInvocation(invocation) != "failed" || len(connectorPlanEffects(plan, invocation)) != 0 {
			t.Fatal("non-success became completion", status)
		}
	}
	result["outcome_status"] = "successful"
	delete(result, "outcome_id")
	if operationPlanStatusFromInvocation(invocation) == "completed" {
		t.Fatal("missing outcome accepted")
	}
}

func TestConnectorConfirmationRetainsPlanAssociationAndOutcome(t *testing.T) {
	plan, invocation, ref := connectorPlanFixture()
	receipt := uuid.NewString()
	invocation["result"] = map[string]any{"state": "requires_confirmation", "operation": ref, "confirmation": map[string]any{"route_receipt": receipt}}
	// Real persistence round trip before resolving after a restart.
	metadata := map[string]any{"operation_plan": plan, "skill_invocations": []any{invocation}}
	encoded, _ := json.Marshal(metadata)
	_ = json.Unmarshal(encoded, &metadata)
	identity := ref
	identity.ActionStepID, identity.PlanPhaseID = "", ""
	resolution := connectorConfirmationResolution{routeReceipt: receipt, state: confirmation.StateCompleted, operation: &identity, outcomeID: uuid.New(), decisionID: uuid.NewString()}
	updated, added, err := applyConnectorConfirmationResolution(metadata, resolution)
	if err != nil || !added {
		t.Fatal(err)
	}
	item := skillInvocationsFromMetadata(updated["skill_invocations"])[0]
	proof, native := connectortools.Evidence(item)
	if !native || proof.Status != "successful" || proof.Operation.ActionStepID != ref.ActionStepID || proof.Operation.PlanPhaseID != ref.PlanPhaseID {
		t.Fatal("lost native confirmation association", proof)
	}
	if phases := mapSliceFromAny(mapFromOperationContext(updated["operation_plan"])["phases"]); phases[0]["status"] != "completed" || phases[1]["status"] == "completed" {
		t.Fatal("wrong confirmation phase completed")
	}
	if _, added, err := applyConnectorConfirmationResolution(updated, resolution); err != nil || added {
		t.Fatal("repeat resolution was not idempotent", err)
	}
}
