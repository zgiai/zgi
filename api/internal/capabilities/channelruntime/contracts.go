// Package channelruntime defines transport-neutral contracts for chat channels.
package channelruntime

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

var (
	// ErrInvalidWebhook indicates that a provider callback is malformed.
	ErrInvalidWebhook = errors.New("invalid channel webhook")
	// ErrWebhookUnauthorized indicates that callback authentication failed.
	ErrWebhookUnauthorized = errors.New("channel webhook authentication failed")
	// ErrUnsupportedEvent indicates that the callback is valid but not actionable.
	ErrUnsupportedEvent = errors.New("unsupported channel event")
	// ErrDeliveryOutcomeUnknown indicates that a provider may have accepted a send.
	ErrDeliveryOutcomeUnknown = errors.New("channel delivery outcome is unknown")
)

// ResolvedChannelCredential is request-scoped provider secret material. Values
// are owned by the recipient and must never be serialized, cached, logged, or
// persisted by channelruntime consumers. Revision and Source describe the lease
// an owner-scoped PersonalConnection resolved from; they carry no secret and no
// owner identifier.
type ResolvedChannelCredential struct {
	Values  map[string]string
	Version int
	// Revision is the source connection revision the lease was read at. It is
	// zero for an installation-owned credential and lets a consumer detect a
	// stale owner-scoped PersonalConnection lease.
	Revision int
	// Source distinguishes embedded legacy credentials from an owner-scoped
	// Connector PersonalConnection so receiver selection cannot accidentally
	// run a Webhook install on a long-connection transport.
	Source CredentialSource
}

// Destroy clears request-scoped provider secret material after use.
func (credential *ResolvedChannelCredential) Destroy() {
	if credential == nil {
		return
	}
	for key := range credential.Values {
		credential.Values[key] = ""
		delete(credential.Values, key)
	}
	credential.Version = 0
	credential.Revision = 0
	credential.Source = ""
}

// WebhookRequest contains one bounded HTTP callback. Headers and Query are
// request-scoped provider metadata and must not be persisted or logged.
type WebhookRequest struct {
	Body       []byte
	Headers    http.Header
	Query      map[string][]string
	ReceivedAt time.Time
}

// WebhookResponse is a provider-owned HTTP response. It is safe to send to the
// provider and must not contain credentials or internal identifiers.
type WebhookResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// VerifiedWebhook is owned by the caller and contains decrypted provider JSON.
// ImmediateResponse is set when the provider expects a verification response
// and the callback must not enter the DigitalHuman runtime.
type VerifiedWebhook struct {
	Body              []byte
	ImmediateResponse *WebhookResponse
}

// WakeReason describes why a normalized message may wake a runtime subject.
// Providers determine the reason; DigitalHuman orchestration only applies the
// configured policy to this normalized value.
type WakeReason string

const (
	WakeReasonNone          WakeReason = "none"
	WakeReasonDirectMessage WakeReason = "direct_message"
	WakeReasonMention       WakeReason = "mention"
	WakeReasonReplyToSelf   WakeReason = "reply_to_self"
)

// InstallationIdentity is the provider-neutral identity of one installed bot.
type InstallationIdentity struct {
	ExternalTenantID string
	ExternalSelfID   string
}

// NormalizationContext contains installation-owned identity required to
// classify mentions and replies without leaking provider semantics upstream.
type NormalizationContext struct {
	Identity InstallationIdentity
}

// ConversationType is the provider-neutral shape of a channel conversation.
type ConversationType string

const (
	ConversationTypeDirect ConversationType = "direct"
	ConversationTypeGroup  ConversationType = "group"
)

// Mention identifies one external identity explicitly mentioned by a message.
type Mention struct {
	Key        string
	ExternalID string
	Name       string
}

// InboundMessage is a bounded, normalized provider event. It contains no
// verification token, encryption key, access token, cookie, or raw callback.
type InboundMessage struct {
	Provider               string
	ProviderEventID        string
	ProviderMessageID      string
	ExternalTenantID       string
	ExternalConversationID string
	ExternalThreadID       string
	RootMessageID          string
	ParentMessageID        string
	ThreadID               string
	ConversationType       ConversationType
	SenderExternalID       string
	// SenderDisplayName is a bounded provider-supplied label for the sender of
	// the current trigger. It is optional and must never be used as identity.
	SenderDisplayName string
	Text              string
	Mentions          []Mention
	WakeReason        WakeReason
	// MentionOnly is true when the provider message contains one or more
	// mentions but no remaining user-authored text after mention placeholders
	// are removed. It is a wake signal, not an inferred user task.
	MentionOnly bool
	OccurredAt  time.Time
}

// ContextStatus describes how much provider context could be prepared for one
// channel invocation. It is intentionally provider-neutral.
type ContextStatus string

const (
	ContextStatusComplete    ContextStatus = "complete"
	ContextStatusPartial     ContextStatus = "partial"
	ContextStatusUnavailable ContextStatus = "unavailable"
)

// ChannelAuthor identifies one sender inside a single provider conversation.
// ExternalID is for adapter-side correlation only and must not be rendered into
// model prompts or persisted in run traces.
type ChannelAuthor struct {
	ExternalID string
	Label      string
	// IsSelf describes the sender's relationship to the DigitalHuman that owns
	// the current run. It does not classify the sender as a person or a bot.
	IsSelf bool
}

// ChannelContextMessage is one bounded, normalized historical message.
type ChannelContextMessage struct {
	MessageID       string
	RootMessageID   string
	ParentMessageID string
	ThreadID        string
	Author          ChannelAuthor
	Text            string
	MessageType     string
	OccurredAt      time.Time
	Edited          bool
	Deleted         bool
}

// ChannelContextRequest is created by the server for one invocation. Provider
// adapters must not accept conversation or installation identifiers supplied by
// a model tool call.
type ChannelContextRequest struct {
	OrganizationID         uuid.UUID
	WorkspaceID            uuid.UUID
	ActorID                uuid.UUID
	InstallationID         uuid.UUID
	ConnectionID           uuid.UUID
	Subject                runtimeidentity.SubjectRef
	RunID                  uuid.UUID
	ExternalConversationID string
	CurrentMessageID       string
	RootMessageID          string
	ParentMessageID        string
	ThreadID               string
	CurrentSenderID        string
	// MentionOnly tells the runtime that the current query is only a channel
	// wake signal. The model must interpret the attributed conversation itself;
	// the server must not synthesize or select a candidate task.
	MentionOnly bool
	OccurredAt  time.Time
	// After is a selection-priority hint for messages sent after the previous
	// successful channel delivery. Providers must not use it as a hard history
	// query boundary because earlier nearby messages may still be relevant.
	After     *time.Time
	PageToken string
	Limit     int
}

// ChannelContextPack contains historical context plus content-free diagnostics.
// Request IDs may be persisted for diagnostics; Messages and author IDs must not.
type ChannelContextPack struct {
	Status             ContextStatus
	CurrentAuthor      ChannelAuthor
	Messages           []ChannelContextMessage
	NextPageToken      string
	TruncatedReason    string
	ProviderRequestIDs []string
	ErrorCode          string
}

// ContextReader is an optional provider capability. Adapters that do not
// implement it continue to support inbound and outbound channel delivery.
type ContextReader interface {
	ReadContext(context.Context, ChannelContextRequest) (*ChannelContextPack, error)
}

// ConversationCapability identifies one provider-neutral ability that can be
// checked against a real, provider-verified conversation.
type ConversationCapability string

const (
	ConversationCapabilityMessageHistory ConversationCapability = "message_history"
	ConversationCapabilityMemberNames    ConversationCapability = "member_names"
)

// CapabilityCheckStatus is deliberately smaller than runtime context status:
// a connection test either proved an ability or returned an actionable,
// provider-normalized failure.
type CapabilityCheckStatus string

const (
	CapabilityCheckPassed CapabilityCheckStatus = "passed"
	CapabilityCheckFailed CapabilityCheckStatus = "failed"
)

// ConversationCapabilityCheck contains only safe product diagnostics. Exact
// permission identifiers are provider-owned public configuration values, not
// credentials.
type ConversationCapabilityCheck struct {
	Capability          ConversationCapability
	Status              CapabilityCheckStatus
	FailureCode         string
	RequiredPermissions []string
}

// ConversationCapabilityTestRequest is constructed from a persisted, verified
// inbound event. External conversation identifiers must never come from a
// browser or model tool call.
type ConversationCapabilityTestRequest struct {
	OrganizationID         uuid.UUID
	WorkspaceID            uuid.UUID
	ActorID                uuid.UUID
	InstallationID         uuid.UUID
	ConnectionID           uuid.UUID
	Subject                runtimeidentity.SubjectRef
	ExternalConversationID string
	CurrentMessageID       string
	CurrentSenderID        string
	OccurredAt             time.Time
}

// ConversationCapabilityTester is an optional provider boundary for checking
// read-only abilities against one real conversation. Delivery remains proven
// separately by the durable channel outbox.
type ConversationCapabilityTester interface {
	TestConversationCapabilities(context.Context, ConversationCapabilityTestRequest) []ConversationCapabilityCheck
}

// ConnectionSpec describes an optional Connection Center credential accepted
// by a provider. DigitalHuman does not interpret provider-specific identifiers.
type ConnectionSpec struct {
	IntegrationID string
	DriverID      string
	AuthMethodID  string
}

// Provisioner owns provider-specific credential validation, merging, and
// installation identity discovery. Returned maps are owned by the caller.
type Provisioner interface {
	PrepareCredentials(current, updates map[string]string) (map[string]string, error)
	ResolveIdentity(context.Context, ResolvedChannelCredential) (InstallationIdentity, error)
	ConnectionSpec() ConnectionSpec
}

// ProviderCapabilities declares optional channel behavior without requiring
// provider-name branches in DigitalHuman orchestration.
type ProviderCapabilities struct {
	ContextRead                bool
	Edit                       bool
	Retract                    bool
	ProcessingMarker           bool
	MessageRoundTripTest       bool
	ConversationCapabilityTest bool
}

// ReceiverRequest contains one installation-scoped, request-owned credential
// lease for a provider-managed inbound connection. Receiver implementations
// must return when Context is cancelled and must not retain Credential after
// Run returns.
type ReceiverRequest struct {
	InstallationID uuid.UUID
	Identity       InstallationIdentity
	Credential     ResolvedChannelCredential
	OnConnected    func()
	// OnStateChange reports transport lifecycle changes, never message content.
	OnStateChange func(ReceiverConnectionState)
}

// InboundMessageHandler accepts a normalized provider message. Returning an
// error asks transports with acknowledgement semantics to retry the message.
type InboundMessageHandler func(context.Context, *InboundMessage) error

// Receiver is an optional long-lived inbound transport. It is deliberately
// independent from HTTP webhooks so providers can use WebSocket/Stream modes
// without leaking transport details into DigitalHuman orchestration.
type Receiver interface {
	Mode(ResolvedChannelCredential) string
	Run(context.Context, ReceiverRequest, InboundMessageHandler) error
}

// ReceiverLeaseScope optionally identifies a provider application shared by
// several installations. Its value is hashed and never exposed to the console.
type ReceiverLeaseScope interface {
	LeaseScope(ResolvedChannelCredential) string
}

// ReceiverConnectionState is the safe, credential-free state exposed to the
// console for one channel installation.
type ReceiverConnectionState string

const (
	ReceiverStateNotApplicable ReceiverConnectionState = "not_applicable"
	ReceiverStatePending       ReceiverConnectionState = "pending"
	ReceiverStateConnecting    ReceiverConnectionState = "connecting"
	ReceiverStateConnected     ReceiverConnectionState = "connected"
	ReceiverStateReconnecting  ReceiverConnectionState = "reconnecting"
	ReceiverStateDisconnected  ReceiverConnectionState = "disconnected"
	ReceiverStateError         ReceiverConnectionState = "error"
)

// ReceiverStatus contains only product-facing connection diagnostics. Error
// codes are stable internal categories and never contain provider responses or
// credentials.
type ReceiverStatus struct {
	Revision  int64                   `json:"-"`
	Mode      string                  `json:"mode"`
	State     ReceiverConnectionState `json:"state"`
	ErrorCode string                  `json:"error_code,omitempty"`
	UpdatedAt time.Time               `json:"updated_at,omitempty"`
}

// ReceiverError carries a safe connection diagnostic. Permanent failures must
// wait for an explicit retry or a configuration revision, not retry forever.
type ReceiverError struct {
	Code      string
	Retryable bool
	Cause     error
}

func (err *ReceiverError) Error() string { return "channel receiver: " + err.Code }
func (err *ReceiverError) Unwrap() error { return err.Cause }

// DeliveryRequest is the immutable intent passed to a provider adapter.
type DeliveryRequest struct {
	OrganizationID         uuid.UUID
	WorkspaceID            uuid.UUID
	ActorID                uuid.UUID
	InstallationID         uuid.UUID
	ConnectionID           uuid.UUID
	Subject                runtimeidentity.SubjectRef
	RunID                  uuid.UUID
	DeliveryID             uuid.UUID
	ExternalConversationID string
	ExternalThreadID       string
	Text                   string
	Mentions               []DeliveryMention
}

// DeliveryMention identifies one provider-native mention that should accompany
// an outbound group message. ExternalID must come from the trusted channel
// envelope; model output must never select or modify it.
type DeliveryMention struct {
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name,omitempty"`
}

// DeliveryReceipt is the safe provider evidence retained after a send.
type DeliveryReceipt struct {
	ProviderMessageID string
	ProviderRequestID string
	Replayed          bool
}

// ProcessingMarkerRequest identifies the trusted inbound provider message
// whose processing state should be reflected in the channel UI. The provider
// message ID always comes from a verified callback, never model output.
type ProcessingMarkerRequest struct {
	OrganizationID   uuid.UUID
	WorkspaceID      uuid.UUID
	InstallationID   uuid.UUID
	ConnectionID     uuid.UUID
	RunID            uuid.UUID
	InboundMessageID string
}

// ProcessingMarkerReceipt contains the provider record needed to remove a
// marker after the run and its final delivery reach a terminal state.
type ProcessingMarkerReceipt struct {
	ProviderMarkerID  string
	ProviderRequestID string
}

// ProcessingMarkerAdapter is an optional provider capability. It intentionally
// stays outside Adapter so providers without native message reactions keep
// their existing acknowledgement behavior.
type ProcessingMarkerAdapter interface {
	AddProcessingMarker(context.Context, ProcessingMarkerRequest) (*ProcessingMarkerReceipt, error)
	RemoveProcessingMarker(context.Context, ProcessingMarkerRequest, string) error
}

// Adapter is the provider boundary for authenticated inbound callbacks and
// governed outbound delivery. Target resolution remains an application concern.
type Adapter interface {
	Provider() string
	Capabilities() ProviderCapabilities
	Verify(context.Context, WebhookRequest, ResolvedChannelCredential) (*VerifiedWebhook, error)
	Normalize(context.Context, *VerifiedWebhook, NormalizationContext) (*InboundMessage, error)
	Acknowledgement() WebhookResponse
	Deliver(context.Context, DeliveryRequest) (*DeliveryReceipt, error)
	Edit(context.Context, DeliveryRequest, string) (*DeliveryReceipt, error)
	Retract(context.Context, DeliveryRequest, string) error
}
