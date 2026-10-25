package agentmemoryruntime

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func blockValue(key, description, content string) agentmemory.SlotValueResponse {
	return agentmemory.SlotValueResponse{SlotResponse: agentmemory.SlotResponse{Key: key, Description: description, MaxChars: 2000, AutoWrite: true}, Content: content, Revision: 1}
}

func TestMemoryBlocksShowEveryFileAndNeverDropOneSilently(t *testing.T) {
	values := []agentmemory.SlotValueResponse{
		blockValue("profile", "Stable facts about the user.", strings.Repeat("资料", 400)),
		blockValue("preferences", "How the user likes answers.", ""),
		blockValue("standing_instructions", "Instructions for every conversation.", strings.Repeat("指令", 400)),
		blockValue("project_context", "Ongoing work.", "Blood type O."),
	}

	// Everything fits: every file appears with its path, size, and description, the empty one included.
	rendered, metadata := RenderBlocks(values, 8000)
	for _, want := range []string{`<file path="/user/profile.md" owner="user" chars="800/2000">`, `<file path="/user/preferences.md" owner="user" chars="0/2000">`,
		"<description>How the user likes answers.</description>", "Blood type O.", "untrusted user context"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered blocks lack %q:\n%s", want, rendered)
		}
	}
	if len(metadata) != 4 {
		t.Fatalf("metadata = %v", metadata)
	}

	// Too little room: the file that does not fit is cut visibly and the rest are listed, not dropped.
	rendered, metadata = RenderBlocks(values, 2100)
	if !strings.Contains(rendered, "[truncated: the rest of this file did not fit the context budget]") {
		t.Fatalf("no truncation marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, `<file path="/user/project_context.md" owner="user" chars="13/2000" omitted="context budget" />`) {
		t.Fatalf("the file after the cut is not listed:\n%s", rendered)
	}
	if len(rendered) > 0 && len([]rune(rendered)) > 2100 {
		t.Fatalf("rendered %d characters over a 2100 budget", len([]rune(rendered)))
	}
	if last := metadata[len(metadata)-1]; last["key"] != "project_context" || last["omitted"] != true {
		t.Fatalf("metadata does not report the omitted file: %v", metadata)
	}

	// The legacy renderer is unchanged.
	if legacy, _ := Render(values, 8000, false); !strings.HasPrefix(legacy, "<user_relationship_memory>") {
		t.Fatalf("legacy render changed: %.80s", legacy)
	}
}
