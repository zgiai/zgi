package skills

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestExternalAppsCatalogIsHiddenConnectedAppsRuntimeCapability(t *testing.T) {
	runtime := NewRuntime(nil, nil)
	metadata, err := runtime.GetSkillMetadata(context.Background(), SkillExternalApps)
	if err != nil {
		t.Fatalf("GetSkillMetadata() error = %v", err)
	}
	if !IsHiddenSystemSkill(SkillExternalApps) || !IsRuntimeManagedSystemSkill(SkillExternalApps) || SkillUserSelectable(*metadata) {
		t.Fatalf("external apps exposure = %#v", SkillExposureForMetadata(*metadata))
	}
	profile := SkillExposureForMetadata(*metadata)
	if profile.Category != SkillExposureHiddenRuntime || profile.SystemAsset || profile.PageContextRequired || profile.GovernanceRisk != SkillGovernanceRiskMixed {
		t.Fatalf("external apps profile = %#v", profile)
	}
	if len(metadata.SupportedCallers) != 2 ||
		metadata.SupportedCallers[0] != SkillCallerAIChat ||
		metadata.SupportedCallers[1] != SkillCallerAgent {
		t.Fatalf("supported callers = %#v", metadata.SupportedCallers)
	}
	if metadata.DependencyType != SkillDependencyStandalone || len(metadata.IntegrationRequirements) != 0 {
		t.Fatalf("hidden facade must not appear as a business integration-dependent Skill: %#v", metadata)
	}

	documents, err := runtime.ListSystemSkillDocuments(context.Background())
	if err != nil {
		t.Fatalf("ListSystemSkillDocuments() error = %v", err)
	}
	var document *SkillDocument
	for index := range documents {
		if documents[index].Metadata.ID == SkillExternalApps {
			document = &documents[index]
			break
		}
	}
	if document == nil {
		t.Fatal("external-apps Skill document not found")
	}
	wantTools := map[string]struct{}{
		"list_connections": {}, "search_actions": {}, "get_action_guide": {}, "execute_action": {},
	}
	for _, definition := range document.Tools {
		if definition.ProviderType != tools.ToolProviderTypeConnector || definition.ProviderID != "external-integrations" {
			t.Fatalf("tool %s provider = %s/%s", definition.Name, definition.ProviderType, definition.ProviderID)
		}
		if definition.Governance == nil {
			t.Fatalf("tool %s has no fail-closed governance manifest", definition.Name)
		}
		delete(wantTools, definition.Name)
	}
	if len(wantTools) != 0 {
		t.Fatalf("external-apps missing tools: %#v", wantTools)
	}
}
