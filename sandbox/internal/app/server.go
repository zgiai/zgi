package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/cache"
	"github.com/zgiai/zgi-sandbox/internal/catalog"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/executor"
	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
	"github.com/zgiai/zgi-sandbox/internal/storage"
)

//go:embed web/*
var webFS embed.FS

const statusClientClosedRequest = 499

type Server struct {
	config    config.Config
	store     *storage.Store
	runner    *runner.Service
	lifecycle *lifecycle.Manager
	executor  *executor.Service
	observer  *observer.Recorder
	policy    *policy.Service
	blueprint catalog.Blueprint
	mux       *http.ServeMux
}

func NewServer(cfg config.Config) (*Server, error) {
	if err := cfg.ValidateStartup(); err != nil {
		return nil, err
	}

	store, err := storage.Open(cfg)
	if err != nil {
		return nil, err
	}

	sandboxCache, err := cache.NewSandboxCache(cfg)
	if err != nil {
		return nil, err
	}

	policyService := policy.NewService(cfg)
	recorder := observer.NewRecorderWithStore(store)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, store, sandboxCache)
	if err != nil {
		return nil, err
	}

	runnerService, err := runner.NewServiceFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	s := &Server{
		config:    cfg,
		store:     store,
		runner:    runnerService,
		lifecycle: manager,
		executor:  executor.NewService(manager, runnerService, recorder, policyService),
		observer:  recorder,
		policy:    policyService,
		blueprint: catalog.DefaultBlueprint(),
		mux:       http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.withRequestID(s.mux)
}

func (s *Server) registerRoutes() {
	assetFS, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}

	static := http.StripPrefix("/assets/", http.FileServer(http.FS(assetFS)))
	s.mux.Handle("/assets/", static)
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)
	s.mux.HandleFunc("/v1/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/blueprint", s.handleBlueprint)
	s.mux.HandleFunc("/v1/policies", s.handlePolicies)
	s.mux.HandleFunc("/v1/sandbox/run", s.handleRun)
	s.mux.HandleFunc("/v1/sandbox/dependencies", s.handleDependencies)
	s.mux.HandleFunc("/v1/sandbox/dependencies/update", s.handleDependencyUpdate)
	s.mux.HandleFunc("/v1/sandboxes", s.handleSandboxes)
	s.mux.HandleFunc("/v1/sandboxes/", s.handleSandboxByID)
	s.mux.HandleFunc("/v1/exec/code", s.handleExecCode)
	s.mux.HandleFunc("/v1/exec/command", s.handleExecCommand)
	s.mux.HandleFunc("/v1/exec/template", s.handleExecTemplate)
	s.mux.HandleFunc("/v1/exec/skill", s.handleExecSkill)
	s.mux.HandleFunc("/v1/files/upload", s.handleUploadFile)
	s.mux.HandleFunc("/v1/files/upload-archive", s.handleUploadArchive)
	s.mux.HandleFunc("/v1/files/download", s.handleDownloadFile)
	s.mux.HandleFunc("/v1/files/info", s.handleFileInfo)
	s.mux.HandleFunc("/v1/files/tree", s.handleFileTree)
	s.mux.HandleFunc("/v1/files/manifest", s.handleFileManifest)
	s.mux.HandleFunc("/v1/files", s.handleDeleteFile)
	s.mux.HandleFunc("/v1/observer/events", s.handleObserverEvents)
	s.mux.HandleFunc("/_zgi/ports/", s.handleInteractiveProxy)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, webFS, "web/index.html")
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                  "ok",
		"service":                 "zgi-sandbox",
		"version":                 "session-v2",
		"worker_id":               s.config.WorkerID,
		"environment":             s.config.Environment,
		"shutdown_timeout_secs":   s.config.ShutdownTimeoutSeconds,
		"runtime_backend":         s.policy.RuntimeBackend(),
		"network_policy_enforced": s.policy.NetworkPolicyEnforced(),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	checks := map[string]string{
		"postgres": "ok",
		"runtime":  "ok",
	}
	status := "ready"
	httpStatus := http.StatusOK

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
		checks["postgres"] = "error"
	}
	if s.runner == nil {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
		checks["runtime"] = "error"
	}

	writeJSON(w, httpStatus, map[string]any{
		"status":      status,
		"ready":       httpStatus == http.StatusOK,
		"service":     "zgi-sandbox",
		"worker_id":   s.config.WorkerID,
		"environment": s.config.Environment,
		"checks":      checks,
	})
}

func (s *Server) handleBlueprint(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, http.StatusOK, s.blueprint)
}

func (s *Server) handlePolicies(w http.ResponseWriter, _ *http.Request) {
	writeEnvelope(w, http.StatusOK, s.policy.Snapshot())
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	activeSandboxes, err := s.lifecycle.ActiveCount()
	if err != nil {
		writeKnownError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, map[string]any{
		"worker_id":        s.config.WorkerID,
		"active_sandboxes": activeSandboxes,
		"runner":           s.runner.Metrics(),
		"observer":         s.observer.Metrics(1000),
		"observer_retention": map[string]any{
			"retention_days": s.config.ObserverRetentionDays,
			"max_events":     s.config.ObserverMaxEvents,
		},
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromHeaders(r)
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(observer.ContextWithRequestID(r.Context(), requestID)))
	})
}

func requestIDFromHeaders(r *http.Request) string {
	for _, header := range []string{"X-Request-ID", "X-Correlation-ID"} {
		value := sanitizeRequestID(r.Header.Get(header))
		if value != "" {
			return value
		}
	}
	return ""
}

func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(min(len(value), 128))
	for _, char := range value {
		if builder.Len() >= 128 {
			break
		}
		if char < 33 || char > 126 {
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func generateRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req_local"
	}
	return "req_" + hex.EncodeToString(buf[:])
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	var req runner.Request
	r.Body = http.MaxBytesReader(w, r.Body, s.maxExecutionRequestBytes())
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if req.EnableNetwork && !s.policy.NetworkPolicyEnforced() {
		message := fmt.Sprintf("runtime backend %q does not enforce network policy", s.policy.RuntimeBackend())
		s.recordPolicyDenied(r.Context(), "", "network_policy_not_enforced", message, map[string]any{
			"runtime_backend":         s.policy.RuntimeBackend(),
			"network_policy_enforced": s.policy.NetworkPolicyEnforced(),
		})
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, message, nil)
		return
	}

	result, err := s.runner.Run(r.Context(), req)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleDependencies(w http.ResponseWriter, r *http.Request) {
	writeEnvelope(w, http.StatusOK, s.policy.DependencyCatalog(r.URL.Query().Get("language")))
}

func (s *Server) handleDependencyUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeEnvelopeWithMessage(w, http.StatusOK, 0, "preview mode: dependency updates are disabled", map[string]any{
		"accepted":           false,
		"available_profiles": s.policy.DependencyCatalog(r.URL.Query().Get("language")),
	})
}

func (s *Server) handleSandboxes(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		items := s.lifecycle.List()
		if organizationID := requestOrganizationID(r, ""); organizationID != "" {
			filtered := make([]sandbox.Sandbox, 0, len(items))
			for _, item := range items {
				if item.OrganizationID == organizationID {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		writeEnvelope(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req lifecycle.CreateRequest
		r.Body = http.MaxBytesReader(w, r.Body, s.maxSmallJSONRequestBytes())
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeDecodeError(w, err)
			return
		}

		box, err := s.lifecycle.Create(req)
		if err != nil {
			writeKnownError(w, err)
			return
		}
		writeEnvelope(w, http.StatusOK, box)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSandboxByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/sandboxes/")
	parts := splitPath(path)
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}

	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			box, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, ""))
			if !ok {
				return
			}
			writeEnvelope(w, http.StatusOK, box)
		case http.MethodDelete:
			box, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, ""))
			if !ok {
				return
			}
			if s.proxyOwnedBoxRequest(w, r, box) {
				return
			}
			if err := s.lifecycle.Delete(id); err != nil {
				writeKnownError(w, err)
				return
			}
			writeEnvelope(w, http.StatusOK, map[string]any{"deleted": true, "sandbox_id": id})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "renew-expiration" && r.Method == http.MethodPost:
		var req struct {
			TTLSeconds int `json:"ttl_seconds"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.maxSmallJSONRequestBytes())
		_ = json.NewDecoder(r.Body).Decode(&req)
		if _, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, "")); !ok {
			return
		}
		box, err := s.lifecycle.Renew(id, req.TTLSeconds)
		if err != nil {
			writeKnownError(w, err)
			return
		}
		writeEnvelope(w, http.StatusOK, box)
	case len(parts) == 2 && parts[1] == "executions" && r.Method == http.MethodGet:
		box, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, ""))
		if !ok {
			return
		}
		if s.proxyOwnedBoxRequest(w, r, box) {
			return
		}
		s.writeSandboxExecutionHistory(w, r, id)
	case len(parts) == 3 && parts[1] == "endpoints" && r.Method == http.MethodGet:
		if _, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, "")); !ok {
			return
		}
		endpoint, err := s.lifecycle.ResolveEndpoint(id, parts[2])
		if err != nil {
			writeKnownError(w, err)
			return
		}
		writeEnvelope(w, http.StatusOK, endpoint)
	case len(parts) == 3 && parts[1] == "endpoints" && r.Method == http.MethodPost:
		box, ok := s.authorizeSandboxOrganization(r.Context(), w, id, requestOrganizationID(r, ""))
		if !ok {
			return
		}
		if s.proxyOwnedBoxRequest(w, r, box) {
			return
		}

		var req lifecycle.RegisterEndpointRequest
		r.Body = http.MaxBytesReader(w, r.Body, s.maxSmallJSONRequestBytes())
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeDecodeError(w, err)
			return
		}

		endpoint, err := s.lifecycle.RegisterEndpoint(id, parts[2], req)
		if err != nil {
			writeKnownError(w, err)
			return
		}
		writeEnvelope(w, http.StatusOK, endpoint)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleExecCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxExecutionRequestBytes())
	if err != nil {
		return
	}

	var req executor.CodeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, req.SandboxID, requestOrganizationID(r, req.OrganizationID))
	if !ok {
		return
	}
	if s.proxyOwnedBoxBodyRequest(w, r, box, body) {
		return
	}

	result, err := s.executor.RunCode(r.Context(), req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleExecCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxExecutionRequestBytes())
	if err != nil {
		return
	}

	var req executor.CommandRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, req.SandboxID, requestOrganizationID(r, req.OrganizationID))
	if !ok {
		return
	}
	if s.proxyOwnedBoxBodyRequest(w, r, box, body) {
		return
	}

	result, err := s.executor.RunCommand(r.Context(), req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleExecTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxSmallJSONRequestBytes())
	if err != nil {
		return
	}

	var req executor.TemplateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}

	result, err := s.executor.RunTemplate(r.Context(), req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleExecSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxExecutionRequestBytes())
	if err != nil {
		return
	}

	var req executor.SkillRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, req.SandboxID, requestOrganizationID(r, req.OrganizationID))
	if !ok {
		return
	}
	if s.proxyOwnedBoxBodyRequest(w, r, box, body) {
		return
	}

	result, err := s.executor.RunSkill(r.Context(), req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxFileUploadRequestBytes())
	if err != nil {
		return
	}

	var req executor.FileWriteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, req.SandboxID, requestOrganizationID(r, req.OrganizationID))
	if !ok {
		return
	}
	if s.proxyOwnedBoxBodyRequest(w, r, box, body) {
		return
	}

	info, err := s.executor.UploadFile(req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, info)
}

func (s *Server) handleUploadArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	body, err := s.readLimitedBody(w, r, s.maxArchiveUploadRequestBytes())
	if err != nil {
		return
	}

	var req executor.ArchiveUploadRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, req.SandboxID, requestOrganizationID(r, req.OrganizationID))
	if !ok {
		return
	}
	if s.proxyOwnedBoxBodyRequest(w, r, box, body) {
		return
	}

	result, err := s.executor.UploadArchive(req)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, r.URL.Query().Get("sandbox_id"), requestOrganizationID(r, ""))
	if !ok {
		return
	}
	if s.proxyOwnedBoxRequest(w, r, box) {
		return
	}

	result, err := s.executor.DownloadFile(r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"), r.URL.Query().Get("encoding"))
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, result)
}

func (s *Server) handleFileInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, r.URL.Query().Get("sandbox_id"), requestOrganizationID(r, ""))
	if !ok {
		return
	}
	if s.proxyOwnedBoxRequest(w, r, box) {
		return
	}

	info, err := s.executor.StatFile(r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"))
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, info)
}

func (s *Server) handleFileTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, r.URL.Query().Get("sandbox_id"), requestOrganizationID(r, ""))
	if !ok {
		return
	}
	if s.proxyOwnedBoxRequest(w, r, box) {
		return
	}

	items, err := s.executor.ListFiles(r.URL.Query().Get("sandbox_id"))
	if err != nil {
		writeKnownError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFileManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, r.URL.Query().Get("sandbox_id"), requestOrganizationID(r, ""))
	if !ok {
		return
	}
	if s.proxyOwnedBoxRequest(w, r, box) {
		return
	}

	options, err := parseFileManifestOptions(r)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	manifest, err := s.executor.BuildFileManifestWithOptions(r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path"), options)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, manifest)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}
	box, ok := s.authorizeSandboxOrganization(r.Context(), w, r.URL.Query().Get("sandbox_id"), requestOrganizationID(r, ""))
	if !ok {
		return
	}
	if s.proxyOwnedBoxRequest(w, r, box) {
		return
	}

	if err := s.executor.DeleteFile(r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("path")); err != nil {
		writeKnownError(w, err)
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{
		"deleted":    true,
		"sandbox_id": r.URL.Query().Get("sandbox_id"),
		"path":       r.URL.Query().Get("path"),
	})
}

func (s *Server) handleObserverEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		writeEnvelopeWithMessage(w, http.StatusUnauthorized, -401, "unauthorized", nil)
		return
	}

	limit, before, ok := parseEventPage(w, r)
	if !ok {
		return
	}

	events := s.observer.Query(observer.Query{
		SandboxID:      r.URL.Query().Get("sandbox_id"),
		Type:           r.URL.Query().Get("type"),
		OrganizationID: r.URL.Query().Get("organization_id"),
		WorkspaceID:    r.URL.Query().Get("workspace_id"),
		AppID:          r.URL.Query().Get("app_id"),
		WorkflowRunID:  r.URL.Query().Get("workflow_run_id"),
		UserID:         r.URL.Query().Get("user_id"),
		RequestID:      r.URL.Query().Get("request_id"),
		Limit:          limit + 1,
		Before:         before,
	})
	writeEventPage(w, events, limit)
}

func (s *Server) writeSandboxExecutionHistory(w http.ResponseWriter, r *http.Request, sandboxID string) {
	limit, before, ok := parseEventPage(w, r)
	if !ok {
		return
	}

	events := s.observer.Query(observer.Query{
		SandboxID:  sandboxID,
		TypePrefix: "exec.",
		RequestID:  r.URL.Query().Get("request_id"),
		Limit:      limit + 1,
		Before:     before,
	})
	writeEventPage(w, events, limit)
}

func parseEventPage(w http.ResponseWriter, r *http.Request) (int, time.Time, bool) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}

	var before time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid before cursor", nil)
			return 0, time.Time{}, false
		}
		before = parsed
	}
	return limit, before, true
}

func writeEventPage(w http.ResponseWriter, events []observer.Event, limit int) {
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	nextCursor := ""
	if hasMore && len(events) > 0 {
		nextCursor = events[len(events)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
	}

	writeEnvelope(w, http.StatusOK, map[string]any{
		"events":      events,
		"limit":       limit,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
	})
}

func (s *Server) handleInteractiveProxy(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/_zgi/ports/"))
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	sandboxID := parts[0]
	port := parts[1]
	box, err := s.lifecycle.Get(sandboxID)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	if s.shouldProxy(*box) {
		s.proxyToWorker(w, r, box.WorkerAddr)
		return
	}

	endpoint, err := s.lifecycle.EndpointTarget(sandboxID, port)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	remainder := "/"
	if len(parts) > 2 {
		remainder += strings.Join(parts[2:], "/")
	}
	target := fmt.Sprintf("%s://%s:%d", endpoint.Scheme, endpoint.TargetHost, endpoint.TargetPort)
	s.proxyToTarget(w, r, target, remainder)
}

func (s *Server) proxyOwnedBoxBodyRequest(w http.ResponseWriter, r *http.Request, box *sandbox.Sandbox, body []byte) bool {
	if box == nil || !s.shouldProxy(*box) {
		return false
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	s.proxyToWorker(w, r, box.WorkerAddr)
	return true
}

func (s *Server) proxyOwnedBoxRequest(w http.ResponseWriter, r *http.Request, box *sandbox.Sandbox) bool {
	if box == nil || !s.shouldProxy(*box) {
		return false
	}

	s.proxyToWorker(w, r, box.WorkerAddr)
	return true
}

func (s *Server) authorizeSandboxOrganization(ctx context.Context, w http.ResponseWriter, sandboxID string, organizationID string) (*sandbox.Sandbox, bool) {
	if strings.TrimSpace(sandboxID) == "" {
		return nil, true
	}

	box, err := s.lifecycle.Get(sandboxID)
	if err != nil {
		writeKnownError(w, err)
		return nil, false
	}

	requestedOrganizationID := strings.TrimSpace(organizationID)
	if requestedOrganizationID == "" || requestedOrganizationID == box.OrganizationID {
		return box, true
	}

	s.recordPolicyDenied(ctx, sandboxID, "cross_organization_sandbox_access_denied", "sandbox does not belong to organization", map[string]any{
		"organization_id":           box.OrganizationID,
		"requested_organization_id": requestedOrganizationID,
	})
	writeEnvelopeWithMessage(w, http.StatusForbidden, -403, "sandbox does not belong to organization", map[string]any{
		"error_type":      "access_denied",
		"code":            "cross_organization_sandbox_access_denied",
		"sandbox_id":      sandboxID,
		"organization_id": requestedOrganizationID,
	})
	return nil, false
}

func requestOrganizationID(r *http.Request, bodyOrganizationID string) string {
	if value := strings.TrimSpace(bodyOrganizationID); value != "" {
		return value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("organization_id")); value != "" {
		return value
	}
	for _, header := range []string{"X-ZGI-Organization-ID", "X-Organization-ID"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) recordPolicyDenied(ctx context.Context, sandboxID string, code string, message string, metadata map[string]any) {
	eventMetadata := map[string]any{
		"status":     "failure",
		"error_type": "policy_denied",
		"code":       code,
	}
	for key, value := range metadata {
		eventMetadata[key] = value
	}
	s.observer.Record("policy.denied", sandboxID, message, observer.MetadataWithContext(ctx, eventMetadata))
}

func (s *Server) shouldProxy(box sandbox.Sandbox) bool {
	return box.WorkerID != "" && box.WorkerID != s.config.WorkerID && box.WorkerAddr != ""
}

func (s *Server) proxyToWorker(w http.ResponseWriter, r *http.Request, destination string) {
	s.proxyToTarget(w, r, destination, r.URL.Path)
}

func (s *Server) proxyToTarget(w http.ResponseWriter, r *http.Request, destination string, path string) {
	target, err := url.Parse(destination)
	if err != nil {
		writeEnvelopeWithMessage(w, http.StatusBadGateway, -502, fmt.Sprintf("invalid proxy target: %v", err), nil)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		writeEnvelopeWithMessage(w, http.StatusBadGateway, -502, fmt.Sprintf("proxy request failed: %v", err), nil)
	}
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.URL.Path = path
		req.URL.RawQuery = r.URL.RawQuery
		req.Host = target.Host
	}
	proxy.ServeHTTP(w, r)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.config.APIKey == "" {
		return true
	}

	return r.Header.Get("X-API-Key") == s.config.APIKey
}

func (s *Server) readLimitedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 1 << 20
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeEnvelopeWithMessage(w, http.StatusRequestEntityTooLarge, -413, "request body too large", nil)
			return nil, err
		}
		writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "failed to read request body", nil)
		return nil, err
	}
	return body, nil
}

func (s *Server) maxExecutionRequestBytes() int64 {
	return maxInt64(4*1024*1024, s.maxFileSizeBytes()*2+64*1024)
}

func (s *Server) maxSmallJSONRequestBytes() int64 {
	return 1 << 20
}

func (s *Server) maxFileUploadRequestBytes() int64 {
	return s.maxFileSizeBytes()*2 + 64*1024
}

func (s *Server) maxArchiveUploadRequestBytes() int64 {
	return s.maxFileSizeBytes()*256*2 + 64*1024
}

func (s *Server) maxFileSizeBytes() int64 {
	if s.config.MaxFileSizeKB <= 0 {
		return 256 * 1024
	}
	return int64(s.config.MaxFileSizeKB) * 1024
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeEnvelopeWithMessage(w, http.StatusRequestEntityTooLarge, -413, "request body too large", nil)
		return
	}
	writeEnvelopeWithMessage(w, http.StatusBadRequest, -400, "invalid request payload", nil)
}

func writeEnvelope(w http.ResponseWriter, status int, data any) {
	writeEnvelopeWithMessage(w, status, 0, "success", data)
}

func writeEnvelopeWithMessage(w http.ResponseWriter, status int, code int, message string, data any) {
	writeJSON(w, status, map[string]any{
		"code":    code,
		"message": message,
		"data":    data,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeKnownError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := -400
	var data any
	var limitErr *policy.LimitError
	var queueErr *runner.QueueTimeoutError
	var cancelErr *runner.CancellationError
	switch {
	case errors.As(err, &limitErr):
		status = http.StatusTooManyRequests
		code = -429
		data = limitErr.ResponseDetails()
	case errors.As(err, &queueErr):
		status = http.StatusTooManyRequests
		code = -429
		data = queueErr.ResponseDetails()
	case errors.As(err, &cancelErr):
		status = statusClientClosedRequest
		code = -499
		data = cancelErr.ResponseDetails()
	case errors.Is(err, context.Canceled):
		status = statusClientClosedRequest
		code = -499
		data = (&runner.CancellationError{Phase: "request"}).ResponseDetails()
	case errors.Is(err, strconv.ErrSyntax):
		status = http.StatusBadRequest
	case strings.Contains(err.Error(), "not found"):
		status = http.StatusNotFound
	case strings.Contains(err.Error(), "expired"):
		status = http.StatusGone
	default:
		status = http.StatusBadRequest
	}
	writeEnvelopeWithMessage(w, status, code, err.Error(), data)
}

func parseFileManifestOptions(r *http.Request) (executor.FileManifestOptions, error) {
	var options executor.FileManifestOptions
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("max_files")); raw != "" {
		options.MaxFiles, err = strconv.Atoi(raw)
		if err != nil || options.MaxFiles <= 0 {
			return options, fmt.Errorf("max_files must be a positive integer")
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("max_total_bytes")); raw != "" {
		options.MaxTotalBytes, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || options.MaxTotalBytes <= 0 {
			return options, fmt.Errorf("max_total_bytes must be a positive integer")
		}
	}
	return options, nil
}

func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
