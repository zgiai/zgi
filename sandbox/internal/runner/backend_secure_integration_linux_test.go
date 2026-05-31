//go:build linux && integration

package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func TestLinuxSecureBackendRunsPythonAndControlsNetwork(t *testing.T) {
	rootfs := os.Getenv("ZGI_SANDBOX_TEST_SECURE_ROOTFS")
	if strings.TrimSpace(rootfs) == "" {
		t.Skip("ZGI_SANDBOX_TEST_SECURE_ROOTFS is not set")
	}
	if _, err := exec.LookPath(defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))); err != nil {
		t.Skipf("bubblewrap unavailable: %v", err)
	}

	cfg := config.FromEnv()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = rootfs
	cfg.BwrapBinary = defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))
	cfg.TimeoutSeconds = 10
	cfg.MaxWorkers = 1
	cfg.OutputLimitKB = 64

	service, err := NewServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("new linux-secure service: %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Language: "python3",
		Code:     "print('secure-ok')",
	})
	if err != nil {
		t.Fatalf("run python in secure backend: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "secure-ok\n" {
		t.Fatalf("unexpected python result: stdout=%q stderr=%q exit=%d", result.Stdout, result.Error, result.ExitCode)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("network-ok"))
	}))
	defer server.Close()

	allowed, err := service.Run(context.Background(), Request{
		Language:      "python3",
		EnableNetwork: true,
		Code: "import urllib.request\n" +
			"print(urllib.request.urlopen('" + server.URL + "', timeout=2).read().decode())",
	})
	if err != nil {
		t.Fatalf("run allowed network request: %v", err)
	}
	if allowed.ExitCode != 0 || strings.TrimSpace(allowed.Stdout) != "network-ok" {
		t.Fatalf("expected allowed network access, got stdout=%q stderr=%q exit=%d", allowed.Stdout, allowed.Error, allowed.ExitCode)
	}

	blocked, err := service.Run(context.Background(), Request{
		Language:      "python3",
		EnableNetwork: false,
		Code: "import urllib.request\n" +
			"urllib.request.urlopen('" + server.URL + "', timeout=2).read()",
	})
	if err != nil {
		t.Fatalf("run blocked network request: %v", err)
	}
	if blocked.ExitCode == 0 {
		t.Fatalf("expected blocked network request to fail, got stdout=%q stderr=%q", blocked.Stdout, blocked.Error)
	}
}

func TestLinuxSecureBackendRunsCommand(t *testing.T) {
	rootfs := os.Getenv("ZGI_SANDBOX_TEST_SECURE_ROOTFS")
	if strings.TrimSpace(rootfs) == "" {
		t.Skip("ZGI_SANDBOX_TEST_SECURE_ROOTFS is not set")
	}
	if _, err := exec.LookPath(defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))); err != nil {
		t.Skipf("bubblewrap unavailable: %v", err)
	}

	cfg := config.FromEnv()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = rootfs
	cfg.BwrapBinary = defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))
	cfg.TimeoutSeconds = 10

	service, err := NewServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("new linux-secure service: %v", err)
	}

	workDir := t.TempDir()
	commandResult, err := service.ExecuteCommand(context.Background(), workDir, "pwd", nil, 5*time.Second)
	if err != nil {
		t.Fatalf("execute command in secure backend: %v", err)
	}
	if commandResult.ExitCode != 0 {
		t.Fatalf("unexpected command result: stdout=%q stderr=%q exit=%d", commandResult.Stdout, commandResult.Error, commandResult.ExitCode)
	}
}

func TestLinuxSecureBackendReportsSignalTermination(t *testing.T) {
	rootfs := os.Getenv("ZGI_SANDBOX_TEST_SECURE_ROOTFS")
	if strings.TrimSpace(rootfs) == "" {
		t.Skip("ZGI_SANDBOX_TEST_SECURE_ROOTFS is not set")
	}
	if _, err := exec.LookPath(defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))); err != nil {
		t.Skipf("bubblewrap unavailable: %v", err)
	}

	cfg := config.FromEnv()
	cfg.RuntimeBackend = "linux-secure"
	cfg.SecureRootFS = rootfs
	cfg.BwrapBinary = defaultBwrap(os.Getenv("ZGI_SANDBOX_TEST_BWRAP_BINARY"))
	cfg.TimeoutSeconds = 10

	service, err := NewServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("new linux-secure service: %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Language: "python3",
		Code:     "import os, signal\nos.kill(os.getpid(), signal.SIGTERM)",
	})
	if err != nil {
		t.Fatalf("run signal termination in secure backend: %v", err)
	}
	if result.ExitCode != 143 {
		t.Fatalf("expected signal exit code 143, got stdout=%q stderr=%q exit=%d", result.Stdout, result.Error, result.ExitCode)
	}
	if !strings.Contains(result.Error, "process terminated by signal") {
		t.Fatalf("expected signal stderr, got %q", result.Error)
	}
}

func defaultBwrap(value string) string {
	if strings.TrimSpace(value) == "" {
		return "bwrap"
	}
	return strings.TrimSpace(value)
}
