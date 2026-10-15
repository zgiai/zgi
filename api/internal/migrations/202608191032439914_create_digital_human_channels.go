package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202608191032439914ID = "202608191032439914_create_digital_human_channels"

const createDigitalHumanChannelsSQL = `
	CREATE TABLE IF NOT EXISTS public.channel_installations (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		provider varchar(32) NOT NULL,
		connection_id uuid NOT NULL,
		name varchar(128) NOT NULL,
		external_tenant_id varchar(255) NOT NULL DEFAULT '',
		external_bot_id varchar(255) NOT NULL DEFAULT '',
		status varchar(16) NOT NULL DEFAULT 'active',
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		updated_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT channel_installations_provider_check CHECK (provider <> ''),
		CONSTRAINT channel_installations_name_check CHECK (name <> ''),
		CONSTRAINT channel_installations_status_check CHECK (status IN ('active', 'disabled')),
		CONSTRAINT channel_installations_revision_check CHECK (revision > 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_installations_connection
		ON public.channel_installations (organization_id, workspace_id, provider, connection_id)
		WHERE deleted_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_channel_installations_scope
		ON public.channel_installations (organization_id, workspace_id, provider, status)
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.external_conversation_mappings (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE RESTRICT,
		external_conversation_id varchar(255) NOT NULL,
		conversation_type varchar(16) NOT NULL,
		local_conversation_id uuid,
		last_event_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT external_conversation_mappings_external_id_check CHECK (external_conversation_id <> ''),
		CONSTRAINT external_conversation_mappings_type_check CHECK (conversation_type IN ('direct', 'group'))
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_external_conversation_mappings_identity
		ON public.external_conversation_mappings (installation_id, external_conversation_id);

	CREATE TABLE IF NOT EXISTS public.channel_bindings (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE RESTRICT,
		conversation_mapping_id uuid NOT NULL REFERENCES public.external_conversation_mappings(id) ON DELETE RESTRICT,
		subject_type varchar(32) NOT NULL,
		subject_id uuid NOT NULL,
		wake_policy varchar(32) NOT NULL DEFAULT 'direct_and_mention',
		enabled boolean NOT NULL DEFAULT true,
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		updated_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT channel_bindings_subject_type_check CHECK (subject_type IN ('digital_human')),
		CONSTRAINT channel_bindings_wake_policy_check CHECK (wake_policy IN ('direct_and_mention', 'mention_only')),
		CONSTRAINT channel_bindings_revision_check CHECK (revision > 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_bindings_conversation
		ON public.channel_bindings (conversation_mapping_id)
		WHERE deleted_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_channel_bindings_subject
		ON public.channel_bindings (organization_id, workspace_id, subject_type, subject_id, enabled)
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.channel_inbound_events (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE RESTRICT,
		binding_id uuid REFERENCES public.channel_bindings(id) ON DELETE RESTRICT,
		provider_event_id varchar(255) NOT NULL,
		provider_message_id varchar(255) NOT NULL DEFAULT '',
		external_conversation_id varchar(255) NOT NULL DEFAULT '',
		sender_external_id varchar(255) NOT NULL DEFAULT '',
		event_type varchar(64) NOT NULL,
		payload_digest varchar(71) NOT NULL,
		status varchar(16) NOT NULL DEFAULT 'received',
		reason_code varchar(128) NOT NULL DEFAULT '',
		invocation_run_id uuid REFERENCES public.digital_human_runs(id) ON DELETE RESTRICT,
		received_at timestamptz NOT NULL,
		processed_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT channel_inbound_events_event_id_check CHECK (provider_event_id <> ''),
		CONSTRAINT channel_inbound_events_event_type_check CHECK (event_type <> ''),
		CONSTRAINT channel_inbound_events_digest_check CHECK (payload_digest <> ''),
		CONSTRAINT channel_inbound_events_status_check CHECK (status IN ('received', 'ignored', 'accepted', 'failed'))
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_inbound_events_provider_event
		ON public.channel_inbound_events (installation_id, provider_event_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_inbound_events_provider_message
		ON public.channel_inbound_events (installation_id, provider_message_id)
		WHERE provider_message_id <> '';
	CREATE INDEX IF NOT EXISTS idx_channel_inbound_events_scope_created
		ON public.channel_inbound_events (organization_id, workspace_id, installation_id, created_at DESC, id DESC);

	CREATE TABLE IF NOT EXISTS public.channel_delivery_outbox (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE RESTRICT,
		binding_id uuid NOT NULL REFERENCES public.channel_bindings(id) ON DELETE RESTRICT,
		inbound_event_id uuid NOT NULL REFERENCES public.channel_inbound_events(id) ON DELETE RESTRICT,
		run_id uuid NOT NULL REFERENCES public.digital_human_runs(id) ON DELETE RESTRICT,
		provider varchar(32) NOT NULL,
		connection_id uuid NOT NULL,
		actor_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		subject_type varchar(32) NOT NULL,
		subject_id uuid NOT NULL,
		external_conversation_id varchar(255) NOT NULL,
		external_thread_id varchar(255) NOT NULL DEFAULT '',
		payload jsonb NOT NULL,
		payload_digest varchar(71) NOT NULL,
		status varchar(16) NOT NULL DEFAULT 'pending',
		attempt_count integer NOT NULL DEFAULT 0,
		available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		claimed_at timestamptz,
		delivered_at timestamptz,
		provider_message_id varchar(255) NOT NULL DEFAULT '',
		provider_request_id varchar(128) NOT NULL DEFAULT '',
		error_code varchar(128) NOT NULL DEFAULT '',
		error_message text NOT NULL DEFAULT '',
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT channel_delivery_outbox_provider_check CHECK (provider <> ''),
		CONSTRAINT channel_delivery_outbox_subject_type_check CHECK (subject_type IN ('digital_human')),
		CONSTRAINT channel_delivery_outbox_conversation_check CHECK (external_conversation_id <> ''),
		CONSTRAINT channel_delivery_outbox_digest_check CHECK (payload_digest <> ''),
		CONSTRAINT channel_delivery_outbox_status_check CHECK (status IN ('pending', 'processing', 'succeeded', 'failed', 'unknown')),
		CONSTRAINT channel_delivery_outbox_attempt_check CHECK (attempt_count >= 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_delivery_outbox_run
		ON public.channel_delivery_outbox (run_id);
	CREATE INDEX IF NOT EXISTS idx_channel_delivery_outbox_claim
		ON public.channel_delivery_outbox (status, available_at, created_at)
		WHERE status IN ('pending', 'processing');

	CREATE OR REPLACE FUNCTION public.enforce_channel_scope_consistency()
	RETURNS trigger AS $$
	DECLARE
		parent_org uuid;
		parent_workspace uuid;
		parent_provider varchar(32);
		parent_connection uuid;
	BEGIN
		IF TG_TABLE_NAME = 'channel_installations' THEN
			SELECT organization_id INTO parent_org FROM public.integration_connections WHERE id = NEW.connection_id AND deleted_at IS NULL;
			IF parent_org IS NULL OR parent_org <> NEW.organization_id THEN
				RAISE EXCEPTION 'channel installation connection scope mismatch' USING ERRCODE = '23514';
			END IF;
			RETURN NEW;
		END IF;

		SELECT organization_id, workspace_id, provider, connection_id
		INTO parent_org, parent_workspace, parent_provider, parent_connection
		FROM public.channel_installations WHERE id = NEW.installation_id AND deleted_at IS NULL;
		IF parent_org IS NULL OR parent_org <> NEW.organization_id OR parent_workspace <> NEW.workspace_id THEN
			RAISE EXCEPTION 'channel installation scope mismatch' USING ERRCODE = '23514';
		END IF;

		IF TG_TABLE_NAME = 'channel_bindings' THEN
			IF NOT EXISTS (
				SELECT 1 FROM public.external_conversation_mappings mapping
				WHERE mapping.id = NEW.conversation_mapping_id
				  AND mapping.installation_id = NEW.installation_id
				  AND mapping.organization_id = NEW.organization_id
				  AND mapping.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel binding conversation scope mismatch' USING ERRCODE = '23514';
			END IF;
			IF NEW.subject_type = 'digital_human' AND NOT EXISTS (
				SELECT 1 FROM public.digital_humans subject
				WHERE subject.id = NEW.subject_id
				  AND subject.organization_id = NEW.organization_id
				  AND subject.workspace_id = NEW.workspace_id
				  AND subject.deleted_at IS NULL
			) THEN
				RAISE EXCEPTION 'channel binding subject scope mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;

		IF TG_TABLE_NAME = 'channel_inbound_events' THEN
			IF NEW.binding_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM public.channel_bindings binding
				WHERE binding.id = NEW.binding_id
				  AND binding.installation_id = NEW.installation_id
				  AND binding.organization_id = NEW.organization_id
				  AND binding.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel inbound binding scope mismatch' USING ERRCODE = '23514';
			END IF;
			IF NEW.invocation_run_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM public.digital_human_runs run
				WHERE run.id = NEW.invocation_run_id
				  AND run.organization_id = NEW.organization_id
				  AND run.workspace_id = NEW.workspace_id
			) THEN
				RAISE EXCEPTION 'channel inbound run scope mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;

		IF TG_TABLE_NAME = 'channel_delivery_outbox' THEN
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
			) THEN
				RAISE EXCEPTION 'channel delivery binding scope mismatch' USING ERRCODE = '23514';
			END IF;
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
			IF NOT EXISTS (
				SELECT 1 FROM public.digital_human_runs run
				WHERE run.id = NEW.run_id
				  AND run.organization_id = NEW.organization_id
				  AND run.workspace_id = NEW.workspace_id
				  AND run.digital_human_id = NEW.subject_id
			) THEN
				RAISE EXCEPTION 'channel delivery run subject mismatch' USING ERRCODE = '23514';
			END IF;
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;

	DROP TRIGGER IF EXISTS trg_channel_installations_scope ON public.channel_installations;
	CREATE TRIGGER trg_channel_installations_scope
		BEFORE INSERT OR UPDATE ON public.channel_installations
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	DROP TRIGGER IF EXISTS trg_external_conversation_mappings_scope ON public.external_conversation_mappings;
	CREATE TRIGGER trg_external_conversation_mappings_scope
		BEFORE INSERT OR UPDATE ON public.external_conversation_mappings
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	DROP TRIGGER IF EXISTS trg_channel_bindings_scope ON public.channel_bindings;
	CREATE TRIGGER trg_channel_bindings_scope
		BEFORE INSERT OR UPDATE ON public.channel_bindings
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	DROP TRIGGER IF EXISTS trg_channel_inbound_events_scope ON public.channel_inbound_events;
	CREATE TRIGGER trg_channel_inbound_events_scope
		BEFORE INSERT OR UPDATE ON public.channel_inbound_events
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();
	DROP TRIGGER IF EXISTS trg_channel_delivery_outbox_scope ON public.channel_delivery_outbox;
	CREATE TRIGGER trg_channel_delivery_outbox_scope
		BEFORE INSERT OR UPDATE ON public.channel_delivery_outbox
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_scope_consistency();

	-- Channels were first keyed by the legacy integration connection. A
	-- database whose Connector retirement already removed integration_connections
	-- skips the link; 202608231300000000 drops it either way.
	DO $$
	BEGIN
		IF to_regclass('public.integration_connections') IS NOT NULL THEN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'channel_installations_connection_id_fkey') THEN
				ALTER TABLE public.channel_installations
					ADD CONSTRAINT channel_installations_connection_id_fkey
					FOREIGN KEY (connection_id) REFERENCES public.integration_connections(id) ON DELETE RESTRICT;
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'channel_delivery_outbox_connection_id_fkey') THEN
				ALTER TABLE public.channel_delivery_outbox
					ADD CONSTRAINT channel_delivery_outbox_connection_id_fkey
					FOREIGN KEY (connection_id) REFERENCES public.integration_connections(id) ON DELETE RESTRICT;
			END IF;
		END IF;
	END $$;
`

func init() {
	registerSchemaMigration(migration202608191032439914ID, up202608191032439914, down202608191032439914)
}

func up202608191032439914(schema *mschema.Builder) error {
	return schema.Raw(createDigitalHumanChannelsSQL)
}

func down202608191032439914(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_channel_delivery_outbox_scope ON public.channel_delivery_outbox;
		DROP TRIGGER IF EXISTS trg_channel_inbound_events_scope ON public.channel_inbound_events;
		DROP TRIGGER IF EXISTS trg_channel_bindings_scope ON public.channel_bindings;
		DROP TRIGGER IF EXISTS trg_external_conversation_mappings_scope ON public.external_conversation_mappings;
		DROP TRIGGER IF EXISTS trg_channel_installations_scope ON public.channel_installations;
		DROP FUNCTION IF EXISTS public.enforce_channel_scope_consistency();
	`); err != nil {
		return err
	}
	for _, table := range []string{
		"channel_delivery_outbox",
		"channel_inbound_events",
		"channel_bindings",
		"external_conversation_mappings",
		"channel_installations",
	} {
		if err := schema.DropIfExists(table); err != nil {
			return err
		}
	}
	return nil
}
