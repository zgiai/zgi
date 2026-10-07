//go:build linux

package runtimeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPackageTimeoutAndCancellationKillDescendants(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("request_cancel_%v", cancelRequest), func(t *testing.T) {
			root := t.TempDir()
			pidFile := filepath.Join(root, "child.pid")
			helper := filepath.Join(root, "apt-fixture")
			script := "#!/bin/sh\nsleep 60 &\necho $! > '" + pidFile + "'\nwait\n"
			if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
			backend := packageTestBackend(t)
			backend.packageInstallTimeout = 150 * time.Millisecond
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan PackageInstallResult, 1)
			go func() { result <- backend.runAPT(ctx, []string{"update"}) }()
			deadline := time.Now().Add(time.Second)
			var pid int
			for time.Now().Before(deadline) {
				if raw, err := os.ReadFile(pidFile); err == nil {
					pid, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
					if pid > 1 {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
			if pid <= 1 {
				t.Fatal("fixture child did not start")
			}
			if cancelRequest {
				cancel()
			}
			select {
			case got := <-result:
				want := "package installation timed out"
				if cancelRequest {
					want = "package installation canceled"
				}
				if got.ExitCode != -1 || got.Error != want {
					t.Fatalf("result=%+v want=%q", got, want)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("installer did not terminate")
			}
			// A killed descendant may await PID 1 reaping, but must not run.
			state, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			if err == nil && !strings.Contains(string(state), ") Z ") {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("descendant still running: %s", state)
			}
		})
	}
}

func TestConcurrentAPTRequestsAreSerialized(t *testing.T) {
	backend := packageTestBackend(t)
	backend.dependencyInstallation, backend.networkMode = true, "package-install"
	root := t.TempDir()
	helper := filepath.Join(root, "apt-fixture")
	lock := filepath.Join(root, "install.lock")
	script := "#!/bin/sh\nmkdir '" + lock + "' || exit 42\nsleep 0.03\nrmdir '" + lock + "'\n"
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	results := make(chan error, 8)
	for range 8 {
		go func() {
			result, err := backend.InstallSystemPackages(t.Context(), []string{"install", "tree"})
			if err == nil && result.ExitCode != 0 {
				err = fmt.Errorf("exit code %d: %s", result.ExitCode, result.Stderr)
			}
			results <- err
		}()
	}
	for range 8 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	ledger, err := backend.readPackageLedger()
	if err != nil || len(ledger.Entries) != 1 {
		t.Fatalf("serialized ledger: %+v error=%v", ledger, err)
	}
}

func TestAPTMetadataRetryAndReplayUseControlledSource(t *testing.T) {
	backend := packageTestBackend(t)
	backend.dependencyInstallation, backend.networkMode = true, "package-install"
	root := t.TempDir()
	helper := filepath.Join(root, "apt-fixture")
	metadata := filepath.Join(root, "metadata")
	log := filepath.Join(root, "commands")
	script := "#!/bin/sh\necho \"$1\" >> '" + log + "'\nif [ \"$1\" = update ]; then touch '" + metadata + "'; exit 0; fi\nif [ ! -f '" + metadata + "' ]; then echo 'E: Unable to locate package tree' >&2; exit 100; fi\n"
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	result, err := backend.InstallSystemPackages(t.Context(), []string{"install", "tree"})
	if err != nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, "metadata refreshed") {
		t.Fatalf("first install=%+v error=%v", result, err)
	}
	// A fresh rootfs loses metadata while the workspace ledger survives.
	if err := os.Remove(metadata); err != nil {
		t.Fatal(err)
	}
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	if err != nil || string(commands) != "install\nupdate\ninstall\ninstall\nupdate\ninstall\n" {
		t.Fatalf("commands=%q error=%v", commands, err)
	}
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	commandsAfter, _ := os.ReadFile(log)
	if string(commandsAfter) != string(commands) {
		t.Fatal("successful ledger replay was repeated")
	}
}

func TestPackageHTTPServerCloseCancelsInstaller(t *testing.T) {
	backend := packageTestBackend(t)
	backend.dependencyInstallation, backend.networkMode = true, "package-install"
	root := t.TempDir()
	started := filepath.Join(root, "started")
	helper := filepath.Join(root, "apt-fixture")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntouch '"+started+"'\nsleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	handler, err := NewHTTPServer(backend, HTTPServerOptions{Token: "fixture-token"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	server := httptest.NewServer(http.HandlerFunc(handler.ServePackageHTTP))
	t.Cleanup(server.Close)
	finished := make(chan error, 1)
	go func() {
		response, err := http.Post(server.URL+"/v1/package-install", "application/json", strings.NewReader(`{"args":["update"]}`))
		if response != nil {
			_ = response.Body.Close()
		}
		finished <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("installer did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server close left package request running")
	}
}
