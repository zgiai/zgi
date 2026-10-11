package connectorfinalize

import (
	"errors"
	"strings"
	"testing"
)

// Apply must freeze the applied migration ledger with one closed statement
// before any SELECT. The relation name is fixed in the constant and no caller
// value is ever interpolated into it.
func TestMigrationLedgerLockIsClosedAndFixed(t *testing.T) {
	const want = `LOCK TABLE public.migrations IN SHARE ROW EXCLUSIVE MODE NOWAIT`
	if migrationLedgerLock != want {
		t.Fatalf("migration ledger lock = %q, want %q", migrationLedgerLock, want)
	}
	if strings.Count(migrationLedgerLock, "public.migrations") != 1 {
		t.Fatalf("ledger lock must name exactly public.migrations: %q", migrationLedgerLock)
	}
	if strings.Contains(migrationLedgerLock, "%") {
		t.Fatalf("ledger lock must not be a format string: %q", migrationLedgerLock)
	}
	if !strings.HasPrefix(migrationLedgerLock, "LOCK TABLE ") {
		t.Fatalf("ledger lock must be a LOCK TABLE statement: %q", migrationLedgerLock)
	}
	if !strings.Contains(migrationLedgerLock, "IN SHARE ROW EXCLUSIVE MODE") || !strings.HasSuffix(migrationLedgerLock, " NOWAIT") {
		t.Fatalf("ledger lock must fail closed with NOWAIT: %q", migrationLedgerLock)
	}
}

// Only the explicit AdvancedCheckpoint opt-in freezes the applied migration
// ledger before readiness is read. Both default paths (schema1 conversion and
// schema2 repeat) pass an empty option, so they never take this lock and keep
// their original read/lock order.
func TestRequiresAdvancedLedgerLockOnlyForExplicitOptIn(t *testing.T) {
	if requiresAdvancedLedgerLock(Options{}) {
		t.Fatal("default Apply requested the migration ledger lock")
	}
	if !requiresAdvancedLedgerLock(Options{AdvancedCheckpoint: advancedHeadID}) {
		t.Fatal("advanced Apply did not request the migration ledger lock first")
	}
}

// The default contract stays strict: only CheckpointID is accepted. The explicit
// opt-in changes the required head to the exact reviewed migration ID.
func TestRequiredCheckpointKeepsDefaultStrictAndHonorsAdvanced(t *testing.T) {
	if got := requiredCheckpoint(Options{}); got != CheckpointID {
		t.Fatalf("default required checkpoint = %q, want %q", got, CheckpointID)
	}
	if got := requiredCheckpoint(Options{AdvancedCheckpoint: advancedHeadID}); got != advancedHeadID {
		t.Fatalf("advanced required checkpoint = %q, want %q", got, advancedHeadID)
	}
}

func sealedReport(checkpoint string) Report {
	report := Report{Version: ReportVersion, Checkpoint: checkpoint}
	report.seal()
	return report
}

func TestValidateApplyReview(t *testing.T) {
	defaultReport := sealedReport(CheckpointID)
	advancedReport := sealedReport(advancedHeadID)
	otherHeadReport := sealedReport(postCheckpointID)
	for _, testCase := range []struct {
		name         string
		option       Options
		report       Report
		expected     string
		wantRejected bool
	}{
		{
			name:     "default_strict_head_with_matching_digest",
			option:   Options{},
			report:   defaultReport,
			expected: defaultReport.Digest,
		},
		{
			name:         "default_rejects_advanced_head",
			option:       Options{},
			report:       advancedReport,
			expected:     advancedReport.Digest,
			wantRejected: true,
		},
		{
			name:     "advanced_accepts_explicit_head",
			option:   Options{AdvancedCheckpoint: advancedHeadID},
			report:   advancedReport,
			expected: advancedReport.Digest,
		},
		{
			name:         "advanced_rejects_registered_checkpoint",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       defaultReport,
			expected:     defaultReport.Digest,
			wantRejected: true,
		},
		{
			name:         "advanced_rejects_other_head",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       otherHeadReport,
			expected:     otherHeadReport.Digest,
			wantRejected: true,
		},
		{
			name:         "empty_report_is_illegal",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       advancedReport,
			expected:     "",
			wantRejected: true,
		},
		{
			name:         "short_report_is_illegal",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       advancedReport,
			expected:     advancedReport.Digest[:63],
			wantRejected: true,
		},
		{
			name:         "non_hex_report_is_illegal",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       advancedReport,
			expected:     strings.Repeat("z", 64),
			wantRejected: true,
		},
		{
			name:         "uppercase_report_is_illegal",
			option:       Options{AdvancedCheckpoint: advancedHeadID},
			report:       advancedReport,
			expected:     strings.ToUpper(advancedReport.Digest),
			wantRejected: true,
		},
		{
			name:         "valid_but_mismatched_report",
			option:       Options{},
			report:       defaultReport,
			expected:     strings.Repeat("0", 64),
			wantRejected: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateApplyReview(testCase.option, testCase.report, testCase.expected)
			if testCase.wantRejected {
				if !errors.Is(err, errApplyReviewChanged) {
					t.Fatalf("validateApplyReview() = %v, want %v", err, errApplyReviewChanged)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateApplyReview() = %v, want nil", err)
			}
		})
	}
}

// A completed schema2 retirement cannot be re-entered through the explicit
// advanced checkpoint. The default repeat path keeps running verify.
func TestValidateApplyEntryRejectsAdvancedOnCompletedSchema2(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		option  Options
		version int
		wantErr error
	}{
		{name: "default_schema1_allowed", option: Options{}, version: 1},
		{name: "default_schema2_repeat_allowed", option: Options{}, version: 2},
		{name: "advanced_schema1_allowed", option: Options{AdvancedCheckpoint: advancedHeadID}, version: 1},
		{name: "advanced_schema2_rejected", option: Options{AdvancedCheckpoint: advancedHeadID}, version: 2, wantErr: errAdvancedAlreadyDone},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateApplyEntry(testCase.option, testCase.version)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("validateApplyEntry(%q, %d) = %v, want %v", testCase.option.AdvancedCheckpoint, testCase.version, err, testCase.wantErr)
			}
		})
	}
}
