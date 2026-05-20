package dto

import (
	"time"

	"github.com/google/uuid"
)

// ModelAvailabilityStatus represents the availability status of a model
type ModelAvailabilityStatus string

const (
	ModelAvailable   ModelAvailabilityStatus = "available"   // Has working channels
	ModelPartial     ModelAvailabilityStatus = "partial"     // Has channels but need config
	ModelUnavailable ModelAvailabilityStatus = "unavailable" // No channels
)

// ModelAvailability represents the availability info for a model
type ModelAvailability struct {
	ModelID     uuid.UUID               `json:"model_id"`
	ModelName   string                  `json:"model_name"`
	Provider    string                  `json:"provider"`
	Status      ModelAvailabilityStatus `json:"status"`
	ChannelInfo ChannelAvailabilityInfo `json:"channel_info"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// ChannelAvailabilityInfo provides details about available channels
type ChannelAvailabilityInfo struct {
	TotalCount       int                `json:"total_count"`
	ReadyCount       int                `json:"ready_count"`
	NeedsConfigCount int                `json:"needs_config_count"`
	Channels         []ChannelBriefInfo `json:"channels"`
	Warnings         []string           `json:"warnings,omitempty"`
}

// ChannelBriefInfo provides brief info about a channel
type ChannelBriefInfo struct {
	ID       uuid.UUID         `json:"id"`
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Status   ChannelStatusType `json:"status"`
	Priority int               `json:"priority"`
	Weight   int               `json:"weight"`
}

// ChannelStatusType represents the status of a channel
type ChannelStatusType string

const (
	ChannelReady       ChannelStatusType = "ready"
	ChannelNeedsConfig ChannelStatusType = "needs_config"
	ChannelInactive    ChannelStatusType = "inactive"
	ChannelError       ChannelStatusType = "error"
)

// BatchCheckRequest is the request to batch check model availability
type BatchCheckRequest struct {
	ModelIDs []uuid.UUID `json:"model_ids" binding:"required"`
}

// BatchCheckResponse is the response of batch check
type BatchCheckResponse struct {
	Results []*ModelAvailability `json:"results"`
}
