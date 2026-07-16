package handler

import (
	"testing"

	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestDatabaseExistingAssetVisibilityPermissionsExcludeDatabaseCreate(t *testing.T) {
	foundView := false
	for _, permission := range databaseExistingAssetVisibilityPermissions {
		if permission == workspace_model.WorkspacePermissionDatabaseView {
			foundView = true
		}
		if permission == workspace_model.WorkspacePermissionDatabaseCreate {
			t.Fatalf("database list visibility permissions should not include %s", permission)
		}
	}
	if !foundView {
		t.Fatalf("database list visibility permissions should include %s", workspace_model.WorkspacePermissionDatabaseView)
	}
}
