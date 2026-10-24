package digitalhumantask

import (
	"testing"
	"time"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

func TestReportsWaitForReviewOnlyWhenTheirTaskAsks(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	report := func(review digitalhuman.TaskReportReview, succeeded bool, result string) *taskReportRow {
		t.Helper()
		runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
		task, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Task " + string(review), Goal: "Do it",
			Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"}, ReportReview: review},
			digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
		if err != nil {
			t.Fatal(err)
		}
		if task.ReportReview == "" {
			t.Fatalf("task without a report review: %#v", task)
		}
		var row automationmodel.AutomationTask
		if err := service.db.First(&row, "id = ?", task.ID.String()).Error; err != nil {
			t.Fatal(err)
		}
		taskRun := &automationmodel.AutomationTaskRun{TaskID: row.ID, TriggerSource: automationmodel.AutomationTriggerSourceManualRun, ScheduledFor: time.Now(),
			Status: automationmodel.AutomationTaskRunStatusSucceeded, RuntimeContext: map[string]any{"attempt": 1}}
		if err := service.db.Create(taskRun).Error; err != nil {
			t.Fatal(err)
		}
		stored, err := finalizeTaskReport(service.db, &row, taskRun, uuid.NewString(), succeeded, false, result, "boom", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		return stored
	}
	for name, tc := range map[string]struct {
		review    digitalhuman.TaskReportReview
		succeeded bool
		want      digitalhuman.TaskReportReviewState
	}{
		"default, done":      {"", true, digitalhuman.TaskReportClosed},
		"default, failed":    {"", false, digitalhuman.TaskReportNew},
		"none, done":         {digitalhuman.TaskReportReviewNone, true, digitalhuman.TaskReportClosed},
		"none, failed":       {digitalhuman.TaskReportReviewNone, false, digitalhuman.TaskReportNew},
		"required, done":     {digitalhuman.TaskReportReviewRequired, true, digitalhuman.TaskReportNew},
		"exceptions, done":   {digitalhuman.TaskReportReviewExceptions, true, digitalhuman.TaskReportClosed},
		"exceptions, failed": {digitalhuman.TaskReportReviewExceptions, false, digitalhuman.TaskReportNew},
	} {
		if got := report(tc.review, tc.succeeded, "Done.").ReviewState; got != string(tc.want) {
			t.Errorf("%s: review state = %s, want %s", name, got, tc.want)
		}
	}
	if !reportAwaitsReview(digitalhuman.TaskReportReviewExceptions, digitalhuman.TaskReportPartial) ||
		reportAwaitsReview(digitalhuman.TaskReportReviewNone, digitalhuman.TaskReportNotDone) {
		t.Fatal("a partial report waits by default, and nothing but a failure waits without review")
	}

	// A closed report can still be sent back for rework.
	closed := report("", true, "Done.")
	reworked, err := service.ReviewTaskReport(t.Context(), access, uuid.MustParse(closed.ID),
		digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework, Feedback: "Add Beijing."},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID})
	if err != nil || reworked.ReviewState != digitalhuman.TaskReportReworkRequested {
		t.Fatalf("rework of a closed report = %#v, %v", reworked, err)
	}
}

func TestANewerReportAndTimeCloseReportsWaitingForReview(t *testing.T) {
	service, _, access, _ := newTaskService(t)
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	task, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Analysis", Goal: "Analyze",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"}, ReportReview: digitalhuman.TaskReportReviewRequired},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatal(err)
	}
	var row automationmodel.AutomationTask
	if err := service.db.First(&row, "id = ?", task.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	finish := func(at time.Time) *taskReportRow {
		t.Helper()
		taskRun := &automationmodel.AutomationTaskRun{TaskID: row.ID, TriggerSource: automationmodel.AutomationTriggerSourceManualRun, ScheduledFor: at,
			Status: automationmodel.AutomationTaskRunStatusSucceeded, RuntimeContext: map[string]any{"attempt": 1}}
		if err := service.db.Create(taskRun).Error; err != nil {
			t.Fatal(err)
		}
		stored, err := finalizeTaskReport(service.db, &row, taskRun, uuid.NewString(), true, false, "Result.", "", at)
		if err != nil {
			t.Fatal(err)
		}
		return stored
	}
	state := func(id string) string {
		t.Helper()
		var stored taskReportRow
		if err := service.db.First(&stored, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return stored.ReviewState
	}
	now := time.Now().UTC()
	first := finish(now.Add(-4 * 24 * time.Hour))
	second := finish(now.Add(-time.Hour))
	if state(first.ID) != string(digitalhuman.TaskReportClosed) || state(second.ID) != string(digitalhuman.TaskReportNew) {
		t.Fatalf("after a newer report: first %s, second %s", state(first.ID), state(second.ID))
	}
	if err := closeExpiredReports(t.Context(), service.db, now.Add(digitalhuman.TaskReportReviewWindow)); err != nil {
		t.Fatal(err)
	}
	if state(second.ID) != string(digitalhuman.TaskReportClosed) {
		t.Fatalf("a report waiting past the review window = %s", state(second.ID))
	}

	// The review can be changed on the task; the default is not stored.
	exceptions := digitalhuman.TaskReportReviewExceptions
	updated, err := service.UpdateTask(t.Context(), access, task.ID, digitalhuman.UpdateTaskRequest{ExpectedRevision: task.Revision, ReportReview: &exceptions},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
	if err != nil || updated.ReportReview != digitalhuman.TaskReportReviewExceptions {
		t.Fatalf("UpdateTask(report_review) = %#v, %v", updated, err)
	}
	if err := service.db.First(&row, "id = ?", task.ID.String()).Error; err != nil || row.DeliveryConfig[reportReviewKey] != nil {
		t.Fatalf("stored delivery config = %#v, %v", row.DeliveryConfig, err)
	}
}
