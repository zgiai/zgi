package agentmemoryworker

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func TestConsolidationEditsAreCheckedOneByOne(t *testing.T) {
	turnID := uuid.New()
	turns := []completedTurn{{ID: turnID, Query: "我喜欢乌龙茶。对，就那个方案。", Answer: "建议方案：每天步行 8000 步。", ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}}
	turnIDs := map[string]struct{}{turnID.String(): {}}
	current := map[string]agentmemory.SlotValueResponse{
		"preferences": {SlotResponse: agentmemory.SlotResponse{Key: "preferences"}, Content: "喜欢乌龙茶。\n喜欢乌龙茶。", Revision: 2},
		"profile":     {SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "住在杭州。", Revision: 1, SourceKind: agentmemory.SourceKindExplicit, SourceMessageID: turnID.String()},
	}
	working := map[string]string{}
	check := func(edit consolidationEdit) (string, string, string) {
		key, content, _, reason := applyConsolidationEdit(edit, current, working, turns, turnIDs)
		if reason == "" {
			working[key] = content
		}
		return key, content, reason
	}

	// A rewrite merges the duplicate line.
	if _, content, reason := check(consolidationEdit{Command: "rewrite", Path: "/user/preferences.md", Content: "喜欢乌龙茶。", Evidence: "我喜欢乌龙茶"}); reason != "" || content != "喜欢乌龙茶。" {
		t.Fatalf("dedupe rewrite = %q, %q", content, reason)
	}
	// A confirmation saves what the assistant proposed, quoting the user's confirmation.
	if _, content, reason := check(consolidationEdit{Command: "insert", Path: "/user/preferences.md", Text: "每日目标：步行 8000 步。", Evidence: "对，就那个方案"}); reason != "" || content != "喜欢乌龙茶。\n每日目标：步行 8000 步。" {
		t.Fatalf("confirmed insert = %q, %q", content, reason)
	}
	for name, edit := range map[string]consolidationEdit{
		"already saved":          {Command: "insert", Path: "/user/preferences.md", Text: "喜欢乌龙茶。", Evidence: "我喜欢乌龙茶"},
		"assistant-only quote":   {Command: "insert", Path: "/user/profile.md", Text: "步行 8000 步", Evidence: "建议方案"},
		"invalid file name":      {Command: "insert", Path: "/user/Other File.md", Text: "x", Evidence: "我喜欢乌龙茶"},
		"over the total limit":   {Command: "insert", Path: "/user/notes.md", Text: "我喜欢乌龙茶" + strings.Repeat("。", agentmemory.MaxMemoryChars), Evidence: "我喜欢乌龙茶"},
		"explicit file rewrite":  {Command: "rewrite", Path: "/user/profile.md", Content: "住在苏州。", Evidence: "我喜欢乌龙茶"},
		"ambiguous str_replace":  {Command: "str_replace", Path: "/user/profile.md", OldText: "不存在", NewText: "x", Evidence: "我喜欢乌龙茶"},
		"secret in the new text": {Command: "insert", Path: "/user/profile.md", Text: "密码是 abc123", Evidence: "我喜欢乌龙茶"},
	} {
		if _, _, reason := check(edit); reason == "" {
			t.Fatalf("%s edit was accepted", name)
		}
	}
	// A new topic gets a new file the model names.
	if key, content, reason := check(consolidationEdit{Command: "insert", Path: "/user/drinks.md", Text: "喜欢乌龙茶。", Evidence: "我喜欢乌龙茶"}); reason != "" || key != "drinks" || content != "喜欢乌龙茶。" {
		t.Fatalf("new file = %q %q, %q", key, content, reason)
	}
	// A plain correction of the explicit file is still allowed.
	if _, content, reason := check(consolidationEdit{Command: "str_replace", Path: "/user/profile.md", OldText: "杭州", NewText: "杭州西湖区", Evidence: "我喜欢乌龙茶"}); reason != "" || content != "住在杭州西湖区。" {
		t.Fatalf("str_replace on explicit file = %q, %q", content, reason)
	}
}

func TestConsolidationModelFollowsTheDeploymentSetting(t *testing.T) {
	turns := []completedTurn{{ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}}
	if provider, model := consolidationModel(turns); provider != "deepseek" || model != "deepseek-v4-flash" {
		t.Fatalf("default model = %s/%s", provider, model)
	}
	t.Setenv(consolidationModelVariable, "openai/gpt-5-mini")
	if provider, model := consolidationModel(turns); provider != "openai" || model != "gpt-5-mini" {
		t.Fatalf("configured model = %s/%s", provider, model)
	}
	t.Setenv(consolidationModelVariable, "not-a-model")
	if provider, _ := consolidationModel(turns); provider != "deepseek" {
		t.Fatalf("malformed setting was used: %s", provider)
	}
}

func TestConsolidationPromptShowsFilesAndInstructions(t *testing.T) {
	turns := []completedTurn{{ID: uuid.New(), Query: "我住在杭州。", ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}}
	values := []agentmemory.SlotValueResponse{{SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "住在上海。"}}
	request, err := (&Runner{}).consolidationChatRequest(turns, agentmemory.FileSlots(values), values, "Keep the customer's plan tier.")
	if err != nil {
		t.Fatal(err)
	}
	system, _ := request.Messages[0].Content.(string)
	for _, want := range []string{"<memory_instructions>\nKeep the customer's plan tier.\n</memory_instructions>", `"path":"/user/profile.md"`, `"limit_chars":10000`, "You name and organize the files"} {
		if !strings.Contains(system, want) {
			t.Fatalf("consolidation prompt lacks %q:\n%s", want, system)
		}
	}
}

func TestConsolidationArchiveMovesTextOutWithoutEvidence(t *testing.T) {
	turnID := uuid.New()
	turns := []completedTurn{{ID: turnID, Query: "今天聊点别的。"}}
	current := map[string]agentmemory.SlotValueResponse{
		"profile": {SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "住在杭州。\n2019 年在京都旅行。", Revision: 3},
	}
	key, content, source, reason := applyConsolidationEdit(consolidationEdit{Command: "archive", Path: "/user/profile.md", OldText: "2019 年在京都旅行。"}, current, map[string]string{}, turns, map[string]struct{}{})
	if reason != "" || key != "profile" || content != "住在杭州。" || source == nil || source.ID != turnID {
		t.Fatalf("archive = %q, %q, %v, %q", key, content, source, reason)
	}
	for name, edit := range map[string]consolidationEdit{
		"whole file":   {Command: "archive", Path: "/user/profile.md", OldText: "住在杭州。\n2019 年在京都旅行。"},
		"not present":  {Command: "archive", Path: "/user/profile.md", OldText: "上海"},
		"missing file": {Command: "archive", Path: "/user/persona.md", OldText: "x"},
	} {
		if _, _, _, reason := applyConsolidationEdit(edit, current, map[string]string{}, turns, map[string]struct{}{}); reason == "" {
			t.Fatalf("%s archive was accepted", name)
		}
	}
}

func TestConsolidationYieldsToANewerConversation(t *testing.T) {
	conversationID := uuid.New()
	turn := &completedTurn{CompletedAt: time.Unix(1_000, 0)}
	forgotLater := agentmemory.SlotValueResponse{SourceConversationID: uuid.NewString(), SourceCompletedAt: 1_100}
	if !supersededByLaterConversation(forgotLater, conversationID, turn) {
		t.Fatal("an edit from an older turn overrode a newer conversation's change")
	}
	sameConversation := forgotLater
	sameConversation.SourceConversationID = conversationID.String()
	earlier := forgotLater
	earlier.SourceCompletedAt = 900
	for name, value := range map[string]agentmemory.SlotValueResponse{"same conversation": sameConversation, "earlier change": earlier, "never written": {}} {
		if supersededByLaterConversation(value, conversationID, turn) {
			t.Fatalf("%s: edit was wrongly superseded", name)
		}
	}
}
