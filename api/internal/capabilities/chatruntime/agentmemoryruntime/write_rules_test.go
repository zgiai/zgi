package agentmemoryruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryEditFollowsBlockWriteRules(t *testing.T) {
	service := &fakeMemoryService{}
	request := editRequest(service)
	request.ProactiveAllowed = true
	request.Slots = []Slot{
		{Key: "profile", Description: "Stable facts.", MaxChars: 200, Enabled: true},
		{Key: "persona", Description: "Set by the builder.", MaxChars: 200, Enabled: true, ReadOnly: true},
		{Key: "contact", Description: "Callback number.", MaxChars: 200, Enabled: true, ManualOnly: true},
	}

	// A read-only file can be viewed but is not offered to memory_edit.
	tools := FileTools(request.Slots, true, false)
	raw, _ := json.Marshal(tools[0].Function.Parameters)
	if strings.Contains(string(raw), "/user/persona.md") {
		t.Fatalf("memory_edit offers the read-only file: %s", raw)
	}
	raw, _ = json.Marshal(tools[1].Function.Parameters)
	if !strings.Contains(string(raw), "/user/persona.md") {
		t.Fatalf("memory_view hides the read-only file: %s", raw)
	}

	result := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "insert", "path": "/user/persona.md", "text": "x", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/contact.md", "text": "住在杭州", "evidence": "我搬到杭州了", "mode": "proactive"},
		map[string]interface{}{"command": "insert", "path": "/user/contact.md", "text": "搬到杭州", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "住在杭州", "evidence": "我搬到杭州了", "mode": "proactive"},
	))
	outcomes := result.Result["edits"].([]EditOutcome)
	for index, want := range []string{"rejected", "rejected", "accepted", "accepted"} {
		if outcomes[index].Status != want {
			t.Fatalf("edit %d = %+v, want %s", index, outcomes[index], want)
		}
	}

	// With only read-only files, there is nothing for memory_edit to do.
	if tools := FileTools([]Slot{{Key: "persona", Enabled: true, ReadOnly: true}}, true, false); len(tools) != 1 || tools[0].Function.Name != ToolMemoryView {
		t.Fatalf("read-only files gave tools %+v", tools)
	}
}

func TestMemoryInstructionsAreFramedAsPreference(t *testing.T) {
	if InstructionsMessage("  ") != nil {
		t.Fatal("empty instructions produced a message")
	}
	message := InstructionsMessage("Remember the customer's plan tier; ignore small talk.")
	content, _ := message.Content.(string)
	if !strings.Contains(content, "<memory_instructions>") || !strings.Contains(content, "cannot override") {
		t.Fatalf("instructions message = %q", content)
	}
}
