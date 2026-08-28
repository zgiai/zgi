package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

type ChannelContextReadinessStatus string

const (
	ChannelContextReadinessReady                 ChannelContextReadinessStatus = "ready"
	ChannelContextReadinessRuntimeVerification   ChannelContextReadinessStatus = "runtime_verification"
	ChannelContextReadinessMissingMessageHistory ChannelContextReadinessStatus = "missing_message_history"
	ChannelContextReadinessMissingMemberNames    ChannelContextReadinessStatus = "missing_member_names"
	ChannelContextReadinessNotSupported          ChannelContextReadinessStatus = "not_supported"
	ChannelContextReadinessUnavailable           ChannelContextReadinessStatus = "temporarily_unavailable"
)

type ChannelContextReadiness struct {
	Status                  ChannelContextReadinessStatus `json:"status"`
	MessageHistoryAvailable bool                          `json:"message_history_available"`
	MemberNamesAvailable    bool                          `json:"member_names_available"`
}

type ChannelInstallationView struct {
	*channelruntime.Installation
	ChannelContext   ChannelContextReadiness       `json:"channel_context"`
	WebhookURL       string                        `json:"webhook_url,omitempty"`
	CredentialMode   string                        `json:"credential_mode"`
	IdentityVerified bool                          `json:"identity_verified"`
	Receiver         channelruntime.ReceiverStatus `json:"receiver"`
}

// ChannelInstallationVerification is the result of an explicit, read-only
// connection check. Provider runtime capabilities are deliberately left in
// ChannelContextReadiness because message history and delivery can only be
// verified against a real external conversation.
type ChannelInstallationVerification struct {
	Status              string                                  `json:"status"`
	CredentialVerified  bool                                    `json:"credential_verified"`
	BotIdentityVerified bool                                    `json:"bot_identity_verified"`
	ChannelContext      ChannelContextReadiness                 `json:"channel_context"`
	CheckedAt           time.Time                               `json:"checked_at"`
	Failure             *ChannelInstallationVerificationFailure `json:"failure,omitempty"`
}

type ChannelInstallationVerificationFailure struct {
	Code                 string `json:"code"`
	ReasonCode           string `json:"reason_code,omitempty"`
	ProviderErrorCode    string `json:"provider_error_code,omitempty"`
	ProviderRequestID    string `json:"provider_request_id,omitempty"`
	ProviderHTTPStatus   int    `json:"provider_http_status,omitempty"`
	ProviderInvalidField string `json:"provider_invalid_field,omitempty"`
	Retryable            bool   `json:"retryable"`
}

type ChannelConnectionTestStatus string

const (
	ChannelConnectionTestConfigurationFailed ChannelConnectionTestStatus = "configuration_failed"
	ChannelConnectionTestAwaitingMessage     ChannelConnectionTestStatus = "awaiting_message"
	ChannelConnectionTestMessageReceived     ChannelConnectionTestStatus = "message_received"
	ChannelConnectionTestReplying            ChannelConnectionTestStatus = "replying"
	ChannelConnectionTestSucceeded           ChannelConnectionTestStatus = "succeeded"
	ChannelConnectionTestFailed              ChannelConnectionTestStatus = "failed"
	ChannelConnectionTestExpired             ChannelConnectionTestStatus = "expired"
)

type ChannelConnectionTestReadiness string

const (
	ChannelConnectionTestReadinessPending     ChannelConnectionTestReadiness = "pending"
	ChannelConnectionTestReadinessAvailable   ChannelConnectionTestReadiness = "available"
	ChannelConnectionTestReadinessPartial     ChannelConnectionTestReadiness = "partial"
	ChannelConnectionTestReadinessUnavailable ChannelConnectionTestReadiness = "unavailable"
)

type ChannelConnectionCapabilityStatus string

const (
	ChannelConnectionCapabilityPending ChannelConnectionCapabilityStatus = "pending"
	ChannelConnectionCapabilityPassed  ChannelConnectionCapabilityStatus = "passed"
	ChannelConnectionCapabilityFailed  ChannelConnectionCapabilityStatus = "failed"
)

const (
	ChannelConnectionCapabilityCredentials    = "credentials"
	ChannelConnectionCapabilityBotIdentity    = "bot_identity"
	ChannelConnectionCapabilityDirectInbound  = "direct_inbound"
	ChannelConnectionCapabilityGroupMention   = "group_mention"
	ChannelConnectionCapabilityMessageHistory = "message_history"
	ChannelConnectionCapabilityMemberNames    = "member_names"
	ChannelConnectionCapabilityReplyDelivery  = "reply_delivery"
)

type ChannelConnectionCapability struct {
	ID                  string                            `json:"id"`
	Status              ChannelConnectionCapabilityStatus `json:"status"`
	FailureCode         string                            `json:"failure_code,omitempty"`
	RequiredPermissions []string                          `json:"required_permissions,omitempty"`
}

// ChannelConnectionTest reports a real inbound-message and final-reply round
// trip. TestCode and TestMessage are operator instructions, not credentials.
type ChannelConnectionTest struct {
	ID               uuid.UUID                        `json:"id,omitempty"`
	Status           ChannelConnectionTestStatus      `json:"status"`
	Readiness        ChannelConnectionTestReadiness   `json:"readiness"`
	ConversationType channelruntime.ConversationType  `json:"conversation_type"`
	Verification     *ChannelInstallationVerification `json:"verification,omitempty"`
	TestCode         string                           `json:"test_code,omitempty"`
	TestMessage      string                           `json:"test_message,omitempty"`
	StartedAt        time.Time                        `json:"started_at,omitempty"`
	ExpiresAt        time.Time                        `json:"expires_at,omitempty"`
	MessageReceived  bool                             `json:"message_received"`
	ReplyDelivered   bool                             `json:"reply_delivered"`
	InboundEventID   *uuid.UUID                       `json:"inbound_event_id,omitempty"`
	DeliveryID       *uuid.UUID                       `json:"delivery_id,omitempty"`
	FailureCode      string                           `json:"failure_code,omitempty"`
	Capabilities     []ChannelConnectionCapability    `json:"capabilities"`
}

type StartChannelConnectionTestRequest struct {
	ConversationType channelruntime.ConversationType `json:"conversation_type"`
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
