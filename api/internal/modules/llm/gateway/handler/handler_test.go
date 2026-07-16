package handler

import (
	"net/http"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
)

func TestClassifyProtocolErrorPreservesUpstreamGuardCode(t *testing.T) {
	got := classifyProtocolError(llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	if got.openAIStatus != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", got.openAIStatus, http.StatusServiceUnavailable)
	}
	if got.openAICode != "private_channel_upstream_unavailable" {
		t.Fatalf("code = %q, want private_channel_upstream_unavailable", got.openAICode)
	}
}
