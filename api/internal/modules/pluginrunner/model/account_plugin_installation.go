package model

import (
	"time"
)

// AccountPluginInstallation represents the relationship between an account
// and an installed plugin version from the marketplace.
type AccountPluginInstallation struct {
	ID                   string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID             string    `gorm:"type:uuid;column:tenant_id;not null;index" json:"tenant_id"`
	MarketplacePluginID  string    `gorm:"type:uuid;not null" json:"marketplace_plugin_id"`
	MarketplaceVersionID string    `gorm:"type:uuid;not null;index" json:"marketplace_version_id"`
	InstalledBy          string    `gorm:"type:uuid;not null" json:"installed_by"`
	InstalledAt          time.Time `gorm:"autoCreateTime" json:"installed_at"`
	Status               string    `gorm:"size:20;default:'active'" json:"status"` // active, disabled
}

// TableName specifies the database table name
func (AccountPluginInstallation) TableName() string {
	return "account_plugin_installations"
}

// InstallationStatus constants
const (
	InstallationStatusActive   = "active"
	InstallationStatusDisabled = "disabled"
)
