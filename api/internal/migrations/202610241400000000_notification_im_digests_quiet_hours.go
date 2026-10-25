package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Stage 4 of ADR 0008: personal IM channels send through the member's own
// Connection, recurring task reports may be held for a daily or weekly
// digest, and each account may set quiet hours that delay its own email and
// IM deliveries.
func init() {
	registerSchemaMigration("202610241400000000_notification_im_digests_quiet_hours", upNotificationIMDigestsQuietHours, downNotificationIMDigestsQuietHours)
}

func upNotificationIMDigestsQuietHours(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.notification_channels ADD COLUMN IF NOT EXISTS connection_id uuid;
		ALTER TABLE public.notification_channels DROP CONSTRAINT IF EXISTS notification_channels_type_check;
		ALTER TABLE public.notification_channels ADD CONSTRAINT notification_channels_type_check
			CHECK (type IN ('email', 'webhook', 'im'));

		ALTER TABLE public.notification_deliveries
			ADD COLUMN IF NOT EXISTS deliver_after timestamptz,
			ADD COLUMN IF NOT EXISTS digest varchar(16),
			ADD COLUMN IF NOT EXISTS digest_due_at timestamptz;
		ALTER TABLE public.notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_status_check;
		ALTER TABLE public.notification_deliveries ADD CONSTRAINT notification_deliveries_status_check
			CHECK (status IN ('pending', 'held', 'sent', 'failed'));
		CREATE INDEX IF NOT EXISTS idx_notification_deliveries_digest
			ON public.notification_deliveries (channel_id, digest_due_at)
			WHERE status = 'held';

		CREATE TABLE IF NOT EXISTS public.notification_preferences (
			organization_id uuid NOT NULL,
			account_id uuid NOT NULL,
			quiet_start smallint,
			quiet_end smallint,
			updated_at timestamptz NOT NULL,
			PRIMARY KEY (organization_id, account_id),
			CONSTRAINT notification_preferences_quiet_check CHECK (
				(quiet_start IS NULL AND quiet_end IS NULL)
				OR (quiet_start BETWEEN 0 AND 1439 AND quiet_end BETWEEN 0 AND 1439 AND quiet_start <> quiet_end))
		);
	`)
}

func downNotificationIMDigestsQuietHours(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TABLE IF EXISTS public.notification_preferences;
		DROP INDEX IF EXISTS public.idx_notification_deliveries_digest;
		UPDATE public.notification_deliveries SET status = 'pending' WHERE status = 'held';
		ALTER TABLE public.notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_status_check;
		ALTER TABLE public.notification_deliveries ADD CONSTRAINT notification_deliveries_status_check
			CHECK (status IN ('pending', 'sent', 'failed'));
		ALTER TABLE public.notification_deliveries
			DROP COLUMN IF EXISTS digest_due_at,
			DROP COLUMN IF EXISTS digest,
			DROP COLUMN IF EXISTS deliver_after;
		DELETE FROM public.notification_channels WHERE type = 'im';
		ALTER TABLE public.notification_channels DROP CONSTRAINT IF EXISTS notification_channels_type_check;
		ALTER TABLE public.notification_channels ADD CONSTRAINT notification_channels_type_check
			CHECK (type IN ('email', 'webhook'));
		ALTER TABLE public.notification_channels DROP COLUMN IF EXISTS connection_id;
	`)
}
