package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestWebSearchSkillCatalogOmittedWithoutConnectorProvider(t *testing.T) {
	manager := tools.NewToolManager(nil)
	svc := &service{skillRuntime: skills.NewRuntime(tools.NewToolEngine(manager), manager)}

	catalog, err := svc.catalogSkillMetadata(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("catalogSkillMetadata() error = %v", err)
	}
	if _, ok := webSearchMetadataForTest(catalog); ok {
		t.Fatal("web-search is visible without its connector provider")
	}
}

func TestWebSearchSkillCatalogAvailableButDefaultDisabledWithConnectorProvider(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(webSearchAvailabilityProviderForTest{}); err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	svc := &service{skillRuntime: skills.NewRuntime(tools.NewToolEngine(manager), manager)}
	organizationID := uuid.New()

	catalog, err := svc.catalogSkillMetadata(context.Background(), organizationID)
	if err != nil {
		t.Fatalf("catalogSkillMetadata() error = %v", err)
	}
	metadata, ok := webSearchMetadataForTest(catalog)
	if !ok {
		t.Fatal("web-search is absent with its connector provider registered")
	}
	if metadata.Status != skills.SkillStatusActive || !metadata.HasTools {
		t.Fatalf("web-search status/tools = %q/%v", metadata.Status, metadata.HasTools)
	}

	defaults := defaultEnabledSkillIDs(catalog)
	if containsSkillIDForTest(defaults, skills.SkillWebSearch) {
		t.Fatalf("web-search unexpectedly appears in default enabled skills: %#v", defaults)
	}
	effective, err := svc.effectiveOrganizationSkillIDs(context.Background(), organizationID, catalog)
	if err != nil {
		t.Fatalf("effectiveOrganizationSkillIDs() error = %v", err)
	}
	if containsSkillIDForTest(effective, skills.SkillWebSearch) {
		t.Fatalf("web-search unexpectedly enabled for an unconfigured organization: %#v", effective)
	}

	rows := organizationSkillConfigRows(organizationID, catalog, effective)
	for _, row := range rows {
		if row != nil && row.SkillID == skills.SkillWebSearch {
			if row.Enabled {
				t.Fatal("web-search organization config row is enabled by default")
			}
			return
		}
	}
	t.Fatal("web-search organization config row was not created")
}

func webSearchMetadataForTest(catalog []skills.SkillDiscoveryMetadata) (skills.SkillDiscoveryMetadata, bool) {
	for _, item := range catalog {
		if item.ID == skills.SkillWebSearch {
			return item, true
		}
	}
	return skills.SkillDiscoveryMetadata{}, false
}

func containsSkillIDForTest(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

type webSearchAvailabilityProviderForTest struct{}

func (webSearchAvailabilityProviderForTest) GetEntity() tools.ToolProviderEntity {
	return tools.ToolProviderEntity{
		Identity: tools.ToolProviderIdentity{
			Name:        skills.SkillWebSearch,
			Label:       tools.I18nText{"en_US": "Web Search"},
			Description: tools.I18nText{"en_US": "Web search availability test connector"},
		},
		ProviderType: tools.ToolProviderTypeConnector,
	}
}

func (webSearchAvailabilityProviderForTest) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeConnector
}

func (webSearchAvailabilityProviderForTest) GetTool(string) (tools.Tool, error) {
	return nil, tools.ErrToolNotFound
}

func (webSearchAvailabilityProviderForTest) GetTools() []tools.Tool {
	return nil
}

func (webSearchAvailabilityProviderForTest) ValidateCredentials(context.Context, map[string]interface{}) error {
	return nil
}
