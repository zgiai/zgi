package approval

import (
	"context"
	"fmt"
	"sync"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ResumeRequest is a resume of a paused workflow that its conversation may
// follow. An approval resume carries the approval form; a question resume
// (the pause also held a question, so the completed answers ride along)
// carries the question inputs.
type ResumeRequest struct {
	WorkflowRunID  string
	Form           *Form
	QuestionInputs map[string]interface{}
}

// ResumeHost is an application that owns the conversation a paused workflow
// reports to, such as a DigitalHuman conversation. When the pause is completed
// outside that conversation (an email or SMS approver, the approval page, or
// a timeout), the host resumes the workflow itself so the conversation follows
// it as it runs.
type ResumeHost interface {
	// ResumeWorkflow reports handled=false when no conversation of this host
	// waits on the workflow; the workflow is then resumed on its own.
	ResumeWorkflow(ctx context.Context, req ResumeRequest) (handled bool, err error)
}

// ResumeHostAbandoner is implemented by a host that can settle its waiting
// conversation when it finally gives up on a resume, so the conversation does
// not keep waiting on a workflow that resumes without it.
type ResumeHostAbandoner interface {
	AbandonWorkflowResume(ctx context.Context, req ResumeRequest, cause error)
}

// ResumeHosts routes workflow resumes to the host that owns the workflow.
// Hosts register after the resume task handlers, so it is consulted when a
// resume runs, not when the handlers are registered.
type ResumeHosts struct {
	mu    sync.RWMutex
	hosts []ResumeHost
	// lastAttempt reports whether the running resume has no retry left.
	lastAttempt func(context.Context) bool
}

func NewResumeHosts() *ResumeHosts {
	return &ResumeHosts{lastAttempt: lastQueueAttempt}
}

func (h *ResumeHosts) Register(host ResumeHost) {
	if h == nil || host == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hosts = append(h.hosts, host)
}

// RetryingResumeCallback is for the approval resume queue. A host that owns
// the workflow but fails to take it returns its error, so the queue retries
// and the conversation can still follow the workflow once the failure clears.
// On the queue's last attempt the workflow resumes alone so it is never blocked.
func (h *ResumeHosts) RetryingResumeCallback(fallback ResumeCallback) ResumeCallback {
	return func(ctx context.Context, form *Form) error {
		return h.resume(ctx, approvalResumeRequest(form), true, func(ctx context.Context) error {
			return callResume(ctx, fallback, form)
		})
	}
}

// ResumeCallback is for callers that cannot retry a resume: a host that fails
// to take the workflow is logged and the workflow resumes alone.
func (h *ResumeHosts) ResumeCallback(fallback ResumeCallback) ResumeCallback {
	return func(ctx context.Context, form *Form) error {
		return h.resume(ctx, approvalResumeRequest(form), false, func(ctx context.Context) error {
			return callResume(ctx, fallback, form)
		})
	}
}

// RetryingQuestionResumeCallback is RetryingResumeCallback for the question
// resume queue, which also resumes a pause whose approval completed after its
// question was answered.
func (h *ResumeHosts) RetryingQuestionResumeCallback(fallback QuestionResumeCallback) QuestionResumeCallback {
	return func(ctx context.Context, workflowRunID string, inputs map[string]interface{}) error {
		return h.resume(ctx, ResumeRequest{WorkflowRunID: workflowRunID, QuestionInputs: inputs}, true, func(ctx context.Context) error {
			if fallback == nil {
				return nil
			}
			return fallback(ctx, workflowRunID, inputs)
		})
	}
}

func approvalResumeRequest(form *Form) ResumeRequest {
	if form == nil {
		return ResumeRequest{}
	}
	return ResumeRequest{WorkflowRunID: form.WorkflowRunID, Form: form}
}

func callResume(ctx context.Context, fallback ResumeCallback, form *Form) error {
	if fallback == nil {
		return nil
	}
	return fallback(ctx, form)
}

func (h *ResumeHosts) resume(ctx context.Context, req ResumeRequest, retrying bool, fallback func(context.Context) error) error {
	if h != nil && req.WorkflowRunID != "" {
		h.mu.RLock()
		hosts := append([]ResumeHost(nil), h.hosts...)
		h.mu.RUnlock()
		for _, host := range hosts {
			handled, err := host.ResumeWorkflow(ctx, req)
			if handled {
				return err
			}
			if err == nil {
				continue
			}
			if retrying && !h.isLastAttempt(ctx) {
				return fmt.Errorf("workflow resume host could not take workflow run %s: %w", req.WorkflowRunID, err)
			}
			logger.ErrorContext(ctx, "workflow resume host could not take the workflow; resuming it alone", "workflow_run_id", req.WorkflowRunID, err)
			if abandoner, ok := host.(ResumeHostAbandoner); ok {
				abandoner.AbandonWorkflowResume(ctx, req, err)
			}
		}
	}
	return fallback(ctx)
}

func (h *ResumeHosts) isLastAttempt(ctx context.Context) bool {
	if h.lastAttempt == nil {
		return lastQueueAttempt(ctx)
	}
	return h.lastAttempt(ctx)
}

// lastQueueAttempt reports whether the running queue task has no retry left.
// Outside a queue task there is nothing to retry.
func lastQueueAttempt(ctx context.Context) bool {
	retried, ok := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	return !ok || !maxOK || retried >= maxRetry
}

// QueuedResumeCallback hands a resume to the approval resume queue, so it is
// retried like a submitted approval. When the queue cannot take it, the resume
// runs right away through fallback.
func QueuedResumeCallback(enqueue func(context.Context, string) error, fallback ResumeCallback) ResumeCallback {
	return func(ctx context.Context, form *Form) error {
		if enqueue != nil && form != nil {
			err := enqueue(ctx, form.ID)
			if err == nil {
				return nil
			}
			logger.WarnContext(ctx, "approval resume could not be queued; resuming it now", "form_id", form.ID, "workflow_run_id", form.WorkflowRunID, err)
		}
		if fallback == nil {
			return nil
		}
		return fallback(ctx, form)
	}
}
