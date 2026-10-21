package agentexecution

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetentionReaperDeletesOnlyAfterSevenDays(t *testing.T) {
	started := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	now := started
	repository := newMemoryRepository()
	backend := &checkpointBackend{content: []byte("retained result")}
	checkpoints := &memoryCheckpointStore{data: make(map[string][]byte)}
	control, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{
		Now: func() time.Time { return now }, Checkpoints: checkpoints, Jobs: &testActiveJobs{},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := control.CreateBinding(context.Background(), testCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !binding.RetentionExpiresAt.Equal(started.Add(defaultRetentionTTL)) || !binding.ExpiresAt.Equal(started.Add(defaultIdleTTL)) {
		t.Fatalf("idle and retention deadlines = %v, %v", binding.ExpiresAt, binding.RetentionExpiresAt)
	}
	now = started.Add(defaultIdleTTL)
	suspended, err := control.SuspendBinding(context.Background(), testCreateRequest().Scope, binding.ID, binding.Generation)
	if err != nil || suspended.CheckpointKey == "" {
		t.Fatalf("suspend = %+v, %v", suspended, err)
	}
	reaper, err := NewRetentionReaper(repository, control, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	now = started.Add(defaultRetentionTTL - time.Second)
	if processed, err := reaper.RunOne(context.Background()); err != nil || processed || len(checkpoints.data) != 1 {
		t.Fatalf("premature retention cleanup = %v, %v, objects=%d", processed, err, len(checkpoints.data))
	}
	now = started.Add(defaultRetentionTTL)
	if _, err := control.ResumeBinding(context.Background(), testCreateRequest().Scope, binding.ID, suspended.Generation); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expired resume = %v", err)
	}
	if processed, err := reaper.RunOne(context.Background()); err != nil || !processed {
		t.Fatalf("retention cleanup = %v, %v", processed, err)
	}
	current, err := control.GetBinding(context.Background(), testCreateRequest().Scope, binding.ID)
	if err != nil || current.Status != BindingStatusDeleted || len(checkpoints.data) != 0 {
		t.Fatalf("binding = %+v, %v, objects=%d", current, err, len(checkpoints.data))
	}
}

func TestBindingActivityExtendsRetentionAndFencesStaleCleanup(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	binding := activeTestBinding()
	binding.RetentionExpiresAt = now.Add(-time.Second)
	binding.Status = BindingStatusSuspended
	repository.bindings[binding.ID] = binding
	expired, err := repository.ExpiredRetainedBindings(context.Background(), now, 1)
	if err != nil || len(expired) != 1 {
		t.Fatalf("expired retention = %+v, %v", expired, err)
	}
	binding.Status = BindingStatusActive
	binding.Generation++
	binding.RetentionExpiresAt = now.Add(defaultRetentionTTL)
	repository.bindings[binding.ID] = binding
	control := newTestService(t, repository, &fakeBackend{})
	if _, err := control.DestroyBinding(context.Background(), Scope{OrganizationID: binding.OrganizationID, SubjectID: binding.SubjectID}, binding.ID, expired[0].Generation); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("stale cleanup = %v", err)
	}
}
