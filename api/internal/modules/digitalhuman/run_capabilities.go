package digitalhuman

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// runCapabilities is what applyRunCapabilities prepared besides the config.
type runCapabilities struct {
	channelContext       *ChannelContextPreparation
	bindTaskConversation func(uuid.UUID)
}

// runCapabilityError names the preparation that failed, so the first run can
// record it on the run.
type runCapabilityError struct {
	stage string // "memory" or "tasks"
	err   error
}

func (e *runCapabilityError) Error() string { return e.err.Error() }

func (e *runCapabilityError) Unwrap() error { return e.err }

// applyRunCapabilities prepares what a run has besides its stored
// configuration: the time, connector notes and policy, channel context,
// memory, and tasks, with their guidance and tools. The first run and every
// continuation of it use it, so a continuation sends the same system prompt
// and tools as the run it continues and can reuse the provider's cached
// prefix (ADR 0032).
func (s *RuntimeService) applyRunCapabilities(ctx context.Context, config *chatruntime.RunConfig, run *Run, resolved *ResolvedExecution, event invocation.Event) (runCapabilities, error) {
	result := runCapabilities{}
	config.ConnectorRunID = run.ID.String()
	location := s.runLocation(ctx, run)
	config.UserTimezone = location.String()
	config.CurrentTimeProvided = true
	addTurnContext(config, currentTimeGuidance(s.runTurnTime(run), location))
	applyRunEndUserBilling(config, run)
	if repository, ok := s.store.(*GormRepository); ok {
		missing, perUserErr := preparePerUserConnectors(ctx, repository.db, repository.connectorCatalog, run)
		if perUserErr != nil {
			logger.WarnContext(ctx, "DigitalHuman per-user connector preparation failed", "run_id", run.ID.String(), perUserErr)
		}
		if note := perUserConnectorNote(missing); note != "" {
			addTurnContext(config, note)
		}
		if note, noteErr := connectorApprovalNote(ctx, repository.db, run, event); noteErr != nil {
			logger.WarnContext(ctx, "DigitalHuman connector approval note failed", "run_id", run.ID.String(), noteErr)
		} else if note != "" {
			addTurnContext(config, note)
		}
	}
	config.ConnectorInteractive = connectorInteractive(event)
	config.ConnectorRequesterID = connectorRequester(event)
	applyScheduledConnectorPolicy(config, run, event)
	if s.channelContext != nil {
		preparation, err := s.channelContext.Prepare(ctx, run, resolved, event, config.ModelProvider, config.Model)
		if err != nil {
			preparation = &ChannelContextPreparation{
				SystemPromptAddendum: strings.Join([]string{
					"Channel conversation context is unavailable for this turn.",
					"Do not guess missing source content.",
					channelContextMissingSourceGuidance,
				}, "\n"),
				Diagnostics: ChannelContextDiagnostics{Status: string(channelruntime.ContextStatusUnavailable), ErrorCode: "channel_context_unavailable"},
			}
		}
		if preparation != nil {
			addTurnContext(config, preparation.SystemPromptAddendum)
			config.RuntimeTools = append(config.RuntimeTools, preparation.Tools...)
		}
		result.channelContext = preparation
	}
	if previewRun(run) {
		addTurnContext(config, previewGuidance)
		return result, nil
	}
	if s.memory != nil {
		preparation, err := s.memory.Prepare(ctx, run, resolved, event)
		if err != nil {
			return result, &runCapabilityError{stage: "memory", err: err}
		}
		if preparation != nil {
			addSystemGuidance(config, preparation.StableGuidance)
			addTurnContext(config, preparation.SystemPromptAddendum)
			config.RuntimeTools = append(config.RuntimeTools, preparation.Tools...)
		}
	}
	if s.tasks != nil {
		preparation, err := s.tasks.Prepare(withRunTimezone(ctx, location), run, event)
		if err != nil {
			return result, &runCapabilityError{stage: "tasks", err: err}
		}
		if preparation != nil {
			addSystemGuidance(config, preparation.StableGuidance)
			addTurnContext(config, preparation.SystemPromptAddendum)
			config.RuntimeTools = append(config.RuntimeTools, preparation.Tools...)
			result.bindTaskConversation = preparation.BindConversation
		}
	}
	return result, nil
}

// continuationTimeWindow is how long a continuation keeps the time its run
// started with. Within it, the continuation's turn context, and so the prefix
// the provider cached for the first run, stays the same; after it, the time
// is brought up to date and the prefix is sent again.
const continuationTimeWindow = 15 * time.Minute

// runTurnTime is the current time a run tells the model: when the run
// started, for the run itself and a continuation soon after it, or now.
func (s *RuntimeService) runTurnTime(run *Run) time.Time {
	now := s.now()
	if run.StartedAt != nil && !run.StartedAt.After(now) && now.Sub(*run.StartedAt) < continuationTimeWindow {
		return *run.StartedAt
	}
	return now
}

// prepareBackgroundContinuationLinux gives a continuation that has no caller
// of its own, such as one resumed after a connector confirmation, the Linux
// tools of the first run. Without them the continuation still runs, as it did
// before it had them.
func (s *RuntimeService) prepareBackgroundContinuationLinux(ctx context.Context, config *chatruntime.RunConfig, run *Run, resolved *ResolvedExecution, event invocation.Event) {
	if s.linux == nil {
		return
	}
	factory, err := s.linux.Prepare(ctx, run, resolved, event)
	if err != nil {
		logger.WarnContext(ctx, "DigitalHuman continuation Linux preparation failed", "run_id", run.ID.String(), err)
		return
	}
	config.RuntimeToolFactory = factory
}

// refuseRuntimeTool keeps a tool's definition but replaces what it does. A
// continuation uses it instead of removing a tool, so its tool list stays
// that of the first run.
func refuseRuntimeTool(config *chatruntime.RunConfig, name string, handler func(context.Context, adapter.ToolCall) skillloop.RuntimeToolResult) {
	for index, tool := range config.RuntimeTools {
		if strings.EqualFold(strings.TrimSpace(tool.Definition.Function.Name), name) {
			config.RuntimeTools[index].Handler = handler
		}
	}
}
