package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type PlaygroundRunListFilter struct {
	WorkspaceID       *uuid.UUID
	AccountID         *uuid.UUID
	SourceContentHash string
	Limit             int
	AllowUnscoped     bool
}

type PlaygroundRunRepository interface {
	Create(ctx context.Context, item *model.PlaygroundRun) error
	GetByID(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error)
	GetByShareToken(ctx context.Context, token string) (*model.PlaygroundRun, error)
	List(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error)
	SetShareEnabled(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter, enabled bool) (*model.PlaygroundRun, error)
}

type playgroundRunRepository struct {
	db *gorm.DB
}

func NewPlaygroundRunRepository(db *gorm.DB) PlaygroundRunRepository {
	return &playgroundRunRepository{db: db}
}

func (r *playgroundRunRepository) Create(ctx context.Context, item *model.PlaygroundRun) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *playgroundRunRepository) GetByID(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter) (*model.PlaygroundRun, error) {
	var item model.PlaygroundRun
	query := r.db.WithContext(ctx).Where("id = ?", id)
	query = applyPlaygroundRunScope(query, filter)
	err := query.First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *playgroundRunRepository) GetByShareToken(ctx context.Context, token string) (*model.PlaygroundRun, error) {
	var item model.PlaygroundRun
	err := r.db.WithContext(ctx).
		Where("share_token = ? AND is_share_enabled = ?", strings.TrimSpace(token), true).
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *playgroundRunRepository) List(ctx context.Context, filter PlaygroundRunListFilter) ([]*model.PlaygroundRun, error) {
	var items []*model.PlaygroundRun
	query := r.db.WithContext(ctx).Model(&model.PlaygroundRun{}).Order("created_at DESC")
	query = applyPlaygroundRunScope(query, filter)
	if hash := strings.TrimSpace(filter.SourceContentHash); hash != "" {
		query = query.Where("source_content_hash = ?", hash)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	err := query.Limit(limit).Find(&items).Error
	return items, err
}

func (r *playgroundRunRepository) SetShareEnabled(ctx context.Context, id uuid.UUID, filter PlaygroundRunListFilter, enabled bool) (*model.PlaygroundRun, error) {
	query := r.db.WithContext(ctx).Model(&model.PlaygroundRun{}).Where("id = ?", id)
	query = applyPlaygroundRunScope(query, filter)
	result := query.Update("is_share_enabled", enabled)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetByID(ctx, id, filter)
}

func applyPlaygroundRunScope(query *gorm.DB, filter PlaygroundRunListFilter) *gorm.DB {
	if filter.WorkspaceID != nil {
		return query.Where("workspace_id = ?", *filter.WorkspaceID)
	}
	if filter.AccountID != nil {
		return query.Where("workspace_id IS NULL AND account_id = ?", *filter.AccountID)
	}
	if filter.AllowUnscoped {
		return query
	}
	return query.Where("1 = 0")
}
