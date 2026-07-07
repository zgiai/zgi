package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	// RecentFilesLimit is the maximum number of recent files to return
	RecentFilesLimit = 20
)

var ErrFolderNameConflict = errors.New("folder name already exists in the same directory")

// FileFolderService defines the interface for file folder operations
type FileFolderService interface {
	// Folder operations
	CreateFolder(ctx context.Context, folder *file_model.FileFolder) (*file_model.FileFolder, error)
	GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error)
	GetFolderWithPermissionCheck(ctx context.Context, id, accountID, tenantID string) (*file_model.FileFolder, error)
	CheckFolderViewPermission(ctx context.Context, folderID, accountID, tenantID string, visibleWorkspaceIDs []string) (bool, error)
	GetFolderWithViewPermissionCheck(ctx context.Context, id, accountID, tenantID string, visibleWorkspaceIDs []string) (*file_model.FileFolder, error)
	UpdateFolder(ctx context.Context, id string, updates map[string]interface{}) (*file_model.FileFolder, error)
	DeleteFolder(ctx context.Context, id string) error
	ListFolders(ctx context.Context, tenantID string, page, limit int, keyword, sort, parentID string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error)
	ListFoldersWithPermissionFilter(ctx context.Context, tenantID, accountID string, page, limit int, keyword, sort, parentID, workspaceID string, visibleWorkspaceIDs []string) ([]*file_model.FileFolder, int64, error)
	GetFolderFileCount(ctx context.Context, folderID string) (int64, error)

	// File-folder operations
	AddFileToFolder(ctx context.Context, fileID, folderID, accountID string) error
	RemoveFileFromFolder(ctx context.Context, fileID, folderID string) error
	ListFilesInFolder(ctx context.Context, folderID string, page, limit int) ([]*file_model.UploadFile, int64, error)
	ListFilesInFolderWithFilters(ctx context.Context, folderID string, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	ListAllFilesWithFilters(ctx context.Context, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID, accountID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	ListFavoriteFiles(ctx context.Context, accountID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error)
	MoveFileToFolder(ctx context.Context, fileID, fromFolderID, toFolderID, accountID string) error
	MoveFilesToFolder(ctx context.Context, fileIDs []string, toFolderID, accountID string) error
	MoveFolderToFolder(ctx context.Context, folderID, targetID, accountID, tenantID string) error

	// File archive operations
	ArchiveFiles(ctx context.Context, fileIDs []string, accountID string) error
	UnarchiveFiles(ctx context.Context, fileIDs []string, accountID string) error

	// Permission check
	CheckFolderEditorPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error)
	GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error)
	GetFolderPermissionTenantDetails(ctx context.Context, folderID string) ([]dto.FileFolderPermissionTenantDetail, error)
	UpdatePartialWorkspaceList(ctx context.Context, folderID string, tenantIDs []string, createdBy string) error
	ClearPartialWorkspaceList(ctx context.Context, folderID string) error

	// Document relation operations
	GetRelatedDocumentCount(ctx context.Context, fileID string) (int, error)
	GetRelatedDatasetCount(ctx context.Context, fileID string) (int, error)
	GetRelatedDocuments(ctx context.Context, fileID string) ([]*dataset_model.Document, error)
	GetRelatedDatasets(ctx context.Context, fileID string) ([]*dataset_model.Dataset, error)
	BatchGetRelatedDatasetCount(ctx context.Context, fileIDs []string) (map[string]int, error)

	// File statistics operations
	GetFileStatistics(ctx context.Context, tenantID, accountID string, visibleWorkspaceIDs []string) (*dto.FileStatisticsResponse, error)
	GetTotalFileCount(ctx context.Context, tenantID string) (int64, error)
	// GetRecentFileCount gets the count of recent files (within last 3 months) for a tenant
	// Note: This method counts all recent files without applying the RecentFilesLimit.
	// For actual file listing with limit, use ListRecentFiles method.
	GetRecentFileCount(ctx context.Context, tenantID string) (int64, error)
	// GetRecentFileCountWithLimit gets the count of recent files (within last 3 months) for a tenant
	// with the same limit logic used in ListRecentFiles handler.
	GetRecentFileCountWithLimit(ctx context.Context, tenantID string) (int64, error)
	GetFavoriteFileCount(ctx context.Context, accountID, tenantID string) (int64, error)
	GetRootFolderFileCount(ctx context.Context, tenantID string) (int64, error)
	GetArchivedFileCount(ctx context.Context, tenantID string) (int64, error)
}

type fileResourceService struct {
	fileFolderRepo repository.FileFolderRepository
	fileRepo       repository.FileRepository
	documentRepo   dataset_repo.DocumentRepository
	datasetRepo    dataset_repo.DatasetRepository
	accountService interfaces.AccountService
}

func NewFileResourceService(
	fileFolderRepo repository.FileFolderRepository,
	fileRepo repository.FileRepository,
	documentRepo dataset_repo.DocumentRepository,
	datasetRepo dataset_repo.DatasetRepository,
	accountService interfaces.AccountService,
) FileFolderService {
	return &fileResourceService{
		fileFolderRepo: fileFolderRepo,
		fileRepo:       fileRepo,
		documentRepo:   documentRepo,
		datasetRepo:    datasetRepo,
		accountService: accountService,
	}
}

// CreateFolder creates a new folder
func (s *fileResourceService) CreateFolder(ctx context.Context, folder *file_model.FileFolder) (*file_model.FileFolder, error) {
	// Validate permission
	if !file_model.IsValidFileFolderPermission(folder.Permission) {
		return nil, fmt.Errorf("invalid folder permission: %s", folder.Permission)
	}

	folder.Name = strings.TrimSpace(folder.Name)
	if err := s.ensureSiblingFolderNameAvailable(ctx, folder.OrganizationID, folder.WorkspaceID, folder.ParentID, folder.Name, nil); err != nil {
		return nil, err
	}

	if err := s.fileFolderRepo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	return folder, nil
}

func needsFolderNameConflictCheck(updates map[string]interface{}) bool {
	_, hasName := updates["name"]
	_, hasParentID := updates["parent_id"]
	_, hasWorkspaceID := updates["workspace_id"]
	return hasName || hasParentID || hasWorkspaceID
}

func normalizeFolderIDUpdate(value interface{}) (*string, bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, true
		}
		return &trimmed, true
	case *string:
		if v == nil {
			return nil, true
		}
		trimmed := strings.TrimSpace(*v)
		if trimmed == "" {
			return nil, true
		}
		return &trimmed, true
	default:
		return nil, false
	}
}

func folderIDUpdateValue(id *string) interface{} {
	if id == nil {
		return nil
	}
	return *id
}

func (s *fileResourceService) ensureSiblingFolderNameAvailable(ctx context.Context, organizationID string, workspaceID *string, parentID *string, name string, excludeFolderID *string) error {
	exists, err := s.fileFolderRepo.FolderNameExists(ctx, organizationID, workspaceID, parentID, name, excludeFolderID)
	if err != nil {
		return fmt.Errorf("failed to check folder name: %w", err)
	}
	if exists {
		return ErrFolderNameConflict
	}
	return nil
}

// GetFolderByID gets a folder by its ID
func (s *fileResourceService) GetFolderByID(ctx context.Context, id string) (*file_model.FileFolder, error) {
	folder, err := s.fileFolderRepo.GetFolderByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	return folder, nil
}

// GetFolderWithPermissionCheck gets a folder and checks permission
func (s *fileResourceService) GetFolderWithPermissionCheck(ctx context.Context, id, accountID, tenantID string) (*file_model.FileFolder, error) {
	folder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	// Check permission
	hasPermission, err := s.CheckFolderEditorPermission(ctx, id, accountID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to check folder permission: %w", err)
	}

	if !hasPermission {
		return nil, fmt.Errorf("no permission to access folder")
	}

	return folder, nil
}

// GetFolderWithViewPermissionCheck gets a folder and checks browse permission.
func (s *fileResourceService) GetFolderWithViewPermissionCheck(ctx context.Context, id, accountID, tenantID string, visibleWorkspaceIDs []string) (*file_model.FileFolder, error) {
	folder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	hasPermission, err := s.CheckFolderViewPermission(ctx, id, accountID, tenantID, visibleWorkspaceIDs)
	if err != nil {
		return nil, err
	}
	if !hasPermission {
		return nil, fmt.Errorf("no permission to access folder")
	}

	return folder, nil
}

// CheckFolderViewPermission checks if a user can browse a folder.
func (s *fileResourceService) CheckFolderViewPermission(ctx context.Context, folderID, accountID, tenantID string, visibleWorkspaceIDs []string) (bool, error) {
	folder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, folderID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get folder: %w", err)
	}

	if folder.CreatedBy == accountID {
		return true, nil
	}

	if len(visibleWorkspaceIDs) == 0 {
		return false, nil
	}

	if folder.WorkspaceID != nil && !slices.Contains(visibleWorkspaceIDs, *folder.WorkspaceID) {
		return false, nil
	}

	switch file_model.FileFolderPermissionType(folder.Permission) {
	case file_model.FileFolderPermissionAllTeam:
		return true, nil
	case file_model.FileFolderPermissionOnlyMe:
		return false, nil
	case file_model.FileFolderPermissionPartialTeam:
		tenantIDs, err := s.fileFolderRepo.GetFolderPermissionTenants(ctx, folder.ID)
		if err != nil {
			return false, fmt.Errorf("failed to get folder permission tenants: %w", err)
		}
		for _, tenantWorkspaceID := range tenantIDs {
			if slices.Contains(visibleWorkspaceIDs, tenantWorkspaceID) {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

// UpdateFolder updates a folder with the given updates
func (s *fileResourceService) UpdateFolder(ctx context.Context, id string, updates map[string]interface{}) (*file_model.FileFolder, error) {
	// Validate permission if it's being updated
	if permission, ok := updates["permission"]; ok {
		var permissionStr string
		switch p := permission.(type) {
		case string:
			permissionStr = p
		case file_model.FileFolderPermissionType:
			permissionStr = string(p)
		default:
			return nil, fmt.Errorf("invalid permission type: %T", permission)
		}

		if !file_model.IsValidFileFolderPermission(permissionStr) {
			return nil, fmt.Errorf("invalid folder permission: %s", permissionStr)
		}
	}

	if needsFolderNameConflictCheck(updates) {
		existingFolder, err := s.fileFolderRepo.GetFolderByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get folder: %w", err)
		}

		nextName := existingFolder.Name
		if rawName, ok := updates["name"]; ok {
			name, ok := rawName.(string)
			if !ok {
				return nil, fmt.Errorf("invalid folder name type: %T", rawName)
			}
			nextName = strings.TrimSpace(name)
			updates["name"] = nextName
		}

		nextParentID := existingFolder.ParentID
		if rawParentID, ok := updates["parent_id"]; ok {
			parentID, ok := normalizeFolderIDUpdate(rawParentID)
			if !ok {
				return nil, fmt.Errorf("invalid parent folder type: %T", rawParentID)
			}
			nextParentID = parentID
			updates["parent_id"] = folderIDUpdateValue(parentID)
		}

		nextWorkspaceID := existingFolder.WorkspaceID
		if rawWorkspaceID, ok := updates["workspace_id"]; ok {
			workspaceID, ok := normalizeFolderIDUpdate(rawWorkspaceID)
			if !ok {
				return nil, fmt.Errorf("invalid workspace type: %T", rawWorkspaceID)
			}
			nextWorkspaceID = workspaceID
			updates["workspace_id"] = folderIDUpdateValue(workspaceID)
		}

		if err := s.ensureSiblingFolderNameAvailable(ctx, existingFolder.OrganizationID, nextWorkspaceID, nextParentID, nextName, &existingFolder.ID); err != nil {
			return nil, err
		}
	}

	folder, err := s.fileFolderRepo.UpdateFolder(ctx, id, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update folder: %w", err)
	}
	return folder, nil
}

// DeleteFolder deletes a folder
func (s *fileResourceService) DeleteFolder(ctx context.Context, id string) error {
	// TODO: Implement cascade delete or check for dependencies
	if err := s.fileFolderRepo.DeleteFolder(ctx, id); err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	return nil
}

// ListFolders lists folders
func (s *fileResourceService) ListFolders(ctx context.Context, tenantID string, page, limit int, keyword, sort, parentID string, workspaceIDs []string) ([]*file_model.FileFolder, int64, error) {
	if len(workspaceIDs) == 0 {
		return []*file_model.FileFolder{}, 0, nil
	}

	var folders []*file_model.FileFolder
	var total int64
	var err error

	if parentID != "" {
		folders, total, err = s.fileFolderRepo.ListFolders(ctx, tenantID, &parentID, page, limit, keyword, sort, workspaceIDs)
	} else {
		folders, total, err = s.fileFolderRepo.ListFolders(ctx, tenantID, nil, page, limit, keyword, sort, workspaceIDs)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list folders: %w", err)
	}

	return folders, total, nil
}

// ListFoldersWithPermissionFilter lists folders with permission filtering
func (s *fileResourceService) ListFoldersWithPermissionFilter(ctx context.Context, tenantID, accountID string, page, limit int, keyword, sort, parentID, workspaceID string, visibleWorkspaceIDs []string) ([]*file_model.FileFolder, int64, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return []*file_model.FileFolder{}, 0, nil
	}

	var parentIDPtr *string
	if parentID != "" {
		parentIDPtr = &parentID
	}

	folders, total, err := s.fileFolderRepo.ListFoldersWithPermissionFilter(ctx, tenantID, accountID, parentIDPtr, page, limit, keyword, sort, workspaceID, visibleWorkspaceIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list folders with permission filter: %w", err)
	}

	return folders, total, nil
}

// GetFolderFileCount gets the count of files in a folder
func (s *fileResourceService) GetFolderFileCount(ctx context.Context, folderID string) (int64, error) {
	return s.fileFolderRepo.GetFolderFileCount(ctx, folderID)
}

// AddFileToFolder adds a file to a folder
func (s *fileResourceService) AddFileToFolder(ctx context.Context, fileID, folderID, accountID string) error {
	// Check if file exists
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Add file to folder
	if err := s.fileFolderRepo.AddFileToFolder(ctx, fileID, folderID, accountID); err != nil {
		return fmt.Errorf("failed to add file to folder: %w", err)
	}

	return nil
}

// RemoveFileFromFolder removes a file from a folder
func (s *fileResourceService) RemoveFileFromFolder(ctx context.Context, fileID, folderID string) error {
	if err := s.fileFolderRepo.RemoveFileFromFolder(ctx, fileID, folderID); err != nil {
		return fmt.Errorf("failed to remove file from folder: %w", err)
	}
	return nil
}

// ListFilesInFolder lists files in a folder
func (s *fileResourceService) ListFilesInFolder(ctx context.Context, folderID string, page, limit int) ([]*file_model.UploadFile, int64, error) {
	files, total, err := s.fileFolderRepo.ListFilesInFolder(ctx, folderID, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list files in folder: %w", err)
	}
	return files, total, nil
}

// ListFilesInFolderWithFilters lists files in a folder with additional filters
func (s *fileResourceService) ListFilesInFolderWithFilters(ctx context.Context, folderID string, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return []*file_model.UploadFile{}, 0, nil
	}

	files, total, err := s.fileFolderRepo.ListFilesInFolderWithFiltersAndTenant(ctx, folderID, page, limit, keyword, sort, extension, processingStatus, startTime, endTime, tenantID, visibleWorkspaceIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list files in folder with filters: %w", err)
	}
	return files, total, nil
}

// ListAllFilesWithFilters lists all files with additional filters
func (s *fileResourceService) ListAllFilesWithFilters(ctx context.Context, page, limit int, keyword, sort, extension, processingStatus string, startTime, endTime *time.Time, tenantID, accountID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return []*file_model.UploadFile{}, 0, nil
	}

	allowAllFolders := false

	files, total, err := s.fileFolderRepo.ListAllFilesWithFiltersAndTenant(ctx, page, limit, keyword, sort, extension, processingStatus, startTime, endTime, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to list all files with filters",
			err,
			zap.String("tenant_id", tenantID),
			zap.String("account_id", accountID),
			zap.Int("visible_workspace_count", len(visibleWorkspaceIDs)),
		)
		return nil, 0, fmt.Errorf("failed to list all files with filters: %w", err)
	}
	return files, total, nil
}

// ListFavoriteFiles lists favorite files for an account with full file details
// Optimized to use a single JOIN query instead of two separate queries
func (s *fileResourceService) ListFavoriteFiles(ctx context.Context, accountID string, page, limit int, keyword, sort, extension string, startTime, endTime *time.Time, tenantID string, visibleWorkspaceIDs []string) ([]*file_model.UploadFile, int64, error) {
	// Use optimized method with JOIN query to get favorite files in a single database call
	if len(visibleWorkspaceIDs) == 0 {
		return []*file_model.UploadFile{}, 0, nil
	}

	allowAllFolders := false

	files, total, err := s.fileFolderRepo.ListFavoriteFilesWithFilters(ctx, accountID, page, limit, keyword, sort, extension, startTime, endTime, tenantID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list favorite files: %w", err)
	}

	return files, total, nil
}

// MoveFileToFolder moves a file from one folder to another
func (s *fileResourceService) MoveFileToFolder(ctx context.Context, fileID, fromFolderID, toFolderID, accountID string) error {
	// Check if file exists
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Move file to folder
	if err := s.fileFolderRepo.MoveFileToFolder(ctx, fileID, fromFolderID, toFolderID, accountID); err != nil {
		return fmt.Errorf("failed to move file to folder: %w", err)
	}

	return nil
}

// MoveFilesToFolder moves multiple files from their current folders to another folder
func (s *fileResourceService) MoveFilesToFolder(ctx context.Context, fileIDs []string, toFolderID, accountID string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		_, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}

		// Get the current folder ID for this file
		fromFolderID, err := s.GetFileFolderID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("failed to get current folder for file %s: %w", fileID, err)
		}

		// Move file to folder
		if err := s.fileFolderRepo.MoveFileToFolder(ctx, fileID, fromFolderID, toFolderID, accountID); err != nil {
			return fmt.Errorf("failed to move file %s to folder: %w", fileID, err)
		}
	}

	return nil
}

// ArchiveFiles archives multiple files
func (s *fileResourceService) ArchiveFiles(ctx context.Context, fileIDs []string, accountID string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		_, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
	}

	// Archive files
	for _, fileID := range fileIDs {
		updates := map[string]interface{}{
			"is_archived": true,
			"archived_at": time.Now(),
			"archived_by": accountID,
		}

		if err := s.fileRepo.Update(ctx, fileID, updates); err != nil {
			return fmt.Errorf("failed to archive file %s: %w", fileID, err)
		}
	}

	return nil
}

// UnarchiveFiles unarchives multiple files
func (s *fileResourceService) UnarchiveFiles(ctx context.Context, fileIDs []string, accountID string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		_, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
	}

	// Unarchive files
	for _, fileID := range fileIDs {
		updates := map[string]interface{}{
			"is_archived": false,
			"archived_at": nil,
			"archived_by": nil,
		}

		if err := s.fileRepo.Update(ctx, fileID, updates); err != nil {
			return fmt.Errorf("failed to unarchive file %s: %w", fileID, err)
		}
	}

	return nil
}

// MoveFolderToFolder moves a folder to another folder with validation
func (s *fileResourceService) MoveFolderToFolder(ctx context.Context, folderID, targetID, accountID, tenantID string) error {
	// Check if trying to move folder to itself
	if folderID == targetID {
		return fmt.Errorf("cannot move folder to itself")
	}

	// Get the folder to move
	folder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, folderID, tenantID)
	if err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}

	// Check if target is empty (moving to root)
	if targetID != "" {
		// Get the target folder
		_, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, targetID, tenantID)
		if err != nil {
			return fmt.Errorf("target folder not found: %w", err)
		}

		// Check if trying to move folder to its parent
		if folder.ParentID != nil && *folder.ParentID == targetID {
			return fmt.Errorf("folder is already in the target folder")
		}

		// Check if trying to move folder to its descendant (child, grandchild, etc.)
		if err := s.checkFolderMoveValidity(ctx, folderID, targetID, tenantID); err != nil {
			return fmt.Errorf("invalid folder move: %w", err)
		}
	}

	var targetParentID *string
	if targetID != "" {
		targetParentID = &targetID
	}
	if err := s.ensureSiblingFolderNameAvailable(ctx, tenantID, folder.WorkspaceID, targetParentID, folder.Name, &folder.ID); err != nil {
		return err
	}

	// Update the folder's parent ID
	updates := map[string]interface{}{}

	// Handle targetID for root folder case
	if targetID == "" {
		// When moving to root, set parent_id to nil
		updates["parent_id"] = nil
	} else {
		// When moving to another folder, set parent_id to targetID
		updates["parent_id"] = targetID
	}

	_, err = s.fileFolderRepo.UpdateFolder(ctx, folderID, updates)
	if err != nil {
		return fmt.Errorf("failed to move folder: %w", err)
	}

	return nil
}

// checkFolderMoveValidity checks if moving a folder to a target folder would create a cycle
func (s *fileResourceService) checkFolderMoveValidity(ctx context.Context, folderID, targetID, tenantID string) error {
	// Traverse up the target folder's hierarchy to check if folderID is an ancestor
	currentTargetID := targetID
	for currentTargetID != "" {
		targetFolder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, currentTargetID, tenantID)
		if err != nil {
			return fmt.Errorf("failed to get target folder: %w", err)
		}

		// If we've reached the folder we're trying to move, it would create a cycle
		if targetFolder.ID == folderID {
			return fmt.Errorf("cannot move folder to its own descendant")
		}

		// Move up to the parent
		if targetFolder.ParentID != nil {
			currentTargetID = *targetFolder.ParentID
		} else {
			break
		}
	}

	return nil
}

// CheckFolderEditorPermission checks if a user has editor permission for a folder
func (s *fileResourceService) CheckFolderEditorPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error) {
	folder, err := s.fileFolderRepo.GetFolderByIDAndTenant(ctx, folderID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get folder: %w", err)
	}

	// Check if user is the creator of the folder
	if folder.CreatedBy == accountID {
		return true, nil
	}

	return false, nil
}

// GetFolderPermissionTenants gets the list of tenant IDs that have permission to access a folder
func (s *fileResourceService) GetFolderPermissionTenants(ctx context.Context, folderID string) ([]string, error) {
	return s.fileFolderRepo.GetFolderPermissionTenants(ctx, folderID)
}

// GetFolderPermissionTenantDetails gets the list of tenants with details that have permission to access a folder
func (s *fileResourceService) GetFolderPermissionTenantDetails(ctx context.Context, folderID string) ([]dto.FileFolderPermissionTenantDetail, error) {
	// Get tenants with details from repository
	tenants, err := s.fileFolderRepo.GetFolderPermissionTenantsWithDetails(ctx, folderID)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	var details []dto.FileFolderPermissionTenantDetail
	for _, tenant := range tenants {
		details = append(details, dto.FileFolderPermissionTenantDetail{
			TenantID:      tenant.ID,
			TenantName:    tenant.Name,
			WorkspaceID:   tenant.ID,
			WorkspaceName: tenant.Name,
		})
	}

	return details, nil
}

// UpdatePartialWorkspaceList updates the list of tenants that have permission to access a folder with partial_team permission
func (s *fileResourceService) UpdatePartialWorkspaceList(ctx context.Context, folderID string, workspaceIDs []string, createdBy string) error {
	// First clear existing permissions
	if err := s.ClearPartialWorkspaceList(ctx, folderID); err != nil {
		return fmt.Errorf("failed to clear partial team list: %w", err)
	}

	// Then add new permissions
	for _, workspaceID := range workspaceIDs {
		permission := &file_model.FileFolderPermission{
			FolderID:    folderID,
			WorkspaceID: workspaceID,
			CreatedBy:   createdBy,
		}
		if err := s.fileFolderRepo.AddFolderPermission(ctx, permission); err != nil {
			return fmt.Errorf("failed to add folder permission for tenant %s: %w", workspaceID, err)
		}
	}

	return nil
}

// ClearPartialWorkspaceList clears all tenant permissions for a folder with partial_team permission
func (s *fileResourceService) ClearPartialWorkspaceList(ctx context.Context, folderID string) error {
	return s.fileFolderRepo.DeleteFolderPermissionsByFolderID(ctx, folderID)
}

// GetRelatedDocumentCount gets the count of documents associated with a file
func (s *fileResourceService) GetRelatedDocumentCount(ctx context.Context, fileID string) (int, error) {
	var count int64
	err := s.documentRepo.(*dataset_repo.DocumentRepositoryImpl).GetDB().WithContext(ctx).
		Table("data_library_document_assets AS assets").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.asset_id = assets.id AND refs.deleted_at IS NULL").
		Joins("JOIN datasets ON datasets.id = refs.dataset_id").
		Joins("JOIN documents ON documents.id = refs.dataset_document_id").
		Where("assets.source_file_id = ? AND assets.deleted_at IS NULL", fileID).
		Distinct("refs.dataset_document_id").
		Count(&count).Error

	if err != nil {
		logger.ErrorContext(ctx, "failed to get related document count",
			err,
			zap.String("file_id", fileID),
		)
		return 0, fmt.Errorf("failed to get related document count: %w", err)
	}

	return int(count), nil
}

// GetRelatedDatasetCount gets the count of datasets associated with a file through its documents
func (s *fileResourceService) GetRelatedDatasetCount(ctx context.Context, fileID string) (int, error) {
	var count int64
	err := s.documentRepo.(*dataset_repo.DocumentRepositoryImpl).GetDB().WithContext(ctx).
		Table("data_library_document_assets AS assets").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.asset_id = assets.id AND refs.deleted_at IS NULL").
		Joins("JOIN datasets ON datasets.id = refs.dataset_id").
		Where("assets.source_file_id = ? AND assets.deleted_at IS NULL", fileID).
		Distinct("refs.dataset_id").
		Count(&count).Error

	if err != nil {
		logger.ErrorContext(ctx, "failed to get related dataset count",
			err,
			zap.String("file_id", fileID),
		)
		return 0, fmt.Errorf("failed to get related dataset count: %w", err)
	}

	return int(count), nil
}

// GetRelatedDocuments gets information about documents associated with a file
func (s *fileResourceService) GetRelatedDocuments(ctx context.Context, fileID string) ([]*dataset_model.Document, error) {
	var documents []*dataset_model.Document
	err := s.documentRepo.(*dataset_repo.DocumentRepositoryImpl).GetDB().WithContext(ctx).
		Table("documents").
		Select("DISTINCT documents.*").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.dataset_document_id = documents.id AND refs.deleted_at IS NULL").
		Joins("JOIN datasets ON datasets.id = refs.dataset_id").
		Joins("JOIN data_library_document_assets AS assets ON assets.id = refs.asset_id AND assets.deleted_at IS NULL").
		Where("assets.source_file_id = ?", fileID).
		Find(&documents).Error

	if err != nil {
		logger.ErrorContext(ctx, "failed to get related documents",
			err,
			zap.String("file_id", fileID),
		)
		return nil, fmt.Errorf("failed to get related documents: %w", err)
	}

	// Get associated dataset information for each document
	for _, doc := range documents {
		dataset, err := s.datasetRepo.GetByID(ctx, doc.DatasetID)
		if err != nil {
			// If getting the dataset fails, log the error but don't interrupt the whole process
			logger.WarnContext(ctx, "failed to get dataset for related document",
				err,
				zap.String("dataset_id", doc.DatasetID),
				zap.String("document_id", doc.ID),
				zap.String("file_id", fileID),
			)
			continue
		}
		// Add the dataset name to the document's metadata
		if doc.DocMetadata == nil {
			doc.DocMetadata = make(dataset_model.JSONMap)
		}
		doc.DocMetadata["dataset_name"] = dataset.Name
	}

	return documents, nil
}

// GetRelatedDatasets gets information about datasets associated with a file through its documents
func (s *fileResourceService) GetRelatedDatasets(ctx context.Context, fileID string) ([]*dataset_model.Dataset, error) {
	var datasets []*dataset_model.Dataset
	err := s.documentRepo.(*dataset_repo.DocumentRepositoryImpl).GetDB().WithContext(ctx).
		Table("datasets").
		Select("DISTINCT datasets.*").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.dataset_id = datasets.id AND refs.deleted_at IS NULL").
		Joins("JOIN data_library_document_assets AS assets ON assets.id = refs.asset_id AND assets.deleted_at IS NULL").
		Where("assets.source_file_id = ?", fileID).
		Find(&datasets).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get related datasets: %w", err)
	}

	return datasets, nil
}

// GetFileFolderID gets the folder ID that a file currently belongs to
func (s *fileResourceService) GetFileFolderID(ctx context.Context, fileID string) (string, error) {
	// Query the file_folder_joins table to find which folder a file belongs to
	return s.fileFolderRepo.GetFileFolderID(ctx, fileID)
}

// GetTotalFileCount gets the total count of files for a tenant
func (s *fileResourceService) GetTotalFileCount(ctx context.Context, tenantID string) (int64, error) {
	return s.fileFolderRepo.GetTotalFileCount(ctx, tenantID)
}

func (s *fileResourceService) GetFileStatistics(ctx context.Context, tenantID, accountID string, visibleWorkspaceIDs []string) (*dto.FileStatisticsResponse, error) {
	if len(visibleWorkspaceIDs) == 0 {
		return &dto.FileStatisticsResponse{}, nil
	}

	allowAllFolders := false

	totalCount, err := s.fileFolderRepo.GetTotalFileCountWithVisibility(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get total file count: %w", err)
	}

	recentCount, err := s.fileFolderRepo.GetRecentFileCountWithVisibility(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent file count: %w", err)
	}
	if recentCount > RecentFilesLimit {
		recentCount = RecentFilesLimit
	}

	favoriteCount, err := s.fileFolderRepo.GetFavoriteFileCountWithVisibility(ctx, accountID, tenantID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorite file count: %w", err)
	}

	rootFolderCount, err := s.fileFolderRepo.GetRootFolderFileCountWithVisibility(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get root folder file count: %w", err)
	}

	archivedCount, err := s.fileFolderRepo.GetArchivedFileCountWithVisibility(ctx, tenantID, accountID, allowAllFolders, visibleWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get archived file count: %w", err)
	}

	return &dto.FileStatisticsResponse{
		TotalCount:      totalCount,
		RecentCount:     recentCount,
		FavoriteCount:   favoriteCount,
		RootFolderCount: rootFolderCount,
		ArchivedCount:   archivedCount,
	}, nil
}

// GetRecentFileCount gets the count of recent files (within last 3 months) for a tenant
// Note: This method counts all recent files without applying the RecentFilesLimit.
// For actual file listing with limit, use ListRecentFiles method.
func (s *fileResourceService) GetRecentFileCount(ctx context.Context, tenantID string) (int64, error) {
	return s.fileFolderRepo.GetRecentFileCount(ctx, tenantID)
}

// GetRecentFileCountWithLimit gets the count of recent files (within last 3 months) for a tenant
// with the same limit logic used in ListRecentFiles handler.
func (s *fileResourceService) GetRecentFileCountWithLimit(ctx context.Context, tenantID string) (int64, error) {
	// Get the actual count
	count, err := s.fileFolderRepo.GetRecentFileCount(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	// Apply the same limit as in ListRecentFiles handler
	if count > RecentFilesLimit {
		return int64(RecentFilesLimit), nil
	}

	return count, nil
}

// GetFavoriteFileCount gets the count of favorite files for an account
func (s *fileResourceService) GetFavoriteFileCount(ctx context.Context, accountID, tenantID string) (int64, error) {
	return s.fileFolderRepo.GetFavoriteFileCount(ctx, accountID, tenantID)
}

// GetRootFolderFileCount gets the count of files in the root folder (not in any folder) for a tenant
func (s *fileResourceService) GetRootFolderFileCount(ctx context.Context, tenantID string) (int64, error) {
	return s.fileFolderRepo.GetRootFolderFileCount(ctx, tenantID)
}

// GetArchivedFileCount gets the count of archived files for a tenant
func (s *fileResourceService) GetArchivedFileCount(ctx context.Context, tenantID string) (int64, error) {
	return s.fileFolderRepo.GetArchivedFileCount(ctx, tenantID)
}

// BatchGetRelatedDatasetCount gets the count of datasets associated with multiple files through active knowledge-base asset refs.
func (s *fileResourceService) BatchGetRelatedDatasetCount(ctx context.Context, fileIDs []string) (map[string]int, error) {
	logger.DebugContext(ctx, "batch get related dataset count started",
		zap.Int("file_count", len(fileIDs)),
	)
	startTime := time.Now()

	if len(fileIDs) == 0 {
		logger.DebugContext(ctx, "batch get related dataset count skipped for empty file list")
		return make(map[string]int), nil
	}

	// Define a struct for the query result
	type result struct {
		FileID       string
		DatasetCount int
	}

	var results []result
	err := s.documentRepo.(*dataset_repo.DocumentRepositoryImpl).GetDB().WithContext(ctx).
		Table("data_library_document_assets AS assets").
		Select("assets.source_file_id AS file_id, COUNT(DISTINCT refs.dataset_id) AS dataset_count").
		Joins("JOIN data_library_knowledge_base_asset_refs AS refs ON refs.asset_id = assets.id AND refs.deleted_at IS NULL").
		Joins("JOIN datasets ON datasets.id = refs.dataset_id").
		Where("assets.source_file_id IN ? AND assets.deleted_at IS NULL", fileIDs).
		Group("assets.source_file_id").
		Scan(&results).Error

	if err != nil {
		logger.ErrorContext(ctx, "failed to batch get related dataset counts",
			err,
			zap.Int("file_count", len(fileIDs)),
		)
		return nil, fmt.Errorf("failed to batch get related dataset counts: %w", err)
	}

	// Convert results to map
	resultMap := make(map[string]int)
	for _, r := range results {
		resultMap[r.FileID] = r.DatasetCount
	}

	// Ensure all file IDs have an entry (set to 0 for files with no related datasets)
	zeroCountFiles := 0
	for _, fileID := range fileIDs {
		if _, exists := resultMap[fileID]; !exists {
			resultMap[fileID] = 0
			zeroCountFiles++
		}
	}

	if zeroCountFiles > 0 {
		logger.DebugContext(ctx, "set zero related dataset count for files",
			zap.Int("zero_count_files", zeroCountFiles),
		)
	}

	duration := time.Since(startTime)
	logger.DebugContext(ctx, "batch get related dataset count completed",
		zap.Int("file_count", len(fileIDs)),
		zap.Int64("latency_ms", duration.Milliseconds()),
	)

	return resultMap, nil
}
