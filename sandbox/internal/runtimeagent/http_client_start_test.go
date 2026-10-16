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

func TestRemoteStartWhoseAnswerIsCutOffIsUnknown(t *testing.T) {
	for _, failure := range []string{"truncated body", "body read timeout"} {
		t.Run(failure, func(t *testing.T) {
			backend := &countingStartBackend{starts: map[string]int{}}
			agent, err := NewHTTPServer(backend, HTTPServerOptions{Token: "test-token"})
			if err != nil {
				t.Fatal(err)
			}
			var cut atomic.Bool
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/v1/jobs" && cut.CompareAndSwap(false, true) {
					// The agent starts the job and its 202 begins, but the body
					// never arrives whole.
					agent.ServeHTTP(httptest.NewRecorder(), r)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Content-Length", "256")
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"data":{"done":fal`))
					w.(http.Flusher).Flush()
					if failure == "body read timeout" {
						<-release
						return
					}
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				agent.ServeHTTP(w, r)
			}))
			defer server.Close()
			defer close(release) // runs first, so Close does not wait on the stalled handler
			client := server.Client()
			client.Timeout = 300 * time.Millisecond
			remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: client, PollInterval: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			spec := agentexecution.JobSpec{JobID: "job-1", Command: "true"}
			sink := func(agentexecution.OutputChunk) error { return nil }
			if _, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink); !errors.Is(err, agentexecution.ErrJobStartUnknown) {
				t.Fatalf("start whose 202 was cut off err = %v, want the outcome reported unknown", err)
			}
			running, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink)
			if err != nil {
				t.Fatalf("retried start: %v", err)
			}
			if result, err := running.Wait(t.Context()); err != nil || result.ExitCode != 0 || backend.startsOf("job-1") != 1 {
				t.Fatalf("attached job result = %+v, err %v, starts %d; want the first execution followed", result, err, backend.startsOf("job-1"))
			}
		})
	}
}

// startingBackend blocks a start, as the agent does while it prepares the
// session, until it is released; it then runs the command unless its start
// context was canceled first (respectContext) or regardless of it.
type startingBackend struct {
	countingStartBackend
	release        chan struct{}
	respectContext bool
	executed       atomic.Bool
	canceled       atomic.Bool
}

func (b *startingBackend) StartJob(ctx context.Context, _ agentexecution.Allocation, spec agentexecution.JobSpec, _ func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
		if b.respectContext {
			return nil, ctx.Err()
		}
		<-b.release
	}
	if b.respectContext && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	b.executed.Store(true)
	return &cancelableRunningJob{backend: b, done: make(chan struct{})}, nil
}

type cancelableRunningJob struct {
	backend *startingBackend
	done    chan struct{}
	once    sync.Once
}

func (j *cancelableRunningJob) Wait(ctx context.Context) (agentexecution.JobResult, error) {
	select {
	case <-j.done:
		return agentexecution.JobResult{ExitCode: -1, Canceled: true}, nil
	case <-ctx.Done():
		return agentexecution.JobResult{}, ctx.Err()
	}
}

func (j *cancelableRunningJob) Cancel(context.Context) error {
	j.backend.canceled.Store(true)
	j.once.Do(func() { close(j.done) })
	return nil
}

func (j *cancelableRunningJob) ResizePTY(context.Context, uint16, uint16) error { return nil }

func TestAgentKeepsACancelSentWhileTheJobIsStarting(t *testing.T) {
	for name, respectContext := range map[string]bool{"start stops on cancel": true, "start completes anyway": false} {
		t.Run(name, func(t *testing.T) {
			backend := &startingBackend{release: make(chan struct{}), respectContext: respectContext}
			agent, err := NewHTTPServer(backend, HTTPServerOptions{Token: "test-token"})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(agent)
			defer server.Close()
			impatient := server.Client()
			impatient.Timeout = 100 * time.Millisecond
			first, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: impatient})
			if err != nil {
				t.Fatal(err)
			}
			remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "test-token", HTTPClient: server.Client(), PollInterval: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			spec := agentexecution.JobSpec{JobID: "job-1", Command: "true"}
			sink := func(agentexecution.OutputChunk) error { return nil }
			// The first start is still preparing when its caller gives up.
			if _, err := first.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink); !errors.Is(err, agentexecution.ErrJobStartUnknown) {
				t.Fatalf("first start err = %v, want unknown", err)
			}
			running, err := remote.StartJob(t.Context(), agentexecution.Allocation{}, spec, sink)
			if err != nil {
				t.Fatalf("reattach to the starting job: %v", err)
			}
			if err := running.Cancel(t.Context()); err != nil {
				t.Fatalf("cancel the starting job: %v", err)
			}
			close(backend.release)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := running.Wait(ctx)
			if err != nil || !result.Canceled {
				t.Fatalf("result = %+v, err %v; want the job canceled", result, err)
			}
			if respectContext && backend.executed.Load() {
				t.Fatal("the command ran although it was canceled while starting")
			}
			if !respectContext && !backend.canceled.Load() {
				t.Fatal("the job that started anyway was never stopped")
			}
		})
	}
}
