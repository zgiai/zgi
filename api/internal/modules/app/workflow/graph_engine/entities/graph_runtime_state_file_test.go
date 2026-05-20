package entities

import (
	"testing"

	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestVariablePoolAdd_NormalizesWorkflowFileArray(t *testing.T) {
	vp := NewVariablePool()

	file := workflowfile.NewFile(
		"tenant-1",
		workflowfile.FileTypeImage,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID("tool-file-1"),
		workflowfile.WithFilename("cat.png"),
		workflowfile.WithExtension(".png"),
		workflowfile.WithMimeType("image/png"),
		workflowfile.WithSize(42),
		workflowfile.WithRemoteURL("https://provider.example/cat.png"),
		workflowfile.WithURL("https://signed.example/cat.png"),
	)

	vp.Add([]string{"imagegen", "files"}, []*workflowfile.File{file})

	variable := vp.Get([]string{"imagegen", "files"})
	if variable == nil {
		t.Fatalf("expected variable to be stored")
	}
	if variable.GetType() != shared.SegmentTypeArrayFile {
		t.Fatalf("variable.GetType() = %s, want %s", variable.GetType(), shared.SegmentTypeArrayFile)
	}

	files, ok := variable.ToObject().([]*File)
	if !ok {
		t.Fatalf("variable.ToObject() type = %T, want []*entities.File", variable.ToObject())
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0] == nil {
		t.Fatalf("expected normalized file to be non-nil")
	}
	if files[0].RemoteURL != "https://signed.example/cat.png" {
		t.Fatalf("files[0].RemoteURL = %q, want %q", files[0].RemoteURL, "https://signed.example/cat.png")
	}
	if files[0].WorkspaceID != "tenant-1" {
		t.Fatalf("files[0].WorkspaceID = %q, want %q", files[0].WorkspaceID, "tenant-1")
	}
	if files[0].Filename != "cat.png" {
		t.Fatalf("files[0].Filename = %q, want %q", files[0].Filename, "cat.png")
	}
	if files[0].MimeType != "image/png" {
		t.Fatalf("files[0].MimeType = %q, want %q", files[0].MimeType, "image/png")
	}
	if files[0].Type != "image" {
		t.Fatalf("files[0].Type = %q, want %q", files[0].Type, "image")
	}
}

func TestVariablePoolMapToFile_UsesURLFieldWhenRemoteURLMissing(t *testing.T) {
	vp := NewVariablePool()

	vp.Add([]string{"imagegen", "file"}, map[string]interface{}{
		"type":            "image",
		"transfer_method": "remote_url",
		"upload_file_id":  "file-1",
		"filename":        "cat.png",
		"mime_type":       "image/png",
		"url":             "https://signed.example/cat.png",
	})

	fileSegment := vp.GetFile([]string{"imagegen", "file"})
	if fileSegment == nil {
		t.Fatalf("expected file segment to be available")
	}
	if fileSegment.Value.RemoteURL != "https://signed.example/cat.png" {
		t.Fatalf("fileSegment.Value.RemoteURL = %q, want %q", fileSegment.Value.RemoteURL, "https://signed.example/cat.png")
	}
	if markdown := fileSegment.Markdown(); markdown != "![cat.png](https://signed.example/cat.png)" {
		t.Fatalf("fileSegment.Markdown() = %q, want %q", markdown, "![cat.png](https://signed.example/cat.png)")
	}
}

func TestVariablePoolMapToFile_NormalizesLegacyTenantIDToWorkspaceID(t *testing.T) {
	vp := NewVariablePool()

	vp.Add([]string{"imagegen", "file"}, map[string]interface{}{
		"type":            "image",
		"transfer_method": "remote_url",
		"upload_file_id":  "file-1",
		"tenant_id":       "ws-1",
		"url":             "https://signed.example/cat.png",
	})

	fileSegment := vp.GetFile([]string{"imagegen", "file"})
	if fileSegment == nil {
		t.Fatalf("expected file segment to be available")
	}
	if fileSegment.Value.WorkspaceID != "ws-1" {
		t.Fatalf("fileSegment.Value.WorkspaceID = %q, want %q", fileSegment.Value.WorkspaceID, "ws-1")
	}
}

func TestVariablePoolMapToFile_InfersConcreteTypeFromLegacyFileType(t *testing.T) {
	vp := NewVariablePool()

	vp.Add([]string{"start-node", "query"}, map[string]interface{}{
		"type":            "file",
		"transfer_method": "local_file",
		"upload_file_id":  "file-1",
		"filename":        "paper.jpg",
		"extension":       "jpg",
		"mime_type":       "image/jpeg",
	})

	fileSegment := vp.GetFile([]string{"start-node", "query"})
	if fileSegment == nil {
		t.Fatalf("expected file segment to be available")
	}
	if fileSegment.Value.Type != "image" {
		t.Fatalf("fileSegment.Value.Type = %q, want %q", fileSegment.Value.Type, "image")
	}

	typeVar := vp.GetWithPath([]string{"start-node", "query", "type"})
	if typeVar == nil {
		t.Fatalf("expected nested type selector to resolve")
	}
	if got := typeVar.ToObject(); got != "image" {
		t.Fatalf("type selector returned %#v, want %q", got, "image")
	}
}
