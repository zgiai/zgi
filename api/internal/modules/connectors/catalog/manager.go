package catalog

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type PluginDefinitionStore interface {
	DefinitionAdmitter
	LoadRuntimeState(context.Context) (PluginRuntimeState, error)
	ListAcceptedVersions(context.Context, string) ([]AcceptedDefinitionVersion, error)
}

// PluginPackageAdmissionPolicy lets the production composition root reject a
// standards-conforming package whose runtime driver is not installed. The
// catalog package itself remains driver-agnostic.
type PluginPackageAdmissionPolicy func([]Definition) error

type PluginManagerOption func(*PluginManager) error

func WithPluginPackageAdmissionPolicy(policy PluginPackageAdmissionPolicy) PluginManagerOption {
	return func(manager *PluginManager) error {
		if policy == nil {
			return ErrPluginAdmissionUnavailable
		}
		manager.packagePolicy = policy
		return nil
	}
}

// PluginManager is the production admission and lifecycle boundary. Plugin
// discovery never receives a Connection repository or credential capability;
// it can only persist secret-free definitions and transition their availability.
type PluginManager struct {
	mu             sync.Mutex
	loader         *PluginLoader
	definitions    PluginDefinitionStore
	lifecycleStore PluginLifecycleStore
	catalog        *Catalog
	lifecycle      *PluginLifecycle
	packagePolicy  PluginPackageAdmissionPolicy
	bootstrapped   bool
}

func NewPluginManager(
	loader *PluginLoader,
	catalog *Catalog,
	definitions PluginDefinitionStore,
	lifecycleStore PluginLifecycleStore,
	options ...PluginManagerOption,
) (*PluginManager, error) {
	if loader == nil || catalog == nil || definitions == nil {
		return nil, ErrPluginAdmissionUnavailable
	}
	initial := catalog.List(Filter{})
	lifecycle, err := NewPluginLifecycle(initial, lifecycleStore)
	if err != nil {
		return nil, err
	}
	manager := &PluginManager{
		loader: loader, definitions: definitions, lifecycleStore: lifecycleStore,
		catalog: catalog, lifecycle: lifecycle,
	}
	for _, option := range options {
		if option == nil {
			return nil, ErrPluginAdmissionUnavailable
		}
		if err := option(manager); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

// Bootstrap restores authoritative state first, then admits new or versioned
// built-in packages. A same-version package already in storage remains the
// authority, and disabled/unavailable packages are never silently reactivated
// by process restart.
func (manager *PluginManager) Bootstrap(ctx context.Context, pluginRoots []string) (PluginAdmissionReport, error) {
	if manager == nil {
		return PluginAdmissionReport{}, ErrPluginAdmissionUnavailable
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.bootstrapped {
		return manager.admitDirectoriesLocked(ctx, pluginRoots), nil
	}
	initial := manager.catalog.List(Filter{})
	state, err := manager.definitions.LoadRuntimeState(ctx)
	if err != nil {
		return PluginAdmissionReport{}, err
	}
	persistedByPlugin := make(map[string][]Definition, len(state.Statuses))
	for _, definition := range state.Definitions {
		persistedByPlugin[definition.PluginID] = append(persistedByPlugin[definition.PluginID], cloneDefinition(definition))
	}
	if len(state.Definitions) > 0 {
		lifecycle, lifecycleErr := NewPluginLifecycleWithStatuses(state.Definitions, state.Statuses, manager.lifecycleStore)
		if lifecycleErr != nil {
			return PluginAdmissionReport{}, lifecycleErr
		}
		if err := manager.catalog.ReplaceActive(state.ActiveDefinitions()); err != nil {
			return PluginAdmissionReport{}, err
		}
		manager.lifecycle = lifecycle
	} else if err := manager.catalog.ReplaceActive(nil); err != nil {
		return PluginAdmissionReport{}, err
	} else {
		manager.lifecycle, err = NewPluginLifecycle(nil, manager.lifecycleStore)
		if err != nil {
			return PluginAdmissionReport{}, err
		}
	}

	for _, definitions := range definitionsByPlugin(initial) {
		pluginID := definitions[0].PluginID
		if status, exists := state.Statuses[pluginID]; exists {
			if status != PluginDefinitionStatusActive {
				continue
			}
			persisted := persistedByPlugin[pluginID]
			if samePluginPackageIdentity(persisted, definitions) ||
				(len(persisted) > 0 && persisted[0].PluginVersion == definitions[0].PluginVersion) {
				continue
			}
		}
		if err := manager.activateLocked(ctx, definitions); err != nil {
			return PluginAdmissionReport{}, fmt.Errorf("admit built-in Connector plugin %s: %w", pluginID, err)
		}
	}
	manager.bootstrapped = true
	return manager.admitDirectoriesLocked(ctx, pluginRoots), nil
}

func samePluginPackageIdentity(left, right []Definition) bool {
	if len(left) != len(right) || len(left) == 0 {
		return false
	}
	byID := make(map[string]Definition, len(left))
	for _, definition := range left {
		byID[definition.ID] = definition
	}
	for _, definition := range right {
		current, exists := byID[definition.ID]
		if !exists || current.PluginID != definition.PluginID || current.PluginVersion != definition.PluginVersion ||
			current.PluginDigest != definition.PluginDigest {
			return false
		}
	}
	return true
}

func (manager *PluginManager) AdmitDirectories(ctx context.Context, pluginRoots []string) PluginAdmissionReport {
	if manager == nil {
		return PluginAdmissionReport{Failures: []PluginPackageFailure{{Err: ErrPluginAdmissionUnavailable}}}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.admitDirectoriesLocked(ctx, pluginRoots)
}

func (manager *PluginManager) admitDirectoriesLocked(ctx context.Context, pluginRoots []string) PluginAdmissionReport {
	loadReport := manager.loader.LoadDirectories(ctx, pluginRoots)
	report := PluginAdmissionReport{LoadReport: loadReport, Failures: append([]PluginPackageFailure(nil), loadReport.Failures...)}
	for _, plugin := range loadReport.Plugins {
		if len(plugin.Definitions) == 0 {
			continue
		}
		if err := manager.activateLocked(ctx, plugin.Definitions); err != nil {
			report.Failures = append(report.Failures, PluginPackageFailure{Err: err})
			continue
		}
		for _, definition := range plugin.Definitions {
			report.AdmittedDefinitions = append(report.AdmittedDefinitions, cloneDefinition(definition))
		}
	}
	return report
}

func (manager *PluginManager) activateLocked(ctx context.Context, definitions []Definition) error {
	if manager.packagePolicy != nil {
		if err := manager.packagePolicy(definitions); err != nil {
			return err
		}
	}
	if err := manager.lifecycle.ValidatePackageActivation(definitions); err != nil {
		return err
	}
	if err := manager.definitions.Admit(ctx, definitions); err != nil {
		return err
	}
	if err := manager.lifecycle.ActivatePackage(definitions); err != nil {
		return fmt.Errorf("activate persisted Connector plugin in runtime: %w", err)
	}
	return manager.catalog.ActivatePackage(definitions)
}

func (manager *PluginManager) Pin(connectorID, operationID string) (PinnedOperation, error) {
	if manager == nil {
		return PinnedOperation{}, ErrPluginDefinitionNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.lifecycle.Pin(connectorID, operationID)
}

// Get and List expose the same member-facing active view as Catalog while
// serializing reads with plugin activation and lifecycle transitions. Runtime
// services should depend on the manager rather than bypassing this boundary so
// they observe either the complete old package or the complete new package.
func (manager *PluginManager) Get(connectorID string) (Definition, bool) {
	if manager == nil {
		return Definition{}, false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.catalog.Get(connectorID)
}

func (manager *PluginManager) List(filter Filter) []Definition {
	if manager == nil {
		return []Definition{}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.catalog.List(filter)
}

func (manager *PluginManager) Impact(ctx context.Context, pluginID string) (PluginImpact, error) {
	if manager == nil {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.lifecycle.Impact(ctx, pluginID)
}

func (manager *PluginManager) Disable(ctx context.Context, pluginID string, force bool) (PluginImpact, error) {
	return manager.transition(ctx, pluginID, PluginDefinitionStatusDisabled, force)
}

func (manager *PluginManager) Remove(ctx context.Context, pluginID string, force bool) (PluginImpact, error) {
	return manager.transition(ctx, pluginID, PluginDefinitionStatusUnavailable, force)
}

func (manager *PluginManager) transition(
	ctx context.Context,
	pluginID string,
	status PluginDefinitionStatus,
	force bool,
) (PluginImpact, error) {
	if manager == nil {
		return PluginImpact{}, ErrPluginDefinitionNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var (
		impact PluginImpact
		err    error
	)
	if status == PluginDefinitionStatusDisabled {
		impact, err = manager.lifecycle.Disable(ctx, pluginID, force)
	} else {
		impact, err = manager.lifecycle.Remove(ctx, pluginID, force)
	}
	if err == nil {
		manager.catalog.DeactivatePlugin(pluginID)
	}
	return impact, err
}

func (manager *PluginManager) AcceptedVersions(
	ctx context.Context,
	pluginID string,
) ([]AcceptedDefinitionVersion, error) {
	if manager == nil {
		return nil, ErrPluginDefinitionNotFound
	}
	return manager.definitions.ListAcceptedVersions(ctx, pluginID)
}

func definitionsByPlugin(definitions []Definition) [][]Definition {
	grouped := make(map[string][]Definition)
	for _, definition := range definitions {
		grouped[definition.PluginID] = append(grouped[definition.PluginID], cloneDefinition(definition))
	}
	pluginIDs := make([]string, 0, len(grouped))
	for pluginID := range grouped {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)
	result := make([][]Definition, 0, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		sort.Slice(grouped[pluginID], func(i, j int) bool { return grouped[pluginID][i].ID < grouped[pluginID][j].ID })
		result = append(result, grouped[pluginID])
	}
	return result
}
