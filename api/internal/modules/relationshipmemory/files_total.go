package relationshipmemory

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/gorm"
)

// checkFilesTotal rejects a write that would take a person's memory files past
// agentmemory.MaxMemoryChars together (ADR 0017). The file being written counts
// with its new content; a DigitalHuman's notes on the person do not count.
func (s *Service) checkFilesTotal(ctx context.Context, tx *gorm.DB, relationshipID uuid.UUID, key, content string) error {
	var items []UserMemoryItem
	if err := tx.Where("relationship_id = ? AND memory_key <> ? AND memory_key NOT LIKE ?", relationshipID, key, NoteKeyPrefix+"%").Find(&items).Error; err != nil {
		return err
	}
	total := utf8.RuneCountInString(content)
	for _, item := range items {
		decoded, err := s.codec.Decode(ctx, EncodedContent{Payload: item.ContentPayload, Encoding: item.ContentEncoding, KeyID: item.ContentKeyID})
		if err != nil {
			return err
		}
		total += utf8.RuneCountInString(decoded)
	}
	if total > agentmemory.MaxMemoryChars {
		return fmt.Errorf("%w: memory would hold %d characters, over its limit of %d", ErrCapacityExceeded, total, agentmemory.MaxMemoryChars)
	}
	return nil
}

// MemoryFiles returns a person's own memory files with one consumer, decoded,
// in name order. A DigitalHuman's notes on the person are left out.
func (s *Service) MemoryFiles(ctx context.Context, relationshipID uuid.UUID) ([]UserMemoryItem, error) {
	var items []UserMemoryItem
	if err := s.db.WithContext(ctx).Where("relationship_id = ? AND memory_key NOT LIKE ?", relationshipID, NoteKeyPrefix+"%").Order("memory_key ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	for index := range items {
		decoded, err := s.codec.Decode(ctx, EncodedContent{Payload: items[index].ContentPayload, Encoding: items[index].ContentEncoding, KeyID: items[index].ContentKeyID})
		if err != nil {
			return nil, err
		}
		items[index].ContentPayload = decoded
	}
	return items, nil
}
