package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultImageRuntimeTaskPollEvery = 15 * time.Second
	defaultImageRuntimeTaskPollBatch = 50
	defaultImageRuntimeTaskMaxAge    = 10 * time.Minute
)

type TaskPoller struct {
	db    *gorm.DB
	tasks *imageTaskRepository
}

func NewTaskPoller(db *gorm.DB) *TaskPoller {
	if db == nil {
		return nil
	}
	return &TaskPoller{db: db, tasks: newImageTaskRepository(db)}
}

func (p *TaskPoller) Start(ctx context.Context) {
	if p == nil || p.tasks == nil {
		log.Println("[image runtime task poller] skipped: missing dependencies")
		return
	}
	ticker := time.NewTicker(defaultImageRuntimeTaskPollEvery)
	defer ticker.Stop()

	log.Printf("[image runtime task poller] started, sweep_every=%s batch=%d", defaultImageRuntimeTaskPollEvery, defaultImageRuntimeTaskPollBatch)
	p.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[image runtime task poller] stopped")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *TaskPoller) poll(ctx context.Context) {
	expiredBefore := time.Now().UTC().Add(-defaultImageRuntimeTaskMaxAge)
	records, err := p.tasks.listExpiredActive(ctx, expiredBefore, defaultImageRuntimeTaskPollBatch)
	if err != nil {
		log.Printf("[image runtime task poller] sweep failed: %v", err)
		return
	}
	for i := range records {
		now := time.Now().UTC()
		records[i].Status = "failed"
		records[i].ErrorMessage = ErrTaskTimeout.Error()
		records[i].UpdatedAt = now
		records[i].CompletedAt = &now
		if err := p.tasks.save(ctx, &records[i]); err != nil {
			log.Printf("[image runtime task poller] task_id=%s failed: %v", records[i].TaskID, err)
			continue
		}
		if err := p.markExpiredTaskMessageFailed(ctx, records[i], ErrTaskTimeout.Error(), now); err != nil {
			log.Printf("[image runtime task poller] task_id=%s message update failed: %v", records[i].TaskID, err)
		}
	}
}

func (p *TaskPoller) markExpiredTaskMessageFailed(ctx context.Context, record imageTaskRecord, errorMessage string, now time.Time) error {
	if p == nil || p.db == nil {
		return nil
	}
	messageID, messageErr := uuid.Parse(strings.TrimSpace(record.MessageID))
	conversationID, conversationErr := uuid.Parse(strings.TrimSpace(record.ConversationID))
	if messageErr != nil || conversationErr != nil {
		return nil
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation runtimemodel.Conversation
		if err := tx.
			Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", conversationID, record.OrganizationID, record.AccountID).
			Take(&conversation).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		var message runtimemodel.Message
		if err := tx.
			Where("id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversation.ID).
			Take(&message).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if message.Status == runtimemodel.MessageStatusCompleted || message.Status == runtimemodel.MessageStatusStopped {
			return nil
		}
		if metadataTaskID := metadataString(message.Metadata, "image_task_id"); metadataTaskID != "" && metadataTaskID != record.TaskID {
			return nil
		}

		metadata := cloneMetadata(message.Metadata)
		imageGeneration := metadataObject(metadata, "image_generation")
		imageGeneration["status"] = "failed"
		if _, ok := imageGeneration["files"]; !ok {
			imageGeneration["files"] = []any{}
		}
		metadata["image_generation"] = imageGeneration
		metadata["image_task_id"] = record.TaskID
		metadata["image_task_status"] = "failed"
		metadata["image_task_error"] = errorMessage
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return err
		}

		if err := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversation.ID).
			Updates(map[string]interface{}{
				"status":               runtimemodel.MessageStatusError,
				"error":                errorMessage,
				"metadata":             datatypes.JSON(metadataJSON),
				"runtime_run_id":       nil,
				"runtime_heartbeat_at": nil,
				"updated_at":           now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&runtimemodel.Conversation{}).
			Where("id = ? AND organization_id = ? AND account_id = ? AND active_message_id = ? AND deleted_at IS NULL", conversationID, record.OrganizationID, record.AccountID, messageID).
			Updates(map[string]interface{}{
				"runtime_status":    runtimemodel.ConversationRuntimeStatusIdle,
				"active_message_id": nil,
				"updated_at":        now,
			}).Error
	})
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func metadataObject(metadata map[string]interface{}, key string) map[string]interface{} {
	value, ok := metadata[key]
	if !ok || value == nil {
		return map[string]interface{}{}
	}
	if object, ok := value.(map[string]interface{}); ok {
		cloned := make(map[string]interface{}, len(object))
		for objectKey, objectValue := range object {
			cloned[objectKey] = objectValue
		}
		return cloned
	}
	var object map[string]interface{}
	raw, err := json.Marshal(value)
	if err == nil {
		_ = json.Unmarshal(raw, &object)
	}
	if object == nil {
		return map[string]interface{}{}
	}
	return object
}

func metadataString(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
