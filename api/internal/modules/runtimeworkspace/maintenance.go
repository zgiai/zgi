package runtimeworkspace

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultAccessEventRetentionDays = 90
	accessEventPurgeBatch           = 5000
	accessEventPurgeMaxBatches      = 100
	maintenancePurgeInterval        = time.Hour
	maintenanceExportsPerRun        = 5
)

// AccessEventRetention is how long access events are kept:
// ZGI_RUNTIME_WORKSPACE_ACCESS_EVENT_RETENTION_DAYS, 90 days by default.
func AccessEventRetention() time.Duration {
	days := defaultAccessEventRetentionDays
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ZGI_RUNTIME_WORKSPACE_ACCESS_EVENT_RETENTION_DAYS"))); err == nil && value > 0 {
		days = value
	}
	return time.Duration(days) * 24 * time.Hour
}

// MaintenanceWorker runs queued end-user exports and, once an hour, purges
// expired exports and access events past their retention. Exports are
// leased with SKIP LOCKED, so every API instance may run it.
type MaintenanceWorker struct {
	service   *Service
	db        *gorm.DB
	now       func() time.Time
	lastPurge time.Time
}

// NewMaintenanceWorker builds the worker.
func NewMaintenanceWorker(db *gorm.DB) *MaintenanceWorker {
	return &MaintenanceWorker{service: NewService(db), db: db, now: func() time.Time { return time.Now().UTC() }}
}

// RunOnce runs a few queued exports and the hourly purge.
func (w *MaintenanceWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.db == nil {
		return nil
	}
	for range maintenanceExportsPerRun {
		job, err := claimEndUserExport(ctx, w.db, w.now())
		if err != nil {
			return err
		}
		if job == nil {
			break
		}
		content, runErr := w.service.runEndUserExport(ctx, *job)
		if err := finishEndUserExport(ctx, w.db, *job, content, runErr, w.now()); err != nil {
			return fmt.Errorf("finish end-user export: %w", err)
		}
	}
	now := w.now()
	if now.Sub(w.lastPurge) < maintenancePurgeInterval {
		return nil
	}
	w.lastPurge = now
	return w.purge(ctx, now)
}

func (w *MaintenanceWorker) purge(ctx context.Context, now time.Time) error {
	if err := w.db.WithContext(ctx).Exec("DELETE FROM runtime_workspace_end_user_exports WHERE expires_at < ?", now).Error; err != nil {
		return fmt.Errorf("purge end-user exports: %w", err)
	}
	cutoff := now.Add(-AccessEventRetention())
	for range accessEventPurgeMaxBatches {
		result := w.db.WithContext(ctx).Exec(`DELETE FROM runtime_workspace_access_events WHERE id IN (
			SELECT id FROM runtime_workspace_access_events WHERE created_at < ? LIMIT ?)`, cutoff, accessEventPurgeBatch)
		if result.Error != nil {
			return fmt.Errorf("purge access events: %w", result.Error)
		}
		if result.RowsAffected < accessEventPurgeBatch {
			break
		}
	}
	return nil
}
