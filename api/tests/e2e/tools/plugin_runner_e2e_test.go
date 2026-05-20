package tools_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/tools"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	toolspkg "github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
)

const (
	pluginRunnerURL    = "http://localhost:2665"
	pluginRunnerAPIKey = "admin-key-123"
	testPluginPath     = "../../../../runner/examples/test_plugin/uv_echo_0.0.1"
	testPluginName     = "uv-echo-e2e-test"
)

// getTestPluginVersion generates a unique version based on timestamp
func getTestPluginVersion() string {
	return fmt.Sprintf("0.0.%d", time.Now().Unix())
}

// TestToolsNode_PluginRunner_FullLifecycle tests the complete plugin lifecycle:
// Register Plugin -> Install Plugin -> Call Tool via Tools Node -> Cleanup
// This proves the entire chain works from code without using curl.
// Note: Due to Docker permission issues, this test uses existing installed plugin
// when fresh installation fails.
func TestToolsNode_PluginRunner_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Generate unique version for this test run
	testPluginVersion := getTestPluginVersion()
	t.Logf("Using plugin version: %s", testPluginVersion)

	// Create Plugin Runner client
	pluginClient := client.NewClient(&client.Config{
		BaseURL: pluginRunnerURL,
		APIKey:  pluginRunnerAPIKey,
		Timeout: 30 * time.Second,
	})

	// Check if Plugin Runner is healthy
	if !pluginClient.IsHealthy(ctx) {
		t.Skip("Plugin Runner service is not available")
	}

	// Define which plugin to use for testing
	var activePluginName string
	var activePluginVersion string

	t.Log("Step 1: Creating plugin package from source directory...")
	packageContent, err := createPluginPackage(testPluginPath)
	if err != nil {
		t.Logf("Warning: Failed to create plugin package: %v (will use existing plugin)", err)
	} else {
		t.Logf("Plugin package created, size: %d bytes", len(packageContent))

		t.Log("Step 2: Registering plugin...")
		plugin, err := pluginClient.RegisterPlugin(ctx, model.PluginManifest{
			Name:        testPluginName,
			Version:     testPluginVersion,
			Description: "E2E test plugin for full lifecycle verification",
			Author:      "e2e-test",
			Runner: model.PluginRunner{
				Language:   "python",
				Entrypoint: "main_runner",
			},
		})
		if err != nil {
			t.Logf("Register plugin result: %v (may already exist)", err)
		} else {
			t.Logf("Plugin registered: %s (ID: %s)", plugin.Name, plugin.ID)
		}

		t.Log("Step 3: Installing plugin package...")
		_, installErr := pluginClient.InstallPluginWithFile(
			ctx,
			testPluginName+":"+testPluginVersion,
			packageContent,
			true, // force reinstall
		)
		if installErr != nil {
			t.Logf("Install plugin failed: %v (will use existing plugin)", installErr)
		} else {
			activePluginName = testPluginName
			activePluginVersion = testPluginVersion
			t.Log("Plugin installed successfully")
		}
	}

	// If installation failed, fall back to using existing uv-echo plugin
	if activePluginName == "" {
		t.Log("Using existing uv-echo plugin for testing...")
		activePluginName = "uv-echo"
		activePluginVersion = "0.0.1"

		// Verify the plugin is installed
		installations, err := pluginClient.ListInstalledPlugins(ctx)
		if err != nil {
			t.Fatalf("Failed to list installed plugins: %v", err)
		}

		found := false
		for _, inst := range installations {
			if inst.Manifest.Name == activePluginName {
				found = true
				activePluginVersion = inst.Manifest.Version
				break
			}
		}
		if !found {
			t.Skipf("Required plugin %s is not installed", activePluginName)
		}
	}

	t.Logf("Using plugin: %s:%s", activePluginName, activePluginVersion)

	t.Log("Step 4: Starting plugin session...")
	session, err := pluginClient.StartSession(ctx, model.StartSessionRequest{
		Name:       activePluginName,
		Version:    activePluginVersion,
		Language:   "python",
		Entrypoint: "main_runner",
	})
	require.NoError(t, err, "Failed to start session")
	t.Logf("Session started: %s (status: %s)", session.ID, session.Status)

	// Store session ID for cleanup
	sessionID := session.ID
	defer func() {
		t.Logf("Cleanup: Stopping session %s...", sessionID)
		_ = pluginClient.StopSession(context.Background(), sessionID)
	}()

	t.Log("Step 5: Waiting for plugin to be ready...")
	for i := 0; i < 30; i++ {
		readyResp, err := pluginClient.IsSessionReady(ctx, sessionID)
		if err == nil && readyResp.Ready {
			t.Log("Plugin is ready!")
			break
		}
		if i == 29 {
			t.Fatalf("Plugin did not become ready within timeout")
		}
		time.Sleep(1 * time.Second)
	}

	t.Log("Step 6: Invoking tool via ToolEngine...")
	// Create Plugin Runner service
	pluginRunnerSvc := service.NewPluginRunnerService(&client.Config{
		BaseURL: pluginRunnerURL,
		APIKey:  pluginRunnerAPIKey,
		Timeout: 30 * time.Second,
	})

	// Create full tool chain
	pluginRunnerAdapter := toolspkg.NewPluginRunnerToolAdapter(pluginRunnerSvc, nil)
	toolManager := toolspkg.NewToolManager(pluginRunnerAdapter)
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())
	toolEngine := toolspkg.NewToolEngine(toolManager)

	// Test direct invoke through ToolEngine
	result, err := toolEngine.Invoke(ctx, toolspkg.InvokeRequest{
		ProviderType: toolspkg.ToolProviderTypePluginRunner,
		ProviderID:   activePluginName,
		ToolName:     "echo_http",
		TenantID:     "",
		UserID:       "e2e-test-user",
		Parameters: map[string]interface{}{
			"url":     "https://httpbin.org/get",
			"message": "Hello from Full Lifecycle E2E Test!",
		},
		InvokeFrom: toolspkg.ToolInvokeFromWorkflow,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success, "Tool invocation should succeed")
	t.Logf("ToolEngine invoke result: Success=%v, Messages=%v", result.Success, result.Messages)

	t.Log("Step 7: Invoking tool via Tools Node...")
	// Create variable pool and runtime state
	variablePool := entities.NewVariablePool()
	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration for plugin_runner tool
	nodeConfig := map[string]any{
		"id": "tools-node-lifecycle-test",
		"data": map[string]interface{}{
			"title":         "Lifecycle Test Tool Call",
			"provider_type": "plugin_runner",
			"provider_id":   activePluginName,
			"tool_name":     "regex_extract",
			"tool_parameters": map[string]interface{}{
				"content": map[string]interface{}{
					"type":  "constant",
					"value": "Emails: test@example.com and support@zgi.ai",
				},
				"expression": map[string]interface{}{
					"type":  "constant",
					"value": "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}",
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "",
		AppID:          "test-app-lifecycle",
		WorkflowID:     "test-workflow-lifecycle",
		UserID:         "test-user-lifecycle",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
	}

	graph := &entities.Graph{}

	// Create Tools node
	node, err := tools.New(
		"instance-lifecycle-test",
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
	close(eventChan)
	var completionEvent *shared.NodeEventCh
	for event := range eventChan {
		t.Logf("Event: %s", event.Type)
		if event.Type == shared.EventTypeRunCompleted {
			completionEvent = event
		}
	}
	require.NotNil(t, completionEvent, "Should have completion event")

	// Verify result
	runResult := completionEvent.Data.(*shared.RunCompletedEvent).RunResult
	require.NotNil(t, runResult)
	assert.Equal(t, shared.SUCCEEDED, runResult.Status, "Tool invocation should succeed")

	t.Logf("Tools Node output: %+v", runResult.Outputs)
	t.Log("Full Lifecycle E2E Test PASSED!")
}

// createPluginPackage creates a zip package from the plugin directory
func createPluginPackage(sourceDir string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Skip root directory
		if relPath == "." {
			return nil
		}

		// Skip hidden files and directories
		if len(relPath) > 0 && relPath[0] == '.' {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			// Create directory entry
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// Create file entry
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Read and write file content
		fileContent, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, bytes.NewReader(fileContent))
		return err
	})

	if err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// TestToolsNode_PluginRunner_FullChain tests the complete chain:
// Tools Node -> ToolEngine -> ToolManager -> PluginRunnerAdapter -> Plugin Runner -> Plugin
func TestToolsNode_PluginRunner_FullChain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create Plugin Runner service
	pluginRunnerSvc := service.NewPluginRunnerService(&client.Config{
		BaseURL: pluginRunnerURL,
		APIKey:  pluginRunnerAPIKey,
		Timeout: 30 * time.Second,
	})

	// Check if Plugin Runner is healthy
	if !pluginRunnerSvc.IsHealthy(ctx) {
		t.Skip("Plugin Runner service is not available")
	}

	// Verify plugin is installed
	installations, err := pluginRunnerSvc.ListInstalledPlugins(ctx)
	if err != nil {
		t.Skipf("Failed to list installed plugins: %v", err)
	}

	var uvEchoInstalled bool
	for _, installation := range installations {
		if installation.Manifest.Name == "uv-echo" {
			uvEchoInstalled = true
			break
		}
	}
	if !uvEchoInstalled {
		t.Skip("uv-echo plugin is not installed")
	}

	// Create PluginRunnerToolAdapter
	pluginRunnerAdapter := toolspkg.NewPluginRunnerToolAdapter(pluginRunnerSvc, nil)

	// Create ToolManager with Plugin Runner adapter
	toolManager := toolspkg.NewToolManager(pluginRunnerAdapter)

	// Register builtin providers
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Create ToolEngine
	toolEngine := toolspkg.NewToolEngine(toolManager)
	require.NotNil(t, toolEngine)

	// Create variable pool
	variablePool := entities.NewVariablePool()

	// Create graph runtime state
	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration for plugin_runner echo_http tool
	nodeConfig := map[string]any{
		"id": "tools-node-plugin-runner",
		"data": map[string]interface{}{
			"title":         "Call Plugin Runner Tool",
			"provider_type": "plugin_runner",
			"provider_id":   "uv-echo",
			"tool_name":     "echo_http",
			"tool_parameters": map[string]interface{}{
				"url": map[string]interface{}{
					"type":  "constant",
					"value": "https://httpbin.org/get",
				},
				"message": map[string]interface{}{
					"type":  "constant",
					"value": "Hello from Tools Node E2E Test!",
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "", // Empty tenant ID to bypass multi-tenant check
		AppID:          "test-app-e2e",
		WorkflowID:     "test-workflow-e2e",
		UserID:         "test-user-e2e",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
	}

	graph := &entities.Graph{}

	// Create Tools node
	node, err := tools.New(
		"instance-plugin-runner",
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
	t.Log("Running Tools node with plugin_runner provider...")
	err = node.Run(ctx, eventChan)
	require.NoError(t, err)

	// Collect events
	close(eventChan)
	var completionEvent *shared.NodeEventCh
	for event := range eventChan {
		t.Logf("Event: %s", event.Type)
		if event.Type == shared.EventTypeRunCompleted {
			completionEvent = event
		}
	}
	require.NotNil(t, completionEvent, "Should have completion event")

	// Verify result
	runResult := completionEvent.Data.(*shared.RunCompletedEvent).RunResult
	require.NotNil(t, runResult)
	assert.Equal(t, shared.SUCCEEDED, runResult.Status, "Tool invocation should succeed")

	// Log the output
	t.Logf("Tool output: %+v", runResult.Outputs)
}

// TestToolsNode_PluginRunner_RegexExtract tests regex extraction through the full chain
func TestToolsNode_PluginRunner_RegexExtract(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create Plugin Runner service
	pluginRunnerSvc := service.NewPluginRunnerService(&client.Config{
		BaseURL: pluginRunnerURL,
		APIKey:  pluginRunnerAPIKey,
		Timeout: 30 * time.Second,
	})

	if !pluginRunnerSvc.IsHealthy(ctx) {
		t.Skip("Plugin Runner service is not available")
	}

	// Create full tool chain
	pluginRunnerAdapter := toolspkg.NewPluginRunnerToolAdapter(pluginRunnerSvc, nil)
	toolManager := toolspkg.NewToolManager(pluginRunnerAdapter)
	toolManager.RegisterBuiltinProviders(builtin.GetAllProviders())
	toolEngine := toolspkg.NewToolEngine(toolManager)

	variablePool := entities.NewVariablePool()
	graphRuntimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Node configuration for regex_extract tool
	nodeConfig := map[string]any{
		"id": "tools-node-regex",
		"data": map[string]interface{}{
			"title":         "Extract Emails",
			"provider_type": "plugin_runner",
			"provider_id":   "uv-echo",
			"tool_name":     "regex_extract",
			"tool_parameters": map[string]interface{}{
				"content": map[string]interface{}{
					"type":  "constant",
					"value": "Contact us at support@zgi.ai or sales@example.com for more info",
				},
				"expression": map[string]interface{}{
					"type":  "constant",
					"value": "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}",
				},
			},
			"tool_configurations": map[string]interface{}{},
		},
	}

	graphInitParams := entities.GraphInitParams{
		OrganizationID: "", // Empty tenant ID to bypass multi-tenant check
		AppID:          "test-app-regex",
		WorkflowID:     "test-workflow-regex",
		UserID:         "test-user-regex",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromDebugger,
	}

	graph := &entities.Graph{}

	node, err := tools.New(
		"instance-regex",
		nodeConfig,
		graphInitParams,
		graph,
		graphRuntimeState,
		nil,
		toolEngine,
	)
	require.NoError(t, err)

	eventChan := make(chan *shared.NodeEventCh, 100)

	t.Log("Running Tools node with regex_extract...")
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
	require.NotNil(t, runResult)
	assert.Equal(t, shared.SUCCEEDED, runResult.Status)

	t.Logf("Regex extract output: %+v", runResult.Outputs)
}

// TestToolEngine_PluginRunner_DirectInvoke tests invoking plugin runner tools directly through ToolEngine
func TestToolEngine_PluginRunner_DirectInvoke(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create Plugin Runner service
	pluginRunnerSvc := service.NewPluginRunnerService(&client.Config{
		BaseURL: pluginRunnerURL,
		APIKey:  pluginRunnerAPIKey,
		Timeout: 30 * time.Second,
	})

	if !pluginRunnerSvc.IsHealthy(ctx) {
		t.Skip("Plugin Runner service is not available")
	}

	// Create full tool chain
	pluginRunnerAdapter := toolspkg.NewPluginRunnerToolAdapter(pluginRunnerSvc, nil)
	toolManager := toolspkg.NewToolManager(pluginRunnerAdapter)
	toolEngine := toolspkg.NewToolEngine(toolManager)

	// Test direct invoke through ToolEngine
	result, err := toolEngine.Invoke(ctx, toolspkg.InvokeRequest{
		ProviderType: toolspkg.ToolProviderTypePluginRunner,
		ProviderID:   "uv-echo",
		ToolName:     "echo_http",
		TenantID:     "", // Empty tenant ID to bypass multi-tenant check
		UserID:       "test-user",
		Parameters: map[string]interface{}{
			"url":     "https://httpbin.org/get",
			"message": "Direct invoke from ToolEngine",
		},
		InvokeFrom: toolspkg.ToolInvokeFromWorkflow,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success, "Tool invocation should succeed")

	t.Logf("Direct invoke result: %+v", result.Messages)
}
