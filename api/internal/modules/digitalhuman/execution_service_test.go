package digitalhuman

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

type linuxExecutionProviderStub struct {
	calls  int
	err    error
	closed bool
}

func (s *linuxExecutionProviderStub) Prepare(_ context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error) {
	s.calls++
	if resolved.Snapshot.Revision != run.ExecutionSnapshotRevision || resolved.Spec.InvocationID != event.ID {
		return nil, ErrRunConflict
	}
	if s.err != nil || s.closed {
		return nil, s.err
	}
	return func(*chatruntime.PreparedChat) []skillloop.RuntimeTool { return nil }, nil
}

func TestRuntimeServiceLinuxPreparationPrecedesChatAndKeepsBilling(t *testing.T) {
	for _, test := range []struct {
		name                  string
		corrupt, deny, closed bool
	}{
		{name: "injected"}, {name: "closed entry", closed: true}, {name: "denied", deny: true}, {name: "corrupt snapshot", corrupt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
			service := newTestRuntimeService(t, store, dispatcher, chat)
			provider := &linuxExecutionProviderStub{closed: test.closed}
			if test.deny {
				provider.err = errors.New("private authorization diagnostic")
			}
			WithLinuxExecutionRuntime(provider)(service)
			app := uuid.New()
			run, _, err := service.SubmitManual(t.Context(), testRunScope(), app, ManualInvocationRequest{RequestID: uuid.New(), Query: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if test.corrupt {
				run.ExecutionSnapshotRevision = "sha256:corrupt"
			}
			err = service.Execute(t.Context(), run.ID)
			if test.deny || test.corrupt {
				if err == nil || chat.prepareCalls != 0 || run.Status != RunStatusFailed || run.ErrorMessage == "private authorization diagnostic" {
					t.Fatal("failed preparation reached chat or leaked its diagnostic")
				}
				wantCalls := 1
				if test.corrupt {
					wantCalls = 0
				}
				if provider.calls != wantCalls {
					t.Fatal("provider ran before stored integrity validation")
				}
				return
			}
			if err != nil || chat.prepareCalls != 1 || provider.calls != 1 || run.Status != RunStatusSucceeded ||
				(chat.config.RuntimeToolFactory == nil) != test.closed || chat.config.BillingAppID != app.String() || chat.config.BillingAppType != "digital_human" {
				t.Fatalf("chat injection changed existing execution/billing: err=%v config=%+v", err, chat.config)
			}
		})
	}
}
