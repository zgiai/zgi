package digitalhumantask

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

type recordingDeliverer struct {
	requests []digitalhuman.TaskResultDeliveryRequest
}

func (d *recordingDeliverer) DeliverTaskResult(_ context.Context, req digitalhuman.TaskResultDeliveryRequest) error {
	d.requests = append(d.requests, req)
	return nil
}

type recordingNotifier struct {
	notices []digitalhuman.TaskReportNotice
}

func (n *recordingNotifier) NotifyTaskReport(_ context.Context, notice digitalhuman.TaskReportNotice) error {
	n.notices = append(n.notices, notice)
	return nil
}

// reportTaskRun prepares a pending run of a shared task that reports back to a
// source conversation.
func reportTaskRun(t *testing.T, db *gorm.DB, maxAttempts int) (*automationmodel.AutomationTask, *automationmodel.AutomationTaskRun, uuid.UUID) {
	t.Helper()
	task, taskRun, _ := createPendingExternalRun(t, db, maxAttempts)
	conversationID := uuid.New()
	ownerKind := automationmodel.AutomationTaskOwnerKind(digitalhuman.TaskOwnerDigitalHuman)
	task.OwnerKind = &ownerKind
	task.SuccessCriteria = []string{"The summary is sent"}
	task.DeliveryConfig = map[string]any{"mode": string(digitalhuman.TaskDeliverySourceConversation), "conversation_id": conversationID.String()}
	if err := db.Save(task).Error; err != nil {
		t.Fatalf("save task: %v", err)
	}
	return task, taskRun, conversationID
}

func finishRun(t *testing.T, db *gorm.DB, state *automationaction.DigitalHumanRunState) (*recordingDeliverer, *recordingNotifier, *recordingDispatcher) {
	t.Helper()
	dispatcher, deliverer, notifier := &recordingDispatcher{}, &recordingDeliverer{}, &recordingNotifier{}
	runner := &reconcilerRunner{state: state}
	handler := NewReconcileHandler(db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })
	handler.SetResultDeliverer(deliverer)
	handler.SetReportNotifier(notifier)
	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	return deliverer, notifier, dispatcher
}

func reportOfRun(t *testing.T, db *gorm.DB, taskRunID string) *taskReportRow {
	t.Helper()
	var rows []taskReportRow
	if err := db.Where("task_run_id = ?", taskRunID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func TestFinishedRunWithoutReportGetsSystemReportDeliveredAndNotified(t *testing.T) {
	db := newReconcilerDB(t)
	task, taskRun, conversationID := reportTaskRun(t, db, 1)
	deliverer, notifier, _ := finishRun(t, db, &automationaction.DigitalHumanRunState{Status: "succeeded", Result: "Summary sent to #qa.\n\nDetails follow."})

	report := reportOfRun(t, db, taskRun.ID)
	if report == nil || !report.Final || report.Source != reportSourceSystem || report.Outcome != string(digitalhuman.TaskReportDone) ||
		report.Summary != "Summary sent to #qa." || len(report.Criteria) != 1 || report.Criteria[0].Status != digitalhuman.TaskCriterionUnknown {
		t.Fatalf("report = %#v", report)
	}
	if len(deliverer.requests) != 1 || deliverer.requests[0].ConversationID != conversationID || deliverer.requests[0].Report == nil ||
		deliverer.requests[0].Report.ID.String() != report.ID {
		t.Fatalf("deliveries = %#v", deliverer.requests)
	}
	if len(notifier.notices) != 1 || notifier.notices[0].RecipientKind != "account" || notifier.notices[0].RecipientID.String() != task.CreatedBy ||
		notifier.notices[0].ReportID.String() != report.ID {
		t.Fatalf("notices = %#v", notifier.notices)
	}
}

func TestSubmittedReportIsKeptWhenTheRunFinishes(t *testing.T) {
	db := newReconcilerDB(t)
	task, taskRun, _ := reportTaskRun(t, db, 1)
	service := NewService(db, &recordingDispatcher{}, nil)
	taskID := uuid.MustParse(task.ID)
	runAccess := digitalhuman.TaskAccessContext{
		OrganizationID: uuid.MustParse(task.OrganizationID), WorkspaceID: uuid.MustParse(task.WorkspaceID), DigitalHumanID: uuid.MustParse(*task.SubjectID),
		Principal: invocation.PrincipalRef{Type: "system", ID: "digital-human-task-scheduler"}, Surface: digitalhuman.SurfaceTask, CurrentTaskID: &taskID,
	}
	request := digitalhuman.SubmitTaskReportRequest{TaskRunID: uuid.MustParse(taskRun.ID), RunID: uuid.New(), Outcome: digitalhuman.TaskReportPartial,
		Summary: "Half of the checks ran.", Criteria: []digitalhuman.TaskReportCriterion{{Criterion: "The summary is sent", Status: digitalhuman.TaskCriterionNotMet, Note: "Channel offline"}}}
	if _, err := service.SubmitTaskReport(t.Context(), runAccess, request); err != nil {
		t.Fatalf("submit report: %v", err)
	}
	invalid := request
	invalid.Outcome = digitalhuman.TaskReportFailed
	if _, err := service.SubmitTaskReport(t.Context(), runAccess, invalid); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("failed outcome from the model: %v", err)
	}
	finishRun(t, db, &automationaction.DigitalHumanRunState{Status: "succeeded", Result: "Some answer"})
	report := reportOfRun(t, db, taskRun.ID)
	if report == nil || !report.Final || report.Source != reportSourceTool || report.Outcome != string(digitalhuman.TaskReportPartial) ||
		len(report.Criteria) != 1 || report.Criteria[0].Note != "Channel offline" {
		t.Fatalf("report = %#v", report)
	}
	if _, err := service.SubmitTaskReport(t.Context(), runAccess, request); err == nil {
		t.Fatal("a finished run accepted another report")
	}
}

func TestFailedRunsReportOnlyAfterTheLastAttempt(t *testing.T) {
	db := newReconcilerDB(t)
	_, retried, _ := reportTaskRun(t, db, 2)
	_, notifier, dispatcher := finishRun(t, db, &automationaction.DigitalHumanRunState{Status: "failed", ErrorMessage: "temporary"})
	if report := reportOfRun(t, db, retried.ID); report != nil || len(notifier.notices) != 0 || len(dispatcher.runIDs) != 1 {
		t.Fatalf("retried attempt report = %#v, notices = %#v", report, notifier.notices)
	}

	db = newReconcilerDB(t)
	_, last, _ := reportTaskRun(t, db, 1)
	deliverer, notifier, _ := finishRun(t, db, &automationaction.DigitalHumanRunState{Status: "failed", ErrorMessage: "The mailbox rejected the login."})
	report := reportOfRun(t, db, last.ID)
	if report == nil || !report.Final || report.Outcome != string(digitalhuman.TaskReportFailed) || report.Issues != "The mailbox rejected the login." {
		t.Fatalf("failure report = %#v", report)
	}
	if len(deliverer.requests) != 1 || deliverer.requests[0].Report.Outcome != digitalhuman.TaskReportFailed || len(notifier.notices) != 1 {
		t.Fatalf("failure was not delivered: %#v, %#v", deliverer.requests, notifier.notices)
	}
}

func TestReportsAreListedReviewedAndReworked(t *testing.T) {
	service, dispatcher, access, _ := newTaskService(t)
	runAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	task, err := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: "Weekly summary", Goal: "Summarize the week",
		Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleOnce, RunAt: &runAt, Timezone: "UTC"}},
		digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID, OperationID: uuid.New(), Source: "console"})
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
	if _, err := finalizeTaskReport(service.db, &row, taskRun, uuid.NewString(), true, false, "The week was quiet.", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	list, err := service.ListTaskReports(t.Context(), access, digitalhuman.TaskReportListQuery{})
	if err != nil || list.Total != 1 || !list.Items[0].CanReview || list.Items[0].TaskName != "Weekly summary" {
		t.Fatalf("reports = %#v, %v", list, err)
	}
	reportID := list.Items[0].ID
	listed, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
	if err != nil || len(listed.Items) != 1 || listed.Items[0].LatestReport == nil || listed.Items[0].LatestReport.ID != reportID {
		t.Fatalf("task latest report = %#v, %v", listed, err)
	}
	// A finished task stays in the open view until its report is reviewed.
	if err := service.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", row.ID).Update("work_status", automationmodel.AutomationWorkStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	openTasks := func(includeUnreviewed bool) int {
		t.Helper()
		result, err := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{Statuses: []digitalhuman.TaskWorkStatus{digitalhuman.TaskWorkStatusPending}, IncludeUnreviewed: includeUnreviewed})
		if err != nil {
			t.Fatal(err)
		}
		return len(result.Items)
	}
	if openTasks(false) != 0 || openTasks(true) != 1 {
		t.Fatalf("open tasks = %d without and %d with unreviewed reports", openTasks(false), openTasks(true))
	}

	mutation := digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: access.Principal.ID}
	accepted, err := service.ReviewTaskReport(t.Context(), access, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportAccept}, mutation)
	if err != nil || accepted.ReviewState != digitalhuman.TaskReportAccepted || accepted.ReviewedAt == nil {
		t.Fatalf("accept = %#v, %v", accepted, err)
	}
	if openTasks(true) != 0 {
		t.Fatal("an accepted report kept its finished task in the open view")
	}
	if _, err := service.ReviewTaskReport(t.Context(), access, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportAccept}, mutation); !errors.Is(err, digitalhuman.ErrTaskConflict) {
		t.Fatalf("accepting twice: %v", err)
	}
	if _, err := service.ReviewTaskReport(t.Context(), access, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework}, mutation); !errors.Is(err, digitalhuman.ErrInvalidRequest) {
		t.Fatalf("rework without feedback: %v", err)
	}
	reworked, err := service.ReviewTaskReport(t.Context(), access, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework, Feedback: "Include the incident on Tuesday."}, mutation)
	if err != nil || reworked.ReviewState != digitalhuman.TaskReportReworkRequested || reworked.ReviewFeedback != "Include the incident on Tuesday." {
		t.Fatalf("rework = %#v, %v", reworked, err)
	}
	if len(dispatcher.runIDs) != 1 {
		t.Fatalf("rework dispatches = %v", dispatcher.runIDs)
	}
	var rework automationmodel.AutomationTaskRun
	if err := service.db.First(&rework, "id = ?", dispatcher.runIDs[0]).Error; err != nil || rework.RuntimeContext["rework_feedback"] != "Include the incident on Tuesday." {
		t.Fatalf("rework run = %#v, %v", rework, err)
	}

	stranger := access
	stranger.Principal.ID = uuid.NewString()
	stranger.Viewer = digitalhuman.TaskViewer{Mode: digitalhuman.TaskViewTaskCenter}
	other, err := service.GetTaskReport(t.Context(), stranger, reportID)
	if err != nil || other.CanReview {
		t.Fatalf("another member's view = %#v, %v", other, err)
	}
	if _, err := service.ReviewTaskReport(t.Context(), stranger, reportID, digitalhuman.ReviewTaskReportRequest{Action: digitalhuman.TaskReportRework, Feedback: "x"}, mutation); !errors.Is(err, digitalhuman.ErrTaskPolicyRejected) {
		t.Fatalf("another member reviewing: %v", err)
	}
}

func TestReportKeepsChosenFilesOnceAndBounded(t *testing.T) {
	ids := make([]uuid.UUID, 0, digitalhuman.MaxTaskReportFiles+2)
	first := uuid.New()
	ids = append(ids, first, first, uuid.Nil)
	for len(ids) < digitalhuman.MaxTaskReportFiles+3 {
		ids = append(ids, uuid.New())
	}
	row := &taskReportRow{ID: uuid.NewString(), DigitalHumanID: uuid.NewString(), TaskID: uuid.NewString(), TaskRunID: uuid.NewString(), Files: reportFileRefs(ids)}
	report := publicTaskReport(row, nil, digitalhuman.TaskAccessContext{})
	if len(report.FileIDs) != digitalhuman.MaxTaskReportFiles || report.FileIDs[0] != first || report.Files == nil {
		t.Fatalf("file ids = %v, files = %#v", report.FileIDs, report.Files)
	}
}
