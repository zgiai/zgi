package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

// workflowApprovalWaitingStore is an optional persistence extension that finds
// the run a generic workflow approval holds.
type workflowApprovalWaitingStore interface {
	RunWaitingOnWorkflowApproval(context.Context, string) (*Run, error)
}

type externalWorkflowContinuationChatRuntime interface {
	workflowContinuationChatRuntime
	chatruntime.ExternalWorkflowApprovalContinuationBeginner
}

var _ approvalruntime.ResumeHost = (*RuntimeService)(nil)

// ResumeApprovalForm resumes a workflow approval completed outside the
// conversation (an email or SMS approver, the approval page, or a timeout)
// together with the DigitalHuman run that waits on it: the workflow events
// stream into the same continuation lifecycle an in-conversation approval
// uses, so the message is summarized, completed, failed or paused again and
// the run settles with it. It reports handled=false when no run waits on the
// form, or when the conversation already took the approval itself.
func (s *RuntimeService) ResumeApprovalForm(ctx context.Context, form *approvalruntime.Form) (bool, error) {
	if s == nil || form == nil || !s.WorkflowContinuationEnabled() {
		return false, nil
	}
	store, ok := s.store.(workflowApprovalWaitingStore)
	if !ok {
		return false, nil
	}
	runtime, ok := s.chat.(externalWorkflowContinuationChatRuntime)
	if !ok {
		return false, nil
	}
	run, err := store.RunWaitingOnWorkflowApproval(ctx, form.WorkflowRunID)
	if err != nil || run == nil || run.ConversationID == nil || run.MessageID == nil {
		return false, err
	}
	scope := runtimeScope(run)
	caller := workflowRunCaller(run)
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return false, err
	}
	config, _, err := s.userInputContinuationConfig(ctx, run, resolved, event, runScope(run))
	if err != nil {
		return false, err
	}
	continuation, err := runtime.BeginExternalWorkflowApprovalContinuation(ctx, scope, caller, config, *run.ConversationID, *run.MessageID)
	if errors.Is(err, chatruntime.ErrInvalidInput) {
		// The conversation took the approval itself, or the message is no
		// longer waiting on it.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx: context.WithoutCancel(ctx), runtime: runtime, scope: scope, continuation: continuation,
	}
	lifecycle.settle = func(settleCtx context.Context, status, answer string, metadata map[string]interface{}) {
		s.settleWorkflowContinuationRun(settleCtx, run, status, answer, metadata)
	}
	waiter, ok := s.store.(runWaitingStore)
	if !ok {
		return true, lifecycle.abortBeforeResume(fmt.Errorf("digital human waiting run resume is unavailable"))
	}
	if err := waiter.ResumeWaitingRun(ctx, run.ID, s.now()); err != nil {
		// The run left waiting meanwhile (for example it was canceled), so it no
		// longer follows this message.
		lifecycle.settle = nil
		return true, lifecycle.abortBeforeResume(err)
	}
	resume := workflowContinuationUnobserved
	if runner, streaming := s.workflowRunner.(workflowContinuationStreamRunner); streaming {
		resume = workflowContinuationStreamed
		err = runner.ResumeApprovalWorkflowStream(ctx, form, lifecycle.handle)
	} else {
		err = s.workflowRunner.ResumeApprovalWorkflow(ctx, form)
	}
	if workflowResumeNotStarted(err) {
		// The workflow is still waiting (another approver on the same pause) or
		// is resumed elsewhere; keep the conversation waiting on it.
		return true, lifecycle.abortBeforeResume(err)
	}
	return true, lifecycle.finish(resume, err)
}

func workflowResumeNotStarted(err error) bool {
	return errors.Is(err, workflowpause.ErrPauseNotFound) || errors.Is(err, workflowpause.ErrPauseNotResumeReady) || errors.Is(err, workflowpause.ErrResumeAlreadyRunning)
}

// workflowRunCaller is the conversation caller the run executed as.
func workflowRunCaller(run *Run) chatruntime.Caller {
	digitalHumanID := run.DigitalHumanID
	runID := run.ID
	caller := chatruntime.Caller{
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID, RunID: &runID,
		Source: digitalHumanConversationSourceForRun(run),
	}
	var event struct {
		Context map[string]any `json:"context"`
	}
	if json.Unmarshal(run.InvocationEvent, &event) == nil {
		if value, ok := event.Context["web_app_id"].(string); ok {
			if parsed, err := uuid.Parse(strings.TrimSpace(value)); err == nil && parsed != uuid.Nil {
				caller.SourceWebAppID = &parsed
			}
		}
	}
	return caller
}
