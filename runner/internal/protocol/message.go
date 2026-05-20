// Package protocol defines the JSON message format for plugin communication.
// This protocol enables bidirectional communication between the host and plugins
// via stdin/stdout.
package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType defines the type of message being sent.
type MessageType string

const (
	// Host -> Plugin
	MessageTypeRequest  MessageType = "request"  // Invoke a plugin capability
	MessageTypeResponse MessageType = "response" // Response to plugin callback

	// Plugin -> Host
	MessageTypeResult   MessageType = "result"   // Result of a request
	MessageTypeStream   MessageType = "stream"   // Streaming result chunk
	MessageTypeCallback MessageType = "callback" // Plugin requests host capability
	MessageTypeError    MessageType = "error"    // Error response
	MessageTypeEnd      MessageType = "end"      // End of stream
	MessageTypeReady    MessageType = "ready"    // Plugin is ready to receive requests
)

// Message is the envelope for all plugin communication.
type Message struct {
	Type      MessageType `json:"type"`
	RequestID string      `json:"request_id"`
	SessionID string      `json:"session_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Data      any         `json:"data,omitempty"`
}

// Request represents a request to invoke a plugin capability.
type Request struct {
	Action     string         `json:"action"`     // e.g., "tool.invoke", "model.invoke"
	Provider   string         `json:"provider"`   // Provider name
	Name       string         `json:"name"`       // Tool/Model name
	Parameters map[string]any `json:"parameters"` // Input parameters
	Timeout    int            `json:"timeout"`    // Timeout in seconds
}

// Result represents a single result from the plugin.
type Result struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StreamChunk represents a streaming response chunk.
type StreamChunk struct {
	Index int    `json:"index"`
	Type  string `json:"type"` // "text", "json", "blob", etc.
	Data  any    `json:"data"`
	Done  bool   `json:"done"`
}

// CallbackRequest represents a plugin requesting host capabilities.
type CallbackRequest struct {
	Type       string         `json:"type"`       // e.g., "llm", "storage", "http"
	Action     string         `json:"action"`     // e.g., "invoke", "get", "set"
	Parameters map[string]any `json:"parameters"` // Request parameters
}

// CallbackResponse represents the host's response to a callback.
type CallbackResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ErrorInfo represents error details.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// NewRequest creates a new request message.
func NewRequest(requestID, sessionID string, req *Request) *Message {
	return &Message{
		Type:      MessageTypeRequest,
		RequestID: requestID,
		SessionID: sessionID,
		Timestamp: time.Now(),
		Data:      req,
	}
}

// NewResult creates a new result message.
func NewResult(requestID string, success bool, data any, errMsg string) *Message {
	return &Message{
		Type:      MessageTypeResult,
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: Result{
			Success: success,
			Data:    data,
			Error:   errMsg,
		},
	}
}

// NewStreamChunk creates a new stream chunk message.
func NewStreamChunk(requestID string, index int, chunkType string, data any, done bool) *Message {
	return &Message{
		Type:      MessageTypeStream,
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: StreamChunk{
			Index: index,
			Type:  chunkType,
			Data:  data,
			Done:  done,
		},
	}
}

// NewError creates a new error message.
func NewError(requestID string, code, message string) *Message {
	return &Message{
		Type:      MessageTypeError,
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: ErrorInfo{
			Code:    code,
			Message: message,
		},
	}
}

// NewEnd creates an end-of-stream message.
func NewEnd(requestID string) *Message {
	return &Message{
		Type:      MessageTypeEnd,
		RequestID: requestID,
		Timestamp: time.Now(),
	}
}

// Encode serializes the message to JSON with a newline terminator.
func (m *Message) Encode() ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}
	return append(b, '\n'), nil
}

// Decode parses a JSON message.
func Decode(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	return &m, nil
}

// DecodeData extracts the typed data from a message.
func DecodeData[T any](m *Message) (*T, error) {
	b, err := json.Marshal(m.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return &v, nil
}
