package modelprogress

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	Phase = "model_processing"

	StageInitial     = "initial"
	StageExtended    = "extended"
	StageLongRunning = "long_running"

	ActivityAwaitingResponse    = "awaiting_response"
	ActivityReviewingToolResult = "reviewing_tool_result"
	ActivityReasoning           = "reasoning"
	ActivityPreparingAction     = "preparing_action"

	SourceRuntime        = "runtime"
	SourceProviderSignal = "provider_signal"
)

// Schedule defines absolute delays from the start of one model call.
type Schedule struct {
	Initial     time.Duration
	Extended    time.Duration
	LongRunning time.Duration
}

// Options describes one model call and how its progress events are emitted.
type Options struct {
	ConversationID  string
	MessageID       string
	Round           int
	Model           string
	InitialActivity string
	Schedule        Schedule
	Emit            func(map[string]interface{})
}

// Tracker owns the staged progress lifecycle for one model call.
type Tracker struct {
	conversationID string
	messageID      string
	round          int
	model          string
	startedAt      time.Time
	emit           func(map[string]interface{})
	cancel         context.CancelFunc
	done           chan struct{}
	stopOnce       sync.Once

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
	stopped           bool
}

// Start creates and starts one progress tracker. Invalid options disable it.
func Start(ctx context.Context, options Options) *Tracker {
	conversationID := strings.TrimSpace(options.ConversationID)
	messageID := strings.TrimSpace(options.MessageID)
	if conversationID == "" || messageID == "" || options.Emit == nil {
		return nil
	}
	trackerCtx, cancel := context.WithCancel(ctx)
	activity := strings.TrimSpace(options.InitialActivity)
	if activity == "" {
		activity = ActivityAwaitingResponse
	}
	tracker := &Tracker{
		conversationID: conversationID,
		messageID:      messageID,
		round:          options.Round,
		model:          strings.TrimSpace(options.Model),
		startedAt:      time.Now(),
		emit:           options.Emit,
		cancel:         cancel,
		done:           make(chan struct{}),
		activity:       activity,
		source:         SourceRuntime,
	}
	go tracker.run(trackerCtx, normalizedSchedule(options.Schedule))
	return tracker
}

func normalizedSchedule(configured Schedule) Schedule {
	schedule := Schedule{
		Initial:     800 * time.Millisecond,
		Extended:    15 * time.Second,
		LongRunning: 45 * time.Second,
	}
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

func (t *Tracker) run(ctx context.Context, schedule Schedule) {
	defer close(t.done)
	stages := []struct {
		name  string
		after time.Duration
	}{
		{name: StageInitial, after: schedule.Initial},
		{name: StageExtended, after: schedule.Extended},
		{name: StageLongRunning, after: schedule.LongRunning},
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

func (t *Tracker) emitStage(stage string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stage = stage
	t.stagesSent++
	t.emitLocked()
}

func (t *Tracker) emitLocked() {
	if t.stopped {
		return
	}
	elapsed := time.Since(t.startedAt)
	if t.eventsSent == 0 {
		t.firstDelay = elapsed
	}
	t.eventsSent++
	t.emit(map[string]interface{}{
		"conversation_id": t.conversationID,
		"message_id":      t.messageID,
		"phase":           Phase,
		"progress_id":     fmt.Sprintf("%s:%d", t.messageID, t.round+1),
		"activity":        t.activity,
		"stage":           t.stage,
		"source":          t.source,
		"status":          "running",
		"round":           t.round + 1,
		"elapsed_ms":      elapsed.Milliseconds(),
		"created_at":      time.Now().Unix(),
	})
}

// ObserveReasoningDelta records a provider reasoning signal without exposing its content.
func (t *Tracker) ObserveReasoningDelta() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.reasoningSeen = true
	t.updateActivityLocked(ActivityReasoning)
}

// ObserveToolCallDelta records that the model is preparing a tool action.
func (t *Tracker) ObserveToolCallDelta() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.toolCallDeltaSeen = true
	t.updateActivityLocked(ActivityPreparingAction)
}

func (t *Tracker) updateActivityLocked(activity string) {
	if t.stopped {
		return
	}
	if activityRank(activity) <= activityRank(t.activity) {
		return
	}
	t.activity = activity
	t.source = SourceProviderSignal
	if t.stage == "" {
		return
	}
	t.activityUpdates++
	t.emitLocked()
}

func activityRank(activity string) int {
	switch activity {
	case ActivityReviewingToolResult:
		return 1
	case ActivityReasoning:
		return 2
	case ActivityPreparingAction:
		return 3
	default:
		return 0
	}
}

// Stop terminates the tracker and waits for its goroutine to exit.
func (t *Tracker) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() {
		t.mu.Lock()
		t.stopped = true
		t.mu.Unlock()
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
		logger.InfoContext(context.Background(), "chat runtime model progress completed",
			"conversation_id", t.conversationID,
			"message_id", t.messageID,
			"model", t.model,
			"model_round", t.round+1,
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
