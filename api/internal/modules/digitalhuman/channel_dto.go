package digitalhuman

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

type ChannelContextReadinessStatus string

const (
	ChannelContextReadinessReady                 ChannelContextReadinessStatus = "ready"
	ChannelContextReadinessRuntimeVerification   ChannelContextReadinessStatus = "runtime_verification"
	ChannelContextReadinessMissingMessageHistory ChannelContextReadinessStatus = "missing_message_history"
	ChannelContextReadinessMissingMemberNames    ChannelContextReadinessStatus = "missing_member_names"
	ChannelContextReadinessUnavailable           ChannelContextReadinessStatus = "temporarily_unavailable"
)

type ChannelContextReadiness struct {
	Status                  ChannelContextReadinessStatus `json:"status"`
	MessageHistoryAvailable bool                          `json:"message_history_available"`
	MemberNamesAvailable    bool                          `json:"member_names_available"`
}

type ChannelInstallationView struct {
	*channelruntime.Installation
	ChannelContext ChannelContextReadiness       `json:"channel_context"`
	WebhookURL     string                        `json:"webhook_url,omitempty"`
	CredentialMode string                        `json:"credential_mode"`
	Receiver       channelruntime.ReceiverStatus `json:"receiver"`
}

type CreateChannelInstallationRequest struct {
	Provider     string            `json:"provider"`
	ConnectionID uuid.UUID         `json:"connection_id,omitempty"`
	Credentials  map[string]string `json:"credentials,omitempty"`
	Name         string            `json:"name"`
}

type UpdateChannelInstallationCredentialRequest struct {
	ExpectedRevision int64             `json:"expected_revision"`
	Credentials      map[string]string `json:"credentials"`
}

type UpdateChannelInstallationRequest struct {
	ExpectedRevision int64                              `json:"expected_revision"`
	Name             *string                            `json:"name,omitempty"`
	Status           *channelruntime.InstallationStatus `json:"status,omitempty"`
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
	Response  channelruntime.WebhookResponse `json:"-"`
	Accepted  bool                           `json:"accepted"`
	Duplicate bool                           `json:"duplicate"`
	Ignored   bool                           `json:"ignored"`
	EventID   uuid.UUID                      `json:"event_id,omitempty"`
	RunID     *uuid.UUID                     `json:"run_id,omitempty"`
}
