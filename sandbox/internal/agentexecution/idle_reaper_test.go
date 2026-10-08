package agentexecution

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestIdleReaperCancelsStalledSuspension(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repository := newMemoryRepository()
		binding := activeTestBinding()
		binding.ExpiresAt = time.Now().Add(-time.Minute)
		repository.bindings[binding.ID] = binding
		reaper, err := NewIdleReaper(repository, stalledSuspender{}, IdleReaperOptions{})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		processed, err := reaper.RunOne(t.Context())
		if !processed || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != idleSuspendTimeout {
			t.Fatalf("stalled attempt = %v, %v after %s", processed, err, time.Since(start))
		}
	})
}

type stalledSuspender struct{}

func (stalledSuspender) SuspendBinding(ctx context.Context, _ Scope, _ string, _ int64) (Binding, error) {
	<-ctx.Done()
	return Binding{}, ctx.Err()
}

func TestIdleReaperBackoffSurvivesRestartWithoutStarvingOtherBindings(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	failed := activeTestBinding()
	failed.Status, failed.UpdatedAt, failed.ExpiresAt = BindingStatusSuspending, now, now.Add(-time.Hour)
	repository.bindings[failed.ID] = failed
	healthy := failed
	healthy.ID, healthy.Status, healthy.ExpiresAt = "healthy", BindingStatusActive, now.Add(-time.Minute)
	repository.bindings[healthy.ID] = healthy
	suspender := &reaperSuspender{failID: failed.ID}
	for range 2 { // A new reaper has no in-memory retry state.
		reaper, err := NewIdleReaper(repository, suspender, IdleReaperOptions{Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		processed, err := reaper.RunOne(t.Context())
		if err != nil || !processed || suspender.lastID != healthy.ID {
			t.Fatalf("healthy binding starved: processed=%v, id=%s, err=%v", processed, suspender.lastID, err)
		}
	}
	repository.mu.Lock()
	delete(repository.bindings, healthy.ID)
	repository.mu.Unlock()
	reaper, err := NewIdleReaper(repository, suspender, IdleReaperOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(IdleSuspendRetryDelay - time.Nanosecond)
	if processed, err := reaper.RunOne(t.Context()); processed || err != nil {
		t.Fatalf("early retry = %v, %v", processed, err)
	}
	now = now.Add(time.Nanosecond)
	if processed, err := reaper.RunOne(t.Context()); !processed || !errors.Is(err, ErrBackendUnavailable) || suspender.lastID != failed.ID {
		t.Fatalf("due retry = %v, %v, id=%s", processed, err, suspender.lastID)
	}
}

type reaperSuspender struct{ failID, lastID string }

func (s *reaperSuspender) SuspendBinding(ctx context.Context, _ Scope, id string, _ int64) (Binding, error) {
	s.lastID = id
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > idleSuspendTimeout {
		return Binding{}, errors.New("idle suspension has no bounded deadline")
	}
	if id == s.failID {
		return Binding{}, ErrBackendUnavailable
	}
	return Binding{}, nil
}

func TestIdleReaperSuspendsExpiredBinding(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	backend := &fakeBackend{}
	service := newTestService(t, repository, backend)
	binding, err := service.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}
	repository.mu.Lock()
	binding.ExpiresAt = now.Add(-time.Second)
	repository.bindings[binding.ID] = binding
	repository.mu.Unlock()
	reaper, err := NewIdleReaper(repository, service, IdleReaperOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new reaper: %v", err)
	}
	processed, err := reaper.RunOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("reap: processed=%v error=%v", processed, err)
	}
	stored, err := repository.Binding(context.Background(), binding.ID)
	if err != nil || stored.Status != BindingStatusSuspended || backend.suspendCalls != 1 {
		t.Fatalf("binding=%+v suspend calls=%d error=%v", stored, backend.suspendCalls, err)
	}
}

func TestBindingActivityFencesStaleIdleReap(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	binding := activeTestBinding()
	binding.ExpiresAt = now.Add(-time.Second)
	repository.bindings[binding.ID] = binding
	expired, err := repository.ExpiredBindings(context.Background(), now, 1)
	if err != nil || len(expired) != 1 {
		t.Fatalf("expired bindings=%+v error=%v", expired, err)
	}
	if _, err := repository.TouchBinding(context.Background(), binding.ID, binding.Generation, now, now.Add(defaultIdleTTL), now.Add(defaultRetentionTTL)); err != nil {
		t.Fatalf("touch binding: %v", err)
	}
	service := newTestService(t, repository, &fakeBackend{})
	if _, err := service.SuspendBinding(context.Background(), Scope{OrganizationID: binding.OrganizationID, SubjectID: binding.SubjectID}, binding.ID, expired[0].Generation); err != ErrGenerationConflict {
		t.Fatalf("stale suspend error = %v, want generation conflict", err)
	}
}

func TestIdleReaperDoesNothingBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	binding := activeTestBinding()
	binding.ExpiresAt = now.Add(time.Second)
	repository.bindings[binding.ID] = binding
	service := newTestService(t, repository, &fakeBackend{})
	reaper, err := NewIdleReaper(repository, service, IdleReaperOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new reaper: %v", err)
	}
	if processed, err := reaper.RunOne(context.Background()); err != nil || processed {
		t.Fatalf("reap: processed=%v error=%v", processed, err)
	}
}
