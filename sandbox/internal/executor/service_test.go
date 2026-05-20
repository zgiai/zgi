package executor

import (
	"context"
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
