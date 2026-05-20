package dataplane

import (
	"time"

	"gorm.io/gorm"
)

// PluginRecord stores the manifest metadata in the control-plane DB.
type PluginRecord struct {
	ID                   uint           `gorm:"primaryKey"`
	ExternalID           string         `gorm:"size:255;index"`
	MarketplacePluginID  string         `gorm:"column:marketplace_plugin_id;size:64;index"` // Marketplace plugin UUID
	MarketplaceVersionID string         `gorm:"column:marketplace_version_id;size:64"`      // Marketplace version UUID
	Name                 string         `gorm:"size:255;not null;index:idx_plugin_name_version,priority:1"`
	Version              string         `gorm:"size:64;not null;index:idx_plugin_name_version,priority:2"`
	ManifestJSON         string         `gorm:"type:text;not null"`
	Status               string         `gorm:"size:32;default:active"`
	CreatedAt            time.Time      `gorm:"autoCreateTime"`
	UpdatedAt            time.Time      `gorm:"autoUpdateTime"`
	DeletedAt            gorm.DeletedAt `gorm:"index"` // Soft delete support
}

func (PluginRecord) TableName() string { return "plugins" }

// PluginInstall tracks where plugin packages are expanded.
type PluginInstall struct {
	ID              uint           `gorm:"primaryKey"`
	PluginID        uint           `gorm:"index;not null"`
	WorkspacePath   string         `gorm:"size:512;not null"`
	PackageChecksum string         `gorm:"size:128"`
	PackageSize     int64          `gorm:"default:0"`
	InstalledBy     string         `gorm:"size:255"`
	Status          string         `gorm:"size:32;default:installed"`
	ErrorMessage    string         `gorm:"size:512"`
	Stage           string         `gorm:"size:32"`
	Source          string         `gorm:"size:255"`
	InstalledAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `gorm:"index"` // Soft delete support
}

func (PluginInstall) TableName() string { return "plugin_installs" }

// Tenant represents a logical workspace; optional for single-tenant deployments.
type Tenant struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"size:255;uniqueIndex"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (Tenant) TableName() string { return "tenants" }

// PluginTenantBinding captures per-tenant configuration for a plugin.
type PluginTenantBinding struct {
	ID         uint           `gorm:"primaryKey"`
	PluginID   uint           `gorm:"index;not null"`
	TenantID   uint           `gorm:"index;not null"`
	ConfigJSON string         `gorm:"type:text"`
	Enabled    bool           `gorm:"default:true"`
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"` // Soft delete support
}

func (PluginTenantBinding) TableName() string { return "plugin_tenant_bindings" }

// PluginRun records execution metadata for auditing or replay.
type PluginRun struct {
	ID          uint      `gorm:"primaryKey"`
	PluginID    uint      `gorm:"index;not null"`
	TenantID    *uint     `gorm:"index"`
	SessionID   string    `gorm:"size:128;index"`
	Status      string    `gorm:"size:32"`
	StartedAt   time.Time `gorm:"autoCreateTime"`
	CompletedAt *time.Time
	LogPath     string `gorm:"size:512"`
}

func (PluginRun) TableName() string { return "plugin_runs" }

// AuditLog captures management actions for auditing.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"`
	Actor     string    `gorm:"size:255"`
	Action    string    `gorm:"size:128;index"`
	Resource  string    `gorm:"size:255"`
	Tenant    string    `gorm:"size:255"`
	Detail    string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (AuditLog) TableName() string { return "audit_logs" }
