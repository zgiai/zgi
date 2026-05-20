package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"go.uber.org/zap"
)

// DocumentIndexingPayload represents the payload for document indexing task
type DocumentIndexingPayload struct {
	DocumentID string `json:"document_id"`
	// DatasetID is redundant for processing but useful for observability in queue inspection
	DatasetID string `json:"dataset_id"`
}

// Task types
const (
	TypeDocumentIndexing = "document:indexing"
)

// NewDocumentIndexingTask creates a new document indexing task with environment prefix support
func NewDocumentIndexingTask(document *dataset_model.Document, taskManager *queue.TaskManager) (*asynq.Task, error) {
	payload := DocumentIndexingPayload{
		DocumentID: document.ID,
		DatasetID:  document.DatasetID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	taskType := TypeDocumentIndexing
	if taskManager != nil {
		taskType = taskManager.GetTaskTypeWithPrefix(taskType)
	}

	// KEY: Asynq task timeout must exceed the indexing context timeout
	// to ensure error handling has time to update document status.
	indexingTimeoutMinutes := 60 // default 1 hour
	if config.GlobalConfig != nil && config.GlobalConfig.VectorStore.IndexingTimeout > 0 {
		indexingTimeoutMinutes = config.GlobalConfig.VectorStore.IndexingTimeout
	}
	asynqTimeout := time.Duration(indexingTimeoutMinutes+5) * time.Minute

	return asynq.NewTask(taskType, payloadBytes,
		asynq.Timeout(asynqTimeout),
		asynq.MaxRetry(0),
	), nil
}

// HandleDocumentIndexingTask handles the document indexing task
func HandleDocumentIndexingTask(documentService interface {
	GetDocumentByID(ctx context.Context, documentID string) (*dataset_model.Document, error)
	RunDocumentIndexing(ctx context.Context, document *dataset_model.Document) error
	UpdateDocumentError(ctx context.Context, documentID string, errorMsg string, stoppedAt *time.Time) error
}) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		ctx = logger.WithFields(ctx,
			zap.String("task_type", t.Type()),
			zap.Int("payload_size", len(t.Payload())),
		)
		var p DocumentIndexingPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			logger.WarnContext(ctx, "failed to decode document indexing task payload", err)
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}
		ctx = logger.WithFields(ctx,
			zap.String("document_id", p.DocumentID),
			zap.String("dataset_id", p.DatasetID),
		)
		logger.DebugContext(ctx, "document indexing task started")

		// Get document by ID
		document, err := documentService.GetDocumentByID(ctx, p.DocumentID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get document for indexing task", err)
			return fmt.Errorf("failed to get document: %v: %w", err, asynq.SkipRetry)
		}

		// Panic recovery to prevent worker crash and update document status
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic in document indexing: %v", r)
				logger.CriticalContext(ctx, "panic recovered in document indexing", err)

				stopTime := time.Now()
				// KEY: Use independent context with timeout to ensure error status
				// is written even when the original ctx is cancelled/expired.
				if document != nil {
					errCtx, errCancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer errCancel()
					_ = documentService.UpdateDocumentError(errCtx, document.ID, fmt.Sprintf("Internal Error: %v", r), &stopTime)
				}
			}
		}()

		if document == nil {
			return fmt.Errorf("document not found: %v: %w", p.DocumentID, asynq.SkipRetry)
		}

		if document.IndexingStatus == dataset_model.DocumentStatusCompleted {
			logger.Info("Skip indexing for completed document", map[string]interface{}{
				"document_id": document.ID,
				"dataset_id":  document.DatasetID,
			})
			return nil
		}

		// Mutex guard: skip if document is actively being processed
		if document.IndexingStatus == dataset_model.DocumentStatusParsing ||
			document.IndexingStatus == dataset_model.DocumentStatusCleaning ||
			document.IndexingStatus == dataset_model.DocumentStatusSplitting ||
			document.IndexingStatus == dataset_model.DocumentStatusIndexing {

			// If it has been stuck in these states for more than 1 hour, allow retry
			if document.UpdatedAt.Before(time.Now().Add(-1 * time.Hour)) {
				logger.Warn("Document stuck in processing state, allowing retry", map[string]interface{}{
					"document_id": document.ID,
					"status":      document.IndexingStatus,
					"updated_at":  document.UpdatedAt,
				})
			} else {
				logger.Info("Skip duplicate indexing; document is processing", map[string]interface{}{
					"document_id": document.ID,
					"dataset_id":  document.DatasetID,
					"status":      document.IndexingStatus,
				})
				return nil
			}
		}

		// Run the indexing process
		if err := documentService.RunDocumentIndexing(ctx, document); err != nil {
			// KEY: Use independent context for error status update.
			// The original ctx may have been cancelled by context timeout,
			// which would cause UpdateDocumentError to silently fail,
			// leaving the document stuck in 'indexing' state forever.
			errCtx, errCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer errCancel()

			stopTime := time.Now()
			if updateErr := documentService.UpdateDocumentError(errCtx, document.ID, err.Error(), &stopTime); updateErr != nil {
				logger.Error("Failed to update document error status", updateErr)
			}

			logger.Error("Failed to index document", err)
			// Return the error with SkipRetry to prevent retries while still updating document error status
			return fmt.Errorf("failed to index document: %v: %w", err, asynq.SkipRetry)
		}

		logger.Info("Document indexing completed", map[string]interface{}{
			"document_id": document.ID,
			"dataset_id":  document.DatasetID,
		})

		return nil
	}
}
