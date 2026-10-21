package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func admissionStores(t *testing.T, limits runtime.AdmissionLimits) (string, []*Store) {
	t.Helper()
	dsn := testutil.CreateTestPostgresDSN(t)
	var stores []*Store
	for range 2 {
		store, err := Open(config.Config{DatabaseURL: dsn, MaxWorkers: 2, AgentRuntimeAdmission: &limits})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		stores = append(stores, store)
	}
	return dsn, stores
}

func admissionBinding(id, org string) runtime.Binding {
	now := time.Now().UTC()
	return runtime.Binding{ID: id, OrganizationID: org, SubjectID: "subject", AgentID: "agent", ConversationID: id,
		ProfileID: "standard", WorkspaceID: id, Backend: "fake", IdempotencyKey: id, RequestHash: id,
		Status: runtime.BindingStatusAllocating, Generation: 1, CreatedAt: now, UpdatedAt: now,
		LastActivityAt: now, ExpiresAt: now.Add(time.Hour), RetentionExpiresAt: now.Add(7 * 24 * time.Hour)}
}

func TestAgentAdmissionAtomicBindingsAcrossStoresAndResume(t *testing.T) {
	_, stores := admissionStores(t, runtime.AdmissionLimits{Bindings: 2, BindingsPerOrg: 1, Jobs: 2, JobsPerOrg: 1, JobsPerBinding: 1, JobTimeoutSeconds: 60})
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for index := range 20 {
		wg.Go(func() {
			binding := admissionBinding(fmt.Sprintf("binding-%d", index), fmt.Sprintf("org-%d", index%2))
			_, created, err := stores[index%2].ReserveBinding(t.Context(), binding)
			if err == nil && created {
				admitted.Add(1)
			} else if !errors.Is(err, runtime.ErrCapacityExceeded) {
				t.Errorf("reserve: %v created=%v", err, created)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 2 {
		t.Fatalf("admitted %d, want exactly 2", admitted.Load())
	}
	var id string
	if err := stores[0].db.QueryRow(`SELECT id FROM agent_runtime_bindings ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	original, err := stores[0].Binding(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, created, err := stores[1].ReserveBinding(t.Context(), original); err != nil || created || duplicate.ID != id {
		t.Fatalf("idempotent retry at capacity: %v %v", created, err)
	}
	original.Status = runtime.BindingStatusSuspended
	original.Generation++
	if err := stores[0].UpdateBinding(t.Context(), original, original.Generation-1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stores[1].ReserveBinding(t.Context(), admissionBinding("replacement", "org-new")); err != nil {
		t.Fatal(err)
	}
	original.Status = runtime.BindingStatusResuming
	original.Generation++
	if err := stores[0].UpdateBinding(t.Context(), original, original.Generation-1); !errors.Is(err, runtime.ErrCapacityExceeded) {
		t.Fatalf("resume over capacity: %v", err)
	}
	retained, err := stores[0].Binding(t.Context(), id)
	if err != nil || retained.Status != runtime.BindingStatusSuspended {
		t.Fatalf("retained binding changed: %+v %v", retained, err)
	}
}

func TestAgentAdmissionJobsPolicyAndReadOnlyAudit(t *testing.T) {
	limits := runtime.AdmissionLimits{Bindings: 4, BindingsPerOrg: 2, Jobs: 2, JobsPerOrg: 1, JobsPerBinding: 1, JobTimeoutSeconds: 60}
	dsn, stores := admissionStores(t, limits)
	bindings := []runtime.Binding{admissionBinding("one", "org-a"), admissionBinding("two", "org-a"), admissionBinding("three", "org-b"), admissionBinding("four", "org-c")}
	for index := range bindings {
		b := &bindings[index]
		if _, _, err := stores[0].ReserveBinding(t.Context(), *b); err != nil {
			t.Fatal(err)
		}
		b.Status, b.Generation, b.BackendRef = runtime.BindingStatusActive, 2, b.ID
		if err := stores[0].UpdateBinding(t.Context(), *b, 1); err != nil {
			t.Fatal(err)
		}
	}
	jobFor := func(index int, id string) runtime.Job {
		b := bindings[index]
		job := testAgentExecutionJob(time.Now(), id, id)
		job.BindingID, job.OrganizationID, job.SubjectID, job.BackendRef = b.ID, b.OrganizationID, b.SubjectID, b.BackendRef
		return job
	}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for index := range 16 {
		wg.Go(func() {
			_, created, err := stores[index%2].CreateJob(t.Context(), jobFor(0, fmt.Sprintf("job-%d", index)))
			if err == nil && created {
				admitted.Add(1)
			} else if !errors.Is(err, runtime.ErrCapacityExceeded) {
				t.Errorf("job admission: %v", err)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("per-binding jobs = %d", admitted.Load())
	}
	if _, _, err := stores[0].CreateJob(t.Context(), jobFor(1, "same-org")); !errors.Is(err, runtime.ErrCapacityExceeded) {
		t.Fatalf("per-org quota: %v", err)
	}
	if _, _, err := stores[1].CreateJob(t.Context(), jobFor(2, "other-org")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stores[0].CreateJob(t.Context(), jobFor(3, "backend-full")); !errors.Is(err, runtime.ErrCapacityExceeded) {
		t.Fatalf("backend quota: %v", err)
	}
	if _, created, err := stores[0].CreateJob(t.Context(), jobFor(2, "other-org")); err != nil || created {
		t.Fatalf("duplicate at capacity: %v %v", created, err)
	}
	invalid := jobFor(3, "unbounded")
	invalid.Timeout = 0
	if _, _, err := stores[0].CreateJob(t.Context(), invalid); !errors.Is(err, runtime.ErrInvalidRequest) {
		t.Fatalf("unbounded Job: %v", err)
	}
	changed := limits
	changed.Jobs = 3
	if err := ReplaceAdmissionPolicy(t.Context(), dsn, "fake", changed); !errors.Is(err, runtime.ErrInvalidTransition) {
		t.Fatalf("changed policy on live backend: %v", err)
	}
	mismatched, err := Open(config.Config{DatabaseURL: dsn, AgentRuntimeAdmission: &changed})
	if err != nil {
		t.Fatal(err)
	}
	defer mismatched.Close()
	if _, _, err := mismatched.ReserveBinding(t.Context(), admissionBinding("mismatch", "org-d")); !errors.Is(err, runtime.ErrAdmissionPolicy) {
		t.Fatalf("mismatched worker admitted: %v", err)
	}
	if _, err := mismatched.Binding(t.Context(), bindings[0].ID); err != nil {
		t.Fatalf("read blocked by policy: %v", err)
	}
	// Audit skips migrations, leases and provider wake-up; resource metadata is
	// deliberately identifiers only. Snapshot drift is a finding, not a delete.
	report, err := ReadRuntimeAudit(t.Context(), dsn, "fake", []runtime.ResourceObservation{{Ref: "one", BindingID: "one", State: "running"}, {Ref: "unattributed", BindingID: "other-deployment", State: "paused"}})
	if err != nil || report.ActiveJobCount != 2 || report.ResourceCount != 2 {
		t.Fatalf("audit: %+v %v", report, err)
	}
	var found bool
	for _, finding := range report.Findings {
		if finding.Code == "unattributed_resource" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing unattributed resource finding")
	}
	// A mismatched process may still finish cleanup transitions.
	for _, b := range bindings {
		b.Status, b.Generation = runtime.BindingStatusDeleted, 3
		if err := mismatched.UpdateBinding(t.Context(), b, 2); err != nil {
			t.Fatalf("cleanup blocked: %v", err)
		}
	}
	if _, err := stores[0].db.ExecContext(context.Background(), `UPDATE agent_runtime_jobs SET status = 'canceled'`); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceAdmissionPolicy(t.Context(), dsn, "fake", changed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stores[0].ReserveBinding(t.Context(), admissionBinding("stale", "org-d")); !errors.Is(err, runtime.ErrAdmissionPolicy) {
		t.Fatalf("stale worker admitted: %v", err)
	}
	if _, _, err := mismatched.ReserveBinding(t.Context(), admissionBinding("fresh", "org-d")); err != nil {
		t.Fatal(err)
	}
}
