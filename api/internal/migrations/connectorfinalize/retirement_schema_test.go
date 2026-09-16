package connectorfinalize

import (
	"slices"
	"testing"
)

// The channel scope inventory is lock-only. Apply and FinalizeEmpty must hold
// ACCESS EXCLUSIVE on both tables before inspect reads their row counts, while
// the tables stay outside every Source, Native and Host snapshot manifest.
func TestLockInventoryCoversChannelScopeTablesWithoutExpandingSnapshots(t *testing.T) {
	locked := lockInventoryTables()
	for _, table := range []string{channelInstallationsTable, channelInstallationCredentialsTable} {
		if !slices.Contains(locked, table) {
			t.Fatalf("fixed lock manifest omits %s", table)
		}
		if slices.Contains(RetiredTables(), table) {
			t.Fatalf("%s leaked into the retired source manifest", table)
		}
		if slices.Contains(NativeTables(), table) {
			t.Fatalf("%s leaked into the native snapshot manifest", table)
		}
		if slices.Contains(hostTables(), table) {
			t.Fatalf("%s leaked into the host snapshot manifest", table)
		}
	}
	seen := make(map[string]struct{}, len(locked))
	for _, table := range locked {
		if _, ok := seen[table]; ok {
			t.Fatalf("fixed lock manifest lists %s twice", table)
		}
		seen[table] = struct{}{}
	}
	if !slices.Equal(locked, lockInventoryTables()) {
		t.Fatal("fixed lock manifest order is not deterministic")
	}
	if locked[len(locked)-1] != "migrations" {
		t.Fatalf("migration ledger is not locked last: %v", locked)
	}
}
