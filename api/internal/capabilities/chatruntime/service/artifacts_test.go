package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	tool_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func TestMergeGeneratedArtifactMetadataPersistsHydratableFile(t *testing.T) {
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
	if files[0]["operation_id"] != "tool_governance:corr-1" || files[0]["correlation_id"] != "corr-1" {
		t.Fatalf("stored generated file operation fields = %#v", files[0])
	}
	audit := governanceMapFromAny(files[0]["asset_operation_audit"])
	if audit["tool_id"] != "file.generate_pdf" || audit["approval_status"] != "approved" {
		t.Fatalf("stored generated file audit = %#v", files[0]["asset_operation_audit"])
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
	tool_file.InitFileSignature(&config.Config{App: config.AppConfig{
		SecretKey:          "test-secret",
		FilesURL:           "http://files.example",
		FilesAccessTimeout: 3600,
	}})
	t.Cleanup(func() {
		tool_file.GlobalFileSignature = previous
	})
}
