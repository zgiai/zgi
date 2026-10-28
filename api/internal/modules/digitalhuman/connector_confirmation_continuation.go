package digitalhuman

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type connectorConfirmationContinuationRuntime interface {
	ClaimConnectorConfirmationContinuation(context.Context, connectorconfirmation.Command, connectorconfirmation.Result) (*chatruntime.ConnectorConfirmationContinuation, error)
	RunConfiguredConnectorConfirmationContinuation(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig,
		*chatruntime.ConnectorConfirmationContinuation, func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error)
}

// ResumeAfterConnectorConfirmation records a decided console confirmation and
// continues the paused turn (ADR 0024). The claim happens in the request; the
// model pass runs after it returns, and the browser follows it on the
// conversation's stream.
func (s *RuntimeService) ResumeAfterConnectorConfirmation(ctx context.Context, command connectorconfirmation.Command, result connectorconfirmation.Result) error {
	if s == nil {
		return ErrRunConflict
	}
	runtime, ok := s.chat.(connectorConfirmationContinuationRuntime)
	if !ok {
		return ErrRunConflict
	}
	claim, err := runtime.ClaimConnectorConfirmationContinuation(ctx, command, result)
	if err != nil {
		return err
	}
	background := context.WithoutCancel(ctx)
	go func() {
		runCtx, cancel := context.WithTimeout(background, runTaskTimeout)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.WarnContext(runCtx, "DigitalHuman connector continuation panicked", "run_id", claim.RunID.String(), fmt.Errorf("%v", recovered))
				_ = s.store.FailRun(runCtx, claim.RunID, "digital_human.run.execution_failed", "The DigitalHuman execution did not complete.", s.now())
			}
		}()
		if err := s.continueAfterConnectorConfirmation(runCtx, runtime, claim); err != nil {
			logger.WarnContext(runCtx, "DigitalHuman connector continuation failed", "run_id", claim.RunID.String(), err)
		}
	}()
	return nil
}

func (s *RuntimeService) continueAfterConnectorConfirmation(ctx context.Context, runtime connectorConfirmationContinuationRuntime, claim *chatruntime.ConnectorConfirmationContinuation) error {
	run, err := s.store.RunByID(ctx, Scope{OrganizationID: claim.OrganizationID, WorkspaceID: claim.WorkspaceID}, claim.DigitalHumanID, claim.RunID)
	if err != nil {
		return err
	}
	if run.Status != RunStatusRunning || run.MessageID == nil || *run.MessageID != claim.MessageID {
		return ErrRunConflict
	}
	fail := func(cause error) error {
		_ = s.store.FailRun(ctx, run.ID, "digital_human.run.execution_failed", "The DigitalHuman execution did not complete.", s.now())
		return cause
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return fail(err)
	}
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		return fail(err)
	}
	s.attachAttachmentReader(&config, run, resolved, event, nil)
	config.ConnectorRunID = run.ID.String()
	config.ConnectorInteractive = event.Origin.Kind == invocation.OriginManual
	location := s.runLocation(ctx, run)
	config.UserTimezone = location.String()
	config.SystemPrompt = strings.TrimSpace(config.SystemPrompt) + "\n\n" + currentTimeGuidance(s.now(), location)
	applyRunEndUserBilling(&config, run)
	callerID := run.DigitalHumanID
	caller := chatruntime.Caller{
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, RunID: &run.ID, Source: digitalHumanConversationSourceForRun(run),
	}
	if webAppID, ok := stringContextValue(event.Context, "web_app_id"); ok {
		if parsed, parseErr := uuid.Parse(webAppID); parseErr == nil && parsed != uuid.Nil {
			caller.SourceWebAppID = &parsed
		}
	}
	result, err := runtime.RunConfiguredConnectorConfirmationContinuation(ctx, runtimeScope(run), caller, config, claim, nil)
	if err != nil {
		return fail(err)
	}
	return s.settleContinuedRun(ctx, run, result)
}

// settleContinuedRun gives a continued run the same end as a first pass: it
// waits again, or completes with the answer.
func (s *RuntimeService) settleContinuedRun(ctx context.Context, run *Run, result *chatruntime.ChatResult) error {
	if result == nil {
		return ErrRunConflict
	}
	switch result.Status {
	case runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion,
		runtimemodel.MessageStatusWaitingClientAction, runtimemodel.MessageStatusWaitingBudget:
		waiter, ok := s.store.(runWaitingStore)
		if !ok {
			return ErrRunConflict
		}
		if result.Status == runtimemodel.MessageStatusWaitingApproval {
			s.notifyTaskApproval(ctx, run)
		}
		return waiter.MarkRunWaiting(ctx, run.ID, result.Status, s.now())
	}
	return s.store.CompleteRunWithDelivery(ctx, run.ID, result.Answer, executionTraceFromMetadata(result.Metadata), s.now())
}
