package tools

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestWorkflowFeishuReadsDispatchSavedActionInDraftAndPublished(t *testing.T) {
	for _, published := range []bool{false, true} {
		for _, operationID := range []string{"feishu.tenant.get", "feishu.chat.members.list"} {
			t.Run(map[bool]string{false: "draft", true: "published"}[published]+"/"+operationID, func(t *testing.T) {
				f := newNativeWorkflowFixture(t, published, "github.user.get")
				definition, ok := f.node.connectorCatalog.Get("feishu")
				if !ok {
					t.Fatal("Feishu catalog missing")
				}
				toolName := ""
				for _, operation := range definition.Operations {
					if operation.ID == operationID {
						toolName = operation.ToolName
					}
				}
				if toolName == "" {
					t.Fatalf("missing native action %s", operationID)
				}
				f.node.nodeData.ProviderID = "feishu"
				f.node.nodeData.ToolName = toolName
				f.node.nodeData.Connector.ConnectorID = "feishu"
				f.node.nodeData.Connector.OperationID = operationID
				f.binding.ConnectorID = "feishu"
				f.binding.Constraint = core.OwnerConstraint(f.binding.OwnerAccountID, []core.Identifier{core.Identifier(operationID)})
				wantInput := map[string]interface{}{}
				data := map[string]interface{}{"tenant": map[string]interface{}{"name": "Fixture tenant", "display_id": "F001", "tenant_key": "fixture"}}
				if operationID == "feishu.chat.members.list" {
					f.node.nodeData.ToolParameters["chat_id"] = &ToolInput{Type: "constant", Value: "oc_fixture"}
					wantInput = map[string]interface{}{"chat_id": "oc_fixture", "page_size": float64(20)}
					data = map[string]interface{}{"members": []interface{}{}, "has_more": false}
				}
				graph, err := json.Marshal(map[string]interface{}{"nodes": []interface{}{map[string]interface{}{
					"id": f.node.NodeID, "data": map[string]interface{}{"provider_type": "connector", "provider_id": "feishu", "tool_name": toolName, "connector": f.node.nodeData.Connector},
				}}})
				if err != nil {
					t.Fatal(err)
				}
				f.graph = string(graph)
				f.expectGraph()
				f.expectBinding(f.binding, true)
				capture := &workflowCatalogDispatchCapture{result: &connectorexecution.ExecutionResult{
					State:   connectorexecution.ExecutionStateCompleted,
					Outcome: core.OperationOutcome{ID: uuid.New(), Status: core.OutcomeStatusSuccessful},
					Value:   map[string]interface{}{"provider": "feishu", "operation": operationID, "data": data, "truncated": false},
				}}
				f.node.connectorDispatcher = capture
				result, err := f.node.executeRun(t.Context(), nil)
				if err != nil || result == nil || result.Status != shared.SUCCEEDED || capture.calls != 1 {
					t.Fatalf("native workflow read failed: result=%#v err=%v calls=%d", result, err, capture.calls)
				}
				identity := capture.invocation.Identity
				if identity.ConnectorID != "feishu" || identity.OperationID.String() != operationID ||
					identity.BindingID != f.binding.ID || identity.ConnectionID != f.binding.ConnectionID || !identity.Target.SameIdentity(f.binding.Target) {
					t.Fatalf("saved action identity changed: %#v", identity)
				}
				if !reflect.DeepEqual(capture.invocation.Input, wantInput) {
					t.Fatalf("business input/default mismatch: got %#v want %#v", capture.invocation.Input, wantInput)
				}
				if err := f.mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
