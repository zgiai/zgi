package orgaudit

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRepositoryListPostgres runs the list query against PostgreSQL. Point
// ZGI_ORG_AUDIT_TEST_DSN at a disposable database.
func TestRepositoryListPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_ORG_AUDIT_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_ORG_AUDIT_TEST_DSN to run the org audit repository test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DROP TABLE IF EXISTS public.org_audit_logs`,
		`DROP TABLE IF EXISTS public.accounts`,
		`CREATE TABLE public.accounts (id uuid PRIMARY KEY, name varchar(255) NOT NULL, email varchar(255))`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(migrationSQL(t)).Error; err != nil {
		t.Fatal(err)
	}

	organization := uuid.New()
	other := uuid.New()
	actor := uuid.New()
	target := uuid.New()
	if err := db.Exec(`INSERT INTO public.accounts (id, name, email) VALUES (?, 'Ada', 'ada@example.com'), (?, 'Bob', 'bob@example.com')`, actor, target).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	logs := []Log{}
	for i := 0; i < 5; i++ {
		logs = append(logs, Log{ID: uuid.New(), OrganizationID: organization, ActorAccountID: &actor, TargetAccountID: &target,
			TargetType: TargetMember, TargetID: target.String(), Action: ActionMemberStatusUpdate, Result: ResultSuccess,
			Before: []byte(`{"status":"active"}`), After: []byte(`{"status":"inactive"}`), CreatedAt: base.Add(time.Duration(i) * time.Hour)})
	}
	logs = append(logs,
		Log{ID: uuid.New(), OrganizationID: organization, ActorAccountID: &target, Action: ActionRoleCreate, Result: ResultDenied, Before: []byte("{}"), After: []byte("{}"), CreatedAt: base.Add(10 * time.Hour)},
		Log{ID: uuid.New(), OrganizationID: other, ActorAccountID: &actor, Action: ActionRoleCreate, Result: ResultSuccess, Before: []byte("{}"), After: []byte("{}"), CreatedAt: base},
	)
	repository := NewRepository(db)
	ctx := context.Background()
	if err := repository.Create(ctx, logs); err != nil {
		t.Fatal(err)
	}

	rows, total, err := repository.List(ctx, Filter{OrganizationID: organization, Page: 1, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if total != 6 || len(rows) != 4 || rows[0].Action != ActionRoleCreate || rows[0].ActorName != "Bob" {
		t.Fatalf("total=%d rows=%d first=%+v", total, len(rows), rows[0])
	}

	rows, total, err = repository.List(ctx, Filter{OrganizationID: organization, ActorAccountID: &actor, TargetAccountID: &target, Action: ActionMemberStatusUpdate, Page: 2, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(rows) != 2 || rows[0].TargetName != "Bob" || rows[0].ActorEmail != "ada@example.com" || string(rows[0].After) != `{"status": "inactive"}` {
		t.Fatalf("total=%d rows=%+v", total, rows)
	}

	from, to := base.Add(time.Hour), base.Add(3*time.Hour)
	_, total, err = repository.List(ctx, Filter{OrganizationID: organization, From: &from, To: &to, Page: 1, Limit: 10})
	if err != nil || total != 2 {
		t.Fatalf("time range total=%d err=%v", total, err)
	}

	_, total, err = repository.List(ctx, Filter{OrganizationID: organization, Result: ResultDenied, Page: 1, Limit: 10})
	if err != nil || total != 1 {
		t.Fatalf("result filter total=%d err=%v", total, err)
	}
}

// migrationSQL reads the table definition from the migration itself, so the
// repository is tested against the schema production runs.
func migrationSQL(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../migrations/202610261000000000_create_org_audit_logs.go")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(source), "`", 3)
	if len(parts) < 3 {
		t.Fatal("migration SQL not found")
	}
	return parts[1]
}
