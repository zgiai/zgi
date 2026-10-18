package service

import (
	"testing"
	"time"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func TestProtectedArtifactHistoryNeverGeneratesLegacyURLs(t *testing.T) {
	artifact := map[string]interface{}{"artifact_id": "tool_file:private", "file_id": "private", "tool_file_id": "private",
		"filename": "result.csv", "extension": ".csv", "transfer_method": "tool_file", "access_mode": "authenticated", "lifecycle": "persistent",
		"url": "https://stale.invalid/public", "download_url": "https://stale.invalid/public?download=1"}
	for _, stored := range []map[string]interface{}{persistentGeneratedArtifact(artifact), persistentConversationArtifact(artifact), recentGeneratedArtifactCandidate(artifact, "message")} {
		if stored["access_mode"] != "authenticated" {
			t.Fatalf("protection marker lost: %#v", stored)
		}
		hydrated := hydrateGeneratedFileURLWithLookup(stored, nil, false, time.Now())
		if hydrated["url"] != nil || hydrated["download_url"] != nil {
			t.Fatalf("protected history signed: %#v", hydrated)
		}
	}
	// The row upgrades legacy-shaped metadata even if its marker was absent.
	delete(artifact, "access_mode")
	rows := map[string]*tool_file.ToolFile{"private": {ExecutionIdentity: &execution.Identity{Version: 1}, Lifecycle: "persistent"}}
	hydrated := hydrateGeneratedFileURLWithLookup(artifact, rows, true, time.Now())
	if hydrated["access_mode"] != "authenticated" || hydrated["url"] != nil || hydrated["download_url"] != nil {
		t.Fatalf("row did not suppress signatures: %#v", hydrated)
	}
	event := hydrateStreamEventGeneratedFileURL(StreamEvent{EventType: streamEventSkillArtifactCreated, Payload: map[string]interface{}{"file_id": "private", "extension": ".csv", "access_mode": "authenticated"}})
	if event.Payload["url"] != nil {
		t.Fatal("live event generated public URL")
	}
}
