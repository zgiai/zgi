package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type captureKey struct{}
type cursorKey struct{}
type continuationKey struct{}
type rawCacheKey struct{}
type captureSlot struct {
	mu          sync.Mutex
	raw         json.RawMessage
	sent        bool
	boundaryErr error
}

func (s *captureSlot) result() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(json.RawMessage(nil), s.raw...)
}
func (s *captureSlot) markSent()                  { s.mu.Lock(); s.sent = true; s.mu.Unlock() }
func (s *captureSlot) wasSent() bool              { s.mu.Lock(); defer s.mu.Unlock(); return s.sent }
func (s *captureSlot) setBoundaryError(err error) { s.mu.Lock(); s.boundaryErr = err; s.mu.Unlock() }
func (s *captureSlot) boundaryError() error       { s.mu.Lock(); defer s.mu.Unlock(); return s.boundaryErr }

type pendingCall struct {
	method string
	slot   *captureSlot
}
type captureTransport struct {
	base            mcp.Transport
	mu              sync.Mutex
	pending         map[string]pendingCall
	discovery       json.RawMessage
	lastError       error
	onNotification  func(string, json.RawMessage)
	maxMessageBytes int64
}

func (t *captureTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn, err := t.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.pending = make(map[string]pendingCall)
	t.mu.Unlock()
	return &captureConnection{base: conn, owner: t}, nil
}
func (t *captureTransport) discoveryResult() json.RawMessage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append(json.RawMessage(nil), t.discovery...)
}
func (t *captureTransport) setBoundaryError(err error) { t.mu.Lock(); t.lastError = err; t.mu.Unlock() }
func (t *captureTransport) boundaryError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastError
}

type captureConnection struct {
	base  mcp.Connection
	owner *captureTransport
}

func (c *captureConnection) SessionID() string { return c.base.SessionID() }
func (c *captureConnection) Close() error      { return c.base.Close() }
func (c *captureConnection) Write(ctx context.Context, msg jsonrpc.Message) error {
	if req, ok := msg.(*jsonrpc.Request); ok && req.ID.IsValid() {
		var object map[string]json.RawMessage
		modified := false
		if cursor, ok := ctx.Value(cursorKey{}).(*string); ok && cursor != nil {
			if json.Unmarshal(req.Params, &object) == nil {
				encoded, _ := json.Marshal(*cursor)
				object["cursor"] = encoded
				modified = true
			}
		}
		if continuation, ok := ctx.Value(continuationKey{}).(*Continuation); ok && continuation != nil {
			if object == nil {
				_ = json.Unmarshal(req.Params, &object)
			}
			if object != nil {
				if continuation.RequestState.Present {
					object["requestState"] = append(json.RawMessage(nil), continuation.RequestState.JSON...)
				}
				if continuation.InputResponses != nil {
					encoded, _ := json.Marshal(continuation.InputResponses)
					object["inputResponses"] = encoded
				}
				modified = true
			}
		}
		if modified {
			req.Params, _ = json.Marshal(object)
		}
		var slot *captureSlot
		if value, ok := ctx.Value(captureKey{}).(*captureSlot); ok {
			slot = value
		}
		if c.owner.maxMessageBytes > 0 {
			encoded, err := jsonrpc.EncodeMessage(msg)
			if err != nil {
				return err
			}
			if int64(len(encoded)) > c.owner.maxMessageBytes {
				limitErr := &MCPError{Kind: ErrLimitExceeded, Operation: "write stdio message"}
				c.owner.setBoundaryError(limitErr)
				if slot != nil {
					slot.setBoundaryError(limitErr)
				}
				return limitErr
			}
		}
		key := fmt.Sprint(req.ID.Raw())
		c.owner.mu.Lock()
		c.owner.pending[key] = pendingCall{method: req.Method, slot: slot}
		c.owner.mu.Unlock()
		// A write can block after the peer receives the request. Conservatively
		// report a possible remote execution once transport I/O begins.
		if slot != nil {
			slot.markSent()
		}
		if err := c.base.Write(ctx, msg); err != nil {
			c.owner.mu.Lock()
			delete(c.owner.pending, key)
			c.owner.mu.Unlock()
			return err
		}
		return nil
	}
	return c.base.Write(ctx, msg)
}
func (c *captureConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	msg, err := c.base.Read(ctx)
	if err != nil {
		return nil, err
	}
	switch typed := msg.(type) {
	case *jsonrpc.Response:
		if c.owner.maxMessageBytes > 0 && int64(len(typed.Result)) > c.owner.maxMessageBytes {
			limitErr := &MCPError{Kind: ErrLimitExceeded, Operation: "read response"}
			c.owner.setBoundaryError(limitErr)
			return nil, limitErr
		}
		key := fmt.Sprint(typed.ID.Raw())
		c.owner.mu.Lock()
		pending := c.owner.pending[key]
		delete(c.owner.pending, key)
		if pending.method == "server/discover" || pending.method == "initialize" {
			c.owner.discovery = append(json.RawMessage(nil), typed.Result...)
		}
		c.owner.mu.Unlock()
		if pending.slot != nil {
			pending.slot.mu.Lock()
			pending.slot.raw = append(json.RawMessage(nil), typed.Result...)
			pending.slot.mu.Unlock()
		}
		// The SDK caches 2026 list/read results with ttlMs, which would bypass
		// this capture layer on later calls. The public client owns its cache.
		if pending.method == "tools/list" || pending.method == "resources/list" || pending.method == "resources/templates/list" || pending.method == "prompts/list" || pending.method == "resources/read" {
			var object map[string]json.RawMessage
			if json.Unmarshal(typed.Result, &object) == nil {
				if _, ok := object["ttlMs"]; ok {
					delete(object, "ttlMs")
					copy := *typed
					copy.Result, _ = json.Marshal(object)
					msg = &copy
				}
			}
		}
	case *jsonrpc.Request:
		if !typed.ID.IsValid() && c.owner.onNotification != nil {
			c.owner.onNotification(typed.Method, append(json.RawMessage(nil), typed.Params...))
		}
	}
	return msg, nil
}
