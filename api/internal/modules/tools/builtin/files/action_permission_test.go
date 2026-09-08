package files

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestFileMutationsUseExactActionPermissions(t *testing.T) {
	workspaceID := "workspace-1"
	scope := fileScope{OrganizationID: "org-1", WorkspaceID: workspaceID, AccountID: "account-1"}
	for _, tc := range []struct {
		permission     workspacemodel.WorkspacePermissionCode
		upload, delete bool
	}{
		{workspacemodel.WorkspacePermissionFileUpload, true, false},
		{workspacemodel.WorkspacePermissionFileDelete, false, true},
		{workspacemodel.WorkspacePermissionFileTextCreate, false, false},
		{workspacemodel.WorkspacePermissionFileUpdate, false, false},
		{workspacemodel.WorkspacePermissionFilePreview, false, false},
		{workspacemodel.WorkspacePermissionFileManage, false, true},
		{workspacemodel.WorkspacePermissionFileUploadCreate, true, false},
	} {
		t.Run(string(tc.permission), func(t *testing.T) {
			perms := &fakeWorkspacePermissionService{granted: []workspacemodel.WorkspacePermissionCode{tc.permission}}
			uploadErr := (&saveFileTool{workspacePerms: perms}).ensureFileCreatable(context.Background(), scope, workspaceID)
			deleteErr := (&deleteFileTool{workspacePerms: perms}).ensureFileManageable(context.Background(), scope, &dto.UploadFile{ID: "file-1", OrganizationID: "org-1", WorkspaceID: &workspaceID})
			if (uploadErr == nil) != tc.upload || (deleteErr == nil) != tc.delete {
				t.Fatalf("upload error=%v (allowed=%v); delete error=%v (allowed=%v)", uploadErr, tc.upload, deleteErr, tc.delete)
			}
			if len(perms.codes) != 2 || perms.codes[0] != workspacemodel.WorkspacePermissionFileUpload || perms.codes[1] != workspacemodel.WorkspacePermissionFileDelete {
				t.Fatalf("wrong action permissions: %v", perms.codes)
			}
		})
	}
}
