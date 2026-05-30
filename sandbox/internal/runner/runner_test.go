package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunPython(t *testing.T) {
	service := NewService(1, 2*time.Second, 4096)

	result, err := service.Run(context.Background(), Request{
		Language: "python3",
		Code:     "print('sandbox-ok')",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", result.ExitCode)
	}

	if result.Stdout != "sandbox-ok\n" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
}

func TestRunUnsupportedLanguage(t *testing.T) {
	service := NewService(1, 2*time.Second, 4096)

	_, err := service.Run(context.Background(), Request{
		Language: "ruby",
		Code:     "puts 'nope'",
	})
	if err == nil {
		t.Fatal("expected an error for unsupported language")
	}
}

func TestSafeBaseEnvDropsDangerousKeys(t *testing.T) {
	env := safeBaseEnv([]string{
		"PATH=/usr/bin",
		"LD_PRELOAD=x",
		"DYLD_INSERT_LIBRARIES=x",
		"NODE_OPTIONS=--require x",
		"ZGI_OK=1",
	})

	for _, item := range env {
		if item == "LD_PRELOAD=x" || item == "DYLD_INSERT_LIBRARIES=x" || item == "NODE_OPTIONS=--require x" {
			t.Fatalf("expected dangerous env to be dropped, got %v", env)
		}
	}
	if len(env) != 2 {
		t.Fatalf("expected safe env entries only, got %v", env)
	}
}

func TestCommandTimeoutKillsShellChildren(t *testing.T) {
	service := NewService(1, 2*time.Second, 4096)
	workDir := t.TempDir()
	pidFile := filepath.Join(workDir, "child.pid")

	result, err := service.ExecuteCommandSpec(context.Background(), CommandSpec{
		WorkDir:        workDir,
		Command:        "sleep 20 & echo $! > child.pid; wait",
		Timeout:        100 * time.Millisecond,
		StdoutLimit:    4096,
		StderrLimit:    4096,
		AllowShellForm: true,
	})
	if err != nil {
		t.Fatalf("expected timeout result, got error: %v", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("expected timeout exit code 124, got %d stderr=%q", result.ExitCode, result.Error)
	}

	rawPID, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(rawPID)))
	if err != nil {
		t.Fatalf("parse child pid: %v", err)
	}

	for i := 0; i < 20; i++ {
		if !processExists(childPID) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected child process %d to be killed with the shell process group", childPID)
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
