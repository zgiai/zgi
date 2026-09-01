package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateRegistrationProvisioningOutboxID = "202609021600000000_create_registration_provisioning_outbox"

const createRegistrationProvisioningOutboxSQL = `
	CREATE TABLE IF NOT EXISTS public.registration_provisioning_outbox (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
		status varchar(16) NOT NULL DEFAULT 'pending',
		completed_step smallint NOT NULL DEFAULT 0,
		attempt_count integer NOT NULL DEFAULT 0,
		next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		lease_owner uuid,
		lease_until timestamptz,
		last_error text NOT NULL DEFAULT '',
		completed_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT registration_provisioning_outbox_status_check
			CHECK (status IN ('pending', 'processing', 'completed')),
		CONSTRAINT registration_provisioning_outbox_step_check
			CHECK (completed_step BETWEEN 0 AND 3),
		CONSTRAINT registration_provisioning_outbox_attempt_check
			CHECK (attempt_count >= 0),
		CONSTRAINT registration_provisioning_outbox_lease_check
			CHECK (
				(status = 'processing' AND lease_owner IS NOT NULL AND lease_until IS NOT NULL)
				OR (status <> 'processing' AND lease_owner IS NULL AND lease_until IS NULL)
			),
		CONSTRAINT registration_provisioning_outbox_completion_check
			CHECK (
				(status = 'completed' AND completed_step = 3 AND completed_at IS NOT NULL)
				OR (status <> 'completed' AND completed_at IS NULL)
			)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_registration_provisioning_outbox_organization
		ON public.registration_provisioning_outbox (organization_id);
	CREATE INDEX IF NOT EXISTS idx_registration_provisioning_outbox_ready
		ON public.registration_provisioning_outbox (next_attempt_at, created_at)
		WHERE status = 'pending';
	CREATE INDEX IF NOT EXISTS idx_registration_provisioning_outbox_expired_lease
		ON public.registration_provisioning_outbox (lease_until)
		WHERE status = 'processing';
`

const rollbackRegistrationProvisioningOutboxSQL = `
	DO $$
	BEGIN
		IF EXISTS (
			SELECT 1
			FROM public.registration_provisioning_outbox
			LIMIT 1
		) THEN
			RAISE EXCEPTION 'cannot remove durable registration provisioning outbox while records exist';
		END IF;
	END
	$$;

	DROP TABLE public.registration_provisioning_outbox;
`

func init() {
	registerSchemaMigration(
		migrationCreateRegistrationProvisioningOutboxID,
		upCreateRegistrationProvisioningOutbox,
		downCreateRegistrationProvisioningOutbox,
	)
}

func upCreateRegistrationProvisioningOutbox(schema *mschema.Builder) error {
	return schema.Raw(createRegistrationProvisioningOutboxSQL)
}

func downCreateRegistrationProvisioningOutbox(schema *mschema.Builder) error {
	return schema.Raw(rollbackRegistrationProvisioningOutboxSQL)
}
