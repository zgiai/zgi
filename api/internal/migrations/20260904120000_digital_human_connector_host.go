package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const digitalHumanConnectorHostSQL = `
ALTER TABLE public.connector_target_bindings DROP CONSTRAINT connector_target_bindings_target_type_check;
ALTER TABLE public.connector_target_bindings ADD CONSTRAINT connector_target_bindings_target_type_check CHECK (target_type IN ('chat','agent','workflow','digital_human'));
ALTER TABLE public.connector_target_bindings DROP CONSTRAINT connector_target_bindings_instance_check;
ALTER TABLE public.connector_target_bindings ADD CONSTRAINT connector_target_bindings_instance_check CHECK (
 (target_type IN ('chat','agent') AND binding_instance_id IS NULL) OR
 (target_type IN ('workflow','digital_human') AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''));
ALTER TABLE public.connector_target_bindings DROP CONSTRAINT connector_target_scope_check;
ALTER TABLE public.connector_target_bindings ADD CONSTRAINT connector_target_scope_check CHECK (
 (target_type = 'chat' AND target_scope = 'live' AND target_version_id = '') OR
 (target_type IN ('agent','workflow','digital_human') AND ((target_scope = 'draft' AND target_version_id = '') OR (target_scope = 'published' AND target_version_id <> ''))));

ALTER TABLE public.connector_lifecycle_decisions DROP CONSTRAINT connector_lifecycle_decisions_target_type_check;
ALTER TABLE public.connector_lifecycle_decisions ADD CONSTRAINT connector_lifecycle_decisions_target_type_check CHECK (target_type IS NULL OR target_type IN ('chat','agent','workflow','digital_human'));
ALTER TABLE public.connector_lifecycle_decisions DROP CONSTRAINT connector_lifecycle_decisions_target_context_check;
ALTER TABLE public.connector_lifecycle_decisions ADD CONSTRAINT connector_lifecycle_decisions_target_context_check CHECK (
 (target_type IS NULL AND target_id IS NULL AND binding_instance_id IS NULL) OR
 (target_type IN ('chat','agent') AND target_id IS NOT NULL AND btrim(target_id) <> '' AND binding_instance_id IS NULL) OR
 (target_type IN ('workflow','digital_human') AND target_id IS NOT NULL AND btrim(target_id) <> '' AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''));
ALTER TABLE public.connector_operation_outcomes DROP CONSTRAINT connector_operation_outcomes_target_type_check;
ALTER TABLE public.connector_operation_outcomes ADD CONSTRAINT connector_operation_outcomes_target_type_check CHECK (target_type IN ('chat','agent','workflow','digital_human'));
ALTER TABLE public.connector_operation_outcomes DROP CONSTRAINT connector_operation_outcomes_target_check;
ALTER TABLE public.connector_operation_outcomes ADD CONSTRAINT connector_operation_outcomes_target_check CHECK (
 (target_type IN ('chat','agent') AND btrim(target_id) <> '' AND binding_instance_id IS NULL) OR
 (target_type IN ('workflow','digital_human') AND btrim(target_id) <> '' AND binding_instance_id IS NOT NULL AND btrim(binding_instance_id) <> ''));

ALTER TABLE public.connector_confirmation_continuations ADD COLUMN origin_version integer NOT NULL DEFAULT 0;
ALTER TABLE public.connector_confirmation_continuations ADD COLUMN digital_human_id varchar(255);
ALTER TABLE public.connector_confirmation_continuations DROP CONSTRAINT connector_confirmation_continuations_surface_check;
ALTER TABLE public.connector_confirmation_continuations ADD CONSTRAINT connector_confirmation_continuations_surface_check CHECK (origin_surface IN ('chat','agent','workflow','digital_human'));
ALTER TABLE public.connector_confirmation_continuations DROP CONSTRAINT connector_confirmation_continuations_origin_refs_check;
ALTER TABLE public.connector_confirmation_continuations ADD CONSTRAINT connector_confirmation_continuations_origin_refs_check CHECK (
 (origin_version = 0 AND digital_human_id IS NULL AND (
   (origin_surface = 'chat' AND conversation_id IS NOT NULL AND message_id IS NOT NULL AND agent_id IS NULL AND workflow_id IS NULL AND run_id IS NULL AND node_id IS NULL) OR
   (origin_surface = 'agent' AND conversation_id IS NOT NULL AND message_id IS NOT NULL AND agent_id IS NOT NULL AND workflow_id IS NULL AND node_id IS NULL) OR
   (origin_surface = 'workflow' AND workflow_id IS NOT NULL AND run_id IS NOT NULL AND node_id IS NOT NULL AND conversation_id IS NULL AND message_id IS NULL AND agent_id IS NULL))) OR
 (origin_version = 2 AND origin_surface = 'digital_human' AND digital_human_id IS NOT NULL AND btrim(digital_human_id) <> '' AND conversation_id IS NOT NULL AND message_id IS NOT NULL AND run_id IS NOT NULL AND agent_id IS NULL AND workflow_id IS NULL AND node_id IS NULL));

ALTER TABLE public.digital_human_runs DROP CONSTRAINT digital_human_runs_status_check;
ALTER TABLE public.digital_human_runs ADD CONSTRAINT digital_human_runs_status_check CHECK (status IN ('queued','running','waiting','canceling','succeeded','failed','canceled','unknown'));
ALTER TABLE public.digital_human_runs ADD COLUMN waiting_reason varchar(32) NOT NULL DEFAULT '';
ALTER TABLE public.digital_human_runs ADD COLUMN waiting_receipt uuid;
ALTER TABLE public.digital_human_runs ADD CONSTRAINT digital_human_runs_waiting_check CHECK (
 (status = 'waiting' AND waiting_reason = 'waiting_approval' AND waiting_receipt IS NOT NULL) OR
 (status = 'waiting' AND waiting_reason IN ('waiting_question','waiting_client_action') AND waiting_receipt IS NULL) OR
 (status <> 'waiting' AND waiting_reason = '' AND waiting_receipt IS NULL));

CREATE TABLE public.connector_host_projection_outbox (
 receipt_id uuid NOT NULL, projection_version integer NOT NULL,
 organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
 origin jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP, acked_at timestamptz,
 PRIMARY KEY (receipt_id, projection_version),
 FOREIGN KEY (receipt_id) REFERENCES public.connector_confirmation_continuations(route_receipt) ON DELETE RESTRICT,
 CHECK (projection_version > 0 AND jsonb_typeof(origin) = 'object'));
CREATE INDEX idx_connector_host_projection_pending ON public.connector_host_projection_outbox(created_at, receipt_id) WHERE acked_at IS NULL;
`

func init() {
	registerSchemaMigration("20260904120000_digital_human_connector_host", upDigitalHumanConnectorHost, nil)
}
func upDigitalHumanConnectorHost(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanConnectorHostSQL)
}
