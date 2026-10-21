package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginLoaderAdmitsAgentPluginsV1ZGIConnectorMetadata(t *testing.T) {
	t.Parallel()

	loader := newPluginLoaderForTest(t)
	root := writePluginManifest(t, map[string]interface{}{
		"$schema":              AgentPluginsV1ManifestSchemaID,
		"name":                 "acme.github",
		"version":              "2.4.1",
		"ignored_client_field": map[string]interface{}{"candidate_key": "not-interpreted"},
		"extensions": map[string]interface{}{
			ZGIConnectorExtensionNamespace: map[string]interface{}{
				"schema_version": ZGIMetadataSchemaVersion1,
				"connectors":     []interface{}{connectorMetadataFixture("github", ClassificationConnector)},
			},
			// Unknown namespaces are opaque even when their value is not an
			// object. ZGI must not assign semantics or fail this plugin.
			"com.example.unknown": "opaque-client-value",
		},
	})

	plugin, err := loader.LoadDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	if plugin.PluginID != "acme.github" || plugin.PluginVersion != "2.4.1" ||
		!sha256HexPattern.MatchString(plugin.PluginDigest) {
		t.Fatalf("loaded plugin identity = %#v", plugin)
	}
	if len(plugin.Warnings) != 1 || plugin.Warnings[0].Code != "unknown_top_level_field" ||
		plugin.Warnings[0].Field != "ignored_client_field" {
		t.Fatalf("manifest warnings = %#v", plugin.Warnings)
	}
	if len(plugin.Failures) != 0 || len(plugin.Definitions) != 1 {
		t.Fatalf("loaded definitions/failures = %d/%#v", len(plugin.Definitions), plugin.Failures)
	}
	definition := plugin.Definitions[0]
	if definition.ID != "github" || definition.PluginID != plugin.PluginID ||
		definition.PluginDigest != plugin.PluginDigest || definition.SourceDialect != SourceDialectAgentPluginsV1 ||
		definition.SourceSchemaVersion != SourceSchemaVersion1 || definition.Classification != ClassificationConnector {
		t.Fatalf("definition identity = %#v", definition)
	}
	if len(definition.Operations) != 1 || definition.Operations[0].Effect != OperationEffectRead ||
		definition.Operations[0].RiskHint != OperationRiskHintLow ||
		definition.Operations[0].InputSchema == nil || definition.Operations[0].OutputSchema == nil {
		t.Fatalf("operation contract = %#v", definition.Operations)
	}
}

func TestPluginLoaderAdmitsDigitalHumanConnectorMetadata(t *testing.T) {
	t.Parallel()

	loader := newPluginLoaderForTest(t)
	metadata := connectorMetadataFixture("digital-human", ClassificationConnector)
	metadata["surfaces"] = []interface{}{"chat", "agent", "workflow", "digital_human"}
	root := writePluginManifest(t, map[string]interface{}{
		"$schema": AgentPluginsV1ManifestSchemaID,
		"name":    "acme.digital-human",
		"version": "2.0.0-digitalhuman.1",
		"extensions": map[string]interface{}{
			ZGIConnectorExtensionNamespace: map[string]interface{}{
				"schema_version": ZGIMetadataSchemaVersion13,
				"connectors":     []interface{}{metadata},
			},
		},
	})

	plugin, err := loader.LoadDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("LoadDirectory() error = %v", err)
	}
	if len(plugin.Failures) != 0 || len(plugin.Definitions) != 1 {
		t.Fatalf("loaded definitions/failures = %d/%#v", len(plugin.Definitions), plugin.Failures)
	}
	definition := plugin.Definitions[0]
	if !definition.IsDigitalHumanContract() || !definition.Supports(SurfaceDigitalHuman) {
		t.Fatalf("digital human metadata not admitted: %#v", definition)
	}
	if definition.ExtensionSchemaVersion != ZGIMetadataSchemaVersion13 || definition.ContractVersion != 4 {
		t.Fatalf("extension/contract version = %q/%d", definition.ExtensionSchemaVersion, definition.ContractVersion)
	}

	manifest, err := BuildAgentPluginManifest([]Definition{definition})
	if err != nil {
		t.Fatalf("BuildAgentPluginManifest() error = %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	extensions, _ := decoded["extensions"].(map[string]interface{})[ZGIConnectorExtensionNamespace].(map[string]interface{})
	if extensions["schema_version"] != ZGIMetadataSchemaVersion13 {
		t.Fatalf("manifest extension version = %#v", extensions["schema_version"])
	}
	roundTripped, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil {
		t.Fatalf("LoadManifestPackage() error = %v", err)
	}
	if len(roundTripped.Failures) != 0 || len(roundTripped.Definitions) != 1 ||
		!roundTripped.Definitions[0].IsDigitalHumanContract() {
		t.Fatalf("round-tripped digital human package = %#v", roundTripped)
	}
}

func TestBuiltInDefinitionsMustRoundTripThroughConformingManifestAdmission(t *testing.T) {
	loader := newPluginLoaderForTest(t)
	definition := pluginContractDefinitionFixture()
	manifest, err := BuildAgentPluginManifest([]Definition{definition})
	if err != nil {
		t.Fatalf("BuildAgentPluginManifest() error = %v", err)
	}
	plugin, err := loader.LoadManifestPackage(context.Background(), manifest)
	if err != nil {
		t.Fatalf("LoadManifestPackage() error = %v", err)
	}
	if plugin.PluginID != definition.PluginID || plugin.PluginVersion != definition.PluginVersion ||
		len(plugin.Definitions) != 1 || len(plugin.Failures) != 0 || !sha256HexPattern.MatchString(plugin.PluginDigest) {
		t.Fatalf("embedded package admission = %#v", plugin)
	}
	got := plugin.Definitions[0]
	if got.ID != definition.ID || got.ProviderID != definition.ProviderID ||
		got.Operations[0].ID != definition.Operations[0].ID || got.PluginDigest != plugin.PluginDigest {
		t.Fatalf("round-tripped definition = %#v", got)
	}
	if got.NameI18n["zh-Hans"] != definition.NameI18n["zh-Hans"] ||
		got.DescriptionI18n["zh-Hans"] != definition.DescriptionI18n["zh-Hans"] ||
		got.KeyFields[0].LabelI18n["zh-Hans"] != definition.KeyFields[0].LabelI18n["zh-Hans"] ||
		got.KeyFields[0].PlaceholderI18n["zh-Hans"] != definition.KeyFields[0].PlaceholderI18n["zh-Hans"] ||
		got.Operations[0].NameI18n["zh-Hans"] != definition.Operations[0].NameI18n["zh-Hans"] ||
		got.Operations[0].DescriptionI18n["zh-Hans"] != definition.Operations[0].DescriptionI18n["zh-Hans"] {
		t.Fatalf("localized metadata did not round trip: %#v", got)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["$schema"] = "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json"
	tampered, _ := json.Marshal(decoded)
	if _, err := loader.LoadManifestPackage(context.Background(), tampered); !errors.Is(err, ErrUnsupportedPluginSchema) {
		t.Fatalf("tampered embedded package error = %v", err)
	}
}

func TestPluginLoaderImplementsManifestFatalAndNonFatalBoundaries(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)

	t.Run("unknown top-level field is reported and ignored", func(t *testing.T) {
		root := writePluginManifest(t, map[string]interface{}{
			"$schema": AgentPluginsV1ManifestSchemaID, "name": "valid-plugin", "unknown": true,
		})
		plugin, err := loader.LoadDirectory(context.Background(), root)
		if err != nil || len(plugin.Warnings) != 1 {
			t.Fatalf("LoadDirectory() = %#v, %v", plugin, err)
		}
	})

	t.Run("non-object extensions is reported and ignored", func(t *testing.T) {
		root := writePluginManifest(t, map[string]interface{}{
			"$schema": AgentPluginsV1ManifestSchemaID, "name": "valid-plugin", "extensions": "invalid",
		})
		plugin, err := loader.LoadDirectory(context.Background(), root)
		if err != nil || len(plugin.Definitions) != 0 || len(plugin.Warnings) != 1 ||
			plugin.Warnings[0].Code != "non_object_extensions" {
			t.Fatalf("LoadDirectory() = %#v, %v", plugin, err)
		}
	})

	tests := []struct {
		name     string
		manifest map[string]interface{}
		want     error
	}{
		{
			name: "unsupported schema",
			manifest: map[string]interface{}{
				"$schema": "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json", "name": "valid-plugin",
			},
			want: ErrUnsupportedPluginSchema,
		},
		{
			name: "invalid plugin name",
			manifest: map[string]interface{}{
				"$schema": AgentPluginsV1ManifestSchemaID, "name": "Invalid-Plugin",
			},
			want: ErrPluginManifestInvalid,
		},
		{
			name: "invalid author schema",
			manifest: map[string]interface{}{
				"$schema": AgentPluginsV1ManifestSchemaID, "name": "valid-plugin",
				"author": map[string]interface{}{"name": "Acme", "extra": true},
			},
			want: ErrPluginManifestInvalid,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := writePluginManifest(t, testCase.manifest)
			_, err := loader.LoadDirectory(context.Background(), root)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("LoadDirectory() error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestPluginLoaderContainsConnectorAndPackageFailures(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)

	goodRoot := writePluginManifest(t, pluginManifestFixture(
		"good-plugin", "1.0.0", connectorMetadataFixture("good", ClassificationConnector),
	))
	badManifestRoot := writePluginManifest(t, map[string]interface{}{
		"$schema": AgentPluginsV1ManifestSchemaID, "name": "Bad Name",
	})
	invalidConnector := connectorMetadataFixture("broken", ClassificationConnector)
	invalidConnector["classification"] = ""
	mixedRoot := writePluginManifest(t, pluginManifestFixture(
		"mixed-plugin", "1.0.0",
		connectorMetadataFixture("mixed-valid", ClassificationDataSource), invalidConnector,
	))

	report := loader.LoadDirectories(context.Background(), []string{badManifestRoot, goodRoot, mixedRoot})
	if len(report.Failures) != 1 {
		t.Fatalf("package failures = %#v", report.Failures)
	}
	if len(report.Definitions) != 2 {
		t.Fatalf("valid definitions = %#v", report.Definitions)
	}
	if len(report.Plugins) != 2 || len(report.Plugins[1].Definitions) != 1 ||
		len(report.Plugins[1].Failures) != 1 || report.Plugins[1].Failures[0].ConnectorIndex != 1 {
		t.Fatalf("partial plugin result = %#v", report.Plugins)
	}
	if report.Definitions[0].ID != "good" || report.Definitions[1].ID != "mixed-valid" {
		t.Fatalf("valid definition order = %#v", report.Definitions)
	}
}

func TestPluginLoaderRejectsUnsupportedFirstReleaseConnectorMetadata(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)

	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{name: "redirect setup", mutate: func(value map[string]interface{}) { value["setup_method"] = "oauth" }},
		{name: "organization ownership", mutate: func(value map[string]interface{}) { value["ownership"] = "organization" }},
		{name: "missing workflow surface", mutate: func(value map[string]interface{}) { value["surfaces"] = []interface{}{"chat", "agent"} }},
		{name: "missing operation schemas", mutate: func(value map[string]interface{}) {
			operations := value["operations"].([]interface{})
			delete(operations[0].(map[string]interface{}), "input_schema")
		}},
		{name: "egress without destination", mutate: func(value map[string]interface{}) {
			operation := value["operations"].([]interface{})[0].(map[string]interface{})
			operation["data_egress"] = true
		}},
		{name: "localized value must be a string", mutate: func(value map[string]interface{}) {
			value["name_i18n"] = map[string]interface{}{"en-US": 42}
		}},
		{name: "localized value cannot be blank", mutate: func(value map[string]interface{}) {
			value["description_i18n"] = map[string]interface{}{"zh-Hans": "   "}
		}},
		{name: "unknown localized metadata remains strict", mutate: func(value map[string]interface{}) {
			value["unknown_i18n"] = map[string]interface{}{"en-US": "Unknown"}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			metadata := connectorMetadataFixture("unsupported", ClassificationConnector)
			testCase.mutate(metadata)
			root := writePluginManifest(t, pluginManifestFixture("unsupported-plugin", "1.0.0", metadata))
			plugin, err := loader.LoadDirectory(context.Background(), root)
			if err != nil {
				t.Fatalf("LoadDirectory() error = %v", err)
			}
			if len(plugin.Definitions) != 0 || len(plugin.Failures) != 1 ||
				!errors.Is(plugin.Failures[0].Err, ErrConnectorMetadataInvalid) {
				t.Fatalf("rejected Connector result = %#v", plugin)
			}
		})
	}
}

func TestPluginLoaderAdmissionHasNoConnectionOrCredentialSideEffect(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)
	root := writePluginManifest(t, pluginManifestFixture(
		"admission-plugin", "1.0.0", connectorMetadataFixture("admitted", ClassificationConnector),
	))
	spy := &definitionAdmitterSpy{}
	report := loader.LoadAndAdmitDirectories(context.Background(), []string{root}, spy)
	if len(report.Failures) != 0 || len(report.AdmittedDefinitions) != 1 || spy.calls != 1 || len(spy.definitions) != 1 {
		t.Fatalf("admission report/spy = %#v / %#v", report, spy)
	}
	encoded, err := json.Marshal(spy.definitions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "candidate-secret-sentinel") || strings.Contains(string(encoded), "encrypted_credentials") {
		t.Fatalf("admitted metadata contains credential material: %s", encoded)
	}
}

func TestPluginLoaderRejectsManifestPathOutsidePluginRoot(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)
	base := t.TempDir()
	root := filepath.Join(base, "plugin")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside-plugin.json")
	contents, err := json.Marshal(map[string]interface{}{
		"$schema": AgentPluginsV1ManifestSchemaID, "name": "outside-plugin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "plugin.json")); err != nil {
		t.Skipf("symlink creation is unavailable on this platform: %v", err)
	}
	_, err = loader.LoadDirectory(context.Background(), root)
	if !errors.Is(err, ErrPluginPackagePathEscape) {
		t.Fatalf("LoadDirectory() error = %v, want path escape", err)
	}
}

func TestPluginLoaderDigestCoversEntirePackage(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)
	root := writePluginManifest(t, pluginManifestFixture(
		"package-digest-plugin", "1.0.0", connectorMetadataFixture("package-digest", ClassificationConnector),
	))
	assetPath := filepath.Join(root, "README.md")
	if err := os.WriteFile(assetPath, []byte("first package contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := loader.LoadDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("second package contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := loader.LoadDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if first.PluginDigest == second.PluginDigest {
		t.Fatalf("package digest did not change after a non-manifest file changed: %q", first.PluginDigest)
	}
	if first.Definitions[0].PluginDigest != first.PluginDigest ||
		second.Definitions[0].PluginDigest != second.PluginDigest {
		t.Fatalf("definition package digests = %q / %q", first.Definitions[0].PluginDigest, second.Definitions[0].PluginDigest)
	}
}

func TestPluginLoaderRejectsAnyPackageSymlinkOutsidePluginRoot(t *testing.T) {
	t.Parallel()
	loader := newPluginLoaderForTest(t)
	root := writePluginManifest(t, pluginManifestFixture(
		"package-link-plugin", "1.0.0", connectorMetadataFixture("package-link", ClassificationConnector),
	))
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside package root"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escaped-asset")); err != nil {
		t.Skipf("symlink creation is unavailable on this platform: %v", err)
	}

	_, err := loader.LoadDirectory(context.Background(), root)
	if !errors.Is(err, ErrPluginPackagePathEscape) {
		t.Fatalf("LoadDirectory() error = %v, want path escape", err)
	}
}

type definitionAdmitterSpy struct {
	calls       int
	definitions []Definition
}

func (spy *definitionAdmitterSpy) Admit(_ context.Context, definitions []Definition) error {
	spy.calls++
	for _, definition := range definitions {
		spy.definitions = append(spy.definitions, cloneDefinition(definition))
	}
	return nil
}

func newPluginLoaderForTest(t *testing.T) *PluginLoader {
	t.Helper()
	loader, err := NewPluginLoader()
	if err != nil {
		t.Fatal(err)
	}
	return loader
}

func writePluginManifest(t *testing.T, manifest map[string]interface{}) string {
	t.Helper()
	root := t.TempDir()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func pluginManifestFixture(name, version string, connectors ...map[string]interface{}) map[string]interface{} {
	items := make([]interface{}, len(connectors))
	for index, connector := range connectors {
		items[index] = connector
	}
	return map[string]interface{}{
		"$schema": AgentPluginsV1ManifestSchemaID,
		"name":    name,
		"version": version,
		"extensions": map[string]interface{}{
			ZGIConnectorExtensionNamespace: map[string]interface{}{
				"schema_version": ZGIMetadataSchemaVersion1,
				"connectors":     items,
			},
		},
	}
}

func connectorMetadataFixture(id string, classification Classification) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "provider_id": id, "driver_id": id + "-rest", "auth_method_id": "personal_api_key",
		"name": "Connector " + id, "description": "Connector fixture", "classification": classification,
		"capability_types": []interface{}{"action", "data"}, "setup_method": "key", "ownership": "personal",
		"surfaces": []interface{}{"chat", "agent", "workflow"},
		"key_fields": []interface{}{map[string]interface{}{
			"name": "api_key", "label": "API key", "required": true, "secret": true,
		}},
		"operations": []interface{}{map[string]interface{}{
			"id": id + ".items.list", "tool_name": "list_" + strings.ReplaceAll(id, ".", "_") + "_items",
			"skill_id": "external-apps", "name": "List items",
			"input_schema": map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
			},
			"output_schema": map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{}, "additionalProperties": true,
			},
			"effect": "read", "data_egress": false, "sensitive_data_allowed": false,
			"risk_hint": "low", "idempotent": true,
		}},
	}
}
