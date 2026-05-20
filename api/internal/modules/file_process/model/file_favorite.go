package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FileFavorite represents a file favorite relationship
type FileFavorite struct {
	ID        string    `json:"id" gorm:"type:varchar(255);primaryKey"`
	FileID    string    `json:"file_id" gorm:"type:varchar(255);not null;index"`
	AccountID string    `json:"account_id" gorm:"type:varchar(255);not null;index"`
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
}

// BeforeCreate GORM hook, generate UUID before creation
func (ff *FileFavorite) BeforeCreate(tx *gorm.DB) error {
	if ff.ID == "" {
		ff.ID = uuid.New().String()
	}
	if ff.CreatedAt.IsZero() {
		ff.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (FileFavorite) TableName() string {
	return "file_favorites"
}