package agents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	agentSSEWriterContextKey  = "agent_sse_writer"
	agentSSEHeartbeatInterval = 15 * time.Second
)

type agentSSEWriter struct {
	context *gin.Context
	mu      sync.Mutex
}

func newAgentSSEWriter(c *gin.Context) *agentSSEWriter {
	return &agentSSEWriter{context: c}
}

func (w *agentSSEWriter) StartHeartbeat(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(agentSSEHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.writeHeartbeat(); err != nil {
					return
				}
			}
		}
	}()
}

func (w *agentSSEWriter) WriteEvent(id, event string, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeAgentSSERaw(w.context, id, event, data)
}

func (w *agentSSEWriter) writeHeartbeat() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := fmt.Fprint(w.context.Writer, ": heartbeat\n\n"); err != nil {
		return err
	}
	w.context.Writer.Flush()
	return nil
}
