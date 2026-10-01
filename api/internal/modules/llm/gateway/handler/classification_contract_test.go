package handler

import (
	"errors"
	"fmt"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// TestPublishedErrorTripleMatchesCatalogAndLedger feeds the same error to the
// public projection and to the ledger classifier: the published code must be
// the ledger's, and (HTTP status, retryable) must be the catalog's for it.
func TestPublishedErrorTripleMatchesCatalogAndLedger(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	catalog := map[string]struct {
		status    int
		retryable bool
	}{}
	for _, definition := range append(llmerrors.CatalogDefinitions(), llmerrors.BillingCatalogDefinitions()...) {
		catalog[definition.Code.String()] = struct {
			status    int
			retryable bool
		}{definition.HTTPStatus, definition.Retryable}
	}
	inputs := map[string]struct {
		err  error
		want string
	}{
		"upstream error":            {adapter.ErrUpstreamError, "llm.invocation.failed"},
		"upstream auth failed":      {adapter.ErrAuthFailed, "llm.provider.auth_failed"},
		"upstream 429":              {adapter.ErrRateLimited, "llm.provider.rate_limited"},
		"own rate limit":            {llmerrors.DomainErrRateLimitExceeded, "llm.rate_limit.exceeded"},
		"platform channel down":     {adapter.ErrPlatformChannelUnavailable, "llm.provider.unavailable"},
		"proxy error":               {adapter.ErrProxyError, "llm.provider.unavailable"},
		"billing unavailable":       {adapter.ErrBillingUnavailable, "llm.provider.unavailable"},
		"model not found":           {gateway.ErrModelNotFound, "llm.model.not_found"},
		"balance not found":         {gateway.ErrBalanceNotFound, "llm.balance.insufficient"},
		"billing failed":            {gateway.ErrBillingFailed, "llm.invocation.failed"},
		"private channel down":      {llmerrors.DomainErrPrivateChannelUpstreamUnavailable, "llm.channel.private_unavailable"},
		"upstream says unsupported": {fmt.Errorf("upstream said: model not supported"), "llm.request.invalid"},
		"bare transport error":      {errors.New("dial tcp: connection refused"), "llm.invocation.failed"},
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			c, _ := newT17Context("en-US")
			wire := handler.localizedProtocolError(c, input.err)
			ledger, _ := gateway.ApplicationCodeForError(input.err)
			if wire.appCode != input.want || ledger.String() != input.want {
				t.Fatalf("wire code %q, ledger code %q, want %q", wire.appCode, ledger.String(), input.want)
			}
			entry, ok := catalog[input.want]
			if !ok {
				t.Fatalf("%s is not cataloged", input.want)
			}
			if wire.openAIStatus != entry.status || wire.retryable != entry.retryable {
				t.Fatalf("wire (%d, %v), catalog (%d, %v)", wire.openAIStatus, wire.retryable, entry.status, entry.retryable)
			}
		})
	}
}

// internalProtocolError publishes llm.invocation.failed with the catalog's 502.
func TestInternalProtocolErrorUsesTheCatalogStatus(t *testing.T) {
	if got := internalProtocolError(); got.openAIStatus != 502 || got.appCode != "llm.invocation.failed" {
		t.Fatalf("internalProtocolError() = (%d, %s), want (502, llm.invocation.failed)", got.openAIStatus, got.appCode)
	}
}
