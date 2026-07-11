package migrations

import (
	"encoding/json"
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const migration20260629120000ID = "20260629120000_canonicalize_workspace_permission_snapshots"

func init() {
	registerSchemaMigration(
		migration20260629120000ID,
		upCanonicalizeWorkspacePermissionSnapshots,
		nil,
	)
}

func upCanonicalizeWorkspacePermissionSnapshots(schema *mschema.Builder) error {
	return schema.DataFix("canonicalize workspace permission snapshots", func(db *gorm.DB) error {
		if err := canonicalizeWorkspaceRolePermissionSnapshots(db); err != nil {
			return err
		}
		return canonicalizeWorkspaceMemberPermissionSnapshots(db)
	})
}

type workspacePermissionSnapshotRow struct {
	ID          string
	Permissions string
}

type workspaceMemberPermissionSnapshotRow struct {
	ID          string
	Permissions string
}

func canonicalizeWorkspaceRolePermissionSnapshots(db *gorm.DB) error {
	var rows []workspacePermissionSnapshotRow
	if err := db.Raw(`
		SELECT id::text AS id, COALESCE(permissions::text, '[]') AS permissions
		FROM public.roles
	`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("failed to read workspace role permission snapshots: %w", err)
	}

	for _, row := range rows {
		permissionsJSON, err := canonicalWorkspaceAssignablePermissionJSON(row.Permissions)
		if err != nil {
			return fmt.Errorf("failed to canonicalize role permissions for %s: %w", row.ID, err)
		}
		if err := db.Table("public.roles").
			Where("id = ?::uuid", row.ID).
			Update("permissions", gorm.Expr("?::jsonb", permissionsJSON)).Error; err != nil {
			return fmt.Errorf("failed to persist canonical role permissions for %s: %w", row.ID, err)
		}
	}

	return nil
}

func canonicalizeWorkspaceMemberPermissionSnapshots(db *gorm.DB) error {
	var rows []workspaceMemberPermissionSnapshotRow
	if err := db.Raw(`
		SELECT id::text AS id, COALESCE(permissions::text, '[]') AS permissions
		FROM public.workspace_members
		WHERE role != 'owner'
	`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("failed to read workspace member permission snapshots: %w", err)
	}

	for _, row := range rows {
		permissionsJSON, err := canonicalWorkspaceAssignablePermissionJSON(row.Permissions)
		if err != nil {
			return fmt.Errorf("failed to canonicalize workspace member permissions for %s: %w", row.ID, err)
		}
		if err := db.Table("public.workspace_members").
			Where("id = ?", row.ID).
			Update("permissions", gorm.Expr("?::jsonb", permissionsJSON)).Error; err != nil {
			return fmt.Errorf("failed to persist canonical workspace member permissions for %s: %w", row.ID, err)
		}
	}

	return nil
}

func canonicalWorkspaceAssignablePermissionJSON(raw string) (string, error) {
	permissions, err := decodeWorkspaceMemberPermissionSeedJSON(raw)
	if err != nil {
		return "", err
	}

	permissions = expandLegacyAgentViewPermissionSnapshot(permissions)
	permissions = expandLegacyAssetViewPermissionSnapshot(permissions)
	sanitized := workspace_model.CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func expandLegacyAgentViewPermissionSnapshot(permissions []string) []string {
	if len(permissions) == 0 {
		return permissions
	}

	hasWorkflowView := false
	for _, permission := range permissions {
		if workspace_model.WorkspacePermissionCode(permission) == workspace_model.WorkspacePermissionWorkflowView {
			hasWorkflowView = true
			break
		}
	}

	expanded := make([]string, 0, len(permissions)+1)
	for _, permission := range permissions {
		expanded = append(expanded, permission)
		if !hasWorkflowView &&
			workspace_model.WorkspacePermissionCode(permission) == workspace_model.WorkspacePermissionAgentView {
			expanded = append(expanded, string(workspace_model.WorkspacePermissionWorkflowView))
			hasWorkflowView = true
		}
	}
	return expanded
}

func expandLegacyAssetViewPermissionSnapshot(permissions []string) []string {
	if len(permissions) == 0 {
		return permissions
	}

	expanded := make([]string, 0, len(permissions)+5)
	add := func(permission string) {
		for _, existing := range expanded {
			if existing == permission {
				return
			}
		}
		expanded = append(expanded, permission)
	}

	for _, permission := range permissions {
		add(permission)
		switch workspace_model.WorkspacePermissionCode(permission) {
		case workspace_model.WorkspacePermissionKnowledgeBaseView:
			add(string(workspace_model.WorkspacePermissionKnowledgeBaseDocumentView))
			add(string(workspace_model.WorkspacePermissionKnowledgeBaseGraphView))
		case workspace_model.WorkspacePermissionDatabaseView:
			add(string(workspace_model.WorkspacePermissionDatabaseSchemaView))
			add(string(workspace_model.WorkspacePermissionDatabaseRecordView))
			add(string(workspace_model.WorkspacePermissionDatabaseOperationLogsView))
		}
	}
	return expanded
}
