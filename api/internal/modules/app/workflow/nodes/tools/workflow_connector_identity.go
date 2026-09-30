package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	runtimetools "github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/database"
	"strings"
)

type ConnectorNodeReference struct {
	BindingID    string `json:"binding_id"`
	ConnectionID string `json:"connection_id"`
	ConnectorID  string `json:"connector_id"`
	OperationID  string `json:"operation_id"`
}

// Graph JSON contains lookup coordinates. The persisted graph and the canonical
// binding must agree before an identity can cross the dispatch boundary.
func (n *Node) workflowConnectorIdentity(ctx context.Context, pool *entities.VariablePool) (*connectordispatch.Identity, error) {
	if n == nil || n.getProviderType() != runtimetools.ToolProviderTypeConnector {
		return nil, nil
	}
	ref := n.nodeData.Connector
	if ref == nil {
		return nil, fmt.Errorf("Native Connector reference is required")
	}
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("Connector database is unavailable")
	}
	org, e1 := uuid.Parse(n.OrganizationID)
	owner, e2 := uuid.Parse(n.UserID)
	bindingID, e3 := uuid.Parse(ref.BindingID)
	connector, e4 := connectorcore.NormalizeIdentifier(ref.ConnectorID)
	operation, e5 := connectorcore.NormalizeIdentifier(ref.OperationID)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || org == uuid.Nil || owner == uuid.Nil || bindingID == uuid.Nil {
		return nil, fmt.Errorf("Connector identity is invalid")
	}
	workspace := n.WorkspaceID
	if workspace == "" {
		workspace = n.TenantID
	}
	var state struct {
		ID          uuid.UUID
		Version     string
		VersionUUID *uuid.UUID
		Graph       string
	}
	err := db.WithContext(ctx).Table("workflows AS workflow").Select("workflow.id, workflow.version, workflow.version_uuid, workflow.graph").Joins("JOIN agents AS owner ON owner.id = workflow.agent_id AND owner.tenant_id = workflow.tenant_id AND owner.deleted_at IS NULL").Joins("JOIN workspaces AS workspace ON workspace.id = workflow.tenant_id AND workspace.status = ?", "normal").Where("workflow.id = ? AND workflow.agent_id = ? AND workflow.tenant_id = ? AND workspace.organization_id = ? AND workflow.deleted_at IS NULL", n.WorkflowID, n.APPID, workspace, org).Take(&state).Error
	if err != nil {
		return nil, fmt.Errorf("Workflow Connector target is unavailable")
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeWorkflow, ID: state.ID.String(), Scope: connectorcore.ScopeDraft, BindingInstanceID: n.NodeID}
	if state.Version != "draft" {
		if state.VersionUUID == nil || *state.VersionUUID == uuid.Nil {
			return nil, fmt.Errorf("Workflow version is unavailable")
		}
		target.ID = state.VersionUUID.String()
		target.Scope = connectorcore.ScopePublished
		target.VersionID = target.ID
	}
	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Data struct {
				Connector   *ConnectorNodeReference `json:"connector"`
				Unavailable json.RawMessage         `json:"connector_unavailable_ref"`
			} `json:"data"`
		} `json:"nodes"`
	}
	if json.Unmarshal([]byte(state.Graph), &graph) != nil {
		return nil, fmt.Errorf("Workflow graph is invalid")
	}
	matched := false
	seenNode := false
	for _, node := range graph.Nodes {
		if node.ID == n.NodeID {
			if seenNode {
				return nil, fmt.Errorf("Workflow node identity is ambiguous")
			}
			seenNode = true
		}
		if node.ID == n.NodeID && node.Data.Connector != nil && (len(node.Data.Unavailable) == 0 || string(node.Data.Unavailable) == "null") && *node.Data.Connector == *ref {
			matched = true
		}
	}
	if !matched {
		return nil, fmt.Errorf("Workflow Connector reference changed")
	}
	binding, err := connectorbindings.NewGormRepository(db).GetBinding(ctx, org, owner, bindingID)
	if err != nil {
		return nil, err
	}
	if binding.OrganizationID != org || binding.OwnerAccountID != owner || binding.ID != bindingID || !binding.Target.SameIdentity(target) || binding.ConnectorID != connector || binding.ConnectionID.String() != ref.ConnectionID || binding.Status != connectorcore.BindingStatusActive {
		return nil, fmt.Errorf("Workflow Connector binding is unavailable")
	}
	if n.connectorCatalog == nil {
		return nil, fmt.Errorf("Connector catalog is unavailable")
	}
	definition, ok := n.connectorCatalog.Get(connector.String())
	if !ok {
		return nil, fmt.Errorf("Connector definition is unavailable")
	}
	matched = false
	for _, op := range definition.Operations {
		if op.ID == operation.String() && op.ToolName == n.nodeData.ToolName {
			matched = true
			break
		}
	}
	if !matched {
		return nil, fmt.Errorf("Connector operation is unavailable")
	}
	targetID, _ := uuid.Parse(target.ID)
	logical, correlation := n.workflowConnectorOperationTrace(pool, targetID, operation)
	return &connectordispatch.Identity{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: binding.ID, ConnectionID: binding.ConnectionID, ConnectorID: connector, Target: target, OperationID: operation, LogicalOperationID: logical, CorrelationID: correlation, Interactive: n.workflowConnectorInteractive()}, nil
}
func (n *Node) workflowConnectorOperationTrace(
	variablePool *entities.VariablePool,
	targetID uuid.UUID,
	operationID connectorcore.Identifier,
) (string, string) {
	// Lookup coordinates only; the host commits final action IDs before dispatch.
	seed := strings.Join([]string{
		n.WorkflowID, targetID.String(), strings.TrimSpace(n.NodeID), operationID.String(),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("workflow-connector-lookup\x00"+seed)).String(),
		uuid.NewSHA1(uuid.NameSpaceOID, []byte("workflow-connector-correlation\x00"+seed)).String()
}

func (n *Node) workflowConnectorInteractive() bool {
	switch strings.ToLower(strings.TrimSpace(n.InvokeFrom)) {
	case "debugger", "web-app":
		return true
	default:
		return false
	}
}
