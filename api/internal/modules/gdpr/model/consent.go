package model

import (
	"time"

	"github.com/google/uuid"
)

// ConsentType defines types of user consents
type ConsentType string

const (
	ConsentTypeMarketing      ConsentType = "marketing"
	ConsentTypeAnalytics      ConsentType = "analytics"
	ConsentTypeDataProcessing ConsentType = "data_processing"
	ConsentTypeThirdParty     ConsentType = "third_party"
)

// UserConsent represents user consent management
type UserConsent struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AccountID   uuid.UUID   `gorm:"type:uuid;not null;index" json:"account_id"`
	ConsentType ConsentType `gorm:"type:varchar(50);not null;index" json:"consent_type"`
	IsGranted   bool        `gorm:"not null" json:"is_granted"`
	GrantedAt   *time.Time  `json:"granted_at,omitempty"`
	RevokedAt   *time.Time  `json:"revoked_at,omitempty"`
	IPAddress   string      `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent   string      `gorm:"type:text" json:"user_agent,omitempty"`
	Version     string      `gorm:"type:varchar(20);default:'1.0'" json:"version"`
	CreatedAt   time.Time   `gorm:"not null;default:NOW()" json:"created_at"`
	UpdatedAt   time.Time   `gorm:"not null;default:NOW()" json:"updated_at"`
}

func (UserConsent) TableName() string {
	return "user_consents"
}
