package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RateLimitLog represents rate limit logs for tracking rate limit violations
type RateLimitLog struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	TenantID         string    `gorm:"type:uuid;not null;index:rate_limit_log_tenant_idx" json:"tenant_id"`
	SubscriptionPlan string    `gorm:"type:varchar(255);not null" json:"subscription_plan"`
	Operation        string    `gorm:"type:varchar(255);not null;index:rate_limit_log_operation_idx" json:"operation"`
	CreatedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
}

// BeforeCreate GORM hook to generate ID if not provided
func (r *RateLimitLog) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies the table name for RateLimitLog
func (RateLimitLog) TableName() string {
	return "rate_limit_logs"
}
