package agentexecution

import (
	"bufio"
	"fmt"
	"io"
)

// WriteAuditMetrics emits bounded Prometheus text, without tenant/resource IDs or
// raw error messages. Callers publish output only after a complete audit succeeds.
func WriteAuditMetrics(target io.Writer, report RuntimeAudit) error {
	w := bufio.NewWriter(target)
	for _, metric := range []struct {
		name  string
		value int64
	}{
		{"zgi_runtime_audit_timestamp_seconds", report.ObservedAt.Unix()},
		{"zgi_runtime_bindings", int64(report.BindingCount)},
		{"zgi_runtime_active_jobs", int64(report.ActiveJobCount)},
		{"zgi_runtime_pending_cleanup", int64(report.PendingCleanup)},
		{"zgi_runtime_compute_resources", int64(report.ResourceCount)},
	} {
		if _, err := fmt.Fprintf(w, "# TYPE %s gauge\n%s{backend=%q} %d\n", metric.name, metric.name, report.Backend, metric.value); err != nil {
			return err
		}
	}
	counts := map[string]int{}
	for _, finding := range report.Findings {
		counts[finding.Code]++
	}
	for _, code := range []string{"unattributed_resource", "unexpected_resource_ref", "resource_after_delete", "suspended_resource_running", "active_resource_not_running", "active_resource_missing", "failed_binding_capacity_reserved", "job_binding_drift", "job_claim_expired", "cleanup_overdue"} {
		if _, err := fmt.Fprintf(w, "zgi_runtime_audit_findings{backend=%q,code=%q} %d\n", report.Backend, code, counts[code]); err != nil {
			return err
		}
	}
	return w.Flush()
}
