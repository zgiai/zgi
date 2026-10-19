package digitalhumantask

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type visibilityFixture struct {
	service                    *Service
	taskCenter, manager        digitalhuman.TaskAccessContext
	memberA, memberB, external digitalhuman.TaskAccessContext
	channel                    digitalhuman.TaskAccessContext
	shared, privateA, privateB *digitalhuman.Task
	conversation               *digitalhuman.Task
}

func newVisibilityFixture(t *testing.T) visibilityFixture {
	t.Helper()
	service, _, sharedAccess, _ := newTaskService(t)
	accountA, accountB := uuid.New(), uuid.New()
	member := func(account uuid.UUID) digitalhuman.TaskAccessContext {
		access := sharedAccess
		access.Principal.ID = account.String()
		access.Owner = digitalhuman.UserTaskOwner(usersubject.AccountSubject(account))
		access.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewMember}
		return access
	}
	fixture := visibilityFixture{service: service, memberA: member(accountA), memberB: member(accountB)}
	fixture.taskCenter = sharedAccess
	fixture.taskCenter.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewTaskCenter}
	fixture.manager = fixture.taskCenter
	fixture.manager.Viewer.Manager = true
	fixture.external = fixture.memberA
	fixture.external.Surface = digitalhuman.SurfaceWebApp
	fixture.external.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewExternal}
	fixture.channel = sharedAccess
	fixture.channel.Owner = digitalhuman.ConversationTaskOwner(uuid.New())
	fixture.channel.Surface = digitalhuman.SurfaceChannel
	fixture.channel.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewChannel}

	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	create := func(access digitalhuman.TaskAccessContext, name string) *digitalhuman.Task {
		t.Helper()
		task, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
			Name: name, Goal: name + " goal", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return task
	}
	fixture.shared = create(fixture.taskCenter, "shared")
	fixture.privateA = create(fixture.memberA, "private A")
	fixture.privateB = create(fixture.memberB, "private B")
	fixture.conversation = create(fixture.channel, "conversation")
	return fixture
}

func listNames(t *testing.T, service *Service, access digitalhuman.TaskAccessContext) map[string]bool {
	t.Helper()
	result, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	names := make(map[string]bool, len(result.Items))
	for _, item := range result.Items {
		names[item.Name] = true
	}
	if int64(len(names)) != result.Total {
		t.Fatalf("total %d does not match %d items", result.Total, len(names))
	}
	return names
}

func assertNames(t *testing.T, label string, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s sees %v, want %v", label, got, want)
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("%s sees %v, want %v", label, got, want)
		}
	}
}

func TestTaskVisibilityDefaultsFromOwnerAndRecordsConfirmation(t *testing.T) {
	fixture := newVisibilityFixture(t)
	for task, want := range map[*digitalhuman.Task]digitalhuman.TaskVisibility{
		fixture.shared: digitalhuman.TaskVisibilityWorkspace, fixture.privateA: digitalhuman.TaskVisibilityRequester,
		fixture.conversation: digitalhuman.TaskVisibilityConversation,
	} {
		if task.Visibility != want {
			t.Fatalf("%s visibility = %q, want %q", task.Name, task.Visibility, want)
		}
	}

	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	proposal, err := fixture.service.CreateTask(t.Context(), fixture.memberA, digitalhuman.CreateTaskRequest{
		Name: "proposal", Goal: "approve me", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "digital_human", ActorRef: fixture.memberA.DigitalHumanID.String(), OperationID: uuid.New(), Proposed: true})
	if err != nil || proposal.ConfirmedAt != nil {
		t.Fatalf("proposal = %#v, error = %v", proposal, err)
	}
	confirmed, err := fixture.service.ApplyTaskAction(t.Context(), fixture.memberA, proposal.ID, digitalhuman.TaskActionConfirm,
		digitalhuman.ApplyTaskActionRequest{ExpectedRevision: proposal.Revision},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: fixture.memberA.Principal.ID, OperationID: uuid.New()})
	if err != nil || confirmed.ConfirmedAt == nil {
		t.Fatalf("confirmed = %#v, error = %v", confirmed, err)
	}
	var row automationmodel.AutomationTask
	if err := fixture.service.db.Where("id = ?", proposal.ID.String()).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if pointerValue(row.ConfirmedByType) != "account" || pointerValue(row.ConfirmedByRef) != fixture.memberA.Principal.ID {
		t.Fatalf("confirmed by = %v/%v", row.ConfirmedByType, row.ConfirmedByRef)
	}
}

func TestTaskVisibilityDisabledKeepsOwnerNamespaces(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "false")
	fixture := newVisibilityFixture(t)
	assertNames(t, "member A", listNames(t, fixture.service, fixture.memberA), "private A")
	assertNames(t, "task center manager", listNames(t, fixture.service, fixture.manager), "shared")
	if occupancy, err := fixture.service.ListTaskOccupancy(t.Context(), fixture.memberA, 0); err != nil || occupancy != nil {
		t.Fatalf("occupancy without the switch = %v, %v", occupancy, err)
	}
	visibility := digitalhuman.TaskVisibilityRestricted
	if _, err := fixture.service.UpdateTask(t.Context(), fixture.manager, fixture.shared.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: fixture.shared.Revision, Visibility: &visibility}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("visibility update without the switch error = %v", err)
	}
}

func TestTaskVisibilityReadRulesAndMutationScope(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	fixture := newVisibilityFixture(t)
	service := fixture.service

	assertNames(t, "task center", listNames(t, service, fixture.taskCenter), "shared")
	assertNames(t, "task center manager", listNames(t, service, fixture.manager), "shared", "private A", "private B", "conversation")
	assertNames(t, "member A", listNames(t, service, fixture.memberA), "shared", "private A")
	assertNames(t, "member B", listNames(t, service, fixture.memberB), "shared", "private B")
	assertNames(t, "channel", listNames(t, service, fixture.channel), "conversation")
	assertNames(t, "external", listNames(t, service, fixture.external), "private A")

	if _, err := service.GetTask(t.Context(), fixture.memberA, fixture.privateB.ID); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("member A GetTask(private B) error = %v", err)
	}
	if _, err := service.GetTask(t.Context(), fixture.memberA, fixture.shared.ID); err != nil {
		t.Fatalf("member A GetTask(shared) error = %v", err)
	}
	if _, err := service.ApplyTaskAction(t.Context(), fixture.memberA, fixture.shared.ID, digitalhuman.TaskActionPause,
		digitalhuman.ApplyTaskActionRequest{ExpectedRevision: fixture.shared.Revision}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("member A pause(shared) error = %v, want not found", err)
	}
	if _, err := service.ApplyTaskAction(t.Context(), fixture.taskCenter, fixture.privateB.ID, digitalhuman.TaskActionPause,
		digitalhuman.ApplyTaskActionRequest{ExpectedRevision: fixture.privateB.Revision}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("non-manager pause(private B) error = %v, want not found", err)
	}
	if _, err := service.ApplyTaskAction(t.Context(), fixture.manager, fixture.privateB.ID, digitalhuman.TaskActionPause,
		digitalhuman.ApplyTaskActionRequest{ExpectedRevision: fixture.privateB.Revision}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: fixture.manager.Principal.ID, OperationID: uuid.New()}); err != nil {
		t.Fatalf("manager pause(private B) error = %v", err)
	}

	restricted := digitalhuman.TaskVisibilityRestricted
	if _, err := service.UpdateTask(t.Context(), fixture.taskCenter, fixture.shared.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: fixture.shared.Revision, Visibility: &restricted}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("non-manager visibility update error = %v", err)
	}
	invalid := digitalhuman.TaskVisibility("everyone")
	if _, err := service.UpdateTask(t.Context(), fixture.manager, fixture.shared.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: fixture.shared.Revision, Visibility: &invalid}, digitalhuman.TaskMutationContext{OperationID: uuid.New()}); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("invalid visibility update error = %v", err)
	}
	updated, err := service.UpdateTask(t.Context(), fixture.manager, fixture.shared.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: fixture.shared.Revision, Visibility: &restricted}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: fixture.manager.Principal.ID, OperationID: uuid.New()})
	if err != nil || updated.Visibility != digitalhuman.TaskVisibilityRestricted {
		t.Fatalf("manager visibility update = %#v, %v", updated, err)
	}
	assertNames(t, "member A after restriction", listNames(t, service, fixture.memberA), "private A")
	assertNames(t, "task center after restriction", listNames(t, service, fixture.taskCenter))
	assertNames(t, "manager after restriction", listNames(t, service, fixture.manager), "shared", "private A", "private B", "conversation")
}

func TestTaskOccupancyRedactsHiddenOpenWork(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	fixture := newVisibilityFixture(t)
	runAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if _, err := fixture.service.CreateTask(t.Context(), fixture.memberB, digitalhuman.CreateTaskRequest{
		Name: "unapproved", Goal: "proposal", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "digital_human", ActorRef: fixture.memberB.DigitalHumanID.String(), OperationID: uuid.New(), Proposed: true}); err != nil {
		t.Fatal(err)
	}

	occupancy, err := fixture.service.ListTaskOccupancy(t.Context(), fixture.memberA, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Member A sees shared and private A; private B and the channel task are
	// committed hidden work. The unapproved proposal is not workload yet.
	if len(occupancy) != 2 {
		t.Fatalf("member A occupancy = %#v, want two hidden tasks", occupancy)
	}
	for _, item := range occupancy {
		if item.NextRunAt == nil || item.WorkStatus != digitalhuman.TaskWorkStatusPending || item.Priority == "" {
			t.Fatalf("occupancy item = %#v", item)
		}
	}
	channel, err := fixture.service.ListTaskOccupancy(t.Context(), fixture.channel, 0)
	if err != nil || len(channel) != 3 {
		t.Fatalf("channel occupancy = %#v, %v; want shared, private A and private B", channel, err)
	}
	for name, access := range map[string]digitalhuman.TaskAccessContext{"external": fixture.external, "manager": fixture.manager} {
		if items, err := fixture.service.ListTaskOccupancy(t.Context(), access, 0); err != nil || items != nil {
			t.Fatalf("%s occupancy = %#v, %v; want none", name, items, err)
		}
	}
}

func TestVisibleTasksReportWhetherTheReaderCanChangeThem(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	fixture := newVisibilityFixture(t)
	check := func(label string, access digitalhuman.TaskAccessContext, want map[string]bool) {
		t.Helper()
		result, err := fixture.service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Items {
			if expected, ok := want[item.Name]; !ok || item.CanChange != expected {
				t.Fatalf("%s: %s can_change = %v, want %v", label, item.Name, item.CanChange, expected)
			}
		}
	}
	check("member A", fixture.memberA, map[string]bool{"shared": false, "private A": true})
	check("task center", fixture.taskCenter, map[string]bool{"shared": true})
	check("manager", fixture.manager, map[string]bool{"shared": true, "private A": true, "private B": true, "conversation": true})
	shared, err := fixture.service.GetTask(t.Context(), fixture.memberA, fixture.shared.ID)
	if err != nil || shared.CanChange {
		t.Fatalf("member A GetTask(shared) = %#v, %v", shared, err)
	}
}
