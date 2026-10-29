package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/gorm"
)

// MessageRecordRepository stores the per-message lists a turn appends to while
// it runs, one row per entry (ADR 0039).
type MessageRecordRepository interface {
	// Upsert inserts a record, or merges its payload into the record with the
	// same message, kind and record ID: keys in the new payload replace the
	// stored ones, and the others are kept.
	Upsert(ctx context.Context, record *runtimemodel.MessageRecord) error
	// ListByMessages returns the payloads of one kind for each message, in
	// sequence order.
	ListByMessages(ctx context.Context, kind string, messageIDs []uuid.UUID) (map[uuid.UUID][]map[string]interface{}, error)
	Count(ctx context.Context, messageID uuid.UUID, kind string) (int64, error)
	SaveSystemPrompt(ctx context.Context, hash, content string) error
	SystemPrompts(ctx context.Context, hashes []string) (map[string]string, error)
}

type messageRecordRepository struct {
	db *gorm.DB
}

func NewMessageRecordRepository(db *gorm.DB) MessageRecordRepository {
	return &messageRecordRepository{db: db}
}

func (r *messageRecordRepository) Upsert(ctx context.Context, record *runtimemodel.MessageRecord) error {
	if record == nil || record.MessageID == uuid.Nil || record.Kind == "" || record.RecordID == "" {
		return fmt.Errorf("upsert message record: message, kind and record id are required")
	}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("encode message record payload: %w", err)
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO chat_runtime_message_records
			(id, message_id, organization_id, kind, record_id, sequence, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?)
		ON CONFLICT (message_id, kind, record_id) DO UPDATE SET
			payload = chat_runtime_message_records.payload || EXCLUDED.payload,
			sequence = CASE WHEN EXCLUDED.sequence > 0 THEN EXCLUDED.sequence ELSE chat_runtime_message_records.sequence END,
			updated_at = EXCLUDED.updated_at`,
		record.ID, record.MessageID, record.OrganizationID, record.Kind, record.RecordID, record.Sequence, string(payload), now, now,
	).Error
}

func (r *messageRecordRepository) ListByMessages(ctx context.Context, kind string, messageIDs []uuid.UUID) (map[uuid.UUID][]map[string]interface{}, error) {
	out := make(map[uuid.UUID][]map[string]interface{}, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}
	var records []runtimemodel.MessageRecord
	if err := r.db.WithContext(ctx).
		Where("kind = ? AND message_id IN ?", kind, messageIDs).
		Order("message_id, sequence, created_at, id").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list message records: %w", err)
	}
	for _, record := range records {
		out[record.MessageID] = append(out[record.MessageID], record.Payload)
	}
	return out, nil
}

func (r *messageRecordRepository) Count(ctx context.Context, messageID uuid.UUID, kind string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&runtimemodel.MessageRecord{}).
		Where("message_id = ? AND kind = ?", messageID, kind).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count message records: %w", err)
	}
	return count, nil
}

func (r *messageRecordRepository) SaveSystemPrompt(ctx context.Context, hash, content string) error {
	if hash == "" {
		return nil
	}
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO chat_runtime_system_prompts (hash, content, created_at) VALUES (?, ?, ?) ON CONFLICT (hash) DO NOTHING`,
		hash, content, time.Now().UTC(),
	).Error
}

func (r *messageRecordRepository) SystemPrompts(ctx context.Context, hashes []string) (map[string]string, error) {
	out := make(map[string]string, len(hashes))
	if len(hashes) == 0 {
		return out, nil
	}
	var prompts []runtimemodel.SystemPrompt
	if err := r.db.WithContext(ctx).Where("hash IN ?", hashes).Find(&prompts).Error; err != nil {
		return nil, fmt.Errorf("load system prompts: %w", err)
	}
	for _, prompt := range prompts {
		out[prompt.Hash] = prompt.Content
	}
	return out, nil
}
