package nodes

import (
	"context"
	"testing"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/httprequest"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/start"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// TestRebuiltFilesIntegration tests integration of rebuilt files
func TestRebuiltFilesIntegration(t *testing.T) {
	// Test start node creation and execution
	t.Run("StartNodeIntegration", func(t *testing.T) {
		testStartNodeIntegration(t)
	})

	// Test HTTP request node creation
	t.Run("HTTPRequestNodeCreation", func(t *testing.T) {
		testHTTPRequestNodeCreation(t)
	})

	// Test factory patterns
	t.Run("FactoryPatterns", func(t *testing.T) {
		testFactoryPatterns(t)
	})
}

func testStartNodeIntegration(t *testing.T) {
	// Create start node configuration
	config := map[string]any{
		"id": "start-test",
		"data": map[string]any{
			"variables": []any{
				map[string]any{
					"variable":    "test_input",
					"label":       "Test Input",
					"description": "Test input description",
					"type":        "text-input",
					"required":    true,
				},
			},
		},
	}

	// Create graph runtime environment
	graphInitParams := entities.GraphInitParams{
		OrganizationID:     "test-tenant",
		AppID:        "test-app",
		WorkflowType: entities.WorkflowTypeWorkflow,
		WorkflowID:   "test-workflow",
		UserID:       "test-user",
		UserFrom:     entities.UserFromAccount,
		InvokeFrom:   entities.InvokeFromWebApp,
		CallDepth:    0,
		GraphConfig:  make(map[string]interface{}),
	}

	// Create variable pool
	variablePool := entities.NewVariablePool()
	variablePool.UserInputs["test_input"] = "Hello World"
	variablePool.SystemVariables.UserID = "test-user"
	variablePool.SystemVariables.AppID = "test-app"

	graphRuntimeState := entities.NewGraphRuntimeState(variablePool)

	// Create start node
	node, err := start.New("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create start node: %v", err)
	}

	// Test node properties
	startNode := node.(*start.Node)
	if startNode.NodeID != "start-test" {
		t.Errorf("Expected node ID 'start-test', got '%s'", startNode.NodeID)
	}

	if startNode.NodeType != shared.Start {
		t.Errorf("Expected node type Start, got %v", startNode.NodeType)
	}

	// Test node execution
	eventChan := make(chan *shared.NodeEventCh, 10)
	defer close(eventChan)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = node.Run(ctx, eventChan)
	if err != nil {
		t.Fatalf("Failed to run start node: %v", err)
	}

	// Verify events were generated
	events := make([]*shared.NodeEventCh, 0)
	for {
		select {
		case event := <-eventChan:
			if event != nil {
				events = append(events, event)
			}
		default:
			goto eventsDone
		}
	}
eventsDone:

	if len(events) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}

	t.Logf("Start node integration test passed with %d events", len(events))
}

func testHTTPRequestNodeCreation(t *testing.T) {
	// Create HTTP request node configuration
	config := map[string]any{
		"id": "http-test",
		"data": map[string]any{
			"method": "GET",
			"url":    "https://httpbin.org/get",
		},
	}

	// Create graph runtime environment
	graphInitParams := entities.GraphInitParams{
		OrganizationID:     "test-tenant",
		AppID:        "test-app",
		WorkflowType: entities.WorkflowTypeWorkflow,
		WorkflowID:   "test-workflow",
		UserID:       "test-user",
		UserFrom:     entities.UserFromAccount,
		InvokeFrom:   entities.InvokeFromWebApp,
		CallDepth:    0,
		GraphConfig:  make(map[string]interface{}),
	}

	variablePool := entities.NewVariablePool()
	graphRuntimeState := entities.NewGraphRuntimeState(variablePool)

	// Create HTTP request node
	node, err := httprequest.NewHTTPRequestNode("instance-1", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		t.Fatalf("Failed to create HTTP request node: %v", err)
	}

	// Test node properties
	httpNode := node.(*httprequest.HTTPRequestNode)
	if httpNode.NodeID != "http-test" {
		t.Errorf("Expected node ID 'http-test', got '%s'", httpNode.NodeID)
	}

	if httpNode.NodeType != shared.HTTPRequest {
		t.Errorf("Expected node type HTTPRequest, got %v", httpNode.NodeType)
	}

	t.Log("HTTP request node creation test passed")
}

func testFactoryPatterns(t *testing.T) {
	// Test HTTPRequestFactory
	factory := httprequest.NewHTTPRequestFactory()
	if factory == nil {
		t.Fatal("Failed to create HTTPRequestFactory")
	}

	// Test default configuration
	defaultConfig := factory.GetDefaultConfig()
	if defaultConfig == nil {
		t.Fatal("Failed to get default configuration")
	}

	if defaultConfig["type"] != "http-request" {
		t.Errorf("Expected type 'http-request', got %v", defaultConfig["type"])
	}

	// Test HTTPRequestHelper
	helper := httprequest.NewHTTPRequestHelper()
	if helper == nil {
		t.Fatal("Failed to create HTTPRequestHelper")
	}

	// Test method validation
	err := helper.ValidateMethod(httprequest.HTTPMethodGET)
	if err != nil {
		t.Errorf("GET method validation failed: %v", err)
	}

	// Test URL validation
	err = helper.ValidateURL("https://example.com")
	if err != nil {
		t.Errorf("URL validation failed: %v", err)
	}

	// Test invalid URL
	err = helper.ValidateURL("invalid-url")
	if err == nil {
		t.Error("Expected URL validation to fail for invalid URL")
	}

	// Test header parsing
	headers, err := helper.ParseHeaders("Content-Type: application/json\nAuthorization: Bearer token")
	if err != nil {
		t.Errorf("Header parsing failed: %v", err)
	}

	if len(headers) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(headers))
	}

	if headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", headers["Content-Type"])
	}

	t.Log("Factory patterns test passed")
}

// BenchmarkStartNodeExecution benchmarks start node execution
func BenchmarkStartNodeExecution(b *testing.B) {
	// Setup
	config := map[string]any{
		"id": "start-bench",
		"data": map[string]any{
			"variables": []any{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "test-tenant",
		AppID:    "test-app",
		UserID:   "test-user",
	}

	variablePool := entities.NewVariablePool()
	variablePool.UserInputs["test"] = "value"

	graphRuntimeState := entities.NewGraphRuntimeState(variablePool)

	node, err := start.New("instance-bench", config, graphInitParams, &entities.Graph{}, graphRuntimeState, nil)
	if err != nil {
		b.Fatalf("Failed to create start node: %v", err)
	}

	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventChan := make(chan *shared.NodeEventCh, 10)
		ctx := context.Background()

		err := node.Run(ctx, eventChan)
		if err != nil {
			b.Fatalf("Failed to run start node: %v", err)
		}

		close(eventChan)
	}
}
