package skillloop

import (
	"context"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/modelprogress"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	modelProgressPhase = modelprogress.Phase

	modelProgressStageInitial     = modelprogress.StageInitial
	modelProgressStageExtended    = modelprogress.StageExtended
	modelProgressStageLongRunning = modelprogress.StageLongRunning

	modelProgressActivityAwaitingResponse    = modelprogress.ActivityAwaitingResponse
	modelProgressActivityReviewingToolResult = modelprogress.ActivityReviewingToolResult
	modelProgressActivityReasoning           = modelprogress.ActivityReasoning
	modelProgressActivityPreparingAction     = modelprogress.ActivityPreparingAction

	modelProgressSourceRuntime        = modelprogress.SourceRuntime
	modelProgressSourceProviderSignal = modelprogress.SourceProviderSignal
)

type ModelProgressSchedule = modelprogress.Schedule
type modelProgressTracker = modelprogress.Tracker

func (r *Runner) modelProgressSchedule() ModelProgressSchedule {
	schedule := ModelProgressSchedule{
		Initial:     800 * time.Millisecond,
		Extended:    15 * time.Second,
		LongRunning: 45 * time.Second,
	}
	if r == nil {
		return schedule
	}
	configured := r.ModelProgressSchedule
	if configured.Initial > 0 {
		schedule.Initial = configured.Initial
	}
	if configured.Extended > schedule.Initial {
		schedule.Extended = configured.Extended
	}
	if configured.LongRunning > schedule.Extended {
		schedule.LongRunning = configured.LongRunning
	}
	return schedule
}

func (r *Runner) startModelProgressTracker(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	model string,
	messages []adapter.Message,
) *modelProgressTracker {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return nil
	}
	return modelprogress.Start(ctx, modelprogress.Options{
		ConversationID:  prepared.Conversation.ID.String(),
		MessageID:       prepared.Message.ID.String(),
		Round:           round,
		Model:           model,
		InitialActivity: initialModelProgressActivity(messages),
		Schedule:        r.modelProgressSchedule(),
		Emit: func(payload map[string]interface{}) {
			r.emitEvent(prepared, EventAgentProgress, payload)
		},
	})
}

func initialModelProgressActivity(messages []adapter.Message) string {
	lastUserIndex := -1
	for index := range messages {
		if strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			lastUserIndex = index
		}
	}
	toolResults := make(map[string]struct{})
	for index := lastUserIndex + 1; index < len(messages); index++ {
		message := messages[index]
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		if callID := strings.TrimSpace(message.ToolCallID); callID != "" {
			toolResults[callID] = struct{}{}
		}
	}
	if len(toolResults) == 0 {
		return modelProgressActivityAwaitingResponse
	}
	for index := lastUserIndex + 1; index < len(messages); index++ {
		for _, call := range messages[index].ToolCalls {
			if _, ok := toolResults[strings.TrimSpace(call.ID)]; !ok {
				continue
			}
			if isModelProgressBusinessToolName(call.Function.Name) {
				return modelProgressActivityReviewingToolResult
			}
		}
	}
	return modelProgressActivityAwaitingResponse
}

func isModelProgressBusinessToolName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || isSkillMetaToolName(name) || isInjectedContextPseudoToolName(name) {
		return false
	}
	switch name {
	case skills.MetaToolActivateSkills, skills.MetaToolSearchSkills:
		return false
	default:
		return true
	}
}
