package migrations

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
)

func TestDeferredRunnerHistoryPolicy(t *testing.T) {
	publicPlan := migrationPlan{validateUnknownMigrations: true}
	legacyPlan := migrationPlan{validateUnknownMigrations: false}
	legacyID := "20251127000002"
	cases := []struct {
		name    string
		plan    migrationPlan
		applied map[string]struct{}
		wantErr bool
	}{
		{name: "public history rejects legacy marker", plan: publicPlan, applied: map[string]struct{}{legacyID: {}}, wantErr: true},
		{name: "legacy bridge permits legacy marker", plan: legacyPlan, applied: map[string]struct{}{legacyID: {}}},
		{name: "retirement marker is known", plan: publicPlan, applied: map[string]struct{}{connectorfinalize.MigrationID: {}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateDeferredMigrationHistoryForPlan(test.plan, test.applied, true)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, test.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), legacyID) {
				t.Fatalf("error does not identify the old marker: %v", err)
			}
		})
	}
}
