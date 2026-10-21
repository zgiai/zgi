package runtimeplatform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func testScope() Scope {
	return Scope{OrganizationID: "org", SubjectID: "user", AgentID: "agent", ConversationID: "conversation", RunID: "run", BindingID: "binding", BindingGeneration: 1, JobID: "job"}
}

func testOptions() Options {
	return Options{
		TTL:       time.Minute,
		Authorize: func(context.Context, Scope, Action) error { return nil },
		Handle: func(context.Context, Scope, Request) (Response, error) {
			return Response{Status: Completed, Data: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
}

func openTestSession(t *testing.T, parent context.Context, options Options) (*Session, string) {
	t.Helper()
	session, token, err := NewSession(parent, testScope(), []Action{ReadFile, ExportFile, CallModel, CallTool}, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	return session, token
}

func testRequest() Request {
	return Request{ID: "request-1", Action: ExportFile, Arguments: json.RawMessage(`{"path":"result.txt"}`)}
}

func TestGrantRejectsEveryScopeMismatch(t *testing.T) {
	for _, field := range []string{"organization", "subject", "agent", "conversation", "run", "binding", "generation", "job"} {
		t.Run(field, func(t *testing.T) {
			var calls atomic.Int64
			options := testOptions()
			options.Handle = func(context.Context, Scope, Request) (Response, error) {
				calls.Add(1)
				return Response{Status: Completed}, nil
			}
			session, token := openTestSession(t, t.Context(), options)
			scope := testScope()
			switch field {
			case "organization":
				scope.OrganizationID = "other"
			case "subject":
				scope.SubjectID = "other"
			case "agent":
				scope.AgentID = "other"
			case "conversation":
				scope.ConversationID = "other"
			case "run":
				scope.RunID = "other"
			case "binding":
				scope.BindingID = "other"
			case "generation":
				scope.BindingGeneration++
			case "job":
				scope.JobID = "other"
			}
			_, err := session.Execute(t.Context(), token, scope, testRequest())
			if !apperror.IsCode(err, catalog.CodeAuthenticationRequired) || calls.Load() != 0 {
				t.Fatalf("mismatch: err=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestGrantExpiryCloseAndRunCancellation(t *testing.T) {
	for _, reason := range []string{"expiry", "close", "run_cancel", "wrong_token"} {
		t.Run(reason, func(t *testing.T) {
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			var seconds atomic.Int64
			options := testOptions()
			options.Now = func() time.Time { return time.Unix(seconds.Load(), 0) }
			session, token := openTestSession(t, parent, options)
			switch reason {
			case "expiry":
				seconds.Store(60)
			case "close":
				session.Close()
				session.Close()
			case "run_cancel":
				cancel()
			case "wrong_token":
				token = strings.Repeat("x", 43)
			}
			if _, err := session.Execute(t.Context(), token, testScope(), testRequest()); !apperror.IsCode(err, catalog.CodeAuthenticationRequired) {
				t.Fatalf("invalid grant accepted: %v", err)
			}
		})
	}
}

func TestRequestIdempotencyAndResponseOwnership(t *testing.T) {
	var calls atomic.Int64
	options := testOptions()
	options.Handle = func(context.Context, Scope, Request) (Response, error) {
		calls.Add(1)
		return Response{Status: Completed, Data: json.RawMessage(`{"ok":true}`)}, nil
	}
	session, token := openTestSession(t, t.Context(), options)
	first, err := session.Execute(t.Context(), token, testScope(), testRequest())
	if err != nil {
		t.Fatal(err)
	}
	first.Data[0] = 'x'
	repeated, err := session.Execute(t.Context(), token, testScope(), testRequest())
	if err != nil || string(repeated.Data) != `{"ok":true}` || calls.Load() != 1 {
		t.Fatalf("repeat: response=%s calls=%d err=%v", repeated.Data, calls.Load(), err)
	}
	changed := testRequest()
	changed.Arguments = json.RawMessage(`{"path":"other.txt"}`)
	if _, err := session.Execute(t.Context(), token, testScope(), changed); !apperror.IsCode(err, catalog.CodeResourceConflict) || calls.Load() != 1 {
		t.Fatalf("changed arguments: calls=%d err=%v", calls.Load(), err)
	}
}

func TestConcurrentDuplicateDoesNotExecuteTwice(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	options := testOptions()
	options.Handle = func(ctx context.Context, _ Scope, _ Request) (Response, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
			return Response{Status: Completed}, nil
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	parent, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	session, token := openTestSession(t, parent, options)
	done := make(chan error, 1)
	go func() { _, err := session.Execute(parent, token, testScope(), testRequest()); done <- err }()
	select {
	case <-entered:
	case <-parent.Done():
		t.Fatal("handler did not start")
	}
	response, err := session.Execute(parent, token, testScope(), testRequest())
	if err != nil || response.Status != Running || calls.Load() != 1 {
		t.Errorf("duplicate: response=%+v calls=%d err=%v", response, calls.Load(), err)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-parent.Done():
		t.Fatal("handler did not finish")
	}
}

func TestLiveAuthorizationAndUncertainFailureNeverReplay(t *testing.T) {
	var revoked atomic.Bool
	var calls atomic.Int64
	options := testOptions()
	options.Authorize = func(context.Context, Scope, Action) error {
		if revoked.Load() {
			return errors.New("private credential")
		}
		return nil
	}
	options.Handle = func(context.Context, Scope, Request) (Response, error) {
		calls.Add(1)
		return Response{}, errors.New("provider token in diagnostic")
	}
	session, token := openTestSession(t, t.Context(), options)
	for range 2 {
		response, err := session.Execute(t.Context(), token, testScope(), testRequest())
		if response.Status != Failed || len(response.Data) != 0 || !apperror.IsCode(err, catalog.CodeInternal) {
			t.Fatalf("failure: response=%+v err=%v", response, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("failed operation replayed %d times", calls.Load())
	}
	revoked.Store(true)
	if _, err := session.Execute(t.Context(), token, testScope(), testRequest()); !apperror.IsCode(err, catalog.CodePermissionDenied) {
		t.Fatalf("revoked cached request accepted: %v", err)
	}
}

func TestCloseCancelsInFlightAndDoesNotPublishResult(t *testing.T) {
	entered := make(chan struct{})
	options := testOptions()
	options.Handle = func(ctx context.Context, _ Scope, _ Request) (Response, error) {
		close(entered)
		<-ctx.Done()
		return Response{Status: Completed, Data: json.RawMessage(`"secret"`)}, nil
	}
	parent, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	session, token := openTestSession(t, parent, options)
	done := make(chan error, 1)
	go func() {
		response, err := session.Execute(parent, token, testScope(), testRequest())
		if len(response.Data) > 0 {
			done <- errors.New("published revoked result")
			return
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-parent.Done():
		t.Fatal("handler did not start")
	}
	session.Close()
	select {
	case err := <-done:
		if !apperror.IsCode(err, catalog.CodeAuthenticationRequired) {
			t.Fatalf("close error=%v", err)
		}
	case <-parent.Done():
		t.Fatal("handler cancellation did not finish")
	}
}

func TestInvalidRequestsAndBounds(t *testing.T) {
	session, token := openTestSession(t, t.Context(), testOptions())
	for _, request := range []Request{
		{ID: "", Action: ReadFile, Arguments: json.RawMessage(`{}`)},
		{ID: "r", Action: "http.proxy", Arguments: json.RawMessage(`{}`)},
		{ID: "r", Action: ReadFile, Arguments: json.RawMessage(`null`)},
		{ID: "r", Action: ReadFile, Arguments: json.RawMessage(`[]`)},
		{ID: "r", Action: ReadFile, Arguments: json.RawMessage(`{} {}`)},
		{ID: "r", Action: ReadFile, Arguments: json.RawMessage(strings.Repeat(" ", MaxRequestBytes+1))},
	} {
		if _, err := session.Execute(t.Context(), token, testScope(), request); !apperror.IsCode(err, catalog.CodeRequestInvalid) {
			t.Fatalf("invalid request accepted: %+v err=%v", request, err)
		}
	}
	for i := range MaxRequests {
		request := testRequest()
		request.ID = strings.Repeat("x", i+1)
		if _, err := session.Execute(t.Context(), token, testScope(), request); err != nil {
			t.Fatal(err)
		}
	}
	request := testRequest()
	request.ID = "overflow"
	if _, err := session.Execute(t.Context(), token, testScope(), request); !apperror.IsCode(err, catalog.CodeQuotaExceeded) {
		t.Fatalf("request cap ignored: %v", err)
	}
}

func TestPendingApprovalIsAReceiptNotRepeatedExecution(t *testing.T) {
	var calls atomic.Int64
	options := testOptions()
	options.Handle = func(context.Context, Scope, Request) (Response, error) {
		calls.Add(1)
		return Response{Status: AwaitingApproval, Data: json.RawMessage(`{"receipt":"approval-1"}`)}, nil
	}
	session, token := openTestSession(t, t.Context(), options)
	for range 2 {
		response, err := session.Execute(t.Context(), token, testScope(), testRequest())
		if err != nil || response.Status != AwaitingApproval {
			t.Fatalf("pending=%+v err=%v", response, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("approval request replayed %d times", calls.Load())
	}
}

func TestSessionRequiresGovernanceAndBoundedLifetime(t *testing.T) {
	for _, alter := range []func(*Options){
		func(o *Options) { o.Authorize = nil },
		func(o *Options) { o.Handle = nil },
		func(o *Options) { o.TTL = 0 },
		func(o *Options) { o.TTL = MaxGrantTTL + time.Second },
	} {
		options := testOptions()
		alter(&options)
		if _, _, err := NewSession(t.Context(), testScope(), []Action{ReadFile}, options); !apperror.IsCode(err, catalog.CodeRequestInvalid) {
			t.Fatalf("bad options accepted: %v", err)
		}
	}
	session, token, err := NewSession(t.Context(), testScope(), []Action{ReadFile}, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.Execute(t.Context(), token, testScope(), testRequest()); !apperror.IsCode(err, catalog.CodePermissionDenied) {
		t.Fatalf("ungranted action accepted: %v", err)
	}
}
