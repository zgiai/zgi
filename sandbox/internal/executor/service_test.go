package executor

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
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
		OrganizationID: "organization-exec",
		WorkspaceID:    "workspace-exec",
		WorkflowRunID:  "run-exec",
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

func TestExecutionEventsIncludeRequestID(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-exec",
		WorkspaceID:    "workspace-exec",
		WorkflowRunID:  "run-exec",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	ctx := observer.ContextWithRequestID(context.Background(), "req_exec_test")
	if _, err := service.RunCommand(ctx, CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	}); err != nil {
		t.Fatalf("expected command run, got %v", err)
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected command event, got %d", len(events))
	}
	if events[0].Metadata["request_id"] != "req_exec_test" {
		t.Fatalf("expected request ID metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["status"] != "success" {
		t.Fatalf("expected success status, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-exec" || events[0].Metadata["workspace_id"] != "workspace-exec" || events[0].Metadata["workflow_run_id"] != "run-exec" {
		t.Fatalf("expected ownership metadata, got %#v", events[0].Metadata)
	}
}

func TestExecutionFailureEventsAvoidSensitivePayloads(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-failed",
		WorkspaceID:    "workspace-failed",
		WorkflowRunID:  "run-failed",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	ctx := observer.ContextWithRequestID(context.Background(), "req_failed_test")
	_, err = service.RunCommand(ctx, CommandRequest{
		SandboxID: box.ID,
		Command:   "python3",
		Args:      []string{"-c", "print('secret')"},
		Env:       map[string]string{"LD_PRELOAD": "secret"},
		Stdin:     "secret-stdin",
		Profile:   "code-short",
	})
	if err == nil {
		t.Fatal("expected command failure")
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected command failure event, got %d", len(events))
	}
	metadata := events[0].Metadata
	if metadata["request_id"] != "req_failed_test" {
		t.Fatalf("expected request ID metadata, got %#v", metadata)
	}
	if metadata["status"] != "failure" || metadata["error_type"] != "validation_error" {
		t.Fatalf("expected structured failure metadata, got %#v", metadata)
	}
	if metadata["organization_id"] != "organization-failed" || metadata["workspace_id"] != "workspace-failed" || metadata["workflow_run_id"] != "run-failed" {
		t.Fatalf("expected ownership metadata, got %#v", metadata)
	}
	if _, ok := metadata["env"]; ok {
		t.Fatalf("expected env to be omitted, got %#v", metadata)
	}
	if _, ok := metadata["stdin"]; ok {
		t.Fatalf("expected stdin to be omitted, got %#v", metadata)
	}
	if _, ok := metadata["args"]; ok {
		t.Fatalf("expected args to be omitted, got %#v", metadata)
	}
}

func TestRunTemplateRendersWithBoundedHelpers(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	result, err := service.RunTemplate(context.Background(), TemplateRequest{
		Template: "Hello {{ upper .name }}",
		Variables: map[string]any{
			"name": "zgi",
		},
	})
	if err != nil {
		t.Fatalf("expected template render, got %v", err)
	}
	if result.Content != "Hello ZGI" || result.Truncated {
		t.Fatalf("unexpected template result: %+v", result)
	}

	events := recorder.Query(observer.Query{Type: "exec.template", Limit: 1})
	if len(events) != 1 || events[0].Metadata["status"] != "success" {
		t.Fatalf("expected template success event, got %#v", events)
	}
}

func TestRunTemplateEnforcesOutputAndVariableLimits(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	result, err := service.RunTemplate(context.Background(), TemplateRequest{
		Template:      "{{ .value }}",
		Variables:     map[string]any{"value": strings.Repeat("x", 32)},
		OutputLimitKB: 1,
	})
	if err != nil {
		t.Fatalf("expected output-limited render, got %v", err)
	}
	if result.Truncated {
		t.Fatalf("did not expect truncation below limit, got %+v", result)
	}

	result, err = service.RunTemplate(context.Background(), TemplateRequest{
		Template:      "{{ .value }}",
		Variables:     map[string]any{"value": strings.Repeat("x", 2048)},
		OutputLimitKB: 1,
	})
	if err != nil {
		t.Fatalf("expected truncated render, got %v", err)
	}
	if !result.Truncated || len(result.Content) != 1024 {
		t.Fatalf("expected truncated 1 KiB result, got len=%d result=%+v", len(result.Content), result)
	}

	_, err = service.RunTemplate(context.Background(), TemplateRequest{
		Template:  "{{ .value }}",
		Variables: map[string]any{"value": strings.Repeat("x", 17*1024)},
	})
	if err == nil || !strings.Contains(err.Error(), "template variable string exceeds") {
		t.Fatalf("expected variable string limit, got %v", err)
	}
}

func TestRunTemplateRejectsMissingVariablesAndUnknownFunctions(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	if _, err := service.RunTemplate(context.Background(), TemplateRequest{Template: "{{ .missing }}"}); err == nil {
		t.Fatal("expected missing variable to be rejected")
	}
	if _, err := service.RunTemplate(context.Background(), TemplateRequest{Template: "{{ env \"HOME\" }}"}); err == nil {
		t.Fatal("expected unknown function to be rejected")
	}
	if _, err := service.RunTemplate(context.Background(), TemplateRequest{Template: "{{ printf \"%s\" .value }}", Variables: map[string]any{"value": "x"}}); err == nil {
		t.Fatal("expected built-in helper outside allowlist to be rejected")
	}
}

func TestFileEventsIncludeOwnershipMetadata(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-files",
		WorkspaceID:    "workspace-files",
		AppID:          "app-files",
		WorkflowRunID:  "run-files",
		UserID:         "user-files",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/hello.txt",
		Content:   "hello",
	}); err != nil {
		t.Fatalf("expected upload, got %v", err)
	}
	if _, err := service.DownloadFile(box.ID, "notes/hello.txt", "utf-8"); err != nil {
		t.Fatalf("expected download, got %v", err)
	}
	if _, err := service.BuildFileManifest(box.ID, "notes"); err != nil {
		t.Fatalf("expected manifest, got %v", err)
	}
	if err := service.DeleteFile(box.ID, "notes/hello.txt"); err != nil {
		t.Fatalf("expected delete, got %v", err)
	}

	for _, eventType := range []string{"files.upload", "files.download", "files.manifest", "files.delete"} {
		events := recorder.Query(observer.Query{SandboxID: box.ID, Type: eventType, Limit: 1})
		if len(events) != 1 {
			t.Fatalf("expected %s event, got %d", eventType, len(events))
		}
		metadata := events[0].Metadata
		if metadata["organization_id"] != "organization-files" || metadata["workspace_id"] != "workspace-files" || metadata["app_id"] != "app-files" || metadata["workflow_run_id"] != "run-files" || metadata["user_id"] != "user-files" {
			t.Fatalf("expected ownership metadata on %s, got %#v", eventType, metadata)
		}
	}
}

func TestUploadArchiveExtractsZipWithStripRoot(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-archive",
		WorkspaceID:    "workspace-archive",
		WorkflowRunID:  "run-archive",
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

	events := service.observer.Query(observer.Query{SandboxID: box.ID, Type: "files.upload_archive", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected archive upload event, got %d", len(events))
	}
	if events[0].Metadata["organization_id"] != "organization-archive" || events[0].Metadata["workspace_id"] != "workspace-archive" || events[0].Metadata["workflow_run_id"] != "run-archive" {
		t.Fatalf("expected ownership metadata on archive event, got %#v", events[0].Metadata)
	}
}

func TestUploadArchiveValidatesSkillManifest(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	result, err := service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      ".",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":             "# Skill",
			"scripts/run.py":       "print('ok')",
			"references/schema.md": "schema",
			"skill.manifest.json":  validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("expected archive upload with valid skill manifest, got %v", err)
	}
	if result.SkillManifest == nil {
		t.Fatal("expected skill manifest in upload result")
	}
	if result.SkillManifest.Entrypoint != "scripts/run.py" || result.SkillManifest.Language != "python3" {
		t.Fatalf("unexpected skill manifest: %+v", result.SkillManifest)
	}
}

func TestUploadArchiveRejectsInvalidSkillManifest(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      ".",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('ok')",
			"skill.manifest.json": validSkillManifestJSON("scripts/missing.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err == nil {
		t.Fatal("expected invalid skill manifest to be rejected")
	}
	if !strings.Contains(err.Error(), "entrypoint is missing") {
		t.Fatalf("expected entrypoint error, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      ".",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"run.py":              "print('ok')",
			"skill.manifest.json": validSkillManifestJSON("run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err == nil || !strings.Contains(err.Error(), "entrypoint must be under scripts") {
		t.Fatalf("expected entrypoint directory error, got %v", err)
	}
}

func TestRunSkillUsesManifestPolicyAndReturnsArtifacts(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-skill",
		WorkspaceID:    "workspace-skill",
		WorkflowRunID:  "run-skill",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/echo",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "import json, os, sys\npayload = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('skill artifact\\n')\nprint(json.dumps({'echo': payload.get('input'), 'ok': True}))\n",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	result, err := service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/echo",
		InputJSON: []byte(`{"input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if result.Command.ExitCode != 0 || !strings.Contains(result.Command.Stdout, `"echo": "hello"`) && !strings.Contains(result.Command.Stdout, `"echo":"hello"`) {
		t.Fatalf("unexpected skill command result: %+v", result.Command)
	}
	if result.Manifest.Entrypoint != "scripts/run.py" || result.Manifest.Language != "python3" {
		t.Fatalf("unexpected manifest: %+v", result.Manifest)
	}
	if len(result.ArtifactManifests) != 1 || result.ArtifactManifests[0].FileCount != 1 {
		t.Fatalf("expected one artifact manifest, got %+v", result.ArtifactManifests)
	}
	if result.ArtifactManifests[0].Items[0].Path != "skills/echo/artifacts/report.txt" {
		t.Fatalf("unexpected artifact path: %+v", result.ArtifactManifests[0].Items[0])
	}

	events := service.observer.Query(observer.Query{SandboxID: box.ID, Type: "exec.skill", Limit: 1})
	if len(events) != 1 || events[0].Metadata["organization_id"] != "organization-skill" {
		t.Fatalf("expected skill execution event with ownership metadata, got %#v", events)
	}
}

func TestRunSkillRejectsManifestArtifactLimit(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/limited",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":       "limited",
			"scripts/run.py": "import os\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/one.txt', 'w').write('one')\nopen('artifacts/two.txt', 'w').write('two')\nprint('ok')\n",
			"skill.manifest.json": `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 1,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`,
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/limited",
	})
	if err == nil {
		t.Fatal("expected artifact count limit")
	}
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "artifact_manifest_file_count_exceeded" {
		t.Fatalf("expected artifact count limit error, got %v", err)
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
	if item.Encoding != "reference" {
		t.Fatalf("expected reference encoding, got %s", item.Encoding)
	}
	if !strings.HasPrefix(item.ContentType, "text/plain") {
		t.Fatalf("expected text content type, got %s", item.ContentType)
	}
}

func TestBuildFileManifestEnforcesArtifactLimits(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	for _, item := range []struct {
		path    string
		content string
	}{
		{path: "artifacts/one.txt", content: "one"},
		{path: "artifacts/two.txt", content: "two"},
	} {
		if _, err := service.UploadFile(FileWriteRequest{
			SandboxID: box.ID,
			Path:      item.path,
			Content:   item.content,
		}); err != nil {
			t.Fatalf("upload %s: %v", item.path, err)
		}
	}

	_, err = service.BuildFileManifestWithOptions(box.ID, "artifacts", FileManifestOptions{MaxFiles: 1})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected file count limit error, got %T %v", err, err)
	}
	if limitErr.Code != "artifact_manifest_file_count_exceeded" || limitErr.Limit != "max_artifact_manifest_files" {
		t.Fatalf("unexpected file count limit error: %+v", limitErr)
	}

	_, err = service.BuildFileManifestWithOptions(box.ID, "artifacts", FileManifestOptions{MaxTotalBytes: 4})
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected total byte limit error, got %T %v", err, err)
	}
	if limitErr.Code != "artifact_manifest_total_bytes_exceeded" || limitErr.Limit != "max_artifact_manifest_total_bytes" {
		t.Fatalf("unexpected byte limit error: %+v", limitErr)
	}

	maxFiles, maxTotalBytes := service.normalizeManifestLimits(FileManifestOptions{MaxFiles: 1000, MaxTotalBytes: 1 << 40})
	if maxFiles != 100 || maxTotalBytes != 64*1024*1024 {
		t.Fatalf("expected request options not to raise manifest limits, got maxFiles=%d maxTotalBytes=%d", maxFiles, maxTotalBytes)
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

func validSkillManifestJSON(entrypoint string) string {
	return `{
  "entrypoint": "` + entrypoint + `",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`
}
