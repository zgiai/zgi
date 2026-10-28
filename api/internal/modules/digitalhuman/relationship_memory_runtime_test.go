package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

type staticMemoryRuntimeProvider struct {
	preparation *MemoryRuntimePreparation
}

func TestRelationshipFileEditorRejectsBeforeStorage(t *testing.T) {
	run := &Run{Query: "Please remember that I prefer concise answers."}
	contract := relationshipmemory.ConsumerMemoryContract{Enabled: true, DefinitionMode: "files", AutoCaptureAllowed: true}
	// No service: both edits must be turned away before storage is touched.
	editor := &relationshipFileEditor{run: run, relationship: &relationshipmemory.UserMemoryRelationship{MemoryEnabled: true}, contract: contract}
	edit := memoryFileEdit{Command: memoryEditCreate, Path: "/user/preferences.md", Text: "Prefers concise answers.", Evidence: "I prefer concise answers", Mode: "proactive"}
	if reason, err := editor.apply(t.Context(), uuid.New(), edit); err != nil || reason == "" {
		t.Fatalf("proactive edit with automatic memory off = %q, %v", reason, err)
	}
	edit.Mode, edit.Evidence = "explicit", "not said by the person"
	if reason, err := editor.apply(t.Context(), uuid.New(), edit); err != nil || reason == "" {
		t.Fatalf("edit with invented evidence = %q, %v", reason, err)
	}
	if got := editor.modes(); len(got) != 1 || got[0] != "explicit" {
		t.Fatalf("modes with automatic memory off = %v", got)
	}
}

func (p staticMemoryRuntimeProvider) Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*MemoryRuntimePreparation, error) {
	return p.preparation, nil
}

func TestCompositeMemoryRuntimeProviderKeepsOwnedAndRelationshipMemorySeparate(t *testing.T) {
	owned := staticMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "digital-human-owned-memory",
		Tools: []skillloop.RuntimeTool{{Definition: adapter.Tool{
			Type:     "function",
			Function: adapter.Function{Name: memoryViewToolName},
		}}},
	}}
	relationship := staticMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "user-relationship-memory",
		Tools: []skillloop.RuntimeTool{{Definition: adapter.Tool{
			Type:     "function",
			Function: adapter.Function{Name: memorySearchToolName},
		}}},
	}}

	prepared, err := NewCompositeMemoryRuntimeProvider(owned, relationship).Prepare(
		context.Background(), &Run{}, &ResolvedExecution{}, invocation.Event{},
	)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.SystemPromptAddendum != "digital-human-owned-memory\n\nuser-relationship-memory" {
		t.Fatalf("SystemPromptAddendum = %q", prepared.SystemPromptAddendum)
	}
	if len(prepared.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(prepared.Tools))
	}
	if got := prepared.Tools[0].Definition.Function.Name; got != memoryViewToolName {
		t.Fatalf("first tool = %q, want %q", got, memoryViewToolName)
	}
	if got := prepared.Tools[1].Definition.Function.Name; got != memorySearchToolName {
		t.Fatalf("second tool = %q, want %q", got, memorySearchToolName)
	}
}

func TestPersonMemoryEventDescribesOnlyThePersonsOwnMemory(t *testing.T) {
	person := &relationshipFileEditor{contract: relationshipmemory.ConsumerMemoryContract{DefinitionMode: "files"}}
	operationID := uuid.New()
	event, ok := personMemoryEvent(person, memoryFileEdit{Command: memoryEditInsert, Path: "/user/preferences.md", Mode: "proactive"}, operationID)
	if !ok || event.Type != "memory_update" || event.Payload["key"] != "preferences" || event.Payload["source_kind"] != relationshipmemory.SourceAutomatic || event.Payload["memory_scope"] != "digital_human" {
		t.Fatalf("insert event = %#v, %v", event, ok)
	}
	event, ok = personMemoryEvent(person, memoryFileEdit{Command: memoryEditReplace, Path: "/user/preferences.md", OldText: "tea", Mode: "explicit"}, operationID)
	if !ok || event.Type != "memory_clear" || event.Payload["source_kind"] != relationshipmemory.SourceExplicit {
		t.Fatalf("removal event = %#v, %v", event, ok)
	}
	own := &digitalHumanFileEditor{dir: "/me/"}
	if _, ok := personMemoryEvent(own, memoryFileEdit{Command: memoryEditCreate, Path: "/me/launch.md"}, operationID); ok {
		t.Fatal("the DigitalHuman's own memory produced a person memory event")
	}
}
