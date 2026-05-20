package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"

	// Import to trigger init() registration
	_ "github.com/zgiai/ginext/internal/modules/tools/builtin/calculator"
	_ "github.com/zgiai/ginext/internal/modules/tools/builtin/filegenerator"
	_ "github.com/zgiai/ginext/internal/modules/tools/builtin/time"
)

func TestToolManager_RegisterBuiltinProvider(t *testing.T) {
	// Create a tool manager without plugin runner
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	// Register all builtin providers
	manager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Verify time provider is registered
	provider, err := manager.GetProvider(context.Background(), tools.ToolProviderTypeBuiltin, "time", "test-tenant")
	require.NoError(t, err)
	assert.NotNil(t, provider)

	// Verify provider entity
	entity := provider.GetEntity()
	assert.Equal(t, "time", entity.Identity.Name)
	assert.Equal(t, "Time Tools", entity.Identity.Label.Get("en_US"))

	// Verify tool is accessible
	tool, err := provider.GetTool("current_time")
	require.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify tool entity
	toolEntity := tool.GetEntity()
	assert.Equal(t, "current_time", toolEntity.Identity.Name)
}

func TestToolManager_InvokeBuiltinTool(t *testing.T) {
	// Create a tool manager without plugin runner
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	// Register all builtin providers
	manager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// Get the tool runtime
	tool, err := manager.GetToolRuntime(
		context.Background(),
		tools.ToolProviderTypeBuiltin,
		"time",
		"current_time",
		"test-tenant",
		tools.ToolInvokeFromWorkflow,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, tool)

	// Invoke the tool
	messages, err := tool.Invoke(
		context.Background(),
		"test-user",
		map[string]interface{}{
			"timezone": "UTC",
			"format":   "2006-01-02",
		},
		nil, nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	// Verify message type and content
	assert.Equal(t, tools.ToolInvokeMessageTypeText, messages[0].Type)
	assert.NotEmpty(t, messages[0].Text)
	t.Logf("Returned time: %s", messages[0].Text)
}

func TestToolManager_ListBuiltinProviders(t *testing.T) {
	// Create a tool manager without plugin runner
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	// Register all builtin providers
	manager.RegisterBuiltinProviders(builtin.GetAllProviders())

	// List all builtin providers
	providers := manager.ListProviders(tools.ToolProviderTypeBuiltin)
	require.GreaterOrEqual(t, len(providers), 2)
	assert.True(t, hasProvider(providers, "time"))
	assert.True(t, hasProvider(providers, "calculator"))
	assert.True(t, hasProvider(providers, "file_generator"))
}

func hasProvider(providers []tools.ToolProvider, name string) bool {
	for _, provider := range providers {
		if provider.GetEntity().Identity.Name == name {
			return true
		}
	}
	return false
}

type runtimeAwareTool struct {
	entity   tools.ToolEntity
	tenantID string
	runtime  *tools.ToolRuntime
}

func newRuntimeAwareTool() *runtimeAwareTool {
	return &runtimeAwareTool{
		entity: tools.ToolEntity{
			Identity: tools.ToolIdentity{
				Name:     "runtime_aware",
				Provider: "test_runtime",
				Label: tools.I18nText{
					"en_US": "Runtime Aware",
				},
			},
		},
	}
}

func (t *runtimeAwareTool) GetEntity() tools.ToolEntity {
	return t.entity
}

func (t *runtimeAwareTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (t *runtimeAwareTool) GetTenantID() string {
	return t.tenantID
}

func (t *runtimeAwareTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	return []tools.ToolInvokeMessage{}, nil
}

func (t *runtimeAwareTool) GetRuntimeParameters(
	ctx context.Context,
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *runtimeAwareTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &runtimeAwareTool{
		entity:   t.entity,
		tenantID: runtime.TenantID,
		runtime:  runtime,
	}
}

func (t *runtimeAwareTool) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

type runtimeAwareProvider struct {
	tool tools.Tool
}

func newRuntimeAwareProvider() *runtimeAwareProvider {
	return &runtimeAwareProvider{
		tool: newRuntimeAwareTool(),
	}
}

func (p *runtimeAwareProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name: "test_runtime",
			Label: tools.I18nText{
				"en_US": "Test Runtime",
			},
			Description: tools.I18nText{
				"en_US": "Test runtime provider",
			},
		},
		ProviderType: tools.ToolProviderTypeBuiltin,
	}
}

func (p *runtimeAwareProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

func (p *runtimeAwareProvider) GetTool(name string) (tools.Tool, error) {
	if name != "runtime_aware" {
		return nil, tools.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *runtimeAwareProvider) GetTools() []tools.Tool {
	return []tools.Tool{p.tool}
}

func (p *runtimeAwareProvider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

func TestToolManager_GetToolRuntimeInjectsRuntime(t *testing.T) {
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	require.NoError(t, manager.RegisterProvider(newRuntimeAwareProvider()))

	tool, err := manager.GetToolRuntime(
		context.Background(),
		tools.ToolProviderTypeBuiltin,
		"test_runtime",
		"runtime_aware",
		"workspace-123",
		tools.ToolInvokeFromWorkflow,
		"",
	)
	require.NoError(t, err)

	runtimeTool, ok := tool.(*runtimeAwareTool)
	require.True(t, ok)
	require.NotNil(t, runtimeTool.runtime)
	assert.Equal(t, "workspace-123", runtimeTool.tenantID)
	assert.Equal(t, "workspace-123", runtimeTool.runtime.TenantID)
	assert.Equal(t, tools.ToolInvokeFromWorkflow, runtimeTool.runtime.InvokeFrom)
}
