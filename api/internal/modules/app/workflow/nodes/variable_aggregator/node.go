package variableaggregator

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/logger"
)

// Node represents a variable aggregator node
type Node struct {
	base.NodeStruct
	NodeData
}

// New creates a new variable aggregator node instance
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := parseVariableAggregatorNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.VariableAggregator,

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

// Run executes the variable aggregator node
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Send start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Execute aggregation logic
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

	// Send completion event
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

// executeRun performs the variable aggregation logic
func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	startTime := time.Now()
	variablePool := n.GraphRuntimeState.VariablePool

	isMultiGroup := n.AdvancedSettings != nil && n.AdvancedSettings.GroupEnabled
	variableCount := len(n.Variables)
	if isMultiGroup {
		variableCount = 0
		for _, group := range n.AdvancedSettings.Groups {
			variableCount += len(group.Variables)
		}
	}

	logger.Info("Variable aggregator node starting",
		"node_id", n.NodeID,
		"tenant_id", n.TenantID,
		"app_id", n.APPID,
		"workflow_id", n.WorkflowID,
		"multi_group", isMultiGroup,
		"total_variables", variableCount,
	)

	var outputs map[string]any
	var inputs map[string]any
	var processData map[string]any

	// Check if multi-group mode is enabled
	if n.AdvancedSettings != nil && n.AdvancedSettings.GroupEnabled {
		// Multi-group mode
		outputs, inputs, processData = n.executeMultiGroupMode(variablePool)
	} else {
		// Single-group mode
		outputs, inputs, processData = n.executeSingleGroupMode(variablePool)
	}

	executionTime := time.Since(startTime).Milliseconds()
	processData["execution_time_ms"] = executionTime

	logger.Info("Variable aggregator node completed",
		"node_id", n.NodeID,
		"tenant_id", n.TenantID,
		"app_id", n.APPID,
		"workflow_id", n.WorkflowID,
		"execution_time_ms", executionTime,
		"status", shared.SUCCEEDED,
	)

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      inputs,
		ProcessData: processData,
		Outputs:     outputs,
	}, nil
}

// executeSingleGroupMode executes single-group aggregation
func (n *Node) executeSingleGroupMode(variablePool *entities.VariablePool) (map[string]any, map[string]any, map[string]any) {
	outputs := make(map[string]any)
	inputs := make(map[string]any)
	processData := make(map[string]any)

	logger.Debug("Executing single-group mode",
		"node_id", n.NodeID,
		"tenant_id", n.TenantID,
		"variable_count", len(n.Variables),
		"output_type", n.OutputType,
	)

	// Check if we have multiple variables - if so, aggregate all of them
	// Multiple variables are automatically aggregated into a single output
	if len(n.Variables) > 1 {
		// Multi-variable mode: aggregate all variables
		selectedVars := []string{}
		aggregatedOutput := make(map[string]any)

		for i, selector := range n.Variables {
			variable := variablePool.GetWithPath(selector)

			// Skip if variable not found
			if variable == nil {
				logger.Debug("Variable not found",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"selector", n.selectorToString(selector),
					"selector_path", selector,
					"index", i,
				)
				continue
			}

			// Check type compatibility
			if !n.isTypeCompatible(variable.GetType(), n.OutputType) {
				logger.Debug("Variable type incompatible",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"selector", n.selectorToString(selector),
					"variable_type", variable.GetType(),
					"expected_type", n.OutputType,
					"index", i,
				)
				continue
			}

			// Get the variable name (last part of selector)
			varName := selector[len(selector)-1]
			aggregatedOutput[varName] = variable.ToObject()
			inputs[n.selectorToString(selector)] = variable.ToObject()
			selectedVars = append(selectedVars, n.selectorToString(selector))

			logger.Info("Variable aggregated",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"selector", n.selectorToString(selector),
				"var_name", varName,
				"type", variable.GetType(),
				"index", i,
			)
		}

		// Store aggregated output
		if len(aggregatedOutput) > 0 {
			outputs["output"] = aggregatedOutput
			processData["selected_variables"] = selectedVars
			processData["count"] = len(selectedVars)
			processData["mode"] = "aggregate_all"

			logger.Info("Variables aggregated",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"count", len(selectedVars),
				"variables", selectedVars,
			)
		} else {
			processData["selected_variable"] = "none"
			processData["mode"] = "aggregate_all"
			logger.Info("No valid variables found for aggregation",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"checked_count", len(n.Variables),
				"output_type", n.OutputType,
			)
		}
	} else {
		// Single variable mode: priority selection (original behavior)
		for i, selector := range n.Variables {
			variable := variablePool.GetWithPath(selector)

			// Skip if variable not found
			if variable == nil {
				logger.Debug("Variable not found",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"selector", n.selectorToString(selector),
					"selector_path", selector,
					"index", i,
				)
				continue
			}

			// Check type compatibility
			if !n.isTypeCompatible(variable.GetType(), n.OutputType) {
				logger.Debug("Variable type incompatible",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"selector", n.selectorToString(selector),
					"variable_type", variable.GetType(),
					"expected_type", n.OutputType,
					"index", i,
				)
				continue
			}

			// Found first valid variable
			outputs["output"] = variable.ToObject()
			inputs[n.selectorToString(selector)] = variable.ToObject()
			processData["selected_variable"] = n.selectorToString(selector)
			processData["selected_index"] = i
			processData["variable_type"] = string(variable.GetType())
			processData["mode"] = "priority_selection"

			logger.Info("Variable selected",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"selector", n.selectorToString(selector),
				"type", variable.GetType(),
				"index", i,
				"output_type", n.OutputType,
			)

			break
		}

		// If no variable was selected, outputs remain empty
		if len(outputs) == 0 {
			processData["selected_variable"] = "none"
			processData["mode"] = "priority_selection"
			logger.Info("No valid variable found",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"checked_count", len(n.Variables),
				"output_type", n.OutputType,
			)
		}
	}

	return outputs, inputs, processData
}

// executeMultiGroupMode executes multi-group aggregation
func (n *Node) executeMultiGroupMode(variablePool *entities.VariablePool) (map[string]any, map[string]any, map[string]any) {
	outputs := make(map[string]any)
	inputs := make(map[string]any)
	processData := make(map[string]any)
	groupResults := make(map[string]any)

	logger.Debug("Executing multi-group mode",
		"node_id", n.NodeID,
		"tenant_id", n.TenantID,
		"group_count", len(n.AdvancedSettings.Groups),
	)

	// Process each group independently
	for _, group := range n.AdvancedSettings.Groups {
		logger.Debug("Processing group",
			"node_id", n.NodeID,
			"tenant_id", n.TenantID,
			"group_name", group.GroupName,
			"variable_count", len(group.Variables),
			"output_type", group.OutputType,
		)
		groupOutput := make(map[string]any)
		selectedVars := []string{}

		// Auto-detect mode: if multiple variables, aggregate all; if single variable, select it
		shouldAggregateAll := len(group.Variables) > 1

		// Iterate through variables in this group
		for i, selector := range group.Variables {
			variable := variablePool.GetWithPath(selector)

			// Skip if variable not found
			if variable == nil {
				logger.Debug("Variable not found in group",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"group", group.GroupName,
					"selector", n.selectorToString(selector),
					"selector_path", selector,
					"index", i,
				)
				continue
			}

			// Check type compatibility
			if !n.isTypeCompatible(variable.GetType(), group.OutputType) {
				logger.Debug("Variable type incompatible in group",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"group", group.GroupName,
					"selector", n.selectorToString(selector),
					"variable_type", variable.GetType(),
					"expected_type", group.OutputType,
					"index", i,
				)
				continue
			}

			// Get the variable name (last part of selector)
			varName := selector[len(selector)-1]

			if shouldAggregateAll {
				// Multi-variable mode: aggregate all variables
				groupOutput[varName] = variable.ToObject()
				inputs[fmt.Sprintf("%s.%s", group.GroupName, n.selectorToString(selector))] = variable.ToObject()
				selectedVars = append(selectedVars, n.selectorToString(selector))

				logger.Info("Variable aggregated in group",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"group", group.GroupName,
					"selector", n.selectorToString(selector),
					"var_name", varName,
					"type", variable.GetType(),
					"index", i,
				)
			} else {
				// Single-variable mode: select first valid variable
				groupOutput["output"] = variable.ToObject()
				inputs[fmt.Sprintf("%s.%s", group.GroupName, n.selectorToString(selector))] = variable.ToObject()
				groupResults[group.GroupName] = map[string]any{
					"selected_variable": n.selectorToString(selector),
					"selected_index":    i,
					"variable_type":     string(variable.GetType()),
				}

				logger.Info("Variable selected for group",
					"node_id", n.NodeID,
					"tenant_id", n.TenantID,
					"group", group.GroupName,
					"selector", n.selectorToString(selector),
					"type", variable.GetType(),
					"index", i,
					"output_type", group.OutputType,
				)

				break
			}
		}

		// For multi-variable mode, record all selected variables
		if shouldAggregateAll && len(selectedVars) > 0 {
			groupResults[group.GroupName] = map[string]any{
				"selected_variables": selectedVars,
				"count":              len(selectedVars),
				"mode":               "aggregate_all",
			}
		}

		// Store group output (even if empty). For multi-variable groups, also expose a
		// stable nested `output` payload so selectors like `group.output` can resolve
		// the entire aggregated object.
		outputs[group.GroupName] = buildGroupOutput(groupOutput, shouldAggregateAll)

		// Log if no variable was selected for this group
		if len(groupOutput) == 0 {
			groupResults[group.GroupName] = map[string]any{
				"selected_variable": "none",
			}
			logger.Info("No valid variable found for group",
				"node_id", n.NodeID,
				"tenant_id", n.TenantID,
				"group", group.GroupName,
				"checked_count", len(group.Variables),
				"output_type", group.OutputType,
			)
		}
	}

	processData["group_results"] = groupResults
	processData["group_count"] = len(n.AdvancedSettings.Groups)

	return outputs, inputs, processData
}

func buildGroupOutput(groupOutput map[string]any, shouldAggregateAll bool) map[string]any {
	if len(groupOutput) == 0 {
		return groupOutput
	}
	if !shouldAggregateAll {
		return groupOutput
	}

	return map[string]any{
		"output": cloneAnyMap(groupOutput),
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

// isTypeCompatible checks if a variable type is compatible with the expected output type
func (n *Node) isTypeCompatible(variableType, expectedType shared.SegmentType) bool {
	// "any" type accepts all types
	if expectedType == shared.SegmentTypeAny {
		return true
	}

	// Variables with "any" type are accepted by all expected types
	if variableType == shared.SegmentTypeAny {
		return true
	}

	// Exact type match required
	return variableType == expectedType
}

// selectorToString converts a selector array to a readable string
func (n *Node) selectorToString(selector []string) string {
	if len(selector) == 0 {
		return ""
	}
	result := selector[0]
	for i := 1; i < len(selector); i++ {
		result += "." + selector[i]
	}
	return result
}
