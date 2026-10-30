package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRecentAssetCandidatesFromReadFileTrace(t *testing.T) {
	messageID := uuid.New()
	parts := &chatRequestParts{}
	applyRecentAssetCandidatesFromBranch(parts, []*runtimemodel.Message{{
		ID:     messageID,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillFileReader,
				"tool_name": "read_file",
				"status":    "success",
				"result": map[string]interface{}{
					"file": map[string]interface{}{
						"id":             "file-1",
						"name":           "invoice.xlsx",
						"workspace_id":   "workspace-1",
						"extension":      "xlsx",
						"content_status": "extracted",
					},
					"content_redacted": true,
				},
			}},
		},
	}})

	if len(parts.RecentAssetCandidates) != 1 {
		t.Fatalf("recent candidates = %#v, want one", parts.RecentAssetCandidates)
	}
	candidate := parts.RecentAssetCandidates[0]
	if candidate.ID != "file-1" || candidate.Name != "invoice.xlsx" || candidate.Extension != "xlsx" || !candidate.Recent {
		t.Fatalf("candidate = %#v, want recent invoice.xlsx", candidate)
	}
	if candidate.Metadata["recent_message_id"] != messageID.String() || candidate.Metadata["content_status"] != "extracted" {
		t.Fatalf("metadata = %#v, want recent message id and content status", candidate.Metadata)
	}
}

func TestRecentAssetCandidatesFromSaveFileToManagementTrace(t *testing.T) {
	messageID := uuid.New()
	parts := &chatRequestParts{}
	applyRecentAssetCandidatesFromBranch(parts, []*runtimemodel.Message{{
		ID:     messageID,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"kind":      "tool_call",
				"skill_id":  skills.SkillFileManager,
				"tool_name": "save_file_to_management",
				"status":    "success",
				"result": map[string]interface{}{
					"status":         "success",
					"target":         "managed_file",
					"upload_file_id": "managed-file-1",
					"filename":       "aichat-plan-smoke.md",
					"extension":      "md",
					"workspace_id":   "workspace-1",
				},
			}},
		},
	}})

	if len(parts.RecentAssetCandidates) != 1 {
		t.Fatalf("recent candidates = %#v, want one", parts.RecentAssetCandidates)
	}
	candidate := parts.RecentAssetCandidates[0]
	if candidate.ID != "managed-file-1" || candidate.Name != "aichat-plan-smoke.md" || candidate.Extension != "md" || !candidate.Recent {
		t.Fatalf("candidate = %#v, want recent managed file", candidate)
	}
	if candidate.Metadata["recent_tool_name"] != "save_file_to_management" || candidate.Metadata["recent_message_id"] != messageID.String() {
		t.Fatalf("metadata = %#v, want save_file_to_management provenance", candidate.Metadata)
	}
}
