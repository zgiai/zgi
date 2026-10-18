package connectorfinalize

import (
	"slices"
	"testing"
)

// Apply and FinalizeEmpty must hold ACCESS EXCLUSIVE on both channel scope
// tables before inspect reads them. They are host tables: the preservation
// proof reads them to refuse an installation that still resolves a retired
// Connection, and the archive keeps them. They never join Source or Native.
func TestLockInventoryCoversChannelScopeTablesAsHostSnapshots(t *testing.T) {
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
		if !slices.Contains(hostTables(), table) {
			t.Fatalf("host snapshot manifest omits %s", table)
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
