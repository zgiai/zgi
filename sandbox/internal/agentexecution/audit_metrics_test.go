package agentexecution

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAuditMetricsExcludeResourceIdentity(t *testing.T) {
	var output bytes.Buffer
	report := RuntimeAudit{Backend: "e2b", ObservedAt: time.Unix(42, 0), BindingCount: 3, Findings: []AuditFinding{{Code: "cleanup_overdue", BindingID: "private-binding", ResourceID: "private-resource"}, {Code: "untrusted-secret"}}}
	if err := WriteAuditMetrics(&output, report); err != nil {
		t.Fatal(err)
	}
	value := output.String()
	if !strings.Contains(value, `zgi_runtime_audit_findings{backend="e2b",code="cleanup_overdue"} 1`) || strings.Contains(value, "private-") || strings.Contains(value, "untrusted-secret") {
		t.Fatalf("unsafe metrics: %s", value)
	}
	if err := WriteAuditMetrics(failingMetricWriter{}, report); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error=%v", err)
	}
}

type failingMetricWriter struct{}

func (failingMetricWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
