package service

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/internal/modules/dataset/repository"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
)

// DatasetFolderService defines the interface for dataset folder operations
type DatasetFolderService interface {
	// CreateFolder creates a new dataset folder
	CreateFolder(ctx context.Context, folder *model.DatasetFolder) (*model.DatasetFolder, error)

	// GetFolderByID retrieves a folder by its ID
	GetFolderByID(ctx context.Context, folderID string) (*model.DatasetFolder, error)

	// GetFoldersByTenant retrieves folders for a specific tenant with pagination
	GetFoldersByTenant(ctx context.Context, tenantID string, page, limit int) ([]*model.DatasetFolder, int64, error)

	// GetFoldersByWorkspaceIDs retrieves folders for tenant id list with pagination
	GetFoldersByWorkspaceIDs(ctx context.Context, tenantID []string, page, limit int) ([]*model.DatasetFolder, int64, error)

	// GetFoldersByParent retrieves folders under a specific parent folder
	GetFoldersByParent(ctx context.Context, parentID *string, tenantIDs []string) ([]*model.DatasetFolder, error)

	// UpdateFolder updates a folder's information
	UpdateFolder(ctx context.Context, folderID string, updateData map[string]interface{}) (*model.DatasetFolder, error)

	// DeleteFolder deletes a folder
	DeleteFolder(ctx context.Context, folderID string) error

	// CheckFolderPermission checks if a user has permission to access a folder
	CheckFolderPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error)

	// CheckFolderEditorPermission checks if a user has permission to edit a folder
	CheckFolderEditorPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error)

	// AddDatasetToFolder adds a dataset to a folder
	AddDatasetToFolder(ctx context.Context, datasetID, folderID, createdBy string) error

	// RemoveDatasetFromFolder removes a dataset from a folder
	RemoveDatasetFromFolder(ctx context.Context, datasetID, folderID string) error

	// GetDatasetsInFolder retrieves datasets in a specific folder
	GetDatasetsInFolder(ctx context.Context, folderID string, page, limit int) ([]*model.Dataset, error)

	// GetFoldersForDataset retrieves all folders that contain a specific dataset
	GetFoldersForDataset(ctx context.Context, datasetID string) ([]*model.DatasetFolder, error)

	// SetDefaultFolderForDataset sets the default folder for a dataset
	SetDefaultFolderForDataset(ctx context.Context, datasetID, folderID string) error

	// GetDefaultFolderForDataset gets the default folder for a dataset
	GetDefaultFolderForDataset(ctx context.Context, datasetID string) (*model.DatasetFolder, error)

	// ListDatasetsWithFolders lists datasets with folder information
	ListDatasetsWithFolders(ctx context.Context, tenantIDs []string, folderID string, keyword, sort string) (*dto.DatasetListWithFoldersResponse, error)

	// ListDatasetsWithFoldersPaginated lists datasets with folder information with pagination support
	ListDatasetsWithFoldersPaginated(ctx context.Context, tenantIDs []string, folderID string, page, limit int, keyword, sort string) (*dto.DatasetListWithFoldersPaginatedResponse, error)

	// ListFoldersWithPagination lists folders with pagination support
	ListFoldersWithPagination(ctx context.Context, tenantIDs []string, parentID string, page, limit int, sort string, keyword string) (*dto.DatasetFolderPaginationResponse, error)

	// ListFoldersWithPaginationWithPermissions lists folders with pagination and permission filtering
	ListFoldersWithPaginationWithPermissions(ctx context.Context, organizationID string, tenantIDs []string, parentID string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, sort string, keyword string) (*dto.DatasetFolderPaginationResponse, error)

	// ListDatasetsWithPagination lists datasets with pagination support
	ListDatasetsWithPagination(ctx context.Context, tenantIDs []string, folderID string, page, limit int, keyword, sort string) (*dto.DatasetListResponse, error)

	// ListDatasetsWithPaginationWithPermissions lists datasets with pagination and permission filtering
	ListDatasetsWithPaginationWithPermissions(ctx context.Context, organizationID string, tenantIDs []string, folderID string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, keyword, sort string) (*dto.DatasetListResponse, error)

	// MoveDatasetToFolder moves a dataset to a folder
	MoveDatasetToFolder(ctx context.Context, datasetID, folderID, accountID string) error
}

type DatasetFolderServiceImpl struct {
	folderRepo     repository.DatasetFolderRepository
	accountService interfaces.AccountService
	tenantService  interfaces.WorkspaceManagementService
}

func NewDatasetFolderService(
	folderRepo repository.DatasetFolderRepository,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
) DatasetFolderService {
	return &DatasetFolderServiceImpl{
		folderRepo:     folderRepo,
		accountService: accountService,
		tenantService:  tenantService,
	}
}

// CreateFolder
func (s *DatasetFolderServiceImpl) CreateFolder(ctx context.Context, folder *model.DatasetFolder) (*model.DatasetFolder, error) {
	if err := s.folderRepo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	return folder, nil
}

// GetFolderByID
func (s *DatasetFolderServiceImpl) GetFolderByID(ctx context.Context, folderID string) (*model.DatasetFolder, error) {
	folder, err := s.folderRepo.GetFolderByID(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder by ID: %w", err)
	}
	return folder, nil
}

// GetFoldersByTenant
func (s *DatasetFolderServiceImpl) GetFoldersByTenant(ctx context.Context, tenantID string, page, limit int) ([]*model.DatasetFolder, int64, error) {
	folders, total, err := s.folderRepo.GetFoldersByTenant(ctx, tenantID, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get folders by tenant: %w", err)
	}
	return folders, total, nil
}

func (s *DatasetFolderServiceImpl) GetFoldersByWorkspaceIDs(ctx context.Context, workspaceIDs []string, page, limit int) ([]*model.DatasetFolder, int64, error) {
	folders, total, err := s.folderRepo.GetFoldersByWorkspaceIDs(ctx, workspaceIDs, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get folders by tenant: %w", err)
	}
	return folders, total, nil
}

// GetFoldersByParent
func (s *DatasetFolderServiceImpl) GetFoldersByParent(ctx context.Context, parentID *string, tenantIDs []string) ([]*model.DatasetFolder, error) {
	folders, err := s.folderRepo.GetFoldersByParent(ctx, parentID, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders by parent: %w", err)
	}
	return folders, nil
}

// UpdateFolder
func (s *DatasetFolderServiceImpl) UpdateFolder(ctx context.Context, folderID string, updateData map[string]interface{}) (*model.DatasetFolder, error) {
	folder, err := s.folderRepo.GetFolderByID(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	for key, value := range updateData {
		switch key {
		case "name":
			if name, ok := value.(string); ok {
				folder.Name = name
			}
		case "description":
			if description, ok := value.(*string); ok {
				folder.Description = description
			}
		case "parent_id":
			if parentID, ok := value.(*string); ok {
				folder.ParentID = parentID
			}
		case "icon":
			if icon, ok := value.(*string); ok {
				folder.Icon = icon
			}
		case "icon_type":
			if iconType, ok := value.(*string); ok {
				folder.IconType = iconType
			}
		case "icon_background":
			if iconBackground, ok := value.(*string); ok {
				folder.IconBackground = iconBackground
			}
		case "position":
			if position, ok := value.(int); ok {
				folder.Position = position
			}
		case "permission":
			if permission, ok := value.(string); ok {
				folder.Permission = permission
			}
		case "tenant_id":
			if tenantID, ok := value.(string); ok {
				folder.WorkspaceID = tenantID
			}
		}
	}

	if err := s.folderRepo.UpdateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("failed to update folder: %w", err)
	}

	return folder, nil
}

// DeleteFolder
func (s *DatasetFolderServiceImpl) DeleteFolder(ctx context.Context, folderID string) error {
	_, err := s.folderRepo.GetFolderByID(ctx, folderID)
	if err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}

	if err := s.folderRepo.DeleteFolder(ctx, folderID); err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	return nil
}

// CheckFolderPermission
func (s *DatasetFolderServiceImpl) CheckFolderPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error) {
	hasPermission, err := s.folderRepo.CheckFolderPermission(ctx, folderID, accountID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to check folder permission: %w", err)
	}
	return hasPermission, nil
}

// CheckFolderEditorPermission
func (s *DatasetFolderServiceImpl) CheckFolderEditorPermission(ctx context.Context, folderID, accountID, tenantID string) (bool, error) {
	hasPermission, err := s.folderRepo.CheckFolderEditorPermission(ctx, folderID, accountID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to check folder editor permission: %w", err)
	}
	return hasPermission, nil
}

// AddDatasetToFolder
func (s *DatasetFolderServiceImpl) AddDatasetToFolder(ctx context.Context, datasetID, folderID, createdBy string) error {
	if err := s.folderRepo.AddDatasetToFolder(ctx, datasetID, folderID, createdBy); err != nil {
		return fmt.Errorf("failed to add dataset to folder: %w", err)
	}
	return nil
}

// RemoveDatasetFromFolder
func (s *DatasetFolderServiceImpl) RemoveDatasetFromFolder(ctx context.Context, datasetID, folderID string) error {
	if err := s.folderRepo.RemoveDatasetFromFolder(ctx, datasetID, folderID); err != nil {
		return fmt.Errorf("failed to remove dataset from folder: %w", err)
	}
	return nil
}

// GetDatasetsInFolder
func (s *DatasetFolderServiceImpl) GetDatasetsInFolder(ctx context.Context, folderID string, page, limit int) ([]*model.Dataset, error) {
	datasets, _, err := s.folderRepo.GetDatasetsInFolder(ctx, folderID, page, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets in folder: %w", err)
	}
	return datasets, nil
}

// GetFoldersForDataset
func (s *DatasetFolderServiceImpl) GetFoldersForDataset(ctx context.Context, datasetID string) ([]*model.DatasetFolder, error) {
	folders, err := s.folderRepo.GetFoldersForDataset(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders for dataset: %w", err)
	}
	return folders, nil
}

// SetDefaultFolderForDataset
func (s *DatasetFolderServiceImpl) SetDefaultFolderForDataset(ctx context.Context, datasetID, folderID string) error {
	if err := s.folderRepo.SetDefaultFolderForDataset(ctx, datasetID, folderID); err != nil {
		return fmt.Errorf("failed to set default folder for dataset: %w", err)
	}
	return nil
}

// GetDefaultFolderForDataset
func (s *DatasetFolderServiceImpl) GetDefaultFolderForDataset(ctx context.Context, datasetID string) (*model.DatasetFolder, error) {
	folder, err := s.folderRepo.GetDefaultFolderForDataset(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default folder for dataset: %w", err)
	}
	return folder, nil
}

// MoveDatasetToFolder moves a dataset to a folder
func (s *DatasetFolderServiceImpl) MoveDatasetToFolder(ctx context.Context, datasetID, folderID, accountID string) error {
	// First remove the dataset from any existing folder associations
	if err := s.folderRepo.RemoveAllFolderAssociationsForDataset(ctx, datasetID); err != nil {
		return fmt.Errorf("failed to remove existing folder associations for dataset: %w", err)
	}

	// If folderID is empty, we're moving to root (no folder association needed)
	if folderID == "" {
		return nil
	}

	// Add the dataset to the new folder
	if err := s.folderRepo.AddDatasetToFolder(ctx, datasetID, folderID, accountID); err != nil {
		return fmt.Errorf("failed to add dataset to folder: %w", err)
	}

	return nil
}

// ListDatasetsWithFolders lists datasets with folder information like a file explorer
func (s *DatasetFolderServiceImpl) ListDatasetsWithFolders(ctx context.Context, tenantIDs []string, folderID string, keyword, sort string) (*dto.DatasetListWithFoldersResponse, error) {
	// Get datasets in the specified folder
	datasets, err := s.folderRepo.GetDatasetsInFolderByID(ctx, folderID, tenantIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets in folder: %w", err)
	}

	// Get subfolders of the specified folder
	var folders []*model.DatasetFolder
	if folderID == "" {
		// For root folder, get folders with nil parent
		folders, err = s.folderRepo.GetFoldersByParent(ctx, nil, tenantIDs)
	} else {
		// For specific folder, get subfolders
		folders, err = s.folderRepo.GetFoldersByParent(ctx, &folderID, tenantIDs)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get subfolders: %w", err)
	}

	// Get current folder information
	var currentFolder *model.DatasetFolder
	if folderID != "" {
		currentFolder, err = s.folderRepo.GetFolderByID(ctx, folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get current folder: %w", err)
		}
	}

	// Convert to response format
	folderResponses := make([]dto.DatasetFolderResponse, 0, len(folders))
	for _, folder := range folders {
		folderResponses = append(folderResponses, dto.DatasetFolderResponse{
			ID:             folder.ID,
			WorkspaceID:    folder.WorkspaceID,
			Name:           folder.Name,
			Description:    folder.Description,
			ParentID:       folder.ParentID,
			CreatedBy:      folder.CreatedBy,
			CreatedAt:      folder.CreatedAt,
			UpdatedBy:      folder.UpdatedBy,
			UpdatedAt:      folder.UpdatedAt,
			Icon:           folder.Icon,
			IconType:       folder.IconType,
			IconBackground: folder.IconBackground,
			Position:       folder.Position,
			Permission:     folder.Permission,
		})
	}

	// Convert datasets to response format with folder information
	datasetResponses := make([]dto.DatasetResponse, 0, len(datasets))
	for _, dataset := range datasets {
		// Convert dataset to response
		datasetResponse := dto.DatasetResponse{
			ID:                     dataset.ID,
			WorkspaceID:            dataset.WorkspaceID,
			Name:                   dataset.Name,
			Description:            dataset.Description,
			Provider:               dataset.Provider,
			CreatedBy:              dataset.CreatedBy,
			CreatedAt:              dataset.CreatedAt,
			UpdatedBy:              dataset.UpdatedBy,
			UpdatedAt:              dataset.UpdatedAt,
			Owner:                  dataset.Owner,
			EmbeddingModel:         dataset.EmbeddingModel,
			EmbeddingModelProvider: dataset.EmbeddingModelProvider,
			CollectionBindingID:    dataset.CollectionBindingID,
			Icon:                   dataset.Icon,
			IconType:               dataset.IconType,
			IconBackground:         dataset.IconBackground,
			AppCount:               dataset.AppCount,
			DocumentCount:          dataset.DocumentCount,
			WordCount:              dataset.WordCount,
			EmbeddingAvailable:     true,
			PartialMemberList:      []interface{}{},
			Tags:                   dataset.Tags,
			DocForm:                dataset.DocForm,
			EnableGraphFlow:        dataset.EnableGraphFlow,
		}

		datasetResponses = append(datasetResponses, datasetResponse)
	}

	response := &dto.DatasetListWithFoldersResponse{
		Datasets:      datasetResponses,
		Folders:       folderResponses,
		CurrentFolder: nil,
	}

	if currentFolder != nil {
		currentFolderResponse := &dto.DatasetFolderResponse{
			ID:             currentFolder.ID,
			WorkspaceID:    currentFolder.WorkspaceID,
			Name:           currentFolder.Name,
			Description:    currentFolder.Description,
			ParentID:       currentFolder.ParentID,
			CreatedBy:      currentFolder.CreatedBy,
			CreatedAt:      currentFolder.CreatedAt,
			UpdatedBy:      currentFolder.UpdatedBy,
			UpdatedAt:      currentFolder.UpdatedAt,
			Icon:           currentFolder.Icon,
			IconType:       currentFolder.IconType,
			IconBackground: currentFolder.IconBackground,
			Position:       currentFolder.Position,
			Permission:     currentFolder.Permission,
		}
		response.CurrentFolder = currentFolderResponse
	}

	return response, nil
}

// ListDatasetsWithFoldersPaginated lists datasets with folder information with pagination support
func (s *DatasetFolderServiceImpl) ListDatasetsWithFoldersPaginated(ctx context.Context, tenantIDs []string, folderID string, page, limit int, keyword, sort string) (*dto.DatasetListWithFoldersPaginatedResponse, error) {
	// Default page and limit if not provided
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	// Get datasets in the specified folder with pagination
	datasets, datasetTotal, err := s.folderRepo.GetDatasetsInFolderByIDWithPagination(ctx, folderID, tenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets in folder: %w", err)
	}

	// Get subfolders of the specified folder with pagination
	var parentIDPtr *string
	if folderID != "" {
		parentIDPtr = &folderID
	}

	folders, folderTotal, err := s.folderRepo.GetFoldersByParentWithPagination(ctx, parentIDPtr, tenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get subfolders: %w", err)
	}

	// Get current folder information
	var currentFolder *model.DatasetFolder
	if folderID != "" {
		currentFolder, err = s.folderRepo.GetFolderByID(ctx, folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get current folder: %w", err)
		}
	}

	// Convert to response format
	folderResponses := make([]dto.DatasetFolderResponse, 0, len(folders))
	for _, folder := range folders {
		folderResponses = append(folderResponses, dto.DatasetFolderResponse{
			ID:             folder.ID,
			WorkspaceID:    folder.WorkspaceID,
			Name:           folder.Name,
			Description:    folder.Description,
			ParentID:       folder.ParentID,
			CreatedBy:      folder.CreatedBy,
			CreatedAt:      folder.CreatedAt,
			UpdatedBy:      folder.UpdatedBy,
			UpdatedAt:      folder.UpdatedAt,
			Icon:           folder.Icon,
			IconType:       folder.IconType,
			IconBackground: folder.IconBackground,
			Position:       folder.Position,
			Permission:     folder.Permission,
		})
	}

	// Convert datasets to response format with folder information
	datasetResponses := make([]dto.DatasetResponse, 0, len(datasets))
	for _, dataset := range datasets {
		// Convert dataset to response
		datasetResponse := dto.DatasetResponse{
			ID:                     dataset.ID,
			WorkspaceID:            dataset.WorkspaceID,
			Name:                   dataset.Name,
			Description:            dataset.Description,
			Provider:               dataset.Provider,
			CreatedBy:              dataset.CreatedBy,
			CreatedAt:              dataset.CreatedAt,
			UpdatedBy:              dataset.UpdatedBy,
			UpdatedAt:              dataset.UpdatedAt,
			Owner:                  dataset.Owner,
			EmbeddingModel:         dataset.EmbeddingModel,
			EmbeddingModelProvider: dataset.EmbeddingModelProvider,
			CollectionBindingID:    dataset.CollectionBindingID,
			Icon:                   dataset.Icon,
			IconType:               dataset.IconType,
			IconBackground:         dataset.IconBackground,
			AppCount:               dataset.AppCount,
			DocumentCount:          dataset.DocumentCount,
			WordCount:              dataset.WordCount,
			EmbeddingAvailable:     true,
			PartialMemberList:      []interface{}{},
			Tags:                   dataset.Tags,
			DocForm:                dataset.DocForm,
			EnableGraphFlow:        dataset.EnableGraphFlow,
		}

		datasetResponses = append(datasetResponses, datasetResponse)
	}

	// Calculate has more flags
	datasetHasMore := int64(page*limit) < datasetTotal
	folderHasMore := int64(page*limit) < folderTotal

	response := &dto.DatasetListWithFoldersPaginatedResponse{
		Datasets:       datasetResponses,
		DatasetTotal:   datasetTotal,
		DatasetPage:    page,
		DatasetLimit:   limit,
		DatasetHasMore: datasetHasMore,
		Folders:        folderResponses,
		FolderTotal:    folderTotal,
		FolderPage:     page,
		FolderLimit:    limit,
		FolderHasMore:  folderHasMore,
		CurrentFolder:  nil,
	}

	if currentFolder != nil {
		currentFolderResponse := &dto.DatasetFolderResponse{
			ID:             currentFolder.ID,
			WorkspaceID:    currentFolder.WorkspaceID,
			Name:           currentFolder.Name,
			Description:    currentFolder.Description,
			ParentID:       currentFolder.ParentID,
			CreatedBy:      currentFolder.CreatedBy,
			CreatedAt:      currentFolder.CreatedAt,
			UpdatedBy:      currentFolder.UpdatedBy,
			UpdatedAt:      currentFolder.UpdatedAt,
			Icon:           currentFolder.Icon,
			IconType:       currentFolder.IconType,
			IconBackground: currentFolder.IconBackground,
			Position:       currentFolder.Position,
			Permission:     currentFolder.Permission,
		}
		response.CurrentFolder = currentFolderResponse
	}

	return response, nil
}

// ListFoldersWithPagination lists folders with pagination support
func (s *DatasetFolderServiceImpl) ListFoldersWithPagination(ctx context.Context, tenantIDs []string, parentID string, page, limit int, sort string, keyword string) (*dto.DatasetFolderPaginationResponse, error) {
	// Default page and limit if not provided
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	var parentIDPtr *string
	if parentID != "" {
		parentIDPtr = &parentID
	}

	// Get folders with pagination
	folders, total, err := s.folderRepo.GetFoldersByParentWithPagination(ctx, parentIDPtr, tenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders: %w", err)
	}

	// Get parent folder information
	var parentFolder *model.DatasetFolder
	if parentID != "" {
		parentFolder, err = s.folderRepo.GetFolderByID(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent folder: %w", err)
		}
	}

	// Convert to response format
	folderResponses := make([]dto.DatasetFolderResponse, 0, len(folders))
	for _, folder := range folders {
		folderResponses = append(folderResponses, dto.DatasetFolderResponse{
			ID:             folder.ID,
			WorkspaceID:    folder.WorkspaceID,
			Name:           folder.Name,
			Description:    folder.Description,
			ParentID:       folder.ParentID,
			CreatedBy:      folder.CreatedBy,
			CreatedAt:      folder.CreatedAt,
			UpdatedBy:      folder.UpdatedBy,
			UpdatedAt:      folder.UpdatedAt,
			Icon:           folder.Icon,
			IconType:       folder.IconType,
			IconBackground: folder.IconBackground,
			Position:       folder.Position,
			Permission:     folder.Permission,
		})
	}

	// Calculate has more flag
	hasMore := int64(page*limit) < total

	response := &dto.DatasetFolderPaginationResponse{
		Folders:      folderResponses,
		Total:        total,
		Page:         page,
		Limit:        limit,
		HasMore:      hasMore,
		ParentFolder: nil,
	}

	if parentFolder != nil {
		parentFolderResponse := &dto.DatasetFolderResponse{
			ID:             parentFolder.ID,
			WorkspaceID:    parentFolder.WorkspaceID,
			Name:           parentFolder.Name,
			Description:    parentFolder.Description,
			ParentID:       parentFolder.ParentID,
			CreatedBy:      parentFolder.CreatedBy,
			CreatedAt:      parentFolder.CreatedAt,
			UpdatedBy:      parentFolder.UpdatedBy,
			UpdatedAt:      parentFolder.UpdatedAt,
			Icon:           parentFolder.Icon,
			IconType:       parentFolder.IconType,
			IconBackground: parentFolder.IconBackground,
			Position:       parentFolder.Position,
			Permission:     parentFolder.Permission,
		}
		response.ParentFolder = parentFolderResponse
	}

	return response, nil
}

// ListFoldersWithPaginationWithPermissions lists folders with pagination and permission filtering
func (s *DatasetFolderServiceImpl) ListFoldersWithPaginationWithPermissions(ctx context.Context, organizationID string, tenantIDs []string, parentID string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, sort string, keyword string) (*dto.DatasetFolderPaginationResponse, error) {
	// Default page and limit if not provided
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	var parentIDPtr *string
	if parentID != "" {
		parentIDPtr = &parentID
	}

	// Get folders with pagination and permission filtering
	folders, total, err := s.folderRepo.GetFoldersByParentWithPaginationWithPermissions(ctx, parentIDPtr, organizationID, tenantIDs, accountID, isGroupAdmin, allGroupTenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders: %w", err)
	}

	// Get parent folder information
	var parentFolder *model.DatasetFolder
	if parentID != "" {
		parentFolder, err = s.folderRepo.GetFolderByID(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent folder: %w", err)
		}
	}

	// Convert to response format
	folderResponses := make([]dto.DatasetFolderResponse, 0, len(folders))
	for _, folder := range folders {
		folderResponses = append(folderResponses, dto.DatasetFolderResponse{
			ID:             folder.ID,
			WorkspaceID:    folder.WorkspaceID,
			Name:           folder.Name,
			Description:    folder.Description,
			ParentID:       folder.ParentID,
			CreatedBy:      folder.CreatedBy,
			CreatedAt:      folder.CreatedAt,
			UpdatedBy:      folder.UpdatedBy,
			UpdatedAt:      folder.UpdatedAt,
			Icon:           folder.Icon,
			IconType:       folder.IconType,
			IconBackground: folder.IconBackground,
			Position:       folder.Position,
			Permission:     folder.Permission,
		})
	}

	// Calculate has more flag
	hasMore := int64(page*limit) < total

	response := &dto.DatasetFolderPaginationResponse{
		Folders:      folderResponses,
		Total:        total,
		Page:         page,
		Limit:        limit,
		HasMore:      hasMore,
		ParentFolder: nil,
	}

	if parentFolder != nil {
		parentFolderResponse := &dto.DatasetFolderResponse{
			ID:             parentFolder.ID,
			WorkspaceID:    parentFolder.WorkspaceID,
			Name:           parentFolder.Name,
			Description:    parentFolder.Description,
			ParentID:       parentFolder.ParentID,
			CreatedBy:      parentFolder.CreatedBy,
			CreatedAt:      parentFolder.CreatedAt,
			UpdatedBy:      parentFolder.UpdatedBy,
			UpdatedAt:      parentFolder.UpdatedAt,
			Icon:           parentFolder.Icon,
			IconType:       parentFolder.IconType,
			IconBackground: parentFolder.IconBackground,
			Position:       parentFolder.Position,
			Permission:     parentFolder.Permission,
		}
		response.ParentFolder = parentFolderResponse
	}

	return response, nil
}

// ListDatasetsWithPagination lists datasets with pagination support
func (s *DatasetFolderServiceImpl) ListDatasetsWithPagination(ctx context.Context, tenantIDs []string, folderID string, page, limit int, keyword, sort string) (*dto.DatasetListResponse, error) {
	// Default page and limit if not provided
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	// Get datasets with pagination
	datasets, total, err := s.folderRepo.GetDatasetsInFolderByIDWithPagination(ctx, folderID, tenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets in folder: %w", err)
	}

	// Get folder information
	if folderID != "" {
		_, err = s.folderRepo.GetFolderByID(ctx, folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get folder: %w", err)
		}
	}

	// Convert datasets to response format
	datasetResponses := make([]dto.DatasetResponse, 0, len(datasets))
	for _, dataset := range datasets {
		// Convert dataset to response
		datasetResponse := dto.DatasetResponse{
			ID:                     dataset.ID,
			WorkspaceID:            dataset.WorkspaceID,
			Name:                   dataset.Name,
			Description:            dataset.Description,
			Provider:               dataset.Provider,
			CreatedBy:              dataset.CreatedBy,
			CreatedAt:              dataset.CreatedAt,
			UpdatedBy:              dataset.UpdatedBy,
			UpdatedAt:              dataset.UpdatedAt,
			Owner:                  dataset.Owner,
			EmbeddingModel:         dataset.EmbeddingModel,
			EmbeddingModelProvider: dataset.EmbeddingModelProvider,
			CollectionBindingID:    dataset.CollectionBindingID,
			Icon:                   dataset.Icon,
			IconType:               dataset.IconType,
			IconBackground:         dataset.IconBackground,
			AppCount:               dataset.AppCount,
			DocumentCount:          dataset.DocumentCount,
			WordCount:              dataset.WordCount,
			EmbeddingAvailable:     true,
			PartialMemberList:      []interface{}{},
			Tags:                   dataset.Tags,
			DocForm:                dataset.DocForm,
			EnableGraphFlow:        dataset.EnableGraphFlow,
		}

		datasetResponses = append(datasetResponses, datasetResponse)
	}

	// Calculate has more flag
	hasMore := int64(page*limit) < total

	response := &dto.DatasetListResponse{
		Data:    datasetResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: hasMore,
	}

	return response, nil
}

// ListDatasetsWithPaginationWithPermissions lists datasets with pagination and permission filtering
func (s *DatasetFolderServiceImpl) ListDatasetsWithPaginationWithPermissions(ctx context.Context, organizationID string, tenantIDs []string, folderID string, accountID string, isGroupAdmin bool, allGroupTenantIDs []string, page, limit int, keyword, sort string) (*dto.DatasetListResponse, error) {
	// Default page and limit if not provided
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	// Get datasets with pagination and permission filtering
	datasets, total, err := s.folderRepo.GetDatasetsInFolderByIDWithPaginationWithPermissions(ctx, folderID, organizationID, tenantIDs, accountID, isGroupAdmin, allGroupTenantIDs, page, limit, keyword)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets in folder: %w", err)
	}

	// Get folder information
	if folderID != "" {
		_, err = s.folderRepo.GetFolderByID(ctx, folderID)
		if err != nil {
			return nil, fmt.Errorf("failed to get folder: %w", err)
		}
	}

	// Convert datasets to response format
	datasetResponses := make([]dto.DatasetResponse, 0, len(datasets))
	for _, dataset := range datasets {
		// Convert dataset to response
		datasetResponse := dto.DatasetResponse{
			ID:                     dataset.ID,
			WorkspaceID:            dataset.WorkspaceID,
			Name:                   dataset.Name,
			Description:            dataset.Description,
			Provider:               dataset.Provider,
			CreatedBy:              dataset.CreatedBy,
			CreatedAt:              dataset.CreatedAt,
			UpdatedBy:              dataset.UpdatedBy,
			UpdatedAt:              dataset.UpdatedAt,
			Owner:                  dataset.Owner,
			EmbeddingModel:         dataset.EmbeddingModel,
			EmbeddingModelProvider: dataset.EmbeddingModelProvider,
			CollectionBindingID:    dataset.CollectionBindingID,
			Icon:                   dataset.Icon,
			IconType:               dataset.IconType,
			IconBackground:         dataset.IconBackground,
			AppCount:               dataset.AppCount,
			DocumentCount:          dataset.DocumentCount,
			WordCount:              dataset.WordCount,
			EmbeddingAvailable:     true,
			PartialMemberList:      []interface{}{},
			Tags:                   dataset.Tags,
			DocForm:                dataset.DocForm,
			EnableGraphFlow:        dataset.EnableGraphFlow,
		}

		datasetResponses = append(datasetResponses, datasetResponse)
	}

	// Calculate has more flag
	hasMore := int64(page*limit) < total

	response := &dto.DatasetListResponse{
		Data:    datasetResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: hasMore,
	}

	return response, nil
}
