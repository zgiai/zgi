package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestReasoningClockSumsSpans(t *testing.T) {
	base := time.Unix(1_000, 0)
	var clock reasoningClock
	clock.observe(base)
	if got := clock.observe(base.Add(2 * time.Second)); got != 2*time.Second {
		t.Fatalf("first span = %v, want 2s", got)
	}
	clock.closeSpan()
	clock.closeSpan() // idempotent
	// Tool execution between rounds is not thinking time.
	clock.observe(base.Add(10 * time.Second))
	if got := clock.observe(base.Add(13 * time.Second)); got != 5*time.Second {
		t.Fatalf("two spans = %v, want 5s", got)
	}
}

func TestAppendPreparedReasoningContentRecordsTextAndDuration(t *testing.T) {
	prepared := &PreparedChat{Message: &runtimemodel.Message{Metadata: map[string]interface{}{"keep": true}}}
	base := time.Unix(1_000, 0)
	appendPreparedReasoningContentAt(prepared, "先想", base)
	appendPreparedReasoningContentAt(prepared, "一下", base.Add(1500*time.Millisecond))

	metadata := prepared.Message.Metadata
	if metadata["reasoning_content"] != "先想一下" {
		t.Fatalf("reasoning_content = %#v", metadata["reasoning_content"])
	}
	if metadata["reasoning_duration_ms"] != int64(1500) {
		t.Fatalf("reasoning_duration_ms = %#v, want 1500", metadata["reasoning_duration_ms"])
	}
	if metadata["keep"] != true {
		t.Fatalf("existing metadata lost: %#v", metadata)
	}
	appendPreparedReasoningContentAt(prepared, "", base.Add(time.Hour))
	appendPreparedReasoningContentAt(nil, "x", base)
	if prepared.Message.Metadata["reasoning_duration_ms"] != int64(1500) {
		t.Fatal("empty reasoning must not move the clock")
	}
}

func TestIsReasoningOnlyMessageEvent(t *testing.T) {
	cases := []struct {
		eventType string
		payload   map[string]interface{}
		want      bool
	}{
		{streamEventMessage, map[string]interface{}{"answer": "", "reasoning_content": "r"}, true},
		{streamEventMessage, map[string]interface{}{"reasoning_content": "r"}, true},
		{streamEventMessage, map[string]interface{}{"answer": "a", "reasoning_content": "r"}, false},
		{streamEventMessage, map[string]interface{}{"answer": "a"}, false},
		{streamEventMessageRetract, map[string]interface{}{"reasoning_content": "r"}, false},
		{streamEventMessage, nil, false},
	}
	for _, tc := range cases {
		if got := isReasoningOnlyMessageEvent(tc.eventType, tc.payload); got != tc.want {
			t.Fatalf("isReasoningOnlyMessageEvent(%q, %#v) = %v, want %v", tc.eventType, tc.payload, got, tc.want)
		}
	}
}

func TestProcessTimelineReasoningStaysOutOfPresentation(t *testing.T) {
	message := &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{}}
	prepared := &PreparedChat{Conversation: &runtimemodel.Conversation{ID: uuid.New()}, Message: message}
	var delivered []StreamEvent
	recorder := newProcessTimelineRecorder(context.Background(), context.Background(), &service{}, prepared, func(event StreamEvent) error {
		delivered = append(delivered, event)
		return nil
	})

	for _, chunk := range []string{"round one ", "thinking"} {
		if err := recorder.RecordEvent(streamEventMessage, map[string]interface{}{"answer": "", "reasoning_content": chunk}); err != nil {
			t.Fatalf("RecordEvent(reasoning) error = %v", err)
		}
	}
	if err := recorder.RecordEvent(streamEventMessage, map[string]interface{}{"answer": "final"}); err != nil {
		t.Fatalf("RecordEvent(answer) error = %v", err)
	}
	if err := recorder.RecordEvent(streamEventMessage, map[string]interface{}{"answer": "", "reasoning_content": " / round two"}); err != nil {
		t.Fatalf("RecordEvent(reasoning) error = %v", err)
	}

	if got := prepared.Message.Metadata["reasoning_content"]; got != "round one thinking / round two" {
		t.Fatalf("reasoning_content = %#v", got)
	}
	if _, ok := prepared.Message.Metadata["reasoning_duration_ms"].(int64); !ok {
		t.Fatalf("reasoning_duration_ms = %#v, want int64", prepared.Message.Metadata["reasoning_duration_ms"])
	}
	items := recorder.presentation.Items
	if len(items) != 1 || stringFromAny(items[0]["content"]) != "final" {
		t.Fatalf("presentation items = %#v, want only the answer segment", items)
	}
	if len(delivered) != 4 {
		t.Fatalf("delivered %d events, want 4", len(delivered))
	}
	if _, ok := delivered[0].Payload["reasoning_duration_ms"]; !ok {
		t.Fatalf("reasoning event payload lacks reasoning_duration_ms: %#v", delivered[0].Payload)
	}
}
