package repository

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"gorm.io/gorm"
)

// FileFavoriteRepository defines the interface for file favorite operations
type FileFavoriteRepository interface {
	// CreateFavorite creates a new file favorite
	CreateFavorite(ctx context.Context, favorite *model.FileFavorite) error
	// DeleteFavorite deletes a file favorite by file ID and account ID
	DeleteFavorite(ctx context.Context, fileID, accountID string) error
	// GetFavorite checks if a file is favorited by an account
	GetFavorite(ctx context.Context, fileID, accountID string) (*model.FileFavorite, error)
	// ListFavorites lists favorites for an account
	ListFavorites(ctx context.Context, accountID string, page, limit int) ([]*model.FileFavorite, int64, error)
	// IsFavorite checks if a file is favorited by an account
	IsFavorite(ctx context.Context, fileID, accountID string) (bool, error)
	// BatchGetFavoriteFileIDs returns a set of file IDs that are favorited by the account from the given file IDs
	BatchGetFavoriteFileIDs(ctx context.Context, fileIDs []string, accountID string) (map[string]bool, error)
}

// fileFavoriteRepository implements FileFavoriteRepository interface
type fileFavoriteRepository struct {
	db *gorm.DB
}

// NewFileFavoriteRepository creates a new file favorite repository
func NewFileFavoriteRepository(db *gorm.DB) FileFavoriteRepository {
	return &fileFavoriteRepository{
		db: db,
	}
}

// CreateFavorite creates a new file favorite
func (r *fileFavoriteRepository) CreateFavorite(ctx context.Context, favorite *model.FileFavorite) error {
	return r.db.WithContext(ctx).Create(favorite).Error
}

// DeleteFavorite deletes a file favorite by file ID and account ID
func (r *fileFavoriteRepository) DeleteFavorite(ctx context.Context, fileID, accountID string) error {
	return r.db.WithContext(ctx).
		Where("file_id = ? AND account_id = ?", fileID, accountID).
		Delete(&model.FileFavorite{}).Error
}

// GetFavorite checks if a file is favorited by an account
func (r *fileFavoriteRepository) GetFavorite(ctx context.Context, fileID, accountID string) (*model.FileFavorite, error) {
	var favorite model.FileFavorite
	err := r.db.WithContext(ctx).
		Where("file_id = ? AND account_id = ?", fileID, accountID).
		First(&favorite).Error
	if err != nil {
		return nil, err
	}
	return &favorite, nil
}

// ListFavorites lists favorites for an account
func (r *fileFavoriteRepository) ListFavorites(ctx context.Context, accountID string, page, limit int) ([]*model.FileFavorite, int64, error) {
	var favorites []*model.FileFavorite
	var total int64

	offset := (page - 1) * limit

	// Get total count
	err := r.db.WithContext(ctx).
		Model(&model.FileFavorite{}).
		Where("account_id = ?", accountID).
		Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err = r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&favorites).Error
	if err != nil {
		return nil, 0, err
	}

	return favorites, total, nil
}

// IsFavorite checks if a file is favorited by an account
func (r *fileFavoriteRepository) IsFavorite(ctx context.Context, fileID, accountID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.FileFavorite{}).
		Where("file_id = ? AND account_id = ?", fileID, accountID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BatchGetFavoriteFileIDs returns a set of file IDs that are favorited by the account from the given file IDs
func (r *fileFavoriteRepository) BatchGetFavoriteFileIDs(ctx context.Context, fileIDs []string, accountID string) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(fileIDs) == 0 {
		return result, nil
	}

	var favorites []*model.FileFavorite
	err := r.db.WithContext(ctx).
		Model(&model.FileFavorite{}).
		Where("file_id IN ? AND account_id = ?", fileIDs, accountID).
		Find(&favorites).Error
	if err != nil {
		return nil, err
	}

	for _, fav := range favorites {
		result[fav.FileID] = true
	}

	return result, nil
}
