package dto

import (
	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

// ============================================================================
// Request types
// ============================================================================
// Note: System channel management has been moved to console-api
// This file only contains tenant route (channel) DTOs

type SyncChannelToTenantsRequest struct {
	ChannelID uuid.UUID `json:"channel_id" binding:"required"`
}

type SyncChannelToTenantsResponse struct {
	ChannelID     uuid.UUID `json:"channel_id"`
	ChannelName   string    `json:"channel_name"`
	TotalTenants  int       `json:"total_tenants"`
	RoutesCreated int       `json:"routes_created"`
	RoutesUpdated int       `json:"routes_updated"`
	RoutesFailed  int       `json:"routes_failed"`
	Errors        []string  `json:"errors,omitempty"`
}

type ListChannelRequest struct {
	ChannelProvider string `form:"channel_provider"`
	IsActive        *bool  `form:"is_active"`
	Page            int    `form:"page,default=1"`
	PageSize        int    `form:"page_size,default=20"`
}

type CreateRouteRequest struct {
	Name            string                            `json:"name" binding:"required"`
	ChannelProvider string                            `json:"channel_provider" binding:"required"`
	APIKey          string                            `json:"api_key"`
	Models          []string                          `json:"models"`
	APIBaseURL      string                            `json:"api_base_url"`
	NativeProtocols channelmodel.NativeProtocolConfig `json:"native_protocols"`
	InitialFunds    int64                             `json:"initial_funds" binding:"gte=0"`
	ModelMaps       map[string]any                    `json:"model_maps"`
	ParamOverride   map[string]any                    `json:"param_override"`
	HeaderOverride  map[string]any                    `json:"header_override"`
	Tags            []string                          `json:"tags"`
	Description     string                            `json:"description"`
	Priority        int                               `json:"priority"`
	Weight          int                               `json:"weight"`
}

type UpdateRouteRequest struct {
	Name            *string                            `json:"name"`
	ChannelProvider *string                            `json:"channel_provider"`
	Models          []string                           `json:"models"`
	APIBaseURL      *string                            `json:"api_base_url"`
	NativeProtocols *channelmodel.NativeProtocolConfig `json:"native_protocols"`
	ModelMaps       map[string]any                     `json:"model_maps"`
	ParamOverride   map[string]any                     `json:"param_override"`
	HeaderOverride  map[string]any                     `json:"header_override"`
	Tags            []string                           `json:"tags"`
	Description     *string                            `json:"description"`
	Priority        *int                               `json:"priority"`
	Weight          *int                               `json:"weight"`
	IsEnabled       *bool                              `json:"is_enabled"`

	// API Key - if provided, will update the associated credential's API key
	APIKey *string `json:"api_key"`
}

type ListRouteRequest struct {
	IsEnabled *bool `form:"is_enabled"`
	Page      int   `form:"page,default=1"`
	PageSize  int   `form:"page_size,default=20"`
}

// ListRoutesAggregatedRequest represents the request to list aggregated routes with pagination and search
type ListRoutesAggregatedRequest struct {
	Search          string `form:"search"`
	ChannelProvider string `form:"channel_provider"`
	Page            int    `form:"page,default=1"`
	PageSize        int    `form:"page_size,default=20"`
}

// SelectRouteRequest represents the request to select a route for a model
type SelectRouteRequest struct {
	Model string `json:"model" binding:"required"`
}

// ToggleRouteRequest represents the request to toggle a route's enabled status
type ToggleRouteRequest struct {
	IsEnabled bool `json:"is_enabled"`
}

// ============================================================================
// Response types
// ============================================================================

// ChannelView represents a clean channel response for private channel list/detail APIs
type ChannelView struct {
	ID               uuid.UUID                         `json:"id"`
	Name             string                            `json:"name"`
	Type             string                            `json:"type,omitempty"` // ZGI_CLOUD or PRIVATE
	ChannelProvider  string                            `json:"channel_provider,omitempty"`
	Models           []string                          `json:"models,omitempty"`
	RemainingFunds   int64                             `json:"remaining_funds"`
	APIBaseURL       string                            `json:"api_base_url,omitempty"`
	NativeProtocols  channelmodel.NativeProtocolConfig `json:"native_protocols,omitempty"`
	ValidationReport map[string]any                    `json:"validation_report,omitempty"`
	Warnings         []string                          `json:"warnings,omitempty"`
	Priority         int                               `json:"priority"`
	Weight           int                               `json:"weight"`
	IsEnabled        bool                              `json:"is_enabled"`
	AutoBan          bool                              `json:"auto_ban"`
	Tags             []string                          `json:"tags,omitempty"`
	Description      string                            `json:"description,omitempty"`
	APIKeyMasked     string                            `json:"api_key_masked,omitempty"`
	CreatedAt        int64                             `json:"created_at"`
	UpdatedAt        int64                             `json:"updated_at"`
}

// PlatformChannelView represents a single ZGI Cloud official channel route.
// Each route has its own priority/weight for load balancing with tenant's private channels.
type PlatformChannelView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider,omitempty"`
	ModelCount int    `json:"model_count"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
	IsEnabled  bool   `json:"is_enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

// UpdatePlatformChannelRequest represents the request to update a platform channel's routing fields.
type UpdatePlatformChannelRequest struct {
	Priority  *int  `json:"priority"`
	Weight    *int  `json:"weight"`
	IsEnabled *bool `json:"is_enabled"`
}

// PlatformChannelAggregatedView represents the official channel as a single entity.
// All underlying platform channels are aggregated into one view for frontend display.
type PlatformChannelAggregatedView struct {
	Name       string `json:"name"`
	ModelCount int    `json:"model_count"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
	IsEnabled  bool   `json:"is_enabled"`
}

// PlatformChannelListResponse represents the response for the platform channel endpoint
type PlatformChannelListResponse struct {
	Channels []*PlatformChannelView `json:"list"`
	Total    int                    `json:"total"`
}

// ChannelListResponse represents the unified response for channel list
type ChannelListResponse struct {
	Channels []*ChannelView `json:"data"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// TestRouteRequest represents the optional request body for testing a route
type TestRouteRequest struct {
	Model string `json:"model"` // Optional: specific model to test; if empty, uses the first model in the route
}

// TestChannelResult represents the result of testing a channel or route
type TestChannelResult struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	ResponseTime int64    `json:"response_time_ms"`
	Models       []string `json:"models,omitempty"`
}

type DraftTestChannelModelRequest struct {
	ChannelProvider string `json:"channel_provider" binding:"required"`
	APIKey          string `json:"api_key"`
	APIBaseURL      string `json:"api_base_url"`
	Model           string `json:"model" binding:"required"`
	TestMethod      string `json:"test_method"`
	Stream          bool   `json:"stream"`
}

type DiscoverDraftChannelModelsRequest struct {
	ChannelProvider string `json:"channel_provider" binding:"required"`
	APIKey          string `json:"api_key"`
	APIBaseURL      string `json:"api_base_url"`
}

type DiscoveredChannelModelView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Provider      string   `json:"provider,omitempty"`
	OwnedBy       string   `json:"owned_by,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Created       int64    `json:"created,omitempty"`
}

type DiscoverDraftChannelModelsResponse struct {
	Models           []DiscoveredChannelModelView `json:"models"`
	Total            int                          `json:"total"`
	ListingSupported bool                         `json:"listing_supported"`
}

type DiscoverOllamaModelsRequest struct {
	APIBaseURL string `json:"api_base_url" binding:"required"`
	APIKey     string `json:"api_key"`
}

type OllamaModelView struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	UseCase      string   `json:"use_case"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type DiscoverOllamaModelsResponse struct {
	Models []OllamaModelView `json:"models"`
	Total  int               `json:"total"`
}

type ChannelModelTestResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	Model          string `json:"model"`
	UseCase        string `json:"use_case,omitempty"`
	TestMethod     string `json:"test_method,omitempty"`
	ResponseTimeMs int64  `json:"response_time_ms"`
}

type UpdateChannelBalanceResponse struct {
	ChannelID   uuid.UUID `json:"channel_id"`
	OldBalance  string    `json:"old_balance"`
	NewBalance  string    `json:"new_balance"`
	Currency    string    `json:"currency"`
	UpdatedAt   string    `json:"updated_at"` // ISO8601 string
	IsUnlimited bool      `json:"is_unlimited"`
}

type AdjustChannelWalletRequest struct {
	Amount int64  `json:"amount" binding:"required,ne=0"`
	Note   string `json:"note"`
}

type AdjustChannelWalletResponse struct {
	ChannelID      uuid.UUID `json:"channel_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Amount         int64     `json:"amount"`
	BalanceBefore  int64     `json:"balance_before"`
	BalanceAfter   int64     `json:"balance_after"`
	Status         string    `json:"status"`
	TransactionID  uuid.UUID `json:"transaction_id"`
	UpdatedAt      string    `json:"updated_at"`
}

type TestChannelModelRequest struct {
	Model      string `json:"model" binding:"required"`
	TestMethod string `json:"test_method"` // chat, embedding, image-gen, rerank
	Stream     bool   `json:"stream"`
}

type BatchTestChannelModelsRequest struct {
	Models     []string `json:"models" binding:"required"`
	TestMethod string   `json:"test_method"`
	Stream     bool     `json:"stream"`
}

type BatchTestChannelModelsStreamResponse struct {
	Model        string `json:"model"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ResponseTime int64  `json:"response_time_ms"`
	Completed    bool   `json:"completed"` // True if this is the final message closing the stream
}

// UpdateOfficialChannelSettingsRequest represents the request to update official channel group settings
type UpdateOfficialChannelSettingsRequest struct {
	GroupID   string `json:"-"`
	Priority  *int   `json:"priority"`
	Weight    *int   `json:"weight"`
	IsEnabled *bool  `json:"is_enabled"`
}

// ============================================================================
// Batch operation types
// ============================================================================

// BatchToggleRoutesRequest represents the request to batch toggle multiple routes
type BatchToggleRoutesRequest struct {
	RouteIDs  []uuid.UUID `json:"route_ids" binding:"required,min=1"`
	IsEnabled bool        `json:"is_enabled"`
}

// BatchDeleteRoutesRequest represents the request to batch delete multiple routes
type BatchDeleteRoutesRequest struct {
	RouteIDs []uuid.UUID `json:"route_ids" binding:"required,min=1"`
}

// BatchOperationResult represents the result of a batch operation
type BatchOperationResult struct {
	TotalCount   int      `json:"total_count"`
	SuccessCount int      `json:"success_count"`
	FailedCount  int      `json:"failed_count"`
	FailedIDs    []string `json:"failed_ids,omitempty"`
}
