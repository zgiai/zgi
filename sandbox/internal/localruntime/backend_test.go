package localruntime

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestBackendDelegatesLifecycleAndDataPlane(t *testing.T) {
	execution := &fakeExecutionBackend{files: map[string][]byte{}}
	provider := &fakeProvider{execution: execution}
	backend, err := New(provider)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	allocation, err := backend.CreateBinding(context.Background(), agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1"})
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if _, err := backend.Capabilities(context.Background(), allocation); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if _, err := backend.UploadFile(context.Background(), allocation, "input.txt", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("upload: %v", err)
	}
	reader, info, err := backend.DownloadFile(context.Background(), allocation, "input.txt")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer reader.Close()
	value, _ := io.ReadAll(reader)
	if info.Size != 5 || string(value) != "hello" {
		t.Fatalf("download = %q (%d)", value, info.Size)
	}
	if err := backend.SuspendBinding(context.Background(), allocation); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	resumed, err := backend.ResumeBinding(context.Background(), agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1"}, allocation)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.BindingRef != "binding-2" {
		t.Fatalf("resumed binding ref = %q", resumed.BindingRef)
	}
	if err := backend.DestroyBinding(context.Background(), resumed); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if provider.connectCalls != 3 || provider.suspendCalls != 1 || provider.resumeCalls != 1 || provider.destroyCalls != 1 {
		t.Fatalf("provider calls = connect:%d suspend:%d resume:%d destroy:%d", provider.connectCalls, provider.suspendCalls, provider.resumeCalls, provider.destroyCalls)
	}
}

func TestBackendRejectsIncompleteProviderAllocation(t *testing.T) {
	backend, err := New(&fakeProvider{createAllocation: agentexecution.Allocation{BindingRef: "binding-only"}})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	allocation, err := backend.CreateBinding(context.Background(), agentexecution.CreateSpec{BindingID: "binding-1"})
	if err == nil || allocation.BindingRef == "" {
		t.Fatalf("create = %+v, %v", allocation, err)
	}
}

type fakeProvider struct {
	execution        agentexecution.ExecutionBackend
	createAllocation agentexecution.Allocation
	connectCalls     int
	suspendCalls     int
	resumeCalls      int
	destroyCalls     int
}

func (p *fakeProvider) Create(context.Context, agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	if p.createAllocation.BindingRef != "" || p.createAllocation.WorkspaceRef != "" {
		return p.createAllocation, nil
	}
	return agentexecution.Allocation{BindingRef: "binding-1", WorkspaceRef: "workspace-1"}, nil
}

func (p *fakeProvider) Suspend(context.Context, agentexecution.Allocation) error {
	p.suspendCalls++
	return nil
}

func (p *fakeProvider) Resume(context.Context, agentexecution.CreateSpec, agentexecution.Allocation) (agentexecution.Allocation, error) {
	p.resumeCalls++
	return agentexecution.Allocation{BindingRef: "binding-2", WorkspaceRef: "workspace-1"}, nil
}

func (p *fakeProvider) Destroy(context.Context, agentexecution.Allocation) error {
	p.destroyCalls++
	return nil
}

func (p *fakeProvider) Connect(context.Context, agentexecution.Allocation) (agentexecution.ExecutionBackend, error) {
	p.connectCalls++
	return p.execution, nil
}

type fakeExecutionBackend struct {
	files map[string][]byte
}

func (b *fakeExecutionBackend) Capabilities(context.Context, agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	return agentexecution.RuntimeCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled"}, nil
}

func (b *fakeExecutionBackend) StartJob(context.Context, agentexecution.Allocation, agentexecution.JobSpec, func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	return nil, nil
}

func (b *fakeExecutionBackend) UploadFile(_ context.Context, _ agentexecution.Allocation, path string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	value, err := io.ReadAll(io.LimitReader(reader, size))
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	b.files[path] = value
	return agentexecution.FileInfo{Path: path, Size: int64(len(value))}, nil
}

func (b *fakeExecutionBackend) DownloadFile(_ context.Context, _ agentexecution.Allocation, path string) (io.ReadCloser, agentexecution.FileInfo, error) {
	value := b.files[path]
	return io.NopCloser(bytes.NewReader(value)), agentexecution.FileInfo{Path: path, Size: int64(len(value))}, nil
}
