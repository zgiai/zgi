package e2b

import (
	"context"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func (b *Backend) PlatformRequests(ctx context.Context, allocation agentexecution.Allocation, jobID string) ([]agentexecution.PlatformRequest, error) {
	if !b.platformCLI {
		return nil, agentexecution.ErrBackendUnavailable
	}
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return nil, err
	}
	return remote.PlatformRequests(ctx, allocation, jobID)
}

func (b *Backend) ResolvePlatformRequest(ctx context.Context, allocation agentexecution.Allocation, jobID string, response agentexecution.PlatformResponse) error {
	if !b.platformCLI {
		return agentexecution.ErrBackendUnavailable
	}
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return err
	}
	return remote.ResolvePlatformRequest(ctx, allocation, jobID, response)
}
