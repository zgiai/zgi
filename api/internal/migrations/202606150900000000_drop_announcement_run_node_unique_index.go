package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202606150900000000ID = "202606150900000000_drop_announcement_run_node_unique_index"

const dropAnnouncementRunNodeUniqueIndexSQL = `
DROP INDEX IF EXISTS public.idx_announcements_run_node
`

const ensureNoDuplicateAnnouncementRunNodeSQL = `
DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM (
			SELECT workflow_run_id, node_id
			FROM public.announcements
			GROUP BY workflow_run_id, node_id
			HAVING COUNT(*) > 1
		) duplicates
	) THEN
		RAISE EXCEPTION 'cannot recreate idx_announcements_run_node because duplicate workflow_run_id/node_id announcements exist';
	END IF;
END $$;
`

const recreateAnnouncementRunNodeUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_announcements_run_node
ON public.announcements (workflow_run_id, node_id)
`

func init() {
	registerSchemaMigration(
		migration202606150900000000ID,
		upDropAnnouncementRunNodeUniqueIndex,
		downDropAnnouncementRunNodeUniqueIndex,
	)
}

func upDropAnnouncementRunNodeUniqueIndex(schema *mschema.Builder) error {
	return schema.Raw(dropAnnouncementRunNodeUniqueIndexSQL)
}

func downDropAnnouncementRunNodeUniqueIndex(schema *mschema.Builder) error {
	if err := schema.Raw(ensureNoDuplicateAnnouncementRunNodeSQL); err != nil {
		return err
	}
	return schema.Raw(recreateAnnouncementRunNodeUniqueIndexSQL)
}
