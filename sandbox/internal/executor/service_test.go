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
	if !strings.HasPrefix(commandResult.ExecutionID, "exec_") {
		t.Fatalf("expected command execution id, got %q", commandResult.ExecutionID)
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
	if !strings.HasPrefix(codeResult.ExecutionID, "exec_") {
		t.Fatalf("expected code execution id, got %q", codeResult.ExecutionID)
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
	result, err := service.RunCommand(ctx, CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	if err != nil {
		t.Fatalf("expected command run, got %v", err)
	}
	if !strings.HasPrefix(result.ExecutionID, "exec_") {
		t.Fatalf("expected execution id, got %q", result.ExecutionID)
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected command event, got %d", len(events))
	}
	if events[0].Metadata["request_id"] != "req_exec_test" {
		t.Fatalf("expected request ID metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["execution_id"] != result.ExecutionID {
		t.Fatalf("expected execution ID metadata, got %#v result=%q", events[0].Metadata, result.ExecutionID)
	}
	if events[0].Metadata["status"] != "success" {
		t.Fatalf("expected success status, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-exec" || events[0].Metadata["workspace_id"] != "workspace-exec" || events[0].Metadata["workflow_run_id"] != "run-exec" {
		t.Fatalf("expected ownership metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["dependency_profile"] != "stdlib" || events[0].Metadata["dependency_profile_version"] != "2026.05.01" {
		t.Fatalf("expected dependency profile metadata, got %#v", events[0].Metadata)
	}
}

func TestExecutionEventsIncludePinnedDependencyProfileVersion(t *testing.T) {
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(config.FromEnv())
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		OrganizationID:    "organization-profile-version",
		DependencyProfile: "workflow-safe",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	if box.DependencyProfile != "workflow-safe" || box.DependencyProfileVersion != "2026.05.01" {
		t.Fatalf("expected pinned dependency profile on sandbox, got %+v", box)
	}

	codeResult, err := service.RunCode(observer.ContextWithRequestID(context.Background(), "req_profile_version_code"), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('profile-version')",
		Profile:   "code-short",
	})
	if err != nil {
		t.Fatalf("expected code run, got %v", err)
	}
	codeEvents := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.code", RequestID: "req_profile_version_code", Limit: 1})
	if len(codeEvents) != 1 {
		t.Fatalf("expected code execution event, got %d", len(codeEvents))
	}
	if codeEvents[0].Metadata["execution_id"] != codeResult.ExecutionID {
		t.Fatalf("expected code execution id metadata, got %#v", codeEvents[0].Metadata)
	}
	if codeEvents[0].Metadata["dependency_profile"] != "workflow-safe" || codeEvents[0].Metadata["dependency_profile_version"] != "2026.05.01" {
		t.Fatalf("expected code dependency profile metadata, got %#v", codeEvents[0].Metadata)
	}

	commandResult, err := service.RunCommand(observer.ContextWithRequestID(context.Background(), "req_profile_version_command"), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	if err != nil {
		t.Fatalf("expected command run, got %v", err)
	}
	commandEvents := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command", RequestID: "req_profile_version_command", Limit: 1})
	if len(commandEvents) != 1 {
		t.Fatalf("expected command execution event, got %d", len(commandEvents))
	}
	if commandEvents[0].Metadata["execution_id"] != commandResult.ExecutionID {
		t.Fatalf("expected command execution id metadata, got %#v", commandEvents[0].Metadata)
	}
	if commandEvents[0].Metadata["dependency_profile"] != "workflow-safe" || commandEvents[0].Metadata["dependency_profile_version"] != "2026.05.01" {
		t.Fatalf("expected command dependency profile metadata, got %#v", commandEvents[0].Metadata)
	}
}

func TestOrganizationExecutionRateLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxExecutionsPerMinutePerOrganization = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-rate",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	}); err != nil {
		t.Fatalf("expected first command to run, got %v", err)
	}

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	limitErr, ok := err.(*policy.LimitError)
	if !ok {
		t.Fatalf("expected organization execution rate LimitError, got %T %v", err, err)
	}
	if limitErr.Code != "organization_execution_rate_limit_exceeded" || limitErr.Limit != "max_executions_per_minute_per_organization" {
		t.Fatalf("unexpected limit error: %+v", limitErr)
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected rate limit failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "organization_execution_rate_limit_exceeded" {
		t.Fatalf("expected structured rate limit metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-rate" {
		t.Fatalf("expected organization metadata, got %#v", events[0].Metadata)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('blocked')",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_execution_rate_limit_exceeded" {
		t.Fatalf("expected code execution rate limit error, got %v", err)
	}

	skillBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-rate-skill",
	})
	if err != nil {
		t.Fatalf("expected skill sandbox create, got %v", err)
	}
	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/rate",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('skill')",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}
	if _, err := service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/rate",
	}); err != nil {
		t.Fatalf("expected first skill run, got %v", err)
	}
	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/rate",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_execution_rate_limit_exceeded" {
		t.Fatalf("expected skill execution rate limit error, got %v", err)
	}
}

func TestOrganizationConcurrentExecutionLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxConcurrentExecutionsPerOrganization = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-concurrent",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	otherBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-other-concurrent",
	})
	if err != nil {
		t.Fatalf("expected other sandbox create, got %v", err)
	}
	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/concurrent",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('skill')",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.8)"},
			TimeoutMS: 2000,
		})
		firstDone <- err
	}()
	waitForOrganizationExecutions(t, service, "organization-concurrent", 1)

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_concurrent_execution_limit_exceeded" || limitErr.Limit != "max_concurrent_executions_per_organization" {
		t.Fatalf("expected organization concurrent command limit error, got %v", err)
	}
	if limitErr.Details["organization_id"] != "organization-concurrent" {
		t.Fatalf("expected organization id in details, got %+v", limitErr.Details)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('blocked')",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected organization concurrent code limit error, got %v", err)
	}

	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/concurrent",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected organization concurrent skill limit error, got %v", err)
	}

	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: otherBox.ID,
		Command:   "pwd",
	}); err != nil {
		t.Fatalf("expected other organization execution to run, got %v", err)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf("expected first command to complete, got %v", err)
	}
	waitForOrganizationExecutions(t, service, "organization-concurrent", 0)

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected concurrent limit failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "organization_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected structured concurrent limit metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-concurrent" {
		t.Fatalf("expected organization metadata, got %#v", events[0].Metadata)
	}
}

func TestServiceConcurrentExecutionLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxConcurrentExecutions = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-service-limit",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/service-limit",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('skill')",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.4)"},
			Profile:   "code-short",
			TimeoutMS: 1000,
		})
		firstDone <- err
	}()
	waitForServiceExecutions(t, service, 1)

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
		Profile:   "skill-python",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "service_concurrent_execution_limit_exceeded" || limitErr.Limit != "max_concurrent_executions" {
		t.Fatalf("expected service concurrent command limit error, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('blocked')",
		Profile:   "skill-python",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "service_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected service concurrent code limit error, got %v", err)
	}

	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/service-limit",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "service_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected service concurrent skill limit error, got %v", err)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf("expected first command to complete, got %v", err)
	}
	waitForServiceExecutions(t, service, 0)

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected service limit failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "service_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected structured service limit metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["limit"] != "max_concurrent_executions" {
		t.Fatalf("expected service limit metadata, got %#v", events[0].Metadata)
	}
}

func TestProfileConcurrentExecutionLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxConcurrentExecutionsPerProfile = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-profile",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/profile",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('skill')",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	codeShortDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.4)"},
			Profile:   "code-short",
			TimeoutMS: 1000,
		})
		codeShortDone <- err
	}()
	waitForProfileExecutions(t, service, "code-short", 1)

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
		Profile:   "code-short",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "profile_concurrent_execution_limit_exceeded" || limitErr.Limit != "max_concurrent_executions_per_profile" {
		t.Fatalf("expected profile concurrent command limit error, got %v", err)
	}
	if limitErr.Details["profile"] != "code-short" {
		t.Fatalf("expected profile in details, got %+v", limitErr.Details)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID: box.ID,
		Language:  "python3",
		Code:      "print('blocked')",
		Profile:   "code-short",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "profile_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected profile concurrent code limit error, got %v", err)
	}

	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
		Profile:   "skill-python",
	}); err != nil {
		t.Fatalf("expected different profile execution to run, got %v", err)
	}

	if err := <-codeShortDone; err != nil {
		t.Fatalf("expected code-short command to complete, got %v", err)
	}
	waitForProfileExecutions(t, service, "code-short", 0)

	skillProfileDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.4)"},
			Profile:   "skill-python",
			TimeoutMS: 1000,
		})
		skillProfileDone <- err
	}()
	waitForProfileExecutions(t, service, "skill-python", 1)

	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/profile",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "profile_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected profile concurrent skill limit error, got %v", err)
	}

	if err := <-skillProfileDone; err != nil {
		t.Fatalf("expected skill-python command to complete, got %v", err)
	}
	waitForProfileExecutions(t, service, "skill-python", 0)

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected profile limit failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "profile_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected structured profile limit metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["profile"] != "code-short" {
		t.Fatalf("expected profile metadata, got %#v", events[0].Metadata)
	}
}

func TestProfileLimitReleasesQueuedOrganizationAdmission(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxConcurrentExecutionsPerProfile = 1
	cfg.MaxConcurrentExecutionsPerOrganization = 1
	cfg.MaxQueuedExecutionsPerOrganization = 2
	cfg.QueueTimeoutMS = 1000
	service := &Service{
		policy:                         policy.NewService(cfg),
		executionChanged:               make(chan struct{}),
		activeExecutionsByProfile:      make(map[string]int),
		activeExecutionsByOrganization: map[string]int{"organization-queued-profile": 1},
		queuedExecutionsByOrganization: make(map[string]int),
	}
	box := &sandbox.Sandbox{OrganizationID: "organization-queued-profile"}

	errCh := make(chan error, 1)
	go func() {
		release, err := service.acquireExecutionAdmission(context.Background(), box, "code-short")
		if release != nil {
			release()
		}
		errCh <- err
	}()
	waitForQueuedOrganizationExecutions(t, service, "organization-queued-profile", 1)

	service.executionMu.Lock()
	service.activeExecutionsByProfile["code-short"] = 1
	service.notifyExecutionChangedLocked()
	service.executionMu.Unlock()

	err := <-errCh
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "profile_concurrent_execution_limit_exceeded" {
		t.Fatalf("expected profile limit error, got %v", err)
	}
	waitForQueuedOrganizationExecutions(t, service, "organization-queued-profile", 0)
}

func TestOrganizationQueuedExecutionLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxConcurrentExecutionsPerOrganization = 1
	cfg.MaxQueuedExecutionsPerOrganization = 1
	cfg.QueueTimeoutMS = 1000
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-queued",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.3)"},
			TimeoutMS: 1000,
		})
		firstDone <- err
	}()
	waitForOrganizationExecutions(t, service, "organization-queued", 1)

	queuedDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "print('queued')"},
			TimeoutMS: 1000,
		})
		queuedDone <- err
	}()
	waitForQueuedOrganizationExecutions(t, service, "organization-queued", 1)

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_queued_execution_limit_exceeded" || limitErr.Limit != "max_queued_executions_per_organization" {
		t.Fatalf("expected organization queued execution limit error, got %v", err)
	}
	if limitErr.Details["organization_id"] != "organization-queued" {
		t.Fatalf("expected organization id in details, got %+v", limitErr.Details)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf("expected first command to complete, got %v", err)
	}
	if err := <-queuedDone; err != nil {
		t.Fatalf("expected queued command to complete, got %v", err)
	}
	waitForOrganizationExecutions(t, service, "organization-queued", 0)
	waitForQueuedOrganizationExecutions(t, service, "organization-queued", 0)

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected queued limit failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "organization_queued_execution_limit_exceeded" {
		t.Fatalf("expected structured queued limit metadata, got %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-queued" {
		t.Fatalf("expected organization metadata, got %#v", events[0].Metadata)
	}
}

func TestOrganizationQueuedExecutionTimeout(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxConcurrentExecutionsPerOrganization = 1
	cfg.MaxQueuedExecutionsPerOrganization = 1
	cfg.QueueTimeoutMS = 50
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-queue-timeout",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.RunCommand(context.Background(), CommandRequest{
			SandboxID: box.ID,
			Command:   "python3",
			Args:      []string{"-c", "import time; time.sleep(0.3)"},
			TimeoutMS: 1000,
		})
		firstDone <- err
	}()
	waitForOrganizationExecutions(t, service, "organization-queue-timeout", 1)

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "pwd",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_execution_queue_timeout" || limitErr.Limit != "queue_timeout_ms" {
		t.Fatalf("expected organization queue timeout error, got %v", err)
	}
	if limitErr.Maximum != cfg.QueueTimeoutMS {
		t.Fatalf("expected queue timeout maximum %d, got %+v", cfg.QueueTimeoutMS, limitErr)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf("expected first command to complete, got %v", err)
	}
	waitForQueuedOrganizationExecutions(t, service, "organization-queue-timeout", 0)
}

func TestWorkspaceByteLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxWorkspaceBytes = 16
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-workspace",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/one.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("expected first upload below workspace limit, got %v", err)
	}

	_, err = service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/two.txt",
		Content:   "1234567890",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_byte_limit_exceeded" || limitErr.Limit != "max_workspace_bytes" {
		t.Fatalf("expected workspace byte limit on upload, got %v", err)
	}
	if _, err := service.StatFile(box.ID, "notes/two.txt"); err == nil {
		t.Fatal("expected rejected upload to leave no file")
	}

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "python3",
		Args:      []string{"-c", "open('generated.txt', 'w').write('1234567890')"},
		Profile:   "code-short",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_byte_limit_exceeded" {
		t.Fatalf("expected workspace byte limit on generated files, got %v", err)
	}
	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 || events[0].Metadata["code"] != "workspace_byte_limit_exceeded" {
		t.Fatalf("expected structured workspace limit event, got %#v", events)
	}

	codeBox, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected code sandbox create, got %v", err)
	}
	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:     codeBox.ID,
		Language:      "python3",
		Code:          "open('code-generated.txt', 'w').write('12345678901234567')",
		BindWorkspace: true,
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_byte_limit_exceeded" {
		t.Fatalf("expected code workspace byte limit, got %v", err)
	}

	skillRecorder := observer.NewRecorder(100)
	skillCfg := config.FromEnv()
	skillCfg.DataDir = t.TempDir()
	skillCfg.MaxWorkspaceBytes = 4096
	skillPolicy := policy.NewService(skillCfg)
	skillManager, err := lifecycle.NewManagerWithConfig(skillRecorder, skillPolicy, skillCfg, nil, nil)
	if err != nil {
		t.Fatalf("expected skill lifecycle manager, got %v", err)
	}
	skillService := NewService(skillManager, runner.NewService(2, 3*time.Second, 4096), skillRecorder, skillPolicy)
	skillBox, err := skillManager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected skill sandbox create, got %v", err)
	}
	_, err = skillService.UploadArchive(ArchiveUploadRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/workspace",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "import os\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/large.txt', 'w').write('x' * 4096)\nprint('done')\n",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}
	_, err = skillService.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/workspace",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_byte_limit_exceeded" {
		t.Fatalf("expected skill workspace byte limit, got %v", err)
	}
}

func TestWorkspaceByteLimitRejectsArchiveBeforeWriting(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxWorkspaceBytes = 8
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "pkg",
		ArchiveBase64: zipBase64(t, map[string]string{
			"a.txt": "12345",
			"b.txt": "67890",
		}),
		Format: "zip",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_byte_limit_exceeded" {
		t.Fatalf("expected archive workspace byte limit, got %v", err)
	}
	if _, err := service.StatFile(box.ID, "pkg/a.txt"); err == nil {
		t.Fatal("expected archive rejection to avoid partial writes")
	}
	if _, err := service.StatFile(box.ID, "pkg/b.txt"); err == nil {
		t.Fatal("expected archive rejection to avoid partial writes")
	}
}

func TestOrganizationWorkspaceByteLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxWorkspaceBytesPerOrganization = 16
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	firstBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-workspace-bytes",
	})
	if err != nil {
		t.Fatalf("expected first sandbox create, got %v", err)
	}
	secondBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-workspace-bytes",
	})
	if err != nil {
		t.Fatalf("expected second sandbox create, got %v", err)
	}
	otherBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-workspace-other",
	})
	if err != nil {
		t.Fatalf("expected other sandbox create, got %v", err)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: firstBox.ID,
		Path:      "notes/one.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("expected first organization upload below limit, got %v", err)
	}

	_, err = service.UploadFile(FileWriteRequest{
		SandboxID: secondBox.ID,
		Path:      "notes/two.txt",
		Content:   "1234567890",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_workspace_byte_limit_exceeded" || limitErr.Limit != "max_workspace_bytes_per_organization" {
		t.Fatalf("expected organization workspace byte limit, got %v", err)
	}
	if limitErr.Details["organization_id"] != "organization-workspace-bytes" {
		t.Fatalf("expected organization id in limit details, got %+v", limitErr.Details)
	}
	if _, err := service.StatFile(secondBox.ID, "notes/two.txt"); err == nil {
		t.Fatal("expected rejected organization upload to leave no file")
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: otherBox.ID,
		Path:      "notes/other.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("expected other organization upload to use separate quota, got %v", err)
	}

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: secondBox.ID,
		Command:   "python3",
		Args:      []string{"-c", "open('generated.txt', 'w').write('1234567890')"},
		Profile:   "code-short",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_workspace_byte_limit_exceeded" {
		t.Fatalf("expected organization workspace byte limit on generated files, got %v", err)
	}
	events := recorder.Query(observer.Query{SandboxID: secondBox.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 || events[0].Metadata["code"] != "organization_workspace_byte_limit_exceeded" || events[0].Metadata["organization_id"] != "organization-workspace-bytes" {
		t.Fatalf("expected structured organization workspace limit event, got %#v", events)
	}
}

func TestWorkspaceFileLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxWorkspaceFiles = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-file-count",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/one.txt",
		Content:   "one",
	}); err != nil {
		t.Fatalf("expected first upload below workspace file limit, got %v", err)
	}

	_, err = service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "notes/two.txt",
		Content:   "two",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_file_count_limit_exceeded" || limitErr.Limit != "max_workspace_files" {
		t.Fatalf("expected workspace file count limit on upload, got %v", err)
	}
	if _, err := service.StatFile(box.ID, "notes/two.txt"); err == nil {
		t.Fatal("expected rejected upload to leave no file")
	}

	_, err = service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "python3",
		Args:      []string{"-c", "open('generated.txt', 'w').write('generated')"},
		Profile:   "code-short",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_file_count_limit_exceeded" {
		t.Fatalf("expected workspace file count limit on generated files, got %v", err)
	}
	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 || events[0].Metadata["code"] != "workspace_file_count_limit_exceeded" {
		t.Fatalf("expected structured workspace file limit event, got %#v", events)
	}

	codeBox, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected code sandbox create, got %v", err)
	}
	if _, err := service.RunCode(context.Background(), CodeRequest{
		SandboxID:     codeBox.ID,
		Language:      "python3",
		Code:          "open('one.txt', 'w').write('one')",
		BindWorkspace: true,
	}); err != nil {
		t.Fatalf("expected first code-generated file below limit, got %v", err)
	}
	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:     codeBox.ID,
		Language:      "python3",
		Code:          "open('two.txt', 'w').write('two')",
		BindWorkspace: true,
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_file_count_limit_exceeded" {
		t.Fatalf("expected code workspace file limit, got %v", err)
	}

	skillRecorder := observer.NewRecorder(100)
	skillCfg := config.FromEnv()
	skillCfg.DataDir = t.TempDir()
	skillCfg.MaxWorkspaceFiles = 4
	skillPolicy := policy.NewService(skillCfg)
	skillManager, err := lifecycle.NewManagerWithConfig(skillRecorder, skillPolicy, skillCfg, nil, nil)
	if err != nil {
		t.Fatalf("expected skill lifecycle manager, got %v", err)
	}
	skillService := NewService(skillManager, runner.NewService(2, 3*time.Second, 4096), skillRecorder, skillPolicy)
	skillBox, err := skillManager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected skill sandbox create, got %v", err)
	}
	_, err = skillService.UploadArchive(ArchiveUploadRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/file-count",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "import os\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/one.txt', 'w').write('one')\nopen('artifacts/two.txt', 'w').write('two')\nprint('done')\n",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}
	_, err = skillService.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: skillBox.ID,
		Path:      "skills/file-count",
	})
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_file_count_limit_exceeded" {
		t.Fatalf("expected skill workspace file limit, got %v", err)
	}
}

func TestWorkspaceFileLimitRejectsArchiveBeforeWriting(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxWorkspaceFiles = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "pkg",
		ArchiveBase64: zipBase64(t, map[string]string{
			"a.txt": "a",
			"b.txt": "b",
		}),
		Format: "zip",
	})
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "workspace_file_count_limit_exceeded" {
		t.Fatalf("expected archive workspace file count limit, got %v", err)
	}
	if _, err := service.StatFile(box.ID, "pkg/a.txt"); err == nil {
		t.Fatal("expected archive rejection to avoid partial writes")
	}
	if _, err := service.StatFile(box.ID, "pkg/b.txt"); err == nil {
		t.Fatal("expected archive rejection to avoid partial writes")
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
	if executionID, _ := metadata["execution_id"].(string); !strings.HasPrefix(executionID, "exec_") {
		t.Fatalf("expected failure execution ID metadata, got %#v", metadata)
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
		OrganizationID: "organization-template",
		WorkspaceID:    "workspace-template",
		AppID:          "app-template",
		WorkflowRunID:  "run-template",
		UserID:         "user-template",
	})
	if err != nil {
		t.Fatalf("expected template render, got %v", err)
	}
	if result.Content != "Hello ZGI" || result.Truncated {
		t.Fatalf("unexpected template result: %+v", result)
	}
	if !strings.HasPrefix(result.ExecutionID, "exec_") {
		t.Fatalf("expected template execution id, got %q", result.ExecutionID)
	}

	events := recorder.Query(observer.Query{Type: "exec.template", Limit: 1})
	if len(events) != 1 || events[0].Metadata["status"] != "success" {
		t.Fatalf("expected template success event, got %#v", events)
	}
	if events[0].Metadata["execution_id"] != result.ExecutionID {
		t.Fatalf("expected template execution ID metadata, got %#v result=%q", events[0].Metadata, result.ExecutionID)
	}
	if events[0].Metadata["organization_id"] != "organization-template" ||
		events[0].Metadata["workspace_id"] != "workspace-template" ||
		events[0].Metadata["app_id"] != "app-template" ||
		events[0].Metadata["workflow_run_id"] != "run-template" ||
		events[0].Metadata["user_id"] != "user-template" {
		t.Fatalf("expected template ownership metadata, got %#v", events[0].Metadata)
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

	if _, err := service.RunTemplate(context.Background(), TemplateRequest{
		Template:       "{{ .missing }}",
		OrganizationID: "organization-template-failure",
		WorkspaceID:    "workspace-template-failure",
		WorkflowRunID:  "run-template-failure",
	}); err == nil {
		t.Fatal("expected missing variable to be rejected")
	}
	events := recorder.Query(observer.Query{Type: "exec.template.failed", OrganizationID: "organization-template-failure", WorkspaceID: "workspace-template-failure", WorkflowRunID: "run-template-failure", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected template failure event with ownership metadata, got %#v", events)
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
	if result.SkillManifest.Entrypoint != "scripts/run.py" || result.SkillManifest.Language != "python3" || result.SkillManifest.DependencyProfile != "stdlib" {
		t.Fatalf("unexpected skill manifest: %+v", result.SkillManifest)
	}
}

func TestUploadArchiveRejectsSkillManifestDependencyProfileMismatch(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		DependencyProfile: "stdlib",
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
			"skill.manifest.json": validSkillManifestJSONWithDependencyProfile("scripts/run.py", "workflow-safe"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match sandbox dependency_profile") {
		t.Fatalf("expected dependency profile mismatch, got %v", err)
	}
}

func TestUploadArchiveDefaultsSkillManifestDependencyProfileToSandbox(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		DependencyProfile: "workflow-safe",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	result, err := service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      ".",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":       "# Skill",
			"scripts/run.py": "print('ok')",
			"skill.manifest.json": `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`,
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("expected missing dependency profile to default to sandbox profile, got %v", err)
	}
	if result.SkillManifest == nil || result.SkillManifest.DependencyProfile != "workflow-safe" {
		t.Fatalf("expected sandbox dependency profile default, got %+v", result.SkillManifest)
	}
}

func TestUploadArchiveRejectsSkillManifestUnavailableDependencyProfile(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		DependencyProfile: "stdlib",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	for name, expected := range map[string]string{
		"missing-profile":     "unsupported dependency profile",
		"python-data-preview": "dependency profile is not enabled",
		"node-basic":          "does not support language",
	} {
		_, err = service.UploadArchive(ArchiveUploadRequest{
			SandboxID: box.ID,
			Path:      ".",
			ArchiveBase64: zipBase64(t, map[string]string{
				"SKILL.md":            "# Skill",
				"scripts/run.py":      "print('ok')",
				"skill.manifest.json": validSkillManifestJSONWithDependencyProfile("scripts/run.py", name),
			}),
			Format:                "zip",
			ValidateSkillManifest: true,
		})
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %s error for %s, got %v", expected, name, err)
		}
	}
}

func TestUploadArchiveUsesConfiguredArtifactManifestCountLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxArtifactManifestFiles = 1
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)
	box, err := manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      ".",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":       "# Skill",
			"scripts/run.py": "print('ok')",
			"skill.manifest.json": `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 2,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`,
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err == nil || !strings.Contains(err.Error(), "max_artifact_count must be between 1 and 1") {
		t.Fatalf("expected configured artifact count limit error, got %v", err)
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
			"scripts/run.py":      "import json, os, sys\npayload = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('skill artifact\\n')\nprint(json.dumps({'echo': payload.get('input'), 'message_id': os.environ.get('ZGI_MESSAGE_ID'), 'ok': True}))\n",
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
		Env:       map[string]string{"ZGI_MESSAGE_ID": "message-skill"},
	})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if result.Command.ExitCode != 0 || !strings.Contains(result.Command.Stdout, `"echo": "hello"`) && !strings.Contains(result.Command.Stdout, `"echo":"hello"`) {
		t.Fatalf("unexpected skill command result: %+v", result.Command)
	}
	if !strings.Contains(result.Command.Stdout, `"message_id": "message-skill"`) && !strings.Contains(result.Command.Stdout, `"message_id":"message-skill"`) {
		t.Fatalf("expected skill env in stdout, got %q", result.Command.Stdout)
	}
	if !strings.HasPrefix(result.ExecutionID, "exec_") || result.Command.ExecutionID != result.ExecutionID {
		t.Fatalf("expected skill execution id to propagate, got result=%q command=%q", result.ExecutionID, result.Command.ExecutionID)
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
	if events[0].Metadata["execution_id"] != result.ExecutionID {
		t.Fatalf("expected skill execution ID metadata, got %#v result=%q", events[0].Metadata, result.ExecutionID)
	}
}

func TestRunSkillRejectsDangerousEnv(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.UploadArchive(ArchiveUploadRequest{
		SandboxID: box.ID,
		Path:      "skills/env",
		ArchiveBase64: zipBase64(t, map[string]string{
			"SKILL.md":            "# Skill",
			"scripts/run.py":      "print('nope')\n",
			"skill.manifest.json": validSkillManifestJSON("scripts/run.py"),
		}),
		Format:                "zip",
		ValidateSkillManifest: true,
	})
	if err != nil {
		t.Fatalf("upload skill package: %v", err)
	}

	_, err = service.RunSkill(context.Background(), SkillRunRequest{
		SandboxID: box.ID,
		Path:      "skills/env",
		Env:       map[string]string{"LD_PRELOAD": "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "env key is not allowed") {
		t.Fatalf("expected dangerous env rejection, got %v", err)
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

func TestRunCodeShortProfileUsesStatelessWorkspaceWithSandboxByDefault(t *testing.T) {
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
		Code:             "import json, pathlib\npathlib.Path('session-marker.txt').write_text('temporary')\nprint(json.dumps({'created': True}))",
		Profile:          "code-short",
		StrictResultJSON: true,
	})
	if err != nil {
		t.Fatalf("expected sandbox-scoped stateless code run, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%q", result.ExitCode, result.Error)
	}
	if _, err := service.StatFile(box.ID, "session-marker.txt"); err == nil {
		t.Fatal("expected code-short to avoid writing into sandbox workspace by default")
	}
}

func TestRunCodeCanExplicitlyBindStatelessProfileToWorkspace(t *testing.T) {
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
		Code:             "import json, pathlib\npathlib.Path('session-marker.txt').write_text('bound')\nprint(json.dumps({'created': True}))",
		Profile:          "code-short",
		StrictResultJSON: true,
		BindWorkspace:    true,
	})
	if err != nil {
		t.Fatalf("expected workspace-bound code run, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d stderr=%q", result.ExitCode, result.Error)
	}
	info, err := service.StatFile(box.ID, "session-marker.txt")
	if err != nil {
		t.Fatalf("expected explicit workspace binding to write marker, got %v", err)
	}
	if info.Size != 5 {
		t.Fatalf("expected marker size, got %+v", info)
	}
}

func TestRunCodeStatelessProfileRejectsNetworkEvenWithSandbox(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:     box.ID,
		Language:      "python3",
		Code:          "print('no network')",
		Profile:       "code-short",
		EnableNetwork: true,
		BindWorkspace: false,
	})
	if err == nil || !strings.Contains(err.Error(), "stateless code execution") {
		t.Fatalf("expected stateless network rejection, got %v", err)
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

func TestRunCodeRejectsOversizedStrictResultJSON(t *testing.T) {
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
		Code:             "import json\nprint(json.dumps({'payload': 'x' * 70000}))",
		Profile:          "code-short",
		StdoutLimitKB:    128,
		StrictResultJSON: true,
	})
	if err == nil || !strings.Contains(err.Error(), "result_json exceeds max size of 65536 bytes") {
		t.Fatalf("expected result JSON size rejection, got %v", err)
	}
}

func TestRunCodeValidatesExpectedOutputSchema(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	schema := []byte(`{
		"type": "object",
		"required": ["echo", "ok"],
		"additional_properties": false,
		"properties": {
			"echo": {"type": "string"},
			"ok": {"type": "boolean"},
			"count": {"type": "integer"}
		}
	}`)
	result, err := service.RunCode(context.Background(), CodeRequest{
		SandboxID:            box.ID,
		Language:             "python3",
		Code:                 "import json\nprint(json.dumps({'echo': 'hello', 'ok': True, 'count': 2}))",
		Profile:              "code-short",
		StrictResultJSON:     true,
		ExpectedOutputSchema: schema,
	})
	if err != nil {
		t.Fatalf("expected schema-valid code result, got %v", err)
	}
	data, ok := result.ResultJSON.(map[string]any)
	if !ok || data["echo"] != "hello" || data["ok"] != true || data["count"] != float64(2) {
		t.Fatalf("unexpected schema-valid result json: %#v", result.ResultJSON)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:            box.ID,
		Language:             "python3",
		Code:                 "import json\nprint(json.dumps({'echo': 'hello', 'extra': 'blocked'}))",
		Profile:              "code-short",
		StrictResultJSON:     true,
		ExpectedOutputSchema: schema,
	})
	if err == nil || !strings.Contains(err.Error(), "expected_output_schema validation failed") {
		t.Fatalf("expected schema validation failure, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:            box.ID,
		Language:             "python3",
		Code:                 "print('plain text')",
		Profile:              "code-short",
		ExpectedOutputSchema: schema,
	})
	if err == nil || !strings.Contains(err.Error(), "expected_output_schema") {
		t.Fatalf("expected schema parse failure for plain stdout, got %v", err)
	}
}

func TestRunCodeRejectsInvalidExpectedOutputSchema(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:            box.ID,
		Language:             "python3",
		Code:                 "print('{}')",
		Profile:              "code-short",
		ExpectedOutputSchema: []byte(`{"type":"unsupported"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid expected_output_schema") {
		t.Fatalf("expected invalid schema rejection, got %v", err)
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

func TestRunCommandRejectsRuntimeDependencyInstall(t *testing.T) {
	service, manager := newTestExecutorService(t)
	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		DependencyProfile: "workflow-safe",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	cases := []CommandRequest{
		{SandboxID: box.ID, Command: "pip", Args: []string{"install", "requests"}},
		{SandboxID: box.ID, Command: "python3", Args: []string{"-m", "pip", "install", "requests"}},
		{SandboxID: box.ID, Command: "uv", Args: []string{"pip", "sync"}},
		{SandboxID: box.ID, Command: "npm install left-pad"},
		{SandboxID: box.ID, Command: "cd /tmp && npm install left-pad"},
		{SandboxID: box.ID, Command: "sh", Args: []string{"-c", "npm install left-pad"}},
		{SandboxID: box.ID, Command: "env", Args: []string{"NODE_ENV=production", "npm", "install"}},
		{SandboxID: box.ID, Command: "pnpm", Args: []string{"add", "left-pad"}},
		{SandboxID: box.ID, Command: "yarn", Args: []string{"add", "left-pad"}},
	}
	for _, req := range cases {
		_, err := service.RunCommand(context.Background(), req)
		var dependencyErr *DependencyInstallError
		if !errors.As(err, &dependencyErr) {
			t.Fatalf("expected dependency install error for %+v, got %T %v", req, err, err)
		}
		if dependencyErr.ResponseDetails()["code"] != "dependency_install_disabled" {
			t.Fatalf("expected structured dependency install code, got %+v", dependencyErr.ResponseDetails())
		}
	}

	events := service.observer.Query(observer.Query{SandboxID: box.ID, Type: "exec.command.failed", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected dependency install failure event, got %d", len(events))
	}
	if events[0].Metadata["error_type"] != "policy_denied" || events[0].Metadata["code"] != "dependency_install_disabled" {
		t.Fatalf("expected dependency install policy event, got %#v", events[0].Metadata)
	}

	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "echo npm install is documented",
		Profile:   "code-short",
	}); err != nil {
		t.Fatalf("expected non-install shell text to run, got %v", err)
	}
	if _, err := service.RunCommand(context.Background(), CommandRequest{
		SandboxID: box.ID,
		Command:   "sh",
		Args:      []string{"-c", "echo npm install is documented"},
		Profile:   "code-short",
	}); err != nil {
		t.Fatalf("expected non-install shell argv text to run, got %v", err)
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

func TestBuildFileManifestEnforcesOrganizationArtifactByteLimit(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxArtifactBytesPerOrganization = 16
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	firstBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-artifacts",
	})
	if err != nil {
		t.Fatalf("expected first sandbox create, got %v", err)
	}
	secondBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-artifacts",
	})
	if err != nil {
		t.Fatalf("expected second sandbox create, got %v", err)
	}
	otherBox, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile: string(sandbox.RuntimeSession),
		OrganizationID: "organization-artifacts-other",
	})
	if err != nil {
		t.Fatalf("expected other sandbox create, got %v", err)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: firstBox.ID,
		Path:      "artifacts/report.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("upload first artifact: %v", err)
	}
	if _, err := service.BuildFileManifest(firstBox.ID, "artifacts"); err != nil {
		t.Fatalf("expected first organization artifact manifest below limit, got %v", err)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: secondBox.ID,
		Path:      "artifacts/report.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("upload second artifact: %v", err)
	}
	_, err = service.BuildFileManifest(secondBox.ID, "artifacts")
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "organization_artifact_byte_limit_exceeded" || limitErr.Limit != "max_artifact_bytes_per_organization" {
		t.Fatalf("expected organization artifact byte limit, got %v", err)
	}
	if limitErr.Details["organization_id"] != "organization-artifacts" {
		t.Fatalf("expected organization id in limit details, got %+v", limitErr.Details)
	}

	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: otherBox.ID,
		Path:      "artifacts/report.txt",
		Content:   "1234567890",
	}); err != nil {
		t.Fatalf("upload other artifact: %v", err)
	}
	if _, err := service.BuildFileManifest(otherBox.ID, "artifacts"); err != nil {
		t.Fatalf("expected other organization artifact manifest to use separate quota, got %v", err)
	}
}

func TestBuildFileManifestUsesConfiguredArtifactLimits(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.MaxArtifactManifestFiles = 1
	cfg.MaxArtifactManifestBytes = 4
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
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

	maxFiles, maxTotalBytes := service.normalizeManifestLimits(FileManifestOptions{MaxFiles: 1000, MaxTotalBytes: 1 << 20})
	if maxFiles != 1 || maxTotalBytes != 4 {
		t.Fatalf("expected configured limits to clamp request options, got maxFiles=%d maxTotalBytes=%d", maxFiles, maxTotalBytes)
	}

	_, err = service.BuildFileManifest(box.ID, "artifacts")
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) || limitErr.Code != "artifact_manifest_file_count_exceeded" || limitErr.Maximum != 1 {
		t.Fatalf("expected configured file count limit error, got %v", err)
	}

	cfg.MaxArtifactManifestFiles = 10
	cfg.DataDir = t.TempDir()
	policyService = policy.NewService(cfg)
	manager, err = lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service = NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)
	box, err = manager.Create(lifecycle.CreateRequest{RuntimeProfile: string(sandbox.RuntimeSession)})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	if _, err := service.UploadFile(FileWriteRequest{
		SandboxID: box.ID,
		Path:      "artifacts/large.txt",
		Content:   "hello",
	}); err != nil {
		t.Fatalf("upload large artifact: %v", err)
	}
	_, err = service.BuildFileManifest(box.ID, "artifacts")
	if !errors.As(err, &limitErr) || limitErr.Code != "artifact_manifest_total_bytes_exceeded" || limitErr.Maximum != 4 {
		t.Fatalf("expected configured byte limit error, got %v", err)
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

func TestRunCodeRejectsNetworkWhenProfileDisallowsIt(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.RuntimeBackend = "linux-secure"
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	service := NewService(manager, runner.NewService(2, 3*time.Second, 4096), recorder, policyService)

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		NetworkEnabled:    true,
		NetworkPolicy:     "workflow-safe",
		DependencyProfile: "stdlib",
	})
	if err != nil {
		t.Fatalf("expected network-enabled sandbox create, got %v", err)
	}

	_, err = service.RunCode(context.Background(), CodeRequest{
		SandboxID:     box.ID,
		Language:      "python3",
		Code:          "print('blocked')",
		Profile:       "skill-python",
		EnableNetwork: true,
	})
	if err == nil || !strings.Contains(err.Error(), "network access is disabled for command profile: skill-python") {
		t.Fatalf("expected profile-level network rejection, got %v", err)
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

func waitForOrganizationExecutions(t *testing.T, service *Service, organizationID string, expected int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.executionMu.Lock()
		active := service.activeExecutionsByOrganization[organizationID]
		service.executionMu.Unlock()
		if active == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d active executions for organization %q", expected, organizationID)
}

func waitForServiceExecutions(t *testing.T, service *Service, expected int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.executionMu.Lock()
		active := service.activeExecutions
		service.executionMu.Unlock()
		if active == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d active service executions", expected)
}

func waitForProfileExecutions(t *testing.T, service *Service, profile string, expected int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.executionMu.Lock()
		active := service.activeExecutionsByProfile[profile]
		service.executionMu.Unlock()
		if active == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d active executions for profile %q", expected, profile)
}

func waitForQueuedOrganizationExecutions(t *testing.T, service *Service, organizationID string, expected int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.executionMu.Lock()
		queued := service.queuedExecutionsByOrganization[organizationID]
		service.executionMu.Unlock()
		if queued == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d queued executions for organization %q", expected, organizationID)
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
	return validSkillManifestJSONWithDependencyProfile(entrypoint, "stdlib")
}

func validSkillManifestJSONWithDependencyProfile(entrypoint string, dependencyProfile string) string {
	return `{
  "entrypoint": "` + entrypoint + `",
  "language": "python3",
  "dependency_profile": "` + dependencyProfile + `",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`
}
