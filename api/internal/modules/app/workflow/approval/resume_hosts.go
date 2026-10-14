package approval

import (
	"context"
	"fmt"
	"sync"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ResumeHost is an application that owns the conversation a paused workflow
// reports to, such as a DigitalHuman conversation. When an approval is
// completed outside that conversation (an email or SMS approver, the approval
// page, or a timeout), the host resumes the workflow itself so the
// conversation follows it as it runs.
type ResumeHost interface {
	// ResumeApprovalForm reports handled=false when no conversation of this
	// host waits on the form; the workflow is then resumed on its own.
	ResumeApprovalForm(ctx context.Context, form *Form) (handled bool, err error)
}

// ResumeHostAbandoner is implemented by a host that can settle its waiting
// conversation when it finally gives up on a form, so the conversation does
// not keep waiting on a workflow that resumes without it.
type ResumeHostAbandoner interface {
	AbandonApprovalForm(ctx context.Context, form *Form, cause error)
}

// ResumeHosts routes approval resumes to the host that owns the workflow.
// Hosts register after the approval task handlers, so it is consulted when a
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
// the form but fails to take it returns its error, so the queue retries and
// the conversation can still follow the workflow once the failure clears. On
// the queue's last attempt the workflow resumes alone so it is never blocked.
func (h *ResumeHosts) RetryingResumeCallback(fallback ResumeCallback) ResumeCallback {
	return h.resumeCallback(fallback, true)
}

// ResumeCallback is for callers that cannot retry a resume (the timeout scan
// marks the form before resuming it): a host that fails to take the form is
// logged and the workflow resumes alone.
func (h *ResumeHosts) ResumeCallback(fallback ResumeCallback) ResumeCallback {
	return h.resumeCallback(fallback, false)
}

func (h *ResumeHosts) resumeCallback(fallback ResumeCallback, retrying bool) ResumeCallback {
	return func(ctx context.Context, form *Form) error {
		if h != nil && form != nil {
			h.mu.RLock()
			hosts := append([]ResumeHost(nil), h.hosts...)
			h.mu.RUnlock()
			for _, host := range hosts {
				handled, err := host.ResumeApprovalForm(ctx, form)
				if handled {
					return err
				}
				if err == nil {
					continue
				}
				if retrying && !h.isLastAttempt(ctx) {
					return fmt.Errorf("approval resume host could not take form %s: %w", form.ID, err)
				}
				logger.ErrorContext(ctx, "approval resume host could not take the form; resuming the workflow alone", "form_id", form.ID, "workflow_run_id", form.WorkflowRunID, err)
				if abandoner, ok := host.(ResumeHostAbandoner); ok {
					abandoner.AbandonApprovalForm(ctx, form, err)
				}
			}
		}
		if fallback == nil {
			return nil
		}
		return fallback(ctx, form)
	}
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
