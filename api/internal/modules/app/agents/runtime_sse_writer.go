package agents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const agentSSEWriterContextKey = "agent_sse_writer"

// agentSSEHeartbeatInterval is a variable so tests can shorten it.
var agentSSEHeartbeatInterval = 15 * time.Second

// agentSSEWriter serializes SSE frames for one request. It must be closed
// before the handler returns: net/http releases the response buffer before it
// cancels the request context, so a heartbeat that only watched the context
// could flush a released writer and crash the process.
type agentSSEWriter struct {
	context           *gin.Context
	heartbeatInterval time.Duration
	mu                sync.Mutex
	closed            bool
	done              chan struct{}
}

func newAgentSSEWriter(c *gin.Context) *agentSSEWriter {
	return &agentSSEWriter{context: c, heartbeatInterval: agentSSEHeartbeatInterval, done: make(chan struct{})}
}

func (w *agentSSEWriter) StartHeartbeat(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(w.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-ticker.C:
				if err := w.writeHeartbeat(); err != nil {
					return
				}
			}
		}
	}()
}

// Close stops the heartbeat; frames written after Close are dropped.
func (w *agentSSEWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	close(w.done)
}

func (w *agentSSEWriter) WriteEvent(id, event string, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	return writeAgentSSERaw(w.context, id, event, data)
}

func (w *agentSSEWriter) writeHeartbeat() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	if _, err := fmt.Fprint(w.context.Writer, ": heartbeat\n\n"); err != nil {
		return err
	}
	w.context.Writer.Flush()
	return nil
}
