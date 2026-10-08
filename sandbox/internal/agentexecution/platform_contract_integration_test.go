//go:build linux && agent_runtime_integration

package agentexecution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/hostedruntime/e2b"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
	"github.com/zgiai/zgi-sandbox/internal/localruntime/containerd"
)

// Run only against a real, explicitly selected backend. Responses below test
// the transport contract, not model billing, business governance or ToolFile
// persistence; those require the API/console acceptance separately.
func TestPlatformCLIRealBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	backend := platformContractBackend(t)
	relay, ok := backend.(agentexecution.PlatformBackend)
	if !ok {
		t.Fatal("backend lacks platform relay")
	}
	identity := fmt.Sprintf("platform-%d", time.Now().UnixNano())
	allocation, err := backend.CreateBinding(ctx, agentexecution.CreateSpec{BindingID: identity, WorkspaceID: identity, ProfileID: "contract", NetworkMode: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		if err := backend.DestroyBinding(cleanup, allocation); err != nil {
			t.Error(err)
		}
	})
	caps, err := backend.Capabilities(ctx, allocation)
	if err != nil || caps.PlatformProtocolVersion != agentexecution.PlatformProtocolVersion {
		t.Fatalf("protocol=%d err=%v", caps.PlatformProtocolVersion, err)
	}
	const script = `set -eu
test "$(id -u)" = 65532
test -z "${ZGI_RUNTIME_AGENT_TOKEN-}"
test ! -w /run/zgi-agent
test ! -r /run/zgi-agent/bootstrap-config.json
test ! -r /run/zgi-agent/token
test ! -r /proc/1/environ
zgi file read --id input --ref admitted-input --path inputs/source.txt
test "$(cat inputs/source.txt)" = platform-input
zgi model call --id model --prompt summarize --max-tokens 32
zgi model call --id model --prompt summarize --max-tokens 32
zgi request status --id model
zgi tool call --id tool --name bound_calculator --arguments '{"value":21}'
mkdir -p artifacts
printf 'platform-result\n' > artifacts/result.txt
zgi file export --id export --path artifacts/result.txt
printf 'platform-stderr\n' >&2
`
	var mu sync.Mutex
	var stdout, stderr bytes.Buffer
	job, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: "platform-contract", Command: "/bin/sh", Args: []string{"-ec", script}, Timeout: time.Minute, Environment: map[string]string{"ZGI_JOB_PLATFORM_ENABLED": "true"}}, func(chunk agentexecution.OutputChunk) error {
		mu.Lock()
		defer mu.Unlock()
		if chunk.Stream == agentexecution.OutputStreamStdout {
			_, err := stdout.Write(chunk.Data)
			return err
		}
		_, err := stderr.Write(chunk.Data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	finished := make(chan struct{})
	var result agentexecution.JobResult
	var waitErr error
	go func() { result, waitErr = job.Wait(ctx); close(finished) }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
poll:
	for {
		select {
		case <-finished:
			break poll
		case <-ctx.Done():
			t.Fatal("platform contract timed out")
		case <-ticker.C:
		}
		requests, err := relay.PlatformRequests(ctx, allocation, "platform-contract")
		if err != nil {
			select {
			case <-finished:
				break poll
			case <-time.After(time.Second):
				mu.Lock()
				out, diagnostics := stdout.String(), stderr.String()
				mu.Unlock()
				t.Fatalf("relay=%v stdout=%q stderr=%q", err, out, diagnostics)
			}
		}
		for _, request := range requests {
			counts[request.ID]++
			if counts[request.ID] != 1 {
				t.Fatal("duplicate handler dispatch")
			}
			response := agentexecution.PlatformResponse{ID: request.ID, Status: "completed"}
			switch request.ID {
			case "input":
				if request.Action != "file.read" {
					t.Fatal("wrong input action")
				}
				data := "platform-input\n"
				if _, err := backend.UploadFile(ctx, allocation, "inputs/source.txt", strings.NewReader(data), int64(len(data))); err != nil {
					t.Fatal(err)
				}
				response.Data = json.RawMessage(`{"workspace_path":"inputs/source.txt"}`)
			case "model":
				response.Data = json.RawMessage(`{"text":"transport-model-result"}`)
			case "tool":
				response.Data = json.RawMessage(`{"result":42}`)
			case "export":
				reader, _, err := backend.DownloadFile(ctx, allocation, "artifacts/result.txt")
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := io.ReadAll(reader)
				closeErr := reader.Close()
				if readErr != nil || closeErr != nil || string(data) != "platform-result\n" {
					t.Fatalf("export bytes=%q read=%v close=%v", data, readErr, closeErr)
				}
				response.Data = json.RawMessage(`{"file_id":"transport-only"}`)
			default:
				t.Fatalf("unexpected request %q", request.ID)
			}
			if err := relay.ResolvePlatformRequest(ctx, allocation, "platform-contract", response); err != nil {
				t.Fatal(err)
			}
		}
	}
	mu.Lock()
	out, diagnostics := stdout.String(), stderr.String()
	mu.Unlock()
	if waitErr != nil || result.ExitCode != 0 || len(counts) != 4 || !strings.Contains(out, `"result":42`) || strings.Count(out, "transport-model-result") != 3 || diagnostics != "platform-stderr\n" {
		t.Fatalf("result=%+v err=%v calls=%v stdout=%q stderr=%q", result, waitErr, counts, out, diagnostics)
	}
	if _, err := relay.PlatformRequests(ctx, allocation, "platform-contract"); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("finished Job retained platform access: %v", err)
	}
	t.Log("real backend CLI: non-root, protected control credentials, file bytes, four relay actions, duplicate receipt, diagnostics and grant cleanup passed")

	// Stop a Job while its CLI waits for a control response; the socket grant
	// must disappear rather than leave an orphaned or retryable mutation.
	waiting, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{JobID: "platform-cancel", Command: "/usr/local/bin/zgi", Args: []string{"model", "call", "--id", "cancel", "--prompt", "waiting"}, Timeout: 30 * time.Second, Environment: map[string]string{"ZGI_JOB_PLATFORM_ENABLED": "true"}}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for {
		requests, err := relay.PlatformRequests(ctx, allocation, "platform-cancel")
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("cancel request did not arrive")
		case <-ticker.C:
		}
	}
	if err := waiting.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	if result, err := waiting.Wait(ctx); err != nil || result.TerminationFailed {
		t.Fatalf("unsafe cancellation: result=%+v err=%v", result, err)
	}
	if _, err := relay.PlatformRequests(ctx, allocation, "platform-cancel"); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("cancelled Job retained platform access: %v", err)
	}
	t.Log("real backend Stop revoked pending CLI request and confirmed process termination")
}

type platformBackendContract interface {
	agentexecution.Backend
	agentexecution.ExecutionBackend
}

func platformContractBackend(t *testing.T) platformBackendContract {
	t.Helper()
	switch os.Getenv("ZGI_PLATFORM_TEST_BACKEND") {
	case "local-gvisor":
		if os.Geteuid() != 0 || os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE") == "" {
			t.Fatal("native root Worker and digest-qualified ZGI_AGENT_RUNTIME_TEST_IMAGE required")
		}
		provider, err := containerd.New(containerd.Options{PlatformCLI: true, Address: os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS"), RootDir: t.TempDir(), Profiles: map[string]containerd.Profile{"contract": {Image: os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE")}}, MemoryBytes: 384 << 20, PIDs: 128, StartupLimit: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = provider.Close() })
		backend, err := localruntime.New(provider)
		if err != nil {
			t.Fatal(err)
		}
		return backend
	case "e2b":
		backend, err := e2b.New(e2b.Options{PlatformCLI: true, APIKey: os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY"), Profiles: map[string]e2b.Profile{"contract": {TemplateID: os.Getenv("ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE")}}, TimeoutSeconds: 300, ReadyTimeout: 90 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return backend
	default:
		t.Fatal("ZGI_PLATFORM_TEST_BACKEND must select local-gvisor or e2b; mocks cannot satisfy this test")
		return nil
	}
}
