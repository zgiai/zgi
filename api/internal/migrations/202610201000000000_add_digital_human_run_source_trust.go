package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Each DigitalHuman run records the source trust level classified once at
// admission. The column is nullable so historical runs keep their meaning and
// the runtime derives their level from the persisted surface.
const digitalHumanRunSourceTrustSQL = `
ALTER TABLE public.digital_human_runs ADD COLUMN IF NOT EXISTS source_trust varchar(32);
ALTER TABLE public.digital_human_runs DROP CONSTRAINT IF EXISTS digital_human_runs_source_trust_check;
ALTER TABLE public.digital_human_runs ADD CONSTRAINT digital_human_runs_source_trust_check CHECK (
 source_trust IS NULL OR source_trust IN ('authorized_operator', 'workspace_member', 'channel_participant', 'external_end_user')
);`

func init() {
	registerSchemaMigration("202610201000000000_add_digital_human_run_source_trust", upAddDigitalHumanRunSourceTrust, downAddDigitalHumanRunSourceTrust)
}

func upAddDigitalHumanRunSourceTrust(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanRunSourceTrustSQL)
}

func downAddDigitalHumanRunSourceTrust(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_runs
			DROP CONSTRAINT IF EXISTS digital_human_runs_source_trust_check,
			DROP COLUMN IF EXISTS source_trust;
	`)
}
