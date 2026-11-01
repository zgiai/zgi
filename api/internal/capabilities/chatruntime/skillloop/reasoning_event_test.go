package skillloop

import "testing"

func TestEmitReasoningChunkUsesMessageEventWithEmptyAnswer(t *testing.T) {
	var events []Event
	runner := &Runner{OnEvent: func(event Event) error {
		events = append(events, event)
		return nil
	}}
	prepared := NewPreparedChat("conversation-1", "message-1", "zgi_cloud", "", nil)

	runner.emitReasoningChunk(prepared, "")
	runner.emitReasoningChunk(nil, "ignored")
	runner.emitReasoningChunk(prepared, "思考")

	if len(events) != 1 {
		t.Fatalf("events = %#v, want one reasoning event", events)
	}
	event := events[0]
	if event.Type != EventMessage {
		t.Fatalf("event type = %q, want %q", event.Type, EventMessage)
	}
	if event.Payload["answer"] != "" || event.Payload["reasoning_content"] != "思考" {
		t.Fatalf("payload = %#v, want empty answer and the reasoning delta", event.Payload)
	}
	if event.Payload["message_id"] != "message-1" || event.Payload["conversation_id"] != "conversation-1" {
		t.Fatalf("payload ids = %#v", event.Payload)
	}
}
