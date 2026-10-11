package storage

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func endUserBinding(id, user, application, backend string) runtime.Binding {
	binding := admissionBinding(id, "organization")
	binding.SubjectID, binding.AgentID, binding.Backend = user, application, backend
	binding.Identity = &runtime.ExecutionIdentity{Version: 1, WorkspaceID: "resource-workspace", ApplicationType: "agent",
		ApplicationID: application, Entry: "webapp", SubjectKind: "account", ConversationID: id, PublicationRef: "published-v1"}
	return binding
}

func TestEndUserBindingAdmissionIsAtomicAcrossBackendsAndEntries(t *testing.T) {
	_, stores := admissionStores(t, runtime.AdmissionLimits{Bindings: 40, BindingsPerOrg: 40, Jobs: 40, JobsPerOrg: 40, JobsPerBinding: 4, JobTimeoutSeconds: 60})
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for index := range 20 {
		wg.Go(func() {
			binding := endUserBinding(fmt.Sprintf("binding-%d", index), "user-a", "application", fmt.Sprintf("backend-%d", index%2))
			if index%2 == 0 {
				binding.Identity.Entry, binding.Identity.PublicationRef = "console", "published-v2"
			}
			_, created, err := stores[index%2].ReserveBinding(t.Context(), binding)
			if err == nil && created {
				admitted.Add(1)
			} else if !errors.Is(err, runtime.ErrCapacityExceeded) {
				t.Errorf("reserve = %v, created=%v", err, created)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 2 {
		t.Fatalf("default end-user binding cap = %d, want 2", admitted.Load())
	}
	for _, binding := range []runtime.Binding{
		endUserBinding("other-user", "user-b", "application", "backend-0"),
		endUserBinding("other-application", "user-a", "other-application", "backend-0"),
	} {
		if _, _, err := stores[0].ReserveBinding(t.Context(), binding); err != nil {
			t.Fatalf("independent subject/application: %v", err)
		}
	}
	var id string
	if err := stores[0].db.QueryRow(`SELECT id FROM agent_runtime_bindings WHERE subject_id='user-a' AND agent_id='application' LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	binding, err := stores[0].Binding(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := stores[1].ReserveBinding(t.Context(), binding); err != nil || created {
		t.Fatalf("idempotent request at capacity = %v, %v", created, err)
	}
	binding.Status, binding.Generation = runtime.BindingStatusSuspended, binding.Generation+1
	if err := stores[0].UpdateBinding(t.Context(), binding, binding.Generation-1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stores[1].ReserveBinding(t.Context(), endUserBinding("replacement", "user-a", "application", "backend-2")); err != nil {
		t.Fatal(err)
	}
	binding.Status, binding.Generation = runtime.BindingStatusResuming, binding.Generation+1
	if err := stores[0].UpdateBinding(t.Context(), binding, binding.Generation-1); !errors.Is(err, runtime.ErrCapacityExceeded) {
		t.Fatalf("resume bypassed user cap: %v", err)
	}
}

func TestEndUserJobAdmissionReservesQueuedWorkAcrossBindings(t *testing.T) {
	_, stores := admissionStores(t, runtime.AdmissionLimits{Bindings: 8, BindingsPerOrg: 8, Jobs: 8, JobsPerOrg: 8, JobsPerBinding: 4, JobTimeoutSeconds: 60})
	bindings := []runtime.Binding{endUserBinding("one", "user-a", "application", "fake"), endUserBinding("two", "user-a", "application", "other-backend"), endUserBinding("three", "user-b", "application", "fake")}
	for index := range bindings {
		binding := &bindings[index]
		if _, _, err := stores[0].ReserveBinding(t.Context(), *binding); err != nil {
			t.Fatal(err)
		}
		binding.Status, binding.Generation = runtime.BindingStatusActive, 2
		if err := stores[0].UpdateBinding(t.Context(), *binding, 1); err != nil {
			t.Fatal(err)
		}
	}
	jobFor := func(binding runtime.Binding, id string) runtime.Job {
		job := testAgentExecutionJob(time.Now(), id, id)
		job.BindingID, job.OrganizationID, job.SubjectID, job.Backend, job.BindingGeneration = binding.ID, binding.OrganizationID, binding.SubjectID, binding.Backend, binding.Generation
		return job
	}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for index := range 16 {
		wg.Go(func() {
			_, created, err := stores[index%2].CreateJob(t.Context(), jobFor(bindings[index%2], fmt.Sprintf("job-%d", index)))
			if err == nil && created {
				admitted.Add(1)
			} else if !errors.Is(err, runtime.ErrCapacityExceeded) {
				t.Errorf("submit = %v", err)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("default end-user Job cap = %d, want 1", admitted.Load())
	}
	if _, _, err := stores[1].CreateJob(t.Context(), jobFor(bindings[2], "other-user-job")); err != nil {
		t.Fatalf("other user blocked: %v", err)
	}
	if _, err := stores[0].db.Exec(`UPDATE agent_runtime_jobs SET status='succeeded' WHERE subject_id='user-a'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stores[0].CreateJob(t.Context(), jobFor(bindings[0], "next-job")); err != nil {
		t.Fatalf("terminal Job retained capacity: %v", err)
	}
}
