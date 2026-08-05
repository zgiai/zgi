package skillloop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	modelProgressPhase = "model_processing"

	modelProgressStageInitial     = "initial"
	modelProgressStageExtended    = "extended"
	modelProgressStageLongRunning = "long_running"

	modelProgressActivityAwaitingResponse    = "awaiting_response"
	modelProgressActivityReviewingToolResult = "reviewing_tool_result"
	modelProgressActivityReasoning           = "reasoning"
	modelProgressActivityPreparingAction     = "preparing_action"

	modelProgressSourceRuntime        = "runtime"
	modelProgressSourceProviderSignal = "provider_signal"
)

// ModelProgressSchedule defines absolute delays from the start of one native
// Agent model call. Tests can replace the defaults with short durations.
type ModelProgressSchedule struct {
	Initial     time.Duration
	Extended    time.Duration
	LongRunning time.Duration
}

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

type modelProgressStage struct {
	name  string
	after time.Duration
}

type modelProgressTracker struct {
	runner    *Runner
	prepared  *PreparedChat
	round     int
	model     string
	startedAt time.Time
	cancel    context.CancelFunc
	done      chan struct{}
	stopOnce  sync.Once

	mu                sync.Mutex
	stage             string
	activity          string
	source            string
	stagesSent        int
	eventsSent        int
	activityUpdates   int
	firstDelay        time.Duration
	reasoningSeen     bool
	toolCallDeltaSeen bool
}

func (r *Runner) startModelProgressTracker(
	ctx context.Context,
	prepared *PreparedChat,
	round int,
	model string,
	messages []adapter.Message,
	enabled bool,
) *modelProgressTracker {
	if !enabled || prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return nil
	}
	trackerCtx, cancel := context.WithCancel(ctx)
	tracker := &modelProgressTracker{
		runner:    r,
		prepared:  prepared,
		round:     round,
		model:     strings.TrimSpace(model),
		startedAt: time.Now(),
		cancel:    cancel,
		done:      make(chan struct{}),
		activity:  initialModelProgressActivity(messages),
		source:    modelProgressSourceRuntime,
	}
	go tracker.run(trackerCtx, r.modelProgressSchedule())
	return tracker
}

func (t *modelProgressTracker) run(ctx context.Context, schedule ModelProgressSchedule) {
	defer close(t.done)
	stages := []modelProgressStage{
		{name: modelProgressStageInitial, after: schedule.Initial},
		{name: modelProgressStageExtended, after: schedule.Extended},
		{name: modelProgressStageLongRunning, after: schedule.LongRunning},
	}
	for _, stage := range stages {
		wait := stage.after - time.Since(t.startedAt)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
			t.emitStage(stage.name)
		}
	}
	<-ctx.Done()
}

func (t *modelProgressTracker) emitStage(stage string) {
	if t == nil || t.runner == nil || t.prepared == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stage = stage
	t.stagesSent++
	t.emitLocked()
}

func (t *modelProgressTracker) emitLocked() {
	elapsed := time.Since(t.startedAt)
	if t.eventsSent == 0 {
		t.firstDelay = elapsed
	}
	t.eventsSent++
	messageID := t.prepared.Message.ID.String()
	t.runner.emitEvent(t.prepared, EventAgentProgress, map[string]interface{}{
		"conversation_id": t.prepared.Conversation.ID.String(),
		"message_id":      messageID,
		"phase":           modelProgressPhase,
		"progress_id":     fmt.Sprintf("%s:%d", messageID, t.round+1),
		"activity":        t.activity,
		"stage":           t.stage,
		"source":          t.source,
		"status":          "running",
		"round":           t.round + 1,
		"elapsed_ms":      elapsed.Milliseconds(),
		"created_at":      time.Now().Unix(),
	})
}

func (t *modelProgressTracker) ObserveReasoningDelta() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reasoningSeen = true
	t.updateActivityLocked(modelProgressActivityReasoning)
}

func (t *modelProgressTracker) ObserveToolCallDelta() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolCallDeltaSeen = true
	t.updateActivityLocked(modelProgressActivityPreparingAction)
}

func (t *modelProgressTracker) updateActivityLocked(activity string) {
	if modelProgressActivityRank(activity) <= modelProgressActivityRank(t.activity) {
		return
	}
	t.activity = activity
	t.source = modelProgressSourceProviderSignal
	if t.stage == "" {
		return
	}
	t.activityUpdates++
	t.emitLocked()
}

func modelProgressActivityRank(activity string) int {
	switch activity {
	case modelProgressActivityReviewingToolResult:
		return 1
	case modelProgressActivityReasoning:
		return 2
	case modelProgressActivityPreparingAction:
		return 3
	default:
		return 0
	}
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

func (t *modelProgressTracker) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() {
		t.cancel()
		<-t.done
		t.mu.Lock()
		firstDelay := t.firstDelay
		stagesSent := t.stagesSent
		eventsSent := t.eventsSent
		activityUpdates := t.activityUpdates
		finalActivity := t.activity
		reasoningSeen := t.reasoningSeen
		toolCallDeltaSeen := t.toolCallDeltaSeen
		t.mu.Unlock()
		logger.InfoContext(context.Background(), "chat runtime native model progress completed",
			"conversation_id", t.prepared.Conversation.ID.String(),
			"message_id", t.prepared.Message.ID.String(),
			"model", t.model,
			"agent_round", t.round+1,
			"first_state_latency_ms", firstDelay.Milliseconds(),
			"progress_stage_count", stagesSent,
			"progress_event_count", eventsSent,
			"activity_update_count", activityUpdates,
			"final_activity", finalActivity,
			"model_duration_ms", time.Since(t.startedAt).Milliseconds(),
			"reasoning_delta_received", reasoningSeen,
			"tool_call_delta_received", toolCallDeltaSeen,
		)
	})
}
