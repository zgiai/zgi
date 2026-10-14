package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/errors/failureprojection"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/gorm"
)

// externalWorkflowApprovalCheckDelay leaves a just-paused run to its own
// conversation first; an external approver needs far longer than this anyway.
const externalWorkflowApprovalCheckDelay = 10 * time.Second

// externalWorkflowApprovalClaimsPerPass bounds the continuations one reconcile
// pass finishes: each one may summarize the workflow with the model, and the
// pass must stay well inside the reconcile task timeout.
const externalWorkflowApprovalClaimsPerPass = 2

// workflowApprovalWaitingStore is an optional persistence extension that lists
// the runs a generic workflow approval holds.
type workflowApprovalWaitingStore interface {
	ListWorkflowApprovalWaitingRuns(context.Context, time.Time, int) ([]*Run, error)
	TouchWaitingRun(context.Context, uuid.UUID, time.Time) error
}

type externalWorkflowContinuationChatRuntime interface {
	workflowContinuationChatRuntime
	chatruntime.ExternalWorkflowApprovalContinuationBeginner
}

// externalWorkflowProgress is the workflow event that tells where a workflow
// went after its approval was completed outside the conversation. An empty
// EventType means the workflow still waits on that approval or is running.
type externalWorkflowProgress struct {
	EventType string
	Payload   map[string]interface{}
}

type externalWorkflowApprovalProbe interface {
	Observe(ctx context.Context, workflowRunID, approvalFormID string) (externalWorkflowProgress, error)
}

// resumeExternalWorkflowApprovals finishes the conversation side of workflow
// approvals completed by their configured approvers (email, SMS, approval
// page). The approval resume worker only resumes the workflow engine, so the
// waiting message and the DigitalHuman run follow the workflow from here.
func (s *RuntimeService) resumeExternalWorkflowApprovals(ctx context.Context, now time.Time, limit int) error {
	if !s.WorkflowContinuationEnabled() {
		return nil
	}
	store, ok := s.store.(workflowApprovalWaitingStore)
	if !ok {
		return nil
	}
	runtime, ok := s.chat.(externalWorkflowContinuationChatRuntime)
	if !ok {
		return nil
	}
	runs, err := store.ListWorkflowApprovalWaitingRuns(ctx, now.Add(-externalWorkflowApprovalCheckDelay), limit)
	if err != nil {
		return err
	}
	var combined error
	claimed := 0
	for _, run := range runs {
		if claimed >= externalWorkflowApprovalClaimsPerPass {
			break
		}
		observed, err := s.resumeExternalWorkflowApproval(ctx, runtime, run)
		if err != nil {
			combined = errors.Join(combined, fmt.Errorf("observe external workflow approval for DigitalHuman run %s: %w", run.ID, err))
		}
		if observed {
			claimed++
		} else if touchErr := store.TouchWaitingRun(ctx, run.ID, s.now()); touchErr != nil {
			combined = errors.Join(combined, touchErr)
		}
	}
	return combined
}

// resumeExternalWorkflowApproval reports whether the run's continuation was
// claimed. A claimed continuation always settles the message and the run.
func (s *RuntimeService) resumeExternalWorkflowApproval(ctx context.Context, runtime externalWorkflowContinuationChatRuntime, run *Run) (bool, error) {
	if run == nil || run.ConversationID == nil || run.MessageID == nil {
		return false, nil
	}
	scope := runtimeScope(run)
	caller := workflowRunCaller(run)
	message, _, err := s.chat.GetMessageByCaller(ctx, scope, caller, *run.MessageID)
	if err != nil {
		return false, err
	}
	if message == nil || message.Status != runtimemodel.MessageStatusWaitingApproval {
		return false, nil
	}
	state := workflowContinuationRecord(message.Metadata)
	workflowRunID, _ := state["workflow_run_id"].(string)
	kind, _ := state["kind"].(string)
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" || strings.TrimSpace(kind) == "digital_human_task" {
		return false, nil
	}
	progress, err := s.workflowApprovalProbe().Observe(ctx, workflowRunID, workflowApprovalFormID(message.Metadata, workflowRunID))
	if err != nil || progress.EventType == "" {
		return false, err
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return false, err
	}
	config, _, err := s.userInputContinuationConfig(ctx, run, resolved, event, runScope(run))
	if err != nil {
		return false, err
	}
	continuation, err := runtime.BeginExternalWorkflowApprovalContinuation(ctx, scope, caller, config, *run.ConversationID, *run.MessageID)
	if err != nil {
		return false, err
	}
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx: context.WithoutCancel(ctx), runtime: runtime, scope: scope, continuation: continuation,
	}
	lifecycle.settle = func(settleCtx context.Context, status, answer string, metadata map[string]interface{}) {
		s.settleWorkflowContinuationRun(settleCtx, run, status, answer, metadata)
	}
	waiter, ok := s.store.(runWaitingStore)
	if !ok {
		return true, lifecycle.abortBeforeResume(fmt.Errorf("digital human waiting run resume is unavailable"))
	}
	if err := waiter.ResumeWaitingRun(ctx, run.ID, s.now()); err != nil {
		// The run left waiting meanwhile (for example it was canceled), so it no
		// longer follows this message.
		lifecycle.settle = nil
		return true, lifecycle.abortBeforeResume(err)
	}
	_ = lifecycle.handle(progress.EventType, progress.Payload)
	return true, lifecycle.finish(workflowContinuationStreamed, nil)
}

func (s *RuntimeService) workflowApprovalProbe() externalWorkflowApprovalProbe {
	if s.externalApprovalProbe != nil {
		return s.externalApprovalProbe
	}
	return gormWorkflowApprovalProbe{db: s.workflowDB}
}

// workflowRunCaller is the conversation caller the run executed as.
func workflowRunCaller(run *Run) chatruntime.Caller {
	digitalHumanID := run.DigitalHumanID
	runID := run.ID
	caller := chatruntime.Caller{
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID, RunID: &runID,
		Source: digitalHumanConversationSourceForRun(run),
	}
	var event struct {
		Context map[string]any `json:"context"`
	}
	if json.Unmarshal(run.InvocationEvent, &event) == nil {
		if value, ok := event.Context["web_app_id"].(string); ok {
			if parsed, err := uuid.Parse(strings.TrimSpace(value)); err == nil && parsed != uuid.Nil {
				caller.SourceWebAppID = &parsed
			}
		}
	}
	return caller
}

// workflowApprovalFormID returns the approval form the message waits on.
func workflowApprovalFormID(metadata map[string]interface{}, workflowRunID string) string {
	runs, _ := metadata["workflow_runs"].([]interface{})
	for _, item := range runs {
		run, _ := item.(map[string]interface{})
		if id, _ := run["workflow_run_id"].(string); strings.TrimSpace(id) != workflowRunID {
			continue
		}
		approval, _ := run["approval"].(map[string]interface{})
		formID, _ := approval["approval_form_id"].(string)
		return strings.TrimSpace(formID)
	}
	return ""
}

type gormWorkflowApprovalProbe struct {
	db *gorm.DB
}

// Observe reads the durable workflow state: a terminal run log ends the
// continuation, and a pause on another approval or question moves the message
// to that request. The approval this message waits on keeps it waiting.
func (p gormWorkflowApprovalProbe) Observe(ctx context.Context, workflowRunID, approvalFormID string) (externalWorkflowProgress, error) {
	if p.db == nil {
		return externalWorkflowProgress{}, nil
	}
	var row struct {
		Status  string
		Outputs *string
		Error   *string
	}
	if err := p.db.WithContext(ctx).Table("workflow_run_logs").Select("status, outputs, error").
		Where("id = ? AND deleted_at IS NULL", workflowRunID).Take(&row).Error; err != nil {
		return externalWorkflowProgress{}, fmt.Errorf("load workflow run log: %w", err)
	}
	if progress, terminal := terminalWorkflowProgress(row.Status, row.Outputs, row.Error); terminal {
		return progress, nil
	}
	if approvalFormID == "" {
		return externalWorkflowProgress{}, nil
	}
	_, reasons, _, err := workflowpause.NewService(p.db).GetActiveByWorkflowRunID(ctx, workflowRunID)
	if errors.Is(err, workflowpause.ErrPauseNotFound) {
		return externalWorkflowProgress{}, nil
	}
	if err != nil {
		return externalWorkflowProgress{}, err
	}
	for _, reason := range reasons {
		if strings.TrimSpace(reason.FormID) == approvalFormID {
			return externalWorkflowProgress{}, nil
		}
	}
	var event struct {
		EventType string
		EventData string
	}
	err = p.db.WithContext(ctx).Model(&workflowpause.RunEvent{}).Select("event_type, event_data").
		Where("workflow_run_id = ? AND event_type IN ?", workflowRunID, []string{"approval_requested", "question_answer_requested"}).
		Order("sequence DESC").Take(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return externalWorkflowProgress{}, nil
	}
	if err != nil {
		return externalWorkflowProgress{}, fmt.Errorf("load workflow request event: %w", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
		return externalWorkflowProgress{}, fmt.Errorf("decode workflow request event: %w", err)
	}
	if formID, _ := payload["approval_form_id"].(string); strings.TrimSpace(formID) == approvalFormID {
		return externalWorkflowProgress{}, nil
	}
	return externalWorkflowProgress{EventType: event.EventType, Payload: payload}, nil
}

// terminalWorkflowProgress maps a terminal workflow run log to the event the
// streamed continuation would have observed.
func terminalWorkflowProgress(status string, outputs, runError *string) (externalWorkflowProgress, bool) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch {
	case normalized == "stopped":
		return externalWorkflowProgress{EventType: "workflow_stopped", Payload: map[string]interface{}{}}, true
	case failureprojection.IsFailureStatus(normalized) || normalized == "expired":
		normalized = "failed"
	case normalized != "succeeded" && normalized != "partial-succeeded":
		return externalWorkflowProgress{}, false
	}
	payload := map[string]interface{}{"status": normalized}
	if outputs != nil && strings.TrimSpace(*outputs) != "" {
		var decoded map[string]interface{}
		if json.Unmarshal([]byte(*outputs), &decoded) == nil && decoded != nil {
			payload["outputs"] = decoded
		}
	}
	if runError != nil && strings.TrimSpace(*runError) != "" {
		payload["error"] = strings.TrimSpace(*runError)
	}
	return externalWorkflowProgress{EventType: "workflow_finished", Payload: payload}, true
}
