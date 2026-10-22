package digitalhumantask

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestTaskCenterShowsAndFiltersRequesters(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	fixture := newVisibilityFixture(t)
	db := fixture.service.db
	for _, account := range []struct{ id, name, email string }{
		{fixture.taskCenter.Principal.ID, "Manager", "manager@example.com"},
		{fixture.memberA.Principal.ID, "Alice", "alice@example.com"},
	} {
		if err := db.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", account.id, account.name, account.email).Error; err != nil {
			t.Fatalf("insert account: %v", err)
		}
	}
	// Private tasks only show to their requester; share A's with the team.
	shared := digitalhuman.TaskVisibilityWorkspace
	if _, err := fixture.service.UpdateTask(t.Context(), fixture.memberA, fixture.privateA.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: fixture.privateA.Revision, Visibility: &shared}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); err != nil {
		t.Fatalf("share task: %v", err)
	}

	result, err := fixture.service.ListTasks(t.Context(), fixture.manager, digitalhuman.TaskListQuery{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	byName := map[string]*digitalhuman.TaskRequester{}
	for _, item := range result.Items {
		byName[item.Name] = item.Requester
	}
	if got := byName["shared"]; got == nil || got.Kind != digitalhuman.TaskRequesterMember || got.Name != "Manager" {
		t.Fatalf("task center task requester = %#v", got)
	}
	if got := byName["private A"]; got == nil || got.Name != "Alice" || got.Email != "alice@example.com" || got.ID != fixture.memberA.Principal.ID {
		t.Fatalf("member task requester = %#v", got)
	}
	if got := byName["conversation"]; got == nil || got.Kind != digitalhuman.TaskRequesterChannel || got.ID != "" {
		t.Fatalf("channel task requester = %#v", got)
	}
	if len(result.Requesters) == 0 {
		t.Fatal("task center lists no requesters")
	}

	filtered, err := fixture.service.ListTasks(t.Context(), fixture.manager, digitalhuman.TaskListQuery{RequesterID: fixture.memberA.Principal.ID})
	if err != nil || filtered.Total != 1 || filtered.Items[0].Name != "private A" {
		t.Fatalf("filter by requester = %#v, %v", filtered, err)
	}
	channels, err := fixture.service.ListTasks(t.Context(), fixture.manager, digitalhuman.TaskListQuery{RequesterKinds: []digitalhuman.TaskRequesterKind{digitalhuman.TaskRequesterChannel}})
	if err != nil || channels.Total != 1 || channels.Items[0].Name != "conversation" {
		t.Fatalf("filter by kind = %#v, %v", channels, err)
	}
	if _, err := fixture.service.ListTasks(t.Context(), fixture.manager, digitalhuman.TaskListQuery{RequesterKinds: []digitalhuman.TaskRequesterKind{"robot"}}); err == nil {
		t.Fatal("unknown requester kind accepted")
	}

	// Requesters see their own tasks elsewhere; only the task center names them.
	own, err := fixture.service.ListTasks(t.Context(), fixture.memberA, digitalhuman.TaskListQuery{})
	if err != nil {
		t.Fatalf("member ListTasks() error = %v", err)
	}
	for _, item := range own.Items {
		if item.Requester != nil {
			t.Fatalf("member view names requester of %q", item.Name)
		}
	}
	if own.Requesters != nil {
		t.Fatal("member view lists requesters")
	}
}
