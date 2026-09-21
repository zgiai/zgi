package connectorfinalize

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// The deferred rule must accept exactly one marker/relation combination and
// fail closed on every partial-migration disagreement.
func TestPendingSetupsDispositionMatrix(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		markerApplied  bool
		relationExists bool
		wantDeferred   bool
		wantErr        string
	}{
		{name: "creator_not_applied_and_relation_absent", markerApplied: false, relationExists: false, wantDeferred: true},
		{name: "creator_applied_and_relation_present", markerApplied: true, relationExists: true, wantDeferred: false},
		{name: "creator_applied_but_relation_missing", markerApplied: true, relationExists: false, wantErr: "missing"},
		{name: "relation_present_but_creator_not_applied", markerApplied: false, relationExists: true, wantErr: "not applied"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			deferred, err := pendingSetupsDisposition(testCase.markerApplied, testCase.relationExists)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("disposition error = %v, want %q", err, testCase.wantErr)
				}
				if deferred {
					t.Fatal("partial migration was deferred instead of rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if deferred != testCase.wantDeferred {
				t.Fatalf("deferred = %v, want %v", deferred, testCase.wantDeferred)
			}
		})
	}
}

func TestNativeInventoryDefersOnlyPendingSetups(t *testing.T) {
	manifest := NativeTables()
	if !slices.Contains(manifest, pendingSetupsTable) {
		t.Fatalf("%s is not part of the native manifest", pendingSetupsTable)
	}
	if slices.Contains(RetiredTables(), pendingSetupsTable) {
		t.Fatalf("%s belongs to the retirement manifest", pendingSetupsTable)
	}
	for _, testCase := range []struct {
		name           string
		markerApplied  bool
		relationExists bool
		wantDeferred   bool
		wantErr        bool
	}{
		{name: "deferred", markerApplied: false, relationExists: false, wantDeferred: true},
		{name: "read", markerApplied: true, relationExists: true, wantDeferred: false},
		{name: "marker_without_relation", markerApplied: true, relationExists: false, wantErr: true},
		{name: "relation_without_marker", markerApplied: false, relationExists: true, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tables, deferred, err := nativeInventory(testCase.markerApplied, testCase.relationExists)
			if testCase.wantErr {
				if err == nil || tables != nil || deferred != nil {
					t.Fatalf("inventory = (%v, %v, %v), want a fail-closed error", tables, deferred, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (len(deferred) == 1 && deferred[0] == pendingSetupsTable) != testCase.wantDeferred {
				t.Fatalf("deferred = %v, want %v", deferred, testCase.wantDeferred)
			}
			if len(tables)+len(deferred) != len(manifest) {
				t.Fatalf("inventory lost a table: tables=%d deferred=%d manifest=%d", len(tables), len(deferred), len(manifest))
			}
			for _, table := range manifest {
				inTables := slices.Contains(tables, table)
				inDeferred := slices.Contains(deferred, table)
				if table == pendingSetupsTable {
					if inTables == testCase.wantDeferred || inDeferred != testCase.wantDeferred {
						t.Fatalf("pending table disposition = (inTables=%v, inDeferred=%v), want deferred %v", inTables, inDeferred, testCase.wantDeferred)
					}
					continue
				}
				if !inTables || inDeferred {
					t.Fatalf("mandatory native table %s was dropped from the inventory", table)
				}
			}
		})
	}
}

func TestDeferredNativeTablesAreCommittedByReportDigest(t *testing.T) {
	withDeferred := Report{Version: ReportVersion, Issues: []Issue{}, DeferredNativeTables: []string{pendingSetupsTable}}
	withDeferred.seal()
	withoutDeferred := Report{Version: ReportVersion, Issues: []Issue{}, DeferredNativeTables: []string{}}
	withoutDeferred.seal()
	if withDeferred.Digest == withoutDeferred.Digest {
		t.Fatal("sealed digest does not commit the deferred native inventory")
	}
	if ReportVersion < 3 {
		t.Fatalf("deferred inventory requires the current report version, got %d", ReportVersion)
	}
}

func TestPendingSetupsMigrationSortsAfterFinalizer(t *testing.T) {
	// The deferral is only sound while the creating migration is applied after
	// the finalizer in the sorted migration chain.
	if pendingSetupsMigration <= MigrationID {
		t.Fatalf("%s must sort after %s", pendingSetupsMigration, MigrationID)
	}
}

// The reviewed archive seals the deferred inventory too, so an archive created
// with a deferred table cannot be replayed against a report that read it.
func TestArchiveBindsDeferredNativeInventory(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	data := snapshot{}
	report := Report{Version: ReportVersion, Issues: []Issue{}, DeferredNativeTables: []string{pendingSetupsTable}}
	report.seal()
	encrypted, err := encryptArchive(archivePayload{Report: report, Data: data}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(encrypted, key, report, data); err != nil {
		t.Fatal(err)
	}
	changed := report
	changed.DeferredNativeTables = []string{}
	changed.seal()
	if verifyArchive(encrypted, key, changed, data) == nil {
		t.Fatal("archive accepted a different deferred native inventory")
	}
}
