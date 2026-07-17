package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	tool_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestMergeGeneratedArtifactMetadataPersistsHydratableFile(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).Unix()
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "file-json",
		"filename":        "evaluation-summary.json",
		"extension":       ".json",
		"mime_type":       "application/json",
		"size":            int64(365),
		"url":             "http://stale.example/preview",
		"download_url":    "http://stale.example/download",
		"transfer_method": "tool_file",
		"skill_id":        "sandbox-backtest-mcp-eval",
		"tool_name":       "run_script",
		"operation_id":    "tool_governance:corr-1",
		"correlation_id":  "corr-1",
		"lifecycle":       "temporary",
		"expires_at":      expiresAt,
		"asset_operation_audit": map[string]interface{}{
			"correlation_id":  "corr-1",
			"tool_id":         "file.generate_pdf",
			"approval_status": "approved",
		},
	})

	files := generatedFilesFromMetadata(metadata["generated_files"])
	if len(files) != 1 {
		t.Fatalf("generated_files = %#v, want one file", metadata["generated_files"])
	}
	if _, ok := files[0]["url"]; ok {
		t.Fatalf("stored generated file should not persist stale url: %#v", files[0])
	}
	if _, ok := files[0]["download_url"]; ok {
		t.Fatalf("stored generated file should not persist stale download_url: %#v", files[0])
	}
	if metadata["generated_file_count"] != 1 {
		t.Fatalf("generated_file_count = %#v, want 1", metadata["generated_file_count"])
	}
	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	if len(artifacts) != 1 {
		t.Fatalf("conversation_artifacts = %#v, want one artifact", metadata["conversation_artifacts"])
	}
	for key, want := range map[string]interface{}{
		"artifact_id":   "tool_file:file-json",
		"artifact_type": "file",
		"status":        conversationArtifactStatusAvailable,
		"lifecycle":     conversationArtifactLifecycleTemp,
		"tool_file_id":  "file-json",
		"filename":      "evaluation-summary.json",
	} {
		if artifacts[0][key] != want {
			t.Fatalf("conversation artifact %s = %#v, want %#v in %#v", key, artifacts[0][key], want, artifacts[0])
		}
	}
	if files[0]["operation_id"] != "tool_governance:corr-1" || files[0]["correlation_id"] != "corr-1" {
		t.Fatalf("stored generated file operation fields = %#v", files[0])
	}
	if files[0]["artifact_id"] != "tool_file:file-json" || files[0]["lifecycle"] != "temporary" || files[0]["expires_at"] != expiresAt {
		t.Fatalf("stored generated file lifecycle = %#v", files[0])
	}
	audit := governanceMapFromAny(files[0]["asset_operation_audit"])
	if audit["tool_id"] != "file.generate_pdf" || audit["approval_status"] != "approved" {
		t.Fatalf("stored generated file audit = %#v", files[0]["asset_operation_audit"])
	}
}

func TestMergeGeneratedArtifactMetadataPersistsManagedFileSignals(t *testing.T) {
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "upload-1",
		"upload_file_id":  "upload-1",
		"filename":        "managed-summary.pdf",
		"extension":       ".pdf",
		"mime_type":       "application/pdf",
		"size":            int64(2048),
		"target":          "managed_file",
		"transfer_method": "local_file",
		"workspace_id":    "workspace-1",
		"folder_id":       "folder-1",
		"url":             "http://files.example/console/api/files/upload-1",
		"download_url":    "/console/api/files/upload-1/download",
		"skill_id":        "file-generator",
		"tool_name":       "generate_pdf",
		"asset_operation_audit": map[string]interface{}{
			"tool_id":    "file.generate_pdf",
			"effect":     "create",
			"asset_type": "file",
		},
	})

	files := generatedFilesFromMetadata(metadata["generated_files"])
	if len(files) != 1 {
		t.Fatalf("generated_files = %#v, want one managed file", metadata["generated_files"])
	}
	file := files[0]
	for key, want := range map[string]interface{}{
		"file_id":         "upload-1",
		"upload_file_id":  "upload-1",
		"target":          "managed_file",
		"transfer_method": "local_file",
		"workspace_id":    "workspace-1",
		"folder_id":       "folder-1",
		"download_url":    "/console/api/files/upload-1/download",
	} {
		if file[key] != want {
			t.Fatalf("managed generated file %s = %#v, want %#v in %#v", key, file[key], want, file)
		}
	}
	audit := governanceMapFromAny(file["asset_operation_audit"])
	if audit["effect"] != "create" || audit["asset_type"] != "file" {
		t.Fatalf("managed generated file audit = %#v, want file create audit", file["asset_operation_audit"])
	}
	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	if len(artifacts) != 1 {
		t.Fatalf("conversation_artifacts = %#v, want one managed artifact", metadata["conversation_artifacts"])
	}
	for key, want := range map[string]interface{}{
		"artifact_id":    "managed_file:upload-1",
		"status":         conversationArtifactStatusSaved,
		"lifecycle":      conversationArtifactLifecycleManaged,
		"file_id":        "upload-1",
		"upload_file_id": "upload-1",
		"target":         "managed_file",
	} {
		if artifacts[0][key] != want {
			t.Fatalf("managed conversation artifact %s = %#v, want %#v in %#v", key, artifacts[0][key], want, artifacts[0])
		}
	}
}

func TestMergeGeneratedArtifactMetadataCarriesProjectionEvidenceIntoManagedFile(t *testing.T) {
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "tool-file-1",
		"tool_file_id":    "tool-file-1",
		"filename":        "chapter.md",
		"content_chars":   2048,
		"content_sha256":  "sha256:chapter",
		"content_summary": "Chapter summary",
	})
	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":             "managed-file-1",
		"upload_file_id":      "managed-file-1",
		"source_tool_file_id": "tool-file-1",
		"filename":            "chapter.md",
		"target":              "managed_file",
	})

	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	var managed map[string]interface{}
	for _, artifact := range artifacts {
		if stringFromAny(artifact["artifact_id"]) == "managed_file:managed-file-1" {
			managed = artifact
			break
		}
	}
	if managed == nil {
		t.Fatalf("managed artifact missing from %#v", artifacts)
	}
	for key, want := range map[string]interface{}{
		"source_tool_file_id": "tool-file-1",
		"content_chars":       2048,
		"content_sha256":      "sha256:chapter",
		"content_summary":     "Chapter summary",
	} {
		if managed[key] != want {
			t.Fatalf("managed artifact %s = %#v, want %#v in %#v", key, managed[key], want, managed)
		}
	}
}

func TestHydrateMessageGeneratedFileURLsAddsManagedFileURL(t *testing.T) {
	restoreToolFileSignature(t)
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id":         "upload-1",
			"upload_file_id":  "upload-1",
			"filename":        "chart.svg",
			"extension":       ".svg",
			"mime_type":       "image/svg+xml",
			"size":            int64(512),
			"target":          "managed_file",
			"transfer_method": "local_file",
		}},
	}}

	hydrateMessageGeneratedFileURLs(message)

	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) != 1 {
		t.Fatalf("generated_files = %#v, want one file", message.Metadata["generated_files"])
	}
	url := stringFromAny(files[0]["url"])
	downloadURL := stringFromAny(files[0]["download_url"])
	if !strings.HasPrefix(url, "http://files.example/console/api/files/upload-1/file-preview?") {
		t.Fatalf("url = %q, want signed managed file preview url", url)
	}
	if !strings.HasPrefix(downloadURL, url+"&as_attachment=true") {
		t.Fatalf("download_url = %q, want preview url plus as_attachment=true", downloadURL)
	}
}

func TestHydrateMessageGeneratedFileURLsRefreshesManagedFileURL(t *testing.T) {
	restoreToolFileSignature(t)
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id":         "upload-1",
			"upload_file_id":  "upload-1",
			"filename":        "chart.svg",
			"extension":       ".svg",
			"mime_type":       "image/svg+xml",
			"target":          "managed_file",
			"transfer_method": "local_file",
			"url":             "http://stale.example/expired-preview",
			"download_url":    "http://stale.example/download",
		}},
	}}

	hydrateMessageGeneratedFileURLs(message)

	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) != 1 {
		t.Fatalf("generated_files = %#v, want one file", message.Metadata["generated_files"])
	}
	url := stringFromAny(files[0]["url"])
	downloadURL := stringFromAny(files[0]["download_url"])
	if !strings.HasPrefix(url, "http://files.example/console/api/files/upload-1/file-preview?") {
		t.Fatalf("url = %q, want refreshed signed managed file preview url", url)
	}
	if strings.Contains(url, "stale.example") {
		t.Fatalf("url = %q, should refresh stale managed file preview url", url)
	}
	if !strings.HasPrefix(downloadURL, url+"&as_attachment=true") {
		t.Fatalf("download_url = %q, want refreshed attachment url", downloadURL)
	}
}

func TestRecentGeneratedArtifactsFromBranchKeepsTemporaryToolFiles(t *testing.T) {
	messageID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	branch := []*runtimemodel.Message{{
		ID:     messageID,
		Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"generated_files": []interface{}{
				map[string]interface{}{
					"file_id":         "managed-1",
					"upload_file_id":  "managed-1",
					"filename":        "already-managed.pdf",
					"target":          "managed_file",
					"transfer_method": "local_file",
				},
				map[string]interface{}{
					"file_id":         "tool-1",
					"filename":        "monthly-sales-bar.svg",
					"extension":       ".svg",
					"mime_type":       "image/svg+xml",
					"transfer_method": "tool_file",
					"skill_id":        "chart-generator",
					"tool_name":       "generate_chart",
				},
			},
		},
	}}

	artifacts := recentGeneratedArtifactsFromBranch(branch)
	if len(artifacts) != 1 {
		t.Fatalf("recent generated artifacts = %#v, want one temporary tool file", artifacts)
	}
	artifact := artifacts[0]
	if artifact["tool_file_id"] != "tool-1" || artifact["filename"] != "monthly-sales-bar.svg" {
		t.Fatalf("recent generated artifact = %#v, want tool-1 monthly-sales-bar.svg", artifact)
	}
	if artifact["source_message_id"] != messageID.String() {
		t.Fatalf("source_message_id = %#v, want %s", artifact["source_message_id"], messageID.String())
	}
}

func TestConversationArtifactsKeepTemporaryAvailableAfterManagementCopy(t *testing.T) {
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "tool-1",
		"tool_file_id":    "tool-1",
		"filename":        "chart.svg",
		"extension":       ".svg",
		"mime_type":       "image/svg+xml",
		"transfer_method": "tool_file",
		"target":          "temporary_artifact",
	})
	metadata = mergeGeneratedArtifactMetadata(metadata, map[string]interface{}{
		"file_id":         "managed-1",
		"upload_file_id":  "managed-1",
		"source_file_id":  "tool-1",
		"filename":        "chart.svg",
		"extension":       ".svg",
		"mime_type":       "image/svg+xml",
		"transfer_method": "local_file",
		"target":          "managed_file",
	})

	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	if len(artifacts) != 2 {
		t.Fatalf("conversation_artifacts = %#v, want temp and managed artifacts", artifacts)
	}
	temp := artifacts[0]
	managed := artifacts[1]
	if temp["artifact_id"] != "tool_file:tool-1" || temp["status"] != conversationArtifactStatusAvailable || temp["managed_file_id"] != nil {
		t.Fatalf("temporary artifact = %#v, want independently available source", temp)
	}
	if managed["artifact_id"] != "managed_file:managed-1" || managed["source_tool_file_id"] != "tool-1" {
		t.Fatalf("managed artifact = %#v, want source tool_file link", managed)
	}

	branch := []*runtimemodel.Message{{
		ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Status:   runtimemodel.MessageStatusCompleted,
		Metadata: metadata,
	}}
	if recent := recentGeneratedArtifactsFromBranch(branch); len(recent) != 1 || recent[0]["tool_file_id"] != "tool-1" {
		t.Fatalf("recent generated artifacts = %#v, want retained tool-1 source", recent)
	}
}

func TestMergeSkillTraceMetadataLinksManagedFileToTemporaryArtifact(t *testing.T) {
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "tool-1",
		"tool_file_id":    "tool-1",
		"filename":        "chapter.md",
		"mime_type":       "text/markdown",
		"target":          "temporary_artifact",
		"content_chars":   4096,
		"content_sha256":  "sha256:chapter",
		"content_summary": "Chapter summary",
	})

	metadata = mergeSkillTraceMetadata(metadata, []skills.SkillTrace{{
		Kind:     "tool_call",
		SkillID:  skills.SkillFileManager,
		ToolName: "save_file_to_management",
		Status:   "success",
		Arguments: map[string]interface{}{
			"source_type":  "tool_file",
			"tool_file_id": "tool-1",
			"filename":     "chapter.md",
		},
		Result: map[string]interface{}{
			"file_id":        "managed-1",
			"upload_file_id": "managed-1",
			"filename":       "chapter.md",
			"target":         "managed_file",
		},
	}})

	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	if len(artifacts) != 2 {
		t.Fatalf("conversation_artifacts = %#v, want temporary and managed artifacts", artifacts)
	}
	managed := artifacts[1]
	if got := stringFromAny(managed["artifact_id"]); got != "managed_file:managed-1" {
		t.Fatalf("managed artifact_id = %q, want managed_file:managed-1", got)
	}
	if got := stringFromAny(managed["source_tool_file_id"]); got != "tool-1" {
		t.Fatalf("managed source_tool_file_id = %q, want tool-1", got)
	}
	if got := stringFromAny(managed["content_sha256"]); got != "sha256:chapter" {
		t.Fatalf("managed content_sha256 = %q, want inherited digest", got)
	}
	if got := stringFromAny(managed["content_summary"]); got != "Chapter summary" {
		t.Fatalf("managed content_summary = %q, want inherited summary", got)
	}
	if got := intValueFromAny(managed["content_chars"]); got != 4096 {
		t.Fatalf("managed content_chars = %d, want 4096", got)
	}
}

func TestRecentGeneratedArtifactsKeepsOlderTempAfterLaterManagedCopy(t *testing.T) {
	generatedMetadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "tool-1",
		"tool_file_id":    "tool-1",
		"filename":        "chart.svg",
		"extension":       ".svg",
		"mime_type":       "image/svg+xml",
		"transfer_method": "tool_file",
		"target":          "temporary_artifact",
	})
	savedMetadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id":         "managed-1",
		"upload_file_id":  "managed-1",
		"source_file_id":  "tool-1",
		"filename":        "chart.svg",
		"extension":       ".svg",
		"mime_type":       "image/svg+xml",
		"transfer_method": "local_file",
		"target":          "managed_file",
	})
	branch := []*runtimemodel.Message{
		{
			ID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			Status:   runtimemodel.MessageStatusCompleted,
			Metadata: generatedMetadata,
		},
		{
			ID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			Status:   runtimemodel.MessageStatusCompleted,
			Metadata: savedMetadata,
		},
	}

	if recent := recentGeneratedArtifactsFromBranch(branch); len(recent) != 1 || recent[0]["tool_file_id"] != "tool-1" {
		t.Fatalf("recent generated artifacts = %#v, want retained tool-1 source", recent)
	}
}

func TestRecentGeneratedArtifactsSkipsExpiredTemporaryFile(t *testing.T) {
	metadata := mergeGeneratedArtifactMetadata(map[string]interface{}{}, map[string]interface{}{
		"file_id": "tool-expired", "tool_file_id": "tool-expired", "filename": "expired.txt",
		"extension": ".txt", "mime_type": "text/plain", "transfer_method": "tool_file",
		"target": "temporary_artifact", "lifecycle": "temporary", "expires_at": time.Now().Add(-time.Minute).Unix(),
	})
	branch := []*runtimemodel.Message{{
		ID: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Status: runtimemodel.MessageStatusCompleted, Metadata: metadata,
	}}
	if recent := recentGeneratedArtifactsFromBranch(branch); len(recent) != 0 {
		t.Fatalf("recent generated artifacts = %#v, want expired source excluded", recent)
	}
}

func TestHydrateGeneratedFileURLsMarksExpiredWithoutSigning(t *testing.T) {
	restoreToolFileSignature(t)
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id": "tool-expired", "filename": "expired.txt", "extension": ".txt",
			"mime_type": "text/plain", "transfer_method": "tool_file",
			"lifecycle": "temporary", "expires_at": time.Now().Add(-time.Minute).Unix(),
		}},
	}}

	hydrateMessageGeneratedFileURLs(message)
	file := generatedFilesFromMetadata(message.Metadata["generated_files"])[0]
	if file["availability"] != conversationArtifactAvailabilityGone || file["url"] != nil || file["download_url"] != nil {
		t.Fatalf("hydrated expired file = %#v", file)
	}
}

func TestHydrateGeneratedFileURLsBackfillsLegacyLifecycle(t *testing.T) {
	restoreToolFileSignature(t)
	expiresAt := time.Now().Add(time.Hour)
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id": "tool-legacy", "filename": "legacy.txt", "extension": ".txt",
			"mime_type": "text/plain", "transfer_method": "tool_file",
		}},
	}}
	hydrateMessageGeneratedFileURLsWithLookup(message, map[string]*tool_file.ToolFile{
		"tool-legacy": {ID: "tool-legacy", Lifecycle: string(tool_file.ToolFileLifecycleTemporary), ExpiresAt: &expiresAt},
	}, true, time.Now())

	file := generatedFilesFromMetadata(message.Metadata["generated_files"])[0]
	if file["lifecycle"] != "temporary" || file["expires_at"] != expiresAt.Unix() || file["availability"] != conversationArtifactAvailabilityLive {
		t.Fatalf("hydrated legacy file = %#v", file)
	}
}

func TestHydrateGeneratedFileURLsMarksMissingLegacyToolFileExpired(t *testing.T) {
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id": "tool-cleaned", "filename": "cleaned.txt", "extension": ".txt",
			"mime_type": "text/plain", "transfer_method": "tool_file",
		}},
	}}
	hydrateMessageGeneratedFileURLsWithLookup(message, map[string]*tool_file.ToolFile{}, true, time.Now())

	file := generatedFilesFromMetadata(message.Metadata["generated_files"])[0]
	if file["availability"] != conversationArtifactAvailabilityGone || file["url"] != nil || file["download_url"] != nil {
		t.Fatalf("hydrated missing legacy file = %#v", file)
	}
}

func TestHydrateMessageGeneratedFileURLsRefreshesSignedURLs(t *testing.T) {
	restoreToolFileSignature(t)
	message := &runtimemodel.Message{Metadata: map[string]interface{}{
		"generated_files": []interface{}{map[string]interface{}{
			"file_id":      "file-json",
			"filename":     "evaluation-summary.json",
			"extension":    ".json",
			"mime_type":    "application/json",
			"size":         int64(365),
			"url":          "http://stale.example/preview",
			"download_url": "http://stale.example/download",
		}},
	}}

	hydrateMessageGeneratedFileURLs(message)

	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) != 1 {
		t.Fatalf("generated_files = %#v, want one file", message.Metadata["generated_files"])
	}
	url := stringFromAny(files[0]["url"])
	downloadURL := stringFromAny(files[0]["download_url"])
	if !strings.HasPrefix(url, "http://files.example/console/api/files/tools/file-json.json?") {
		t.Fatalf("url = %q, want refreshed signed tool file url", url)
	}
	if strings.Contains(url, "download=1") {
		t.Fatalf("url = %q, should not force download", url)
	}
	if !strings.HasPrefix(downloadURL, url+"&download=1") {
		t.Fatalf("download_url = %q, want preview url plus download=1", downloadURL)
	}
}

func restoreToolFileSignature(t *testing.T) {
	t.Helper()
	previous := tool_file.GlobalFileSignature
	previousConfig := config.GlobalConfig
	cfg := &config.Config{App: config.AppConfig{
		SecretKey:          "test-secret",
		FilesURL:           "http://files.example",
		FilesAccessTimeout: 3600,
	}}
	config.GlobalConfig = cfg
	tool_file.InitFileSignature(cfg)
	t.Cleanup(func() {
		tool_file.GlobalFileSignature = previous
		config.GlobalConfig = previousConfig
	})
}
