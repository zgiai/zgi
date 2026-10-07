package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/packagesource"
	"github.com/zgiai/zgi-sandbox/internal/runtimeagent"
)

func TestNewRejectsControlledHTTPProfile(t *testing.T) {
	_, err := New(Options{APIKey: "provider-secret", Profiles: map[string]Profile{
		"standard": {TemplateID: "template-zgi-runtime", DependencyPreparationMode: runtimeagent.DependencyPreparationModeControlledHTTP},
	}})
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("controlled-http profile error = %v, want ErrBackendUnavailable", err)
	}
}

func TestValidateRuntimeCapabilitiesRequiresBakedDependencySupport(t *testing.T) {
	profile := Profile{TemplateID: "template-zgi-runtime", DependencyPreparation: true}
	base := agentexecution.RuntimeCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled"}
	if err := validateRuntimeCapabilities(base, profile); !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("missing dependency capability error = %v, want ErrBackendUnavailable", err)
	}
	base.DependencyPreparation = true
	base.DependencyPreparationMode = runtimeagent.DependencyPreparationModeOfflineBundled
	if err := validateRuntimeCapabilities(base, profile); err != nil {
		t.Fatalf("offline dependency capability error = %v", err)
	}
	base.DependencyPreparationMode = runtimeagent.DependencyPreparationModeControlledHTTP
	if err := validateRuntimeCapabilities(base, profile); !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("wrong dependency mode error = %v, want ErrBackendUnavailable", err)
	}
}

func TestValidateRuntimeCapabilitiesRejectsLegacyDirectPackageTemplate(t *testing.T) {
	profile := Profile{DependencyInstallation: true}
	capabilities := agentexecution.RuntimeCapabilities{
		Shell: true, FileTransfer: true, NetworkMode: "package-install",
		DependencyInstallation: true, PackageNetworkMode: "package-install", PackageNetworkEnforced: true,
	}
	if err := validateRuntimeCapabilities(capabilities, profile); !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("legacy package template error=%v, expected fail-closed", err)
	}
	capabilities.EgressProxyAddress = "127.0.0.1:3128"
	if err := validateRuntimeCapabilities(capabilities, profile); err != nil {
		t.Fatalf("guarded template rejected: %v", err)
	}
}

func TestBackendUsesSecuredOfflineE2BContract(t *testing.T) {
	var mu sync.Mutex
	var created createSandboxRequest
	var agentToken string
	var uploadedToken string
	var uploadedConfig packagesource.Bootstrap
	var uploadOrder []string
	var runtimeNetworkMode string
	connectCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/v2/") || strings.HasPrefix(request.URL.Path, "/sandboxes/") {
			if request.Header.Get("X-API-KEY") != "provider-secret" {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v2/sandboxes":
			if err := json.NewDecoder(request.Body).Decode(&created); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			agentToken = created.Environment[agentTokenEnvironment]
			runtimeNetworkMode = created.Environment["ZGI_RUNTIME_AGENT_NETWORK_MODE"]
			mu.Unlock()
			writeJSON(writer, http.StatusCreated, sandboxConnection{
				SandboxID: "sandbox-1", Domain: "e2b.test",
				EnvdAccessToken: "envd-create", TrafficAccessToken: "traffic-create",
			})
		case request.Method == http.MethodPost && request.URL.Path == "/files":
			filePath := request.URL.Query().Get("path")
			if request.Header.Get("X-Access-Token") != "envd-create" || request.Header.Get("E2b-Sandbox-Port") != "49983" || (filePath != runtimeAgentBootstrapTokenPath && filePath != "/run/zgi-agent/bootstrap-config.json") || request.URL.Query().Get("username") != runtimeAgentUser {
				http.Error(writer, "bad bootstrap upload", http.StatusUnauthorized)
				return
			}
			value, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			uploadOrder = append(uploadOrder, filePath)
			if filePath == runtimeAgentBootstrapTokenPath {
				uploadedToken = string(value)
			} else if err := json.Unmarshal(value, &uploadedConfig); err != nil {
				mu.Unlock()
				http.Error(writer, "invalid bootstrap config", http.StatusBadRequest)
				return
			}
			mu.Unlock()
			writeJSON(writer, http.StatusOK, []map[string]string{{"path": runtimeAgentBootstrapTokenPath}})
		case request.Method == http.MethodPost && request.URL.Path == "/v2/sandboxes/sandbox-1/connect":
			mu.Lock()
			connectCalls++
			mu.Unlock()
			writeJSON(writer, http.StatusOK, sandboxConnection{
				SandboxID: "sandbox-1", Domain: "e2b.test",
				EnvdAccessToken: "envd-connect", TrafficAccessToken: "traffic-connect",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/envs":
			if request.Header.Get("X-Access-Token") != "envd-connect" || request.Header.Get("E2b-Sandbox-Port") != "49983" {
				http.Error(writer, "bad envd authentication", http.StatusUnauthorized)
				return
			}
			mu.Lock()
			token := agentToken
			mu.Unlock()
			writeJSON(writer, http.StatusOK, map[string]string{agentTokenEnvironment: token})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/capabilities":
			mu.Lock()
			token := agentToken
			mu.Unlock()
			if request.Header.Get("Authorization") != "Bearer "+token || request.Header.Get("E2b-Traffic-Access-Token") == "" || request.Header.Get("E2b-Sandbox-Port") != "49984" {
				http.Error(writer, "bad Runtime Agent authentication", http.StatusUnauthorized)
				return
			}
			capabilities := agentexecution.RuntimeCapabilities{Shell: true, FileTransfer: true, PTY: true, NetworkMode: runtimeNetworkMode}
			if runtimeNetworkMode == "package-install" {
				capabilities.DependencyInstallation = true
				capabilities.PackageNetworkMode = "package-install"
				capabilities.PackageNetworkEnforced = true
				capabilities.EgressProxyAddress = "127.0.0.1:3128"
			}
			writeJSON(writer, http.StatusOK, map[string]any{"data": capabilities})
		case request.Method == http.MethodPost && request.URL.Path == "/sandboxes/sandbox-1/pause":
			writeJSON(writer, http.StatusConflict, map[string]string{"message": "already paused"})
		case request.Method == http.MethodDelete && request.URL.Path == "/sandboxes/sandbox-1":
			writer.WriteHeader(http.StatusNotFound)
		default:
			http.Error(writer, fmt.Sprintf("unexpected %s %s", request.Method, request.URL.Path), http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	backend, err := New(Options{
		APIURL: server.URL, APIKey: "provider-secret",
		Profiles: map[string]Profile{
			"standard": {TemplateID: "template-zgi-runtime"},
			"package":  {TemplateID: "template-zgi-runtime", DependencyInstallation: true, PackageNetworkMode: "package-install", EgressAllowedHosts: []string{"pkg.internal"}},
		},
		HTTPClient: server.Client(), ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	backend.client.sandboxURL = func(uint16, string, string) string { return server.URL }
	spec := agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "standard"}
	allocation, err := backend.CreateBinding(context.Background(), spec)
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if allocation.BindingRef != "sandbox-1" || allocation.WorkspaceRef != "sandbox-1" {
		t.Fatalf("allocation = %+v", allocation)
	}
	if created.TemplateID != "template-zgi-runtime" || created.AllowInternet || created.Network.AllowPublicTraffic || !created.AutoPause || !created.AutoPauseMemory || created.AutoResume.Enabled || len(created.Network.DenyOut) != 1 || created.Network.DenyOut[0] != "0.0.0.0/0" {
		t.Fatalf("unsafe create request = %+v", created)
	}
	if created.Metadata["zgi.binding_id"] != "binding-1" || created.Metadata["zgi.profile_id"] != "standard" || len(created.Metadata) != 2 {
		t.Fatalf("metadata = %#v", created.Metadata)
	}
	if agentToken == "" || agentToken == "provider-secret" {
		t.Fatalf("agent token was not independently generated")
	}
	if uploadedToken != agentToken {
		t.Fatalf("bootstrap token was not uploaded through envd")
	}
	if capabilities, err := backend.Capabilities(context.Background(), allocation); err != nil || capabilities.NetworkMode != "disabled" {
		t.Fatalf("capabilities=%+v error=%v", capabilities, err)
	}
	if err := backend.SuspendBinding(context.Background(), allocation); err != nil {
		t.Fatalf("suspend idempotently: %v", err)
	}
	resumed, err := backend.ResumeBinding(context.Background(), spec, allocation)
	if err != nil || resumed != allocation {
		t.Fatalf("resume=%+v error=%v", resumed, err)
	}
	if err := backend.DestroyBinding(context.Background(), allocation); err != nil {
		t.Fatalf("destroy idempotently: %v", err)
	}
	mu.Lock()
	gotConnectCalls := connectCalls
	mu.Unlock()
	if gotConnectCalls != 2 {
		t.Fatalf("connect calls = %d, want 2", gotConnectCalls)
	}
	spec.HomeSnapshotID = "prepared"
	spec.HomeSnapshotRef = "approved-prebuilt-template"
	if _, err := backend.CreateBinding(context.Background(), spec); err != nil {
		t.Fatalf("create from approved Home Snapshot: %v", err)
	}
	if created.TemplateID != "approved-prebuilt-template" {
		t.Fatalf("template used = %q", created.TemplateID)
	}
	packageSpec := agentexecution.CreateSpec{BindingID: "binding-package", WorkspaceID: "workspace-package", ProfileID: "package", NetworkMode: "package-install"}
	if _, err := backend.CreateBinding(context.Background(), packageSpec); err != nil {
		t.Fatalf("create package binding: %v", err)
	}
	if created.AllowInternet || len(created.Network.DenyOut) != 1 || created.Network.DenyOut[0] != "0.0.0.0/0" || len(created.Network.AllowOut) != 1 || created.Network.AllowOut[0] != "pkg.internal" || created.Network.AllowPublicTraffic {
		t.Fatalf("unsafe package network request = %+v", created.Network)
	}
	if created.Environment["ZGI_RUNTIME_AGENT_NETWORK_MODE"] != "package-install" || created.Environment["ZGI_RUNTIME_AGENT_EGRESS_MODE"] != runtimeagent.EgressModeDirect || created.Environment["ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED"] != "true" || created.Environment["ZGI_RUNTIME_AGENT_JOB_UID"] != "65532" {
		t.Fatalf("package runtime environment = %#v", created.Environment)
	}
	mu.Lock()
	defer mu.Unlock()
	if uploadedConfig.ProfileIdentity != "template-zgi-runtime" || len(uploadedConfig.AllowedHosts) != 1 || uploadedConfig.AllowedHosts[0] != "pkg.internal" || len(uploadOrder) < 2 || uploadOrder[len(uploadOrder)-2] != "/run/zgi-agent/bootstrap-config.json" || uploadOrder[len(uploadOrder)-1] != runtimeAgentBootstrapTokenPath {
		t.Fatalf("bootstrap config/order=%+v %v", uploadedConfig, uploadOrder)
	}
}

func TestRuntimeCapabilitiesRejectMismatchedPackageSources(t *testing.T) {
	profile := Profile{DependencyInstallation: true, PackageSources: packagesource.Config{PyPI: "https://packages.example/simple"}}
	caps := agentexecution.RuntimeCapabilities{Shell: true, FileTransfer: true, DependencyInstallation: true, PackageNetworkMode: "package-install", NetworkMode: "package-install", PackageNetworkEnforced: true, EgressProxyAddress: "127.0.0.1:3128"}
	if err := validateRuntimeCapabilities(caps, profile); err == nil {
		t.Fatal("old template silently accepted a source override")
	}
	caps.PackageSourceDigest = profile.PackageSources.Digest()
	if err := validateRuntimeCapabilities(caps, profile); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAllocationAttemptIncludesPausedSandboxes(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v2/sandboxes" || request.Header.Get("X-API-KEY") != "provider-secret" {
			http.Error(writer, "unexpected control request", http.StatusBadRequest)
			return
		}
		if request.URL.Query().Get("metadata") != "zgi.attempt_id=ara-test" {
			http.Error(writer, "unfiltered allocation list", http.StatusBadRequest)
			return
		}
		requests++
		if requests == 1 {
			writer.Header().Set("X-Next-Token", "next")
			writeJSON(writer, http.StatusOK, []map[string]any{{"sandboxID": "paused-1", "state": "paused", "metadata": map[string]string{"zgi.attempt_id": "ara-test"}}})
			return
		}
		if request.URL.Query().Get("nextToken") != "next" {
			http.Error(writer, "wrong pagination token", http.StatusBadRequest)
			return
		}
		writeJSON(writer, http.StatusOK, []map[string]any{{"sandboxID": "running-2", "state": "running", "metadata": map[string]string{"zgi.attempt_id": "ara-test"}}})
	}))
	defer server.Close()
	backend, err := New(Options{APIURL: server.URL, APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template"}}, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := backend.FindAllocations(context.Background(), "ara-test")
	if err != nil || len(allocations) != 2 || allocations[0].BindingRef != "paused-1" || allocations[1].BindingRef != "running-2" {
		t.Fatalf("allocations = %+v, %v", allocations, err)
	}
}

func TestBackendClassifiesProviderLimitsWithoutReturningBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"message":"secret-provider-detail"}`))
	}))
	t.Cleanup(server.Close)
	backend, err := New(Options{
		APIURL: server.URL, APIKey: "provider-secret", HTTPClient: server.Client(),
		Profiles: map[string]Profile{"standard": {TemplateID: "template"}}, ReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	_, err = backend.CreateBinding(context.Background(), agentexecution.CreateSpec{BindingID: "binding", ProfileID: "standard"})
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) || strings.Contains(fmt.Sprint(err), "secret-provider-detail") {
		t.Fatalf("create error = %v", err)
	}
}

func TestBackendRejectsUnapprovedProfilesAndInvalidConfiguration(t *testing.T) {
	if _, err := New(Options{}); !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("empty config error = %v", err)
	}
	if _, err := New(Options{APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template", DependencyInstallation: true, PackageNetworkMode: "package-install"}}}); !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("E2B package installation error = %v, want invalid request", err)
	}
	if _, err := New(Options{APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template", DependencyInstallation: true, PackageNetworkMode: "package-install", EgressAllowedHosts: []string{"pkg.internal"}}}}); err != nil {
		t.Fatalf("valid E2B package profile rejected: %v", err)
	}
	backend, err := New(Options{
		APIKey: "provider-secret", Profiles: map[string]Profile{"standard": {TemplateID: "template"}},
	})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	_, err = backend.CreateBinding(context.Background(), agentexecution.CreateSpec{ProfileID: "unknown"})
	if !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("unapproved profile error = %v", err)
	}
	_, err = backend.CreateBinding(context.Background(), agentexecution.CreateSpec{ProfileID: "standard", NetworkMode: "controlled-http"})
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("unsupported network mode error = %v, want ErrBackendUnavailable", err)
	}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
