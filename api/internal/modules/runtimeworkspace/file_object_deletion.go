package runtimeworkspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	fileObjectDeletionBatchSize = 100
	fileObjectDeletionLease     = time.Minute
	fileObjectDeletionRetryBase = 5 * time.Minute
	fileObjectDeletionRetryMax  = 24 * time.Hour
)

// FileObjectDeletion is the durable work item created when Runtime Workspace
// metadata is removed before the retention period has elapsed. The object is
// deleted only after next_attempt_at, so a failed or interrupted worker never
// causes the object to disappear early.
type FileObjectDeletion struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ObjectKey     string    `gorm:"column:object_key"`
	AttemptCount  int       `gorm:"column:attempt_count"`
	NextAttemptAt time.Time `gorm:"column:next_attempt_at"`
	LastError     string    `gorm:"column:last_error"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (FileObjectDeletion) TableName() string { return "file_object_deletions" }

// FileObjectDeletionWorker consumes deferred object deletion rows. It claims
// each row with a short lease before touching storage, which makes concurrent
// API instances safe without holding a database transaction during an object
// store call. Failed deletes remain in the table with an exponential retry.
type FileObjectDeletionWorker struct {
	db      *gorm.DB
	storage storage.Storage
	now     func() time.Time
}

func NewFileObjectDeletionWorker(db *gorm.DB, objectStorage storage.Storage) *FileObjectDeletionWorker {
	return &FileObjectDeletionWorker{db: db, storage: objectStorage, now: func() time.Time { return time.Now().UTC() }}
}

// RunOnce deletes all currently due rows up to a bounded batch size. A storage
// failure is recorded and returned after the batch so other due objects still
// make progress.
func (w *FileObjectDeletionWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.db == nil || w.storage == nil {
		return nil
	}

	var firstErr error
	for i := 0; i < fileObjectDeletionBatchSize; i++ {
		item, claimed, err := w.claim(ctx, w.now().UTC())
		if err != nil {
			return fmt.Errorf("claim deferred file object deletion: %w", err)
		}
		if !claimed {
			break
		}

		if err := w.storage.Delete(item.ObjectKey); err != nil && !w.objectAbsentAfterDeleteError(item.ObjectKey, err) {
			if retryErr := w.reschedule(ctx, item, err); retryErr != nil {
				return errors.Join(
					fmt.Errorf("record deferred file object deletion failure: %w", retryErr),
					err,
				)
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("delete deferred file object %q: %w", item.ObjectKey, err)
			}
			continue
		}

		if err := w.db.WithContext(ctx).Where("id = ?", item.ID).Delete(&FileObjectDeletion{}).Error; err != nil {
			return fmt.Errorf("acknowledge deferred file object deletion: %w", err)
		}
	}
	return firstErr
}

// objectAbsentAfterDeleteError treats a failed delete as done when the object
// is already gone. Backends report a missing key differently (local storage
// returns ENOENT, others use provider-specific not-found errors), so only an
// explicit Exists=false from the backend counts; an unanswered check keeps
// the row for another attempt.
func (w *FileObjectDeletionWorker) objectAbsentAfterDeleteError(key string, deleteErr error) bool {
	if errors.Is(deleteErr, os.ErrNotExist) {
		return true
	}
	exists, err := w.storage.Exists(key)
	return err == nil && !exists
}

func (w *FileObjectDeletionWorker) claim(ctx context.Context, now time.Time) (*FileObjectDeletion, bool, error) {
	var item FileObjectDeletion
	claimed := false
	err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("next_attempt_at <= ?", now).
			Order("next_attempt_at ASC, created_at ASC").
			Take(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		leaseUntil := now.Add(fileObjectDeletionLease)
		if err := tx.Model(&FileObjectDeletion{}).Where("id = ?", item.ID).Updates(map[string]any{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"next_attempt_at": leaseUntil,
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		item.AttemptCount++
		item.NextAttemptAt = leaseUntil
		claimed = true
		return nil
	})
	return &item, claimed, err
}

func (w *FileObjectDeletionWorker) reschedule(ctx context.Context, item *FileObjectDeletion, deleteErr error) error {
	now := w.now().UTC()
	return w.db.WithContext(ctx).Model(&FileObjectDeletion{}).Where("id = ?", item.ID).Updates(map[string]any{
		"next_attempt_at": now.Add(fileObjectDeletionRetryDelay(item.AttemptCount)),
		"last_error":      deleteErr.Error(),
		"updated_at":      now,
	}).Error
}

func fileObjectDeletionRetryDelay(attemptCount int) time.Duration {
	delay := fileObjectDeletionRetryBase
	for i := 1; i < attemptCount && delay < fileObjectDeletionRetryMax; i++ {
		delay *= 2
		if delay >= fileObjectDeletionRetryMax {
			return fileObjectDeletionRetryMax
		}
	}
	return delay
}
