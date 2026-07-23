package agents

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentSSEWriterHeartbeatIsTransportOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/stream", nil)
	w := newAgentSSEWriter(context)

	if err := w.writeHeartbeat(); err != nil {
		t.Fatalf("writeHeartbeat: %v", err)
	}
	if err := w.WriteEvent("event-1", "message_chunk", gin.H{"answer": "ok"}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}

	body := recorder.Body.String()
	if !strings.HasPrefix(body, ": heartbeat\n\n") {
		t.Fatalf("stream body = %q, want heartbeat comment prefix", body)
	}
	if strings.Contains(body, "event: heartbeat") {
		t.Fatalf("heartbeat must not be emitted as a business event: %q", body)
	}
	for _, expected := range []string{"id: event-1", "event: message_chunk", `"answer":"ok"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("stream body = %q, want %q", body, expected)
		}
	}
}
