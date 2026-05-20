package model

import "time"

// OrgPluginSubscription represents the subscription relationship between an organization and a plugin.
// This is the source of truth for "which organization can use which plugin" in zgi-api.
// Updated to support member-level subscriptions with account_id and installation_id.
type OrgPluginSubscription struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	GroupID        string    `gorm:"type:uuid;not null;index:idx_group_account_installation,unique" json:"group_id"`         // Organization ID
	AccountID      string    `gorm:"type:uuid;index:idx_group_account_installation,unique" json:"account_id,omitempty"`      // Member account ID
	InstallationID string    `gorm:"type:uuid;index:idx_group_account_installation,unique" json:"installation_id,omitempty"` // Reference to account_plugin_installations
	PluginID       string    `gorm:"not null;size:255;index:idx_egps_plugin_id" json:"plugin_id"`                            // e.g., "regex:1.0.0" (kept for compatibility)
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	Config         string    `gorm:"type:text" json:"config,omitempty"`
	SubscribedBy   string    `gorm:"type:uuid" json:"subscribed_by,omitempty"` // Account that performed the subscription
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the database table name
func (OrgPluginSubscription) TableName() string {
	return "enterprise_group_plugin_subscriptions"
}

// SubscribePluginRequest represents the request to subscribe an organization to a plugin
type SubscribePluginRequest struct {
	Config string `json:"config,omitempty"` // Optional JSON config
}

// SubscriptionResponse represents the subscription info returned to clients
type SubscriptionResponse struct {
	ID           uint      `json:"id"`
	GroupID      string    `json:"group_id"`
	PluginID     string    `json:"plugin_id"`
	Enabled      bool      `json:"enabled"`
	Config       string    `json:"config,omitempty"`
	SubscribedBy string    `json:"subscribed_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
