package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"gorm.io/datatypes"
)

type recordingWorkflowRunStore struct {
	RunStore
	calls []string
}

func (s *recordingWorkflowRunStore) CompleteRunWithDelivery(_ context.Context, _ uuid.UUID, summary string, _ datatypes.JSON, _ time.Time) error {
	s.calls = append(s.calls, "complete:"+summary)
	return nil
}

func (s *recordingWorkflowRunStore) FailRun(_ context.Context, _ uuid.UUID, code, _ string, _ time.Time) error {
	s.calls = append(s.calls, "fail:"+code)
	return nil
}

func (s *recordingWorkflowRunStore) MarkRunWaiting(_ context.Context, _ uuid.UUID, reason string, _ time.Time) error {
	s.calls = append(s.calls, "waiting:"+reason)
	return nil
}

func (s *recordingWorkflowRunStore) ResumeWaitingRun(context.Context, uuid.UUID, time.Time) error {
	s.calls = append(s.calls, "resume")
	return nil
}

func (s *recordingWorkflowRunStore) CancelWaitingRun(context.Context, uuid.UUID, time.Time) error {
	return nil
}

// settledLifecycle wires a lifecycle to a recording run store the same way
// continueWorkflowApproval does for a run held by a workflow approval.
func settledLifecycle(runtime *fakeWorkflowContinuationRuntime, emitted *[]chatruntime.StreamEvent) (*digitalHumanWorkflowContinuation, *recordingWorkflowRunStore) {
	store := &recordingWorkflowRunStore{}
	service := &RuntimeService{store: store, now: time.Now}
	run := &Run{ID: uuid.New()}
	lifecycle := newTestWorkflowContinuationLifecycle(runtime, emitted)
	lifecycle.settle = func(ctx context.Context, status, answer string, metadata map[string]interface{}) {
		service.settleWorkflowContinuationRun(ctx, run, status, answer, metadata)
	}
	return lifecycle, store
}

func TestWorkflowContinuationSettlesRunWhenWorkflowCompletes(t *testing.T) {
	runtime := &fakeWorkflowContinuationRuntime{summaryResult: &chatruntime.ChatResult{
		Status: runtimemodel.MessageStatusCompleted, Answer: "approved by finance", Metadata: map[string]interface{}{},
	}}
	var emitted []chatruntime.StreamEvent
	lifecycle, store := settledLifecycle(runtime, &emitted)
	_ = lifecycle.handle("workflow_finished", map[string]interface{}{"status": "succeeded", "outputs": map[string]interface{}{"decision": "ok"}})
	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.calls) != 1 || store.calls[0] != "complete:approved by finance" {
		t.Fatalf("run store calls = %v, want the run completed with the final answer", store.calls)
	}
}

func TestWorkflowContinuationSettlesRunWhenPausedAgain(t *testing.T) {
	for name, test := range map[string]struct{ event, want string }{
		"approval": {"approval_requested", "waiting:" + runtimemodel.MessageStatusWaitingApproval},
		"question": {"question_answer_requested", "waiting:" + runtimemodel.MessageStatusWaitingQuestion},
	} {
		t.Run(name, func(t *testing.T) {
			var emitted []chatruntime.StreamEvent
			lifecycle, store := settledLifecycle(&fakeWorkflowContinuationRuntime{}, &emitted)
			_ = lifecycle.handle(test.event, map[string]interface{}{})
			if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
				t.Fatal(err)
			}
			if len(store.calls) != 1 || store.calls[0] != test.want {
				t.Fatalf("run store calls = %v, want [%s]", store.calls, test.want)
			}
		})
	}
}

func TestWorkflowContinuationSettlesRunWhenStoppedOrFailed(t *testing.T) {
	var emitted []chatruntime.StreamEvent
	stopped, stoppedStore := settledLifecycle(&fakeWorkflowContinuationRuntime{}, &emitted)
	_ = stopped.handle("workflow_stopped", map[string]interface{}{})
	if err := stopped.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatal(err)
	}
	if len(stoppedStore.calls) != 1 || stoppedStore.calls[0] != "fail:digital_human.run.canceled" {
		t.Fatalf("stopped run store calls = %v", stoppedStore.calls)
	}

	failed, failedStore := settledLifecycle(&fakeWorkflowContinuationRuntime{}, &emitted)
	if err := failed.finish(workflowContinuationStreamed, errors.New("upstream exploded")); err == nil {
		t.Fatal("finish() swallowed the workflow error")
	}
	if len(failedStore.calls) != 1 || failedStore.calls[0] != "fail:digital_human.run.execution_failed" {
		t.Fatalf("failed run store calls = %v", failedStore.calls)
	}
}

func TestWorkflowContinuationRestoresRunWaitingWhenNothingWasSubmitted(t *testing.T) {
	var emitted []chatruntime.StreamEvent
	lifecycle, store := settledLifecycle(&fakeWorkflowContinuationRuntime{}, &emitted)
	if err := lifecycle.finish(workflowContinuationNotResumed, errors.New("wrong token")); err == nil {
		t.Fatal("finish() swallowed the validation error")
	}
	want := "waiting:" + runtimemodel.MessageStatusWaitingApproval
	if len(store.calls) != 1 || store.calls[0] != want {
		t.Fatalf("run store calls = %v, want the run back to waiting so the approval can be retried", store.calls)
	}
}

func TestWorkflowContinuationLeavesRunAloneWhenItIsNotHeld(t *testing.T) {
	var emitted []chatruntime.StreamEvent
	lifecycle := newTestWorkflowContinuationLifecycle(&fakeWorkflowContinuationRuntime{}, &emitted)
	_ = lifecycle.handle("workflow_finished", map[string]interface{}{"status": "succeeded"})
	if err := lifecycle.finish(workflowContinuationStreamed, nil); err != nil {
		t.Fatalf("a lifecycle without a held run must still finish: %v", err)
	}
}

func TestWorkflowHeldRunCoversWorkflowApprovalsAndQuestions(t *testing.T) {
	waiting := func(surface, reason, origin string) *Run {
		return &Run{Status: RunStatusWaiting, Surface: surface, WaitingReason: reason, WaitingApprovalOrigin: origin}
	}
	approval, question := runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion
	for name, test := range map[string]struct {
		run  *Run
		held bool
	}{
		"console workflow approval":      {waiting(SurfaceConsole, approval, workflowApprovalOrigin), true},
		"webapp workflow approval":       {waiting(SurfaceWebApp, approval, workflowApprovalOrigin), true},
		"api workflow question":          {waiting(SurfaceAPI, question, ""), true},
		"console workflow question":      {waiting(SurfaceConsole, question, ""), true},
		"platform CLI approval":          {waiting(SurfaceConsole, approval, platformCLIApprovalOrigin), false},
		"connector receipt approval":     {waiting(SurfaceConsole, approval, ""), false},
		"channel question keeps its own": {waiting(SurfaceChannel, question, ""), false},
		"budget wait":                    {waiting(SurfaceConsole, runtimemodel.MessageStatusWaitingBudget, ""), false},
		"running run":                    {&Run{Status: RunStatusRunning, Surface: SurfaceConsole}, false},
		"no run":                         {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			if held := workflowHeldRun(test.run) != nil; held != test.held {
				t.Fatalf("held = %v, want %v", held, test.held)
			}
		})
	}
}
