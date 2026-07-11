package APIKey

import (
	"time"

	"github.com/google/uuid"
)

// APIKey represents an API key record in the database
type APIKey struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AgentID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"agent_id"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	KeyHash    string     `gorm:"type:varchar(255);not null;unique" json:"-"`
	KeyPrefix  string     `gorm:"type:varchar(20);not null" json:"key_prefix"`
	Name       string     `gorm:"type:varchar(255);not null" json:"name"`
	Status     string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	ExpiresAt  *time.Time `gorm:"type:timestamp" json:"expires_at"`
	UsageCount int64      `gorm:"default:0" json:"usage_count"`
	LastUsedAt *time.Time `gorm:"type:timestamp" json:"last_used_at"`
	CreatedAt  time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName returns the table name for the APIKey model
func (APIKey) TableName() string {
	return "agent_api_keys"
}

// CreateAPIKeyRequest represents the request to create a new API key
type CreateAPIKeyRequest struct {
	AgentID   uuid.UUID  `json:"agent_id" binding:"required"`
	Name      string     `json:"name" binding:"required,min=1,max=255"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyRequestWithoutAgentID represents the request to create an API key without agent_id in body
type CreateAPIKeyRequestWithoutAgentID struct {
	Name      string     `json:"name" binding:"required,min=1,max=255"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UpdateAPIKeyRequest represents the request to update an API key
type UpdateAPIKeyRequest struct {
	Name   *string `json:"name,omitempty" binding:"omitempty,min=1,max=255"`
	Status *string `json:"status,omitempty" binding:"omitempty,oneof=active inactive"`
}

// APIKeyResponse represents the response when returning API key information
type APIKeyResponse struct {
	ID        uuid.UUID  `json:"id"`
	AgentID   uuid.UUID  `json:"agent_id"`
	KeyPrefix string     `json:"key_prefix"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateAPIKeyResponse represents the response when creating a new API key
type CreateAPIKeyResponse struct {
	APIKeyResponse
	APIKey string `json:"api_key"` // Only returned once during creation
}

// ListAPIKeysResponse represents the response for listing API keys
type ListAPIKeysResponse struct {
	APIKeys []APIKeyResponse `json:"api_keys"`
	Total   int              `json:"total"`
}

// APIKeyStatus constants
const (
	APIKeyStatusActive   = "active"
	APIKeyStatusInactive = "inactive"
	APIKeyStatusRevoked  = "revoked"
)

// ToResponse converts APIKey model to APIKeyResponse
func (a *APIKey) ToResponse() APIKeyResponse {
	return APIKeyResponse{
		ID:        a.ID,
		AgentID:   a.AgentID,
		KeyPrefix: a.KeyPrefix,
		Name:      a.Name,
		Status:    a.Status,
		ExpiresAt: a.ExpiresAt,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

// ToCreateResponse converts APIKey model to CreateAPIKeyResponse
func (a *APIKey) ToCreateResponse(apiKey string) CreateAPIKeyResponse {
	return CreateAPIKeyResponse{
		APIKeyResponse: a.ToResponse(),
		APIKey:         apiKey,
	}
}

// IsExpired checks if the API key is expired
func (a *APIKey) IsExpired() bool {
	if a.ExpiresAt == nil {
		return false
	}
	return a.ExpiresAt.Before(time.Now())
}

// IsActive checks if the API key is active and not expired
func (a *APIKey) IsActive() bool {
	return a.Status == APIKeyStatusActive && !a.IsExpired()
}
