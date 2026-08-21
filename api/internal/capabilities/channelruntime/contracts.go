// Package channelruntime defines transport-neutral contracts for chat channels.
package channelruntime

import (
	"context"
	"errors"
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

// SecurityMaterial is request-scoped provider secret material. It must never be
// serialized, cached, logged, or persisted by channelruntime consumers.
type SecurityMaterial struct {
	VerificationToken string
	EncryptKey        string
}

// Destroy clears request-scoped provider secret material after verification.
func (material *SecurityMaterial) Destroy() {
	if material == nil {
		return
	}
	material.VerificationToken = ""
	material.EncryptKey = ""
}

// WebhookRequest contains the exact callback bytes and authentication headers.
type WebhookRequest struct {
	Body       []byte
	Timestamp  string
	Nonce      string
	Signature  string
	ReceivedAt time.Time
}

// VerifiedWebhook is owned by the caller and contains decrypted provider JSON.
// Challenge is non-empty only for provider URL verification requests.
type VerifiedWebhook struct {
	Body      []byte
	Challenge string
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
	ConversationType       ConversationType
	SenderExternalID       string
	Text                   string
	Mentions               []Mention
	OccurredAt             time.Time
}

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
}

// DeliveryReceipt is the safe provider evidence retained after a send.
type DeliveryReceipt struct {
	ProviderMessageID string
	ProviderRequestID string
	Replayed          bool
}

// Adapter is the provider boundary for authenticated inbound callbacks and
// governed outbound delivery. Target resolution remains an application concern.
type Adapter interface {
	Provider() string
	Verify(context.Context, WebhookRequest, SecurityMaterial) (*VerifiedWebhook, error)
	Normalize(context.Context, *VerifiedWebhook) (*InboundMessage, error)
	Deliver(context.Context, DeliveryRequest) (*DeliveryReceipt, error)
	Edit(context.Context, DeliveryRequest, string) (*DeliveryReceipt, error)
	Retract(context.Context, DeliveryRequest, string) error
}
