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
	"regexp"
	"strconv"
	"strings"
	"time"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

const agentExecutionResponseLimit = 2 << 20
const agentExecutionFileLimit = 64 << 20
const agentExecutionMaxBindingReplacements = 16

var agentExecutionReleaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type agentExecutionScope = execution.Scope

type agentExecutionBinding struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Generation    int64  `json:"generation"`
	NetworkPolicy string `json:"network_policy"`
	NetworkMode   string `json:"network_mode"`
	Recreated     bool   `json:"-"`
}

type agentExecutionJob struct {
	BindingGeneration int64      `json:"binding_generation"`
	BindingID         string     `json:"binding_id"`
	CancelRequested   bool       `json:"cancel_requested"`
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	Generation        int64      `json:"generation"`
	ExitCode          *int       `json:"exit_code"`
	FailureCode       string     `json:"failure_code"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

type agentExecutionOutputChunk struct {
	Sequence int64  `json:"sequence"`
	Stream   string `json:"stream"`
	Data     []byte `json:"data"`
}

type agentExecutionCapabilities struct {
	PlatformProtocolVersion   int    `json:"platform_protocol_version,omitempty"`
	Shell                     bool   `json:"shell"`
	FileTransfer              bool   `json:"file_transfer"`
	DependencyPreparation     bool   `json:"dependency_preparation"`
	DependencyPreparationMode string `json:"dependency_preparation_mode"`
	DependencyInstallation    bool   `json:"dependency_installation"`
	PackageNetworkMode        string `json:"package_network_mode"`
	PackageNetworkEnforced    bool   `json:"package_network_enforced"`
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
	releaseRequired bool
	releaseID       string
	baseURL         string
	apiKey          string
	client          *http.Client
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
	releaseID := strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_RELEASE_ID"))
	required := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AGENT_RUNTIME_RELEASE_REQUIRED")))
	invalidRelease := (required != "" && required != "true" && required != "false") ||
		(releaseID != "" && !agentExecutionReleaseIDPattern.MatchString(releaseID))
	if invalidRelease {
		// Keep recovery transports available, but never emit malformed headers
		// or downgrade new admission when operator configuration is invalid.
		releaseID = ""
	}
	if !validAgentRuntimeGatewayURL(baseURL) || apiKey == "" {
		return nil
	}
	return &agentExecutionHTTPGateway{
		releaseRequired: required == "true" || invalidRelease,
		releaseID:       releaseID,
		baseURL:         baseURL, apiKey: apiKey,
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
	if path != "release" {
		if err := g.verifyExecutionIdentity(ctx, scope); err != nil {
			return err
		}
	}
	if (g.releaseRequired || g.releaseID != "") && method == http.MethodPost && (path == "bindings" || strings.HasSuffix(path, "/resume") || strings.HasSuffix(path, "/jobs")) {
		if g.releaseID == "" {
			return fmt.Errorf("runtime release identity is missing or invalid; new execution is disabled")
		}
		var release struct {
			ID                      string `json:"id"`
			ContractVersion         int    `json:"contract_version"`
			PlatformProtocolVersion int    `json:"platform_protocol_version"`
		}
		if err := g.request(ctx, scope, http.MethodGet, "release", nil, &release); err != nil {
			return fmt.Errorf("runtime release verification failed: %w", err)
		}
		if release.ID != g.releaseID || release.ContractVersion != 1 || release.PlatformProtocolVersion != 1 {
			return fmt.Errorf("API and Worker runtime releases do not match")
		}
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
	req.Header.Set("X-ZGI-Runtime-Release", g.releaseID)
	if err := setExecutionIdentityHeader(req, scope); err != nil {
		return err
	}
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
	return g.uploadRuntimeFile(ctx, scope, "bindings/"+url.PathEscape(bindingID)+"/files?path="+url.QueryEscape(filePath), data)
}

func (g *agentExecutionHTTPGateway) uploadRuntimeFile(ctx context.Context, scope agentExecutionScope, route string, data []byte) error {
	if g == nil || g.client == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return fmt.Errorf("agent execution gateway is unavailable")
	}
	if err := g.verifyExecutionIdentity(ctx, scope); err != nil {
		return err
	}
	endpoint := g.baseURL + "/v1/agent-runtime/" + route
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create agent execution upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("X-ZGI-Organization-ID", scope.OrganizationID)
	req.Header.Set("X-ZGI-Subject-ID", scope.SubjectID)
	req.Header.Set("Content-Type", "application/octet-stream")
	if err := setExecutionIdentityHeader(req, scope); err != nil {
		return err
	}
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
	return g.downloadRuntimeFile(ctx, scope, "bindings/"+url.PathEscape(bindingID)+"/files?path="+url.QueryEscape(filePath))
}

func (g *agentExecutionHTTPGateway) downloadRuntimeFile(ctx context.Context, scope agentExecutionScope, route string) ([]byte, error) {
	if g == nil || g.client == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return nil, fmt.Errorf("agent execution gateway is unavailable")
	}
	if err := g.verifyExecutionIdentity(ctx, scope); err != nil {
		return nil, err
	}
	endpoint := g.baseURL + "/v1/agent-runtime/" + route
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent execution download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("X-ZGI-Organization-ID", scope.OrganizationID)
	req.Header.Set("X-ZGI-Subject-ID", scope.SubjectID)
	if err := setExecutionIdentityHeader(req, scope); err != nil {
		return nil, err
	}
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
