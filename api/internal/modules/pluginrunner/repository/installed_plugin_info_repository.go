package repository

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"gorm.io/gorm"
)

// InstalledPluginInfoRepository defines the interface for installed plugin info data access
type InstalledPluginInfoRepository interface {
	// Create creates a new installed plugin info record
	Create(ctx context.Context, info *model.InstalledPluginInfo) error

	// GetByID returns an installed plugin info by ID
	GetByID(ctx context.Context, id string) (*model.InstalledPluginInfo, error)

	// GetByMarketplaceVersionID returns an installed plugin info by marketplace version ID
	GetByMarketplaceVersionID(ctx context.Context, versionID string) (*model.InstalledPluginInfo, error)

	// GetByName returns an installed plugin info by plugin name
	GetByName(ctx context.Context, name string) (*model.InstalledPluginInfo, error)

	// GetByNameAndVersion returns an installed plugin info by plugin name and version
	GetByNameAndVersion(ctx context.Context, name, version string) (*model.InstalledPluginInfo, error)

	// List returns all installed plugin info records
	List(ctx context.Context) ([]model.InstalledPluginInfo, error)

	// Update updates an installed plugin info record
	Update(ctx context.Context, info *model.InstalledPluginInfo) error

	// Delete deletes an installed plugin info record
	Delete(ctx context.Context, id string) error

	// DeleteByMarketplaceVersionID deletes an installed plugin info record by version ID
	DeleteByMarketplaceVersionID(ctx context.Context, versionID string) error
}

type installedPluginInfoRepository struct {
	db *gorm.DB
}

// NewInstalledPluginInfoRepository creates a new installed plugin info repository
func NewInstalledPluginInfoRepository(db *gorm.DB) InstalledPluginInfoRepository {
	return &installedPluginInfoRepository{db: db}
}

// Create creates a new installed plugin info record
func (r *installedPluginInfoRepository) Create(ctx context.Context, info *model.InstalledPluginInfo) error {
	return r.db.WithContext(ctx).Create(info).Error
}

// GetByID returns an installed plugin info by ID
func (r *installedPluginInfoRepository) GetByID(ctx context.Context, id string) (*model.InstalledPluginInfo, error) {
	var info model.InstalledPluginInfo
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetByMarketplaceVersionID returns an installed plugin info by marketplace version ID
func (r *installedPluginInfoRepository) GetByMarketplaceVersionID(ctx context.Context, versionID string) (*model.InstalledPluginInfo, error) {
	var info model.InstalledPluginInfo
	err := r.db.WithContext(ctx).Where("marketplace_version_id = ?", versionID).First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetByName returns an installed plugin info by plugin name
func (r *installedPluginInfoRepository) GetByName(ctx context.Context, name string) (*model.InstalledPluginInfo, error) {
	var info model.InstalledPluginInfo
	err := r.db.WithContext(ctx).Where("plugin_name = ?", name).First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetByNameAndVersion returns an installed plugin info by plugin name and version
func (r *installedPluginInfoRepository) GetByNameAndVersion(ctx context.Context, name, version string) (*model.InstalledPluginInfo, error) {
	var info model.InstalledPluginInfo
	err := r.db.WithContext(ctx).
		Where("plugin_name = ? AND plugin_version = ?", name, version).
		First(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// List returns all installed plugin info records
func (r *installedPluginInfoRepository) List(ctx context.Context) ([]model.InstalledPluginInfo, error) {
	var infos []model.InstalledPluginInfo
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&infos).Error
	return infos, err
}

// Update updates an installed plugin info record
func (r *installedPluginInfoRepository) Update(ctx context.Context, info *model.InstalledPluginInfo) error {
	return r.db.WithContext(ctx).Save(info).Error
}

// Delete deletes an installed plugin info record
func (r *installedPluginInfoRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.InstalledPluginInfo{}, "id = ?", id).Error
}

// DeleteByMarketplaceVersionID deletes an installed plugin info record by version ID
func (r *installedPluginInfoRepository) DeleteByMarketplaceVersionID(ctx context.Context, versionID string) error {
	return r.db.WithContext(ctx).Delete(&model.InstalledPluginInfo{}, "marketplace_version_id = ?", versionID).Error
}
