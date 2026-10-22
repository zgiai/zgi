package digitalhumantask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxReportSummaryRunes   = 500
	maxReportBodyRunes      = 20000
	maxReportIssuesRunes    = 4000
	maxReportCriteria       = 20
	maxReportCriterionRunes = 500
	maxReworkFeedbackRunes  = 2000
	reportSourceTool        = "report_tool"
	reportSourceSystem      = "system"
)

// taskReportRow stores one task report (ADR 0008).
type taskReportRow struct {
	ID                string                             `gorm:"column:id;type:uuid;primaryKey"`
	OrganizationID    string                             `gorm:"column:organization_id;type:uuid"`
	WorkspaceID       string                             `gorm:"column:workspace_id;type:uuid"`
	DigitalHumanID    string                             `gorm:"column:digital_human_id;type:uuid"`
	TaskID            string                             `gorm:"column:task_id;type:uuid"`
	TaskRunID         string                             `gorm:"column:task_run_id;type:uuid"`
	DigitalHumanRunID *string                            `gorm:"column:digital_human_run_id;type:uuid"`
	Attempt           int                                `gorm:"column:attempt"`
	Outcome           string                             `gorm:"column:outcome"`
	Summary           string                             `gorm:"column:summary"`
	Body              string                             `gorm:"column:body"`
	Criteria          []digitalhuman.TaskReportCriterion `gorm:"column:criteria;type:jsonb;serializer:json"`
	Issues            string                             `gorm:"column:issues"`
	Files             []map[string]any                   `gorm:"column:files;type:jsonb;serializer:json"`
	Source            string                             `gorm:"column:source"`
	Final             bool                               `gorm:"column:final"`
	ReviewState       string                             `gorm:"column:review_state"`
	ReviewFeedback    *string                            `gorm:"column:review_feedback"`
	ReviewedByType    *string                            `gorm:"column:reviewed_by_type"`
	ReviewedByRef     *string                            `gorm:"column:reviewed_by_ref"`
	ReviewedAt        *time.Time                         `gorm:"column:reviewed_at"`
	ReworkTaskRunID   *string                            `gorm:"column:rework_task_run_id;type:uuid"`
	CreatedAt         time.Time                          `gorm:"column:created_at"`
	UpdatedAt         time.Time                          `gorm:"column:updated_at"`
}

func (taskReportRow) TableName() string { return "digital_human_task_reports" }

var _ digitalhuman.TaskReportService = (*Service)(nil)

// SubmitTaskReport stores the report the DigitalHuman submits during a
// scheduled run of the current task. A run may resubmit until the scheduler
// finalizes its report.
func (s *Service) SubmitTaskReport(ctx context.Context, access digitalhuman.TaskAccessContext, req digitalhuman.SubmitTaskReportRequest) (*digitalhuman.TaskReport, error) {
	if access.CurrentTaskID == nil || req.TaskRunID == uuid.Nil || req.RunID == uuid.Nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	if err := validateReportContent(req); err != nil {
		return nil, err
	}
	var stored *taskReportRow
	var task *automationmodel.AutomationTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		loaded, err := s.loadTask(ctx, tx, access, *access.CurrentTaskID, false)
		if err != nil {
			return err
		}
		task = loaded
		var taskRun automationmodel.AutomationTaskRun
		if err := tx.Where("id = ? AND task_id = ?", req.TaskRunID, task.ID).First(&taskRun).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return digitalhuman.ErrTaskNotFound
			}
			return err
		}
		if taskRun.FinishedAt != nil {
			return digitalhuman.ErrTaskConflict
		}
		var existing taskReportRow
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_run_id = ?", req.TaskRunID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := s.now()
		runID := req.RunID.String()
		row := &taskReportRow{
			ID: uuid.NewString(), OrganizationID: task.OrganizationID, WorkspaceID: task.WorkspaceID, DigitalHumanID: pointerValue(task.SubjectID),
			TaskID: task.ID, TaskRunID: req.TaskRunID.String(), DigitalHumanRunID: &runID, Attempt: taskRunAttempt(&taskRun),
			Outcome: string(req.Outcome), Summary: strings.TrimSpace(req.Summary), Body: strings.TrimSpace(req.Body),
			Criteria: normalizeCriteria(req.Criteria), Issues: strings.TrimSpace(req.Issues), Files: reportFileRefs(req.FileIDs),
			Source: reportSourceTool, ReviewState: string(digitalhuman.TaskReportNew), CreatedAt: now, UpdatedAt: now,
		}
		if err == nil {
			if existing.Final {
				return digitalhuman.ErrTaskConflict
			}
			row.ID, row.CreatedAt = existing.ID, existing.CreatedAt
			if err := tx.Model(&taskReportRow{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"outcome": row.Outcome, "summary": row.Summary, "body": row.Body, "criteria": row.Criteria, "issues": row.Issues,
				"files": row.Files, "digital_human_run_id": runID, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(row).Error; err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	return publicTaskReport(stored, task, access), nil
}

// ListTaskReports lists final reports of the tasks the reader can see, newest
// first. A report of a single task also includes reports of retried attempts.
func (s *Service) ListTaskReports(ctx context.Context, access digitalhuman.TaskAccessContext, query digitalhuman.TaskReportListQuery) (*digitalhuman.TaskReportList, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(query.Page, query.PageSize)
	visibleTasks, err := applyRequesterFilter(s.taskQuery(ctx, access), digitalhuman.TaskListQuery{RequesterKinds: query.RequesterKinds, RequesterID: query.RequesterID})
	if err != nil {
		return nil, err
	}
	base := s.db.WithContext(ctx).Model(&taskReportRow{}).
		Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ?", access.OrganizationID, access.WorkspaceID, access.DigitalHumanID).
		Where("task_id IN (?)", visibleTasks.Select("id"))
	if query.TaskID != nil {
		base = base.Where("task_id = ?", *query.TaskID)
	} else {
		base = base.Where("final")
	}
	if len(query.ReviewStates) > 0 {
		states := make([]string, 0, len(query.ReviewStates))
		for _, state := range query.ReviewStates {
			states = append(states, string(state))
		}
		base = base.Where("review_state IN ?", states)
	}
	if len(query.Outcomes) > 0 {
		outcomes := make([]string, 0, len(query.Outcomes))
		for _, outcome := range query.Outcomes {
			outcomes = append(outcomes, string(outcome))
		}
		base = base.Where("outcome IN ?", outcomes)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count digital human task reports: %w", err)
	}
	var rows []*taskReportRow
	if err := base.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list digital human task reports: %w", err)
	}
	tasks, err := s.tasksByID(ctx, rows)
	if err != nil {
		return nil, err
	}
	items := make([]*digitalhuman.TaskReport, 0, len(rows))
	for _, row := range rows {
		items = append(items, publicTaskReport(row, tasks[row.TaskID], access))
	}
	if err := s.attachReportRequesters(ctx, access, rows, tasks, items); err != nil {
		return nil, err
	}
	return &digitalhuman.TaskReportList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// GetTaskReport loads one report of a task the reader can see.
func (s *Service) GetTaskReport(ctx context.Context, access digitalhuman.TaskAccessContext, reportID uuid.UUID) (*digitalhuman.TaskReport, error) {
	row, task, err := s.loadReport(ctx, s.db, access, reportID, false)
	if err != nil {
		return nil, normalizeError(err)
	}
	report := publicTaskReport(row, task, access)
	if err := s.attachReportRequesters(ctx, access, []*taskReportRow{row}, map[string]*automationmodel.AutomationTask{row.TaskID: task}, []*digitalhuman.TaskReport{report}); err != nil {
		return nil, err
	}
	return report, nil
}

// attachReportRequesters sets who asked for each report's task, for readers
// who see requesters.
func (s *Service) attachReportRequesters(ctx context.Context, access digitalhuman.TaskAccessContext, rows []*taskReportRow, tasks map[string]*automationmodel.AutomationTask, items []*digitalhuman.TaskReport) error {
	if !showsRequesters(access) || len(rows) == 0 {
		return nil
	}
	keys := make([]requesterKey, 0, len(rows))
	for _, row := range rows {
		if task := tasks[row.TaskID]; task != nil {
			keys = append(keys, taskRequesterKey(task))
		}
	}
	requesters, err := s.describeRequesters(ctx, access, keys)
	if err != nil {
		return err
	}
	for index, row := range rows {
		task := tasks[row.TaskID]
		if task == nil {
			continue
		}
		if requester := requesters[taskRequesterKey(task)]; requester != nil {
			copied := *requester
			items[index].Requester = &copied
		}
	}
	return nil
}

// ReviewTaskReport accepts a final report or asks for rework. Rework records
// the feedback and runs the task again with it.
func (s *Service) ReviewTaskReport(ctx context.Context, access digitalhuman.TaskAccessContext, reportID uuid.UUID, req digitalhuman.ReviewTaskReportRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.TaskReport, error) {
	if access.CurrentTaskID != nil {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	feedback := strings.TrimSpace(req.Feedback)
	switch req.Action {
	case digitalhuman.TaskReportAccept:
	case digitalhuman.TaskReportRework:
		if feedback == "" || utf8.RuneCountInString(feedback) > maxReworkFeedbackRunes {
			return nil, digitalhuman.ErrInvalidRequest
		}
	default:
		return nil, digitalhuman.ErrInvalidRequest
	}
	if mutation.OperationID == uuid.Nil {
		mutation.OperationID = uuid.New()
	}
	var reviewed *taskReportRow
	var reviewedTask *automationmodel.AutomationTask
	var reworkRunID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, task, err := s.loadReport(ctx, tx, access, reportID, true)
		if err != nil {
			return err
		}
		if !canReviewReport(task, access) {
			return digitalhuman.ErrTaskPolicyRejected
		}
		if !row.Final || !reviewTransitionAllowed(digitalhuman.TaskReportReviewState(row.ReviewState), req.Action) {
			return digitalhuman.ErrTaskConflict
		}
		now := s.now()
		actorType, actorRef := strings.TrimSpace(mutation.ActorType), strings.TrimSpace(mutation.ActorRef)
		if actorType == "" {
			actorType, actorRef = "account", accessActorID(access).String()
		}
		updates := map[string]any{"reviewed_by_type": actorType, "reviewed_by_ref": actorRef, "reviewed_at": now, "updated_at": now}
		if req.Action == digitalhuman.TaskReportAccept {
			row.ReviewState = string(digitalhuman.TaskReportAccepted)
		} else {
			row.ReviewState, row.ReviewFeedback = string(digitalhuman.TaskReportReworkRequested), &feedback
			lockedTask, err := s.lockReviewedTask(ctx, tx, access, task)
			if err != nil {
				return err
			}
			if err := applyTransition(lockedTask, digitalhuman.TaskActionRunNow, now); err != nil {
				return err
			}
			run := &automationmodel.AutomationTaskRun{TaskID: lockedTask.ID, TriggerSource: automationmodel.AutomationTriggerSourceManualRun, ScheduledFor: now,
				Status: automationmodel.AutomationTaskRunStatusQueued, RuntimeContext: map[string]any{"attempt": 1, "trigger_mode": "rework", "rework_report_id": row.ID, "rework_feedback": feedback}}
			if err := tx.Create(run).Error; err != nil {
				return err
			}
			reworkRunID = run.ID
			updates["review_feedback"], updates["rework_task_run_id"] = feedback, run.ID
			lockedTask.Revision++
			lockedTask.UpdatedBy = accessActorID(access).String()
			values, err := taskUpdateValues(lockedTask, now)
			if err != nil {
				return err
			}
			if err := tx.Model(&automationmodel.AutomationTask{}).Where("id = ?", lockedTask.ID).Updates(values).Error; err != nil {
				return err
			}
			task = lockedTask
		}
		updates["review_state"] = row.ReviewState
		if err := tx.Model(&taskReportRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
		action, summary := "report_accepted", "Task report accepted"
		if req.Action == digitalhuman.TaskReportRework {
			action, summary = "rework_requested", "Rework requested on a task report"
		}
		if err := tx.Create(newActivity(task.ID, mutation, action, summary, map[string]any{"report_id": row.ID})).Error; err != nil {
			return err
		}
		row.ReviewedAt = &now
		reviewed, reviewedTask = row, task
		return nil
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	if reworkRunID != "" {
		if err := s.dispatcher.DispatchTaskRun(ctx, reworkRunID); err != nil {
			return nil, err
		}
	}
	return publicTaskReport(reviewed, reviewedTask, access), nil
}

func (s *Service) loadReport(ctx context.Context, db *gorm.DB, access digitalhuman.TaskAccessContext, reportID uuid.UUID, lock bool) (*taskReportRow, *automationmodel.AutomationTask, error) {
	if err := validateBoundary(access); err != nil || reportID == uuid.Nil {
		return nil, nil, digitalhuman.ErrInvalidRequest
	}
	query := db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?", reportID, access.OrganizationID, access.WorkspaceID, access.DigitalHumanID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row taskReportRow
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, digitalhuman.ErrTaskNotFound
		}
		return nil, nil, err
	}
	task, err := s.loadTask(ctx, db, access, uuid.MustParse(row.TaskID), false)
	if err != nil {
		return nil, nil, err
	}
	return &row, task, nil
}

func (s *Service) tasksByID(ctx context.Context, rows []*taskReportRow) (map[string]*automationmodel.AutomationTask, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TaskID)
	}
	result := make(map[string]*automationmodel.AutomationTask, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var tasks []*automationmodel.AutomationTask
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("load tasks of reports: %w", err)
	}
	for _, task := range tasks {
		result[task.ID] = task
	}
	return result, nil
}

// latestReports returns the latest final report of each task.
func (s *Service) latestReports(ctx context.Context, taskIDs []string) (map[string]*digitalhuman.TaskReportSummary, error) {
	result := make(map[string]*digitalhuman.TaskReportSummary, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	var rows []*taskReportRow
	if err := s.db.WithContext(ctx).Where(`task_id IN ? AND final AND created_at = (SELECT MAX(latest.created_at)
		FROM digital_human_task_reports AS latest WHERE latest.task_id = digital_human_task_reports.task_id AND latest.final)`, taskIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load latest task reports: %w", err)
	}
	for _, row := range rows {
		result[row.TaskID] = &digitalhuman.TaskReportSummary{ID: uuid.MustParse(row.ID), Outcome: digitalhuman.TaskReportOutcome(row.Outcome),
			Summary: row.Summary, ReviewState: digitalhuman.TaskReportReviewState(row.ReviewState), CreatedAt: row.CreatedAt}
	}
	return result, nil
}

// finalizeTaskReport concludes the report of a finished task run inside the
// scheduler's transaction. A run that succeeded keeps the report it
// submitted, or gets one written from its answer; a run that failed for good
// gets a failure report. Retried attempts keep their reports unfinalized.
func finalizeTaskReport(tx *gorm.DB, task *automationmodel.AutomationTask, taskRun *automationmodel.AutomationTaskRun, digitalHumanRunID string, succeeded, willRetry bool, result, errorMessage string, now time.Time) (*taskReportRow, error) {
	if willRetry {
		return nil, nil
	}
	var row taskReportRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_run_id = ?", taskRun.ID).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	exists := err == nil
	if exists && row.Final {
		return nil, nil
	}
	if !exists {
		row = taskReportRow{ID: uuid.NewString(), OrganizationID: task.OrganizationID, WorkspaceID: task.WorkspaceID, DigitalHumanID: pointerValue(task.SubjectID),
			TaskID: task.ID, TaskRunID: taskRun.ID, Attempt: taskRunAttempt(taskRun), Criteria: []digitalhuman.TaskReportCriterion{}, Files: []map[string]any{},
			Source: reportSourceSystem, ReviewState: string(digitalhuman.TaskReportNew), CreatedAt: now}
		if digitalHumanRunID != "" {
			runID := digitalHumanRunID
			row.DigitalHumanRunID = &runID
		}
		if succeeded {
			row.Outcome, row.Summary, row.Body = string(digitalhuman.TaskReportDone), reportSummaryFrom(result), strings.TrimSpace(result)
			row.Criteria = unknownCriteria(task.SuccessCriteria)
		}
	}
	if !succeeded {
		row.Outcome = string(digitalhuman.TaskReportFailed)
		message := strings.TrimSpace(errorMessage)
		if message == "" {
			message = "The task run did not finish."
		}
		if !exists {
			row.Summary = reportSummaryFrom(message)
			row.Criteria = unknownCriteria(task.SuccessCriteria)
		}
		row.Issues = strings.TrimSpace(strings.TrimSpace(row.Issues) + "\n" + message)
	}
	row.Final, row.UpdatedAt = true, now
	if err := tx.Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func publicTaskReport(row *taskReportRow, task *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) *digitalhuman.TaskReport {
	report := &digitalhuman.TaskReport{
		ID: uuid.MustParse(row.ID), DigitalHumanID: uuid.MustParse(row.DigitalHumanID), TaskID: uuid.MustParse(row.TaskID), TaskRunID: uuid.MustParse(row.TaskRunID),
		Attempt: row.Attempt, Outcome: digitalhuman.TaskReportOutcome(row.Outcome), Summary: row.Summary, Body: row.Body, Criteria: row.Criteria,
		Issues: row.Issues, Source: row.Source, Final: row.Final, ReviewState: digitalhuman.TaskReportReviewState(row.ReviewState), ReviewedAt: row.ReviewedAt,
		CreatedAt: row.CreatedAt,
	}
	if report.Criteria == nil {
		report.Criteria = []digitalhuman.TaskReportCriterion{}
	}
	report.Files = []digitalhuman.TaskReportFile{}
	for _, ref := range row.Files {
		if id, err := uuid.Parse(fmt.Sprint(ref["id"])); err == nil {
			report.FileIDs = append(report.FileIDs, id)
		}
	}
	if row.DigitalHumanRunID != nil {
		if id, err := uuid.Parse(*row.DigitalHumanRunID); err == nil {
			report.DigitalHumanRunID = &id
		}
	}
	if row.ReviewFeedback != nil {
		report.ReviewFeedback = *row.ReviewFeedback
	}
	if task != nil {
		report.TaskName = task.Name
		report.ClientReference = pointerValue(task.ClientReference)
		report.CanReview = row.Final && access.CurrentTaskID == nil && canReviewReport(task, access)
		report.RequestedByReader = taskHeldBy(task, access)
		report.Origin = taskReportOrigin(task, report.RequestedByReader)
	}
	return report
}

// canReviewReport mirrors who may change the task: its requester or a task
// center manager.
func canReviewReport(task *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) bool {
	return taskCenterManager(access) || taskHeldBy(task, access)
}

// taskHeldBy reports whether the reader holds the task: its requester, or the
// member who created a task in the task center, reading it as their own.
func taskHeldBy(task *automationmodel.AutomationTask, access digitalhuman.TaskAccessContext) bool {
	if task == nil {
		return false
	}
	if taskRequestedBy(task, access) {
		return true
	}
	return pointerValue(task.OwnerKind) == string(automationmodel.AutomationTaskOwnerDigitalHuman) &&
		task.OwnerSubjectID == nil && task.OwnerConversationID == nil && task.CreatedBy == accessActorID(access).String()
}

func reviewTransitionAllowed(current digitalhuman.TaskReportReviewState, action digitalhuman.TaskReportReviewAction) bool {
	switch action {
	case digitalhuman.TaskReportAccept:
		return current == digitalhuman.TaskReportNew
	case digitalhuman.TaskReportRework:
		return current == digitalhuman.TaskReportNew || current == digitalhuman.TaskReportAccepted
	}
	return false
}

func validateReportContent(req digitalhuman.SubmitTaskReportRequest) error {
	summary := strings.TrimSpace(req.Summary)
	if !req.Outcome.Valid() || summary == "" || utf8.RuneCountInString(summary) > maxReportSummaryRunes ||
		utf8.RuneCountInString(req.Body) > maxReportBodyRunes || utf8.RuneCountInString(req.Issues) > maxReportIssuesRunes ||
		len(req.Criteria) > maxReportCriteria {
		return digitalhuman.ErrInvalidRequest
	}
	for _, criterion := range req.Criteria {
		text := strings.TrimSpace(criterion.Criterion)
		if text == "" || utf8.RuneCountInString(text) > maxReportCriterionRunes || utf8.RuneCountInString(criterion.Note) > maxReportCriterionRunes {
			return digitalhuman.ErrInvalidRequest
		}
		switch criterion.Status {
		case digitalhuman.TaskCriterionMet, digitalhuman.TaskCriterionNotMet, digitalhuman.TaskCriterionUnknown:
		default:
			return digitalhuman.ErrInvalidRequest
		}
	}
	return nil
}

func normalizeCriteria(criteria []digitalhuman.TaskReportCriterion) []digitalhuman.TaskReportCriterion {
	result := make([]digitalhuman.TaskReportCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		result = append(result, digitalhuman.TaskReportCriterion{Criterion: strings.TrimSpace(criterion.Criterion), Status: criterion.Status, Note: strings.TrimSpace(criterion.Note)})
	}
	return result
}

func unknownCriteria(criteria []string) []digitalhuman.TaskReportCriterion {
	result := make([]digitalhuman.TaskReportCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		if text := strings.TrimSpace(criterion); text != "" {
			result = append(result, digitalhuman.TaskReportCriterion{Criterion: text, Status: digitalhuman.TaskCriterionUnknown})
		}
	}
	return result
}

// reportSummaryFrom takes the first paragraph of a free-text answer as the
// report's one-line outcome.
func reportSummaryFrom(text string) string {
	text = strings.TrimSpace(text)
	if index := strings.Index(text, "\n\n"); index > 0 {
		text = text[:index]
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 200 {
		return string(runes[:200]) + "…"
	}
	return text
}

func taskRunAttempt(run *automationmodel.AutomationTaskRun) int {
	if attempt, ok := numericValue(run.RuntimeContext["attempt"]); ok && attempt > 0 {
		return attempt
	}
	return 1
}

func reportDelivery(row *taskReportRow) *digitalhuman.TaskReportDelivery {
	return &digitalhuman.TaskReportDelivery{ID: uuid.MustParse(row.ID), Outcome: digitalhuman.TaskReportOutcome(row.Outcome), Summary: row.Summary, Body: row.Body, Issues: row.Issues}
}

// reportNotice addresses a final report to its task's requester. A shared
// task's requester is the account that created it. Channel tasks have no
// personal requester; their report goes back to the channel.
func reportNotice(row *taskReportRow, task *automationmodel.AutomationTask) (digitalhuman.TaskReportNotice, bool) {
	notice := digitalhuman.TaskReportNotice{
		OrganizationID: uuid.MustParse(row.OrganizationID), WorkspaceID: uuid.MustParse(row.WorkspaceID), DigitalHumanID: uuid.MustParse(row.DigitalHumanID),
		TaskID: uuid.MustParse(row.TaskID), ReportID: uuid.MustParse(row.ID), Outcome: digitalhuman.TaskReportOutcome(row.Outcome), Summary: row.Summary,
	}
	if task == nil {
		return notice, false
	}
	notice.TaskName = task.Name
	notice.ClientReference = pointerValue(task.ClientReference)
	notice.ChannelIDs = taskNotifyChannels(task)
	notice.Shared = teamReportableTask(task) && storedTaskVisibility(task) == digitalhuman.TaskVisibilityWorkspace
	if row.DigitalHumanRunID != nil {
		if id, err := uuid.Parse(*row.DigitalHumanRunID); err == nil {
			notice.RunID = &id
		}
	}
	for _, ref := range row.Files {
		if id, err := uuid.Parse(fmt.Sprint(ref["id"])); err == nil {
			notice.FileIDs = append(notice.FileIDs, id)
		}
	}
	switch digitalhuman.TaskOwnerKind(pointerValue(task.OwnerKind)) {
	case digitalhuman.TaskOwnerUserSubject:
		id, err := uuid.Parse(pointerValue(task.OwnerSubjectID))
		if err != nil || id == uuid.Nil {
			return notice, false
		}
		notice.RecipientKind, notice.RecipientID = pointerValue(task.OwnerSubjectKind), id
	case digitalhuman.TaskOwnerDigitalHuman:
		id, err := uuid.Parse(strings.TrimSpace(task.CreatedBy))
		if err != nil || id == uuid.Nil {
			return notice, false
		}
		notice.RecipientKind, notice.RecipientID = "account", id
	default:
		return notice, false
	}
	return notice, true
}

// reportFileRefs stores the run files a report chose, at most
// MaxTaskReportFiles of them, once each.
func reportFileRefs(ids []uuid.UUID) []map[string]any {
	refs := []map[string]any{}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] || len(refs) == digitalhuman.MaxTaskReportFiles {
			continue
		}
		seen[id] = true
		refs = append(refs, map[string]any{"id": id.String()})
	}
	return refs
}

// taskReportOrigin says where a task was asked for, for following its report
// up: the console conversation of a holder who asked there, or the channel
// chat it came from.
func taskReportOrigin(task *automationmodel.AutomationTask, holder bool) *digitalhuman.TaskReportOrigin {
	if task == nil {
		return nil
	}
	snapshotValue := func(key string) string {
		value, _ := task.SourceSnapshot[key].(string)
		return strings.TrimSpace(value)
	}
	switch pointerValue(task.OriginSurface) {
	case digitalhuman.SurfaceChannel:
		origin := &digitalhuman.TaskReportOrigin{Kind: "channel", ConversationType: snapshotValue("conversation_type")}
		if reply, _ := task.DeliveryConfig["reply_to"].(map[string]any); reply != nil {
			provider, _ := reply["provider"].(string)
			origin.Provider = strings.TrimSpace(provider)
		}
		return origin
	case digitalhuman.SurfaceConsole:
		if conversation := snapshotValue("conversation_id"); holder && validConversationID(conversation) {
			return &digitalhuman.TaskReportOrigin{Kind: "conversation", ConversationID: conversation}
		}
	}
	return nil
}

// lockReviewedTask locks the task of a report being reworked. The owner scope
// of the reader's entry does not cover a task center task its creator reads as
// their own, so a holder's task is locked by its identity and boundary, and
// held again under the lock.
func (s *Service) lockReviewedTask(ctx context.Context, tx *gorm.DB, access digitalhuman.TaskAccessContext, task *automationmodel.AutomationTask) (*automationmodel.AutomationTask, error) {
	taskID := uuid.MustParse(task.ID)
	locked, err := s.loadTask(ctx, tx, access, taskID, true)
	// The entry's scope either misses the task or sees it read-only.
	outOfScope := errors.Is(err, digitalhuman.ErrTaskNotFound) || errors.Is(err, digitalhuman.ErrTaskPolicyRejected)
	if !outOfScope || !taskHeldBy(task, access) {
		return locked, err
	}
	var held automationmodel.AutomationTask
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND organization_id = ? AND workspace_id = ? AND task_kind = ? AND subject_type = ? AND subject_id = ?", taskID, access.OrganizationID, access.WorkspaceID,
			automationmodel.AutomationTaskKindDigitalHuman, subjectTypeDigitalHuman, access.DigitalHumanID).
		First(&held).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, digitalhuman.ErrTaskNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock reviewed digital human task: %w", err)
	}
	if !taskHeldBy(&held, access) {
		return nil, digitalhuman.ErrTaskNotFound
	}
	return &held, nil
}
