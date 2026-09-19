package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// AppendForMessage appends a parent-linked batch and advances the owning
// message head in one transaction. It is the canonical writer for new
// transcript data; Append remains for legacy migration/import callers.
func (r *transcriptEntryRepository) AppendForMessage(ctx context.Context, messageID uuid.UUID, expectedHead *uuid.UUID, entries []*runtimemodel.TranscriptEntry) (uuid.UUID, error) {
	if messageID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("append transcript chain: message id is empty")
	}
	if len(entries) == 0 {
		if expectedHead == nil {
			return uuid.Nil, nil
		}
		return *expectedHead, nil
	}
	var newHead uuid.UUID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var message runtimemodel.Message
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, conversation_id, transcript_head_entry_id").
			Where("id = ?", messageID).Take(&message)
		if query.Error != nil {
			return fmt.Errorf("load transcript message head: %w", query.Error)
		}
		var organizationIDRaw string
		if err := tx.Table("chat_runtime_conversations").Where("id = ?", message.ConversationID).Pluck("organization_id", &organizationIDRaw).Error; err != nil {
			return fmt.Errorf("load transcript conversation scope: %w", err)
		}
		organizationID, err := uuid.Parse(organizationIDRaw)
		if err != nil {
			return fmt.Errorf("parse transcript conversation scope: %w", err)
		}
		if !sameUUIDPtr(message.TranscriptHeadEntryID, expectedHead) {
			return fmt.Errorf("transcript head changed for message %s", messageID)
		}
		head := cloneUUIDPtr(expectedHead)
		now := time.Now().UTC()
		for index, entry := range entries {
			if err := validateTranscriptEntry(entry, index); err != nil {
				return err
			}
			if entry.ConversationID != message.ConversationID || entry.OrganizationID != organizationID || entry.MessageID != messageID {
				return fmt.Errorf("append transcript chain: entry %s scope does not match message", entry.ID)
			}
			if !sameUUIDPtr(entry.ParentEntryID, head) {
				return fmt.Errorf("append transcript chain: entry %s parent does not match head", entry.ID)
			}
			if entry.CreatedAt.IsZero() {
				entry.CreatedAt = now.Add(time.Duration(index) * time.Nanosecond)
			}
			if entry.EventID == "" {
				entry.EventID = entry.ID.String()
			}
			if err := tx.Create(entry).Error; err != nil {
				return fmt.Errorf("append transcript chain entry %s: %w", entry.ID, err)
			}
			id := entry.ID
			head = &id
		}
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND transcript_head_entry_id IS NOT DISTINCT FROM ?", messageID, uuidValue(expectedHead)).
			Updates(map[string]interface{}{"transcript_head_entry_id": newUUIDValue(head), "updated_at": now})
		if result.Error != nil {
			return fmt.Errorf("advance transcript message head: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("transcript head changed for message %s", messageID)
		}
		if head != nil {
			newHead = *head
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return newHead, nil
}

func (r *transcriptEntryRepository) AppendSummaryForMessage(ctx context.Context, messageID uuid.UUID, expectedHead, boundary *uuid.UUID, summary, eventID string) (uuid.UUID, error) {
	if messageID == uuid.Nil || boundary == nil || *boundary == uuid.Nil {
		return uuid.Nil, fmt.Errorf("append transcript summary: message and boundary are required")
	}
	if summary == "" {
		return uuid.Nil, fmt.Errorf("append transcript summary: summary is empty")
	}
	var entryID uuid.UUID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var message runtimemodel.Message
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, conversation_id, transcript_head_entry_id").
			Where("id = ?", messageID).Take(&message).Error; err != nil {
			return fmt.Errorf("load transcript message head: %w", err)
		}
		if eventID != "" {
			var existing runtimemodel.TranscriptEntry
			if err := tx.Where("message_id = ? AND event_id = ?", messageID, eventID).Take(&existing).Error; err == nil {
				entryID = existing.ID
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("check transcript summary idempotency: %w", err)
			}
		}
		if !sameUUIDPtr(message.TranscriptHeadEntryID, expectedHead) {
			return fmt.Errorf("transcript head changed for message %s", messageID)
		}
		if expectedHead == nil || *expectedHead == uuid.Nil {
			return fmt.Errorf("append transcript summary: boundary is not reachable from the current head")
		}
		current := *expectedHead
		seen := map[uuid.UUID]struct{}{}
		foundBoundary := false
		for steps := 0; current != uuid.Nil && steps < 10000; steps++ {
			if _, ok := seen[current]; ok {
				return fmt.Errorf("append transcript summary: parent chain contains a cycle")
			}
			seen[current] = struct{}{}
			var row struct {
				ID             uuid.UUID
				ParentEntryID  *uuid.UUID
				ConversationID uuid.UUID
			}
			if err := tx.Table("chat_runtime_message_transcript_entries").Select("id, parent_entry_id, conversation_id").Where("id = ?", current).Take(&row).Error; err != nil {
				return fmt.Errorf("validate transcript summary boundary: %w", err)
			}
			if row.ConversationID != message.ConversationID {
				return fmt.Errorf("append transcript summary: boundary belongs to another conversation")
			}
			if current == *boundary {
				foundBoundary = true
				break
			}
			if row.ParentEntryID == nil {
				break
			}
			current = *row.ParentEntryID
		}
		if !foundBoundary {
			return fmt.Errorf("append transcript summary: boundary is not reachable from the current head")
		}
		var organizationIDRaw string
		if err := tx.Table("chat_runtime_conversations").Where("id = ?", message.ConversationID).Pluck("organization_id", &organizationIDRaw).Error; err != nil {
			return fmt.Errorf("load transcript conversation scope: %w", err)
		}
		organizationID, err := uuid.Parse(organizationIDRaw)
		if err != nil {
			return fmt.Errorf("parse transcript conversation scope: %w", err)
		}
		if eventID == "" {
			eventID = uuid.NewString()
		}
		entryID = uuid.New()
		payload := map[string]interface{}{"role": "user", "content": summary}
		entry := &runtimemodel.TranscriptEntry{
			ID: entryID, MessageID: messageID, ConversationID: message.ConversationID,
			OrganizationID: organizationID, ParentEntryID: cloneUUIDPtr(expectedHead),
			CoversThroughEntryID: cloneUUIDPtr(boundary), EventID: eventID,
			Kind: runtimemodel.TranscriptEntryKindSummary, Preview: payload, Payload: payload,
			ByteSize: len(summary), CreatedAt: time.Now().UTC(),
		}
		if err := tx.Create(entry).Error; err != nil {
			return fmt.Errorf("append transcript summary: %w", err)
		}
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND transcript_head_entry_id IS NOT DISTINCT FROM ?", messageID, uuidValue(expectedHead)).
			Updates(map[string]interface{}{"transcript_head_entry_id": entryID, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return fmt.Errorf("advance transcript summary head: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("transcript head changed for message %s", messageID)
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return entryID, nil
}

func (r *transcriptEntryRepository) SetMessageHead(ctx context.Context, messageID uuid.UUID, head *uuid.UUID) error {
	if messageID == uuid.Nil {
		return fmt.Errorf("set transcript message head: message id is empty")
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ?", messageID).
		Updates(map[string]interface{}{"transcript_head_entry_id": newUUIDValue(head), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return fmt.Errorf("set transcript message head: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("set transcript message head: message %s not found", messageID)
	}
	return nil
}

func (r *transcriptEntryRepository) LoadPreviewChain(ctx context.Context, headID uuid.UUID, stopAt *uuid.UUID, limit int) ([]*runtimemodel.TranscriptEntryPreview, error) {
	if headID == uuid.Nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10000
	}
	chain := make([]*runtimemodel.TranscriptEntryPreview, 0, minTranscriptChainLimit(limit))
	seen := make(map[uuid.UUID]struct{}, minTranscriptChainLimit(limit))
	current := headID
	var summaryBoundary *uuid.UUID
	for len(chain) < limit && current != uuid.Nil {
		if _, ok := seen[current]; ok {
			return nil, fmt.Errorf("cycle detected in transcript entry parent chain")
		}
		seen[current] = struct{}{}
		var entry runtimemodel.TranscriptEntryPreview
		if err := r.db.WithContext(ctx).Model(&runtimemodel.TranscriptEntry{}).
			Select("id, message_id, parent_entry_id, covers_through_entry_id, kind, preview, byte_size").
			Where("id = ?", current).Take(&entry).Error; err != nil {
			return nil, fmt.Errorf("load transcript entry chain at %s: %w", current, err)
		}
		chain = append(chain, &entry)
		if entry.Kind == runtimemodel.TranscriptEntryKindSummary && summaryBoundary == nil {
			summaryBoundary = cloneUUIDPtr(entry.CoversThroughEntryID)
		}
		if summaryBoundary != nil && current == *summaryBoundary {
			break
		}
		if stopAt != nil && *stopAt != uuid.Nil && current == *stopAt {
			break
		}
		if entry.ParentEntryID == nil || *entry.ParentEntryID == uuid.Nil {
			break
		}
		current = *entry.ParentEntryID
	}
	if len(chain) >= limit && current != uuid.Nil {
		return nil, fmt.Errorf("transcript entry parent chain exceeds limit %d", limit)
	}
	return chain, nil
}

func (r *transcriptEntryRepository) ListForCopyByHead(ctx context.Context, headID uuid.UUID, stopAt *uuid.UUID) ([]*runtimemodel.TranscriptEntry, error) {
	if headID == uuid.Nil {
		return nil, nil
	}
	chain := make([]*runtimemodel.TranscriptEntry, 0)
	seen := map[uuid.UUID]struct{}{}
	current := headID
	for len(chain) < 10000 && current != uuid.Nil {
		if _, ok := seen[current]; ok {
			return nil, fmt.Errorf("cycle detected in transcript entry parent chain")
		}
		seen[current] = struct{}{}
		var entry runtimemodel.TranscriptEntry
		if err := r.db.WithContext(ctx).Where("id = ?", current).Take(&entry).Error; err != nil {
			return nil, fmt.Errorf("load transcript entry for copy at %s: %w", current, err)
		}
		chain = append(chain, &entry)
		if stopAt != nil && *stopAt != uuid.Nil && current == *stopAt {
			break
		}
		if entry.ParentEntryID == nil || *entry.ParentEntryID == uuid.Nil {
			break
		}
		current = *entry.ParentEntryID
	}
	if len(chain) >= 10000 && current != uuid.Nil {
		return nil, fmt.Errorf("transcript entry parent chain exceeds copy limit")
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain, nil
}

func validateTranscriptEntry(entry *runtimemodel.TranscriptEntry, index int) error {
	if entry == nil {
		return fmt.Errorf("append transcript chain: entry %d is nil", index)
	}
	if entry.ID == uuid.Nil || entry.MessageID == uuid.Nil || entry.ConversationID == uuid.Nil || entry.OrganizationID == uuid.Nil {
		return fmt.Errorf("append transcript chain: entry %d is missing ids", index)
	}
	if entry.Kind == "" || len(entry.Payload) == 0 {
		return fmt.Errorf("append transcript chain: entry %s is missing kind or payload", entry.ID)
	}
	if len(entry.Preview) == 0 {
		entry.Preview = entry.Payload
	}
	return nil
}

func sameUUIDPtr(left, right *uuid.UUID) bool {
	if left == nil || *left == uuid.Nil {
		return right == nil || *right == uuid.Nil
	}
	return right != nil && *right == *left
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copy := *value
	return &copy
}

func uuidValue(value *uuid.UUID) interface{} {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return *value
}

func newUUIDValue(value *uuid.UUID) interface{} {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	return *value
}

func minTranscriptChainLimit(limit int) int {
	if limit < 32 {
		return limit
	}
	return 32
}

// previewColumns is the explicit whitelist for every projection/list read.
// payload deliberately stays out of it: reading it would detoast the largest
// column in the table on each message list and each context projection.
const transcriptEntryPreviewColumns = "id, message_id, parent_entry_id, covers_through_entry_id, kind, preview, byte_size"

func (r *transcriptEntryRepository) ListPreviewsByMessage(ctx context.Context, messageID uuid.UUID) ([]*runtimemodel.TranscriptEntryPreview, error) {
	var message runtimemodel.Message
	if err := r.db.WithContext(ctx).Select("transcript_head_entry_id").Where("id = ?", messageID).Take(&message).Error; err != nil {
		return nil, fmt.Errorf("load transcript message head: %w", err)
	}
	if message.TranscriptHeadEntryID == nil {
		return nil, nil
	}
	chain, err := r.LoadPreviewChain(ctx, *message.TranscriptHeadEntryID, nil, 0)
	if err != nil {
		return nil, err
	}
	previews := make([]*runtimemodel.TranscriptEntryPreview, 0, len(chain))
	for index := len(chain) - 1; index >= 0; index-- {
		entry := chain[index]
		if entry == nil {
			continue
		}
		if entry.MessageID != messageID {
			// A message head reaches through its parent message's entries. The
			// target message's entries are therefore not necessarily the first
			// rows encountered when walking the shared chain from the root.
			// Filter ancestors instead of stopping at the first one.
			continue
		}
		if entry.Kind == runtimemodel.TranscriptEntryKindSummary {
			continue
		}
		previews = append(previews, entry)
	}
	return previews, nil
}

func (r *transcriptEntryRepository) GetPayload(ctx context.Context, id, organizationID uuid.UUID) (*runtimemodel.TranscriptEntry, error) {
	var entry runtimemodel.TranscriptEntry
	err := r.db.WithContext(ctx).
		Select("id, message_id, conversation_id, organization_id, parent_entry_id, covers_through_entry_id, event_id, kind, payload, byte_size, created_at").
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
