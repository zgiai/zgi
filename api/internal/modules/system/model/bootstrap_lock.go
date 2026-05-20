// Package model defines system bootstrap persistence models.
package model

import "time"

// BootstrapLock serializes first-time setup work across concurrent starters.
type BootstrapLock struct {
	Key       string    `gorm:"column:key;type:varchar(64);primaryKey" json:"key"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName returns the bootstrap lock table name.
func (BootstrapLock) TableName() string {
	return "zgi_bootstrap_locks"
}
