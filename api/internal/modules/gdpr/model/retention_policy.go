package model

import (
	"time"

	"github.com/google/uuid"
)

// DataRetentionPolicy represents configurable data retention rules
type DataRetentionPolicy struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DataType            string    `gorm:"type:varchar(50);not null;uniqueIndex" json:"data_type"`
	Description         string    `gorm:"type:text" json:"description,omitempty"`
	RetentionDays       int       `gorm:"not null" json:"retention_days"`
	AnonymizeAfterDays  *int      `json:"anonymize_after_days,omitempty"`
	HardDeleteAfterDays *int      `json:"hard_delete_after_days,omitempty"`
	IsActive            bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt           time.Time `gorm:"not null;default:NOW()" json:"created_at"`
	UpdatedAt           time.Time `gorm:"not null;default:NOW()" json:"updated_at"`
}

func (DataRetentionPolicy) TableName() string {
	return "data_retention_policies"
}

// ShouldAnonymize checks if data should be anonymized based on age in days
func (p *DataRetentionPolicy) ShouldAnonymize(ageInDays int) bool {
	if p.AnonymizeAfterDays == nil {
		return false
	}
	return ageInDays >= *p.AnonymizeAfterDays
}

// ShouldHardDelete checks if data should be hard deleted based on age in days
func (p *DataRetentionPolicy) ShouldHardDelete(ageInDays int) bool {
	if p.HardDeleteAfterDays == nil {
		return false
	}
	return ageInDays >= *p.HardDeleteAfterDays
}
