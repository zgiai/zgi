//go:build linux

package runtimeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/zgiai/zgi-sandbox/internal/platformcli"
)

func TestPlatformCLIChild(t *testing.T) {
	if os.Getenv("ZGI_TEST_PLATFORM_CHILD") != "true" {
		return
	}
	client, err := platformcli.NewClient(os.Getenv("ZGI_PLATFORM_SOCKET"), os.Getenv("ZGI_PLATFORM_JOB_ID"), os.Getenv("ZGI_PLATFORM_TOKEN"))
	if err != nil {
		os.Exit(3)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	exit := platformcli.Run(ctx, client, []string{"file", "export", "--id", "export-1", "--path", "result.txt"}, os.Stdout, os.Stderr)
	os.Exit(exit)
}

func TestPlatformJobUsesSeparateUnixSocketAndAuthenticatedControlRelay(t *testing.T) {
	root := t.TempDir()
	socket := filepath.Join(root, "platform.sock")
	backend, err := New(Options{WorkspaceRoot: filepath.Join(root, "workspace"), NetworkMode: "disabled", PlatformCLI: true, PlatformSocketPath: socket})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	platformServer := &http.Server{Handler: backend.PlatformHandler(), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- platformServer.Serve(listener) }()
	defer func() {
		_ = platformServer.Close()
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	}()
	handler, err := NewHTTPServer(backend, HTTPServerOptions{Token: "root-control-test-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "root-control-test-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	var outputMu sync.Mutex
	job, err := remote.StartJob(ctx, agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "platform-job", Command: executable, Args: []string{"-test.run=^TestPlatformCLIChild$"}, Timeout: 4 * time.Second,
		Environment: map[string]string{"ZGI_JOB_PLATFORM_ENABLED": "true", "ZGI_TEST_PLATFORM_CHILD": "true", "ZGI_PLATFORM_TOKEN": "caller-forged-token"},
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		_, err := output.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		requests, err := remote.PlatformRequests(ctx, agentexecution.Allocation{}, "platform-job")
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) > 0 {
			if len(requests) != 1 || requests[0].ID != "export-1" {
				t.Fatalf("requests=%+v", requests)
			}
			if err := remote.ResolvePlatformRequest(ctx, agentexecution.Allocation{}, "platform-job", agentexecution.PlatformResponse{ID: "export-1", Status: "completed", Data: json.RawMessage(`{"file_id":"test-file"}`)}); err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("Job request did not reach control channel")
		}
	}
	result, err := job.Wait(ctx)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	outputMu.Lock()
	got := output.String()
	outputMu.Unlock()
	if !strings.Contains(got, `"file_id":"test-file"`) || strings.Contains(got, "caller-forged-token") || strings.Contains(got, "root-control-test-token") {
		t.Fatalf("output=%q", got)
	}
	if _, err := backend.platform.Pending("platform-job"); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("finished Job retained a grant: %v", err)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/jobs/platform-job/platform-requests", nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("privileged relay missing auth: HTTP %d", response.StatusCode)
	}
}

func TestPlatformDisabledAndCallerGrantsCannotEnableIt(t *testing.T) {
	backend := newTestBackend(t)
	capabilities, err := backend.Capabilities(t.Context(), agentexecution.Allocation{})
	if err != nil || capabilities.PlatformProtocolVersion != 0 {
		t.Fatalf("legacy capabilities=%+v err=%v", capabilities, err)
	}
	if _, err := backend.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "denied-platform", Command: "/bin/true", Timeout: time.Second, Environment: map[string]string{"ZGI_JOB_PLATFORM_ENABLED": "true"},
	}, func(agentexecution.OutputChunk) error { return nil }); !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("unapproved platform enabled: %v", err)
	}
	var output bytes.Buffer
	var outputMu sync.Mutex
	job, err := backend.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{
		JobID: "forged-platform", Command: "/bin/sh", Args: []string{"-c", `printf '%s' "${ZGI_PLATFORM_TOKEN-unavailable}"`}, Environment: map[string]string{"ZGI_PLATFORM_TOKEN": "forged"},
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		_, err := output.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := job.Wait(t.Context()); err != nil || result.ExitCode != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	outputMu.Lock()
	defer outputMu.Unlock()
	if output.String() != "unavailable" {
		t.Fatalf("caller grant inherited: %q", output.String())
	}
}
