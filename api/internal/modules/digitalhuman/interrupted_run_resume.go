package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// staleRuntimeLeaseError is the error the chat runtime records on a turn
// whose process stopped (its lease expired).
const staleRuntimeLeaseError = "runtime_lease_expired"

type interruptedTurnResumeRuntime interface {
	RunConfiguredInterruptedTurnResume(
		context.Context,
		chatruntime.Scope,
		chatruntime.Caller,
		chatruntime.RunConfig,
		uuid.UUID,
		uuid.UUID,
		func(chatruntime.StreamEvent) error,
	) (*chatruntime.ChatResult, error)
}

type runningRunLister interface {
	ListRunningRunsWithMessage(context.Context, time.Time, int) ([]*Run, error)
}

// resumeInterruptedRuns resumes runs whose turn stopped with its process
// (ADR 0035). Such a run is still running, but the chat runtime has marked its
// message runtime_lease_expired. The turn resumes in the background with the
// capabilities of its first run; a turn the runtime will not resume, such as
// one stopped during a change, is left for MarkStaleRunsUnknown.
func (s *RuntimeService) resumeInterruptedRuns(ctx context.Context, updatedBefore time.Time, limit int) error {
	resumer, ok := s.chat.(interruptedTurnResumeRuntime)
	if !ok {
		return nil
	}
	lister, ok := s.store.(runningRunLister)
	if !ok {
		return nil
	}
	runs, err := lister.ListRunningRunsWithMessage(ctx, updatedBefore, limit)
	if err != nil {
		return err
	}
	var combined error
	for _, run := range runs {
		if run == nil || run.Status != RunStatusRunning || run.MessageID == nil || run.ConversationID == nil {
			continue
		}
		message, _, loadErr := s.chat.GetMessageByCaller(ctx, runtimeScope(run), runConversationCaller(run), *run.MessageID)
		if loadErr != nil {
			combined = errors.Join(combined, loadErr)
			continue
		}
		if !interruptedTurnMessage(message) {
			continue
		}
		s.resumeInterruptedRunInBackground(ctx, resumer, run)
	}
	return combined
}

// interruptedTurnMessage reports a message the chat runtime stopped with its
// process and has not resumed yet.
func interruptedTurnMessage(message *runtimemodel.Message) bool {
	if message == nil || message.Status != runtimemodel.MessageStatusError || message.Error == nil ||
		strings.TrimSpace(*message.Error) != staleRuntimeLeaseError {
		return false
	}
	_, resumed := message.Metadata["interrupted_turn_resume"]
	return !resumed
}

func (s *RuntimeService) resumeInterruptedRunInBackground(ctx context.Context, resumer interruptedTurnResumeRuntime, run *Run) {
	background := context.WithoutCancel(ctx)
	go func() {
		runCtx, cancel := context.WithTimeout(background, runTaskTimeout)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.WarnContext(runCtx, "DigitalHuman interrupted run resume panicked", "run_id", run.ID.String(), fmt.Errorf("%v", recovered))
			}
		}()
		if err := s.resumeInterruptedRun(runCtx, resumer, run); err != nil {
			logger.WarnContext(runCtx, "DigitalHuman interrupted run was not resumed", "run_id", run.ID.String(), err)
		}
	}()
}

func (s *RuntimeService) resumeInterruptedRun(ctx context.Context, resumer interruptedTurnResumeRuntime, run *Run) error {
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return err
	}
	config, cleanup, err := s.userInputContinuationConfig(ctx, run, resolved, event, runScope(run))
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := resumer.RunConfiguredInterruptedTurnResume(
		// The caller carries the run, as the first run's did, so its
		// connector access resolves the same way.
		ctx, runtimeScope(run), workflowRunCaller(run), config, *run.ConversationID, *run.MessageID,
		func(chatruntime.StreamEvent) error { return nil },
	)
	if err != nil {
		// A turn that cannot be resumed stays for MarkStaleRunsUnknown.
		return err
	}
	logger.InfoContext(ctx, "DigitalHuman interrupted run resumed", "run_id", run.ID.String(), "status", result.Status)
	return s.settleContinuedRun(ctx, run, result)
}
