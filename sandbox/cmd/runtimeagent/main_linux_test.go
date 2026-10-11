//go:build linux

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeAgentTokenReadsAndRemovesFile(t *testing.T) {
	t.Setenv("ZGI_RUNTIME_AGENT_TOKEN", "environment-token")
	tokenPath := filepath.Join(t.TempDir(), "bootstrap-token")
	if err := os.WriteFile(tokenPath, []byte("file-token\n"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_TOKEN_FILE", tokenPath)
	token, err := runtimeAgentToken()
	if err != nil || token != "file-token" {
		t.Fatalf("token = %q, error = %v", token, err)
	}
	if _, err := os.Lstat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token file remains after read: %v", err)
	}
}

func TestRuntimeAgentTokenRejectsSymlink(t *testing.T) {
	tokenDir := t.TempDir()
	tokenPath := filepath.Join(tokenDir, "token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	linkPath := filepath.Join(tokenDir, "token-link")
	if err := os.Symlink(tokenPath, linkPath); err != nil {
		t.Fatalf("symlink token: %v", err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_TOKEN_FILE", linkPath)
	if _, err := runtimeAgentToken(); err == nil {
		t.Fatal("symlink token file was accepted")
	}
}

func TestListenRuntimeAgentUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "runtime.sock")
	listener, cleanup, err := listenRuntimeAgent("unix://" + socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if listener.Addr().Network() != "unix" {
		t.Fatalf("network = %q, want unix", listener.Addr().Network())
	}
	client, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	_ = client.Close()
	cleanup()
	if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket remains after cleanup: %v", err)
	}
}

func TestListenRuntimeAgentDoesNotReplaceRegularFile(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "runtime.sock")
	if err := os.WriteFile(socketPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := listenRuntimeAgent("unix://" + socketPath); err == nil {
		t.Fatal("regular file was accepted as a socket")
	}
	value, err := os.ReadFile(socketPath)
	if err != nil || string(value) != "keep" {
		t.Fatalf("regular file changed: value=%q error=%v", value, err)
	}
}
