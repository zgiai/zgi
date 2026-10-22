package migrations

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddDigitalHumanEndUserInstancesMigrationContract(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS public.digital_human_end_user_instances.*subject_id uuid NOT NULL REFERENCES public.user_subjects.*idx_digital_human_end_user_instances_subject.*ADD COLUMN IF NOT EXISTS end_user_instance_id`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)INSERT INTO public.digital_human_end_user_instances.*ON CONFLICT \(digital_human_id, subject_id\) DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upAddDigitalHumanEndUserInstances(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddDigitalHumanEndUserInstancesBackfillsHistoricalEndUsersPostgres(t *testing.T) {
	db := openExecutionFileTestPostgres(t)
	if err := db.Exec(`
		CREATE TABLE public.user_subjects (id uuid PRIMARY KEY);
		CREATE TABLE public.digital_human_runs (
			id uuid PRIMARY KEY, organization_id uuid NOT NULL, workspace_id uuid NOT NULL, digital_human_id uuid NOT NULL,
			principal_id uuid, user_subject_kind varchar(32) NOT NULL DEFAULT '', user_subject_id uuid,
			surface varchar(64) NOT NULL, source_trust varchar(32), created_at timestamptz NOT NULL
		);`).Error; err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, digitalHumanID := uuid.New(), uuid.New(), uuid.New()
	apiUser, webUser, member, missing, keyID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, subject := range []uuid.UUID{apiUser, webUser, member} {
		if err := db.Exec(`INSERT INTO user_subjects (id) VALUES (?)`, subject).Error; err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	runs := []struct {
		kind, surface string
		trust         any
		subject       uuid.UUID
		at            time.Time
	}{
		{"api_end_user", "digital_human_api", "external_end_user", apiUser, base.Add(2 * time.Hour)},
		{"api_end_user", "digital_human_api", nil, apiUser, base},
		{"account", "digital_human_webapp", "external_end_user", webUser, base.Add(time.Hour)},
		{"account", "digital_human_console", "workspace_member", member, base},
		{"api_end_user", "digital_human_api", "external_end_user", missing, base},
	}
	for _, run := range runs {
		if err := db.Exec(`INSERT INTO digital_human_runs (id, organization_id, workspace_id, digital_human_id, principal_id, user_subject_kind, user_subject_id, surface, source_trust, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New(), organizationID, workspaceID, digitalHumanID, keyID, run.kind, run.subject, run.surface, run.trust, run.at).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := upAddDigitalHumanEndUserInstances(mschema.New(db)); err != nil {
			t.Fatal(err)
		}
	}
	var instances []struct {
		SubjectID    uuid.UUID
		IssuerType   string
		IssuerID     *uuid.UUID
		FirstSeenAt  time.Time
		LastActiveAt time.Time
	}
	if err := db.Raw(`SELECT subject_id, issuer_type, issuer_id, first_seen_at, last_active_at FROM digital_human_end_user_instances ORDER BY first_seen_at`).Scan(&instances).Error; err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %#v, want the API and WebApp end users only", instances)
	}
	api, web := instances[0], instances[1]
	if api.SubjectID != apiUser || api.IssuerType != "digital_human_api" || api.IssuerID == nil || *api.IssuerID != keyID ||
		!api.FirstSeenAt.Equal(base) || !api.LastActiveAt.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("API end user instance = %#v", api)
	}
	if web.SubjectID != webUser || web.IssuerType != "digital_human_webapp" || web.IssuerID != nil {
		t.Fatalf("WebApp end user instance = %#v", web)
	}
	if err := db.Exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, status, first_seen_at, last_active_at, created_at, updated_at)
		VALUES (gen_random_uuid(), ?, ?, ?, 'account', ?, 'digital_human_webapp', 'paused', now(), now(), now(), now())`, organizationID, workspaceID, digitalHumanID, member).Error; err == nil {
		t.Fatal("unknown instance status was admitted")
	}
}
