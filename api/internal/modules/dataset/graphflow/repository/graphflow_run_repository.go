package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrGraphFlowRunNotClaimable = errors.New("graph flow run is not claimable")

type GraphFlowRunRepository struct {
	db *gorm.DB
}

func NewGraphFlowRunRepository(db *gorm.DB) *GraphFlowRunRepository {
	return &GraphFlowRunRepository{db: db}
}

func (r *GraphFlowRunRepository) WithTx(tx *gorm.DB) *GraphFlowRunRepository {
	return NewGraphFlowRunRepository(tx)
}

func (r *GraphFlowRunRepository) CreateOrGet(ctx context.Context, run *model.GraphFlowRun) (*model.GraphFlowRun, bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(run)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return run, true, nil
	}

	existing, err := r.FindByIdempotencyKey(ctx, run.DatasetID, run.IdempotencyKey)
	return existing, false, err
}

func (r *GraphFlowRunRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.GraphFlowRun, error) {
	var run model.GraphFlowRun
	if err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *GraphFlowRunRepository) FindByIdempotencyKey(ctx context.Context, datasetID uuid.UUID, key string) (*model.GraphFlowRun, error) {
	var run model.GraphFlowRun
	if err := r.db.WithContext(ctx).
		Where("dataset_id = ? AND idempotency_key = ?", datasetID, key).
		First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *GraphFlowRunRepository) Claim(ctx context.Context, id uuid.UUID, leaseDuration time.Duration) (*model.GraphFlowRun, error) {
	now := time.Now().UTC()
	leaseExpiresAt := now.Add(leaseDuration)
	result := r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status = ?", id, model.GraphFlowRunStatusPending).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusProcessing,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"started_at":       gorm.Expr("COALESCE(started_at, ?)", now),
			"heartbeat_at":     now,
			"lease_expires_at": leaseExpiresAt,
			"updated_at":       now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrGraphFlowRunNotClaimable
	}
	return r.FindByID(ctx, id)
}

func (r *GraphFlowRunRepository) Heartbeat(ctx context.Context, id uuid.UUID, leaseDuration time.Duration) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status = ?", id, model.GraphFlowRunStatusProcessing).
		Updates(map[string]any{
			"heartbeat_at":     now,
			"lease_expires_at": now.Add(leaseDuration),
			"updated_at":       now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrGraphFlowRunNotClaimable
	}
	return nil
}

func (r *GraphFlowRunRepository) MarkReady(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status = ?", id, model.GraphFlowRunStatusProcessing).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusReady,
			"progress":         100,
			"finished_at":      now,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		}).Error
}

func (r *GraphFlowRunRepository) MarkFailed(ctx context.Context, id uuid.UUID, code, message string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status IN ?", id, []string{model.GraphFlowRunStatusPending, model.GraphFlowRunStatusProcessing}).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusFailed,
			"error_code":       code,
			"error_message":    message,
			"finished_at":      now,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		}).Error
}

func (r *GraphFlowRunRepository) Retry(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status = ?", id, model.GraphFlowRunStatusFailed).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusPending,
			"progress":         0,
			"error_code":       nil,
			"error_message":    nil,
			"finished_at":      nil,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       time.Now().UTC(),
		}).Error
}

func (r *GraphFlowRunRepository) Cancel(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status IN ?", id, []string{model.GraphFlowRunStatusPending, model.GraphFlowRunStatusProcessing}).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusCancelled,
			"finished_at":      now,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		}).Error
}

func (r *GraphFlowRunRepository) Supersede(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("id = ? AND status IN ?", id, []string{
			model.GraphFlowRunStatusPending,
			model.GraphFlowRunStatusProcessing,
			model.GraphFlowRunStatusReady,
		}).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusSuperseded,
			"finished_at":      now,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		}).Error
}

func (r *GraphFlowRunRepository) RequeueExpired(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.GraphFlowRun{}).
		Where("status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?", model.GraphFlowRunStatusProcessing, now).
		Updates(map[string]any{
			"status":           model.GraphFlowRunStatusPending,
			"lease_expires_at": nil,
			"heartbeat_at":     nil,
			"updated_at":       now,
		})
	return result.RowsAffected, result.Error
}
