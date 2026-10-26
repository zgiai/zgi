package skills

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	toolprovider "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestConnectorNativeSkillToolResolvesAndCarriesCurrentIdentity(t *testing.T) {
	t.Parallel()

	definition := connectorSkillTestDefinition()
	router := &connectorSkillRouterSpy{}
	dispatcher, err := connectordispatch.New(router)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := toolprovider.NewProvider(definition)
	if err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
	operation := definition.Operations[0]
	resolved := connectorSkillResolved(definition, operation, true)
	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.NewString()
	resolver := &connectorIdentityResolverSpy{}
	execCtx := ExecutionContext{
		OrganizationID: organizationID.String(), UserID: ownerID.String(),
		ConversationID: conversationID, MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat,
		ConnectorIdentityResolver: resolver,
		ConnectorActionResolver:   newConnectorActionResolverForTest(),
	}

	result, err := runtime.CallSkillTool(
		context.Background(), resolved, SkillConnectorRuntime, operation.ToolName,
		map[string]interface{}{}, execCtx, "call-native-1",
	)
	if err != nil || result == nil || router.calls != 1 || resolver.calls != 1 {
		t.Fatalf("native call result = %#v, err = %v, router calls = %d, resolver calls = %d", result, err, router.calls, resolver.calls)
	}
	request := resolver.requests[0]
	if request.OrganizationID != organizationID || request.OwnerAccountID != ownerID ||
		request.ActingPrincipalID != ownerID || request.ConnectorID.String() != definition.ID ||
		request.OperationID.String() != operation.ID || request.Target.Type != core.TargetTypeChat ||
		request.Target.ID != conversationID || !request.Interactive ||
		!strings.HasPrefix(request.LogicalOperationID, "connector-lookup-") ||
		!strings.HasPrefix(request.CorrelationID, "connector-correlation-") {
		t.Fatalf("identity resolution request = %#v", request)
	}
	invocation := router.requests[0]
	if invocation.BindingID != resolver.bindingID || invocation.ConnectionID != resolver.connectionID ||
		!strings.HasPrefix(invocation.LogicalOperationID, "connector-operation-") || invocation.CorrelationID != request.CorrelationID ||
		invocation.Target != request.Target || invocation.OperationID != request.OperationID {
		t.Fatalf("Connector execution request = %#v, resolution request = %#v", invocation, request)
	}

	// A new model call ID still resolves the same persisted action, allowing
	// the Router's durable claim to suppress resubmission.
	if _, err := runtime.CallSkillTool(
		context.Background(), resolved, SkillConnectorRuntime, operation.ToolName,
		map[string]interface{}{}, execCtx, "call-native-2",
	); err != nil {
		t.Fatal(err)
	}
	if len(resolver.requests) != 2 || resolver.requests[1].LogicalOperationID != request.LogicalOperationID ||
		resolver.requests[1].CorrelationID != request.CorrelationID {
		t.Fatalf("retry identities = %#v", resolver.requests)
	}
	if router.requests[1].LogicalOperationID != invocation.LogicalOperationID {
		t.Fatal("new tool call ID created another action")
	}
}

// A DigitalHuman host is not an AIChat or Agent caller even though it reuses
// the chat runtime. The exact host id, published version, owner and persisted
// binding instance must survive into the Connector identity, and the host's
// own interactivity must be the only source of the interactive flag.
func TestConnectorDigitalHumanToolCarriesExactHostBindingIdentity(t *testing.T) {
	t.Parallel()

	for _, interactive := range []bool{true, false} {
		interactive := interactive
		t.Run(fmt.Sprintf("interactive=%t", interactive), func(t *testing.T) {
			t.Parallel()

			definition := connectorSkillTestDefinition()
			router := &connectorSkillRouterSpy{}
			dispatcher, err := connectordispatch.New(router)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := toolprovider.NewProvider(definition)
			if err != nil {
				t.Fatal(err)
			}
			manager := tools.NewToolManager(nil)
			if err := manager.RegisterProvider(provider); err != nil {
				t.Fatal(err)
			}
			runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
			operation := definition.Operations[0]
			resolved := connectorSkillResolved(definition, operation, true)
			instanceID := uuid.NewString()
			resolved.Skills[0].Tools[0].ConnectorBindingInstanceID = instanceID

			organizationID := uuid.New()
			ownerID := uuid.New()
			digitalHumanID := uuid.NewString()
			versionID := uuid.NewString()
			resolver := &connectorIdentityResolverSpy{}
			execCtx := ExecutionContext{
				OrganizationID: organizationID.String(), UserID: ownerID.String(),
				ConversationID: uuid.NewString(), AppID: digitalHumanID, MessageID: uuid.NewString(),
				ConnectorRunID:            uuid.NewString(),
				InvokeFrom:                tools.ToolInvokeFromAIChat,
				ConnectorTargetType:       core.TargetTypeDigitalHuman,
				ConnectorTargetScope:      core.ScopePublished,
				ConnectorTargetVersionID:  versionID,
				ConnectorInteractive:      interactive,
				ConnectorIdentityResolver: resolver,
				ConnectorActionResolver:   newConnectorActionResolverForTest(),
			}
			result, err := runtime.CallSkillTool(
				context.Background(), resolved, SkillConnectorRuntime, operation.ToolName,
				map[string]interface{}{}, execCtx, "call-digital-human",
			)
			if err != nil || result == nil || resolver.calls != 1 || router.calls != 1 {
				t.Fatalf(
					"digital human call = (%#v, %v), resolver calls = %d, router calls = %d",
					result, err, resolver.calls, router.calls,
				)
			}
			wantTarget := core.TargetRef{
				Type: core.TargetTypeDigitalHuman, ID: digitalHumanID,
				Scope: core.ScopePublished, VersionID: versionID, BindingInstanceID: instanceID,
			}
			request := resolver.requests[0]
			if request.Target != wantTarget {
				t.Fatalf("digital human target = %#v, want %#v", request.Target, wantTarget)
			}
			switch request.Target.Type {
			case core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow:
				t.Fatalf("digital human target impersonated another surface: %#v", request.Target)
			}
			if request.OrganizationID != organizationID || request.OwnerAccountID != ownerID ||
				request.ActingPrincipalID != ownerID {
				t.Fatalf("digital human owner identity = %#v", request)
			}
			if request.Interactive != interactive {
				t.Fatalf("interactive = %v, want %v", request.Interactive, interactive)
			}
			if invocation := router.requests[0]; invocation.Target != wantTarget {
				t.Fatalf("dispatched target = %#v, want %#v", invocation.Target, wantTarget)
			}
		})
	}
}

// ConnectorBindingInstanceID is meaningful only for a DigitalHuman host. A Chat
// or Agent caller must keep producing its existing target, so a projected
// instance can never leak into another surface's identity.
func TestConnectorBindingInstanceIDDoesNotChangeChatOrAgentTargets(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		invokeFrom tools.ToolInvokeFrom
		targetType core.TargetType
	}{
		{name: "chat", invokeFrom: tools.ToolInvokeFromAIChat, targetType: core.TargetTypeChat},
		{name: "agent", invokeFrom: tools.ToolInvokeFromAgent, targetType: core.TargetTypeAgent},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			definition := connectorSkillTestDefinition()
			router := &connectorSkillRouterSpy{}
			dispatcher, err := connectordispatch.New(router)
			if err != nil {
				t.Fatal(err)
			}
			provider, err := toolprovider.NewProvider(definition)
			if err != nil {
				t.Fatal(err)
			}
			manager := tools.NewToolManager(nil)
			if err := manager.RegisterProvider(provider); err != nil {
				t.Fatal(err)
			}
			runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
			operation := definition.Operations[0]
			resolved := connectorSkillResolved(definition, operation, true)
			resolved.Skills[0].Tools[0].ConnectorBindingInstanceID = uuid.NewString()

			appID := uuid.NewString()
			resolver := &connectorIdentityResolverSpy{}
			execCtx := ExecutionContext{
				OrganizationID: uuid.NewString(), UserID: uuid.NewString(),
				ConversationID: uuid.NewString(), AppID: appID, MessageID: uuid.NewString(),
				InvokeFrom: test.invokeFrom, ConnectorIdentityResolver: resolver,
				ConnectorActionResolver: newConnectorActionResolverForTest(),
			}
			if test.invokeFrom == tools.ToolInvokeFromAgent {
				execCtx.RuntimeParameters = map[string]interface{}{"agent_runtime_source": "console"}
			}
			if _, err := runtime.CallSkillTool(
				context.Background(), resolved, SkillConnectorRuntime, operation.ToolName,
				map[string]interface{}{}, execCtx, "call-shared-surface",
			); err != nil {
				t.Fatal(err)
			}
			request := resolver.requests[0]
			if request.Target.Type != test.targetType || request.Target.BindingInstanceID != "" {
				t.Fatalf("target = %#v, want type %s without a binding instance", request.Target, test.targetType)
			}
		})
	}
}

func newConnectorActionResolverForTest() connectordispatch.ActionInstanceResolver {
	ledger := connectordispatch.ActionLedger{}
	return func(_ context.Context, identity connectordispatch.Identity, input any) (connectordispatch.Identity, error) {
		result, _, err := connectordispatch.ResolveActionInstance(&ledger, "turn", identity.CorrelationID, identity, input)
		return result, err
	}
}

func TestConnectorActionPersistenceFailureSubmitsNothing(t *testing.T) {
	definition := connectorSkillTestDefinition()
	router := &connectorSkillRouterSpy{}
	dispatcher, err := connectordispatch.New(router)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := toolprovider.NewProvider(definition)
	if err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
	execCtx := ExecutionContext{OrganizationID: uuid.NewString(), UserID: uuid.NewString(), ConversationID: uuid.NewString(), MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat, ConnectorIdentityResolver: &connectorIdentityResolverSpy{}, ConnectorActionResolver: func(context.Context, connectordispatch.Identity, any) (connectordispatch.Identity, error) {
		return connectordispatch.Identity{}, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}}
	op := definition.Operations[0]
	_, err = runtime.CallSkillTool(t.Context(), connectorSkillResolved(definition, op, true), SkillConnectorRuntime, op.ToolName, map[string]interface{}{}, execCtx, "call-persist-failed")
	if err == nil || router.calls != 0 {
		t.Fatalf("persistence failure reached provider: err=%v calls=%d", err, router.calls)
	}
}

func TestConnectorNativeSkillToolWithoutResolverFailsClosed(t *testing.T) {
	t.Parallel()

	definition := connectorSkillTestDefinition()
	router := &connectorSkillRouterSpy{}
	dispatcher, err := connectordispatch.New(router)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := toolprovider.NewProvider(definition)
	if err != nil {
		t.Fatal(err)
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(provider); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager).WithConnectorDispatcher(dispatcher)
	operation := definition.Operations[0]
	_, err = runtime.CallSkillTool(
		context.Background(), connectorSkillResolved(definition, operation, true),
		SkillConnectorRuntime, operation.ToolName, map[string]interface{}{}, ExecutionContext{
			OrganizationID: uuid.NewString(), UserID: uuid.NewString(),
			ConversationID: uuid.NewString(), MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat,
		}, "call-native-missing-resolver",
	)
	if !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) || router.calls != 0 {
		t.Fatalf("missing resolver error = %v, router calls = %d", err, router.calls)
	}
}

func TestConnectorAgentRuntimeInteractiveRequiresUserPresentSource(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		parameters map[string]interface{}
		want       bool
	}{
		{name: "console", parameters: map[string]interface{}{"agent_runtime_source": "console"}, want: true},
		{name: "webapp", parameters: map[string]interface{}{"agent_runtime_source": "webapp"}, want: true},
		{name: "normalized webapp", parameters: map[string]interface{}{"agent_runtime_source": " WEBAPP "}, want: true},
		{name: "external api", parameters: map[string]interface{}{"agent_runtime_source": "external-api"}},
		{name: "migration", parameters: map[string]interface{}{"agent_runtime_source": "migration"}},
		{name: "unknown", parameters: map[string]interface{}{"agent_runtime_source": "future-source"}},
		{name: "missing", parameters: nil},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := connectorAgentRuntimeInteractive(test.parameters); got != test.want {
				t.Fatalf("connectorAgentRuntimeInteractive() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNonNativeConnectorSkillToolCannotReachProvider(t *testing.T) {
	t.Parallel()

	legacyTool := &legacyConnectorSkillTool{entity: tools.ToolEntity{
		Identity:    tools.ToolIdentity{Name: "legacy_read", Provider: "legacy-provider", Label: tools.I18nText{"en_US": "Legacy read"}},
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false},
	}}
	legacyProvider := &legacyConnectorSkillProvider{tool: legacyTool}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(legacyProvider); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: "legacy-skill", TimeoutSeconds: 5},
		Tools: []SkillToolDefinition{{
			Name: "legacy_read", ProviderType: tools.ToolProviderTypeConnector, ProviderID: "legacy-provider",
			InputSchema: legacyTool.entity.InputSchema,
		}},
	}}}
	result, err := runtime.CallSkillTool(
		context.Background(), resolved, "legacy-skill", "legacy_read", map[string]interface{}{},
		ExecutionContext{OrganizationID: uuid.NewString(), UserID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat},
		"call-legacy",
	)
	if err == nil || result == nil || legacyTool.calls != 0 {
		t.Fatalf("legacy call result = %#v, err = %v, calls = %d", result, err, legacyTool.calls)
	}
}

func connectorSkillResolved(
	definition catalog.Definition,
	operation catalog.Operation,
	native bool,
) *ResolvedSkills {
	return &ResolvedSkills{Skills: []SkillDocument{{
		Metadata: SkillMetadata{ID: SkillConnectorRuntime, TimeoutSeconds: 5},
		Tools: []SkillToolDefinition{{
			Name: operation.ToolName, ProviderType: tools.ToolProviderTypeConnector, ProviderID: definition.ID,
			InputSchema: operation.InputSchema, OutputSchema: operation.OutputSchema,
			ConnectorNative: native, ConnectorOperationID: operation.ID,
		}},
	}}}
}

func connectorSkillTestDefinition() catalog.Definition {
	objectSchema := map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
	}
	return catalog.Definition{
		ID: "test-connector", PluginID: "test-plugin", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "test-connector", DriverID: "test-driver", AuthMethodID: "personal-key",
		Name: "Test Connector", Description: "Test Connector operations.",
		Classification:  catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeData},
		SetupMethod:     catalog.SetupMethodKey, Ownership: catalog.OwnershipPersonal,
		Surfaces:  catalog.FirstReleaseSurfaces(),
		KeyFields: []catalog.KeyField{{Name: "api_key", Label: "API Key", Required: true, Secret: true}},
		Operations: []catalog.Operation{{
			ID: "test.item.read", ToolName: "read_test_item", SkillID: SkillConnectorRuntime,
			Name: "Read test item", Description: "Read one test item.",
			InputSchema: objectSchema, OutputSchema: objectSchema,
			Effect: catalog.OperationEffectRead, RiskHint: catalog.OperationRiskHintLow, Idempotent: true,
		}},
	}
}

type connectorIdentityResolverSpy struct {
	requests     []connectordispatch.IdentityResolutionRequest
	calls        int
	bindingID    uuid.UUID
	connectionID uuid.UUID
}

func (spy *connectorIdentityResolverSpy) ResolveConnectorIdentity(
	_ context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	spy.calls++
	spy.requests = append(spy.requests, request)
	if spy.bindingID == uuid.Nil {
		spy.bindingID = uuid.New()
	}
	if spy.connectionID == uuid.Nil {
		spy.connectionID = uuid.New()
	}
	return connectordispatch.Identity{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, BindingID: spy.bindingID, ConnectionID: spy.connectionID,
		ConnectorID: request.ConnectorID, Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
		Interactive: request.Interactive, Confirmation: request.Confirmation,
	}, nil
}

type connectorSkillRouterSpy struct {
	requests []execution.ExecutionRequest
	calls    int
}

func (spy *connectorSkillRouterSpy) Route(
	_ context.Context,
	request execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	spy.calls++
	spy.requests = append(spy.requests, request)
	return execution.ExecutionResult{
		State: execution.ExecutionStateCompleted,
		Value: map[string]interface{}{"ok": true},
	}, nil
}

type legacyConnectorSkillProvider struct {
	tool *legacyConnectorSkillTool
}

func (provider *legacyConnectorSkillProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: "legacy-provider", Label: tools.I18nText{"en_US": "Legacy provider"}},
		ProviderType: tools.ToolProviderTypeConnector, Tools: []tools.ToolEntity{provider.tool.entity},
	}
}

func (*legacyConnectorSkillProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (provider *legacyConnectorSkillProvider) GetTool(name string) (tools.Tool, error) {
	if name != provider.tool.entity.Identity.Name {
		return nil, tools.ErrToolNotFound
	}
	return provider.tool, nil
}

func (provider *legacyConnectorSkillProvider) GetTools() []tools.Tool {
	return []tools.Tool{provider.tool}
}

func (*legacyConnectorSkillProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

type legacyConnectorSkillTool struct {
	entity  tools.ToolEntity
	runtime *tools.ToolRuntime
	calls   int
}

func (tool *legacyConnectorSkillTool) GetEntity() tools.ToolEntity { return tool.entity }

func (*legacyConnectorSkillTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (tool *legacyConnectorSkillTool) GetTenantID() string {
	if tool.runtime == nil {
		return ""
	}
	return tool.runtime.TenantID
}

func (tool *legacyConnectorSkillTool) Invoke(
	_ context.Context,
	_ string,
	parameters map[string]interface{},
	_ *string,
	_ *string,
	_ *string,
) ([]tools.ToolInvokeMessage, error) {
	tool.calls++
	if !reflect.DeepEqual(parameters, map[string]interface{}{}) {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: map[string]interface{}{"legacy": true}}}, nil
}

func (*legacyConnectorSkillTool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (tool *legacyConnectorSkillTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	tool.runtime = runtime
	return tool
}

func (*legacyConnectorSkillTool) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}
