package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"

	// Import to trigger init() registration
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/chartgenerator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/filegenerator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/intentrouter"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
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
		nil,
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
	assert.True(t, hasProvider(providers, "chart_generator"))
	assert.True(t, hasProvider(providers, "file_generator"))
	assert.True(t, hasProvider(providers, "intent_router"))
}

func TestToolManager_RunnerProviderTypeAlias(t *testing.T) {
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	provider := tools.NewPluginRunnerProvider(
		tools.ToolProviderEntity{
			Identity: tools.ToolProviderIdentity{
				Name: "echo",
				Label: tools.I18nText{
					"en_US": "Echo",
				},
			},
			ProviderType: tools.ToolProviderTypePluginRunner,
		},
		"test-tenant",
		nil,
	)
	require.NoError(t, manager.RegisterProvider(provider))

	providers := manager.ListProviders(tools.ToolProviderTypeRunner)
	require.Len(t, providers, 1)
	assert.Equal(t, "echo", providers[0].GetEntity().Identity.Name)

	got, err := manager.GetProvider(context.Background(), tools.ToolProviderTypeRunner, "echo", "test-tenant")
	require.NoError(t, err)
	assert.Equal(t, "echo", got.GetEntity().Identity.Name)
	assert.Equal(t, tools.ToolProviderTypePluginRunner, tools.NormalizeToolProviderType(tools.ToolProviderTypeRunner))
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
	capture  *runtimeAwareCapture
}

type runtimeAwareCapture struct {
	runtime        *tools.ToolRuntime
	userID         string
	parameters     map[string]interface{}
	conversationID *string
	appID          *string
	messageID      *string
}

func newRuntimeAwareTool(captures ...*runtimeAwareCapture) *runtimeAwareTool {
	capture := &runtimeAwareCapture{}
	if len(captures) > 0 && captures[0] != nil {
		capture = captures[0]
	}
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
		capture: capture,
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
	if t.capture != nil {
		t.capture.runtime = t.runtime
		t.capture.userID = userID
		t.capture.parameters = toolParameters
		t.capture.conversationID = conversationID
		t.capture.appID = appID
		t.capture.messageID = messageID
	}
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
		capture:  t.capture,
	}
}

func (t *runtimeAwareTool) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

type runtimeAwareProvider struct {
	tool    tools.Tool
	capture *runtimeAwareCapture
}

func newRuntimeAwareProvider() *runtimeAwareProvider {
	capture := &runtimeAwareCapture{}
	return &runtimeAwareProvider{
		tool:    newRuntimeAwareTool(capture),
		capture: capture,
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
		nil,
	)
	require.NoError(t, err)

	runtimeTool, ok := tool.(*runtimeAwareTool)
	require.True(t, ok)
	require.NotNil(t, runtimeTool.runtime)
	assert.Equal(t, "workspace-123", runtimeTool.tenantID)
	assert.Equal(t, "workspace-123", runtimeTool.runtime.TenantID)
	assert.Equal(t, tools.ToolInvokeFromWorkflow, runtimeTool.runtime.InvokeFrom)
}

func TestToolEngine_InvokeForWorkflowUsesWorkflowInvokePath(t *testing.T) {
	manager := tools.NewToolManager(nil)
	require.NotNil(t, manager)

	provider := newRuntimeAwareProvider()
	require.NoError(t, manager.RegisterProvider(provider))

	engine := tools.NewToolEngine(manager)
	result, err := engine.InvokeForWorkflow(context.Background(), tools.WorkflowToolInvokeRequest{
		TenantID:     "workspace-123",
		AppID:        "workflow-app",
		UserID:       "workflow-user",
		ProviderType: tools.ToolProviderTypeBuiltin,
		ProviderID:   "test_runtime",
		ToolName:     "runtime_aware",
		ToolConfigurations: map[string]interface{}{
			"value": "from-workflow",
		},
		ToolCredentials: map[string]interface{}{
			"token": "workflow-credential",
		},
		ConversationID: "workflow-conversation",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	require.NotNil(t, provider.capture.runtime)
	assert.Equal(t, tools.ToolInvokeFromWorkflow, provider.capture.runtime.InvokeFrom)
	assert.Equal(t, "workspace-123", provider.capture.runtime.TenantID)
	assert.Equal(t, "workflow-user", provider.capture.userID)
	assert.Equal(t, "from-workflow", provider.capture.parameters["value"])
	assert.Equal(t, map[string]interface{}{"token": "workflow-credential"}, provider.capture.parameters["__credentials"])
	require.NotNil(t, provider.capture.conversationID)
	assert.Equal(t, "workflow-conversation", *provider.capture.conversationID)
	require.NotNil(t, provider.capture.appID)
	assert.Equal(t, "workflow-app", *provider.capture.appID)
	assert.Nil(t, provider.capture.messageID)
}
