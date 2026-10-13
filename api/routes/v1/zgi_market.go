package v1

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/zgimarket"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// RegisterZGIMarketRoutes registers the market routes of an organization's members.
// Path: /workspaces/current/zgi-market/*
// A request is forwarded to the console's market proxy as the caller's current organization.
// importer, when given, lets administrators install a market skill into the organization's skills.
func RegisterZGIMarketRoutes(
	router *gin.RouterGroup,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	importer marketSkillImporter,
	marketTools *zgimarket.Tools,
) {
	settings := zgimarket.Config{}
	if config.GlobalConfig != nil {
		settings.ConsoleURL = config.GlobalConfig.Console.APIURL
		settings.InternalKey = config.GlobalConfig.Console.InternalAPIKey
		settings.GatewayURL = config.GlobalConfig.Console.ZGIMarketGatewayURL
		settings.OAuthRedirectURL = config.GlobalConfig.Console.ZGIMarketOAuthRedirectURL
		if settings.OAuthRedirectURL != "" && !zgimarket.ValidOAuthRedirectURL(settings.OAuthRedirectURL) {
			logger.Error("ZGI_MARKET_OAUTH_REDIRECT_URL is ignored", fmt.Errorf(
				"it must be an absolute https URL without credentials, query, or fragment"))
		}
	}
	proxy := zgimarket.NewProxy(
		settings,
		func(c *gin.Context) string { return util.GetOrganizationID(c) },
		func(c *gin.Context) string { return c.GetString("account_id") },
		func(c *gin.Context, organizationID, accountID string) (bool, bool, error) {
			member, err := accountService.IsOrganizationMember(c.Request.Context(), organizationID, accountID)
			if err != nil || !member {
				return false, false, err
			}
			admin, err := accountService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
			return true, admin, err
		},
	)
	if importer != nil {
		proxy.WithSkillInstaller(marketSkillInstaller{skills: importer})
	}
	proxy.WithTools(marketTools)
	market := router.Group("/workspaces/current/zgi-market",
		middleware.SetAccountService(accountService),
		middleware.SetWorkspaceManagementService(tenantService),
		// The market belongs to the organization, not to a workspace: a member whose account has
		// no current workspace, as in the personal space, still reaches it.
		middleware.JWT(),
		setOrganizationContext(accountService),
	)
	market.Any("/*path", proxy.Forward)
}

// marketSkillOrigin is the origin recorded on a skill installed from ZGI Market.
const marketSkillOrigin = "zgi_market"

// marketSkillImporter is the part of the chat runtime that imports skill packages.
type marketSkillImporter interface {
	ImportCustomSkillPackage(ctx context.Context, scope runtimeservice.Scope, data []byte, filename string, origin runtimeservice.SkillPackageOrigin, overwrite bool) (*runtimeservice.SkillImportPreview, *skills.SkillDiscoveryMetadata, error)
	ListInstalledSkillPackages(ctx context.Context, scope runtimeservice.Scope, source string) ([]runtimeservice.InstalledSkillPackage, error)
	GetSkillConfig(ctx context.Context, scope runtimeservice.Scope) (*runtimeservice.SkillConfig, error)
	UpdateSkillConfig(ctx context.Context, scope runtimeservice.Scope, req runtimedto.UpdateSkillConfigRequest) (*runtimeservice.SkillConfig, error)
}

// marketSkillInstaller imports a market skill package as an organization skill.
type marketSkillInstaller struct {
	skills marketSkillImporter
}

func marketSkillScope(organizationID, accountID string) (runtimeservice.Scope, error) {
	organization, err := uuid.Parse(organizationID)
	if err != nil {
		return runtimeservice.Scope{}, fmt.Errorf("invalid organization: %w", err)
	}
	account, err := uuid.Parse(accountID)
	if err != nil {
		return runtimeservice.Scope{}, fmt.Errorf("invalid account: %w", err)
	}
	return runtimeservice.Scope{OrganizationID: organization, AccountID: account}, nil
}

func (i marketSkillInstaller) Install(
	ctx context.Context, organizationID, accountID string, pkg zgimarket.SkillPackage, overwrite bool,
) (*zgimarket.SkillInstall, error) {
	scope, err := marketSkillScope(organizationID, accountID)
	if err != nil {
		return nil, err
	}
	preview, installed, err := i.skills.ImportCustomSkillPackage(ctx, scope, pkg.Data, pkg.Filename, runtimeservice.SkillPackageOrigin{
		Source: marketSkillOrigin, Reference: pkg.Slug, Version: pkg.Version, PackageSHA256: pkg.SHA256,
	}, overwrite)
	if errors.Is(err, runtimeservice.ErrInvalidInput) {
		// The import refused the package; the administrator is told why.
		return &zgimarket.SkillInstall{Version: pkg.Version, ValidationErrors: []string{err.Error()}}, nil
	}
	if err != nil {
		return nil, err
	}
	result := &zgimarket.SkillInstall{Version: pkg.Version}
	if preview != nil {
		result.Warnings, result.ValidationErrors = preview.Warnings, preview.ValidationErrors
	}
	switch {
	case installed != nil:
		result.Installed, result.SkillID, result.Name = true, installed.ID, installed.Name
		// Installing is the administrator's decision to use the skill, so a new one is switched on
		// for the organization. One that was installed before keeps the setting it has.
		if preview == nil || !preview.WillOverwrite {
			result.Enabled = i.enable(ctx, scope, installed.ID)
		} else {
			result.Enabled = installed.Enabled
		}
	case preview != nil && preview.CanImport && preview.ExistingSkill != nil:
		result.Conflict = preview.ExistingSkill.Name
	}
	return result, nil
}

// enable adds the skill to the skills the organization has switched on and reports whether it is
// on. A failure leaves the skill installed and off; the administrator can switch it on by hand.
func (i marketSkillInstaller) enable(ctx context.Context, scope runtimeservice.Scope, skillID string) bool {
	current, err := i.skills.GetSkillConfig(ctx, scope)
	if err != nil || current == nil {
		return false
	}
	if slices.Contains(current.EnabledSkillIDs, skillID) {
		return true
	}
	enabled := append(slices.Clone(current.EnabledSkillIDs), skillID)
	_, err = i.skills.UpdateSkillConfig(ctx, scope, runtimedto.UpdateSkillConfigRequest{EnabledSkillIDs: enabled})
	return err == nil
}

func (i marketSkillInstaller) Installed(ctx context.Context, organizationID, accountID string) ([]zgimarket.InstalledSkill, error) {
	scope, err := marketSkillScope(organizationID, accountID)
	if err != nil {
		return nil, err
	}
	packages, err := i.skills.ListInstalledSkillPackages(ctx, scope, marketSkillOrigin)
	if err != nil {
		return nil, err
	}
	installed := make([]zgimarket.InstalledSkill, 0, len(packages))
	for _, pkg := range packages {
		installed = append(installed, zgimarket.InstalledSkill{
			Slug: pkg.Origin.Reference, SkillID: pkg.SkillID, Name: pkg.Name, Version: pkg.Origin.Version,
		})
	}
	return installed, nil
}

// NewZGIMarketTools builds the market tools of this instance and shares them with the chat
// runtimes and the tool manager. It returns nil when the market is not configured.
func NewZGIMarketTools(db *gorm.DB, sealer zgimarket.SecretSealer, toolManager *tools.ToolManager) *zgimarket.Tools {
	if config.GlobalConfig == nil || db == nil || sealer == nil || config.GlobalConfig.Console.APIURL == "" {
		return nil
	}
	console := config.GlobalConfig.Console
	var gateway *zgimarket.GatewayConfig
	if console.ZGIMarketGatewayURL != "" {
		server := config.GlobalConfig.Server
		production := strings.EqualFold(server.Environment, "production") || strings.EqualFold(server.Mode, "release")
		allowPrivate := console.ZGIMarketAllowPrivateTargets && !production
		gateway = &zgimarket.GatewayConfig{URL: console.ZGIMarketGatewayURL, AllowPrivateTargets: allowPrivate}
	}
	marketTools, err := zgimarket.NewTools(db, sealer, gateway)
	if err != nil {
		logger.Error("ZGI Market tools are off", err)
		return nil
	}
	zgimarket.Share(marketTools)
	if gateway != nil && toolManager != nil {
		toolManager.SetMarketToolSource(marketTools)
	}
	return marketTools
}
