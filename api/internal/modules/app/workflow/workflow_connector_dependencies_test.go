package workflow

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestWorkflowDraftSelectionsRejectRetiredHandlesAndInvalidOperations(t *testing.T) {
	definitions, _ := catalog.BuiltinDefinitions()
	c, _ := catalog.New(definitions...)
	s := &WorkflowService{executor: NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{ConnectorCatalog: c})}
	for _, kind := range []string{"retired", "operation", "connection", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			data := map[string]interface{}{"provider_type": "connector", "connector": map[string]interface{}{"connection_id": uuid.NewString(), "connector_id": "github", "operation_id": "github.user.get"}}
			switch kind {
			case "retired":
				delete(data, "connector")
				data["connection_id"] = uuid.NewString()
			case "operation":
				data["connector"].(map[string]interface{})["operation_id"] = "github.unknown"
			case "connection":
				data["connector"].(map[string]interface{})["connection_id"] = "invalid"
			}
			nodes := []interface{}{map[string]interface{}{"id": "node", "data": data}}
			if kind == "duplicate" {
				nodes = append(nodes, nodes[0])
			}
			if _, err := s.workflowConnectorBindingRows(t.Context(), &Workflow{}, map[string]interface{}{"nodes": nodes}, uuid.NewString()); err == nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
}
func TestWorkflowPublishRequiresConfiguredNativeReference(t *testing.T) {
	for _, data := range []map[string]interface{}{{"provider_type": "connector"}, {"provider_type": "connector", "connector_unavailable_ref": uuid.NewString()}, {"provider_type": "connector", "connector": map[string]interface{}{"connection_id": uuid.NewString()}}} {
		if validateWorkflowConnectorNodesConfigured(map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "node", "data": data}}}) == nil {
			t.Fatal("unconfigured publish accepted")
		}
	}
}
func TestWorkflowRewriteUpdatesEachNodeBindingAndPreservesBusinessInput(t *testing.T) {
	graph := map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "node", "data": map[string]interface{}{"provider_type": "connector", "connector": map[string]interface{}{"operation_id": "github.user.get"}, "tool_parameters": map[string]interface{}{"query": "keep"}}}}}
	raw, _ := json.Marshal(graph)
	workflow := &Workflow{Graph: string(raw)}
	binding := core.TargetBinding{ID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", Target: core.TargetRef{Type: core.TargetTypeWorkflow, ID: uuid.NewString(), BindingInstanceID: "node"}, Revision: 2}
	if err := rewriteWorkflowConnectorIDs(workflow, []core.TargetBinding{binding}); err != nil {
		t.Fatal(err)
	}
	nodes, _ := workflowConnectorGraphNodes(workflow.GetGraphDict())
	data := workflowNodeData(nodes[0])
	ref := data["connector"].(map[string]interface{})
	if ref["binding_id"] != binding.ID.String() || ref["operation_id"] != "github.user.get" || data["tool_parameters"].(map[string]interface{})["query"] != "keep" {
		t.Fatal("snapshot rewrite lost values")
	}
}
func TestConnectorExportDoesNotContainOwnerHandles(t *testing.T) {
	ref := map[string]interface{}{"connection_id": uuid.NewString(), "binding_id": uuid.NewString(), "revision": 2, "connector_id": "github", "operation_id": "github.user.get"}
	data := map[string]interface{}{"provider_type": "connector", "connector": ref}
	graph := map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "node", "data": data}}}
	SanitizeGraph(graph)
	if ref["connection_id"] != nil || ref["binding_id"] != nil || ref["operation_id"] != "github.user.get" {
		t.Fatal("export retained credentials coordinates")
	}
}
