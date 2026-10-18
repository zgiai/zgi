package invocation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

func TestValidateEventProducesStableIdempotencyKey(t *testing.T) {
	subject := validDigitalHumanSubject()
	occurredAt := time.Date(2026, time.August, 18, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	input := Event{
		ID:   " event-123 ",
		Type: " CHANNEL.MENTION ",
		Origin: Origin{
			Kind:           " CHANNEL ",
			Provider:       " FEISHU ",
			InstallationID: " tenant-installation ",
		},
		Target:     subject,
		Sender:     PrincipalRef{Type: " EXTERNAL_USER ", ID: " user-1 "},
		OccurredAt: occurredAt,
		Context:    map[string]any{"message": map[string]any{"id": "message-1"}},
		ReplyTo:    &ReplyAddress{Provider: " FEISHU ", ConversationID: " chat-1 "},
	}

	first, err := ValidateEvent(input)
	if err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
	secondInput := input
	secondInput.Context = map[string]any{"message": map[string]any{"id": "message-1", "edited": true}}
	second, err := ValidateEvent(secondInput)
	if err != nil {
		t.Fatalf("ValidateEvent(second) error = %v", err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("idempotency keys differ: %q vs %q", first.IdempotencyKey, second.IdempotencyKey)
	}
	if !strings.HasPrefix(first.IdempotencyKey, idempotencyPrefix) {
		t.Fatalf("IdempotencyKey = %q, want prefix %q", first.IdempotencyKey, idempotencyPrefix)
	}
	if first.OccurredAt.Location() != time.UTC {
		t.Fatalf("OccurredAt location = %v, want UTC", first.OccurredAt.Location())
	}
	if first.Origin.Provider != "feishu" || first.Sender.ID != "user-1" {
		t.Fatalf("event not normalized: %#v", first)
	}

	input.Context["message"].(map[string]any)["id"] = "mutated"
	if got := first.Context["message"].(map[string]any)["id"]; got != "message-1" {
		t.Fatalf("returned context shares caller ownership: id = %v", got)
	}
}

func TestValidateEventRejectsMismatchedIdempotencyKey(t *testing.T) {
	event := validEvent()
	event.IdempotencyKey = "invocation:v1:not-canonical"
	_, err := ValidateEvent(event)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("ValidateEvent() error = %v, want ErrInvalidEvent", err)
	}
}

func TestCanonicalIdempotencyKeyRejectsUnsupportedSchema(t *testing.T) {
	event := validEvent()
	event.SchemaVersion = "invocation_event.v2"
	if _, err := CanonicalIdempotencyKey(event); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("CanonicalIdempotencyKey() error = %v, want ErrInvalidEvent", err)
	}
}

func TestValidateEventRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(Event) Event
	}{
		{name: "unsupported schema", mutate: func(event Event) Event { event.SchemaVersion = "invocation_event.v2"; return event }},
		{name: "missing id", mutate: func(event Event) Event { event.ID = ""; return event }},
		{name: "mismatched origin", mutate: func(event Event) Event { event.Origin.Kind = OriginSchedule; return event }},
		{name: "missing provider", mutate: func(event Event) Event { event.Origin.Provider = ""; return event }},
		{name: "missing sender", mutate: func(event Event) Event { event.Sender.ID = ""; return event }},
		{name: "missing occurred at", mutate: func(event Event) Event { event.OccurredAt = time.Time{}; return event }},
		{name: "empty reply", mutate: func(event Event) Event { event.ReplyTo = &ReplyAddress{}; return event }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateEvent(tt.mutate(validEvent()))
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("ValidateEvent() error = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	want, err := ValidateEvent(validEvent())
	if err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	got, err := ValidateEvent(decoded)
	if err != nil {
		t.Fatalf("ValidateEvent(decoded) error = %v", err)
	}
	if got.IdempotencyKey != want.IdempotencyKey || !got.OccurredAt.Equal(want.OccurredAt) {
		t.Fatalf("JSON round trip = %#v, want identity %#v", got, want)
	}
}

func validEvent() Event {
	return Event{
		ID:         "event-123",
		Type:       EventTypeChannelMention,
		Origin:     Origin{Kind: OriginChannel, Provider: "feishu", InstallationID: "installation-1"},
		Target:     validDigitalHumanSubject(),
		Sender:     PrincipalRef{Type: PrincipalExternalUser, ID: "user-1"},
		OccurredAt: time.Date(2026, time.August, 18, 4, 30, 0, 0, time.UTC),
		Context:    map[string]any{"message_id": "message-1"},
	}
}

func validDigitalHumanSubject() runtimeidentity.SubjectRef {
	return runtimeidentity.SubjectRef{
		Type:           runtimeidentity.SubjectTypeDigitalHuman,
		ID:             uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		OrganizationID: uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		WorkspaceID:    uuid.MustParse("33333333-3333-4333-8333-333333333333"),
	}
}
