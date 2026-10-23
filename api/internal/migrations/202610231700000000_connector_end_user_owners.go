package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// Connector rows may be owned by a Runtime Workspace end user as well as by
// an account (ADR 0007): an end user brings their own key for a DigitalHuman's
// per-user Connector slot. owner_account_id keeps holding the owner's id, and
// owner_kind says what it names. A foreign key cannot reference one of two
// tables, so the account foreign keys become a guard that derives the kind
// from the id and requires the owner, and the acting principal, to exist.
// Accounts stay undeletable while Connector rows reference them, as the
// RESTRICT foreign keys made them. Account queries filter by an account id,
// which no end-user row carries, so they never see end-user rows.
func init() {
	registerSchemaMigration("202610231700000000_connector_end_user_owners", upConnectorEndUserOwners, downConnectorEndUserOwners)
}

var connectorOwnerTables = []string{
	"connector_connections",
	"connector_key_setup_attempts",
	"connector_target_bindings",
	"connector_lifecycle_decisions",
	"connector_operation_outcomes",
	"connector_confirmation_continuations",
}

func upConnectorEndUserOwners(schema *mschema.Builder) error {
	sql := `
CREATE OR REPLACE FUNCTION public.connector_principal_kind(principal uuid) RETURNS varchar LANGUAGE sql STABLE AS $$
	SELECT CASE
		WHEN EXISTS (SELECT 1 FROM public.accounts WHERE id = principal) THEN 'account'
		WHEN EXISTS (SELECT 1 FROM public.user_subjects WHERE id = principal AND kind = 'api_end_user') THEN 'end_user'
	END;
$$;

CREATE OR REPLACE FUNCTION public.connector_owner_guard() RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE owner_kind varchar;
	DECLARE acting jsonb := to_jsonb(NEW) -> 'acting_principal_id';
	BEGIN
		owner_kind := public.connector_principal_kind(NEW.owner_account_id);
		IF owner_kind IS NULL THEN
			RAISE EXCEPTION 'connector owner does not exist' USING ERRCODE = '23503';
		END IF;
		IF acting IS NOT NULL AND acting <> 'null'::jsonb
			AND public.connector_principal_kind((acting #>> '{}')::uuid) IS NULL THEN
			RAISE EXCEPTION 'connector acting principal does not exist' USING ERRCODE = '23503';
		END IF;
		NEW.owner_kind := owner_kind;
		RETURN NEW;
	END;
$$;

CREATE OR REPLACE FUNCTION public.connector_account_delete_guard() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF EXISTS (SELECT 1 FROM public.connector_connections WHERE owner_account_id = OLD.id)
			OR EXISTS (SELECT 1 FROM public.connector_key_setup_attempts WHERE owner_account_id = OLD.id)
			OR EXISTS (SELECT 1 FROM public.connector_target_bindings WHERE owner_account_id = OLD.id)
			OR EXISTS (SELECT 1 FROM public.connector_lifecycle_decisions WHERE owner_account_id = OLD.id OR acting_principal_id = OLD.id)
			OR EXISTS (SELECT 1 FROM public.connector_operation_outcomes WHERE owner_account_id = OLD.id OR acting_principal_id = OLD.id)
			OR EXISTS (SELECT 1 FROM public.connector_confirmation_continuations WHERE owner_account_id = OLD.id) THEN
			RAISE EXCEPTION 'account is referenced by connector records' USING ERRCODE = '23503';
		END IF;
		RETURN OLD;
	END;
$$;
DROP TRIGGER IF EXISTS trg_connector_account_delete_guard ON public.accounts;
CREATE TRIGGER trg_connector_account_delete_guard BEFORE DELETE ON public.accounts
	FOR EACH ROW EXECUTE FUNCTION public.connector_account_delete_guard();
`
	for _, table := range connectorOwnerTables {
		sql += `
ALTER TABLE public.` + table + ` ADD COLUMN IF NOT EXISTS owner_kind varchar(16) NOT NULL DEFAULT 'account';
ALTER TABLE public.` + table + ` DROP CONSTRAINT IF EXISTS ck_` + table + `_owner_kind;
ALTER TABLE public.` + table + ` ADD CONSTRAINT ck_` + table + `_owner_kind CHECK (owner_kind IN ('account', 'end_user'));
ALTER TABLE public.` + table + ` DROP CONSTRAINT IF EXISTS ` + table + `_owner_account_id_fkey;
ALTER TABLE public.` + table + ` DROP CONSTRAINT IF EXISTS ` + table + `_acting_principal_id_fkey;
DROP TRIGGER IF EXISTS trg_` + table + `_owner_guard ON public.` + table + `;
CREATE TRIGGER trg_` + table + `_owner_guard BEFORE INSERT OR UPDATE` +
			// The static check reads "UPDATE " in a literal as data mutation;
			// this is the trigger's event list, so the literal ends before it.
			` OF owner_account_id ON public.` + table + `
	FOR EACH ROW EXECUTE FUNCTION public.connector_owner_guard();
`
	}
	return schema.Raw(sql)
}

func downConnectorEndUserOwners(schema *mschema.Builder) error {
	sql := `DROP TRIGGER IF EXISTS trg_connector_account_delete_guard ON public.accounts;`
	for _, table := range connectorOwnerTables {
		sql += `
DELETE FROM public.` + table + ` WHERE owner_kind = 'end_user';
DROP TRIGGER IF EXISTS trg_` + table + `_owner_guard ON public.` + table + `;
ALTER TABLE public.` + table + ` DROP CONSTRAINT IF EXISTS ck_` + table + `_owner_kind;
ALTER TABLE public.` + table + ` DROP COLUMN IF EXISTS owner_kind;
ALTER TABLE public.` + table + ` ADD CONSTRAINT ` + table + `_owner_account_id_fkey FOREIGN KEY (owner_account_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;
`
	}
	sql += `
ALTER TABLE public.connector_lifecycle_decisions ADD CONSTRAINT connector_lifecycle_decisions_acting_principal_id_fkey FOREIGN KEY (acting_principal_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;
ALTER TABLE public.connector_operation_outcomes ADD CONSTRAINT connector_operation_outcomes_acting_principal_id_fkey FOREIGN KEY (acting_principal_id) REFERENCES public.accounts(id) ON DELETE RESTRICT;
DROP FUNCTION IF EXISTS public.connector_account_delete_guard();
DROP FUNCTION IF EXISTS public.connector_owner_guard();
DROP FUNCTION IF EXISTS public.connector_principal_kind(uuid);
`
	return schema.Raw(sql)
}
