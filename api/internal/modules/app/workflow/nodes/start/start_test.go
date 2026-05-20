package start

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestStartNode_New(t *testing.T) {
	config := map[string]any{
		"id": "start-node-1",
		"data": map[string]any{
			"variables": []any{
				map[string]any{
					"variable":    "user_input",
					"label":       "User Input",
					"description": "User query content",
					"type":        "text-input",
					"required":    true,
					"max_length":  500,
				},
				map[string]any{
					"variable":           "file_input",
					"label":              "File Input",
					"type":               "file",
					"required":           false,
					"allowed_types":      []any{"image", "document"},
					"allowed_extensions": []any{".jpg", ".png", ".pdf"},
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

	// Create variable pool
	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs: map[string]any{
			"user_input":   "This is user's test input",
			"number_input": 42,
		},
		SystemVariables:       systemVars,
		EnvironmentVariables:  make([]entities.Variable, 0),
		ConversationVariables: make([]entities.Variable, 0),
	}

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, graph, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create start node: %v", err)
	}

	startNode := node.(*Node)

	// Verify node basic information
	if startNode.NodeID != "start-node-1" {
		t.Errorf("Expected node ID 'start-node-1', got '%s'", startNode.NodeID)
	}

	if startNode.NodeType != shared.Start {
		t.Errorf("Expected node type Start, got %v", startNode.NodeType)
	}

	// Verify variable parsing
	if len(startNode.NodeData.Variables) != 2 {
		t.Errorf("Expected 2 variables, got %d", len(startNode.NodeData.Variables))
	}

	// Verify first variable
	var1 := startNode.NodeData.Variables[0]
	if var1.Val != "user_input" {
		t.Errorf("Expected variable name 'user_input', got '%s'", var1.Val)
	}
	if var1.Label != "User Input" {
		t.Errorf("Expected label 'User Input', got '%s'", var1.Label)
	}
	if var1.Kind != TextInput {
		t.Errorf("Expected type TextInput, got %v", var1.Kind)
	}
	if !var1.Required {
		t.Error("Expected variable to be required")
	}

	// Verify second variable (file type)
	var2 := startNode.NodeData.Variables[1]
	if var2.Val != "file_input" {
		t.Errorf("Expected variable name 'file_input', got '%s'", var2.Val)
	}
	if var2.Kind != File {
		t.Errorf("Expected type File, got %v", var2.Kind)
	}
	if var2.Required {
		t.Error("Expected file variable to be optional")
	}
	if len(var2.AllowFileTypes) != 2 {
		t.Errorf("Expected 2 allowed file types, got %d", len(var2.AllowFileTypes))
	}
}

func TestStartNode_ExecuteRun(t *testing.T) {
	// Create minimum configuration
	config := map[string]any{
		"id": "start-node-1",
		"data": map[string]any{
			"variables": []any{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		WorkflowType:   "workflow",
		WorkflowID:     "test-workflow",
		UserID:         "test-user",
	}

	// Create system variables
	systemVars := &entities.SystemVariable{
		UserID:     "test-user",
		AppID:      "test-app",
		WorkflowID: "test-workflow",
	}

	// Create variable pool with test data
	variablePool := &entities.VariablePool{
		VariableDictionary: make(map[string]map[string]entities.Variable),
		UserInputs: map[string]any{
			"query":     "User's question",
			"file_list": []any{"file1.txt", "file2.pdf"},
		},
		SystemVariables: systemVars,
	}

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create start node: %v", err)
	}

	startNode := node.(*Node)

	// Execute node
	result, err := startNode.executeRun(context.Background())
	if err != nil {
		t.Fatalf("Failed to execute start node: %v", err)
	}

	// Verify execution result
	if result.Status != shared.SUCCEEDED {
		t.Errorf("Expected status SUCCEEDED, got %v", result.Status)
	}

	// Verify input data
	if query, ok := result.Inputs["query"].(string); !ok || query != "User's question" {
		t.Errorf("Expected query 'User's question', got '%v'", result.Inputs["query"])
	}

	// Verify system variables have correct prefix
	if userID, ok := result.Inputs["sys.user_id"].(string); !ok || userID != "test-user" {
		t.Errorf("Expected sys.user_id 'test-user', got '%v'", result.Inputs["sys.user_id"])
	}
	if appID, ok := result.Inputs["sys.app_id"].(string); !ok || appID != "test-app" {
		t.Errorf("Expected sys.app_id 'test-app', got '%v'", result.Inputs["sys.app_id"])
	}

	// Verify output matches input
	if len(result.Outputs) != len(result.Inputs) {
		t.Errorf("Expected outputs length %d, got %d", len(result.Inputs), len(result.Outputs))
	}

	// Verify specific output values
	for key, value := range result.Inputs {
		if outputValue, exists := result.Outputs[key]; !exists {
			t.Errorf("Output key %s not found", key)
		} else {
			// Don't directly compare values because they may contain incomparable types
			if key == "query" {
				if str1, ok1 := value.(string); ok1 {
					if str2, ok2 := outputValue.(string); ok2 {
						if str1 != str2 {
							t.Errorf("Output[%s] = %v, expected %v", key, outputValue, value)
						}
					} else {
						t.Errorf("Output[%s] type mismatch", key)
					}
				}
			}
		}
	}
}

func TestStartNode_Run(t *testing.T) {
	// Create basic configuration
	config := map[string]any{
		"id": "start-node-1",
		"data": map[string]any{
			"variables": []any{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:          "test-app",
		UserID:         "test-user",
	}

	// Create variable pool
	variablePool := &entities.VariablePool{
		UserInputs: map[string]any{
			"test_input": "Test data",
		},
		SystemVariables: &entities.SystemVariable{
			UserID: "test-user",
			AppID:  "test-app",
		},
	}

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	node, err := New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create start node: %v", err)
	}

	// Create event channel
	eventChan := make(chan *shared.NodeEventCh, 10)

	// Run node
	err = node.Run(context.Background(), eventChan)
	if err != nil {
		t.Fatalf("Failed to run start node: %v", err)
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
	if startEvent.NodeID != "start-node-1" {
		t.Errorf("Expected node ID 'start-node-1', got '%s'", startEvent.NodeID)
	}

	// Verify completion event
	completeEvent := events[1]
	if completeEvent.Type != shared.EventTypeRunCompleted {
		t.Errorf("Expected second event to be RunCompleted, got %v", completeEvent.Type)
	}
	if completeEvent.NodeID != "start-node-1" {
		t.Errorf("Expected node ID 'start-node-1', got '%s'", completeEvent.NodeID)
	}

	// Verify result data in completion event
	runCompletedData, ok := completeEvent.Data.(*shared.RunCompletedEvent)
	if !ok {
		t.Fatalf("Expected RunCompletedEvent data, got %T", completeEvent.Data)
	}

	if runCompletedData.RunResult.Status != shared.SUCCEEDED {
		t.Errorf("Expected result status SUCCEEDED, got %v", runCompletedData.RunResult.Status)
	}
}
