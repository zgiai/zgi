// Package invocation defines durable events that can wake a runtime subject.
package invocation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

const (
	// SchemaVersionV1 is the first durable invocation event contract.
	SchemaVersionV1   = "invocation_event.v1"
	idempotencyPrefix = "invocation:v1:"
)

// ErrInvalidEvent indicates that an invocation event cannot be durably accepted.
var ErrInvalidEvent = errors.New("invalid invocation event")

// OriginKind identifies the transport that produced an invocation event.
type OriginKind string

const (
	// OriginManual is a direct console or internal manual request.
	OriginManual OriginKind = "manual"
	// OriginChannel is an event received from a chat channel adapter.
	OriginChannel OriginKind = "channel"
	// OriginSchedule is an event emitted by the durable scheduler.
	OriginSchedule OriginKind = "schedule"
	// OriginEmail is an event received by an email adapter.
	OriginEmail OriginKind = "email"
	// OriginWebhook is an event received by a generic webhook adapter.
	OriginWebhook OriginKind = "webhook"
	// OriginAPI is a direct API invocation.
	OriginAPI OriginKind = "api"
)

// EventType describes why a runtime subject should be considered for execution.
type EventType string

const (
	// EventTypeManual requests a manual execution.
	EventTypeManual EventType = "manual"
	// EventTypeChannelMention reports an explicit channel mention.
	EventTypeChannelMention EventType = "channel.mention"
	// EventTypeChannelMessage reports a channel message selected by wake policy.
	EventTypeChannelMessage EventType = "channel.message"
	// EventTypeSchedule reports a scheduled trigger.
	EventTypeSchedule EventType = "schedule"
	// EventTypeEmailReceived reports a received email.
	EventTypeEmailReceived EventType = "email.received"
	// EventTypeWebhook reports a generic webhook trigger.
	EventTypeWebhook EventType = "webhook"
	// EventTypeAPI reports a direct API trigger.
	EventTypeAPI EventType = "api"
)

// PrincipalType identifies who or what initiated an event.
type PrincipalType string

const (
	// PrincipalAccount is an authenticated ZGI account.
	PrincipalAccount PrincipalType = "account"
	// PrincipalExternalUser is a user identified by an external provider.
	PrincipalExternalUser PrincipalType = "external_user"
	// PrincipalSystem is a platform-owned system identity.
	PrincipalSystem PrincipalType = "system"
	// PrincipalService is an authenticated service identity.
	PrincipalService PrincipalType = "service"
)

// Origin identifies the installation and provider that produced an event.
type Origin struct {
	Kind           OriginKind `json:"kind"`
	Provider       string     `json:"provider,omitempty"`
	InstallationID string     `json:"installation_id,omitempty"`
}

// PrincipalRef identifies the event initiator for authorization and audit.
// It is never a long-term memory ownership or retrieval key.
type PrincipalRef struct {
	Type PrincipalType `json:"type"`
	ID   string        `json:"id"`
}

// ReplyAddress describes where a committed response may be delivered.
type ReplyAddress struct {
	Provider       string `json:"provider,omitempty"`
	InstallationID string `json:"installation_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	ThreadID       string `json:"thread_id,omitempty"`
	RecipientID    string `json:"recipient_id,omitempty"`
}

// Event is the durable, transport-neutral wake-up contract.
type Event struct {
	SchemaVersion  string                     `json:"schema_version"`
	ID             string                     `json:"id"`
	Type           EventType                  `json:"type"`
	Origin         Origin                     `json:"origin"`
	Target         runtimeidentity.SubjectRef `json:"target"`
	Sender         PrincipalRef               `json:"sender"`
	OccurredAt     time.Time                  `json:"occurred_at"`
	IdempotencyKey string                     `json:"idempotency_key"`
	Context        map[string]any             `json:"context,omitempty"`
	ReplyTo        *ReplyAddress              `json:"reply_to,omitempty"`
}

// ValidateEvent normalizes an event, verifies its idempotency key when
// supplied, and returns a context copy owned by the returned value.
func ValidateEvent(event Event) (Event, error) {
	event.SchemaVersion = strings.TrimSpace(event.SchemaVersion)
	if event.SchemaVersion == "" {
		event.SchemaVersion = SchemaVersionV1
	}
	if event.SchemaVersion != SchemaVersionV1 {
		return Event{}, fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidEvent, event.SchemaVersion)
	}
	event.ID = strings.TrimSpace(event.ID)
	if event.ID == "" {
		return Event{}, fmt.Errorf("%w: id is required", ErrInvalidEvent)
	}
	event.Type = EventType(strings.ToLower(strings.TrimSpace(string(event.Type))))
	if !validEventType(event.Type) {
		return Event{}, fmt.Errorf("%w: unsupported type %q", ErrInvalidEvent, event.Type)
	}

	var err error
	event.Origin, err = validateOrigin(event.Origin)
	if err != nil {
		return Event{}, err
	}
	if !eventTypeMatchesOrigin(event.Type, event.Origin.Kind) {
		return Event{}, fmt.Errorf("%w: type %q does not match origin %q", ErrInvalidEvent, event.Type, event.Origin.Kind)
	}
	event.Target, err = runtimeidentity.ValidateSubjectRef(event.Target)
	if err != nil {
		return Event{}, fmt.Errorf("%w: target: %v", ErrInvalidEvent, err)
	}
	event.Sender, err = validatePrincipal(event.Sender)
	if err != nil {
		return Event{}, err
	}
	if event.OccurredAt.IsZero() {
		return Event{}, fmt.Errorf("%w: occurred_at is required", ErrInvalidEvent)
	}
	event.OccurredAt = event.OccurredAt.UTC()
	event.Context, err = cloneContext(event.Context)
	if err != nil {
		return Event{}, err
	}
	if event.ReplyTo != nil {
		reply := normalizeReplyAddress(*event.ReplyTo)
		if reply == (ReplyAddress{}) {
			return Event{}, fmt.Errorf("%w: reply_to is empty", ErrInvalidEvent)
		}
		event.ReplyTo = &reply
	}

	wantKey, err := CanonicalIdempotencyKey(event)
	if err != nil {
		return Event{}, err
	}
	providedKey := strings.TrimSpace(event.IdempotencyKey)
	if providedKey != "" && providedKey != wantKey {
		return Event{}, fmt.Errorf("%w: idempotency_key does not match canonical event identity", ErrInvalidEvent)
	}
	event.IdempotencyKey = wantKey
	return event, nil
}

// CanonicalIdempotencyKey derives the stable key used to deduplicate transport
// retries. Message content and mutable context deliberately do not affect it.
func CanonicalIdempotencyKey(event Event) (string, error) {
	schemaVersion := strings.TrimSpace(event.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = SchemaVersionV1
	}
	if schemaVersion != SchemaVersionV1 {
		return "", fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidEvent, schemaVersion)
	}
	target, err := runtimeidentity.ValidateSubjectRef(event.Target)
	if err != nil {
		return "", fmt.Errorf("%w: target: %v", ErrInvalidEvent, err)
	}
	eventID := strings.TrimSpace(event.ID)
	if eventID == "" {
		return "", fmt.Errorf("%w: id is required", ErrInvalidEvent)
	}
	origin, err := validateOrigin(event.Origin)
	if err != nil {
		return "", err
	}
	payload := struct {
		SchemaVersion string                     `json:"schema_version"`
		EventID       string                     `json:"event_id"`
		Origin        Origin                     `json:"origin"`
		Target        runtimeidentity.SubjectRef `json:"target"`
	}{
		SchemaVersion: schemaVersion,
		EventID:       eventID,
		Origin:        origin,
		Target:        target,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode idempotency identity: %v", ErrInvalidEvent, err)
	}
	digest := sha256.Sum256(encoded)
	return idempotencyPrefix + hex.EncodeToString(digest[:]), nil
}

func validateOrigin(origin Origin) (Origin, error) {
	origin.Kind = OriginKind(strings.ToLower(strings.TrimSpace(string(origin.Kind))))
	origin.Provider = strings.ToLower(strings.TrimSpace(origin.Provider))
	origin.InstallationID = strings.TrimSpace(origin.InstallationID)
	switch origin.Kind {
	case OriginManual, OriginSchedule, OriginAPI:
	case OriginChannel, OriginEmail, OriginWebhook:
		if origin.Provider == "" {
			return Origin{}, fmt.Errorf("%w: origin.provider is required for %s", ErrInvalidEvent, origin.Kind)
		}
	default:
		return Origin{}, fmt.Errorf("%w: unsupported origin.kind %q", ErrInvalidEvent, origin.Kind)
	}
	return origin, nil
}

func validatePrincipal(principal PrincipalRef) (PrincipalRef, error) {
	principal.Type = PrincipalType(strings.ToLower(strings.TrimSpace(string(principal.Type))))
	principal.ID = strings.TrimSpace(principal.ID)
	switch principal.Type {
	case PrincipalAccount, PrincipalExternalUser, PrincipalSystem, PrincipalService:
	default:
		return PrincipalRef{}, fmt.Errorf("%w: unsupported sender.type %q", ErrInvalidEvent, principal.Type)
	}
	if principal.ID == "" {
		return PrincipalRef{}, fmt.Errorf("%w: sender.id is required", ErrInvalidEvent)
	}
	return principal, nil
}

func validEventType(eventType EventType) bool {
	switch eventType {
	case EventTypeManual, EventTypeChannelMention, EventTypeChannelMessage, EventTypeSchedule,
		EventTypeEmailReceived, EventTypeWebhook, EventTypeAPI:
		return true
	default:
		return false
	}
}

func eventTypeMatchesOrigin(eventType EventType, origin OriginKind) bool {
	switch eventType {
	case EventTypeManual:
		return origin == OriginManual
	case EventTypeChannelMention, EventTypeChannelMessage:
		return origin == OriginChannel
	case EventTypeSchedule:
		return origin == OriginSchedule
	case EventTypeEmailReceived:
		return origin == OriginEmail
	case EventTypeWebhook:
		return origin == OriginWebhook
	case EventTypeAPI:
		return origin == OriginAPI
	default:
		return false
	}
}

func cloneContext(context map[string]any) (map[string]any, error) {
	if len(context) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return nil, fmt.Errorf("%w: context is not JSON serializable: %v", ErrInvalidEvent, err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("%w: decode context copy: %v", ErrInvalidEvent, err)
	}
	return cloned, nil
}

func normalizeReplyAddress(reply ReplyAddress) ReplyAddress {
	reply.Provider = strings.ToLower(strings.TrimSpace(reply.Provider))
	reply.InstallationID = strings.TrimSpace(reply.InstallationID)
	reply.ConversationID = strings.TrimSpace(reply.ConversationID)
	reply.ThreadID = strings.TrimSpace(reply.ThreadID)
	reply.RecipientID = strings.TrimSpace(reply.RecipientID)
	return reply
}
