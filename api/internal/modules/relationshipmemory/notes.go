package relationshipmemory

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Notes (ADR 0015): a DigitalHuman's notes on an external end user live in
// that person's relationship memory under the notes/ key namespace. The
// DigitalHuman owns them: the model and operators change them, and the person
// may read and delete them but not edit them. Keeping them here gives them
// the history, revert, archive, export, and erasure of relationship memory.
const NoteKeyPrefix = "notes/"

// maxNoteNameChars is how long a note's own name may be inside the key.
var maxNoteNameChars = maxMemoryKeyChars - utf8.RuneCountInString(NoteKeyPrefix)

var noteCategories = map[string]struct{}{
	"fact": {}, "decision": {}, "open_loop": {}, "experience": {}, "reflection": {},
}

// IsNoteKey reports whether a relationship item key is a DigitalHuman note.
func IsNoteKey(key string) bool { return strings.HasPrefix(strings.TrimSpace(key), NoteKeyPrefix) }

// NoteKey is the item key of the note with this name.
func NoteKey(name string) string { return NoteKeyPrefix + strings.TrimSpace(name) }

// NoteName is a note's own name, without the namespace.
func NoteName(key string) string { return strings.TrimPrefix(strings.TrimSpace(key), NoteKeyPrefix) }

// ValidNoteName reports whether a note name fits its key.
func ValidNoteName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.Contains(name, "/") && utf8.RuneCountInString(name) <= maxNoteNameChars
}

// Notes lists a relationship's notes, newest first, with their content.
func (s *Service) Notes(ctx context.Context, relationshipID uuid.UUID) ([]UserMemoryItem, error) {
	var items []UserMemoryItem
	if err := s.db.WithContext(ctx).Where("relationship_id = ? AND memory_key LIKE ?", relationshipID, NoteKeyPrefix+"%").
		Order("updated_at DESC, memory_key ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	for index := range items {
		content, err := s.codec.Decode(ctx, EncodedContent{Payload: items[index].ContentPayload, Encoding: items[index].ContentEncoding, KeyID: items[index].ContentKeyID})
		if err != nil {
			return nil, fmt.Errorf("decode note: %w", err)
		}
		items[index].ContentPayload = content
	}
	return items, nil
}

// ExpireNotes moves notes unchanged since before into their person's archive
// (ADR 0015), so an expired note stops shaping replies but can still be
// found. It handles up to limit notes and reports how many it moved.
func (s *Service) ExpireNotes(ctx context.Context, before time.Time, limit int) (int, error) {
	var expired []UserMemoryItem
	if err := s.db.WithContext(ctx).Where("memory_key LIKE ? AND updated_at < ?", NoteKeyPrefix+"%", before).
		Order("updated_at ASC").Limit(limit).Find(&expired).Error; err != nil {
		return 0, fmt.Errorf("list expired notes: %w", err)
	}
	moved := 0
	for _, item := range expired {
		content, err := s.codec.Decode(ctx, EncodedContent{Payload: item.ContentPayload, Encoding: item.ContentEncoding, KeyID: item.ContentKeyID})
		if err != nil {
			return moved, fmt.Errorf("decode expired note: %w", err)
		}
		metadata := MutationMetadata{ActorType: "system", Source: "note_expiry", SourceKind: SourceManager, Reason: "the note expired; kept in the archive"}
		if _, err := s.AddArchiveEntry(ctx, item.RelationshipID, ArchiveRequest{Content: content, Tags: item.Category, SourceKey: item.MemoryKey, Metadata: metadata}); err != nil {
			return moved, err
		}
		if _, err := s.DeleteItem(ctx, item.RelationshipID, item.MemoryKey, item.Revision, metadata); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

// InstanceNotes returns a DigitalHuman's notes on one person, newest first,
// for the person's read-only view.
func (s *Service) InstanceNotes(ctx context.Context, workspaceID, digitalHumanID, subjectID uuid.UUID) ([]InstanceNote, error) {
	notes := []InstanceNote{}
	if workspaceID == uuid.Nil || digitalHumanID == uuid.Nil || subjectID == uuid.Nil {
		return notes, nil
	}
	relationship, err := s.FindRelationship(ctx, workspaceID, subjectID, ConsumerDigitalHuman, digitalHumanID)
	if err == ErrRelationshipNotFound {
		return notes, nil
	}
	if err != nil {
		return nil, err
	}
	items, err := s.Notes(ctx, relationship.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		notes = append(notes, InstanceNote{
			Key: NoteName(item.MemoryKey), Path: "/user/notes/" + strings.ReplaceAll(NoteName(item.MemoryKey), "/", "_") + ".md",
			Tags: item.Category, Content: item.ContentPayload, Revision: item.Revision, UpdatedAt: item.UpdatedAt,
		})
	}
	return notes, nil
}
