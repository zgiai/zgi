package agentexecution

import (
	"testing"
	"time"
)

func TestAuditInventoryIdentifiesDriftWithoutActing(t *testing.T) {
	now := time.Now()
	bindings := []Binding{{ID: "active", BackendRef: "a", Status: BindingStatusActive}, {ID: "missing", Status: BindingStatusActive},
		{ID: "paused", BackendRef: "p", Status: BindingStatusSuspended}, {ID: "failed", Status: BindingStatusFailed}, {ID: "deleted", BackendRef: "d", Status: BindingStatusDeleted}}
	resources := []ResourceObservation{{Ref: "a", BindingID: "active", State: "paused"}, {Ref: "p", BindingID: "paused", State: "running"},
		{Ref: "d", BindingID: "deleted", State: "running"}, {Ref: "unknown", BindingID: "other", State: "running"}}
	jobs := []Job{{ID: "job", BindingID: "missing", BackendRef: "old", Status: JobStatusRunning}}
	cleanup := []CleanupTask{{ID: "cleanup", NotBefore: now.Add(-time.Hour)}}
	report := AuditInventory("fake", now, bindings, jobs, cleanup, resources)
	want := map[string]bool{"active_resource_not_running": true, "active_resource_missing": true, "suspended_resource_running": true, "failed_binding_capacity_reserved": true, "resource_after_delete": true, "unattributed_resource": true, "job_claim_expired": true, "job_binding_drift": true, "cleanup_overdue": true}
	for _, finding := range report.Findings {
		delete(want, finding.Code)
	}
	if len(want) != 0 {
		t.Fatalf("missing findings: %+v report=%+v", want, report)
	}
	if bindings[0].Status != BindingStatusActive {
		t.Fatal("audit mutated input")
	}
}
