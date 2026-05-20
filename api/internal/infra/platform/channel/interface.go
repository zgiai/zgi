package channel

import (
	"context"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
)

// OfficialChannel represents a platform-provided channel for routing decisions.
type OfficialChannel struct {
	ID         string
	Name       string
	Provider   string
	Models     []string
	Priority   int
	Weight     int
	APIBaseURL string // console-api internal endpoint (e.g. "https://console.zgi.ai/v1/internal")
}

// ChannelProvider defines the interface for fetching official channels.
type ChannelProvider interface {
	// ListChannels retrieves the list of official channels available for routing
	ListChannels(ctx context.Context, tenantID string) ([]*OfficialChannel, error)
}

// ToTenantRoute converts an OfficialChannel to a virtual LLMRoute for unified routing.
// The route is treated as a standard adapter route with base_url pointing to console-api.
// IsOfficial is still set for per-org priority/weight management.
func (o *OfficialChannel) ToTenantRoute(tenantID uuid.UUID) *channelmodel.LLMRoute {
	return &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  tenantID,
		Type:            shared.RouteTypeZGICloud,
		Name:            o.Name,
		ChannelProvider: o.Provider,
		APIBaseURL:      o.APIBaseURL,
		Models:          o.Models,
		Priority:        o.Priority,
		Weight:          o.Weight,
		IsEnabled:       true,
		IsOfficial:      true,
	}
}
