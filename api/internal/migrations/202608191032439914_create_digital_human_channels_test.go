package migrations

import (
	"strings"
	"testing"
)

func TestDigitalHumanChannelsMigrationSeparatesRunAndDeliveryReliability(t *testing.T) {
	compact := strings.ToLower(strings.Join(strings.Fields(createDigitalHumanChannelsSQL), " "))
	required := []string{
		"create table if not exists public.channel_installations",
		"create table if not exists public.external_conversation_mappings",
		"create table if not exists public.channel_bindings",
		"create table if not exists public.channel_inbound_events",
		"create table if not exists public.channel_delivery_outbox",
		"unique index if not exists idx_channel_inbound_events_provider_event",
		"unique index if not exists idx_channel_inbound_events_provider_message",
		"unique index if not exists idx_channel_delivery_outbox_run",
		"references public.integration_connections(id) on delete restrict",
		"payload_digest varchar(71) not null",
		"enforce_channel_scope_consistency",
		"channel delivery provider connection mismatch",
		"channel delivery inbound event mismatch",
		"channel delivery run subject mismatch",
	}
	for _, token := range required {
		if !strings.Contains(compact, token) {
			t.Errorf("migration is missing %q", token)
		}
	}
	for _, forbidden := range []string{"agent_id", "access_token", "refresh_token", "client_secret", "encrypt_key", "verification_token", "cookie"} {
		if strings.Contains(compact, forbidden) {
			t.Errorf("migration contains forbidden credential or Agent token %q", forbidden)
		}
	}
}
