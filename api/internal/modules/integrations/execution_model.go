package integrations

import (
	"time"

	"github.com/google/uuid"
)

type ExecutionRecord struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID        *uuid.UUID `gorm:"type:uuid" json:"workspace_id,omitempty"`
	AccountID          *uuid.UUID `gorm:"type:uuid" json:"account_id,omitempty"`
	AppID              *uuid.UUID `gorm:"type:uuid" json:"app_id,omitempty"`
	ConversationID     *uuid.UUID `gorm:"type:uuid" json:"conversation_id,omitempty"`
	MessageID          *uuid.UUID `gorm:"type:uuid" json:"message_id,omitempty"`
	ConnectionID       *uuid.UUID `gorm:"type:uuid" json:"connection_id,omitempty"`
	IntegrationID      string     `gorm:"size:64;not null" json:"integration_id"`
	DriverID           string     `gorm:"size:64;not null" json:"driver_id"`
	ActionID           string     `gorm:"size:128;not null" json:"action_id"`
	InvokeFrom         string     `gorm:"size:32;not null" json:"invoke_from"`
	Status             string     `gorm:"size:32;not null" json:"status"`
	ProviderRequestID  *string    `gorm:"size:128" json:"provider_request_id,omitempty"`
	ProviderErrorCode  *string    `gorm:"size:128" json:"provider_error_code,omitempty"`
	ProviderHTTPStatus *int       `json:"provider_http_status,omitempty"`
	RetryAfterAt       *time.Time `json:"retry_after_at,omitempty"`
	DurationMS         int64      `gorm:"not null" json:"duration_ms"`
	CostUSD            *float64   `gorm:"type:numeric(20,8)" json:"cost_usd,omitempty"`
	InputHMAC          *string    `gorm:"size:64" json:"-"`
	ResultCount        int        `gorm:"not null" json:"result_count"`
	AttemptCount       int        `gorm:"not null" json:"attempt_count"`
	ErrorCode          *string    `gorm:"size:64" json:"error_code,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (ExecutionRecord) TableName() string { return "integration_executions" }

type ExecutionCompletion struct {
	Status             string     `json:"status"`
	ProviderRequestID  string     `json:"provider_request_id,omitempty"`
	ProviderErrorCode  string     `json:"provider_error_code,omitempty"`
	ProviderHTTPStatus *int       `json:"provider_http_status,omitempty"`
	RetryAfterAt       *time.Time `json:"retry_after_at,omitempty"`
	DurationMS         int64      `json:"duration_ms"`
	CostUSD            *float64   `json:"cost_usd,omitempty"`
	ResultCount        int        `json:"result_count"`
	AttemptCount       int        `json:"attempt_count"`
	ErrorCode          string     `json:"error_code,omitempty"`
}
