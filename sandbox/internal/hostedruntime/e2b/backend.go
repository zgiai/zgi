// Package e2b implements the hosted Agent execution backend using E2B's
// current REST control plane and the ZGI Runtime Agent data-plane protocol.
// Provider credentials and short-lived connection tokens are never returned
// in Allocation values and therefore are never persisted by the control plane.
package e2b

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
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
	TemplateID                string               `json:"template_id"`
	DependencyPreparation     bool                 `json:"dependency_preparation,omitempty"`
	DependencyPreparationMode string               `json:"dependency_preparation_mode,omitempty"`
	DependencyInstallation    bool                 `json:"dependency_installation,omitempty"`
	PackageNetworkMode        string               `json:"package_network_mode,omitempty"`
	EgressAllowedHosts        []string             `json:"egress_allowed_hosts,omitempty"`
	PackageSources            packagesource.Config `json:"package_sources,omitempty"`
	PackageSourceCheck        bool                 `json:"package_source_check,omitempty"`
}

// EgressProxyConfig is operator-managed E2B network configuration. It is
// supplied from worker environment, never from Agent configuration or a
// persisted Binding.
type EgressProxyConfig struct {
	Address  string
	Username string
	Password string
}

type Options struct {
	PlatformCLI    bool
	APIURL         string
	Domain         string
	APIKey         string
	HTTPClient     *http.Client
	Profiles       map[string]Profile
	AgentPort      uint16
	EnvdPort       uint16
	TimeoutSeconds int
	ReadyTimeout   time.Duration
	EgressProxy    *EgressProxyConfig
}

type Backend struct {
	platformCLI    bool
	client         *controlClient
	profiles       map[string]Profile
	agentPort      uint16
	envdPort       uint16
	timeoutSeconds int
	readyTimeout   time.Duration
	egressProxy    *EgressProxyConfig
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
	if options.EgressProxy != nil {
		if err := validateEgressProxy(*options.EgressProxy); err != nil {
			return nil, err
		}
	}
	profiles := make(map[string]Profile, len(options.Profiles))
	for name, profile := range options.Profiles {
		name = strings.TrimSpace(name)
		profile.TemplateID = strings.TrimSpace(profile.TemplateID)
		if name == "" || profile.TemplateID == "" {
			return nil, fmt.Errorf("%w: E2B profile names and template IDs are required", agentexecution.ErrInvalidRequest)
		}
		if strings.EqualFold(strings.TrimSpace(profile.DependencyPreparationMode), runtimeagent.DependencyPreparationModeControlledHTTP) {
			return nil, fmt.Errorf("%w: E2B does not provide the controlled HTTPS egress bridge", agentexecution.ErrBackendUnavailable)
		}
		packageMode := strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode))
		if packageMode != "" && packageMode != "package-install" {
			return nil, fmt.Errorf("%w: E2B package network mode %q is not available", agentexecution.ErrBackendUnavailable, packageMode)
		}
		if profile.DependencyInstallation && packageMode != "package-install" {
			return nil, fmt.Errorf("%w: E2B dependency installation requires package-install network mode", agentexecution.ErrInvalidRequest)
		}
		if packageMode == "package-install" {
			if err := validatePackageSourceAllowlist(profile.EgressAllowedHosts, profile.PackageSources); err != nil {
				return nil, fmt.Errorf("%w: E2B package source allowlist: %v", agentexecution.ErrInvalidRequest, err)
			}
		}
		profile.EgressAllowedHosts = append([]string(nil), profile.EgressAllowedHosts...)
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
		platformCLI: options.PlatformCLI,
		client:      client, profiles: profiles, agentPort: options.AgentPort, envdPort: options.EnvdPort,
		timeoutSeconds: options.TimeoutSeconds, readyTimeout: options.ReadyTimeout,
		egressProxy: cloneEgressProxy(options.EgressProxy),
	}, nil
}

func (b *Backend) CreateBinding(ctx context.Context, spec agentexecution.CreateSpec) (agentexecution.Allocation, error) {
	networkMode := strings.ToLower(strings.TrimSpace(spec.NetworkMode))
	if networkMode == "" {
		networkMode = "disabled"
	}
	if networkMode != "disabled" && networkMode != "package-install" {
		return agentexecution.Allocation{}, fmt.Errorf("%w: E2B runtime network mode %q is not available", agentexecution.ErrBackendUnavailable, networkMode)
	}
	profile, ok := b.profiles[strings.TrimSpace(spec.ProfileID)]
	if !ok {
		return agentexecution.Allocation{}, fmt.Errorf("%w: E2B profile is not approved", agentexecution.ErrInvalidRequest)
	}
	if spec.HomeSnapshotID != "" {
		if strings.TrimSpace(spec.HomeSnapshotRef) == "" {
			return agentexecution.Allocation{}, fmt.Errorf("%w: approved E2B Home Snapshot template is missing", agentexecution.ErrInvalidRequest)
		}
		profile.TemplateID = spec.HomeSnapshotRef
	}
	if networkMode == "package-install" {
		if !profile.DependencyInstallation || strings.ToLower(strings.TrimSpace(profile.PackageNetworkMode)) != "package-install" {
			return agentexecution.Allocation{}, fmt.Errorf("%w: E2B profile does not approve package installation", agentexecution.ErrBackendUnavailable)
		}
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
	environment := map[string]string{
		"ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED":           fmt.Sprint(b.platformCLI),
		agentTokenEnvironment:                              token,
		"ZGI_RUNTIME_AGENT_LISTEN":                         fmt.Sprintf("tcp://0.0.0.0:%d", b.agentPort),
		"ZGI_RUNTIME_AGENT_NETWORK_MODE":                   networkMode,
		"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED": fmt.Sprintf("%t", profile.DependencyPreparation),
		"ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_MODE":    strings.TrimSpace(profile.DependencyPreparationMode),
	}
	if b.platformCLI {
		environment["ZGI_RUNTIME_AGENT_JOB_UID"] = "65532"
		environment["ZGI_RUNTIME_AGENT_JOB_GID"] = "65532"
	}
	if networkMode == "package-install" {
		allowedHostsJSON, marshalErr := json.Marshal(profile.EgressAllowedHosts)
		if marshalErr != nil {
			return agentexecution.Allocation{}, fmt.Errorf("encode approved package hosts: %w", marshalErr)
		}
		environment["ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON"] = string(allowedHostsJSON)
		environment["ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED"] = "true"
		environment["ZGI_RUNTIME_AGENT_PACKAGE_NETWORK_MODE"] = "package-install"
		environment["ZGI_RUNTIME_AGENT_EGRESS_MODE"] = runtimeagent.EgressModeDirect
		environment["ZGI_RUNTIME_AGENT_JOB_UID"] = "65532"
		environment["ZGI_RUNTIME_AGENT_JOB_GID"] = "65532"
		environment["ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON"] = profile.PackageSources.JSON()
		environment["ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK"] = fmt.Sprint(profile.PackageSourceCheck)
		environment["ZGI_RUNTIME_AGENT_PROFILE_IDENTITY"] = profile.TemplateID
	}
	connection, err := b.client.create(ctx, createSandboxRequest{
		TemplateID:     profile.TemplateID,
		Metadata:       metadata,
		Environment:    environment,
		TimeoutSeconds: b.timeoutSeconds, AllowInternet: false,
		AutoPause: true, AutoPauseMemory: true, AutoResume: autoResumeConfig{Enabled: false},
		Network: networkConfig{
			AllowOut: append([]string(nil), profile.EgressAllowedHosts...),
			// E2B's network API accepts the IPv4 deny CIDR but rejects ::/0.
			// E2B sandboxes do not expose an IPv6 egress path, so the provider's
			// IPv4 deny plus the explicit package allowlist is the supported
			// package-only boundary.
			DenyOut:            []string{"0.0.0.0/0"},
			EgressProxy:        e2bProxyConfig(b.egressProxy, networkMode == "package-install"),
			AllowPublicTraffic: false,
		},
	})
	if err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("create E2B environment: %w", err)
	}
	allocation := allocationFor(connection.SandboxID)
	if networkMode == "package-install" || b.platformCLI {
		policy := &packagesource.RuntimeConfig{NetworkMode: networkMode, EgressMode: runtimeagent.EgressModeBridge, DependencyPreparation: profile.DependencyPreparation, DependencyPreparationMode: profile.DependencyPreparationMode}
		if b.platformCLI || networkMode == "package-install" {
			policy.JobUID, policy.JobGID = 65532, 65532
		}
		if networkMode == "package-install" {
			policy.EgressMode, policy.PackageNetworkMode, policy.DependencyInstallation = runtimeagent.EgressModeDirect, "package-install", true
		}
		config, marshalErr := json.Marshal(packagesource.Bootstrap{PlatformCLI: b.platformCLI, Runtime: policy, Sources: profile.PackageSources, AllowedHosts: profile.EgressAllowedHosts, ProfileIdentity: profile.TemplateID, Check: networkMode == "package-install" && profile.PackageSourceCheck})
		if marshalErr != nil {
			err = marshalErr
		} else {
			err = b.client.uploadBootstrapFile(ctx, connection, b.envdPort, "/run/zgi-agent/bootstrap-config.json", string(config))
		}
	}
	if err == nil {
		err = b.client.uploadBootstrapToken(ctx, connection, b.envdPort, token)
	}
	if err == nil {
		var remote *runtimeagent.RemoteBackend
		remote, err = b.remote(connection, token)
		if err == nil {
			err = b.waitReady(ctx, remote, allocation, profile)
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

func (b *Backend) ResumeBinding(ctx context.Context, spec agentexecution.CreateSpec, allocation agentexecution.Allocation) (agentexecution.Allocation, error) {
	sandboxID, err := validateAllocation(allocation)
	if err != nil {
		return agentexecution.Allocation{}, err
	}
	remote, connection, err := b.connect(ctx, sandboxID)
	if err != nil {
		return agentexecution.Allocation{}, fmt.Errorf("resume E2B environment: %w", err)
	}
	resumed := allocationFor(connection.SandboxID)
	profile, ok := b.profiles[strings.TrimSpace(spec.ProfileID)]
	if !ok {
		return agentexecution.Allocation{}, fmt.Errorf("%w: E2B profile is not approved", agentexecution.ErrInvalidRequest)
	}
	if err := b.waitReady(ctx, remote, resumed, profile); err != nil {
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
	capabilities, err := remote.Capabilities(ctx, allocation)
	if !b.platformCLI {
		capabilities.PlatformProtocolVersion = 0
	}
	if err == nil && capabilities.NetworkMode == "package-install" {
		err = validateRuntimeCapabilities(capabilities, Profile{DependencyInstallation: true})
	}
	return capabilities, err
}

func (b *Backend) StartJob(ctx context.Context, allocation agentexecution.Allocation, spec agentexecution.JobSpec, sink func(agentexecution.OutputChunk) error) (agentexecution.RunningJob, error) {
	if spec.Environment["ZGI_JOB_PLATFORM_ENABLED"] == "true" && !b.platformCLI {
		return nil, agentexecution.ErrBackendUnavailable
	}
	remote, err := b.dataPlane(ctx, allocation)
	if err != nil {
		return nil, err
	}
	// Existing Bindings may still point at a template predating the Job proxy
	// guard. Reject execution there too, not only on create/resume; file retrieval
	// and cleanup remain available without launching untrusted commands.
	capabilities, err := remote.Capabilities(ctx, allocation)
	if err != nil {
		return nil, err
	}
	if capabilities.NetworkMode == "package-install" {
		if err := validateRuntimeCapabilities(capabilities, Profile{DependencyInstallation: true}); err != nil {
			return nil, err
		}
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
	// Large attachment transfers can outlive the short control-plane timeout.
	// Copy the client so control calls retain their timeout; request contexts
	// still bound readiness checks, cancellation and individual operations.
	dataClient := *b.client.httpClient
	if dataClient.Timeout > 0 && dataClient.Timeout < 2*time.Minute {
		dataClient.Timeout = 2 * time.Minute
	}
	return runtimeagent.NewRemote(runtimeagent.RemoteOptions{
		BaseURL: baseURL, Token: token, HTTPClient: &dataClient,
		Headers: http.Header{
			"E2B-Traffic-Access-Token": []string{connection.TrafficAccessToken},
			"E2b-Sandbox-Id":           []string{connection.SandboxID},
			"E2b-Sandbox-Port":         []string{fmt.Sprint(b.agentPort)},
		},
	})
}

func (b *Backend) waitReady(parent context.Context, remote *runtimeagent.RemoteBackend, allocation agentexecution.Allocation, profile Profile) error {
	ctx, cancel := context.WithTimeout(parent, b.readyTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		capabilities, err := remote.Capabilities(ctx, allocation)
		if err == nil {
			if b.platformCLI && capabilities.PlatformProtocolVersion != agentexecution.PlatformProtocolVersion {
				return fmt.Errorf("%w: approved E2B template lacks platform CLI protocol", agentexecution.ErrBackendUnavailable)
			}
			return validateRuntimeCapabilities(capabilities, profile)
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: Runtime Agent readiness timed out: %v", agentexecution.ErrBackendUnavailable, lastErr)
		case <-ticker.C:
		}
	}
}

func validateRuntimeCapabilities(capabilities agentexecution.RuntimeCapabilities, profile Profile) error {
	if profile.PackageSources != (packagesource.Config{}) && capabilities.PackageSourceDigest != profile.PackageSources.Digest() {
		return fmt.Errorf("%w: E2B template did not apply approved package sources", agentexecution.ErrBackendUnavailable)
	}
	expectedNetworkMode := "disabled"
	if profile.DependencyInstallation {
		expectedNetworkMode = "package-install"
	}
	if !capabilities.Shell || !capabilities.FileTransfer || capabilities.NetworkMode != expectedNetworkMode {
		return fmt.Errorf("%w: E2B Runtime Agent reported unsafe capabilities", agentexecution.ErrBackendUnavailable)
	}
	if profile.DependencyPreparation {
		expectedMode := strings.ToLower(strings.TrimSpace(profile.DependencyPreparationMode))
		if expectedMode == "" {
			expectedMode = runtimeagent.DependencyPreparationModeOfflineBundled
		}
		if !capabilities.DependencyPreparation || strings.ToLower(strings.TrimSpace(capabilities.DependencyPreparationMode)) != expectedMode {
			return fmt.Errorf("%w: E2B Runtime Agent template does not provide the approved dependency preparation capability", agentexecution.ErrBackendUnavailable)
		}
	}
	if profile.DependencyInstallation {
		if capabilities.LanguagePackageLedgerVersion != agentexecution.LanguagePackageLedgerVersion {
			return fmt.Errorf("%w: E2B template lacks retained dependency artifact recovery; rebuild the package template", agentexecution.ErrBackendUnavailable)
		}
		if capabilities.SystemPackageLedgerVersion != agentexecution.SystemPackageLedgerVersion {
			return fmt.Errorf("%w: E2B template lacks fixed-version system package recovery; rebuild the package template", agentexecution.ErrBackendUnavailable)
		}
		if !capabilities.DependencyInstallation || capabilities.PackageNetworkMode != "package-install" || capabilities.NetworkMode != "package-install" || !capabilities.PackageNetworkEnforced || capabilities.EgressProxyAddress != "127.0.0.1:3128" {
			return fmt.Errorf("%w: E2B Runtime Agent template does not provide the guarded package installation capability; rebuild the package template", agentexecution.ErrBackendUnavailable)
		}
	}
	return nil
}

func validateEgressProxy(proxy EgressProxyConfig) error {
	address := strings.TrimSpace(proxy.Address)
	if address == "" {
		return fmt.Errorf("%w: E2B egress proxy address is required", agentexecution.ErrInvalidRequest)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return fmt.Errorf("%w: E2B egress proxy address must use host:port", agentexecution.ErrInvalidRequest)
	}
	if strings.ContainsAny(address, "\r\n") || (proxy.Password != "" && proxy.Username == "") {
		return fmt.Errorf("%w: E2B egress proxy credentials are invalid", agentexecution.ErrInvalidRequest)
	}
	return nil
}

func cloneEgressProxy(proxy *EgressProxyConfig) *EgressProxyConfig {
	if proxy == nil {
		return nil
	}
	copy := *proxy
	return &copy
}

func e2bProxyConfig(proxy *EgressProxyConfig, enabled bool) *egressProxyConfig {
	if !enabled || proxy == nil {
		return nil
	}
	return &egressProxyConfig{Address: proxy.Address, Username: proxy.Username, Password: proxy.Password}
}

func validatePackageSourceAllowlist(hosts []string, sources ...packagesource.Config) error {
	if err := (egressproxy.Policy{AllowedHosts: hosts}).Validate(); err != nil {
		return err
	}
	knownExact := map[string]struct{}{
		"pypi.org": {}, "files.pythonhosted.org": {}, "registry.npmjs.org": {}, "registry.yarnpkg.com": {},
		"deb.debian.org": {}, "security.debian.org": {}, "snapshot.debian.org": {},
	}
	for _, source := range sources {
		if err := source.Validate(hosts); err != nil {
			return err
		}
		for _, host := range source.Hosts() {
			knownExact[strings.ToLower(host)] = struct{}{}
		}
	}
	for _, raw := range hosts {
		host := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "*."))
		known := strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local")
		if _, ok := knownExact[host]; ok {
			known = true
		}
		if host == "pythonhosted.org" || strings.HasSuffix(host, ".pythonhosted.org") {
			known = true
		}
		if !known {
			return fmt.Errorf("host %q is not an approved package source", raw)
		}
	}
	return nil
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
