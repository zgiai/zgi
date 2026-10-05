package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const agentExecutionResponseLimit = 2 << 20
const agentExecutionFileLimit = 64 << 20
const agentExecutionMaxBindingReplacements = 16

type agentExecutionScope struct {
	OrganizationID string
	SubjectID      string
}

type agentExecutionBinding struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Generation    int64  `json:"generation"`
	NetworkPolicy string `json:"network_policy"`
	NetworkMode   string `json:"network_mode"`
	Recreated     bool   `json:"-"`
}

type agentExecutionJob struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	ExitCode   *int   `json:"exit_code"`
}

type agentExecutionOutputChunk struct {
	Sequence int64  `json:"sequence"`
	Stream   string `json:"stream"`
	Data     []byte `json:"data"`
}

type agentExecutionCapabilities struct {
	Shell                     bool   `json:"shell"`
	FileTransfer              bool   `json:"file_transfer"`
	DependencyPreparation     bool   `json:"dependency_preparation"`
	DependencyPreparationMode string `json:"dependency_preparation_mode"`
	NetworkMode               string `json:"network_mode"`
	EgressProxyAddress        string `json:"egress_proxy_address,omitempty"`
	MaxUploadBytes            int64  `json:"max_upload_bytes"`
	MaxDownloadBytes          int64  `json:"max_download_bytes"`
}

type agentExecutionGateway interface {
	CreateBinding(context.Context, agentExecutionScope, string, string, string, string, ...string) (agentExecutionBinding, error)
	GetBinding(context.Context, agentExecutionScope, string) (agentExecutionBinding, error)
	ResumeBinding(context.Context, agentExecutionScope, string, int64) (agentExecutionBinding, error)
	Capabilities(context.Context, agentExecutionScope, string) (agentExecutionCapabilities, error)
	SubmitJob(context.Context, agentExecutionScope, string, string, string, []string, time.Duration) (agentExecutionJob, error)
	GetJob(context.Context, agentExecutionScope, string) (agentExecutionJob, error)
	CancelJob(context.Context, agentExecutionScope, string, int64) error
	JobOutput(context.Context, agentExecutionScope, string, int64, int) ([]agentExecutionOutputChunk, error)
	UploadFile(context.Context, agentExecutionScope, string, string, []byte) error
	DownloadFile(context.Context, agentExecutionScope, string, string) ([]byte, error)
}

type agentExecutionHTTPGateway struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type agentExecutionGatewayHTTPError struct {
	statusCode int
	code       string
}

func (e agentExecutionGatewayHTTPError) Error() string {
	return fmt.Sprintf("agent execution gateway returned HTTP %d (%s)", e.statusCode, e.code)
}

func agentExecutionGatewayFromEnv() agentExecutionGateway {
	baseURL := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_URL"))
	apiKey := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_API_KEY"))
	if !validAgentRuntimeGatewayURL(baseURL) || apiKey == "" {
		return nil
	}
	return &agentExecutionHTTPGateway{
		baseURL: baseURL, apiKey: apiKey,
		client: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (g *agentExecutionHTTPGateway) request(ctx context.Context, scope agentExecutionScope, method, path string, input any, output any) error {
	if g == nil || g.client == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return fmt.Errorf("agent execution gateway is unavailable")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode agent execution request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+"/v1/agent-runtime/"+path, body)
	if err != nil {
		return fmt.Errorf("create agent execution request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("X-ZGI-Organization-ID", scope.OrganizationID)
	req.Header.Set("X-ZGI-Subject-ID", scope.SubjectID)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("agent execution gateway request: %w", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, agentExecutionResponseLimit+1)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode agent execution response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentExecutionGatewayHTTPError{statusCode: resp.StatusCode, code: envelope.Error.Code}
	}
	if output == nil {
		return nil
	}
	if len(envelope.Data) == 0 || len(envelope.Data) > agentExecutionResponseLimit {
		return fmt.Errorf("agent execution gateway response is missing or too large")
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode agent execution result: %w", err)
	}
	return nil
}

func (g *agentExecutionHTTPGateway) CreateBinding(ctx context.Context, scope agentExecutionScope, agentID, conversationID, profileID, backend string, homeSnapshotIDs ...string) (agentExecutionBinding, error) {
	key := "conversation:" + conversationID
	homeSnapshotID := ""
	if len(homeSnapshotIDs) > 0 {
		homeSnapshotID = strings.TrimSpace(homeSnapshotIDs[0])
	}
	networkPolicy := "offline"
	if len(homeSnapshotIDs) > 1 && strings.TrimSpace(homeSnapshotIDs[1]) != "" {
		networkPolicy = strings.TrimSpace(homeSnapshotIDs[1])
	}
	seen := make(map[string]struct{})
	for attempt := 0; attempt <= agentExecutionMaxBindingReplacements; attempt++ {
		var result agentExecutionBinding
		err := g.request(ctx, scope, http.MethodPost, "bindings", map[string]any{
			"agent_id": agentID, "conversation_id": conversationID, "profile_id": profileID,
			"backend": backend, "idempotency_key": key, "home_snapshot_id": homeSnapshotID,
			"network_policy": networkPolicy,
		}, &result)
		var gatewayErr agentExecutionGatewayHTTPError
		if errors.As(err, &gatewayErr) && gatewayErr.statusCode == http.StatusConflict && gatewayErr.code == "idempotency_conflict" {
			// A published Agent configuration may change while an older
			// conversation remains pinned to its original runtime template.
			// Read that owner-scoped Binding instead of silently replacing it.
			err = g.request(ctx, scope, http.MethodGet, "bindings/by-idempotency?key="+url.QueryEscape(key), nil, &result)
		}
		if err != nil {
			return agentExecutionBinding{}, err
		}
		if result.ID == "" {
			return agentExecutionBinding{}, fmt.Errorf("agent execution binding response has no id")
		}
		if result.Status != "deleted" && result.Status != "failed" {
			result.Recreated = attempt > 0
			return result, nil
		}
		if _, exists := seen[result.ID]; exists {
			return agentExecutionBinding{}, fmt.Errorf("agent execution binding replacement cycle")
		}
		seen[result.ID] = struct{}{}
		key = "conversation:" + conversationID + ":after:" + result.ID
	}
	return agentExecutionBinding{}, fmt.Errorf("agent execution binding replacement limit exceeded")
}

func (g *agentExecutionHTTPGateway) GetBinding(ctx context.Context, scope agentExecutionScope, bindingID string) (agentExecutionBinding, error) {
	var result agentExecutionBinding
	err := g.request(ctx, scope, http.MethodGet, "bindings/"+url.PathEscape(bindingID), nil, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) ResumeBinding(ctx context.Context, scope agentExecutionScope, bindingID string, generation int64) (agentExecutionBinding, error) {
	var result agentExecutionBinding
	err := g.request(ctx, scope, http.MethodPost, "bindings/"+url.PathEscape(bindingID)+"/resume", map[string]any{"generation": generation}, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) Capabilities(ctx context.Context, scope agentExecutionScope, bindingID string) (agentExecutionCapabilities, error) {
	var result agentExecutionCapabilities
	err := g.request(ctx, scope, http.MethodGet, "bindings/"+url.PathEscape(bindingID)+"/capabilities", nil, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) SubmitJob(ctx context.Context, scope agentExecutionScope, bindingID, idempotencyKey, command string, args []string, timeout time.Duration) (agentExecutionJob, error) {
	var result agentExecutionJob
	err := g.request(ctx, scope, http.MethodPost, "bindings/"+url.PathEscape(bindingID)+"/jobs", map[string]any{
		"idempotency_key": idempotencyKey, "command": command, "args": args,
		"timeout_ms": timeout.Milliseconds(),
	}, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) GetJob(ctx context.Context, scope agentExecutionScope, jobID string) (agentExecutionJob, error) {
	var result agentExecutionJob
	err := g.request(ctx, scope, http.MethodGet, "jobs/"+url.PathEscape(jobID), nil, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) CancelJob(ctx context.Context, scope agentExecutionScope, jobID string, generation int64) error {
	return g.request(ctx, scope, http.MethodPost, "jobs/"+url.PathEscape(jobID)+"/cancel", map[string]any{"generation": generation}, nil)
}

func (g *agentExecutionHTTPGateway) JobOutput(ctx context.Context, scope agentExecutionScope, jobID string, after int64, limit int) ([]agentExecutionOutputChunk, error) {
	var result []agentExecutionOutputChunk
	path := "jobs/" + url.PathEscape(jobID) + "/output?after=" + strconv.FormatInt(after, 10) + "&limit=" + strconv.Itoa(limit)
	err := g.request(ctx, scope, http.MethodGet, path, nil, &result)
	return result, err
}

func (g *agentExecutionHTTPGateway) UploadFile(ctx context.Context, scope agentExecutionScope, bindingID, filePath string, data []byte) error {
	if g == nil || g.client == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return fmt.Errorf("agent execution gateway is unavailable")
	}
	endpoint := g.baseURL + "/v1/agent-runtime/bindings/" + url.PathEscape(bindingID) + "/files?path=" + url.QueryEscape(filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create agent execution upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("X-ZGI-Organization-ID", scope.OrganizationID)
	req.Header.Set("X-ZGI-Subject-ID", scope.SubjectID)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("agent execution upload request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("agent execution upload returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (g *agentExecutionHTTPGateway) DownloadFile(ctx context.Context, scope agentExecutionScope, bindingID, filePath string) ([]byte, error) {
	if g == nil || g.client == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return nil, fmt.Errorf("agent execution gateway is unavailable")
	}
	endpoint := g.baseURL + "/v1/agent-runtime/bindings/" + url.PathEscape(bindingID) + "/files?path=" + url.QueryEscape(filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent execution download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("X-ZGI-Organization-ID", scope.OrganizationID)
	req.Header.Set("X-ZGI-Subject-ID", scope.SubjectID)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent execution download request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent execution download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength < 0 || resp.ContentLength > agentExecutionFileLimit {
		return nil, fmt.Errorf("agent execution download size is missing or exceeds the limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, agentExecutionFileLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read agent execution download: %w", err)
	}
	if int64(len(data)) != resp.ContentLength || len(data) > agentExecutionFileLimit {
		return nil, fmt.Errorf("agent execution download size changed")
	}
	return data, nil
}
