// Package platformcli implements the sandbox-only CLI transport. The client
// connects to a Unix socket; package proxies and arbitrary API URLs are unused.
package platformcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/platformrelay"
)

type Client struct {
	client    *http.Client
	transport *http.Transport
	jobID     string
	token     string
}

// NewClient accepts only an absolute Unix socket and an ephemeral Job grant.
// The fixed HTTP authority is protocol framing, not a DNS/network destination.
func NewClient(socket, jobID, token string) (*Client, error) {
	if !filepath.IsAbs(socket) || filepath.Clean(socket) != socket || strings.ContainsRune(socket, '\x00') || socket == "/" || jobID == "" || len(jobID) > 128 || strings.ContainsAny(jobID, "\r\n\x00") || len(token) != 43 || strings.ContainsAny(token, "\r\n\x00") {
		return nil, errors.New("platform CLI session is unavailable")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socket)
	}}
	return &Client{
		jobID: jobID, token: token, transport: transport,
		client: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (c *Client) Close() { c.transport.CloseIdleConnections() }

// Call queues an operation once, then polls only its receipt. A transport error
// is returned to the caller; there is no automatic resubmission or new ID.
func (c *Client) Call(ctx context.Context, request agentexecution.PlatformRequest) (agentexecution.PlatformResponse, error) {
	response, err := c.request(ctx, http.MethodPost, "/v1/requests", request)
	if err != nil {
		return agentexecution.PlatformResponse{}, err
	}
	if response.ID != request.ID {
		return agentexecution.PlatformResponse{}, errors.New("platform CLI response correlation is invalid")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for response.Status == "queued" || response.Status == "running" {
		select {
		case <-ctx.Done():
			return agentexecution.PlatformResponse{}, ctx.Err()
		case <-ticker.C:
		}
		response, err = c.Status(ctx, request.ID)
		if err != nil {
			return agentexecution.PlatformResponse{}, err
		}
	}
	return response, nil
}

func (c *Client) Status(ctx context.Context, id string) (agentexecution.PlatformResponse, error) {
	response, err := c.request(ctx, http.MethodGet, "/v1/requests/"+url.PathEscape(id), nil)
	if err == nil && response.ID != id {
		return agentexecution.PlatformResponse{}, errors.New("platform CLI response correlation is invalid")
	}
	return response, err
}

func (c *Client) request(ctx context.Context, method, path string, input any) (agentexecution.PlatformResponse, error) {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || len(encoded) > platformrelay.MaxRequestBytes+1024 {
			return agentexecution.PlatformResponse{}, errors.New("platform CLI request is invalid or too large")
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://runtime-platform.local"+path, body)
	if err != nil {
		return agentexecution.PlatformResponse{}, errors.New("platform CLI request is invalid")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-ZGI-Job-ID", c.jobID)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return agentexecution.PlatformResponse{}, ctx.Err()
		}
		return agentexecution.PlatformResponse{}, errors.New("platform CLI control channel is unavailable; do not automatically replay an uncertain operation")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agentexecution.PlatformResponse{}, fmt.Errorf("platform CLI request rejected (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, platformrelay.MaxResponseBytes+1025))
	if err != nil || len(data) > platformrelay.MaxResponseBytes+1024 {
		return agentexecution.PlatformResponse{}, errors.New("platform CLI response is invalid or too large")
	}
	var result agentexecution.PlatformResponse
	if err := json.Unmarshal(data, &result); err != nil || result.ID == "" ||
		(result.Status != "queued" && result.Status != "running" && result.Status != "completed" && result.Status != "awaiting_approval" && result.Status != "failed") {
		return agentexecution.PlatformResponse{}, errors.New("platform CLI response is invalid")
	}
	return result, nil
}
