package agentexecution

import "testing"

func TestPublicJobFailureCodeDoesNotExposeDiagnosticText(t *testing.T) {
	for _, test := range []struct {
		status       JobStatus
		reason, want string
	}{
		{JobStatusSucceeded, "private token", ""},
		{JobStatusFailed, "private token https://user:password@host", "job_failed"},
		{JobStatusFailed, "language_package_recovery_failed private token", "job_failed"},
		{JobStatusFailed, "agent runtime backend unavailable: retained Python or Node dependency recovery failed; verify the original runtime profile and retained artifact integrity before retrying; no network or latest-version fallback", "language_package_recovery_failed"},
		{JobStatusFailed, "agent runtime backend unavailable: system package recovery failed; verify the retained ledger, original profile and sources, and locked package availability before retrying; no latest-version fallback", "package_recovery_failed"},
	} {
		if got := jobResponseFrom(Job{Status: test.status, FailureReason: test.reason}).FailureCode; got != test.want {
			t.Fatalf("failure code = %q, want %q", got, test.want)
		}
	}
}

func TestPublicJobFailureCodePreservesCommandExitFailure(t *testing.T) {
	exitCode := 2
	response := jobResponseFrom(Job{Status: JobStatusFailed, ExitCode: &exitCode, FailureReason: "command exited with status 2"})
	if response.FailureCode != "" || response.ExitCode == nil || *response.ExitCode != exitCode {
		t.Fatalf("ordinary command failure changed: %+v", response)
	}
}
