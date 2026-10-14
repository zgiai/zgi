package approval

import (
	"context"
	"errors"
	"testing"
)

type stubResumeHost struct {
	handled bool
	err     error
	calls   int
}

func (h *stubResumeHost) ResumeApprovalForm(context.Context, *Form) (bool, error) {
	h.calls++
	return h.handled, h.err
}

func TestResumeHostsLetTheOwningHostResumeTheForm(t *testing.T) {
	hostErr := errors.New("summary failed")
	for name, test := range map[string]struct {
		host         *stubResumeHost
		wantErr      error
		wantFallback bool
	}{
		"host resumes":          {host: &stubResumeHost{handled: true}, wantFallback: false},
		"host resume fails":     {host: &stubResumeHost{handled: true, err: hostErr}, wantErr: hostErr, wantFallback: false},
		"no waiting host":       {host: &stubResumeHost{}, wantFallback: true},
		"host fails to take it": {host: &stubResumeHost{err: hostErr}, wantFallback: true},
	} {
		t.Run(name, func(t *testing.T) {
			hosts := NewResumeHosts()
			hosts.Register(test.host)
			fallbackCalls := 0
			err := hosts.ResumeCallback(func(context.Context, *Form) error {
				fallbackCalls++
				return nil
			})(context.Background(), &Form{ID: "form-1", WorkflowRunID: "run-1"})
			if !errors.Is(err, test.wantErr) || test.host.calls != 1 || (fallbackCalls == 1) != test.wantFallback {
				t.Fatalf("err = %v, host calls = %d, fallback calls = %d", err, test.host.calls, fallbackCalls)
			}
		})
	}
}

func TestNilResumeHostsResumeTheWorkflowAlone(t *testing.T) {
	var hosts *ResumeHosts
	called := false
	if err := hosts.ResumeCallback(func(context.Context, *Form) error {
		called = true
		return nil
	})(context.Background(), &Form{}); err != nil || !called {
		t.Fatalf("err = %v, fallback called = %v", err, called)
	}
}
