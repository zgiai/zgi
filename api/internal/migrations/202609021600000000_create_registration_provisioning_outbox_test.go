package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestCreateRegistrationProvisioningOutboxExecutesThroughSchemaBuilder(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE IF NOT EXISTS public.registration_provisioning_outbox.*idx_registration_provisioning_outbox_expired_lease").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upCreateRegistrationProvisioningOutbox(mschema.New(db)); err != nil {
		t.Fatalf("up migration error = %v", err)
	}
	mock.ExpectExec("(?s)DO \\$\\$.*cannot remove durable registration provisioning outbox while records exist.*DROP TABLE public.registration_provisioning_outbox").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := downCreateRegistrationProvisioningOutbox(mschema.New(db)); err != nil {
		t.Fatalf("down migration error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("migration database expectations: %v", err)
	}
}

func TestCreateRegistrationProvisioningOutboxMigrationContract(t *testing.T) {
	sql := strings.ToLower(createRegistrationProvisioningOutboxSQL)
	for _, fragment := range []string{
		"create table if not exists public.registration_provisioning_outbox",
		"organization_id uuid not null",
		"account_id uuid not null",
		"completed_step smallint not null default 0",
		"next_attempt_at timestamptz not null",
		"lease_owner uuid",
		"lease_until timestamptz",
		"status = 'processing' and lease_owner is not null and lease_until is not null",
		"unique index if not exists idx_registration_provisioning_outbox_organization",
		"where status = 'pending'",
		"where status = 'processing'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration is missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"organization_name", "organization_created_at", "owner_email"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("outbox migration must not persist mutable or personal snapshot field %q", forbidden)
		}
	}
	rollback := strings.ToLower(rollbackRegistrationProvisioningOutboxSQL)
	for _, fragment := range []string{
		"if exists (",
		"from public.registration_provisioning_outbox",
		"raise exception 'cannot remove durable registration provisioning outbox while records exist'",
		"drop table public.registration_provisioning_outbox",
	} {
		if !strings.Contains(rollback, fragment) {
			t.Fatalf("fail-closed rollback is missing %q", fragment)
		}
	}

	found := false
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationCreateRegistrationProvisioningOutboxID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("migration %s is not registered", migrationCreateRegistrationProvisioningOutboxID)
	}
}
