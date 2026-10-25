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
	slots := []agentmemory.RuntimeSlot{
		{Key: "preferences", Description: "How the user likes things.", MaxChars: 60, Enabled: true},
		{Key: "profile", Description: "Stable facts.", MaxChars: 200, Enabled: true},
	}
	current := map[string]agentmemory.SlotValueResponse{
		"preferences": {SlotResponse: agentmemory.SlotResponse{Key: "preferences"}, Content: "喜欢乌龙茶。\n喜欢乌龙茶。", Revision: 2},
		"profile":     {SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "住在杭州。", Revision: 1, SourceKind: agentmemory.SourceKindExplicit, SourceMessageID: turnID.String()},
	}
	working := map[string]string{}
	check := func(edit consolidationEdit) (string, string, string) {
		key, content, _, reason := applyConsolidationEdit(edit, slots, current, working, turns, turnIDs)
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
		"unknown file":           {Command: "insert", Path: "/user/other.md", Text: "x", Evidence: "我喜欢乌龙茶"},
		"over the limit":         {Command: "insert", Path: "/user/preferences.md", Text: "一段很长很长的偏好描述，一定会让这个文件超过它的字数上限，所以应该被跳过而不是写入，再多写一些字让它足够长。", Evidence: "我喜欢乌龙茶"},
		"explicit file rewrite":  {Command: "rewrite", Path: "/user/profile.md", Content: "住在苏州。", Evidence: "我喜欢乌龙茶"},
		"ambiguous str_replace":  {Command: "str_replace", Path: "/user/profile.md", OldText: "不存在", NewText: "x", Evidence: "我喜欢乌龙茶"},
		"secret in the new text": {Command: "insert", Path: "/user/profile.md", Text: "密码是 abc123", Evidence: "我喜欢乌龙茶"},
	} {
		if _, _, reason := check(edit); reason == "" {
			t.Fatalf("%s edit was accepted", name)
		}
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

func TestConsolidationLeavesProtectedFilesAndCarriesInstructions(t *testing.T) {
	turnID := uuid.New()
	turns := []completedTurn{{ID: turnID, Query: "我住在杭州。", ModelProvider: "deepseek", ModelName: "deepseek-v4-flash"}}
	slots := []agentmemory.RuntimeSlot{
		{Key: "persona", Description: "Set by the builder.", MaxChars: 200, Enabled: true, ReadOnly: true},
		{Key: "contact", Description: "Callback number.", MaxChars: 200, Enabled: true, ManualOnly: true},
		{Key: "profile", Description: "Stable facts.", MaxChars: 200, Enabled: true},
	}
	for _, path := range []string{"/user/persona.md", "/user/contact.md"} {
		edit := consolidationEdit{Command: "insert", Path: path, Text: "住在杭州", Evidence: "我住在杭州"}
		if _, _, _, reason := applyConsolidationEdit(edit, slots, map[string]agentmemory.SlotValueResponse{}, map[string]string{}, turns, map[string]struct{}{turnID.String(): {}}); reason == "" {
			t.Fatalf("background edit of %s was accepted", path)
		}
	}
	request, err := (&Runner{}).consolidationChatRequest(turns, slots, nil, "Keep the customer's plan tier.")
	if err != nil {
		t.Fatal(err)
	}
	system, _ := request.Messages[0].Content.(string)
	if !strings.Contains(system, "<memory_instructions>\nKeep the customer's plan tier.\n</memory_instructions>") || !strings.Contains(system, `"access":"context only"`) {
		t.Fatalf("consolidation prompt lacks instructions or file access:\n%s", system)
	}
}

func TestConsolidationArchiveMovesTextOutWithoutEvidence(t *testing.T) {
	turnID := uuid.New()
	turns := []completedTurn{{ID: turnID, Query: "今天聊点别的。"}}
	slots := []agentmemory.RuntimeSlot{
		{Key: "profile", MaxChars: 200, Enabled: true},
		{Key: "persona", MaxChars: 200, Enabled: true, ReadOnly: true},
	}
	current := map[string]agentmemory.SlotValueResponse{
		"profile": {SlotResponse: agentmemory.SlotResponse{Key: "profile"}, Content: "住在杭州。\n2019 年在京都旅行。", Revision: 3},
		"persona": {SlotResponse: agentmemory.SlotResponse{Key: "persona"}, Content: "x", Revision: 1},
	}
	key, content, source, reason := applyConsolidationEdit(consolidationEdit{Command: "archive", Path: "/user/profile.md", OldText: "2019 年在京都旅行。"}, slots, current, map[string]string{}, turns, map[string]struct{}{})
	if reason != "" || key != "profile" || content != "住在杭州。" || source == nil || source.ID != turnID {
		t.Fatalf("archive = %q, %q, %v, %q", key, content, source, reason)
	}
	for name, edit := range map[string]consolidationEdit{
		"whole file":  {Command: "archive", Path: "/user/profile.md", OldText: "住在杭州。\n2019 年在京都旅行。"},
		"not present": {Command: "archive", Path: "/user/profile.md", OldText: "上海"},
		"read-only":   {Command: "archive", Path: "/user/persona.md", OldText: "x"},
	} {
		if _, _, _, reason := applyConsolidationEdit(edit, slots, current, map[string]string{}, turns, map[string]struct{}{}); reason == "" {
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
