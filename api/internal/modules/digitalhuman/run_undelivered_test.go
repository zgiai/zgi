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

type archivingRunDispatcher struct {
	*fakeRunDispatcher
	archived  map[uuid.UUID]bool
	inspected []uuid.UUID
}

func (d *archivingRunDispatcher) RunDeliveryArchived(_ context.Context, runID uuid.UUID) (bool, error) {
	d.inspected = append(d.inspected, runID)
	return d.archived[runID], nil
}

func (s *fakeRunStore) ListQueuedRuns(_ context.Context, updatedBefore time.Time, limit int) ([]*Run, error) {
	runs := make([]*Run, 0, limit)
	for _, run := range s.runs {
		if run.Status == RunStatusQueued && run.UpdatedAt.Before(updatedBefore) && len(runs) < limit {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func (s *fakeRunStore) FailQueuedRun(_ context.Context, runID uuid.UUID, code, message string, now time.Time) (bool, error) {
	run := s.runs[runID]
	if run == nil || run.Status != RunStatusQueued {
		return false, nil
	}
	run.Status, run.ErrorCode, run.ErrorMessage, run.CompletedAt, run.UpdatedAt = RunStatusFailed, code, message, &now, now
	return true, nil
}

func TestRuntimeServiceFailsQueuedRunWhoseDeliveryWasArchived(t *testing.T) {
	store := newFakeRunStore()
	now := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	service := newTestRuntimeService(t, store, &fakeRunDispatcher{}, &fakeDigitalHumanChat{})
	service.now = func() time.Time { return now }
	old := now.Add(-undeliveredRunWindow - time.Minute)
	archived := &Run{ID: uuid.New(), Status: RunStatusQueued, UpdatedAt: old}
	pending := &Run{ID: uuid.New(), Status: RunStatusQueued, UpdatedAt: old}
	recent := &Run{ID: uuid.New(), Status: RunStatusQueued, UpdatedAt: now.Add(-time.Second)}
	started := &Run{ID: uuid.New(), Status: RunStatusRunning, UpdatedAt: now.Add(-time.Second)}
	for _, run := range []*Run{archived, pending, recent, started} {
		store.runs[run.ID] = run
	}
	dispatcher := &archivingRunDispatcher{fakeRunDispatcher: &fakeRunDispatcher{}, archived: map[uuid.UUID]bool{
		archived.ID: true, recent.ID: true, started.ID: true,
	}}
	service.dispatcher = dispatcher

	for attempt := 0; attempt < 2; attempt++ {
		if err := service.Reconcile(t.Context()); err != nil {
			t.Fatalf("Reconcile() attempt %d error = %v", attempt, err)
		}
	}

	if archived.Status != RunStatusFailed || archived.ErrorCode != runQueueUnavailableCode || archived.ErrorMessage == "" || archived.CompletedAt == nil {
		t.Fatalf("archived run = %s %q %q, want failed with %s", archived.Status, archived.ErrorCode, archived.ErrorMessage, runQueueUnavailableCode)
	}
	if pending.Status != RunStatusQueued {
		t.Fatalf("run with a live delivery = %s, want queued", pending.Status)
	}
	if recent.Status != RunStatusQueued {
		t.Fatalf("run inside the delivery window = %s, want queued", recent.Status)
	}
	if started.Status != RunStatusRunning {
		t.Fatalf("claimed run = %s, want running", started.Status)
	}
	for _, id := range dispatcher.inspected {
		if id == recent.ID || id == started.ID {
			t.Fatalf("inspected delivery of run %s outside the queued window", id)
		}
	}
}

func TestGormRepositoryFailQueuedRunKeepsClaimedRuns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	now := time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)
	queued := storedRun(RunStatusQueued, now.Add(-time.Hour))
	fresh := storedRun(RunStatusQueued, now)
	running := storedRun(RunStatusRunning, now.Add(-time.Hour))
	for _, run := range []*Run{queued, fresh, running} {
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
	}

	listed, err := repo.ListQueuedRuns(t.Context(), now.Add(-time.Minute), 10)
	if err != nil || len(listed) != 1 || listed[0].ID != queued.ID {
		t.Fatalf("ListQueuedRuns() = %v, %v, want only the old queued run", listed, err)
	}
	failed, err := repo.FailQueuedRun(t.Context(), queued.ID, runQueueUnavailableCode, runQueueUnavailableMessage, now)
	if err != nil || !failed {
		t.Fatalf("FailQueuedRun(queued) = %v, %v", failed, err)
	}
	failed, err = repo.FailQueuedRun(t.Context(), queued.ID, runQueueUnavailableCode, runQueueUnavailableMessage, now)
	if err != nil || failed {
		t.Fatalf("FailQueuedRun(already failed) = %v, %v, want no change", failed, err)
	}
	failed, err = repo.FailQueuedRun(t.Context(), running.ID, runQueueUnavailableCode, runQueueUnavailableMessage, now)
	if err != nil || failed {
		t.Fatalf("FailQueuedRun(running) = %v, %v, want no change", failed, err)
	}

	var stored Run
	if err := db.Where("id = ?", queued.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != RunStatusFailed || stored.ErrorCode != runQueueUnavailableCode || stored.CompletedAt == nil {
		t.Fatalf("stored run = %s %q, want failed with %s", stored.Status, stored.ErrorCode, runQueueUnavailableCode)
	}
	var claimed Run
	if err := db.Where("id = ?", running.ID).Take(&claimed).Error; err != nil {
		t.Fatal(err)
	}
	if claimed.Status != RunStatusRunning {
		t.Fatalf("running run = %s, want running", claimed.Status)
	}
}

func storedRun(status RunStatus, updatedAt time.Time) *Run {
	empty := datatypes.JSON(`{}`)
	id := uuid.New()
	return &Run{
		ID: id, IdempotencyKey: id.String(), Status: status, CreatedAt: updatedAt, UpdatedAt: updatedAt,
		InvocationEvent: empty, RunSpec: empty, ExecutionSnapshot: empty, ExecutionTrace: empty,
	}
}
