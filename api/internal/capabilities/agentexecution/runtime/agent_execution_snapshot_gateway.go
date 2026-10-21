package executionruntime

import "context"

// The snapshot is fixed by the approved published Agent configuration. Tool
// arguments never get to select or replace the base environment.
type agentExecutionSnapshotGateway struct {
	agentExecutionGateway
	homeSnapshotID string
}

func (g agentExecutionSnapshotGateway) CreateBinding(ctx context.Context, scope agentExecutionScope, agentID, conversationID, profileID, backend string, options ...string) (agentExecutionBinding, error) {
	if len(options) == 0 {
		return g.agentExecutionGateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend, g.homeSnapshotID)
	}
	return g.agentExecutionGateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend, g.homeSnapshotID, options[len(options)-1])
}
