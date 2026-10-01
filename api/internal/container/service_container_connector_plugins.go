package container

import (
	"context"
	"fmt"
	"sort"

	connectortools "github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// GetConnectorPluginLoader returns the only production admission parser for
// built-in and filesystem Connector Plugin packages.
func (c *ServiceContainer) GetConnectorPluginLoader() *connectorcatalog.PluginLoader {
	if c.connectorPluginLoader == nil {
		loader, err := connectorcatalog.NewPluginLoader()
		if err != nil {
			panic(fmt.Sprintf("initialize Connector plugin loader: %v", err))
		}
		c.connectorPluginLoader = loader
	}
	return c.connectorPluginLoader
}

func (c *ServiceContainer) GetConnectorPluginLifecycleStore() *connectorcatalog.GormPluginLifecycleStore {
	if c.connectorPluginLifecycleStore == nil {
		c.connectorPluginLifecycleStore = connectorcatalog.NewGormPluginLifecycleStore(c.db)
	}
	return c.connectorPluginLifecycleStore
}

// GetConnectorPluginManager is the production catalog, admission, lifecycle,
// and accepted-version boundary. It deliberately has no Connection or
// credential dependency.
func (c *ServiceContainer) GetConnectorPluginManager() *connectorcatalog.PluginManager {
	if c.connectorPluginManager == nil {
		manager, err := connectorcatalog.NewPluginManager(
			c.GetConnectorPluginLoader(),
			c.GetConnectorCatalog(),
			c.GetConnectorDefinitionRepository(),
			c.GetConnectorPluginLifecycleStore(),
			connectorcatalog.WithPluginPackageAdmissionPolicy(
				connectorcatalog.ValidateBuiltinPackage,
			),
		)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector plugin manager: %v", err))
		}
		c.connectorPluginManager = manager
	}
	return c.connectorPluginManager
}

// BootstrapConnectorPlugins restores accepted versions before admitting a
// configured set of trusted filesystem packages. A rejected external package
// remains isolated in the returned report and cannot hide valid packages.
func (c *ServiceContainer) BootstrapConnectorPlugins(
	ctx context.Context,
) (connectorcatalog.PluginAdmissionReport, error) {
	ctx = connectoraudit.WithPlatformActor(ctx, connectoraudit.PlatformActor{
		Kind: connectoraudit.PlatformActorSystem,
	})
	roots := []string(nil)
	if c != nil && c.config != nil {
		roots = append(roots, c.config.Connectors.PluginDirectories...)
	}
	report, err := c.GetConnectorPluginManager().Bootstrap(ctx, roots)
	if err != nil {
		return report, err
	}
	logConnectorPluginAdmissionFailures(ctx, report)
	if c.toolManager != nil {
		if err := c.refreshConnectorToolProviders(c.toolManager); err != nil {
			return report, err
		}
	}
	return report, nil
}

// AdmitConnectorPluginDirectories is a maintainer entry point for explicitly
// trusted local package roots. It never reads or creates member Connections.
func (c *ServiceContainer) AdmitConnectorPluginDirectories(
	ctx context.Context,
	roots []string,
) (connectorcatalog.PluginAdmissionReport, error) {
	if !connectorPlatformMaintainerActor(ctx) {
		return connectorcatalog.PluginAdmissionReport{}, connectorcatalog.ErrPluginAdmissionUnavailable
	}
	report, err := c.GetConnectorPluginManager().Bootstrap(ctx, append([]string(nil), roots...))
	if err != nil {
		return report, err
	}
	logConnectorPluginAdmissionFailures(ctx, report)
	if c.toolManager != nil {
		if refreshErr := c.refreshConnectorToolProviders(c.toolManager); refreshErr != nil {
			return report, refreshErr
		}
	}
	return report, nil
}

func logConnectorPluginAdmissionFailures(ctx context.Context, report connectorcatalog.PluginAdmissionReport) {
	componentFailures := 0
	for _, plugin := range report.LoadReport.Plugins {
		componentFailures += len(plugin.Failures)
	}
	if len(report.Failures) == 0 && componentFailures == 0 {
		return
	}
	logger.WarnContext(
		ctx,
		"Connector plugin admission completed with isolated failures",
		"package_failure_count", len(report.Failures),
		"component_failure_count", componentFailures,
	)
}

func (c *ServiceContainer) InspectConnectorPluginImpact(
	ctx context.Context,
	pluginID string,
) (connectorcatalog.PluginImpact, error) {
	return c.GetConnectorPluginManager().Impact(ctx, pluginID)
}

func (c *ServiceContainer) DisableConnectorPlugin(
	ctx context.Context,
	pluginID string,
	force bool,
) (connectorcatalog.PluginImpact, error) {
	if !connectorPlatformMaintainerActor(ctx) {
		return connectorcatalog.PluginImpact{}, connectorcatalog.ErrPluginAdmissionUnavailable
	}
	impact, err := c.GetConnectorPluginManager().Disable(ctx, pluginID, force)
	if err != nil {
		return impact, err
	}
	if c.toolManager != nil {
		if err := c.refreshConnectorToolProviders(c.toolManager); err != nil {
			return impact, err
		}
	}
	return impact, nil
}

func (c *ServiceContainer) RemoveConnectorPlugin(
	ctx context.Context,
	pluginID string,
	force bool,
) (connectorcatalog.PluginImpact, error) {
	if !connectorPlatformMaintainerActor(ctx) {
		return connectorcatalog.PluginImpact{}, connectorcatalog.ErrPluginAdmissionUnavailable
	}
	impact, err := c.GetConnectorPluginManager().Remove(ctx, pluginID, force)
	if err != nil {
		return impact, err
	}
	if c.toolManager != nil {
		if err := c.refreshConnectorToolProviders(c.toolManager); err != nil {
			return impact, err
		}
	}
	return impact, nil
}

func connectorPlatformMaintainerActor(ctx context.Context) bool {
	actor, ok := connectoraudit.PlatformActorFromContext(ctx)
	return ok && actor.Kind == connectoraudit.PlatformActorPlatformMaintainer && actor.ID != ""
}

func (c *ServiceContainer) ListAcceptedConnectorPluginVersions(
	ctx context.Context,
	pluginID string,
) ([]connectorcatalog.AcceptedDefinitionVersion, error) {
	return c.GetConnectorPluginManager().AcceptedVersions(ctx, pluginID)
}

func (c *ServiceContainer) loadBuiltinConnectorPluginDefinitions(
	ctx context.Context,
) ([]connectorcatalog.Definition, error) {
	projected, err := connectorcatalog.BuiltinDefinitions()
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]connectorcatalog.Definition)
	for _, definition := range projected {
		grouped[definition.PluginID] = append(grouped[definition.PluginID], definition)
	}
	pluginIDs := make([]string, 0, len(grouped))
	for pluginID := range grouped {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)

	definitions := make([]connectorcatalog.Definition, 0, len(projected))
	for _, pluginID := range pluginIDs {
		manifest, err := connectorcatalog.BuildAgentPluginManifest(grouped[pluginID])
		if err != nil {
			return nil, fmt.Errorf("build built-in Connector plugin %s: %w", pluginID, err)
		}
		plugin, err := c.GetConnectorPluginLoader().LoadManifestPackage(ctx, manifest)
		if err != nil {
			return nil, fmt.Errorf("admit built-in Connector plugin %s: %w", pluginID, err)
		}
		if len(plugin.Failures) != 0 || len(plugin.Definitions) != len(grouped[pluginID]) {
			return nil, fmt.Errorf("admit built-in Connector plugin %s: %w", pluginID, connectorcatalog.ErrConnectorMetadataInvalid)
		}
		definitions = append(definitions, plugin.Definitions...)
	}
	return definitions, nil
}

func (c *ServiceContainer) refreshConnectorToolProviders(
	manager *tools.ToolManager,
) error {
	if c == nil || c.config == nil {
		return nil
	}
	if manager == nil {
		return fmt.Errorf("register Connector-native Tool providers: ToolManager is unavailable")
	}
	c.connectorToolProviderMu.Lock()
	defer c.connectorToolProviderMu.Unlock()
	if c.connectorRegisteredProviders == nil {
		c.connectorRegisteredProviders = make(map[string]struct{})
	}

	definitions := c.GetConnectorPluginManager().List(connectorcatalog.Filter{})
	active := make(map[string]connectorcatalog.Definition, len(definitions))
	for _, definition := range definitions {
		active[definition.ID] = definition
	}
	for providerID := range c.connectorRegisteredProviders {
		if _, remainsActive := active[providerID]; remainsActive {
			continue
		}
		if err := manager.UnregisterProvider(tools.ToolProviderTypeConnector, providerID); err != nil {
			return fmt.Errorf("unregister inactive Connector-native Tool provider %s: %w", providerID, err)
		}
		delete(c.connectorRegisteredProviders, providerID)
	}

	for _, definition := range definitions {
		provider, err := connectortools.NewProvider(definition)
		if err != nil {
			return fmt.Errorf("initialize Connector-native Tool provider %s: %w", definition.ID, err)
		}
		if err := manager.RegisterProvider(provider); err != nil {
			return fmt.Errorf("register Connector-native Tool provider %s: %w", definition.ID, err)
		}
		c.connectorRegisteredProviders[definition.ID] = struct{}{}
	}
	return nil
}
