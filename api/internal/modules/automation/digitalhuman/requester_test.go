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
	// The directory finds people by name, email, or ID prefix, newest first.
	// A manager also sees member B's private task, so three people asked.
	page, err := fixture.service.ListTaskRequesters(t.Context(), fixture.manager, digitalhuman.TaskRequesterQuery{})
	if err != nil || page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("requester directory = %#v, %v", page, err)
	}
	for query, want := range map[string]string{"ali": "Alice", "MANAGER@": "Manager", fixture.memberA.Principal.ID[:8]: "Alice"} {
		found, err := fixture.service.ListTaskRequesters(t.Context(), fixture.manager, digitalhuman.TaskRequesterQuery{Query: query})
		if err != nil || found.Total != 1 || found.Items[0].Name != want {
			t.Fatalf("search %q = %#v, %v", query, found, err)
		}
	}
	if _, err := fixture.service.ListTaskRequesters(t.Context(), fixture.memberA, digitalhuman.TaskRequesterQuery{}); err == nil {
		t.Fatal("a member listed requesters")
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
}

func TestTaskCenterLabelsAPIEndUsers(t *testing.T) {
	fixture := newVisibilityFixture(t)
	subject := uuid.New()
	if err := fixture.service.db.Exec("INSERT INTO digital_human_end_user_instances (organization_id, workspace_id, digital_human_id, subject_id) VALUES (?, ?, ?, ?)",
		fixture.manager.OrganizationID.String(), fixture.manager.WorkspaceID.String(), fixture.manager.DigitalHumanID.String(), subject.String()).Error; err != nil {
		t.Fatalf("insert instance: %v", err)
	}
	labeled, err := fixture.service.LabelTaskRequester(t.Context(), fixture.manager, subject, "  VIP Zhang  ")
	if err != nil || labeled.Name != "VIP Zhang" {
		t.Fatalf("label = %#v, %v", labeled, err)
	}
	if _, err := fixture.service.LabelTaskRequester(t.Context(), fixture.taskCenter, subject, "x"); err == nil {
		t.Fatal("a reader without update permission labeled a requester")
	}
	if _, err := fixture.service.LabelTaskRequester(t.Context(), fixture.manager, uuid.New(), "x"); err == nil {
		t.Fatal("labeled a requester with no instance")
	}
}
