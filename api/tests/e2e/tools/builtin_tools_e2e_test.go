package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/tools"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	toolspkg "github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"

	// Import to trigger init() registration
	_ "github.com/zgiai/ginext/internal/modules/tools/builtin/time"
)

// TestToolsNode_BuiltinTime tests the Tools node with builtin time tool
func TestToolsNode_BuiltinTime(t *testing.T) {
	ctx := context.Background()

	// Create tool manager with builtin providers
	toolManager := toolspkg.NewToolManager(nil)
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Create tool engine
	toolEngine := toolspkg.NewToolEngine(toolManager)
	require.NotNil(t, toolEngine)

	// Create variable pool with some test data
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "user_name"}, "TestUser")

	// Create graph runtime state
	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration for builtin time tool
	nodeConfig := map[string]any{
		"id": "tools-node-1",
		"data": map[string]interface{}{
			"title":         "Get Current Time",
			"provider_type": "builtin",
			"provider_id":   "time",
			"tool_name":     "current_time",
			"tool_parameters": map[string]interface{}{
				"timezone": map[string]interface{}{
					"type":  "constant",
					"value": "Asia/Shanghai",
				},
				"format": map[string]interface{}{
					"type":  "constant",
					"value": "2006-01-02 15:04:05",
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	// Create graph init params
	graphInitParams := entities.GraphInitParams{
		OrganizationID:     "test-tenant-123",
		AppID:        "test-app-123",
		WorkflowID:   "test-workflow-123",
		UserID:       "test-user-123",
		WorkflowType: entities.WorkflowTypeWorkflow,
		UserFrom:     entities.UserFromAccount,
		InvokeFrom:   entities.InvokeFromDebugger,
	}

	// Create graph
	graph := &entities.Graph{}

	// Create node
	node, err := tools.New(
		"instance-1",
		nodeConfig,
		graphInitParams,
		graph,
		graphRuntimeState,
		nil,
		toolEngine,
	)
	require.NoError(t, err)
	require.NotNil(t, node)

	// Create event channel
	eventChan := make(chan *shared.NodeEventCh, 100)

	// Run the node
	err = node.Run(ctx, eventChan)
	require.NoError(t, err)

	// Collect events
	var events []*shared.NodeEventCh
	close(eventChan)
	for event := range eventChan {
		events = append(events, event)
	}

	// Verify events
	require.GreaterOrEqual(t, len(events), 2, "Should have at least start and complete events")

	// Check start event
	assert.Equal(t, shared.EventTypeRunStarted, events[0].Type)

	// Check completion event (last non-stream event)
	var completionEvent *shared.NodeEventCh
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == shared.EventTypeRunCompleted {
			completionEvent = events[i]
			break
		}
	}
	require.NotNil(t, completionEvent, "Should have completion event")

	// Verify result
	runResult := completionEvent.Data.(*shared.RunCompletedEvent).RunResult
	require.NotNil(t, runResult)
	assert.Equal(t, shared.SUCCEEDED, runResult.Status)

	// Verify output contains text
	outputs := runResult.Outputs
	require.NotNil(t, outputs)

	text, ok := outputs["text"].(string)
	require.True(t, ok, "Output should contain text")
	assert.NotEmpty(t, text, "Text output should not be empty")
	t.Logf("Time output: %s", text)
}

// TestToolsNode_VariableParameter tests variable parameter resolution
func TestToolsNode_VariableParameter(t *testing.T) {
	ctx := context.Background()

	// Create tool manager with builtin providers
	toolManager := toolspkg.NewToolManager(nil)
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Create tool engine
	toolEngine := toolspkg.NewToolEngine(toolManager)

	// Create variable pool with timezone from previous node
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "timezone"}, "America/New_York")

	// Create graph runtime state
	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration with variable parameter
	nodeConfig := map[string]any{
		"id": "tools-node-2",
		"data": map[string]interface{}{
			"title":         "Get Time with Variable TZ",
			"provider_type": "builtin",
			"provider_id":   "time",
			"tool_name":     "current_time",
			"tool_parameters": map[string]interface{}{
				"timezone": map[string]interface{}{
					"type":  "variable",
					"value": []string{"start", "timezone"}, // Reference to variable
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID:     "test-tenant-123",
		AppID:        "test-app-123",
		WorkflowID:   "test-workflow-123",
		UserID:       "test-user-123",
		WorkflowType: entities.WorkflowTypeWorkflow,
		UserFrom:     entities.UserFromAccount,
		InvokeFrom:   entities.InvokeFromDebugger,
	}

	graph := &entities.Graph{}

	node, err := tools.New(
		"instance-2",
		nodeConfig,
		graphInitParams,
		graph,
		graphRuntimeState,
		nil,
		toolEngine,
	)
	require.NoError(t, err)

	eventChan := make(chan *shared.NodeEventCh, 100)

	err = node.Run(ctx, eventChan)
	require.NoError(t, err)

	// Collect and verify
	close(eventChan)
	var completionEvent *shared.NodeEventCh
	for event := range eventChan {
		if event.Type == shared.EventTypeRunCompleted {
			completionEvent = event
		}
	}
	require.NotNil(t, completionEvent)

	runResult := completionEvent.Data.(*shared.RunCompletedEvent).RunResult
	assert.Equal(t, shared.SUCCEEDED, runResult.Status)
	assert.NotEmpty(t, runResult.Outputs["text"])
	t.Logf("Time output (NY timezone): %s", runResult.Outputs["text"])
}

// TestToolsNode_MixedParameter tests mixed parameter resolution with template
func TestToolsNode_MixedParameter(t *testing.T) {
	ctx := context.Background()

	// Create tool manager with builtin providers
	toolManager := toolspkg.NewToolManager(nil)
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Create tool engine
	toolEngine := toolspkg.NewToolEngine(toolManager)

	// Create variable pool
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "date_format"}, "2006-01-02")
	variablePool.Add([]string{"start", "separator"}, " | ")
	variablePool.Add([]string{"start", "time_format"}, "15:04:05")

	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration with mixed parameter (template)
	nodeConfig := map[string]any{
		"id": "tools-node-3",
		"data": map[string]interface{}{
			"title":         "Get Time with Mixed Format",
			"provider_type": "builtin",
			"provider_id":   "time",
			"tool_name":     "current_time",
			"tool_parameters": map[string]interface{}{
				"format": map[string]interface{}{
					"type":  "mixed",
					"value": "{{#start.date_format#}}{{#start.separator#}}{{#start.time_format#}}",
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID:     "test-tenant-123",
		AppID:        "test-app-123",
		WorkflowID:   "test-workflow-123",
		UserID:       "test-user-123",
		WorkflowType: entities.WorkflowTypeWorkflow,
		UserFrom:     entities.UserFromAccount,
		InvokeFrom:   entities.InvokeFromDebugger,
	}

	graph := &entities.Graph{}

	node, err := tools.New(
		"instance-3",
		nodeConfig,
		graphInitParams,
		graph,
		graphRuntimeState,
		nil,
		toolEngine,
	)
	require.NoError(t, err)

	eventChan := make(chan *shared.NodeEventCh, 100)

	err = node.Run(ctx, eventChan)
	require.NoError(t, err)

	close(eventChan)
	var completionEvent *shared.NodeEventCh
	for event := range eventChan {
		if event.Type == shared.EventTypeRunCompleted {
			completionEvent = event
		}
	}
	require.NotNil(t, completionEvent)

	runResult := completionEvent.Data.(*shared.RunCompletedEvent).RunResult
	assert.Equal(t, shared.SUCCEEDED, runResult.Status)

	text := runResult.Outputs["text"].(string)
	assert.NotEmpty(t, text)
	assert.Contains(t, text, " | ", "Should contain the separator from variable")
	t.Logf("Time output (mixed format): %s", text)
}
