package relationshipmemory

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/gorm"
)

// checkFilesTotal rejects a write that would take a user's Agent memory files
// past agentmemory.MaxMemoryChars together (ADR 0017). The file being written
// counts with its new content.
func (s *Service) checkFilesTotal(ctx context.Context, tx *gorm.DB, relationshipID uuid.UUID, key, content string) error {
	var items []UserMemoryItem
	if err := tx.Where("relationship_id = ? AND memory_key <> ?", relationshipID, key).Find(&items).Error; err != nil {
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
