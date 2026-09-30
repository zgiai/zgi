package agentexecution

import (
	"context"
	"testing"
	"time"
)

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
