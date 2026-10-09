package workflow

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestWorkflowReferencePlacementMatchesOptimisticSelection(t *testing.T) {
	refID := uuid.New()
	connectionID := uuid.New()
	operationID, err := core.NormalizeIdentifier("github-mcp.issue.list")
	if err != nil {
		t.Fatal(err)
	}
	reference := core.UnavailableReference{ID: refID}
	command := core.ReferenceCommand{ReferenceID: refID, Action: "replace", ConnectionID: connectionID, OperationID: operationID}
	selected := map[string]any{
		"provider_type": "connector",
		"connector": map[string]any{
			"connection_id": connectionID.String(),
			"operation_id":  operationID.String(),
		},
	}
	if !workflowReferencePlacementMatches(selected, reference, command) {
		t.Fatal("matching optimistic selection rejected")
	}
	selected["connector"].(map[string]any)["connection_id"] = uuid.NewString()
	if workflowReferencePlacementMatches(selected, reference, command) {
		t.Fatal("a different connection was accepted")
	}
	selected["connector_unavailable_ref"] = uuid.NewString()
	if workflowReferencePlacementMatches(selected, reference, command) {
		t.Fatal("a different reference marker was accepted")
	}
	selected["connector_unavailable_ref"] = refID.String()
	if !workflowReferencePlacementMatches(selected, reference, command) {
		t.Fatal("the original reference marker was rejected")
	}
	if !workflowReferencePlacementMatches(map[string]any{"provider_type": "connector"}, reference, core.ReferenceCommand{Action: "detach"}) {
		t.Fatal("optimistically detached node was rejected")
	}
}
