package handler

import (
	"encoding/base64"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/dto"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/repository"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// Plugin Runner error codes (module 15)
var (
	ErrPluginRunnerListFailed     = response.ErrorCode{Code: 215001, Message: "Failed to list plugins", UserVisible: true}
	ErrPluginRunnerGetFailed      = response.ErrorCode{Code: 215002, Message: "Failed to get plugin", UserVisible: true}
	ErrPluginRunnerRegisterFailed = response.ErrorCode{Code: 215003, Message: "Failed to register plugin", UserVisible: true}
	ErrPluginRunnerDeleteFailed   = response.ErrorCode{Code: 215004, Message: "Failed to delete plugin", UserVisible: true}
	ErrPluginRunnerInstallFailed  = response.ErrorCode{Code: 215005, Message: "Failed to install plugin", UserVisible: true}
	ErrPluginRunnerSessionFailed  = response.ErrorCode{Code: 215007, Message: "Failed to manage session", UserVisible: true}
	ErrPluginRunnerAccessDenied   = response.ErrorCode{Code: 215009, Message: "Access denied to this plugin", UserVisible: true}
	ErrInvalidRequest             = response.ErrorCode{Code: 115001, Message: "Invalid request", UserVisible: true}
)

// Handler handles plugin runner API requests
type Handler struct {
	service             service.PluginRunnerService
	installService      service.AccountInstallationService
	infoRepo            repository.InstalledPluginInfoRepository
	accountService      interfaces.AccountService
	subscriptionService service.MemberSubscriptionService
}

// NewHandler creates a new handler
func NewHandler(
	svc service.PluginRunnerService,
	installService service.AccountInstallationService,
	infoRepo repository.InstalledPluginInfoRepository,
	accountService interfaces.AccountService,
	subscriptionService service.MemberSubscriptionService,
) *Handler {
	return &Handler{
		service:             svc,
		installService:      installService,
		infoRepo:            infoRepo,
		accountService:      accountService,
		subscriptionService: subscriptionService,
	}
}

// ListPlugins lists all registered plugins
// GET /workspaces/current/plugin-runner/management/plugins
func (h *Handler) ListPlugins(c *gin.Context) {
	plugins, err := h.service.ListPlugins(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerListFailed, err.Error())
		return
	}

	result := make([]dto.PluginResponse, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, dto.PluginResponse{
			ID:         p.ID,
			Name:       p.Name,
			Version:    p.Version,
			Language:   p.Runner.Language,
			Entrypoint: p.Runner.Entrypoint,
		})
	}
	response.Success(c, result)
}

// GetPlugin gets a specific plugin by ID
// GET /workspaces/current/plugin-runner/management/plugins/:id
func (h *Handler) GetPlugin(c *gin.Context) {
	pluginID := c.Param("id")
	plugin, err := h.service.GetPlugin(c.Request.Context(), pluginID)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerGetFailed, err.Error())
		return
	}

	response.Success(c, dto.PluginResponse{
		ID:         plugin.ID,
		Name:       plugin.Name,
		Version:    plugin.Version,
		Language:   plugin.Runner.Language,
		Entrypoint: plugin.Runner.Entrypoint,
	})
}

// RegisterPlugin registers a new plugin
// POST /workspaces/current/plugin-runner/management/plugins
func (h *Handler) RegisterPlugin(c *gin.Context) {
	var req dto.RegisterPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, ErrInvalidRequest, err.Error())
		return
	}

	manifest := model.PluginManifest{
		Name:        req.Name,
		Version:     req.Version,
		Description: req.Description,
		Author:      req.Author,
		Tags:        req.Tags,
		Runner: model.PluginRunner{
			Language:   req.Language,
			Entrypoint: req.Entrypoint,
		},
	}

	plugin, err := h.service.RegisterPlugin(c.Request.Context(), manifest)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerRegisterFailed, err.Error())
		return
	}

	response.Success(c, dto.PluginResponse{
		ID:         plugin.ID,
		Name:       plugin.Name,
		Version:    plugin.Version,
		Language:   plugin.Runner.Language,
		Entrypoint: plugin.Runner.Entrypoint,
	})
}

// DeletePlugin removes the organization installation for a plugin version.
// DELETE /workspaces/current/plugin-runner/management/plugins/:id
// The plugin package remains in the runner unless no organization references it.
func (h *Handler) DeletePlugin(c *gin.Context) {
	// The primary API semantics use marketplace_version_id.
	// During compatibility window, author:name:version is still accepted and normalized internally.
	marketplaceVersionIdentifier := c.Param("id")
	organizationID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}

	if err := h.removeOrganizationInstallation(c.Request.Context(), organizationID, marketplaceVersionIdentifier); err != nil {
		if errors.Is(err, errInstallationNotFound) {
			response.Fail(c, ErrPluginRunnerAccessDenied)
			return
		}
		response.FailWithMessage(c, ErrPluginRunnerDeleteFailed, err.Error())
		return
	}

	response.Success(c, nil)
}

// InstallPlugin installs a plugin package (file upload)
// POST /workspaces/current/plugin-runner/management/plugins/:id/install
func (h *Handler) InstallPlugin(c *gin.Context) {
	pluginID := c.Param("id")
	forceStr := c.PostForm("force")
	force := forceStr == "true"

	file, err := c.FormFile("package")
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, "Failed to get package file: "+err.Error())
		return
	}

	f, err := file.Open()
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, "Failed to open package file: "+err.Error())
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, "Failed to read package file: "+err.Error())
		return
	}

	installation, err := h.service.InstallPlugin(c.Request.Context(), pluginID, content, force)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, err.Error())
		return
	}

	response.Success(c, dto.InstallationResponse{
		Name:            installation.Manifest.Name,
		Version:         installation.Manifest.Version,
		Description:     installation.Manifest.Description,
		Path:            installation.Path,
		InstalledAt:     installation.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
		PackageChecksum: installation.PackageChecksum,
	})
}

// InstallPluginBase64 installs a plugin package (base64 encoded)
// POST /workspaces/current/plugin-runner/management/plugins/:id/install-base64
func (h *Handler) InstallPluginBase64(c *gin.Context) {
	pluginID := c.Param("id")

	var req dto.InstallPluginBase64Request
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, ErrInvalidRequest, err.Error())
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.PackageBase64)
	if err != nil {
		response.FailWithMessage(c, ErrInvalidRequest, "Invalid base64 content: "+err.Error())
		return
	}

	installation, err := h.service.InstallPlugin(c.Request.Context(), pluginID, content, req.Force)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, err.Error())
		return
	}

	response.Success(c, dto.InstallationResponse{
		Name:            installation.Manifest.Name,
		Version:         installation.Manifest.Version,
		Description:     installation.Manifest.Description,
		Path:            installation.Path,
		InstalledAt:     installation.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
		PackageChecksum: installation.PackageChecksum,
	})
}

// InstallFromMarketplace downloads plugin from Marketplace and installs to Runner
// POST /workspaces/current/plugin-runner/management/plugins/install-from-marketplace
// TODO: After successful installation, consider whether to automatically create a subscription record for the organization. No rollback is needed on installation failure (currently subscription and installation are separate).
func (h *Handler) InstallFromMarketplace(c *gin.Context) {
	var req dto.InstallFromMarketplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, ErrInvalidRequest, err.Error())
		return
	}

	if req.PluginID == "" || req.VersionID == "" {
		response.FailWithMessage(c, ErrInvalidRequest, "plugin_id and version_id are required")
		return
	}

	organizationID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}
	userID := c.GetString("account_id")

	installation, err := h.service.InstallFromMarketplace(c.Request.Context(), organizationID, req.PluginID, req.VersionID, userID, req.Force)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, err.Error())
		return
	}

	response.Success(c, dto.InstallationResponse{
		ID:              req.PluginID,  // Return marketplace plugin UUID
		VersionID:       req.VersionID, // Return marketplace version UUID
		Name:            installation.Manifest.Name,
		Version:         installation.Manifest.Version,
		Description:     installation.Manifest.Description,
		Path:            installation.Path,
		InstalledAt:     installation.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
		PackageChecksum: installation.PackageChecksum,
	})
}

// ReinstallFromMarketplace reinstalls plugin from Marketplace and updates declaration
// POST /workspaces/current/plugin-runner/management/plugins/reinstall-from-marketplace
func (h *Handler) ReinstallFromMarketplace(c *gin.Context) {
	var req dto.InstallFromMarketplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, ErrInvalidRequest, err.Error())
		return
	}

	if req.PluginID == "" || req.VersionID == "" {
		response.FailWithMessage(c, ErrInvalidRequest, "plugin_id and version_id are required")
		return
	}

	organizationID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return
	}
	userID := c.GetString("account_id")

	installation, err := h.service.ReinstallFromMarketplace(c.Request.Context(), organizationID, req.PluginID, req.VersionID, userID)
	if err != nil {
		if errors.Is(err, service.ErrPluginInstallationNotFound) {
			response.Success(c, dto.ReinstallFromMarketplaceResponse{Status: "not_installed"})
			return
		}
		response.FailWithMessage(c, ErrPluginRunnerInstallFailed, err.Error())
		return
	}

	resp := dto.ReinstallFromMarketplaceResponse{Status: "reinstalled"}
	if installation != nil {
		resp.Installation = &dto.InstallationResponse{
			ID:              req.PluginID,
			VersionID:       req.VersionID,
			Name:            installation.Manifest.Name,
			Version:         installation.Manifest.Version,
			Description:     installation.Manifest.Description,
			Path:            installation.Path,
			InstalledAt:     installation.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
			PackageChecksum: installation.PackageChecksum,
		}
	}

	response.Success(c, resp)
}

// ListInstalledPlugins lists all installed plugins
// GET /workspaces/current/plugin-runner/management/plugins/installed
func (h *Handler) ListInstalledPlugins(c *gin.Context) {
	if h.installService != nil && h.infoRepo != nil {
		organizationID, err := getOrganizationIDFromContext(c, h.accountService)
		if err != nil {
			response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
			return
		}
		result, err := h.listOrganizationInstallations(c.Request.Context(), organizationID)
		if err != nil {
			response.FailWithMessage(c, ErrPluginRunnerListFailed, err.Error())
			return
		}
		response.Success(c, result)
		return
	}

	installations, err := h.service.ListInstalledPlugins(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerListFailed, err.Error())
		return
	}

	result := make([]dto.InstallationResponse, 0, len(installations))
	for _, i := range installations {
		result = append(result, dto.InstallationResponse{
			ID:              i.Manifest.MarketplacePluginID,
			VersionID:       i.Manifest.MarketplaceVersionID,
			Name:            i.Manifest.Name,
			Version:         i.Manifest.Version,
			Description:     i.Manifest.Description,
			Path:            i.Path,
			InstalledAt:     i.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
			PackageChecksum: i.PackageChecksum,
		})
	}
	response.Success(c, result)
}

// ListSessions lists all active sessions
// GET /workspaces/current/plugin-runner/sessions
func (h *Handler) ListSessions(c *gin.Context) {
	sessions, err := h.service.ListSessions(c.Request.Context())
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerSessionFailed, err.Error())
		return
	}

	result := make([]dto.SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		resp := dto.SessionResponse{
			ID:         s.ID,
			PluginName: s.Manifest.Name,
			Version:    s.Manifest.Version,
			Status:     string(s.Status),
			StartedAt:  s.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
			PID:        s.PID,
		}
		if s.FinishedAt != nil {
			resp.FinishedAt = s.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		result = append(result, resp)
	}
	response.Success(c, result)
}

// StopSession stops a running session
// POST /workspaces/current/plugin-runner/sessions/:id/stop
func (h *Handler) StopSession(c *gin.Context) {
	sessionID := c.Param("id")
	if err := h.service.StopSession(c.Request.Context(), sessionID); err != nil {
		response.FailWithMessage(c, ErrPluginRunnerSessionFailed, err.Error())
		return
	}
	response.Success(c, nil)
}

// TenantListPlugins lists plugins installed for the current organization.
// GET /workspaces/current/plugin-runner/plugins
func (h *Handler) TenantListPlugins(c *gin.Context) {
	organizationID, _, ok := h.requireOrganizationMember(c)
	if !ok {
		return
	}

	result, err := h.listOrganizationPlugins(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerListFailed, err.Error())
		return
	}

	response.Success(c, result)
}

// TenantGetPlugin gets a specific plugin if the organization has installed it.
// GET /workspaces/current/plugin-runner/plugins/:id
func (h *Handler) TenantGetPlugin(c *gin.Context) {
	pluginID := c.Param("id")

	organizationID, _, ok := h.requireOrganizationMember(c)
	if !ok {
		return
	}

	installation, info, err := h.resolveInstallationInfo(c.Request.Context(), organizationID, pluginID)
	if err != nil {
		if errors.Is(err, errInstallationNotFound) {
			response.Fail(c, ErrPluginRunnerAccessDenied)
			return
		}
		response.FailWithMessage(c, ErrPluginRunnerGetFailed, err.Error())
		return
	}

	resp := buildTenantPluginResponse(*info, installation.Status)
	response.Success(c, resp)
}

func (h *Handler) RegisterTenantRoutes(router *gin.RouterGroup, accountService interfaces.AccountService, tenantService interfaces.WorkspaceManagementService, subHandler *SubscriptionHandler) {
	workspaces := router.Group("/workspaces",
		middleware.SetAccountService(accountService),
		middleware.SetWorkspaceManagementService(tenantService),
		middleware.JWTWithTenant(),
	)
	{
		current := workspaces.Group("/current")
		{
			pluginRunner := current.Group("/plugin-runner")
			{
				pluginRunner.GET("/plugins", h.TenantListPlugins)
				pluginRunner.GET("/plugins/:id", h.TenantGetPlugin)
				subHandler.RegisterRoutes(pluginRunner)
			}
		}
	}
}
