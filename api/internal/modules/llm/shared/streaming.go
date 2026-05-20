package shared

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// StreamDoneSentinel is the OpenAI-compatible SSE completion marker.
	StreamDoneSentinel = "[DONE]"
	// ChatCompletionChunkObject is the object type for chat completion stream chunks.
	ChatCompletionChunkObject = "chat.completion.chunk"
)

// StreamWriter writes OpenAI-compatible SSE chunks without changing internal
// stream response serialization.
type StreamWriter struct {
	context *gin.Context
	flusher http.Flusher
}

// RawEventStreamWriter writes provider-native SSE events without reshaping data.
type RawEventStreamWriter struct {
	context *gin.Context
	flusher http.Flusher
}

// NewStreamWriter prepares the response for Server-Sent Events.
func NewStreamWriter(c *gin.Context) (*StreamWriter, error) {
	if c == nil || c.Writer == nil {
		return nil, errors.New("streaming context is not ready")
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	return &StreamWriter{
		context: c,
		flusher: flusher,
	}, nil
}

// NewRawEventStreamWriter prepares the response for provider-native SSE events.
func NewRawEventStreamWriter(c *gin.Context) (*RawEventStreamWriter, error) {
	if c == nil || c.Writer == nil {
		return nil, errors.New("streaming context is not ready")
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	return &RawEventStreamWriter{
		context: c,
		flusher: flusher,
	}, nil
}

// WriteError writes a stream error event and flushes it immediately.
func (w *StreamWriter) WriteError(err error) {
	w.context.SSEvent("error", err.Error())
	w.flusher.Flush()
}

// WriteObject writes a JSON stream chunk with OpenAI-compatible choices shape.
func (w *StreamWriter) WriteObject(resp adapter.StreamResponse) {
	resp = normalizeStreamChunk(resp)
	w.context.SSEvent("message", resp)
	w.flusher.Flush()
}

// WriteFinalUsage writes a valid usage-only stream chunk from an internal done frame.
func (w *StreamWriter) WriteFinalUsage(resp adapter.StreamResponse, model string) {
	if resp.Usage == nil {
		return
	}

	w.context.SSEvent("message", usageStreamChunk(resp, model))
	w.flusher.Flush()
}

// WriteDone writes the OpenAI-compatible SSE completion marker.
func (w *StreamWriter) WriteDone() {
	w.context.SSEvent("message", StreamDoneSentinel)
	w.flusher.Flush()
}

// WriteEvent writes a raw SSE event using the upstream protocol event name.
func (w *RawEventStreamWriter) WriteEvent(event adapter.RawStreamEvent) error {
	if event.Done {
		return nil
	}
	eventName := strings.TrimSpace(event.Event)
	if eventName == "" {
		eventName = "message"
	}
	if event.Data == nil {
		return nil
	}

	if _, err := fmt.Fprintf(w.context.Writer, "event:%s\n", eventName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.context.Writer, "data:%s\n\n", strings.TrimSpace(string(event.Data))); err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}

// WriteRawError writes a provider-native error event.
func (w *RawEventStreamWriter) WriteRawError(data []byte) error {
	if _, err := fmt.Fprint(w.context.Writer, "event:error\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.context.Writer, "data:%s\n\n", strings.TrimSpace(string(data))); err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}

func normalizeStreamChunk(resp adapter.StreamResponse) adapter.StreamResponse {
	if resp.Choices == nil {
		resp.Choices = []adapter.StreamChoice{}
	}
	return resp
}

func usageStreamChunk(resp adapter.StreamResponse, model string) adapter.StreamResponse {
	chunk := adapter.StreamResponse{
		ID:      resp.ID,
		Object:  resp.Object,
		Created: resp.Created,
		Model:   resp.Model,
		Choices: []adapter.StreamChoice{},
		Usage:   resp.Usage,
	}
	if chunk.Object == "" {
		chunk.Object = ChatCompletionChunkObject
	}
	if chunk.Model == "" {
		chunk.Model = model
	}
	return chunk
}
