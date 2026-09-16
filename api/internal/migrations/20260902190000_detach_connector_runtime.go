package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260902190000ID = "20260902190000_detach_connector_runtime"

// Historical tables remain available to the offline converter. No live native
// write may execute a trigger or foreign key that reads those tables.
const detachConnectorRuntimeSQL = `
DROP TRIGGER IF EXISTS connector_connections_legacy_collision_guard ON public.connector_connections;
DROP TRIGGER IF EXISTS agent_resource_bindings_integration_connection_guard ON public.agent_resource_bindings;
DROP TRIGGER IF EXISTS agent_resource_bindings_legacy_projection_read_only_guard ON public.agent_resource_bindings;
DROP TRIGGER IF EXISTS connector_legacy_identity_mappings_personal_guard ON public.connector_legacy_identity_mappings;
DROP TRIGGER IF EXISTS connector_reversal_baselines_identity_guard ON public.connector_reversal_baselines;
ALTER TABLE public.connector_reversal_baselines DROP CONSTRAINT IF EXISTS connector_reversal_baselines_legacy_fk;
DO $$ DECLARE source_table text; BEGIN
  FOREACH source_table IN ARRAY ARRAY['integration_connections','integration_action_policies','integration_connection_grants','agent_resource_bindings','aichat_integration_preferences'] LOOP
    IF to_regclass('public.' || source_table) IS NOT NULL THEN
      EXECUTE format('DROP TRIGGER IF EXISTS connector_source_barrier ON public.%I', source_table);
      EXECUTE format('DROP TRIGGER IF EXISTS connector_source_invalidation ON public.%I', source_table);
    END IF;
  END LOOP;
END $$;
DROP FUNCTION IF EXISTS public.enforce_connector_connection_legacy_collision();
DROP FUNCTION IF EXISTS public.enforce_agent_integration_connection_binding();
DROP FUNCTION IF EXISTS public.enforce_legacy_agent_integration_projection_read_only();
DROP FUNCTION IF EXISTS public.enforce_connector_legacy_personal_mapping();
DROP FUNCTION IF EXISTS public.enforce_connector_reversal_baseline_identity();
DROP FUNCTION IF EXISTS public.connector_reversal_projection_authorized(public.agent_resource_bindings, public.agent_resource_bindings);
DROP FUNCTION IF EXISTS public.invalidate_connector_legacy_source_row();
DROP FUNCTION IF EXISTS public.lock_connector_legacy_source_statement();
DROP FUNCTION IF EXISTS public.connector_native_host_projection_owned(jsonb, boolean);
DROP FUNCTION IF EXISTS public.connector_legacy_source_digest(uuid, uuid);

ALTER TABLE public.connector_connections ADD COLUMN connection_constraint jsonb;
ALTER TABLE public.connector_target_bindings
  ADD COLUMN binding_constraint jsonb,
  ADD COLUMN target_scope varchar(16) NOT NULL DEFAULT 'draft',
  ADD COLUMN target_version_id varchar(255) NOT NULL DEFAULT '';
UPDATE public.connector_target_bindings SET target_scope = 'live' WHERE target_type = 'chat';
UPDATE public.connector_target_bindings b SET target_scope = 'published', target_version_id = b.target_id
  WHERE b.target_type = 'workflow' AND EXISTS (
    SELECT 1 FROM public.workflows w WHERE w.version_uuid::text = b.target_id AND w.version <> 'draft');
ALTER TABLE public.connector_target_bindings ADD CONSTRAINT connector_target_scope_check CHECK (
  (target_type = 'chat' AND target_scope = 'live' AND target_version_id = '') OR
  (target_type IN ('agent','workflow') AND ((target_scope = 'draft' AND target_version_id = '') OR
  (target_scope = 'published' AND target_version_id <> ''))));
DROP INDEX public.idx_connector_target_bindings_logical_key;
CREATE UNIQUE INDEX idx_connector_target_bindings_logical_key ON public.connector_target_bindings
 (organization_id, owner_account_id, target_type, target_id, target_scope, target_version_id, connector_id, COALESCE(binding_instance_id, ''));

ALTER TABLE public.connector_lifecycle_decisions ADD COLUMN target_scope varchar(16), ADD COLUMN target_version_id varchar(255);
ALTER TABLE public.connector_operation_outcomes ADD COLUMN target_scope varchar(16) NOT NULL DEFAULT '', ADD COLUMN target_version_id varchar(255) NOT NULL DEFAULT '';
CREATE TABLE public.connector_invocation_claims (
  receipt_id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
  acting_principal_id uuid NOT NULL, binding_id uuid NOT NULL, connection_id uuid NOT NULL,
  connector_id varchar(128) NOT NULL, target_type varchar(16) NOT NULL,
  target_id varchar(255) NOT NULL, target_scope varchar(16) NOT NULL,
  target_version_id varchar(255) NOT NULL DEFAULT '', binding_instance_id varchar(255) NOT NULL DEFAULT '',
  operation_id varchar(128) NOT NULL, logical_operation_id varchar(128) NOT NULL, correlation_id varchar(128) NOT NULL,
  input_digest varchar(64) NOT NULL, contract_digest varchar(64) NOT NULL,
  binding_revision integer NOT NULL, credential_version integer NOT NULL,
  state varchar(32) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id, logical_operation_id), UNIQUE (receipt_id, organization_id, owner_account_id),
  CHECK (state IN ('claimed','awaiting_confirmation','submitted','successful','failed','unknown','denied','blocked','cancelled')),
  CHECK (input_digest ~ '^[a-f0-9]{64}$' AND contract_digest ~ '^[a-f0-9]{64}$'),
  CHECK (binding_revision > 0 AND credential_version > 0)
);
-- Claims intentionally have no cascading connection/host FK: a deleted host
-- must never remove a submission's permanent duplicate/unknown barrier.
ALTER TABLE public.connector_confirmation_continuations DROP CONSTRAINT connector_confirmation_continuations_route_owner_fk;
ALTER TABLE public.connector_confirmation_continuations ADD CONSTRAINT connector_confirmation_continuations_native_claim_fk
  FOREIGN KEY (route_receipt, organization_id, owner_account_id)
  REFERENCES public.connector_invocation_claims(receipt_id, organization_id, owner_account_id) NOT VALID;

CREATE TABLE public.connector_unavailable_references (
  id uuid PRIMARY KEY, organization_id uuid NOT NULL, owner_account_id uuid NOT NULL,
  target_type varchar(16) NOT NULL, target_id varchar(255) NOT NULL,
  target_scope varchar(16) NOT NULL, target_version_id varchar(255) NOT NULL DEFAULT '',
  binding_instance_id varchar(255) NOT NULL DEFAULT '', former_reference_id varchar(255) NOT NULL,
  label varchar(255) NOT NULL, reason_code varchar(64) NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'needs_replacement', revision integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (revision > 0), CHECK (status IN ('needs_replacement','explicitly_replaced','detached')),
  UNIQUE (organization_id,owner_account_id,target_type,target_id,target_scope,target_version_id,binding_instance_id,former_reference_id)
);
CREATE TABLE public.connector_schema_readiness (
  id integer PRIMARY KEY CHECK (id = 1), schema_version integer NOT NULL,
  state varchar(32) NOT NULL CHECK (state IN ('conversion_required','ready')),
  report_digest varchar(64), verified_at timestamptz
);
INSERT INTO public.connector_schema_readiness(id,schema_version,state)
SELECT 1,1,CASE WHEN EXISTS(SELECT 1 FROM public.connector_connections)
  OR EXISTS(SELECT 1 FROM public.integration_connections)
  OR EXISTS(SELECT 1 FROM public.connector_execution_route_claims)
  OR EXISTS(SELECT 1 FROM public.agent_resource_bindings WHERE binding_type='integration_connection')
  OR EXISTS(SELECT 1 FROM public.aichat_integration_preferences)
  OR EXISTS(SELECT 1 FROM public.agents_configs WHERE agent_mode LIKE '%integration_bindings%' OR agent_mode LIKE '%connector_bindings%')
  OR EXISTS(SELECT 1 FROM public.agent_published_versions WHERE config_snapshot::text LIKE '%integration_bindings%' OR config_snapshot::text LIKE '%connector_bindings%')
  OR EXISTS(SELECT 1 FROM public.workflows WHERE graph::text LIKE '%connector%')
  OR EXISTS(SELECT 1 FROM public.workflow_runs WHERE graph::text LIKE '%connector%')
  OR EXISTS(SELECT 1 FROM public.chat_runtime_custom_skills)
  THEN 'conversion_required' ELSE 'ready' END;
CREATE TABLE public.connector_conversion_records (
  source_kind varchar(64) NOT NULL, source_id varchar(255) NOT NULL,
  source_digest varchar(64) NOT NULL, destination_id uuid, result varchar(32) NOT NULL,
  converted_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(source_kind, source_id),
  CHECK (source_digest ~ '^[a-f0-9]{64}$')
);

CREATE FUNCTION public.advance_connector_invocation_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME = 'connector_operation_outcomes' THEN
    UPDATE public.connector_invocation_claims SET state = NEW.status
      WHERE organization_id=NEW.organization_id AND logical_operation_id=NEW.logical_operation_id AND state='submitted';
  ELSIF NEW.logical_operation_id IS NOT NULL AND NEW.decision IN ('dispatch','confirmation','execution') THEN
    UPDATE public.connector_invocation_claims SET state = NEW.status
      WHERE organization_id=NEW.organization_id AND logical_operation_id=NEW.logical_operation_id
      AND state IN ('claimed','awaiting_confirmation')
      AND NEW.status IN ('submitted','denied','blocked','cancelled');
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER connector_invocation_decision_state AFTER INSERT ON public.connector_lifecycle_decisions
  FOR EACH ROW EXECUTE FUNCTION public.advance_connector_invocation_state();
CREATE TRIGGER connector_invocation_outcome_state AFTER INSERT ON public.connector_operation_outcomes
  FOR EACH ROW EXECUTE FUNCTION public.advance_connector_invocation_state();

CREATE FUNCTION public.protect_connector_invocation_claim() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'Connector invocation barriers are permanent' USING ERRCODE = '23514';
  END IF;
  IF (to_jsonb(NEW) - 'state') IS DISTINCT FROM (to_jsonb(OLD) - 'state') OR NOT (
    NEW.state = OLD.state OR
    (OLD.state IN ('claimed','awaiting_confirmation') AND NEW.state IN ('awaiting_confirmation','submitted','denied','blocked','cancelled')) OR
    (OLD.state = 'submitted' AND NEW.state IN ('successful','failed','unknown'))
  ) THEN
    RAISE EXCEPTION 'Connector claim identity or terminal state cannot change' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER connector_invocation_claim_guard BEFORE UPDATE OR DELETE ON public.connector_invocation_claims
  FOR EACH ROW EXECUTE FUNCTION public.protect_connector_invocation_claim();

CREATE OR REPLACE FUNCTION public.enforce_connector_operation_outcome() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM public.connector_lifecycle_decisions AS decision
    JOIN public.connector_invocation_claims AS claim ON claim.organization_id = decision.organization_id
      AND claim.logical_operation_id = decision.logical_operation_id
  WHERE decision.id = NEW.decision_id AND decision.status = 'submitted' AND claim.state = 'submitted'
    AND decision.organization_id = NEW.organization_id AND decision.owner_account_id = NEW.owner_account_id
    AND decision.acting_principal_id = NEW.acting_principal_id AND decision.connector_id = NEW.connector_id
    AND decision.connection_id = NEW.connection_id AND decision.target_type = NEW.target_type
    AND decision.target_id = NEW.target_id AND decision.target_scope = NEW.target_scope
    AND decision.target_version_id IS NOT DISTINCT FROM NEW.target_version_id
    AND decision.binding_instance_id IS NOT DISTINCT FROM NEW.binding_instance_id
    AND decision.operation_id = NEW.operation_id AND decision.logical_operation_id = NEW.logical_operation_id
    AND claim.owner_account_id = NEW.owner_account_id AND claim.acting_principal_id = NEW.acting_principal_id
    AND claim.connector_id = NEW.connector_id AND claim.connection_id = NEW.connection_id
    AND claim.target_type = NEW.target_type AND claim.target_id = NEW.target_id AND claim.target_scope = NEW.target_scope
    AND claim.target_version_id = COALESCE(NEW.target_version_id, '')
    AND claim.binding_instance_id = COALESCE(NEW.binding_instance_id, '') AND claim.operation_id = NEW.operation_id
    FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Connector outcome requires its exact native claim and submitted decision' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;
`

func init()                                          { registerSchemaMigration(migration20260902190000ID, up20260902190000, down20260902190000) }
func up20260902190000(schema *mschema.Builder) error { return schema.Raw(detachConnectorRuntimeSQL) }
func down20260902190000(schema *mschema.Builder) error {
	return schema.Raw(`DO $$ BEGIN RAISE EXCEPTION 'Connector detachment is forward-only; restore the maintenance-window backup to roll back'; END $$;`)
}
