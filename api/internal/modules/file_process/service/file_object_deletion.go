package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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
// Removing an object is idempotent, so two workers racing on the same row is harmless and
// no lease is required: the loser deletes an object that is already gone and clears a row
// that is already gone.
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

// ProcessOne retries a single pending removal, so a caller that just failed can wake the
// work without waiting for the next sweep.
func (p *FileObjectDeletionProcessor) ProcessOne(ctx context.Context, id string) error {
	if p == nil || p.db == nil || p.storage == nil || id == "" {
		return nil
	}

	var entry model.FileObjectDeletion
	if err := p.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load pending file object deletion: %w", err)
	}
	return p.process(ctx, &entry)
}

func (p *FileObjectDeletionProcessor) process(ctx context.Context, entry *model.FileObjectDeletion) error {
	if err := p.storage.Delete(entry.ObjectKey); err != nil {
		return p.reschedule(ctx, entry, err)
	}

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

// fileObjectDeletionID keeps id generation in one place for the enqueue path.
func fileObjectDeletionID() string {
	return uuid.NewString()
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
