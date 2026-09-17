package service

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const (
	fileObjectDeletionTaskType = "files:object_deletion_retry"

	// Retry pacing: quick enough to recover from a blip, capped so a permanently
	// unreachable object does not hammer the storage backend.
	fileObjectDeletionBaseBackoff = 30 * time.Second
	fileObjectDeletionMaxBackoff  = time.Hour

	// last_error is diagnostic only; keep a failing provider message bounded.
	fileObjectDeletionMaxErrorLength = 500
)

// FileObjectDeletionProcessor removes objects whose file record was already deleted but
// whose content could not be removed at the time.
type FileObjectDeletionProcessor struct {
	db      *gorm.DB
	storage storage.Storage
	now     func() time.Time
}

func NewFileObjectDeletionProcessor(db *gorm.DB, backend storage.Storage) *FileObjectDeletionProcessor {
	return &FileObjectDeletionProcessor{
		db:      db,
		storage: backend,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// ProcessPending removes pending objects, earliest deadline first, and reports how many
// were cleared.
//
// An absent object counts as done. The backends disagree about a missing key - S3 and OSS
// report success, while the local and OpenDAL backends return ENOENT - so a plain "delete
// succeeded" test would leave rows that can never clear on the default self-hosted
// storage. Two workers racing on the same row is harmless for the same reason.
func (p *FileObjectDeletionProcessor) ProcessPending(ctx context.Context, limit int) (int, error) {
	if p == nil || p.db == nil || p.storage == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 25
	}

	var pending []model.FileObjectDeletion
	if err := p.db.WithContext(ctx).
		Where("next_attempt_at <= ?", p.now()).
		Order("next_attempt_at ASC, created_at ASC").
		Limit(limit).
		Find(&pending).Error; err != nil {
		return 0, fmt.Errorf("list pending file object deletions: %w", err)
	}

	removed := 0
	for i := range pending {
		if ctx.Err() != nil {
			// A storage delete cannot be interrupted, so stop before starting more of
			// them; the remaining rows stay pending for the next sweep.
			break
		}
		if err := p.process(ctx, &pending[i]); err != nil {
			// One unreachable key must not stall the rest of the batch.
			logger.Warn("File object deletion retry failed",
				"id", pending[i].ID,
				"key", pending[i].ObjectKey,
				"attempts", pending[i].AttemptCount,
				"error", err.Error(),
			)
			continue
		}
		removed++
	}
	return removed, nil
}

func (p *FileObjectDeletionProcessor) process(ctx context.Context, entry *model.FileObjectDeletion) error {
	if entry.ObjectKey == "" {
		// Nothing to remove, and an empty key would only make a backend fail forever.
		logger.Warn("Dropping a pending file object deletion without an object key", "id", entry.ID)
		return p.clear(ctx, entry)
	}

	if err := p.storage.Delete(entry.ObjectKey); err != nil {
		absent, checkErr := p.objectIsAbsent(entry.ObjectKey)
		if checkErr != nil || !absent {
			return p.reschedule(ctx, entry, err)
		}
		// The removal is fulfilled whatever the backend reported.
		logger.Info("File object was already absent", "id", entry.ID, "key", entry.ObjectKey)
	}

	return p.clear(ctx, entry)
}

// objectIsAbsent asks the backend whether the object is really gone. Only an explicit
// answer counts; a backend that cannot say leaves the row pending for another attempt.
func (p *FileObjectDeletionProcessor) objectIsAbsent(key string) (bool, error) {
	exists, err := p.storage.Exists(key)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

func (p *FileObjectDeletionProcessor) clear(ctx context.Context, entry *model.FileObjectDeletion) error {
	if err := p.db.WithContext(ctx).
		Where("id = ?", entry.ID).
		Delete(&model.FileObjectDeletion{}).Error; err != nil {
		return fmt.Errorf("clear completed file object deletion: %w", err)
	}

	logger.Info("File object removed by the deletion retry worker",
		"id", entry.ID,
		"key", entry.ObjectKey,
		"attempts", entry.AttemptCount,
	)
	return nil
}

func (p *FileObjectDeletionProcessor) reschedule(ctx context.Context, entry *model.FileObjectDeletion, cause error) error {
	now := p.now()
	attempts := entry.AttemptCount + 1

	if err := p.db.WithContext(ctx).
		Model(&model.FileObjectDeletion{}).
		Where("id = ?", entry.ID).
		Updates(map[string]any{
			"attempt_count":   attempts,
			"next_attempt_at": now.Add(fileObjectDeletionBackoff(attempts)),
			"last_error":      fileObjectDeletionErrorText(cause),
			"updated_at":      now,
		}).Error; err != nil {
		return fmt.Errorf("reschedule file object deletion: %w", err)
	}

	return fmt.Errorf("file object deletion attempt %d failed: %w", attempts, cause)
}

func fileObjectDeletionBackoff(attempt int) time.Duration {
	backoff := fileObjectDeletionBaseBackoff
	for i := 1; i < attempt && backoff < fileObjectDeletionMaxBackoff; i++ {
		backoff *= 2
	}
	if backoff > fileObjectDeletionMaxBackoff {
		backoff = fileObjectDeletionMaxBackoff
	}
	return backoff
}

func fileObjectDeletionErrorText(cause error) string {
	if cause == nil {
		return ""
	}
	message := cause.Error()
	if len(message) > fileObjectDeletionMaxErrorLength {
		return message[:fileObjectDeletionMaxErrorLength]
	}
	return message
}

// FileObjectDeletionTask sweeps pending object removals. The deleting request already
// attempts the removal inline, so this is the safety net for whatever it could not finish.
type FileObjectDeletionTask struct{}

var _ pkgscheduler.ScheduledTask = (*FileObjectDeletionTask)(nil)

func (*FileObjectDeletionTask) TaskType() string { return fileObjectDeletionTaskType }

func (*FileObjectDeletionTask) CronSpec() string { return "" }

func (*FileObjectDeletionTask) Interval() time.Duration { return time.Minute }

func (*FileObjectDeletionTask) Payload() []byte { return nil }

func (*FileObjectDeletionTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Minute),
		// A late duplicate sweep would only repeat an idempotent delete.
		asynq.Unique(30 * time.Second),
	}
}

type FileObjectDeletionTaskHandler struct {
	processor *FileObjectDeletionProcessor
	batchSize int
}

var _ pkgscheduler.TaskHandler = (*FileObjectDeletionTaskHandler)(nil)

func NewFileObjectDeletionTaskHandler(processor *FileObjectDeletionProcessor, batchSize int) *FileObjectDeletionTaskHandler {
	if batchSize <= 0 {
		batchSize = 50
	}
	return &FileObjectDeletionTaskHandler{processor: processor, batchSize: batchSize}
}

func (h *FileObjectDeletionTaskHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.processor == nil {
		return nil
	}
	_, err := h.processor.ProcessPending(ctx, h.batchSize)
	return err
}
