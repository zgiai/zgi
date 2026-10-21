package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestExternalAppsCatalogIsHiddenConnectedAppsRuntimeCapability(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	metadata, err := runtime.GetSkillMetadata(context.Background(), SkillConnectorRuntime)
	if err != nil {
		t.Fatalf("GetSkillMetadata() error = %v", err)
	}
	if !IsHiddenSystemSkill(SkillConnectorRuntime) || !IsRuntimeManagedSystemSkill(SkillConnectorRuntime) || SkillUserSelectable(*metadata) {
		t.Fatalf("external apps exposure = %#v", SkillExposureForMetadata(*metadata))
	}
	profile := SkillExposureForMetadata(*metadata)
	if profile.Category != SkillExposureHiddenRuntime || profile.SystemAsset || profile.PageContextRequired || profile.GovernanceRisk != SkillGovernanceRiskMixed {
		t.Fatalf("external apps profile = %#v", profile)
	}
	if len(metadata.SupportedCallers) != 3 ||
		metadata.SupportedCallers[0] != SkillCallerAIChat ||
		metadata.SupportedCallers[1] != SkillCallerAgent ||
		metadata.SupportedCallers[2] != SkillCallerDigitalHuman {
		t.Fatalf("supported callers = %#v", metadata.SupportedCallers)
	}
	if metadata.DependencyType != SkillDependencyStandalone || len(metadata.ConnectorRequirements) != 0 {
		t.Fatalf("hidden facade must not appear as a business integration-dependent Skill: %#v", metadata)
	}

	documents, err := runtime.ListSystemSkillDocuments(context.Background())
	if err != nil {
		t.Fatalf("ListSystemSkillDocuments() error = %v", err)
	}
	var document *SkillDocument
	for index := range documents {
		if documents[index].Metadata.ID == SkillConnectorRuntime {
			document = &documents[index]
			break
		}
	}
	if document == nil {
		t.Fatal("external-apps Skill document not found")
	}
	if len(document.Tools) != 0 || !strings.Contains(document.Instructions, "server") || strings.Contains(document.Instructions, "execute_action") {
		t.Fatalf("native capability document has static facade tools: %#v", document)
	}
}

func TestConnectorCapabilityDocumentNeedsNoStaticProvider(t *testing.T) {
	runtime := NewRuntime(nil, tools.NewToolManager(nil))
	doc, err := runtime.loadSkillDocument(SkillConnectorRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.validateSkillTools(context.Background(), doc); err != nil {
		t.Fatal(err)
	}
	doc.Metadata.References = append(doc.Metadata.References, SkillReference{Path: "references/missing.md"})
	if runtime.validateSkillReferences(doc) == nil {
		t.Fatal("missing reference accepted")
	}
	doc.Tools = []SkillToolDefinition{{Name: "read_item", ProviderType: tools.ToolProviderTypeBuiltin, ProviderID: "missing-provider"}}
	if runtime.validateSkillTools(context.Background(), doc) == nil {
		t.Fatal("missing ordinary tool accepted")
	}
}
