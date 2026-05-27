package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ToolManager is the central manager for all tool providers
type ToolManager struct {
	mu sync.RWMutex

	// Plugin runner tool manager
	pluginRunnerManager PluginRunnerToolManagerInterface

	// Registered providers by type
	providers map[ToolProviderType]map[string]ToolProvider
}

// PluginRunnerToolManagerInterface defines the interface for plugin runner tool manager
type PluginRunnerToolManagerInterface interface {
	FetchToolProviders(ctx context.Context, tenantID string) ([]ToolProviderEntity, error)
	FetchToolProvider(ctx context.Context, tenantID, providerName string) (*ToolProviderEntity, error)
	InvokeTool(ctx context.Context, tenantID, userID, provider, tool string, params map[string]interface{}) ([]ToolInvokeMessage, error)
	ValidateCredentials(ctx context.Context, tenantID, userID, provider string, credentials map[string]interface{}) (bool, error)
	StopReusableSessionsByWorkflowRunID(ctx context.Context, workflowRunID string) (int, error)
	SweepStaleReusableSessions(ctx context.Context, maxAge time.Duration) (int, error)
}

// NewToolManager creates a new ToolManager
func NewToolManager(pluginRunnerManager PluginRunnerToolManagerInterface) *ToolManager {
	return &ToolManager{
		pluginRunnerManager: pluginRunnerManager,
		providers: map[ToolProviderType]map[string]ToolProvider{
			// Supported types
			ToolProviderTypeBuiltin:      make(map[string]ToolProvider),
			ToolProviderTypePluginRunner: make(map[string]ToolProvider),
			// Not supported - reserved for future extension
			// ToolProviderTypeAPI:      make(map[string]ToolProvider),
			// ToolProviderTypeWorkflow: make(map[string]ToolProvider),
		},
	}
}

// RegisterBuiltinProviders registers all provided builtin tool providers
func (m *ToolManager) RegisterBuiltinProviders(providers []ToolProvider) {
	for _, provider := range providers {
		_ = m.RegisterProvider(provider)
	}
}

// RegisterProvider registers a tool provider
func (m *ToolManager) RegisterProvider(provider ToolProvider) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	providerType := NormalizeToolProviderType(provider.GetProviderType())
	entity := provider.GetEntity()

	if _, exists := m.providers[providerType]; !exists {
		m.providers[providerType] = make(map[string]ToolProvider)
	}

	m.providers[providerType][entity.Identity.Name] = provider
	return nil
}

// UnregisterProvider unregisters a tool provider
func (m *ToolManager) UnregisterProvider(providerType ToolProviderType, providerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	providerType = NormalizeToolProviderType(providerType)
	if providers, exists := m.providers[providerType]; exists {
		delete(providers, providerName)
	}
	return nil
}

// GetProvider returns a tool provider by type and name
func (m *ToolManager) GetProvider(ctx context.Context, providerType ToolProviderType, providerName, tenantID string) (ToolProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providerType = NormalizeToolProviderType(providerType)

	// For plugin runner type, dynamically fetch from plugin runner service
	if providerType == ToolProviderTypePluginRunner && m.pluginRunnerManager != nil {
		providerEntity, err := m.pluginRunnerManager.FetchToolProvider(ctx, tenantID, providerName)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plugin runner provider: %w", err)
		}
		return NewPluginRunnerProvider(*providerEntity, tenantID, m.pluginRunnerManager), nil
	}

	// Check registered providers
	if providers, exists := m.providers[providerType]; exists {
		if provider, found := providers[providerName]; found {
			return provider, nil
		}
	}

	return nil, fmt.Errorf("provider %s of type %s not found", providerName, providerType)
}

// GetToolRuntime returns a tool runtime for the given tool
func (m *ToolManager) GetToolRuntime(
	ctx context.Context,
	providerType ToolProviderType,
	providerID string,
	toolName string,
	tenantID string,
	invokeFrom ToolInvokeFrom,
	credentialID string,
	runtimeParameters map[string]interface{},
) (Tool, error) {
	// Get the provider
	provider, err := m.GetProvider(ctx, providerType, providerID, tenantID)
	if err != nil {
		return nil, err
	}

	// Get the tool from provider
	tool, err := provider.GetTool(toolName)
	if err != nil {
		return nil, fmt.Errorf("tool %s not found in provider %s: %w", toolName, providerID, err)
	}

	runtime := &ToolRuntime{
		TenantID:          tenantID,
		RuntimeParameters: copyRuntimeParameters(runtimeParameters),
		InvokeFrom:        invokeFrom,
	}

	return tool.ForkToolRuntime(runtime), nil
}

func copyRuntimeParameters(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

// ListProviders lists all registered providers of a given type
func (m *ToolManager) ListProviders(providerType ToolProviderType) []ToolProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providerType = NormalizeToolProviderType(providerType)

	var result []ToolProvider
	if providers, exists := m.providers[providerType]; exists {
		for _, provider := range providers {
			result = append(result, provider)
		}
	}
	return result
}

// ListPluginRunnerProviders lists all plugin runner providers for a tenant
func (m *ToolManager) ListPluginRunnerProviders(ctx context.Context, tenantID string) ([]ToolProviderEntity, error) {
	if m.pluginRunnerManager == nil {
		return nil, fmt.Errorf("plugin runner manager not configured")
	}
	return m.pluginRunnerManager.FetchToolProviders(ctx, tenantID)
}

// ListAllProviders lists all providers for a tenant
func (m *ToolManager) ListAllProviders(ctx context.Context, tenantID string) ([]ToolProviderEntity, error) {
	var result []ToolProviderEntity

	// Add registered providers
	m.mu.RLock()
	for _, providersByType := range m.providers {
		for _, provider := range providersByType {
			result = append(result, provider.GetEntity())
		}
	}
	m.mu.RUnlock()

	// Add plugin runner providers
	if m.pluginRunnerManager != nil {
		pluginProviders, err := m.pluginRunnerManager.FetchToolProviders(ctx, tenantID)
		if err == nil {
			result = append(result, pluginProviders...)
		}
	}

	return result, nil
}

// StopReusableSessionsByWorkflowRunID force-cleans reusable plugin sessions for one workflow run.
func (m *ToolManager) StopReusableSessionsByWorkflowRunID(ctx context.Context, workflowRunID string) (int, error) {
	if m.pluginRunnerManager == nil {
		return 0, nil
	}
	return m.pluginRunnerManager.StopReusableSessionsByWorkflowRunID(ctx, workflowRunID)
}

// SweepStaleReusableSessions stops stale reusable plugin sessions older than maxAge.
func (m *ToolManager) SweepStaleReusableSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	if m.pluginRunnerManager == nil {
		return 0, nil
	}
	return m.pluginRunnerManager.SweepStaleReusableSessions(ctx, maxAge)
}

// ============================================
// Plugin Runner Provider Implementation
// ============================================

// PluginRunnerProvider implements ToolProvider for plugin runner tools
type PluginRunnerProvider struct {
	entity   ToolProviderEntity
	tenantID string
	manager  PluginRunnerToolManagerInterface
}

// NewPluginRunnerProvider creates a new PluginRunnerProvider
func NewPluginRunnerProvider(entity ToolProviderEntity, tenantID string, manager PluginRunnerToolManagerInterface) *PluginRunnerProvider {
	return &PluginRunnerProvider{
		entity:   entity,
		tenantID: tenantID,
		manager:  manager,
	}
}

// GetEntity returns the provider entity
func (p *PluginRunnerProvider) GetEntity() ToolProviderEntity {
	return p.entity
}

// GetProviderType returns the provider type
func (p *PluginRunnerProvider) GetProviderType() ToolProviderType {
	return ToolProviderTypePluginRunner
}

// GetTool returns a tool by name
// For plugin runner, tools are dynamically created since they're not known until runtime
func (p *PluginRunnerProvider) GetTool(name string) (Tool, error) {
	// For plugin runner providers, create tool dynamically
	// since the actual tools are defined in the plugin, not in our metadata
	toolEntity := ToolEntity{
		Identity: ToolIdentity{
			Name:     name,
			Provider: p.entity.Identity.Name,
			Label:    I18nText{"en_US": name},
		},
		Parameters: []ToolParameter{},
	}
	return NewPluginRunnerToolImpl(toolEntity, p.tenantID, p.entity.Identity.Name, p.manager), nil
}

// GetTools returns all tools
func (p *PluginRunnerProvider) GetTools() []Tool {
	var tools []Tool
	for _, toolEntity := range p.entity.Tools {
		tools = append(tools, NewPluginRunnerToolImpl(toolEntity, p.tenantID, p.entity.Identity.Name, p.manager))
	}
	return tools
}

// ValidateCredentials validates the provider credentials
func (p *PluginRunnerProvider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	valid, err := p.manager.ValidateCredentials(ctx, p.tenantID, "", p.entity.Identity.Name, credentials)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("credentials validation failed")
	}
	return nil
}

// ============================================
// Plugin Runner Tool Implementation
// ============================================

// PluginRunnerToolImpl implements Tool for plugin runner tools
type PluginRunnerToolImpl struct {
	entity       ToolEntity
	tenantID     string
	providerName string
	manager      PluginRunnerToolManagerInterface
	runtime      *ToolRuntime
}

// NewPluginRunnerToolImpl creates a new PluginRunnerToolImpl
func NewPluginRunnerToolImpl(entity ToolEntity, tenantID, providerName string, manager PluginRunnerToolManagerInterface) *PluginRunnerToolImpl {
	return &PluginRunnerToolImpl{
		entity:       entity,
		tenantID:     tenantID,
		providerName: providerName,
		manager:      manager,
		runtime: &ToolRuntime{
			TenantID:          tenantID,
			RuntimeParameters: make(map[string]interface{}),
		},
	}
}

// GetEntity returns the tool entity
func (t *PluginRunnerToolImpl) GetEntity() ToolEntity {
	return t.entity
}

// GetProviderType returns the provider type
func (t *PluginRunnerToolImpl) GetProviderType() ToolProviderType {
	return ToolProviderTypePluginRunner
}

// GetTenantID returns the tenant ID
func (t *PluginRunnerToolImpl) GetTenantID() string {
	return t.tenantID
}

// Invoke invokes the tool with the given parameters
func (t *PluginRunnerToolImpl) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]ToolInvokeMessage, error) {
	// Merge runtime parameters with tool parameters
	mergedParams := make(map[string]interface{})
	if t.runtime != nil {
		for k, v := range t.runtime.RuntimeParameters {
			mergedParams[k] = v
		}
	}
	for k, v := range toolParameters {
		mergedParams[k] = v
	}

	return t.manager.InvokeTool(ctx, t.tenantID, userID, t.providerName, t.entity.Identity.Name, mergedParams)
}

// GetRuntimeParameters gets the runtime parameters for the tool
func (t *PluginRunnerToolImpl) GetRuntimeParameters(
	ctx context.Context,
	conversationID *string,
	appID *string,
	messageID *string,
) ([]ToolParameter, error) {
	return t.entity.Parameters, nil
}

// ForkToolRuntime creates a copy of the tool with new runtime parameters
func (t *PluginRunnerToolImpl) ForkToolRuntime(runtime *ToolRuntime) Tool {
	newTool := &PluginRunnerToolImpl{
		entity:       t.entity,
		tenantID:     t.tenantID,
		providerName: t.providerName,
		manager:      t.manager,
		runtime:      runtime,
	}
	if runtime != nil && runtime.TenantID != "" {
		newTool.tenantID = runtime.TenantID
	}
	return newTool
}

// ValidateCredentials validates the credentials
func (t *PluginRunnerToolImpl) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	valid, err := t.manager.ValidateCredentials(ctx, t.tenantID, "", t.providerName, credentials)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("credentials validation failed")
	}
	return nil
}
