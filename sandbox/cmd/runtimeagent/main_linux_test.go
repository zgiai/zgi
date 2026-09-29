//go:build linux

package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

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
