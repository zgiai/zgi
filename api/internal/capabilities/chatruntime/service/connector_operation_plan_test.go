package service

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
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
