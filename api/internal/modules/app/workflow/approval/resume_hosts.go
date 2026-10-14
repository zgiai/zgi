package approval

import (
	"context"
	"sync"

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

// ResumeHosts routes approval resumes to the host that owns the workflow.
// Hosts register after the approval task handlers, so it is consulted when a
// resume runs, not when the handlers are registered.
type ResumeHosts struct {
	mu    sync.RWMutex
	hosts []ResumeHost
}

func NewResumeHosts() *ResumeHosts {
	return &ResumeHosts{}
}

func (h *ResumeHosts) Register(host ResumeHost) {
	if h == nil || host == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hosts = append(h.hosts, host)
}

// ResumeCallback lets a registered host resume the form and otherwise falls
// back to resuming the workflow alone. A host that fails before it took the
// form never blocks the workflow: the fallback still resumes it.
func (h *ResumeHosts) ResumeCallback(fallback ResumeCallback) ResumeCallback {
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
				if err != nil {
					logger.WarnContext(ctx, "approval resume host could not take the form; resuming the workflow alone", "form_id", form.ID, "workflow_run_id", form.WorkflowRunID, err)
				}
			}
		}
		if fallback == nil {
			return nil
		}
		return fallback(ctx, form)
	}
}
