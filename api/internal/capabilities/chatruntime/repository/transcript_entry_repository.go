package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/gorm"
)

// transcriptEntryRepository stores the append-only protocol messages of a turn.
//
// payload is the original message and is never rewritten; preview is the
// effective form and may be replaced by budget pruning. Every read path that
// feeds the model context or a list must use the preview projection types so a
// query never detoasts payload.
type transcriptEntryRepository struct {
	db *gorm.DB
}

func NewTranscriptEntryRepository(db *gorm.DB) TranscriptEntryRepository {
	return &transcriptEntryRepository{db: db}
}

func (r *transcriptEntryRepository) Append(ctx context.Context, entries []*runtimemodel.TranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i, entry := range entries {
		if entry == nil {
			return fmt.Errorf("append transcript entries: entry %d is nil", i)
		}
		if entry.ID == uuid.Nil {
			return fmt.Errorf("append transcript entries: entry %d has an empty id", i)
		}
		if entry.MessageID == uuid.Nil || entry.ConversationID == uuid.Nil || entry.OrganizationID == uuid.Nil {
			return fmt.Errorf("append transcript entries: entry %s is missing scope ids", entry.ID)
		}
		if entry.Kind == "" {
			return fmt.Errorf("append transcript entries: entry %s has an empty kind", entry.ID)
		}
		if len(entry.Payload) == 0 {
			return fmt.Errorf("append transcript entries: entry %s has an empty payload", entry.ID)
		}
		if len(entry.Preview) == 0 {
			entry.Preview = entry.Payload
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = now
		}
	}
	if err := r.db.WithContext(ctx).Create(&entries).Error; err != nil {
		return fmt.Errorf("append transcript entries: %w", err)
	}
	return nil
}

func (r *transcriptEntryRepository) NextPositions(ctx context.Context, conversationID, messageID uuid.UUID) (int, int64, error) {
	seq, err := r.maxSeq(ctx, messageID)
	if err != nil {
		return 0, 0, err
	}
	cseq, err := r.MaxCSeq(ctx, conversationID)
	if err != nil {
		return 0, 0, err
	}
	return seq + 1, cseq + 1, nil
}

func (r *transcriptEntryRepository) maxSeq(ctx context.Context, messageID uuid.UUID) (int, error) {
	var maxSeq int64
	err := r.db.WithContext(ctx).
		Model(&runtimemodel.TranscriptEntry{}).
		Select("COALESCE(MAX(seq), 0)").
		Where("message_id = ?", messageID).
		Scan(&maxSeq).Error
	if err != nil {
		return 0, fmt.Errorf("read transcript entry max seq: %w", err)
	}
	return int(maxSeq), nil
}

func (r *transcriptEntryRepository) MaxCSeq(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var maxCSeq int64
	err := r.db.WithContext(ctx).
		Model(&runtimemodel.TranscriptEntry{}).
		Select("COALESCE(MAX(cseq), 0)").
		Where("conversation_id = ?", conversationID).
		Scan(&maxCSeq).Error
	if err != nil {
		return 0, fmt.Errorf("read transcript entry max cseq: %w", err)
	}
	return maxCSeq, nil
}

// previewColumns is the explicit whitelist for every projection/list read.
// payload deliberately stays out of it: reading it would detoast the largest
// column in the table on each message list and each context projection.
const transcriptEntryPreviewColumns = "id, message_id, cseq, seq, kind, preview, byte_size"

func (r *transcriptEntryRepository) ListPreviewsByConversation(ctx context.Context, conversationID uuid.UUID, afterCSeq int64, limit int) ([]*runtimemodel.TranscriptEntryPreview, error) {
	query := r.db.WithContext(ctx).
		Model(&runtimemodel.TranscriptEntry{}).
		Select(transcriptEntryPreviewColumns).
		Where("conversation_id = ? AND cseq > ?", conversationID, afterCSeq).
		Order("cseq ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var previews []*runtimemodel.TranscriptEntryPreview
	if err := query.Find(&previews).Error; err != nil {
		return nil, fmt.Errorf("list transcript entry previews: %w", err)
	}
	return previews, nil
}

func (r *transcriptEntryRepository) ListPreviewsByMessage(ctx context.Context, messageID uuid.UUID) ([]*runtimemodel.TranscriptEntryPreview, error) {
	var previews []*runtimemodel.TranscriptEntryPreview
	err := r.db.WithContext(ctx).
		Model(&runtimemodel.TranscriptEntry{}).
		Select(transcriptEntryPreviewColumns).
		Where("message_id = ?", messageID).
		Order("seq ASC").
		Find(&previews).Error
	if err != nil {
		return nil, fmt.Errorf("list message transcript entry previews: %w", err)
	}
	return previews, nil
}

// ListForCopy reads whole entries, the payload included, for branching a
// conversation. It is the only read path that selects payload, and it exists
// because a branch must own its own facts instead of referencing the source.
func (r *transcriptEntryRepository) ListForCopy(ctx context.Context, conversationID uuid.UUID, throughCSeq int64) ([]*runtimemodel.TranscriptEntry, error) {
	var entries []*runtimemodel.TranscriptEntry
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND cseq <= ?", conversationID, throughCSeq).
		Order("cseq ASC").
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("list conversation transcript entries for copy: %w", err)
	}
	return entries, nil
}

func (r *transcriptEntryRepository) GetPayload(ctx context.Context, id, organizationID uuid.UUID) (*runtimemodel.TranscriptEntry, error) {
	var entry runtimemodel.TranscriptEntry
	err := r.db.WithContext(ctx).
		Select("id, message_id, conversation_id, organization_id, cseq, seq, kind, payload, byte_size, created_at").
		Where("id = ? AND organization_id = ?", id, organizationID).
		Take(&entry).Error
	if err != nil {
		return nil, wrapNotFound(err, "transcript entry")
	}
	return &entry, nil
}

// UpdatePreview replaces the derived effective form. It reads only the narrow
// columns it needs so the payload column is never loaded.
func (r *transcriptEntryRepository) UpdatePreview(ctx context.Context, id uuid.UUID, preview map[string]interface{}, byteSize int) error {
	if len(preview) == 0 {
		return fmt.Errorf("update transcript entry preview: empty preview")
	}
	var entry runtimemodel.TranscriptEntry
	err := r.db.WithContext(ctx).
		Select("id, preview, byte_size").
		Where("id = ?", id).
		Take(&entry).Error
	if err != nil {
		return wrapNotFound(err, "transcript entry")
	}
	entry.Preview = preview
	entry.ByteSize = byteSize
	if err := r.db.WithContext(ctx).
		Model(&runtimemodel.TranscriptEntry{}).
		Select("preview", "byte_size").
		Where("id = ?", id).
		Updates(&entry).Error; err != nil {
		return fmt.Errorf("update transcript entry preview: %w", err)
	}
	return nil
}

// DeleteByMessage hard-deletes the retained facts of one deleted turn. A turn
// can only be removed as a whole (leaf deletion), so this never truncates a
// context that is still in use.
func (r *transcriptEntryRepository) DeleteByMessage(ctx context.Context, messageID uuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Delete(&runtimemodel.TranscriptEntry{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete message transcript entries: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteByConversation hard-deletes the retained facts of a conversation. It is
// the retention path; it is never part of the agent request path.
func (r *transcriptEntryRepository) DeleteByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Delete(&runtimemodel.TranscriptEntry{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete conversation transcript entries: %w", result.Error)
	}
	return result.RowsAffected, nil
}
