package end

import (
	"context"
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestEndNode_New(t *testing.T) {
	config := map[string]any{
		"id": "end-node-1",
		"data": map[string]any{
			"outputs": []any{
				map[string]any{
					"variable":       "final_result",
					"value_selector": []any{"llm_node_1", "text"},
				},
				map[string]any{
					"variable": "status",
					"selector": []any{"process_node_1", "status"},
				},
			},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		WorkflowID:     "test-workflow",
		UserID:         "test-user",
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
		CallDepth:      0,
	}

	graph := &entities.Graph{}

	// Create system variables
	systemVars := &entities.SystemVariable{
		UserID:     "test-user",
		AppID:      "test-app",
		WorkflowID: "test-workflow",
	}

	// Create variable pool, simulate some output variables
	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs:         make(map[string]any),
		SystemVariables:    systemVars,
	}

	// Add some test variables to variable pool
	variablePool.Add([]string{"llm_node_1", "text"}, "这是LLM生成的文本内容")
	variablePool.Add([]string{"process_node_1", "status"}, "completed")

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, graph, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create end node: %v", err)
	}

	endNode := node.(*Node)

	// Verify node basic information
	if endNode.NodeID != "end-node-1" {
		t.Errorf("Expected node ID 'end-node-1', got '%s'", endNode.NodeID)
	}

	if endNode.NodeType != shared.End {
		t.Errorf("Expected node type End, got %v", endNode.NodeType)
	}

	// Verify output variable parsing
	if len(endNode.NodeData.Outputs) != 2 {
		t.Errorf("Expected 2 outputs, got %d", len(endNode.NodeData.Outputs))
	}

	// Verify first output variable
	output1 := endNode.NodeData.Outputs[0]
	if output1.Variable != "final_result" {
		t.Errorf("Expected variable name 'final_result', got '%s'", output1.Variable)
	}
	if len(output1.ValueSelector) != 2 || output1.ValueSelector[0] != "llm_node_1" || output1.ValueSelector[1] != "text" {
		t.Errorf("Expected value_selector ['llm_node_1', 'text'], got %v", output1.ValueSelector)
	}

	// Verify second output variable
	output2 := endNode.NodeData.Outputs[1]
	if output2.Variable != "status" {
		t.Errorf("Expected variable name 'status', got '%s'", output2.Variable)
	}
	if len(output2.ValueSelector) != 2 || output2.ValueSelector[0] != "process_node_1" || output2.ValueSelector[1] != "status" {
		t.Errorf("Expected value_selector ['process_node_1', 'status'], got %v", output2.ValueSelector)
	}
}

func TestEndNode_ExecuteRun(t *testing.T) {
	// Create minimal configuration
	config := map[string]any{
		"id": "end-node-1",
		"data": map[string]any{
			"outputs": []any{
				map[string]any{
					"variable":       "result",
					"value_selector": []any{"llm_node_1", "text"},
				},
			},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		WorkflowType:   "workflow",
		WorkflowID:     "test-workflow",
		UserID:         "test-user",
	}

	// Create variable pool with test data
	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs:         make(map[string]any),
		SystemVariables: &entities.SystemVariable{
			UserID: "test-user",
			AppID:  "test-app",
		},
	}

	// Add test variables
	variablePool.Add([]string{"llm_node_1", "text"}, "生成的最终结果文本")

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create end node: %v", err)
	}

	endNode := node.(*Node)

	// Execute node
	result, err := endNode.executeRun(context.Background())
	if err != nil {
		t.Fatalf("Failed to execute end node: %v", err)
	}

	// Verify execution result
	if result.Status != shared.SUCCEEDED {
		t.Errorf("Expected status SUCCEEDED, got %v", result.Status)
	}

	// Verify output data
	if result.Outputs["result"] != "生成的最终结果文本" {
		t.Errorf("Expected result '生成的最终结果文本', got '%v'", result.Outputs["result"])
	}

	// Verify that in end node, inputs and outputs are the same
	if len(result.Inputs) != len(result.Outputs) {
		t.Errorf("Expected inputs length %d, got %d", len(result.Outputs), len(result.Inputs))
	}

	for key, value := range result.Outputs {
		if inputValue, exists := result.Inputs[key]; !exists {
			t.Errorf("Input key %s not found", key)
		} else if inputValue != value {
			t.Errorf("Input[%s] = %v, expected %v", key, inputValue, value)
		}
	}
}

func TestEndNode_Run(t *testing.T) {
	// Create basic configuration
	config := map[string]any{
		"id": "end-node-1",
		"data": map[string]any{
			"outputs": []any{
				map[string]any{
					"variable":       "final_output",
					"value_selector": []any{"prev_node", "result"},
				},
			},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		UserID:         "test-user",
	}

	// Create variable pool
	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs:         make(map[string]any),
		SystemVariables: &entities.SystemVariable{
			UserID: "test-user",
			AppID:  "test-app",
		},
	}

	// Add test data
	variablePool.Add([]string{"prev_node", "result"}, "工作流执行完成")

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create end node: %v", err)
	}

	// Create event channel
	eventChan := make(chan *shared.NodeEventCh, 10)

	// Run node
	err = node.Run(context.Background(), eventChan)
	if err != nil {
		t.Fatalf("Failed to run end node: %v", err)
	}

	// Verify event sequence
	events := make([]*shared.NodeEventCh, 0)
	close(eventChan)
	for event := range eventChan {
		events = append(events, event)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// Verify start event
	startEvent := events[0]
	if startEvent.Type != shared.EventTypeRunStarted {
		t.Errorf("Expected first event to be RunStarted, got %v", startEvent.Type)
	}
	if startEvent.NodeID != "end-node-1" {
		t.Errorf("Expected node ID 'end-node-1', got '%s'", startEvent.NodeID)
	}

	// Verify completion event
	completeEvent := events[1]
	if completeEvent.Type != shared.EventTypeRunCompleted {
		t.Errorf("Expected second event to be RunCompleted, got %v", completeEvent.Type)
	}
	if completeEvent.NodeID != "end-node-1" {
		t.Errorf("Expected node ID 'end-node-1', got '%s'", completeEvent.NodeID)
	}

	// Verify result data in completion event
	runCompletedData, ok := completeEvent.Data.(*shared.RunCompletedEvent)
	if !ok {
		t.Fatalf("Expected RunCompletedEvent data, got %T", completeEvent.Data)
	}

	if runCompletedData.RunResult.Status != shared.SUCCEEDED {
		t.Errorf("Expected result status SUCCEEDED, got %v", runCompletedData.RunResult.Status)
	}

	// Verify final output
	if runCompletedData.RunResult.Outputs["final_output"] != "工作流执行完成" {
		t.Errorf("Expected final_output '工作流执行完成', got '%v'", runCompletedData.RunResult.Outputs["final_output"])
	}
}

func TestEndNode_ExecuteRun_UsesNestedSelector(t *testing.T) {
	config := map[string]any{
		"id": "end-node-1",
		"data": map[string]any{
			"outputs": []any{
				map[string]any{
					"variable":       "result",
					"value_selector": []any{"source", "payload", "text"},
				},
			},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		UserID:         "test-user",
	}

	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs:         make(map[string]any),
		SystemVariables: &entities.SystemVariable{
			UserID: "test-user",
			AppID:  "test-app",
		},
	}
	variablePool.Add([]string{"source", "payload"}, map[string]any{
		"text": "nested value",
	})

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create end node: %v", err)
	}

	result, err := node.(*Node).executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}
	if got := result.Outputs["result"]; got != "nested value" {
		t.Fatalf("result = %#v, want %q", got, "nested value")
	}
}

func TestParseVariableSelector(t *testing.T) {
	// Test value_selector field
	data1 := map[string]any{
		"variable":       "test_var",
		"value_selector": []any{"node1", "output"},
	}

	selector1, err := parseVariableSelector(data1)
	if err != nil {
		t.Fatalf("Failed to parse variable selector: %v", err)
	}

	if selector1.Variable != "test_var" {
		t.Errorf("Expected variable 'test_var', got '%s'", selector1.Variable)
	}
	if len(selector1.ValueSelector) != 2 || selector1.ValueSelector[0] != "node1" || selector1.ValueSelector[1] != "output" {
		t.Errorf("Expected value_selector ['node1', 'output'], got %v", selector1.ValueSelector)
	}

	// Test selector field (backward compatibility)
	data2 := map[string]any{
		"variable": "test_var2",
		"selector": []any{"node2", "result"},
	}

	selector2, err := parseVariableSelector(data2)
	if err != nil {
		t.Fatalf("Failed to parse variable selector: %v", err)
	}

	if selector2.Variable != "test_var2" {
		t.Errorf("Expected variable 'test_var2', got '%s'", selector2.Variable)
	}
	if len(selector2.ValueSelector) != 2 || selector2.ValueSelector[0] != "node2" || selector2.ValueSelector[1] != "result" {
		t.Errorf("Expected value_selector ['node2', 'result'], got %v", selector2.ValueSelector)
	}

	// Test error cases
	invalidData := map[string]any{
		"variable": "test_var3",
		// Missing selector field
	}

	_, err = parseVariableSelector(invalidData)
	if err == nil {
		t.Error("Expected error for missing selector field")
	}
}
