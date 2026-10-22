package digitalhuman

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunsNameAndFilterTheirRequesters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE accounts (id TEXT, name TEXT, email TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE digital_human_end_user_instances (organization_id TEXT, workspace_id TEXT, digital_human_id TEXT, subject_id TEXT, label TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
	digitalHumanID := uuid.New()
	member, apiUser := uuid.New(), uuid.New()
	db.Exec(`INSERT INTO accounts VALUES (?, 'Ada', 'ada@example.com')`, member.String())
	db.Exec(`INSERT INTO digital_human_end_user_instances VALUES (?, ?, ?, ?, 'Customer A')`, scope.OrganizationID.String(), scope.WorkspaceID.String(), digitalHumanID.String(), apiUser.String())
	now := time.Now()
	add := func(surface, subjectKind string, subject *uuid.UUID, event string) {
		run := &Run{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			DigitalHumanVersionID: uuid.New(), IdempotencyKey: uuid.NewString(), InvocationEvent: datatypes.JSON(event), RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionTrace: datatypes.JSON(`{}`), Query: "q",
			Status: RunStatusSucceeded, Surface: surface, UserSubjectKind: subjectKind, UserSubjectID: subject, CreatedAt: now, UpdatedAt: now}
		now = now.Add(time.Second)
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
	}
	add(SurfaceConsole, "account", &member, `{}`)
	add(SurfaceAPI, "api_end_user", &apiUser, `{}`)
	add(SurfaceChannel, "", nil, `{"origin":{"kind":"channel","provider":"feishu"}}`)
	add(SurfaceWebApp, "", nil, `{}`)
	add(SurfaceTask, "", nil, `{}`)

	repo := &GormRepository{db: db}
	ctx := context.Background()
	count := func(query ListQuery) int64 {
		t.Helper()
		query.Page, query.PageSize = 1, 20
		_, total, err := repo.ListRuns(ctx, scope, digitalHumanID, query)
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	// A task run without a requesting subject belongs to a manager's task.
	if got := count(ListQuery{RequesterKinds: []TaskRequesterKind{TaskRequesterMember}}); got != 2 {
		t.Fatalf("member runs = %d", got)
	}
	if got := count(ListQuery{RequesterKinds: []TaskRequesterKind{TaskRequesterAPIUser, TaskRequesterChannel}}); got != 2 {
		t.Fatalf("api and channel runs = %d", got)
	}
	if got := count(ListQuery{RequesterKinds: []TaskRequesterKind{TaskRequesterVisitor}}); got != 1 {
		t.Fatalf("visitor runs = %d", got)
	}
	if got := count(ListQuery{RequesterID: apiUser.String()}); got != 1 {
		t.Fatalf("runs of one requester = %d", got)
	}
	if _, _, err := repo.ListRuns(ctx, scope, digitalHumanID, ListQuery{Page: 1, PageSize: 20, RequesterKinds: []TaskRequesterKind{"robot"}}); err != ErrInvalidRequest {
		t.Fatalf("unknown requester kind err = %v", err)
	}

	runs, _, err := repo.ListRuns(ctx, scope, digitalHumanID, ListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DescribeRunRequesters(ctx, scope, digitalHumanID, runs); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, run := range runs {
		if run.Requester == nil {
			got[run.Surface] = ""
			continue
		}
		got[run.Surface] = string(run.Requester.Kind) + ":" + run.Requester.Name
	}
	want := map[string]string{
		SurfaceConsole: "member:Ada", SurfaceAPI: "api_user:Customer A", SurfaceChannel: "channel:feishu",
		SurfaceWebApp: "visitor:", SurfaceTask: "",
	}
	for surface, value := range want {
		if got[surface] != value {
			t.Fatalf("requester of %s = %q, want %q (all %v)", surface, got[surface], value, got)
		}
	}
}
