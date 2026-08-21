package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

type CreateChannelInstallationRequest struct {
	Provider     string    `json:"provider"`
	ConnectionID uuid.UUID `json:"connection_id"`
	Name         string    `json:"name"`
}

type UpdateChannelInstallationRequest struct {
	ExpectedRevision int64                              `json:"expected_revision"`
	Name             *string                            `json:"name,omitempty"`
	Status           *channelruntime.InstallationStatus `json:"status,omitempty"`
}

// DeleteChannelInstallationRequest controls whether deleting a workspace
// installation also removes its live conversation bindings. Historical events
// and delivery records remain available for audit.
type DeleteChannelInstallationRequest struct {
	RemoveBindings bool `json:"remove_bindings" form:"remove_bindings"`
}

// ChannelInstallationUsage identifies a DigitalHuman that currently uses an
// installation. Installations are workspace resources, so usage must not be
// inferred from only the DigitalHuman whose page requested the list.
type ChannelInstallationUsage struct {
	DigitalHumanID   uuid.UUID `json:"digital_human_id"`
	DigitalHumanName string    `json:"digital_human_name"`
	BindingCount     int64     `json:"binding_count"`
}

// ChannelInstallationView exposes workspace-wide usage alongside the
// installation so clients can explain and prevent unsafe deletion.
type ChannelInstallationView struct {
	*channelruntime.Installation
	BindingCount int64                       `json:"binding_count"`
	UsedBy       []*ChannelInstallationUsage `json:"used_by"`
}

// ChannelConversationDiscovery is a verified provider conversation observed
// through an installation but not yet assigned to a DigitalHuman. It contains
// routing identifiers only; inbound message bodies remain outside discovery.
type ChannelConversationDiscovery struct {
	InstallationID         uuid.UUID                       `json:"installation_id"`
	ExternalConversationID string                          `json:"external_conversation_id"`
	ConversationType       channelruntime.ConversationType `json:"conversation_type"`
	SenderExternalID       string                          `json:"sender_external_id"`
	EventCount             int64                           `json:"event_count"`
	LastEventAt            time.Time                       `json:"last_event_at"`
}

type CreateChannelBindingRequest struct {
	InstallationID         uuid.UUID                       `json:"installation_id"`
	ExternalConversationID string                          `json:"external_conversation_id"`
	ConversationType       channelruntime.ConversationType `json:"conversation_type"`
	WakePolicy             channelruntime.WakePolicy       `json:"wake_policy"`
}

type ChannelEventListResult struct {
	Items    []*channelruntime.InboundEvent `json:"items"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
	HasMore  bool                           `json:"has_more"`
}

type ChannelDeliveryListResult struct {
	Items    []*channelruntime.Delivery `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	HasMore  bool                       `json:"has_more"`
}

type ChannelWebhookResult struct {
	Challenge string     `json:"challenge,omitempty"`
	Accepted  bool       `json:"accepted"`
	Duplicate bool       `json:"duplicate"`
	Ignored   bool       `json:"ignored"`
	EventID   uuid.UUID  `json:"event_id,omitempty"`
	RunID     *uuid.UUID `json:"run_id,omitempty"`
}
