package digitalhumantask

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingDispatcher struct{ runIDs []string }

func (d *recordingDispatcher) DispatchTaskRun(_ context.Context, runID string) error {
	d.runIDs = append(d.runIDs, runID)
	return nil
}

func TestServiceProposalLifecycleAndOptimisticRevision(t *testing.T) {
	service, dispatcher, scope, digitalHumanID := newTaskService(t)
	runAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	operationID := uuid.New()
	request := digitalhuman.CreateTaskRequest{
		Name: "Prepare launch brief", Goal: "Deliver a reviewable launch brief",
		SuccessCriteria: []string{"Includes owners", "Includes dates"}, Priority: digitalhuman.TaskPriorityHigh,
		Schedule:     digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
		DeliveryMode: digitalhuman.TaskDeliverySourceConversation,
	}
	mutation := digitalhuman.TaskMutationContext{
		ActorType: "digital_human", ActorRef: digitalHumanID.String(), OperationID: operationID,
		Source: "channel:feishu", SourceConversationID: "chat-1", SourceConversationType: "group", Proposed: true,
		ReplyTo: &invocation.ReplyAddress{Provider: "feishu", InstallationID: uuid.NewString(), ConversationID: "chat-1"},
	}

	created, err := service.CreateTask(t.Context(), scope, digitalHumanID, request, mutation)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if created.WorkStatus != digitalhuman.TaskWorkStatusProposed || created.LifecycleStatus != string(automationmodel.AutomationTaskStatusDraft) || created.Revision != 1 || created.NextRunAt != nil || created.ProposalExpiresAt == nil || created.SourceConversationType != "group" {
		t.Fatalf("created task = %#v", created)
	}
	replayed, err := service.CreateTask(t.Context(), scope, digitalHumanID, request, mutation)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replayed CreateTask() = %#v, %v", replayed, err)
	}

	visible, err := service.ListTasks(t.Context(), scope, digitalHumanID, digitalhuman.TaskListQuery{SourceConversationID: "chat-1"})
	if err != nil || visible.Total != 1 {
		t.Fatalf("ListTasks(source) total = %v, error = %v", visible, err)
	}
	hidden, err := service.ListTasks(t.Context(), scope, digitalHumanID, digitalhuman.TaskListQuery{SourceConversationID: "chat-2"})
	if err != nil || hidden.Total != 0 {
		t.Fatalf("ListTasks(other source) total = %v, error = %v", hidden, err)
	}

	confirmed, err := service.ApplyTaskAction(t.Context(), scope, digitalHumanID, created.ID, digitalhuman.TaskActionConfirm, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 1}, mutationWithOperation(mutation))
	if err != nil || confirmed.WorkStatus != digitalhuman.TaskWorkStatusPending || confirmed.NextRunAt == nil || confirmed.ProposalExpiresAt != nil || confirmed.Revision != 2 {
		t.Fatalf("Confirm() task = %#v, error = %v", confirmed, err)
	}
	paused, err := service.ApplyTaskAction(t.Context(), scope, digitalHumanID, created.ID, digitalhuman.TaskActionPause, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 2}, mutationWithOperation(mutation))
	if err != nil || paused.LifecycleStatus != string(automationmodel.AutomationTaskStatusPaused) || paused.NextRunAt != nil {
		t.Fatalf("Pause() task = %#v, error = %v", paused, err)
	}
	stored, err := service.GetTask(t.Context(), scope, digitalHumanID, created.ID, "chat-1")
	if err != nil || stored.NextRunAt != nil {
		t.Fatalf("stored paused task = %#v, error = %v", stored, err)
	}
	if _, err := service.UpdateTask(t.Context(), scope, digitalHumanID, created.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: 2}, mutationWithOperation(mutation)); !errors.Is(err, digitalhuman.ErrTaskConflict) {
		t.Fatalf("stale UpdateTask() error = %v, want conflict", err)
	}
	resumed, err := service.ApplyTaskAction(t.Context(), scope, digitalHumanID, created.ID, digitalhuman.TaskActionResume, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 3}, mutationWithOperation(mutation))
	if err != nil || resumed.NextRunAt == nil {
		t.Fatalf("Resume() task = %#v, error = %v", resumed, err)
	}
	running, err := service.ApplyTaskAction(t.Context(), scope, digitalHumanID, created.ID, digitalhuman.TaskActionRunNow, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 4}, mutationWithOperation(mutation))
	if err != nil || running.WorkStatus != digitalhuman.TaskWorkStatusInProgress || len(dispatcher.runIDs) != 1 {
		t.Fatalf("RunNow() task = %#v, dispatched = %v, error = %v", running, dispatcher.runIDs, err)
	}
	runs, err := service.ListTaskRuns(t.Context(), scope, digitalHumanID, created.ID, 1, 20, "chat-1")
	if err != nil || runs.Total != 1 || runs.Items[0].Status != string(automationmodel.AutomationTaskRunStatusQueued) {
		t.Fatalf("ListTaskRuns() = %#v, error = %v", runs, err)
	}
}

func TestChannelTaskPolicyDefaultsOffAndUsesRevision(t *testing.T) {
	service, _, scope, digitalHumanID := newTaskService(t)
	installationID := uuid.New()
	subjectType := runtimeidentity.SubjectTypeDigitalHuman
	installation := &channelruntime.Installation{
		ID: installationID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Provider: "feishu", ConnectionID: installationID, Name: "Feishu", DefaultSubjectType: &subjectType,
		DefaultSubjectID: &digitalHumanID, Status: channelruntime.InstallationStatusActive, Revision: 1,
		CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID,
	}
	if err := service.db.Create(installation).Error; err != nil {
		t.Fatalf("create installation: %v", err)
	}
	policy, err := service.GetChannelTaskPolicy(t.Context(), scope, digitalHumanID, installationID)
	if err != nil || policy.Enabled || policy.Revision != 0 {
		t.Fatalf("default policy = %#v, error = %v", policy, err)
	}
	policy, err = service.UpdateChannelTaskPolicy(t.Context(), scope, digitalHumanID, installationID, digitalhuman.UpdateChannelTaskPolicyRequest{Enabled: true, ExpectedRevision: 0})
	if err != nil || !policy.Enabled || policy.Revision != 1 {
		t.Fatalf("enabled policy = %#v, error = %v", policy, err)
	}
	if _, err := service.UpdateChannelTaskPolicy(t.Context(), scope, digitalHumanID, installationID, digitalhuman.UpdateChannelTaskPolicyRequest{Enabled: false, ExpectedRevision: 0}); !errors.Is(err, digitalhuman.ErrTaskConflict) {
		t.Fatalf("stale policy update error = %v, want conflict", err)
	}
}

func TestServiceStoresCanonicalCronSchedule(t *testing.T) {
	service, _, scope, digitalHumanID := newTaskService(t)
	created, err := service.CreateTask(t.Context(), scope, digitalHumanID, digitalhuman.CreateTaskRequest{
		Name: "Weekly review", Goal: "Review open work every Monday",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 9 * * 1", Timezone: "Asia/Shanghai"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask(cron) error = %v", err)
	}
	if created.Schedule.Cron != "0 9 * * 1" || created.NextRunAt == nil {
		t.Fatalf("created cron task = %#v", created)
	}
	var stored automationmodel.AutomationTask
	if err := service.db.First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatalf("load cron task: %v", err)
	}
	if stored.ScheduleConfig["cron_expr"] != "0 9 * * 1" {
		t.Fatalf("stored schedule config = %#v", stored.ScheduleConfig)
	}
}

func TestServiceRejectsCompletingAnActiveRecurringTask(t *testing.T) {
	service, _, scope, digitalHumanID := newTaskService(t)
	created, err := service.CreateTask(t.Context(), scope, digitalHumanID, digitalhuman.CreateTaskRequest{
		Name: "Daily reminder", Goal: "Remind the source conversation every day",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 8 * * *", Timezone: "Asia/Shanghai"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask(cron) error = %v", err)
	}

	_, err = service.SetTaskWorkStatus(
		t.Context(), scope, digitalHumanID, created.ID, digitalhuman.TaskWorkStatusCompleted, created.Revision,
		digitalhuman.TaskMutationContext{ActorType: "digital_human", ActorRef: digitalHumanID.String(), OperationID: uuid.New(), Source: "schedule"},
	)
	if !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("SetTaskWorkStatus(completed) error = %v, want policy rejection", err)
	}
	stored, err := service.GetTask(t.Context(), scope, digitalHumanID, created.ID, "")
	if err != nil || stored.WorkStatus != digitalhuman.TaskWorkStatusPending || stored.LifecycleStatus != string(automationmodel.AutomationTaskStatusActive) {
		t.Fatalf("stored recurring task = %#v, error = %v", stored, err)
	}
}

func TestServiceListTasksAppliesHistoryWindow(t *testing.T) {
	service, _, scope, digitalHumanID := newTaskService(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	create := func(name string) *digitalhuman.Task {
		t.Helper()
		item, err := service.CreateTask(t.Context(), scope, digitalHumanID, digitalhuman.CreateTaskRequest{
			Name: name, Goal: "Retain a bounded task history", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual, Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"})
		if err != nil {
			t.Fatalf("CreateTask(%q) error = %v", name, err)
		}
		return item
	}
	recent, old, open := create("Recent completion"), create("Old completion"), create("Still open")
	for _, update := range []struct {
		id        uuid.UUID
		updatedAt time.Time
	}{
		{id: recent.ID, updatedAt: now.Add(-24 * time.Hour)},
		{id: old.ID, updatedAt: now.Add(-45 * 24 * time.Hour)},
	} {
		if err := service.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", update.id).UpdateColumns(map[string]any{
			"work_status": automationmodel.AutomationWorkStatusCompleted,
			"status":      automationmodel.AutomationTaskStatusCompleted,
			"updated_at":  update.updatedAt,
		}).Error; err != nil {
			t.Fatalf("complete task %s: %v", update.id, err)
		}
	}
	after := now.Add(-30 * 24 * time.Hour)
	result, err := service.ListTasks(t.Context(), scope, digitalHumanID, digitalhuman.TaskListQuery{
		Statuses: []digitalhuman.TaskWorkStatus{digitalhuman.TaskWorkStatusCompleted}, UpdatedAfter: &after, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListTasks(history) error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != recent.ID {
		t.Fatalf("history items = %#v, want only recent task %s; old = %s, open = %s", result.Items, recent.ID, old.ID, open.ID)
	}
}

func TestServiceListTasksCountsSuccessfulRunsWithinWindow(t *testing.T) {
	service, _, scope, digitalHumanID := newTaskService(t)
	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	createTask := func(subjectID uuid.UUID, name string) *digitalhuman.Task {
		t.Helper()
		task, err := service.CreateTask(t.Context(), scope, subjectID, digitalhuman.CreateTaskRequest{
			Name: name, Goal: "Exercise the task summary", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 8 * * *", Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: scope.ActorID.String(), OperationID: uuid.New(), Source: "console"})
		if err != nil {
			t.Fatalf("CreateTask(%q) error = %v", name, err)
		}
		return task
	}
	currentTask := createTask(digitalHumanID, "Daily reminder")
	otherTask := createTask(uuid.New(), "Other digital human")
	createRun := func(taskID uuid.UUID, status automationmodel.AutomationTaskRunStatus, finishedAt time.Time) {
		t.Helper()
		run := &automationmodel.AutomationTaskRun{
			TaskID: taskID.String(), TriggerSource: automationmodel.AutomationTriggerSourceScheduler,
			ScheduledFor: finishedAt.Add(-time.Minute), FinishedAt: &finishedAt, Status: status,
		}
		if err := service.db.Create(run).Error; err != nil {
			t.Fatalf("create task run: %v", err)
		}
	}
	createRun(currentTask.ID, automationmodel.AutomationTaskRunStatusSucceeded, start.Add(8*time.Hour))
	createRun(currentTask.ID, automationmodel.AutomationTaskRunStatusFailed, start.Add(9*time.Hour))
	createRun(currentTask.ID, automationmodel.AutomationTaskRunStatusSucceeded, start.Add(-time.Second))
	createRun(currentTask.ID, automationmodel.AutomationTaskRunStatusSucceeded, end)
	createRun(otherTask.ID, automationmodel.AutomationTaskRunStatusSucceeded, start.Add(10*time.Hour))

	result, err := service.ListTasks(t.Context(), scope, digitalHumanID, digitalhuman.TaskListQuery{
		RunFinishedAfter: &start, RunFinishedBefore: &end, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListTasks(run window) error = %v", err)
	}
	if result.SuccessfulRunCount != 1 {
		t.Fatalf("SuccessfulRunCount = %d, want 1", result.SuccessfulRunCount)
	}
	if _, err := service.ListTasks(t.Context(), scope, digitalHumanID, digitalhuman.TaskListQuery{
		RunFinishedAfter: &end, RunFinishedBefore: &start,
	}); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("ListTasks(invalid run window) error = %v, want invalid request", err)
	}
}

func newTaskService(t *testing.T) (*Service, *recordingDispatcher, digitalhuman.Scope, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:digital-human-tasks-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&automationmodel.AutomationTask{}, &automationmodel.AutomationTaskAction{},
		&automationmodel.AutomationTaskRun{}, &automationmodel.AutomationActionRun{},
		&automationmodel.AutomationTaskActivity{}, &channelruntime.Installation{}, &channelTaskPolicy{},
	); err != nil {
		t.Fatalf("migrate task service: %v", err)
	}
	dispatcher := &recordingDispatcher{}
	scope := digitalhuman.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	return NewService(db, dispatcher, nil), dispatcher, scope, uuid.New()
}

func mutationWithOperation(value digitalhuman.TaskMutationContext) digitalhuman.TaskMutationContext {
	value.OperationID = uuid.New()
	return value
}
