package migrations

import (
	"strings"
	"testing"
)

func TestConnectorRouteRolloutSnapshotMigrationPinsCompleteResolution(t *testing.T) {
	for _, fragment := range []string{
		"ADD COLUMN provider_phase", "ADD COLUMN route_ownership", "ADD COLUMN rollout_revision",
		"route_ownership = 'migrated'", "rollout_revision >= 1", "provider_phase IN ('legacy', 'shadow', 'reversing')",
		"path = 'legacy'", "provider_phase = 'canary'", "path IN ('legacy', 'connector_native')",
		"provider_phase = 'connector' AND path = 'connector_native'",
		"DROP TRIGGER connector_execution_route_claims_immutable", "CREATE TRIGGER connector_execution_route_claims_immutable",
	} {
		if !strings.Contains(addConnectorRouteRolloutSnapshotSQL, fragment) {
			t.Fatalf("route rollout migration missing %q", fragment)
		}
	}
	if !strings.Contains(dropConnectorRouteRolloutSnapshotSQL, "cannot remove Connector route rollout snapshots while claims exist") {
		t.Fatal("route rollout downgrade does not fail closed on durable claims")
	}
}
