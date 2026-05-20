package model

import (
	"time"

	"github.com/google/uuid"
)

// ActionType defines GDPR operation types
type ActionType string

const (
	ActionTypeDataExport     ActionType = "data_export"
	ActionTypeDataErasure    ActionType = "data_erasure"
	ActionTypeDataAnonymize  ActionType = "data_anonymize"
	ActionTypeConsentChange  ActionType = "consent_change"
	ActionTypeDataAccess     ActionType = "data_access"
	ActionTypeRetentionClean ActionType = "retention_clean"
)

// AuditStatus defines audit log status
type AuditStatus string

const (
	AuditStatusPending   AuditStatus = "pending"
	AuditStatusCompleted AuditStatus = "completed"
	AuditStatusFailed    AuditStatus = "failed"
)

// GDPRAuditLog represents audit trail for GDPR operations
type GDPRAuditLog struct {
	ID           uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ActionType   ActionType             `gorm:"type:varchar(50);not null;index" json:"action_type"`
	ActorID      *uuid.UUID             `gorm:"type:uuid;index" json:"actor_id,omitempty"`
	ActorEmail   string                 `gorm:"type:varchar(255)" json:"actor_email,omitempty"`
	SubjectID    uuid.UUID              `gorm:"type:uuid;not null;index" json:"subject_id"`
	SubjectEmail string                 `gorm:"type:varchar(255)" json:"subject_email,omitempty"`
	TenantID     *uuid.UUID             `gorm:"type:uuid;index" json:"tenant_id,omitempty"`
	Details      map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"details,omitempty"`
	IPAddress    string                 `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent    string                 `gorm:"type:text" json:"user_agent,omitempty"`
	Status       AuditStatus            `gorm:"type:varchar(20);not null;default:'completed'" json:"status"`
	ErrorMessage string                 `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time              `gorm:"not null;default:NOW();index" json:"created_at"`
}

func (GDPRAuditLog) TableName() string {
	return "gdpr_audit_logs"
}
