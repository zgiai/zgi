package container

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

// Exercise the actual composition root, including any host-specific wrapping,
// rather than testing only the raw FeishuDefinition returned by the catalog.
func TestConnectorBootstrapUpgradesFixedFeishuTemplateFromHistoricalDefinition(t *testing.T) {
	for _, status := range []catalog.PluginDefinitionStatus{catalog.PluginDefinitionStatusActive, catalog.PluginDefinitionStatusDisabled} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			c := &ServiceContainer{}
			definitions := c.GetConnectorCatalog()
			target, found := definitions.Get("feishu")
			if !found || target.PluginDigest == "" {
				t.Fatal("production assembly did not admit the Feishu package")
			}
			old := historicalFeishuTemplateDefinition(t, c)
			originalDigest, err := catalog.DefinitionContractDigest(old)
			if err != nil || originalDigest != "8313044f4f770570c1e567ad24af7f30b7b497ae6fa8b19b2eacf3d8a405c430" {
				t.Fatalf("historical contract drift: %s %v", originalDigest, err)
			}
			state := catalog.PluginRuntimeState{
				Definitions: definitions.List(catalog.Filter{}),
				Statuses:    map[string]catalog.PluginDefinitionStatus{},
			}
			for i, definition := range state.Definitions {
				state.Statuses[definition.PluginID] = catalog.PluginDefinitionStatusActive
				if definition.ID == old.ID {
					state.Definitions[i] = old
					state.Statuses[definition.PluginID] = status
				}
			}
			history := catalog.AcceptedDefinitionVersion{Definition: old, ContractDigest: originalDigest, Status: status}
			historyJSON, err := json.Marshal([]catalog.AcceptedDefinitionVersion{history})
			if err != nil {
				t.Fatal(err)
			}
			store := &feishuBootstrapStore{containerPluginDefinitionStore: containerPluginDefinitionStore{
				state: state, versions: []catalog.AcceptedDefinitionVersion{history},
			}}
			assertHistoryUnchanged := func() {
				t.Helper()
				afterJSON, err := json.Marshal(store.versions)
				if err != nil || string(afterJSON) != string(historyJSON) {
					t.Fatal("historical accepted definition or evidence changed")
				}
				afterDigest, err := catalog.DefinitionContractDigest(store.versions[0].Definition)
				if err != nil || afterDigest != originalDigest {
					t.Fatal("historical contract changed in place")
				}
			}
			manager, err := catalog.NewPluginManager(c.GetConnectorPluginLoader(), definitions, store, nil,
				catalog.WithPluginPackageAdmissionPolicy(catalog.ValidateBuiltinPackage))
			if err != nil {
				t.Fatal(err)
			}
			c.connectorPluginManager = manager
			if _, err := c.BootstrapConnectorPlugins(ctx); err != nil {
				t.Fatalf("historical Feishu -> production %s failed: %v", target.PluginVersion, err)
			}
			expectedWrites := 1
			if status != catalog.PluginDefinitionStatusActive {
				expectedWrites = 0
			}
			if store.admitCalls != expectedWrites {
				t.Fatalf("admit calls = %d, want %d", store.admitCalls, expectedWrites)
			}
			active, found := manager.Get(old.ID)
			if status == catalog.PluginDefinitionStatusActive {
				if !found || !reflect.DeepEqual(active, target) {
					t.Fatal("bootstrap did not activate the exact production package")
				}
			} else if found {
				t.Fatal("bootstrap reactivated a disabled Feishu connector")
			}
			assertHistoryUnchanged()
			if c.db != nil || c.connectorConnectionRepo != nil || c.connectorBindingRepo != nil {
				t.Fatal("metadata bootstrap initialized live storage or user connection/binding repositories")
			}

			// A fresh process repeats production assembly, then restores the
			// persisted catalog without another admission or a disabled reset.
			restarted := &ServiceContainer{}
			restartedManager, err := catalog.NewPluginManager(restarted.GetConnectorPluginLoader(), restarted.GetConnectorCatalog(), store, nil,
				catalog.WithPluginPackageAdmissionPolicy(catalog.ValidateBuiltinPackage))
			if err != nil {
				t.Fatal(err)
			}
			restarted.connectorPluginManager = restartedManager
			if _, err := restarted.BootstrapConnectorPlugins(ctx); err != nil {
				t.Fatal(err)
			}
			if store.admitCalls != expectedWrites {
				t.Fatal("restart repeated the template migration")
			}
			after, found := restartedManager.Get(old.ID)
			if found != (status == catalog.PluginDefinitionStatusActive) || (found && !reflect.DeepEqual(after, target)) {
				t.Fatal("restart changed activation or accepted content")
			}
			assertHistoryUnchanged()
		})
	}
}

func historicalFeishuTemplateDefinition(t *testing.T, c *ServiceContainer) catalog.Definition {
	t.Helper()
	old, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	old.PluginVersion = "1.0.2"
	old.Operations = slices.DeleteFunc(old.Operations, func(op catalog.Operation) bool {
		return op.ID == "feishu.contact.search" || op.ID == "feishu.tenant.get" || op.ID == "feishu.chat.members.list"
	})
	old.Setup.TemplateID = "feishu-app-template-v2"
	old.Setup.TemplateDigest = "4930f0a57d31ed17ed4d86384fd05219cfce7c5b4f20bbfcadec2329307634cc"
	manifest, err := catalog.BuildAgentPluginManifest([]catalog.Definition{old})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := c.GetConnectorPluginLoader().LoadManifestPackage(context.Background(), manifest)
	if err != nil || len(loaded.Failures) != 0 || len(loaded.Definitions) != 1 {
		t.Fatalf("load historical package: %v", err)
	}
	return loaded.Definitions[0]
}

type feishuBootstrapStore struct {
	containerPluginDefinitionStore
	admitCalls int
}

func (store *feishuBootstrapStore) Admit(ctx context.Context, definitions []catalog.Definition) error {
	store.admitCalls++
	return store.containerPluginDefinitionStore.Admit(ctx, definitions)
}
