package digitalhumantask

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// knownChannels accepts only the team channels it was given.
type knownChannels []uuid.UUID

func (k knownChannels) ActiveWorkspaceChannels(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) (bool, error) {
	for _, id := range ids {
		if !slices.Contains(k, id) {
			return false, nil
		}
	}
	return true, nil
}

func TestTaskTeamChannelsFollowTheRequester(t *testing.T) {
	fixture := newVisibilityFixture(t)
	service, ctx := fixture.service, t.Context()
	first, second := uuid.New(), uuid.New()
	mutation := func(access digitalhuman.TaskAccessContext) digitalhuman.TaskMutationContext {
		return digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"}
	}
	update := func(access digitalhuman.TaskAccessContext, id uuid.UUID, req digitalhuman.UpdateTaskRequest) (*digitalhuman.Task, error) {
		t.Helper()
		current, err := service.GetTask(ctx, access, id)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		req.ExpectedRevision = current.Revision
		return service.UpdateTask(ctx, access, id, req, mutation(access))
	}
	channels := func(ids ...uuid.UUID) *[]uuid.UUID { return &ids }
	shared := digitalhuman.TaskVisibilityWorkspace
	private := digitalhuman.TaskVisibilityRequester

	// No deployment-wide channels: nothing can be chosen.
	if _, err := update(fixture.memberA, fixture.privateA.ID, digitalhuman.UpdateTaskRequest{Visibility: &shared, NotifyChannelIDs: channels(first)}); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("channels without a checker: %v", err)
	}
	service.SetNotifyChannels(knownChannels{first, second})

	// A private task has no team to report to.
	if _, err := update(fixture.memberA, fixture.privateA.ID, digitalhuman.UpdateTaskRequest{NotifyChannelIDs: channels(first)}); !errors.Is(err, digitalhuman.ErrTaskVisibilityForbidden) {
		t.Fatalf("channels on a private task: %v", err)
	}
	// The requester shares it and picks a channel in the same change.
	task, err := update(fixture.memberA, fixture.privateA.ID, digitalhuman.UpdateTaskRequest{Visibility: &shared, NotifyChannelIDs: channels(first, first)})
	if err != nil || !slices.Equal(task.NotifyChannelIDs, []uuid.UUID{first}) {
		t.Fatalf("share with a channel = %#v, %v", task, err)
	}
	if _, err := update(fixture.memberA, task.ID, digitalhuman.UpdateTaskRequest{NotifyChannelIDs: channels(first, uuid.New())}); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("unknown channel: %v", err)
	}
	if read, _ := service.GetTask(ctx, fixture.memberA, task.ID); read.NotifyChannelAccess != digitalhuman.TaskNotifyChannelsManage {
		t.Fatalf("requester access = %q", read.NotifyChannelAccess)
	}

	// A manager may stop the reports but not send them somewhere new.
	if read, _ := service.GetTask(ctx, fixture.manager, task.ID); read.NotifyChannelAccess != digitalhuman.TaskNotifyChannelsRemove {
		t.Fatalf("manager access = %q", read.NotifyChannelAccess)
	}
	if _, err := update(fixture.manager, task.ID, digitalhuman.UpdateTaskRequest{NotifyChannelIDs: channels(first, second)}); !errors.Is(err, digitalhuman.ErrTaskVisibilityForbidden) {
		t.Fatalf("manager adding a channel: %v", err)
	}
	if task, err = update(fixture.manager, task.ID, digitalhuman.UpdateTaskRequest{NotifyChannelIDs: channels()}); err != nil || len(task.NotifyChannelIDs) != 0 {
		t.Fatalf("manager removing channels = %#v, %v", task, err)
	}

	// Withdrawing the task from the team drops its team channels for good.
	if _, err = update(fixture.memberA, task.ID, digitalhuman.UpdateTaskRequest{NotifyChannelIDs: channels(first, second)}); err != nil {
		t.Fatal(err)
	}
	if task, err = update(fixture.memberA, task.ID, digitalhuman.UpdateTaskRequest{Visibility: &private}); err != nil || len(task.NotifyChannelIDs) != 0 {
		t.Fatalf("withdrawn task = %#v, %v", task, err)
	}
	if task, err = update(fixture.memberA, task.ID, digitalhuman.UpdateTaskRequest{Visibility: &shared}); err != nil || len(task.NotifyChannelIDs) != 0 {
		t.Fatalf("re-shared task = %#v, %v", task, err)
	}

	// Task center tasks may choose channels when created; members' tasks
	// start private, and external tasks never report to the team.
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	create := func(access digitalhuman.TaskAccessContext) (*digitalhuman.Task, error) {
		return service.CreateTask(ctx, access, digitalhuman.CreateTaskRequest{
			Name: "reported", Goal: "report", NotifyChannelIDs: []uuid.UUID{second},
			Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"},
		}, mutation(access))
	}
	if created, err := create(fixture.taskCenter); err != nil || !slices.Equal(created.NotifyChannelIDs, []uuid.UUID{second}) {
		t.Fatalf("task center create = %#v, %v", created, err)
	}
	for _, access := range []digitalhuman.TaskAccessContext{fixture.memberA, fixture.external} {
		if _, err := create(access); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
			t.Fatalf("create with channels as %s: %v", access.Viewer.Mode, err)
		}
	}
}

func TestReportDigestBelongsToScheduledTasks(t *testing.T) {
	cron := &automationmodel.AutomationTask{ScheduleType: automationmodel.AutomationScheduleTypeCron,
		DeliveryConfig: withReportDigest(map[string]any{"mode": "task_only"}, digitalhuman.TaskReportDigestWeekly)}
	if got := taskReportDigest(cron); got != digitalhuman.TaskReportDigestWeekly || cron.DeliveryConfig["mode"] != "task_only" {
		t.Fatalf("a scheduled task's digest = %q, config %#v", got, cron.DeliveryConfig)
	}
	once := &automationmodel.AutomationTask{ScheduleType: automationmodel.AutomationScheduleTypeOnce, DeliveryConfig: cron.DeliveryConfig}
	if got := taskReportDigest(once); got != digitalhuman.TaskReportDigestNone {
		t.Fatalf("a one-off task kept a digest: %q", got)
	}
	if cleared := withReportDigest(cron.DeliveryConfig, digitalhuman.TaskReportDigestNone); cleared[reportDigestKey] != nil {
		t.Fatalf("clearing the digest left %#v", cleared)
	}
	if digitalhuman.TaskReportDigest("hourly").Valid() {
		t.Fatal("an unknown digest is valid")
	}
}
