package agents

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// returnTrackingWriter records writes that reach the response after the
// handler has returned, when net/http may already have released its buffer.
type returnTrackingWriter struct {
	gin.ResponseWriter
	returned  *atomic.Bool
	lateWrite *atomic.Bool
}

func (w *returnTrackingWriter) Write(data []byte) (int, error) {
	w.checkLate()
	return w.ResponseWriter.Write(data)
}

func (w *returnTrackingWriter) WriteString(s string) (int, error) {
	w.checkLate()
	return w.ResponseWriter.WriteString(s)
}

func (w *returnTrackingWriter) Flush() {
	w.checkLate()
	w.ResponseWriter.Flush()
}

func (w *returnTrackingWriter) checkLate() {
	if w.returned.Load() {
		w.lateWrite.Store(true)
	}
}

func TestAgentSSEHeartbeatStopsBeforeHandlerReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := agentSSEHeartbeatInterval
	agentSSEHeartbeatInterval = time.Millisecond
	t.Cleanup(func() { agentSSEHeartbeatInterval = previous })

	var returned, lateWrite atomic.Bool
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Writer = &returnTrackingWriter{ResponseWriter: c.Writer, returned: &returned, lateWrite: &lateWrite}
		c.Next()
		returned.Store(true)
	})
	engine.GET("/stream", func(c *gin.Context) {
		defer setupAgentSSE(c).Close()
		_ = writeAgentSSE(c, "message_start", gin.H{})
		time.Sleep(20 * time.Millisecond)
	})
	server := httptest.NewServer(engine)
	defer server.Close()

	for i := 0; i < 5; i++ {
		resp, err := http.Get(server.URL + "/stream")
		if err != nil {
			t.Fatalf("GET /stream: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		time.Sleep(10 * time.Millisecond)
		if lateWrite.Load() {
			t.Fatal("heartbeat wrote to the response after the handler returned")
		}
		returned.Store(false)
	}
}

func TestAgentSSEWriterDropsFramesAfterClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/stream", nil)
	w := newAgentSSEWriter(context)
	w.Close()
	w.Close()

	if err := w.writeHeartbeat(); err != nil {
		t.Fatalf("writeHeartbeat: %v", err)
	}
	if err := w.WriteEvent("event-1", "message_chunk", gin.H{"answer": "ok"}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if got := recorder.Body.String(); got != "" {
		t.Fatalf("closed writer wrote %q", got)
	}
}
