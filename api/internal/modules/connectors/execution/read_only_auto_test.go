package execution

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
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
