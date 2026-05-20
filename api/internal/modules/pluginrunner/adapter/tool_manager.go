package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// PluginRunnerToolManager adapts PluginRunnerService for workflow tool invocation
// It supports multi-tenant plugin invocation with proper session management
type PluginRunnerToolManager struct {
	service service.PluginRunnerService
}

// NewPluginRunnerToolManager creates a new PluginRunnerToolManager
func NewPluginRunnerToolManager(svc service.PluginRunnerService) *PluginRunnerToolManager {
	return &PluginRunnerToolManager{
		service: svc,
	}
}

// GetService returns the underlying plugin runner service
func (m *PluginRunnerToolManager) GetService() service.PluginRunnerService {
	return m.service
}

// ToolInvokeMessage represents a tool invocation result message
type ToolInvokeMessage struct {
	Type string      `json:"type"`
	Text string      `json:"text,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// CredentialType represents the type of credentials
type CredentialType string

const (
	CredentialTypeAPIKey CredentialType = "api_key"
	CredentialTypeOAuth  CredentialType = "oauth2"
)

// PluginToolProviderEntity represents a plugin tool provider entity
type PluginToolProviderEntity struct {
	PluginID               string               `json:"plugin_id"`
	PluginUniqueIdentifier string               `json:"plugin_unique_identifier"`
	Declaration            model.PluginManifest `json:"declaration"`
}

// FetchToolProviders fetches tool providers for the given tenant
func (m *PluginRunnerToolManager) FetchToolProviders(ctx context.Context, tenantID string) ([]PluginToolProviderEntity, error) {
	// In our implementation, we don't have tenant-specific plugin listings
	// We'll return all installed plugins as providers
	installations, err := m.service.ListInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	var providers []PluginToolProviderEntity
	for _, installation := range installations {
		provider := PluginToolProviderEntity{
			PluginID:               installation.Manifest.Name,
			PluginUniqueIdentifier: fmt.Sprintf("%s@%s", installation.Manifest.Name, installation.Manifest.Version),
			Declaration:            installation.Manifest,
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

// FetchToolProvider fetches a specific tool provider
func (m *PluginRunnerToolManager) FetchToolProvider(ctx context.Context, tenantID string, providerName string) (*PluginToolProviderEntity, error) {
	// In our implementation, we'll look for an installed plugin with the given name
	installations, err := m.service.ListInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	for _, installation := range installations {
		if installation.Manifest.Name == providerName {
			provider := &PluginToolProviderEntity{
				PluginID:               installation.Manifest.Name,
				PluginUniqueIdentifier: fmt.Sprintf("%s@%s", installation.Manifest.Name, installation.Manifest.Version),
				Declaration:            installation.Manifest,
			}
			return provider, nil
		}
	}

	return nil, fmt.Errorf("plugin provider %s not found", providerName)
}

// Invoke invokes a tool with the given parameters
// This method handles the complete lifecycle: start session -> wait ready -> invoke -> stop session
func (m *PluginRunnerToolManager) Invoke(
	ctx context.Context,
	tenantID string,
	userID string,
	toolProvider string,
	toolName string,
	credentials map[string]interface{},
	credentialType CredentialType,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]ToolInvokeMessage, error) {
	logger.Debug("invoking plugin tool",
		"tenant_id", tenantID,
		"user_id", userID,
		"tool_provider", toolProvider,
		"tool_name", toolName)

	// Parse tenant ID
	tenantIDUint := parseTenantID(tenantID)

	// Start session with tenant context
	// The tenant ID is passed to the plugin runner for access control and auditing
	session, err := m.service.StartSession(ctx, model.StartSessionRequest{
		Name:       toolProvider,
		Version:    "latest",
		Entrypoint: "main",
		TenantID:   tenantIDUint,
	}, client.WithTenantID(tenantID), client.WithActor(userID))
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Ensure session is stopped when done
	defer func() {
		if stopErr := m.service.StopSession(ctx, session.ID); stopErr != nil {
			logger.Warn("failed to stop session", "session_id", session.ID, "error", stopErr)
		}
	}()

	// Wait for session to be ready (max 30 seconds)
	if err := m.service.WaitForSessionReady(ctx, session.ID, 30*time.Second); err != nil {
		return nil, fmt.Errorf("session failed to become ready: %w", err)
	}

	// Invoke tool in the session
	resp, err := m.service.InvokeTool(ctx, model.ToolInvokeRequest{
		SessionID:  session.ID,
		Provider:   toolProvider,
		Tool:       toolName,
		Parameters: toolParameters,
		Timeout:    30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	// Convert response to ToolInvokeMessage format
	return m.convertResponse(resp)
}

// InvokeWithSession invokes a tool using an existing session (for batch operations)
func (m *PluginRunnerToolManager) InvokeWithSession(
	ctx context.Context,
	sessionID string,
	toolProvider string,
	toolName string,
	toolParameters map[string]interface{},
) ([]ToolInvokeMessage, error) {
	resp, err := m.service.InvokeTool(ctx, model.ToolInvokeRequest{
		SessionID:  sessionID,
		Provider:   toolProvider,
		Tool:       toolName,
		Parameters: toolParameters,
		Timeout:    30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	return m.convertResponse(resp)
}

// StartToolSession starts a session for tool invocation (caller manages lifecycle)
func (m *PluginRunnerToolManager) StartToolSession(
	ctx context.Context,
	tenantID string,
	userID string,
	toolProvider string,
) (*model.Session, error) {
	tenantIDUint := parseTenantID(tenantID)

	session, err := m.service.StartSession(ctx, model.StartSessionRequest{
		Name:       toolProvider,
		Version:    "latest",
		Entrypoint: "main",
		TenantID:   tenantIDUint,
	}, client.WithTenantID(tenantID), client.WithActor(userID))
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Wait for session to be ready
	if err := m.service.WaitForSessionReady(ctx, session.ID, 30*time.Second); err != nil {
		_ = m.service.StopSession(ctx, session.ID)
		return nil, fmt.Errorf("session failed to become ready: %w", err)
	}

	return session, nil
}

// StopToolSession stops a tool session
func (m *PluginRunnerToolManager) StopToolSession(ctx context.Context, sessionID string) error {
	return m.service.StopSession(ctx, sessionID)
}

// convertResponse converts a plugin runner response to ToolInvokeMessage format
func (m *PluginRunnerToolManager) convertResponse(resp *model.InvokeResponse) ([]ToolInvokeMessage, error) {
	var messages []ToolInvokeMessage
	if resp.Success {
		if resp.Data != nil {
			// Handle different response types
			if text, ok := resp.Data["text"].(string); ok && text != "" {
				messages = append(messages, ToolInvokeMessage{
					Type: "text",
					Text: text,
				})
			} else {
				messages = append(messages, ToolInvokeMessage{
					Type: "json",
					Data: resp.Data,
				})
			}
		} else {
			messages = append(messages, ToolInvokeMessage{
				Type: "text",
				Text: "Tool executed successfully",
			})
		}
	} else {
		return nil, fmt.Errorf("tool invocation failed: %s", resp.Error)
	}

	return messages, nil
}

// ValidateProviderCredentials validates the credentials of a provider
func (m *PluginRunnerToolManager) ValidateProviderCredentials(
	ctx context.Context,
	tenantID string,
	userID string,
	provider string,
	credentials map[string]interface{},
) (bool, error) {
	// In our simplified implementation, we'll just check if the plugin exists
	_, err := m.FetchToolProvider(ctx, tenantID, provider)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetRuntimeParameters gets the runtime parameters of a tool
func (m *PluginRunnerToolManager) GetRuntimeParameters(
	ctx context.Context,
	tenantID string,
	userID string,
	provider string,
	credentials map[string]interface{},
	tool string,
	conversationID *string,
	appID *string,
	messageID *string,
) ([]map[string]interface{}, error) {
	// In our simplified implementation, we'll return empty parameters
	// A more complete implementation would fetch tool schema from the plugin
	return []map[string]interface{}{}, nil
}

// ============================================
// Multi-Tenant Operations
// ============================================

// CreateWorkspace creates a new tenant in the plugin runner
func (m *PluginRunnerToolManager) CreateWorkspace(ctx context.Context, name string) (*model.Tenant, error) {
	return m.service.CreateWorkspace(ctx, name)
}

// ListPluginTenants lists all tenants that have access to a plugin
func (m *PluginRunnerToolManager) ListPluginTenants(ctx context.Context, pluginID string) ([]model.PluginTenantBinding, error) {
	return m.service.ListPluginTenants(ctx, pluginID)
}

// EnablePluginForTenant enables a plugin for a specific tenant
func (m *PluginRunnerToolManager) EnablePluginForTenant(ctx context.Context, pluginID string, tenantID uint, config *model.TenantConfig) error {
	return m.service.EnablePluginTenant(ctx, pluginID, tenantID, config)
}

// DisablePluginForTenant disables a plugin for a specific tenant
func (m *PluginRunnerToolManager) DisablePluginForTenant(ctx context.Context, pluginID string, tenantID uint) error {
	return m.service.DisablePluginTenant(ctx, pluginID, tenantID)
}

// ============================================
// Utility Functions
// ============================================

// parseTenantID converts string tenant ID to uint
func parseTenantID(tenantID string) uint {
	if tenantID == "" {
		return 0
	}
	var id uint
	fmt.Sscanf(tenantID, "%d", &id)
	return id
}
