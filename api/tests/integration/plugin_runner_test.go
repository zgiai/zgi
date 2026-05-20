package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
)

const (
	defaultPluginRunnerURL = "http://localhost:15000"
	defaultAPIKey          = "admin-key-123" // Use admin key for plugin management
)

func getPluginRunnerClient() *client.Client {
	pluginCfg := config.Current().PluginRunner
	url := pluginCfg.BaseURL
	if url == "" {
		url = defaultPluginRunnerURL
	}
	apiKey := pluginCfg.APIKey
	if apiKey == "" {
		apiKey = defaultAPIKey
	}
	return client.NewClient(&client.Config{
		BaseURL: url,
		APIKey:  apiKey,
		Timeout: 30 * time.Second,
	})
}

// TestPluginRunnerClient_ListPlugins tests the plugin listing functionality
func TestPluginRunnerClient_ListPlugins(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	c := getPluginRunnerClient()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	plugins, err := c.ListPlugins(ctx)
	if err != nil {
		t.Skipf("Plugin Runner not available: %v", err)
	}

	t.Logf("Found %d plugins", len(plugins))
	for _, plugin := range plugins {
		t.Logf("Plugin: %s (version: %s)", plugin.Name, plugin.Version)
	}

	assert.NotNil(t, plugins)
}

// TestPluginRunnerClient_ListInstalledPlugins tests listing installed plugins
func TestPluginRunnerClient_ListInstalledPlugins(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	c := getPluginRunnerClient()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	plugins, err := c.ListInstalledPlugins(ctx)
	if err != nil {
		t.Skipf("Plugin Runner not available: %v", err)
	}

	t.Logf("Found %d installed plugins", len(plugins))
	for _, plugin := range plugins {
		t.Logf("Installed: %s (version: %s)",
			plugin.Manifest.Name, plugin.Manifest.Version)
	}

	assert.NotNil(t, plugins)
}

// TestPluginRunnerClient_SessionLifecycle tests session creation and cleanup
func TestPluginRunnerClient_SessionLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	c := getPluginRunnerClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, list installed plugins
	plugins, err := c.ListInstalledPlugins(ctx)
	if err != nil {
		t.Skipf("Plugin Runner not available: %v", err)
	}

	if len(plugins) == 0 {
		t.Skip("No plugins installed, skipping session test")
	}

	// Use the first available plugin
	plugin := plugins[0]
	t.Logf("Testing with plugin: %s:%s", plugin.Manifest.Name, plugin.Manifest.Version)

	// Create session
	sessionReq := model.StartSessionRequest{
		Name:       plugin.Manifest.Name,
		Version:    plugin.Manifest.Version,
		Entrypoint: plugin.Manifest.Runner.Entrypoint,
	}

	session, err := c.StartSession(ctx, sessionReq)
	if err != nil {
		t.Skipf("Failed to start session: %v", err)
	}

	require.NotEmpty(t, session.ID)
	t.Logf("Created session: %s (status: %s)", session.ID, session.Status)

	// Clean up - stop session
	defer func() {
		err := c.StopSession(ctx, session.ID)
		if err != nil {
			t.Logf("Warning: Failed to stop session: %v", err)
		}
	}()

	// Wait for session to be ready
	for i := 0; i < 10; i++ {
		readyResp, err := c.IsSessionReady(ctx, session.ID)
		if err == nil && readyResp.Ready {
			t.Log("Session is ready")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Get session status
	sessionDetails, err := c.GetSession(ctx, session.ID)
	if err != nil {
		t.Logf("Failed to get session details: %v", err)
	} else {
		t.Logf("Session status: %s", sessionDetails.Status)
	}
}

// TestPluginRunnerClient_ToolInvocation tests invoking a tool
func TestPluginRunnerClient_ToolInvocation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	c := getPluginRunnerClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List installed plugins
	plugins, err := c.ListInstalledPlugins(ctx)
	if err != nil {
		t.Skipf("Plugin Runner not available: %v", err)
	}

	if len(plugins) == 0 {
		t.Skip("No plugins installed")
	}

	// Find a suitable echo plugin
	var selectedPlugin *model.Installation
	for i, p := range plugins {
		if p.Manifest.Name == "echo-plugin-a" || p.Manifest.Name == "uv-echo" {
			selectedPlugin = &plugins[i]
			break
		}
	}

	if selectedPlugin == nil {
		selectedPlugin = &plugins[0]
	}

	t.Logf("Testing with plugin: %s:%s", selectedPlugin.Manifest.Name, selectedPlugin.Manifest.Version)

	// Create session
	sessionReq := model.StartSessionRequest{
		Name:       selectedPlugin.Manifest.Name,
		Version:    selectedPlugin.Manifest.Version,
		Entrypoint: selectedPlugin.Manifest.Runner.Entrypoint,
	}

	session, err := c.StartSession(ctx, sessionReq)
	if err != nil {
		t.Skipf("Failed to start session: %v", err)
	}

	defer func() {
		_ = c.StopSession(ctx, session.ID)
	}()

	// Wait for session ready
	ready := false
	for i := 0; i < 20; i++ {
		readyResp, err := c.IsSessionReady(ctx, session.ID)
		if err == nil && readyResp.Ready {
			ready = true
			break
		}
		// Check if session failed
		details, _ := c.GetSession(ctx, session.ID)
		if details != nil && details.Status == model.SessionStatusFailed {
			t.Skipf("Session failed, check plugin configuration")
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !ready {
		t.Skip("Session not ready within timeout")
	}

	// Invoke tool
	toolReq := model.ToolInvokeRequest{
		SessionID:  session.ID,
		Provider:   selectedPlugin.Manifest.Name,
		Tool:       "echo",
		Parameters: map[string]interface{}{"message": "Hello from integration test"},
	}

	result, err := c.InvokeTool(ctx, toolReq)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("Tool result: %+v", result)
	assert.True(t, result.Success || result.Data != nil)
}
