package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/dto"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

var errInstallationNotFound = errors.New("plugin is not installed for this organization")

func (h *Handler) requireOrganizationMember(c *gin.Context) (string, string, bool) {
	organizationID, err := getOrganizationIDFromContext(c, h.accountService)
	if err != nil {
		response.FailWithMessage(c, ErrOrgNotInContext, err.Error())
		return "", "", false
	}

	accountID := getAccountIDFromContext(c)
	if accountID == "" {
		response.Fail(c, response.ErrPermissionDenied)
		return "", "", false
	}

	isMember, err := ensureOrganizationMember(c, h.accountService, organizationID, accountID)
	if err != nil {
		response.FailWithMessage(c, ErrPluginRunnerAccessDenied, err.Error())
		return "", "", false
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return "", "", false
	}

	return organizationID, accountID, true
}

func (h *Handler) listOrganizationPlugins(ctx context.Context, organizationID string) ([]dto.TenantPluginResponse, error) {
	if h.installService == nil || h.infoRepo == nil {
		return nil, errors.New("installation service is not configured")
	}

	installations, err := h.installService.ListByTenant(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.TenantPluginResponse, 0, len(installations))
	for _, inst := range installations {
		if inst.Status != model.InstallationStatusActive {
			continue
		}
		info, err := h.infoRepo.GetByMarketplaceVersionID(ctx, inst.MarketplaceVersionID)
		if err != nil {
			continue
		}
		result = append(result, buildTenantPluginResponse(*info, inst.Status))
	}
	return result, nil
}

func (h *Handler) listOrganizationInstallations(ctx context.Context, organizationID string) ([]dto.InstallationResponse, error) {
	if h.installService == nil || h.infoRepo == nil {
		return nil, errors.New("installation service is not configured")
	}

	installations, err := h.installService.ListByTenant(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.InstallationResponse, 0, len(installations))
	for _, inst := range installations {
		info, err := h.infoRepo.GetByMarketplaceVersionID(ctx, inst.MarketplaceVersionID)
		if err != nil {
			continue
		}
		result = append(result, buildInstallationResponse(inst, info))
	}
	return result, nil
}

func (h *Handler) resolveInstallationInfo(ctx context.Context, organizationID, identifier string) (*model.AccountPluginInstallation, *model.InstalledPluginInfo, error) {
	if h.installService == nil || h.infoRepo == nil {
		return nil, nil, errors.New("installation service is not configured")
	}

	versionID, err := h.resolveMarketplaceVersionID(ctx, identifier)
	if err != nil {
		return nil, nil, err
	}

	installation, err := h.installService.GetInstallation(ctx, organizationID, versionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errInstallationNotFound
		}
		return nil, nil, err
	}

	info, err := h.infoRepo.GetByMarketplaceVersionID(ctx, versionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errInstallationNotFound
		}
		return nil, nil, err
	}

	return installation, info, nil
}

func (h *Handler) resolveMarketplaceVersionID(ctx context.Context, identifier string) (string, error) {
	if identifier == "" {
		return "", errInstallationNotFound
	}
	// If it is a UUID, it is directly regarded as the Marketplace Version ID
	if isUUID(identifier) {
		return identifier, nil
	}

	name, version, ok := splitPluginNameVersion(identifier)
	if !ok {
		return "", errInstallationNotFound
	}

	info, err := h.infoRepo.GetByNameAndVersion(ctx, name, version)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errInstallationNotFound
		}
		return "", err
	}

	return info.MarketplaceVersionID, nil
}

func buildInstallationResponse(installation model.AccountPluginInstallation, info *model.InstalledPluginInfo) dto.InstallationResponse {
	description := ""
	name := ""
	version := ""
	if info != nil {
		name = info.PluginName
		version = info.PluginVersion
		description, _ = extractProviderMetadata(info.Declaration)
	}

	resp := dto.InstallationResponse{
		ID:          installation.MarketplacePluginID,
		VersionID:   installation.MarketplaceVersionID,
		Name:        name,
		Version:     version,
		Description: description,
		Path:        "",
		InstalledAt: installation.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	return resp
}

func buildTenantPluginResponse(info model.InstalledPluginInfo, status string) dto.TenantPluginResponse {
	description, tags := extractProviderMetadata(info.Declaration)
	return dto.TenantPluginResponse{
		ID:          info.MarketplaceVersionID,
		Name:        info.PluginName,
		Version:     info.PluginVersion,
		Description: description,
		Language:    "runner",
		Tags:        tags,
		Enabled:     status == model.InstallationStatusActive,
	}
}

func extractProviderMetadata(data json.RawMessage) (string, []string) {
	if len(data) == 0 {
		return "", nil
	}

	var decl model.PluginDeclaration
	if err := json.Unmarshal(data, &decl); err != nil {
		return "", nil
	}

	return selectDescription(decl.Provider.Description), decl.Provider.Tags
}

func selectDescription(values map[string]string) string {
	if values == nil {
		return ""
	}
	if desc, ok := values["en_US"]; ok && desc != "" {
		return desc
	}
	for _, desc := range values {
		if desc != "" {
			return desc
		}
	}
	return ""
}

func splitPluginNameVersion(pluginID string) (string, string, bool) {
	if pluginID == "" {
		return "", "", false
	}
	if parts := strings.Split(pluginID, "@"); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	parts := strings.Split(pluginID, ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	if len(parts) == 3 && parts[1] != "" && parts[2] != "" {
		return parts[1], parts[2], true
	}
	return "", "", false
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func (h *Handler) removeOrganizationInstallation(ctx context.Context, organizationID, identifier string) error {
	if h.installService == nil || h.subscriptionService == nil || h.infoRepo == nil {
		return errors.New("installation services are not configured")
	}

	installation, info, err := h.resolveInstallationInfo(ctx, organizationID, identifier)
	if err != nil {
		return err
	}

	runnerPluginID := ""
	if info != nil && info.PluginAuthor != "" {
		runnerPluginID = fmt.Sprintf("%s:%s:%s", info.PluginAuthor, info.PluginName, info.PluginVersion)
	}
	logger.Info(
		"deleting plugin installation",
		"organization_id",
		organizationID,
		"input_id",
		identifier,
		"resolved_marketplace_version_id",
		installation.MarketplaceVersionID,
		"resolved_external_id",
		runnerPluginID,
	)

	// Check if the plugin version has active subscriptions
	canDelete, err := h.subscriptionService.CanDeleteInstallation(ctx, installation.ID)
	if err != nil {
		return fmt.Errorf("failed to check subscriptions: %w", err)
	}
	if !canDelete {
		return fmt.Errorf("plugin version has active subscriptions")
	}

	if err := h.installService.Uninstall(ctx, organizationID, installation.MarketplaceVersionID); err != nil {
		return fmt.Errorf("failed to remove organization installation: %w", err)
	}

	count, err := h.installService.CountByMarketplaceVersionID(ctx, installation.MarketplaceVersionID)
	if err != nil {
		return fmt.Errorf("failed to count installations: %w", err)
	}
	if count == 0 {
		if err := h.cleanupRunnerVersion(ctx, info); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) cleanupRunnerVersion(ctx context.Context, info *model.InstalledPluginInfo) error {
	if info == nil {
		return nil
	}

	//  Determine the plugin author:name:version based on the info information
	//	This ID is in the format author:name:version
	pluginID, err := h.resolveRunnerPluginID(ctx, info)
	if err != nil {
		return err
	}
	if pluginID == "" {
		return nil
	}

	logger.Info(
		"deleting plugin from runner",
		"resolved_marketplace_version_id",
		info.MarketplaceVersionID,
		"resolved_external_id",
		pluginID,
	)

	if err := h.service.DeletePlugin(ctx, pluginID); err != nil {
		return fmt.Errorf("failed to delete plugin from runner: %w", err)
	}

	if h.infoRepo != nil {
		if err := h.infoRepo.DeleteByMarketplaceVersionID(ctx, info.MarketplaceVersionID); err != nil {
			return fmt.Errorf("failed to delete plugin info: %w", err)
		}
	}

	return nil
}

func (h *Handler) resolveRunnerPluginID(ctx context.Context, info *model.InstalledPluginInfo) (string, error) {
	if info.PluginAuthor != "" {
		return fmt.Sprintf("%s:%s:%s", info.PluginAuthor, info.PluginName, info.PluginVersion), nil
	}

	installations, err := h.service.ListInstalledPlugins(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list runner plugins: %w", err)
	}
	for _, inst := range installations {
		if inst.Manifest.Name == info.PluginName && inst.Manifest.Version == info.PluginVersion {
			author := inst.Manifest.Author
			if author == "" {
				return "", nil
			}
			return fmt.Sprintf("%s:%s:%s", author, inst.Manifest.Name, inst.Manifest.Version), nil
		}
	}

	return "", nil
}
