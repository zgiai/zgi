package memory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) WithTransaction(ctx context.Context, fn func(store) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) GetSetting(ctx context.Context, accountID uuid.UUID) (*AccountMemorySetting, error) {
	var setting AccountMemorySetting
	err := r.db.WithContext(ctx).Where("account_id = ?", accountID).First(&setting).Error
	if err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *Repository) UpsertSetting(ctx context.Context, setting *AccountMemorySetting) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}).Create(setting).Error
}

func (r *Repository) LockAccount(ctx context.Context, accountID uuid.UUID) error {
	now := time.Now()
	setting := &AccountMemorySetting{
		AccountID: accountID,
		Enabled:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}},
		DoNothing: true,
	}).Create(setting).Error; err != nil {
		return err
	}

	var locked AccountMemorySetting
	return r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ?", accountID).
		First(&locked).Error
}

func (r *Repository) ListEntries(ctx context.Context, accountID uuid.UUID, enabledOnly bool) ([]*AccountMemoryEntry, error) {
	var entries []*AccountMemoryEntry
	query := r.db.WithContext(ctx).Where("account_id = ?", accountID)
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("updated_at DESC").Find(&entries).Error
	return entries, err
}

func (r *Repository) CreateEntry(ctx context.Context, entry *AccountMemoryEntry) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *Repository) CreateEvent(ctx context.Context, event *AccountMemoryEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *Repository) GetEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) (*AccountMemoryEntry, error) {
	var entry AccountMemoryEntry
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND id = ?", accountID, entryID).
		First(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *Repository) UpdateEntryScoped(ctx context.Context, accountID, entryID uuid.UUID, values map[string]interface{}) (*AccountMemoryEntry, error) {
	if err := r.db.WithContext(ctx).
		Model(&AccountMemoryEntry{}).
		Where("account_id = ? AND id = ?", accountID, entryID).
		Updates(values).Error; err != nil {
		return nil, err
	}
	return r.GetEntryScoped(ctx, accountID, entryID)
}

func (r *Repository) DeleteEntryScoped(ctx context.Context, accountID, entryID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("account_id = ? AND id = ?", accountID, entryID).
		Delete(&AccountMemoryEntry{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
