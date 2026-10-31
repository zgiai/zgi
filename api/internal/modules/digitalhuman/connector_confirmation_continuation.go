package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"

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

// ResumeAfterConnectorConfirmation records a decided confirmation (console,
// channel reply or API) and continues the paused turn (ADR 0024). The claim
// happens in the request; the model pass runs after it returns, the browser
// follows it on the conversation's stream, and a channel receives its answer.
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
	// The continuation prepares what the first run had, so it sends the same
	// system prompt and tools and reuses their cached prefix (ADR 0032).
	s.prepareBackgroundContinuationLinux(ctx, &config, run, resolved, event)
	if _, err := s.applyRunCapabilities(ctx, &config, run, resolved, event); err != nil {
		return fail(err)
	}
	callerID := run.DigitalHumanID
	caller := chatruntime.Caller{
		Type: runtimemodel.ConversationCallerDigitalHuman, ID: &callerID, RunID: &run.ID, Source: digitalHumanConversationSourceForRun(run),
	}
	if webAppID, ok := stringContextValue(event.Context, "web_app_id"); ok {
		if parsed, parseErr := uuid.Parse(webAppID); parseErr == nil && parsed != uuid.Nil {
			caller.SourceWebAppID = &parsed
		}
	}
	// A channel shows the continued turn's progress as the first pass did.
	var onEvent func(chatruntime.StreamEvent) error
	if event.Origin.Kind == invocation.OriginChannel && s.transient != nil {
		progress := newChannelProgressCollector(s.transient, run.ID)
		onEvent = func(streamEvent chatruntime.StreamEvent) error { return progress.Handle(ctx, streamEvent) }
	}
	result, err := runtime.RunConfiguredConnectorConfirmationContinuation(ctx, runtimeScope(run), caller, config, claim, onEvent)
	if err != nil {
		if event.Origin.Kind == invocation.OriginChannel {
			s.deliverChannelContinuationFailure(ctx, run, claim.RouteReceipt)
		}
		return fail(err)
	}
	return s.settleContinuedRun(ctx, run, result)
}

// channelContinuationFailedNotice tells a channel that the turn stopped after
// the decided operation; the channel has no run page of its own.
const channelContinuationFailedNotice = "操作已处理，但数字人继续执行时出错，请到 ZGI 控制台的运行记录查看。"

// deliverChannelContinuationFailure is best effort: the run fails either way.
func (s *RuntimeService) deliverChannelContinuationFailure(ctx context.Context, run *Run, receipt string) {
	deliveryStore, ok := s.store.(channelFinalDeliveryStore)
	if !ok || run == nil {
		return
	}
	if err := deliveryStore.EnqueueRunChannelFinalDelivery(context.WithoutCancel(ctx), run.ID, "connector-continuation-failed:"+receipt, channelContinuationFailedNotice, s.now()); err != nil {
		logger.WarnContext(ctx, "failed to deliver digital human channel continuation failure", "run_id", run.ID.String(), err)
	}
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
		if err := waiter.MarkRunWaiting(ctx, run.ID, result.Status, s.now()); err != nil {
			return err
		}
		s.deliverContinuedChannelWaitingPrompt(ctx, run, result)
		return nil
	}
	return s.store.CompleteRunWithDelivery(ctx, run.ID, result.Answer, executionTraceFromMetadata(result.Metadata), s.now())
}

// deliverContinuedChannelWaitingPrompt asks a channel what a continued run now
// waits for, as a first pass does; a further confirmation names its own
// receipt, so it is a new prompt.
func (s *RuntimeService) deliverContinuedChannelWaitingPrompt(ctx context.Context, run *Run, result *chatruntime.ChatResult) {
	if run == nil || run.MessageID == nil || len(run.InvocationEvent) == 0 {
		return
	}
	var event invocation.Event
	if json.Unmarshal(run.InvocationEvent, &event) != nil || event.Origin.Kind != invocation.OriginChannel {
		return
	}
	waiting := *run
	if refreshed, err := s.store.RunByID(ctx, Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID}, run.DigitalHumanID, run.ID); err == nil && refreshed != nil {
		waiting.WaitingReceipt = refreshed.WaitingReceipt
	}
	s.deliverChannelWaitingPrompt(ctx, &waiting, *run.MessageID, result.Status, result.Answer, result.Metadata)
}
