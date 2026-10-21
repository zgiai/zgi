package migrations

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestDigitalHumanConnectorResourceTypeMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)ALTER TABLE public.digital_human_resource_bindings.*ADD CONSTRAINT dh_resource_bindings_type_check").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := upDigitalHumanConnectorResources(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationDigitalHumanConnectorResourcesID {
			if migration.Rollback != nil {
				t.Fatal("resource type expansion must not remove support for saved native bindings")
			}
			return
		}
	}
	t.Fatal("resource type migration is not registered")
}

// Use the production binding table and immutability trigger in a random schema.
// This never falls back to application database configuration.
func TestDigitalHumanConnectorResourceTypePostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN for isolated PostgreSQL validation")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid isolated PostgreSQL configuration")
	}
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "historical_upgrade"}[upgrade], func(t *testing.T) {
			schema := "dh_connector_type_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			})
			exec := func(statement string, args ...any) {
				t.Helper()
				if _, err := db.ExecContext(ctx, strings.ReplaceAll(statement, "public.", schema+"."), args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`
			CREATE FUNCTION public.uuid_generate_v4() RETURNS uuid LANGUAGE SQL AS 'SELECT gen_random_uuid()';
			CREATE TABLE public.organizations (id uuid PRIMARY KEY);
			CREATE TABLE public.workspaces (id uuid PRIMARY KEY);
			CREATE TABLE public.accounts (id uuid PRIMARY KEY);
			CREATE TABLE public.digital_humans (id uuid PRIMARY KEY);
			CREATE TABLE public.digital_human_versions (
			 id uuid PRIMARY KEY, digital_human_id uuid, organization_id uuid, workspace_id uuid, state varchar(16));
			`)
			exec(createDigitalHumanPersonaTemplatesAndBindingsSQL)
			org, workspace, human, draft, history := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			exec("INSERT INTO public.organizations VALUES ($1)", org)
			exec("INSERT INTO public.workspaces VALUES ($1)", workspace)
			exec("INSERT INTO public.digital_humans VALUES ($1)", human)
			exec("INSERT INTO public.digital_human_versions VALUES ($1,$2,$3,$4,'draft'), ($5,$2,$3,$4,'draft')", draft, human, org, workspace, history)
			insert := strings.ReplaceAll(`INSERT INTO public.digital_human_resource_bindings
			 (organization_id,workspace_id,digital_human_id,digital_human_version_id,binding_type,resource_id,access_mode,resource_revision,binding_revision)
			 VALUES ($1,$2,$3,$4,$5,$6,'execute','fixture:1','fixture:binding')`, "public.", schema+".")
			insertBinding := func(version uuid.UUID, kind, resource string) error {
				_, err := db.ExecContext(ctx, insert, org, workspace, human, version, kind, resource)
				return err
			}
			requireConstraint := func(err error, constraint string) {
				t.Helper()
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "23514" || (constraint != "" && pgErr.ConstraintName != constraint) {
					t.Fatalf("expected check violation %q, got %v", constraint, err)
				}
			}
			var before string
			if upgrade {
				if err := insertBinding(history, "integration_connection", "historical"); err != nil {
					t.Fatal(err)
				}
				exec("UPDATE public.digital_human_versions SET state='published' WHERE id=$1", history)
				if err := db.QueryRowContext(ctx, "SELECT to_jsonb(b)::text FROM "+schema+".digital_human_resource_bindings b WHERE resource_id='historical'").Scan(&before); err != nil {
					t.Fatal(err)
				}
				requireConstraint(insertBinding(draft, "connector", "before-upgrade"), "dh_resource_bindings_type_check")
			}
			exec(allowDigitalHumanConnectorResourcesSQL)
			for _, kind := range []string{"skill", "knowledge_dataset", "database", "database_table", "workflow", "integration_connection", "connector"} {
				if err := insertBinding(draft, kind, kind); err != nil {
					t.Fatalf("valid type %s rejected: %v", kind, err)
				}
			}
			requireConstraint(insertBinding(draft, "invalid_resource_type", "invalid"), "dh_resource_bindings_type_check")
			if upgrade {
				var after string
				if err := db.QueryRowContext(ctx, "SELECT to_jsonb(b)::text FROM "+schema+".digital_human_resource_bindings b WHERE resource_id='historical'").Scan(&after); err != nil {
					t.Fatal(err)
				}
				if after != before {
					t.Fatal("upgrade modified a published historical binding")
				}
			}
			exec("UPDATE public.digital_human_versions SET state='published' WHERE id=$1", draft)
			_, err := db.ExecContext(ctx, "UPDATE "+schema+".digital_human_resource_bindings SET display_name='changed' WHERE resource_id='connector'")
			requireConstraint(err, "")
			_, err = db.ExecContext(ctx, "DELETE FROM "+schema+".digital_human_resource_bindings WHERE resource_id='connector'")
			requireConstraint(err, "")
			requireConstraint(insertBinding(draft, "connector", "published-insert"), "")
		})
	}
}
