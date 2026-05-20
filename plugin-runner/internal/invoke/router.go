// Package invoke provides session routing and request management for plugin invocation.
package invoke

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"plugin_runner/internal/protocol"
)

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrRequestNotFound  = errors.New("request not found")
	ErrRequestTimeout   = errors.New("request timeout")
	ErrPluginNotReady   = errors.New("plugin not ready")
	ErrSessionClosed    = errors.New("session closed")
	ErrDuplicateRequest = errors.New("duplicate request id")
)

// WaitMode controls when SendSyncWithMode should return.
type WaitMode string

const (
	WaitModeFirst    WaitMode = "first"
	WaitModeTerminal WaitMode = "terminal"
)

// StreamMode controls how stream messages are handled when waiting for terminal messages.
type StreamMode string

const (
	StreamModeFirst     StreamMode = "first"
	StreamModeAggregate StreamMode = "aggregate"
)

// ResponseHandler is called when a response is received for a request.
type ResponseHandler func(msg *protocol.Message)

// CallbackHandler is called when a plugin requests host capabilities.
type CallbackHandler func(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse

// PendingRequest tracks an in-flight request.
type PendingRequest struct {
	ID        string
	CreatedAt time.Time
	Timeout   time.Duration
	Handler   ResponseHandler
	Done      chan struct{}
}

// Router manages request routing for a single plugin session.
type Router struct {
	sessionID string
	mu        sync.RWMutex
	pending   map[string]*PendingRequest
	writer    func([]byte) error
	callback  CallbackHandler
	ready     bool
	readyCh   chan struct{}
	closed    bool
}

// NewRouter creates a new request router.
func NewRouter(sessionID string, writer func([]byte) error) *Router {
	return &Router{
		sessionID: sessionID,
		pending:   make(map[string]*PendingRequest),
		writer:    writer,
		readyCh:   make(chan struct{}),
	}
}

// SetCallbackHandler sets the handler for plugin callbacks.
func (r *Router) SetCallbackHandler(h CallbackHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callback = h
}

// WaitReady blocks until the plugin signals readiness or context is cancelled.
func (r *Router) WaitReady(ctx context.Context) error {
	r.mu.RLock()
	if r.ready {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	select {
	case <-r.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsReady returns whether the plugin has signaled readiness.
func (r *Router) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// Send sends a request to the plugin and returns immediately.
// Use the handler to receive responses.
func (r *Router) Send(ctx context.Context, req *protocol.Request, timeout time.Duration, handler ResponseHandler) (string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", ErrSessionClosed
	}
	r.mu.Unlock()

	requestID := uuid.NewString()
	msg := protocol.NewRequest(requestID, r.sessionID, req)

	data, err := msg.Encode()
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	pending := &PendingRequest{
		ID:        requestID,
		CreatedAt: time.Now(),
		Timeout:   timeout,
		Handler:   handler,
		Done:      make(chan struct{}),
	}

	r.mu.Lock()
	if _, exists := r.pending[requestID]; exists {
		r.mu.Unlock()
		return "", ErrDuplicateRequest
	}
	r.pending[requestID] = pending
	r.mu.Unlock()

	if err := r.writer(data); err != nil {
		r.mu.Lock()
		delete(r.pending, requestID)
		r.mu.Unlock()
		return "", fmt.Errorf("write request: %w", err)
	}

	// Start timeout watcher
	if timeout > 0 {
		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case <-timer.C:
				r.mu.Lock()
				if p, ok := r.pending[requestID]; ok {
					delete(r.pending, requestID)
					r.mu.Unlock()
					if p.Handler != nil {
						p.Handler(protocol.NewError(requestID, "timeout", ErrRequestTimeout.Error()))
					}
					close(p.Done)
				} else {
					r.mu.Unlock()
				}
			case <-pending.Done:
				// Request completed normally
			}
		}()
	}

	return requestID, nil
}

// SendSync sends a request and blocks until a response is received or timeout.
func (r *Router) SendSync(ctx context.Context, req *protocol.Request, timeout time.Duration) (*protocol.Message, error) {
	return r.SendSyncWithMode(ctx, req, timeout, WaitModeFirst, StreamModeFirst)
}

// SendSyncWithMode sends a request and waits based on wait/stream mode configuration.
func (r *Router) SendSyncWithMode(
	ctx context.Context,
	req *protocol.Request,
	timeout time.Duration,
	waitMode WaitMode,
	streamMode StreamMode,
) (*protocol.Message, error) {
	if normalizeWaitMode(waitMode) == WaitModeTerminal {
		return r.sendSyncTerminal(ctx, req, timeout, normalizeStreamMode(streamMode))
	}

	return r.sendSyncFirst(ctx, req, timeout)
}

func (r *Router) sendSyncFirst(ctx context.Context, req *protocol.Request, timeout time.Duration) (*protocol.Message, error) {
	respCh := make(chan *protocol.Message, 1)

	requestID, err := r.Send(ctx, req, timeout, func(msg *protocol.Message) {
		select {
		case respCh <- msg:
		default:
		}
	})
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	pending := r.pending[requestID]
	r.mu.RUnlock()

	if pending == nil {
		return nil, ErrRequestNotFound
	}

	select {
	case msg := <-respCh:
		return msg, nil
	case <-ctx.Done():
		r.Complete(requestID)
		return nil, ctx.Err()
	case <-pending.Done:
		select {
		case msg := <-respCh:
			return msg, nil
		default:
			return nil, ErrRequestTimeout
		}
	}
}

func (r *Router) sendSyncTerminal(
	ctx context.Context,
	req *protocol.Request,
	timeout time.Duration,
	streamMode StreamMode,
) (*protocol.Message, error) {
	var mu sync.Mutex
	messages := make([]*protocol.Message, 0, 8)

	requestID, err := r.Send(ctx, req, timeout, func(msg *protocol.Message) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	pending := r.pending[requestID]
	r.mu.RUnlock()

	if pending == nil {
		return nil, ErrRequestNotFound
	}

	select {
	case <-ctx.Done():
		r.Complete(requestID)
		return nil, ctx.Err()
	case <-pending.Done:
	}

	mu.Lock()
	collected := append([]*protocol.Message(nil), messages...)
	mu.Unlock()

	if len(collected) == 0 {
		return nil, ErrRequestTimeout
	}

	if streamMode == StreamModeAggregate {
		return aggregateTerminalMessages(collected)
	}

	return collected[0], nil
}

func aggregateTerminalMessages(messages []*protocol.Message) (*protocol.Message, error) {
	var requestID string
	var terminalResult *protocol.Message
	var terminalError *protocol.Message
	var firstStreamChunk *protocol.StreamChunk
	var textBuilder strings.Builder
	streamCount := 0

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if requestID == "" {
			requestID = msg.RequestID
		}

		switch msg.Type {
		case protocol.MessageTypeResult:
			terminalResult = msg
		case protocol.MessageTypeError:
			terminalError = msg
		case protocol.MessageTypeStream:
			chunk, err := protocol.DecodeData[protocol.StreamChunk](msg)
			if err != nil {
				continue
			}
			streamCount++
			if firstStreamChunk == nil {
				firstStreamChunk = chunk
			}

			if chunk.Type != "text" {
				continue
			}

			switch data := chunk.Data.(type) {
			case map[string]any:
				if text, ok := data["text"].(string); ok {
					textBuilder.WriteString(text)
				}
			case string:
				textBuilder.WriteString(data)
			}
		}
	}

	if terminalResult != nil {
		return terminalResult, nil
	}
	if terminalError != nil {
		return terminalError, nil
	}

	if streamCount == 0 {
		return messages[len(messages)-1], nil
	}

	if requestID == "" {
		requestID = "unknown"
	}

	if textBuilder.Len() > 0 {
		return protocol.NewResult(requestID, true, map[string]any{"text": textBuilder.String()}, ""), nil
	}

	if firstStreamChunk != nil {
		if dataMap, ok := firstStreamChunk.Data.(map[string]any); ok {
			return protocol.NewResult(requestID, true, dataMap, ""), nil
		}
		return protocol.NewResult(requestID, true, map[string]any{"data": firstStreamChunk.Data}, ""), nil
	}

	return protocol.NewResult(requestID, true, map[string]any{"stream_count": streamCount}, ""), nil
}

func normalizeWaitMode(mode WaitMode) WaitMode {
	if strings.EqualFold(string(mode), string(WaitModeTerminal)) {
		return WaitModeTerminal
	}
	return WaitModeFirst
}

func normalizeStreamMode(mode StreamMode) StreamMode {
	if strings.EqualFold(string(mode), string(StreamModeAggregate)) {
		return StreamModeAggregate
	}
	return StreamModeFirst
}

// HandleMessage processes an incoming message from the plugin.
func (r *Router) HandleMessage(msg *protocol.Message) error {
	switch msg.Type {
	case protocol.MessageTypeReady:
		r.mu.Lock()
		if !r.ready {
			r.ready = true
			close(r.readyCh)
		}
		r.mu.Unlock()
		return nil

	case protocol.MessageTypeResult, protocol.MessageTypeStream, protocol.MessageTypeError, protocol.MessageTypeEnd:
		r.mu.RLock()
		pending, ok := r.pending[msg.RequestID]
		r.mu.RUnlock()

		if !ok {
			// Request already completed or unknown
			return nil
		}

		if pending.Handler != nil {
			pending.Handler(msg)
		}

		// Complete the request on result, error, or end
		if msg.Type == protocol.MessageTypeResult || msg.Type == protocol.MessageTypeError || msg.Type == protocol.MessageTypeEnd {
			r.Complete(msg.RequestID)
		}
		return nil

	case protocol.MessageTypeCallback:
		return r.handleCallback(msg)

	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// handleCallback processes a plugin callback request.
func (r *Router) handleCallback(msg *protocol.Message) error {
	r.mu.RLock()
	handler := r.callback
	r.mu.RUnlock()

	if handler == nil {
		// No callback handler, send error response
		resp := &protocol.Message{
			Type:      protocol.MessageTypeResponse,
			RequestID: msg.RequestID,
			Timestamp: time.Now(),
			Data: protocol.CallbackResponse{
				Success: false,
				Error:   "callback handler not configured",
			},
		}
		data, _ := resp.Encode()
		return r.writer(data)
	}

	// Decode callback request
	cbReq, err := protocol.DecodeData[protocol.CallbackRequest](msg)
	if err != nil {
		resp := &protocol.Message{
			Type:      protocol.MessageTypeResponse,
			RequestID: msg.RequestID,
			Timestamp: time.Now(),
			Data: protocol.CallbackResponse{
				Success: false,
				Error:   fmt.Sprintf("decode callback request: %v", err),
			},
		}
		data, _ := resp.Encode()
		return r.writer(data)
	}

	// Execute callback in goroutine to not block message processing
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cbResp := handler(ctx, cbReq)
		if cbResp == nil {
			cbResp = &protocol.CallbackResponse{
				Success: false,
				Error:   "callback returned nil",
			}
		}

		resp := &protocol.Message{
			Type:      protocol.MessageTypeResponse,
			RequestID: msg.RequestID,
			Timestamp: time.Now(),
			Data:      cbResp,
		}
		data, _ := resp.Encode()
		_ = r.writer(data)
	}()

	return nil
}

// Complete marks a request as completed and removes it from pending.
func (r *Router) Complete(requestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pending, ok := r.pending[requestID]; ok {
		delete(r.pending, requestID)
		select {
		case <-pending.Done:
		default:
			close(pending.Done)
		}
	}
}

// Close closes the router and cancels all pending requests.
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	for id, pending := range r.pending {
		delete(r.pending, id)
		if pending.Handler != nil {
			pending.Handler(protocol.NewError(id, "session_closed", ErrSessionClosed.Error()))
		}
		select {
		case <-pending.Done:
		default:
			close(pending.Done)
		}
	}
}

// PendingCount returns the number of pending requests.
func (r *Router) PendingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pending)
}
