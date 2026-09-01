package model

import (
	"time"
)

// Setup represents system setup status
type Setup struct {
	Version        string    `gorm:"primaryKey;column:version" json:"version"`
	SetupAt        time.Time `gorm:"column:setup_at" json:"setup_at"`
	OrganizationID *string   `gorm:"column:organization_id;type:uuid" json:"organization_id,omitempty"`
	WorkspaceID    *string   `gorm:"column:workspace_id;type:uuid" json:"workspace_id,omitempty"`
}

// TableName returns table name
func (Setup) TableName() string {
	return "zgi_setups"
}
