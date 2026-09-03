package catalog

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogAcceptsFirstReleaseContract(t *testing.T) {
	t.Parallel()

	definition := validDefinition()
	catalog, err := New(definition)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, ok := catalog.Get(definition.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find admitted definition", definition.ID)
	}
	if got.Classification != ClassificationConnector || got.SetupMethod != SetupMethodKey || got.Ownership != OwnershipPersonal {
		t.Fatalf("first-release contract = classification %q, setup %q, ownership %q", got.Classification, got.SetupMethod, got.Ownership)
	}
	if !reflect.DeepEqual(got.Surfaces, FirstReleaseSurfaces()) {
		t.Fatalf("surfaces = %#v, want %#v", got.Surfaces, FirstReleaseSurfaces())
	}
	if len(got.KeyFields) != 1 || !got.KeyFields[0].Required || !got.KeyFields[0].Secret {
		t.Fatalf("key fields = %#v", got.KeyFields)
	}
	if len(got.Operations) != 1 || got.Operations[0].ID != "example.item.list" || got.Operations[0].ToolName != "list_example_items" {
		t.Fatalf("operations = %#v", got.Operations)
	}
}

func TestCatalogRejectsUnsupportedOrAmbiguousFirstReleaseDefinitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "not Agent Plugins 1.0", mutate: func(value *Definition) { value.SourceSchemaVersion = "1.1.0-draft" }},
		{name: "unclassified", mutate: func(value *Definition) { value.Classification = "" }},
		{name: "redirect setup", mutate: func(value *Definition) { value.SetupMethod = SetupMethod("oauth") }},
		{name: "organization ownership", mutate: func(value *Definition) { value.Ownership = Ownership("organization") }},
		{name: "missing chat", mutate: func(value *Definition) { value.Surfaces = []Surface{SurfaceAgent, SurfaceWorkflow} }},
		{name: "optional key only", mutate: func(value *Definition) { value.KeyFields[0].Required = false }},
		{name: "non-secret key field", mutate: func(value *Definition) { value.KeyFields[0].Secret = false }},
		{name: "duplicate operation id", mutate: func(value *Definition) {
			value.Operations = append(value.Operations, Operation{ID: "example.item.list", ToolName: "list_other_items", Name: "List other items"})
		}},
		{name: "duplicate tool name", mutate: func(value *Definition) {
			value.Operations = append(value.Operations, Operation{ID: "example.item.search", ToolName: "list_example_items", Name: "Search items"})
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			definition := validDefinition()
			testCase.mutate(&definition)
			if _, err := New(definition); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}
}

func TestCatalogReturnsDefensiveCopiesAndFiltersClassification(t *testing.T) {
	t.Parallel()

	connector := validDefinition()
	dataSource := validDefinition()
	dataSource.ID = "example-search"
	dataSource.PluginID = "zgi.example-search"
	dataSource.DriverID = "example-search-rest"
	dataSource.Classification = ClassificationDataSource
	dataSource.CapabilityTypes = []CapabilityType{CapabilityTypeData}

	catalog, err := New(connector, dataSource)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	items := catalog.List(Filter{Classification: ClassificationDataSource})
	if len(items) != 1 || items[0].ID != dataSource.ID {
		t.Fatalf("filtered items = %#v", items)
	}
	items[0].Surfaces[0] = Surface("mutated")
	items[0].NameI18n["zh-Hans"] = "被篡改"
	items[0].KeyFields[0].Name = "mutated"
	items[0].KeyFields[0].LabelI18n["zh-Hans"] = "被篡改"
	items[0].Operations[0].ID = "mutated"
	items[0].Operations[0].NameI18n["zh-Hans"] = "被篡改"

	again, ok := catalog.Get(dataSource.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find definition", dataSource.ID)
	}
	if again.Surfaces[0] != SurfaceChat || again.NameI18n["zh-Hans"] != "示例" ||
		again.KeyFields[0].Name != "api_key" || again.KeyFields[0].LabelI18n["zh-Hans"] != "API 密钥" ||
		again.Operations[0].ID != "example.item.list" || again.Operations[0].NameI18n["zh-Hans"] != "列出项目" {
		t.Fatalf("catalog state was mutated through a returned value: %#v", again)
	}
}

func TestCatalogRejectsInvalidOptionalLocalizedMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "invalid locale", mutate: func(value *Definition) { value.NameI18n = LocalizedText{"not a locale": "Example"} }},
		{name: "blank value", mutate: func(value *Definition) { value.DescriptionI18n = LocalizedText{"zh-Hans": "   "} }},
		{name: "duplicate locale casing", mutate: func(value *Definition) {
			value.Operations[0].NameI18n = LocalizedText{"en-US": "List", "en-us": "List duplicate"}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			definition := validDefinition()
			testCase.mutate(&definition)
			if _, err := New(definition); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("New() error = %v, want invalid definition", err)
			}
		})
	}
}

func TestCatalogKeepsScalarOnlyDefinitionsCompatible(t *testing.T) {
	definition := validDefinition()
	definition.NameI18n, definition.DescriptionI18n = nil, nil
	for index := range definition.KeyFields {
		definition.KeyFields[index].LabelI18n, definition.KeyFields[index].PlaceholderI18n = nil, nil
	}
	for index := range definition.Operations {
		definition.Operations[index].NameI18n, definition.Operations[index].DescriptionI18n = nil, nil
	}
	if _, err := New(definition); err != nil {
		t.Fatalf("scalar-only definition rejected: %v", err)
	}
	encoded, err := json.Marshal(definition)
	if err != nil || strings.Contains(string(encoded), "_i18n") {
		t.Fatalf("scalar-only wire metadata = %s, %v", encoded, err)
	}
}

func TestCatalogRejectsDuplicateStableIdentity(t *testing.T) {
	t.Parallel()

	first := validDefinition()
	second := validDefinition()
	second.PluginID = "zgi.different-plugin"

	if _, err := New(first, second); err == nil {
		t.Fatal("New() error = nil for duplicate connector identity")
	}
}

func TestCatalogActivatesCompatiblePackageAtomicallyAndDeactivatesPresentation(t *testing.T) {
	current := validDefinition()
	current.PluginDigest = strings.Repeat("a", 64)
	second := cloneDefinition(current)
	second.ID = "second"
	second.ProviderID = "second"
	second.DriverID = "second-rest"
	second.Operations[0].ID = "second.items.list"
	second.Operations[0].ToolName = "list_second_items"
	catalog, err := New(current, second)
	if err != nil {
		t.Fatal(err)
	}

	updated := cloneDefinition(current)
	updated.PluginVersion = "1.1.0"
	updated.PluginDigest = strings.Repeat("b", 64)
	updated.Name = "Updated display"
	updated.Operations = append(updated.Operations, additiveOperationFixture())
	if err := catalog.ActivatePackage([]Definition{updated}); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("ActivatePackage(removal) error = %v", err)
	}
	if got, _ := catalog.Get(current.ID); got.PluginVersion != current.PluginVersion {
		t.Fatalf("failed package activation partially changed catalog: %#v", got)
	}

	updatedSecond := cloneDefinition(second)
	updatedSecond.PluginVersion = updated.PluginVersion
	updatedSecond.PluginDigest = updated.PluginDigest
	if err := catalog.ActivatePackage([]Definition{updated, updatedSecond}); err != nil {
		t.Fatalf("ActivatePackage() error = %v", err)
	}
	if got, _ := catalog.Get(current.ID); got.PluginVersion != "1.1.0" || got.Name != "Updated display" {
		t.Fatalf("active definition = %#v", got)
	}
	catalog.DeactivatePlugin(current.PluginID)
	if _, found := catalog.Get(current.ID); found || len(catalog.List(Filter{})) != 0 {
		t.Fatal("inactive plugin remained available in the member catalog")
	}
}

func validDefinition() Definition {
	return Definition{
		ID:                  "example",
		PluginID:            "zgi.example",
		PluginVersion:       "1.0.0",
		SourceDialect:       SourceDialectAgentPluginsV1,
		SourceSchemaVersion: SourceSchemaVersion1,
		ProviderID:          "example",
		DriverID:            "example-rest",
		AuthMethodID:        "personal_api_key",
		Name:                "Example",
		NameI18n:            LocalizedText{"en-US": "Example", "zh-Hans": "示例"},
		Description:         "Example connector",
		DescriptionI18n:     LocalizedText{"en-US": "Example connector", "zh-Hans": "示例连接器"},
		Classification:      ClassificationConnector,
		CapabilityTypes:     []CapabilityType{CapabilityTypeAction},
		SetupMethod:         SetupMethodKey,
		Ownership:           OwnershipPersonal,
		Surfaces:            FirstReleaseSurfaces(),
		KeyFields: []KeyField{{
			Name: "api_key", Label: "API key",
			LabelI18n:       LocalizedText{"en-US": "API key", "zh-Hans": "API 密钥"},
			Placeholder:     "Enter an API key",
			PlaceholderI18n: LocalizedText{"en-US": "Enter an API key", "zh-Hans": "请输入 API 密钥"},
			Required:        true, Secret: true,
		}},
		Operations: []Operation{{
			ID: "example.item.list", ToolName: "list_example_items", Name: "List items",
			NameI18n:        LocalizedText{"en-US": "List items", "zh-Hans": "列出项目"},
			Description:     "List example items",
			DescriptionI18n: LocalizedText{"en-US": "List example items", "zh-Hans": "列出示例项目"},
		}},
	}
}
