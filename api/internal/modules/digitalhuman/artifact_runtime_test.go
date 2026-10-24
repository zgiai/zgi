package digitalhuman

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
)

func TestRunsKeepArtifactsForTheirRequester(t *testing.T) {
	account, endUser, actor := uuid.New(), uuid.New(), uuid.New()
	for name, tc := range map[string]struct {
		run        Run
		context    map[string]any
		owner      artifacts.OwnerKind
		visibility artifacts.Visibility
		member     bool
	}{
		"member chat":      {run: Run{UserSubjectKind: "account", UserSubjectID: &account}, owner: artifacts.OwnerAccount, visibility: artifacts.VisibilityOwner, member: true},
		"end user":         {run: Run{UserSubjectKind: "api_end_user", UserSubjectID: &endUser}, owner: artifacts.OwnerAPIEndUser, visibility: artifacts.VisibilityOwner},
		"task center task": {run: Run{Surface: SurfaceTask, ActorID: &actor}, context: map[string]any{"task_owner_kind": "digital_human"}, owner: artifacts.OwnerAccount, visibility: artifacts.VisibilityTeam, member: true},
		"channel task":     {run: Run{Surface: SurfaceTask}, context: map[string]any{"task_owner_kind": "conversation"}, owner: artifacts.OwnerWorkspace, visibility: artifacts.VisibilityTeam, member: true},
		"channel run":      {run: Run{Surface: SurfaceChannel}, owner: artifacts.OwnerWorkspace, visibility: artifacts.VisibilityTeam, member: true},
	} {
		t.Run(name, func(t *testing.T) {
			tc.run.WorkspaceID = uuid.New()
			owner, visibility, member, ok := runArtifactOwner(&tc.run, tc.context)
			if !ok || owner.Kind != tc.owner || visibility != tc.visibility || member != tc.member {
				t.Fatalf("owner = %v %v member=%v ok=%v", owner, visibility, member, ok)
			}
		})
	}
	if _, _, _, ok := runArtifactOwner(&Run{Surface: SurfaceConsole}, nil); ok {
		t.Fatal("a run without a requester kept artifacts")
	}
}

func artifactServiceForTest(t *testing.T) *artifacts.Service {
	t.Helper()
	_, repo := newTestService(t)
	if err := repo.db.AutoMigrate(&artifacts.Artifact{}, &artifacts.Version{}); err != nil {
		t.Fatal(err)
	}
	return artifacts.NewService(repo.db, ToolFileArtifactBlobs{})
}

func TestArtifactToolsPublishVersionsAndReadAcrossRuns(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED", "true")
	run, file, _ := newBackgroundTestFile(t, SurfaceTask)
	service := artifactServiceForTest(t)
	actor := uuid.New()
	run.ActorID = &actor
	taskID := uuid.New()
	event := invocation.Event{Context: map[string]any{"task_owner_kind": "digital_human", "task_id": taskID.String()}}
	preparation, err := NewArtifactRuntimeProvider(nil, service).Prepare(t.Context(), run, event)
	if err != nil || preparation == nil || len(preparation.Tools) != 4 {
		t.Fatalf("preparation = %#v, %v", preparation, err)
	}
	tools := map[string]func(string) map[string]any{}
	for _, tool := range preparation.Tools {
		handler := tool.Handler
		tools[tool.Definition.Function.Name] = func(args string) map[string]any {
			result := handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Name: tool.Definition.Function.Name, Arguments: args}})
			data, _ := result.Result["data"].(map[string]any)
			if data == nil {
				return result.Result
			}
			return data
		}
	}
	first := tools[artifactPublishToolName](`{"title":"Weekly report","kind":"document","content":"# Week 1"}`)
	second := tools[artifactPublishToolName](`{"title":"Weekly report","kind":"document","content":"# Week 2"}`)
	if first["artifact_id"] != second["artifact_id"] || second["version"] != 2 {
		t.Fatalf("publishes = %v, %v", first, second)
	}
	attached := tools[artifactPublishToolName](`{"title":"Result","kind":"file","file_id":"tool_file:` + file.ID + `"}`)
	if attached["status"] != "published" {
		t.Fatalf("file publish = %v", attached)
	}
	if foreign := tools[artifactPublishToolName](`{"title":"Other","kind":"file","file_id":"` + uuid.NewString() + `"}`); foreign["status"] != "rejected" {
		t.Fatalf("another file was published: %v", foreign)
	}

	// A later run of the same requester finds and reads the artifact.
	later := *run
	later.ID = uuid.New()
	preparation, _ = NewArtifactRuntimeProvider(nil, service).Prepare(t.Context(), &later, event)
	if !strings.Contains(preparation.SystemPromptAddendum, `"Weekly report" (document, v2`) {
		t.Fatalf("later run is not told about the artifact: %s", preparation.SystemPromptAddendum)
	}
	var listed, read map[string]any
	for _, tool := range preparation.Tools {
		call := adapter.ToolCall{Function: adapter.FunctionCall{Name: tool.Definition.Function.Name}}
		switch tool.Definition.Function.Name {
		case artifactListToolName:
			call.Function.Arguments = `{"query":"weekly"}`
			listed = tool.Handler(t.Context(), call).Result["data"].(map[string]any)
		case artifactReadToolName:
			args, _ := json.Marshal(map[string]any{"artifact_id": first["artifact_id"], "version": 1})
			call.Function.Arguments = string(args)
			read = tool.Handler(t.Context(), call).Result["data"].(map[string]any)
		}
	}
	if listed["total"] != int64(1) || read["content"] != "# Week 1" || read["current_version"] != 2 {
		t.Fatalf("listed = %v, read = %v", listed, read)
	}

	// An end user's run of the same DigitalHuman sees none of it.
	endUser := uuid.New()
	stranger := *run
	stranger.UserSubjectKind, stranger.UserSubjectID = "api_end_user", &endUser
	result, err := service.List(t.Context(), artifacts.Reader{Scope: artifactScope(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID),
		Principal: artifacts.Owner{Kind: artifacts.OwnerAPIEndUser, ID: endUser}}, artifacts.ListQuery{})
	if err != nil || result.Total != 0 {
		t.Fatalf("end user sees %d artifacts, %v", result.Total, err)
	}
}

func TestReportFilesAreKeptAsArtifacts(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED", "true")
	run, file, _ := newBackgroundTestFile(t, SurfaceTask)
	trace, _ := json.Marshal(map[string]any{"generated_files": []map[string]any{{"tool_file_id": file.ID}}})
	run.ExecutionTrace = datatypes.JSON(trace)
	runs := newFakeRunStore()
	runs.runs[run.ID] = run
	service := artifactServiceForTest(t)
	capture := NewArtifactReportCapture(&RuntimeService{store: runs}, service)
	requester, taskID := uuid.New(), uuid.New()
	notice := TaskReportNotice{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, DigitalHumanID: run.DigitalHumanID, TaskID: taskID,
		ReportID: uuid.New(), RunID: &run.ID, RecipientKind: "account", RecipientID: requester, Shared: true}
	for range 2 {
		if err := capture.NotifyTaskReport(t.Context(), notice); err != nil {
			t.Fatal(err)
		}
	}
	reader := artifacts.Reader{Scope: artifactScope(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID), Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: uuid.New()}, Member: true}
	result, err := service.List(t.Context(), reader, artifacts.ListQuery{TaskID: &taskID})
	if err != nil || result.Total != 1 || result.Items[0].Visibility != artifacts.VisibilityTeam || result.Items[0].CurrentVersion != 1 || result.Items[0].Title != file.Name {
		t.Fatalf("captured = %#v, %v", result, err)
	}
	_, _, data, err := service.Content(t.Context(), reader, result.Items[0].ID, 0)
	if err != nil || string(data) != "verified-result" {
		t.Fatalf("content = %q, %v", data, err)
	}
}

func TestFirstMessageOfANewConversationGetsArtifactTools(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED", "true")
	account := uuid.New()
	run := &Run{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New(), UserSubjectKind: "account", UserSubjectID: &account}
	preparation, err := NewArtifactRuntimeProvider(nil, artifactServiceForTest(t)).Prepare(t.Context(), run, invocation.Event{})
	if err != nil || preparation == nil || len(preparation.Tools) != 4 || preparation.BindConversation == nil {
		t.Fatalf("preparation = %#v, %v", preparation, err)
	}
	// Once the run has its conversation, its files can be published.
	tools := artifactTools{run: run}
	if tools.conversationID() != nil {
		t.Fatal("a run without a conversation has one")
	}
	conversation := uuid.Nil
	tools.conversation = &conversation
	bound := uuid.New()
	conversation = bound
	if got := tools.conversationID(); got == nil || *got != bound {
		t.Fatalf("bound conversation = %v", got)
	}
}
