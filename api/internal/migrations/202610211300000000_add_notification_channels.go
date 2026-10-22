package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Notification channels deliver inbox events outside the console (ADR 0008).
// A channel belongs to an account (personal email or webhook), a workspace
// (team webhook), or a DigitalHuman (the webhook of its API integrations).
// Each delivery of a notification to a channel is tracked for retries.
const notificationChannelsSQL = `
CREATE TABLE IF NOT EXISTS public.notification_channels (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	workspace_id uuid,
	owner_kind varchar(32) NOT NULL,
	owner_id uuid NOT NULL,
	type varchar(16) NOT NULL,
	name varchar(128) NOT NULL DEFAULT '',
	target text NOT NULL,
	secret_envelope text,
	events jsonb NOT NULL DEFAULT '[]'::jsonb,
	status varchar(16) NOT NULL,
	verify_code_hash varchar(128),
	verify_expires_at timestamptz,
	verify_attempts integer NOT NULL DEFAULT 0,
	verify_sent_at timestamptz,
	verified_at timestamptz,
	created_by uuid,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	CONSTRAINT notification_channels_owner_check CHECK (owner_kind IN ('account', 'workspace', 'digital_human')),
	CONSTRAINT notification_channels_type_check CHECK (type IN ('email', 'webhook')),
	CONSTRAINT notification_channels_status_check CHECK (status IN ('pending', 'active', 'disabled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_channels_target
	ON public.notification_channels (owner_kind, owner_id, type, target);
CREATE INDEX IF NOT EXISTS idx_notification_channels_owner
	ON public.notification_channels (organization_id, owner_kind, owner_id);

CREATE TABLE IF NOT EXISTS public.notification_deliveries (
	id uuid PRIMARY KEY,
	notification_id uuid NOT NULL REFERENCES public.inbox_notifications(id) ON DELETE CASCADE,
	channel_id uuid NOT NULL REFERENCES public.notification_channels(id) ON DELETE CASCADE,
	status varchar(16) NOT NULL,
	attempts integer NOT NULL DEFAULT 0,
	last_error text,
	sent_at timestamptz,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	CONSTRAINT notification_deliveries_status_check CHECK (status IN ('pending', 'sent', 'failed'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_deliveries_unique
	ON public.notification_deliveries (notification_id, channel_id);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status
	ON public.notification_deliveries (status, updated_at);`

func init() {
	registerSchemaMigration("202610211300000000_add_notification_channels", upAddNotificationChannels, downAddNotificationChannels)
}

func upAddNotificationChannels(schema *mschema.Builder) error {
	return schema.Raw(notificationChannelsSQL)
}

func downAddNotificationChannels(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.notification_deliveries;
		DROP TABLE IF EXISTS public.notification_channels;
	`)
}
