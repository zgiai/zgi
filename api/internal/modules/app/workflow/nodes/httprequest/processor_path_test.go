package httprequest

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestResolveFiles_UsesNestedSelector(t *testing.T) {
	vpool := entities.NewVariablePool()
	vpool.Add([]string{"start", "payload"}, map[string]any{
		"attachment": map[string]any{
			"type":            "image",
			"transfer_method": "remote_url",
			"upload_file_id":  "file-1",
			"id":              "file-1",
			"workspace_id":    "ws-1",
			"url":             "https://example.com/paper.jpg",
			"filename":        "paper.jpg",
			"extension":       "jpg",
			"mime_type":       "image/jpeg",
		},
	})

	processor := &HTTPRequestProcessor{variablePool: vpool}
	files, err := processor.resolveFiles([]string{"start", "payload", "attachment"})
	if err != nil {
		t.Fatalf("resolveFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected non-nil file")
	}
}
