package llm

import (
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

func TestNodeFetchFiles_UsesVariablePoolFileArray(t *testing.T) {
	vpool := entities.NewVariablePool()
	testURL := "https://example.com/paper.jpg"
	vpool.Add([]string{"start-node", "attachments"}, []*file.File{
		file.NewFile(
			"tenant-1",
			file.FileTypeImage,
			file.FileTransferMethodRemoteURL,
			file.WithRemoteURL(testURL),
			file.WithMimeType("image/jpeg"),
			file.WithFilename("paper.jpg"),
		),
	})

	node := &Node{}
	files, err := node.fetchFiles(vpool, []string{"start-node", "attachments"})
	if err != nil {
		t.Fatalf("fetchFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected non-nil file")
	}
	if files[0].RemoteURL == nil || *files[0].RemoteURL != testURL {
		t.Fatalf("files[0].RemoteURL = %v, want %q", files[0].RemoteURL, testURL)
	}
}

func TestNodeFetchFiles_EmptySelectorReturnsEmptySlice(t *testing.T) {
	node := &Node{}
	files, err := node.fetchFiles(entities.NewVariablePool(), nil)
	if err != nil {
		t.Fatalf("fetchFiles() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("len(files) = %d, want 0", len(files))
	}
}

func TestNodeFetchFiles_MissingVariableReturnsEmptySlice(t *testing.T) {
	node := &Node{}
	files, err := node.fetchFiles(entities.NewVariablePool(), []string{"start-node", "missing"})
	if err != nil {
		t.Fatalf("fetchFiles() error = %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("len(files) = %d, want 0", len(files))
	}
}
