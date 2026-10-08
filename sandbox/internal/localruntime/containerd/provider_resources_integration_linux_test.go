//go:build linux && agent_runtime_integration

package containerd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/localruntime"
)

// This is a separate incremental check: it does not repeat the accepted CPU or
// memory matrix. A PID event alone is not proof that an interrupted job stopped.
func TestContainerdGVisorPIDExhaustionTermination(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("PID exhaustion check requires the native worker node")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE must contain a pinned image")
	}
	backend, allocation, cgroup := newResourceBinding(t, image, "pid-termination", 512*1024*1024, 64, 50000)
	const retained = "pid-quarantine-retained"
	if _, err := backend.UploadFile(t.Context(), allocation, "artifacts/retained.txt", strings.NewReader(retained), int64(len(retained))); err != nil {
		t.Fatal(err)
	}
	before := readCgroupCounter(t, filepath.Join(cgroup, "pids.events"), "max")
	running, err := backend.StartJob(t.Context(), allocation, agentexecution.JobSpec{
		JobID: "pid-termination", Command: "python3", Timeout: 20 * time.Second,
		Args: []string{"-c", `import os, time
children = []
try:
    for i in range(96):
        try:
            pid = os.fork()
        except OSError:
            break
        if pid == 0:
            time.sleep(8)
            os._exit(0)
        children.append(pid)
finally:
    for pid in children:
        os.waitpid(pid, 0)`},
	}, func(agentexecution.OutputChunk) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for readCgroupCounter(t, filepath.Join(cgroup, "pids.events"), "max") <= before {
		if time.Now().After(deadline) {
			t.Fatal("PID limit was not reached")
		}
		time.Sleep(50 * time.Millisecond)
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), 5*time.Second)
	result, waitErr := running.Wait(waitCtx)
	cancelWait()
	if waitErr != nil && !result.TerminationFailed && result.ExitCode != -1 {
		t.Fatalf("interrupted PID job reported success: result=%+v err=%v", result, waitErr)
	}
	t.Logf("PID exhaustion terminal classification: exit=%d termination_failed=%v observation_error=%v", result.ExitCode, result.TerminationFailed, waitErr != nil)
	// Exercise the same real provider suspension used by Worker quarantine.
	// The Worker persistence transition is covered separately, not claimed by
	// this provider-level integration test.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelCleanup()
	if err := backend.SuspendBinding(cleanupCtx, allocation); err != nil {
		t.Fatalf("suspend exhausted allocation: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cgroup, "pids.current"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err == nil && strings.TrimSpace(string(data)) != "0" {
		t.Fatalf("quarantined allocation still has processes: %s", data)
	}
	spec := agentexecution.CreateSpec{BindingID: "pid-termination", WorkspaceID: "pid-termination", OrganizationID: "contract-org", SubjectID: "contract-user", ProfileID: "contract"}
	resumed, err := backend.ResumeBinding(t.Context(), spec, allocation)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := backend.DownloadFile(t.Context(), resumed, "artifacts/retained.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 64))
	if err != nil || string(content) != retained {
		t.Fatalf("quarantine lost retained file: content=%q err=%v", content, err)
	}
}

func TestContainerdGVisorResourceLimits(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("containerd gVisor resource test must run as root on the worker node")
	}
	image := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_TEST_IMAGE"))
	if image == "" {
		t.Fatal("ZGI_AGENT_RUNTIME_TEST_IMAGE must contain a preloaded digest-qualified image")
	}

	t.Run("cpu and pids", func(t *testing.T) {
		backend, allocation, cgroup := newResourceBinding(t, image, "cpu", 512*1024*1024, 64, 50000)
		assertCgroupValue(t, filepath.Join(cgroup, "memory.max"), "536870912")
		assertCgroupValue(t, filepath.Join(cgroup, "cpu.max"), "50000 100000")
		assertCgroupValue(t, filepath.Join(cgroup, "pids.max"), "64")

		before := readCgroupCounter(t, filepath.Join(cgroup, "cpu.stat"), "nr_throttled")
		result, output := runContractJob(t, backend, allocation, agentexecution.JobSpec{
			JobID: "cpu", Command: "python3", Timeout: 15 * time.Second,
			Args: []string{"-c", "import time; start=time.process_time(); exec('while time.process_time()-start<1: pass'); print('cpu-ok')"},
		})
		if result.ExitCode != 0 || !strings.Contains(string(output), "cpu-ok") {
			t.Fatalf("CPU job = %+v, output = %q", result, output)
		}
		if after := readCgroupCounter(t, filepath.Join(cgroup, "cpu.stat"), "nr_throttled"); after <= before {
			t.Fatalf("CPU throttling count = %d, want greater than %d", after, before)
		}

		before = readCgroupCounter(t, filepath.Join(cgroup, "pids.events"), "max")
		running, err := backend.StartJob(t.Context(), allocation, agentexecution.JobSpec{
			JobID: "pids", Command: "python3", Timeout: 20 * time.Second,
			Args: []string{"-c", `import errno, os, time
children = []
try:
    for i in range(96):
        try:
            pid = os.fork()
        except OSError as exc:
            print(f"fork-denied={exc.errno} at={i}", flush=True)
            break
        if pid == 0:
            time.sleep(2)
            os._exit(0)
        children.append(pid)
finally:
    for pid in children:
        os.waitpid(pid, 0)
print(f"spawned={len(children)}", flush=True)`},
		}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatalf("start PID job: %v", err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if after := readCgroupCounter(t, filepath.Join(cgroup, "pids.events"), "max"); after > before {
				waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				result, waitErr := running.Wait(waitContext)
				if waitErr != nil {
					// Exhausting the shared cgroup can also interrupt the runtime agent.
					t.Logf("PID limit enforced; job channel closed under exhaustion: %v", waitErr)
				} else {
					t.Logf("PID limit enforced; job result: %+v", result)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		waitContext, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, waitErr := running.Wait(waitContext)
		t.Fatalf("PID limit event count did not increase from %d; job = %+v, wait error = %v", before, result, waitErr)
	})

	t.Run("memory", func(t *testing.T) {
		backend, allocation, cgroup := newResourceBinding(t, image, "mem", 256*1024*1024, 64, 100000)
		assertCgroupValue(t, filepath.Join(cgroup, "memory.max"), "268435456")
		before := readCgroupCounter(t, filepath.Join(cgroup, "memory.events"), "oom_kill")
		running, err := backend.StartJob(t.Context(), allocation, agentexecution.JobSpec{
			JobID: "memory", Command: "python3", Timeout: 20 * time.Second,
			Args: []string{"-c", "x=bytearray(384*1024*1024); print('unexpected-allocation', flush=True)"},
		}, func(agentexecution.OutputChunk) error { return nil })
		if err != nil {
			t.Fatalf("start memory job: %v", err)
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if after := readCgroupCounter(t, filepath.Join(cgroup, "memory.events"), "oom_kill"); after > before {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		waitContext, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, waitErr := running.Wait(waitContext)
		t.Fatalf("memory OOM kill count did not increase from %d; job = %+v, wait error = %v", before, result, waitErr)
	})
}

func newResourceBinding(t *testing.T, image, bindingID string, memory int64, pids, cpuQuota int64) (*localruntime.Backend, agentexecution.Allocation, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "zgi-res-")
	if err != nil {
		t.Fatalf("create resource test directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	provider, err := New(Options{
		Address:      strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_CONTAINERD_ADDRESS")),
		RootDir:      root,
		Profiles:     map[string]Profile{"contract": {Image: image}},
		StartupLimit: 45 * time.Second,
		MemoryBytes:  memory,
		PIDs:         pids,
		CPUPeriod:    100000,
		CPUQuota:     cpuQuota,
	})
	if err != nil {
		t.Fatalf("new resource provider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	backend, err := localruntime.New(provider)
	if err != nil {
		t.Fatalf("new resource backend: %v", err)
	}
	spec := agentexecution.CreateSpec{
		BindingID: bindingID, WorkspaceID: bindingID,
		OrganizationID: "contract-org", SubjectID: "contract-user", ProfileID: "contract",
	}
	allocation, err := backend.CreateBinding(t.Context(), spec)
	if err != nil {
		t.Fatalf("create resource binding: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := backend.DestroyBinding(ctx, allocation); err != nil {
			t.Errorf("destroy resource binding: %v", err)
		}
	})
	container, err := provider.client.LoadContainer(t.Context(), allocation.BindingRef)
	if err != nil {
		t.Fatalf("load resource container: %v", err)
	}
	task, err := container.Task(t.Context(), nil)
	if err != nil {
		t.Fatalf("load resource task: %v", err)
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", task.Pid()))
	if err != nil {
		t.Fatalf("read resource cgroup: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return backend, allocation, filepath.Join("/sys/fs/cgroup", strings.TrimPrefix(path, "/"))
		}
	}
	t.Fatalf("unified cgroup path not found in %q", data)
	return nil, agentexecution.Allocation{}, ""
}

func assertCgroupValue(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func readCgroupCounter(t *testing.T, path, key string) uint64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == key {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				t.Fatalf("parse %s %s: %v", path, key, err)
			}
			return value
		}
	}
	t.Fatalf("%s does not contain counter %q", path, key)
	return 0
}
