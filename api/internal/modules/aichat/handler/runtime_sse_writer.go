package handler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	runtimeSSEWriterContextKey  = "aichat_runtime_sse_writer"
	runtimeSSEHeartbeatInterval = 15 * time.Second
)

type runtimeSSEWriter struct {
	context *gin.Context
	mu      sync.Mutex
}

func newRuntimeSSEWriter(c *gin.Context) *runtimeSSEWriter {
	return &runtimeSSEWriter{context: c}
}

func (w *runtimeSSEWriter) StartHeartbeat(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(runtimeSSEHeartbeatInterval)
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

func (w *runtimeSSEWriter) WriteEvent(id, event string, data interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeSSERaw(w.context, id, event, data)
}

func (w *runtimeSSEWriter) writeHeartbeat() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := fmt.Fprint(w.context.Writer, ": heartbeat\n\n"); err != nil {
		return err
	}
	w.context.Writer.Flush()
	return nil
}
