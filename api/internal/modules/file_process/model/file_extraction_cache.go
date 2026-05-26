package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FileExtractionCache stores explicit extraction results keyed by parser settings.
type FileExtractionCache struct {
	ID        string    `json:"id" gorm:"type:varchar(255);primaryKey"`
	FileID    string    `json:"file_id" gorm:"type:varchar(255);not null;index:idx_file_extraction_caches_file_key,unique"`
	CacheKey  string    `json:"cache_key" gorm:"type:varchar(255);not null;index:idx_file_extraction_caches_file_key,unique"`
	Content   string    `json:"content" gorm:"type:longtext;not null"`
	Source    string    `json:"source" gorm:"type:varchar(255);not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null"`
}

func (c *FileExtractionCache) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	return nil
}

func (FileExtractionCache) TableName() string {
	return "file_extraction_caches"
}
