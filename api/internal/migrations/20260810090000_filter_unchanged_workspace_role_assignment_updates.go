package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationFilterUnchangedWorkspaceRoleAssignmentUpdatesID = "20260810090000_filter_unchanged_workspace_role_assignment_updates"

func init() {
	registerSchemaMigration(
		migrationFilterUnchangedWorkspaceRoleAssignmentUpdatesID,
		upFilterUnchangedWorkspaceRoleAssignmentUpdates,
		nil,
	)
}

func upFilterUnchangedWorkspaceRoleAssignmentUpdates(schema *mschema.Builder) error {
	return schema.Raw(enforceActiveWorkspaceRoleTemplateAssignmentFunctionSQL)
}
