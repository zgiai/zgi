package executor

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
)

func TestModelChannelBoundedDownload(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: "session"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		limit         int64
		valid         bool
	}{
		{"request.json", `{"sequence":1}`, 14, true},
		{"large.json", strings.Repeat("x", 20), 10, false},
		{"invalid-limit.json", "x", -1, false},
	} {
		if _, err := service.UploadFile(FileWriteRequest{SandboxID: box.ID, Path: tc.name, Content: tc.content}); err != nil {
			t.Fatal(err)
		}
		content, err := service.DownloadFileWithLimit(box.ID, tc.name, "utf-8", tc.limit)
		if tc.valid {
			if err != nil || content.Content != tc.content || content.MaxBytes != tc.limit {
				t.Fatalf("content=%v err=%v", content, err)
			}
		} else if err == nil {
			t.Fatal("unbounded download accepted")
		}
	}
	if err := syscall.Mkfifo(filepath.Join(box.RootPath, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DownloadFileWithLimit(box.ID, "pipe", "base64", 100); err == nil {
		t.Fatal("FIFO accepted")
	}
	if err := os.Symlink("request.json", filepath.Join(box.RootPath, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DownloadFileWithLimit(box.ID, "link", "base64", 100); err == nil {
		t.Fatal("symlink accepted")
	}
}
