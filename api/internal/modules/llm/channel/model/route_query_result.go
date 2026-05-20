package model

import (
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
)

// ============================================================================
// Route Query Result - for load balancing
// ============================================================================

// RouteSource indicates whether a route is explicitly configured or implicitly inherited
type RouteSource string

const (
	RouteSourceExplicit RouteSource = "EXPLICIT"
	RouteSourceImplicit RouteSource = "IMPLICIT"
)

// RouteQueryResult represents the result of a route query for load balancing
type RouteQueryResult struct {
	RouteID          uuid.UUID              `json:"route_id"`
	OrganizationID   uuid.UUID              `json:"organization_id"`
	Type             shared.RouteType       `json:"type"`
	Source           RouteSource            `json:"source"`
	Name             string                 `json:"name"`
	ChannelProvider  string                 `json:"channel_provider"`
	Models           []string               `gorm:"type:jsonb;serializer:json" json:"models"`
	APIBaseURL       string                 `json:"api_base_url"`
	Priority         int                    `json:"priority"`
	Weight           int                    `json:"weight"`
	APIKeyCiphertext string                 `json:"-"`
	ModelMaps        map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"model_maps,omitempty"`
	ParamOverride    map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"param_override,omitempty"`
	HeaderOverride   map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"header_override,omitempty"`
}

// IsImplicit returns true if this route is implicitly inherited from system channel
func (r *RouteQueryResult) IsImplicit() bool {
	return r.Source == RouteSourceImplicit
}
