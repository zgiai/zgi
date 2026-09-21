package catalog

import (
	"reflect"
	"testing"
)

func TestPresentationRegistryIsCoveredAndStable(t *testing.T) {
	if err := ValidatePresentationRegistry(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"weixin", "wecom", "dingtalk", "feishu", "gmail", "qqmail", "netease163", "github", "web-search"} {
		metadata, ok := PresentationMetadataFor(id)
		if !ok {
			t.Fatalf("missing presentation metadata for %s", id)
		}
		if err := ValidatePresentationMetadata(metadata); err != nil {
			t.Fatalf("invalid metadata for %s: %v", id, err)
		}
		if len(metadata.Tags) == 0 {
			t.Fatalf("metadata for %s has no tags", id)
		}
	}
}

func TestPresentationMappingsAndUnknownFallback(t *testing.T) {
	tests := []struct {
		id       string
		category ConnectorCategory
		tags     []ConnectorTag
	}{
		{"weixin", ConnectorCategoryCommunication, []ConnectorTag{ConnectorTagSocial, ConnectorTagMessaging}},
		{"wecom", ConnectorCategoryCommunication, []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration}},
		{"dingtalk", ConnectorCategoryProductivity, []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration, ConnectorTagWorkflow}},
		{"feishu", ConnectorCategoryProductivity, []ConnectorTag{ConnectorTagEnterprise, ConnectorTagMessaging, ConnectorTagCollaboration, ConnectorTagDocuments, ConnectorTagTables}},
		{"gmail", ConnectorCategoryCommunication, []ConnectorTag{ConnectorTagEmail}},
		{"qqmail", ConnectorCategoryCommunication, []ConnectorTag{ConnectorTagEmail}},
		{"netease163", ConnectorCategoryCommunication, []ConnectorTag{ConnectorTagEmail}},
		{"github", ConnectorCategoryDeveloper, []ConnectorTag{ConnectorTagCode, ConnectorTagRepository, ConnectorTagIssues, ConnectorTagCollaboration}},
		{"web-search", ConnectorCategoryInformation, []ConnectorTag{ConnectorTagSearch, ConnectorTagWeb, ConnectorTagResearch}},
	}
	for _, test := range tests {
		got, ok := PresentationFor(test.id)
		if !ok || got.Category != test.category || !reflect.DeepEqual(got.Tags, test.tags) {
			t.Fatalf("PresentationFor(%s) = %#v, %t", test.id, got, ok)
		}
		got.Tags[0] = ConnectorTagIntegration
		again, _ := PresentationFor(test.id)
		if !reflect.DeepEqual(again.Tags, test.tags) {
			t.Fatalf("PresentationFor(%s) leaked mutable tags", test.id)
		}
	}
	unknown, ok := PresentationFor("unreviewed.example")
	if ok || unknown.Category != ConnectorCategoryOther || len(unknown.Tags) != 0 {
		t.Fatalf("unknown fallback = %#v, %t", unknown, ok)
	}
}

func TestCatalogPresentationFiltersDoNotChangeDefinitionContract(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		category ConnectorCategory
		want     string
	}{
		{ConnectorCategoryInformation, "web-search"},
		{ConnectorCategoryDeveloper, "github"},
	} {
		items := catalog.List(Filter{Category: test.category})
		if len(items) != 1 || items[0].ID != test.want {
			t.Fatalf("category %s = %#v", test.category, items)
		}
	}
	items := catalog.List(Filter{Tag: ConnectorTagEmail})
	if len(items) != 3 {
		t.Fatalf("email tag count = %d", len(items))
	}
	before, err := DefinitionContractDigest(definitions[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := PresentationFor(definitions[0].ID); !ok {
		t.Fatalf("missing presentation for %s", definitions[0].ID)
	}
	after, err := DefinitionContractDigest(definitions[0])
	if err != nil || before != after {
		t.Fatalf("presentation metadata changed definition digest: %s -> %s (%v)", before, after, err)
	}
}

func TestPresentationMetadataRejectsUnsafeOrDuplicateValues(t *testing.T) {
	base := PresentationMetadata{
		ConnectorID: "example",
		Category:    ConnectorCategoryProductivity,
		Tags:        []ConnectorTag{ConnectorTagCollaboration},
		SourceURLs:  []string{"https://example.com/docs"},
		Publisher:   "Example",
		ReviewedAt:  "2026-09-12",
		Rationale:   "Public product documentation.",
	}
	for _, mutate := range []func(*PresentationMetadata){
		func(value *PresentationMetadata) { value.Tags = []ConnectorTag{ConnectorTagEmail, ConnectorTagEmail} },
		func(value *PresentationMetadata) { value.Tags = []ConnectorTag{ConnectorTag("arbitrary")} },
		func(value *PresentationMetadata) { value.SourceURLs = []string{"http://example.com/docs"} },
		func(value *PresentationMetadata) {
			value.SourceURLs = []string{"https://example.com/docs#private"}
		},
		func(value *PresentationMetadata) { value.Category = ConnectorCategory("social") },
	} {
		value := base
		mutate(&value)
		if ValidatePresentationMetadata(value) == nil {
			t.Fatalf("unsafe metadata was accepted: %#v", value)
		}
	}
}
