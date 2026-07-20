package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

const workflowExecutionLeaseSafetyMargin = 10 * time.Second

var workflowExecutionLeaseRetryDelays = [...]time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second}

type workflowExecutionOwnerContextKey struct{}

type workflowExecutionLeaseRenewer interface {
	RenewExecutionLease(context.Context, workflowpause.ExecutionClaim, time.Duration) (time.Time, error)
}

type workflowExecutionOwner struct {
	WorkflowRunID   string
	ExecutionID     string
	Generation      int64
	PauseID         string
	PauseGeneration int64
}

func withWorkflowExecutionOwner(ctx context.Context, owner workflowExecutionOwner) context.Context {
	if ctx == nil || owner.ExecutionID == "" || owner.Generation <= 0 {
		return ctx
	}
	ctx = context.WithValue(ctx, workflowExecutionOwnerContextKey{}, owner)
	return withWorkflowDBStatementMetrics(ctx, owner.WorkflowRunID)
}

func workflowExecutionOwnerFromContext(ctx context.Context) (workflowExecutionOwner, bool) {
	if ctx == nil {
		return workflowExecutionOwner{}, false
	}
	owner, ok := ctx.Value(workflowExecutionOwnerContextKey{}).(workflowExecutionOwner)
	return owner, ok && owner.ExecutionID != "" && owner.Generation > 0
}

func workflowExecutionOwnerFromRun(run *WorkflowRunLog) workflowExecutionOwner {
	if run == nil || run.ActiveExecutionID == nil {
		return workflowExecutionOwner{}
	}
	return workflowExecutionOwner{WorkflowRunID: run.ID, ExecutionID: *run.ActiveExecutionID, Generation: run.ExecutionGeneration}
}

func claimWorkflowResume(ctx context.Context, pauseService *workflowpause.Service, run *WorkflowRunLog, pauseID string) (*workflowpause.ExecutionClaim, error) {
	if run == nil || run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
		return nil, nil
	}
	claim, err := pauseService.ClaimResume(ctx, run.ID, pauseID, workflowExecutionLeaseDuration)
	if err != nil || claim == nil {
		return claim, err
	}
	// ClaimResume commits workflow_resumed outside the dispatcher. Hydrate a
	// small committed window so Redis also receives adjacent interaction events
	// (for example approval_result_filled) that were committed by subpackages.
	if !publishWorkflowCommittedTailWindow(ctx, pauseService, run.TenantID, run.ID, claim.EventCursor) {
		publishWorkflowCommittedTail(ctx, run.ID, claim.Event)
	}
	return claim, nil
}

func startWorkflowExecutionLeaseRenewal(ctx context.Context, pauseService *workflowpause.Service, claim workflowpause.ExecutionClaim) (context.Context, func()) {
	return startWorkflowExecutionLeaseRenewalWithInterval(ctx, pauseService, claim, workflowExecutionLeaseDuration, workflowExecutionLeaseDuration/3)
}

func startWorkflowExecutionLeaseRenewalWithInterval(
	ctx context.Context,
	pauseService workflowExecutionLeaseRenewer,
	claim workflowpause.ExecutionClaim,
	leaseDuration time.Duration,
	renewInterval time.Duration,
) (context.Context, func()) {
	if pauseService == nil || claim.ExecutionID == "" {
		return ctx, func() {}
	}
	if leaseDuration <= 0 {
		leaseDuration = workflowExecutionLeaseDuration
	}
	if renewInterval <= 0 {
		renewInterval = leaseDuration / 3
	}
	executionCtx, cancelExecution := context.WithCancelCause(ctx)
	leaseCtx, stopLease := context.WithCancel(executionCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		leaseExpires := claim.LeaseExpires
		if leaseExpires.IsZero() {
			leaseExpires = time.Now().Add(leaseDuration)
		}
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				renewedUntil, err := renewWorkflowExecutionLeaseWithRetry(leaseCtx, pauseService, claim, leaseExpires, leaseDuration)
				if err != nil {
					cancelExecution(err)
					return
				}
				leaseExpires = renewedUntil
			}
		}
	}()
	return executionCtx, func() {
		stopLease()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func renewWorkflowExecutionLeaseWithRetry(
	ctx context.Context,
	pauseService workflowExecutionLeaseRenewer,
	claim workflowpause.ExecutionClaim,
	currentLeaseExpires time.Time,
	leaseDuration time.Duration,
) (time.Time, error) {
	for attempt := 0; ; attempt++ {
		renewedUntil, err := pauseService.RenewExecutionLease(ctx, claim, leaseDuration)
		if err == nil {
			return renewedUntil, nil
		}
		if errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
			recordWorkflowLeaseRenewalFailure(ctx, "ownership_lost")
			return time.Time{}, workflowpause.ErrExecutionOwnershipLost
		}
		delay := workflowExecutionLeaseRetryDelays[min(attempt, len(workflowExecutionLeaseRetryDelays)-1)]
		if !time.Now().Add(delay).Before(currentLeaseExpires.Add(-workflowExecutionLeaseSafetyMargin)) {
			recordWorkflowLeaseRenewalFailure(ctx, "safety_deadline")
			return time.Time{}, fmt.Errorf("renew workflow execution lease before safety deadline: %w", err)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return time.Time{}, context.Cause(ctx)
		case <-timer.C:
		}
	}
}
