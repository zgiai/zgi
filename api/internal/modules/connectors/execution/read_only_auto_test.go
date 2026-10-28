package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func readOnlyAutoHarness(t *testing.T, operationID string, input any, interactive bool, mode string) *executionHarness {
	t.Helper()
	harness := newDigitalHumanServerOpsHarness(t, digitalHumanServerOpsRequest(operationID, input, interactive))
	harness.bindings.binding.Constraint.AuthorizationMode = mode
	return harness
}

func TestReadOnlyAutoRunsAProvenReadWithoutConfirmation(t *testing.T) {
	for name, harness := range map[string]*executionHarness{
		"command":  readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h && systemctl status nginx"}, true, core.AuthorizationModeReadOnlyAuto),
		"snapshot": readOnlyAutoHarness(t, serverops.ActionSystemSnapshot, nil, true, core.AuthorizationModeReadOnlyAuto),
	} {
		result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
		if err != nil || result.State != ExecutionStateCompleted || result.Confirmation != nil {
			t.Fatalf("%s: InvokeConnector() = %#v, %v", name, result, err)
		}
		if harness.provider.calls != 1 || len(harness.audit.outcomes) != 1 || len(harness.audit.decisions) != 1 {
			t.Fatalf("%s: provider/outcomes/decisions = %d/%d/%d", name,
				harness.provider.calls, len(harness.audit.outcomes), len(harness.audit.decisions))
		}
	}
}

func TestReadOnlyAutoStillAsksEverythingElse(t *testing.T) {
	cases := map[string]*executionHarness{
		"change command": readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "systemctl restart nginx"}, true, core.AuthorizationModeReadOnlyAuto),
		"sensitive read": readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "cat /srv/app/.env"}, true, core.AuthorizationModeReadOnlyAuto),
		"service write":  readOnlyAutoHarness(t, serverops.ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, true, core.AuthorizationModeReadOnlyAuto),
		"always ask":     readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, true, core.AuthorizationModeAsk),
	}
	for name, harness := range cases {
		result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
		if err != nil || result.State != ExecutionStateRequiresConfirmation {
			t.Fatalf("%s: InvokeConnector() = %#v, %v", name, result, err)
		}
		if harness.provider.calls != 0 {
			t.Fatalf("%s: the provider ran without approval", name)
		}
	}
}

func TestReadOnlyAutoDoesNotCoverUnattendedRuns(t *testing.T) {
	harness := readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, false, core.AuthorizationModeReadOnlyAuto)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) || result.State != ExecutionStateBlocked {
		t.Fatalf("unattended InvokeConnector() = %#v, %v", result, err)
	}
	if harness.provider.calls != 0 {
		t.Fatal("an unattended read ran")
	}
}

func TestReadOnlyAutoKeepsAPendingApprovalCard(t *testing.T) {
	harness := readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, true, core.AuthorizationModeAsk)
	pending, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation {
		t.Fatalf("pending InvokeConnector() = %#v, %v", pending, err)
	}
	// The person switches the binding to read_only_auto before deciding.
	harness.bindings.binding.Constraint.AuthorizationMode = core.AuthorizationModeReadOnlyAuto
	result, err := harness.service.InvokeConnector(context.Background(), confirmedDigitalHumanRequest(t, harness, pending))
	if err != nil || result.State != ExecutionStateCompleted || harness.provider.calls != 1 {
		t.Fatalf("confirmed InvokeConnector() = %#v, %v", result, err)
	}
}

func TestUnattendedSSHCallsUseTheTaskGrantOnlyForProvenReads(t *testing.T) {
	read := readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, false, core.AuthorizationModeAsk)
	readSpy := &backgroundAuthorizerSpy{allow: true}
	read.service.WithBackgroundAuthorizer(readSpy)
	result, err := read.service.InvokeConnector(context.Background(), read.pinned)
	if err != nil || result.State != ExecutionStateCompleted || readSpy.calls != 1 || read.provider.calls != 1 {
		t.Fatalf("granted unattended read = %#v, %v (grant lookups %d)", result, err, readSpy.calls)
	}

	for name, harness := range map[string]*executionHarness{
		"change command": readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "systemctl restart nginx"}, false, core.AuthorizationModeReadOnlyAuto),
		"service write":  readOnlyAutoHarness(t, serverops.ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, false, core.AuthorizationModeReadOnlyAuto),
	} {
		spy := &backgroundAuthorizerSpy{allow: true}
		harness.service.WithBackgroundAuthorizer(spy)
		result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
		if !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) || result.State != ExecutionStateBlocked {
			t.Fatalf("%s: unattended InvokeConnector() = %#v, %v", name, result, err)
		}
		if spy.calls != 0 || harness.provider.calls != 0 || harness.credentials.calls != 0 {
			t.Fatalf("%s: a change consumed a task grant or reached the server", name)
		}
	}
}

func trustedHarness(t *testing.T, input map[string]any, interactive bool, mode string) *executionHarness {
	t.Helper()
	harness := readOnlyAutoHarness(t, serverops.ActionCommandRun, input, interactive, mode)
	digest, err := catalog.DefinitionContractDigest(catalog.ServerOpsSSHDefinition())
	if err != nil {
		t.Fatal(err)
	}
	constraint := harness.bindings.binding.Constraint
	constraint.AuthorizationConnectionRevision = harness.connections.connection.Revision
	constraint.AuthorizationCredentialVersion = harness.connections.connection.CredentialVersion
	constraint.AuthorizationContractDigest = digest
	return harness
}

func TestTrustedBindingRunsChangesOnlyForItsPresentOwner(t *testing.T) {
	change := map[string]any{"command": "systemctl restart nginx"}
	owner := trustedHarness(t, change, true, core.AuthorizationModeTrusted)
	ctx := WithPresentRequester(context.Background(), owner.pinned.Request.OwnerAccountID)
	result, err := owner.service.InvokeConnector(ctx, owner.pinned)
	if err != nil || result.State != ExecutionStateCompleted || owner.provider.calls != 1 {
		t.Fatalf("trusted owner InvokeConnector() = %#v, %v", result, err)
	}

	for name, run := range map[string]func(*executionHarness) context.Context{
		"no one present":  func(*executionHarness) context.Context { return context.Background() },
		"another account": func(*executionHarness) context.Context { return WithPresentRequester(context.Background(), uuid.New()) },
		"stale credential": func(h *executionHarness) context.Context {
			h.connections.connection.CredentialVersion++
			return WithPresentRequester(context.Background(), h.pinned.Request.OwnerAccountID)
		},
	} {
		harness := trustedHarness(t, change, true, core.AuthorizationModeTrusted)
		result, err := harness.service.InvokeConnector(run(harness), harness.pinned)
		if err != nil || result.State != ExecutionStateRequiresConfirmation || harness.provider.calls != 0 {
			t.Fatalf("%s: InvokeConnector() = %#v, %v", name, result, err)
		}
	}
}

func TestTrustedUnattendedBindingRunsChangesInTheOwnersTaskRun(t *testing.T) {
	change := map[string]any{"command": "systemctl restart nginx"}
	harness := trustedHarness(t, change, false, core.AuthorizationModeTrustedUnattended)
	spy := &backgroundAuthorizerSpy{allow: true}
	harness.service.WithBackgroundAuthorizer(spy)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateCompleted || harness.provider.calls != 1 || spy.calls != 1 || !spy.request.Trusted {
		t.Fatalf("trusted unattended InvokeConnector() = %#v, %v (spy %#v)", result, err, spy)
	}

	interactiveOnly := trustedHarness(t, change, false, core.AuthorizationModeTrusted)
	interactiveSpy := &backgroundAuthorizerSpy{allow: true}
	interactiveOnly.service.WithBackgroundAuthorizer(interactiveSpy)
	if _, err := interactiveOnly.service.InvokeConnector(context.Background(), interactiveOnly.pinned); !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) ||
		interactiveOnly.provider.calls != 0 || interactiveSpy.calls != 0 {
		t.Fatalf("interactive-only trust ran an unattended change: %v", err)
	}
}

type commandRuleSourceStub struct {
	rules []serverops.CommandRule
	err   error
}

func (s commandRuleSourceStub) CommandRules(context.Context, uuid.UUID) ([]serverops.CommandRule, error) {
	return s.rules, s.err
}

func TestOrganizationCommandRulesComeBeforeEveryApprovalPolicy(t *testing.T) {
	rules := commandRuleSourceStub{rules: []serverops.CommandRule{
		{Prefix: "touch /tmp/forbidden", Action: serverops.RuleForbid},
		{Prefix: "systemctl restart", Action: serverops.RuleAsk},
		{Prefix: "df", Action: serverops.RuleAsk},
	}}

	forbidden := trustedHarness(t, map[string]any{"command": "touch /tmp/forbidden"}, true, core.AuthorizationModeTrusted)
	forbidden.service.WithCommandRules(rules)
	ctx := WithPresentRequester(context.Background(), forbidden.pinned.Request.OwnerAccountID)
	if _, err := forbidden.service.InvokeConnector(ctx, forbidden.pinned); !core.IsErrorCode(err, core.ErrorCodeCommandForbidden) || forbidden.provider.calls != 0 {
		t.Fatalf("a forbidden command under full trust = %v (provider calls %d)", err, forbidden.provider.calls)
	}

	asked := trustedHarness(t, map[string]any{"command": "systemctl restart nginx"}, true, core.AuthorizationModeTrusted)
	asked.service.WithCommandRules(rules)
	result, err := asked.service.InvokeConnector(WithPresentRequester(context.Background(), asked.pinned.Request.OwnerAccountID), asked.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || asked.provider.calls != 0 {
		t.Fatalf("an ask rule under full trust = %#v, %v", result, err)
	}

	readOnly := readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, true, core.AuthorizationModeReadOnlyAuto)
	readOnly.service.WithCommandRules(rules)
	if result, err := readOnly.service.InvokeConnector(context.Background(), readOnly.pinned); err != nil || result.State != ExecutionStateRequiresConfirmation {
		t.Fatalf("an ask rule on a read-only command = %#v, %v", result, err)
	}

	unattended := trustedHarness(t, map[string]any{"command": "systemctl restart nginx"}, false, core.AuthorizationModeTrustedUnattended)
	unattended.service.WithCommandRules(rules)
	spy := &backgroundAuthorizerSpy{allow: true}
	unattended.service.WithBackgroundAuthorizer(spy)
	if _, err := unattended.service.InvokeConnector(context.Background(), unattended.pinned); !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) ||
		unattended.provider.calls != 0 || spy.calls != 0 {
		t.Fatalf("an ask rule in a trusted task run = %v (spy %d)", err, spy.calls)
	}

	failing := readOnlyAutoHarness(t, serverops.ActionCommandRun, map[string]any{"command": "df -h"}, true, core.AuthorizationModeReadOnlyAuto)
	failing.service.WithCommandRules(commandRuleSourceStub{err: context.DeadlineExceeded})
	if _, err := failing.service.InvokeConnector(context.Background(), failing.pinned); err == nil || failing.provider.calls != 0 {
		t.Fatal("an unreadable rule store did not fail closed")
	}
}
