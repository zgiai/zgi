package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPluginManagerBootstrapsPersistsCompatibleActivationAndPinsInFlightVersion(t *testing.T) {
	definition := pluginContractDefinitionFixture()
	catalog, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	definitionStore := &memoryPluginDefinitionStore{state: PluginRuntimeState{Statuses: map[string]PluginDefinitionStatus{}}}
	lifecycleStore := &pluginLifecycleStoreSpy{impact: PluginImpact{
		PluginID:    definition.PluginID,
		Definitions: []DefinitionImpact{{ConnectorID: definition.ID, Status: PluginDefinitionStatusActive}},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, definitionStore, lifecycleStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if definitionStore.admitCalls != 1 {
		t.Fatalf("built-in plugin admissions = %d, want 1", definitionStore.admitCalls)
	}
	oldPin, err := manager.Pin(definition.ID, definition.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	updated := cloneDefinition(definition)
	updated.PluginVersion = "1.1.0"
	updated.PluginDigest = strings.Repeat("b", 64)
	updated.Name = "Updated display"
	updated.Operations = append(updated.Operations, additiveOperationFixture())
	root := writeDefinitionPluginPackage(t, []Definition{updated})
	report := manager.AdmitDirectories(context.Background(), []string{root})
	if len(report.Failures) != 0 || len(report.AdmittedDefinitions) != 1 || definitionStore.admitCalls != 2 {
		t.Fatalf("compatible admission = %#v, calls=%d", report, definitionStore.admitCalls)
	}
	active, found := catalog.Get(definition.ID)
	if !found || active.PluginVersion != "1.1.0" || active.Name != "Updated display" {
		t.Fatalf("active catalog definition = %#v, found=%v", active, found)
	}
	newPin, err := manager.Pin(definition.ID, definition.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if oldPin.PluginVersion != "1.0.0" || newPin.PluginVersion != "1.1.0" ||
		oldPin.PluginDigest == newPin.PluginDigest {
		t.Fatalf("old/new pins = %#v / %#v", oldPin, newPin)
	}
	if oldPin.Operation.ID != definition.Operations[0].ID {
		t.Fatalf("in-flight pin changed after activation: %#v", oldPin)
	}
}

func TestPluginManagerRejectsIncompatibleUpdateBeforePersistence(t *testing.T) {
	definition := pluginContractDefinitionFixture()
	catalog, _ := New(definition)
	definitionStore := &memoryPluginDefinitionStore{state: PluginRuntimeState{
		Definitions: []Definition{definition},
		Statuses:    map[string]PluginDefinitionStatus{definition.PluginID: PluginDefinitionStatusActive},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, definitionStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	baselineCalls := definitionStore.admitCalls
	incompatible := cloneDefinition(definition)
	incompatible.PluginVersion = "2.0.0"
	incompatible.PluginDigest = strings.Repeat("c", 64)
	incompatible.Operations[0].RiskHint = OperationRiskHintHigh
	report := manager.AdmitDirectories(context.Background(), []string{writeDefinitionPluginPackage(t, []Definition{incompatible})})
	if len(report.Failures) != 1 || definitionStore.admitCalls != baselineCalls {
		t.Fatalf("incompatible admission = %#v, calls=%d", report, definitionStore.admitCalls)
	}
	active, _ := catalog.Get(definition.ID)
	if active.PluginVersion != definition.PluginVersion || active.Operations[0].RiskHint != definition.Operations[0].RiskHint {
		t.Fatalf("incompatible package replaced active contract: %#v", active)
	}
}

func TestPluginManagerBootstrapActivatesCompatibleNewBuiltinVersion(t *testing.T) {
	current := pluginContractDefinitionFixture()
	updated := cloneDefinition(current)
	current.NameI18n, current.DescriptionI18n = nil, nil
	for index := range current.KeyFields {
		current.KeyFields[index].LabelI18n, current.KeyFields[index].PlaceholderI18n = nil, nil
	}
	for index := range current.Operations {
		current.Operations[index].NameI18n, current.Operations[index].DescriptionI18n = nil, nil
	}
	updated.PluginVersion = "1.1.0"
	updated.PluginDigest = strings.Repeat("b", 64)
	updated.Name = "Updated built-in"
	updated.Operations = append(updated.Operations, additiveOperationFixture())
	catalog, err := New(updated)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPluginDefinitionStore{state: PluginRuntimeState{
		Definitions: []Definition{current},
		Statuses:    map[string]PluginDefinitionStatus{current.PluginID: PluginDefinitionStatusActive},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	active, found := manager.Get(current.ID)
	if !found || active.PluginVersion != updated.PluginVersion || active.Name != updated.Name || store.admitCalls != 1 ||
		active.NameI18n["zh-Hans"] != updated.NameI18n["zh-Hans"] || active.Operations[0].NameI18n["zh-Hans"] != updated.Operations[0].NameI18n["zh-Hans"] {
		t.Fatalf("active built-in = %#v, found=%v, admits=%d", active, found, store.admitCalls)
	}
}

func TestPluginManagerBootstrapRetainsIncompatibleAcceptedExtension(t *testing.T) {
	builtin := pluginContractDefinitionFixture()
	current := cloneDefinition(builtin)
	current.PluginVersion += "-digitalhuman.1"
	current.ExtensionSchemaVersion, current.ContractVersion = ZGIMetadataSchemaVersion13, 4
	current.PluginDigest = strings.Repeat("b", 64)
	current.Surfaces = append(current.Surfaces, SurfaceDigitalHuman)
	catalog, err := New(builtin)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPluginDefinitionStore{state: PluginRuntimeState{
		Definitions: []Definition{current},
		Statuses:    map[string]PluginDefinitionStatus{current.PluginID: PluginDefinitionStatusActive},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Bootstrap(context.Background(), nil)
	if err != nil || len(report.Failures) != 1 || !errors.Is(report.Failures[0].Err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("Bootstrap() = %#v, %v", report, err)
	}
	active, found := manager.Get(current.ID)
	if !found || active.PluginDigest != current.PluginDigest || !active.Supports(SurfaceDigitalHuman) || store.admitCalls != 0 {
		t.Fatalf("accepted package replaced: %#v, admits=%d", active, store.admitCalls)
	}
	pin, err := manager.Pin(current.ID, current.Operations[0].ID)
	if err != nil || pin.PluginVersion != current.PluginVersion {
		t.Fatalf("accepted operation pin = %#v, %v", pin, err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil || store.admitCalls != 0 {
		t.Fatalf("repeated Bootstrap() = %v, admits=%d", err, store.admitCalls)
	}
}

func TestPluginManagerBootstrapDoesNotReactivateInactiveBuiltin(t *testing.T) {
	definition := pluginContractDefinitionFixture()
	catalog, err := New(definition)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPluginDefinitionStore{state: PluginRuntimeState{
		Definitions: []Definition{definition},
		Statuses:    map[string]PluginDefinitionStatus{definition.PluginID: PluginDefinitionStatusDisabled},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if _, found := manager.Get(definition.ID); found || store.admitCalls != 0 {
		t.Fatalf("disabled built-in was reactivated, found=%v, admits=%d", found, store.admitCalls)
	}
	if _, err := manager.Pin(definition.ID, definition.Operations[0].ID); !errors.Is(err, ErrPluginDefinitionInactive) {
		t.Fatalf("Pin(disabled after restart) error = %v", err)
	}
}

func TestPluginManagerForcedDisableHidesCatalogButPreservesPinnedContractAndImpact(t *testing.T) {
	definition := pluginContractDefinitionFixture()
	catalog, _ := New(definition)
	connectionID := uuid.New()
	store := &pluginLifecycleStoreSpy{impact: PluginImpact{
		PluginID:    definition.PluginID,
		Definitions: []DefinitionImpact{{ConnectorID: definition.ID, Status: PluginDefinitionStatusActive}},
		Connections: []ConnectionImpact{{ID: connectionID, ConnectorID: definition.ID, State: "ready"}},
		Bindings:    []BindingImpact{{ID: uuid.New(), ConnectionID: connectionID, ConnectorID: definition.ID, Status: "active"}},
	}}
	definitions := &memoryPluginDefinitionStore{state: PluginRuntimeState{
		Definitions: []Definition{definition},
		Statuses:    map[string]PluginDefinitionStatus{definition.PluginID: PluginDefinitionStatusActive},
	}}
	manager, err := NewPluginManager(newPluginLoaderForTest(t), catalog, definitions, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Bootstrap(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	pinned, err := manager.Pin(definition.ID, definition.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	impact, err := manager.Disable(context.Background(), definition.PluginID, false)
	if err == nil || !impact.HasReferences() {
		t.Fatalf("default disable = %#v, %v", impact, err)
	}
	if _, found := catalog.Get(definition.ID); !found {
		t.Fatal("blocked disable hid active catalog item")
	}
	impact, err = manager.Disable(context.Background(), definition.PluginID, true)
	if err != nil || !impact.HasReferences() {
		t.Fatalf("forced disable = %#v, %v", impact, err)
	}
	if _, found := catalog.Get(definition.ID); found {
		t.Fatal("forced-disabled plugin remained usable")
	}
	if pinned.PluginVersion != definition.PluginVersion || pinned.Operation.ID != definition.Operations[0].ID {
		t.Fatalf("held in-flight pin changed: %#v", pinned)
	}
}

func writeDefinitionPluginPackage(t *testing.T, definitions []Definition) string {
	t.Helper()
	manifest, err := BuildAgentPluginManifest(definitions)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

type memoryPluginDefinitionStore struct {
	state      PluginRuntimeState
	admitCalls int
	admitted   [][]Definition
	versions   []AcceptedDefinitionVersion
}

func (store *memoryPluginDefinitionStore) Admit(_ context.Context, definitions []Definition) error {
	store.admitCalls++
	cloned := make([]Definition, len(definitions))
	for index, definition := range definitions {
		cloned[index] = cloneDefinition(definition)
	}
	store.admitted = append(store.admitted, cloned)
	byID := make(map[string]Definition, len(store.state.Definitions)+len(cloned))
	for _, definition := range store.state.Definitions {
		byID[definition.ID] = cloneDefinition(definition)
	}
	for _, definition := range cloned {
		byID[definition.ID] = cloneDefinition(definition)
		if store.state.Statuses == nil {
			store.state.Statuses = make(map[string]PluginDefinitionStatus)
		}
		store.state.Statuses[definition.PluginID] = PluginDefinitionStatusActive
	}
	store.state.Definitions = store.state.Definitions[:0]
	for _, definition := range byID {
		store.state.Definitions = append(store.state.Definitions, definition)
	}
	return nil
}

func (store *memoryPluginDefinitionStore) LoadRuntimeState(context.Context) (PluginRuntimeState, error) {
	state := PluginRuntimeState{Statuses: make(map[string]PluginDefinitionStatus, len(store.state.Statuses))}
	for pluginID, status := range store.state.Statuses {
		state.Statuses[pluginID] = status
	}
	for _, definition := range store.state.Definitions {
		state.Definitions = append(state.Definitions, cloneDefinition(definition))
	}
	return state, nil
}

func (store *memoryPluginDefinitionStore) ListAcceptedVersions(context.Context, string) ([]AcceptedDefinitionVersion, error) {
	return append([]AcceptedDefinitionVersion(nil), store.versions...), nil
}
