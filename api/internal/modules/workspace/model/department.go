package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DepartmentStatus department status enum
type DepartmentStatus string

const (
	DepartmentStatusActive   DepartmentStatus = "active"
	DepartmentStatusArchived DepartmentStatus = "archived"
)

// Department department model
type Department struct {
	ID             string           `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID string           `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`
	ParentID       *string          `gorm:"type:uuid;index" json:"parent_id"`
	Name           string           `gorm:"type:varchar(255);not null" json:"name"`
	SortOrder      int              `gorm:"not null;default:0" json:"sort_order"`
	Status         DepartmentStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	CreatedBy      *string          `gorm:"type:uuid" json:"created_by"`

	// Relationships
	Parent   *Department  `gorm:"foreignKey:ParentID" json:"-"`
	Children []Department `gorm:"foreignKey:ParentID" json:"children,omitempty"`
}

// TableName specifies table name
func (Department) TableName() string {
	return "departments"
}

// BeforeCreate hook to set ID and timestamps
func (d *Department) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	now := time.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate hook to update timestamp
func (d *Department) BeforeUpdate(tx *gorm.DB) error {
	d.UpdatedAt = time.Now()
	return nil
}

// IsActive checks if department is active
func (d *Department) IsActive() bool {
	return d.Status == DepartmentStatusActive
}

// IsRoot checks if department is root (no parent)
func (d *Department) IsRoot() bool {
	return d.ParentID == nil || *d.ParentID == ""
}
