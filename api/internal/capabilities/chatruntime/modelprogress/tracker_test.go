package modelprogress

import (
	"context"
	"testing"
	"time"
)

func TestTrackerStopPreventsFurtherEvents(t *testing.T) {
	events := make(chan map[string]interface{}, 4)
	tracker := Start(context.Background(), Options{
		ConversationID: "conversation-1",
		MessageID:      "message-1",
		Schedule: Schedule{
			Initial:     time.Millisecond,
			Extended:    time.Hour,
			LongRunning: 2 * time.Hour,
		},
		Emit: func(payload map[string]interface{}) {
			events <- payload
		},
	})

	select {
	case event := <-events:
		if got := event["stage"]; got != StageInitial {
			t.Fatalf("initial event stage = %v, want %q", got, StageInitial)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial progress event")
	}

	tracker.Stop()
	tracker.ObserveReasoningDelta()
	tracker.ObserveToolCallDelta()
	tracker.emitStage(StageExtended)
	tracker.Stop()

	select {
	case event := <-events:
		t.Fatalf("event after Stop() = %#v, want none", event)
	default:
	}
}
