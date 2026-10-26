package digitalhuman

import (
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// A task run without the Linux execution runtime still writes its work into a
// task conversation, which the console chat list (source console) never shows.
func TestTaskRunConversationStaysOutOfConsoleChatList(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	source, _, event, _ := backgroundLinuxFixture(t, SurfaceTask)
	delete(event.Context, backgroundExecutionEntryKey)
	scope := Scope{OrganizationID: source.OrganizationID, WorkspaceID: source.WorkspaceID, ActorID: *source.ActorID, PrincipalID: *source.ActorID, PrincipalType: source.PrincipalType, Surface: SurfaceTask}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "Execute the confirmed task \"daily report\".", nil)
	if err != nil || !created {
		t.Fatalf("submit: created=%t error=%v", created, err)
	}
	if _, background := event.Context[backgroundExecutionEntryKey]; background {
		t.Fatal("fixture unexpectedly opted into the Linux runtime")
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	if chat.prepareCalls != 1 || chat.caller.Source != runtimemodel.ConversationSourceTask {
		t.Fatalf("task run persisted its conversation as %q, want %q", chat.caller.Source, runtimemodel.ConversationSourceTask)
	}
	if listed := digitalHumanConversationSourceForSurface(SurfaceConsole); listed == chat.caller.Source {
		t.Fatalf("console chat list source %q includes task run conversations", listed)
	}
}

func TestEveryTaskRunReadsItsConversationAsTask(t *testing.T) {
	for name, run := range map[string]*Run{
		"task without runtime marker": {Surface: SurfaceTask, InvocationEvent: []byte(`{"context":{}}`)},
		"Linux runtime task":          {Surface: SurfaceTask, InvocationEvent: []byte(`{"context":{"linux_execution_entry":"task"}}`)},
	} {
		if got := digitalHumanConversationSourceForRun(run); got != runtimemodel.ConversationSourceTask {
			t.Errorf("%s: source = %q, want task", name, got)
		}
	}
	if got := digitalHumanConversationSourceForRun(&Run{Surface: SurfaceConsole}); got != runtimemodel.ConversationSourceConsole {
		t.Errorf("console run source = %q, want console", got)
	}
}
