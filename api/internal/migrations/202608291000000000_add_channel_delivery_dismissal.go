package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration202608291000000000ID  = "202608291000000000_add_channel_delivery_dismissal"
	addChannelDeliveryDismissalSQL = `
		ALTER TABLE public.channel_delivery_outbox
			ADD COLUMN dismissed_at timestamptz,
			ADD COLUMN dismissed_by uuid REFERENCES public.accounts(id) ON DELETE RESTRICT,
			DROP CONSTRAINT IF EXISTS channel_delivery_outbox_status_check,
			ADD CONSTRAINT channel_delivery_outbox_status_check
				CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'unknown', 'dismissed')),
			ADD CONSTRAINT channel_delivery_outbox_dismissal_check CHECK (
				(status = 'dismissed' AND dismissed_at IS NOT NULL AND dismissed_by IS NOT NULL)
				OR (status <> 'dismissed' AND dismissed_at IS NULL AND dismissed_by IS NULL)
			);
	`
)

func init() {
	registerSchemaMigration(migration202608291000000000ID, up202608291000000000, down202608291000000000)
}

func up202608291000000000(schema *mschema.Builder) error {
	return schema.Raw(addChannelDeliveryDismissalSQL)
}

func down202608291000000000(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.channel_delivery_outbox
		SET status = 'failed', dismissed_at = NULL, dismissed_by = NULL
		WHERE status = 'dismissed';

		ALTER TABLE public.channel_delivery_outbox
			DROP CONSTRAINT IF EXISTS channel_delivery_outbox_dismissal_check,
			DROP CONSTRAINT IF EXISTS channel_delivery_outbox_status_check,
			ADD CONSTRAINT channel_delivery_outbox_status_check
				CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'unknown')),
			DROP COLUMN IF EXISTS dismissed_by,
			DROP COLUMN IF EXISTS dismissed_at;
	`)
}
