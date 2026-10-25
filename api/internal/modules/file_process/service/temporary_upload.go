package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

const (
	temporaryUploadAttemptTTL        = 24 * time.Hour
	temporaryUploadCancellationGrace = time.Hour
	temporaryUploadCleanupTaskType   = "files:temporary_upload_cleanup"
)

var ErrTemporaryUploadCancelled = errors.New("temporary upload was cancelled")

// TemporaryUploadLifecycle is intentionally optional at the shared FileService boundary.
// Only chat attachment uploads have an attempt id; regular temporary files keep their
// existing retention behavior.
type TemporaryUploadLifecycle interface {
	CancelTemporaryUpload(ctx context.Context, attemptID, ownerID string) error
	ClaimTemporaryFilesInTx(ctx context.Context, tx *gorm.DB, fileIDs []string, ownerID, usedBy string) error
}

func (s *fileService) beginTemporaryUploadAttempt(ctx context.Context, attemptID, ownerID, fileID, objectKey string) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.FileUploadAttempt
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, "id = ?", attemptID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&model.FileUploadAttempt{
				ID:        attemptID,
				OwnerID:   ownerID,
				FileID:    fileID,
				ObjectKey: objectKey,
				Status:    model.FileUploadAttemptPending,
				ExpiresAt: now.Add(temporaryUploadAttemptTTL),
				CreatedAt: now,
				UpdatedAt: now,
			}).Error
		}
		if err != nil {
			return fmt.Errorf("load temporary upload attempt: %w", err)
		}
		if attempt.OwnerID != ownerID {
			return fmt.Errorf("temporary upload attempt belongs to another owner")
		}
		if attempt.Status == model.FileUploadAttemptCancelled {
			return ErrTemporaryUploadCancelled
		}
		if attempt.FileID != "" && attempt.FileID != fileID {
			return fmt.Errorf("temporary upload attempt is already bound to another file")
		}
		if attempt.ObjectKey != "" && attempt.ObjectKey != objectKey {
			return fmt.Errorf("temporary upload attempt is already bound to another object")
		}
		if attempt.Status == model.FileUploadAttemptClaimed || attempt.Status == model.FileUploadAttemptCompleted {
			return fmt.Errorf("temporary upload attempt has already completed")
		}
		return tx.Model(&model.FileUploadAttempt{}).Where("id = ?", attemptID).Updates(map[string]interface{}{
			"file_id":    fileID,
			"object_key": objectKey,
			"updated_at": now,
		}).Error
	})
}

func (s *fileService) markTemporaryUploadCompleted(ctx context.Context, tx *gorm.DB, attemptID string) error {
	if attemptID == "" {
		return nil
	}
	var attempt model.FileUploadAttempt
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, "id = ?", attemptID).Error; err != nil {
		return fmt.Errorf("load temporary upload attempt before completion: %w", err)
	}
	if attempt.Status == model.FileUploadAttemptCancelled {
		return ErrTemporaryUploadCancelled
	}
	if attempt.Status != model.FileUploadAttemptPending {
		return fmt.Errorf("temporary upload attempt is not pending")
	}
	return tx.WithContext(ctx).Model(&model.FileUploadAttempt{}).Where("id = ?", attemptID).Updates(map[string]interface{}{
		"status":     model.FileUploadAttemptCompleted,
		"updated_at": time.Now().UTC(),
	}).Error
}

func (s *fileService) ensureTemporaryUploadAttemptPending(ctx context.Context, tx *gorm.DB, attemptID, ownerID, fileID string) error {
	var attempt model.FileUploadAttempt
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, "id = ?", attemptID).Error; err != nil {
		return fmt.Errorf("load temporary upload attempt before recording file: %w", err)
	}
	if attempt.OwnerID != ownerID || attempt.FileID != fileID {
		return fmt.Errorf("temporary upload attempt does not match the uploaded file")
	}
	if attempt.Status == model.FileUploadAttemptCancelled {
		return ErrTemporaryUploadCancelled
	}
	if attempt.Status != model.FileUploadAttemptPending {
		return fmt.Errorf("temporary upload attempt is not pending")
	}
	return nil
}

func (s *fileService) CancelTemporaryUpload(ctx context.Context, attemptID, ownerID string) error {
	if attemptID == "" || ownerID == "" {
		return fmt.Errorf("temporary upload cancellation requires an attempt and owner")
	}
	if _, err := uuid.Parse(attemptID); err != nil {
		return fmt.Errorf("invalid upload attempt id: %w", err)
	}
	var pending *model.FileObjectDeletion
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.FileUploadAttempt
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, "id = ?", attemptID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A cancellation may race the upload request before it has created its row. Keep
			// a tombstone so a later upload with the same attempt id cannot resurrect it.
			return tx.Create(&model.FileUploadAttempt{
				ID:        attemptID,
				OwnerID:   ownerID,
				Status:    model.FileUploadAttemptCancelled,
				ExpiresAt: now.Add(temporaryUploadCancellationGrace),
				CreatedAt: now,
				UpdatedAt: now,
			}).Error
		}
		if err != nil {
			return fmt.Errorf("load temporary upload attempt for cancellation: %w", err)
		}
		if attempt.OwnerID != ownerID {
			return fmt.Errorf("temporary upload attempt belongs to another owner")
		}
		if attempt.Status == model.FileUploadAttemptClaimed || attempt.Status == model.FileUploadAttemptCancelled {
			return nil
		}

		if attempt.Status == model.FileUploadAttemptCompleted && attempt.FileID != "" {
			objectKey := attempt.ObjectKey
			var file model.UploadFile
			fileErr := tx.WithContext(ctx).Where("id = ?", attempt.FileID).First(&file).Error
			if fileErr == nil {
				if file.Used {
					return tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error
				}
				if !file.IsTemporary || file.CreatedBy != ownerID {
					return tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error
				}
				// The upload record is the canonical object reference. If the attempt and
				// record disagree, fail safe by retaining the file and dropping only the
				// malformed attempt instead of deleting an unrelated object.
				if objectKey != "" && objectKey != file.Key {
					return tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error
				}
				objectKey = file.Key
				if err := tx.Where("id = ?", file.ID).Delete(&model.UploadFile{}).Error; err != nil {
					return fmt.Errorf("delete cancelled temporary file record: %w", err)
				}
			}
			if fileErr != nil && !errors.Is(fileErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("load completed temporary file for cancellation: %w", fileErr)
			}
			if objectKey != "" {
				pending = model.NewFileObjectDeletion(objectKey, now)
				if err := tx.Create(pending).Error; err != nil {
					return fmt.Errorf("record cancelled temporary object deletion: %w", err)
				}
			}
			return tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error
		}

		return tx.Model(&model.FileUploadAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]interface{}{
			"status":     model.FileUploadAttemptCancelled,
			"expires_at": now.Add(temporaryUploadCancellationGrace),
			"updated_at": now,
		}).Error
	})
	if err != nil {
		return err
	}
	if pending != nil {
		s.removeObjectWithRetry(ctx, pending)
	}
	return nil
}

func (s *fileService) ClaimTemporaryFilesInTx(ctx context.Context, tx *gorm.DB, fileIDs []string, ownerID, usedBy string) error {
	if tx == nil || len(fileIDs) == 0 || ownerID == "" {
		return nil
	}
	var attempts []model.FileUploadAttempt
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("file_id IN ? AND owner_id = ?", fileIDs, ownerID).
		Find(&attempts).Error; err != nil {
		return fmt.Errorf("load temporary upload attempts for claim: %w", err)
	}
	attemptByFileID := make(map[string]*model.FileUploadAttempt, len(attempts))
	for i := range attempts {
		attemptByFileID[attempts[i].FileID] = &attempts[i]
	}

	// Lock the file rows after the attempt rows. Cleanup uses the same order, so a
	// send racing expiry either claims the file first or observes that it disappeared
	// and aborts the message transaction; it can never commit a reference to a file
	// that cleanup removed in between resolution and message creation.
	var files []model.UploadFile
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ? AND is_temporary = ? AND created_by = ?", fileIDs, true, ownerID).
		Find(&files).Error; err != nil {
		return fmt.Errorf("load temporary files for claim: %w", err)
	}
	fileByID := make(map[string]*model.UploadFile, len(files))
	for i := range files {
		fileByID[files[i].ID] = &files[i]
	}
	for _, fileID := range fileIDs {
		file, ok := fileByID[fileID]
		if !ok {
			return fmt.Errorf("temporary file %s is no longer available", fileID)
		}
		attempt := attemptByFileID[fileID]
		if attempt == nil {
			// Files created before attempt tracking remain valid and intentionally keep
			// their existing retention semantics.
			continue
		}
		switch attempt.Status {
		case model.FileUploadAttemptClaimed:
			if !file.Used {
				return fmt.Errorf("temporary file %s has a claimed attempt but is not marked used", fileID)
			}
			continue
		case model.FileUploadAttemptCompleted:
			if !file.Used {
				result := tx.WithContext(ctx).Model(&model.UploadFile{}).
					Where("id = ? AND is_temporary = ? AND created_by = ? AND used = ?", fileID, true, ownerID, false).
					Updates(map[string]interface{}{"used": true, "used_by": usedBy, "used_at": gorm.Expr("NOW()")})
				if result.Error != nil {
					return fmt.Errorf("claim temporary file %s: %w", fileID, result.Error)
				}
				if result.RowsAffected == 0 {
					return fmt.Errorf("temporary file %s is no longer available", fileID)
				}
			}
			if err := tx.WithContext(ctx).Model(&model.FileUploadAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]interface{}{
				"status":     model.FileUploadAttemptClaimed,
				"updated_at": time.Now().UTC(),
			}).Error; err != nil {
				return fmt.Errorf("mark temporary upload attempt claimed: %w", err)
			}
		default:
			return fmt.Errorf("temporary file %s is not available for chat", attempt.FileID)
		}
	}
	return nil
}

type FileUploadAttemptCleanupProcessor struct {
	db  *gorm.DB
	now func() time.Time
}

func NewFileUploadAttemptCleanupProcessor(db *gorm.DB) *FileUploadAttemptCleanupProcessor {
	return &FileUploadAttemptCleanupProcessor{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (p *FileUploadAttemptCleanupProcessor) ProcessExpired(ctx context.Context, limit int) (int, error) {
	if p == nil || p.db == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	var attempts []model.FileUploadAttempt
	if err := p.db.WithContext(ctx).
		Where("status IN ? AND expires_at <= ?", []model.FileUploadAttemptStatus{model.FileUploadAttemptPending, model.FileUploadAttemptCompleted, model.FileUploadAttemptClaimed, model.FileUploadAttemptCancelled}, p.now()).
		Order("expires_at ASC, created_at ASC").Limit(limit).Find(&attempts).Error; err != nil {
		return 0, fmt.Errorf("list expired temporary upload attempts: %w", err)
	}
	count := 0
	for i := range attempts {
		if ctx.Err() != nil {
			break
		}
		removed, err := p.processOne(ctx, attempts[i].ID)
		if err != nil {
			logger.Warn("Temporary upload attempt cleanup failed", "attempt_id", attempts[i].ID, "error", err.Error())
			continue
		}
		if removed {
			count++
		}
	}
	return count, nil
}

func (p *FileUploadAttemptCleanupProcessor) processOne(ctx context.Context, attemptID string) (bool, error) {
	now := p.now()
	removed := false
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt model.FileUploadAttempt
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&attempt, "id = ?", attemptID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if attempt.ExpiresAt.After(now) {
			return nil
		}
		if attempt.Status == model.FileUploadAttemptClaimed {
			if err := tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error; err != nil {
				return err
			}
			removed = true
			return nil
		}
		objectKey := attempt.ObjectKey
		if attempt.FileID != "" {
			var file model.UploadFile
			err := tx.WithContext(ctx).Where("id = ?", attempt.FileID).First(&file).Error
			switch {
			case err == nil && (file.Used || !file.IsTemporary || file.CreatedBy != attempt.OwnerID):
				if err := tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error; err != nil {
					return err
				}
				removed = true
				return nil
			case err == nil && file.IsTemporary:
				// A mismatched object key means the lifecycle row is malformed. Preserve
				// the upload rather than risk deleting an object named by another record.
				if objectKey != "" && objectKey != file.Key {
					if err := tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error; err != nil {
						return err
					}
					removed = true
					return nil
				}
				objectKey = file.Key
				if err := tx.Where("id = ?", file.ID).Delete(&model.UploadFile{}).Error; err != nil {
					return fmt.Errorf("delete expired temporary file record: %w", err)
				}
			case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
				return err
			}
		}
		if objectKey != "" {
			pending := model.NewFileObjectDeletion(objectKey, now)
			if err := tx.Create(pending).Error; err != nil {
				return fmt.Errorf("record expired temporary object deletion: %w", err)
			}
		}
		if err := tx.Delete(&model.FileUploadAttempt{}, "id = ?", attempt.ID).Error; err != nil {
			return err
		}
		removed = true
		return nil
	})
	return removed, err
}

type FileUploadAttemptCleanupTask struct{}

var _ pkgscheduler.ScheduledTask = (*FileUploadAttemptCleanupTask)(nil)

func (*FileUploadAttemptCleanupTask) TaskType() string        { return temporaryUploadCleanupTaskType }
func (*FileUploadAttemptCleanupTask) CronSpec() string        { return "" }
func (*FileUploadAttemptCleanupTask) Interval() time.Duration { return 5 * time.Minute }
func (*FileUploadAttemptCleanupTask) Payload() []byte         { return nil }
func (*FileUploadAttemptCleanupTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(0), asynq.Timeout(2 * time.Minute), asynq.Unique(4 * time.Minute)}
}

type FileUploadAttemptCleanupTaskHandler struct {
	processor *FileUploadAttemptCleanupProcessor
}

func NewFileUploadAttemptCleanupTaskHandler(processor *FileUploadAttemptCleanupProcessor) *FileUploadAttemptCleanupTaskHandler {
	return &FileUploadAttemptCleanupTaskHandler{processor: processor}
}

func (h *FileUploadAttemptCleanupTaskHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.processor == nil {
		return nil
	}
	_, err := h.processor.ProcessExpired(ctx, 100)
	return err
}
