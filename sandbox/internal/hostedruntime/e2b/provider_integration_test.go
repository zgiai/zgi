//go:build e2b_integration && linux

package e2b

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestE2BContract(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_KEY"))
	templateID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE"))
	if apiKey == "" || templateID == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_E2B_API_KEY and ZGI_AGENT_RUNTIME_E2B_TEST_TEMPLATE are required")
	}
	backend, err := New(Options{
		APIURL: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_API_URL")),
		Domain: strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_E2B_DOMAIN")),
		APIKey: apiKey, Profiles: map[string]Profile{"integration": {TemplateID: templateID}},
		TimeoutSeconds: 300, ReadyTimeout: 90 * time.Second,
	})
	if err != nil {
		t.Fatalf("new E2B backend: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	spec := agentexecution.CreateSpec{
		BindingID:   "e2b-integration-" + fmt.Sprint(time.Now().UnixNano()),
		WorkspaceID: "workspace-integration", ProfileID: "integration",
	}
	allocation, err := backend.CreateBinding(ctx, spec)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := backend.DestroyBinding(cleanupContext, allocation); err != nil {
			t.Errorf("cleanup E2B binding: %v", err)
		}
	})

	content := []byte("managed-runtime-input\n")
	if _, err := backend.UploadFile(ctx, allocation, "input.txt", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("upload file: %v", err)
	}
	var outputMu sync.Mutex
	var stdout, stderr bytes.Buffer
	running, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-job", Command: "/bin/sh",
		Args:    []string{"-c", "cat input.txt > output.txt; printf stdout-ok; printf stderr-ok >&2"},
		Timeout: 30 * time.Second,
	}, func(chunk agentexecution.OutputChunk) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if chunk.Stream == agentexecution.OutputStreamStdout {
			_, _ = stdout.Write(chunk.Data)
		} else {
			_, _ = stderr.Write(chunk.Data)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	result, err := running.Wait(ctx)
	if err != nil || result.ExitCode != 0 || stdout.String() != "stdout-ok" || stderr.String() != "stderr-ok" {
		t.Fatalf("job result=%+v stdout=%q stderr=%q error=%v", result, stdout.String(), stderr.String(), err)
	}

	if err := backend.SuspendBinding(ctx, allocation); err != nil {
		t.Fatalf("suspend binding: %v", err)
	}
	allocation, err = backend.ResumeBinding(ctx, spec, allocation)
	if err != nil {
		t.Fatalf("resume binding: %v", err)
	}
	reader, info, err := backend.DownloadFile(ctx, allocation, "output.txt")
	if err != nil {
		t.Fatalf("download retained file: %v", err)
	}
	value, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || !bytes.Equal(value, content) || info.Size != int64(len(content)) {
		t.Fatalf("retained file=%q info=%+v error=%v", value, info, readErr)
	}

	networkJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-network-denied", Command: "python3",
		Args:    []string{"-c", "import urllib.request; urllib.request.urlopen('https://example.com', timeout=5)"},
		Timeout: 15 * time.Second,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start network test: %v", err)
	}
	networkResult, err := networkJob.Wait(ctx)
	if err != nil || networkResult.ExitCode == 0 {
		t.Fatalf("network escape result=%+v error=%v", networkResult, err)
	}

	cancelJob, err := backend.StartJob(ctx, allocation, agentexecution.JobSpec{
		JobID: "e2b-cancel", Command: "/bin/sh", Args: []string{"-c", "sleep 60 & wait"}, Timeout: time.Minute,
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatalf("start cancellation test: %v", err)
	}
	if err := cancelJob.Cancel(ctx); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	cancelResult, err := cancelJob.Wait(ctx)
	if err != nil || !cancelResult.Canceled {
		t.Fatalf("cancel result=%+v error=%v", cancelResult, err)
	}
}
