package migrations

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
)

func TestRunningWithoutDeferredModeRequiresAnExplicitRetirementExit(t *testing.T) {
	ledger := func(ids ...string) map[string]struct{} {
		applied := map[string]struct{}{}
		for _, id := range ids {
			applied[id] = struct{}{}
		}
		return applied
	}
	const later = "20261016110000_add_tool_usage_upstream_scope"
	for name, test := range map[string]struct {
		applied map[string]struct{}
		refused bool
	}{
		"fresh database":                         {ledger(), false},
		"history before retirement":              {ledger(connectorfinalize.DetachCheckpointID, connectorfinalize.CheckpointID), false},
		"retired database with later migrations": {ledger(connectorfinalize.CheckpointID, connectorfinalize.MigrationID, later), false},
		"historical branch with a compatible later migration": {
			ledger(connectorfinalize.DetachCheckpointID, "20260903183000_add_video_runtime_task_poster_url"), false,
		},
		"database that ran deferred and kept legacy storage": {ledger(connectorfinalize.CheckpointID, later), true},
	} {
		t.Run(name, func(t *testing.T) {
			err := requireDeferredRetirementExit(test.applied)
			if (err != nil) != test.refused {
				t.Fatalf("requireDeferredRetirementExit() = %v, want refused %v", err, test.refused)
			}
			if test.refused && (!strings.Contains(err.Error(), "connector-finalize") || !strings.Contains(err.Error(), "No migrations were applied")) {
				t.Fatalf("refusal %q does not tell the operator what to do", err)
			}
		})
	}
}
