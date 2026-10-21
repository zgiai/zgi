package digitalhuman

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TaskReportOutcome is how far a task run got, as the report states it.
type TaskReportOutcome string

const (
	TaskReportDone    TaskReportOutcome = "done"
	TaskReportPartial TaskReportOutcome = "partial"
	TaskReportNotDone TaskReportOutcome = "not_done"
	// TaskReportFailed is written by the scheduler when a run fails after its
	// last attempt. The DigitalHuman never submits it.
	TaskReportFailed TaskReportOutcome = "failed"
)

// Valid reports whether the DigitalHuman may submit this outcome.
func (o TaskReportOutcome) Valid() bool {
	return o == TaskReportDone || o == TaskReportPartial || o == TaskReportNotDone
}

// TaskReportCriterionStatus is the DigitalHuman's own assessment of one
// success criterion. A person confirms it by accepting the report.
type TaskReportCriterionStatus string

const (
	TaskCriterionMet     TaskReportCriterionStatus = "met"
	TaskCriterionNotMet  TaskReportCriterionStatus = "not_met"
	TaskCriterionUnknown TaskReportCriterionStatus = "unknown"
)

// TaskReportCriterion assesses one success criterion of the task.
type TaskReportCriterion struct {
	Criterion string                    `json:"criterion"`
	Status    TaskReportCriterionStatus `json:"status"`
	Note      string                    `json:"note,omitempty"`
}

// TaskReportReviewState records what the requester decided about a report.
type TaskReportReviewState string

const (
	TaskReportNew             TaskReportReviewState = "new"
	TaskReportAccepted        TaskReportReviewState = "accepted"
	TaskReportReworkRequested TaskReportReviewState = "rework_requested"
)

// TaskReport is what one task run hands back (ADR 0008). Reports are visible
// to whoever can see their task.
type TaskReport struct {
	ID                uuid.UUID             `json:"id"`
	DigitalHumanID    uuid.UUID             `json:"digital_human_id"`
	TaskID            uuid.UUID             `json:"task_id"`
	TaskName          string                `json:"task_name"`
	TaskRunID         uuid.UUID             `json:"task_run_id"`
	DigitalHumanRunID *uuid.UUID            `json:"digital_human_run_id,omitempty"`
	Attempt           int                   `json:"attempt"`
	Outcome           TaskReportOutcome     `json:"outcome"`
	Summary           string                `json:"summary"`
	Body              string                `json:"body,omitempty"`
	Criteria          []TaskReportCriterion `json:"criteria"`
	Issues            string                `json:"issues,omitempty"`
	Source            string                `json:"source"`
	Final             bool                  `json:"final"`
	ReviewState       TaskReportReviewState `json:"review_state"`
	ReviewFeedback    string                `json:"review_feedback,omitempty"`
	ReviewedAt        *time.Time            `json:"reviewed_at,omitempty"`
	// CanReview tells the reader whether it may accept the report or ask for
	// rework: only the task's requester or a task center manager may.
	CanReview bool `json:"can_review"`
	// FileIDs are the run files the DigitalHuman chose to attach. When empty,
	// the report carries every file its run produced.
	FileIDs []uuid.UUID `json:"-"`
	// Files are the run files the report carries, filled in when it is read.
	Files     []TaskReportFile `json:"files"`
	CreatedAt time.Time        `json:"created_at"`
}

// TaskReportFile is a file a report carries, downloaded through the report.
type TaskReportFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// MaxTaskReportFiles bounds the files one report carries.
const MaxTaskReportFiles = 8

// TaskReportSummary is the latest report shown with a task.
type TaskReportSummary struct {
	ID          uuid.UUID             `json:"id"`
	Outcome     TaskReportOutcome     `json:"outcome"`
	Summary     string                `json:"summary"`
	ReviewState TaskReportReviewState `json:"review_state"`
	CreatedAt   time.Time             `json:"created_at"`
}

// SubmitTaskReportRequest is the report the DigitalHuman submits at the end of
// a scheduled task run.
type SubmitTaskReportRequest struct {
	TaskRunID uuid.UUID
	RunID     uuid.UUID
	Outcome   TaskReportOutcome
	Summary   string
	Body      string
	Criteria  []TaskReportCriterion
	Issues    string
	// FileIDs attach chosen run files; empty attaches every run file.
	FileIDs []uuid.UUID
}

// TaskReportListQuery filters a report feed. TaskID narrows it to one task.
type TaskReportListQuery struct {
	TaskID       *uuid.UUID
	ReviewStates []TaskReportReviewState
	Outcomes     []TaskReportOutcome
	Page         int
	PageSize     int
}

// TaskReportList is one page of reports.
type TaskReportList struct {
	Items    []*TaskReport `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// TaskReportReviewAction is accept or rework.
type TaskReportReviewAction string

const (
	TaskReportAccept TaskReportReviewAction = "accept"
	TaskReportRework TaskReportReviewAction = "rework"
)

// ReviewTaskReportRequest accepts a report or asks for rework with feedback.
// Rework runs the task again with the feedback.
type ReviewTaskReportRequest struct {
	Action   TaskReportReviewAction `json:"action"`
	Feedback string                 `json:"feedback,omitempty"`
}

// TaskReportService stores and serves task reports.
type TaskReportService interface {
	SubmitTaskReport(ctx context.Context, access TaskAccessContext, req SubmitTaskReportRequest) (*TaskReport, error)
	ListTaskReports(ctx context.Context, access TaskAccessContext, query TaskReportListQuery) (*TaskReportList, error)
	GetTaskReport(ctx context.Context, access TaskAccessContext, reportID uuid.UUID) (*TaskReport, error)
	ReviewTaskReport(ctx context.Context, access TaskAccessContext, reportID uuid.UUID, req ReviewTaskReportRequest, mutation TaskMutationContext) (*TaskReport, error)
}
