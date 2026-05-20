package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
)

// ============================================
// Session Management API
// ============================================

// StartSession starts a new plugin session
func (c *Client) StartSession(ctx context.Context, req model.StartSessionRequest, opts ...RequestOption) (*model.Session, error) {
	var result model.Session
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/sessions", req, &result, opts...); err != nil {
		return nil, fmt.Errorf("start session failed: %w", err)
	}
	return &result, nil
}

// ListSessions returns all active sessions
func (c *Client) ListSessions(ctx context.Context, opts ...RequestOption) ([]model.Session, error) {
	var result []model.Session
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/sessions", nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("list sessions failed: %w", err)
	}
	return result, nil
}

// GetSession returns the details of a specific session
func (c *Client) GetSession(ctx context.Context, sessionID string, opts ...RequestOption) (*model.Session, error) {
	var result model.Session
	path := fmt.Sprintf("/api/v1/sessions/%s", sessionID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("get session failed: %w", err)
	}
	return &result, nil
}

// StopSession stops a running session
func (c *Client) StopSession(ctx context.Context, sessionID string, opts ...RequestOption) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/stop", sessionID)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, nil, opts...); err != nil {
		return fmt.Errorf("stop session failed: %w", err)
	}
	return nil
}

// IsSessionReady checks if a session is ready to accept requests
func (c *Client) IsSessionReady(ctx context.Context, sessionID string, opts ...RequestOption) (*model.SessionReadyResponse, error) {
	var result model.SessionReadyResponse
	path := fmt.Sprintf("/api/v1/invoke/sessions/%s/ready", sessionID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("check session ready failed: %w", err)
	}
	return &result, nil
}
