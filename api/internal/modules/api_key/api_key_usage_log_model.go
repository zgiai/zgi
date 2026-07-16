package APIKey

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// APIKeyUsageLog represents the agent_api_key_usage_logs table
type APIKeyUsageLog struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	APIKeyID           uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_api_key_usage_logs_api_key_id" json:"api_key_id"`
	AgentID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_api_key_usage_logs_agent_id" json:"agent_id"`
	OperationLogID     *uuid.UUID     `gorm:"type:uuid" json:"operation_log_id"`
	RequestPath        string         `gorm:"type:varchar(500);not null" json:"request_path"`
	RequestIP          string         `gorm:"type:varchar(45);not null" json:"request_ip"`
	UserAgent          *string        `gorm:"type:text" json:"user_agent"`
	RequestHeaders     datatypes.JSON `gorm:"type:json" json:"request_headers"`
	RequestBodySize    int64          `gorm:"type:bigint;default:0" json:"request_body_size"`
	ResponseStatusCode int            `gorm:"type:integer;not null" json:"response_status_code"`
	ResponseBodySize   int64          `gorm:"type:bigint;default:0" json:"response_body_size"`
	ResponseTimeMS     int            `gorm:"type:integer;not null" json:"response_time_ms"`
	TokensUsed         int64          `gorm:"type:integer;default:0" json:"tokens_used"`
	CostAmount         float64        `gorm:"type:numeric(10,6);default:0" json:"cost_amount"`
	Currency           string         `gorm:"type:varchar(3);default:'USD'" json:"currency"`
	ErrorMessage       *string        `gorm:"type:text" json:"error_message"`
	Metadata           datatypes.JSON `gorm:"type:json" json:"metadata"`
	CreatedAt          time.Time      `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	TenantID           uuid.UUID      `gorm:"type:uuid;index:idx_agent_api_key_usage_logs_tenant_id" json:"tenant_id"`
}

// TableName returns the table name for APIKeyUsageLog
func (APIKeyUsageLog) TableName() string {
	return "agent_api_key_usage_logs"
}

// APIKeyUsageLogResponse represents the response for API key usage log
type APIKeyUsageLogResponse struct {
	ID            uuid.UUID  `json:"id"`
	APIKeyID      uuid.UUID  `json:"api_key_id"`
	AgentID       uuid.UUID  `json:"agent_id"`
	RequestTime   time.Time  `json:"request_time"`
	ResponseTime  *time.Time `json:"response_time"`
	StatusCode    int        `json:"status_code"`
	TokensUsed    int64      `json:"tokens_used"`
	ErrorMessage  *string    `json:"error_message"`
	ClientIP      *string    `json:"client_ip"`
	UserAgent     *string    `json:"user_agent"`
	RequestPath   *string    `json:"request_path"`
	RequestMethod *string    `json:"request_method"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ListAPIKeyUsageLogsResponse represents the response for listing API key usage logs
type ListAPIKeyUsageLogsResponse struct {
	Logs    []APIKeyUsageLogResponse `json:"logs"`
	Total   int64                    `json:"total"`
	Page    int                      `json:"page"`
	Limit   int                      `json:"limit"`
	HasMore bool                     `json:"has_more"`
}

// ToResponse converts APIKeyUsageLog to APIKeyUsageLogResponse
func (log *APIKeyUsageLog) ToResponse() APIKeyUsageLogResponse {
	responseTime := log.CreatedAt.Add(time.Duration(log.ResponseTimeMS) * time.Millisecond)
	userAgent := log.UserAgent
	requestPath := log.RequestPath
	clientIP := log.RequestIP
	requestMethod := ""
	return APIKeyUsageLogResponse{
		ID:            log.ID,
		APIKeyID:      log.APIKeyID,
		AgentID:       log.AgentID,
		RequestTime:   log.CreatedAt,
		ResponseTime:  &responseTime,
		StatusCode:    log.ResponseStatusCode,
		TokensUsed:    log.TokensUsed,
		ErrorMessage:  log.ErrorMessage,
		ClientIP:      &clientIP,
		UserAgent:     userAgent,
		RequestPath:   &requestPath,
		RequestMethod: &requestMethod,
		CreatedAt:     log.CreatedAt,
	}
}

// CalculateResponseTime calculates the response time in milliseconds
func (log *APIKeyUsageLog) CalculateResponseTime() *float64 {
	result := float64(log.ResponseTimeMS)
	return &result
}
