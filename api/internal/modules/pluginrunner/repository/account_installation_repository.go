package repository

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"gorm.io/gorm"
)

// AccountInstallationRepository defines the interface for account-plugin installation data access
type AccountInstallationRepository interface {
	// Create creates a new installation record
	Create(ctx context.Context, inst *model.AccountPluginInstallation) error

	// GetByID returns an installation by ID
	GetByID(ctx context.Context, id string) (*model.AccountPluginInstallation, error)

	// GetByTenantAndVersion returns an installation by tenant and marketplace version IDs
	GetByTenantAndVersion(ctx context.Context, tenantID, marketplaceVersionID string) (*model.AccountPluginInstallation, error)

	// ListByTenantID returns all installations for a tenant
	ListByTenantID(ctx context.Context, tenantID string) ([]model.AccountPluginInstallation, error)

	// ListByTenantAndPlugin returns installations for a specific plugin
	ListByTenantAndPlugin(ctx context.Context, tenantID, marketplacePluginID string) ([]model.AccountPluginInstallation, error)

	// ListByVersionID returns all installations for a marketplace version
	ListByVersionID(ctx context.Context, marketplaceVersionID string) ([]model.AccountPluginInstallation, error)

	// CountByMarketplaceVersionID returns number of installations for a version
	CountByMarketplaceVersionID(ctx context.Context, marketplaceVersionID string) (int64, error)

	// Delete deletes an installation record
	Delete(ctx context.Context, id string) error

	// DeleteByTenantAndVersion deletes by tenant and marketplace version IDs
	DeleteByTenantAndVersion(ctx context.Context, tenantID, marketplaceVersionID string) error
}

type accountInstallationRepository struct {
	db *gorm.DB
}

// NewAccountInstallationRepository creates a new account installation repository
func NewAccountInstallationRepository(db *gorm.DB) AccountInstallationRepository {
	return &accountInstallationRepository{db: db}
}

// Create creates a new installation record
func (r *accountInstallationRepository) Create(ctx context.Context, inst *model.AccountPluginInstallation) error {
	return r.db.WithContext(ctx).Create(inst).Error
}

// GetByID returns an installation by ID
func (r *accountInstallationRepository) GetByID(ctx context.Context, id string) (*model.AccountPluginInstallation, error) {
	var inst model.AccountPluginInstallation
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&inst).Error
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// GetByTenantAndVersion returns an installation by tenant and marketplace version IDs
func (r *accountInstallationRepository) GetByTenantAndVersion(ctx context.Context, tenantID, marketplaceVersionID string) (*model.AccountPluginInstallation, error) {
	var inst model.AccountPluginInstallation
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace_version_id = ?", tenantID, marketplaceVersionID).
		First(&inst).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

// ListByTenantID returns all installations for a tenant
func (r *accountInstallationRepository) ListByTenantID(ctx context.Context, tenantID string) ([]model.AccountPluginInstallation, error) {
	var insts []model.AccountPluginInstallation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("installed_at DESC").
		Find(&insts).Error
	return insts, err
}

// ListByTenantAndPlugin returns installations for a specific plugin
func (r *accountInstallationRepository) ListByTenantAndPlugin(ctx context.Context, tenantID, marketplacePluginID string) ([]model.AccountPluginInstallation, error) {
	var insts []model.AccountPluginInstallation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace_plugin_id = ?", tenantID, marketplacePluginID).
		Order("installed_at DESC").
		Find(&insts).Error
	return insts, err
}

// ListByVersionID returns all installations for a marketplace version
func (r *accountInstallationRepository) ListByVersionID(ctx context.Context, marketplaceVersionID string) ([]model.AccountPluginInstallation, error) {
	var insts []model.AccountPluginInstallation
	err := r.db.WithContext(ctx).
		Where("marketplace_version_id = ?", marketplaceVersionID).
		Find(&insts).Error
	return insts, err
}

// CountByMarketplaceVersionID returns number of installations for a version
func (r *accountInstallationRepository) CountByMarketplaceVersionID(ctx context.Context, marketplaceVersionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.AccountPluginInstallation{}).
		Where("marketplace_version_id = ?", marketplaceVersionID).
		Count(&count).Error
	return count, err
}

// Delete deletes an installation record
func (r *accountInstallationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.AccountPluginInstallation{}, "id = ?", id).Error
}

// DeleteByTenantAndVersion deletes by tenant and marketplace version IDs
func (r *accountInstallationRepository) DeleteByTenantAndVersion(ctx context.Context, tenantID, marketplaceVersionID string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND marketplace_version_id = ?", tenantID, marketplaceVersionID).
		Delete(&model.AccountPluginInstallation{}).Error
}
