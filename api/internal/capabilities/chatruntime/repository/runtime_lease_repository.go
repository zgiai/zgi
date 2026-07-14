package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type runtimeLeaseRepository struct {
	db *gorm.DB
}

type runtimeRunIDContextKey struct{}

func WithRuntimeRunID(ctx context.Context, runID uuid.UUID) context.Context {
	return context.WithValue(ctx, runtimeRunIDContextKey{}, runID)
}

func runtimeRunIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	runID, ok := ctx.Value(runtimeRunIDContextKey{}).(uuid.UUID)
	return runID, ok && runID != uuid.Nil
}

func scopeRuntimeRunOwnership(ctx context.Context, query *gorm.DB) *gorm.DB {
	runID, ok := runtimeRunIDFromContext(ctx)
	if !ok {
		return query
	}
	return query.Where("runtime_run_id = ?", runID)
}

func NewRuntimeLeaseRepository(db *gorm.DB) RuntimeLeaseRepository {
	return &runtimeLeaseRepository{db: db}
}

func (r *runtimeLeaseRepository) Begin(ctx context.Context, messageID, runID uuid.UUID, at time.Time) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", messageID, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"runtime_run_id":       runID,
			"runtime_heartbeat_at": at,
		})
	if result.Error != nil {
		return fmt.Errorf("begin chat runtime lease: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *runtimeLeaseRepository) Renew(ctx context.Context, messageID, runID uuid.UUID, at time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND runtime_run_id = ? AND deleted_at IS NULL AND status IN ?", messageID, runID, activeMessageStatuses()).
		Update("runtime_heartbeat_at", at)
	if result.Error != nil {
		return false, fmt.Errorf("renew chat runtime lease: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *runtimeLeaseRepository) Release(ctx context.Context, messageID, runID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND runtime_run_id = ?", messageID, runID).
		Updates(map[string]interface{}{
			"runtime_run_id":       nil,
			"runtime_heartbeat_at": nil,
		}).Error; err != nil {
		return fmt.Errorf("release chat runtime lease: %w", err)
	}
	return nil
}

func (r *runtimeLeaseRepository) ListExpiredActiveIDs(ctx context.Context, heartbeatCutoff, legacyCutoff time.Time) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.expiredActiveQuery(ctx, heartbeatCutoff, legacyCutoff).Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("list expired chat runtime leases: %w", err)
	}
	return ids, nil
}

func (r *runtimeLeaseRepository) MarkExpiredActiveAsError(ctx context.Context, heartbeatCutoff, legacyCutoff time.Time, message string) ([]uuid.UUID, error) {
	var expired []runtimemodel.Message
	result := r.db.WithContext(ctx).Model(&expired).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}}}).
		Where("deleted_at IS NULL AND status IN ?", activeMessageStatuses()).
		Where("(runtime_heartbeat_at IS NOT NULL AND runtime_heartbeat_at < ?) OR (runtime_heartbeat_at IS NULL AND updated_at < ?)", heartbeatCutoff, legacyCutoff).
		Updates(map[string]interface{}{
			"status":               runtimemodel.MessageStatusError,
			"error":                message,
			"runtime_run_id":       nil,
			"runtime_heartbeat_at": nil,
			"updated_at":           time.Now(),
		})
	if result.Error != nil {
		return nil, fmt.Errorf("expire chat runtime leases: %w", result.Error)
	}
	ids := make([]uuid.UUID, 0, len(expired))
	for index := range expired {
		ids = append(ids, expired[index].ID)
	}
	return ids, nil
}

func (r *runtimeLeaseRepository) expiredActiveQuery(ctx context.Context, heartbeatCutoff, legacyCutoff time.Time) *gorm.DB {
	return r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("deleted_at IS NULL AND status IN ?", activeMessageStatuses()).
		Where("(runtime_heartbeat_at IS NOT NULL AND runtime_heartbeat_at < ?) OR (runtime_heartbeat_at IS NULL AND updated_at < ?)", heartbeatCutoff, legacyCutoff)
}
