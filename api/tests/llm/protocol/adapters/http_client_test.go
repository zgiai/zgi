package adapters_test

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestParseSSEEventsReportsJSONErrorResponse(t *testing.T) {
	eventChan := make(chan adapter.RawStreamEvent, 1)
	errChan := make(chan error, 1)

	adapter.ParseSSEEvents(
		strings.NewReader(`{"error":{"message":"Forbidden","type":"rix_api_error","code":"bad_response"}}`),
		eventChan,
		errChan,
	)

	select {
	case err := <-errChan:
		if err == nil || !strings.Contains(err.Error(), "Forbidden") {
			t.Fatalf("error = %v, want upstream Forbidden error", err)
		}
	default:
		t.Fatal("expected upstream JSON error")
	}

	if event, ok := <-eventChan; ok {
		t.Fatalf("event = %#v, want closed event channel", event)
	}
}

func TestParseSSEEventsPreservesNativeEvent(t *testing.T) {
	eventChan := make(chan adapter.RawStreamEvent, 1)
	errChan := make(chan error, 1)

	adapter.ParseSSEEvents(
		strings.NewReader("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n"),
		eventChan,
		errChan,
	)

	event, ok := <-eventChan
	if !ok {
		t.Fatal("event channel closed before event")
	}
	if event.Event != "response.output_text.delta" {
		t.Fatalf("event name = %q, want response.output_text.delta", event.Event)
	}
	if !strings.Contains(string(event.Data), `"delta":"hi"`) {
		t.Fatalf("event data = %s, want delta", string(event.Data))
	}
}
