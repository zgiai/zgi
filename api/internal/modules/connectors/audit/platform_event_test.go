package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestPlatformLifecycleEventRecordsAndVerifiesIdenticalReplay(t *testing.T) {
	repository, mock := openAuditRepositoryMock(t)
	event := platformEventFixture()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repository.CreatePlatformEvent(context.Background(), event); err != nil {
		t.Fatalf("CreatePlatformEvent() error = %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`(?s)SELECT .* FROM "connector_platform_lifecycle_events" WHERE subject_type = \$1 AND subject_id = \$2 AND correlation_id = \$3.*LIMIT \$4`).
		WithArgs(event.SubjectType, event.SubjectID, event.CorrelationID, 1).
		WillReturnRows(platformEventRows(t, event))
	replay := event
	replay.ID = uuid.New()
	replay.CreatedAt = replay.CreatedAt.Add(time.Hour)
	if err := repository.CreatePlatformEvent(context.Background(), replay); err != nil {
		t.Fatalf("CreatePlatformEvent(replay) error = %v", err)
	}
	assertAuditMock(t, mock)
}

func TestPlatformLifecycleEventFailsClosedOnConflictingReplayAndRejectsSecrets(t *testing.T) {
	t.Run("conflicting actor", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		event := platformEventFixture()
		existing := event
		existing.Actor = PlatformActor{Kind: PlatformActorSystem}

		mock.ExpectBegin()
		mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectQuery(`(?s)SELECT .* FROM "connector_platform_lifecycle_events" WHERE subject_type = \$1 AND subject_id = \$2 AND correlation_id = \$3.*LIMIT \$4`).
			WithArgs(event.SubjectType, event.SubjectID, event.CorrelationID, 1).
			WillReturnRows(platformEventRows(t, existing))
		if err := repository.CreatePlatformEvent(context.Background(), event); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("CreatePlatformEvent(conflict) error = %v, want conflict", err)
		}
		assertAuditMock(t, mock)
	})

	t.Run("secret detail", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		event := platformEventFixture()
		event.Details["provider_key"] = "platform-event-secret-sentinel"
		if err := repository.CreatePlatformEvent(context.Background(), event); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
			t.Fatalf("CreatePlatformEvent(secret) error = %v, want invalid input", err)
		}
		assertAuditMock(t, mock)
	})

	t.Run("storage failure", func(t *testing.T) {
		repository, mock := openAuditRepositoryMock(t)
		event := platformEventFixture()
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
			WillReturnError(errors.New("forced platform audit failure"))
		mock.ExpectRollback()
		if err := repository.CreatePlatformEvent(context.Background(), event); !core.IsErrorCode(err, core.ErrorCodeInternal) {
			t.Fatalf("CreatePlatformEvent(storage failure) error = %v, want internal", err)
		}
		assertAuditMock(t, mock)
	})
}

func TestPlatformActorContextRequiresKnownActorKind(t *testing.T) {
	ctx := WithPlatformActor(context.Background(), PlatformActor{
		Kind: PlatformActorPlatformMaintainer,
		ID:   uuid.NewString(),
	})
	actor, ok := PlatformActorFromContext(ctx)
	if !ok || actor.Kind != PlatformActorPlatformMaintainer || actor.ID == "" {
		t.Fatalf("PlatformActorFromContext() = %#v, %v", actor, ok)
	}
	if _, ok := PlatformActorFromContext(WithPlatformActor(context.Background(), PlatformActor{Kind: "member"})); ok {
		t.Fatal("unknown platform actor kind was accepted")
	}
	if _, ok := PlatformActorFromContext(WithPlatformActor(context.Background(), PlatformActor{Kind: PlatformActorPlatformMaintainer})); ok {
		t.Fatal("platform maintainer without identity was accepted")
	}
}

func platformEventFixture() PlatformLifecycleEvent {
	return PlatformLifecycleEvent{
		ID: uuid.New(), SubjectType: "connector_plugin", SubjectID: "zgi.github",
		Actor:    PlatformActor{Kind: PlatformActorPlatformMaintainer, ID: uuid.NewString()},
		Decision: "plugin_disabled", Status: core.DecisionStatusSuccessful,
		CorrelationID: "plugin-event-" + uuid.NewString(),
		Details: map[string]any{
			"from_status": "active", "to_status": "disabled", "forced": true,
			"plugin_digest": strings.Repeat("a", 64), "connector_ids": []string{"github"},
		},
		CreatedAt: time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC),
	}
}

func platformEventRows(t *testing.T, event PlatformLifecycleEvent) *sqlmock.Rows {
	t.Helper()
	record, err := newPlatformLifecycleEventRecord(event)
	if err != nil {
		t.Fatal(err)
	}
	return sqlmock.NewRows([]string{
		"id", "subject_type", "subject_id", "actor_kind", "actor_id", "decision",
		"status", "correlation_id", "details", "created_at",
	}).AddRow(
		record.ID, record.SubjectType, record.SubjectID, record.ActorKind, record.ActorID,
		record.Decision, record.Status, record.CorrelationID, record.Details, record.CreatedAt,
	)
}
