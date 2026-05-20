package dto

import "mime/multipart"

// ============================================
// Admin Plugin Management DTOs
// ============================================

// RegisterPluginRequest represents the request to register a plugin
type RegisterPluginRequest struct {
	Name        string   `json:"name" binding:"required"`
	Version     string   `json:"version" binding:"required"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Language    string   `json:"language" binding:"required"`
	Entrypoint  string   `json:"entrypoint" binding:"required"`
}

// InstallPluginRequest represents the request to install a plugin (file upload)
type InstallPluginRequest struct {
	Force   bool                  `form:"force"`
	Package *multipart.FileHeader `form:"package" binding:"required"`
}

// InstallPluginBase64Request represents the request to install a plugin (base64)
type InstallPluginBase64Request struct {
	Force         bool   `json:"force"`
	PackageBase64 string `json:"package_b64" binding:"required"`
}

// InstallFromMarketplaceRequest represents the request to install a plugin from Marketplace
type InstallFromMarketplaceRequest struct {
	PluginID  string `json:"plugin_id" binding:"required"`
	VersionID string `json:"version_id" binding:"required"`
	Force     bool   `json:"force"`
}

// PluginResponse represents the response for a plugin
type PluginResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Language    string   `json:"language"`
	Entrypoint  string   `json:"entrypoint"`
	Tags        []string `json:"tags,omitempty"`
}

// InstallationResponse represents the response for an installation
type InstallationResponse struct {
	ID              string `json:"id"`                   // Marketplace plugin UUID
	VersionID       string `json:"version_id,omitempty"` // Marketplace version UUID
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description,omitempty"`
	Path            string `json:"path"`
	InstalledAt     string `json:"installed_at"`
	PackageChecksum string `json:"package_checksum,omitempty"`
}

// ReinstallFromMarketplaceResponse represents the response for marketplace reinstall
type ReinstallFromMarketplaceResponse struct {
	Status       string                `json:"status"`
	Installation *InstallationResponse `json:"installation,omitempty"`
}

// ============================================
// Tenant Plugin DTOs
// ============================================

// TenantPluginResponse represents a plugin available to a tenant
type TenantPluginResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Language    string   `json:"language"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     bool     `json:"enabled"`
}

// ============================================
// Session Management DTOs (Admin debug only)
// ============================================

// SessionResponse represents a session response
type SessionResponse struct {
	ID         string `json:"id"`
	PluginName string `json:"plugin_name"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	PID        int    `json:"pid,omitempty"`
}
