package approval

import (
	"context"
	"errors"
	"testing"
)

type stubResumeHost struct {
	handled   bool
	err       error
	calls     int
	requests  []ResumeRequest
	abandoned []error
}

func (h *stubResumeHost) AbandonWorkflowResume(_ context.Context, _ ResumeRequest, cause error) {
	h.abandoned = append(h.abandoned, cause)
}

func (h *stubResumeHost) ResumeWorkflow(_ context.Context, req ResumeRequest) (bool, error) {
	h.calls++
	h.requests = append(h.requests, req)
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

func TestRetryingResumeHostsKeepTheQueueRetryWhenTheOwningHostFails(t *testing.T) {
	hostErr := errors.New("temporary database error")
	for name, test := range map[string]struct {
		lastAttempt  bool
		wantErr      bool
		wantFallback bool
	}{
		"retry left":   {lastAttempt: false, wantErr: true},
		"last attempt": {lastAttempt: true, wantFallback: true},
	} {
		t.Run(name, func(t *testing.T) {
			hosts := NewResumeHosts()
			hosts.lastAttempt = func(context.Context) bool { return test.lastAttempt }
			hosts.Register(&stubResumeHost{err: hostErr})
			fallbackCalls := 0
			err := hosts.RetryingResumeCallback(func(context.Context, *Form) error {
				fallbackCalls++
				return nil
			})(context.Background(), &Form{ID: "form-1", WorkflowRunID: "run-1"})
			if errors.Is(err, hostErr) != test.wantErr || (fallbackCalls == 1) != test.wantFallback {
				t.Fatalf("err = %v, fallback calls = %d", err, fallbackCalls)
			}
		})
	}
	hosts := NewResumeHosts()
	hosts.lastAttempt = func(context.Context) bool { return false }
	hosts.Register(&stubResumeHost{})
	fallbackCalls := 0
	if err := hosts.RetryingResumeCallback(func(context.Context, *Form) error {
		fallbackCalls++
		return nil
	})(context.Background(), &Form{ID: "form-1", WorkflowRunID: "run-1"}); err != nil || fallbackCalls != 1 {
		t.Fatalf("no waiting host: err = %v, fallback calls = %d", err, fallbackCalls)
	}
}

func TestLastQueueAttemptOutsideAQueueTask(t *testing.T) {
	if !lastQueueAttempt(context.Background()) {
		t.Fatal("a resume outside a queue task has no retry left")
	}
}

func TestResumeHostsAbandonTheConversationOnlyWhenGivingUp(t *testing.T) {
	hostErr := errors.New("model unavailable")
	for name, test := range map[string]struct {
		retrying, lastAttempt bool
		wantAbandoned         int
	}{
		"queue retry left":   {retrying: true, lastAttempt: false, wantAbandoned: 0},
		"queue last attempt": {retrying: true, lastAttempt: true, wantAbandoned: 1},
		"cannot retry":       {retrying: false, wantAbandoned: 1},
	} {
		t.Run(name, func(t *testing.T) {
			host := &stubResumeHost{err: hostErr}
			hosts := NewResumeHosts()
			hosts.lastAttempt = func(context.Context) bool { return test.lastAttempt }
			hosts.Register(host)
			callback := hosts.ResumeCallback
			if test.retrying {
				callback = hosts.RetryingResumeCallback
			}
			_ = callback(func(context.Context, *Form) error { return nil })(context.Background(), &Form{ID: "form-1", WorkflowRunID: "run-1"})
			if len(host.abandoned) != test.wantAbandoned || (test.wantAbandoned == 1 && !errors.Is(host.abandoned[0], hostErr)) {
				t.Fatalf("abandoned = %v, want %d", host.abandoned, test.wantAbandoned)
			}
		})
	}
}

func TestQueuedResumeCallbackRetriesTimeoutsThroughTheQueue(t *testing.T) {
	var queued []string
	fallbackCalls := 0
	fallback := func(context.Context, *Form) error {
		fallbackCalls++
		return nil
	}
	queuedResume := QueuedResumeCallback(func(_ context.Context, formID string) error {
		queued = append(queued, formID)
		return nil
	}, fallback)
	if err := queuedResume(context.Background(), &Form{ID: "form-1"}); err != nil || len(queued) != 1 || queued[0] != "form-1" || fallbackCalls != 0 {
		t.Fatalf("err = %v, queued = %v, fallback calls = %d; want the resume queued", err, queued, fallbackCalls)
	}
	unavailable := QueuedResumeCallback(func(context.Context, string) error { return errors.New("redis unavailable") }, fallback)
	if err := unavailable(context.Background(), &Form{ID: "form-2"}); err != nil || fallbackCalls != 1 {
		t.Fatalf("err = %v, fallback calls = %d; want the resume run right away", err, fallbackCalls)
	}
}

func TestQuestionResumesReachTheHostWithTheirAnswers(t *testing.T) {
	host := &stubResumeHost{handled: true}
	hosts := NewResumeHosts()
	hosts.Register(host)
	fallbackCalls := 0
	inputs := map[string]interface{}{"query": "Q4"}
	err := hosts.RetryingQuestionResumeCallback(func(context.Context, string, map[string]interface{}) error {
		fallbackCalls++
		return nil
	})(context.Background(), "run-1", inputs)
	if err != nil || fallbackCalls != 0 || len(host.requests) != 1 {
		t.Fatalf("err = %v, fallback calls = %d, requests = %#v", err, fallbackCalls, host.requests)
	}
	if req := host.requests[0]; req.WorkflowRunID != "run-1" || req.Form != nil || req.QuestionInputs["query"] != "Q4" {
		t.Fatalf("request = %#v, want the question resume with its answers", req)
	}

	failing := &stubResumeHost{err: errors.New("temporary database error")}
	retrying := NewResumeHosts()
	retrying.lastAttempt = func(context.Context) bool { return false }
	retrying.Register(failing)
	if err := retrying.RetryingQuestionResumeCallback(func(context.Context, string, map[string]interface{}) error {
		fallbackCalls++
		return nil
	})(context.Background(), "run-1", inputs); err == nil || fallbackCalls != 0 {
		t.Fatalf("err = %v, fallback calls = %d; want the question resume retried", err, fallbackCalls)
	}
}
