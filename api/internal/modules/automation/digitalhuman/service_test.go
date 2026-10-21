package digitalhumantask

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
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
	service, dispatcher, access, digitalHumanID := newTaskService(t)
	access.Owner = digitalhuman.ConversationTaskOwner(uuid.New())
	access.Surface = digitalhuman.SurfaceChannel
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

	created, err := service.CreateTask(t.Context(), access, request, mutation)
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if created.WorkStatus != digitalhuman.TaskWorkStatusProposed || created.LifecycleStatus != string(automationmodel.AutomationTaskStatusDraft) || created.Revision != 1 || created.NextRunAt != nil || created.ProposalExpiresAt == nil || created.SourceConversationType != "group" {
		t.Fatalf("created task = %#v", created)
	}
	replayed, err := service.CreateTask(t.Context(), access, request, mutation)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replayed CreateTask() = %#v, %v", replayed, err)
	}

	visible, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
	if err != nil || visible.Total != 1 {
		t.Fatalf("ListTasks(source) total = %v, error = %v", visible, err)
	}
	otherConversation := access
	otherConversation.Owner = digitalhuman.ConversationTaskOwner(uuid.New())
	hidden, err := service.ListTasks(t.Context(), otherConversation, digitalhuman.TaskListQuery{})
	if err != nil || hidden.Total != 0 {
		t.Fatalf("ListTasks(other source) total = %v, error = %v", hidden, err)
	}

	confirmed, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionConfirm, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 1}, mutationWithOperation(mutation))
	if err != nil || confirmed.WorkStatus != digitalhuman.TaskWorkStatusPending || confirmed.NextRunAt == nil || confirmed.ProposalExpiresAt != nil || confirmed.Revision != 2 {
		t.Fatalf("Confirm() task = %#v, error = %v", confirmed, err)
	}
	paused, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionPause, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 2}, mutationWithOperation(mutation))
	if err != nil || paused.LifecycleStatus != string(automationmodel.AutomationTaskStatusPaused) || paused.NextRunAt != nil {
		t.Fatalf("Pause() task = %#v, error = %v", paused, err)
	}
	stored, err := service.GetTask(t.Context(), access, created.ID)
	if err != nil || stored.NextRunAt != nil {
		t.Fatalf("stored paused task = %#v, error = %v", stored, err)
	}
	if _, err := service.UpdateTask(t.Context(), access, created.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: 2}, mutationWithOperation(mutation)); !errors.Is(err, digitalhuman.ErrTaskConflict) {
		t.Fatalf("stale UpdateTask() error = %v, want conflict", err)
	}
	resumed, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionResume, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 3}, mutationWithOperation(mutation))
	if err != nil || resumed.NextRunAt == nil {
		t.Fatalf("Resume() task = %#v, error = %v", resumed, err)
	}
	running, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionRunNow, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: 4}, mutationWithOperation(mutation))
	if err != nil || running.WorkStatus != digitalhuman.TaskWorkStatusInProgress || len(dispatcher.runIDs) != 1 {
		t.Fatalf("RunNow() task = %#v, dispatched = %v, error = %v", running, dispatcher.runIDs, err)
	}
	runs, err := service.ListTaskRuns(t.Context(), access, created.ID, 1, 20)
	if err != nil || runs.Total != 1 || runs.Items[0].Status != string(automationmodel.AutomationTaskRunStatusQueued) {
		t.Fatalf("ListTaskRuns() = %#v, error = %v", runs, err)
	}
}

func TestServiceResumeClearsBlockedWorkStatus(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	created, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
		Name: "Blocked task", Goal: "Recover a task after a scheduler failure",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: func() *time.Time {
			value := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
			return &value
		}(), Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionPause, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: created.Revision}, mutationWithOperation(digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	blocked := automationmodel.AutomationWorkStatusBlocked
	paused := automationmodel.AutomationTaskStatusPaused
	if err := service.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", created.ID).Updates(map[string]any{"work_status": blocked, "status": paused, "next_run_at": nil}).Error; err != nil {
		t.Fatalf("seed blocked task: %v", err)
	}
	resumed, err := service.ApplyTaskAction(t.Context(), access, created.ID, digitalhuman.TaskActionResume, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: created.Revision + 1}, mutationWithOperation(digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"}))
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if resumed.WorkStatus != digitalhuman.TaskWorkStatusPending || resumed.LifecycleStatus != string(automationmodel.AutomationTaskStatusActive) || resumed.NextRunAt == nil {
		t.Fatalf("resumed task = %#v, want pending/active with next run", resumed)
	}
}

func TestServiceApprovedLifecycleUpdateReactivatesCompletedTask(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	created, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
		Name: "Completed task", Goal: "Re-activate after approval",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: func() *time.Time {
			value := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
			return &value
		}(), Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	completed := automationmodel.AutomationWorkStatusCompleted
	status := automationmodel.AutomationTaskStatusCompleted
	if err := service.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", created.ID).Updates(map[string]any{
		"work_status": completed,
		"status":      status,
		"next_run_at": nil,
	}).Error; err != nil {
		t.Fatalf("seed completed task: %v", err)
	}
	runAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	lifecycle := string(automationmodel.AutomationTaskStatusActive)
	updated, err := service.UpdateTask(t.Context(), access, created.ID, digitalhuman.UpdateTaskRequest{
		ExpectedRevision: created.Revision,
		LifecycleStatus:  &lifecycle,
		Schedule:         &digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}
	if updated.LifecycleStatus != string(automationmodel.AutomationTaskStatusActive) || updated.WorkStatus != digitalhuman.TaskWorkStatusPending || updated.NextRunAt == nil {
		t.Fatalf("updated task = %#v, want active/pending with next run", updated)
	}
}

func TestServiceStoresCanonicalCronSchedule(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	created, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
		Name: "Weekly review", Goal: "Review open work every Monday",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 9 * * 1", Timezone: "Asia/Shanghai"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
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

func TestServicePreviewTaskScheduleUsesCanonicalTimezoneAndCron(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	preview, err := service.PreviewTaskSchedule(t.Context(), access, digitalhuman.TaskSchedulePreviewRequest{
		Type: digitalhuman.TaskScheduleOnce, LocalRunAt: "2026-12-15T09:30", Timezone: "Asia/Shanghai",
	})
	if err != nil || preview.NextRunAt == nil || preview.Schedule.RunAt == nil ||
		preview.Schedule.RunAt.UTC().Format(time.RFC3339) != "2026-12-15T01:30:00Z" {
		t.Fatalf("once preview = %#v, error = %v", preview, err)
	}
	preview, err = service.PreviewTaskSchedule(t.Context(), access, digitalhuman.TaskSchedulePreviewRequest{
		Type: digitalhuman.TaskScheduleCron, Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai",
	})
	if err != nil || preview.NextRunAt == nil || preview.Schedule.Cron != "0 9 * * 1-5" {
		t.Fatalf("cron preview = %#v, error = %v", preview, err)
	}
	for _, request := range []digitalhuman.TaskSchedulePreviewRequest{
		{Type: digitalhuman.TaskScheduleCron, Cron: "not a cron", Timezone: "UTC"},
		{Type: digitalhuman.TaskScheduleCron, Cron: "0 9 * * *", Timezone: "Invalid/Timezone"},
		{Type: digitalhuman.TaskScheduleOnce, LocalRunAt: "not a date", Timezone: "UTC"},
		{Type: digitalhuman.TaskScheduleOnce, LocalRunAt: "2026-03-08T02:30", Timezone: "America/New_York"},
		{Type: digitalhuman.TaskScheduleOnce, LocalRunAt: "2026-11-01T01:30", Timezone: "America/New_York"},
		{Type: digitalhuman.TaskScheduleManual, Timezone: "UTC"},
	} {
		if _, err := service.PreviewTaskSchedule(t.Context(), access, request); err == nil {
			t.Fatalf("invalid schedule accepted: %#v", request)
		}
	}
}

func TestServiceRejectsCompletingAnActiveRecurringTask(t *testing.T) {
	service, _, access, digitalHumanID := newTaskService(t)
	created, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
		Name: "Daily reminder", Goal: "Remind the source conversation every day",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 8 * * *", Timezone: "Asia/Shanghai"},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("CreateTask(cron) error = %v", err)
	}

	_, err = service.SetTaskWorkStatus(
		t.Context(), access, created.ID, digitalhuman.TaskWorkStatusCompleted, created.Revision,
		digitalhuman.TaskMutationContext{ActorType: "digital_human", ActorRef: digitalHumanID.String(), OperationID: uuid.New(), Source: "schedule"},
	)
	if !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("SetTaskWorkStatus(completed) error = %v, want policy rejection", err)
	}
	stored, err := service.GetTask(t.Context(), access, created.ID)
	if err != nil || stored.WorkStatus != digitalhuman.TaskWorkStatusPending || stored.LifecycleStatus != string(automationmodel.AutomationTaskStatusActive) {
		t.Fatalf("stored recurring task = %#v, error = %v", stored, err)
	}
}

func TestServiceListTasksAppliesHistoryWindow(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	create := func(name string) *digitalhuman.Task {
		t.Helper()
		item, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
			Name: name, Goal: "Retain a bounded task history", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual, Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
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
	result, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{
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
	service, _, access, digitalHumanID := newTaskService(t)
	start := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	createTask := func(subjectID uuid.UUID, name string) *digitalhuman.Task {
		t.Helper()
		taskAccess := access
		taskAccess.DigitalHumanID = subjectID
		task, err := service.CreateTask(t.Context(), taskAccess, digitalhuman.CreateTaskRequest{
			Name: name, Goal: "Exercise the task summary", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: "0 8 * * *", Timezone: "UTC"},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
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

	result, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{
		RunFinishedAfter: &start, RunFinishedBefore: &end, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListTasks(run window) error = %v", err)
	}
	if result.SuccessfulRunCount != 1 {
		t.Fatalf("SuccessfulRunCount = %d, want 1", result.SuccessfulRunCount)
	}
	if _, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{
		RunFinishedAfter: &end, RunFinishedBefore: &start,
	}); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("ListTasks(invalid run window) error = %v, want invalid request", err)
	}
}

func TestServiceIsolatesUserAndSharedTaskOwners(t *testing.T) {
	service, _, sharedAccess, _ := newTaskService(t)
	accountA, accountB := uuid.New(), uuid.New()
	privateA := sharedAccess
	privateA.Principal.ID = accountA.String()
	privateA.Owner = digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountA))
	privateB := sharedAccess
	privateB.Principal.ID = accountB.String()
	privateB.Owner = digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountB))
	request := digitalhuman.CreateTaskRequest{Name: "Private follow-up", Goal: "Keep user work private", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual}}

	taskA, err := service.CreateTask(t.Context(), privateA, request, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: accountA.String(), OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("create account A task: %v", err)
	}
	if taskA.OwnerKind != digitalhuman.TaskOwnerUserSubject {
		t.Fatalf("owner kind = %q, want user_subject", taskA.OwnerKind)
	}
	if _, err := service.CreateTask(t.Context(), privateB, request, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: accountB.String(), OperationID: uuid.New(), Source: "console"}); err != nil {
		t.Fatalf("create account B task: %v", err)
	}

	for name, access := range map[string]digitalhuman.TaskAccessContext{"account A": privateA, "account B": privateB} {
		result, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
		if err != nil || result.Total != 1 {
			t.Fatalf("%s list = %#v, error = %v", name, result, err)
		}
	}
	if result, err := service.ListTasks(t.Context(), sharedAccess, digitalhuman.TaskListQuery{}); err != nil || result.Total != 0 {
		t.Fatalf("shared list = %#v, error = %v", result, err)
	}
	if _, err := service.GetTask(t.Context(), privateB, taskA.ID); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("account B GetTask(account A) error = %v, want not found", err)
	}
	operations := []struct {
		name string
		call func() error
	}{
		{name: "update", call: func() error {
			_, err := service.UpdateTask(t.Context(), privateB, taskA.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: taskA.Revision}, digitalhuman.TaskMutationContext{OperationID: uuid.New()})
			return err
		}},
		{name: "cancel", call: func() error {
			_, err := service.ApplyTaskAction(t.Context(), privateB, taskA.ID, digitalhuman.TaskActionCancel, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: taskA.Revision}, digitalhuman.TaskMutationContext{OperationID: uuid.New()})
			return err
		}},
		{name: "runs", call: func() error {
			_, err := service.ListTaskRuns(t.Context(), privateB, taskA.ID, 1, 20)
			return err
		}},
		{name: "activities", call: func() error {
			_, err := service.ListTaskActivities(t.Context(), privateB, taskA.ID, 1, 20)
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.call(); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
				t.Fatalf("account B %s(account A) error = %v, want not found", operation.name, err)
			}
		})
	}
}

func TestServiceLimitsScheduledAccessToCurrentTaskStatus(t *testing.T) {
	service, _, sharedAccess, _ := newTaskService(t)
	created, err := service.CreateTask(t.Context(), sharedAccess, digitalhuman.CreateTaskRequest{
		Name: "Scheduled work", Goal: "Execute one bounded task", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual},
	}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: sharedAccess.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatalf("create shared task: %v", err)
	}
	currentID := created.ID
	scheduled := digitalhuman.TaskAccessContext{
		OrganizationID: sharedAccess.OrganizationID, WorkspaceID: sharedAccess.WorkspaceID, DigitalHumanID: sharedAccess.DigitalHumanID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"},
		Surface:   digitalhuman.SurfaceTask, CurrentTaskID: &currentID,
	}
	if _, err := service.GetTask(t.Context(), scheduled, created.ID); err != nil {
		t.Fatalf("scheduled GetTask(current) error = %v", err)
	}
	if _, err := service.GetTask(t.Context(), scheduled, uuid.New()); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("scheduled GetTask(other) error = %v, want not found", err)
	}
	if _, err := service.UpdateTask(t.Context(), scheduled, created.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: created.Revision}, digitalhuman.TaskMutationContext{}); !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("scheduled UpdateTask error = %v, want policy rejection", err)
	}
	if _, err := service.SetTaskWorkStatus(t.Context(), scheduled, created.ID, digitalhuman.TaskWorkStatusCompleted, created.Revision, digitalhuman.TaskMutationContext{}); !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("scheduled completion error = %v, want policy rejection", err)
	}
}

func newTaskService(t *testing.T) (*Service, *recordingDispatcher, digitalhuman.TaskAccessContext, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:digital-human-tasks-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&usersubject.Subject{},
		&automationmodel.AutomationTask{}, &automationmodel.AutomationTaskAction{},
		&automationmodel.AutomationTaskRun{}, &automationmodel.AutomationActionRun{},
		&automationmodel.AutomationTaskActivity{}, &taskReportRow{},
	); err != nil {
		t.Fatalf("migrate task service: %v", err)
	}
	dispatcher := &recordingDispatcher{}
	digitalHumanID, accountID := uuid.New(), uuid.New()
	access := digitalhuman.TaskAccessContext{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: digitalHumanID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: accountID.String()},
		Owner:     digitalhuman.SharedTaskOwner(), Surface: digitalhuman.SurfaceConsole,
	}
	return NewService(db, dispatcher, nil), dispatcher, access, digitalHumanID
}

func mutationWithOperation(value digitalhuman.TaskMutationContext) digitalhuman.TaskMutationContext {
	value.OperationID = uuid.New()
	return value
}
