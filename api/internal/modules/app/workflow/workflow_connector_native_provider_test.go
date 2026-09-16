package workflow

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"testing"
)

func TestWorkflowNativeCatalogSelectionsUseOwnedOperationIdentity(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	s := &WorkflowService{executor: NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{ConnectorCatalog: c})}
	for _, definition := range definitions {
		for _, op := range definition.Operations {
			t.Run(op.ID, func(t *testing.T) {
				owner, connection := uuid.New(), uuid.New()
				graph := map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "node", "data": map[string]interface{}{"provider_type": "connector", "connector": map[string]interface{}{"connection_id": connection.String(), "connector_id": definition.ID, "operation_id": op.ID}}}}}
				rows, err := s.workflowConnectorBindingRows(t.Context(), &Workflow{}, graph, owner.String())
				if err != nil || len(rows) != 1 {
					t.Fatalf("rows=%#v err=%v", rows, err)
				}
				if rows[0].OwnerID != owner || rows[0].ConnectionID != connection || rows[0].OperationID.String() != op.ID || rows[0].NodeID != "node" {
					t.Fatal("lost operation identity")
				}
			})
		}
	}
}
