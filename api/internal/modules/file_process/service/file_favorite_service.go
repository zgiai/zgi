package service

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/file_process/model"
	"github.com/zgiai/ginext/internal/modules/file_process/repository"
)

// FileFavoriteService defines the interface for file favorite operations
type FileFavoriteService interface {
	// FavoriteFile favorites a file for an account
	FavoriteFile(ctx context.Context, fileID, accountID string) error
	// UnfavoriteFile unfavorites a file for an account
	UnfavoriteFile(ctx context.Context, fileID, accountID string) error
	// BatchFavoriteFiles favorites multiple files for an account
	BatchFavoriteFiles(ctx context.Context, fileIDs []string, accountID string) error
	// BatchUnfavoriteFiles unfavorites multiple files for an account
	BatchUnfavoriteFiles(ctx context.Context, fileIDs []string, accountID string) error
	// IsFileFavorite checks if a file is favorited by an account
	IsFileFavorite(ctx context.Context, fileID, accountID string) (bool, error)
	// ListFavorites lists favorite files for an account
	ListFavorites(ctx context.Context, accountID string, page, limit int) ([]*model.FileFavorite, int64, error)
	// BatchCheckFavorites checks if multiple files are favorited by an account, returns a map of fileID -> isFavorite
	BatchCheckFavorites(ctx context.Context, fileIDs []string, accountID string) (map[string]bool, error)
}

// fileFavoriteService implements FileFavoriteService interface
type fileFavoriteService struct {
	fileFavoriteRepo repository.FileFavoriteRepository
	fileRepo         repository.FileRepository
}

// NewFileFavoriteService creates a new file favorite service
func NewFileFavoriteService(
	fileFavoriteRepo repository.FileFavoriteRepository,
	fileRepo repository.FileRepository,
) FileFavoriteService {
	return &fileFavoriteService{
		fileFavoriteRepo: fileFavoriteRepo,
		fileRepo:         fileRepo,
	}
}

// FavoriteFile favorites a file for an account
func (s *fileFavoriteService) FavoriteFile(ctx context.Context, fileID, accountID string) error {
	// Check if file exists
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Check if already favorited
	isFavorite, err := s.fileFavoriteRepo.IsFavorite(ctx, fileID, accountID)
	if err != nil {
		return fmt.Errorf("failed to check favorite status: %w", err)
	}

	if isFavorite {
		// Already favorited, nothing to do
		return nil
	}

	// Create favorite record
	favorite := &model.FileFavorite{
		FileID:    fileID,
		AccountID: accountID,
	}

	return s.fileFavoriteRepo.CreateFavorite(ctx, favorite)
}

// UnfavoriteFile unfavorites a file for an account
func (s *fileFavoriteService) UnfavoriteFile(ctx context.Context, fileID, accountID string) error {
	// Check if file exists
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Delete favorite record
	return s.fileFavoriteRepo.DeleteFavorite(ctx, fileID, accountID)
}

// BatchFavoriteFiles favorites multiple files for an account
func (s *fileFavoriteService) BatchFavoriteFiles(ctx context.Context, fileIDs []string, accountID string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		_, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file %s not found: %w", fileID, err)
		}

		// Check if already favorited
		isFavorite, err := s.fileFavoriteRepo.IsFavorite(ctx, fileID, accountID)
		if err != nil {
			return fmt.Errorf("failed to check favorite status for file %s: %w", fileID, err)
		}

		if isFavorite {
			// Already favorited, skip
			continue
		}

		// Create favorite record
		favorite := &model.FileFavorite{
			FileID:    fileID,
			AccountID: accountID,
		}

		err = s.fileFavoriteRepo.CreateFavorite(ctx, favorite)
		if err != nil {
			return fmt.Errorf("failed to favorite file %s: %w", fileID, err)
		}
	}

	return nil
}

// BatchUnfavoriteFiles unfavorites multiple files for an account
func (s *fileFavoriteService) BatchUnfavoriteFiles(ctx context.Context, fileIDs []string, accountID string) error {
	for _, fileID := range fileIDs {
		// Check if file exists
		_, err := s.fileRepo.GetByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("file %s not found: %w", fileID, err)
		}

		// Delete favorite record
		err = s.fileFavoriteRepo.DeleteFavorite(ctx, fileID, accountID)
		if err != nil {
			return fmt.Errorf("failed to unfavorite file %s: %w", fileID, err)
		}
	}

	return nil
}

// IsFileFavorite checks if a file is favorited by an account
func (s *fileFavoriteService) IsFileFavorite(ctx context.Context, fileID, accountID string) (bool, error) {
	// Check if file exists
	_, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		return false, fmt.Errorf("file not found: %w", err)
	}

	return s.fileFavoriteRepo.IsFavorite(ctx, fileID, accountID)
}

// ListFavorites lists favorite files for an account
func (s *fileFavoriteService) ListFavorites(ctx context.Context, accountID string, page, limit int) ([]*model.FileFavorite, int64, error) {
	return s.fileFavoriteRepo.ListFavorites(ctx, accountID, page, limit)
}

// BatchCheckFavorites checks if multiple files are favorited by an account, returns a map of fileID -> isFavorite
func (s *fileFavoriteService) BatchCheckFavorites(ctx context.Context, fileIDs []string, accountID string) (map[string]bool, error) {
	return s.fileFavoriteRepo.BatchGetFavoriteFileIDs(ctx, fileIDs, accountID)
}
