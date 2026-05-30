package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

func TestCodeCommandAndFileFlow(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	info, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/hello.txt",
		Content:   "hello sandbox",
	})
	if err != nil {
		t.Fatalf("expected upload, got %v", err)
	}
	if info.Size == 0 {
		t.Fatal("expected uploaded file to have content")
	}

	downloaded, err := service.DownloadFile(box.ID, "notes/hello.txt", "utf-8")
	if err != nil {
		t.Fatalf("expected download, got %v", err)
	}
	if downloaded.Content != "hello sandbox" {
		t.Fatalf("unexpected downloaded content: %q", downloaded.Content)
	}

	files, err := service.ListFiles(box.ID)
	if err != nil {
		t.Fatalf("expected file list, got %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected workspace files after upload")
	}

	commandResult, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	if err != nil {
		t.Fatalf("expected command run, got %v", err)
	}
	if commandResult.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", commandResult.ExitCode)
	}

	codeResult, err := service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('session-ok')",
	})
	if err != nil {
		t.Fatalf("expected code run, got %v", err)
	}
	if codeResult.Stdout != "session-ok\n" {
		t.Fatalf("unexpected stdout: %q stderr=%q exit=%d backend=%q", codeResult.Stdout, codeResult.Error, codeResult.ExitCode, codeResult.Backend)
	}
}

func TestUploadArchiveExtractsZipWithStripRoot(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	result, err := service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/weather",
		ArchiveBase64: zipBase64(t, map[string]string{
			"weather/SKILL.md":             "# Weather",
			"weather/references/schema.md": "schema",
			"weather/scripts/run.py":       "print('ok')",
		}),
		Format:    "zip",
		StripRoot: true,
	})
	if err != nil {
		t.Fatalf("expected archive upload, got %v", err)
	}
	if result.FileCount != 3 {
		t.Fatalf("expected 3 uploaded files, got %d", result.FileCount)
	}

	downloaded, err := service.DownloadFile(box.ID, "skills/weather/SKILL.md", "utf-8")
	if err != nil {
		t.Fatalf("expected uploaded SKILL.md, got %v", err)
	}
	if downloaded.Content != "# Weather" {
		t.Fatalf("unexpected skill content: %q", downloaded.Content)
	}
}

func TestUploadArchiveRejectsZipSlip(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID:     box.ID,
		Path:          ".",
		ArchiveBase64: zipBase64(t, map[string]string{"../escape.txt": "nope"}),
		Format:        "zip",
	})
	if err == nil {
		t.Fatal("expected zip slip archive to be rejected")
	}
}

func TestUploadArchiveRejectsSymlinkAndRollsBack(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "pkg/ok.txt",
		Content:   "previous",
	}); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID:     box.ID,
		Path:          "pkg",
		ArchiveBase64: zipBase64WithSymlink(t),
		Format:        "zip",
	})
	if err == nil {
		t.Fatal("expected symlink archive to be rejected")
	}
	downloaded, err := service.DownloadFile(box.ID, "pkg/ok.txt", "utf-8")
	if err != nil {
		t.Fatalf("expected existing file to be restored, got %v", err)
	}
	if downloaded.Content != "previous" {
		t.Fatalf("expected existing file to be restored, got %q", downloaded.Content)
	}
}

func TestRunCodeRejectsDeniedNetwork(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		NetworkEnabled:    false,
		NetworkPolicy:     "deny-by-default",
		DependencyProfile: "stdlib",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if _, err := service.RunCode(context.Background(), CodeRequest{
		SandboxID:     box.ID,
		Language:      "python3",
		Code:          "print('blocked')",
		EnableNetwork: true,
	}); err == nil {
		t.Fatal("expected network-enabled run to be rejected")
	}
}

func newTestExecutorService(t *testing.T) (*Service, *lifecycle.Manager) {
	t.Helper()

	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	return NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService), manager
}

func zipBase64(t *testing.T, files map[string]string) string {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func zipBase64WithSymlink(t *testing.T) string {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	okWriter, err := writer.Create("ok.txt")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := okWriter.Write([]byte("ok")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}

	header := &zip.FileHeader{Name: "link"}
	header.SetMode(0o777 | fs.ModeSymlink)
	linkWriter, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("create symlink zip entry: %v", err)
	}
	if _, err := linkWriter.Write([]byte("ok.txt")); err != nil {
		t.Fatalf("write symlink zip entry: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}
