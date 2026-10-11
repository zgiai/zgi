package agentexecution

import (
	"sort"
	"time"
)

// ResourceObservation contains identifiers only; never connection tokens or
// provider metadata maps. It is evidence, not authorization to delete anything.
type ResourceObservation struct {
	Ref       string `json:"ref"`
	BindingID string `json:"binding_id"`
	State     string `json:"state"`
}

type AuditFinding struct {
	Code       string `json:"code"`
	BindingID  string `json:"binding_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
}

type RuntimeAudit struct {
	Backend        string         `json:"backend"`
	ObservedAt     time.Time      `json:"observed_at"`
	BindingCount   int            `json:"binding_count"`
	ActiveJobCount int            `json:"active_job_count"`
	PendingCleanup int            `json:"pending_cleanup"`
	ResourceCount  int            `json:"resource_count"`
	Findings       []AuditFinding `json:"findings"`
}

// AuditInventory compares bounded, complete observations. Control-plane and
// provider snapshots are not atomic; confirm drift on a second observation.
func AuditInventory(backend string, now time.Time, bindings []Binding, jobs []Job, cleanup []CleanupTask, resources []ResourceObservation) RuntimeAudit {
	report := RuntimeAudit{Backend: backend, ObservedAt: now.UTC(), BindingCount: len(bindings), ActiveJobCount: len(jobs), PendingCleanup: len(cleanup), ResourceCount: len(resources), Findings: []AuditFinding{}}
	byID := make(map[string]Binding, len(bindings))
	byRef := make(map[string]ResourceObservation, len(resources))
	for _, binding := range bindings {
		byID[binding.ID] = binding
	}
	for _, resource := range resources {
		byRef[resource.Ref] = resource
		binding, known := byID[resource.BindingID]
		code := ""
		switch {
		case !known:
			// Could belong to another deployment sharing this provider account.
			code = "unattributed_resource"
		case binding.Status == BindingStatusAllocating || binding.Status == BindingStatusResuming:
			continue // An in-progress allocation has not published its ref yet.
		case binding.BackendRef != resource.Ref:
			code = "unexpected_resource_ref"
		case binding.Status == BindingStatusDeleted:
			code = "resource_after_delete"
		case binding.Status == BindingStatusSuspended && resource.State == "running":
			code = "suspended_resource_running"
		case binding.Status == BindingStatusActive && resource.State != "running":
			code = "active_resource_not_running"
		}
		if code != "" {
			report.Findings = append(report.Findings, AuditFinding{Code: code, BindingID: resource.BindingID, ResourceID: resource.Ref})
		}
	}
	for _, binding := range bindings {
		if binding.Status == BindingStatusActive {
			resource, found := byRef[binding.BackendRef]
			if !found || resource.BindingID != binding.ID {
				report.Findings = append(report.Findings, AuditFinding{Code: "active_resource_missing", BindingID: binding.ID})
			}
		}
		if binding.Status == BindingStatusFailed {
			report.Findings = append(report.Findings, AuditFinding{Code: "failed_binding_capacity_reserved", BindingID: binding.ID})
		}
	}
	for _, job := range jobs {
		binding, exists := byID[job.BindingID]
		if !exists || binding.Status != BindingStatusActive || binding.BackendRef != job.BackendRef {
			report.Findings = append(report.Findings, AuditFinding{Code: "job_binding_drift", BindingID: job.BindingID, ResourceID: job.ID})
		}
		if job.Status == JobStatusRunning && (job.ClaimExpiresAt == nil || !job.ClaimExpiresAt.After(now)) {
			report.Findings = append(report.Findings, AuditFinding{Code: "job_claim_expired", BindingID: job.BindingID, ResourceID: job.ID})
		}
	}
	for _, task := range cleanup {
		if task.NotBefore.Before(now.Add(-5*time.Minute)) && (task.LeaseExpiresAt == nil || !task.LeaseExpiresAt.After(now)) {
			report.Findings = append(report.Findings, AuditFinding{Code: "cleanup_overdue", BindingID: task.BindingID, ResourceID: task.ID})
		}
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.BindingID != b.BindingID {
			return a.BindingID < b.BindingID
		}
		return a.ResourceID < b.ResourceID
	})
	return report
}
