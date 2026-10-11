package localruntime

import (
	"context"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func (b *Backend) platformCLIEnabled() bool {
	admission, ok := b.provider.(interface{ PlatformCLIEnabled() bool })
	return ok && admission.PlatformCLIEnabled()
}

func (b *Backend) PlatformRequests(ctx context.Context, allocation agentexecution.Allocation, jobID string) ([]agentexecution.PlatformRequest, error) {
	if !b.platformCLIEnabled() {
		return nil, agentexecution.ErrBackendUnavailable
	}
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return nil, err
	}
	backend, ok := connection.(agentexecution.PlatformBackend)
	if !ok {
		return nil, agentexecution.ErrBackendUnavailable
	}
	return backend.PlatformRequests(ctx, allocation, jobID)
}

func (b *Backend) ResolvePlatformRequest(ctx context.Context, allocation agentexecution.Allocation, jobID string, response agentexecution.PlatformResponse) error {
	if !b.platformCLIEnabled() {
		return agentexecution.ErrBackendUnavailable
	}
	connection, err := b.connect(ctx, allocation)
	if err != nil {
		return err
	}
	backend, ok := connection.(agentexecution.PlatformBackend)
	if !ok {
		return agentexecution.ErrBackendUnavailable
	}
	return backend.ResolvePlatformRequest(ctx, allocation, jobID, response)
}
