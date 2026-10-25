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

// The creator of a task center task reads its reports as their own and can
// accept and then rework one there; another member can do neither.
func TestTaskCenterCreatorReworksFromTheirOwnReports(t *testing.T) {
	service, _, shared, _ := newTaskService(t)
	shared.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewTaskCenter}
	creator := uuid.MustParse(shared.Principal.ID)
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	task, err := service.CreateTask(t.Context(), shared, digitalhuman.CreateTaskRequest{Name: "Weekly team report", Goal: "Summarize the team's week",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"}, ReportReview: digitalhuman.TaskReportReviewRequired},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: creator.String(), OperationID: uuid.New(), Source: "console"})
	if err != nil {
		t.Fatal(err)
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
	if _, err := finalizeTaskReport(service.db, &row, taskRun, uuid.NewString(), true, false, "No progress data was available.", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	member := func(account uuid.UUID) digitalhuman.TaskAccessContext {
		access := shared
		access.Principal.ID = account.String()
		access.Owner = digitalhuman.UserTaskOwner(usersubject.AccountSubject(account))
		access.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewMember}
		return access
	}
	own := member(creator)
	list, err := service.ListTaskReports(t.Context(), own, digitalhuman.TaskReportListQuery{})
	if err != nil || list.Total != 1 || !list.Items[0].CanReview || !list.Items[0].RequestedByReader {
		t.Fatalf("creator's reports = %#v, %v", list, err)
	}
	reportID := list.Items[0].ID
	mutation := digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: creator.String()}
	if _, err := service.ReviewTaskReport(t.Context(), own, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportAccept}, mutation); err != nil {
		t.Fatalf("accept: %v", err)
	}
	reworked, err := service.ReviewTaskReport(t.Context(), own, reportID,
		digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework, Feedback: "Use the sales team's weekly sheet."}, mutation)
	if err != nil || reworked.ReviewState != digitalhuman.TaskReportReworkRequested {
		t.Fatalf("rework from the creator's own reports = %#v, %v", reworked, err)
	}

	other := member(uuid.New())
	if _, err := service.ReviewTaskReport(t.Context(), other, reportID,
		digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework, Feedback: "x"}, mutation); err == nil ||
		(!errors.Is(err, digitalhuman.ErrTaskPolicyRejected) && !errors.Is(err, digitalhuman.ErrTaskNotFound)) {
		t.Fatalf("another member's rework err = %v", err)
	}
}
