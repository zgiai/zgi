package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608251100000000ID = "202608251100000000_allow_scheduled_channel_deliveries"

const allowScheduledChannelDeliveriesSQL = `
	CREATE OR REPLACE FUNCTION public.enforce_channel_delivery_scope_consistency()
	RETURNS trigger AS $$
	DECLARE
		parent_org uuid;
		parent_workspace uuid;
		parent_provider varchar(32);
		parent_connection uuid;
		run_origin varchar(32);
		run_reply_provider text;
		run_reply_installation text;
		run_reply_conversation text;
		run_reply_thread text;
	BEGIN
		SELECT organization_id, workspace_id, provider, connection_id
		INTO parent_org, parent_workspace, parent_provider, parent_connection
		FROM public.channel_installations
		WHERE id = NEW.installation_id AND deleted_at IS NULL;
		IF parent_org IS NULL OR parent_org <> NEW.organization_id OR parent_workspace <> NEW.workspace_id THEN
			RAISE EXCEPTION 'channel installation scope mismatch' USING ERRCODE = '23514';
		END IF;
		IF NEW.provider <> parent_provider OR NEW.connection_id <> parent_connection THEN
			RAISE EXCEPTION 'channel delivery provider connection mismatch' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.channel_bindings binding
			WHERE binding.id = NEW.binding_id
			  AND binding.installation_id = NEW.installation_id
			  AND binding.organization_id = NEW.organization_id
			  AND binding.workspace_id = NEW.workspace_id
			  AND binding.subject_type = NEW.subject_type
			  AND binding.subject_id = NEW.subject_id
			  AND binding.enabled = true
			  AND binding.deleted_at IS NULL
		) THEN
			RAISE EXCEPTION 'channel delivery binding scope mismatch' USING ERRCODE = '23514';
		END IF;
		SELECT
			run.invocation_event->'origin'->>'kind',
			run.invocation_event->'reply_to'->>'provider',
			run.invocation_event->'reply_to'->>'installation_id',
			run.invocation_event->'reply_to'->>'conversation_id',
			COALESCE(run.invocation_event->'reply_to'->>'thread_id', '')
		INTO run_origin, run_reply_provider, run_reply_installation, run_reply_conversation, run_reply_thread
		FROM public.digital_human_runs run
		WHERE run.id = NEW.run_id
		  AND run.organization_id = NEW.organization_id
		  AND run.workspace_id = NEW.workspace_id
		  AND run.digital_human_id = NEW.subject_id;
		IF NOT FOUND THEN
			RAISE EXCEPTION 'channel delivery run subject mismatch' USING ERRCODE = '23514';
		END IF;
		IF NEW.inbound_event_id IS NOT NULL THEN
			IF NOT EXISTS (
				SELECT 1 FROM public.channel_inbound_events inbound
				WHERE inbound.id = NEW.inbound_event_id
				  AND inbound.installation_id = NEW.installation_id
				  AND inbound.binding_id = NEW.binding_id
				  AND inbound.invocation_run_id = NEW.run_id
				  AND inbound.organization_id = NEW.organization_id
				  AND inbound.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel delivery inbound event mismatch' USING ERRCODE = '23514';
			END IF;
		ELSIF run_origin <> 'schedule'
			OR LOWER(COALESCE(run_reply_provider, '')) <> LOWER(NEW.provider)
			OR COALESCE(run_reply_installation, '') <> NEW.installation_id::text
			OR COALESCE(run_reply_conversation, '') <> NEW.external_conversation_id
			OR COALESCE(run_reply_thread, '') <> COALESCE(NEW.external_thread_id, '') THEN
			RAISE EXCEPTION 'scheduled channel delivery reply address mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;

	DROP TRIGGER IF EXISTS trg_channel_delivery_outbox_scope ON public.channel_delivery_outbox;
	CREATE TRIGGER trg_channel_delivery_outbox_scope
		BEFORE INSERT OR UPDATE ON public.channel_delivery_outbox
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_delivery_scope_consistency();
`

func init() {
	registerSchemaMigration(migration202608251100000000ID, up202608251100000000, down202608251100000000)
}

func up202608251100000000(schema *mschema.Builder) error {
	return schema.Raw(allowScheduledChannelDeliveriesSQL)
}

func down202608251100000000(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TRIGGER IF EXISTS trg_channel_delivery_outbox_scope ON public.channel_delivery_outbox;
		DROP FUNCTION IF EXISTS public.enforce_channel_delivery_scope_consistency();
		CREATE TRIGGER trg_channel_delivery_outbox_scope
			BEFORE INSERT OR UPDATE ON public.channel_delivery_outbox
			FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	`)
}
