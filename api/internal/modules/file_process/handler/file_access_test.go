package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/dto"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestAuthorizeFileAccessRejectsCrossOrganizationFile(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-2",
				CreatedBy:      "account-1",
			},
		},
	}

	_, ok := authorizeFileViewAccess(c, fileService, &fileAccessPermissionChecker{}, "file-1")

	if ok {
		t.Fatalf("authorizeFileViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestAuthorizeFileViewAccessAllowsWorkspaceDownloadPermission(t *testing.T) {
	c, _ := newFileAccessTestContext("account-1", "org-1")
	workspaceID := "workspace-1"
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
			},
		},
	}
	permissionChecker := &fileAccessPermissionChecker{
		allowed: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionFileDownload: true,
		},
	}

	uploadFile, ok := authorizeFileViewAccess(c, fileService, permissionChecker, "file-1")

	if !ok {
		t.Fatalf("authorizeFileViewAccess ok = false, want true")
	}
	if uploadFile == nil || uploadFile.ID != "file-1" {
		t.Fatalf("uploadFile = %#v, want file-1", uploadFile)
	}
	wantPermissions := fileReadablePermissionCodes()
	if !reflect.DeepEqual(permissionChecker.lastPermissions, wantPermissions) {
		t.Fatalf("permissions = %#v, want %#v", permissionChecker.lastPermissions, wantPermissions)
	}
	if containsWorkspacePermission(permissionChecker.lastPermissions, workspace_model.WorkspacePermissionFileView) ||
		containsWorkspacePermission(permissionChecker.lastPermissions, workspace_model.WorkspacePermissionFileManage) {
		t.Fatalf("permissions = %#v should use fine file permissions", permissionChecker.lastPermissions)
	}
}

func TestAuthorizeFileManageAccessRejectsWorkspaceFileWithoutManagePermission(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	workspaceID := "workspace-1"
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
			},
		},
	}

	_, ok := authorizeFileManageAccess(c, fileService, &fileAccessPermissionChecker{}, "file-1")

	if ok {
		t.Fatalf("authorizeFileManageAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileManageAccessRejectsOrganizationFileForNonCreatorNonAdmin(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				CreatedBy:      "account-2",
			},
		},
	}

	_, ok := authorizeFileManageAccess(c, fileService, &fileAccessPermissionChecker{}, "file-1")

	if ok {
		t.Fatalf("authorizeFileManageAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileManageAccessAllowsOrganizationFileCreator(t *testing.T) {
	c, _ := newFileAccessTestContext("account-1", "org-1")
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				CreatedBy:      "account-1",
			},
		},
	}

	uploadFile, ok := authorizeFileManageAccess(c, fileService, nil, "file-1")

	if !ok {
		t.Fatalf("authorizeFileManageAccess ok = false, want true")
	}
	if uploadFile == nil || uploadFile.ID != "file-1" {
		t.Fatalf("uploadFile = %#v, want file-1", uploadFile)
	}
}

func TestAuthorizeFileManageAccessRejectsOrganizationAdminWithoutWorkspacePermission(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				CreatedBy:      "account-2",
			},
		},
	}

	_, ok := authorizeFileManageAccess(c, fileService, &fileAccessPermissionChecker{}, "file-1")

	if ok {
		t.Fatalf("authorizeFileManageAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileActionAccessUsesFinePermissions(t *testing.T) {
	tests := []struct {
		name           string
		authorize      func(*gin.Context, interfaces.FileService, fileWorkspacePermissionChecker, string) (*dto.UploadFile, bool)
		permissionCode workspace_model.WorkspacePermissionCode
	}{
		{
			name:           "delete",
			authorize:      authorizeFileDeleteAccess,
			permissionCode: workspace_model.WorkspacePermissionFileDelete,
		},
		{
			name:           "move",
			authorize:      authorizeFileMoveAccess,
			permissionCode: workspace_model.WorkspacePermissionFileMove,
		},
		{
			name:           "archive",
			authorize:      authorizeFileArchiveAccess,
			permissionCode: workspace_model.WorkspacePermissionFileArchive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newFileAccessTestContext("account-1", "org-1")
			workspaceID := "workspace-1"
			fileService := &fileAccessFileService{
				files: map[string]*dto.UploadFile{
					"file-1": {
						ID:             "file-1",
						OrganizationID: "org-1",
						WorkspaceID:    &workspaceID,
						CreatedBy:      "account-2",
					},
				},
			}
			permissionChecker := &fileAccessPermissionChecker{
				allowed: map[workspace_model.WorkspacePermissionCode]bool{
					tt.permissionCode: true,
				},
			}

			uploadFile, ok := tt.authorize(c, fileService, permissionChecker, "file-1")

			if !ok {
				t.Fatalf("%s ok = false, want true", tt.name)
			}
			if uploadFile == nil || uploadFile.ID != "file-1" {
				t.Fatalf("uploadFile = %#v, want file-1", uploadFile)
			}
			wantPermissions := []workspace_model.WorkspacePermissionCode{tt.permissionCode}
			if !reflect.DeepEqual(permissionChecker.lastPermissions, wantPermissions) {
				t.Fatalf("permissions = %#v, want %#v", permissionChecker.lastPermissions, wantPermissions)
			}
		})
	}
}

func TestAuthorizeFileAccessRestrictsTemporaryFilesToCreator(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: "org-1",
				CreatedBy:      "account-2",
				IsTemporary:    true,
			},
		},
	}

	_, ok := authorizeFileViewAccess(c, fileService, &fileAccessPermissionChecker{}, "file-1")

	if ok {
		t.Fatalf("authorizeFileViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileFolderManageAllowsWorkspaceManagePermission(t *testing.T) {
	c, _ := newFileAccessTestContext("account-1", "org-1")
	workspaceID := "workspace-1"
	folderService := &fileAccessFolderService{
		folders: map[string]*file_model.FileFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionAllTeam),
			},
		},
	}
	permissionChecker := &fileAccessPermissionChecker{
		allowed: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionFileFolderManage: true,
		},
	}

	folder, ok := authorizeFileFolderManageAccess(c, folderService, permissionChecker, "folder-1")

	if !ok {
		t.Fatalf("authorizeFileFolderManageAccess ok = false, want true")
	}
	if folder == nil || folder.ID != "folder-1" {
		t.Fatalf("folder = %#v, want folder-1", folder)
	}
	if permissionChecker.lastWorkspaceID != workspaceID {
		t.Fatalf("workspaceID = %q, want %q", permissionChecker.lastWorkspaceID, workspaceID)
	}
	wantPermissions := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionFileFolderManage}
	if !reflect.DeepEqual(permissionChecker.lastPermissions, wantPermissions) {
		t.Fatalf("permissions = %#v, want %#v", permissionChecker.lastPermissions, wantPermissions)
	}
}

func TestAuthorizeFileFolderManageRejectsWorkspaceFolderWithoutManagePermission(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	workspaceID := "workspace-1"
	folderService := &fileAccessFolderService{
		folders: map[string]*file_model.FileFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionAllTeam),
			},
		},
	}

	_, ok := authorizeFileFolderManageAccess(c, folderService, &fileAccessPermissionChecker{}, "folder-1")

	if ok {
		t.Fatalf("authorizeFileFolderManageAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileFolderViewRejectsOnlyMeForNonCreator(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	folderService := &fileAccessFolderService{
		folders: map[string]*file_model.FileFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionOnlyMe),
			},
		},
	}

	_, ok := authorizeFileFolderViewAccess(c, folderService, &fileAccessPermissionChecker{}, "folder-1")

	if ok {
		t.Fatalf("authorizeFileFolderViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeFileFolderViewAllowsPartialTeamWorkspacePermission(t *testing.T) {
	c, _ := newFileAccessTestContext("account-1", "org-1")
	workspaceID := "workspace-1"
	sharedWorkspaceID := "workspace-2"
	folderService := &fileAccessFolderService{
		folders: map[string]*file_model.FileFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionPartialTeam),
			},
		},
		partialWorkspaces: map[string][]string{
			"folder-1": {sharedWorkspaceID},
		},
	}
	permissionChecker := &fileAccessPermissionChecker{
		allowedByWorkspace: map[string]map[workspace_model.WorkspacePermissionCode]bool{
			workspaceID: {
				workspace_model.WorkspacePermissionFileFolderView: true,
			},
			sharedWorkspaceID: {
				workspace_model.WorkspacePermissionFileFolderView: true,
			},
		},
	}

	folder, ok := authorizeFileFolderViewAccess(c, folderService, permissionChecker, "folder-1")

	if !ok {
		t.Fatalf("authorizeFileFolderViewAccess ok = false, want true")
	}
	if folder == nil || folder.ID != "folder-1" {
		t.Fatalf("folder = %#v, want folder-1", folder)
	}
}

func TestCanListFavoriteFileFiltersInvisibleWorkspaceFile(t *testing.T) {
	workspaceID := "workspace-1"
	handler := &FileFavoriteHandler{
		fileService: &fileAccessFileService{
			files: map[string]*dto.UploadFile{
				"file-1": {
					ID:             "file-1",
					OrganizationID: "org-1",
					WorkspaceID:    &workspaceID,
					CreatedBy:      "account-2",
				},
			},
		},
		enterpriseService: &fileAccessPermissionChecker{},
	}

	allowed, err := handler.canListFavoriteFile(context.Background(), "org-1", "account-1", "file-1")

	if err != nil {
		t.Fatalf("canListFavoriteFile err = %v, want nil", err)
	}
	if allowed {
		t.Fatalf("canListFavoriteFile allowed = true, want false")
	}
}

func TestAuthorizeWorkspaceUploadUsesDirectWorkspacePermission(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	permissionChecker := &fileUploadPermissionChecker{
		allowed: false,
	}
	handler := &FileHandler{
		enterpriseService: permissionChecker,
		tenantService: &fileUploadWorkspaceService{
			role: ptrWorkspaceRole(workspace_model.WorkspaceRoleAdmin),
		},
	}

	ok := handler.authorizeWorkspaceUpload(c, "org-1", "workspace-1", "account-1")

	if ok {
		t.Fatalf("authorizeWorkspaceUpload ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if permissionChecker.lastWorkspaceID != "workspace-1" {
		t.Fatalf("workspace id = %q, want workspace-1", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionFileUploadCreate {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionFileUploadCreate)
	}
}

func TestAuthorizeWorkspaceUploadLegacyFallbackRequiresWorkspaceMembership(t *testing.T) {
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	handler := &FileHandler{
		tenantService: &fileUploadWorkspaceService{},
	}

	ok := handler.authorizeWorkspaceUpload(c, "org-1", "workspace-1", "account-1")

	if ok {
		t.Fatalf("authorizeWorkspaceUpload ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeWorkspaceUploadLegacyFallbackAllowsWorkspaceMember(t *testing.T) {
	c, _ := newFileAccessTestContext("account-1", "org-1")
	handler := &FileHandler{
		tenantService: &fileUploadWorkspaceService{
			role: ptrWorkspaceRole(workspace_model.WorkspaceRoleNormal),
		},
	}

	ok := handler.authorizeWorkspaceUpload(c, "org-1", "workspace-1", "account-1")

	if !ok {
		t.Fatalf("authorizeWorkspaceUpload ok = false, want true")
	}
}

func TestPatchFolderRequiresManageBeforeBindingRequest(t *testing.T) {
	folderID := "11111111-1111-1111-1111-111111111111"
	workspaceID := "workspace-1"
	folderService := &fileResourcePermissionFolderService{
		folders: map[string]*file_model.FileFolder{
			folderID: {
				ID:             folderID,
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionAllTeam),
			},
		},
	}
	handler := &FileResourceHandler{
		fileFolderService: folderService,
		enterpriseService: &fileResourcePermissionChecker{checker: &fileAccessPermissionChecker{}},
	}
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPatch, "/file-folders/"+folderID, bytes.NewBufferString("{"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "folder_id", Value: folderID}}

	handler.PatchFolder(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if folderService.updateFolderCalls != 0 {
		t.Fatalf("UpdateFolder calls = %d, want 0", folderService.updateFolderCalls)
	}
}

func TestMoveFilesToFolderRequiresTargetFolderManageBeforeMove(t *testing.T) {
	folderID := "11111111-1111-1111-1111-111111111111"
	fileID := "22222222-2222-2222-2222-222222222222"
	workspaceID := "workspace-1"
	folderService := &fileResourcePermissionFolderService{
		folders: map[string]*file_model.FileFolder{
			folderID: {
				ID:             folderID,
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
				Permission:     string(file_model.FileFolderPermissionAllTeam),
			},
		},
	}
	handler := &FileResourceHandler{
		fileFolderService: folderService,
		enterpriseService: &fileResourcePermissionChecker{checker: &fileAccessPermissionChecker{}},
	}
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/file-folders/move-files", bytes.NewBufferString(`{"file_ids":["`+fileID+`"],"folder_id":"`+folderID+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.MoveFilesToFolder(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if folderService.moveFilesToFolderCalls != 0 {
		t.Fatalf("MoveFilesToFolder calls = %d, want 0", folderService.moveFilesToFolderCalls)
	}
}

func TestMoveFilesToFolderRequiresFileManageBeforeMove(t *testing.T) {
	folderID := "11111111-1111-1111-1111-111111111111"
	fileID := "22222222-2222-2222-2222-222222222222"
	workspaceID := "workspace-1"
	folderService := &fileResourcePermissionFolderService{
		folders: map[string]*file_model.FileFolder{
			folderID: {
				ID:             folderID,
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-1",
				Permission:     string(file_model.FileFolderPermissionAllTeam),
			},
		},
	}
	fileService := &fileAccessFileService{
		files: map[string]*dto.UploadFile{
			fileID: {
				ID:             fileID,
				OrganizationID: "org-1",
				WorkspaceID:    &workspaceID,
				CreatedBy:      "account-2",
			},
		},
	}
	handler := &FileResourceHandler{
		fileFolderService: folderService,
		fileService:       fileService,
		enterpriseService: &fileResourcePermissionChecker{checker: &fileAccessPermissionChecker{}},
	}
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/file-folders/move-files", bytes.NewBufferString(`{"file_ids":["`+fileID+`"],"folder_id":"`+folderID+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.MoveFilesToFolder(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if folderService.moveFilesToFolderCalls != 0 {
		t.Fatalf("MoveFilesToFolder calls = %d, want 0", folderService.moveFilesToFolderCalls)
	}
}

func TestGetFileStatisticsReturnsZeroWithoutVisibleWorkspace(t *testing.T) {
	folderService := &fileResourcePermissionFolderService{
		statistics: &dto.FileStatisticsResponse{TotalCount: 99},
	}
	handler := &FileResourceHandler{
		fileFolderService: folderService,
		enterpriseService: &fileResourcePermissionChecker{
			checker: &fileAccessPermissionChecker{},
			workspaces: []*workspace_model.Workspace{
				{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal},
			},
		},
	}
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodGet, "/files/statistics", nil)

	handler.GetFileStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if folderService.getFileStatisticsCalls != 0 {
		t.Fatalf("GetFileStatistics calls = %d, want 0", folderService.getFileStatisticsCalls)
	}

	var body struct {
		Data dto.FileStatisticsResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.TotalCount != 0 || body.Data.RecentCount != 0 || body.Data.FavoriteCount != 0 || body.Data.RootFolderCount != 0 || body.Data.ArchivedCount != 0 {
		t.Fatalf("statistics = %#v, want zero value", body.Data)
	}
}

func TestGetFileStatisticsScopesServiceToVisibleWorkspaces(t *testing.T) {
	folderService := &fileResourcePermissionFolderService{
		statistics: &dto.FileStatisticsResponse{
			TotalCount:      3,
			RecentCount:     2,
			FavoriteCount:   1,
			RootFolderCount: 1,
			ArchivedCount:   4,
		},
	}
	handler := &FileResourceHandler{
		fileFolderService: folderService,
		enterpriseService: &fileResourcePermissionChecker{
			checker: &fileAccessPermissionChecker{
				allowedByWorkspace: map[string]map[workspace_model.WorkspacePermissionCode]bool{
					"workspace-visible": {
						workspace_model.WorkspacePermissionFileDownload: true,
					},
				},
			},
			workspaces: []*workspace_model.Workspace{
				{ID: "workspace-visible", Status: workspace_model.WorkspaceStatusNormal},
				{ID: "workspace-denied", Status: workspace_model.WorkspaceStatusNormal},
				{ID: "workspace-archived", Status: workspace_model.WorkspaceStatusArchived},
			},
		},
	}
	c, recorder := newFileAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodGet, "/files/statistics", nil)

	handler.GetFileStatistics(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if folderService.getFileStatisticsCalls != 1 {
		t.Fatalf("GetFileStatistics calls = %d, want 1", folderService.getFileStatisticsCalls)
	}
	wantWorkspaceIDs := []string{"workspace-visible"}
	if !reflect.DeepEqual(folderService.lastVisibleWorkspaceIDs, wantWorkspaceIDs) {
		t.Fatalf("visible workspace IDs = %#v, want %#v", folderService.lastVisibleWorkspaceIDs, wantWorkspaceIDs)
	}

	var body struct {
		Data dto.FileStatisticsResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(body.Data, *folderService.statistics) {
		t.Fatalf("statistics = %#v, want %#v", body.Data, *folderService.statistics)
	}
}

func newFileAccessTestContext(accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/file-1", nil)
	c.Set("account_id", accountID)
	util.SetOrganizationID(c, organizationID)
	return c, recorder
}

type fileAccessPermissionChecker struct {
	allowed            map[workspace_model.WorkspacePermissionCode]bool
	allowedByWorkspace map[string]map[workspace_model.WorkspacePermissionCode]bool
	lastWorkspaceID    string
	lastPermissions    []workspace_model.WorkspacePermissionCode
}

func (f *fileAccessPermissionChecker) CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	f.lastWorkspaceID = workspaceID
	f.lastPermissions = append([]workspace_model.WorkspacePermissionCode(nil), permissionCodes...)
	if workspacePermissions, ok := f.allowedByWorkspace[workspaceID]; ok {
		for _, permissionCode := range permissionCodes {
			if workspacePermissions[permissionCode] {
				return true, nil
			}
		}
		return false, nil
	}
	for _, permissionCode := range permissionCodes {
		if f.allowed[permissionCode] {
			return true, nil
		}
	}
	return false, nil
}

type fileUploadPermissionChecker struct {
	interfaces.OrganizationService
	allowed         bool
	lastWorkspaceID string
	lastPermission  workspace_model.WorkspacePermissionCode
}

func (f *fileUploadPermissionChecker) CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	f.lastWorkspaceID = workspaceID
	f.lastPermission = permissionCode
	return f.allowed, nil
}

type fileUploadWorkspaceService struct {
	interfaces.WorkspaceManagementService
	role *workspace_model.WorkspaceMemberRole
}

func (f *fileUploadWorkspaceService) GetUserRole(ctx context.Context, accountID, workspaceID string) (*workspace_model.WorkspaceMemberRole, error) {
	return f.role, nil
}

func ptrWorkspaceRole(role workspace_model.WorkspaceMemberRole) *workspace_model.WorkspaceMemberRole {
	return &role
}

type fileAccessFolderService struct {
	folders           map[string]*file_model.FileFolder
	partialWorkspaces map[string][]string
}

func (f *fileAccessFolderService) GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error) {
	folder, ok := f.folders[id]
	if !ok {
		return nil, file_model.ErrFileNotFound
	}
	return folder, nil
}

func (f *fileAccessFolderService) GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error) {
	return f.partialWorkspaces[folderID], nil
}

func containsWorkspacePermission(permissions []workspace_model.WorkspacePermissionCode, want workspace_model.WorkspacePermissionCode) bool {
	for _, permission := range permissions {
		if permission == want {
			return true
		}
	}
	return false
}

type fileResourcePermissionFolderService struct {
	service.FileFolderService
	folders                 map[string]*file_model.FileFolder
	partialWorkspaces       map[string][]string
	updateFolderCalls       int
	moveFilesToFolderCalls  int
	statistics              *dto.FileStatisticsResponse
	getFileStatisticsCalls  int
	lastVisibleWorkspaceIDs []string
}

func (f *fileResourcePermissionFolderService) GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error) {
	folder, ok := f.folders[id]
	if !ok {
		return nil, file_model.ErrFileNotFound
	}
	return folder, nil
}

func (f *fileResourcePermissionFolderService) GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error) {
	return f.partialWorkspaces[folderID], nil
}

func (f *fileResourcePermissionFolderService) UpdateFolder(ctx context.Context, id string, updates map[string]interface{}) (*file_model.FileFolder, error) {
	f.updateFolderCalls++
	return f.GetFolderByID(ctx, id)
}

func (f *fileResourcePermissionFolderService) MoveFilesToFolder(ctx context.Context, fileIDs []string, folderID string, accountID string) error {
	f.moveFilesToFolderCalls++
	return nil
}

func (f *fileResourcePermissionFolderService) GetFileStatistics(ctx context.Context, tenantID, accountID string, visibleWorkspaceIDs []string) (*dto.FileStatisticsResponse, error) {
	f.getFileStatisticsCalls++
	f.lastVisibleWorkspaceIDs = append([]string(nil), visibleWorkspaceIDs...)
	if f.statistics != nil {
		return f.statistics, nil
	}
	return &dto.FileStatisticsResponse{}, nil
}

type fileResourcePermissionChecker struct {
	interfaces.OrganizationService
	checker    *fileAccessPermissionChecker
	workspaces []*workspace_model.Workspace
}

func (f *fileResourcePermissionChecker) CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	return f.checker.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, permissionCodes...)
}

func (f *fileResourcePermissionChecker) GetOrganizationWorkspacesList(ctx context.Context, organizationID string) ([]*workspace_model.Workspace, error) {
	return append([]*workspace_model.Workspace(nil), f.workspaces...), nil
}

type fileAccessFileService struct {
	files map[string]*dto.UploadFile
}

func (f *fileAccessFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return nil
}

func (f *fileAccessFileService) UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, tenantID string, userRole file_model.CreatedByRole, source *interfaces.FileSource, teamTenantID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error) {
	return nil, nil
}

func (f *fileAccessFileService) GetFilePreview(ctx context.Context, fileID string) (string, error) {
	return "", nil
}

func (f *fileAccessFileService) GetFilePreviewWithOCR(ctx context.Context, fileID string, enableOCR bool) (string, error) {
	return "", nil
}

func (f *fileAccessFileService) GetFile(ctx context.Context, fileID string) (string, error) {
	return "", nil
}

func (f *fileAccessFileService) ExtractFileWithSetting(ctx context.Context, fileID string, setting interfaces.FileExtractionSetting) (string, error) {
	return "", nil
}

func (f *fileAccessFileService) GetSupportedFileTypes() []string {
	return nil
}

func (f *fileAccessFileService) IsFileSizeWithinLimit(extension string, fileSize int64) bool {
	return true
}

func (f *fileAccessFileService) ParseFileContent(ctx context.Context, uploadFileID string) {}

func (f *fileAccessFileService) GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error) {
	uploadFile, ok := f.files[fileID]
	if !ok {
		return nil, file_model.ErrFileNotFound
	}
	return uploadFile, nil
}

func (f *fileAccessFileService) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	return nil, nil
}

func (f *fileAccessFileService) ListFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (f *fileAccessFileService) ListArchivedFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error) {
	return nil, nil
}

func (f *fileAccessFileService) GetStorageUsage(ctx context.Context, tenantID string) (int64, error) {
	return 0, nil
}

func (f *fileAccessFileService) DeleteFiles(ctx context.Context, fileIDs []string) error {
	return nil
}

func (f *fileAccessFileService) UpdateContentText(ctx context.Context, fileID string, contentText string) error {
	return nil
}

func (f *fileAccessFileService) CleanupExpiredTemporaryFiles(ctx context.Context, ttl time.Duration) (int64, error) {
	return 0, nil
}

func (f *fileAccessFileService) GetFileURL(ctx context.Context, fileID string) (string, error) {
	return "", nil
}
