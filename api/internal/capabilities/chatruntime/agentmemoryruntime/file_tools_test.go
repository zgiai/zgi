package agentmemoryruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func editRequest(service *fakeMemoryService, values ...agentmemory.SlotValueResponse) MutationRequest {
	messageID := uuid.New()
	return MutationRequest{
		MemoryService: service, WorkspaceID: uuid.New(), AgentID: uuid.New(), UserID: uuid.New(), UserScope: "account",
		Slots: []Slot{
			{Key: "profile", Description: "Stable facts about the user.", MaxChars: 200, Enabled: true},
			{Key: "contact", Description: "Callback phone number for support.", MaxChars: 200, Enabled: true},
		},
		CurrentValues:      values,
		MutationMetadata:   agentmemory.MutationMetadata{SourceMessageID: &messageID},
		LatestUserMessage:  "就选 B，帮我记住。我搬到杭州了。",
		RecentUserMessages: []string{"给我三个选项，其中 B 是每天步行 8000 步。"},
		ProactiveAllowed:   false,
		CallID:             "call-1",
	}
}

func edits(t *testing.T, items ...map[string]interface{}) string {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{"edits": items})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestMemoryEditAppliesEachEditOnItsOwnAndWritesOneValuePerFile(t *testing.T) {
	service := &fakeMemoryService{}
	current := blockValue("profile", "", "住在上海。\n喜欢美式咖啡。")
	current.Revision = 4
	request := editRequest(service, current)
	result := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "str_replace", "path": "/user/profile.md", "old_text": "住在上海。", "new_text": "住在杭州。", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "每日目标：步行 8000 步。", "evidence": "B 是每天步行 8000 步", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "喜欢美式咖啡。", "evidence": "就选 B", "mode": "explicit"},
		map[string]interface{}{"command": "str_replace", "path": "/user/profile.md", "old_text": "不存在的文字", "new_text": "x", "evidence": "就选 B", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "Lives near the office.", "evidence": "the assistant said so", "mode": "explicit"},
		map[string]interface{}{"command": "clear", "path": "/user/profile.md", "evidence": "就选 B", "mode": "proactive"},
	))
	if result.Error != nil {
		t.Fatalf("partially valid call failed: %v", result.Error)
	}
	outcomes := result.Result["edits"].([]EditOutcome)
	wantStatus := []string{"accepted", "accepted", "rejected", "rejected", "rejected", "rejected"}
	for index, want := range wantStatus {
		if outcomes[index].Status != want {
			t.Fatalf("edit %d = %+v, want %s (all: %+v)", index, outcomes[index], want, outcomes)
		}
	}
	for index, reason := range map[int]string{2: "already in the file", 3: "appears 0 times", 4: "evidence", 5: "disabled"} {
		if !strings.Contains(outcomes[index].Reason, reason) {
			t.Fatalf("edit %d reason = %q, want it to mention %q", index, outcomes[index].Reason, reason)
		}
	}
	if service.calls != 1 || len(service.request.Operations) != 1 {
		t.Fatalf("writes = %d calls, %+v", service.calls, service.request.Operations)
	}
	written := service.request.Operations[0]
	if written.Content != "住在杭州。\n喜欢美式咖啡。\n每日目标：步行 8000 步。" || written.ExpectedRevision != 4 || written.Mode != "explicit" {
		t.Fatalf("written value = %+v", written)
	}

	// Another call in the same turn writes under different operation IDs.
	again := editRequest(&fakeMemoryService{}, current)
	again.MutationMetadata = request.MutationMetadata
	again.CallID = "call-2"
	second := ExecuteEdits(t.Context(), again, edits(t, map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "New line.", "evidence": "就选 B", "mode": "explicit"}))
	if second.Response.Operations[0].OperationID == written.OperationID.String() {
		t.Fatal("a second call reused the first call's operation ID")
	}
}

func TestMemoryEditRejectsEverythingAsOneValidationError(t *testing.T) {
	service := &fakeMemoryService{}
	result := ExecuteEdits(t.Context(), editRequest(service), edits(t,
		map[string]interface{}{"command": "insert", "path": "/user/Bad Name.md", "text": "x", "evidence": "就选 B", "mode": "explicit"}))
	if result.Error == nil || !ValidationError(result.Error) || service.calls != 0 {
		t.Fatalf("all-rejected call = %+v, writes %d", result, service.calls)
	}
}

func TestMemoryEditRejectsContactDetailsAndIdentifiers(t *testing.T) {
	service := &fakeMemoryService{}
	request := editRequest(service)
	request.LatestUserMessage = "请记住我的回电号码是 13800001111，身份证号 110101199003071234。"
	result := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "insert", "path": "/user/contact.md", "text": "回电号码：13800001111", "evidence": "回电号码是 13800001111", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "手机号 13800001111", "evidence": "回电号码是 13800001111", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/contact.md", "text": "身份证号 110101199003071234", "evidence": "身份证号 110101199003071234", "mode": "explicit"},
	))
	outcomes := result.Result["edits"].([]EditOutcome)
	if outcomes[0].Status != "rejected" || outcomes[1].Status != "rejected" || outcomes[2].Status != "rejected" {
		t.Fatalf("contact outcomes = %+v", outcomes)
	}
}

func TestMemoryViewReturnsWholeNumberedFiles(t *testing.T) {
	request := editRequest(nil, blockValue("profile", "", "第一行\n第二行"))
	view := ExecuteView(request, `{"path":"/user/profile.md"}`)
	files := view["files"].([]map[string]interface{})
	if view["status"] != "success" || len(files) != 1 || files[0]["content"] != "1\t第一行\n2\t第二行" {
		t.Fatalf("view = %+v", view)
	}
	if listing := ExecuteView(request, `{}`); len(listing["files"].([]map[string]interface{})) != 1 || listing["used_chars"] != 7 {
		t.Fatalf("listing = %+v", listing)
	}
	if missing := ExecuteView(request, `{"path":"/user/nope.md"}`); missing["status"] != "failed" {
		t.Fatalf("unknown file view = %+v", missing)
	}
}

func TestMemoryEditArchivesOldTextToMakeRoom(t *testing.T) {
	service := &fakeMemoryService{}
	// The file fills the user's whole memory.
	old := strings.Repeat("旧", agentmemory.MaxMemoryChars-12)
	request := editRequest(service, blockValue("profile", "", "坐长途航班喜欢靠过道。\n"+old))
	type archived struct{ content, key string }
	var kept []archived
	request.Archive = func(_ context.Context, content, sourceKey string) error {
		kept = append(kept, archived{content, sourceKey})
		return nil
	}
	full := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "我搬到杭州了。", "evidence": "我搬到杭州了", "mode": "explicit"}))
	if full.Error == nil || !strings.Contains(full.Result["edits"].([]EditOutcome)[0].Reason, "archive") {
		t.Fatalf("insert into a full file = %+v", full.Result)
	}
	result := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "archive", "path": "/user/profile.md", "old_text": old, "evidence": "", "mode": "explicit"},
		map[string]interface{}{"command": "insert", "path": "/user/profile.md", "text": "住在杭州。", "evidence": "我搬到杭州了", "mode": "explicit"},
		map[string]interface{}{"command": "archive", "path": "/user/profile.md", "old_text": "坐长途航班喜欢靠过道。\n住在杭州。", "evidence": "", "mode": "explicit"}))
	if result.Error != nil {
		t.Fatalf("archive then insert failed: %v (%+v)", result.Error, result.Result)
	}
	outcomes := result.Result["edits"].([]EditOutcome)
	if outcomes[0].Status != "accepted" || outcomes[1].Status != "accepted" || !strings.Contains(outcomes[2].Reason, "cannot empty") {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if len(kept) != 1 || kept[0].content != old || kept[0].key != "profile" {
		t.Fatalf("archived = %+v", kept)
	}
	if written := service.request.Operations[0]; written.Content != "坐长途航班喜欢靠过道。\n住在杭州。" || !strings.Contains(written.Reason, "archive") {
		t.Fatalf("written = %+v", written)
	}

	request.Archive = nil
	if rejected := ExecuteEdits(t.Context(), request, edits(t,
		map[string]interface{}{"command": "archive", "path": "/user/profile.md", "old_text": old, "evidence": "", "mode": "explicit"})); rejected.Error == nil {
		t.Fatal("archive was accepted without an archive")
	}
}
