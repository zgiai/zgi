package handler

import (
	"testing"

	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestDatabaseExistingAssetVisibilityPermissionsExcludeDatabaseCreate(t *testing.T) {
	for _, permission := range databaseExistingAssetVisibilityPermissions {
		if permission == workspace_model.WorkspacePermissionDatabaseCreate {
			t.Fatalf("database list visibility permissions should not include %s", permission)
		}
	}
}
