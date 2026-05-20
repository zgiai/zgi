package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zgiai/ginext/internal/modules/system/model"
)

// BootstrapLockRepository manages bootstrap serialization records.
type BootstrapLockRepository struct {
	db *gorm.DB
}

// NewBootstrapLockRepository creates a new bootstrap lock repository.
func NewBootstrapLockRepository(db *gorm.DB) *BootstrapLockRepository {
	return &BootstrapLockRepository{db: db}
}

// WithTx binds the repository to a transaction.
func (r *BootstrapLockRepository) WithTx(tx *gorm.DB) *BootstrapLockRepository {
	return &BootstrapLockRepository{db: tx}
}

// EnsureLockRow creates the bootstrap lock row if it does not already exist.
func (r *BootstrapLockRepository) EnsureLockRow(ctx context.Context, key string) error {
	lock := model.BootstrapLock{
		Key:       key,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&lock).Error; err != nil {
		return fmt.Errorf("ensure bootstrap lock row: %w", err)
	}

	return nil
}

// LockForUpdate acquires a row-level lock for the requested bootstrap key.
func (r *BootstrapLockRepository) LockForUpdate(ctx context.Context, key string) (*model.BootstrapLock, error) {
	var lock model.BootstrapLock
	if err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("key = ?", key).
		Take(&lock).Error; err != nil {
		return nil, fmt.Errorf("lock bootstrap row: %w", err)
	}

	return &lock, nil
}
