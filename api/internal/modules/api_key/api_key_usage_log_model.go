package APIKey

import (
	"time"

	"github.com/google/uuid"
)

// APIKeyUsageLog represents the agent_api_key_usage_logs table
type APIKeyUsageLog struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	APIKeyID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_api_key_usage_logs_api_key_id" json:"api_key_id"`
	AgentID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_api_key_usage_logs_agent_id" json:"agent_id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_api_key_usage_logs_tenant_id" json:"tenant_id"`
	RequestTime   time.Time  `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP;index:idx_api_key_usage_logs_request_time" json:"request_time"`
	ResponseTime  *time.Time `gorm:"type:timestamp" json:"response_time"`
	StatusCode    int        `gorm:"type:integer" json:"status_code"`
	TokensUsed    int64      `gorm:"type:bigint;default:0" json:"tokens_used"`
	ErrorMessage  *string    `gorm:"type:text" json:"error_message"`
	ClientIP      *string    `gorm:"type:varchar(45)" json:"client_ip"`      // IPv4 or IPv6
	UserAgent     *string    `gorm:"type:varchar(500)" json:"user_agent"`
	RequestPath   *string    `gorm:"type:varchar(500)" json:"request_path"`   // API endpoint path
	RequestMethod *string    `gorm:"type:varchar(10)" json:"request_method"`  // GET, POST, etc.
	CreatedAt     time.Time  `gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
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
	return APIKeyUsageLogResponse{
		ID:            log.ID,
		APIKeyID:      log.APIKeyID,
		AgentID:       log.AgentID,
		RequestTime:   log.RequestTime,
		ResponseTime:  log.ResponseTime,
		StatusCode:    log.StatusCode,
		TokensUsed:    log.TokensUsed,
		ErrorMessage:  log.ErrorMessage,
		ClientIP:      log.ClientIP,
		UserAgent:     log.UserAgent,
		RequestPath:   log.RequestPath,
		RequestMethod: log.RequestMethod,
		CreatedAt:     log.CreatedAt,
	}
}

// CalculateResponseTime calculates the response time in milliseconds
func (log *APIKeyUsageLog) CalculateResponseTime() *float64 {
	if log.ResponseTime == nil {
		return nil
	}
	duration := log.ResponseTime.Sub(log.RequestTime).Milliseconds()
	result := float64(duration)
	return &result
}
