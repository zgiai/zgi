package digitalhuman

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const channelProcessingMarkerIDName = "digital-human-channel-processing-marker-v1"

var channelProcessingMarkerNamespace = uuid.MustParse("e3172771-4d96-451e-a1a6-33fac7cf4c80")

// ChannelProcessingMarkerStore is deliberately separate from ChannelStore so
// provider-neutral tests and alternate stores do not acquire an unrelated
// reaction persistence surface.
type ChannelProcessingMarkerStore interface {
	CreateProcessingMarker(context.Context, *channelruntime.ProcessingMarker) (bool, error)
	ClaimProcessingMarkerBatch(context.Context, int, time.Time) ([]*channelruntime.ProcessingMarker, error)
	ActivateProcessingMarker(context.Context, uuid.UUID, string, string, time.Time) error
	RemoveProcessingMarker(context.Context, uuid.UUID, time.Time) error
	RetryProcessingMarkerRemoval(context.Context, uuid.UUID, time.Time, string, string) error
	FailProcessingMarker(context.Context, uuid.UUID, string, string, time.Time) error
	RecoverStaleProcessingMarkers(context.Context, time.Time, time.Time) error
}

func processingMarkerID(inboundEventID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(channelProcessingMarkerNamespace, append([]byte(channelProcessingMarkerIDName+":"), inboundEventID[:]...))
}

func (r *GormRepository) CreateProcessingMarker(ctx context.Context, marker *channelruntime.ProcessingMarker) (bool, error) {
	if marker == nil || marker.ID == uuid.Nil || marker.OrganizationID == uuid.Nil || marker.WorkspaceID == uuid.Nil ||
		marker.InstallationID == uuid.Nil || marker.InboundEventID == uuid.Nil || marker.RunID == uuid.Nil ||
		strings.TrimSpace(marker.Provider) == "" || strings.TrimSpace(marker.ProviderMessageID) == "" ||
		marker.Status != channelruntime.ProcessingMarkerPendingAdd || marker.AvailableAt.IsZero() {
		return false, ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "inbound_event_id"}},
		DoNothing: true,
	}).Create(marker)
	if result.Error != nil {
		return false, fmt.Errorf("create channel processing marker: %w", result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *GormRepository) ClaimProcessingMarkerBatch(ctx context.Context, limit int, now time.Time) ([]*channelruntime.ProcessingMarker, error) {
	if limit <= 0 || now.IsZero() {
		return nil, ErrInvalidRequest
	}
	items := make([]*channelruntime.ProcessingMarker, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// If a very fast run finished before the add task was claimed, there is
		// nothing to display or clean up. Avoid flashing Typing after completion.
		if err := tx.Model(&channelruntime.ProcessingMarker{}).Where(
			`status = ? AND EXISTS (
				SELECT 1 FROM digital_human_runs runs
				WHERE runs.id = channel_processing_markers.run_id
				AND runs.status IN ('succeeded', 'failed', 'canceled', 'unknown')
			)`,
			channelruntime.ProcessingMarkerPendingAdd,
		).Updates(map[string]any{
			"status": channelruntime.ProcessingMarkerRemoved, "removed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		terminalCleanup := `status = ? AND available_at <= ? AND EXISTS (
			SELECT 1 FROM digital_human_runs runs
			WHERE runs.id = channel_processing_markers.run_id
			AND (
				runs.status IN ('failed', 'canceled', 'unknown')
				OR (runs.status = 'succeeded' AND NOT EXISTS (
					SELECT 1 FROM channel_delivery_outbox deliveries
					WHERE deliveries.run_id = runs.id
					AND deliveries.delivery_kind = 'final'
					AND deliveries.status IN ('pending', 'processing')
				))
			)
		)`
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where(
			`(status = ? AND available_at <= ? AND EXISTS (
				SELECT 1 FROM digital_human_runs runs
				WHERE runs.id = channel_processing_markers.run_id
				AND runs.status IN ('queued', 'running', 'canceling')
			)) OR (`+terminalCleanup+")",
			channelruntime.ProcessingMarkerPendingAdd, now,
			channelruntime.ProcessingMarkerActive, now,
		).Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			next := channelruntime.ProcessingMarkerAdding
			if item.Status == channelruntime.ProcessingMarkerActive {
				next = channelruntime.ProcessingMarkerRemoving
			}
			result := tx.Model(&channelruntime.ProcessingMarker{}).Where("id = ? AND status = ?", item.ID, item.Status).
				Updates(map[string]any{
					"status": next, "claimed_at": now,
					"attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrChannelDeliveryConflict
			}
			item.Status, item.ClaimedAt, item.AttemptCount = next, &now, item.AttemptCount+1
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim channel processing markers: %w", err)
	}
	return items, nil
}

func (r *GormRepository) ActivateProcessingMarker(ctx context.Context, markerID uuid.UUID, providerMarkerID, providerRequestID string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&channelruntime.ProcessingMarker{}).Where(
		"id = ? AND status = ?", markerID, channelruntime.ProcessingMarkerAdding,
	).Updates(map[string]any{
		"status": channelruntime.ProcessingMarkerActive, "claimed_at": nil, "available_at": now,
		"provider_marker_id": strings.TrimSpace(providerMarkerID), "provider_request_id": strings.TrimSpace(providerRequestID),
		"error_code": "", "error_message": "", "updated_at": now,
	})
	return processingMarkerUpdateError(result, "activate channel processing marker")
}

func (r *GormRepository) RemoveProcessingMarker(ctx context.Context, markerID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&channelruntime.ProcessingMarker{}).Where(
		"id = ? AND status = ?", markerID, channelruntime.ProcessingMarkerRemoving,
	).Updates(map[string]any{
		"status": channelruntime.ProcessingMarkerRemoved, "claimed_at": nil, "removed_at": now,
		"error_code": "", "error_message": "", "updated_at": now,
	})
	return processingMarkerUpdateError(result, "remove channel processing marker")
}

func (r *GormRepository) RetryProcessingMarkerRemoval(ctx context.Context, markerID uuid.UUID, availableAt time.Time, code, message string) error {
	result := r.db.WithContext(ctx).Model(&channelruntime.ProcessingMarker{}).Where(
		"id = ? AND status = ?", markerID, channelruntime.ProcessingMarkerRemoving,
	).Updates(map[string]any{
		"status": channelruntime.ProcessingMarkerActive, "claimed_at": nil, "available_at": availableAt,
		"error_code": strings.TrimSpace(code), "error_message": strings.TrimSpace(message), "updated_at": time.Now().UTC(),
	})
	return processingMarkerUpdateError(result, "retry channel processing marker removal")
}

func (r *GormRepository) FailProcessingMarker(ctx context.Context, markerID uuid.UUID, code, message string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&channelruntime.ProcessingMarker{}).Where(
		"id = ? AND status IN ?", markerID,
		[]channelruntime.ProcessingMarkerStatus{channelruntime.ProcessingMarkerAdding, channelruntime.ProcessingMarkerRemoving},
	).Updates(map[string]any{
		"status": channelruntime.ProcessingMarkerFailed, "claimed_at": nil,
		"error_code": strings.TrimSpace(code), "error_message": strings.TrimSpace(message), "updated_at": now,
	})
	return processingMarkerUpdateError(result, "fail channel processing marker")
}

func (r *GormRepository) RecoverStaleProcessingMarkers(ctx context.Context, claimedBefore, now time.Time) error {
	if claimedBefore.IsZero() || now.IsZero() {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&channelruntime.ProcessingMarker{}).Where(
			"status = ? AND claimed_at < ?", channelruntime.ProcessingMarkerAdding, claimedBefore,
		).Updates(map[string]any{
			"status": channelruntime.ProcessingMarkerActive, "claimed_at": nil, "available_at": now,
			"provider_marker_id": "", "error_code": "channel.processing_marker.add_interrupted",
			"error_message": "Processing marker creation was interrupted and will be reconciled.", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&channelruntime.ProcessingMarker{}).Where(
			"status = ? AND claimed_at < ?", channelruntime.ProcessingMarkerRemoving, claimedBefore,
		).Updates(map[string]any{
			"status": channelruntime.ProcessingMarkerActive, "claimed_at": nil, "available_at": now,
			"error_code":    "channel.processing_marker.remove_interrupted",
			"error_message": "Processing marker removal was interrupted and will be retried.", "updated_at": now,
		}).Error
	})
}

func processingMarkerUpdateError(result *gorm.DB, operation string) error {
	if result == nil {
		return ErrInvalidRequest
	}
	if result.Error != nil {
		return fmt.Errorf("%s: %w", operation, result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrChannelDeliveryConflict
	}
	return nil
}

var _ ChannelProcessingMarkerStore = (*GormRepository)(nil)
