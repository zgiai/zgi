package gateway

import (
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestStableInvocationErrorCodeMatchesPublicGenericUpstreamProjection(t *testing.T) {
	if got := stableInvocationErrorCode(adapter.ErrUpstreamError); got != llmerrors.AppCodeInvocationFailed.String() {
		t.Fatalf("stableInvocationErrorCode() = %q, want %q", got, llmerrors.AppCodeInvocationFailed.String())
	}
}
