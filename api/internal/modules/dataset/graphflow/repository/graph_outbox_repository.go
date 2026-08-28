package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GraphOutboxRepository struct {
	db *gorm.DB
}

func NewGraphOutboxRepository(db *gorm.DB) *GraphOutboxRepository {
	return &GraphOutboxRepository{db: db}
}

func (r *GraphOutboxRepository) WithTx(tx *gorm.DB) *GraphOutboxRepository {
	return NewGraphOutboxRepository(tx)
}

func (r *GraphOutboxRepository) CreateOrGet(ctx context.Context, event *model.GraphOutboxEvent) (*model.GraphOutboxEvent, bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(event)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return event, true, nil
	}
	var existing model.GraphOutboxEvent
	err := r.db.WithContext(ctx).
		Where("event_type = ? AND aggregate_key = ? AND status IN ?", event.EventType, event.AggregateKey, []string{
			model.GraphOutboxStatusPending,
			model.GraphOutboxStatusProcessing,
		}).
		First(&existing).Error
	if err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func (r *GraphOutboxRepository) ClaimBatch(ctx context.Context, limit int, now time.Time) ([]*model.GraphOutboxEvent, error) {
	if limit <= 0 {
		return nil, nil
	}
	var events []*model.GraphOutboxEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("status = ? AND available_at <= ?", model.GraphOutboxStatusPending, now).
			Order("available_at ASC, created_at ASC").
			Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&events).Error; err != nil || len(events) == 0 {
			return err
		}
		ids := make([]uuid.UUID, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		return tx.Model(&model.GraphOutboxEvent{}).
			Where("id IN ? AND status = ?", ids, model.GraphOutboxStatusPending).
			Updates(map[string]any{
				"status":        model.GraphOutboxStatusProcessing,
				"claimed_at":    now,
				"attempt_count": gorm.Expr("attempt_count + 1"),
				"updated_at":    now,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		event.Status = model.GraphOutboxStatusProcessing
		event.ClaimedAt = &now
		event.AttemptCount++
	}
	return events, nil
}

func (r *GraphOutboxRepository) Confirm(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GraphOutboxEvent{}).
		Where("id = ? AND status = ?", id, model.GraphOutboxStatusProcessing).
		Updates(map[string]any{
			"status":        model.GraphOutboxStatusConfirmed,
			"confirmed_at":  now,
			"error_message": nil,
			"updated_at":    now,
		}).Error
}

func (r *GraphOutboxRepository) Retry(ctx context.Context, id uuid.UUID, availableAt time.Time, message string) error {
	return r.db.WithContext(ctx).Model(&model.GraphOutboxEvent{}).
		Where("id = ? AND status IN ?", id, []string{model.GraphOutboxStatusProcessing, model.GraphOutboxStatusFailed}).
		Updates(map[string]any{
			"status":        model.GraphOutboxStatusPending,
			"available_at":  availableAt,
			"claimed_at":    nil,
			"error_message": message,
			"updated_at":    time.Now().UTC(),
		}).Error
}

func (r *GraphOutboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, message string) error {
	return r.db.WithContext(ctx).Model(&model.GraphOutboxEvent{}).
		Where("id = ? AND status = ?", id, model.GraphOutboxStatusProcessing).
		Updates(map[string]any{
			"status":        model.GraphOutboxStatusFailed,
			"error_message": message,
			"updated_at":    time.Now().UTC(),
		}).Error
}

func (r *GraphOutboxRepository) RequeueStale(ctx context.Context, claimedBefore, availableAt time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.GraphOutboxEvent{}).
		Where("status = ? AND claimed_at IS NOT NULL AND claimed_at < ?", model.GraphOutboxStatusProcessing, claimedBefore).
		Updates(map[string]any{
			"status":       model.GraphOutboxStatusPending,
			"claimed_at":   nil,
			"available_at": availableAt,
			"updated_at":   time.Now().UTC(),
		})
	return result.RowsAffected, result.Error
}
