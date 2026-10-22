package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type exportPersisterStub struct{}

func (exportPersisterStub) Persist(_ context.Context, _ execution.Scope, _ string, filePath string, data []byte) (map[string]interface{}, error) {
	return map[string]interface{}{"file_id": uuid.NewString(), "filename": filePath, "mime_type": "application/pdf", "size": len(data)}, nil
}

func TestExportedFilesAreKeptAndCanBeArchivedByTheRun(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED", "true")
	run, _, _ := newBackgroundTestFile(t, SurfaceTask)
	service := artifactServiceForTest(t)
	actor := uuid.New()
	run.ActorID = &actor
	event := map[string]any{"task_owner_kind": "digital_human"}
	persister := withExportArtifacts(exportPersisterStub{}, service, *run, event)
	conversation := run.ConversationID.String()
	export := func(data string) map[string]interface{} {
		t.Helper()
		result, err := persister.Persist(t.Context(), execution.Scope{}, conversation, "out/deck.pdf", []byte(data))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first, second := export("v1"), export("v2")
	if first["kept_artifact_id"] == nil || first["kept_artifact_id"] != second["kept_artifact_id"] || second["kept_artifact_version"] != 2 {
		t.Fatalf("a file exported again in the conversation must version: %v %v", first, second)
	}

	preparation, err := NewArtifactRuntimeProvider(nil, service).Prepare(t.Context(), run, invocation.Event{Context: event})
	if err != nil || preparation == nil {
		t.Fatalf("preparation = %#v, %v", preparation, err)
	}
	call := func(name, args string) map[string]any {
		t.Helper()
		for _, tool := range preparation.Tools {
			if tool.Definition.Function.Name == name {
				result := tool.Handler(t.Context(), adapter.ToolCall{Function: adapter.FunctionCall{Name: name, Arguments: args}})
				if data, ok := result.Result["data"].(map[string]any); ok {
					return data
				}
				return result.Result
			}
		}
		t.Fatalf("tool %s is missing", name)
		return nil
	}
	if published := call(artifactPublishToolName, `{"title":"Deck","kind":"file","file_id":"`+uuid.NewString()+`"}`); published["status"] != "rejected" {
		t.Fatalf("publish of a foreign file = %v", published)
	}
	id := first["kept_artifact_id"].(string)
	if archived := call(artifactArchiveToolName, `{"artifact_id":"`+id+`"}`); archived["status"] != "archived" {
		t.Fatalf("archive = %v", archived)
	}
	if listed := call(artifactListToolName, `{}`); listed["total"] != int64(0) {
		t.Fatalf("an archived artifact is still listed: %v", listed)
	}
	if listed := call(artifactListToolName, `{"archived":"only"}`); listed["total"] != int64(1) {
		t.Fatalf("archived artifacts cannot be found: %v", listed)
	}
	if restored := call(artifactArchiveToolName, `{"artifact_id":"`+id+`","restore":true}`); restored["status"] != "restored" {
		t.Fatalf("restore = %v", restored)
	}

	stranger := *run
	other := uuid.New()
	stranger.ActorID = &other
	result, err := service.List(t.Context(), artifacts.Reader{Scope: artifactScope(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID),
		Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: other}, Member: true}, artifacts.ListQuery{})
	if err != nil || result.Total != 1 {
		t.Fatalf("a task center export is shared with the team: %d %v", result.Total, err)
	}
	if _, err := service.SetArchived(t.Context(), artifacts.Reader{Scope: artifactScope(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID),
		Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: other}, Member: true}, uuid.MustParse(id), true); err == nil {
		t.Fatal("a colleague archived another member's artifact")
	}

	t.Setenv("ZGI_DIGITAL_HUMAN_ARTIFACTS_ENABLED", "false")
	if off := export("v3"); off["kept_artifact_id"] != nil {
		t.Fatalf("an export was kept while artifacts are off: %v", off)
	}
}
