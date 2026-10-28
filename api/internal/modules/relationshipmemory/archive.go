package relationshipmemory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The user-owned archive (ADR 0014): memory a user keeps that is not shown in
// every conversation. It takes what does not fit in the user's files, is
// found by memory_search, and is managed by the user like their other memory.
// Each change is recorded in the version history under the key archive/<id>.

const (
	maxArchiveEntries      = 500
	maxArchiveContentRunes = 2000
	archiveSearchWindow    = 500
	archiveKeyPrefix       = "archive/"
)

type UserMemoryArchiveEntry struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	RelationshipID  uuid.UUID `gorm:"type:uuid;not null;index" json:"relationship_id"`
	Tags            string    `gorm:"type:varchar(64);not null;default:''" json:"tags,omitempty"`
	SourceKey       string    `gorm:"type:varchar(64);not null;default:''" json:"source_key,omitempty"`
	SourceKind      string    `gorm:"type:varchar(32);not null;default:''" json:"source_kind,omitempty"`
	ContentPayload  string    `gorm:"type:text;not null;default:''" json:"content"`
	ContentEncoding string    `gorm:"type:varchar(24);not null;default:'plain'" json:"-"`
	ContentKeyID    *string   `gorm:"type:varchar(128)" json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (UserMemoryArchiveEntry) TableName() string { return "user_memory_archive_entries" }

type ArchiveRequest struct {
	Content   string
	Tags      string
	SourceKey string
	Metadata  MutationMetadata
}

// ArchiveKey is the history key of an archive entry.
func ArchiveKey(id uuid.UUID) string { return archiveKeyPrefix + id.String() }

// AddArchiveEntry keeps content in the user's archive. When the archive is
// full, the oldest entry makes room and its removal is recorded.
func (s *Service) AddArchiveEntry(ctx context.Context, relationshipID uuid.UUID, req ArchiveRequest) (*UserMemoryArchiveEntry, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" || utf8.RuneCountInString(content) > maxArchiveContentRunes {
		return nil, fmt.Errorf("%w: archive content is empty or too long", ErrPolicyRejected)
	}
	if agentmemory.ContainsSensitiveContent(content) {
		return nil, fmt.Errorf("%w: sensitive content cannot be saved", ErrPolicyRejected)
	}
	encoded, err := s.codec.Encode(ctx, content)
	if err != nil {
		return nil, err
	}
	now := s.now()
	entry := UserMemoryArchiveEntry{
		ID: uuid.New(), RelationshipID: relationshipID, Tags: truncateHistoryText(req.Tags, 64), SourceKey: truncateHistoryText(req.SourceKey, 64),
		SourceKind: normalizedSourceKind(req.Metadata.SourceKind), ContentPayload: encoded.Payload, ContentEncoding: encoded.Encoding, ContentKeyID: encoded.KeyID,
		CreatedAt: now, UpdatedAt: now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationship, err := lockActiveRelationship(tx, relationshipID, req.Metadata)
		if err != nil {
			return err
		}
		// The same text from the same source is kept once, so a retried
		// writer does not archive it twice.
		var sameSource []UserMemoryArchiveEntry
		if err := tx.Where("relationship_id = ? AND source_key = ?", relationshipID, entry.SourceKey).Order("created_at DESC").Limit(50).Find(&sameSource).Error; err != nil {
			return err
		}
		for _, existing := range sameSource {
			decoded, err := s.codec.Decode(ctx, EncodedContent{Payload: existing.ContentPayload, Encoding: existing.ContentEncoding, KeyID: existing.ContentKeyID})
			if err == nil && decoded == content {
				entry = existing
				return errArchiveDuplicate
			}
		}
		var count int64
		if err := tx.Model(&UserMemoryArchiveEntry{}).Where("relationship_id = ?", relationshipID).Count(&count).Error; err != nil {
			return err
		}
		if count >= maxArchiveEntries {
			var oldest UserMemoryArchiveEntry
			if err := tx.Where("relationship_id = ?", relationshipID).Order("created_at ASC, id ASC").Take(&oldest).Error; err != nil {
				return err
			}
			removal := req.Metadata
			removal.Reason, removal.Evidence = "the archive is full; the oldest entry made room", ""
			if err := s.recordArchiveVersion(ctx, tx, relationship, removal, VersionDelete, &oldest, nil); err != nil {
				return err
			}
			if err := tx.Delete(&oldest).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		return s.recordArchiveVersion(ctx, tx, relationship, req.Metadata, VersionCreate, nil, &entry)
	})
	if err != nil && !errors.Is(err, errArchiveDuplicate) {
		return nil, err
	}
	entry.ContentPayload = content
	return &entry, nil
}

var errArchiveDuplicate = errors.New("archive already holds this text")

// ArchiveEntries lists a relationship's archive, newest first, decoded.
func (s *Service) ArchiveEntries(ctx context.Context, relationshipID uuid.UUID, limit int) ([]UserMemoryArchiveEntry, error) {
	if limit <= 0 || limit > maxArchiveEntries {
		limit = maxArchiveEntries
	}
	var entries []UserMemoryArchiveEntry
	if err := s.db.WithContext(ctx).Where("relationship_id = ?", relationshipID).Order("created_at DESC, id DESC").Limit(limit).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("list memory archive: %w", err)
	}
	for index := range entries {
		content, err := s.codec.Decode(ctx, EncodedContent{Payload: entries[index].ContentPayload, Encoding: entries[index].ContentEncoding, KeyID: entries[index].ContentKeyID})
		if err != nil {
			return nil, fmt.Errorf("decode memory archive: %w", err)
		}
		entries[index].ContentPayload = content
	}
	return entries, nil
}

// SearchArchive ranks archive entries by how many search terms they contain.
// Terms are the query's words and, for runs of CJK text, their two-character
// pieces, so a question finds entries that share its words.
func (s *Service) SearchArchive(ctx context.Context, relationshipID uuid.UUID, query string, limit int) ([]UserMemoryArchiveEntry, error) {
	terms := archiveSearchTerms(query)
	if len(terms) == 0 || limit <= 0 {
		return []UserMemoryArchiveEntry{}, nil
	}
	entries, err := s.ArchiveEntries(ctx, relationshipID, archiveSearchWindow)
	if err != nil {
		return nil, err
	}
	type scored struct {
		entry UserMemoryArchiveEntry
		score int
	}
	matches := make([]scored, 0, len(entries))
	for _, entry := range entries {
		text := strings.ToLower(entry.ContentPayload + " " + entry.Tags + " " + entry.SourceKey)
		score := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				score += utf8.RuneCountInString(term)
			}
		}
		if score > 0 {
			matches = append(matches, scored{entry, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	out := make([]UserMemoryArchiveEntry, 0, limit)
	for _, match := range matches {
		out = append(out, match.entry)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func archiveSearchTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		if utf8.RuneCountInString(term) >= 2 && !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		add(word)
		runes := []rune(word)
		if len(runes) > 2 && unicode.Is(unicode.Han, runes[0]) {
			for i := 0; i+2 <= len(runes); i++ {
				add(string(runes[i : i+2]))
			}
		}
	}
	return terms
}

// UpdateArchiveEntry replaces an entry's content.
func (s *Service) UpdateArchiveEntry(ctx context.Context, relationshipID, entryID uuid.UUID, content string, metadata MutationMetadata) (*UserMemoryArchiveEntry, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > maxArchiveContentRunes || agentmemory.ContainsSensitiveContent(content) {
		return nil, fmt.Errorf("%w: archive content is empty, too long, or sensitive", ErrPolicyRejected)
	}
	encoded, err := s.codec.Encode(ctx, content)
	if err != nil {
		return nil, err
	}
	var after UserMemoryArchiveEntry
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationship, err := lockActiveRelationship(tx, relationshipID, metadata)
		if err != nil {
			return err
		}
		var before UserMemoryArchiveEntry
		if err := tx.First(&before, "id = ? AND relationship_id = ?", entryID, relationshipID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRelationshipNotFound
			}
			return err
		}
		after = before
		after.ContentPayload, after.ContentEncoding, after.ContentKeyID, after.UpdatedAt = encoded.Payload, encoded.Encoding, encoded.KeyID, s.now()
		if err := tx.Save(&after).Error; err != nil {
			return err
		}
		return s.recordArchiveVersion(ctx, tx, relationship, metadata, VersionUpdate, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	after.ContentPayload = content
	return &after, nil
}

// DeleteArchiveEntry removes one entry.
func (s *Service) DeleteArchiveEntry(ctx context.Context, relationshipID, entryID uuid.UUID, metadata MutationMetadata) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		relationship, err := lockActiveRelationship(tx, relationshipID, metadata)
		if err != nil {
			return err
		}
		var entry UserMemoryArchiveEntry
		if err := tx.First(&entry, "id = ? AND relationship_id = ?", entryID, relationshipID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRelationshipNotFound
			}
			return err
		}
		if err := s.recordArchiveVersion(ctx, tx, relationship, metadata, VersionDelete, &entry, nil); err != nil {
			return err
		}
		return tx.Delete(&entry).Error
	})
}

// lockActiveRelationship locks the relationship an archive change writes to
// and applies the same fences as a file change: memory must be on unless a
// manager acts, automatic capture must be on for an automatic writer, and a
// writer that read memory at an earlier epoch (before it was cleared or
// turned off) is refused before anything is archived.
func lockActiveRelationship(tx *gorm.DB, relationshipID uuid.UUID, meta MutationMetadata) (*UserMemoryRelationship, error) {
	var relationship UserMemoryRelationship
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&relationship, "id = ?", relationshipID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRelationshipNotFound
		}
		return nil, err
	}
	source := normalizedSourceKind(meta.SourceKind)
	if relationship.Status != RelationshipActive || (!relationship.MemoryEnabled && source != SourceManager) ||
		(source == SourceAutomatic && !relationship.AutoCaptureEnabled) {
		return nil, ErrAccessDenied
	}
	if meta.ExpectedMemoryEpoch != nil && relationship.MemoryEpoch != *meta.ExpectedMemoryEpoch {
		return nil, ErrRevisionConflict
	}
	return &relationship, nil
}

// recordArchiveVersion writes an archive change into the version history.
func (s *Service) recordArchiveVersion(ctx context.Context, tx *gorm.DB, relationship *UserMemoryRelationship, meta MutationMetadata, action string, before, after *UserMemoryArchiveEntry) error {
	asItem := func(entry *UserMemoryArchiveEntry) *UserMemoryItem {
		if entry == nil {
			return nil
		}
		return &UserMemoryItem{ContentPayload: entry.ContentPayload, ContentEncoding: entry.ContentEncoding, ContentKeyID: entry.ContentKeyID}
	}
	entry := after
	if entry == nil {
		entry = before
	}
	return s.recordVersion(ctx, tx, relationship, meta, action, ArchiveKey(entry.ID), nil, asItem(before), asItem(after))
}
