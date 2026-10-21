package digitalhuman

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEndUserInstanceAdmissionPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman end-user instance PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "End-user instance integration"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "End-user instance integration"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{actorID, "Instance operator", "end-user-instance-" + actorID.String() + "@example.invalid"}},
	} {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	repository := NewRepository(tx)
	service := NewService(repository)
	firstSeen := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := service.Create(t.Context(), scope, CreateRequest{
		Name: "Instance subject", Persona: "Serve many end users.", DefaultModel: "test-model",
		MemoryPolicy: map[string]any{"automatic_extraction": false}, TriggerPolicy: map[string]any{"manual": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID
	subject, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, digitalHumanID, "customer-42")
	if err != nil {
		t.Fatal(err)
	}
	keyID := uuid.New()
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED", "true")

	admit := func(at time.Time) (*Run, error) {
		t.Helper()
		runID := uuid.New()
		event, err := json.Marshal(invocation.Event{ID: runID.String(), OccurredAt: at, Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: organizationID, WorkspaceID: workspaceID,
		}})
		if err != nil {
			t.Fatal(err)
		}
		subjectID, revision := subject.ID, "sha256:"+runID.String()
		run := &Run{
			ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: digitalHumanID,
			DigitalHumanVersionID: detail.Published.ID, PrincipalType: string(invocation.PrincipalService), PrincipalID: &keyID,
			UserSubjectKind: string(subject.Kind), UserSubjectID: &subjectID, Surface: SurfaceAPI, SourceTrust: sourceTrustPointer(SourceTrustExternalEndUser),
			IdempotencyKey: "end-user-instance-" + runID.String(), InvocationEvent: event, RunSpec: datatypes.JSON(`{}`), RunSpecRevision: revision,
			ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionSnapshotRevision: revision, ExecutionTrace: datatypes.JSON(`{}`),
			Query: "Hello", Status: RunStatusQueued, CreatedAt: at, UpdatedAt: at,
		}
		outbox := &RunOutbox{ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, RunID: runID, Status: RunOutboxPending, AvailableAt: at, CreatedAt: at, UpdatedAt: at}
		instance := endUserInstanceForRun(Scope{PrincipalID: keyID}, run, at)
		if instance == nil {
			t.Fatal("API end-user run got no instance")
		}
		return run, repository.CreateRunWithOutboxForEndUser(t.Context(), run, outbox, instance)
	}
	load := func() EndUserInstance {
		t.Helper()
		var rows []EndUserInstance
		if err := tx.Where("digital_human_id = ? AND subject_id = ?", digitalHumanID, subject.ID).Find(&rows).Error; err != nil || len(rows) != 1 {
			t.Fatalf("instances = %#v, %v; want exactly one", rows, err)
		}
		return rows[0]
	}

	// First contact registers the subject, creates the instance, and links the run.
	first, err := admit(firstSeen)
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	instance := load()
	if first.EndUserInstanceID == nil || *first.EndUserInstanceID != instance.ID || instance.Status != EndUserInstanceActive ||
		instance.IssuerType != SurfaceAPI || instance.IssuerID == nil || *instance.IssuerID != keyID || !instance.FirstSeenAt.Equal(firstSeen) {
		t.Fatalf("first run %v, instance %#v", first.EndUserInstanceID, instance)
	}
	if _, err := usersubject.NewService(tx).Require(t.Context(), subject); err != nil {
		t.Fatalf("subject was not registered: %v", err)
	}

	// Later activity reuses the instance and only moves its activity forward.
	second, err := admit(firstSeen.Add(time.Hour))
	if err != nil {
		t.Fatalf("second admission: %v", err)
	}
	instance = load()
	if *second.EndUserInstanceID != instance.ID || !instance.FirstSeenAt.Equal(firstSeen) || !instance.LastActiveAt.Equal(firstSeen.Add(time.Hour)) {
		t.Fatalf("second run %v, instance %#v", second.EndUserInstanceID, instance)
	}
	if _, err := admit(firstSeen.Add(30 * time.Minute)); err != nil {
		t.Fatalf("late admission: %v", err)
	}
	if instance = load(); !instance.LastActiveAt.Equal(firstSeen.Add(time.Hour)) {
		t.Fatalf("an older run moved activity back: %#v", instance)
	}

	// A suspended instance admits no run, and nothing about the attempt is kept.
	if err := tx.Model(&EndUserInstance{}).Where("id = ?", instance.ID).Update("status", EndUserInstanceSuspended).Error; err != nil {
		t.Fatal(err)
	}
	suspended, err := admit(firstSeen.Add(2 * time.Hour))
	if !errors.Is(err, ErrEndUserSuspended) {
		t.Fatalf("suspended admission error = %v", err)
	}
	var stored int64
	if err := tx.Model(&Run{}).Where("id = ?", suspended.ID).Count(&stored).Error; err != nil || stored != 0 {
		t.Fatalf("suspended run stored = %d, %v", stored, err)
	}
	if instance = load(); !instance.LastActiveAt.Equal(firstSeen.Add(time.Hour)) {
		t.Fatalf("a rejected run recorded activity: %#v", instance)
	}

	// An erased instance is a tombstone; a returning end user starts over in it.
	if err := tx.Model(&EndUserInstance{}).Where("id = ?", instance.ID).Update("status", EndUserInstanceErased).Error; err != nil {
		t.Fatal(err)
	}
	returned, err := admit(firstSeen.Add(3 * time.Hour))
	if err != nil {
		t.Fatalf("admission after erasure: %v", err)
	}
	instance = load()
	if *returned.EndUserInstanceID != instance.ID || instance.Status != EndUserInstanceActive || !instance.FirstSeenAt.Equal(firstSeen.Add(3*time.Hour)) {
		t.Fatalf("returning end user instance = %#v", instance)
	}
}
