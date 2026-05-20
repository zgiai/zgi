package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/manager"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/registry"
)

// HTTPServer exposes a lightweight REST API to control plugin sessions.
type HTTPServer struct {
	cfg    *config.Config
	mgr    *manager.Manager
	reg    *registry.Registry
	log    *zap.Logger
	engine *gin.Engine
	server *http.Server

	rateMu      sync.Mutex
	reqCounters map[string]int
	windowStart time.Time
}

// New constructs the HTTP server and routes.
func New(cfg *config.Config, mgr *manager.Manager, reg *registry.Registry, log *zap.Logger) *HTTPServer {
	gin.SetMode(gin.DebugMode)
	r := gin.New()
	r.Use(gin.Recovery())

	s := &HTTPServer{
		cfg:         cfg,
		mgr:         mgr,
		reg:         reg,
		log:         log,
		engine:      r,
		reqCounters: make(map[string]int),
		windowStart: time.Now(),
		server: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: r,
		},
	}

	r.Use(s.apiKeyMiddleware())
	r.Use(s.rateLimitMiddleware())

	r.GET("/healthz", s.handleHealth)
	v1 := r.Group("/api/v1")
	{
		// Register plugin
		v1.POST("/plugins", s.handleCreatePlugin)
		// Install plugin
		v1.POST("/plugins/:id/install", s.handleInstallByID)
		// Check installed plugins
		v1.GET("/plugins/installed", s.handleListInstalled)
		// Start session
		v1.POST("/sessions", s.handleLaunch)
		v1.POST("/sessions/:id/stop", s.handleStop)

		v1.GET("/sessions/:id", s.handleGet)
		v1.GET("/sessions", s.handleList)
		v1.GET("/plugins", s.handleListPlugins)
		v1.GET("/plugins/:id", s.handleGetPlugin)
		// Delete plugin by ID. Primary ID is marketplace_version_id,
		// with compatibility support for author:name:version.
		v1.DELETE("/plugins/:id", s.handleDeletePlugin)

		// NOTE: Tenant binding routes removed.
		// Permission decisions are now managed by zgi-api-go (TenantPluginSubscription).
		// Runner trusts the caller for all operations.
	}

	// Register invoke routes for plugin communication
	s.registerInvokeRoutes(v1)

	return s
}

// Start runs the HTTP server.
func (s *HTTPServer) Start(_ context.Context) error {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("http server error", zap.Error(err))
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *HTTPServer) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}

func (s *HTTPServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *HTTPServer) handleLaunch(c *gin.Context) {
	var payload launchPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	if payload.Language == "" {
		payload.Language = string(plugin.LanguagePython)
	}

	// Resolve tenant ID: prefer Header, then Body
	tenantID := payload.TenantID
	if tenantID == 0 {
		if tidHeader := c.GetHeader("X-Tenant-ID"); tidHeader != "" {
			if tid, err := strconv.ParseUint(tidHeader, 10, 64); err == nil {
				tenantID = uint(tid)
			}
		}
	}

	workflowRunID := strings.TrimSpace(payload.WorkflowRunID)
	if workflowRunID == "" {
		workflowRunID = strings.TrimSpace(c.GetHeader("X-Workflow-Run-ID"))
	}
	sessionPolicy := strings.TrimSpace(payload.SessionPolicy)
	if sessionPolicy == "" {
		sessionPolicy = strings.TrimSpace(c.GetHeader("X-Session-Policy"))
	}
	sessionIdleTTLSeconds := payload.SessionIdleTTLSeconds
	if sessionIdleTTLSeconds <= 0 {
		sessionIdleTTLSeconds = readPositiveIntHeader(c.GetHeader("X-Session-Idle-TTL-Seconds"))
	}
	sessionMaxLifetimeSeconds := payload.SessionMaxLifetimeSeconds
	if sessionMaxLifetimeSeconds <= 0 {
		sessionMaxLifetimeSeconds = readPositiveIntHeader(c.GetHeader("X-Session-Max-Lifetime-Seconds"))
	}

	manifest := plugin.Manifest{
		ID:      payload.ID,
		Name:    payload.Name,
		Version: payload.Version,
		Runner: plugin.Runner{
			Language:   plugin.Language(payload.Language),
			Entrypoint: payload.Entrypoint,
		},
	}

	snap, err := s.mgr.Launch(c.Request.Context(), manager.LaunchRequest{
		Manifest:                  manifest,
		WorkingDir:                payload.WorkingDir,
		Env:                       payload.Env,
		Args:                      payload.Args,
		TenantID:                  tenantID,
		WorkflowRunID:             workflowRunID,
		SessionPolicy:             sessionPolicy,
		SessionIdleTTLSeconds:     sessionIdleTTLSeconds,
		SessionMaxLifetimeSeconds: sessionMaxLifetimeSeconds,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, snap)
}

func (s *HTTPServer) handleList(c *gin.Context) {
	c.JSON(http.StatusOK, s.mgr.List())
}

func (s *HTTPServer) handleGetPlugin(c *gin.Context) {
	id := c.Param("id")
	manifest, err := s.reg.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %s not found", id)})
		return
	}
	c.JSON(http.StatusOK, manifest)
}

func (s *HTTPServer) handleDeletePlugin(c *gin.Context) {
	roleVal, exists := c.Get(string(ctxRoleKey))
	if !exists || !requireAdmin(roleVal.(requestRole)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	id := c.Param("id")

	lookupBy := "unknown"
	resolvedMarketplaceVersionID := ""
	resolvedExternalID := ""
	if rec, resolveErr := s.reg.ResolvePluginRecord(c.Request.Context(), id); resolveErr == nil && rec != nil {
		resolvedMarketplaceVersionID = rec.MarketplaceVersionID
		resolvedExternalID = rec.ExternalID
		switch {
		case id == rec.MarketplaceVersionID:
			lookupBy = "marketplace_version_id"
		case id == rec.ExternalID:
			lookupBy = "external_id"
		default:
			lookupBy = "fallback"
		}
	}

	manifest, err := s.reg.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %s not found", id)})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if err := s.reg.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.log.Info(
		"deleting plugin",
		zap.String("input_id", id),
		zap.String("resolved_marketplace_version_id", resolvedMarketplaceVersionID),
		zap.String("resolved_external_id", resolvedExternalID),
		zap.String("lookup_by", lookupBy),
	)
	// FIXME: s.mgr.Uninstall parameter needs to be changed, and the author is missing.
	// Best-effort uninstall from local storage.
	if err := s.mgr.Uninstall(ctx, manifest.Name, manifest.Version); err != nil {
		s.log.Warn("uninstall plugin failed", zap.String("id", id), zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (s *HTTPServer) handleInstallByID(c *gin.Context) {
	roleVal, exists := c.Get(string(ctxRoleKey))
	if !exists || !requireAdmin(roleVal.(requestRole)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	id := c.Param("id")
	manifest, err := s.reg.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %s not found", id)})
		return
	}

	var pkgBytes []byte
	var forceOverwrite bool
	contentType := c.ContentType()

	// Priority 1: multipart/form-data file upload
	if strings.HasPrefix(contentType, "multipart/") {
		// Try to read uploaded file (prefer "file" then "package")
		fileHeader, err := c.FormFile("file")
		if err != nil {
			var packageErr error
			fileHeader, packageErr = c.FormFile("package")
			if packageErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "multipart request requires 'file' or 'package' field",
					"hint":  "use multipart/form-data with field name 'file' or 'package'",
				})
				return
			}
		}

		// Check fileHeader is not nil before using it
		if fileHeader == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "uploaded file header is empty",
				"hint":  "ensure the file field contains a valid file",
			})
			return
		}

		pkgBytes, err = readUploadedFile(fileHeader)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("read uploaded package: %v", err)})
			return
		}

		// Read force flag from form field
		forceStr := c.PostForm("force")
		forceOverwrite = forceStr == "true" || forceStr == "1"

		s.log.Info("plugin package uploaded via multipart",
			zap.String("plugin_id", id),
			zap.String("filename", fileHeader.Filename),
			zap.Int64("size", fileHeader.Size),
			zap.Bool("force", forceOverwrite))
	} else {
		// Priority 2: JSON with base64 encoded package (backward compatibility)
		var payload installPackagePayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid JSON payload: %v", err),
				"hint":  "use application/json with 'package_b64' field or multipart/form-data with 'file' field",
			})
			return
		}
		if payload.PackageBase64 == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "package_b64 is required in JSON mode",
				"hint":  "provide 'package_b64' field or use multipart/form-data upload",
			})
			return
		}
		pkgBytes, err = base64.StdEncoding.DecodeString(payload.PackageBase64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid base64 encoding: %v", err)})
			return
		}
		forceOverwrite = payload.Force
		s.log.Info("plugin package uploaded via base64",
			zap.String("plugin_id", id),
			zap.Int("size", len(pkgBytes)),
			zap.Bool("force", forceOverwrite))
	}

	if len(pkgBytes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package is empty"})
		return
	}

	installation, err := s.mgr.Install(c.Request.Context(), manager.InstallRequest{
		Manifest: *manifest,
		Package:  pkgBytes,
		Operator: actorFromContext(c),
		Source:   "api",
		Force:    forceOverwrite,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, installation)
}

func (s *HTTPServer) handleListInstalled(c *gin.Context) {
	c.JSON(http.StatusOK, s.mgr.ListInstalled())
}

func (s *HTTPServer) handleCreatePlugin(c *gin.Context) {
	var payload createPluginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid payload: %v", err)})
		return
	}

	if payload.Manifest.Runner.Language == "" {
		payload.Manifest.Runner.Language = plugin.LanguagePython
	}

	if err := payload.Manifest.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	saved, err := s.reg.Save(c.Request.Context(), payload.Manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, saved)
}

func (s *HTTPServer) handleGet(c *gin.Context) {
	id := c.Param("id")
	snap, ok := s.mgr.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("session %s not found", id)})
		return
	}
	c.JSON(http.StatusOK, snap)
}

func (s *HTTPServer) handleStop(c *gin.Context) {
	id := c.Param("id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := s.mgr.Stop(ctx, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}

func (s *HTTPServer) handleListPlugins(c *gin.Context) {
	manifests, err := s.reg.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, manifests)
}

// NOTE: Tenant binding handlers removed.
// handleCreateTenant, handleListPluginTenants, handleEnableTenantBinding,
// handleDisableTenantBinding, handleToggleTenantBinding have been deprecated.
// Permission decisions are now managed by zgi-api-go (TenantPluginSubscription).

type launchPayload struct {
	ID                        string            `json:"id"`
	Name                      string            `json:"name"`
	Version                   string            `json:"version"`
	Language                  string            `json:"language"`
	Entrypoint                string            `json:"entrypoint"`
	WorkingDir                string            `json:"working_dir"`
	Args                      []string          `json:"args"`
	Env                       map[string]string `json:"env"`
	TenantID                  uint              `json:"tenant_id"` // Optional: for multi-tenant mode
	WorkflowRunID             string            `json:"workflow_run_id"`
	SessionPolicy             string            `json:"session_policy"`
	SessionIdleTTLSeconds     int               `json:"session_idle_ttl_seconds"`
	SessionMaxLifetimeSeconds int               `json:"session_max_lifetime_seconds"`
}

type installPayload struct {
	Manifest      plugin.Manifest `json:"manifest" form:"manifest"`
	PackageBase64 string          `json:"package_b64" form:"package_b64"`
}

type installPackagePayload struct {
	PackageBase64 string `json:"package_b64" form:"package_b64"`
	Force         bool   `json:"force" form:"force"`
}

type createPluginPayload struct {
	Manifest plugin.Manifest `json:"manifest"`
}

type createTenantPayload struct {
	Name string `json:"name"`
}

type requestRole string

func readUploadedFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer file.Close()
	return io.ReadAll(file)
}

const (
	roleAdmin    requestRole = "admin"
	roleReadonly requestRole = "readonly"
	roleNone     requestRole = "none"
)

type ctxKey string

const (
	ctxRoleKey   ctxKey = "role"
	ctxTenantKey ctxKey = "tenant"
	ctxActorKey  ctxKey = "actor"
)

func (s *HTTPServer) apiKeyMiddleware() gin.HandlerFunc {
	adminKeys := parseKeys(s.cfg.APIKey, s.cfg.AdminAPIKeys)
	roKeys := parseKeys("", s.cfg.ReadonlyAPIKeys)
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/healthz" {
			c.Next()
			return
		}
		role := authenticateRequest(c.Request, adminKeys, roKeys)
		if role == roleNone {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		if role == roleReadonly && c.Request.Method != http.MethodGet {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "readonly"})
			return
		}
		tenant := c.GetHeader("X-Tenant-ID")
		if tenant == "" {
			tenant = "default"
		}
		c.Set(string(ctxRoleKey), role)
		c.Set(string(ctxTenantKey), tenant)
		c.Set(string(ctxActorKey), c.GetHeader("X-Actor"))
		ctx := manager.WithTenant(c.Request.Context(), tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func parseKeys(single string, csv string) []string {
	var keys []string
	if strings.TrimSpace(single) != "" {
		keys = append(keys, strings.TrimSpace(single))
	}
	for _, k := range strings.Split(csv, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

func readPositiveIntHeader(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func authenticateRequest(r *http.Request, adminKeys, roKeys []string) requestRole {
	token := r.Header.Get("X-API-Key")
	if token == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return roleNone
	}
	for _, k := range adminKeys {
		if token == k {
			return roleAdmin
		}
	}
	for _, k := range roKeys {
		if token == k {
			return roleReadonly
		}
	}
	return roleNone
}

func (s *HTTPServer) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg.RateLimitPerMinute <= 0 && s.cfg.TenantRateLimitPerMinute <= 0 {
			c.Next()
			return
		}
		tenant, exists := c.Get(string(ctxTenantKey))
		if !exists {
			tenant = "default"
		}
		key := fmt.Sprintf("%s:%s", tenant.(string), c.Request.URL.Path)
		if exceeded := ginRateLimit(c, s.cfg, s.log, key); exceeded {
			return
		}
		c.Next()
	}
}

func requireAdmin(role requestRole) bool {
	return role == roleAdmin
}

func actorFromContext(c *gin.Context) string {
	actor := c.GetHeader("X-Actor")
	if strings.TrimSpace(actor) != "" {
		return actor
	}
	return "system"
}
