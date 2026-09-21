package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608210930000000ID = "202608210930000000_enable_digital_human_channel_progress"

const enableDigitalHumanChannelProgressSQL = `
	ALTER TABLE public.channel_delivery_outbox
		ADD COLUMN IF NOT EXISTS delivery_kind varchar(24) NOT NULL DEFAULT 'final',
		ADD COLUMN IF NOT EXISTS delivery_key varchar(128) NOT NULL DEFAULT 'final',
		ADD COLUMN IF NOT EXISTS sequence bigint NOT NULL DEFAULT 9223372036854775807;

	ALTER TABLE public.channel_delivery_outbox
		DROP CONSTRAINT IF EXISTS channel_delivery_outbox_kind_check;
	ALTER TABLE public.channel_delivery_outbox
		ADD CONSTRAINT channel_delivery_outbox_kind_check CHECK (
			delivery_kind IN ('acknowledgement', 'progress', 'final')
		);

	DROP INDEX IF EXISTS public.idx_channel_delivery_outbox_run;
	CREATE INDEX IF NOT EXISTS idx_channel_delivery_outbox_run
		ON public.channel_delivery_outbox (run_id, sequence, created_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_delivery_outbox_stage
		ON public.channel_delivery_outbox (run_id, delivery_kind, delivery_key);
`

func init() {
	registerSchemaMigration(migration202608210930000000ID, up202608210930000000, down202608210930000000)
}

func up202608210930000000(schema *mschema.Builder) error {
	return schema.Raw(enableDigitalHumanChannelProgressSQL)
}

func down202608210930000000(schema *mschema.Builder) error {
	return schema.Raw(`
		DELETE FROM public.channel_delivery_outbox WHERE delivery_kind <> 'final';
		DROP INDEX IF EXISTS public.idx_channel_delivery_outbox_stage;
		DROP INDEX IF EXISTS public.idx_channel_delivery_outbox_run;
		ALTER TABLE public.channel_delivery_outbox
			DROP CONSTRAINT IF EXISTS channel_delivery_outbox_kind_check,
			DROP COLUMN IF EXISTS sequence,
			DROP COLUMN IF EXISTS delivery_key,
			DROP COLUMN IF EXISTS delivery_kind;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_delivery_outbox_run
			ON public.channel_delivery_outbox (run_id);
	`)
}
