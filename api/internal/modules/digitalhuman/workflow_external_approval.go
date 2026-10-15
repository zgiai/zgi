package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/errors/failureprojection"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// workflowApprovalWaitingStore is an optional persistence extension that finds
// the run a generic workflow approval holds.
type workflowApprovalWaitingStore interface {
	RunHeldByWorkflowApproval(context.Context, string) (*Run, bool, error)
}

// errWorkflowResumedElsewhere keeps a retaken resume queued while the workflow
// runs in another execution: the queue drops the plain pause errors.
var errWorkflowResumedElsewhere = errors.New("workflow is being resumed elsewhere; the conversation follows it once it pauses or finishes")

type externalWorkflowContinuationChatRuntime interface {
	workflowContinuationChatRuntime
	chatruntime.ExternalWorkflowApprovalContinuationBeginner
}

var (
	_ approvalruntime.ResumeHost          = (*RuntimeService)(nil)
	_ approvalruntime.ResumeHostAbandoner = (*RuntimeService)(nil)
)

// workflowApprovalAbandonedMessage is shown on a conversation whose approval
// was completed while the conversation could not follow the workflow.
const workflowApprovalAbandonedMessage = "The workflow approval was completed, but this conversation could not continue. Check the workflow run for its result."

type messageFailer interface {
	FailMessage(context.Context, chatruntime.Scope, chatruntime.FailMessageRequest) (*runtimemodel.Message, error)
}

// ResumeWorkflow resumes a workflow approval completed outside the
// conversation (an email or SMS approver, the approval page, or a timeout)
// together with the DigitalHuman run that waits on it: the workflow events
// stream into the same continuation lifecycle an in-conversation approval
// uses, so the message is summarized, completed, failed or paused again and
// the run settles with it. A question resume, carrying the answers given
// before the approval completed the pause, is followed the same way. It
// reports handled=false when no run waits on the workflow, or when the
// conversation already took the approval itself.
func (s *RuntimeService) ResumeWorkflow(ctx context.Context, req approvalruntime.ResumeRequest) (bool, error) {
	if s == nil || (req.Form == nil && req.QuestionInputs == nil) || !s.WorkflowContinuationEnabled() {
		return false, nil
	}
	store, ok := s.store.(workflowApprovalWaitingStore)
	if !ok {
		return false, nil
	}
	runtime, ok := s.chat.(externalWorkflowContinuationChatRuntime)
	if !ok {
		return false, nil
	}
	run, retaken, err := store.RunHeldByWorkflowApproval(ctx, req.WorkflowRunID)
	if err != nil || run == nil || run.ConversationID == nil || run.MessageID == nil {
		return false, err
	}
	scope := runtimeScope(run)
	caller := workflowRunCaller(run)
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return false, err
	}
	config, _, err := s.userInputContinuationConfig(ctx, run, resolved, event, runScope(run))
	if err != nil {
		return false, err
	}
	continuation, err := runtime.BeginExternalWorkflowApprovalContinuation(ctx, scope, caller, config, *run.ConversationID, *run.MessageID)
	if errors.Is(err, chatruntime.ErrInvalidInput) {
		// The conversation took the approval itself, or the message is no
		// longer waiting on it.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx: context.WithoutCancel(ctx), runtime: runtime, scope: scope, continuation: continuation,
	}
	lifecycle.settle = func(settleCtx context.Context, status, answer string, metadata map[string]interface{}) {
		s.settleWorkflowContinuationRun(settleCtx, run, status, answer, metadata)
	}
	// A retaken run is normally running already; one whose worker was lost
	// right after claiming the message is still waiting and starts here.
	if run.Status == RunStatusWaiting {
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
	}
	resume, err := s.resumeHostedWorkflow(ctx, req, lifecycle.handle)
	if workflowResumeNotStarted(err) {
		if retaken {
			return true, s.followRetakenWorkflow(ctx, lifecycle, req.WorkflowRunID, err)
		}
		// The workflow is still waiting (another approver on the same pause) or
		// is resumed elsewhere; keep the conversation waiting on it.
		return true, lifecycle.abortBeforeResume(err)
	}
	return true, lifecycle.finish(resume, err)
}

// followRetakenWorkflow settles a continuation whose earlier worker was lost
// after it claimed the message, when this resume could not run the workflow
// itself. It never puts the message back to waiting on the request it already
// took: either another execution still holds the workflow, and the queue
// retries, or the workflow moved on, and its durable state says where.
func (s *RuntimeService) followRetakenWorkflow(ctx context.Context, lifecycle *digitalHumanWorkflowContinuation, workflowRunID string, resumeErr error) error {
	if errors.Is(resumeErr, workflowpause.ErrResumeAlreadyRunning) {
		return resumeErr
	}
	progress, err := s.workflowProgress().Read(ctx, workflowRunID)
	if err != nil {
		return errors.Join(resumeErr, err)
	}
	if len(progress) == 0 {
		return fmt.Errorf("%w: %s", errWorkflowResumedElsewhere, workflowRunID)
	}
	for _, event := range progress {
		_ = lifecycle.handle(event.EventType, event.Payload)
	}
	return lifecycle.finish(workflowContinuationStreamed, nil)
}

// resumeHostedWorkflow resumes the workflow the way the request was queued:
// by its approval form, or with the question answers of the pause.
func (s *RuntimeService) resumeHostedWorkflow(ctx context.Context, req approvalruntime.ResumeRequest, onEvent func(string, map[string]interface{}) error) (workflowContinuationResume, error) {
	runner, streaming := s.workflowRunner.(workflowContinuationStreamRunner)
	switch {
	case req.Form != nil && streaming:
		return workflowContinuationStreamed, runner.ResumeApprovalWorkflowStream(ctx, req.Form, onEvent)
	case req.Form != nil:
		return workflowContinuationUnobserved, s.workflowRunner.ResumeApprovalWorkflow(ctx, req.Form)
	case streaming:
		return workflowContinuationStreamed, runner.ResumeQuestionAnswerWorkflowStream(ctx, req.WorkflowRunID, req.QuestionInputs, onEvent)
	default:
		return workflowContinuationUnobserved, s.workflowRunner.ResumeQuestionAnswerWorkflow(ctx, req.WorkflowRunID, req.QuestionInputs)
	}
}

func workflowResumeNotStarted(err error) bool {
	return errors.Is(err, workflowpause.ErrPauseNotFound) || errors.Is(err, workflowpause.ErrPauseNotResumeReady) || errors.Is(err, workflowpause.ErrResumeAlreadyRunning)
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

// AbandonWorkflowResume settles the run and the message that wait on the
// workflow when its resume gives up taking it: the workflow then resumes
// alone, and the conversation reports that it could not follow it instead of
// waiting forever.
func (s *RuntimeService) AbandonWorkflowResume(ctx context.Context, req approvalruntime.ResumeRequest, cause error) {
	if s == nil || req.WorkflowRunID == "" {
		return
	}
	store, ok := s.store.(workflowApprovalWaitingStore)
	if !ok {
		return
	}
	ctx = context.WithoutCancel(ctx)
	run, _, err := store.RunHeldByWorkflowApproval(ctx, req.WorkflowRunID)
	if err != nil || run == nil || run.ConversationID == nil || run.MessageID == nil {
		if err != nil {
			logger.WarnContext(ctx, "failed to find the DigitalHuman run of an abandoned workflow approval", "workflow_run_id", req.WorkflowRunID, err)
		}
		return
	}
	logger.WarnContext(ctx, "DigitalHuman conversation stops following its workflow approval", "run_id", run.ID.String(), "workflow_run_id", req.WorkflowRunID, "cause", cause)
	if failer, ok := s.chat.(messageFailer); ok {
		if _, err := failer.FailMessage(ctx, runtimeScope(run), chatruntime.FailMessageRequest{
			ConversationID: *run.ConversationID, MessageID: *run.MessageID, ErrorMessage: workflowApprovalAbandonedMessage,
		}); err != nil {
			logger.WarnContext(ctx, "failed to fail the message of an abandoned workflow approval", "run_id", run.ID.String(), err)
		}
	}
	if err := s.store.FailRun(ctx, run.ID, "digital_human.run.execution_failed", workflowApprovalAbandonedMessage, s.now()); err != nil {
		logger.WarnContext(ctx, "failed to fail the run of an abandoned workflow approval", "run_id", run.ID.String(), err)
	}
}

// externalWorkflowProgress is a workflow event that tells where a workflow
// went, read from its durable state.
type externalWorkflowProgress struct {
	EventType string
	Payload   map[string]interface{}
}

type workflowProgressReader interface {
	// Read returns the workflow's terminal event, or the requests of the pause
	// it waits on now; nothing while it runs or is being resumed.
	Read(ctx context.Context, workflowRunID string) ([]externalWorkflowProgress, error)
}

func (s *RuntimeService) workflowProgress() workflowProgressReader {
	if s.progressReader != nil {
		return s.progressReader
	}
	return gormWorkflowProgressReader{db: s.workflowDB}
}

type gormWorkflowProgressReader struct {
	db *gorm.DB
}

func (r gormWorkflowProgressReader) Read(ctx context.Context, workflowRunID string) ([]externalWorkflowProgress, error) {
	if r.db == nil {
		return nil, nil
	}
	var row struct {
		Status  string
		Outputs *string
		Error   *string
	}
	if err := r.db.WithContext(ctx).Table("workflow_run_logs").Select("status, outputs, error").
		Where("id = ? AND deleted_at IS NULL", workflowRunID).Take(&row).Error; err != nil {
		return nil, fmt.Errorf("load workflow run log: %w", err)
	}
	if progress, terminal := terminalWorkflowProgress(row.Status, row.Outputs, row.Error); terminal {
		return []externalWorkflowProgress{progress}, nil
	}
	pause, _, _, err := workflowpause.NewService(r.db).GetActiveByWorkflowRunID(ctx, workflowRunID)
	if errors.Is(err, workflowpause.ErrPauseNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pause.Status != workflowpause.RunPauseStatusPaused {
		// Ready to resume or being resumed: not settled yet.
		return nil, nil
	}
	var events []struct {
		EventType string
		EventData string
	}
	if err := r.db.WithContext(ctx).Model(&workflowpause.RunEvent{}).Select("event_type, event_data").
		Where("workflow_run_id = ? AND pause_id = ? AND pause_generation = ? AND event_type IN ?", workflowRunID, pause.ID, pause.Generation,
			[]string{workflowpause.EventApprovalRequested, workflowpause.EventQuestionAnswerRequested}).
		Order("sequence ASC").Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load workflow pause requests: %w", err)
	}
	progress := make([]externalWorkflowProgress, 0, len(events))
	for _, event := range events {
		payload := map[string]interface{}{}
		if err := json.Unmarshal([]byte(event.EventData), &payload); err != nil {
			return nil, fmt.Errorf("decode workflow pause request: %w", err)
		}
		progress = append(progress, externalWorkflowProgress{EventType: event.EventType, Payload: payload})
	}
	return progress, nil
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
