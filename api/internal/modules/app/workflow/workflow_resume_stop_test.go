package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestDrainApprovalResumeStreamTreatsDurableStopAsTerminal(t *testing.T) {
	run := &WorkflowRunLog{ID: "run-stopped-during-resume", RuntimeProtocolVersion: workflowRuntimeProtocolVersionV2}
	service := &WorkflowService{workflowRunLogRepo: &mockWorkflowRunLogRepo{runsByID: map[string]*WorkflowRunLog{
		run.ID: {ID: run.ID, Status: dto.WorkflowRunStatusStopped, RuntimeProtocolVersion: workflowRuntimeProtocolVersionV2},
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (&WorkflowHandler{}).drainApprovalResumeStream(
		ctx,
		nil,
		service,
		run,
		make(chan *WorkflowStreamEvent),
		bufferedWorkflowResumeError(errors.New("context canceled")),
		make(chan map[string]interface{}),
		time.Time{},
		"WORKFLOW",
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("drain stopped resume error = %v, want nil", err)
	}
}

func bufferedWorkflowResumeError(err error) <-chan error {
	ch := make(chan error, 1)
	ch <- err
	return ch
}
