package end

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/database"
)

// VariableSelector represents a variable selector
type VariableSelector struct {
	Variable      string   `json:"variable"`
	ValueSelector []string `json:"value_selector"`
}

// NodeData represents the data for an end node
type NodeData struct {
	base.NodeData
	Outputs []VariableSelector
}

type Node struct {
	base.NodeStruct
	NodeData
}

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := getData(config)
	if err != nil {
		return nil, err
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.End,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		NodeData: nd,
	}, nil

}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		// Send failure event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	// Get output variable configuration
	outputVariables := n.NodeData.Outputs
	outputs := make(map[string]any)

	for _, variableSelector := range outputVariables {
		// Skip empty variable names
		if variableSelector.Variable == "" {
			continue
		}

		// Skip system variables that start with "sys."
		if strings.HasPrefix(variableSelector.Variable, base.SYSTEM_VARIABLE_NODE_ID+".") {
			continue
		}

		// Get variables from variable pool
		variable := n.GraphRuntimeState.VariablePool.GetWithPath(variableSelector.ValueSelector)
		// Convert variable value
		var value any
		if variable != nil {
			value = variable.ToObject()
		} else {
			value = nil
		}

		// Set output values
		outputs[variableSelector.Variable] = value
	}

	// Persist conversation variables (not end-node outputs) for advanced-chat workflows
	n.persistConversationVariablesIfNeeded(ctx)

	// Return node run result
	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  outputs, // In end node, inputs and outputs are the same
		Outputs: outputs,
	}, nil
}

// persistConversationVariablesIfNeeded saves the actual conversation variables
// from VariablePool (not end-node outputs) to persistent storage.
func (n *Node) persistConversationVariablesIfNeeded(ctx context.Context) {
	if n.WorkflowType != "advanced-chat" {
		return
	}

	sysVars := n.GraphRuntimeState.VariablePool.SystemVariables
	if sysVars == nil {
		return
	}

	if sysVars.ConversationID == "" || sysVars.AppID == "" {
		return
	}

	conversationID, err := uuid.Parse(sysVars.ConversationID)
	if err != nil {
		return
	}

	appID, err := uuid.Parse(sysVars.AppID)
	if err != nil {
		return
	}

	convVars := n.extractConversationVariables()
	if len(convVars) == 0 {
		return
	}

	_ = n.saveConversationVariables(ctx, conversationID, appID, convVars)
}

// extractConversationVariables extracts actual conversation variables from
// the VariablePool's VariableDictionary, not from end node outputs.
func (n *Node) extractConversationVariables() map[string]interface{} {
	pool := n.GraphRuntimeState.VariablePool
	if pool == nil {
		return nil
	}

	conversationDict, exists := pool.VariableDictionary[entities.ConversationVariableNodeId]
	if !exists || len(conversationDict) == 0 {
		return nil
	}

	variables := make(map[string]interface{}, len(conversationDict))
	for name, variable := range conversationDict {
		if variable != nil {
			variables[name] = variable.GetValue()
		}
	}
	return variables
}

func (n *Node) saveConversationVariables(ctx context.Context, conversationID, appID uuid.UUID, variables map[string]interface{}) error {
	db := database.GetDB()
	variableRepo := conversation.NewWorkflowConversationVariableRepository(db)
	variableService := conversation.NewWorkflowConversationVariableService(variableRepo)

	return variableService.SaveConversationVariables(ctx, conversationID, appID, variables)
}

func getData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	dataMap, ok := data.(map[string]any)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data must be object")
	}

	var outputs []VariableSelector
	if outputsData, exists := dataMap["outputs"]; exists {
		if outputsList, ok := outputsData.([]any); ok {
			for _, outputData := range outputsList {
				if outputMap, ok := outputData.(map[string]any); ok {
					variableSelector, err := parseVariableSelector(outputMap)
					if err != nil {
						continue
					}
					outputs = append(outputs, variableSelector)
				}
			}
		}
	}

	nodeData := NodeData{
		Outputs: outputs,
	}

	return nodeData, nodeIDStr, nil
}

func parseVariableSelector(data map[string]any) (VariableSelector, error) {
	var variableSelector VariableSelector

	if variable, ok := data["variable"].(string); ok {
		variableSelector.Variable = variable
	} else {
		return VariableSelector{}, fmt.Errorf("variable field is required and must be string")
	}

	var selectorData any
	if valueSelector, exists := data["value_selector"]; exists {
		selectorData = valueSelector
	} else if selector, exists := data["selector"]; exists {
		selectorData = selector
	} else {
		return VariableSelector{}, fmt.Errorf("value_selector or selector field is required")
	}

	if selectorArray, ok := selectorData.([]any); ok {
		valSelector := make([]string, 0, len(selectorArray))
		for _, item := range selectorArray {
			if str, ok := item.(string); ok {
				valSelector = append(valSelector, str)
			}
		}
		variableSelector.ValueSelector = valSelector
	} else {
		return VariableSelector{}, fmt.Errorf("selector must be array")
	}

	return variableSelector, nil
}
