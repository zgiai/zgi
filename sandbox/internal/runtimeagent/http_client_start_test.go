package runtimeagent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// countingStartBackend runs every started job to completion at once and
// counts how many times a job was really started.
type countingStartBackend struct {
	mu     sync.Mutex
	starts map[string]int
}

func (b *countingStartBackend) Capabilities(context.Context, agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	return agentexecution.RuntimeCapabilities{Shell: true}, nil
}

func (b *countingStartBackend) StartJob(_ context.Context, _ agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	b.mu.Lock()
	b.starts[spec.JobID]++
	b.mu.Unlock()
	if err := sink(agentexecution.OutputChunk{JobID: spec.JobID, Sequence: 0, Stream: agentexecution.OutputStreamStdout, Data: []byte("ran\n")}); err != nil {
		return nil, err
	}
	return finishedRunningJob{}, nil
}

func (b *countingStartBackend) UploadFile(context.Context, agentexecution.Allocation, string, io.Reader, int64) (agentexecution.FileInfo, error) {
	return agentexecution.FileInfo{}, agentexecution.ErrInvalidRequest
}

func (b *countingStartBackend) DownloadFile(context.Context, agentexecution.Allocation, string) (io.ReadCloser, agentexecution.FileInfo, error) {
	return nil, agentexecution.FileInfo{}, agentexecution.ErrNotFound
}

func (b *countingStartBackend) startsOf(jobID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts[jobID]
}

type finishedRunningJob struct{}

func (finishedRunningJob) Wait(context.Context) (agentexecution.JobResult, error) {
	return agentexecution.JobResult{ExitCode: 0}, nil
}
func (finishedRunningJob) Cancel(context.Context) error                    { return nil }
func (finishedRunningJob) ResizePTY(context.Context, uint16, uint16) error { return nil }

func TestRemoteStartWhoseAnswerIsLostAttachesToTheJobItStarted(t *testing.T) {
	backend := &countingStartBackend{starts: map[string]int{}}
	agent, err := NewHTTPServer(backend, HTTPServerOptions{Token: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	var dropped atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/jobs" && dropped.CompareAndSwap(false, true) {
			// The agent accepts and starts the job, but its answer is lost.
			agent.ServeHTTP(httptest.NewRecorder(), r)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		agent.ServeHTTP(w, r)
	}))
	defer server.Close()
	remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client(), PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	spec := agentexecution.JobSpec{JobID: "job-1", Command: "true"}
	var output []string
	sink := func(chunk agentexecution.OutputChunk) error {
		output = append(output, string(chunk.Data))
		return nil
	}

	if _, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink); !errors.Is(err, agentexecution.ErrJobStartUnknown) {
		t.Fatalf("start with a lost answer err = %v, want the outcome reported unknown", err)
	}
	if backend.startsOf("job-1") != 1 {
		t.Fatalf("agent started the job %d times, want it running although the answer was lost", backend.startsOf("job-1"))
	}

	running, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink)
	if err != nil {
		t.Fatalf("retried start: %v", err)
	}
	result, err := running.Wait(t.Context())
	if err != nil || result.ExitCode != 0 || backend.startsOf("job-1") != 1 || len(output) != 1 || output[0] != "ran\n" {
		t.Fatalf("attached job result = %+v, err %v, starts %d, output %q; want the first execution followed, not a second one", result, err, backend.startsOf("job-1"), output)
	}
}

func TestRemoteStartRejectedByTheAgentIsNotUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"idempotency_conflict","message":"conflict"}}`))
	}))
	defer server.Close()
	remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = remote.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "job-1", Command: "true"}, func(agentexecution.OutputChunk) error { return nil })
	if err == nil || errors.Is(err, agentexecution.ErrJobStartUnknown) {
		t.Fatalf("rejected start err = %v, want a definite failure", err)
	}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer gateway.Close()
	remote, err = NewRemote(RemoteOptions{BaseURL: gateway.URL, Token: "test-token", HTTPClient: gateway.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, agentexecution.JobSpec{JobID: "job-1", Command: "true"}, func(agentexecution.OutputChunk) error { return nil }); !errors.Is(err, agentexecution.ErrJobStartUnknown) {
		t.Fatalf("start behind a failing gateway err = %v, want the outcome reported unknown", err)
	}
}
