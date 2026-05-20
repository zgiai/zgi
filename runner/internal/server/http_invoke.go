package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/runner/internal/protocol"
)

// InvokeRequest is the payload for invoke API calls.
type InvokeRequest struct {
	SessionID  string         `json:"session_id" binding:"required"`
	Action     string         `json:"action" binding:"required"`
	Provider   string         `json:"provider"`
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters"`
	Timeout    int            `json:"timeout"` // seconds
	WaitMode   string         `json:"wait_mode,omitempty"`
	StreamMode string         `json:"stream_mode,omitempty"`
}

// InvokeResponse wraps the plugin response.
type InvokeResponse struct {
	RequestID string `json:"request_id"`
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

// registerInvokeRoutes adds invoke-related endpoints.
func (s *HTTPServer) registerInvokeRoutes(v1 *gin.RouterGroup) {
	invoke := v1.Group("/invoke")
	{
		invoke.POST("", s.handleInvoke)
		invoke.POST("/tool", s.handleInvokeTool)
		invoke.GET("/sessions/:id/ready", s.handleCheckReady)
	}
}

// handleInvoke handles generic plugin invocation.
func (s *HTTPServer) handleInvoke(c *gin.Context) {
	var req InvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	session, ok := s.mgr.GetSession(req.SessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("session %s not found", req.SessionID)})
		return
	}

	snap := session.Snapshot()
	if snap.Status != "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("session %s is not running (status: %s)", req.SessionID, snap.Status)})
		return
	}

	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	protoReq := &protocol.Request{
		Action:     req.Action,
		Provider:   req.Provider,
		Name:       req.Name,
		Parameters: req.Parameters,
		Timeout:    req.Timeout,
	}

	session.TouchActivity()
	msg, err := session.SendRequestWithMode(ctx, protoReq, timeout, req.WaitMode, req.StreamMode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := InvokeResponse{
		RequestID: msg.RequestID,
	}

	switch msg.Type {
	case protocol.MessageTypeResult:
		result, err := protocol.DecodeData[protocol.Result](msg)
		if err != nil {
			resp.Success = false
			resp.Error = fmt.Sprintf("decode result: %v", err)
		} else {
			resp.Success = result.Success
			resp.Data = result.Data
			resp.Error = result.Error
		}
	case protocol.MessageTypeStream:
		// Handle stream message - extract data from stream chunk
		chunk, err := protocol.DecodeData[protocol.StreamChunk](msg)
		if err != nil {
			resp.Success = false
			resp.Error = fmt.Sprintf("decode stream chunk: %v", err)
		} else {
			resp.Success = true
			resp.Data = chunk.Data
		}
	case protocol.MessageTypeError:
		errInfo, _ := protocol.DecodeData[protocol.ErrorInfo](msg)
		resp.Success = false
		if errInfo != nil {
			resp.Error = errInfo.Message
		} else {
			resp.Error = "unknown error"
		}
	case protocol.MessageTypeEnd:
		resp.Success = true
		resp.Data = nil
	default:
		resp.Success = false
		resp.Error = fmt.Sprintf("unexpected message type: %s", msg.Type)
	}

	c.JSON(http.StatusOK, resp)
}

// ToolInvokeRequest is a convenience wrapper for tool invocation.
type ToolInvokeRequest struct {
	SessionID  string         `json:"session_id" binding:"required"`
	Provider   string         `json:"provider" binding:"required"`
	Tool       string         `json:"tool" binding:"required"`
	Parameters map[string]any `json:"parameters"`
	Timeout    int            `json:"timeout"` // seconds
	WaitMode   string         `json:"wait_mode,omitempty"`
	StreamMode string         `json:"stream_mode,omitempty"`
}

// handleInvokeTool handles tool-specific invocation.
func (s *HTTPServer) handleInvokeTool(c *gin.Context) {
	var req ToolInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	// Convert to generic invoke request
	invokeReq := InvokeRequest{
		SessionID:  req.SessionID,
		Action:     "tool.invoke",
		Provider:   req.Provider,
		Name:       req.Tool,
		Parameters: req.Parameters,
		Timeout:    req.Timeout,
		WaitMode:   req.WaitMode,
		StreamMode: req.StreamMode,
	}

	session, ok := s.mgr.GetSession(invokeReq.SessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("session %s not found", invokeReq.SessionID)})
		return
	}

	snap := session.Snapshot()
	if snap.Status != "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("session %s is not running (status: %s)", invokeReq.SessionID, snap.Status)})
		return
	}

	timeout := time.Duration(invokeReq.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	protoReq := &protocol.Request{
		Action:     invokeReq.Action,
		Provider:   invokeReq.Provider,
		Name:       invokeReq.Name,
		Parameters: invokeReq.Parameters,
		Timeout:    invokeReq.Timeout,
	}

	session.TouchActivity()
	msg, err := session.SendRequestWithMode(ctx, protoReq, timeout, invokeReq.WaitMode, invokeReq.StreamMode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := InvokeResponse{
		RequestID: msg.RequestID,
	}

	switch msg.Type {
	case protocol.MessageTypeResult:
		result, err := protocol.DecodeData[protocol.Result](msg)
		if err != nil {
			resp.Success = false
			resp.Error = fmt.Sprintf("decode result: %v", err)
		} else {
			resp.Success = result.Success
			resp.Data = result.Data
			resp.Error = result.Error
		}
	case protocol.MessageTypeStream:
		// Handle stream message - extract data from stream chunk
		chunk, err := protocol.DecodeData[protocol.StreamChunk](msg)
		if err != nil {
			resp.Success = false
			resp.Error = fmt.Sprintf("decode stream chunk: %v", err)
		} else {
			resp.Success = true
			resp.Data = chunk.Data
		}
	case protocol.MessageTypeError:
		errInfo, _ := protocol.DecodeData[protocol.ErrorInfo](msg)
		resp.Success = false
		if errInfo != nil {
			resp.Error = errInfo.Message
		} else {
			resp.Error = "unknown error"
		}
	case protocol.MessageTypeEnd:
		resp.Success = true
		resp.Data = nil
	default:
		resp.Success = false
		resp.Error = fmt.Sprintf("unexpected message type: %s", msg.Type)
	}

	c.JSON(http.StatusOK, resp)
}

// handleCheckReady checks if a plugin session is ready to receive requests.
func (s *HTTPServer) handleCheckReady(c *gin.Context) {
	id := c.Param("id")
	session, ok := s.mgr.GetSession(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("session %s not found", id)})
		return
	}

	ready := session.IsReady()
	status := session.Snapshot().Status

	c.JSON(http.StatusOK, gin.H{
		"session_id": id,
		"ready":      ready,
		"status":     status,
	})
}
