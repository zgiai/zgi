package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/parser"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/repository"
	"github.com/zgiai/ginext/pkg/logger"
)

// ... (omitted)

// AccountInstallationService defines the interface for account plugin installation operations
type AccountInstallationService interface {
	// Install installs a plugin for an organization.
	Install(ctx context.Context, tenantID, marketplacePluginID, marketplaceVersionID, installedBy string) (*model.AccountPluginInstallation, error)

	// Uninstall removes a plugin installation for an organization.
	Uninstall(ctx context.Context, tenantID, marketplaceVersionID string) error

	// GetInstallation returns an installation by organization and marketplace version IDs.
	GetInstallation(ctx context.Context, tenantID, marketplaceVersionID string) (*model.AccountPluginInstallation, error)

	// ListByTenant returns all installations for an organization.
	ListByTenant(ctx context.Context, tenantID string) ([]model.AccountPluginInstallation, error)

	// CountByMarketplaceVersionID returns number of installations for a version
	CountByMarketplaceVersionID(ctx context.Context, marketplaceVersionID string) (int64, error)

	// ListDeclarationsByTenant returns all plugin declarations installed by a tenant
	ListDeclarationsByTenant(ctx context.Context, tenantID string) ([]model.PluginDeclaration, error)

	// GetDeclarationByProviderName returns a specific declaration by provider name for a tenant
	GetDeclarationByProviderName(ctx context.Context, tenantID, providerName string) (*model.PluginDeclaration, error)

	// InstallFromDirectory installs a plugin by parsing YAML files from directory
	InstallFromDirectory(ctx context.Context, tenantID, marketplacePluginID, marketplaceVersionID, installedBy, pluginDir string) (*model.AccountPluginInstallation, error)
}

type accountInstallationService struct {
	installRepo repository.AccountInstallationRepository
	infoRepo    repository.InstalledPluginInfoRepository
}

// NewAccountInstallationService creates a new account installation service
func NewAccountInstallationService(
	installRepo repository.AccountInstallationRepository,
	infoRepo repository.InstalledPluginInfoRepository,
) AccountInstallationService {
	return &accountInstallationService{
		installRepo: installRepo,
		infoRepo:    infoRepo,
	}
}

// Install installs a plugin for an organization.
func (s *accountInstallationService) Install(ctx context.Context, tenantID, marketplacePluginID, marketplaceVersionID, installedBy string) (*model.AccountPluginInstallation, error) {
	// Check if already installed
	existing, err := s.installRepo.GetByTenantAndVersion(ctx, tenantID, marketplaceVersionID)
	if err == nil {
		return existing, nil // Already installed
	}

	inst := &model.AccountPluginInstallation{
		TenantID:             tenantID,
		MarketplacePluginID:  marketplacePluginID,
		MarketplaceVersionID: marketplaceVersionID,
		InstalledBy:          installedBy,
		Status:               model.InstallationStatusActive,
	}

	if err := s.installRepo.Create(ctx, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// Uninstall removes a plugin installation for an organization.
func (s *accountInstallationService) Uninstall(ctx context.Context, tenantID, marketplaceVersionID string) error {
	return s.installRepo.DeleteByTenantAndVersion(ctx, tenantID, marketplaceVersionID)
}

// GetInstallation returns an installation by organization and marketplace version IDs.
func (s *accountInstallationService) GetInstallation(ctx context.Context, tenantID, marketplaceVersionID string) (*model.AccountPluginInstallation, error) {
	return s.installRepo.GetByTenantAndVersion(ctx, tenantID, marketplaceVersionID)
}

// ListByTenant returns all installations for an organization.
func (s *accountInstallationService) ListByTenant(ctx context.Context, tenantID string) ([]model.AccountPluginInstallation, error) {
	return s.installRepo.ListByTenantID(ctx, tenantID)
}

// CountByMarketplaceVersionID returns number of installations for a version
func (s *accountInstallationService) CountByMarketplaceVersionID(ctx context.Context, marketplaceVersionID string) (int64, error) {
	return s.installRepo.CountByMarketplaceVersionID(ctx, marketplaceVersionID)
}

// ListDeclarationsByTenant returns all plugin declarations installed by a tenant
func (s *accountInstallationService) ListDeclarationsByTenant(ctx context.Context, tenantID string) ([]model.PluginDeclaration, error) {
	installations, err := s.installRepo.ListByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var declarations []model.PluginDeclaration
	for _, inst := range installations {
		if inst.Status != model.InstallationStatusActive {
			continue
		}

		// Lookup plugin info by marketplace_version_id
		info, err := s.infoRepo.GetByMarketplaceVersionID(ctx, inst.MarketplaceVersionID)
		if err != nil {
			logger.Warn("plugin info not found for installation, skipping", "tenant_id", tenantID, "version_id", inst.MarketplaceVersionID, "error", err)
			continue // Skip if info not found
		}

		var parsed model.PluginDeclaration
		if err := json.Unmarshal(info.Declaration, &parsed); err != nil {
			logger.Error(fmt.Sprintf("failed to unmarshal plugin declaration, skipping version_id=%s", inst.MarketplaceVersionID), err)
			continue // Skip if parsing fails
		}
		declarations = append(declarations, parsed)
	}

	return declarations, nil
}

// GetDeclarationByProviderName returns a specific declaration by provider name for a tenant
func (s *accountInstallationService) GetDeclarationByProviderName(ctx context.Context, tenantID, providerName string) (*model.PluginDeclaration, error) {
	declarations, err := s.ListDeclarationsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	for _, decl := range declarations {
		if decl.Provider.Name == providerName {
			return &decl, nil
		}
	}

	return nil, nil // Not found
}

// InstallFromDirectory installs a plugin by parsing YAML files from a directory
// This is the primary installation method that reads declaration from YAML
func (s *accountInstallationService) InstallFromDirectory(ctx context.Context, tenantID, marketplacePluginID, marketplaceVersionID, installedBy, pluginDir string) (*model.AccountPluginInstallation, error) {
	// Parse YAML files from plugin directory
	declaration, err := parser.ParsePluginDirectory(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin YAML: %w", err)
	}

	// Convert declaration to JSON for storage
	declarationJSON, err := json.Marshal(declaration)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal declaration: %w", err)
	}

	// Create or update plugin info
	info := &model.InstalledPluginInfo{
		MarketplacePluginID:  marketplacePluginID,
		MarketplaceVersionID: marketplaceVersionID,
		PluginName:           declaration.Provider.Name,
		PluginVersion:        "", // Will be filled from marketplace
		PluginAuthor:         declaration.Provider.Author,
		Declaration:          declarationJSON,
	}

	// Check if already exists
	existing, err := s.infoRepo.GetByMarketplaceVersionID(ctx, marketplaceVersionID)
	if err != nil {
		// Create new
		if err := s.infoRepo.Create(ctx, info); err != nil {
			return nil, fmt.Errorf("failed to create plugin info: %w", err)
		}
	} else {
		// Update existing
		existing.Declaration = declarationJSON
		if err := s.infoRepo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update plugin info: %w", err)
		}
	}

	// Create installation record
	return s.Install(ctx, tenantID, marketplacePluginID, marketplaceVersionID, installedBy)
}
