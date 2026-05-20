// Package callback provides handlers for plugin callback requests.
// This enables plugins to request capabilities from the host platform.
package callback

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"plugin_runner/internal/protocol"
)

// Handler processes callback requests from plugins.
type Handler struct {
	log        *zap.Logger
	httpClient *http.Client
	handlers   map[string]TypeHandler
}

// TypeHandler processes a specific callback type.
type TypeHandler func(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse

// Config configures the callback handler.
type Config struct {
	HTTPTimeout time.Duration
}

// New creates a new callback handler.
func New(log *zap.Logger, cfg Config) *Handler {
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}

	h := &Handler{
		log: log,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		handlers: make(map[string]TypeHandler),
	}

	// Register built-in handlers
	h.registerBuiltinHandlers()

	return h
}

// registerBuiltinHandlers sets up the default callback handlers.
func (h *Handler) registerBuiltinHandlers() {
	// HTTP callback - allows plugins to make HTTP requests through the host
	h.handlers["http"] = h.handleHTTP

	// Storage callback - simple key-value storage
	h.handlers["storage"] = h.handleStorage

	// Log callback - structured logging through host
	h.handlers["log"] = h.handleLog
}

// RegisterHandler adds a custom callback handler.
func (h *Handler) RegisterHandler(callbackType string, handler TypeHandler) {
	h.handlers[callbackType] = handler
}

// Handle processes a callback request from a plugin.
func (h *Handler) Handle(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse {
	if req == nil {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   "nil callback request",
		}
	}

	handler, ok := h.handlers[req.Type]
	if !ok {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown callback type: %s", req.Type),
		}
	}

	return handler(ctx, req)
}

// handleHTTP processes HTTP callback requests.
func (h *Handler) handleHTTP(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse {
	method, _ := req.Parameters["method"].(string)
	url, _ := req.Parameters["url"].(string)
	body, _ := req.Parameters["body"].(string)
	headers, _ := req.Parameters["headers"].(map[string]any)

	if url == "" {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   "url is required",
		}
	}

	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("create request: %v", err),
		}
	}

	// Set headers
	for k, v := range headers {
		if s, ok := v.(string); ok {
			httpReq.Header.Set(k, s)
		}
	}

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("http request: %v", err),
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("read response: %v", err),
		}
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	return &protocol.CallbackResponse{
		Success: true,
		Data: map[string]any{
			"status_code": resp.StatusCode,
			"headers":     respHeaders,
			"body":        string(respBody),
		},
	}
}

// Simple in-memory storage for demo purposes
var memStorage = make(map[string]any)

// handleStorage processes storage callback requests.
func (h *Handler) handleStorage(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse {
	action := req.Action
	key, _ := req.Parameters["key"].(string)

	if key == "" {
		return &protocol.CallbackResponse{
			Success: false,
			Error:   "key is required",
		}
	}

	switch action {
	case "get":
		value, ok := memStorage[key]
		if !ok {
			return &protocol.CallbackResponse{
				Success: true,
				Data:    nil,
			}
		}
		return &protocol.CallbackResponse{
			Success: true,
			Data:    value,
		}

	case "set":
		value := req.Parameters["value"]
		memStorage[key] = value
		return &protocol.CallbackResponse{
			Success: true,
		}

	case "delete":
		delete(memStorage, key)
		return &protocol.CallbackResponse{
			Success: true,
		}

	default:
		return &protocol.CallbackResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown storage action: %s", action),
		}
	}
}

// handleLog processes log callback requests.
func (h *Handler) handleLog(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse {
	level, _ := req.Parameters["level"].(string)
	message, _ := req.Parameters["message"].(string)
	fields, _ := req.Parameters["fields"].(map[string]any)

	var zapFields []zap.Field
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	switch level {
	case "debug":
		h.log.Debug(message, zapFields...)
	case "info":
		h.log.Info(message, zapFields...)
	case "warn":
		h.log.Warn(message, zapFields...)
	case "error":
		h.log.Error(message, zapFields...)
	default:
		h.log.Info(message, zapFields...)
	}

	return &protocol.CallbackResponse{
		Success: true,
	}
}
