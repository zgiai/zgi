package providers

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ProbeMessaging performs the provider half of setup without committing a
// Connection. The worker must call it outside a database transaction, then
// authenticate and persist its result before a separate promotion transaction.
func (runtime *Runtime) ProbeMessaging(ctx context.Context, request connections.CandidateVerificationRequest) (connections.CandidateVerificationResult, error) {
	if runtime == nil {
		return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	entry, ok := runtime.drivers[request.ConnectorID]
	if !ok || !entry.definition.IsMessagingCredentials() {
		return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
	}
	return runtime.Verify(ctx, request)
}
