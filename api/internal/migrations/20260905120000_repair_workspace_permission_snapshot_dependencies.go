package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationRepairWorkspacePermissionSnapshotDependenciesID = "20260905120000_repair_workspace_permission_snapshot_dependencies"

func init() {
	registerSchemaMigration(
		migrationRepairWorkspacePermissionSnapshotDependenciesID,
		upRepairWorkspacePermissionSnapshotDependencies,
		nil,
	)
}

func upRepairWorkspacePermissionSnapshotDependencies(schema *mschema.Builder) error {
	// The original canonicalization migration has already run on deployed
	// databases. Use a new migration ID so the dependency expansion added later
	// is applied to those existing role and member snapshots as well.
	return schema.DataFix("repair workspace permission snapshot dependencies", func(db *gorm.DB) error {
		if err := canonicalizeWorkspaceRolePermissionSnapshots(db); err != nil {
			return err
		}
		return canonicalizeWorkspaceMemberPermissionSnapshots(db)
	})
}
