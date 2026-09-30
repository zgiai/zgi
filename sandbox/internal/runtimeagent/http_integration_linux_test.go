//go:build linux

package runtimeagent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestRemoteWorkspaceArchiveRoundTripAndFailureTrailer(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "result.json"), []byte(`{"total":3}`), 0o640); err != nil {
		t.Fatal(err)
	}
	source, err := New(Options{WorkspaceRoot: sourceDir})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	handler, err := NewHTTPServer(source, HTTPServerOptions{Token: "archive-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "archive-token"})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := remote.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	destinationDir := t.TempDir()
	destination, err := New(Options{WorkspaceRoot: destinationDir})
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	destinationHandler, err := NewHTTPServer(destination, HTTPServerOptions{Token: "archive-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer destinationHandler.Close()
	destinationServer := httptest.NewServer(destinationHandler)
	defer destinationServer.Close()
	destinationRemote, err := NewRemote(RemoteOptions{BaseURL: destinationServer.URL, Token: "archive-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := destinationRemote.ImportWorkspace(context.Background(), agentexecution.Allocation{}, bytes.NewReader(archive)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destinationDir, "result.json"))
	if err != nil || string(got) != `{"total":3}` {
		t.Fatalf("restored file = %q, %v", got, err)
	}
	if err := os.Symlink("result.json", filepath.Join(sourceDir, "unsafe-link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	broken, err := remote.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, broken)
	_ = broken.Close()
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("failed archive error = %v", err)
	}
}

func TestRemoteBackendRunsRuntimeAgentContract(t *testing.T) {
	local := newTestBackend(t)
	handler, err := NewHTTPServer(local, HTTPServerOptions{Token: "runtime-token"})
	if err != nil {
		t.Fatalf("new HTTP server: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	unauthorized, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "wrong-token"})
	if err != nil {
		t.Fatalf("new unauthorized client: %v", err)
	}
	if _, err := unauthorized.Capabilities(context.Background(), agentexecution.Allocation{}); err == nil {
		t.Fatal("invalid runtime token was accepted")
	}

	remote, err := NewRemote(RemoteOptions{
		BaseURL: server.URL, Token: "runtime-token", PollInterval: 5 * time.Millisecond,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("new remote backend: %v", err)
	}
	capabilities, err := remote.Capabilities(context.Background(), agentexecution.Allocation{})
	if err != nil || !capabilities.PTY || !capabilities.FileTransfer {
		t.Fatalf("capabilities=%+v error=%v", capabilities, err)
	}

	content := []byte("remote-artifact")
	info, err := remote.UploadFile(context.Background(), agentexecution.Allocation{}, "input/data.txt", bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("remote upload: %v", err)
	}
	reader, downloaded, err := remote.DownloadFile(context.Background(), agentexecution.Allocation{}, info.Path)
	if err != nil {
		t.Fatalf("remote download: %v", err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(value, content) || downloaded.Size != int64(len(content)) {
		t.Fatalf("download=%q info=%+v error=%v", value, downloaded, readErr)
	}

	var outputMu sync.Mutex
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sink := func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if chunk.Stream == agentexecution.OutputStreamStdout {
			_, _ = stdout.Write(chunk.Data)
		} else {
			_, _ = stderr.Write(chunk.Data)
		}
		return nil
	}
	spec := agentexecution.JobSpec{
		JobID: "remote-job", Command: "/bin/sh",
		Args: []string{"-c", "printf 'hello'; printf 'warning' >&2"}, Timeout: time.Second,
	}
	running, err := remote.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink)
	if err != nil {
		t.Fatalf("start remote job: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || result.ExitCode != 0 || stdout.String() != "hello" || stderr.String() != "warning" {
		t.Fatalf("result=%+v stdout=%q stderr=%q error=%v", result, stdout.String(), stderr.String(), err)
	}

	same, err := remote.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink)
	if err != nil {
		t.Fatalf("repeat remote job: %v", err)
	}
	if result, err := same.Wait(context.Background()); err != nil || result.ExitCode != 0 {
		t.Fatalf("wait repeated job: result=%+v error=%v", result, err)
	}
	spec.Command = "/bin/false"
	if _, err := remote.StartJob(context.Background(), agentexecution.Allocation{}, spec, sink); !errors.Is(err, agentexecution.ErrIdempotencyConflict) {
		t.Fatalf("changed remote job error = %v", err)
	}
}

func TestUnixRemoteBackendContract(t *testing.T) {
	local := newTestBackend(t)
	handler, err := NewHTTPServer(local, HTTPServerOptions{Token: "unix-secret"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	socketPath := filepath.Join(t.TempDir(), "runtime.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	remote, err := NewUnixRemote(socketPath, "unix-secret", 5*time.Millisecond)
	if err != nil {
		t.Fatalf("new Unix remote: %v", err)
	}
	capabilities, err := remote.Capabilities(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !capabilities.Shell || capabilities.NetworkMode != "disabled" {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}

func TestRemoteBackendAddsProxyHeadersWithoutOverridingAuthorization(t *testing.T) {
	local := newTestBackend(t)
	handler, err := NewHTTPServer(local, HTTPServerOptions{Token: "runtime-token"})
	if err != nil {
		t.Fatalf("new HTTP server: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("E2B-Traffic-Access-Token") != "traffic-token" {
			http.Error(writer, "missing traffic token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)

	remote, err := NewRemote(RemoteOptions{
		BaseURL: server.URL, Token: "runtime-token",
		Headers: http.Header{"E2B-Traffic-Access-Token": []string{"traffic-token"}},
	})
	if err != nil {
		t.Fatalf("new remote backend: %v", err)
	}
	if _, err := remote.Capabilities(context.Background(), agentexecution.Allocation{}); err != nil {
		t.Fatalf("capabilities: %v", err)
	}

	_, err = NewRemote(RemoteOptions{
		BaseURL: server.URL, Token: "runtime-token",
		Headers: http.Header{"Authorization": []string{"Bearer attacker"}},
	})
	if !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("authorization override error = %v", err)
	}
}

func TestRemoteBackendCancelsAndResizesPTY(t *testing.T) {
	local := newTestBackend(t)
	handler, err := NewHTTPServer(local, HTTPServerOptions{Token: "runtime-token"})
	if err != nil {
		t.Fatalf("new HTTP server: %v", err)
	}
	t.Cleanup(func() { _ = handler.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "runtime-token", PollInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("new remote backend: %v", err)
	}

	var output bytes.Buffer
	var outputMu sync.Mutex
	running, err := remote.StartJob(context.Background(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "remote-pty", Command: "/bin/sh", Args: []string{"-c", "printf ready; sleep 30"},
		PTY: true, PTYColumns: 80, PTYRows: 24, Timeout: time.Minute,
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		_, _ = output.Write(chunk.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("start remote PTY: %v", err)
	}
	if err := running.ResizePTY(context.Background(), 100, 40); err != nil {
		t.Fatalf("resize remote PTY: %v", err)
	}
	if err := running.Cancel(context.Background()); err != nil {
		t.Fatalf("cancel remote PTY: %v", err)
	}
	result, err := running.Wait(context.Background())
	if err != nil || !result.Canceled {
		t.Fatalf("canceled result=%+v error=%v", result, err)
	}
	outputMu.Lock()
	gotOutput := output.String()
	outputMu.Unlock()
	if gotOutput != "" && !strings.Contains(gotOutput, "ready") {
		t.Fatalf("unexpected PTY output %q", gotOutput)
	}
}
