package skills_test

import (
	"context"
	skillspkg "github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	journaltools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/workjournal"
	"strings"
	"testing"
)

func TestWorkJournalProvidesOneRuntimeForDifferentBusinessPrompts(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(journaltools.NewProvider(nil, nil)); err != nil {
		t.Fatal(err)
	}
	runtime := skillspkg.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, "catalog")
	enabled, err := runtime.ResolveEnabledSkills(context.Background(), []string{"work-journal"})
	if err != nil {
		t.Fatal(err)
	}
	doc, trace, err := runtime.LoadSkill(context.Background(), enabled, "work-journal")
	if err != nil || trace.Status != "success" {
		t.Fatal(trace, err)
	}
	want := []string{"save_record", "read_record", "list_records", "record_history", "prepare_operation", "dispatch_operation", "read_operation", "list_operations", "recover_operation"}
	if !sameStrings(toolNames(doc.Tools), want) {
		t.Fatal(toolNames(doc.Tools))
	}
	for _, tool := range doc.Tools {
		if tool.ProviderID != journaltools.ProviderID {
			t.Fatal(tool)
		}
	}
	for _, name := range []string{"social-operations.md", "customer-support.md"} {
		body, _, err := runtime.ReadReference(context.Background(), enabled, "work-journal", name)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"work_key", "prepare_operation", "dispatch_operation", "unknown"} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing %s", name, required)
			}
		}
	}
	if strings.Contains(doc.Instructions, "daily_seven_day_samples") || strings.Contains(doc.Instructions, "publication_language") {
		t.Fatal("business policy leaked into infrastructure instructions")
	}
}

// The same reference must be available in source checkouts and API binaries
// running without a catalog directory on disk.
func TestWorkJournalResearchReferenceAvailableInBothCatalogs(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(journaltools.NewProvider(nil, nil)); err != nil {
		t.Fatal(err)
	}
	var source string
	for _, root := range []string{"catalog", "internal/modules/skills/catalog"} {
		runtime := skillspkg.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, root)
		enabled, err := runtime.ResolveEnabledSkills(t.Context(), []string{"work-journal"})
		if err != nil {
			t.Fatal(root, err)
		}
		body, trace, err := runtime.ReadReference(t.Context(), enabled, "work-journal", "research-collection.md")
		if err != nil || trace.Status != "success" || strings.TrimSpace(body) == "" {
			t.Fatalf("%s: reference not loaded: %v, %v", root, trace, err)
		}
		if source == "" {
			source = body
		} else if body != source {
			t.Fatal("embedded research reference differs from the source catalog")
		}
	}
}
