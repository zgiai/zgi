package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// workflowConnectorAction is the provider-authoritative identity of one
// connector tool. Workflow graphs persist the provider tool name, while the
// binding ledger must retain the stable Action ID used by runtime governance.
type workflowConnectorAction struct {
	ActionID   string
	AccessMode string
}

type workflowConnectorActionResolver func(
	ctx context.Context,
	integrationID string,
	toolName string,
) (workflowConnectorAction, error)

// workflowConnectorDependency is one configured connector node in a Workflow
// graph. NodeID is part of the authorization identity: two nodes that happen to
// use the same connection are still separate binding instances.
type workflowConnectorDependency struct {
	NodeID        string
	NodeTitle     string
	IntegrationID string
	ToolName      string
	ActionID      string
	ConnectionID  string
	AccessMode    string
}

func extractWorkflowConnectorDependencies(
	ctx context.Context,
	graph map[string]interface{},
	resolve workflowConnectorActionResolver,
) ([]workflowConnectorDependency, error) {
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return []workflowConnectorDependency{}, nil
	}
	if resolve == nil {
		return nil, fmt.Errorf("workflow connector Action resolver is required")
	}

	dependencies := make([]workflowConnectorDependency, 0)
	seenNodeIDs := make(map[string]struct{}, len(nodes))
	for index, node := range nodes {
		data := workflowNodeData(node)
		if !strings.EqualFold(strings.TrimSpace(workflowString(data, "provider_type")), "connector") {
			continue
		}

		nodeID := strings.TrimSpace(workflowString(node, "id"))
		if nodeID == "" {
			return nil, fmt.Errorf("workflow connector node at index %d has no id", index)
		}
		if _, exists := seenNodeIDs[nodeID]; exists {
			return nil, fmt.Errorf("workflow connector node id %q is duplicated", nodeID)
		}
		seenNodeIDs[nodeID] = struct{}{}

		connectionID := strings.TrimSpace(workflowString(data, "connection_id"))
		// An unconfigured connector node is allowed in a draft/import, but it
		// has no executable dependency and therefore must not create a ledger row.
		if connectionID == "" {
			continue
		}
		parsedConnectionID, parseErr := uuid.Parse(connectionID)
		if parseErr != nil || parsedConnectionID == uuid.Nil {
			return nil, fmt.Errorf("workflow connector node %q has an invalid connection_id", nodeID)
		}

		integrationID := strings.ToLower(strings.TrimSpace(workflowString(data, "provider_id")))
		toolName := strings.TrimSpace(workflowString(data, "tool_name"))
		if integrationID == "" || toolName == "" {
			return nil, fmt.Errorf("workflow connector node %q requires provider_id and tool_name", nodeID)
		}
		action, resolveErr := resolve(ctx, integrationID, toolName)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve workflow connector node %q Action: %w", nodeID, resolveErr)
		}
		actionID := strings.ToLower(strings.TrimSpace(action.ActionID))
		if actionID == "" {
			return nil, fmt.Errorf("workflow connector node %q resolved to an empty Action ID", nodeID)
		}
		accessMode := strings.ToLower(strings.TrimSpace(action.AccessMode))
		if accessMode != "read" && accessMode != "write" {
			return nil, fmt.Errorf("workflow connector node %q resolved to invalid access mode %q", nodeID, action.AccessMode)
		}

		title := strings.TrimSpace(workflowString(data, "title"))
		if title == "" {
			title = strings.TrimSpace(workflowString(data, "tool_label"))
		}
		if title == "" {
			title = toolName
		}
		dependencies = append(dependencies, workflowConnectorDependency{
			NodeID:        nodeID,
			NodeTitle:     title,
			IntegrationID: integrationID,
			ToolName:      toolName,
			ActionID:      actionID,
			ConnectionID:  parsedConnectionID.String(),
			AccessMode:    accessMode,
		})
	}

	sort.Slice(dependencies, func(i, j int) bool {
		if dependencies[i].NodeID != dependencies[j].NodeID {
			return dependencies[i].NodeID < dependencies[j].NodeID
		}
		if dependencies[i].IntegrationID != dependencies[j].IntegrationID {
			return dependencies[i].IntegrationID < dependencies[j].IntegrationID
		}
		return dependencies[i].ConnectionID < dependencies[j].ConnectionID
	})
	return dependencies, nil
}

// validateWorkflowConnectorNodesConfigured is a publish-time gate. Drafts and
// imports may keep a connector node while its connection is being configured,
// but an executable published graph must never contain such a node.
func validateWorkflowConnectorNodesConfigured(graph map[string]interface{}) error {
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return err
	}
	for index, node := range nodes {
		data := workflowNodeData(node)
		if !strings.EqualFold(strings.TrimSpace(workflowString(data, "provider_type")), "connector") {
			continue
		}
		nodeID := strings.TrimSpace(workflowString(node, "id"))
		if nodeID == "" {
			nodeID = fmt.Sprintf("index %d", index)
		}
		if strings.TrimSpace(workflowString(data, "connection_id")) == "" {
			return fmt.Errorf("workflow connector node %q must be reconnected before publishing", nodeID)
		}
	}
	return nil
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

// clearWorkflowConnectorConnection removes one connection only from the
// selected connector node instances. It deliberately keeps provider/tool
// configuration so the draft remains editable and can be reconnected later.
func clearWorkflowConnectorConnection(
	graph map[string]interface{},
	connectionID string,
	nodeIDs map[string]struct{},
) (bool, error) {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return false, fmt.Errorf("workflow connector connection id is required")
	}
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return false, err
	}
	changed := false
	for _, node := range nodes {
		nodeID := strings.TrimSpace(workflowString(node, "id"))
		if len(nodeIDs) > 0 {
			if _, selected := nodeIDs[nodeID]; !selected {
				continue
			}
		}
		data := workflowNodeData(node)
		if !strings.EqualFold(strings.TrimSpace(workflowString(data, "provider_type")), "connector") ||
			!strings.EqualFold(strings.TrimSpace(workflowString(data, "connection_id")), connectionID) {
			continue
		}
		delete(data, "connection_id")
		changed = true
	}
	return changed, nil
}

func clearAllWorkflowConnectorConnections(graph map[string]interface{}) ([]string, error) {
	nodes, err := workflowConnectorGraphNodes(graph)
	if err != nil {
		return nil, err
	}
	cleared := make([]string, 0)
	for _, node := range nodes {
		data := workflowNodeData(node)
		if !strings.EqualFold(strings.TrimSpace(workflowString(data, "provider_type")), "connector") {
			continue
		}
		if strings.TrimSpace(workflowString(data, "connection_id")) == "" {
			continue
		}
		delete(data, "connection_id")
		cleared = append(cleared, strings.TrimSpace(workflowString(node, "id")))
	}
	sort.Strings(cleared)
	return cleared, nil
}
