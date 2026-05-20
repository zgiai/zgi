package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DepartmentMember department member association model
type DepartmentMember struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	DepartmentID string    `gorm:"type:uuid;not null;index" json:"department_id"`
	AccountID    string    `gorm:"type:uuid;not null;index" json:"account_id"`
	CreatedAt    time.Time `json:"created_at"`

	// Relationships - commented out for modular architecture
	// Department Department `gorm:"foreignKey:DepartmentID" json:"-"`
	// Account    Account    `gorm:"foreignKey:AccountID" json:"-"`
}

// TableName specifies table name
func (DepartmentMember) TableName() string {
	return "department_members"
}

// BeforeCreate hook to set ID and timestamp
func (dm *DepartmentMember) BeforeCreate(tx *gorm.DB) error {
	if dm.ID == "" {
		dm.ID = uuid.New().String()
	}
	if dm.CreatedAt.IsZero() {
		dm.CreatedAt = time.Now()
	}
	return nil
}
