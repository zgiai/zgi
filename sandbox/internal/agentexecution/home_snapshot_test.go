package agentexecution

import (
	"context"
	"errors"
	"testing"
)

type fakeHomeSnapshots map[string]HomeSnapshot

func (s fakeHomeSnapshots) HomeSnapshot(_ context.Context, id string) (HomeSnapshot, error) {
	snapshot, ok := s[id]
	if !ok {
		return HomeSnapshot{}, ErrNotFound
	}
	return snapshot, nil
}

type capturedHomeBackend struct {
	fakeBackend
	spec CreateSpec
}

func (b *capturedHomeBackend) CreateBinding(ctx context.Context, spec CreateSpec) (Allocation, error) {
	b.spec = spec
	return b.fakeBackend.CreateBinding(ctx, spec)
}

func TestApprovedHomeSnapshotIsBoundToImmutableArtifact(t *testing.T) {
	repository := newMemoryRepository()
	backend := &capturedHomeBackend{}
	snapshots := fakeHomeSnapshots{"prepared": {
		ID: "prepared", OrganizationID: "org-1", ProfileID: "profile-1",
		Backend: "fake", BackendRef: "approved-artifact-v1", Status: "approved",
	}}
	service, err := NewService(repository, map[string]Backend{"fake": backend}, ServiceOptions{Snapshots: snapshots})
	if err != nil {
		t.Fatal(err)
	}
	request := testCreateRequest()
	request.HomeSnapshotID = "prepared"
	binding, err := service.CreateBinding(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if binding.HomeSnapshotRef != "approved-artifact-v1" || backend.spec.HomeSnapshotRef != binding.HomeSnapshotRef {
		t.Fatalf("binding = %+v; provider spec = %+v", binding, backend.spec)
	}
	snapshots["prepared"] = HomeSnapshot{
		ID: "prepared", OrganizationID: "org-2", ProfileID: "profile-1",
		Backend: "fake", BackendRef: "unowned", Status: "approved",
	}
	continued, err := service.CreateBinding(context.Background(), request)
	if err != nil || continued.ID != binding.ID || continued.HomeSnapshotRef != "approved-artifact-v1" {
		t.Fatalf("pinned existing binding after template revocation = %+v, %v", continued, err)
	}
	request.IdempotencyKey = "new-request"
	if _, err := service.CreateBinding(context.Background(), request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization template accepted: %v", err)
	}
	if stored, err := repository.Binding(context.Background(), binding.ID); err != nil || stored.HomeSnapshotRef != "approved-artifact-v1" {
		t.Fatalf("saved immutable reference = %+v, %v", stored, err)
	}
}
