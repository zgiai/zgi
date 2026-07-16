//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
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
}

func TestHydrateMessageGeneratedFileURLsRefreshesSignedURLs(t *testing.T) {
	restoreToolFileSignature(t)
	message := &aichatmodel.Message{Metadata: map[string]interface{}{
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
