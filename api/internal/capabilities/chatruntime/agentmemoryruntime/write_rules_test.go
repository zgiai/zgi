package agentmemoryruntime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

// The model names memory files itself (ADR 0017): a new name creates a
// file, a malformed path is rejected, and all files together stay within
// the total limit.
func TestMemoryEditCreatesFilesWithinTheTotalLimit(t *testing.T) {
	service := &fakeMemoryService{}
	full := blockValue("notes", "", strings.Repeat("x", agentmemory.MaxMemoryChars-10))
	request := editRequest(service, full)
	request.ProactiveAllowed = true

	raw, _ := json.Marshal(FileTools(true, false)[0].Function.Parameters)
	if strings.Contains(string(raw), `"enum":["/user/`) {
		t.Fatalf("memory_edit still limits paths to a list: %s", raw)
	}

	result := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "insert", "path": "/user/home_city.md", "text": "杭州", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/Home City.md", "text": "杭州", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/travel.md", "text": "搬到杭州了，以后常去西湖边走走", "evidence": "我搬到杭州了", "mode": "proactive"},
	))
	outcomes := result.Result["edits"].([]EditOutcome)
	for index, want := range []string{"accepted", "rejected", "rejected"} {
		if outcomes[index].Status != want {
			t.Fatalf("edit %d = %+v, want %s", index, outcomes[index], want)
		}
	}
	if !strings.Contains(outcomes[2].Reason, "over its limit") {
		t.Fatalf("over-limit edit reason = %q", outcomes[2].Reason)
	}
	if len(service.request.Operations) != 1 || service.request.Operations[0].Key != "home_city" {
		t.Fatalf("operations = %+v, want one new file home_city", service.request.Operations)
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
