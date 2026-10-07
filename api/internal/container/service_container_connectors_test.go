package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/gorm"
)

func TestConnectorPluginMaintainerChangesRequireAuditableActorIdentity(t *testing.T) {
	container := &ServiceContainer{}
	if _, err := container.AdmitConnectorPluginDirectories(context.Background(), nil); !errors.Is(err, connectorcatalog.ErrPluginAdmissionUnavailable) {
		t.Fatalf("AdmitConnectorPluginDirectories(no actor) error = %v", err)
	}
	if _, err := container.DisableConnectorPlugin(context.Background(), "zgi.github", true); !errors.Is(err, connectorcatalog.ErrPluginAdmissionUnavailable) {
		t.Fatalf("DisableConnectorPlugin(no actor) error = %v", err)
	}
	if _, err := container.RemoveConnectorPlugin(context.Background(), "zgi.github", true); !errors.Is(err, connectorcatalog.ErrPluginAdmissionUnavailable) {
		t.Fatalf("RemoveConnectorPlugin(no actor) error = %v", err)
	}
}

func TestConnectorServicesInitializeWithoutLegacyExternalIntegrations(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		Connectors: config.ConnectorsConfig{
			CredentialActiveKeyID: "connector-primary",
			CredentialKeys: map[string]string{
				"connector-primary": strings.Repeat("c", 32),
			},
		},
	}}

	definitionCatalog := container.GetConnectorCatalog()
	githubDefinition, githubFound := definitionCatalog.Get("github")
	exaDefinition, exaFound := definitionCatalog.Get("web-search")
	if !githubFound || githubDefinition.Classification != connectorcatalog.ClassificationConnector {
		t.Fatalf("GitHub Connector definition = %#v, found=%v", githubDefinition, githubFound)
	}
	if !exaFound || exaDefinition.Classification != connectorcatalog.ClassificationConnector ||
		exaDefinition.PluginVersion != "1.2.1-digitalhuman.1" || len(exaDefinition.CapabilityTypes) != 1 ||
		exaDefinition.CapabilityTypes[0] != connectorcatalog.CapabilityTypeData {
		t.Fatalf("Exa Connector definition = %#v, found=%v", exaDefinition, exaFound)
	}
	for _, definition := range []connectorcatalog.Definition{githubDefinition, exaDefinition} {
		if len(definition.PluginDigest) != 64 || definition.SourceDialect != connectorcatalog.SourceDialectAgentPluginsV1 {
			t.Fatalf("built-in definition bypassed Agent Plugin admission: %#v", definition)
		}
	}
	if container.GetConnectorCandidateVerifier() == nil || container.GetConnectorCredentialSealer() == nil ||
		container.GetConnectorConnectionService() == nil || container.GetConnectorMaintenanceService() == nil {
		t.Fatal("Connector service graph is incomplete")
	}
	if container.GetConnectorBindingRepository() == nil || container.GetConnectorTargetAuthority() == nil ||
		container.GetConnectorBindingService() == nil || container.GetConnectorAuditRepository() == nil {
		t.Fatal("Connector-native binding, authority, and audit graph is incomplete")
	}
	// Rollout constructors retain a database handle but do not query it while
	// assembling the graph. Install a non-nil test handle only after the older
	// container dependencies (which intentionally exercise nil-db construction)
	// have initialized.
	container.db = &gorm.DB{}
	if container.GetConnectorCredentialResolver() == nil || container.GetConnectorNativeProviderInvoker() == nil ||
		container.GetConnectorTrustedRiskResolver() == nil ||
		container.GetConnectorExecutionService() == nil ||
		container.GetConnectorExecutionRouter() == nil {
		t.Fatal("Connector-native execution adapter graph is incomplete")
	}
	if container.GetConnectorChatRuntimeResolver() == nil || container.GetConnectorAgentRuntimeResolver() == nil {
		t.Fatal("Connector Chat/Agent runtime resolvers must be available without an opt-in flag")
	}
}

func TestBuiltinConnectorCatalogProjectsSupportedSurfacesForDigitalHumans(t *testing.T) {
	container := &ServiceContainer{}
	catalog := container.GetConnectorCatalog()

	for _, id := range []string{"github", "web-search", "feishu", "dingtalk", "wecom", "weixin", "serverops_ssh"} {
		definition, found := catalog.Get(id)
		if !found {
			t.Fatalf("builtin connector %q is missing from the catalog", id)
		}
		if !definition.Supports(connectorcatalog.SurfaceDigitalHuman) {
			t.Fatalf("builtin connector %q does not support the Digital-Human surface: %v", id, definition.Surfaces)
		}
	}

	// Mail providers retain their separate mail credential contract and are not
	// silently exposed as Digital-Human host connectors by the projection.
	for _, id := range []string{"gmail", "qqmail", "netease163"} {
		definition, found := catalog.Get(id)
		if !found {
			t.Fatalf("builtin mail connector %q is missing from the catalog", id)
		}
		if definition.Supports(connectorcatalog.SurfaceDigitalHuman) {
			t.Fatalf("mail connector %q was unexpectedly projected to Digital-Human: %v", id, definition.Surfaces)
		}
	}
}

// TestConnectorCandidateVerifierRegistersNetworkFreeSSHServerOpsDriver pins the
// production SSH Server Operations wiring. providers.NewRuntime fails closed
// when any non-messaging, non-mail built-in lacks its driver, so a successful
// verifier assembly while the SSH contract is admitted proves the SSH driver is
// registered. Assembly builds the capability through serverops.New, which is
// constructor-only and never dials, so the whole graph is constructed without
// network work and remains the single cached instance.
func TestConnectorCandidateVerifierRegistersNetworkFreeSSHServerOpsDriver(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	if _, admitted := container.GetConnectorCatalog().Get(connectorcatalog.ServerOpsSSHConnectorID); !admitted {
		t.Fatal("SSH Server Operations contract is not admitted")
	}
	verifier := container.GetConnectorCandidateVerifier()
	if verifier == nil {
		t.Fatal("candidate verifier did not register the SSH Server Operations driver")
	}
	if container.GetConnectorCandidateVerifier() != verifier {
		t.Fatal("candidate verifier getter is not stable")
	}
}

// TestServerOpsSSHHostKeyProbeReusesCandidateCapability pins the shared SSH
// capability wiring: the probe getter returns the cached adapter client, and
// assembling the candidate verifier reuses the exact same *serverops.Service
// instead of building a second capability for probing.
func TestServerOpsSSHHostKeyProbeReusesCandidateCapability(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	probe := container.GetServerOpsSSHHostKeyProbe()
	if probe == nil {
		t.Fatal("host-key probe is not wired")
	}
	if container.GetServerOpsSSHHostKeyProbe() != probe {
		t.Fatal("host-key probe getter is not stable")
	}
	service := container.connectorServerOpsService
	client := container.connectorServerOpsSSHClient
	if service == nil || client == nil {
		t.Fatal("server operations capability/client was not cached")
	}
	if any(probe) != any(client) {
		t.Fatal("probe does not return the cached adapter client")
	}
	if container.GetConnectorCandidateVerifier() == nil {
		t.Fatal("candidate verifier did not register the SSH driver")
	}
	if container.connectorServerOpsService != service || container.connectorServerOpsSSHClient != client {
		t.Fatal("candidate verifier built a second server operations capability")
	}
}

func TestConnectorPluginAdmissionRefreshesToolsAndInvalidPackageIsIsolated(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	definitionCatalog := container.GetConnectorCatalog()
	store := &containerPluginDefinitionStore{state: connectorcatalog.PluginRuntimeState{
		Statuses: map[string]connectorcatalog.PluginDefinitionStatus{},
	}}
	base, found := definitionCatalog.Get("github")
	if !found {
		t.Fatal("built-in GitHub definition is missing")
	}
	compatible := base
	compatible.PluginDigest = ""
	validRoot := writeContainerPluginPackage(t, compatible)
	// Directory packages hash all files, unlike embedded manifest packages.
	// Seed the accepted directory version so re-admission checks its exact bytes.
	loaded, err := container.GetConnectorPluginLoader().LoadDirectory(context.Background(), validRoot)
	if err != nil || len(loaded.Failures) != 0 {
		t.Fatalf("load accepted directory package: %v", err)
	}
	store.state.Definitions = loaded.Definitions
	store.state.Statuses[base.PluginID] = connectorcatalog.PluginDefinitionStatusActive
	lifecycleStore := &containerPluginLifecycleStore{}
	manager, err := connectorcatalog.NewPluginManager(
		container.GetConnectorPluginLoader(), definitionCatalog, store, lifecycleStore,
		connectorcatalog.WithPluginPackageAdmissionPolicy(
			connectorcatalog.ValidateBuiltinPackage,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	container.connectorPluginManager = manager
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	container.toolManager = tools.NewToolManager(nil)
	if err := container.registerConnectorToolProviders(container.toolManager); err != nil {
		t.Fatal(err)
	}

	changed := compatible
	changed.Name = "GitHub Cloud"
	changedRoot := writeContainerPluginPackage(t, changed)
	unsupported := base
	unsupported.ID = "calendar"
	unsupported.PluginID = "zgi.calendar"
	unsupported.PluginDigest = ""
	unsupported.ProviderID = "calendar"
	unsupported.DriverID = "calendar-rest"
	unsupported.AuthMethodID = "api_key"
	unsupported.Name = "Calendar"
	unsupported.Description = "Calendar Connector"
	unsupported.Operations = append([]connectorcatalog.Operation(nil), base.Operations[:1]...)
	unsupported.Operations[0].ID = "calendar.events.list"
	unsupported.Operations[0].ToolName = "list_calendar_events"
	unsupported.Operations[0].Name = "List calendar events"
	unsupportedRoot := writeContainerPluginPackage(t, unsupported)
	invalidRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(invalidRoot, "plugin.json"), []byte(`{"$schema":"unsupported"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	maintainerContext := connectoraudit.WithPlatformActor(context.Background(), connectoraudit.PlatformActor{
		Kind: connectoraudit.PlatformActorPlatformMaintainer,
		ID:   uuid.NewString(),
	})
	changedReport, err := container.AdmitConnectorPluginDirectories(maintainerContext, []string{changedRoot})
	if err != nil || len(changedReport.Failures) != 1 || len(changedReport.AdmittedDefinitions) != 0 {
		t.Fatalf("same-version content replacement was not rejected: %v", err)
	}
	report, err := container.AdmitConnectorPluginDirectories(
		maintainerContext, []string{invalidRoot, unsupportedRoot, validRoot},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 2 || len(report.AdmittedDefinitions) != 1 ||
		report.AdmittedDefinitions[0].ID != compatible.ID {
		t.Fatalf("plugin admission report = %#v", report)
	}
	provider, err := container.toolManager.GetProvider(
		context.Background(), tools.ToolProviderTypeConnector, compatible.ID, "unused-tenant",
	)
	if err != nil || len(provider.GetEntity().Tools) != len(compatible.Operations) {
		t.Fatalf("updated Connector tool provider = %#v, %v", provider, err)
	}
	active, found := manager.Get("github")
	if !found || active.PluginVersion != compatible.PluginVersion || active.Name != compatible.Name {
		t.Fatalf("compatible package was not activated: %#v, found=%v", active, found)
	}
	if _, found := manager.Get("web-search"); !found {
		t.Fatal("invalid package hid an independent valid built-in plugin")
	}
	if _, found := manager.Get(unsupported.ID); found {
		t.Fatal("unsupported runtime driver became usable")
	}

	lifecycleStore.impact = connectorcatalog.PluginImpact{
		PluginID: compatible.PluginID,
		Definitions: []connectorcatalog.DefinitionImpact{{
			ConnectorID: compatible.ID, Status: connectorcatalog.PluginDefinitionStatusActive,
		}},
		Connections: []connectorcatalog.ConnectionImpact{{
			ID: uuid.New(), ConnectorID: compatible.ID, State: "ready",
		}},
	}
	if impact, err := container.DisableConnectorPlugin(maintainerContext, compatible.PluginID, false); err == nil ||
		!impact.HasReferences() {
		t.Fatalf("default disable = %#v, %v", impact, err)
	}
	if _, found := manager.Get(compatible.ID); !found {
		t.Fatal("blocked disable hid active plugin")
	}
	if _, err := container.DisableConnectorPlugin(maintainerContext, compatible.PluginID, true); err != nil {
		t.Fatal(err)
	}
	if _, found := manager.Get(compatible.ID); found {
		t.Fatal("disabled plugin remained in active catalog")
	}
	if _, err := container.toolManager.GetProvider(
		context.Background(), tools.ToolProviderTypeConnector, compatible.ID, "unused-tenant",
	); err == nil {
		t.Fatal("disabled plugin remained in Tool discovery")
	}
}

func TestConnectorRuntimeFailsClosedWithoutConfiguration(t *testing.T) {
	container := &ServiceContainer{}
	if resolver := container.GetConnectorChatRuntimeResolver(); resolver != nil {
		t.Fatalf("missing configuration exposed Chat runtime resolver: %#v", resolver)
	}
	if resolver := container.GetConnectorAgentRuntimeResolver(); resolver != nil {
		t.Fatalf("missing configuration exposed Agent runtime resolver: %#v", resolver)
	}
	if protector := container.GetConnectorConfirmationPayloadProtector(); protector != nil {
		t.Fatalf("missing configuration exposed confirmation protector: %#v", protector)
	}
	if store := container.GetConnectorConfirmationContinuationStore(); store != nil {
		t.Fatalf("missing configuration exposed confirmation store: %#v", store)
	}
	confirmationService := container.GetConnectorRuntimeConfirmationService()
	if confirmationService == nil {
		t.Fatal("missing configuration did not expose its fail-closed runtime boundary")
	}
	_, err := confirmationService.Continue(context.Background(), connectorruntimeconfirmation.Command{})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConnectorUnavailable) {
		t.Fatalf("unconfigured confirmation Continue() error = %v, want unavailable", err)
	}
	if container.connectorCredentialCipher != nil || container.connectorRuntimeConfirmation != nil {
		t.Fatal("unconfigured confirmation boundary initialized Connector cryptography or runtime")
	}
}

func TestConnectorConfirmationPayloadProtectorRoundTripsInputAndBindsAAD(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	protector := container.GetConnectorConfirmationPayloadProtector()
	if protector == nil {
		t.Fatal("confirmation payload protector is nil")
	}
	organizationID, ownerID := uuid.New(), uuid.New()
	receipt := connectorexecution.RouteClaimReceipt(uuid.New().String())
	conversationID := uuid.New().String()
	secret := strings.Repeat("连接器机密输入", 2500)
	definitions, err := connectorcatalog.BuiltinDefinitions()
	if err != nil || len(definitions) == 0 || len(definitions[0].Operations) == 0 {
		t.Fatalf("load accepted Connector contract: %v", err)
	}
	definition, operation := definitions[0], definitions[0].Operations[0]
	contractDigest, err := connectorcatalog.DefinitionContractDigest(definition)
	if err != nil {
		t.Fatalf("accepted Connector contract digest: %v", err)
	}
	acceptedOperation := connectorcatalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID,
		PluginVersion: definition.PluginVersion, PluginDigest: definition.PluginDigest,
		ContractDigest: contractDigest, Definition: definition, Operation: operation,
	}
	frozen := connectorexecution.FrozenConfirmationRequest{
		RouteReceipt: receipt,
		Request: connectorexecution.ExecutionRequest{
			OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
			BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: connectorcore.Identifier(definition.ID),
			Target:             connectorcore.TargetRef{Type: connectorcore.TargetTypeChat, ID: conversationID},
			OperationID:        connectorcore.Identifier(operation.ID),
			LogicalOperationID: "logical-operation-1", CorrelationID: "correlation-1",
			Interactive: true, Input: map[string]any{"query": secret, "count": float64(2)},
		},
		Origin: connectorexecution.ConfirmationOrigin{
			Surface: connectorcore.TargetTypeChat, ConversationID: conversationID, MessageID: uuid.New().String(),
		},
		AcceptedOperation: acceptedOperation,
	}
	envelope, err := protector.Seal(context.Background(), organizationID, receipt, frozen)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if strings.Contains(string(envelope), secret) || strings.TrimSpace(string(envelope)) == "" {
		t.Fatalf("Seal() returned unsafe envelope: %q", envelope)
	}
	opened, err := protector.Open(context.Background(), organizationID, receipt, envelope)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	openedInput, ok := opened.Request.Input.(map[string]any)
	if !ok || openedInput["query"] != secret || openedInput["count"] != float64(2) {
		t.Fatalf("opened input = %#v", opened.Request.Input)
	}
	if opened.RouteReceipt != frozen.RouteReceipt || opened.Origin != frozen.Origin ||
		opened.Request.OrganizationID != frozen.Request.OrganizationID ||
		opened.Request.OwnerAccountID != frozen.Request.OwnerAccountID ||
		opened.Request.BindingID != frozen.Request.BindingID || opened.Request.ConnectionID != frozen.Request.ConnectionID ||
		opened.Request.Target != frozen.Request.Target || opened.Request.Confirmation != nil {
		t.Fatalf("opened frozen request = %#v", opened)
	}
	if opened.AcceptedOperation.ConnectorID != acceptedOperation.ConnectorID ||
		opened.AcceptedOperation.PluginVersion != acceptedOperation.PluginVersion ||
		opened.AcceptedOperation.PluginDigest != acceptedOperation.PluginDigest ||
		opened.AcceptedOperation.ContractDigest != acceptedOperation.ContractDigest ||
		opened.AcceptedOperation.Definition.PluginDigest != acceptedOperation.Definition.PluginDigest ||
		opened.AcceptedOperation.Operation.ID != acceptedOperation.Operation.ID {
		t.Fatalf("opened accepted operation = %#v", opened.AcceptedOperation)
	}
	for _, tampered := range []struct {
		organizationID uuid.UUID
		receipt        connectorexecution.RouteClaimReceipt
	}{
		{organizationID: uuid.New(), receipt: receipt},
		{organizationID: organizationID, receipt: connectorexecution.RouteClaimReceipt(uuid.New().String())},
	} {
		if _, err = protector.Open(context.Background(), tampered.organizationID, tampered.receipt, envelope); err == nil ||
			strings.Contains(err.Error(), secret) {
			t.Fatalf("tampered Open() error = %v", err)
		}
	}
}

func TestConnectorConfirmationServiceGraphIsStableAndFailsClosedWithoutDatabase(t *testing.T) {
	container := &ServiceContainer{db: &gorm.DB{}, config: connectorOnlyTestConfig()}
	routerFixture := &connectorConfirmationRouterFixture{}
	router, err := connectorexecution.NewNativeRouter(routerFixture, routerFixture)
	if err != nil {
		t.Fatalf("initialize confirmation Router fixture: %v", err)
	}
	container.connectorExecutionRouter = router
	firstProtector := container.GetConnectorConfirmationPayloadProtector()
	if firstProtector == nil || firstProtector != container.GetConnectorConfirmationPayloadProtector() {
		t.Fatal("confirmation payload protector getter is not stable")
	}
	firstStore := container.GetConnectorConfirmationContinuationStore()
	if firstStore == nil || firstStore != container.GetConnectorConfirmationContinuationStore() {
		t.Fatal("confirmation continuation store getter is not stable")
	}
	firstGate := container.GetConnectorRuntimeConfirmationService()
	if firstGate == nil || firstGate != container.GetConnectorRuntimeConfirmationService() {
		t.Fatal("runtime confirmation service getter is not stable")
	}
	if coreService := container.getConnectorRuntimeConfirmationCore(); coreService == nil ||
		coreService != container.getConnectorRuntimeConfirmationCore() {
		t.Fatal("runtime confirmation core service is not stable")
	}
	if _, err := firstGate.Continue(context.Background(), connectorruntimeconfirmation.Command{}); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
		t.Fatalf("configured confirmation boundary did not reach command validation: %v", err)
	}

	container.db = nil
	_, err = firstGate.Continue(context.Background(), connectorruntimeconfirmation.Command{})
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConnectorUnavailable) {
		t.Fatalf("retained confirmation gate error = %v, want unavailable", err)
	}
	if container.GetConnectorConfirmationContinuationStore() != nil || container.getConnectorRuntimeConfirmationCore() != nil {
		t.Fatal("missing database exposed cached confirmation persistence or runtime")
	}
	if container.GetConnectorConfirmationPayloadProtector() != firstProtector {
		t.Fatal("purpose-separated payload protection incorrectly depends on a database")
	}
}

// connectorConfirmationRouterFixture keeps the graph-stability test focused on
// confirmation construction instead of initializing unrelated provider and LLM
// dependencies through the production execution Router.
type connectorConfirmationRouterFixture struct{}

func (*connectorConfirmationRouterFixture) InvokeConnector(
	context.Context,
	connectorexecution.PinnedExecution,
) (connectorexecution.ExecutionResult, error) {
	return connectorexecution.ExecutionResult{}, nil
}

func (*connectorConfirmationRouterFixture) InvokeLegacy(
	context.Context,
	connectorexecution.PinnedExecution,
) (connectorexecution.ExecutionResult, error) {
	return connectorexecution.ExecutionResult{}, nil
}

func TestConnectorCredentialGraphUsesOnlyConnectorKeyring(t *testing.T) {
	container := &ServiceContainer{config: &config.Config{
		Connectors: config.ConnectorsConfig{
			CredentialActiveKeyID: "connector-primary",
			CredentialKeys: map[string]string{
				"connector-primary": strings.Repeat("n", 32),
			},
		},
	}}

	if container.GetConnectorConnectionService() == nil {
		t.Fatal("Connector connection service did not initialize from its own keyring")
	}
}

func TestConnectorToolDiscoveryIndependentOfLegacyExternalIntegrations(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	manager := tools.NewToolManager(nil)

	if err := container.registerConnectorToolProviders(manager); err != nil {
		t.Fatal(err)
	}
	for _, providerID := range []string{"github", "web-search"} {
		provider, err := manager.GetProvider(
			context.Background(),
			tools.ToolProviderTypeConnector,
			providerID,
			"unused-tenant",
		)
		if err != nil {
			t.Fatalf("discover Connector provider %s: %v", providerID, err)
		}
		entity := provider.GetEntity()
		if entity.ProviderType != tools.ToolProviderTypeConnector || len(entity.Tools) == 0 {
			t.Fatalf("Connector provider %s entity = %#v", providerID, entity)
		}
		for _, toolEntity := range entity.Tools {
			if toolEntity.ConnectorOperationID == "" || len(toolEntity.InputSchema) == 0 ||
				len(toolEntity.OutputSchema) == 0 {
				t.Fatalf("Connector provider %s projected Tool = %#v", providerID, toolEntity)
			}
		}
		tool, err := provider.GetTool(entity.Tools[0].Identity.Name)
		if err != nil {
			t.Fatalf("resolve Connector-native Tool %s/%s: %v", providerID, entity.Tools[0].Identity.Name, err)
		}
		if _, err := tool.Invoke(context.Background(), "", nil, nil, nil, nil); err == nil {
			t.Fatal("catalog entry admitted execution through ToolEngine")
		}
	}
}

func TestConnectorToolRegistrationReplacesSameIDLegacyManagerSlot(t *testing.T) {
	container := &ServiceContainer{config: connectorOnlyTestConfig()}
	manager := tools.NewToolManager(nil)
	legacy := &emptyLegacyConnectorProvider{id: "github"}
	if err := manager.RegisterProvider(legacy); err != nil {
		t.Fatal(err)
	}

	if err := container.registerConnectorToolProviders(
		manager,
	); err != nil {
		t.Fatal(err)
	}
	provider, err := manager.GetProvider(
		context.Background(), tools.ToolProviderTypeConnector, "github", "unused-tenant",
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider == legacy || len(provider.GetEntity().Tools) == 0 {
		t.Fatalf("same-ID legacy manager slot was not replaced: %#v", provider)
	}
	tool, err := provider.GetTool(provider.GetEntity().Tools[0].Identity.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Invoke(context.Background(), "", nil, nil, nil, nil); err == nil {
		t.Fatal("catalog entry admitted execution through ToolEngine")
	}
}

func connectorOnlyTestConfig() *config.Config {
	return &config.Config{
		Connectors: config.ConnectorsConfig{
			CredentialActiveKeyID: "connector-primary",
			CredentialKeys: map[string]string{
				"connector-primary": strings.Repeat("c", 32),
			},
		},
	}
}

type emptyLegacyConnectorProvider struct{ id string }

func (provider *emptyLegacyConnectorProvider) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity:     tools.ToolProviderIdentity{Name: provider.id},
		ProviderType: tools.ToolProviderTypeConnector,
	}
}

func (*emptyLegacyConnectorProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (*emptyLegacyConnectorProvider) GetTool(string) (tools.Tool, error) {
	return nil, tools.ErrToolNotFound
}

func (*emptyLegacyConnectorProvider) GetTools() []tools.Tool { return nil }

func (*emptyLegacyConnectorProvider) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}

func writeContainerPluginPackage(t *testing.T, definitions ...connectorcatalog.Definition) string {
	t.Helper()
	manifest, err := connectorcatalog.BuildAgentPluginManifest(definitions)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

type containerPluginDefinitionStore struct {
	state    connectorcatalog.PluginRuntimeState
	versions []connectorcatalog.AcceptedDefinitionVersion
}

func (store *containerPluginDefinitionStore) Admit(_ context.Context, definitions []connectorcatalog.Definition) error {
	byID := make(map[string]connectorcatalog.Definition, len(store.state.Definitions)+len(definitions))
	for _, definition := range store.state.Definitions {
		byID[definition.ID] = definition
	}
	if store.state.Statuses == nil {
		store.state.Statuses = make(map[string]connectorcatalog.PluginDefinitionStatus)
	}
	for _, definition := range definitions {
		byID[definition.ID] = definition
		store.state.Statuses[definition.PluginID] = connectorcatalog.PluginDefinitionStatusActive
	}
	store.state.Definitions = store.state.Definitions[:0]
	for _, definition := range byID {
		store.state.Definitions = append(store.state.Definitions, definition)
	}
	return nil
}

func (store *containerPluginDefinitionStore) LoadRuntimeState(context.Context) (connectorcatalog.PluginRuntimeState, error) {
	return store.state, nil
}

func (store *containerPluginDefinitionStore) ListAcceptedVersions(
	context.Context,
	string,
) ([]connectorcatalog.AcceptedDefinitionVersion, error) {
	return append([]connectorcatalog.AcceptedDefinitionVersion(nil), store.versions...), nil
}

type containerPluginLifecycleStore struct {
	impact connectorcatalog.PluginImpact
}

func (store *containerPluginLifecycleStore) InspectPluginImpact(
	_ context.Context,
	pluginID string,
) (connectorcatalog.PluginImpact, error) {
	if pluginID != store.impact.PluginID {
		return connectorcatalog.PluginImpact{}, fmt.Errorf("unexpected plugin %s", pluginID)
	}
	return store.impact, nil
}

func (store *containerPluginLifecycleStore) TransitionPlugin(
	_ context.Context,
	pluginID string,
	_ connectorcatalog.PluginDefinitionStatus,
	force bool,
) (connectorcatalog.PluginImpact, error) {
	if pluginID != store.impact.PluginID {
		return connectorcatalog.PluginImpact{}, fmt.Errorf("unexpected plugin %s", pluginID)
	}
	if store.impact.HasReferences() && !force {
		return store.impact, connectorcatalog.ErrPluginLifecycleBlocked
	}
	return store.impact, nil
}

func (*connectorConfirmationRouterFixture) PrepareInvocation(context.Context, connectorexecution.ExecutionRequest) (connectorexecution.InvocationSnapshot, error) {
	return connectorexecution.InvocationSnapshot{}, nil
}
func (*connectorConfirmationRouterFixture) Claim(context.Context, connectorexecution.InvocationClaim) (connectorexecution.RouteClaimReceipt, error) {
	return connectorexecution.RouteClaimReceipt(uuid.NewString()), nil
}
func (*connectorConfirmationRouterFixture) Resume(context.Context, connectorexecution.InvocationClaim, connectorexecution.RouteClaimReceipt) error {
	return nil
}
