package consolesync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"github.com/zgiai/zgi/api/internal/observability"
)

const (
	apiPrefix        = "/v1/internal/pre"
	pathHeartbeat    = "/heartbeat"
	pathCommands     = "/commands"
	pathDesiredState = "/desired-state"
)

// Drift is one field where the local value disagrees with the console desired state.
type Drift struct {
	Scope    string `json:"scope"`
	Field    string `json:"field"`
	ScopeID  string `json:"scope_id,omitempty"`
	Desired  string `json:"desired,omitempty"`
	Observed string `json:"observed,omitempty"`
}

// HealthInfo is the coarse health snapshot reported to console.
type HealthInfo struct {
	Status            string `json:"status,omitempty"`
	ConsecutiveErrors int    `json:"consecutive_errors,omitempty"`
	LastError         string `json:"last_error,omitempty"`
}

// HeartbeatRequest is the heartbeat body. Identity never appears here.
type HeartbeatRequest struct {
	AppliedVersion int64      `json:"applied_version"`
	CursorSeq      int64      `json:"cursor_seq"`
	AccountStatus  string     `json:"account_status,omitempty"`
	Version        string     `json:"version,omitempty"`
	Drift          []Drift    `json:"drift,omitempty"`
	Health         HealthInfo `json:"health,omitempty"`
}

// HeartbeatResponse tells pre how to proceed.
type HeartbeatResponse struct {
	ServerTime          time.Time `json:"server_time"`
	NextPollIntervalSec int       `json:"next_poll_interval_sec"`
	DesiredVersion      int64     `json:"desired_version"`
	CredentialStatus    string    `json:"credential_status"`
}

// CommandEnvelope is one command delivered by console.
type CommandEnvelope struct {
	ID             string     `json:"id"`
	Seq            int64      `json:"seq"`
	Type           string     `json:"type"`
	Payload        string     `json:"payload"`
	IdempotencyKey string     `json:"idempotency_key"`
	Priority       string     `json:"priority"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// PullCommandsResponse is the answer to GET /commands.
type PullCommandsResponse struct {
	Commands            []CommandEnvelope `json:"commands"`
	ServerTime          time.Time         `json:"server_time"`
	NextPollIntervalSec int               `json:"next_poll_interval_sec"`
	DesiredVersion      int64             `json:"desired_version"`
}

// AckRequest acknowledges one command.
type AckRequest struct {
	Status string `json:"status"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DesiredStateResponse is the authoritative projection served by console.
type DesiredStateResponse struct {
	Version       int64  `json:"version"`
	AccountStatus string `json:"account_status,omitempty"`
	Quota         string `json:"quota,omitempty"`
}

// APIError is a non-2xx console answer. StatusCode drives the stale/backoff decisions.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "console api error"
	}
	if e.Message == "" {
		return fmt.Sprintf("console api status %d", e.StatusCode)
	}
	return fmt.Sprintf("console api status %d: %s", e.StatusCode, e.Message)
}

// Unauthorized reports whether console rejected the instance credential.
func (e *APIError) Unauthorized() bool {
	return e != nil && (e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden)
}

// Client is the outbound HTTPS client for the control plane. Every request is signed
// with the per-instance credential; console resolves the instance from that signature.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates the control-plane client.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultHTTPTimeout
	}
	return &Client{
		baseURL: baseURL,
		http:    observability.HTTPClient(&http.Client{Timeout: timeout}),
	}
}

// Heartbeat reports the local state and receives the desired version.
func (c *Client) Heartbeat(ctx context.Context, req HeartbeatRequest) (*HeartbeatResponse, error) {
	var out HeartbeatResponse
	if err := c.doJSON(ctx, http.MethodPost, pathHeartbeat, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PullCommands fetches the commands after the given cursor.
func (c *Client) PullCommands(ctx context.Context, afterSeq int64, limit int) (*PullCommandsResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	query := "?after_seq=" + strconv.FormatInt(afterSeq, 10) + "&limit=" + strconv.Itoa(limit)
	var out PullCommandsResponse
	if err := c.doJSON(ctx, http.MethodGet, pathCommands+query, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Ack reports the terminal result of one command.
func (c *Client) Ack(ctx context.Context, commandID string, req AckRequest) error {
	return c.doJSON(ctx, http.MethodPost, pathCommands+"/"+commandID+"/ack", req, nil)
}

// DesiredState fetches the authoritative projection.
func (c *Client) DesiredState(ctx context.Context) (*DesiredStateResponse, error) {
	var out DesiredStateResponse
	if err := c.doJSON(ctx, http.MethodGet, pathDesiredState, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(
	ctx context.Context,
	method, path string,
	payload interface{},
	out interface{},
) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("console sync base url is empty")
	}

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal console sync request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	url := c.baseURL + apiPrefix + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("build console sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The instance credential is the only identity console accepts on this surface.
	consolecred.SignRequest(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call console sync api: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read console sync response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{StatusCode: resp.StatusCode, Message: errorMessage(raw)}
	}

	if out == nil {
		return nil
	}
	var envelope struct {
		Code    interface{}     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode console sync envelope: %w", err)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode console sync data: %w", err)
	}
	return nil
}

func errorMessage(raw []byte) string {
	var envelope struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Message != "" {
		if code, ok := envelope.Code.(string); ok && code != "" {
			return code + ": " + envelope.Message
		}
		return envelope.Message
	}
	return string(raw)
}
