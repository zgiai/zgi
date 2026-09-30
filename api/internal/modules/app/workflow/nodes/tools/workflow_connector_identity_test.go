package tools

import (
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"testing"
	"time"
)

type nativeWorkflowFixture struct {
	node      *Node
	mock      sqlmock.Sqlmock
	binding   core.TargetBinding
	graph     string
	version   string
	versionID any
}

func newNativeWorkflowFixture(t *testing.T, published bool, operationID string) *nativeWorkflowFixture {
	t.Helper()
	_, mock := openWorkflowConnectorVerifierDB(t)
	org, owner, workflow, connection, bindingID, workspace, agent := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	target := core.TargetRef{Type: core.TargetTypeWorkflow, ID: workflow.String(), Scope: core.ScopeDraft, BindingInstanceID: "connector-node"}
	version := "draft"
	var versionID any
	if published {
		id := uuid.New()
		versionID = id
		version = "v1"
		target.ID = id.String()
		target.VersionID = id.String()
		target.Scope = core.ScopePublished
	}
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := c.Get("github")
	toolName := ""
	for _, op := range definition.Operations {
		if op.ID == operationID {
			toolName = op.ToolName
		}
	}
	if toolName == "" {
		t.Fatal("missing test operation")
	}
	ref := &ConnectorNodeReference{BindingID: bindingID.String(), ConnectionID: connection.String(), ConnectorID: "github", OperationID: operationID}
	n := &Node{nodeData: NodeData{ProviderType: "connector", ProviderID: "github", ToolName: toolName, Connector: ref, ToolParameters: map[string]*ToolInput{}, HasWorkflowOutputsSnapshot: true}, NodeStruct: base.NodeStruct{InstanceID: uuid.NewString(), NodeID: target.BindingInstanceID, WorkflowID: workflow.String(), APPID: agent.String(), TenantID: workspace.String(), WorkspaceID: workspace.String(), OrganizationID: org.String(), UserID: owner.String(), InvokeFrom: "debugger"}, connectorCatalog: c}
	pool := entities.NewVariablePool()
	pool.Add([]string{"sys", "workflow_run_id"}, uuid.NewString())
	n.GraphRuntimeState = entities.NewGraphRuntimeState(pool)
	ledger := dispatch.ActionLedger{}
	n.connectorActionResolver = func(ctx context.Context, identity dispatch.Identity, input any) (dispatch.Identity, error) {
		result, _, err := dispatch.ResolveActionInstance(&ledger, shared.ExecutionStepID(ctx, n.NodeID), identity.CorrelationID, identity, input)
		return result, err
	}
	graph, _ := json.Marshal(map[string]any{"nodes": []any{map[string]any{"id": n.NodeID, "data": map[string]any{"provider_type": "connector", "provider_id": "github", "tool_name": toolName, "connector": ref}}}})
	now := time.Now().UTC()
	return &nativeWorkflowFixture{node: n, mock: mock, graph: string(graph), version: version, versionID: versionID, binding: core.TargetBinding{ID: bindingID, OrganizationID: org, OwnerAccountID: owner, ConnectionID: connection, ConnectorID: "github", Target: target, Status: core.BindingStatusActive, Constraint: core.OwnerConstraint(owner, []core.Identifier{core.Identifier(operationID)}), Revision: 1, CreatedAt: now, UpdatedAt: now}}
}
func (f *nativeWorkflowFixture) expectGraph() {
	f.mock.ExpectQuery(`SELECT workflow.id, workflow.version, workflow.version_uuid, workflow.graph FROM workflows AS workflow`).WithArgs("normal", f.node.WorkflowID, f.node.APPID, f.node.WorkspaceID, f.binding.OrganizationID, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "version", "version_uuid", "graph"}).AddRow(f.node.WorkflowID, f.version, f.versionID, f.graph))
}
func (f *nativeWorkflowFixture) expectBinding(binding core.TargetBinding, found bool) {
	rows := sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connection_id", "connector_id", "target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id", "status", "binding_constraint", "revision", "created_at", "updated_at"})
	if found {
		constraint, _ := json.Marshal(binding.Constraint)
		rows.AddRow(binding.ID, binding.OrganizationID, binding.OwnerAccountID, binding.ConnectionID, binding.ConnectorID, binding.Target.Type, binding.Target.ID, binding.Target.EffectiveScope(), binding.Target.VersionID, binding.Target.BindingInstanceID, binding.Status, string(constraint), binding.Revision, binding.CreatedAt, binding.UpdatedAt)
	}
	f.mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3`).WithArgs(f.binding.OrganizationID, f.binding.OwnerAccountID, f.binding.ID, 1).WillReturnRows(rows)
}
func TestWorkflowConnectorIdentityUsesPersistedDraftAndEveryPublishedVersion(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "draft", true: "published"}[published], func(t *testing.T) {
			f := newNativeWorkflowFixture(t, published, "github.user.get")
			f.expectGraph()
			f.expectBinding(f.binding, true)
			identity, err := f.node.workflowConnectorIdentity(t.Context(), f.node.GraphRuntimeState.VariablePool)
			if err != nil || identity == nil || !identity.Target.SameIdentity(f.binding.Target) || identity.BindingID != f.binding.ID || identity.OwnerAccountID != f.binding.OwnerAccountID {
				t.Fatalf("identity=%#v error=%v", identity, err)
			}
			if err := f.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestWorkflowConnectorIdentityRejectsChangedOrRevokedReferences(t *testing.T) {
	for _, kind := range []string{"graph", "unavailable", "missing", "owner", "connection", "node", "version", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			f := newNativeWorkflowFixture(t, true, "github.user.get")
			binding := f.binding
			switch kind {
			case "graph":
				f.graph = `{"nodes":[]}`
			case "unavailable":
				var graph map[string]any
				json.Unmarshal([]byte(f.graph), &graph)
				graph["nodes"].([]any)[0].(map[string]any)["data"].(map[string]any)["connector_unavailable_ref"] = uuid.NewString()
				encoded, _ := json.Marshal(graph)
				f.graph = string(encoded)
			case "owner":
				binding.OwnerAccountID = uuid.New()
			case "connection":
				binding.ConnectionID = uuid.New()
			case "node":
				binding.Target.BindingInstanceID = "other"
			case "version":
				binding.Target.ID = uuid.NewString()
				binding.Target.VersionID = binding.Target.ID
			case "disabled":
				binding.Status = core.BindingStatusDisabled
				now := time.Now()
				binding.DisabledAt = &now
			}
			f.expectGraph()
			if kind != "graph" && kind != "unavailable" {
				f.expectBinding(binding, kind != "missing")
			}
			identity, err := f.node.workflowConnectorIdentity(t.Context(), f.node.GraphRuntimeState.VariablePool)
			if err == nil || identity != nil {
				t.Fatalf("accepted %s", kind)
			}
			if err := f.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
