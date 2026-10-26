package execution

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type backgroundAuthorizerSpy struct {
	calls   int
	allow   bool
	request BackgroundAuthorizationRequest
}

func (s *backgroundAuthorizerSpy) AuthorizeBackground(_ context.Context, r BackgroundAuthorizationRequest) (bool, error) {
	s.calls++
	s.request = r
	return s.allow, nil
}

func TestBoundedBackgroundAuthorizationPrecedesCredentials(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, map[string]any{"command": "uptime"}, false)
	h := newDigitalHumanServerOpsHarness(t, request)
	spy := &backgroundAuthorizerSpy{allow: true}
	h.service.WithBackgroundAuthorizer(spy)
	result, err := h.service.InvokeConnector(context.Background(), h.pinned)
	if err != nil || result.State == ExecutionStateBlocked || h.provider.calls != 1 {
		t.Fatalf("authorized background invocation failed: %#v %v", result, err)
	}
	if spy.calls != 1 || spy.request.Binding.ID != request.BindingID || spy.request.Connection.ID != request.ConnectionID || spy.request.Request.LogicalOperationID != request.LogicalOperationID || spy.request.ContractDigest == "" {
		t.Fatalf("incomplete restored authority: %#v", spy)
	}
	if !h.audit.decisions[0].Details["preauthorized"].(bool) {
		t.Fatal("background authority missing from audit")
	}
}
func TestBackgroundAuthorizationDoesNotReplaceInteractiveOrDraftGate(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, map[string]any{}, interactive)
		if !interactive {
			request.Target.Scope = core.ScopeDraft
			request.Target.VersionID = ""
		}
		h := newDigitalHumanServerOpsHarness(t, request)
		spy := &backgroundAuthorizerSpy{allow: true}
		h.service.WithBackgroundAuthorizer(spy)
		_, _ = h.service.InvokeConnector(context.Background(), h.pinned)
		if spy.calls != 0 || h.provider.calls != 0 || h.credentials.calls != 0 {
			t.Fatal("invalid background path reached provider", interactive, spy.calls)
		}
	}
}
func TestNoBackgroundGrantPreservesBlocking(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, map[string]any{}, false)
	h := newDigitalHumanServerOpsHarness(t, request)
	spy := &backgroundAuthorizerSpy{}
	h.service.WithBackgroundAuthorizer(spy)
	result, err := h.service.InvokeConnector(context.Background(), h.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) || result.State != ExecutionStateBlocked || h.provider.calls != 0 || h.credentials.calls != 0 {
		t.Fatal(result, err)
	}
	if spy.calls != 1 {
		t.Fatal("background lookup absent")
	}

}

func TestTrustedLowBackgroundStillNeedsExplicitGrant(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, false)
		h := newDigitalHumanServerOpsHarness(t, request)
		h.risk.classification = RiskTrustedLow
		spy := &backgroundAuthorizerSpy{allow: allowed}
		h.service.WithBackgroundAuthorizer(spy)
		_, err := h.service.InvokeConnector(context.Background(), h.pinned)
		if allowed && (err != nil || h.provider.calls != 1) {
			t.Fatal("explicit read grant rejected", err)
		}
		if !allowed && (h.provider.calls != 0 || h.credentials.calls != 0 || err == nil) {
			t.Fatal("trusted-low bypassed grant")
		}
	}
}
