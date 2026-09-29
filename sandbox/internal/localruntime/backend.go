// Package localruntime connects the Agent execution control plane to an
// isolated environment provider. The provider owns compute and connection
// secrets; only opaque allocation references are persisted by callers.
package localruntime

import (
	"context"
	"fmt"
	"io"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// Provider owns the physical lifecycle and returns a short-lived data-plane
// connection on demand. Connect implementations must not persist credentials
// in Agent execution database records.
type Provider interface {
	Create(context.Context, agentexecution.CreateSpec) (agentexecution.Allocation, error)
	Suspend(context.Context, agentexecution.Allocation) error
	Resume(context.Context, agentexecution.CreateSpec, agentexecution.Allocation) (agentexecution.Allocation, error)
	Destroy(context.Context, agentexecution.Allocation) error
	Connect(context.Context, agentexecution.Allocation) (agentexecution.ExecutionBackend, error)
}

// Backend implements both control-plane and data-plane contracts while
// keeping provider-specific handles and credentials out of those contracts.
type Backend struct {
	provider Provider
}

var (
	_ agentexecution.Backend          = (*Backend)(nil)
	_ agentexecution.ExecutionBackend = (*Backend)(nil)
)

func New(provider Provider) (*Backend, error) {
	if provider == nil {
		return nil, fmt.Errorf("%w: local runtime provider is required", agentexecution.ErrInvalidRequest)
	}
	return &Backend{provider: provider}, nil
}

func (b *Backend) CreateBinding(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	allocation, err := b.provider.Create(ctx, spec)
	if err != nil {
		return allocation, fmt.Errorf("create local runtime environment: %w", err)
	}
	if err := validateAllocation(allocation); err != nil {
		return allocation, err
	}
	return allocation, nil
}

func (b *Backend) SuspendBinding(ctx context.Context, allocation agentexecution.Allocation) error {
	if err := validateAllocation(allocation); err != nil {
		return err
	}
	if err := b.provider.Suspend(ctx, allocation); err != nil {
		return fmt.Errorf("suspend local runtime environment: %w", err)
	}
	return nil
}

func (b *Backend) ResumeBinding(ctx context.Context, spec agentexecution.CreateSpec, allocation agentexecution.Allocation) (agentexecution.Allocation, error) {
	if err := validateAllocation(allocation); err != nil {
		return agentexecution.Allocation{}, err
	}
	resumed, err := b.provider.Resume(ctx, spec, allocation)
	if err != nil {
		return resumed, fmt.Errorf("resume local runtime environment: %w", err)
	}
	if err := validateAllocation(resumed); err != nil {
		return resumed, err
	}
	return resumed, nil
}

func (b *Backend) DestroyBinding(ctx context.Context, allocation agentexecution.Allocation) error {
	if err := validateAllocation(allocation); err != nil {
		return err
	}
	if err := b.provider.Destroy(ctx, allocation); err != nil {
		return fmt.Errorf("destroy local runtime environment: %w", err)
	}
	return nil
}

func (b *Backend) Capabilities(ctx context.Context, allocation agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return agentexecution.RuntimeCapabilities{}, err
	}
	return connection.Capabilities(ctx, allocation)
}

func (b *Backend) StartJob(ctx context.Context, allocation agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return nil, err
	}
	return connection.StartJob(ctx, allocation, spec, sink)
}

func (b *Backend) UploadFile(ctx context.Context, allocation agentexecution.Allocation, path string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	return connection.UploadFile(ctx, allocation, path, reader, size)
}

func (b *Backend) DownloadFile(ctx context.Context, allocation agentexecution.Allocation, path string) (io.ReadCloser, agentexecution.FileInfo, error) {
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	return connection.DownloadFile(ctx, allocation, path)
}

func (b *Backend) connect(ctx context.Context, allocation agentexecution.Allocation) (agentexecution.ExecutionBackend, error) {
	if err := validateAllocation(allocation); err != nil {
		return nil, err
	}
	connection, err := b.provider.Connect(ctx, allocation)
	if err != nil {
		return nil, fmt.Errorf("connect local runtime environment: %w", err)
	}
	if connection == nil {
		return nil, fmt.Errorf("%w: local runtime provider returned no data plane", agentexecution.ErrBackendUnavailable)
	}
	return connection, nil
}

func validateAllocation(allocation agentexecution.Allocation) error {
	if allocation.BindingRef == "" || allocation.WorkspaceRef == "" {
		return fmt.Errorf("%w: local runtime allocation references are required", agentexecution.ErrBackendUnavailable)
	}
	return nil
}
