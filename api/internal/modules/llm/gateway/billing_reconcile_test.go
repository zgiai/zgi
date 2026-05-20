package gateway

import (
	"testing"
	"time"
)

func TestReconcileBackoff(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "zero attempts uses base", attempts: 0, want: defaultReconcileBaseBackoff},
		{name: "first retry uses base", attempts: 1, want: defaultReconcileBaseBackoff},
		{name: "second retry doubles", attempts: 2, want: 2 * defaultReconcileBaseBackoff},
		{name: "third retry doubles again", attempts: 3, want: 4 * defaultReconcileBaseBackoff},
		{name: "cap at max backoff", attempts: 16, want: defaultReconcileMaxBackoff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconcileBackoff(tt.attempts)
			if got != tt.want {
				t.Fatalf("reconcileBackoff(%d)=%s, want %s", tt.attempts, got, tt.want)
			}
		})
	}
}

func TestInvocationResultFromBillingStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "success status", status: "success", want: "success"},
		{name: "error status", status: "error", want: "error"},
		{name: "upper error status", status: "ERROR", want: "error"},
		{name: "empty defaults to success", status: "", want: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationResultFromBillingStatus(tt.status)
			if got != tt.want {
				t.Fatalf("invocationResultFromBillingStatus(%q)=%q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
