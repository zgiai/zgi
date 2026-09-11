package tools

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"testing"
)

type workflowCatalogDispatchCapture struct {
	result     *connectorexecution.ExecutionResult
	calls      int
	invocation connectordispatch.Invocation
	err        error
}

func (capture *workflowCatalogDispatchCapture) Dispatch(_ context.Context, invocation connectordispatch.Invocation) (connectorexecution.ExecutionResult, error) {
	capture.calls++
	capture.invocation = invocation
	if capture.result != nil {
		return *capture.result, capture.err
	}
	return connectorexecution.ExecutionResult{
		State:   connectorexecution.ExecutionStateCompleted,
		Outcome: core.OperationOutcome{ID: uuid.New(), Status: core.OutcomeStatusSuccessful},
		Value:   map[string]interface{}{"provider": "native"},
	}, nil
}

func TestNativeWorkflowNeverCompletesOnUnprovenOutcome(t *testing.T) {
	for _, status := range []core.OutcomeStatus{core.OutcomeStatusFailed, core.OutcomeStatusUnknown, ""} {
		for _, withError := range []bool{false, true} {
			t.Run(string(status)+map[bool]string{false: "/result", true: "/error"}[withError], func(t *testing.T) {
				f := newNativeWorkflowFixture(t, false, "github.user.get")
				f.expectGraph()
				f.expectBinding(f.binding, true)
				capture := &workflowCatalogDispatchCapture{result: &connectorexecution.ExecutionResult{State: connectorexecution.ExecutionStateCompleted, Outcome: core.OperationOutcome{ID: uuid.New(), Status: status}, Value: map[string]interface{}{"api_key": "PRIVATE-PROVIDER-OUTPUT"}}}
				if withError {
					capture.err = core.NewError(core.ErrorCodeProviderUnavailable, false, nil)
				}
				f.node.connectorDispatcher = capture
				result, err := f.node.executeRun(t.Context(), nil)
				if err != nil || result == nil || result.Status != shared.FAILED || result.Err == nil || capture.calls != 1 {
					t.Fatalf("native terminal result: status=%v err=%v calls=%d", result, err, capture.calls)
				}
				if _, exists := result.Outputs["api_key"]; exists {
					t.Fatal("failure retained raw provider output")
				}
				if result.Outputs["operation"] == nil || result.Outputs["connector_presentation"] == nil {
					t.Fatal("failure lost authoritative outcome evidence")
				}
				if err := f.mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestNativeWorkflowStopsOnRejectedOrUnresolvedDispatch(t *testing.T) {
	for _, state := range []connectorexecution.ExecutionState{connectorexecution.ExecutionStateRejected, connectorexecution.ExecutionStateUnresolved} {
		f := newNativeWorkflowFixture(t, false, "github.user.get")
		f.expectGraph()
		f.expectBinding(f.binding, true)
		disposition := connectorexecution.DispatchUnknown
		if state == connectorexecution.ExecutionStateRejected {
			disposition = connectorexecution.DispatchNotSubmitted
		}
		capture := &workflowCatalogDispatchCapture{result: &connectorexecution.ExecutionResult{State: state, Evidence: &connectorexecution.DispatchEvidence{Version: 1, Disposition: disposition, DiagnosticCode: core.ErrorCodeConflict}}, err: core.NewError(core.ErrorCodeConflict, false, nil)}
		f.node.connectorDispatcher = capture
		result, err := f.node.executeRun(t.Context(), nil)
		if err != nil || result == nil || result.Status != shared.FAILED || result.Err == nil || capture.calls != 1 || result.Outputs["execution_evidence"] == nil {
			t.Fatalf("state=%s result=%+v err=%v", state, result, err)
		}
		if err := f.mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecuteRunDispatchesActualNativeCatalogTool(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "draft", true: "published"}[published], func(t *testing.T) {
			f := newNativeWorkflowFixture(t, published, "github.user.get")
			f.expectGraph()
			f.expectBinding(f.binding, true)
			capture := &workflowCatalogDispatchCapture{}
			f.node.connectorDispatcher = capture
			result, err := f.node.executeRun(t.Context(), nil)
			if err != nil || result == nil || result.Status != shared.SUCCEEDED || capture.calls != 1 || capture.invocation.Identity.BindingID != f.binding.ID {
				t.Fatalf("dispatch=%#v result=%#v err=%v", capture, result, err)
			}
			if err := f.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
