package agents

import (
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"github.com/zgiai/zgi/api/internal/dto"
	"testing"
)

func TestFileUploadPolicyDraftSnapshotAndLegacySave(t *testing.T) {
	cfg := &AgentsConfig{}
	selection := &chatfile.Policy{AllowedFileTypes: []string{"document"}}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{FileUpload: true, FileUploadPolicy: selection}); err != nil {
		t.Fatal(err)
	}
	selection.AllowedFileTypes[0] = "image"
	snapshot := agentConfigSnapshot("agent", cfg)
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	loaded := agentConfigResponseFromSnapshot("agent", decoded)
	if loaded.FileUploadPolicy == nil || loaded.FileUploadPolicy.AllowedFileTypes[0] != "document" {
		t.Fatalf("snapshot=%+v", loaded.FileUploadPolicy)
	}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{FileUpload: true}); err != nil {
		t.Fatal(err)
	}
	if got := agentConfigResponse("agent", cfg).FileUploadPolicy; got == nil || got.AllowedFileTypes[0] != "document" {
		t.Fatalf("legacy save broadened=%+v", got)
	}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{FileUploadPolicy: &chatfile.Policy{AllowedFileTypes: []string{"custom"}}}); err == nil {
		t.Fatal("custom allowed")
	}
	if got := agentConfigResponseFromSnapshot("agent", map[string]interface{}{}).FileUploadPolicy; got != nil {
		t.Fatal("legacy snapshot changed")
	}
}
