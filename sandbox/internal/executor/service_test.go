package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestFileOperationsRejectSymlinkPaths(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(box.RootPath, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := service.DownloadFile(box.ID, "link.txt", "utf-8"); err == nil {
		t.Fatal("expected download through symlink to be rejected")
	}
	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "link.txt",
		Content:   "changed",
	}); err == nil {
		t.Fatal("expected upload through symlink to be rejected")
	}
	content, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if string(content) != "outside" {
		t.Fatalf("expected outside file to remain unchanged, got %q", content)
	}

	if _, err := service.StatFile(box.ID, "link.txt"); err == nil {
		t.Fatal("expected stat through symlink to be rejected")
	}
}

func TestRunCommandRejectsSymlinkWorkingSubpath(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if err := os.Symlink(t.TempDir(), filepath.Join(box.RootPath, "linked-workdir")); err != nil {
		t.Fatalf("create symlink workdir: %v", err)
	}
	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID:      box.ID,
		Command:        "pwd",
		WorkingSubpath: "linked-workdir",
	}); err == nil {
		t.Fatal("expected command working_subpath symlink to be rejected")
	}
}

func TestRunCommandSupportsStdinEnvAndOutputLimits(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	result, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID:     box.ID,
		Command:       "python3",
		Args:          []string{"-c", "import os,sys; data=sys.stdin.read(); print(os.environ['ZGI_TEST_TOKEN'] + ':' + data); print('x'*64)"},
		Stdin:         "payload",
		Env:           map[string]string{"ZGI_TEST_TOKEN": "ok"},
		Profile:       "code-short",
		StdoutLimitKB: 1,
	})
	if err != nil {
		t.Fatalf("expected command run, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%q", result.ExitCode, result.Error)
	}
	if !strings.Contains(result.Stdout, "ok:payload") {
		t.Fatalf("expected stdin/env in stdout, got %q", result.Stdout)
	}
}

func TestRunCodeSupportsInputJSONAndStructuredResult(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	result, err := service.RunCode(context.Background(), CodeRequest{
		SandboxID:        box.ID,
		Language:         "python3",
		Code:             "import json,sys\npayload=json.loads(sys.stdin.read())\nprint(json.dumps({'echo': payload['input'], 'ok': True}))",
		InputJSON:        []byte(`{"input":"hello"}`),
		Profile:          "code-short",
		StrictResultJSON: true,
	})
	if err != nil {
		t.Fatalf("expected code run, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%q", result.ExitCode, result.Error)
	}
	data, ok := result.ResultJSON.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result_json object, got %#v", result.ResultJSON)
	}
	if data["echo"] != "hello" || data["ok"] != true {
		t.Fatalf("unexpected result_json: %#v", data)
	}
}

func TestRunCodeWithoutSandboxUsesStatelessWorkspace(t *testing.T) {
	service, _ := newTestExecutorService(t)

	first, err := service.RunCode(context.Background(), CodeRequest{
		Language:         "python3",
		Code:             "import json, pathlib\npathlib.Path('marker.txt').write_text('leftover')\nprint(json.dumps({'created': True}))",
		Profile:          "code-short",
		StrictResultJSON: true,
	})
	if err != nil {
		t.Fatalf("expected first stateless code run, got %v", err)
	}
	if first.ExitCode != 0 {
		t.Fatalf("expected first run to succeed, got exit=%d stderr=%q", first.ExitCode, first.Error)
	}

	second, err := service.RunCode(context.Background(), CodeRequest{
		Language:         "python3",
		Code:             "import json, pathlib\nprint(json.dumps({'exists': pathlib.Path('marker.txt').exists()}))",
		Profile:          "code-short",
		StrictResultJSON: true,
	})
	if err != nil {
		t.Fatalf("expected second stateless code run, got %v", err)
	}
	data, ok := second.ResultJSON.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result_json object, got %#v", second.ResultJSON)
	}
	if data["exists"] != false {
		t.Fatalf("expected stateless workspace cleanup between runs, got %#v", data)
	}
}

func TestRunCodeWithoutSandboxRejectsNetwork(t *testing.T) {
	service, _ := newTestExecutorService(t)

	_, err := service.RunCode(context.Background(), CodeRequest{
		Language:      "python3",
		Code:          "print('nope')",
		Profile:       "code-short",
		EnableNetwork: true,
	})
	if err == nil {
		t.Fatal("expected stateless network request to be rejected")
	}
}

func TestRunCodeStrictResultJSONRejectsPlainText(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:        box.ID,
		Language:         "python3",
		Code:             "print('plain text')",
		Profile:          "code-short",
		StrictResultJSON: true,
	})
	if err == nil {
		t.Fatal("expected strict result JSON failure")
	}
	if !strings.Contains(err.Error(), "strict_result_json") {
		t.Fatalf("expected strict_result_json error, got %v", err)
	}
}

func TestRunCodeRejectsOversizedInputJSON(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('nope')",
		Profile:   "code-short",
		InputJSON: []byte(strings.Repeat("x", 64*1024+1)),
	})
	if err == nil {
		t.Fatal("expected oversized input_json to be rejected")
	}
}

func TestRunCommandRejectsDangerousEnv(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "python3",
		Args:      []string{"-c", "print('nope')"},
		Env:       map[string]string{"LD_PRELOAD": "x"},
	}); err == nil {
		t.Fatal("expected dangerous env to be rejected")
	}
}

func TestBuildFileManifestReturnsHashesAndContentTypes(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "artifacts/report.txt",
		Content:   "hello manifest\n",
	}); err != nil {
		t.Fatalf("upload artifact: %v", err)
	}

	manifest, err := service.BuildFileManifest(box.ID, "artifacts")
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if manifest.FileCount != 1 || manifest.TotalSize != int64(len("hello manifest\n")) {
		t.Fatalf("unexpected manifest totals: %+v", manifest)
	}
	item := manifest.Items[0]
	if item.Path != "artifacts/report.txt" {
		t.Fatalf("unexpected manifest path: %s", item.Path)
	}
	if item.SHA256 == "" {
		t.Fatal("expected sha256")
	}
	if !strings.HasPrefix(item.ContentType, "text/plain") {
		t.Fatalf("expected text content type, got %s", item.ContentType)
	}
}

func TestDeleteFileRejectsSandboxRoot(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if err := service.DeleteFile(box.ID, "."); err == nil {
		t.Fatal("expected sandbox root delete to be rejected")
	}
	if _, err := os.Stat(box.RootPath); err != nil {
		t.Fatalf("expected sandbox root to remain, got %v", err)
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
