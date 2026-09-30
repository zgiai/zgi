// Package e2b implements the hosted Agent execution backend using E2B's
// current REST control plane and the ZGI Runtime Agent data-plane protocol.
// Provider credentials and short-lived connection tokens are never returned
// in Allocation values and therefore are never persisted by the control plane.
package e2b

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/runtimeagent"
)

const (
	defaultAPIURL         = "https://api.e2b.app"
	defaultDomain         = "e2b.app"
	defaultAgentPort      = 49984
	defaultEnvdPort       = 49983
	defaultTimeoutSeconds = 1800
	agentTokenEnvironment = "ZGI_RUNTIME_AGENT_TOKEN"
)

type Profile struct {
	TemplateID string `json:"template_id"`
}

type Options struct {
	APIURL         string
	Domain         string
	APIKey         string
	HTTPClient     *http.Client
	Profiles       map[string]Profile
	AgentPort      uint16
	EnvdPort       uint16
	TimeoutSeconds int
	ReadyTimeout   time.Duration
}

type Backend struct {
	client         *controlClient
	profiles       map[string]Profile
	agentPort      uint16
	envdPort       uint16
	timeoutSeconds int
	readyTimeout   time.Duration
}

var (
	_ agentexecution.Backend              = (*Backend)(nil)
	_ agentexecution.ExecutionBackend     = (*Backend)(nil)
	_ agentexecution.WorkspaceArchiver    = (*Backend)(nil)
	_ agentexecution.AllocationDiscoverer = (*Backend)(nil)
)

func New(options Options) (*Backend, error) {
	options.APIURL = strings.TrimRight(strings.TrimSpace(options.APIURL), "/")
	if options.APIURL == "" {
		options.APIURL = defaultAPIURL
	}
	options.Domain = strings.TrimSpace(options.Domain)
	if options.Domain == "" {
		options.Domain = defaultDomain
	}
	options.APIKey = strings.TrimSpace(options.APIKey)
	if options.APIKey == "" {
		return nil, fmt.Errorf("%w: E2B API key is required", agentexecution.ErrInvalidRequest)
	}
	if err := validateControlURL(options.APIURL); err != nil {
		return nil, err
	}
	if options.AgentPort == 0 {
		options.AgentPort = defaultAgentPort
	}
	if options.EnvdPort == 0 {
		options.EnvdPort = defaultEnvdPort
	}
	if options.AgentPort == options.EnvdPort {
		return nil, fmt.Errorf("%w: Runtime Agent and envd ports must differ", agentexecution.ErrInvalidRequest)
	}
	if options.TimeoutSeconds == 0 {
		options.TimeoutSeconds = defaultTimeoutSeconds
	}
	if options.TimeoutSeconds < 60 || options.TimeoutSeconds > 86400 {
		return nil, fmt.Errorf("%w: E2B timeout must be between 60 and 86400 seconds", agentexecution.ErrInvalidRequest)
	}
	if options.ReadyTimeout <= 0 {
		options.ReadyTimeout = 30 * time.Second
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	profiles := make(map[string]Profile, len(options.Profiles))
	for name, profile := range options.Profiles {
		name = strings.TrimSpace(name)
		profile.TemplateID = strings.TrimSpace(profile.TemplateID)
		if name == "" || profile.TemplateID == "" {
			return nil, fmt.Errorf("%w: E2B profile names and template IDs are required", agentexecution.ErrInvalidRequest)
		}
		profiles[name] = profile
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("%w: at least one approved E2B profile is required", agentexecution.ErrInvalidRequest)
	}
	client := &controlClient{
		apiURL: options.APIURL, domain: options.Domain, apiKey: options.APIKey,
		httpClient: options.HTTPClient, sandboxURL: defaultSandboxURL,
	}
	return &Backend{
		client: client, profiles: profiles, agentPort: options.AgentPort, envdPort: options.EnvdPort,
		timeoutSeconds: options.TimeoutSeconds, readyTimeout: options.ReadyTimeout,
	}, nil
}

func (b *Backend) CreateBinding(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	profile, ok := b.profiles[strings.TrimSpace(spec.ProfileID)]
	if !ok {
		return agentexecution.Allocation{}, fmt.Errorf("%w: E2B profile is not approved", agentexecution.ErrInvalidRequest)
	}
	token, err := newAgentToken()
	if err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("create Runtime Agent token: %w", err)
	}
	metadata := map[string]string{
		"zgi.binding_id": spec.BindingID,
		"zgi.profile_id": spec.ProfileID,
	}
	if spec.AllocationAttemptID != "" {
		metadata["zgi.attempt_id"] = spec.AllocationAttemptID
	}
	connection, err := b.client.create(ctx, createSandboxRequest{
		TemplateID: profile.TemplateID,
		Metadata:   metadata,
		Environment: map[string]string{
			agentTokenEnvironment:            token,
			"ZGI_RUNTIME_AGENT_LISTEN":       fmt.Sprintf("tcp://0.0.0.0:%d", b.agentPort),
			"ZGI_RUNTIME_AGENT_NETWORK_MODE": "disabled",
		},
		TimeoutSeconds: b.timeoutSeconds, AllowInternet: false,
		AutoPause: true, AutoPauseMemory: true, AutoResume: autoResumeConfig{Enabled: false},
		Network: networkConfig{DenyOut: []string{"0.0.0.0/0"}, AllowPublicTraffic: false},
	})
	if err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("create E2B environment: %w", err)
	}
	allocation := allocationFor(connection.SandboxID)
	err = b.client.uploadBootstrapToken(ctx, connection, b.envdPort, token)
	if err == nil {
		var remote *runtimeagent.RemoteBackend
		remote, err = b.remote(connection, token)
		if err == nil {
			err = b.waitReady(ctx, remote, allocation)
		}
	}
	if err != nil {
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_ = b.client.destroy(cleanupContext, connection.SandboxID)
		cancel()
		return agentexecution.Allocation{}, fmt.Errorf("start E2B Runtime Agent: %w", err)
	}
	return allocation, nil
}

func (b *Backend) FindAllocations(ctx context.Context, attemptID string) ([]agentexecution.Allocation, error) {
	if strings.TrimSpace(attemptID) == "" {
		return nil, agentexecution.ErrInvalidRequest
	}
	ids, err := b.client.listByAttempt(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	allocations := make([]agentexecution.Allocation, 0, len(ids))
	for _, id := range ids {
		allocations = append(allocations, allocationFor(id))
	}
	return allocations, nil
}

func (b *Backend) SuspendBinding(ctx context.Context, allocation agentexecution.Allocation) error {
	sandboxID, err := validateAllocation(allocation)
	if err != nil {
		return err
	}
	if err := b.client.pause(ctx, sandboxID); err != nil {
		return fmt.Errorf("pause E2B environment: %w", err)
	}
	return nil
}

func (b *Backend) ResumeBinding(ctx context.Context, _ agentexecution.CreateSpec, allocation agentexecution.Allocation) (agentexecution.Allocation, error) {
	sandboxID, err := validateAllocation(allocation)
	if err != nil {
		return agentexecution.Allocation{}, err
	}
	remote, connection, err := b.connect(ctx, sandboxID)
	if err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("resume E2B environment: %w", err)
	}
	resumed := allocationFor(connection.SandboxID)
	if err := b.waitReady(ctx, remote, resumed); err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("resume E2B Runtime Agent: %w", err)
	}
	return resumed, nil
}

func (b *Backend) DestroyBinding(ctx context.Context, allocation agentexecution.Allocation) error {
	sandboxID, err := validateAllocation(allocation)
	if err != nil {
		return err
	}
	if err := b.client.destroy(ctx, sandboxID); err != nil {
		return fmt.Errorf("destroy E2B environment: %w", err)
	}
	return nil
}

func (b *Backend) Capabilities(ctx context.Context, allocation agentexecution.Allocation) (agentexecution.RuntimeCapabilities, error) {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return agentexecution.RuntimeCapabilities{}, err
	}
	return remote.Capabilities(ctx, allocation)
}

func (b *Backend) StartJob(ctx context.Context, allocation agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return nil, err
	}
	return remote.StartJob(ctx, allocation, spec, sink)
}

func (b *Backend) UploadFile(ctx context.Context, allocation agentexecution.Allocation, path string, reader io.Reader, size int64) (agentexecution.FileInfo, error) {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return agentexecution.FileInfo{}, err
	}
	return remote.UploadFile(ctx, allocation, path, reader, size)
}

func (b *Backend) DownloadFile(ctx context.Context, allocation agentexecution.Allocation, path string) (io.ReadCloser, agentexecution.FileInfo, error) {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return nil, agentexecution.FileInfo{}, err
	}
	return remote.DownloadFile(ctx, allocation, path)
}

func (b *Backend) ExportWorkspace(ctx context.Context, allocation agentexecution.Allocation) (io.ReadCloser, error) {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return nil, err
	}
	return remote.ExportWorkspace(ctx, allocation)
}

func (b *Backend) ImportWorkspace(ctx context.Context, allocation agentexecution.Allocation, archive io.Reader) error {
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return err
	}
	return remote.ImportWorkspace(ctx, allocation, archive)
}

func (b *Backend) dataPlane(ctx context.Context, allocation agentexecution.Allocation) (*runtimeagent.RemoteBackend, error) {
	sandboxID, err := validateAllocation(allocation)
	if err != nil {
		return nil, err
	}
	remote, _, err := b.connect(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("connect E2B data plane: %w", err)
	}
	return remote, nil
}

func (b *Backend) connect(ctx context.Context, sandboxID string) (*runtimeagent.RemoteBackend, sandboxConnection, error) {
	connection, err := b.client.connect(ctx, sandboxID, b.timeoutSeconds)
	if err != nil {
		return nil, sandboxConnection{}, err
	}
	environment, err := b.client.environment(ctx, connection, b.envdPort)
	if err != nil {
		return nil, sandboxConnection{}, err
	}
	token := strings.TrimSpace(environment[agentTokenEnvironment])
	if token == "" {
		return nil, sandboxConnection{}, fmt.Errorf("%w: E2B template did not expose Runtime Agent bootstrap state", agentexecution.ErrBackendUnavailable)
	}
	remote, err := b.remote(connection, token)
	return remote, connection, err
}

func (b *Backend) remote(connection sandboxConnection, token string) (*runtimeagent.RemoteBackend, error) {
	baseURL := b.client.sandboxURL(b.agentPort, connection.SandboxID, connection.Domain)
	return runtimeagent.NewRemote(runtimeagent.RemoteOptions{
		BaseURL: baseURL, Token: token, HTTPClient: b.client.httpClient,
		Headers: http.Header{
			"E2B-Traffic-Access-Token": []string{connection.TrafficAccessToken},
			"E2b-Sandbox-Id":           []string{connection.SandboxID},
			"E2b-Sandbox-Port":         []string{fmt.Sprint(b.agentPort)},
		},
	})
}

func (b *Backend) waitReady(parent context.Context, remote *runtimeagent.RemoteBackend, allocation agentexecution.Allocation) error {
	ctx, cancel := context.WithTimeout(parent, b.readyTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		capabilities, err := remote.Capabilities(ctx, allocation)
		if err == nil {
			if !capabilities.Shell || !capabilities.FileTransfer || capabilities.NetworkMode != "disabled" {
				return fmt.Errorf("%w: E2B Runtime Agent reported unsafe capabilities", agentexecution.ErrBackendUnavailable)
			}
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: Runtime Agent readiness timed out: %v", agentexecution.ErrBackendUnavailable, lastErr)
		case <-ticker.C:
		}
	}
}

func validateAllocation(allocation agentexecution.Allocation) (string, error) {
	bindingRef := strings.TrimSpace(allocation.BindingRef)
	workspaceRef := strings.TrimSpace(allocation.WorkspaceRef)
	if bindingRef == "" || workspaceRef == "" || bindingRef != workspaceRef {
		return "", fmt.Errorf("%w: E2B allocation is invalid", agentexecution.ErrInvalidRequest)
	}
	return bindingRef, nil
}

func allocationFor(sandboxID string) agentexecution.Allocation {
	return agentexecution.Allocation{BindingRef: sandboxID, WorkspaceRef: sandboxID}
}

func newAgentToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validateControlURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"))) {
		return fmt.Errorf("%w: E2B API URL must use HTTPS (loopback HTTP is allowed for tests)", agentexecution.ErrInvalidRequest)
	}
	return nil
}

func defaultSandboxURL(port uint16, sandboxID string, domain string) string {
	return fmt.Sprintf("https://%d-%s.%s", port, sandboxID, domain)
}
