package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// The in-app inbox stores one notification per recipient, event, and source
// (ADR 0008). Recipients are user subjects, so external API end users can
// have an inbox as well as accounts.
const inboxNotificationsSQL = `
CREATE TABLE IF NOT EXISTS public.inbox_notifications (
	id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	workspace_id uuid,
	recipient_kind varchar(32) NOT NULL,
	recipient_id uuid NOT NULL,
	event varchar(64) NOT NULL,
	source_type varchar(64) NOT NULL,
	source_id uuid NOT NULL,
	title text NOT NULL DEFAULT '',
	summary text NOT NULL DEFAULT '',
	link text NOT NULL DEFAULT '',
	context jsonb NOT NULL DEFAULT '{}'::jsonb,
	read_at timestamptz,
	created_at timestamptz NOT NULL,
	CONSTRAINT inbox_notifications_recipient_check CHECK (recipient_kind IN ('account', 'api_end_user'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_notifications_source
	ON public.inbox_notifications (recipient_kind, recipient_id, event, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_inbox_notifications_recipient
	ON public.inbox_notifications (organization_id, recipient_kind, recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inbox_notifications_unread
	ON public.inbox_notifications (organization_id, recipient_kind, recipient_id) WHERE read_at IS NULL;`

func init() {
	registerSchemaMigration("202610211200000000_add_inbox_notifications", upAddInboxNotifications, downAddInboxNotifications)
}

func upAddInboxNotifications(schema *mschema.Builder) error {
	return schema.Raw(inboxNotificationsSQL)
}

func downAddInboxNotifications(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.inbox_notifications;`)
}
