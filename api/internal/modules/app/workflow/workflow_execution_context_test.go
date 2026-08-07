package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type ownershipLostLeaseRenewer struct{}

func (ownershipLostLeaseRenewer) RenewExecutionLease(context.Context, workflowpause.ExecutionClaim, time.Duration) (time.Time, error) {
	return time.Time{}, workflowpause.ErrExecutionOwnershipLost
}

func TestWorkflowExecutionContextSurvivesTransportCancellation(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	executionCtx, cancelExecution := newWorkflowExecutionContext(requestCtx)
	defer cancelExecution()

	cancelRequest()
	select {
	case <-executionCtx.Done():
		t.Fatalf("transport cancellation propagated to workflow execution")
	default:
	}

	cancelExecution()
	select {
	case <-executionCtx.Done():
	default:
		t.Fatalf("explicit execution cancellation did not stop workflow")
	}
}

func TestWorkflowExecutionLeaseOwnershipLossCancelsOwnerContext(t *testing.T) {
	claim := workflowpause.ExecutionClaim{
		WorkflowRunID: "run-1",
		ExecutionID:   "execution-1",
		Generation:    1,
		LeaseExpires:  time.Now().Add(time.Second),
	}
	executionCtx, stop := startWorkflowExecutionLeaseRenewalWithInterval(
		context.Background(),
		ownershipLostLeaseRenewer{},
		claim,
		time.Second,
		time.Millisecond,
	)
	defer stop()

	select {
	case <-executionCtx.Done():
		if !errors.Is(context.Cause(executionCtx), workflowpause.ErrExecutionOwnershipLost) {
			t.Fatalf("unexpected cancellation cause: %v", context.Cause(executionCtx))
		}
		if workflowshared.IsContextCancellation(executionCtx, context.Canceled) {
			t.Fatal("ownership loss must not be classified as user cancellation")
		}
		if got := workflowshared.ResolveContextError(executionCtx, context.Canceled); !errors.Is(got, workflowpause.ErrExecutionOwnershipLost) {
			t.Fatalf("resolved execution error = %v, want ownership loss", got)
		}
	case <-time.After(time.Second):
		t.Fatal("ownership loss did not cancel execution context")
	}
}
