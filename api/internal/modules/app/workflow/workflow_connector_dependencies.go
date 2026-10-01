package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"github.com/google/uuid"
	core "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type workflowConnectorSelection struct {
	NodeID       string
	OwnerID      uuid.UUID
	BindingID    uuid.UUID
	ConnectionID uuid.UUID
	ConnectorID  core.Identifier
	OperationID  core.Identifier
	AuthorizationMode string
}

func (s *WorkflowService) workflowConnectorBindingRows(ctx context.Context, workflow *Workflow, graph map[string]interface{}, accountID string) ([]workflowConnectorSelection, error) {
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return nil, err
	}
	owner, ownerErr := uuid.Parse(accountID)
	result := []workflowConnectorSelection{}
	seen := map[string]bool{}
	for _, node := range nodes {
		data := workflowNodeData(node)
		if workflowString(data, "provider_type") != "connector" {
			continue
		}
		if _, unavailable := data["connector_unavailable_ref"]; unavailable {
			continue
		}
		ref, ok := data["connector"].(map[string]interface{})
		if !ok {
			if workflowString(data, "connection_id") != "" {
				return nil, fmt.Errorf("Connector node requires a native reference")
			}
			continue
		}
		connectionValue := workflowString(ref, "connection_id")
		if connectionValue == "" {
			continue
		}
		nodeID := workflowString(node, "id")
		connection, err := uuid.Parse(connectionValue)
		connector, e1 := core.NormalizeIdentifier(workflowString(ref, "connector_id"))
		operation, e2 := core.NormalizeIdentifier(workflowString(ref, "operation_id"))
		if ownerErr != nil || owner == uuid.Nil || err != nil || connection == uuid.Nil || e1 != nil || e2 != nil || nodeID == "" || seen[nodeID] {
			return nil, fmt.Errorf("Connector node identity is invalid")
		}
		seen[nodeID] = true
		if s.executor == nil || s.executor.connectorCatalog == nil {
			return nil, fmt.Errorf("Connector catalog is unavailable")
		}
		definition, found := s.executor.connectorCatalog.Get(connector.String())
		if !found {
			return nil, fmt.Errorf("Connector is unavailable")
		}
		matched := false
		for _, op := range definition.Operations {
			if op.ID == operation.String() {
				matched = true
				data["provider_id"] = definition.ID
				data["tool_name"] = op.ToolName
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("Connector operation is unavailable")
		}
		var binding uuid.UUID
		if value := workflowString(ref, "binding_id"); value != "" {
			binding, err = uuid.Parse(value)
			if err != nil || binding == uuid.Nil {
				return nil, fmt.Errorf("Connector binding is invalid")
			}
		}
		authorizationMode := strings.TrimSpace(workflowString(ref, "authorization_mode"))
		if authorizationMode != "" && authorizationMode != core.AuthorizationModeAsk && authorizationMode != core.AuthorizationModeDirect {
			return nil, fmt.Errorf("Connector authorization mode is invalid")
		}
		result = append(result, workflowConnectorSelection{NodeID: nodeID, OwnerID: owner, ConnectionID: connection, ConnectorID: connector, OperationID: operation, BindingID: binding, AuthorizationMode: authorizationMode})
	}
	return result, nil
}
func validateWorkflowConnectorNodesConfigured(graph map[string]interface{}) error {
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		data := workflowNodeData(node)
		if workflowString(data, "provider_type") != "connector" {
			continue
		}
		ref, ok := data["connector"].(map[string]interface{})
		if !ok || workflowString(ref, "connection_id") == "" || workflowString(ref, "binding_id") == "" || data["connector_unavailable_ref"] != nil {
			return fmt.Errorf("Connector node %s must be replaced before publishing", workflowString(node, "id"))
		}
	}
	return nil
}
func rewriteWorkflowConnectorIDs(workflow *Workflow, bindings []core.TargetBinding) error {
	graph := workflow.GetGraphDict()
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return err
	}
	byNode := map[string]core.TargetBinding{}
	for _, binding := range bindings {
		byNode[binding.Target.BindingInstanceID] = binding
	}
	for _, node := range nodes {
		data := workflowNodeData(node)
		if workflowString(data, "provider_type") != "connector" {
			continue
		}
		ref, ok := data["connector"].(map[string]interface{})
		if !ok {
			continue
		}
		binding, found := byNode[workflowString(node, "id")]
		if found {
			ref["binding_id"] = binding.ID.String()
			ref["connection_id"] = binding.ConnectionID.String()
			ref["connector_id"] = binding.ConnectorID.String()
			ref["revision"] = binding.Revision
		} else {
			delete(ref, "binding_id")
		}
	}
	encoded, err := json.Marshal(graph)
	if err == nil {
		workflow.Graph = string(encoded)
	}
	return err
}
func workflowConnectorGraphNodes(graph map[string]interface{}) ([]map[string]interface{}, error) {
	if len(graph) == 0 {
		return []map[string]interface{}{}, nil
	}
	raw, exists := graph["nodes"]
	if !exists || raw == nil {
		return []map[string]interface{}{}, nil
	}
	switch typed := raw.(type) {
	case []interface{}:
		nodes := make([]map[string]interface{}, 0, len(typed))
		for index, item := range typed {
			node, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("workflow graph node at index %d is not an object", index)
			}
			nodes = append(nodes, node)
		}
		return nodes, nil
	case []map[string]interface{}:
		return append([]map[string]interface{}(nil), typed...), nil
	default:
		return nil, fmt.Errorf("workflow graph nodes must be an array")
	}
}

func workflowString(values map[string]interface{}, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, _ := values[key].(string)
	return value
}
